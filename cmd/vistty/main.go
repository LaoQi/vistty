package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/wayland"
	"github.com/LaoQi/vistty/terminal"
)

func main() {
	backendFlag := flag.String("backend", "drm", "display backend: drm or wayland")
	shellFlag := flag.String("shell", "/bin/bash", "shell to run")
	fontFlag := flag.String("font", "", "font file path")
	fontSizeFlag := flag.Float64("fontsize", 14, "font size in pixels")
	widthFlag := flag.Int("width", 800, "window width")
	heightFlag := flag.Int("height", 600, "window height")
	flag.Parse()

	opts := terminal.DefaultOptions()
	opts.Shell = *shellFlag
	opts.FontPath = *fontFlag
	opts.FontSize = *fontSizeFlag
	opts.Width = *widthFlag
	opts.Height = *heightFlag

	var backend platform.Backend
	var err error
	switch *backendFlag {
	case "drm":
		backend, err = drm.NewDRMBackend()
	case "wayland":
		backend, err = wayland.NewWaylandBackend()
	default:
		fmt.Fprintf(os.Stderr, "unknown backend: %s\n", *backendFlag)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create backend: %v\n", err)
		os.Exit(1)
	}
	defer backend.Close()

	term, err := terminal.New(backend, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create terminal: %v\n", err)
		os.Exit(1)
	}
	defer term.Close()

	if err := term.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "terminal error: %v\n", err)
		os.Exit(1)
	}
}
