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

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Print hardware and firmware info of the Einstar Vega scanner",
	Long:  `Retrieve and print hardware specs, battery status, firmware version, and memory/storage path info from the connected Einstar Vega.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := viper.GetString("ip")
		if ip == "" {
			return fmt.Errorf("--ip is required (or set VEGA_IP environment variable)")
		}
		port := viper.GetString("port")

		client := vega.NewClient(net.JoinHostPort(ip, port))
		ctx := context.Background()

		fmt.Println("Connecting to Einstar Vega scanner...")
		if err := client.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to scanner: %w", err)
		}

		info, err := client.GetDeviceInfo(ctx)
		if err != nil {
			return fmt.Errorf("getting device info: %w", err)
		}

		fmt.Println("\n--- Device Information ---")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		
		// Sort keys for deterministic output
		keys := make([]string, 0, len(info))
		for k := range info {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(w, "%s:\t%v\n", k, info[k])
		}
		w.Flush()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
