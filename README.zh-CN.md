# Vistty

[English](./README.md) | **简体中文**

> **提醒：这是一个 vibe 产品。** 出于乐趣、实验和学习目的而构建，并非生产可用软件。请接受粗糙的边缘、缺失的功能和随时可能发生的破坏性变更，风险自负。

Vistty 是一个直接运行在 Linux DRM/KMS 子系统上的虚拟终端仿真器，无需 X11 或
Wayland 显示服务器，定位类似
[kmscon](https://www.freedesktop.org/wiki/Software/KMScon/)。

它通过 DRM/KMS 渲染：既可用 dumb buffer（CPU 路径），也可经 Atomic Modesetting
使用 GBM/EGL/GLES GPU 加速，并支持多屏独立显示模式。同时附带一个纯 Go 的
Wayland 窗口后端，用于在桌面会话内开发与调试。整个项目为纯 Go 且禁用 CGO
（`CGO_ENABLED=0`）——所有原生接口（DRM、GBM、EGL、GLES、evdev、Wayland）均经
`syscall`/`ioctl` 或 `purego` dlopen 访问。

在手写 xterm-256 兼容转义解析器之上，Vistty 提供 CJK 双宽渲染、内置 Sarasa CJK
字体 + NerdFont fallback + 自合成 Block Elements、彩色 Emoji、中文拼音输入法、
由 `init.lua` 驱动的 Lua 插件系统、带多种预设的主题系统，以及支持多标签的
OSD 标签栏。

## 文档

设计文档、各功能实施记录、性能/内存分析及已归档（被取代）的设计笔记位于
[`work_docs/`](./work_docs/README.md)，分为 `design/`、`implementation/`、
`analysis/`、`archive/`、`test-data/` 五类。

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
