# Vistty

[English](./README.md) | **简体中文**

> **提醒：这是一个 vibe 产品。** 出于乐趣、实验和学习目的而构建，并非生产可用软件。请接受粗糙的边缘、缺失的功能和随时可能发生的破坏性变更，风险自负。

Vistty 是一个直接运行在 Linux DRM/KMS 子系统上的虚拟终端仿真器，无需 X11 或
Wayland 显示服务器，定位类似
[kmscon](https://www.freedesktop.org/wiki/Software/KMScon/)。同时附带一个 Wayland
窗口后端，用于在桌面会话内开发与调试。

## 功能

- DRM/KMS 直出：`drm` 使用 dumb buffer + page flip（CPU 渲染），`drm-gbm`
  通过 Atomic Modesetting 使用 GBM/EGL/GLES GPU 加速。自动探测按
  `drm-gbm` → `drm` → `wayland` 顺序尝试。
- Wayland 窗口后端：纯 Go Wayland wire 协议层（无外部 Wayland 绑定），通过
  `wl_shm` 共享内存提交帧，含 `zxdg_decoration_manager_v1` SSD/CSD 协商与自绘
  CSD 按钮。
- 多屏支持：枚举所有已连接的 connector；每输出独立显示模式；可按名称或
  索引选择主屏。
- 内置字体：Sarasa Fixed SC 子集（等宽 + CJK）为主字体，NerdFont PUA 子集
  （Powerline/Nerd 图标）作 fallback。经 `golang.org/x/image/font/opentype`
  光栅化并配合 glyph atlas 缓存；Block Elements（U+2580-259F）缺失时自合成。
  支持实时缩放（Super + `=` / `-` / `0`）。
- 彩色 Emoji 渲染：`font/emoji.go` 内嵌 1353 字形子集（由
  NotoColorEmoji.ttf 经 `cmd/gen-emoji` 直接解析 SFNT/cmap/CBLC/CBDT 提取）；
  CPU 混合（BGRA 预乘）与 GPU shader `isColor` 分支 + 独立 `UploadColorGlyph`
  双路径。
- xterm-256 兼容 VT：手写 9 状态转义序列解析器，覆盖 CSI/OSC/ESC/DCS，含备用屏、
  滚动区域、DEC line drawing、括号粘贴、焦点上报、动态光标样式、OSC 10/11/12 颜色
  查询与设置、CJK 双宽渲染。
- 插件系统：`gopher-lua` Lua 5.1 VM 由 `init.lua` 驱动，分层 `vistty.*` API
  （input bind/bind_keys/pressed、term、tab、screen、zoom、ui、pinyin、keybind、
  theme，生命周期钩子 `on_exit`/`on_tab_new`/`on_tab_close`/`on_tab_switch`/
  `on_screen_switch`/`on_title_change`/`on_resize`/`on_zoom`/`on_activate`，以及
  运行环境查询 `backend_name`/`is_wayland`/`is_drm`）。Super + `R` 热重载。
- 主题系统：终端 `Theme`（DefFg/DefBg/CursorColor/Palette[16]）与 OSD
  `OSDTheme` 全部走 Lua 配置（`vistty.config.theme` 静态声明 +
  `vistty.theme.apply/get/default` 动态 API）。七个内置预设：
  dracula、solarized_dark、solarized_light、gruvbox、monokai、nord、one_dark。
  主题切换覆盖运行时 OSC 颜色修改。
- OSD 标签栏 + 多终端标签：顶部标签栏支持 CJK 双宽渲染、水平滚动（活动标签始终
  可见）、单标题截断省略号、按标签关闭。Wayland 下可拖拽标签栏移动窗口。
- 中文拼音输入法：顶层 `pinyin` 包提供包级查询函数
  （`Lookup`/`FormatPreedit`/`Split`/`SplitFuzzy`），内嵌 rime-ice 词库，
  支持前缀推断 + 尾部补全的宽松切分（`SplitFuzzy`），Lua 层管理交互状态并渲染
  单行候选词面板。
- 斜体渲染：font 层预生成斜体字形（slope=0.1，双线性插值抗锯齿），CPU 与 GPU
  统一走正常 blend/atlas 路径，独立 italicAtlas 以 `(rune, italic)` 为 key。
- 纯 Go，禁用 CGO（`CGO_ENABLED=0`）：所有原生接口（DRM、GBM、EGL、GLES、
  evdev、Wayland、opentype）均经 `syscall`/`ioctl` 或 `purego` dlopen 访问，构建
  无需 C 工具链。
- TTY 绑定、VT 切换（SIGUSR1/2）、两阶段优雅退出、pprof/trace 性能采集、
  PTY 会话录制用于离线回放基准测试，以及通过 `-version` 查询版本信息
  （git `describe --tags`）。

## 构建

需 Go 环境（模块声明 `go 1.26.4`），目标平台 `linux/amd64`：

```bash
go build ./...
go vet ./...
go test ./...
```

带版本注入的构建可使用辅助脚本：

```bash
./scripts/build.sh
```

## 运行

```bash
# 自动探测后端（drm-gbm → drm → wayland）
go run ./cmd/vistty

# 强制 DRM/KMS dumb buffer（CPU 渲染）
go run ./cmd/vistty -backend drm

# 强制 DRM/KMS GBM/EGL GPU 加速
go run ./cmd/vistty -backend drm-gbm

# 强制 Wayland 窗口（桌面会话内开发调试）
go run ./cmd/vistty -backend wayland

# 绑定 tty2（setsid + TIOCSCTTY 获取控制终端）
go run ./cmd/vistty -backend drm -tty 2

# 列出所有显示输出后退出
go run ./cmd/vistty -list-outputs

# 按 connector 名称选择主屏
go run ./cmd/vistty -primary HDMI-A-1

# 查看版本信息后退出
go run ./cmd/vistty -version

# 使用自定义 init.lua
go run ./cmd/vistty -config ./my-init.lua
```

### 常用参数

| 参数              | 说明                                                              |
|-------------------|-------------------------------------------------------------------|
| `-backend`        | `auto`（默认）、`wayland`、`drm` 或 `drm-gbm`                     |
| `-config`         | `init.lua` 脚本路径（默认 `~/.config/vistty/init.lua`）           |
| `-tty`            | 绑定到指定 TTY，如 `2` 或 `/dev/tty2`（仅 DRM）                   |
| `-list-outputs`   | 列出所有显示输出后退出                                            |
| `-primary`        | 按 connector 名称（如 `HDMI-A-1`）或索引选择主屏                  |
| `-version`        | 打印版本信息后退出                                                |
| `-cpuprofile`     | 输出 CPU profile 到文件                                           |
| `-memprofile`     | 输出堆 profile 到文件                                             |
| `-mutexprofile`   | 输出 mutex profile 到文件                                         |
| `-trace`          | 输出执行 trace 到文件                                             |
| `-fps`            | 向 stderr 打印每帧耗时                                            |
| `-record`         | 录制 PTY 输出到文件，用于离线回放                                 |

### 配置

运行时选项（shell、字体、字号、主屏、错误日志、主题）通过 `init.lua` 中的
`vistty.config` 表声明，而非命令行参数。默认配置路径为
`~/.config/vistty/init.lua`，可用 `-config` 指定自定义路径。仓库附带带注释的示例
[`examples/init.lua`](./examples/init.lua)：

```lua
vistty.config = {
    backend   = "auto",       -- auto / wayland / drm / drm-gbm
    shell     = "/bin/bash",
    font      = "",           -- 外部字体路径；为空时用内置字体
    fallback_font = "",       -- NerdFont fallback；设为 "" 禁用
    fontsize  = 14,
    primary   = "",           -- 如 "HDMI-A-1"
    error_log = "",           -- 设置后默认 ~/.local/share/vistty/error.log
    theme     = require("themes.gruvbox"),
}
```

内置主题预设位于 [`examples/themes/`](./examples/themes)：
`dracula`、`solarized_dark`、`solarized_light`、`gruvbox`、`monokai`、`nord`、
`one_dark`，以及默认的 `xterm`。

### 快捷键

mod 键按后端自适应：Wayland 用 `Alt`，DRM 用 `Super`。默认绑定（见
`examples/init.lua`）：

- Mod + `=` / Mod + `-` / Mod + `0`：放大 / 缩小 / 重置字号
- Mod + `T` / Mod + `W`：新建标签 / 关闭标签
- Mod + `Tab`：下一个标签
- Mod + `1..9`：切换到第 N 个标签
- Mod + `←` / Mod + `→`：上一屏 / 下一屏
- Mod + `R`：热重载 `init.lua`
- Mod + `Q`：退出
- `Ctrl` + `Space`：切换拼音输入法

## 底层支持

| 关注点       | 方案                                                              |
|--------------|-------------------------------------------------------------------|
| DRM/KMS      | 项目内 `ioctl` 封装（参考 NeowayLabs/drm）                        |
| 帧缓冲       | DRM dumb buffer + CPU 渲染；经 `purego` 的 GBM/EGL/GLES 路径      |
| 输入         | `holoplot/go-evdev` + inotify 热插拔（DRM）；内置 XKB keymap（Wayland） |
| PTY          | `creack/pty`                                                      |
| 转义解析     | 项目内 VTE 状态机（xterm-256 兼容）                               |
| 终端缓冲区   | 项目内 Cell/Line/Buffer                                           |
| 字体         | `golang.org/x/image/font/opentype` + glyph atlas 缓存 + 内置 emoji 子集 |
| 文本整形     | 暂未引入（预留 `go-text/typesetting/harfbuzz`）                   |
| Wayland 协议 | 纯 Go `wl.go` wire 协议层（零 CGO）                               |
| 插件 VM      | `gopher-lua`（Lua 5.1）                                           |
| 拼音词库     | `go:embed` rime-ice，紧凑索引 + 二分查找                          |

## 协议

Vistty 基于 [GNU 通用公共许可证 v2 或更高版本](./LICENSE) 授权。
