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
| 3 | 输入处理 | `holoplot/go-evdev` + inotify | 纯 Go，含 Grab 独占、uinput；DRM 后端 inotify 热插拔 |
| 4 | PTY 管理 | `creack/pty` | Go 生态标准方案，纯 Go |
| 5 | 转义序列解析 | 自研 | 参考 go-vte 状态机 + darktile termutil |
| 6 | 终端缓冲区 | 自研 | 参考 darktile termutil Cell/Line/Buffer |
| 7 | 字体解析+光栅化 | `golang.org/x/image/font/opentype` | 内置 Sarasa Fixed SC 子集（等宽+CJK）+ NerdFont PUA fallback 子集（Powerline/Nerd图标）+ Block Elements 合成 |
| 8 | 文本整形 | 初期不引入 | 后续按需引入 go-text/typesetting/harfbuzz |
| 9 | 渲染合成 | 自研渲染管线 | glyph cache + double buffer + GPU instanced draw |
| 10 | Wayland 窗口后端 | 自研纯 Go 协议层 | wl.go 实现 Wayland wire 协议最小子集，零 CGO |
| 11 | OSD 四边 UI 层 | 自研 | 顶部多终端标签栏 + 底/左/右插件面板；实现 render.Overlay 接口 |
| 12 | 插件系统 | `gopher-lua` | Lua 5.1 VM；init.lua 驱动配置+钩子；分层命名空间 API |
| 13 | 拼音输入法 | 自研 `pinyin` 包 | 包级查询函数（Lookup/FormatPreedit/Split/SplitFuzzy）+ rime-ice 词库；交互状态全在 Lua 层；SplitFuzzy 支持前缀推断+尾部补全 |
| 14 | Emoji 彩色渲染 | 自研 `font/emoji.go` + `cmd/gen-emoji` | 零新依赖；构建期自研 SFNT/cmap/CBLC/CBDT 解析提取单 rune emoji PNG（CBDT format17），gzip 内嵌 2.7MB/1353 emoji；运行时复用 pinyin/dict.go 紧凑索引模式（buf常驻+二分查找+PNG零复制）+ image/png 解码 + draw.BiLinear 缩放到 2*cellW×cellH NRGBA；CPU blendColorGlyph(BGRA预乘)+GPU shader isColor分支+独立 UploadColorGlyph 双路径 |

## 架构

### 分层

```
cmd/vistty (入口，-backend 选择后端 + -config 指定 init.lua + PluginManager 注入)
    └── session (协调层：枚举输出 + 焦点路由 + 渲染编排 + 标签管理)
            ├── terminal (纯逻辑会话：PTY + screen + parser + CSI 执行)
            │       ├── vte (转义解析)
            │       └── screen (缓冲区)
            ├── session.Slave (输出绑定：surface + compositor + terms[] + osd)
            ├── plugins (Lua VM + 钩子暂存/激活 + PluginContext 接口)
            │       └── pinyin (包级查询：Lookup/FormatPreedit/Split/SplitFuzzy，Lua 层管理交互状态)
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
     DecoMode() uint32       // 0=未知/无协议, 1=CSD, 2=SSD
 }

 type WindowMover interface {
     StartMove(serial uint32)    // Wayland: xdg_toplevel.move；DRM: 不实现
     StartResize(serial uint32, edge uint32)  // Wayland: xdg_toplevel.resize
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
| InputSource | go-evdev 读 /dev/input/eventN + inotify 热插拔 | wl_keyboard + wl_pointer 事件 + capabilities 动态创建/释放 |
| 键盘映射 | 自研 scancode→Unicode | 简化 XKB keymap 解析 |
| 窗口管理 | CRTC/Connector 全屏 | XDG Shell 窗口 + zxdg_decoration SSD/CSD + 自绘 CSD 按钮 |
| VT 切换 | SIGUSR1/2 + KD_GRAPHICS | 不适用 |
| GPU 渲染 | GBM+EGL+GLES instanced draw（每 Surface 独立 EGLContext） | 不支持（wl_shm CPU 路径） |

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
                                               backBuf → Surface.Data()
                                                              ↓
                                               Compositor.Render()（所有 slave 绘制）
                                                              ↓
                                               Compositor.Present() → Surface.Swap()（page flip）
```

### Goroutine 模型

| goroutine | 职责 |
|-----------|------|
| main | Run() LockOSThread，渲染主循环（seqCh/ticker.Render/resize/scale/tabReq/mouseEv/exit/signal）；两阶段渲染：Render 所有 slave → Present 所有 slave |
| backend-loop | backend.Run()（DRM: 空操作; Wayland: dispatch 事件循环） |
| pty-read | PTY stdout → Read → FeedInto → seqCh |
| seq-relay+exit | Terminal.SeqCh()/EofCh()/Done() → unifiedSeqCh / m.exitCh |
| input | InputSource 事件 → terminal |
| input-watch | inotify 监听 /dev/input 热插拔 — 仅 DRM |
| resize-fanin | reflect.Select 扇入所有 slave 的 ResizeEvents — 多屏 |
| drm-event | DRM fd 事件读取（Page Flip 完成）— 仅 DRM；用 EventReader 缓存残差防多事件丢失 |
| vt-signal | SIGUSR1/2 VT 切换 — 仅 DRM |

### 退出路径

| 触发源 | 路径 |
|--------|------|
| 信号 | sigCh（主循环内）→ signalClose() → wg.Wait() → backend.Stop() → input.Close() → cleanup() |
| Wayland 窗口关闭 | toplevel close → backend.Done() → signalClose() → 两阶段关闭 |
| PTY 退出 | exit-watch → handleTermExit 移除标签 → 无剩余 terminal 时 signalClose() |
| Close() 幂等 | sync.Once 保护，重复调用安全 |

两阶段关闭：signalClose() 只关 done+pty（不触碰 Wayland 对象）→ backend.Stop() 解锁 Run() → 安全销毁

### 包目录结构

```
github.com/LaoQi/vistty/
├── cmd/vistty/main.go          # 入口
├── cmd/gen-dict/main.go        # 词库预处理工具（rime yaml -> dict.bin）
├── cmd/gen-emoji/main.go       # Emoji 字形提取工具（NotoColorEmoji.ttf -> emoji.bin.gz）
├── pinyin/                     # 拼音输入法（顶层包，非 internal）
│   ├── pinyin.go               # 查询引擎 Lookup/FormatPreedit + Candidate + 模糊权重降级
│   ├── syllable.go             # 音节表 + DP 切分 Split/SplitFuzzy
│   ├── dict.go                 # go:embed dict.bin.gz + 紧凑索引 dictIndex
│   └── data/dict.bin.gz        # rime-ice 预处理词库
├── session/                    # 协调层（master/slave/render_loop + master_test）
├── terminal/                   # 纯逻辑会话（terminal/theme/charset/options/render_harness）
├── font/                       # face/atlas/metrics/embedded/cache/shear + emoji.go + assets/ + data/emoji.bin.gz
├── internal/
│   ├── runeutil/               # RuneWidth/IsWide/StringWidth + emoji.go（IsEmojiRune）
│   ├── debug/                  # Debugf/Errorf/Warningf + 环境变量/文件配置
│   ├── plugins/                # gopher-lua VM + vistty.* API（manager/context/config + api_*.go）
│   ├── vte/                    # 转义序列解析器（xterm-256 兼容）
│   ├── screen/                 # cell/line/buffer(环形)/history/cursor/selection
│   ├── render/                 # compositor/draw/cursor/overlay
│   ├── ui/                     # osd + theme（OSD 标签栏 + 插件面板 + CSD）
│   ├── version/                # ldflags 注入 + ReadBuildInfo VCS fallback
│   ├── perf/replay/            # 三级归因 benchmark
│   └── platform/
│       ├── surface.go / output.go / input.go / backend.go / keymap.go
│       ├── drm/                # DRM/KMS 后端（ioctl 封装：dumb/flip/mmap/event/atomic/property/plane）
│       ├── gbm/                # GBM GPU 后端（device/surface/atomic + purego dlopen）
│       ├── gl/                 # GLES + EGL purego dlopen
│       ├── gpu/                # GPU instanced draw 核心（renderer/shader/atlas，后端无关）
│       └── wayland/            # Wayland 后端（backend/surface/input/keymap + 自研 wl.go）
├── examples/                   # init.lua + statusbar.lua + ime.lua + themes/ 预设主题
├── scripts/                    # build.sh + quick-update.sh + gen-dict-*.sh + gen-font-subset.sh + gbm-bench.sh + gbm-check.sh + htop-init.lua
├── .github/workflows/release.yml # CI Release（v* tag 触发）
├── reference/                  # 参考项目源码（foot 终端克隆，已 gitignore）
└── work_docs/                  # 开发过程文档（implementation-changelog.md 等）
```

### 依赖方向

```
cmd/vistty → terminal, plugins, debug, platform/gbm (GBM 组装注入), ui, version
cmd/gen-dict → 无内部依赖（独立词库预处理工具）
cmd/gen-emoji → internal/runeutil（共享 IsEmojiRune）
pinyin → 无内部依赖（顶层包，go:embed 词库）
terminal → screen, vte, render, platform, font, debug, runeutil
session → render, font, platform, terminal, ui, plugins (PluginContext 接口), debug
plugins → terminal, platform, ui, pinyin, version, debug（不依赖 session，通过 PluginContext 依赖倒置）
render → font, platform (Surface 接口)
ui → render (Overlay/GlyphProvider/GPUGlyphUploader 接口), font, platform, runeutil
platform/drm → platform (Surface/GBMProvider 接口), go-evdev, golang.org/x/sys/unix (inotify/epoll), debug
platform/gbm → platform (GBMProvider/Surface/Output), platform/gl, platform/gpu, debug
platform/gpu → platform/gl, platform (CellInstance/GPURenderer), debug
platform/gl → purego
platform/wayland → 无外部依赖（自研 wl.go）
screen, vte → 无内部依赖
runeutil → 无内部依赖（golang.org/x/text/width）
font → 无内部依赖（顶层包），golang.org/x/image/font/opentype, golang.org/x/image/draw（emoji 缩放）
version → 无内部依赖（runtime/debug，ldflags 注入 + ReadBuildInfo VCS fallback）
plugins → gopher-lua
debug → 无内部依赖
```

**依赖规则：** `drm` 不依赖 `gbm`（GBMProvider 接口由 cmd/vistty 注入）；`render` 不依赖 `ui`（Overlay 接口依赖倒置）；`plugins` 不依赖 `session`（PluginContext 接口依赖倒置）。

## 关键约束
- CGO_ENABLED=0：所有内核接口通过 syscall/unix ioctl 或纯 Go 库实现
- DRM 是内核 UAPI — 基于 ioctl，需精确匹配 C 结构体内存布局
- GBM 是 Mesa 用户空间库 — 通过 purego dlopen 访问，非 ioctl
- DRM Page Flip 事件读取在 drm-event goroutine（EventReader 有状态缓存防多事件丢失），flip 提交在 render main goroutine；两者通过 channel/回调跨 goroutine 通信
- 键盘映射：DRM 自研 scancode→Unicode，Wayland 自研 XKB keymap 解析，初期仅 US 布局

## 预留扩展点

| 扩展点 | 位置 | 方式 |
|--------|------|------|
| 硬件光标 | `platform/drm/` | DRM Plane ioctl |
| 文本整形 | `font/shaper.go` | 集成 go-text/typesetting/harfbuzz |
| Sixel 图形 | `vte/sixel.go` | 扩展 Parser DCS 处理 |
| X11 窗口后端 | `platform/x11/` | 新增 Backend 实现 |
| 完整 XKB 支持 | `platform/wayland/keymap.go` | 可选 purego dlopen libxkbcommon.so |
| Emoji VS16/ZWJ 序列 | `terminal/terminal.go` + `screen/cell.go` | VS16/VS15 感知（Cell.Attr 标记 emoji presentation）+ ZWJ 序列组合（EmojiID 引用全局序列池） |

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

GBM 实机性能测试：
```bash
sudo ./scripts/gbm-bench.sh -t 2 -d 90         # 主测试（drm-gbm + htop，90s）
./scripts/gbm-check.sh <pid>                     # 运行中检测（进程/帧/DRM/错误）
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
go run ./cmd/vistty -version                # 查看版本信息（go run 显示 develop，go build 显示 commit）
./scripts/build.sh                          # 带版本注入构建（git describe --tags → ldflags）
```

## 已实现功能概要

> 完整实现细节见 `work_docs/implementation-changelog.md`

- DRM/KMS dumb buffer CPU 渲染 + GBM/EGL/GLES GPU instanced draw
- Wayland wl_shm CPU 渲染后端（自研 wl.go + zxdg_decoration + 两阶段 resize + wl_buffer.release 跳帧）
- 自动后端探测（drm-gbm → drm → wayland）
- xterm-256 兼容转义序列 + 文本属性 + parser 硬化 + PtyWrite 异步写队列
- 斜体渲染（ShearGlyph 预生成 + italicAtlas 独立缓存）
- CJK 双宽 + scroll region + alternate screen + deferred wrap
- 可配置 scrollback（Lua scrollback 配置项 + NewBuffer 签名 + term.history_len() API + Shift+PageUp/Down 半页滚动）
- Emoji 彩色渲染（CBDT format17 + 紧凑索引 + CPU/GPU 双路径，仅单 rune，VS16/ZWJ 未实现）
- 内置 Sarasa Fixed SC + NerdFont PUA fallback + Block Elements 合成 + FaceCache
- 终端配色主题系统（Lua 配置 + 7 预设 + OSC 10/11/12 + 字段级 fallback）
- GPU glyph atlas + instanced draw（GLES 3.00）
- 多屏 DRM 输出 + 每屏独立 EGLContext + 两阶段渲染 60fps
- OSD 标签栏 + 多终端标签 + CSD 自绘 + 水平滚动
- 插件系统（gopher-lua + vistty.* API + 热重载 + 生命周期钩子 + 多屏感知）
- StatusBar 底部面板宿主 + IME left provider
- 中文拼音输入法（Lookup/FormatPreedit/Split/SplitFuzzy + rime-ice 词库 + 自适应分页）
- 动态缩放 + dirty 跳帧 + 光标闪烁 + 插件/IME 主动请求渲染（vistty.request_render + PluginContext.RequestRender）
- VT 管理 + 输入热插拔 + 两阶段关闭 + Close 幂等
- GBM flip 超时兜底 + EventReader + atomic commit modeset 重试
- BSU 同步更新（DECSET 2026）
- Damage Tracking 双层 dirty + 环形缓冲区 grid + 渲染调度优化
