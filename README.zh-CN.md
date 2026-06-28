# Vistty

> **提醒：这是一个 vibe 产品。** 出于乐趣、实验和学习目的而构建，并非生产可用软件。请接受粗糙的边缘、缺失的功能和随时可能发生的破坏性变更，风险自负。

Vistty 是一个直接运行在 Linux DRM/KMS 子系统上的虚拟终端仿真器，无需 X11 或
Wayland 显示服务器，定位类似
[kmscon](https://www.freedesktop.org/wiki/Software/KMScon/)。同时附带一个 Wayland
窗口后端，用于在桌面会话内开发与调试。

## 功能

- DRM/KMS 直出：`drm` 使用 dumb buffer + page flip（CPU 渲染），`drm-gbm`
  通过 Atomic Modesetting 使用 GBM/EGL/GLES GPU 加速。自动探测按
  `drm-gbm` → `drm` → `wayland` 顺序尝试。
- Wayland 窗口后端：自研纯 Go Wayland wire 协议层（无外部 Wayland 绑定），通过
  `wl_shm` 共享内存提交帧。
- 多屏支持：枚举所有已连接的 connector；支持镜像/独立两种显示模式；可按名称或
  索引选择主屏。
- 内置字体：Sarasa Fixed SC 子集（等宽 + CJK），经
  `golang.org/x/image/font/opentype` 光栅化并配合 glyph atlas 缓存；支持实时
  缩放（Super + `=` / `-` / `0`）。
- xterm-256 兼容 VT：手写 9 状态转义序列解析器，覆盖 CSI/OSC/ESC/DCS，含备用屏、
  滚动区域、DEC line drawing、括号粘贴、焦点上报、动态光标样式、OSC 10/11 颜色
  查询与设置、CJK 双宽渲染。
- 纯 Go，禁用 CGO（`CGO_ENABLED=0`）：所有原生接口（DRM、GBM、EGL、GLES、
  evdev、Wayland、opentype）均经 `syscall`/`ioctl` 或 `purego` dlopen 访问，构建
  无需 C 工具链。
- TTY 绑定、VT 切换（SIGUSR1/2）、优雅退出、pprof/trace 性能采集，以及 PTY 会话
  录制用于离线回放基准测试。

## 构建

需 Go 环境（模块声明 `go 1.26.4`），目标平台 `linux/amd64`：

```bash
go build ./...
go vet ./...
go test ./...
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
```

### 常用参数

| 参数            | 说明                                                          |
|-----------------|---------------------------------------------------------------|
| `-backend`      | `auto`（默认）、`wayland`、`drm` 或 `drm-gbm`                   |
| `-shell`        | 运行的 shell（默认 `/bin/bash`）                              |
| `-font`         | 外部字体文件路径（为空时用内置字体）                          |
| `-fontsize`     | 字号，单位像素（默认 14）                                     |
| `-mode`         | `mirror` 或 `independent`（默认 `independent`）              |
| `-primary`      | 按 connector 名称（如 `HDMI-A-1`）或索引选择主屏             |
| `-tty`          | 绑定到指定 TTY，如 `2` 或 `/dev/tty2`（仅 DRM）               |
| `-list-outputs` | 列出所有显示输出后退出                                        |
| `-cpuprofile`   | 输出 CPU profile 到文件                                       |
| `-memprofile`   | 输出堆 profile 到文件                                         |
| `-trace`        | 输出执行 trace 到文件                                         |
| `-fps`          | 向 stderr 打印每帧耗时                                        |
| `-record`       | 录制 PTY 输出到文件，用于离线回放                             |

### 快捷键

- Super + `=` / Super + `-` / Super + `0`：放大 / 缩小 / 重置字号
- Super + `1..9`：切换焦点到第 N 块屏（独立模式）
- Super + Tab：在多屏间轮转焦点（独立模式）

## 底层支持

| 关注点       | 方案                                                              |
|--------------|-------------------------------------------------------------------|
| DRM/KMS      | 自研 `ioctl` 封装（参考 NeowayLabs/drm）                          |
| 帧缓冲       | DRM dumb buffer + CPU 渲染；经 `purego` 的 GBM/EGL/GLES 路径      |
| 输入         | `holoplot/go-evdev`（DRM）；自研 XKB keymap（Wayland）            |
| PTY          | `creack/pty`                                                      |
| 转义解析     | 自研 VTE 状态机（xterm-256 兼容）                                 |
| 终端缓冲区   | 自研 Cell/Line/Buffer                                             |
| 字体         | `golang.org/x/image/font/opentype` + glyph atlas 缓存            |
| Wayland 协议 | 自研纯 Go `wl.go` wire 协议层（零 CGO）                          |

## 协议

Vistty 基于 [GNU 通用公共许可证 v2](./LICENSE) 授权。
