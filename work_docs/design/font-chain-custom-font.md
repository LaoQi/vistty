# 自定义主字体时禁用默认字体查找链 — 设计方案

**状态：已实施（2026-08-13）**

## 背景与问题

字体补丁系统（vfp）落地后，字体查找链为：

```
primary → patch（assets/*.vfp 重组装 TTF）→ nerd fallback（NerdFontFallback.ttf）
```

该链在**默认主字体（内嵌 Sarasa Fixed SC）**下是自洽的：

- patch 层 hhea ascent/descent 硬编码为 Sarasa 实测值（1976/-440 @2048 upem，`font/assemble.go` patchAscent/patchDescent 常量），`TestAssembleMetricsMatchPrimary` 保证二者不漂移；
- patch 字形当前也提取自 Sarasa，风格一致。

但当用户通过 Lua 配置 `font` 指定**自定义主字体**时，两个假设同时失效：

1. **基线对齐失准**：ChainFace 对齐公式 `YOffset += patchAscent - primaryAscent` 中 patchAscent 仍是 Sarasa 的 1976，与自定义主字体的真实 ascent 的差值表现为所有补丁字形的固定垂直偏移。
2. **风格混搭**：patch 字形（Sarasa 风格）与自定义主字体混排视觉不一致；nerd fallback 虽用自身真实 hhea（公式数学自洽），风格混搭同样存在。

根本性质：patch/nerd 是**围绕内嵌 Sarasa 校准的配套资产**，与自定义主字体无配套关系。

## 目标

- 用户指定自定义主字体（`font` 配置非空）后，**默认查找链整体不装配**（patch + 内嵌 nerd 均不进入 ChainFace）。
- 用户**显式**指定的 `fallback_font` 仍然生效（显式配置优先，对齐/风格后果由用户自担）。
- 默认配置（未指定 `font`）行为与现状完全一致，零回归。

## 行为矩阵

| `font`（主） | `fallback_font` | 装配链 | 说明 |
|---|---|---|---|
| 默认（空） | 默认（空） | Sarasa → patch → nerd | 现状 |
| 默认（空） | 自定义路径 | Sarasa → patch → 自定义 | 现状 |
| 自定义路径 | 默认（空） | 仅自定义主字体 | **新行为**：patch/nerd 均不装配 |
| 自定义路径 | 自定义路径 | 自定义主 → 自定义 fallback | **新行为**：无 patch |

## 设计

### 判定条件

唯一判定：`terminal.Options.FontPath != ""`（即 Lua `font` 配置项非空）。`fallback_font` 不影响判定——它是链尾替换而非链开关。

### 改动点（预估，实施时确认）

1. **`session/master.go` `NewMaster`**：
   - `FontPath != ""` 时：`fallbackFontData` 不再默认取 `font.EmbeddedFallbackFontData()`（仅在 `FallbackFontPath != ""` 时读用户文件）；跳过 `EmbeddedPatchFontData()` 调用，`patchFontData` 保持 nil。
   - 现状代码路径（master.go:116-150 附近）已按条件分支组织，改动为收窄默认值注入条件。
2. **`terminal/render_harness.go`**：与 master 对称——当前独立调用 `font.EmbeddedPatchFontData()`（render_harness.go:48）与内嵌 fallback 默认值，需按 `opts.FontPath != ""` 同样收窄。
3. **`session/slave.go` `InitIndependent` 无需改动**：空 patch/fallback 数据自动退化为短链/单字体（NewChainFaceCache 跳过空 extraDatas、无 extras 走 NewFaceCache），退化逻辑已存在且经测试。

### 用户影响与文档

- 自定义主字体缺字形（CJK、假名、nerd 图标等）→ 渲染为空白/tofu，由用户自担；配置文档（`examples/init.lua` 注释或配置说明）需补一句说明此行为。
- 用户 Lua 插件（如自定义 statusbar）若依赖 nerd 图标，指定自定义主字体后图标消失 → 引导其自定义 `fallback_font` 指向磁盘上的 Nerd 字体文件（内嵌 NerdFontFallback.ttf 不提供引用方式，有意为之）。
- UI 内置组件（TabBar/StatusBar/Dialog/CSD）不依赖 PUA 字形（已确认 internal/ui 与 examples 无硬编码 nerd 引用），禁用 nerd 不影响内置 UI。

## 测试用例（实施时）

| # | 用例 | 断言 |
|---|------|------|
| 1 | 默认配置构建链 | ChainFace 3 级（primary+patch+nerd），与现状一致 |
| 2 | 仅自定义 font | 链仅 1 级；patch rune（如动态生成的测试补丁 rune）与 U+E0B0 均 miss |
| 3 | 自定义 font + 自定义 fallback_font | 链 2 级；fallback rune hit、patch rune miss |
| 4 | 默认 font + 自定义 fallback_font | 链 3 级（Sarasa→patch→自定义），现状不回归 |
| 5 | render_harness 路径 | 与 master 对称用例 2 |
| 6 | 回归 | `go test ./...` 全绿 |

测试注入方式：master/harness 的字体数据装配逻辑抽为纯函数（如 `resolveFontChain(opts) (primary []byte, extras [][]byte, err error)`），表驱动测试矩阵 1-5 不依赖真实后端。

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 用户困惑"图标怎么没了" | 配置文档显式说明：指定 `font` 即完整接管字体链，需要 nerd 图标时以 `fallback_font` 指向磁盘 Nerd 字体 |
| 两条装配路径（master / render_harness）行为分叉 | 抽共享纯函数 `resolveFontChain`，两处调用 |
| 未来 vfp v2 携带源字体 metrics 后本方案是否回退 | 不回退——风格混搭问题仍在；禁用默认链是更符合用户预期的语义（"我指定了字体"=完整掌控） |

## 不做的事

- 不改 ChainFace/assemble/vfp 任何机制；
- 不引入 `font_chain` 之类新配置项（行为由 `font` 是否非空唯一决定，避免配置维度爆炸）；
- 不影响 emoji 路径（emoji 走独立 rune 区间分发，与 ChainFace 无关——实施时复核 `terminal` 渲染分发确认）。
