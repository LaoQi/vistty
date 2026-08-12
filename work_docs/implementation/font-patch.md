# 字体补丁系统（vfp）实施方案

## 概述

为终端模拟器 vistty 增加字体补丁机制：后续补充字形（假名、Control Pictures、数学符号等）以 **append-only 补丁文件** 形式入库，打包时随二进制自动携带，程序启动时合并为 **逻辑上的单一字体** 参与渲染。

**整体状态：已完成（3 阶段全部审计通过，最终回归通过）**

**核心约束**：

1. **append-only 入库**：补丁文件一旦提交永不修改，新补丁 = 新文件。规避 git 对同一二进制文件反复 diff 的存储膨胀（与 emoji.bin.gz 一次性入库模式一致）。
2. **打包自动合并**：`go build` 直接 embed 终态补丁产物，构建零外部依赖（不需要 Python/fontTools）。
3. **逻辑单一字体**：运行时合并所有补丁 → 重组装为内存 TTF → `opentype.Parse` 得到单一 `*opentype.Font`，对渲染层表现为一个普通字体。

**方案选型**：自研紧凑补丁格式（vfp，emoji.bin 模式扩展），存矢量轮廓（glyf），否决位图路线（动态缩放 zoom 是已实现功能，位图与 size 绑定不可行）。

## 核心设计

### 布局与命名纪律

```
font/assets/
├── SarasaFixedSC-Regular.ttf      # 基础字体（不变）
├── NerdFontFallback.ttf           # Nerd fallback（不变）
├── LICENSE
├── 000-jp-kana.vfp                # 补丁，append-only，入库
├── 001-math-symbols.vfp
└── 002-control-pictures.vfp
```

- 命名：固定位数前缀（3 位，支持 1000 个补丁）+ 语义名，如 `000-jp-kana.vfp`
- **合并顺序 = 文件名升序**（前缀序号天然字典序），无 manifest.txt
- **冲突语义**：重复 rune 时序号大者优先（后合并覆盖）
- 新增补丁：取 `max(现有前缀)+1`，gen 工具 `-o` 自动分配

### vfp 格式 v1

```
Header (12B):
  magic     4B   "VFP1"
  count     4B   字形数
  dataSize  4B   数据区字节数
Index[count] (12B, 按 rune 升序):
  rune      4B
  glyphOff  4B   相对数据区起始的偏移
  advance   2B   font units（已归一化到 2048 upem）
  lsb       2B   int16
Data (dataSize 字节):
  连续排列的 glyf 简单字形字节（复合字形已在构建期展开为简单字形；
  空轮廓字形如 space 占 0 字节，由相邻 glyphOff 差值表达）
```

- 坐标统一归一化到 **unitsPerEm = 2048**（gen 工具按源字体 upem 缩放）
- 字形长度 = `next.glyphOff - cur.glyphOff`（末字形 = `dataSize - glyphOff`），索引不冗余存 len
- 索引运行时不单独拷贝：与 emoji.bin 相同，buf 常驻 + 二分查找（`font/emoji.go:93-102` 模式）

### gen-fontpatch 构建工具（cmd/gen-fontpatch）

纯 Go，基于 `golang.org/x/image/font/sfnt`（已有依赖），可行性依据：

| sfnt API | 用途 |
|----------|------|
| `sfnt.Parse` | 解析源 TTF |
| `Font.GlyphIndex(b, r)` | cmap 查 rune → GlyphIndex，0 = 缺失跳过 |
| `Font.LoadGlyph(b, x, ppem, nil)` | 轮廓段提取。**复合字形自动展开**（loadGlyf 递归处理 compound，sfnt.go:1427）；坐标按 `ppem/unitsPerEm` 缩放 — 传 `ppem = 2048<<6` 直接完成 upem 归一化；返回 Y 轴向下，编码时翻转回 Y-up |
| `Font.GlyphAdvance(b, x, ppem, h)` | advance（同样传 2048<<6 归一） |
| `Segments.Bounds()` | xMin → lsb |

流程：

```
源 TTF + rune 清单（参数: --unicodes=U+3040-30FF,U+31F0-31FF）
  → sfnt.Parse
  → 逐 rune：GlyphIndex → LoadGlyph(2048<<6) → Segments
  → Segments → glyf 简单字形编码（Y 翻转、fixed→int16 round、
    轮廓切分（MoveTo 起新轮廓）、QuadTo 转 off-curve 点、
    flags/delta 编码（初版不压缩，全 int16 delta，优化项）
  → 索引按 rune 升序 → 写 vfp
```

注意：sfnt 对彩色位图字形返回 `ErrColoredGlyph`，跳过并计入 skipped 返回值（cmd 工具输出独立计数）。空轮廓（space）正常记录零长数据。

### 运行时加载与合并（font/patch.go）

```go
//go:embed assets
var assetsFS embed.FS  // 嵌入整个 assets 目录（含 .vfp）

// 启动一次性（sync.Once，emoji 模式）
names, _ := fs.Glob(assetsFS, "assets/*.vfp")
sort.Strings(names)                    // 前缀序号 = 合并顺序
patches := parseAll(...)               // 逐个校验 magic/边界
merged := mergeIndex(patches)          // 并集，冲突后者（序号大）优先
ttf := assembleTTF(merged)             // 重组装内存 TTF
parsed, err := opentype.Parse(ttf)     // 单一 *opentype.Font
```

### TTF 重组装（font/assemble.go）

将合并后的字形重组装为合法内存 TTF。必需表（按 sfnt.Parse 实际读取路径确定）：

| 表 | 内容 | 备注 |
|----|------|------|
| head | unitsPerEm=2048、indexToLocFormat=1（long）、checkSumAdjustment 正确计算、bounds 取并集 | sfnt 不校验 checksum，但按规范算对（0xB1B0AFBA 差值） |
| hhea | ascent/descent 取各补丁源字体的归一化值（vfp header 不带，合并时从首补丁源继承；初版可用经验值 1638/-410，渲染对齐由 ChainFace 的 ascent 差值公式兜底） | numHMetrics = numGlyphs |
| maxp | version 0x00010000 + numGlyphs + TrueType 扩展字段 | |
| hmtx | numGlyphs 条 (advance, lsb)，来自索引 | |
| cmap | 全 BMP 时 format 4；含非 BMP 时 format 12（sfnt 支持 0/4/6/12） | 初版统一生成 format 12 单 subtable（platform 3/10），覆盖 BMP+非 BMP |
| loca | long 格式，numGlyphs+1 条 | glyph 0（.notdef）为空（loca[0]==loca[1]） |
| glyf | 数据区顺序拼接，4B 对齐 | |
| post | format 3.0 最简（32B） | |
| name | 最小 1 条 | 可选，防御性提供 |
| OS/2 | 最小 v0（78B） | sfnt 仅 xHeight/capHeight 路径读取，防御性提供 |

### ChainFace N 级 fallback（face.go 扩展）

现有 `FallbackFace` 硬编码两级，泛化为 N 级：

```go
type ChainFace struct {
    faces   []*OpenTypeFace  // 有序：primary → patches → nerd
    metrics Metrics          // primary 的
    ascents []int            // 各级 ascent 缓存
}
func NewChainFace(primary *OpenTypeFace, fallbacks ...*OpenTypeFace) *ChainFace
```

查找顺序：

```
ChainFace.Glyph(r):
  ① primary.Glyph(r)   [Sarasa，含 synthBlockElement 合成]
  ② patches.Glyph(r)   [vfp 重组 TTF]
  ③ nerd.Glyph(r)      [NerdFontFallback]
     hit → YOffset += levelAscent - primaryAscent → 返回
  ④ nil
```

`FallbackFaceCache` 相应泛化为 `ChainFaceCache`（或多参构造），保持 `FaceCacheProvider` 接口不变，session/compositor 零感知。

## 实施阶段

### 阶段 1: vfp 格式 + cmd/gen-fontpatch

- **状态**: 已审计
- **目标**: vfp 格式读写（font/vfp.go）+ gen 工具（font/genpatch.go 核心逻辑 + cmd/gen-fontpatch CLI），可从源 TTF 生成合法 vfp
- **实施内容**:
  1. `font/vfp.go`：`WriteVFP(w io.Writer, entries []VfpEntry, glyfData []byte)` / `ParseVFP(data []byte) (*VfpFile, error)`；`VfpFile.Find(r rune) (off, advance, lsb int, ok bool)` 二分查找；Parse 时校验 magic、索引升序、glyphOff 单调不越界
  2. `font/genpatch.go`：`GenPatch(fontData []byte, runes []rune) (vfpData []byte, missing []rune, skipped []rune, err error)`；segments→glyf 编码器（`encodeGlyf(segs Segments) []byte`）；upem 归一化
  3. `cmd/gen-fontpatch/main.go`：CLI 参数 `--font`、`--unicodes`（pyftsubset 风格 U+XXXX-YYYY 区间）、`-o` 输出（缺省自动分配 `font/assets/NNN-name.vfp` 序号）；打印生成摘要（字形数/缺失 rune/文件大小）
- **测试用例**（`font/vfp_test.go` + `font/genpatch_test.go`）:

  | # | 用例 | 断言 |
  |---|------|------|
  | 1 | TestVfpRoundTrip | 写入 3 条目（含 1 条零长 glyf）→ Parse 读回，rune/advance/lsb/off 全等 |
  | 2 | TestVfpBadMagic | 篡改 magic → Parse 报错 |
  | 3 | TestVfpIndexUnsorted | 构造乱序索引 → Parse 报错 |
  | 4 | TestVfpBounds | glyphOff 越界 / dataSize 截断 / index 截断 → 均报错 |
  | 5 | TestVfpEmpty | count=0 → Parse 成功，Find 恒 miss |
  | 6 | TestVfpFind | 升序索引二分查找：首/中/末 hit + 区间外 miss |
  | 7 | TestGenSimpleGlyph | 从 Sarasa 提取 'A'：vfp 含 U+0041，advance>0，glyf 非空且 numContours>0 |
  | 8 | TestGenCompositeGlyph | 扫描 Sarasa 前 256 rune 找 numContours<0 的复合字形（无则 Skip）→ 提取后输出 numContours>0（已展开） |
  | 9 | TestGenMissingRune | 提取 Sarasa 缺失 rune（如 U+3040 假名）→ 进 missing 列表，不进 vfp |
  | 10 | TestGenSpaceGlyph | U+0020 → 条目存在、零长 glyf、advance>0 |
  | 11 | TestGenUpemNormalize | 归一化纯函数：源 upem=1000 坐标 (500,500) → 2048 下 (1024,1024)（round 误差 ±1） |
  | 12 | TestGenIndexSorted | 乱序输入 runes → 输出索引升序 |
  | 13 | TestGenRoundTrip | GenPatch 输出 → ParseVFP 读回 → 全部 rune Find hit |
  | 14 | TestAutoNumber（cmd 层，可用纯函数测） | 模拟现有 000/002 → 分配 003 |

- **验证标准**: `go build ./...`；`go test ./font/ -run 'Vfp|Gen'` 全过；`go vet ./font/`
- **审计结果**: 通过。vfp 格式字节布局实现正确（EncodeVFP 自动排序+重分配 glyphOff+重复 rune 报错；ParseVFP 校验 magic/升序/单调/边界）；GenPatch 基于 sfnt（GlyphIndex→LoadGlyph(2048<<6) 归一化+复合自动展开→GlyphAdvance/GlyphBounds），encodeGlyfSimple 产出合法 TrueType 简单字形。**关键验证**：临时审计测试将 GenPatch 生成的 'A'/'B'/'中' 字形装配成最小 TTF，经 opentype 渲染——advance 与源字体一致、bitmap 非空（复合字形 '中' 正确展开渲染），证明编码无逻辑错误。**关键发现（供阶段2）**：short loca 要求字形偏移为偶数，装配时 glyf 数据必须 2/4 字节对齐并 pad。13 测试用例 + cmd 2 用例全过；build/vet 全过。

### 阶段 2: 运行时合并 + TTF 重组装

- **状态**: 已审计
- **目标**: `font/patch.go`（embed.FS 加载 + 合并）+ `font/assemble.go`（重组 TTF），输出可被 `opentype.Parse` 解析、字形可渲染的单一字体
- **实施内容**:
  1. `font/patch.go`：`LoadPatches(f embed.FS, pattern string) (*MergedPatch, error)` — glob + sort + ParseVFP + 合并索引（冲突后者覆盖）；`MergedPatch.FontData() ([]byte, error)` 调 assemble 产出 TTF 字节；`sync.Once` 懒加载 + `EmbeddedPatchFontData() ([]byte, error)` 公开入口
  2. `font/assemble.go`：`assembleTTF(glyphs []assembledGlyph) []byte` — 9 张表构建 + 表目录（tag/checksum/offset/length，4B 对齐）+ checkSumAdjustment 回填；cmap format 12 单 subtable
- **测试用例**（`font/patch_test.go` + `font/assemble_test.go`）:

  测试数据策略：**零测试数据文件入库** — 测试内动态调用 `GenPatch` 从内嵌 Sarasa 提取 rune 集合生成 vfp（如 patchA='A'..'C'，patchB='中'、U+1F600 非 BMP 用例另选源字体实有 rune），全链路自洽。

  | # | 用例 | 断言 |
  |---|------|------|
  | 1 | TestLoadSorted | 注入乱序文件名的 FS（抽象 `LoadPatchesFrom(names, data)` 便于测试）→ 按 000/001/002 顺序合并 |
  | 2 | TestMergeNoConflict | 两补丁无交集 → 并集，全部 Find hit |
  | 3 | TestMergeConflict | 两补丁含同 rune 不同 advance → 合并后取序号大者的 advance |
  | 4 | TestLoadNone | 无 .vfp → 返回空 MergedPatch，`FontData()` 返回 nil，不报错 |
  | 5 | TestLoadCorrupt | 任一 .vfp 坏 magic → 报错且错误信息含文件名 |
  | 6 | TestAssembleParseable | 重组 TTF → `opentype.Parse` 成功 + `NumGlyphs() == count+1`（含 .notdef） |
  | 7 | TestAssembleCmap | 每个补丁 rune `GlyphIndex != 0`；未收录 rune → 0 |
  | 8 | TestAssembleAdvance | `GlyphAdvance(rune)` == vfp 索引 advance（±1 round 误差） |
  | 9 | TestAssembleRender | `NewFace` 后 'A' 光栅化 bitmap 非空、宽高>0；与 Sarasa 同 rune 对比：advance 相等、bitmap 像素 IoU > 0.95（容忍 round/翻转误差） |
  | 10 | TestAssembleCompositeExpanded | 阶段1用例8的复合字形 → 重组后渲染 bitmap 与源字体 IoU > 0.95 |
  | 11 | TestAssembleNonBMP | 含非 BMP rune（如源字体中 U+2XXXX 实有字形）→ cmap format 12 路径 hit |
  | 12 | TestAssembleEmptyGlyph | space 渲染不崩溃、bitmap 空、advance 正确 |
  | 13 | TestAssembleChecksum | head.checkSumAdjustment 使全文件 uint32 累加和 == 0xB1B0AFBA |
  | 14 | TestAssembleMultiPatch | 3 补丁合并 → 全部 rune 可渲染 |
  | 15 | BenchmarkAssemble | 合并+重组+Parse 耗时记录（预期 ms 级，不设硬断言） |

- **验证标准**: `go test ./font/` 全过；重组字体经 fontTools `TTFont` 打开校验无异常（手动一次，非 CI）；`go vet ./font/`
- **审计结果**: 通过。assemble.go 构建 10 张表（head 54B/hhea 36B/maxp 32B/hmtx/cmap format12/loca long/glyf/post 3.0/name/OS2 78B），表目录按 tag 升序、4 字节对齐、checkSumAdjustment 正确（TestAssembleChecksum 验证全文件求和 0xB1B0AFBA）。loca 采用 **long 格式**规避 short 偶数对齐坑；glyf 每段 4 字节对齐。cmap 单 format12（platform 3/10）覆盖 BMP+非 BMP。patch.go 用 fs.Glob+sort 定合并顺序，map 去重后覆盖，FontData 空集返回 nil。**关键发现**：`//go:embed assets`（FS）+ 原各文件 `[]byte` 嵌入经实测 **Go 会去重相同文件内容**（二进制仅含 1 份 Sarasa），无体积翻倍；但仍将 embedded.go 重构为单一 assetsFS 数据源 + readAsset 缓存（零复制语义），patch.go 复用共享 assetsFS，架构更简洁。15 新测试 + 全量回归 19 包全绿；build/vet 通过。fontTools 不可用，但以 golang.org/x/image sfnt/opentype（实际运行消费者）验证为准。
- **审计要点**: 表目录 checksum 计算（uint32 大端累加 × padding）；loca long 格式偏移正确性；cmap format 12 分组（连续 rune 合并 group）；head.bounds 并集

### 阶段 3: ChainFace + 集成

- **状态**: 已审计
- **目标**: N 级 fallback 链接入 session，补丁字形端到端可渲染，无补丁时行为与现状一致
- **实施内容**:
  1. `font/face.go`：`ChainFace`（N 级，每级缓存 ascent，命中后 `YOffset += levelAscent - primaryAscent`）；保留 `NewFallbackFace` 作为 `NewChainFace(primary, fallback)` 的薄封装（向后兼容）
  2. `font/cache.go`：`NewChainFaceCache(primaryData []byte, fallbackDatas [][]byte, dpi float64)` 或 `NewFallbackFaceCache` 扩展变参；实现 `FaceCacheProvider`（接口不变）
  3. `session/master.go`：无自定义 fallback 路径时，补丁字体数据自动加入链（`EmbeddedPatchFontData()` 非空时插入第②级）；自定义 `fallback_font` 时补丁仍生效（补丁链 + 自定义尾部）
  4. `font/embedded_test.go`：CI 兜底——遍历 assets 下全部 `*.vfp` 执行 ParseVFP 校验（防手滑提交坏文件）
  5. 文档：`work_docs/test-data/font-test.md` 增补补丁字形测试字符；`AGENTS.md` 模块表更新
- **测试用例**（`font/chain_face_test.go` + 集成）:

  | # | 用例 | 断言 |
  |---|------|------|
  | 1 | TestChainPrimaryHit | '中' → Sarasa glyph（bitmap 与单 FaceCache 结果一致） |
  | 2 | TestChainPatchHit | 补丁 rune（测试动态生成，如提取的 'A'）→ 第②级 glyph |
  | 3 | TestChainNerdHit | U+E0B0 Powerline → 第③级 glyph |
  | 4 | TestChainMiss | 生僻 rune（如 U+10FFFF）→ nil, nil |
  | 5 | TestChainBaselineAlign | 构造已知 ascent 差的 3 级 face → 第②/③级 glyph YOffset 各加 `levelAscent - primaryAscent` |
  | 6 | TestChainMetrics | 返回 primary metrics |
  | 7 | TestChainClose | 全部级 Close 被调用且仅一次（注入计数 face） |
  | 8 | TestChainSingleLevel | 仅 primary → 行为等价 OpenTypeFace |
  | 9 | TestChainCacheGetFace | 同 size 两次 GetFace 返回同一实例；不同 size 不同实例 |
  | 10 | TestChainCacheCloseCascade | cache.Close 级联关闭所有 size 的所有级 |
  | 11 | TestEmbeddedPatchesValid | assets 全部 .vfp ParseVFP 通过（无补丁时 vacuous pass） |
  | 12 | TestZoomPatchGlyph | ChainFaceCache.GetFace(新 size) → 补丁 rune Glyph 非 nil 且尺寸随 size 变化 |
  | 13 | TestRegressionExisting | 现有 `go test ./...` 全绿（fallback/atlas/compositor 无回归） |

- **验证标准**: `go build ./...`；`go test ./...` 无回归；`go vet ./...`；Wayland 后端实测渲染补丁字形（test-data 文档）
- **审计结果**: 通过。ChainFace 用内部 glyphFace 接口（可注入计数 fake）实现 N 级链，ascent 差对齐公式（`YOffset += ascents[i]-ascents[0]`）与旧 FallbackFace 一致；NewFallbackFace/NewFallbackFaceCache 保留为薄封装（返回 *ChainFace/*ChainFaceCache，FaceCacheProvider 接口零改动）。session 集成：patch 层经 font.EmbeddedPatchFontData() 注入，恒在 nerd 之前且不受自定义 fallback_font 配置影响（master.patchFontData 独立字段）；InitIndependent 增加 patchFontData 参数，slave/render_harness 构造 extraDatas=[patch?, fallback?]→NewChainFaceCache，空数据自动退化（甚至退化为单字体 NewFaceCache）。**端到端验证**：用 CLI 生成真实 .vfp 放入 assets → EmbeddedPatchFontData() 返回合并 TTF（非 nil）→ 移除后恢复 nil。全量 24 包回归通过；build/vet 通过。zoom 兼容（TestZoomPatchGlyph）验证 GetFace 新 size 后 patch 字形可渲染。
- **审计要点**: 链顺序（patch 在 nerd 前）；master.go 自定义 fallback_font 时 patch 插入位置；Close 幂等

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| TTF 装配边缘情况（cmap/loca/checksum） | 阶段2用例6-13 矩阵覆盖 + fontTools 手动交叉校验一次 |
| QuadTo→glyf 编码错误（隐含中点规则） | 阶段2用例9/10 与源字体渲染 IoU 对比兜底 |
| Y 翻转/round 累积误差 | IoU 0.95 阈值而非像素全等 |
| 补丁体积膨胀（不压缩 flags） | 初版接受；优化项：flag 压缩/repeat 编码，gzip embed（参考 emoji.bin.gz） |
| 不同源字体 ascent 不一致 | ChainFace 每级 ascent 差值公式已有先例（fallback-font.md 阶段1审计通过） |

## 变更记录

| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-08-13 | - | 创建实施方案 | 用户确认 vfp 自定义格式 + append-only 入库 + assets/NNN-name.vfp 命名 + 序号即顺序（无 manifest） |
| 2026-08-13 | 阶段1 | 实施完成 | vfp.go + genpatch.go + cmd/gen-fontpatch + 13+2 测试 |
| 2026-08-13 | 阶段1 | 审计通过 | 装配最小 TTF 渲染验证 encodeGlyfSimple 正确；发现 short loca 需偶数对齐（阶段2 注意） |
| 2026-08-13 | 阶段2 | 实施完成 | assemble.go + patch.go + 15 测试 |
| 2026-08-13 | 阶段2 | 审计通过 | long loca 规避对齐坑；checkSumAdjustment 验证；embedded.go 重构为单一 assetsFS（实测 Go 已去重，无体积翻倍）；全量回归通过 |
| 2026-08-13 | 阶段3 | 实施完成 | ChainFace/ChainFaceCache + session 集成 + 14 测试 |
| 2026-08-13 | 阶段3 | 审计通过 | glyphFace 接口注入 fake；薄封装向后兼容；patch 恒在 nerd 前且不受 fallback_font 配置影响；CLI 真实 .vfp 端到端验证；24 包回归通过 |
| 2026-08-13 | 整体 | 已完成 | 3 阶段全部审计通过，最终回归全绿 |
| 2026-08-13 | 阶段3 | 实施完成 | ChainFace（N 级 fallback，faces 存 glyphFace 接口便于 Close 计数测试）+ ChainFaceCache（extraDatas 跳过空条目）；NewFallbackFace/NewFallbackFaceCache 薄封装保持兼容；session 链序 primary→patch→nerd，patch 不受自定义 fallback_font 影响（patchFontData 独立字段+embedded 注入）；TestEmbeddedPatchesValid 兜底；11 新链测试 + GenPatch/LoadPatches 真实管线 + fallback 用例迁移；全量 24 包回归通过 |
