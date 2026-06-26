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

