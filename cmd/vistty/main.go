package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/gbm"
	"github.com/LaoQi/vistty/internal/platform/wayland"
	"github.com/LaoQi/vistty/internal/plugins"
	"github.com/LaoQi/vistty/internal/version"
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
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile to file")
	traceFile := flag.String("trace", "", "write execution trace to file")
	fpsFlag := flag.Bool("fps", false, "print per-frame timing to stderr")
	recordPath := flag.String("record", "", "record PTY output to file")
	ttyFlag := flag.String("tty", "", "bind to specified tty (e.g. 2 or /dev/tty2), DRM only")
	listOutputsFlag := flag.Bool("list-outputs", false, "list all display outputs and exit")
	configFlag := flag.String("config", "", "init.lua script path (default ~/.config/vistty/init.lua)")
	versionFlag := flag.Bool("version", false, "print version info and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version.String())
		return nil
	}

	configPath := *configFlag
	if configPath == "" {
		configPath = plugins.DefaultInitPath()
	}

	pm := plugins.NewPluginManager(configPath)
	runCfg, err := pm.Load()
	if err != nil {
		pm.Close()
		return fmt.Errorf("load init.lua: %w", err)
	}

	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	backendName := runCfg.Backend
	if explicit["backend"] {
		backendName = *backendFlag
	}

	if runCfg.ErrorLog != "" {
		if err := debug.ConfigureError(runCfg.ErrorLog, true); err != nil {
			fmt.Fprintf(os.Stderr, "configure error log: %v\n", err)
		}
	}

	resolvedTty := resolveTtyPath(*ttyFlag)
	if resolvedTty != "" {
		debug.Debugf("resolved tty path: %s\n", resolvedTty)
	}

	opts := terminal.DefaultOptions()
	opts.Shell = runCfg.Shell
	opts.FontPath = runCfg.FontPath
	opts.FallbackFontPath = runCfg.FallbackFontPath
	opts.FontSize = runCfg.FontSize
	opts.Primary = runCfg.Primary

	if *recordPath != "" {
		f, err := os.Create(*recordPath)
		if err != nil {
			pm.Close()
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
		pm.Close()
		return fmt.Errorf("start profiling: %w", err)
	}
	defer prof.stop()

	var backend platform.Backend
	switch backendName {
	case "auto":
		fd, probeErr := drm.ProbeDetailed()
		if probeErr == nil {
			debug.Debugf("auto: DRM probe succeeded, trying drm-gbm\n")
			drmBackend, beErr := drm.NewDRMBackendFromFD(fd, resolvedTty)
			if beErr != nil {
				debug.Warningf("auto: DRM backend init failed: %v, trying wayland\n", beErr)
			} else {
				gbmDev, gbmErr := gbm.NewGBMDevice(drmBackend.FD())
				if gbmErr == nil {
					drmBackend.SetGBMProvider(gbmDev)
					debug.Debugf("auto: GBM initialized, using drm-gbm\n")
					backendName = "drm-gbm"
				} else {
					debug.Warningf("auto: GBM init failed: %v, using drm (dumb buffer)\n", gbmErr)
					backendName = "drm"
				}
				backend = drmBackend
			}
		}
		if backend == nil && wayland.Probe() {
			debug.Debugf("auto: Wayland probe succeeded, using wayland backend\n")
			if resolvedTty != "" {
				debug.Warningf("-tty is ignored by wayland backend\n")
			}
			backend, err = wayland.NewWaylandBackend()
			backendName = "wayland"
		}
		if backend == nil {
			pm.Close()
			return fmt.Errorf("no suitable display backend found (tried drm-gbm, drm, wayland)")
		}
	case "drm-gbm":
		var drmBackend *drm.DRMBackend
		drmBackend, err = drm.NewDRMBackend(resolvedTty)
		if err != nil {
			pm.Close()
			return fmt.Errorf("drm-gbm: failed to create DRM backend: %w", err)
		}
		gbmDev, gbmErr := gbm.NewGBMDevice(drmBackend.FD())
		if gbmErr != nil {
			drmBackend.Close()
			pm.Close()
			if strings.Contains(gbmErr.Error(), "DRM_CLIENT_CAP_ATOMIC") {
				return fmt.Errorf("drm-gbm: kernel does not support atomic modesetting, use -backend drm for dumb buffer")
			}
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
		pm.Close()
		return fmt.Errorf("unknown backend: %s (valid: auto, wayland, drm, drm-gbm)", backendName)
	}
	if err != nil {
		pm.Close()
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
		pm.Close()
		return fmt.Errorf("failed to create master: %w", err)
	}
	defer m.Close()
	defer pm.Close()

	m.SetPluginManager(pm)
	pm.SetBackendName(backendName)
	pm.Activate(m)

	if runCfg.TermTheme != nil {
		m.ApplyTheme(*runCfg.TermTheme, *runCfg.OSDTheme)
	}

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

