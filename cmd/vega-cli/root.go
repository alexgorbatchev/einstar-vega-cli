package main

import (
	"fmt"
	"net"
	"os"

	"github.com/alexgorbatchev/einstar-vega-cli/internal/tui"
	"github.com/alexgorbatchev/einstar-vega-cli/internal/vega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	rootCmd = &cobra.Command{
		Use:   "vega-cli",
		Short: "A command-line downloader and TUI for Einstar Vega 3D Scanner",
		Long: `A command-line downloader and TUI for Einstar Vega 3D Scanner.
It allows you to explore the scanner's file system, view device info,
and download projects or individual files over the network without having to register your device.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			ip := viper.GetString("ip")
			if ip == "" {
				return fmt.Errorf("--ip is required (or set VEGA_IP environment variable)")
			}
			port := viper.GetString("port")
			outDir := viper.GetString("output")

			client := vega.NewClient(net.JoinHostPort(ip, port))
			app := tui.NewApp(client, outDir)
			if err := app.Run(); err != nil {
				return fmt.Errorf("running TUI: %w", err)
			}
			return nil
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().String("ip", "", "device IP address (can be set via VEGA_IP)")
	rootCmd.PersistentFlags().StringP("port", "p", "8080", "device port (can be set via VEGA_PORT)")
	rootCmd.PersistentFlags().StringP("output", "o", "projects", "output directory where to store projects (can be set via VEGA_OUTPUT)")

	_ = viper.BindPFlag("ip", rootCmd.PersistentFlags().Lookup("ip"))
	_ = viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
}

func initConfig() {
	viper.SetEnvPrefix("VEGA")
	viper.AutomaticEnv()
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
