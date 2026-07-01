# GBM 模式 CPU 开销热点分析与优化计划

> 评估日期：2026-07-01
> 环境：Intel N100, linux/amd64, CGO_ENABLED=0
> 评估方式：`internal/perf/replay` 三层归因 benchmark + pprof CPU/mem profile

## 一、L3 渲染层 CPU Profile（离线 benchmark，CPU fallback 路径）

| 函数 | flat% | 说明 |
|------|-------|------|
| `render.BlendGlyphAlpha` | **62.06%** | 逐像素 alpha 混合，绝对热点 |
| `render.FillRect` | **29.07%** | 逐像素 uint32 写入 |
| `render.Compositor.Render` | 2.27% | cell 遍历 + 颜色解析 |
| `font.Atlas.Get` → `mapaccess2_fast32` | 1.37% | 字形缓存 map 查找 |

> CPU fallback 路径数据。**GBM GPU 模式下 BlendGlyphAlpha 和 FillRect 完全消除**，由 GPU instanced draw 替代。

## 二、GBM GPU 模式每帧 CPU 开销分布

### 渲染阶段 (`renderGPU` → `DrawInstances`)

| 开销点 | 来源 | 量级 | 占比 |
|--------|------|------|------|
| cell 遍历 + CellInstance 构建 | `renderGPU` rows×cols 循环 | O(cells) | ~30-40% |
| 字体 atlas map 查找 | `getGlyph` → `font.Atlas.Get` | O(cells) | ~10-15% |
| GPU atlas UploadGlyph (仅新字形) | `gpu.UploadGlyph` alpha→RGBA | 稳态 O(0) | ~0% 稳态 |
| BufferSubData 上传 instances | `gpu.DrawInstances` | ~150KB/帧 | ~5-10% |
| GL 状态设置 | `VertexAttribPointer`×11 + `Divisor`×18 | ~35 次 GL 调用 | ~10-15% |
| eglMakeCurrent | `gpu.BeginFrame()` | 1 次 C 调用 | ~1-2% |

### 提交阶段 (`Swap`)

| 开销点 | 来源 | 量级 | 占比 |
|--------|------|------|------|
| waitForFlipComplete | flip 同步（可能阻塞） | 0~16ms | 最大瓶颈 |
| eglMakeCurrent (第2次) | `Swap()` | 1 次 C 调用 | ~1% |
| eglSwapBuffers | 触发 GBM buffer swap | 1 次 C 调用 | ~2-5% |
| gbm_surface_lock_front_buffer | 获取 BO | 1 次 C 调用 | ~1% |
| drmModeAddFB | ioctl | 1 次 | ~1% |
| AtomicCommit | ioctl (NonBlock) | 1 次 | ~1% |
| releaseBO (上一帧释放) | drmRmFB + SurfaceReleaseBuffer | 2 次 C 调用 | ~1% |

### 锁竞争

| 锁 | 竞争场景 | 影响 |
|----|----------|------|
| `commitMu` | `Swap()` vs `onFlipComplete()` | 3 次加锁/帧，中等 |
| `terminal RWMutex` | Render 读锁 vs PTY 写入写锁 | 高，PTY 持写锁时阻塞渲染 |

## 三、L1/L2 层 CPU 开销（对比参考）

| 层 | 热点 | flat% |
|----|------|-------|
| L1-parser | `runtime.memmove` | 23.4% |
| L1-parser | `runtime.memclrNoHeapPointers` | 16.1% |
| L1-parser | `vte.feedGround` (cum) | 28.8% |
| L2-screen | `runtime.memmove` | 21.5% |
| L2-screen | `runtime.memclrNoHeapPointers` | 10.9% |
| L2-screen | `vte.feedGround` (cum) | 22.3% |

内存分配热点（L3 mem profile）：
- `vte.feedGround` 分配 **62.4%** 堆内存（544MB）
- `vte.FeedInto` 累计 **93%**

## 四、Benchmark 数据汇总

### L3-render（CPU 路径，fakeSurface）

| workload | ns/op | B/op | allocs/op |
|----------|-------|------|-----------|
| plain_text_4k | 665,794 | 657 | 0 |
| plain_text_64k | 662,578 | 9,671 | 0 |
| cjk_scroll_4k | 905,362 | 430 | 0 |
| cjk_scroll_64k | 878,526 | 4,141 | 0 |
| sgr_cursor_4k | 418,178 | 182 | 0 |
| sgr_cursor_64k | 837,153 | 4,365 | 1 |
| scroll_stress | 806,417 | 5,861 | 0 |
| tui_redraw | 918,449 | 3,599 | 0 |

> L3 渲染在 80×24 终端下约 0.4-0.9ms/帧，远低于 16ms 帧预算。GBM GPU 模式下进一步降低（消除 BlendGlyph/FillRect），瓶颈转移到 GL 状态设置和 flip 同步。

### GBM UploadGlyph（GPU 路径，离线测试）

| 子测试 | ns/op | B/op | allocs/op |
|--------|-------|------|-----------|
| UploadGlyphCold | 29.66 | 4 | 0 |
| UploadGlyphWarm | 16.52 | 0 | 0 |

> 稳态字形上传极快（16ns/次）。atlas cache 命中率是关键。

## 五、优化计划（按阶段实施）

### 阶段1（P0）：VAO 缓存 attribute 配置

- **文件**：`internal/platform/gl/gles.go`, `internal/platform/gpu/renderer.go`
- **问题**：每帧 `DrawInstances` 执行 11 次 `VertexAttribPointer` + 9 次 `VertexAttribDivisor` + 9 次重置 = 29 次 GL 调用
- **方案**：GLES 3.0 VAO 缓存 attribute 配置，初始化时设置一次，每帧只 `BindVertexArray`
- **预期**：减少 29 次/帧 GL 调用 → 1 次 `BindVertexArray`
- **风险**：低。GLES 3.0 核心支持 VAO，`HasInstancedDraw()` 已保证 GLES 3.0+

### 阶段2（P1）：renderGPU cell 遍历优化

- **文件**：`internal/render/compositor.go`
- **问题**：per-cell 重复计算 `float32(defBg.R)/255`、`float32(c.metrics.Ascent)` 等
- **方案**：提取循环不变量到循环外，减少冗余 float 转换
- **预期**：减少 15 次/cell float 转换
- **风险**：低

### 阶段3（P2）：instance VBO 预分配 + UploadGlyph 批量优化

- **文件**：`internal/platform/gpu/renderer.go`
- **问题**：
  - instance VBO 预分配 `width*height*80` 字节，1080p ~160MB VRAM
  - UploadGlyph alpha→RGBA 逐像素循环
- **方案**：
  - VBO 上限改为 `cols*rows` 实际值或合理上限（如 4096）
  - alpha→RGBA 转换用 unsafe 批量写入或 SIMD
- **预期**：VRAM 从 ~160MB 降至 ~1MB；新字形上传加速
- **风险**：低

### 待评估（架构级，暂不实施）

| 优化 | 风险 | 说明 |
|------|------|------|
| terminal 双缓冲消除 RWMutex 竞争 | 高 | 需改动 screen.Buffer 架构 |
| commitMu 合并加锁 | 中 | 增加锁持有时间，需仔细评估 |

## 六、结论

GBM GPU 模式下 CPU 热点已从像素操作（BlendGlyphAlpha 62%）转移到 **(1) GL 状态设置开销（无 VAO 缓存）和 (2) flip 同步等待 + 锁竞争**。阶段1-3 优化可将 CPU 端每帧开销降低约 40-50%。

## 七、实施记录

### 阶段1（P0）：VAO 缓存 — 已完成

- `internal/platform/gl/gles.go`：添加 `GenVertexArrays`/`BindVertexArray`/`DeleteVertexArrays`/`HasVAO`（3 个 optional 符号）
- `internal/platform/gpu/renderer.go`：`Init()` 创建 VAO 缓存全部 attribute 配置；`DrawInstances` VAO 路径仅 `BindVertexArray`（减少 29 次/帧 GL 调用），GLES 2.0 fallback 保留原逻辑；`Close()` 释放 VAO
- 验证：build/vet/test 全通过

### 阶段2（P1）：renderGPU 循环不变量提取 — 已完成

- `internal/render/compositor.go`：`renderGPU` 循环前提取 8 个不变量（`defBgR/G/B`、`originXF/YF`、`ascentF`、`cellWF`、`cellHF`），消除 per-cell 冗余 float32 转换
- 验证：build/vet/test 全通过

### 阶段3（P2）：instance VBO 预分配上限 — 已完成

- `internal/platform/gpu/renderer.go`：`Init()` 中 `maxInstances` 从 `c.width*c.height`（~160MB）改为 `65536`（~5MB）；`DrawInstances` 添加截断保护
- UploadGlyph alpha→RGBA 优化未实施（稳态 0 次，收益极低，有正确性风险）
- 验证：build/vet/test 全通过

## 八、实机测试方案

### 测试目标

在真实 DRM/KMS 环境下验证 GBM GPU 渲染管线的正确性和性能，检测：
1. 屏幕是否正常显示（htop 界面渲染正确）
2. 是否存在卡死/冻结（flip 超时、D 状态线程、帧停止推进）
3. CPU/内存占用水平
4. 帧时间分布和丢帧率
5. 锁竞争情况

### 测试环境要求

| 条件 | 说明 |
|------|------|
| 空闲 TTY | 如 tty2（无 X11/Wayland 合成器占用） |
| root 权限 | /dev/ttyN 仅 root 可写；DRM master 需无其他合成器 |
| video/input 组 | 用户需在 video 和 input 组中 |
| htop | 已安装（/usr/bin/htop） |

### 测试文件

| 文件 | 用途 |
|------|------|
| `scripts/htop-init.lua` | htop 专用 init.lua（shell=/usr/bin/htop, backend=drm-gbm） |
| `scripts/gbm-bench.sh` | 主测试脚本（编译→启动→监控→收集→报告） |
| `scripts/gbm-check.sh` | 运行中检测脚本（进程状态/帧推进/DRM 状态/错误检测） |

### 测试步骤

#### 步骤1：主测试（自动化）

```bash
# 在 SSH 或另一个 TTY 上执行（需要 root）
sudo ./scripts/gbm-bench.sh -t 2 -d 90

# 参数：
#   -t 2     使用 tty2
#   -d 90    运行 90 秒
```

脚本自动完成：
1. 编译 vistty（CGO_ENABLED=0）
2. 检查 htop 和 DRM 设备
3. 以 root 启动 vistty（drm-gbm + htop + profiling + fps）
4. 后台监控进程资源（每 2 秒采样 RSS/线程数）
5. 运行指定时长后 SIGTERM 正常关闭
6. 生成汇总报告

#### 步骤2：运行中检测（手动，从另一个终端）

```bash
# 从 SSH 或另一个 TTY 执行
./scripts/gbm-check.sh <vistty-pid>

# 检测项：
#   1. 进程存活 + 状态（D 状态 = 卡死）
#   2. 帧是否在推进（3 秒内新帧数）
#   3. DRM CRTC 状态
#   4. fd 数量（泄漏检测）
#   5. flip 超时 / 渲染错误 / EGL 错误
```

#### 步骤3：人工观察屏幕

在 vistty 运行期间，按 `Ctrl+Alt+F2` 切换到 tty2 观察：
- htop 界面是否正常渲染（进程列表、颜色、光标）
- 底部状态栏是否显示时钟
- 画面是否流畅（无卡顿/撕裂/花屏）
- 按 `Super+Q` 是否能正常退出

#### 步骤4：Profile 分析

```bash
# CPU 热点
go tool pprof -top /tmp/vistty-bench-<TS>/cpu.prof

# 内存分配
go tool pprof -top /tmp/vistty-bench-<TS>/mem.prof

# 锁竞争
go tool pprof -top /tmp/vistty-bench-<TS>/mutex.prof

# 交互式分析
go tool pprof /tmp/vistty-bench-<TS>/cpu.prof
```

### 产出文件（/tmp/vistty-bench-\<TS\>/）

| 文件 | 内容 |
|------|------|
| `vistty` | 编译后的二进制 |
| `cpu.prof` | CPU profile（全程采样） |
| `mem.prof` | 堆内存快照（退出时） |
| `mutex.prof` | 互斥锁 profile（退出时） |
| `fps.log` | 每帧耗时（stderr） |
| `debug.log` | 调试日志 |
| `monitor.log` | 外部资源监控（每 2 秒采样） |
| `report.txt` | 汇总报告 |

### 判定标准

| 指标 | 通过 | 失败 |
|------|------|------|
| 运行时长 | ≥ 目标时长 | 提前退出 |
| 退出码 | 0 | 非 0 |
| GPU 路径 | instanced draw ready | fallback CPU |
| 平均帧时间 | < 16ms | ≥ 16ms |
| P99 帧时间 | < 32ms | ≥ 32ms |
| 丢帧率 | < 5% | ≥ 5% |
| Flip 超时 | 0 次 | > 0 次 |
| D 状态线程 | 0 个 | > 0 个 |
| 峰值 RSS | < 100MB | ≥ 100MB |
| fd 泄漏 | fd 数稳定 | 持续增长 |
| 屏幕显示 | htop 正常渲染 | 花屏/黑屏/冻结 |

## 九、实机测试结果（2026-07-01）

> 环境：Intel N100, drm-gbm, GLES 3.2, 双屏(crtc=88 + crtc=145), htop, 90s

### 总体判定：✅ 全部通过

| 指标 | 结果 | 阈值 | 判定 |
|------|------|------|------|
| 运行时长 | 90s | ≥90s | ✅ |
| 退出码 | 0 | 0 | ✅ |
| GPU 路径 | instanced draw ready (GLES 3.2) | ready | ✅ |
| 平均帧时间 | 6.38ms | <16ms | ✅ |
| P50 帧时间 | 6.31ms | - | ✅ |
| P95 帧时间 | 8.08ms | - | ✅ |
| P99 帧时间 | 9.57ms | <32ms | ✅ |
| 最大帧时间 | 46.08ms（首帧初始化） | - | ⚠️ 仅首2帧 |
| 丢帧率 | 0.5%（2/416，仅首2帧） | <5% | ✅ |
| 平均 FPS | 156.8 | >60 | ✅ |
| Flip 超时 | 0 | 0 | ✅ |
| 渲染错误 | 0 | 0 | ✅ |
| 线程数 | 34（稳定） | - | ✅ |
| RSS | ~28MB（稳定，无增长） | <100MB | ✅ |
| fd 泄漏 | 无 | - | ✅ |

### CPU Profile 热点分析（实机，drm-gbm + GPU instanced draw）

CPU 采样率极低（4s/92s = 4.34%），说明 **CPU 几乎空闲**，GPU 承担了绝大部分渲染工作。

| 函数 | flat% | cum% | 说明 |
|------|-------|------|------|
| `runtime.cgocall` | **36.0%** | 36.3% | purego → C 调用开销（EGL/GLES/GBM） |
| `renderGPU` 自身 | 9.8% | **66.5%** | cell 遍历 + instance 构建 |
| `Syscall6` (ioctl) | 15.3% | 15.3% | DRM ioctl（AddFB/AtomicCommit/epoll） |
| `mapaccess2_fast32` | 6.0% | 11.5% | 字形 atlas cache map 查找 |
| `DrawInstances` | 0.3% | **25.3%** | GPU instanced draw 提交 |
| `BufferSubData` | 0% | **19.5%** | instance VBO 数据上传 |
| `MakeCurrent` (egl) | 0.3% | 12.0% | EGL context 切换 |
| `getGlyph` | 0.8% | 12.3% | 字形查找（Atlas.Get + 首次光栅化） |
| `UploadGlyph` | 1.5% | 5.0% | 新字形上传到 GPU atlas |
| `Swap` | 0.3% | 4.5% | GBM page flip 提交 |

**renderGPU 内部开销分解**（focus=renderGPU）：

| 子项 | 占 renderGPU | 说明 |
|------|-------------|------|
| `cgocall`（purego→C） | 50.4% | DrawInstances + MakeCurrent + BufferSubData 等 GL 调用 |
| `mapaccess2_fast32` | 9.0% | 字形 cache 查找 |
| `getGlyph` | 1.1% | 字形查找入口 |
| `UploadGlyph` | 2.3% | 新字形上传 |
| `DrawInstances` | 0.4% | GPU draw call 提交 |
| `renderGPU` 自身 | 14.7% | cell 遍历 + CellInstance 构建 + 颜色解析 |

### 内存 Profile

| 分配源 | 大小 | 说明 |
|--------|------|------|
| `ime/pinyin.loadDict` | 53.4MB | 拼音词库（go:embed dict.bin.gz 解压） |
| `io.ReadAll` | 31.5MB | 词库解压读取 |
| `platform.map.init.0` | 0.5MB | DRM 属性映射 |
| **总计** | **85.9MB** | 词库占 98.8%，渲染相关 <1MB |

### 互斥锁 Profile

| 锁 | delay | 说明 |
|----|-------|------|
| `gbm.GBMSurface.Swap` → `sync.Mutex.Unlock` | 19.98ms (93.1%) | commitMu 竞争（Swap vs onFlipComplete） |
| `runtime.unlock` | 1.48ms (6.9%) | 运行时内部锁 |

commitMu 锁竞争总延迟仅 ~20ms（90 秒内），**几乎可忽略**。

### 多屏渲染

- 双屏同时渲染：crtc=88（stride=3328, ~832px 宽）+ crtc=145（stride=10240, ~2560px 宽）
- 两屏各 400 帧，帧计数一致，无丢帧
- 两阶段渲染（Render→Present）正确工作

### 首帧开销

前 2 帧耗时 41ms/46ms，后续稳定在 6-8ms。首帧开销来自：
- GPU shader 编译 + 链接
- VAO 创建 + attribute 配置
- Atlas 纹理分配（2048×2048）
- 首批字形 UploadGlyph
- 首次 modeset（atomic commit with AllowModeset）

### 关键结论

1. **GPU instanced draw 完全生效**：CPU 采样率仅 4.34%，CPU 近乎空闲
2. **VAO 优化生效**：DrawInstances 中无 VertexAttribPointer/Divisor 开销（已被 VAO 缓存）
3. **帧时间优秀**：P50=6.3ms, P99=9.6ms，远低于 16ms 帧预算
4. **无卡死/无错误/无泄漏**：90 秒运行稳定
5. **剩余 CPU 热点**：`cgocall`(36%) + `mapaccess2_fast32`(6%)，前者是 purego 固有开销，后者是字形 cache 查找
