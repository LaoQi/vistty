# Vistty — Agent 笔记

## 项目
基于 DRM/KMS 的虚拟终端仿真器（功能类似 kmscon），直接运行在 Linux DRM/KMS 上，无需 X11 或 Wayland。同时支持 Wayland 窗口后端用于开发和调试。

## 语言与构建
- Go，**CGO_ENABLED=0** — 禁止任何 C 互操作。所有原生依赖必须通过 syscall/ioctl 封装或纯 Go 库实现。
- 目标平台：仅 **linux/amd64**
- 模块路径：`github.com/LaoQi/vistty`

## 模块与选型

| # | 模块 | 选型 | 说明 |
|---|------|------|------|
| 1 | DRM/KMS 后端 | 参考 NeowayLabs/drm 自研 | ioctl 封装模式按需实现 |
| 2 | 帧缓冲管理 | 自研抽象接口 | DRM dumb buffer CPU 渲染 + GBM GPU 渲染 |
| 3 | 输入处理 | `holoplot/go-evdev` | 纯 Go，含 Grab 独占、uinput |
| 4 | PTY 管理 | `creack/pty` | Go 生态标准方案，纯 Go |
| 5 | 转义序列解析 | 自研 | 参考 go-vte 状态机 + darktile termutil |
| 6 | 终端缓冲区 | 自研 | 参考 darktile termutil Cell/Line/Buffer |
| 7 | 字体解析+光栅化 | `golang.org/x/image/font/opentype` | 内置 Sarasa Fixed SC 子集（等宽+CJK） |
| 8 | 文本整形 | 初期不引入 | 后续按需引入 go-text/typesetting/harfbuzz |
| 9 | 渲染合成 | 自研渲染管线 | glyph cache + double buffer + GPU instanced draw |
| 10 | Wayland 窗口后端 | 自研纯 Go 协议层 | wl.go 实现 Wayland wire 协议最小子集，零 CGO |
| 11 | OSD 四边 UI 层 | 自研 | 顶部多终端标签栏 + 底/左/右插件面板；实现 render.Overlay 接口 |
| 12 | 插件系统 | `gopher-lua` | Lua 5.1 VM；init.lua 驱动配置+钩子；分层命名空间 API |

## 架构

### 分层

```
cmd/vistty (入口，-backend 选择后端 + -config 指定 init.lua + PluginManager 注入)
    └── session (协调层：枚举输出 + 焦点路由 + 渲染编排 + 缩放热键 + 标签管理)
            ├── terminal (纯逻辑会话：PTY + screen + parser + CSI 执行)
            │       ├── vte (转义解析)
            │       └── screen (缓冲区)
            ├── session.Slave (输出绑定：surface + compositor + terms[] + osd)
            ├── plugins (Lua VM + 钩子暂存/激活 + PluginContext 接口)
            └── render (合成+光标+overlay 扩展) → font (opentype + glyph cache)
                    ├── ui (OSD 标签栏 + 插件面板，实现 render.Overlay)
                    └── 依赖 platform.Surface 接口
                            ├── platform/drm (DRM 直出，多 connector 多屏)
                            └── platform/wayland (Wayland 窗口，单虚拟输出)
```

### 核心接口

```go
// platform/surface.go
type Surface interface {
    Size() (width, height int)
    Data() []byte           // 帧缓冲像素数据（BGRA32）
    Stride() int
    Swap() error
    Close() error
    ResizeEvents() <-chan ResizeEvent
    OutputID() uint32
    DirectRender() bool     // 堆/memfd 内存 true；dumb mmap 设备内存 false
}

// platform/output.go
type Output interface {
    ID() uint32             // connector ID
    ConnectorID() uint32
    CrtcID() uint32
    Name() string           // 如 "HDMI-A-1"
    Size() (int, int)       // mode 分辨率
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
    CreateSurfaceFor(out Output) (Surface, error)
    ListOutputs() ([]Output, error)
    CreateInputSource() (InputSource, error)
    Run(func())
    Done() <-chan struct{}
    Stop() error
    Close() error
}
```

### 两种后端对比

| | DRM 后端 | Wayland 后端 |
|---|---------|-------------|
| Surface | Dumb buffer mmap / GBM BO + Page Flip | wl_shm 共享内存 + wl_surface.commit |
| InputSource | go-evdev 读 /dev/input/eventN | wl_keyboard + wl_pointer 事件 |
| 键盘映射 | 自研 scancode→Unicode | 简化 XKB keymap 解析 |
| 窗口管理 | CRTC/Connector 全屏 | XDG Shell 窗口 + SSD 标题栏 |
| VT 切换 | SIGUSR1/2 + KD_GRAPHICS | 不适用 |
| GPU 渲染 | GBM+EGL+GLES instanced draw | 不支持（wl_shm CPU 路径） |

### 数据流

```
PTY stdout → vte.Parser → []Sequence → screen.Buffer 操作
                                                              ↓
输入事件 ← InputSource → terminal → PTY stdin           render.Compositor
                                                              ↓
                                               font.Atlas → alpha混合 → backBuf
                                                              ↓
                                               OSD.RenderCPU/RenderGPU 叠加标签栏
                                                              ↓
                                               backBuf → Surface.Data() → Surface.Swap()
```

### Goroutine 模型

| goroutine | 职责 |
|-----------|------|
| main | Run() LockOSThread，渲染主循环（seqCh/ticker.Render/resize/scale/tabReq/exit） |
| backend-loop | backend.Run()（DRM: 空操作; Wayland: dispatch 事件循环） |
| pty-read | PTY stdout → Read → FeedInto → seqCh |
| seq-relay | Terminal.SeqCh() → unifiedSeqCh 中继 |
| exit-watch | Terminal.EofCh()/Done() → m.exitCh |
| input | InputSource 事件 → terminal |
| signal | SIGINT/SIGTERM/SIGHUP/SIGQUIT → Close() |
| drm-event | DRM fd 事件（Page Flip 完成）— 仅 DRM |
| vt-signal | SIGUSR1/2 VT 切换 — 仅 DRM |

### 退出路径

| 触发源 | 路径 |
|--------|------|
| 信号 | signalLoop → signalClose() → wg.Wait() → backend.Stop() → input.Close() → cleanup() |
| Wayland 窗口关闭 | toplevel close → backend.Done() → signalClose() → 两阶段关闭 |
| PTY 退出 | exit-watch → handleTermExit 移除标签 → 无剩余 terminal 时 signalClose() |
| Close() 幂等 | sync.Once 保护，重复调用安全 |

两阶段关闭：signalClose() 只关 done+pty（不触碰 Wayland 对象）→ backend.Stop() 解锁 Run() → 安全销毁

### 包目录结构

```
github.com/LaoQi/vistty/
├── cmd/vistty/main.go          # 入口
├── session/                     # 协调层
│   ├── master.go                # Master + 标签生命周期 + PluginContext 实现
│   ├── slave.go                 # Slave 输出绑定 + OSD 联动
│   ├── render_loop.go           # 主循环 + handleKey/Resize/Scale + 插件 OnRender
│   ├── keybind.go               # KeybindTable/ResolvedKeybind
│   └── master_test.go
├── terminal/
│   ├── terminal.go              # 纯逻辑会话：PTY + screen + parser + CSI/ESC/Control
│   ├── charset.go               # G0/G1/GL + DEC line drawing
│   ├── options.go               # Options + OnTitle/OnDefaultColor 回调
│   ├── render_harness.go        # 性能测量桥接 API
│   └── rune_width.go            # ASCII 快路径 + x/text/width
├── internal/
│   ├── debug/                   # Debugf/Errorf/Warningf + 环境变量/文件配置
│   ├── plugins/                 # gopher-lua VM + init.lua + vistty.* API
│   │   ├── manager.go           # PluginManager + 钩子暂存/激活
│   │   ├── context.go           # PluginContext 接口 + TabInfo
│   │   ├── config.go            # RunConfig + readConfig + toKeybindTable
│   │   └── api_*.go             # vistty.input/term/tab/screen/zoom/ui API
│   ├── vte/                     # 转义序列解析器（xterm-256 兼容）
│   │   ├── parser.go / csi.go / osc.go / esc.go / control.go / sgr.go
│   ├── screen/                  # cell.go / line.go / buffer.go / history.go / cursor.go / selection.go
│   ├── font/                    # face.go / atlas.go / metrics.go / embedded.go / cache.go + assets/
│   ├── render/                  # compositor.go / draw.go / cursor.go / overlay.go
│   ├── ui/                      # osd.go (OSD + Tab + Config + PanelPrimitive + Render)
│   ├── perf/replay/             # 三级归因 benchmark
│   └── platform/
│       ├── surface.go / output.go / input.go / backend.go / keymap.go
│       ├── drm/                 # DRM/KMS 后端（ioctl/codes/types/device/master/mode/dumb/flip/mmap/event/atomic/property/plane/cap）
│       ├── gbm/                 # GBM GPU 后端（device/surface/atomic/gbm + purego dlopen）
│       ├── gl/                  # GLES + EGL purego dlopen
│       ├── gpu/                 # GPU instanced draw 核心（renderer/shader/atlas，后端无关）
│       └── wayland/             # Wayland 后端（backend/surface/input/keymap + 自研 wl.go）
├── examples/init.lua            # 插件示例配置
└── work_docs/                   # 开发过程文档
```

### 依赖方向

```
cmd/vistty → terminal, plugins, debug, platform/gbm (GBM 组装注入), ui
terminal → screen, vte, render, platform, debug
session → render, font, platform, terminal, ui, plugins (PluginContext 接口), debug
plugins → terminal, platform, ui, debug（不依赖 session，通过 PluginContext 依赖倒置）
render → font, platform (Surface 接口)
ui → render (Overlay/GlyphProvider/GPUGlyphUploader 接口), font, platform
platform/drm → platform (Surface/GBMProvider 接口), go-evdev, debug
platform/gbm → platform (GBMProvider/Surface/Output), platform/gl, platform/gpu, debug
platform/gpu → platform/gl, platform (CellInstance/GPURenderer), debug
platform/gl → purego
platform/wayland → 无外部依赖（自研 wl.go）
screen, vte → 无外部依赖
font → golang.org/x/image/font/opentype
plugins → gopher-lua
debug → 无内部依赖
```

**依赖规则：** `drm` 不依赖 `gbm`（GBMProvider 接口由 cmd/vistty 注入）；`render` 不依赖 `ui`（Overlay 接口依赖倒置）；`plugins` 不依赖 `session`（PluginContext 接口依赖倒置）。

## 关键约束
- CGO_ENABLED=0：所有内核接口通过 syscall/unix ioctl 或纯 Go 库实现
- DRM 是内核 UAPI — 基于 ioctl，需精确匹配 C 结构体内存布局
- GBM 是 Mesa 用户空间库 — 通过 purego dlopen 访问，非 ioctl
- DRM Page Flip 回调必须在同一线程 — 所有 DRM 操作集中在 drm-event goroutine
- 键盘映射：DRM 自研 scancode→Unicode，Wayland 自研 XKB keymap 解析，初期仅 US 布局

## 预留扩展点

| 扩展点 | 位置 | 方式 |
|--------|------|------|
| 硬件光标 | `platform/drm/` | DRM Plane ioctl |
| 文本整形 | `font/shaper.go` | 集成 go-text/typesetting/harfbuzz |
| Sixel 图形 | `vte/sixel.go` | 扩展 Parser DCS 处理 |
| X11 窗口后端 | `platform/x11/` | 新增 Backend 实现 |
| 完整 XKB 支持 | `platform/wayland/keymap.go` | 可选 purego dlopen libxkbcommon.so |
| 主题/配色 | `screen/` Color 类型 | 预定义主题+用户自定义 |

## 参考项目
- **NeowayLabs/drm** — 纯 Go DRM ioctl 封装参考
- **liamg/darktile** — 终端仿真器，termutil 的 ANSI/CSI/OSC/Sixel 解析和 Cell/Buffer 实现参考
- **danielgatis/go-vte** — VTE 风格转义序列解析器
- **holoplot/go-evdev** — 纯 Go evdev 库
- **s-rah/nyctal** / **fntlnz/godisplay** — 纯 Go Wayland 合成器/DRM 后端参考

## 开发命令

```bash
go build ./...       # 构建全部
go vet ./...         # 静态分析
go test ./...        # 运行测试
```

性能评估：
```bash
go test -run=^$ -bench=BenchmarkLayers -benchmem -benchtime=5s ./internal/perf/replay/
./vistty -backend wayland -cpuprofile wl.prof -memprofile wl.mem -fps 2>fps.log
./vistty -backend drm -cpuprofile drm.prof -fps 2>fps.log
./vistty -record session.bin
```

调试日志：
```bash
VISTTY_DEBUG=1 ./vistty -backend drm
VISTTY_DEBUG=1 VISTTY_DEBUG_FILE=/tmp/vistty.log ./vistty -backend drm
VISTTY_DEBUG=1 VISTTY_DEBUG_FILE=/tmp/vistty.log VISTTY_DEBUG_STDERR=0 ./vistty -backend drm
```

运行：
```bash
go run ./cmd/vistty                         # 自动探测后端（drm-gbm → drm → wayland）
go run ./cmd/vistty -backend drm            # DRM/KMS dumb buffer（CPU 渲染）
go run ./cmd/vistty -backend drm-gbm        # DRM/KMS GBM/EGL GPU 加速
go run ./cmd/vistty -backend wayland        # Wayland 窗口（开发调试）
go run ./cmd/vistty -backend drm -tty 2     # 绑定 tty2
go run ./cmd/vistty -config ~/my-init.lua   # 指定自定义 init.lua
go run ./cmd/vistty -list-outputs           # 列出所有输出设备
go run ./cmd/vistty -primary HDMI-A-1       # 指定主屏
```

## 已实现功能概要

- DRM/KMS dumb buffer CPU 渲染 + GBM/EGL/GLES GPU instanced draw 渲染
- Wayland wl_shm CPU 渲染后端（自研 wl.go 协议层，含 SSD 窗口装饰）
- 自动后端探测（drm-gbm → drm → wayland）
- xterm-256 兼容转义序列（CSI/OSC/ESC/SGR，含 OSC 10/11 默认颜色）
- CJK 双宽字符 + scroll region 感知换行 + alternate screen + deferred wrap
- 内置 Sarasa Fixed SC 字体 + FaceCache 缩放优化（6-72pt）
- GPU glyph atlas + instanced draw shader（GLES 3.00）
- 多屏 DRM 输出 + 独立显示模式 + 主屏选择
- OSD 标签栏 + 多终端标签（Mod+T/W/左右切换）
- 插件系统（gopher-lua init.lua + vistty.* API + 面板渲染 + 热重载）
- 动态缩放（Mod+=/-/0）+ dirty 跳帧 + 光标时间戳闪烁
- 错误日志文件（~/.local/share/vistty/error.log）
- VT 管理 + TTY 绑定 + SIGKILL 子进程退出死锁修复
- 两阶段关闭 + Close 幂等 + 渲染错误容错
