package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/alexgorbatchev/einstar-vega-cli/internal/vega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var downloadCmd = &cobra.Command{
	Use:   "download [project_name]",
	Short: "Download projects from the Einstar Vega scanner in headless mode",
	Long: `Download a specific project or all projects from the Einstar Vega scanner directly from the command line in headless mode without launching the TUI.

If no project name is provided, you can use the --all flag to download all available projects.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := viper.GetString("ip")
		if ip == "" {
			return fmt.Errorf("--ip is required (or set VEGA_IP environment variable)")
		}
		port := viper.GetString("port")
		outDir := viper.GetString("output")

		downloadAll, _ := cmd.Flags().GetBool("all")
		if len(args) == 0 && !downloadAll {
			return fmt.Errorf("either a project name must be specified or --all flag must be set")
		}

		client := vega.NewClient(net.JoinHostPort(ip, port))
		ctx := context.Background()

		fmt.Println("Connecting to Einstar Vega scanner...")
		if err := client.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to scanner: %w", err)
		}
		fmt.Println("Successfully connected to scanner.")

		projects, err := client.GetProjectsInfo(ctx)
		if err != nil {
			return fmt.Errorf("fetching projects list: %w", err)
		}

		var targets []string
		if downloadAll {
			for name := range projects {
				targets = append(targets, name)
			}
		} else {
			projName := args[0]
			if _, ok := projects[projName]; !ok {
				fmt.Printf("Error: project %q not found. Available projects:\n", projName)
				for name := range projects {
					fmt.Printf(" - %s\n", name)
				}
				return fmt.Errorf("project not found")
			}
			targets = append(targets, projName)
		}

		for _, name := range targets {
			proj := projects[name]
			fmt.Printf("\nScanning project %q (%s)...\n", name, proj.Path)
			files, err := client.ListFilePaths(ctx, proj.Path)
			if err != nil {
				fmt.Printf("Error listing files for %s: %v\n", name, err)
				continue
			}

			fmt.Printf("Found %d files in project %q. Starting download...\n", len(files), name)
			for i, absPath := range files {
				rel, err := filepath.Rel(proj.Path, absPath)
				if err != nil {
					rel = filepath.Base(absPath)
				}

				destPath := filepath.Join(outDir, name, rel)
				
				// Get size
				size, err := client.GetFileInfo(ctx, absPath)
				if err != nil {
					fmt.Printf("[%d/%d] Error getting info for %s: %v\n", i+1, len(files), rel, err)
					continue
				}

				if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
					fmt.Printf("[%d/%d] Error creating output directory for %s: %v\n", i+1, len(files), rel, err)
					continue
				}

				fmt.Printf("[%d/%d] %s (%.2f MB)...\n", i+1, len(files), rel, float64(size)/1048576)

				f, err := os.Create(destPath)
				if err != nil {
					fmt.Printf("  Error creating file: %v\n", err)
					continue
				}

				lastPct := -1
				err = client.DownloadFile(ctx, absPath, size, f, func(downloaded, total int64) {
					if total > 0 {
						pct := int(float64(downloaded) * 100 / float64(total))
						if pct != lastPct && pct % 10 == 0 { // Print progress in 10% steps
							fmt.Printf("  %d%% (%d/%d bytes)\n", pct, downloaded, total)
							lastPct = pct
						}
					}
				})
				_ = f.Close()

				if err != nil {
					fmt.Printf("  Error downloading file: %v\n", err)
				} else {
					fmt.Printf("  Done.\n")
				}
			}
		}

		fmt.Println("\nAll downloads complete.")
		return nil
	},
}

func init() {
	downloadCmd.Flags().Bool("all", false, "Download all available projects")
	rootCmd.AddCommand(downloadCmd)
}
