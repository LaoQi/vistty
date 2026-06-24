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
cmd/vistty (入口，选择后端)
    └── terminal (胶水层)
            ├── vte (转义解析)
            ├── screen (缓冲区)
            ├── font (opentype + glyph cache)
            └── render (合成+光标)
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
| SIGINT/SIGTERM/SIGHUP/SIGQUIT | signalLoop → signalClose()（只关 done+pty）→ wg.Wait() → backend.Stop() → input.Close() → cleanup() |
| Wayland 窗口关闭 | toplevel.SetCloseHandler → backend.notifyClose() → close(doneCh) → Run() select 感知 backend.Done() → signalClose() → 两阶段关闭 |
| PTY 退出 | ptyReadLoop → eofCh/done → signalClose() → 两阶段关闭 |
| 两阶段关闭 | 阶段1: signalClose() 只关 done+pty（不触碰 Wayland 对象）→ 阶段2: backend.Stop() 关连接解锁 Run() → 等待 backend.Run() 退出 → input.Close()+cleanup()（无并发 map 访问）|
| Close() 幂等 | sync.Once 保护 signalClose/cleanup/input.Close/backend.Stop，重复调用安全 |

### 包目录结构

```
github.com/LaoQi/vistty/
├── cmd/vistty/main.go
├── terminal/
│   ├── terminal.go             # 胶水层：execCSI/execESC/execPrint/execControl/handleMode/applySGR
│   ├── charset.go              # 字符集状态管理（G0/G1/GL + DEC line drawing 映射）
│   └── options.go              # Options + OnTitle 回调
├── internal/
│   ├── vte/                    # 转义序列解析器（xterm-256 兼容）
│   │   ├── parser.go           # 状态机（9 状态 + privateMarker 分发）
│   │   ├── csi.go              # CSI 语义：privateMarker ?/>/=/</ + intermed SP/" 分发
│   │   ├── osc.go              # OSC 语义：0/1/2/7/8/10/11/52
│   │   ├── esc.go              # ESC 语义：G0/G1 字符集指定 ( ) * + + DECSC/RC
│   │   ├── control.go          # C0 控制字符识别
│   │   ├── sgr.go              # SGR 解析：22 → BoldOff+DimOff
├── internal/
│   ├── screen/                 # 终端屏幕缓冲区
│   │   ├── cell.go / line.go / buffer.go / history.go / cursor.go / selection.go
│   ├── font/                   # 字体管理
│   │   ├── face.go / atlas.go / metrics.go / embedded.go
│   │   └── assets/             # 内置字体资源（Sarasa Fixed SC 子集 + LICENSE）
│   ├── render/                 # 渲染合成
│   │   ├── compositor.go / draw.go / cursor.go
│   └── platform/               # 平台抽象层
│       ├── surface.go          # Surface 接口
│       ├── input.go            # InputSource 接口 + 事件类型
│       ├── backend.go          # Backend 接口
│       ├── keymap.go           # 共享键映射函数（DRM/Wayland通用）
│       ├── drm/                # DRM/KMS 后端
│       │   ├── backend.go / surface.go / input.go / display.go / vt.go
│       │   └── internal/       # DRM 底层（不对外暴露）
│       │       ├── ioctl.go / codes.go / types.go
│       │       ├── device.go / master.go / mode.go / dumb.go
│       │       ├── flip.go / mmap.go / event.go
│       │       ├── atomic.go / property.go / plane.go  (预留)
│       └── wayland/            # Wayland 后端
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

运行：
```bash
go run ./cmd/vistty                         # 自动探测后端（DRM优先，回退Wayland）
go run ./cmd/vistty -backend drm            # 强制 DRM/KMS 直出
go run ./cmd/vistty -backend wayland        # 强制 Wayland 窗口（开发调试）
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
- ✅ ptyReadLoop 非阻塞读（goroutine + channel 模式，避免 Close 后 Read 卡死）
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

xterm-256 VT 支持详情：
- 解析层(vte)：SGR 22 同时关闭 Bold+Dim；CSI q 按 intermed 区分 DECSCUSR(SP)/DECSCA(")/Unknown；新增 CSI 命令 X(ECH)/n(DSR)/c(DA1)/g(TBC)；私有标记 > 分发 DA2(>c)；ESC ( ) G0/G1 字符集指定；OSC 2 窗口标题
- 执行层(terminal)：hostWriter 响应回写通道；savedCursorState 完整状态保存（位置+SGR+字符集）；DECAWM(?7) 自动换行开关；DECCKM(?1) 应用光标键；?47/?1047/?1048 备用屏幕；?1004 焦点标志；?2004 括号粘贴标志；DECSCUSR 6 种光标样式含闪烁/稳定；DEC line drawing 字符集转换+SO/SI；tab stop 动态管理；ECH 擦除字符；ED case3 清 scrollback；DSR/DA1/DA2 响应回写；OSC 标题 OnTitle 回调
- DA1 响应：`CSI ?62;4c`（VT220 + SGR 颜色）
- DA2 响应：`CSI >0;0;0c`
- 测试架构：newTerminalForTest + feedBytes 无 IO 测试入口

Wayland 后端实现细节：
- 5个文件：backend.go（连接+全局绑定+事件循环+错误处理）、surface.go（双缓冲wl_shm+XDG toplevel）、input.go（wl_keyboard/wl_pointer+修饰键跟踪+keymap handler 解析）、keymap.go（XKB keymap 解析 + evdev code 索引 + US 布局回退）、wire.go（修正的Wayland线格式编码）
- 使用 memfd_create + mmap 创建共享内存
- 支持 shm format 协商（通过 wl_shm.format 事件动态选择格式）
- 支持窗口resize（Configure事件驱动重新分配buffer）
- XDG toplevel 关闭事件通过 backend.Done() 通知 terminal 退出
- Display.SetErrorHandler 捕获合成器协议错误，便于调试

go-wayland 库已知 bug（已在 wire.go 中修复）：
- PutString 写入 padded length 而非 actual length（含 NUL 终止符）到 uint32 长度字段
- ShmFormat 枚举常量使用顺序索引（0, 1）而非 DRM FourCC 码，但合成器实际发送的是顺序索引值
- wire.go 重新实现了 registryBind、toplevelSetTitle、toplevelSetAppId、shmCreatePool、shmPoolCreateBuffer、compositorCreateSurface、xdgWmBaseGetXdgSurface、xdgSurfaceGetToplevel，使用 encoding/binary 正确编码

待完善：
- font 包测试文件已添加（atlas_test.go, face_test.go）
- Wayland 后端无自动化测试（需 Wayland 合成器环境）
