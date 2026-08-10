# Vistty 渲染热点分析与优化

## 采样方法

```bash
# 构建（CGO 禁用）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/vistty ./cmd/vistty

# 20 秒实跑 + CPU/内存采样（SIGINT 优雅退出 flush profile，5s 兜底强杀）
WAYLAND_DISPLAY=wayland-1 XDG_RUNTIME_DIR=/run/user/1000 \
  timeout -s INT --kill-after=5 20 /tmp/vistty \
  -backend wayland -cpuprofile /tmp/vistty.prof -memprofile /tmp/vistty.mem -fps \
  2>/tmp/vistty.fps.log

# 分析
go tool pprof -top -cum -nodecount=30 /tmp/vistty /tmp/vistty.prof
go tool pprof -list='<func>' /tmp/vistty /tmp/vistty.prof
```

> **注意：`timeout -s KILL` 强杀会得到 0 字节 profile。** Go 的 pprof 把 CPU 采样缓存在内存，
> 仅在 `StopCPUProfile()`（`defer prof.stop()`）时刷盘，SIGKILL 无法被捕获故全丢。
> 必须用可捕获信号（SIGINT）收尾以触发程序自身的 signalLoop 优雅退出 flush profile。

## 采样环境

- 后端：Wayland（连入 niri `wayland-1`；DRM master 被 niri 占用，DRM 后端无法 SET_MASTER）
- 时长 20.01s，1199 帧（~60fps，单帧 3-10ms）
- CPU 占用 35.89%（单线程渲染循环），Total samples = 7.18s
- 内存极干净：总分配仅 5.7KB，全是启动期一次性（NewCompositor backBuf + 字体解析），**帧内零分配**

## 热点排行（行级 pprof 验证）

| # | 函数 | flat | 占比 | 根因 |
|---|------|------|------|------|
| 1 | `wayland.WaylandSurface.Swap` | 2.59s | **36.8%** | `swapBR` 逐像素 B/R 交换循环 |
| 2 | `render.fillRect` | 2.21s | **30.9%** | 每个 cell 都调一次背景填充 |
| 3 | `render.copyAllToSurface`→memmove | 1.07s | **15.0%** | 全屏 backBuf→surface memcpy（CPU 路径固有） |
| 4 | `render.getGlyph`→`Atlas.Get` | 0.65s | 9.0% | 每可见 rune 一次 map 查找 |

前三项合计 **82.7%**。

### #1 Swap swapBR（36.8%）— 最大优化点

`surface.go:171-174`，全部 2.59s 花在循环里（位运算 1.26s + 循环 1.12s），Attach/Damage/Commit 仅 0.04s。

根因：`backend.go:146-148` 取 niri 广播的**首个** shm 格式，而 niri 首个是 BGR 序（XBGR/ABGR/BGRX/BGRA），触发 `swapBR=true`，每帧对全屏逐像素交换 B/R 通道。

**修复**：XRGB8888 是 Wayland 必备格式（合成器必须支持），改为优先选 RGB 序格式（XRGB8888 > ARGB8888），遍历所有格式事件按优先级挑选，而非取首个。直接消除这 36.8%。

### #2 fillRect（30.9%）

`compositor.go:135` 对**每个 cell** 调 `fillRect` 填背景，内层逐像素写（draw.go:19-20 占 1.41s）。典型终端多数格子是默认背景，却仍逐格填充，且逐 cell 的边界检查开销大（line 7-18 占 650ms）。

**修复**：每帧开始无条件用默认背景色全屏清除 backBuf 一次（连续写入 cache 友好），cell 循环中仅对解析后背景色 ≠ 默认背景色的 cell（如 vim 状态栏、彩色背景文本）做局部 fillRect。默认背景 cell 跳过。

### #3 copyAllToSurface（15.0%）

`compositor.go:239` 每帧 `copy()` 全缓冲到 surface.Data（memmove 1.07s）。CPU/dumb-buffer 路径固有开销，GPU/GBM 路径下自然消除，暂不处理。

### #4 Atlas.Get（9.0%）

`mapaccess2_fast32` 5% — 每可见 rune 一次 map 查找。可加 ASCII（rune<128）数组直接索引快路径，收益较小、优先级低，暂不处理。

## 优化预期

- #1 修复：消除 36.8%（swapBR 循环全除）
- #2 修复：fillRect 从逐 cell 改为全屏一次 + 少量局部，预计砍 ~20%
- 合计预计 CPU 占用从 35.89% 降至 ~15%，单帧耗时下降

## 最终结果

### 自主协议层替代 go-wayland

移除 `rajveermalviya/go-wayland` 依赖，自研纯 Go Wayland 协议层（`wl.go`，727 行）：
- `conn` 核心：unix.Socket 直连 + Sendmsg/Recvmsg + 事件分发 + fd 传递（SCM_RIGHTS）
- 全部协议对象：Display/Registry/Callback/Compositor/Shm/ShmPool/Buffer/Surface/Seat/Keyboard/Pointer/XdgWmBase/XdgSurface/XdgToplevel
- 修复了多个 wire opcode 错误（wl_surface attach/damage/destroy、wl_seat get_keyboard/get_pointer/release、wl_keyboard/pointer release、xdg_surface set_window_geometry）
- 修复了 fd GC 回收问题（用 unix.Socket 替代 net.DialUnix+File()）

### 性能对比

| 指标 | 优化前 | 优化后 | 降幅 |
|------|--------|--------|------|
| CPU 占用 | 35.89% (7.18s) | 15.80% (3.16s) | **-56%** |
| Swap (swapBR) | 2.59s (36.8%) | 0.01s (0.3%) | **-99.6%** |
| fillRect | 2.21s (30.9%) | 0.59s (18.7%) | **-73%** |
| memmove | 1.07s (15.0%) | 1.41s (44.6%) | (占比升因总量降) |
| 帧率 | ~60fps | ~60fps | 持平 |

swapBR 循环完全消除（XRGB8888 选中，无需 B/R 交换）。fillRect 从逐 cell 改为全屏清除 + 仅非默认背景 cell 局部填充。

## 第二轮：DRM 模式热点优化（FeedAll 分配 + Atlas 去锁）

### 采样环境

- 后端：DRM `-backend drm`（dumb buffer CPU 渲染）
- 输出：HDMI-A-1 2560×1440（independent 模式，另含 DSI-1 800×1280）
- 负载：`/bin/bash` 无限循环 echo（中英混合持续滚动）
- 帧时间 ~16ms（≈60fps，PTY 空闲时）；PTY 输出堆积时主循环被 Apply 占据，渲染帧被饿死

### 优化前热点

CPU（Duration 5.07s，Total 2.58s，50.87%）：

| # | 函数 | flat | 占比 | 根因 |
|---|------|------|------|------|
| 1 | `render.blendGlyph` | 0.77s | 29.84% | 逐像素 alpha 混合（dumb buffer 固有） |
| 2 | `render.fillRect` | 0.71s | 27.52% | 每帧全屏背景清除 |
| 3 | `runtime.memmove` | 0.46s | 17.83% | backBuf→Surface 全量拷贝（dumb buffer 固有） |
| 4 | `sync/atomic.(*Int32).Add` | 0.14s | 5.43% | Atlas RWMutex 锁计数 |
| 5 | `font.(*Atlas).Get` | 0.27s | 10.47% | 含 RLock 0.09s + mapaccess 0.11s |
| 6 | `sync.(*RWMutex).RLock` | 0.09s | 3.49% | 每可见 rune 一次读锁 |

MEM（198.36MB，alloc_space）：

| # | 函数 | 分配 | 占比 | 根因 |
|---|------|------|------|------|
| 1 | `vte.(*Parser).FeedAll` | 156.77MB | 79.03% | 每次返回 `make([]Sequence,len)+copy` |
| 2 | `render.NewCompositor` | 17.97MB | 9.06% | backBuf 初始化（一次性） |
| 3 | `screen.NewLine` | 12.05MB | 6.08% | 滚动新行 |

### 优化项

#### P0：FeedAll 分配消除（parser.go + terminal.go + render_loop.go）

`FeedAll` 每次 PTY 数据到达都 `make+copy` 一份 Sequence 切片发往 `seqCh`（cap=64），防 `p.seqs[:0]` 复用冲突，占 79% 堆分配。

- `vte/parser.go`：新增 `FeedInto(data, dst) []Sequence`，用 `append(dst[:0], p.seqs...)` 复用调用方提供的底层数组；`FeedAll` 保留为 `FeedInto(data, nil)` 兼容测试与同步路径
- `terminal/terminal.go`：新增包级 `seqPool`（`sync.Pool`，`New` cap=4096）；`PtyReadLoop` 改为 `Get`→`FeedInto`→发 `seqCh`，主线程消费后经 `ReturnSeqPool` 归还
- `session/render_loop.go`：Apply 后调 `terminal.ReturnSeqPool(seqs)`
- **cap=4096 关键**：单次 4096 字节读取最多产生 ~3500 个 Sequence（每 ASCII 字符一个 ActionPrint），cap=256 会导致 `append` 反复 grow（实测残留 109.85MB）；cap=4096 后 `append(dst[:0],...)` 零分配

#### P1：Atlas 去锁（atlas.go）

`Atlas.Get/Put/Clear` 的 `sync.RWMutex` 在渲染主线程单线程访问下无并发竞争（`getGlyph`→`Get/Put` 全在 `Render` 内，`Render` 仅主线程 `renderFrame` 调用；`SetFace` 重建 atlas 亦在主线程）。

去掉 `mu` 字段及所有 `Lock/RLock/Unlock`，消除 `atomic.Add` 5.43% + `RLock` 3.49% = 8.92% CPU。

### 优化后对比

MEM（运行期持续分配，99.09MB 总）：

| 项 | 优化前 | 优化后 | 降幅 |
|------|--------|--------|------|
| `FeedAll`/`FeedInto` make+copy | 156.77MB | **0MB**（append 零 grow） | -100% |
| 运行期持续分配合计 | ~169MB | ~9MB（dispatch 6.65 + NewLine 2.51） | **-94%** |
| pool slice 预分配（`init.func1`） | — | 68.80MB（一次性，稳态复用） | 新增 |

CPU（锁开销消除）：

| 项 | 优化前 | 优化后 |
|------|--------|--------|
| `sync/atomic.(*Int32).Add` | 5.43% | **0**（消除） |
| `sync.(*RWMutex).RLock` | 3.49% | **0**（消除） |
| `font.(*Atlas).Get` | 10.47%（含锁） | 6.34%（纯 mapaccess） |
| `render.getGlyph` | 10.85% | 6.80% |

### 未优化（固有成本）

`blendGlyph`/`fillRect`/`memmove` 三项占 CPU ~72%，是 dumb buffer CPU 渲染固有开销（逐像素 alpha 混合 + 全屏背景清除 + backBuf→Surface 全量拷贝），需走 GBM GPU 路径（GLES 纹理上传 + `glDrawArrays` 全屏 quad）解决，`-backend drm` 下无法消除。

## 第三轮：GBM 模式 cpuBuf 中间拷贝消除

### 背景

GBM 模式 profile 发现 `copyAllToSurface`（backBuf→cpuBuf 全量拷贝）占 18.82%（2.05s/10.89s），是 GBM 特有的额外中间拷贝——Compositor 渲染到自己的 backBuf，再拷贝到 GBMSurface.cpuBuf，最后 `glTexSubImage2D` 上传 GPU。

### 方案

Compositor 每帧 Render 开头将 `backBuf` 直接绑定到 `surface.Data()`，跳过 `copyAllToSurface`。但 dumb buffer 的 mmap 是**设备内存**，逐像素 blendGlyph 直接写极慢（实测帧时间 16ms→216ms 灾难性回归），需区分对待。

**`Surface.DirectRender() bool` 接口方法**：
- `DRMSurface`（dumb buffer）：`false` — mmap 设备内存，需 backBuf 中间缓冲 + 批量 memcpy
- `GBMSurface`：`true` — cpuBuf 是堆内存，直接渲染
- `WaylandSurface`：`true` — memfd_create mmap 普通内存，直接渲染

Compositor 据 `directRender` 标志决定路径：true 时每帧绑定 `surface.Data()`，跳过 copyAll；false 时保留独立 backBuf + copyAllToSurface。GBMSurface 构造时调 `ensureCPUBuf` 确保 `Data()` 可用。

### 优化后对比（GBM 模式）

| 指标 | 优化前 | 优化后 | 变化 |
|------|--------|--------|------|
| 帧时间 | ~43ms | ~33ms | **-23%** |
| CPU 占用 | 48.46% (10.89s) | 44.24% (9.93s) | -8.8% |
| memmove | 2.05s (18.82%) | 0.26s (2.62%) | **-87%** |
| cgocall（纹理上传） | 1.49s (13.68%) | 1.74s (17.52%) | 占比升（总量降） |
| blendGlyph | 3.03s (27.82%) | 3.00s (30.21%) | 持平 |
| fillRect | 2.83s (25.99%) | 3.25s (32.73%) | 采样波动 |

`-backend drm` 模式无回归（帧时间保持 ~16ms，backBuf + copyAll 路径不变）。

### 剩余热点

| 函数 | 占比 | 性质 |
|------|------|------|
| `fillRect` | 32.73% | CPU 全屏背景清除（每帧） |
| `blendGlyph` | 30.21% | CPU 逐像素 alpha 混合 |
| `cgocall`→`glTexSubImage2D` | 17.52% | 全屏纹理上传（14.7MB/帧） |

三项合计 80%，均为 CPU 渲染固有成本。消除需 P2：GPU glyph atlas + instanced draw。

## P2 评估：GPU Glyph Atlas 方案

### 现状

当前渲染管线：CPU 逐 cell 执行 `fillRect`（背景）+ `blendGlyph`（alpha 混合），每帧全屏 ~3500 cell × 2 操作 = 7000 次逐像素循环。然后 `glTexSubImage2D` 全屏上传到 GPU，GPU 只做全屏 quad 纹理贴图（`glDrawArrays` 4 顶点）。**GPU 完全没有参与字符合成**，只是显示输出。

### 目标

将字符合成移至 GPU：CPU 只上传字形位图到 GPU 纹理 atlas，渲染时用 instanced draw 让 GPU 对每个 cell 做混合。

### 方案设计

```
CPU 侧（每帧）:
  1. 遍历 cell grid，构建 instance buffer（每 cell: x, y, glyphUV, fgColor, bgColor）
  2. glBufferSubData 上传 instance buffer（~3500 cell × 32B = 112KB/帧，远小于 14.7MB）
  3. glDrawArraysInstanced 绘制

GPU 侧（shader）:
  vertex:   计算单个 quad 的 4 角位置 + atlas UV
  fragment: 采样 atlas glyph alpha → 与 fg/bg 混合
```

### 关键组件

| 组件 | 实现 | 复杂度 |
|------|------|--------|
| Glyph Atlas 纹理 | 预光栅化所有字形到单张大纹理（如 2048×2048），LRU 淘汰 | 中（复用现有 font.Atlas 逻辑改写为纹理上传） |
| Instance Buffer | VBO 存每 cell 的位置/UV/颜色 | 中 |
| Shader | vertex + fragment，fragment 内 alpha 混合 | 中 |
| 背景清除 | shader 内判断 bgColor != default 则填充，或全屏 clear + instance 覆盖 | 低 |

### 预期收益

| 热点 | 当前 | 预期 | 节省 |
|------|------|------|------|
| fillRect | 3.25s (32.73%) | 0（GPU shader 内混合） | -100% |
| blendGlyph | 3.00s (30.21%) | 0（GPU shader 内混合） | -100% |
| glTexSubImage2D | 1.74s (17.52%) | ~0.2s（112KB instance buffer vs 14.7MB 全屏） | -88% |
| **合计** | 7.99s (80%) | ~0.2s (2%) | **-97%** |

预计帧时间从 33ms 降至 ~5ms，CPU 占用从 44% 降至 <10%。

### 实施难点与风险

1. **GLES 2.0 限制**：当前 shader 是 GLSL ES 1.00（`attribute`/`varying`），instanced draw 需要 `GL_ANGLE_instanced_arrays` 或 GLES 3.0（`glDrawArraysInstanced` + `#version 300 es`）。Intel i915 支持 GLES 3.0，但需检测运行时版本。

2. **Atlas 纹理管理**：字形按需光栅化，首次出现需 `glTexSubImage2D` 上传单个 glyph 到 atlas 纹理的对应区域。需维护空闲区域分配（shelf packing 或简化为固定网格）。

3. **颜色混合精度**：CPU blendGlyph 用 `uint16` 运算 + `>>8` 四舍五入，GPU shader 用 float 混合，可能有细微色差。对终端场景可接受。

4. **双宽字符**：CJK 占 2 cell 宽，instance buffer 需标记双宽，shader 内 quad 宽度 ×2。

5. **光标渲染**：当前 `drawCursor` 在 CPU 侧，需移至 instance buffer 或单独 draw call。

6. **dumb buffer 不受益**：`-backend drm` 无 GPU，此方案仅 GBM 模式受益。dumb buffer 仍需 CPU 渲染。

### 实施工作量评估

| 阶段 | 内容 | 估算 |
|------|------|------|
| 1 | GLES 版本检测 + 升级到 GLES 3.0 context | 0.5 天 |
| 2 | Glyph Atlas 纹理管理（shelf packing + 按需上传） | 1.5 天 |
| 3 | Instance buffer 构建（cell grid → instance data） | 1 天 |
| 4 | Shader 重写（instanced vertex + fragment 混合） | 1 天 |
| 5 | 光标/下划线/删除线/粗体斜体处理 | 1 天 |
| 6 | 测试与调优（多屏/CJK/缩放/滚动） | 1 天 |
| **合计** | | **~6 天** |

### 结论

P2 可将 GBM 模式 CPU 占用从 44% 降至 <10%，帧时间从 33ms 降至 ~5ms。但实施复杂度高（6 天），且仅 GBM 模式受益。当前 GBM 33ms 帧率（~30fps）已可满足终端交互需求，P2 建议作为后续优化储备，优先级低于功能完善（文本整形、Sixel、配置系统等）。

## P2 实施：GPU Glyph Atlas + Instanced Draw

### 已完成

1. **GLES 3.0 context**（gbm_device.go）：EGL context 从 ES2 升级到 ES3，失败自动回退 ES2
2. **GLES 3.0 函数加载**（gles.go）：`glDrawArraysInstanced`/`glVertexAttribDivisor`/`glBufferSubData`/`glUniform2f/4f/2i/3fv/4fv`/`glTexStorage2D`，optional 容错加载
3. **GPU Glyph Atlas 纹理**（gbm_surface.go）：1024×1024 R8 格式纹理，shelf packing 按需上传字形 alpha 位图
4. **Instance Buffer**（compositor.go renderGPU）：遍历 cell grid 构建 `[]CellInstance`（位置/atlasUV/fg/bg/flags），`BufferSubData` 上传（~112KB vs 14.7MB 全屏）
5. **Instanced Draw Shader**（GLES 3.00）：vertex shader 用 instance 属性计算 quad 位置+atlas UV，fragment shader 采样 atlas alpha + fg/bg 混合
6. **GPURenderer 接口**（platform/gpu.go）：Compositor 类型断言 Surface 是否实现 GPURenderer，是则走 GPU 路径，否则 CPU 路径

### 性能验证（稳态 profile）

| 指标 | GBM CPU 模式 | GBM GPU 模式 | 变化 |
|------|-------------|-------------|------|
| blendGlyph | 30.21% (3.00s) | **0%** | -100% |
| fillRect | 32.73% (3.25s) | **0%** | -100% |
| memmove | 2.62% | **0%** | -100% |
| cgocall | 17.52% | 55.00% (220ms) | 含 glDrawArraysInstanced |
| DrawInstances | — | 7.50% (30ms) | GPU draw 开销极小 |
| CPU 占用 | 44.24% | **12.49%** | **-72%** |
| 帧时间 | ~33ms | ~16ms | **-52%** |

### 待完善

- ~~渲染 23 帧后停止~~（已修复：Swap flip 同步顺序改为提交前等上次 flip，避免 EBUSY）
- ~~光标简化为反转色~~（已实现反转色光标）
- ~~下划线/删除线/斜体未实现~~（已实现：AttrFlags bit0=underline/bit1=crossedOut/bit2=italic，shader 内画线+skew）
- ~~atlas LRU 淘汰未实现~~（已实现：满时重置+2048×2048 纹理存 ~10000 字形）
- 粗体用偏移 1px 简化（atlas 内无独立粗体字形，可后续用 shader 加粗）

