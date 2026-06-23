# Vistty — Agent 笔记

## 项目
基于 DRM/KMS 的虚拟终端仿真器（功能类似 kmscon），直接运行在 Linux DRM/KMS 上，无需 X11 或 Wayland。同时支持 Wayland 窗口后端用于开发和调试。

## 语言与构建
- Go，**CGO_ENABLED=0** — 禁止任何 C 互操作。所有原生依赖（DRM、Wayland、输入设备）必须通过 syscall/ioctl 封装或纯 Go 库实现。
- 目标平台：仅 **linux/amd64**（DRM/KMS 为 Linux 专属）。
- 模块路径：`github.com/LaoQi/vistty`

## 模块与选型

| # | 模块 | 选型 | 说明 |
|---|------|------|------|
| 1 | DRM/KMS 后端 | 参考 NeowayLabs/drm 自研 | 参考其 ioctl 封装模式按需实现，避免历史包袱 |
| 2 | 帧缓冲管理 | 自研抽象接口 | 初期用 DRM dumb buffer + CPU 渲染，接口预留 GBM 扩展 |
| 3 | 输入处理 | `holoplot/go-evdev` | 纯 Go，含 Grab 独占、uinput、活跃维护 |
| 4 | PTY 管理 | `creack/pty` | Go 生态标准方案，纯 Go，★2046 |
| 5 | 转义序列解析 | 自研 | 参考 danielgatis/go-vte 状态机 + liamg/darktile termutil 的 CSI/OSC/Sixel 实现 |
| 6 | 终端缓冲区 | 自研 | 参考 liamg/darktile termutil 的 Cell/Line/Buffer 架构 |
| 7 | 字体解析+光栅化 | `golang.org/x/image/font/opentype` | Go 官方扩展库，等宽字体够用 |
| 8 | 文本整形 | 初期不引入 | 后续按需引入 `go-text/typesetting/harfbuzz`（HarfBuzz 完整移植） |
| 9 | 渲染合成 | 自研渲染管线 | glyph cache + dirty tracking + double buffer |
| 10 | Wayland 窗口后端 | `rajveermalviya/go-wayland` | 纯 Go 无 CGO，30+ 协议，XDG Shell 完整 |

## 架构

### 分层

```
cmd/vistty (入口，选择后端)
    └── terminal (胶水层)
            ├── vte (转义解析)
            ├── screen (缓冲区)
            ├── font (opentype + glyph cache)
            └── render (合成+脏区域+光标)
                    └── 依赖 platform.Surface 接口
                            ├── platform/drm (DRM 直出)
                            └── platform/wayland (Wayland 窗口)
```

### 核心接口

```go
// platform/surface.go
type Surface interface {
    Size() (width, height int)
    Data() []byte           // 帧缓冲像素数据（BGRA32）
    Stride() int            // 行字节数
    Swap() error            // 提交当前帧
    Close() error
}

// platform/input.go
type InputSource interface {
    KeyEvents() <-chan KeyEvent
    MouseEvents() <-chan MouseEvent
    Close() error
}

// platform/backend.go
type Backend interface {
    CreateSurface(width, height int) (Surface, error)
    CreateInputSource() (InputSource, error)
    Run(func())             // 事件循环
    Done() <-chan struct{}  // 后端关闭通知（Wayland窗口关闭等）
    Close() error
}
```

### 两种后端对比

| | DRM 后端 | Wayland 后端 |
|---|---------|-------------|
| Surface | Dumb buffer mmap + Page Flip | wl_shm 共享内存 + wl_surface.commit |
| InputSource | go-evdev 读 /dev/input/eventN | wl_keyboard + wl_pointer 事件 |
| 键盘映射 | 自研 scancode→Unicode | 简化 XKB keymap 解析 |
| 窗口管理 | CRTC/Connector 全屏 | XDG Shell 窗口（可调整大小） |
| VT 切换 | SIGUSR1/2 + KD_GRAPHICS | 不适用 |
| 双缓冲 | 2 个 dumb buffer + Page Flip | 2 个 wl_shm buffer + commit |

### 数据流

```
PTY stdout → vte.Parser → []Sequence → screen.Buffer 操作 → 标记脏区域
                                                              ↓
输入事件 ← InputSource.KeyEvents() → terminal 处理 → PTY stdin   render.Compositor
                                                                    ↓
                                                    font.Atlas → alpha混合 → Surface.Data()
                                                                        ↓
                                                                 Surface.Swap()
```

### Goroutine 模型

| goroutine | 职责 | 两种后端差异 |
|-----------|------|-------------|
| main | Run() 阻塞等待 done/backend.Done() | 无差异 |
| backend-loop | backend.Run() | DRM: 空操作; Wayland: dispatch 事件循环 |
| pty-read | PTY stdout → parser → screen | 无差异 |
| input | InputSource 事件 → terminal | 无差异（接口统一） |
| render | 渲染循环 → compositor → Surface.Swap | 无差异 |
| signal | SIGINT/SIGTERM/SIGHUP/SIGQUIT → Close() | 无差异 |
| drm-event | DRM fd 事件（Page Flip 完成） | 仅 DRM |
| vt-signal | SIGUSR1/2 VT 切换 | 仅 DRM |

### 退出路径

| 触发源 | 路径 |
|--------|------|
| SIGINT/SIGTERM/SIGHUP/SIGQUIT | signalLoop → Close() → close(done) → Run() 返回 → defer backend.Close() |
| Wayland 窗口关闭 | toplevel.SetCloseHandler → backend.notifyClose() → close(doneCh) → Run() select 感知 backend.Done() → Close() |
| PTY 退出 | ptyReadLoop → close(done) → Run() 返回 → defer 链 |
| Close() 幂等 | sync.Once 保护，重复调用安全 |

### 包目录结构

```
github.com/LaoQi/vistty/
├── cmd/vistty/main.go
├── terminal/
│   ├── terminal.go
│   └── options.go
├── internal/
│   ├── vte/                    # 转义序列解析器
│   │   ├── parser.go
│   │   ├── csi.go / osc.go / esc.go / control.go / sgr.go
│   ├── screen/                 # 终端屏幕缓冲区
│   │   ├── cell.go / line.go / buffer.go / history.go / cursor.go / selection.go
│   ├── font/                   # 字体管理
│   │   ├── face.go / atlas.go / metrics.go
│   ├── render/                 # 渲染合成
│   │   ├── compositor.go / draw.go / cursor.go
│   └── platform/               # 平台抽象层
│       ├── surface.go          # Surface 接口
│       ├── input.go            # InputSource 接口 + 事件类型
│       ├── backend.go          # Backend 接口
│       ├── drm/                # DRM/KMS 后端
│       │   ├── backend.go / surface.go / input.go / display.go / vt.go
│       │   └── internal/       # DRM 底层（不对外暴露）
│       │       ├── ioctl.go / codes.go / types.go
│       │       ├── device.go / master.go / mode.go / dumb.go
│       │       ├── flip.go / mmap.go / event.go
│       │       ├── atomic.go / property.go / plane.go  (预留)
│       └── wayland/            # Wayland 后端
│           ├── backend.go / surface.go / input.go
│           ├── window.go / shm.go / seat.go / keymap.go
```

### 依赖方向

```
cmd/vistty → terminal
terminal → screen, vte, render, platform
render → font, platform (Surface 接口)
platform/drm → platform/drm/internal (DRM ioctl), go-evdev
platform/wayland → go-wayland
screen, vte → 无外部依赖（纯逻辑）
font → golang.org/x/image/font/opentype
```

**依赖规则：** `platform/drm/internal` 不依赖任何其他内部包；`vte` 和 `screen` 无外部依赖；上层通过接口解耦，不依赖具体后端实现。

## 关键约束
- 禁用 CGO 意味着：不能使用 libdrm/libgbm/libevdev/libfreetype 的 cgo 封装。所有内核接口必须通过 `syscall` / `unix` 包的 ioctl 调用或纯 Go 重新实现访问。
- DRM 是内核 UAPI — 基于 ioctl，纯 Go 访问可行，但需要精确匹配 C 结构体内存布局。
- GBM 是 Mesa 用户空间库，不是内核 UAPI — 无法通过纯 ioctl 重写，因此初期用 DRM dumb buffer 替代。
- 终端直接渲染到帧缓冲 — DRM 模式无显示服务器参与，Wayland 模式通过共享内存提交。
- DRM Page Flip 回调必须在同一线程执行 — 所有 DRM 操作集中在 `drm-event` goroutine。
- 键盘映射：DRM 模式自研 scancode→Unicode，Wayland 模式自研简化 XKB keymap 解析。初期仅支持 US 键盘布局。

## 预留扩展点

| 扩展点 | 位置 | 方式 |
|--------|------|------|
| GBM 渲染后端 | `platform/drm/surface.go` | 新增 GBMSurface 实现 Surface |
| GPU 加速渲染 | `render/` | 新增 GPUCompositor |
| 硬件光标 | `platform/drm/` | DRM Plane ioctl |
| Atomic Modesetting | `platform/drm/internal/atomic.go` | 已预留文件 |
| 文本整形 | `font/shaper.go` | 集成 go-text/typesetting/harfbuzz |
| Sixel 图形 | `vte/sixel.go` | 扩展 Parser DCS 处理 |
| 多终端/Tab | `terminal/` | Terminal 工厂+切换逻辑 |
| X11 窗口后端 | `platform/x11/` | 新增 Backend 实现 |
| 完整 XKB 支持 | `platform/wayland/keymap.go` | 可选 purego dlopen libxkbcommon.so |
| 配置系统 | `terminal/options.go` | 配置文件+命令行参数 |
| 主题/配色 | `screen/` Color 类型 | 预定义主题+用户自定义 |

## 参考项目
- **NeowayLabs/drm** — 纯 Go DRM ioctl 封装，基础 modesetting 可用但缺少 Page Flip/Atomic/Master
- **liamg/darktile** — GPU 终端仿真器（已废弃，依赖 CGO），其 `termutil` 包含完整的 ANSI/CSI/OSC/Sixel 解析和 Cell/Buffer 实现，是转义序列解析和终端缓冲区的最佳参考
- **danielgatis/go-vte** — VTE 风格转义序列解析器，接近硬件级精度
- **holoplot/go-evdev** — 纯 Go evdev 库，含 Grab/uinput
- **rajveermalviya/go-wayland** — 纯 Go Wayland 客户端协议绑定，30+ 协议，XDG Shell 完整
- **s-rah/nyctal** / **fntlnz/godisplay** — 纯 Go Wayland 合成器，DRM 后端实现可参考

## 开发命令

```bash
go build ./...       # 构建全部
go vet ./...         # 静态分析
go test ./...        # 运行测试
```

运行：
```bash
go run ./cmd/vistty -backend drm       # DRM/KMS 直出
go run ./cmd/vistty -backend wayland   # Wayland 窗口（开发调试）
```

## 实施状态

全部三个阶段已完成：

- ✅ 阶段1：底层模块（drm/internal, platform接口, vte, screen）
- ✅ 阶段2：中间层模块（font, render, drm后端）
- ✅ 阶段3：上层模块（wayland后端, terminal胶水层, cmd/vistty入口）
- ✅ 阶段1审计与修复完成
- ✅ 外部关闭信号处理完善（Close幂等化、PTY回收、Wayland窗口关闭、信号扩展、统一退出路径）

Wayland 后端实现细节：
- 4个文件：backend.go（连接+全局绑定+事件循环）、surface.go（双缓冲wl_shm+XDG toplevel）、input.go（wl_keyboard/wl_pointer+修饰键跟踪）、keymap.go（简化XKB keymap解析+US布局回退）
- 使用 memfd_create + mmap 创建共享内存
- 支持窗口resize（Configure事件驱动重新分配buffer）
- XDG toplevel 关闭事件通过 backend.Done() 通知 terminal 退出

待完善：
- font 包缺少测试文件
- Wayland 后端无自动化测试（需 Wayland 合成器环境）
