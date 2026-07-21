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
| main | Run() LockOSThread，渲染主循环（seqCh/ticker.Render/resize/scale/tabReq/mouseEv/exit）；两阶段渲染：Render 所有 slave → Present 所有 slave |
| backend-loop | backend.Run()（DRM: 空操作; Wayland: dispatch 事件循环） |
| pty-read | PTY stdout → Read → FeedInto → seqCh |
| seq-relay | Terminal.SeqCh() → unifiedSeqCh 中继 |
| exit-watch | Terminal.EofCh()/Done() → m.exitCh |
| input | InputSource 事件 → terminal |
| input-watch | inotify 监听 /dev/input 热插拔 — 仅 DRM |
| resize-fanin | reflect.Select 扇入所有 slave 的 ResizeEvents — 多屏 |
| signal | SIGINT/SIGTERM/SIGHUP/SIGQUIT → Close() |
| drm-event | DRM fd 事件读取（Page Flip 完成）— 仅 DRM；用 EventReader 缓存残差防多事件丢失 |
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
├── cmd/gen-dict/main.go        # 词库预处理工具（rime yaml → dict.bin，支持 -order-weight）
├── cmd/gen-emoji/main.go       # Emoji 字形提取工具（NotoColorEmoji.ttf → emoji.bin.gz，自研 SFNT/cmap/CBLC/CBDT 解析）
├── scripts/gen-dict-ice.sh     # rime-ice 词库重建脚本（git clone rime-ice → gen-dict → gzip）
├── pinyin/                     # 拼音输入法（顶层包，非 internal）
│   ├── pinyin.go               # 包级查询引擎（Lookup/FormatPreedit）+ Candidate 类型 + 模糊权重降级 + Lookup 值类型数组（map[string]int+[]seen，零指针分配）
│   ├── syllable.go             # 全拼音节表（414 个）+ DP 切分 Split（长音节优先）+ SplitFuzzy（前缀推断+尾部补全）
│   ├── dict.go                 # go:embed dict.bin.gz + 紧凑索引 dictIndex（buf 常驻+keyOffsets/keyRanges 二分查找+unsafe.String 零复制，HeapAlloc 94MB→24MB）
│   └── data/dict.bin.gz        # 预处理词库（rime-ice 精简 45万 key/89万 entry，gzip 压缩 7.6MB，解压 18.3MB）
├── session/                     # 协调层
│   ├── master.go                # Master + 标签生命周期 + PluginContext 实现
│   ├── slave.go                 # Slave 输出绑定 + OSD 联动
│   ├── render_loop.go           # 主循环 + handleKey/Resize/Scale + 插件 OnRender
│   └── master_test.go
├── terminal/
│   ├── terminal.go              # 纯逻辑会话：PTY + screen + parser + CSI/ESC/Control
│   ├── theme.go                 # Theme 结构体（DefFg/DefBg/CursorColor/Palette[16]）+ DefaultTheme
│   ├── charset.go               # G0/G1/GL + DEC line drawing
│   ├── options.go               # Options + OnTitle/OnDefaultColor/OnCursorColor 回调 + Theme 字段
│   └── render_harness.go        # 性能测量桥接 API
├── font/                    # face.go（OpenTypeFace + FallbackFace primary→fallback→synth + synthBlockElement 块字符合成 U+2580-259F）/ atlas.go / metrics.go / embedded.go（Sarasa + NerdFontFallback go:embed）/ cache.go（FaceCache + FaceCacheProvider 接口 + FallbackFaceCache）/ shear.go（斜体字形预生成）+ emoji.go（EmojiFace：go:embed emoji.bin.gz + 紧凑索引 + PNG解码+BiLinear缩放+NRGBA，baseline对齐 dstH=ascent/YOffset=-ascent）+ assets/（SarasaFixedSC-Regular.ttf 6.7MB + NerdFontFallback.ttf 1.17MB PUA+Dingbats+Greek+Misc子集）+ data/emoji.bin.gz（1353 emoji，2.7MB gzip）
├── internal/
│   ├── runeutil/               # runeutil.go (RuneWidth/IsWide/StringWidth，ASCII 快路径 + x/text/width) + emoji.go (IsEmojiRune 精确范围表 + isEmojiModifier，RuneWidth 对 emoji 基字符返回 2；Dingbats 仅精确码位非整块)
│   ├── debug/                   # Debugf/Errorf/Warningf + 环境变量/文件配置
│   ├── plugins/                 # gopher-lua VM + init.lua + vistty.* API
│   │   ├── manager.go           # PluginManager + 钩子暂存/激活 + pinyin.Init 注入
│   │   ├── context.go           # PluginContext 接口 + TabInfo + ScreenInfo + ApplyTheme
│   │   ├── config.go            # RunConfig + readConfig + parseLuaTheme（vistty.config.theme 解析）
│   │   ├── api_env.go           # vistty.backend_name/backend.is_wayland/is_drm/on_activate 运行环境查询
│   │   ├── api_screen.go        # vistty.screen.focus/prev/next/count/focused_idx/focused_output_id/list 屏幕查询
│   │   ├── api_lifecycle.go     # vistty.on_exit/on_tab_new/on_tab_close/on_tab_switch/on_screen_switch/on_title_change/on_resize/on_zoom 生命周期钩子
│   │   ├── api_theme.go         # vistty.theme.apply/get/default 主题 API
│   │   └── api_*.go             # vistty.input/term/tab/screen/zoom/ui/pinyin/keybind API
│   ├── vte/                     # 转义序列解析器（xterm-256 兼容）
│   │   ├── parser.go / csi.go / osc.go / esc.go / control.go / sgr.go
│   ├── screen/                  # cell.go（AttrClean dirty 位）/ line.go（dirty 字段）/ buffer.go（环形缓冲 offset+mask + DamageRows/View/All/Line/Cursor API）/ history.go / cursor.go / selection.go
│   ├── render/                  # compositor.go / draw.go / cursor.go / overlay.go
│   ├── ui/                      # osd.go (OSD + Tab + Config + PanelPrimitive + Render + CSD 按钮 + HitTestTabBar) + theme.go (OSDTheme + DefaultOSDTheme)
│   ├── version/                 # version.go（ldflags 注入 + ReadBuildInfo VCS fallback，-version 命令行 + vistty.version()/version_info() Lua API）
│   ├── perf/replay/             # 三级归因 benchmark
│   └── platform/
│       ├── surface.go / output.go / input.go / backend.go / keymap.go
│       ├── drm/                 # DRM/KMS 后端（ioctl/codes/types/device/master/mode/dumb/flip/mmap/event/atomic/property/plane/cap）
│       ├── gbm/                 # GBM GPU 后端（device/surface/atomic/gbm + purego dlopen）
│       ├── gl/                  # GLES + EGL purego dlopen
│       ├── gpu/                 # GPU instanced draw 核心（renderer/shader/atlas，后端无关）
│       └── wayland/             # Wayland 后端（backend/surface/input/keymap + 自研 wl.go）
├── examples/init.lua            # 插件示例配置（含拼音输入法 + StatusBar + 主题配置 + mod+J/K 滚动）
├── examples/statusbar.lua       # StatusBar 模块：底部面板宿主（右侧固定区 CPU 温度+日期时间 + 左侧弹性 IME provider 区域 + │ 分隔 + 多屏宽度跟踪）
├── examples/ime.lua             # IME 模块：Ctrl+Space 切换 + 白名单按键捕获 + 自适应分页 + StatusBar left provider 注册 + 多屏宽度感知
├── examples/themes/             # 预设主题 Lua 文件（dracula/solarized_dark/solarized_light/gruvbox/monokai/nord/one_dark）
├── scripts/
│   ├── gbm-bench.sh             # GBM 实机性能测试脚本（编译→启动→监控→报告）
│   ├── gbm-check.sh             # GBM 运行中检测脚本（进程/帧/DRM/错误）
│   ├── gen-dict-ice.sh          # rime-ice 词库重建脚本（git clone rime-ice → gen-dict → gzip）
│   ├── gen-dict-luna.sh         # rime-luna-pinyin 词库重建脚本（-order-weight 倒序赋权）
│   ├── gen-font-subset.sh       # NerdFontFallback.ttf 重建脚本（pyftsubset 双源 + fonttools 合并 PUA+Dingbats/Greek/Misc）
│   └── htop-init.lua            # htop 专用 init.lua（shell=/usr/bin/htop, backend=drm-gbm）
├── .github/workflows/release.yml # CI Release（v* tag 触发：复用 build.sh 编译 linux x86_64 + 打包 vistty/examples/README/LICENSE → GitHub Release，附 sha256）
├── reference/                   # 参考项目源码（foot 终端克隆，已 gitignore，仅分析对照）
└── work_docs/                   # 开发过程文档（含 implementation-emoji.md emoji 方案、implementation-foot-optimization.md foot 优化方案）
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

- DRM/KMS dumb buffer CPU 渲染 + GBM/EGL/GLES GPU instanced draw 渲染
- Wayland wl_shm CPU 渲染后端（自研 wl.go 协议层，含 zxdg_decoration_manager_v1 SSD/CSD 协议 + DecoMode 状态跟踪 + 自绘 CSD 装饰）；两阶段 resize（onConfigure 仅记 pending 尺寸 + 投递 ResizeEvent，buffer 替换延迟到渲染线程 Data() 持锁 applyResizeLocked 执行，消除 use-after-munmap 竞争）；wl_buffer.release 跟踪（released *bool 共享指针 + onRelease 回调，Swap 未释放则跳帧防撕裂）；dispatch 动态扩容 inBuf（>8KB 消息不误判连接关闭）+ MSG_CTRUNC 检查；SOCK_CLOEXEC
- 自动后端探测（drm-gbm → drm → wayland）
- xterm-256 兼容转义序列（CSI/OSC/ESC/SGR，含 OSC 10/11 默认颜色）+ 文本属性（Bold/Italic/Underline/CrossedOut/Dim/Blink/Reverse）；DECSTBM 缺省参数重置（ESC[r/ESC[3r 正确解释）；parser 硬化（Params[32]int + curParam 65535 钳制 + OSC/DCS data 64KB 上限防 DoS + feedUTF8 非法续字节重派发 + ANSI SM/RM 区分 Private）；PtyWrite 异步写队列（writeCh cap=64 + ptyWriteLoop 独立 goroutine，持锁调用方不阻塞，满时丢弃+warning，nil writeCh 回退同步写供测试）
- 斜体渲染：font 层 ShearGlyph(slope=0.1, align=0.5) 预生成斜体字形（顶部向右、双线性插值抗锯齿、居中左移均衡溢出），CPU/GPU 统一走正常 BlendGlyph/atlas 路径；移除 render 层 blendGlyphItalic 位移与 shader italic 分支；italicAtlas 独立缓存，UploadGlyph 以 (rune,italic) 为 key
- 斜体渲染：font 层 ShearGlyph(slope=0.1, align=0.5) 预生成斜体字形（顶部向右、双线性插值抗锯齿、居中对齐左右均衡溢出），CPU/GPU 统一走正常 BlendGlyph/atlas 路径，italicAtlas 独立缓存；移除 render 层位移与 shader italic 分支
- CJK 双宽字符（终端 cell + OSD 面板）+ scroll region 感知换行 + alternate screen + deferred wrap
- Emoji 彩色渲染：自研 `font/emoji.go`（EmojiFace）+ `cmd/gen-emoji` 构建工具；构建期自研 SFNT/cmap/CBLC/CBDT 解析从 NotoColorEmoji.ttf 提取单 rune emoji PNG（CBDT format17），gzip 内嵌 2.7MB/1353 emoji；运行时复用 pinyin/dict.go 紧凑索引（buf常驻+二分查找+PNG零复制）+ image/png 解码 + draw.BiLinear 缩放到 2*cellW×ascent NRGBA（非预乘匹配混合管线，baseline对齐 dstH=ascent/YOffset=-ascent）；CPU blendColorGlyph(BGRA预乘)+GPU shader isColor分支+独立 UploadColorGlyph 双路径；emojiIndex sync.Once 单例避免多屏重复解压；IsEmojiRune 移至 runeutil 包，RuneWidth 对 emoji 基字符强制返回 2（修复 Ambiguous 宽度 emoji 截半，排除 ZWJ/VS 修饰符）；Dingbats/Misc Symbols 仅精确码位非整块（修复 ✕ U+2715 被误判 emoji 导致 getGlyph 路由 emojiFace miss 不回退）；compositor.getGlyph emojiFace miss 时回退常规 FallbackFace（防御性双路径）；P0+P1 范围（单 rune emoji，VS16/ZWJ 未实现）
- 内置 Sarasa Fixed SC 字体 + FaceCache 缩放优化（6-72pt）；双字体 fallback 链（FallbackFace 实现 Face 接口，primary Sarasa miss→NerdFont PUA+Dingbats+Greek+Misc fallback，YOffset 对齐 primary baseline，compositor/GPU 零改动；FaceCacheProvider 接口统一 FaceCache/FallbackFaceCache，GetFace 适配向后兼容）；NerdFontFallback.ttf 由 scripts/gen-font-subset.sh 可复现生成（pyftsubset 双源：NotoMonoNerdFontMono PUA + DejaVuSansMono Dingbats/Greek/Misc，fonttools 合并，1.17MB/3909字形，补齐 ✕ U+2715 π U+03C0 等主字体缺失字形）；Block Elements 合成（synthBlockElement U+2580-259F 硬边几何，主字体子集缺失时兜底，DEC line drawing ▒ 同步修复）；color256 6×6×6 色立方体公式修正（ri/gi/bi 索引，level==0→0，修复 56 色偏蓝/偏青）；PTY 环境补充 COLORTERM=truecolor（避免 TUI 降级 256 色命中 color256 bug）；init.lua fallback_font 可配置/禁用
- 终端配色主题系统（terminal.Theme DefFg/DefBg/CursorColor/Palette[16] + ui.OSDTheme 8 色；全部走 plugin Lua 配置：vistty.config.theme 静态声明 + vistty.theme.apply/get/default 动态 API；字段级 fallback 缺失字段用 DefaultTheme/DefaultOSDTheme 兜底；7 个内置预设 Lua 主题 dracula/solarized_dark/solarized_light/gruvbox/monokai/nord/one_dark；OSC 12 光标色解析 + OnCursorColor 回调；Compositor.defColor 加 sync.Mutex 快照方案修复 OSC 回调 data race；主题切换覆盖 OSC 运行时修改；ApplyTheme 同步 m.opts.Theme 确保新建终端继承当前主题；cmd/vistty 启动时 opts.Theme = runCfg.TermTheme）
- GPU glyph atlas + instanced draw shader（GLES 3.00）+ VAO 缓存 attribute 配置
- 多屏 DRM 输出 + 独立显示模式 + 主屏选择 + 每屏独立 EGLContext + scanout buffer 跟踪 + wait-for-flip 同步（5s 超时兜底）+ 两阶段渲染（Render→Present）60fps
- OSD 标签栏 + 多终端标签（通过 init.lua vistty.input.bind 配置快捷键）+ 面板启用/禁用时自动 resize 终端 + clip 区域越界裁剪 + CSD 模式自绘窗口控制按钮（─□✕）+ 标签栏拖拽移动窗口 + 顶部栏 CJK 双宽渲染（osdCell.w 列数倍率，对齐底部栏修复方案）+ 单标题 16 列宽截断省略号 + 水平滚动（active tab 始终可见，scroll 偏移对齐 tab 边界）
- 插件系统（gopher-lua init.lua + vistty.* API + bind/bind_keys/pressed + 面板渲染 + 热重载 + vistty.exit() 退出 + 运行环境查询 vistty.backend_name()/backend.is_wayland()/is_drm() + on_activate 钩子在 Run() 内首帧前执行（避免主循环未启动时阻塞）+ PluginContext 投递方法非阻塞（tabReqCh/scaleReqCh cap=8，满时丢弃+warning）+ mod 键按后端自适应 wayland=ALT/drm=SUPER + 生命周期钩子 on_exit/on_tab_new/on_tab_close/on_tab_switch/on_screen_switch/on_title_change/on_resize/on_zoom；on_title_change 经主线程 ticker 缓冲避免 terminal 写锁内 PCall 死锁；多屏信息查询 vistty.screen.focused_output_id()/screen.list() + 渲染上下文 ctx:output_id()；OnRender 签名传递 outputID 支持多屏感知渲染）
- StatusBar 底部面板宿主架构（statusbar.lua 启用 bottom 面板 + 注册 on_render 钩子统一布局渲染；左侧弹性 provider 区域 + 右侧固定区 CPU 温度+日期时间 + │ 分隔；register_left/unregister_left provider 注册机制 + left_available_width 多屏宽度跟踪 _left_widths[oid]；IME 作为 left provider 注册，渲染仅对 focused 屏幕激活）
- IME 模块重构（ime.lua：Ctrl+Space 切换键移入 ime.lua 内部 setup_toggle；init(statusbar_ref) 接收 StatusBar 引用 + 通过 statusbar_ref.left_available_width(oid) 查询多屏宽度；render(ctx, avail_w, h, oid) 适配 provider 接口 + ime_widths[oid] 多屏宽度缓存；分页索引 0-based→1-based 修复；init.lua 简化为 require statusbar + statusbar.init() 一行启动）
- 中文拼音输入法（pinyin 顶层包 + 包级查询函数 Lookup/FormatPreedit/Split/SplitFuzzy + go:embed rime-ice 词库 + 底部单行候选词面板 + Lua 层交互状态管理+自适应分页）
- SplitFuzzy 宽松切分：前缀推断（如 "n"→na/ni/...）+ 尾部未完成音节补全（如 "nih"→ni+h*），补全候选词权重×0.5 降级
- 组合词权重降级：composeFromSingleChars 组合词 weight /=100（单字百万级 weight 降至十万级以下，低于字典真实词组）；多分割方案按音节数差异指数降级 splitFactor=1/10^extraSyllables（最优分割长音节优先不降级，短音节分割组合词大幅压低，防止"大起啊哦"类噪声压过"大桥"类真实词组）
- 拼音字典内存优化：dict.bin 紧凑索引替代 map[string][]dictEntry 展开（dictIndex: 解压 buf 常驻 + keyOffsets/keyRanges 二分查找 + unsafe.String 零复制 word），HeapAlloc 94.2MB→23.7MB，加载峰值 170MB→30MB；Lookup 值类型数组（map[string]int+[]seen）消除 *seen 指针逃逸
- 动态缩放（通过 init.lua bind 配置）+ dirty 跳帧 + 光标时间戳闪烁
- 错误日志文件（~/.local/share/vistty/error.log）；Errorf 统一追加换行符，调用方无需手动加 \n；无 EV_KEY 能力输入设备日志降级为 Debugf
- VT 管理 + TTY 绑定 + SIGKILL 子进程退出死锁修复；退出时 CRTC 恢复带原 connector ID（SetCrtc 不再传 nil connectors）+ 错误日志；DRM/evdev/tty 打开路径统一 O_CLOEXEC（防 fd 泄漏子进程）；GBM/EGL/GLES Loader Close()（purego.Dlclose）+ init 失败逐级回滚
- 输入设备热插拔（DRM: inotify + epoll 监听 /dev/input + ready channel 同步 Close 与 watchLoop 避免 fd 复用竞态 + IN_Q_OVERFLOW 溢出重扫；Wayland: wl_seat.capabilities 动态创建/释放 keyboard/pointer）
- 两阶段关闭 + Close 幅等 + 渲染错误容错
- GBM flip 超时兜底（channel+select+time.After 5s，防止内核 flip 事件丢失导致 Swap 永久阻塞）；超时分支模拟 onFlipComplete 轮转三缓冲（releaseBO=scanout; scanout=committed; committed=nil），避免 committed 帧覆盖泄漏 BO+FB；VT 切换 SetActive(false) 清 flipPending 同轮转，消除切回首帧 5s 卡顿
- DRM EventReader 有状态事件读取（缓存残差 buffer 逐个解析，防止多屏同时 flip 完成时事件丢失）
- GBM active/closed 统一 commitMu 保护（消除跨锁 data race）
- GBM atomic commit modeset 失败时 disable-then-enable 重试（先 ACTIVE=0/MODE_ID=0/FB_ID=0 disable CRTC+connector+plane，再重新 enable）+ 诊断日志（crtc/conn/plane/fbID/modeBlob/flags + 每个 property ID/value）；surfaceAtomicInfo 保存 mode *ModeInfoPublic 供 legacy 回退
- 版本信息（internal/version 包：ldflags 注入 git describe --tags --always --dirty + ReadBuildInfo VCS fallback；-version 命令行查询 + vistty.version()/version_info() Lua API；scripts/build.sh 构建脚本）
- BSU 同步更新（DECSET 2026）：TUI 应用重绘期间合并渲染不触发 dirty，1 秒超时兜底（timer goroutine 独立加锁+幂等+锁外回调 OnRenderRequest→renderReqCh）+ DECRQM ?2026$p 响应 ?2026;Ps$y（vte parseCSIPrivate 补 case 'p' 路由 DSR）；enable/disable 锁内调用（Go RWMutex 不可重入），syncUpdateExpired timer 回调独立加锁
- Damage Tracking 双层 dirty（参考 foot render.c）：screen 层 cell AttrClean=1<<7 位 + line dirty 字段 + Buffer DamageRows/View/All/Line/DamageCell API；terminal 所有写操作标记 damage（executeSequences 光标移动统一 DamageCursor 旧行+新行、execPrint 仅触碰 cell 用 DamageCell 替代 DamageLine 消除 O(cols²)、eraseChars/deleteChars/insertChars DamageLine、alt screen/Resize/SetScrollOffset/SetTheme DamageAll）；buffer 写操作 ScrollUp/ScrollDown/Clear/ClearAll/ClearRect 自动 damage；compositor CPU 路径（useDirty=!directRender，DRM dumb）逐 cell 背景清除（非 Clean cell 清 bg+绘字形，Clean cell 跳过保留上一帧像素，修复光标移动整行擦除 bug），Wayland directRender 路径全量（buffer 切换安全）；tab 切换时 damageActiveScreen 对新 active terminal 调用 DamageAll（清除 Clean 位+标记 line dirty，强制全量重绘，防止 backBuf 残留前一个 terminal 内容；覆盖 tabPrev/tabNext/tabSwitch + handleTermExit 场景）；GPU 路径保持全量 instances（Clear 重绘需全部 cell）+ gpuDisabled 永久禁用标志（BeginFrame 失败后不重试）；光标行总是渲染（光标闪烁 blinkOff 不残影）；InsertLines/DeleteLines 以 [cursor.Row, scrollBot] 为区域不进 history（xterm IL/DL 语义）
- 环形缓冲区 grid（参考 foot grid.h）：screen Buffer 改为环形（capacity=nextPow2(rows), mask=capacity-1, offset 头指针），lineAt(row)/physRow(row) 统一行访问 (offset+row)&mask；全屏滚动 offset+=n O(1)（仅 push history + 底部新行 NewLine+Fill），region 滚动逐行 copy O(region)（新行 NewLine 避免指针别名）；ScrollDown offset-=n；Resize 重建环形复用旧行 + clamp saved cursor；history 保持独立 []*Line；scrollback 规则：仅全屏 ScrollUp 进 history，ScrollDown 与区域 ScrollUp/ScrollDown 不进（xterm 语义）
- 渲染调度优化：cursorBlinkTicker 500ms 异步驱动光标闪烁（替代 15 tick 250ms 兜底），无 dirty 时零渲染（稳态仅 500ms 光标闪烁触发一次渲染）；delayed render 评估：已有 60fps ticker 天然合并 seqCh 序列，无需额外 timer
