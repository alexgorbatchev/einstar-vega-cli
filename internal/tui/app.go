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
	
	// Track file sizes
	nodeSizes map[*tview.TreeNode]uint64
	
	lastTreeWidth int

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
		nodeSizes:     make(map[*tview.TreeNode]uint64),
	}

	a.setupUI()
	return a
}

func (a *App) setupUI() {
	a.tree.SetBorder(true).SetTitle(" Files ").SetTitleAlign(tview.AlignLeft)
	a.infoView.SetBorder(true).SetTitle(" Device Info ").SetTitleAlign(tview.AlignLeft)
	a.logView.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)

	topFlex := tview.NewFlex().
		AddItem(a.tree, 0, 1, true).
		AddItem(a.infoView, 0, 1, false)

	legend := tview.NewTextView().
		SetDynamicColors(true).
		SetText(" [yellow]Space[white] Select  [yellow]Enter[white] Expand  [yellow]d[white] Download  [yellow]r[white] Rename  [yellow]x[white] Delete  [yellow]q[white] Quit")

	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topFlex, 0, 3, true).
		AddItem(a.logView, 0, 1, false).
		AddItem(legend, 1, 0, false)

	a.app.SetRoot(a.layout, true).SetFocus(a.tree)

	a.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		_, _, w, _ := a.tree.GetInnerRect()
		if w != a.lastTreeWidth && w > 0 {
			a.lastTreeWidth = w
			a.realignAllNodes()
		}
		return false
	})

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

	fmt.Fprint(a.logView, "INFO: Application started.\n")
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
	a.mu.Unlock()

	a.app.QueueUpdateDraw(func() {
		root := tview.NewTreeNode("/").
			SetColor(tcell.ColorGreen).
			SetSelectable(false)
		
	// Sort project names
	var projectNames []string
	for name := range a.projects {
		if name == "Logs" { // Skip Logs per user request
			continue
		}
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)
		
		var projectNodes []*tview.TreeNode

		for _, name := range projectNames {
			proj := a.projects[name]
			node := tview.NewTreeNode(" " + name + " ").
				SetColor(tcell.ColorYellow).
				SetSelectable(true).
				SetExpanded(false).
				SetReference(proj.Path)
				
			a.loadedDirs[node] = proj.Path
			root.AddChild(node)
			projectNodes = append(projectNodes, node)
		}
		
		a.tree.SetRoot(root).SetCurrentNode(root)
		
		// Kick off background scan for all projects
		go a.backgroundScanAllProjects(projectNodes)
	})
	
	a.log("INFO: Projects loaded.")
}

func (a *App) updateDeviceInfo() {
	a.app.QueueUpdateDraw(func() {
		a.infoView.Clear()
		fmt.Fprintf(a.infoView, "[yellow]Einstar Vega[white]\n\n")
		
		a.mu.Lock()
		defer a.mu.Unlock()
		
		var regularKeys []string
		var deviceParams interface{}
		var boardInfo interface{}

		for k, v := range a.deviceInfo {
			if k == "deviceParams" {
				deviceParams = v
			} else if k == "boardInfo" {
				boardInfo = v
			} else {
				regularKeys = append(regularKeys, k)
			}
		}

		sort.Strings(regularKeys)

		maxLen := 0
		for _, k := range regularKeys {
			if len(k) > maxLen {
				maxLen = len(k)
			}
		}

		for _, k := range regularKeys {
			val := a.deviceInfo[k]
			if k == "batteryValue" {
				if vF, ok := val.(float64); ok {
					val = fmt.Sprintf("%.0f%%", vF*100)
				}
			}
			fmt.Fprintf(a.infoView, "[blue]%-*s[white] %v\n", maxLen+1, k+":", val)
		}

		if deviceParams != nil {
			fmt.Fprintf(a.infoView, "\n[yellow]Device Params[white]\n")
			if dpStr, ok := deviceParams.(string); ok {
				parts := strings.Split(dpStr, "_")
				var parsedParts []struct{k, v string}
				maxPLen := 0
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					if idx := strings.Index(p, ":"); idx != -1 {
						k := strings.TrimSpace(p[:idx])
						v := strings.TrimSpace(p[idx+1:])
						parsedParts = append(parsedParts, struct{k, v string}{k, v})
						if len(k) > maxPLen {
							maxPLen = len(k)
						}
					} else {
						parsedParts = append(parsedParts, struct{k, v string}{p, ""})
					}
				}
				for _, p := range parsedParts {
					if p.v != "" {
						fmt.Fprintf(a.infoView, "  [blue]%-*s[white] %s\n", maxPLen+1, p.k+":", p.v)
					} else {
						fmt.Fprintf(a.infoView, "  [blue]%s[white]\n", p.k)
					}
				}
			} else {
				fmt.Fprintf(a.infoView, "  %v\n", deviceParams)
			}
		}

		if boardInfo != nil {
			fmt.Fprintf(a.infoView, "\n[yellow]Board Info[white]\n")
			switch val := boardInfo.(type) {
			case map[string]interface{}:
				var keys []string
				maxBLen := 0
				for k := range val {
					keys = append(keys, k)
					if len(k) > maxBLen {
						maxBLen = len(k)
					}
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Fprintf(a.infoView, "  [blue]%-*s[white] %v\n", maxBLen+1, k+":", val[k])
				}
			case map[interface{}]interface{}:
				type kv struct {
					k string
					v interface{}
				}
				var kvs []kv
				maxBLen := 0
				for k, v := range val {
					kStr := fmt.Sprintf("%v", k)
					kvs = append(kvs, kv{kStr, v})
					if len(kStr) > maxBLen {
						maxBLen = len(kStr)
					}
				}
				sort.Slice(kvs, func(i, j int) bool { return kvs[i].k < kvs[j].k })
				for _, pair := range kvs {
					fmt.Fprintf(a.infoView, "  [blue]%-*s[white] %v\n", maxBLen+1, pair.k+":", pair.v)
				}
			default:
				fmt.Fprintf(a.infoView, "  %v\n", boardInfo)
			}
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

	_, ok := ref.(string)
	if !ok {
		return
	}

	// It's a leaf node without children. Let's try to load it if it's a directory.
	// Since we don't know ahead of time if it's a directory or file, we can try to list its paths.
	// But actually, the API /listFilePaths returns a flat list of ALL files inside a project.
	// Let's modify our logic: when a project node is selected, we fetch all files and build the subtree.

	if isProjectNode(node, a.projects) {
		// Just toggle, we already load files in the background!
		node.SetExpanded(!node.IsExpanded())
	} else {
		// Just toggle
		node.SetExpanded(!node.IsExpanded())
	}
}

func getOriginalName(node *tview.TreeNode) string {
	text := node.GetText()
	text = strings.TrimSuffix(text, " <- ")
	text = strings.TrimPrefix(text, " ")
	
	if idx := strings.Index(text, "  "); idx != -1 {
		text = text[:idx]
	}
	
	return strings.TrimRight(text, " ")
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

	if a.selectedNodes[node] {
		delete(a.selectedNodes, node)
		a.updateNodeColor(node)
		a.updateNodeTextLocked(node)
	} else {
		a.selectedNodes[node] = true
		node.SetColor(tcell.ColorAqua) // Mark as selected
		a.updateNodeTextLocked(node)
	}
}

func (a *App) updateNodeColor(node *tview.TreeNode) {
	if len(node.GetChildren()) > 0 || isProjectNode(node, a.projects) {
		node.SetColor(tcell.ColorYellow) // Dir
	} else {
		node.SetColor(tcell.ColorWhite) // File
	}
}

func (a *App) updateNodeText(node *tview.TreeNode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updateNodeTextLocked(node)
}

func (a *App) realignAllNodes() {
	root := a.tree.GetRoot()
	if root != nil {
		a.mu.Lock()
		a.realignNodeRecursive(root)
		a.mu.Unlock()
	}
}

func (a *App) realignNodeRecursive(node *tview.TreeNode) {
	a.updateNodeTextLocked(node)
	for _, child := range node.GetChildren() {
		a.realignNodeRecursive(child)
	}
}

func getParent(root, target *tview.TreeNode) *tview.TreeNode {
	for _, child := range root.GetChildren() {
		if child == target {
			return root
		}
		if p := getParent(child, target); p != nil {
			return p
		}
	}
	return nil
}

func (a *App) updateNodeTextLocked(node *tview.TreeNode) {
	origName := getOriginalName(node)
	isSelected := a.selectedNodes[node]
	size := a.nodeSizes[node]

	sizeStr := ""
	if size > 0 || len(node.GetChildren()) == 0 { // Show 0 B for empty files
		sizeStr = formatSize(size)
	}

	if node == a.tree.GetRoot() {
		node.SetText(origName)
		return
	}

	level := node.GetLevel()
	if level < 1 {
		level = 1
	}

	width := a.lastTreeWidth
	if width <= 0 {
		_, _, width, _ = a.tree.GetInnerRect()
		if width <= 0 {
			if width <= 0 {
				width = 80 // Reasonable default if everything fails
			}
		}
	}

	// Determine exact tview visual offset
	// By default: Level 0 (root) has 0 graphics.
	// Level 1: `├─` = 2 cells.
	// Level 2: `│ ├─` = 4 cells.
	// Level 3: `│ │ ├─` = 6 cells.
	// General formula for tview tree node text X-coordinate (assuming default indent of 2 and root visible):
	// textX = level * 2
	textStartCol := (level * 2) + 1 // +1 for our manual space

	// Ensure the size string itself is a fixed width (e.g. 9 chars for "14.5 MB" or "842 B")
	// so that the numbers align perfectly in a vertical column, regardless of their length.
	if sizeStr != "" {
		sizeWidth := 9
		if len(sizeStr) < sizeWidth {
			sizeStr = strings.Repeat(" ", sizeWidth-len(sizeStr)) + sizeStr
		}
	}

	var rightBlock string
	if isSelected {
		if sizeStr != "" {
			rightBlock = sizeStr + " <- "
		} else {
			rightBlock = "<- "
		}
	} else {
		rightBlock = sizeStr + " "
	}

	// Calculate padding to push rightBlock to the right edge.
	// We no longer need the - 1 here, because rightBlock already includes the trailing space!
	paddingLen := width - textStartCol - len(origName) - len(rightBlock)
	if paddingLen < 2 {
		paddingLen = 2
	}

	padding := strings.Repeat(" ", paddingLen)
	newText := fmt.Sprintf(" %s%s%s", origName, padding, rightBlock)
	node.SetText(newText)
}

func (a *App) backgroundScanAllProjects(projectNodes []*tview.TreeNode) {
	ctx := context.Background()
	
	for _, pNode := range projectNodes {
		ref := pNode.GetReference()
		if ref == nil {
			continue
		}
		basePath := ref.(string)
		
		paths, err := a.client.ListFilePaths(ctx, basePath)
		if err != nil {
			continue
		}

		a.app.QueueUpdateDraw(func() {
			leaves := buildTree(pNode, basePath, paths)
			// Spawn a new goroutine for fetching sizes so we don't block the UI thread
			// and we don't need any channel synchronization!
			go a.fetchFileSizes(leaves)
		})
	}
}

	// buildTree constructs a tree from a flat list of paths.
	func buildTree(root *tview.TreeNode, basePath string, paths []string) []*tview.TreeNode {
		// Simple map-based approach to build the tree.
		nodes := make(map[string]*tview.TreeNode)
		nodes[""] = root // root corresponds to basePath
		var leaves []*tview.TreeNode

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
						leaves = append(leaves, node)
					} else {
						node.SetColor(tcell.ColorYellow) // Dir
					}
					
					nodes[currentPath] = node
					nodes[parentPath].AddChild(node)
				}
			}
		}
		return leaves
	}

func (a *App) propagateSizeUp(node *tview.TreeNode, addedSize uint64) {
	// TreeNode doesn't have GetParent. We must search from root or traverse our own structure.
	// We can recursively walk the tree and update all node sizes.
	// But it's easier to just compute sizes bottom-up in a full tree pass.
}

func (a *App) refreshAllNodeSizes() {
	a.app.QueueUpdateDraw(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		
		root := a.tree.GetRoot()
		if root != nil {
			a.calcNodeSizeLocked(root)
		}
	})
}

func (a *App) calcNodeSizeLocked(node *tview.TreeNode) uint64 {
	children := node.GetChildren()
	
	if len(children) == 0 {
		return a.nodeSizes[node]
	}

	var total uint64
	for _, child := range children {
		total += a.calcNodeSizeLocked(child)
	}

	a.nodeSizes[node] = total
	a.updateNodeTextLocked(node)

	return total
}

func (a *App) fetchFileSizes(nodes []*tview.TreeNode) {
	ctx := context.Background()
	for _, node := range nodes {
		ref := node.GetReference()
		if ref == nil {
			continue
		}
		path, ok := ref.(string)
		if !ok {
			continue
		}

		size, err := a.client.GetFileInfo(ctx, path)
		if err != nil {
			continue
		}
		
		a.mu.Lock()
		a.nodeSizes[node] = size
		a.mu.Unlock()

		a.app.QueueUpdateDraw(func() {
			a.updateNodeText(node)
		})

		// Small sleep to not overwhelm the scanner
		time.Sleep(20 * time.Millisecond)
	}
	
	// Final update for this project
	a.refreshAllNodeSizes()
}

	func formatSize(b uint64) string {
		const unit = 1024
		if b < unit {
			return fmt.Sprintf("%d B", b)
		}
		div, exp := int64(unit), 0
		for n := b / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
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
