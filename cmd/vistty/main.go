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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	backendFlag := flag.String("backend", "auto", "display backend: auto, drm, or wayland")
	shellFlag := flag.String("shell", "/bin/bash", "shell to run")
	fontFlag := flag.String("font", "", "font file path")
	fontSizeFlag := flag.Float64("fontsize", 14, "font size in pixels")
	widthFlag := flag.Int("width", 800, "window width")
	heightFlag := flag.Int("height", 600, "window height")
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile to file")
	traceFile := flag.String("trace", "", "write execution trace to file")
	fpsFlag := flag.Bool("fps", false, "print per-frame timing to stderr")
	recordPath := flag.String("record", "", "record PTY output to file")
	flag.Parse()

	debugLog := os.Getenv("VISTTY_DEBUG") != ""

	opts := terminal.DefaultOptions()
	opts.Shell = *shellFlag
	opts.FontPath = *fontFlag
	opts.FontSize = *fontSizeFlag
	opts.Width = *widthFlag
	opts.Height = *heightFlag

	var recordFile *os.File
	if *recordPath != "" {
		f, err := os.Create(*recordPath)
		if err != nil {
			return fmt.Errorf("create record file: %w", err)
		}
		defer f.Close()
		recordFile = f
		opts.RecordWriter = f
	}
	_ = recordFile

	prof := &profileConfig{
		cpuProfile:   *cpuProfile,
		memProfile:   *memProfile,
		mutexProfile: *mutexProfile,
		traceFile:    *traceFile,
		fps:          *fpsFlag,
	}
	if err := prof.start(); err != nil {
		return fmt.Errorf("start profiling: %w", err)
	}
	defer prof.stop()

	var backend platform.Backend
	var err error
	switch *backendFlag {
	case "auto":
		if drm.Probe() {
			if debugLog {
				fmt.Fprintf(os.Stderr, "auto: DRM probe succeeded, using DRM backend\n")
			}
			backend, err = drm.NewDRMBackend()
		} else if wayland.Probe() {
			if debugLog {
				fmt.Fprintf(os.Stderr, "auto: DRM probe failed, Wayland probe succeeded, using Wayland backend\n")
			}
			backend, err = wayland.NewWaylandBackend()
		} else {
			return fmt.Errorf("no suitable display backend found (tried DRM and Wayland)")
		}
	case "drm":
		backend, err = drm.NewDRMBackend()
	case "wayland":
		backend, err = wayland.NewWaylandBackend()
	default:
		return fmt.Errorf("unknown backend: %s", *backendFlag)
	}
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	term, err := terminal.New(backend, opts)
	if err != nil {
		return fmt.Errorf("failed to create terminal: %w", err)
	}
	defer term.Close()

	if prof.fps {
		term.EnableFPSLogging()
	}

	if err := term.Run(); err != nil {
		return fmt.Errorf("terminal error: %w", err)
	}
	return nil
}
