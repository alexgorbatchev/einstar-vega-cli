package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/alexgorbatchev/einstar-vega-cli/internal/vega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects on the Einstar Vega scanner",
	Long:  `List all projects stored on the Einstar Vega scanner, including their paths and creation timestamps.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := viper.GetString("ip")
		if ip == "" {
			return fmt.Errorf("--ip is required (or set VEGA_IP environment variable)")
		}
		port := viper.GetString("port")

		client := vega.NewClient(net.JoinHostPort(ip, port))
		ctx := context.Background()

		projects, err := client.GetProjectsInfo(ctx)
		if err != nil {
			return fmt.Errorf("getting projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found on the device.")
			return nil
		}

		// Sort by name
		names := make([]string, 0, len(projects))
		for name := range projects {
			names = append(names, name)
		}
		sort.Strings(names)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tPATH\tDATE TIME")
		for _, name := range names {
			p := projects[name]
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, p.Path, p.DateTime)
		}
		w.Flush()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(projectsCmd)
}
