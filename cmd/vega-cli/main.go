package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/alexgorbatchev/einstar-vega-cli/internal/tui"
	"github.com/alexgorbatchev/einstar-vega-cli/internal/vega"
)

var version = "dev"

func main() {
	addr := flag.String("a", "192.168.30.3", "device address")
	outDir := flag.String("o", "projects", "output directory where to store the projects data")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("vega-cli %s\n", version)
		os.Exit(0)
	}

	client := vega.NewClient(*addr)

	app := tui.NewApp(client, *outDir)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
