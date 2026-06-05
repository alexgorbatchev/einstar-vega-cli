package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/einstar/vega-cli/internal/vega"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App is the main TUI application.
type App struct {
	client *vega.Client
	outDir string
	app    *tview.Application

	tree        *tview.TreeView
	infoView    *tview.TextView
	logView     *tview.TextView
	layout      *tview.Flex

	deviceInfo map[string]interface{}
	projects   map[string]vega.ProjectInfo
	
	// Track nodes for lazy loading
	loadedDirs map[*tview.TreeNode]string
	
	// Track selected nodes for multi-download
	selectedNodes map[*tview.TreeNode]bool

	mu sync.Mutex
}

// NewApp creates a new TUI app.
func NewApp(client *vega.Client, outDir string) *App {
	a := &App{
		client:        client,
		outDir:        outDir,
		app:           tview.NewApplication(),
		tree:          tview.NewTreeView(),
		infoView:      tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true),
		logView:       tview.NewTextView().SetDynamicColors(true).SetMaxLines(1000),
		loadedDirs:    make(map[*tview.TreeNode]string),
		selectedNodes: make(map[*tview.TreeNode]bool),
	}

	a.setupUI()
	return a
}

func (a *App) setupUI() {
	a.tree.SetBorder(true).SetTitle(" Files ")
	a.infoView.SetBorder(true).SetTitle(" Device Info ")
	a.logView.SetBorder(true).SetTitle(" Logs ")

	topFlex := tview.NewFlex().
		AddItem(a.tree, 0, 1, true).
		AddItem(a.infoView, 0, 1, false)

	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topFlex, 0, 3, true).
		AddItem(a.logView, 0, 1, false)

	a.app.SetRoot(a.layout, true).SetFocus(a.tree)

	a.tree.SetSelectedFunc(a.onTreeSelected)
	a.tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case ' ':
				a.toggleSelection()
				return nil
			case 'd':
				a.downloadSelected()
				return nil
			case 'q':
				a.app.Stop()
				return nil
			case 'x':
				a.deleteSelected()
				return nil
			case 'r':
				a.renameSelected()
				return nil
			}
		}
		return event
	})

	fmt.Fprint(a.logView, "INFO: Application started. Press Space to select, 'd' to download, 'x' to delete, 'r' to rename, 'q' to quit.\n")
}

// log writes a message to the log view safely.
func (a *App) log(msg string) {
	txt := fmt.Sprintf("%s %s\n", time.Now().Format("15:04:05"), msg)
	
	// Safe to call from background goroutines
	a.app.QueueUpdateDraw(func() {
		fmt.Fprint(a.logView, txt)
		a.logView.ScrollToEnd()
	})
}

// Run starts the TUI application and loads initial data.
func (a *App) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Fprint(a.logView, "INFO: Connecting to device...\n")
	if err := a.client.Connect(ctx); err != nil {
		fmt.Fprintf(a.logView, "[red]ERROR: Could not connect:[white] %v\n", err)
	} else {
		fmt.Fprint(a.logView, "[green]INFO: Connected successfully.[white]\n")
		go a.loadData()
	}

	return a.app.Run()
}

func (a *App) loadData() {
	ctx := context.Background()
	
	a.log("INFO: Fetching device info...")
	info, err := a.client.GetDeviceInfo(ctx)
	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: fetching device info:[white] %v", err))
	} else {
		a.mu.Lock()
		a.deviceInfo = info
		a.mu.Unlock()
		a.updateDeviceInfo()
	}

	a.log("INFO: Fetching projects...")
	projects, err := a.client.GetProjectsInfo(ctx)
	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: fetching projects:[white] %v", err))
		return
	}
	
	a.mu.Lock()
	a.projects = projects
	
	// Add logs node based on ssdPath if available
	if a.deviceInfo != nil {
		if ssdPath, ok := a.deviceInfo["ssdPath"].(string); ok {
			a.projects["Logs"] = vega.ProjectInfo{
				Path:     filepath.Join(ssdPath, "TX3App/log"),
				DateTime: time.Now().String(),
			}
		}
	}
	a.mu.Unlock()

	a.app.QueueUpdateDraw(func() {
		root := tview.NewTreeNode(" Vega Scanner").
			SetColor(tcell.ColorGreen).
			SetSelectable(false)
		
		// Sort project names
		var projectNames []string
		for name := range a.projects {
			projectNames = append(projectNames, name)
		}
		sort.Strings(projectNames)
		
		for _, name := range projectNames {
			proj := a.projects[name]
			node := tview.NewTreeNode(" " + name + " ").
				SetColor(tcell.ColorYellow).
				SetSelectable(true).
				SetExpanded(false).
				SetReference(proj.Path)
				
			a.loadedDirs[node] = proj.Path
			root.AddChild(node)
		}
		
		a.tree.SetRoot(root).SetCurrentNode(root)
	})
	
	a.log("INFO: Projects loaded.")
}

func (a *App) updateDeviceInfo() {
	a.app.QueueUpdateDraw(func() {
		a.infoView.Clear()
		fmt.Fprintf(a.infoView, "[yellow]Einstar Vega[white]\n\n")
		
		a.mu.Lock()
		defer a.mu.Unlock()
		
		for k, v := range a.deviceInfo {
			fmt.Fprintf(a.infoView, "[blue]%s:[white] %v\n", k, v)
		}
	})
}

// onTreeSelected is called when a tree node is selected (Enter key).
// We use it to expand/collapse directories and lazy load their contents.
func (a *App) onTreeSelected(node *tview.TreeNode) {
	if len(node.GetChildren()) > 0 {
		node.SetExpanded(!node.IsExpanded())
		return
	}

	ref := node.GetReference()
	if ref == nil {
		return
	}

	path, ok := ref.(string)
	if !ok {
		return
	}

	// It's a leaf node without children. Let's try to load it if it's a directory.
	// Since we don't know ahead of time if it's a directory or file, we can try to list its paths.
	// But actually, the API /listFilePaths returns a flat list of ALL files inside a project.
	// Let's modify our logic: when a project node is selected, we fetch all files and build the subtree.

	if isProjectNode(node, a.projects) {
		node.SetColor(tcell.ColorGray)
		go a.loadProjectFiles(node, path)
	} else {
		// Just toggle
		node.SetExpanded(!node.IsExpanded())
	}
}

func getOriginalName(node *tview.TreeNode) string {
	return strings.TrimSuffix(strings.TrimPrefix(node.GetText(), " "), " * ")
}

func isProjectNode(node *tview.TreeNode, projects map[string]vega.ProjectInfo) bool {
	name := getOriginalName(node)
	for pName := range projects {
		if name == pName {
			return true
		}
	}
	return false
}

func (a *App) toggleSelection() {
	node := a.tree.GetCurrentNode()
	if node == nil || node == a.tree.GetRoot() {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	origName := getOriginalName(node)

	if a.selectedNodes[node] {
		delete(a.selectedNodes, node)
		a.updateNodeColor(node)
		node.SetText(" " + origName + " ")
	} else {
		a.selectedNodes[node] = true
		node.SetColor(tcell.ColorAqua) // Mark as selected
		node.SetText(" " + origName + " * ")
	}
}

func (a *App) updateNodeColor(node *tview.TreeNode) {
	if len(node.GetChildren()) > 0 || isProjectNode(node, a.projects) {
		node.SetColor(tcell.ColorYellow) // Dir
	} else {
		node.SetColor(tcell.ColorWhite) // File
	}
}

func (a *App) loadProjectFiles(node *tview.TreeNode, basePath string) {
	ctx := context.Background()
	a.log(fmt.Sprintf("INFO: Fetching files for %s...", getOriginalName(node)))
	
	paths, err := a.client.ListFilePaths(ctx, basePath)
	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: fetching files:[white] %v", err))
		a.app.QueueUpdateDraw(func() { node.SetColor(tcell.ColorYellow) })
		return
	}

	// Reconstruct the tree from flat paths
	a.app.QueueUpdateDraw(func() {
		node.SetColor(tcell.ColorYellow)
		if len(paths) == 0 {
			a.log(fmt.Sprintf("INFO: No files found in %s", getOriginalName(node)))
			return
		}

		buildTree(node, basePath, paths)
		node.SetExpanded(true)
	})
	
	a.log(fmt.Sprintf("INFO: Loaded %d files for %s", len(paths), getOriginalName(node)))
}

// buildTree constructs a tree from a flat list of paths.
func buildTree(root *tview.TreeNode, basePath string, paths []string) {
	// Simple map-based approach to build the tree.
	nodes := make(map[string]*tview.TreeNode)
	nodes[""] = root // root corresponds to basePath

	// Sort paths so we iterate in deterministic alphabetical order
	sort.Strings(paths)

	for _, p := range paths {
		// Remove basePath from p to get relative path
		rel, err := filepath.Rel(basePath, p)
		if err != nil {
			rel = p // Fallback
		}

		parts := strings.Split(filepath.ToSlash(rel), "/")
		currentPath := ""
		
		for i, part := range parts {
			parentPath := currentPath
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = currentPath + "/" + part
			}
			
			if _, exists := nodes[currentPath]; !exists {
				node := tview.NewTreeNode(" " + part + " ").
					SetSelectable(true).
					SetExpanded(false).
					SetReference(p) // absolute path
				
				if i == len(parts)-1 {
					node.SetColor(tcell.ColorWhite) // File
				} else {
					node.SetColor(tcell.ColorYellow) // Dir
				}
				
				nodes[currentPath] = node
				nodes[parentPath].AddChild(node)
			}
		}
	}
}

// downloadSelected triggers downloading the selected file or directory.
func (a *App) downloadSelected() {
	a.mu.Lock()
	var nodesToDownload []*tview.TreeNode
	for node := range a.selectedNodes {
		nodesToDownload = append(nodesToDownload, node)
	}
	a.mu.Unlock()

	// If nothing is explicitly selected, download the currently focused node
	if len(nodesToDownload) == 0 {
		if node := a.tree.GetCurrentNode(); node != nil && node != a.tree.GetRoot() {
			nodesToDownload = append(nodesToDownload, node)
		} else {
			return
		}
	}

	go func() {
		for _, node := range nodesToDownload {
			a.downloadNode(node)
			
			// Unmark after starting download
			a.mu.Lock()
			delete(a.selectedNodes, node)
			a.app.QueueUpdateDraw(func() {
				a.updateNodeColor(node)
			})
			a.mu.Unlock()
		}
	}()
}

func (a *App) downloadNode(node *tview.TreeNode) {
	ref := node.GetReference()
	if ref == nil {
		return
	}

	path, ok := ref.(string)
	if !ok {
		return
	}

	isDir := len(node.GetChildren()) > 0 || isProjectNode(node, a.projects)

	if isDir {
		basePath := a.findBasePath(node)
		if basePath == "" {
			a.log("[red]ERROR: Could not determine base path for download[white]")
			return
		}
		
		var files []string
		a.gatherFiles(node, &files)
		
		if len(files) == 0 {
			if isProjectNode(node, a.projects) {
				ctx := context.Background()
				paths, err := a.client.ListFilePaths(ctx, path)
				if err == nil {
					files = paths
				}
			}
		}

		if len(files) == 0 {
			a.log("[yellow]WARN: No files to download[white]")
			return
		}

			a.log(fmt.Sprintf("INFO: Starting directory download (%d files)...", len(files)))
			for _, f := range files {
				a.downloadSingleFile(f, basePath)
			}
			a.log(fmt.Sprintf("[green]INFO: Directory download complete: %s[white]", getOriginalName(node)))
		} else {
			basePath := a.findBasePath(node)
			a.downloadSingleFile(path, basePath)
		}
	}

func (a *App) gatherFiles(node *tview.TreeNode, files *[]string) {
	if len(node.GetChildren()) == 0 {
		if ref := node.GetReference(); ref != nil {
			if path, ok := ref.(string); ok {
				*files = append(*files, path)
			}
		}
	} else {
		for _, child := range node.GetChildren() {
			a.gatherFiles(child, files)
		}
	}
}

func (a *App) findBasePath(node *tview.TreeNode) string {
	curr := node
	for curr != nil {
		if isProjectNode(curr, a.projects) {
			if ref := curr.GetReference(); ref != nil {
				if path, ok := ref.(string); ok {
					return path
				}
			}
		}
		// Look up parent manually since TreeNode doesn't have GetParent
		// Actually, we can search from root if needed, but it's easier to just pass the project node down.
		// For now, let's trace from root to find the node and its project parent.
		break
	}
	
	// Brute force trace from root
	root := a.tree.GetRoot()
	for _, p := range root.GetChildren() {
		if isNodeInSubtree(p, node) {
			if ref := p.GetReference(); ref != nil {
				if path, ok := ref.(string); ok {
					// We also need the project name to append to outDir
					return path
				}
			}
		}
	}
	
	return ""
}

func isNodeInSubtree(root, target *tview.TreeNode) bool {
	if root == target {
		return true
	}
	for _, child := range root.GetChildren() {
		if isNodeInSubtree(child, target) {
			return true
		}
	}
	return false
}

func (a *App) getProjectNameForNode(node *tview.TreeNode) string {
	root := a.tree.GetRoot()
	for _, p := range root.GetChildren() {
		if isNodeInSubtree(p, node) {
			return getOriginalName(p)
		}
	}
	return "unknown"
}

func (a *App) deleteSelected() {
	a.log("[yellow]WARN: Delete is currently unsupported by the known API endpoints.[white]")
}

func (a *App) renameSelected() {
	a.log("[yellow]WARN: Rename is currently unsupported by the known API endpoints.[white]")
}

func (a *App) downloadSingleFile(absPath, basePath string) {
	ctx := context.Background()
	
	rel, err := filepath.Rel(basePath, absPath)
	if err != nil {
		rel = filepath.Base(absPath)
	}

	// We need the project node to know the project name
	projName := "unknown"
	// Find project name from paths or tree
	// For simplicity, let's assume the basePath matches a project
	for name, proj := range a.projects {
		if proj.Path == basePath {
			projName = name
			break
		}
	}

	destPath := filepath.Join(a.outDir, projName, rel)
	
	a.log(fmt.Sprintf("INFO: Getting size for %s", rel))
	size, err := a.client.GetFileInfo(ctx, absPath)
	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: getting info for %s:[white] %v", rel, err))
		return
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		a.log(fmt.Sprintf("[red]ERROR: creating directory for %s:[white] %v", rel, err))
		return
	}

	f, err := os.Create(destPath)
	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: creating file %s:[white] %v", rel, err))
		return
	}
	defer f.Close()

	a.log(fmt.Sprintf("INFO: Downloading %s (%.2f MB)...", rel, float64(size)/1048576))
	
	lastLog := time.Now()
	err = a.client.DownloadFile(ctx, absPath, size, f, func(downloaded, total int64) {
		if time.Since(lastLog) > 1*time.Second {
			a.app.QueueUpdateDraw(func() {
				// Don't flood logs, just update
				// In a real app we might have a dedicated progress bar
			})
			lastLog = time.Now()
		}
	})

	if err != nil {
		a.log(fmt.Sprintf("[red]ERROR: downloading %s:[white] %v", rel, err))
		return
	}

	a.log(fmt.Sprintf("[green]SUCCESS: Downloaded %s[white]", rel))
}
