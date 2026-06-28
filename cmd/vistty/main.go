package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/LaoQi/vistty/internal/config"
	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/gbm"
	"github.com/LaoQi/vistty/internal/platform/wayland"
	"github.com/LaoQi/vistty/session"
	"github.com/LaoQi/vistty/terminal"
)

func main() {
	if err := run(); err != nil {
		debug.Errorf("fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	backendFlag := flag.String("backend", "auto", "display backend: auto, wayland, drm, or drm-gbm")
	shellFlag := flag.String("shell", "/bin/bash", "shell to run")
	fontFlag := flag.String("font", "", "font file path")
	fontSizeFlag := flag.Float64("fontsize", 14, "font size in pixels")
	primaryFlag := flag.String("primary", "", "primary output name or index")
	modeFlag := flag.String("mode", "independent", "display mode: mirror or independent")
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile to file")
	traceFile := flag.String("trace", "", "write execution trace to file")
	fpsFlag := flag.Bool("fps", false, "print per-frame timing to stderr")
	recordPath := flag.String("record", "", "record PTY output to file")
	ttyFlag := flag.String("tty", "", "bind to specified tty (e.g. 2 or /dev/tty2), DRM only")
	listOutputsFlag := flag.Bool("list-outputs", false, "list all display outputs and exit")
	errorLogFlag := flag.String("errorlog", "", "error log file path (default ~/.local/share/vistty/error.log)")
	configFlag := flag.String("config", "", "config file path (default ~/.config/vistty/config.jsonc)")
	genConfigFlag := flag.Bool("gen-config", false, "print default config to stdout and exit")
	flag.Parse()

	if *genConfigFlag {
		fmt.Print(config.Default().Generate())
		return nil
	}

	configPath := *configFlag
	if configPath == "" {
		if p, err := config.DefaultPath(); err == nil {
			configPath = p
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if !explicit["backend"] && cfg.Backend != "" {
		*backendFlag = cfg.Backend
	}
	if !explicit["shell"] && cfg.Shell != "" {
		*shellFlag = cfg.Shell
	}
	if !explicit["font"] {
		*fontFlag = cfg.Font
	}
	if !explicit["fontsize"] && cfg.FontSize != 0 {
		*fontSizeFlag = cfg.FontSize
	}
	if !explicit["primary"] {
		*primaryFlag = cfg.Primary
	}
	if !explicit["mode"] && cfg.Mode != "" {
		*modeFlag = cfg.Mode
	}
	if !explicit["record"] {
		*recordPath = cfg.Record
	}

	if explicit["errorlog"] {
		if *errorLogFlag != "" {
			if err := debug.ConfigureError(*errorLogFlag, true); err != nil {
				fmt.Fprintf(os.Stderr, "configure error log: %v\n", err)
			}
		}
	} else if cfg.ErrorLog != "" {
		if err := debug.ConfigureError(cfg.ErrorLog, true); err != nil {
			fmt.Fprintf(os.Stderr, "configure error log: %v\n", err)
		}
	}

	resolvedTty := resolveTtyPath(*ttyFlag)
	if resolvedTty != "" {
		debug.Debugf("resolved tty path: %s\n", resolvedTty)
	}

	opts := terminal.DefaultOptions()
	opts.Shell = *shellFlag
	opts.FontPath = *fontFlag
	opts.FontSize = *fontSizeFlag
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
	switch *backendFlag {
	case "auto":
		if drm.Probe() {
			debug.Debugf("auto: DRM probe succeeded, trying drm-gbm\n")
			var drmBackend *drm.DRMBackend
			drmBackend, err = drm.NewDRMBackend(resolvedTty)
			if err == nil && drm.HasAtomic(drmBackend.FD()) {
				gbmDev, gbmErr := gbm.NewGBMDevice(drmBackend.FD())
				if gbmErr == nil {
					drmBackend.SetGBMProvider(gbmDev)
					debug.Debugf("auto: GBM initialized, using drm-gbm\n")
					backend = drmBackend
				} else {
					debug.Warningf("auto: GBM init failed: %v, fallback to drm (dumb buffer)\n", gbmErr)
					backend = drmBackend
				}
			} else if err == nil {
				debug.Debugf("auto: no atomic modesetting, using drm (dumb buffer)\n")
				backend = drmBackend
			}
		}
		if backend == nil && wayland.Probe() {
			debug.Debugf("auto: DRM unavailable, Wayland probe succeeded, using wayland backend\n")
			if resolvedTty != "" {
				debug.Warningf("-tty is ignored by wayland backend\n")
			}
			backend, err = wayland.NewWaylandBackend()
		}
		if backend == nil {
			return fmt.Errorf("no suitable display backend found (tried drm-gbm, drm, wayland)")
		}
	case "drm-gbm":
		var drmBackend *drm.DRMBackend
		drmBackend, err = drm.NewDRMBackend(resolvedTty)
		if err != nil {
			return fmt.Errorf("drm-gbm: failed to create DRM backend: %w", err)
		}
		if !drm.HasAtomic(drmBackend.FD()) {
			drmBackend.Close()
			return fmt.Errorf("drm-gbm: kernel does not support atomic modesetting, use -backend drm for dumb buffer")
		}
		gbmDev, gbmErr := gbm.NewGBMDevice(drmBackend.FD())
		if gbmErr != nil {
			drmBackend.Close()
			return fmt.Errorf("drm-gbm: GBM init failed: %w", gbmErr)
		}
		drmBackend.SetGBMProvider(gbmDev)
		backend = drmBackend
	case "drm":
		var drmBackend *drm.DRMBackend
		drmBackend, err = drm.NewDRMBackend(resolvedTty)
		backend = drmBackend
	case "wayland":
		if resolvedTty != "" {
			debug.Warningf("-tty is ignored by wayland backend\n")
		}
		backend, err = wayland.NewWaylandBackend()
	default:
		return fmt.Errorf("unknown backend: %s (valid: auto, wayland, drm, drm-gbm)", *backendFlag)
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

	m, err := session.NewMaster(backend, opts)
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
