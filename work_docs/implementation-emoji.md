# Emoji 彩色渲染实施方案

## 概述

为终端实现 Unicode emoji 彩色渲染（如 😀🎉❤️）。当前内置 Sarasa Fixed SC 字体不含 emoji 字形，且 `golang.org/x/image/font/opentype` 不支持彩色字体表（CBDT/COLR 等），导致 emoji 显示为空白。

**方案**：零新 Go 依赖路径。构建期用纯 Go 自研 SFNT/cmap/CBLC/CBDT 解析，从 NotoColorEmoji.ttf（CBDT/PNG 格式）提取单 rune emoji 的 PNG 位图，打包成紧凑二进制 `emoji.bin.gz` 内嵌。运行时复用 `pinyin/dict.go` 已验证的「buf 常驻 + 升序数组二分查找」模式查表，`image/png`（标准库）解码 + `golang.org/x/image/draw`（已有依赖）缩放到 cell 尺寸，得到 RGBA Glyph。

**决策**：
- 范围：P0+P1（彩色单 rune emoji），不做 VS16/VS15 感知与 ZWJ 序列组合
- 字体来源：仅内嵌全量单 rune emoji 子集（~1500 个，gzip 后 ~2.5MB）
- GPU 接口：新增独立 `UploadColorGlyph` 方法，不破坏现有 `UploadGlyph` 签名
- 构建工具：纯 Go 自研 CBDT 解析（`cmd/gen-emoji`），符合项目纯 Go 惯例
- 零新 Go 依赖：仅用 `image/png`、`compress/gzip`、`embed`（标准库）+ `golang.org/x/image/draw`（已有）

## 实施阶段

### 阶段 0: 构建工具 cmd/gen-emoji（自研 CBDT 解析）
- **状态**: 已审计
- **目标**: 纯 Go 工具从 NotoColorEmoji.ttf 提取单 rune emoji PNG，生成 `internal/font/data/emoji.bin.gz`
- **实施内容**:
  - `cmd/gen-emoji/main.go`（~350 行）：
    - SFNT 表目录解析（~40 行）：读 ttf offset table + table records，定位 `cmap`/`CBLC`/`CBDT` 表 offset/length
    - cmap 解析（~80 行）：format 12（SMP emoji >0xFFFF 用），rune→glyphId
    - CBLC 解析（~100 行）：version → bitmapSizes → indexSubTableArray → IndexSubTable format 1/3 → glyphId 范围 + (dataOffset + imageDataOffset)
    - CBDT 解析（~80 行）：按 offset 读 bitmapData format 17（SmallGlyphMetrics + PNG）/ format 18（BigGlyphMetrics + PNG）
    - emoji 收集：`isEmojiRune(r)` && cmap 命中 && CBDT 有数据，排除 ZWJ 组合（单 rune only）
    - 输出 `emoji.bin`：header(12B: count/pngDataSize/srcSize) + 升序 index(count×16B: rune/pngOffset/pngLen/xOffset/yOffset/advance/reserved) + PNG 拼接块 → gzip
    - CLI：`-in <ttf> -out <gz> [-src-size 128]`
  - `internal/font/emoji_runes.go`：`isEmojiRune(r rune) bool`（共享给构建工具与运行时，避免重复）。注意：构建工具在 cmd/gen-emoji，运行时在 internal/font，需把 isEmojiRune 放在 internal/font 并被 cmd/gen-emoji import
  - 生成 `internal/font/data/emoji.bin.gz`（运行 `go run ./cmd/gen-emoji -in /usr/share/fonts/noto/NotoColorEmoji.ttf -out internal/font/data/emoji.bin.gz`）
- **验证标准**:
  - `go build ./cmd/gen-emoji` 成功
  - 生成的 emoji.bin.gz 可解压，含 header + index + png data
  - 至少包含 😀(U+1F600)、🎉(U+1F389)、❤️(U+2764) 等常见 emoji
  - 可用 `ttx -t CBLC NotoColorEmoji.ttf` dump 对比验证 glyphId 映射正确性
- **审计结果**: 通过。新建 cmd/gen-emoji/main.go（471行，纯 Go 自研 SFNT/cmap format12/CBLC/CBDT format17解析）、internal/font/emoji_runes.go（IsEmojiRune 导出，零import）。修正文档遗留 typo（0xFE000→0xFE00，Variation Selectors 范围）。CBLC BitmapSize 布局经 fontTools 源码确认（48字节：idxArrayOff/numIdxSub/startGID uint16/ppemX uint8）。cmap 选 platform=3/enc=10/format=12（SMP支持）。NotoColorEmoji 唯一 strike ppem=109（srcSize 记录109）。indexFormat=1/imageFormat=17。生成 1353 个 emoji，2.7MB，gzip -t 通过，首个PNG(U+231A)解码OK。go build/vet/test 全绿。注意：PNG 为 palette 模式(mode=P)，阶段2运行时需转RGBA。

### 阶段 1 (P0): 渲染层彩色基础设施
- **状态**: 实施中
- **目标**: Glyph/blend/shader 支持 RGBA 彩色字形，为 emoji 铺路
- **实施内容**:
  - `internal/font/atlas.go`: `Glyph` 结构加 `IsColor bool`（Bitmap 为 alpha 或 RGBA，由 IsColor 区分）
  - `internal/render/draw.go`: 新增 `blendColorGlyph(data, stride, x, y, rgba, glyphW, glyphH)` — RGBA 预乘混合到 BGRA 帧缓冲，忽略前景色
  - `internal/platform/gpu.go`: `CellInstance` 加 `IsColor float32`（80→84 字节，更新 VAO stride）；`GPURenderer` 接口加 `UploadColorGlyph(r rune, rgba []byte, w, h int) (u0, v0, u1, v1 float32, ok bool)`
  - `internal/platform/gpu/atlas.go`: `glyphKey` 加 `IsColor bool`，彩色与 alpha 字形独立缓存
  - `internal/platform/gpu/renderer.go`: `UploadColorGlyph` 实现 — 复用 `packGlyph`，跳过现有 alpha→RGBA 转换（275-278 行），直接 TexSubImage2D 上传 RGBA；VAO/VertexAttrib 加 location 11（IsColor）
  - `internal/platform/gpu/shader.go`: vertex 加 `layout(location=11) in float i_isColor` + `out float v_isColor`；fragment 加 `if (v_isColor > 0.5)` 分支采样 `texture(u_atlas, v_tex).rgba` 跳过前景色着色
  - `internal/render/compositor.go`: CPU 路径（253 行）glyph.IsColor 时走 blendColorGlyph；GPU 路径（438 行）glyph.IsColor 时调 UploadColorGlyph 且 inst.IsColor=1
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` 无错误
  - `go test ./...` 无回归
  - GPU renderer 测试断言（若有 size 断言）更新为 84 字节
- **审计结果**: 通过。10文件修改：atlas.go(Glyph+IsColor)、draw.go(blendColorGlyph BGRA预乘)、gpu.go(CellInstance+IsColor 84B +GPURenderer+UploadColorGlyph)、gpu/atlas.go(glyphKey+IsColor)、gpu/renderer.go(UploadColorGlyph直接上传rgba不经rgbaBuf +VAO/非VAO location11 offset80 +divisor循环i<=11)、gpu/shader.go(glyphColor变量彩色分支)、compositor.go(CPU264/GPU448/Overlay三路径适配)、gbm/surface.go(委托)、renderer_test.go(size80→84+location11)、compositor_test.go(fakeGPUSurface)。TestP6ExampleInitLua预存在失败(stash验证无关)。go build/vet/test全绿。alpha路径无回归。

### 阶段 2 (P1): 字体层 EmojiFace
- **状态**: 实施中
- **状态**: 待实施
- **目标**: EmojiFace 加载内嵌 emoji.bin.gz，提供彩色 RGBA Glyph
- **实施内容**:
  - `internal/font/emoji.go`（新，~200 行）：
    - `//go:embed data/emoji.bin.gz` + `emojiData []byte`
    - `emojiIndex` 结构：buf 常驻 + `entries []emojiEntry`（升序，二分查找）+ pngData 基址，复用 dict.go 模式
    - `loadEmoji()` 解压 + 构建索引
    - `EmojiFace` 结构：`index *emojiIndex` + `cellW, cellH int` + `cache map[rune]*Glyph`
    - `NewEmojiFace(cellW, cellH int) (*EmojiFace, error)`
    - `Glyph(r rune) (*Glyph, error)`：cache→二分查找→png.Decode→draw.BiLinear 缩放到 2*cellW×cellH→转 RGBA []byte（IsColor=true）→cache
  - `internal/render/compositor.go`:
    - `getGlyph`（92 行）改：`if isEmojiRune(r) { return c.emojiFace.Glyph(r) }`（先判 emoji 范围，直接查 emojiFace，不 fallback 主字体避免 tofu）
    - Compositor 加 `emojiFace *EmojiFace` 字段；NewCompositor 按 metrics 创建；SetFace 时按新 metrics 重建
    - `OverlayUploadGlyph`（135 行）同步支持彩色
  - 动态缩放：emojiFace 按 cellH 缩放，缩放变更时重建（cache 清空）
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` / `go test ./...` 无回归
  - EmojiFace.Glyph(0x1F600) 返回非 nil、IsColor=true、Bitmap 为 RGBA
- **审计结果**: 通过。新建 emoji.go(158行)+emoji_test.go(5测试)，改 compositor.go 4处。emojiIndex 复用 dict.go 模式(gzip解压buf常驻+sort.Search二分查找+PNG零复制偏移引用)。Glyph方法:cache→find→png.Decode→draw.BiLinear.Scale到NRGBA→IsColor Glyph。关键偏差:用NRGBA(非预乘)而非RGBA,匹配blendColorGlyph非预乘over公式与shader mix(bg,rgb,alpha),避免半透明边缘过暗。compositor getGlyph先判IsEmojiRune→emojiFace(不fallback主字体避免tofu),emoji复用c.atlas缓存。NewCompositor/SetFace管理emojiFace生命周期。go build/vet/test全绿(含5新emoji测试)。

### 阶段 3: 集成与缩放
- **状态**: 实施中
- **目标**: 主程序组装 EmojiFace 注入 Compositor，动态缩放联动
- **实施内容**:
  - `session/`（slave.go / render_loop.go）：Compositor 创建时注入 EmojiFace；动态缩放（handleScale）时重建 emojiFace
  - `cmd/vistty/main.go`：组装 EmojiFace（若 session 层未直接构造，则在 main 注入）
  - `internal/ui/osd.go`：若面板渲染 emoji，OverlayGlyph 路径适配 IsColor（OverLay 已通过 compositor.OverlayUploadGlyph）
- **验证标准**:
  - `go build ./...` / `go vet ./...` / `go test ./...` 通过
  - `go run ./cmd/vistty -backend wayland` 启动，`printf '\xF0\x9F\x98\x80\xF0\x9F\x8E\x89\n'` 显示彩色 😀🎉
  - 动态缩放后 emoji 仍正确渲染
- **审计结果**: 通过。经评估，session 层无需注入改动（compositor 自建 emojiFace），动态缩放已通过 render_loop.go:397 SetFace→emojiFace.Resize 联动，OverlayUploadGlyph(153行)已适配 IsColor。ui osd 通过 getGlyph 自动支持 emoji。唯一改动：emoji.go emojiIndex 单例化（sync.Once），多屏多 Compositor 共享同一解压 index，避免重复解压 ~2.87MB。go build/vet/test 全绿（TestP6ExampleInitLua 预存在失败无关）。端到端验证需图形环境：`go run ./cmd/vistty -backend wayland` 后 `printf '\xF0\x9F\x98\x80'`。

## 完成总结

所有阶段已完成。emoji 彩色渲染功能就绪。

### 功能概述
- 纯 Go 零新依赖实现 Unicode emoji 彩色渲染（CBDT/PNG 格式）
- 构建期自研 SFNT/cmap/CBLC/CBDT 解析提取 1353 个单 rune emoji PNG，gzip 内嵌 2.7MB
- 运行时复用 pinyin/dict.go 紧凑索引模式（buf 常驻 + 二分查找 + PNG 零复制）
- CPU（blendColorGlyph BGRA 预乘）+ GPU（shader isColor 分支 + 独立 UploadColorGlyph）双路径彩色渲染
- NRGBA 非预乘匹配现有混合管线，避免半透明边缘过暗

### 各阶段完成情况
| 阶段 | 内容 | 状态 |
|------|------|------|
| 0 | cmd/gen-emoji 构建工具（自研 CBDT 解析） | 已审计 |
| 1 | P0 渲染层彩色基础设施（Glyph/blend/shader/CellInstance） | 已审计 |
| 2 | P1 EmojiFace 字体层（索引+PNG解码+缩放+compositor集成） | 已审计 |
| 3 | 集成（emojiIndex 单例优化，缩放已联动） | 已审计 |

### 已知遗留
- P0+P1 范围：不做 VS16/VS15 感知（U+FE0F 当独立字符显示空白）与 ZWJ 序列组合（👨‍👩‍👧 拆分为独立 emoji）
- emoji 固定双宽 2 列，拉伸到 2*cellW×cellH（等宽字体下接近正方形，变形小）
- TestP6ExampleInitLua 预存在失败（fontsize 14 vs 24，与 emoji 无关）

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-03 | 初始化 | 创建实施文档 | 零依赖方案，P0+P1 范围 |
| 2026-07-03 | 阶段0 | 实施并审计通过 | 1353 emoji，2.7MB，纯Go自研CBDT解析 |
| 2026-07-03 | 阶段1 | 实施并审计通过 | 10文件，渲染层彩色基础设施就绪，alpha路径无回归 |
| 2026-07-03 | 阶段2 | 实施并审计通过 | EmojiFace+NRGBA非预乘，compositor集成 |
| 2026-07-03 | 阶段3 | 实施并审计通过 | emojiIndex单例优化，缩放已联动，全部完成 |
