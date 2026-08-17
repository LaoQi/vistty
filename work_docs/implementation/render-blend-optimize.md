# CPU 渲染路径优化 — 混合函数行级裁剪与特化

## 概述

基于 `work_docs/analysis/optimize.md` 的渲染热点分析，对 `internal/render/draw.go` 的四个混合原语（`FillRect`/`FillRectBlend`/`BlendGlyph(Alpha)`/`blendColorGlyph`）实施两项优化：

- **#1 行级裁剪**：将逐像素边界检查提升为"每行一次计算连续写入区间"，内层退化为纯切片循环（零边界检查）；
- **#2 ca=255 特化**：`BlendGlyph` 拆出独立循环，`combined = alpha*255/255` 数学上恒等于 `alpha`，省去每像素的乘除路径。

同时完成 **#7 测试基础设施扩展**——这是优化的前置验收网：既有 benchmark 从未覆盖 backBuf（DRM dumb）路径，且 `sgr_cursor` workload 会 panic。测试体系保证所有优化在像素级等价（可见区域逐字节不变）的前提下落地。

**优化决策点**：水平 bleed quirk 采用方案 B（修复为行级裁剪）。可见区域像素逐字节不变，变化的只有损坏性写入（负 x 写上一行末尾、右侧越界写下一行开头）；3 个 quirk 测试按设计翻转。

---

## 测试基础设施扩展（#7）

### 修复：render_harness theme 未注入 panic

`NewRenderHarness` 未初始化 `t.theme`，SGR 38/48 触发 `ansiColor` 时 nil 解引用 → `sgr_cursor` benchmark 崩溃。修复：镜像 `NewTerminal` 的 theme 初始化逻辑（opts.Theme 或 DefaultTheme）。

### 发现：既有 render benchmark 路径覆盖缺口

`fakeSurface.DirectRender()=true` → Compositor 走 directRender 路径（逐帧全量重绘），**backBuf 路径（DirectRender=false，即 DRM dumb buffer 实际生产路径：dirty 逐 cell 重绘 + copyAllToSurface）从未被 benchmark 覆盖**。

### 补强：BenchmarkRenderPaths 矩阵

`internal/perf/replay/`：

- `fakeSurface` 增加 `direct` 模式字段 + `newFakeSurfaceMode`；
- `Config` 新增 `FullRedraw`（每帧 `DamageAll()`，模拟滚动最坏情况）与 `DirectRender`；
- 新矩阵 `BenchmarkRenderPaths`：`{direct, backbuf_full, backbuf_steady} × {80x24, 240x67≈1080p} × {plain_text, tui_redraw}`。

关键基线（240x67 ≈ 1080p，plain_text）：direct 5.08ms、backbuf_full 5.66ms、backbuf_steady 2.65ms（主要为全帧拷贝，#6 收益上限所在）。

### 像素级测试体系（`internal/render/`）

**`draw_edge_test.go`** 五层结构：

1. **参考实现** `refFillRect`/`refFillRectBlend`/`refBlendGlyphAlpha`/`refBlendColorGlyph`：优化前 draw.go 语义的逐字拷贝，作为行为基线；
2. **表驱动边缘用例**：~50 用例覆盖四边裁剪、完全屏外、零尺寸、1x1 缓冲、stride padding、alpha 极值（0/1/254/255）、combined 边界；
3. **随机化差异化对拍**：3 函数 × 300 固定种子用例，随机 dst 初值 + 几何 + 颜色，实现 vs 参考逐字节对比（定义域排除水平 bleed：x∈[0, bufW-gw]，垂直全域）；
4. **舍入穷举**：`alpha∈[0,255] × src/dst 代表值` 验证 `(src*a+dst*ia+128)>>8` 逐位精确（SWAR 优化最易破坏的约束）；
5. **行级裁剪断言**：负 x 不写上一行末尾、右侧越界不写下一行开头、部分左裁剪可见部分正常绘制（优化后翻转的 3 个 quirk 测试）。

**`compositor_cpu_test.go`** — 确定性 `patternFace`（alpha 为 (rune,x,y) 纯函数）+ 独立期望帧构建器（`applyCell`/`buildExpected` 用 ref* 参考实现复刻 compositor per-cell 语义）：

- **黄金帧测试**：bold/italic（ShearGlyph+italicAtlas）/underline/crossedout/reverse/dim/宽字符/自定义颜色混合内容，backBuf 与 surface 双重逐字节对比；
- 宽字符 bg 横跨 + 延续 cell 跳过、属性精确像素值、粗体双重混合语义固化；
- **双帧/多帧幂等性**（#4/#6 前提）：无 damage 帧、DamageAll 全量帧、光标行重复重绘逐字节一致；
- 历史滚动：offset 渲染、999 超界钳制、scrollChanged 全量重绘、往返一致性；
- `copyAllToSurface`：等 stride 全拷贝、padding stride 逐行映射（sentinel 验证 padding 不被触碰）、行数不足裁剪、nil data 安全；
- directRender 直写路径、Overlay insets origin 偏移。

**`draw_bench_test.go`** — 各原语微基准（1920x1080 帧缓冲）：BlendGlyph / BlendGlyphAlpha(ca=200) / blendColorGlyph / FillRectCell / FillRectFullScreen / FillRectBlendHalf。

---

## #1/#2 优化方案审计（实施前）

### 不可变约束

1. 混合数学逐位不变：`(src*a + dst*ia + 128) >> 8`、`combined = alpha*ca/255`；
2. 写入顺序不变：bold 两次混合顺序、underline/crossedout 在字形之后覆盖；
3. 行为域内写入集合不变：差异化对拍定义域内逐字节一致。

### #1 行级裁剪：结构

```
行范围（循环外一次）：rowLo = max(0, -y)；rowHi = min(glyphH, rowsTotal-y)
                      rowsTotal = ceil(len(stride))，等价于当前"row*stride < len"语义
每行一次：colLo = max(0, -x)；colEnd = min(glyphW, stride/4 - x, bufCols - x)
          bufCols = (L-1)/4 + 1，L = len-3-offset（末行 buffer 尾部保护，仅 len%stride≠0 时）
内层：srcRow/dstBase 切片化，零边界检查
```

**正确性论证**：当前实现的每行写入像素集合本就是连续区间（px 随 col 单调，`px<0` 与 `px+3>=len` 均为单调条件），"每行一次算区间"与逐像素检查是同一写入集合的不同求值方式——非近似。

### 决策点：水平 bleed（方案 B，采纳）

| Quirk | 旧行为 | 新行为 |
|---|---|---|
| FillRect x<0, row>0 | 写上一行末尾（且本行少写） | 写本行 `[0, x+w)` |
| BlendGlyph x<0, row>0 | col<0 像素 bleed 到上一行末尾 | 正确裁剪 |
| 右侧越界（col≥行宽） | bleed 到下一行开头 | 正确裁剪 |

方案 B 可见区域逐字节不变，变化的仅为损坏性写入；compositor 调用点 `px = originX + col*cellW ≥ 0` 无依赖；唯一受影响合法场景是最后一列 2 倍宽 emoji 的涂抹——属修复。三个 quirk 测试按设计翻转，其余测试定义域不含 bleed 无需改动。

### #2 ca=255 特化：数学等价性

`uint16(alpha)*255/255`（alpha∈[0,255]）恒等于 `alpha`（乘积 ≤ 65025 无溢出，除以自身得原值）。`BlendGlyph` 独立循环直接取 `combined=alpha`，省每像素一次乘+除（编译器常数除强度削减后的乘移序列）；`BlendGlyphAlpha` ca=255 委托特化路径、ca=0 早退；结果等价由差异化对拍（ca=255 用例）与舍入穷举覆盖。

---

## 实施记录

### `internal/render/draw.go`

- 四个函数统一重构为行级裁剪 + 切片化内层循环；
- `fullRows` 优化：`len%stride==0`（生产 buffer 均为整行，`backBuf`/surface 皆 `len=stride*h`）时跳过逐行 `bufCols` 钳制与 `L<=0` 检查——实测收益最大单项（详见验证）；
- #2：`blendGlyph` 独立特化循环（combined=alpha），`BlendGlyph` 变为薄包装，`BlendGlyphAlpha` ca=255 委托 / ca=0 早退；
- `blendColorGlyph` 消除逐像素 `(i/4)*4`（i 恒为 4 倍数，直接 `dstBase+i`）。

### `internal/render/compositor.go`

- underline/crossedout 循环：去掉逐像素 `off+3 < len` 检查，改为循环前一次 `xEnd` 钳制到 `backStride/4` + 行有效性判断。

### `internal/render/draw_edge_test.go`

- 3 个 quirk 测试翻转为正确裁剪断言：
  - `TestFillRectNegativeXClipsToRow`（写本行 `[0, x+w)`，无 bleed）
  - `TestBlendGlyphNegativeXClipsToRow`（col<0 跳过，可见列正常）
  - `TestBlendGlyphRightOverflowClipsToRow`（越界像素跳过）

---

## 验证结果

### 测试

`go build ./...` + `go vet ./...` + `go test ./...` 全通过。差异化对拍、舍入穷举、黄金帧、幂等性、历史滚动等**原样通过零修改**——证明可见像素集合与混合数学逐字节不变。

### 性能（`draw_bench_test.go`，benchtime=2s）

| 基准 | 优化前 | 优化后 | 变化 |
|---|---|---|---|
| BlendGlyph 8x16 | 294.1 ns | 256.9 ns | -13% |
| BlendGlyphAlpha ca=200 | 378.4 ns | 316.9 ns | -16% |
| blendColorGlyph 18x16 | 987.2 ns | 782.7 ns | -21% |
| FillRectCell | 58.4 ns | 64.4 ns | 见 A/B 注 |
| FillRectFullScreen | 581 µs | 478 µs | -18%（见 A/B） |
| FillRectBlendHalf | 341.4 ns | 314.5 ns | -8% |

**同进程 A/B 关键结论**：跨会话基准存在 CPU 频率/环境噪声（同一份旧代码在不同会话测得 581µs 与 948µs），微基准横跨会话不可直接比较。临时 `fillrect_ab_test.go` 同进程 A/B 证明：`fullRows` 优化后 FullScreen **new 480µs vs old 948µs（约 2x）**、Cell 64ns vs 75ns（约 15%）。blend 三函数收益全部来自边界检查消除；编译器对常数除法已做强度削减，剩余 combined 计算开销有限——这就是为何 #1/#2 单项目标（1.6-2.2x）达成的实际约为 1.13-1.26x，2-5x 差距需 #5 SWAR 触及。

### BenchmarkRenderPaths（240x67 ≈ 1080p，50x）

backbuf_full/plain_text 5.66ms → 4.61ms（-19%）；tui_redraw 与 backbuf_steady 受环境噪声影响读数接近。backbuf_steady 2.65ms → 1.36ms 含频率波动，不作为精确结论。

---

## 变更文件清单

| 文件 | 变更 |
|---|---|
| `internal/render/draw.go` | #1 行级裁剪 + #2 ca=255 特化 + fullRows 优化 + blendColorGlyph 除 |
| `internal/render/compositor.go` | underline/crossedout 循环钳制 |
| `internal/render/draw_edge_test.go` | 新增：参考实现 + 边缘用例 + 差异化对拍 + 舍入穷举 + 裁剪断言 |
| `internal/render/draw_bench_test.go` | 新增：原语微基准 |
| `internal/render/compositor_cpu_test.go` | 新增：黄金帧 + 幂等性 + 历史 + copyAllToSurface + 直写 + insets |
| `terminal/render_harness.go` | 修复 theme 未注入 panic（sgr_cursor benchmark） |
| `internal/perf/replay/harness.go` | fakeSurface direct 模式 + Config FullRedraw/DirectRender |
| `internal/perf/replay/bench_test.go` | BenchmarkRenderPaths 矩阵 |

---

## 变更记录

| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-08-18 | #7 | 实施完成 | theme panic 修复 + fakeSurface 模式 + BenchmarkRenderPaths 矩阵 |
| 2026-08-18 | #7 | 实施完成 | draw_edge_test（参考实现/边缘/对拍/舍入/裁剪断言）+ draw_bench + compositor_cpu_test |
| 2026-08-18 | 审计 | 方案确认 | 采用方案 B（修复水平 bleed）；#2 数学等价性证明 |
| 2026-08-18 | #1/#2 | 实施完成 | draw.go 四函数行级裁剪 + ca=255 特化 + fullRows + compositor 循环钳制 |
| 2026-08-18 | #1/#2 | 验证通过 | 全量测试零修改通过；同进程 A/B 确认 FullScreen 2x、blend 1.13-1.26x |
| 2026-08-18 | #1/#2 | 收尾 | 临时 A/B 文件删除，全量 build/vet/test 通过 |