# Vistty

**English** | [简体中文](./README.zh-CN.md)

> **Heads up: this is a vibe product.** It is built for fun, experimentation and learning, not for production use. Expect rough edges, missing pieces and breaking changes. Use it at your own risk.

Vistty is a virtual terminal emulator that runs directly on the Linux DRM/KMS
subsystem, with no X11 or Wayland display server required. It is similar in
spirit to [kmscon](https://www.freedesktop.org/wiki/Software/KMScon/). A
Wayland window backend is also included for development and debugging inside a
desktop session.

## Features

- DRM/KMS direct rendering: dumb buffer + page flip (`drm`), or GBM/EGL/GLES
  GPU acceleration via Atomic Modesetting (`drm-gbm`). Auto-detect tries
  `drm-gbm` first, then `drm`, then `wayland`.
- Wayland window backend: in-tree pure-Go Wayland wire protocol layer
  (no external Wayland bindings), using `wl_shm` for zero-CGO shared memory,
  with `zxdg_decoration_manager_v1` SSD/CSD negotiation and self-drawn CSD
  buttons.
- Multi-monitor support: enumerates every connected connector; independent
  display mode per output; primary output selection by name or index.
- Built-in font: Sarasa Fixed SC subset (monospace + CJK) as primary, with a
  NerdFont PUA subset as fallback (Powerline/Nerd icons). Rasterized via
  `golang.org/x/image/font/opentype` with a glyph atlas cache. Block Elements
  (U+2580-259F) are synthesized when missing. Live font scaling
  (Super + `=` / `-` / `0`).
- Color emoji rendering: an in-tree `font/emoji.go` embeds a 1353-glyph
  subset extracted from NotoColorEmoji.ttf (direct SFNT/cmap/CBLC/CBDT
  parsing in `cmd/gen-emoji`). CPU blending (BGRA premultiplied) and a GPU shader
  `isColor` branch with a separate `UploadColorGlyph` path.
- xterm-256 compatible VT: a hand-written 9-state escape sequence parser with
  CSI/OSC/ESC/DCS handling, alternate screen, scroll regions, DEC line drawing,
  bracketed paste, focus reporting, dynamic cursor styles, OSC 10/11/12 color
  query/set, and CJK double-width rendering.
- Plugin system: a `gopher-lua` Lua 5.1 VM driven by `init.lua`, exposing a
  layered `vistty.*` API (input bind/bind_keys/pressed, term, tab, screen, zoom,
  ui, pinyin, keybind, theme, lifecycle hooks `on_exit`/`on_tab_new`/
  `on_tab_close`/`on_tab_switch`/`on_screen_switch`/`on_title_change`/`on_resize`/
  `on_zoom`/`on_activate`, plus runtime environment queries
  `backend_name`/`is_wayland`/`is_drm`). Hot reload with Super + `R`.
- Theming: terminal `Theme` (DefFg/DefBg/CursorColor/Palette[16]) and OSD
  `OSDTheme` are both configured from Lua (`vistty.config.theme` static declaration
  + `vistty.theme.apply/get/default` dynamic API). Seven built-in presets:
  dracula, solarized_dark, solarized_light, gruvbox, monokai, nord, one_dark.
  Runtime OSC color changes are overridden by theme switching.
- OSD tab bar + multi-terminal tabs: top tab bar with CJK double-width rendering,
  horizontal scrolling (active tab always visible), single-title truncation, and
  per-tab close. Tab bar drag moves the window (Wayland).
- Chinese Pinyin input method: a top-level `pinyin` package with package-level
  query functions (`Lookup`/`FormatPreedit`/`Split`/`SplitFuzzy`) over an embedded
  rime-ice dictionary, with prefix inference + tail completion fuzzy splitting and
  a single-line candidate panel driven from Lua.
- Italic rendering: glyphs are pre-sheared (slope 0.1) in the font layer via
  bilinear interpolation and served through the normal blend/atlas path; the CPU
  and GPU renderers share a single italic atlas keyed by `(rune, italic)`.
- Pure Go, CGO disabled (`CGO_ENABLED=0`): every native interface (DRM, GBM,
  EGL, GLES, evdev, Wayland, opentype) is reached via `syscall`/`ioctl` or
  `purego` dlopen. No C toolchain is needed to build.
- TTY binding, VT switching (SIGUSR1/2), graceful two-phase shutdown,
  pprof/trace profiling hooks, PTY session recording for offline replay
  benchmarking, and version info via `-version` (git `describe --tags`).

## Build

Requires Go (module declares `go 1.26.4`) on `linux/amd64`:

```bash
go build ./...
go vet ./...
go test ./...
```

A version-stamped build is available via the helper script:

```bash
./scripts/build.sh
```

## Run

```bash
# Auto-detect backend (drm-gbm → drm → wayland)
go run ./cmd/vistty

# Force DRM/KMS with dumb buffer (CPU rendering)
go run ./cmd/vistty -backend drm

# Force DRM/KMS with GBM/EGL GPU acceleration
go run ./cmd/vistty -backend drm-gbm

# Force Wayland window (development/debugging inside a desktop session)
go run ./cmd/vistty -backend wayland

# Bind to tty2 (setsid + TIOCSCTTY to acquire the controlling terminal)
go run ./cmd/vistty -backend drm -tty 2

# List all display outputs and exit
go run ./cmd/vistty -list-outputs

# Select primary output by connector name
go run ./cmd/vistty -primary HDMI-A-1

# Print version info and exit
go run ./cmd/vistty -version

# Use a custom init.lua
go run ./cmd/vistty -config ./my-init.lua
```

### Common flags

| Flag             | Description                                                          |
|------------------|----------------------------------------------------------------------|
| `-backend`       | `auto` (default), `wayland`, `drm`, or `drm-gbm`                     |
| `-config`        | `init.lua` script path (default `~/.config/vistty/init.lua`)         |
| `-tty`           | bind to a TTY, e.g. `2` or `/dev/tty2` (DRM only)                    |
| `-list-outputs`  | list all display outputs and exit                                    |
| `-primary`       | primary output by connector name (e.g. `HDMI-A-1`) or index          |
| `-version`       | print version info and exit                                          |
| `-cpuprofile`    | write a CPU profile to file                                          |
| `-memprofile`    | write a heap profile to file                                         |
| `-mutexprofile`  | write a mutex profile to file                                        |
| `-trace`         | write an execution trace to file                                     |
| `-fps`           | print per-frame timing to stderr                                     |
| `-record`        | record PTY output to a file for offline replay                       |

### Configuration

Runtime options (shell, font, font size, primary output, error log, theme) are
declared in `init.lua` through the `vistty.config` table rather than command-line
flags. The default config path is `~/.config/vistty/init.lua`; pass a custom path
with `-config`. A documented example ships in [`examples/init.lua`](./examples/init.lua):

```lua
vistty.config = {
    backend   = "auto",       -- auto / wayland / drm / drm-gbm
    shell     = "/bin/bash",
    font      = "",           -- external font path; built-in font when empty
    fallback_font = "",       -- NerdFont fallback; disable with ""
    fontsize  = 14,
    primary   = "",           -- e.g. "HDMI-A-1"
    error_log = "",           -- default ~/.local/share/vistty/error.log when set
    theme     = require("themes.gruvbox"),
}
```

Built-in theme presets live in [`examples/themes/`](./examples/themes):
`dracula`, `solarized_dark`, `solarized_light`, `gruvbox`, `monokai`, `nord`,
`one_dark`, plus an `xterm` default.

### Keyboard shortcuts

The modifier key adapts to the backend: `Alt` on Wayland, `Super` on DRM.
Defaults (see `examples/init.lua`) are:

- Mod + `=` / Mod + `-` / Mod + `0` : enlarge / shrink / reset font
- Mod + `T` / Mod + `W` : new tab / close tab
- Mod + `Tab` : next tab
- Mod + `1..9` : switch to tab N
- Mod + `Left` / Mod + `Right` : previous / next screen
- Mod + `R` : hot-reload `init.lua`
- Mod + `Q` : quit
- `Ctrl` + `Space` : toggle the Pinyin input method

## Underlying support

| Concern           | Approach                                                              |
|-------------------|-----------------------------------------------------------------------|
| DRM/KMS           | In-tree `ioctl` wrappers (inspired by NeowayLabs/drm)                 |
| Framebuffer       | DRM dumb buffer + CPU rendering; GBM/EGL/GLES path via `purego`       |
| Input             | `holoplot/go-evdev` + inotify hotplug (DRM); in-tree XKB keymap (Wayland) |
| PTY               | `creack/pty`                                                          |
| Escape parsing    | In-tree VTE state machine (xterm-256 compatible)                      |
| Terminal buffer   | In-tree Cell/Line/Buffer                                              |
| Font              | `golang.org/x/image/font/opentype` + glyph atlas cache + bundled emoji subset |
| Text shaping      | Not integrated yet (reserved for `go-text/typesetting/harfbuzz`)      |
| Wayland protocol  | In-tree pure-Go `wl.go` wire protocol layer (zero CGO)                |
| Plugin VM         | `gopher-lua` (Lua 5.1)                                                |
| Pinyin dictionary | `go:embed` rime-ice, compact index with binary-search lookup          |

## License

Vistty is licensed under the [GNU General Public License v2 or later](./LICENSE).
