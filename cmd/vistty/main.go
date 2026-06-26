package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/wayland"
	"github.com/LaoQi/vistty/master"
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
	primaryFlag := flag.String("primary", "", "primary output name or index")
	modeFlag := flag.String("mode", "independent", "display mode: mirror or independent")
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile to file")
	traceFile := flag.String("trace", "", "write execution trace to file")
	fpsFlag := flag.Bool("fps", false, "print per-frame timing to stderr")
	recordPath := flag.String("record", "", "record PTY output to file")
	ttyFlag := flag.String("tty", "", "bind to specified tty (e.g. 2 or /dev/tty2), DRM only")
	noGBMFlag := flag.Bool("nogbm", false, "disable GBM/EGL, use dumb buffer (DRM only)")
	listOutputsFlag := flag.Bool("list-outputs", false, "list all display outputs and exit")
	flag.Parse()

	debugLog := os.Getenv("VISTTY_DEBUG") != ""

	resolvedTty := resolveTtyPath(*ttyFlag)
	if resolvedTty != "" && debugLog {
		fmt.Fprintf(os.Stderr, "resolved tty path: %s\n", resolvedTty)
	}

	opts := terminal.DefaultOptions()
	opts.Shell = *shellFlag
	opts.FontPath = *fontFlag
	opts.FontSize = *fontSizeFlag
	opts.Width = *widthFlag
	opts.Height = *heightFlag
	opts.Primary = *primaryFlag
	opts.Mode = *modeFlag

	if *recordPath != "" {
		f, err := os.Create(*recordPath)
		if err != nil {
			return fmt.Errorf("create record file: %w", err)
		}
		defer f.Close()
		opts.RecordWriter = f
	}

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
			backend, err = drm.NewDRMBackend(resolvedTty, *noGBMFlag)
		} else if wayland.Probe() {
			if debugLog {
				fmt.Fprintf(os.Stderr, "auto: DRM probe failed, Wayland probe succeeded, using Wayland backend\n")
			}
			if resolvedTty != "" {
				fmt.Fprintf(os.Stderr, "warning: -tty is ignored by wayland backend\n")
			}
			backend, err = wayland.NewWaylandBackend()
		} else {
			return fmt.Errorf("no suitable display backend found (tried DRM and Wayland)")
		}
	case "drm":
		backend, err = drm.NewDRMBackend(resolvedTty, *noGBMFlag)
	case "wayland":
		if resolvedTty != "" {
			fmt.Fprintf(os.Stderr, "warning: -tty is ignored by wayland backend\n")
		}
		backend, err = wayland.NewWaylandBackend()
	default:
		return fmt.Errorf("unknown backend: %s", *backendFlag)
	}
	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}
	defer backend.Close()

	if *listOutputsFlag {
		outputs, err := backend.ListOutputs()
		if err != nil {
			return fmt.Errorf("list outputs: %w", err)
		}
		for i, o := range outputs {
			w, h := o.Size()
			fmt.Printf("[%d] %s  %dx%d  (id=%d, connector=%d, crtc=%d)\n", i, o.Name(), w, h, o.ID(), o.ConnectorID(), o.CrtcID())
		}
		return nil
	}

	m, err := master.New(backend, opts)
	if err != nil {
		return fmt.Errorf("failed to create master: %w", err)
	}
	defer m.Close()

	if prof.fps {
		m.EnableFPSLogging()
	}

	if err := m.Run(); err != nil {
		return fmt.Errorf("master error: %w", err)
	}
	return nil
}

func resolveTtyPath(tty string) string {
	if tty == "" {
		return ""
	}
	if strings.HasPrefix(tty, "/dev/") {
		return tty
	}
	if _, err := strconv.Atoi(tty); err == nil {
		return "/dev/tty" + tty
	}
	return tty
}
