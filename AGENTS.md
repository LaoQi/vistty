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
| 7 | 字体解析+光栅化 | `golang.org/x/image/font/opentype` | Go 官方扩展库，内置 Sarasa Fixed SC 子集（等宽+CJK） |
| 8 | 文本整形 | 初期不引入 | 后续按需引入 `go-text/typesetting/harfbuzz`（HarfBuzz 完整移植） |
| 9 | 渲染合成 | 自研渲染管线 | glyph cache + double buffer |
| 10 | Wayland 窗口后端 | `rajveermalviya/go-wayland` | 纯 Go 无 CGO，30+ 协议，XDG Shell 完整 |

## 架构

### 分层

```
cmd/vistty (入口，选择后端 + -mode/-primary 参数)
    └── master (协调层：枚举输出 + 焦点路由 + 渲染编排 + 缩放热键)
            ├── terminal (纯逻辑会话：PTY + screen + parser + CSI 执行)
            │       ├── vte (转义解析)
            │       └── screen (缓冲区)
            ├── slave (输出绑定：surface + backBuf + terms[] + active 预留)
            └── render (合成+光标) → font (opentype + glyph cache)
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
    Stride() int            // 行字节数
    Swap() error            // 提交当前帧
    Close() error
    ResizeEvents() <-chan ResizeEvent
    OutputID() uint32       // 绑定的输出 ID
}

// platform/output.go
type Output interface {
    ID() uint32             // connector ID，唯一标识
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
    CreateSurface(width, height int) (Surface, error)        // 兼容旧路径
    CreateSurfaceFor(out Output) (Surface, error)             // 按输出创建
    ListOutputs() ([]Output, error)                           // 枚举所有输出
    CreateInputSource() (InputSource, error)
    Run(func())             // 事件循环
    Done() <-chan struct{}  // 后端关闭通知（Wayland窗口关闭等）
    Stop() error            // 关闭事件循环连接（不销毁对象），解锁 Run() 退出
    Close() error           // 销毁所有后端资源
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
PTY stdout → vte.Parser → []Sequence → screen.Buffer 操作
                                                              ↓
输入事件 ← InputSource.KeyEvents() → terminal 处理 → PTY stdin   render.Compositor
                                                                    ↓
                                                    font.Atlas → alpha混合 → backBuf（离屏缓冲区）
                                                                        ↓
                                                        backBuf 全量拷贝 → Surface.Data()
                                                                        ↓
                                                                 Surface.Swap()
```

### Goroutine 模型

| goroutine | 职责 | 两种后端差异 |
|-----------|------|-------------|
| main | Run() LockOSThread 绑定，承载渲染主循环（seqCh/ticker.Render/resize/scale/eof），等 done/backend.Done() | 无差异 |
| backend-loop | backend.Run() | DRM: 空操作; Wayland: dispatch 事件循环 |
| pty-read | PTY stdout → Read 长循环 → FeedAll → seqCh | 无差异 |
| input | InputSource 事件 → terminal | 无差异（接口统一） |
| signal | SIGINT/SIGTERM/SIGHUP/SIGQUIT → Close() | 无差异 |
| drm-event | DRM fd 事件（Page Flip 完成） | 仅 DRM |
| vt-signal | SIGUSR1/2 VT 切换 | 仅 DRM |

### 退出路径

| 触发源 | 路径 |
|--------|------|
| SIGINT/SIGTERM/SIGHUP/SIGQUIT | signalLoop → signalClose()（只关 done+pty）→ wg.Wait() → backend.Stop() → input.Close() → cleanup() |
| Wayland 窗口关闭 | toplevel.SetCloseHandler → backend.notifyClose() → close(doneCh) → Run() select 感知 backend.Done() → signalClose() → 两阶段关闭 |
| PTY 退出 | ptyReadLoop → eofCh/done → signalClose() → 两阶段关闭 |
| 两阶段关闭 | 阶段1: signalClose() 只关 done+pty（不触碰 Wayland 对象）→ 阶段2: backend.Stop() 关连接解锁 Run() → 等待 backend.Run() 退出 → input.Close()+cleanup()（无并发 map 访问）|
| Close() 幂等 | sync.Once 保护 signalClose/cleanup/input.Close/backend.Stop，重复调用安全 |

### 包目录结构

```
github.com/LaoQi/vistty/
├── cmd/vistty/main.go          # 入口：-mode/-primary/-backend/-tty 参数 + profiling
├── master/                     # 协调层：枚举输出 + 焦点路由 + 渲染编排 + 缩放热键
│   ├── master.go               # Master 结构 + New(initMirror/initIndependent) + session池
│   ├── render_loop.go           # 统一主循环（镜像裁剪分发/独立串行）+ handleKey + setFocus
│   └── master_test.go          # 集成测试（Close幂等/PTY退出/输入无死锁）
├── slave/                      # 输出绑定：surface + backBuf + terms[] + active 预留
│   └── slave.go                # Slave 结构 + InitIndependent + ActiveTerm/BindTerminal
├── terminal/
│   ├── terminal.go             # 纯逻辑会话：PTY + screen + parser + CSI/ESC/Control 执行
│   ├── charset.go              # 字符集状态管理（G0/G1/GL + DEC line drawing 映射）
│   ├── options.go              # Options + Primary/Mode + OnTitle 回调 + RecordWriter
│   ├── render_harness.go       # 导出桥接 API：NewRenderHarness/FeedBytes/RenderFrame（性能测量用）
│   └── rune_width.go           # 宽度判断（ASCII 快路径 + x/text/width）
├── internal/
│   ├── vte/                    # 转义序列解析器（xterm-256 兼容）
│   │   ├── parser.go           # 状态机（9 状态 + privateMarker 分发）
│   │   ├── csi.go              # CSI 语义：privateMarker ?/>/=/</ + intermed SP/" 分发
│   │   ├── osc.go              # OSC 语义：0/1/2/7/8/10/11/52
│   │   ├── esc.go              # ESC 语义：G0/G1 字符集指定 ( ) * + + DECSC/RC
│   │   ├── control.go          # C0 控制字符识别
│   │   ├── sgr.go              # SGR 解析：22 → BoldOff+DimOff
│   ├── screen/                 # 终端屏幕缓冲区
│   │   ├── cell.go / line.go / buffer.go / history.go / cursor.go / selection.go
│   ├── font/                   # 字体管理
│   │   ├── face.go / atlas.go / metrics.go / embedded.go / cache.go
│   │   └── assets/             # 内置字体资源（Sarasa Fixed SC 子集 + LICENSE）
│   ├── render/                 # 渲染合成
│   │   ├── compositor.go / draw.go / cursor.go
│   ├── perf/                   # 性能评估工具
│   │   └── replay/             # 离线回放 + 三级归因 benchmark（L1 parser/L2 screen/L3 render）
│   │       ├── harness.go / genworkload.go / bench_test.go / embed.go
│   └── platform/               # 平台抽象层
│       ├── surface.go          # Surface 接口 + OutputID()
│       ├── output.go           # Output 接口（ID/ConnectorID/CrtcID/Name/Size）
│       ├── input.go            # InputSource 接口 + 事件类型 + Modifiers(ModSuper)
│       ├── backend.go          # Backend 接口 + ListOutputs/CreateSurfaceFor
│       ├── keymap.go           # 共享键映射函数（DRM/Wayland通用，左右Win ModSuper）
│       ├── drm/                # DRM/KMS 后端（多 connector 多屏）
│       │   ├── backend.go / surface.go / input.go / display.go / vt.go
│       │   └── internal/       # DRM 底层（不对外暴露）
│       │       ├── ioctl.go / codes.go / types.go
│       │       ├── device.go / master.go / mode.go / dumb.go
│       │       ├── flip.go / mmap.go / event.go
│       │       ├── atomic.go / property.go / plane.go  # Atomic Modesetting ioctl 封装
│       │       └── gbm/            # GBM + EGL purego dlopen（github.com/ebitengine/purego，跨架构支持 amd64/arm64）
│       │           ├── gbm.go / egl.go
│       └── wayland/            # Wayland 后端（单虚拟输出）
│           ├── backend.go / surface.go / input.go
│           ├── keymap.go / wire.go    # wire.go: 修正的 Wayland 线格式编码
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

性能评估：
```bash
# 离线三级归因 benchmark（L1 parser / L2 screen / L3 render）
go test -run=^$ -bench=BenchmarkLayers -benchmem -benchtime=5s ./internal/perf/replay/

# 在线 pprof（需显示后端）
./vistty -backend wayland -cpuprofile wl.prof -memprofile wl.mem -fps 2>fps.log
./vistty -backend drm -cpuprofile drm.prof -fps 2>fps.log
./vistty -record session.bin   # 录制 PTY 输出用于回放
```

运行：
```bash
go run ./cmd/vistty                         # 自动探测后端（DRM优先，回退Wayland）
go run ./cmd/vistty -backend drm            # 强制 DRM/KMS 直出
go run ./cmd/vistty -backend wayland        # 强制 Wayland 窗口（开发调试）
go run ./cmd/vistty -backend drm -tty 2     # 绑定 tty2（setsid+TIOCSCTTY 设控制终端）
```

## 实施状态

全部三个阶段已完成：

- ✅ 阶段1：底层模块（drm/internal, platform接口, vte, screen）
- ✅ 阶段2：中间层模块（font, render, drm后端）
- ✅ 阶段3：上层模块（wayland后端, terminal胶水层, cmd/vistty入口）
- ✅ 阶段1审计与修复完成
- ✅ 外部关闭信号处理完善（Close幂等化、PTY回收、Wayland窗口关闭、信号扩展、统一退出路径）
- ✅ 渲染双缓冲（Compositor离屏backBuf + 全量拷贝到Surface）
- ✅ Wayland 线格式修复（wire.go: 修正 PutString 字符串长度 bug + ShmFormat 枚举值协商）
- ✅ 两阶段关闭流程（signalClose 不触碰 Wayland 对象 → backend.Stop 解锁 Run → 安全销毁）
- ✅ ptyReadLoop 长周期化（单 goroutine 直接 Read→FeedAll→seqCh，消除每帧临时 goroutine 分配；done+pty.Close 让 Read 返回 err 退出）
- ✅ 渲染主线程化（Run() LockOSThread 绑定，eventLoop select 并入 main，wg.Add(3)；CGO=0 下保证渲染 goroutine 不被线程迁移）
- ✅ 强制初始渲染（Run() 启动前 Render 一次，确保 Wayland surface 被映射）
- ✅ VISTTY_DEBUG 环境变量调试日志
- ✅ 自动后端探测（DRM优先轻量探测 → 回退Wayland，-backend auto 默认）
- ✅ 内置 Sarasa Fixed SC 字体（子集化 6.7MB，等宽+CJK，无需系统字体）
- ✅ 按键长按支持（terminal 层 delay timer + rate ticker，DRM 过滤内核 autorepeat）
- ✅ 滚动状态下按键自动回到底部（非滚动键重置 scrollOffset 并发送到 PTY）
- ✅ CJK 双宽字符支持（x/text/width 判断 + 占位符机制 + 渲染双宽 + 光标双宽）
- ✅ 移除脏矩阵裁剪（每帧全量重绘，删除 dirty tracking 基础设施）
- ✅ 渲染坐标修复（Bold 阴影 Y 坐标 + 光标内字形基线对齐）
- ✅ DRM framebuffer 尺寸修复（CreateSurface 无条件使用 mode 分辨率，修复 SETCRTC ENOSPC）
- ✅ xterm-256 VT 兼容性改进（119 测试全通过）
- ✅ 输入处理修复（ESC 键经 Rune 路径发送 0x1b；Wayland keymap handler 实现；DRM 移除功能键事件丢弃；XKB keycode 与 evdev code 8 偏移修正）
- ✅ scroll region 感知换行（LF/IND/NEL/autoWrap 双宽换行 + RI 均按 scrollTop/scrollBot 触发 region 内滚动，修复 vim 备用屏幕+状态栏场景最后一行不滚动）
- ✅ alternate screen 规范化（altScreen 标记，ScrollUp 不 push scrollback；ClearAll 清 scrollback 用于 RIS/altBuf 切换，Clear 保留 scrollback 供 ED 2）
- ✅ 字体 Face 缓存（font.FaceCache：一次 opentype.Parse + 按 size 缓存 face，缩放回切同 size 零开销）
- ✅ 动态放大缩小（Mod+= 放大 / Mod+- 缩小 / Mod+0 重置，6-72pt 范围，重算行列数 + 同步 PTY winsize；原 Ctrl 改为 Win/Super 键）
- ✅ handleResize PTY winsize 同步修复（原 handleResize 漏调 pty.Setsize，窗口 resize 后 shell 不知情）
- ✅ glyph 位图缓存扩容（Atlas 4096 → 8192，减少 CJK 字符 LRU 淘汰）
- ✅ deferred wrap 精细化（SGR/charset designate/DSR/DA 等纯属性命令不再重置 WrapPending，仅光标移动/擦除/滚动/换行命令重置；修复 nvim 行尾字符后发 SGR 导致下一字符覆盖而非换行）
- ✅ 擦除区域保留当前 SGR 背景色（EL/ED/ECH/DCH/ICH/ScrollUp/ScrollDown 新行使用 curBg 填充而非 default 黑色，符合 xterm 规范）
- ✅ 性能评估基础设施（internal/perf/replay 三级归因 benchmark L1/L2/L3 + cmd/vistty pprof 集成 -cpuprofile/-memprofile/-trace/-mutexprofile/-fps/-record）
- ✅ 9 项性能优化（parser 预分配 -99.6% allocs / fillRect uint32 / history 所有权移交 / blendGlyph >>8 / atlas 读路径去锁 / copyAll 整块 / rune_width ASCII 快路径 / InsertLines 批量化 / swapBR uint32）
- ✅ 内存分配热点消除（Sequence.Params []int→[16]int+NParams 内嵌数组，删除 copyInts 堆分配，CSI allocs -99.7%；ParseSGR 预分配 cap=8；Parser.seqs 预分配 cap=256 消除 growslice，L1 解析提速 1.3-1.8x）
- ✅ master/slave 多屏架构（Terminal 简化为纯逻辑会话，剥离渲染/IO/主循环；master 协调层枚举输出+焦点路由+渲染编排；slave 输出绑定+terms[]预留tabs）
- ✅ 多屏 DRM 输出支持（findOutputs 返回所有 connected，DisplayInfo 实现 Output 接口；eventLoop 按 ev.CrtcID 路由 notifyFlip，修复多屏 flip 串扰）
- ✅ 镜像/独立双模式（-mode mirror|independent；镜像 master 集中渲染裁剪分发，独立每 slave 自持 compositor 串行渲染）
- ✅ 主屏选择参数（-primary <名称|索引>，按 connector name 如 HDMI-A-1 或数字索引匹配）
- ✅ Mod 键焦点路由（independent 模式 Mod+1..9 切焦点屏 / Mod+Tab 轮转；setFocus 投递 renderReqCh 主线程渲染避免并发）
- ✅ 右 Win 键支持（keymap.go 补 126:ModSuper，DRM 路径左右 Win 均识别）
- ✅ DRM Atomic Modesetting ioctl 封装（atomic.go/property.go/plane.go：AtomicCommit/GetObjectProperties/GetProperty/CreateBlob/GetPlaneResources/GetPlane + 8 结构体 + 9 ioctl 码 + 编译时大小校验）
- ✅ DRM Atomic Modesetting ioctl 封装（atomic.go/property.go/plane.go：AtomicCommit/GetObjectProperties/GetProperty/CreateBlob/GetPlaneResources/GetPlane + 8 结构体 + 9 ioctl 码 + 编译时大小校验）
- ✅ GBM + EGL purego dlopen（github.com/ebitengine/purego：Dlopen+Dlsym+RegisterFunc 替代自研汇编+ELF解析；跨架构支持 amd64/arm64；CGO=0 纯 Go 调用 C 库函数）
- ✅ GBMSurface + AtomicCommitor（GBMDevice 共享 gbm_device+EGLDisplay+EGLContext；GBMSurface 实现Surface Swap: eglSwapBuffers→lock_front_buffer→AddFB→CommitSingle→wait flipCh；AtomicCommitor 属性ID缓存+primary plane发现+多CRTC同步批提交）
- ✅ GBM 可选初始化与回退（backend.go：HasAtomic→NewGBMDevice 成功 useGBM=true，失败静默回退 dumb buffer；eventLoop 按 ev.CrtcID 路由 GBM surfaces）

字体缓存与缩放实现详情：
- `font.FaceCache`（cache.go）：缓存 `*opentype.Font`（全局唯一，一次 Parse）+ `map[size]*OpenTypeFace`（无上限，惰性创建）。缓存拥有 face 所有权，调用方只借用，退出时 `Close()` 统一释放
- `font.newFaceFromParsed`（face.go 抽取）：复用已 Parse 对象构造 face，避免 NewOpenTypeFace 每次重新 Parse
- `font.EmbeddedFontData()`（embedded.go）：暴露嵌入字体原始字节，供 FaceCache 共享单份拷贝
- `Compositor.SetFace`（compositor.go）：替换 face + 重建 Atlas(8192) + 更新 metrics（旧 face 不 Close，归 FaceCache 管）
- `Master.handleScale`（master/render_loop.go）：在主线程（渲染线程）执行（与 Render 同线程，无并发竞争）→ faceCache.Get → SetFace → 重算 cols/rows → Resize → SetPtySize → 立即 Render；镜像模式 master 全局 font，独立模式每 slave 独立 faceCache
- `scaleReqCh`（cap=1 非阻塞）：inputLoop 投递 → 主线程消费，rapid input 自动合并；select 含 `t.done` 防 goroutine 泄漏
- 旧 `NewOpenTypeFace` API 保留（向后兼容，测试使用）

scroll region 与换行语义：
- `Buffer.LineFeed()` 封装 IND/LF 语义：光标在 scrollBot 时 region 内 ScrollUp，否则下移；region 外底部钳住不动
- `Buffer.ReverseIndex()` 封装 RI 语义：光标在 scrollTop 时 region 内 ScrollDown，否则上移
- terminal 层 5 处换行（execPrint 双宽/autoWrap、execControl LF/VT/FF、ESCIndex、ESCNextLine）+ ESCReverseIndex 全部改用上述方法，消除原 `Rows()-1` 阈值忽略 scrollBot 的 bug
- `ScrollBot()` 访问器补齐结构性缺口（原仅有 ScrollTop，scrollBot 私有无法被 terminal 层读取）

xterm-256 VT 支持详情：
- 解析层(vte)：SGR 22 同时关闭 Bold+Dim；CSI q 按 intermed 区分 DECSCUSR(SP)/DECSCA(")/Unknown；新增 CSI 命令 X(ECH)/n(DSR)/c(DA1)/g(TBC)；私有标记 > 分发 DA2(>c)；ESC ( ) G0/G1 字符集指定；OSC 2 窗口标题
- 执行层(terminal)：hostWriter 响应回写通道；savedCursorState 完整状态保存（位置+SGR+字符集）；DECAWM(?7) 自动换行开关；DECCKM(?1) 应用光标键；?47/?1047/?1048 备用屏幕；?1004 焦点标志；?2004 括号粘贴标志；DECSCUSR 6 种光标样式含闪烁/稳定；DEC line drawing 字符集转换+SO/SI；tab stop 动态管理；ECH 擦除字符；ED case3 清 scrollback；DSR/DA1/DA2 响应回写；OSC 标题 OnTitle 回调
- DA1 响应：`CSI ?62;4c`（VT220 + SGR 颜色）
- DA2 响应：`CSI >0;0;0c`
- 测试架构：newTerminalForTest + feedBytes 无 IO 测试入口

Wayland 后端实现细节：
- 5个文件：backend.go（连接+全局绑定+事件循环+错误处理）、surface.go（双缓冲wl_shm+XDG toplevel）、input.go（wl_keyboard/wl_pointer+修饰键跟踪+keymap handler 解析）、keymap.go（XKB keymap 解析 + evdev code 索引 + US 布局回退）、wire.go（修正的Wayland线格式编码）
- 使用 memfd_create + mmap 创建共享内存
- 支持 shm format 协商（通过 wl_shm.format 事件取合成器首个支持的格式）；backBuf 恒以 BGRA 字节序写入，当选中 BGR 序格式（XBGR/ABGR/BGRX/BGRA8888）时 backend 置 swapBR 标志，surface.Swap() 提交前逐像素交换 B/R 通道修正颜色
- 支持窗口resize（Configure事件驱动重新分配buffer）
- XDG toplevel 关闭事件通过 backend.Done() 通知 terminal 退出
- Display.SetErrorHandler 捕获合成器协议错误，便于调试

go-wayland 库已知 bug（已在 wire.go 中修复）：
- PutString 写入 padded length 而非 actual length（含 NUL 终止符）到 uint32 长度字段
- ShmFormat 枚举常量使用顺序索引（0, 1）而非 DRM FourCC 码，但事件解析 Uint32 读原始字节得到的是真实 FourCC；故 backend.go 自定义 wlFmt* 裸 FourCC 常量，不用库常量
- wire.go 重新实现了 registryBind、toplevelSetTitle、toplevelSetAppId、shmCreatePool、shmPoolCreateBuffer、compositorCreateSurface、xdgWmBaseGetXdgSurface、xdgSurfaceGetToplevel，使用 encoding/binary 正确编码

待完善：
- font 包测试文件已添加（atlas_test.go, face_test.go）
- Wayland 后端无自动化测试（需 Wayland 合成器环境）
- ✅ 指定 TTY 绑定（-tty 参数：纯数字→/dev/ttyN、/dev/ 前缀原样；DRM 后端 setsid+TIOCSCTTY 设控制终端，Wayland 后端忽略并警告）
- ✅ VT 管理容错降级（tty 获取失败不报错退出，打印警告并跳过 VT 管理；SSH 远程无控制终端场景仍能 DRM 渲染到物理屏，仅无 VT 切换信号）
- ✅ GBM 绕过开关（-nogbm：跳过 GBM/EGL 初始化走 dumb buffer；DSI-1 输出 eglCreateWindowSurface 失败时可绕过，SSH 远程 -nogbm 实测 dumb buffer 链路打通：PTY→解析→渲染→SetCRTC 正常）
- ✅ 退出死锁修复（SignalClose 新增 SIGKILL 子进程，打破 close(master fd) 不能唤醒阻塞 read(ptmx) 的循环依赖；ptyReadLoop 不再卡住 wg.Wait；DRMInput.Close 加 sync.Once 幂等防 panic；SSH 远程 timeout/SIGTERM 现能优雅退出）
