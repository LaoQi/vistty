# Vistty

> **Heads up: this is a vibe product.** It is built for fun, experimentation and learning, not for production use. Expect rough edges, missing pieces and breaking changes. Use it at your own risk.

Vistty is a virtual terminal emulator that runs directly on the Linux DRM/KMS
subsystem, with no X11 or Wayland display server required. It is similar in
spirit to [kmscon](https://www.freedesktop.org/wiki/Software/KMScon/). A
Wayland window backend is also included for development and debugging inside a
desktop session.

## Features

- DRM/KMS direct rendering: dumb buffer + page flip, with optional GBM/EGL/GLES
  GPU acceleration path (Atomic Modesetting) that falls back to dumb buffer when
  unavailable.
- Wayland window backend: self-contained pure-Go Wayland wire protocol layer
  (no external Wayland bindings), using `wl_shm` for zero-CGLO shared memory.
- Multi-monitor support: enumerates every connected connector; mirror or
  independent display modes; primary output selection by name or index.
- Built-in font: Sarasa Fixed SC subset (monospace + CJK), rasterized via
  `golang.org/x/image/font/opentype` with a glyph atlas cache. Live font scaling
  (Super + `=` / `-` / `0`).
- xterm-256 compatible VT: a hand-written 9-state escape sequence parser with
  CSI/OSC/ESC/DCS handling, alternate screen, scroll regions, DEC line drawing,
  bracketed paste, focus reporting, dynamic cursor styles, OSC 10/11 color
  query/set, and CJK double-width rendering.
- Pure Go, CGO disabled (`CGO_ENABLED=0`): every native interface (DRM, GBM,
  EGL, GLES, evdev, Wayland, opentype) is reached via `syscall`/`ioctl` or
  `purego` dlopen. No C toolchain is needed to build.
- TTY binding, VT switching (SIGUSR1/2), graceful shutdown, pprof/trace
  profiling hooks, and PTY session recording for offline replay benchmarking.

## Build

Requires Go (module declares `go 1.26.4`) on `linux/amd64`:

```bash
go build ./...
go vet ./...
go test ./...
```

## Run

```bash
# Auto-detect backend (DRM preferred, falls back to Wayland)
go run ./cmd/vistty

# Force DRM/KMS direct rendering
go run ./cmd/vistty -backend drm

# Force Wayland window (development/debugging inside a desktop session)
go run ./cmd/vistty -backend wayland

# Bind to tty2 (setsid + TIOCSCTTY to acquire the controlling terminal)
go run ./cmd/vistty -backend drm -tty 2

# Generate default config (prints JSONC with comments to stdout)
go run ./cmd/vistty -gen-config > ~/.config/vistty/config.jsonc

# Use a custom config file
go run ./cmd/vistty -config ./my-config.jsonc
```

### Common flags

| Flag            | Description                                                       |
|-----------------|-------------------------------------------------------------------|
| `-backend`      | `auto` (default), `drm`, or `wayland`                             |
| `-shell`        | shell to run (default `/bin/bash`)                               |
| `-font`         | external font file path (built-in font used when empty)          |
| `-fontsize`     | font size in pixels (default 14)                                  |
| `-mode`         | `mirror` or `independent` (default `independent`)                |
| `-primary`      | primary output by connector name (e.g. `HDMI-A-1`) or index      |
| `-tty`          | bind to a TTY, e.g. `2` or `/dev/tty2` (DRM only)                 |
| `-nogbm`        | disable GBM/EGL, use dumb buffer (DRM only)                       |
| `-list-outputs` | list all display outputs and exit                                 |
| `-errorlog`     | error log file path (default `~/.local/share/vistty/error.log`)   |
| `-config`       | config file path (default `~/.config/vistty/config.jsonc`)       |
| `-gen-config`   | print default config (JSONC with comments) to stdout and exit   |
| `-cpuprofile`   | write a CPU profile to file                                       |
| `-memprofile`   | write a heap profile to file                                      |
| `-trace`        | write an execution trace to file                                  |
| `-fps`          | print per-frame timing to stderr                                  |
| `-record`       | record PTY output to a file for offline replay                    |

### Keyboard shortcuts

- Super + `=` / Super + `-` / Super + `0` : enlarge / shrink / reset font
- Super + `1..9` : switch focus to output N (independent mode)
- Super + Tab : cycle focus across outputs (independent mode)

## Underlying support

| Concern           | Approach                                                              |
|-------------------|-----------------------------------------------------------------------|
| DRM/KMS           | Self-implemented `ioctl` wrappers (inspired by NeowayLabs/drm)        |
| Framebuffer       | DRM dumb buffer + CPU rendering; GBM/EGL/GLES path via `purego`       |
| Input             | `holoplot/go-evdev` (DRM); self-implemented XKB keymap (Wayland)      |
| PTY               | `creack/pty`                                                          |
| Escape parsing    | Self-implemented VTE state machine (xterm-256 compatible)             |
| Terminal buffer   | Self-implemented Cell/Line/Buffer                                     |
| Font              | `golang.org/x/image/font/opentype` + glyph atlas cache                |
| Wayland protocol  | Self-implemented pure-Go `wl.go` wire protocol layer (zero CGO)      |

## License

Vistty is licensed under the [GNU General Public License v2](./LICENSE).
