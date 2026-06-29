# Emoji 支持方案

## 当前问题

1. **内嵌 Sarasa Fixed SC 字体不含 emoji 字形** → emoji 显示为空白
2. **`golang.org/x/image/opentype` 不支持彩色字体** → CBDT 返回 `ErrColoredGlyph`，COLR/CPAL/sbix/SVG 表完全不解析
3. **VTE 解析器无 variation selector 感知** → U+FE0F 被当作独立字符写入 Cell
4. **渲染管线全链路仅支持 alpha 单色混合** → `Glyph.Bitmap` 是 8-bit alpha，`blendGlyph` / GPU shader 只做前景色×alpha
5. **Cell 仅存单个 Rune** → ZWJ 序列（👨‍👩‍👧）等组合 emoji 无法表示

## 选型：`go-text/typesetting` + 系统 emoji 字体

AGENTS.md 已预留 `go-text/typesetting/harfbuzz` 作为文本整形扩展点。该库：

- 纯 Go，零 CGO，符合项目硬约束
- 支持 COLRv0/v1、CBDT/CBLC、sbix、SVG 四种彩色字形格式
- `font.Face.GlyphData()` 返回 `GlyphBitmap`（PNG 格式）/ `GlyphColor`（COLR 路径）/ `GlyphSVG`
- Harfbuzz 引擎可处理 ZWJ / variation selector / skin tone
- 依赖：`golang.org/x/image v0.23.0`（项目当前 v0.43.0，兼容）

系统 emoji 字体示例路径：

- `/usr/share/fonts/noto/NotoColorEmoji.ttf`（CBDT 格式，~10MB）
- `/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf`
- `/usr/share/fonts/noto-cjk/NotoColorEmoji.ttf`

## 分层实施计划

### P0 — 渲染层基础设施

Glyph 结构 + 彩色混合 + GPU shader，为 emoji 彩色渲染铺路。

| 改动 | 文件 | 说明 |
|------|------|------|
| `Glyph` 新增 `IsColor bool` | `font/atlas.go` | `Bitmap` 为 alpha（w*h）或 RGBA（w*h*4），由 `IsColor` 区分 |
| `blendColorGlyph` | `render/draw.go` | RGBA 位图直接 alpha 预乘混合到帧缓冲，忽略 Cell 前景色 |
| `Compositor.getGlyph` 扩展 | `render/compositor.go` | 返回 RGBA Glyph 时走 `blendColorGlyph` |
| `CellInstance` 新增 `IsColor float32` | `platform/gpu.go` | 80→84 字节，需更新测试断言 |
| `UploadGlyph` 支持 RGBA | `gpu/renderer.go` | `IsColor=true` 时直接写入 RGBA 而非 alpha→RGBA 转换 |
| GPU fragment shader 扩展 | `gpu/shader.go` | `if (isColor > 0.5)` 分支：直接采样 RGBA 纹理，跳过前景色着色 |

#### Glyph 结构变更

```go
// font/atlas.go
type Glyph struct {
    Rune     rune
    Bitmap   []byte   // alpha 位图 (w*h) 或 RGBA 位图 (w*h*4)
    Width    int
    Height   int
    XOffset  int
    YOffset  int
    Advance  int
    IsColor  bool     // true = Bitmap 是 RGBA 格式，渲染时忽略前景色
}
```

#### CPU blendColorGlyph

```go
// render/draw.go
func blendColorGlyph(data []byte, stride int, x, y int, rgba []byte, glyphW, glyphH int) {
    for gy := 0; gy < glyphH; gy++ {
        for gx := 0; gx < glyphW; gx++ {
            srcOff := (gy*glyphW + gx) * 4
            sr, sg, sb, sa := rgba[srcOff], rgba[srcOff+1], rgba[srcOff+2], rgba[srcOff+3]
            if sa == 0 {
                continue
            }
            px := (y+gy)*stride + (x+gx)*4
            if sa == 255 {
                data[px+0] = sb  // BGRA
                data[px+1] = sg
                data[px+2] = sr
                data[px+3] = 255
                continue
            }
            // alpha 预乘混合
            a := uint16(sa)
            ia := 255 - a
            data[px+0] = uint8((uint16(sb)*a + uint16(data[px+0])*ia + 128) >> 8)
            data[px+1] = uint8((uint16(sg)*a + uint16(data[px+1])*ia + 128) >> 8)
            data[px+2] = uint8((uint16(sr)*a + uint16(data[px+2])*ia + 128) >> 8)
            data[px+3] = uint8((a + uint16(data[px+3])*ia + 128) >> 8)
        }
    }
}
```

#### GPU shader 扩展

```glsl
// vertex shader — 新增 v_isColor varying
v_isColor = i_isColor;

// fragment shader — 彩色字形分支
if (inGlyph > 0.5 && v_hasGlyph > 0.5) {
    if (v_isColor > 0.5) {
        vec4 texColor = texture(u_atlas, v_tex);
        alpha = texColor.a;
        glyphColor = texColor.rgb;
    } else {
        alpha = texture(u_atlas, v_tex).r;
        glyphColor = v_fg;
    }
}
vec3 color = mix(bg, glyphColor, alpha);
```

### P1 — 字体层（`go-text/typesetting` + EmojiFace）

| 改动 | 文件 | 说明 |
|------|------|------|
| `go get go-text/typesetting` | `go.mod` | 新增依赖 |
| `font/emoji.go` 新文件 | `font/` | `EmojiFace` 结构：加载系统 emoji 字体，`Glyph(rune) (*Glyph, error)` 返回 `IsColor=true` 的 RGBA Glyph |
| emoji 字形光栅化 | `font/emoji.go` | `GlyphBitmap` (PNG) → `image.Decode` → RGBA `[]byte`；`GlyphColor` (COLR) → 路径填充到 `image.RGBA` → `[]byte` |
| emoji 字体查找 | `font/emoji.go` | 按优先级搜索：配置指定 > 常见系统路径 > 内嵌子集 |
| `Compositor` 新增 `emojiFace` | `render/compositor.go` | `getGlyph` 先查主字体，未命中且 rune 在 emoji 范围则查 emojiFace |
| `FaceCache` 扩展 | `font/cache.go` | 缓存 EmojiFace，缩放时同步重建 |
| 配置支持 | `internal/config/` | `emoji_font` 字段，允许用户指定 emoji 字体路径 |

#### EmojiFace 核心逻辑

```go
// font/emoji.go
type EmojiFace struct {
    face    *font.Face    // go-text/typesetting/font
    size    float64
    dpi     float64
    cache   map[rune]*Glyph
}

func NewEmojiFace(fontPath string, size float64, dpi float64) (*EmojiFace, error)

func (e *EmojiFace) Glyph(r rune) (*Glyph, error) {
    if g, ok := e.cache[r]; ok {
        return g, nil
    }
    gid, ok := e.face.Font.Cmap.Lookup(r)
    if !ok {
        return nil, nil
    }
    data := e.face.GlyphData(gid)
    switch d := data.(type) {
    case font.GlyphBitmap:
        if d.Format == font.PNG {
            img, _ := png.Decode(bytes.NewReader(d.Data))
            return e.rasterizeImage(img, r), nil
        }
    case font.GlyphColor:
        // COLR: 用 go-text 渲染器填充路径 → image.RGBA
        return e.rasterizeCOLR(gid, r), nil
    }
    return nil, nil
}
```

#### emoji 字体搜索优先级

1. 配置文件 `emoji_font` 字段指定路径
2. 系统常见路径（按顺序尝试）：
   - `/usr/share/fonts/noto/NotoColorEmoji.ttf`
   - `/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf`
   - `/usr/share/fonts/noto-cjk/NotoColorEmoji.ttf`
   - `/usr/share/fonts/truetype/noto/NotoColorEmoji-Regular.ttf`
   - `/usr/share/fonts/google-noto-emoji/NotoColorEmoji.ttf`
   - `/usr/local/share/fonts/NotoColorEmoji.ttf`
3. 均未找到 → emoji 支持降级为关闭（不影响普通字符渲染）

#### emoji rune 范围判断

```go
func isEmojiRune(r rune) bool {
    // 基础 emoji 范围
    switch {
    case 0x1F600 <= r && r <= 0x1F64F: // Emoticons
        return true
    case 0x1F300 <= r && r <= 0x1F5FF: // Misc Symbols and Pictographs
        return true
    case 0x1F680 <= r && r <= 0x1F6FF: // Transport and Map
        return true
    case 0x1F1E6 <= r && r <= 0x1F1FF: // Regional Indicators (flag)
        return true
    case 0x2600 <= r && r <= 0x26FF:   // Misc Symbols
        return true
    case 0x2700 <= r && r <= 0x27BF:   // Dingbats
        return true
    case 0xFE000 <= r && r <= 0xFE0F:  // Variation Selectors
        return true
    case 0x1F900 <= r && r <= 0x1F9FF: // Supplemental Symbols
        return true
    case 0x1FA00 <= r && r <= 0x1FA6F: // Chess Symbols
        return true
    case 0x1FA70 <= r && r <= 0x1FAFF: // Symbols Extended-A
        return true
    case 0x231A <= r && r <= 0x231B:   // Watch/Hourglass
        return true
    case 0x23E9 <= r && r <= 0x23F3:   // Media playback symbols
        return true
    case 0x23F8 <= r && r <= 0x23FA:   // Media control symbols
        return true
    case 0x25AA <= r && r <= 0x25FE:   // Geometric shapes
        return true
    case 0x2614 <= r && r <= 0x2615:   // Umbrella/Hot beverage
        return true
    case 0x2648 <= r && r <= 0x2653:   // Zodiac
        return true
    case 0x267F <= r && r <= 0x267F:   // Wheelchair
        return true
    case 0x2693 <= r && r <= 0x2693:   // Anchor
        return true
    case 0x26A1 <= r && r <= 0x26A1:   // High voltage
        return true
    case 0x26AA <= r && r <= 0x26AB:   // Circles
        return true
    case 0x26BD <= r && r <= 0x26BE:   // Sports
        return true
    case 0x26C4 <= r && r <= 0x26C5:   // Snowman/Sun
        return true
    case 0x26CE <= r && r <= 0x26CE:   // Ophiuchus
        return true
    case 0x26D4 <= r && r <= 0x26D4:   // No entry
        return true
    case 0x26EA <= r && r <= 0x26EA:   // Church
        return true
    case 0x26F2 <= r && r <= 0x26F3:   // Fountain/Golf
        return true
    case 0x26F5 <= r && r <= 0x26F5:   // Sailboat
        return true
    case 0x26FA <= r && r <= 0x26FA:   // Tent
        return true
    case 0x26FD <= r && r <= 0x26FD:   // Fuel pump
        return true
    case 0x2702 <= r && r <= 0x2702:   // Scissors
        return true
    case 0x2705 <= r && r <= 0x2705:   // Check mark
        return true
    case 0x2708 <= r && r <= 0x270D:   // Transport/sign
        return true
    case 0x270F <= r && r <= 0x270F:   // Pencil
        return true
    case 0x2B50 <= r && r <= 0x2B50:   // Star
        return true
    case 0x2B55 <= r && r <= 0x2B55:   // Circle
        return true
    case 0x200D == r:                   // ZWJ
        return true
    case 0x20E3 == r:                   // Combining Enclosing Keycap
        return true
    case 0xE0020 <= r && r <= 0xE007F:  // Tags (flag emoji)
        return true
    }
    return false
}
```

### P2 — VTE variation selector 感知

| 改动 | 文件 | 说明 |
|------|------|------|
| VS16/VS15 消费 | `terminal/terminal.go` | `execPrint` 检测 U+FE0F → 修改前一个 Cell 标记为 emoji presentation（不写入新 Cell）；U+FE0E → 标记为 text presentation |
| Cell 扩展 | `screen/cell.go` | 利用 `Attr`（uint16）空闲 bit 标记 emoji presentation，无需增加字段大小 |

#### Variation Selector 处理逻辑

```go
// terminal/terminal.go execPrint 内
func (t *Terminal) execPrint(seq vte.Sequence) {
    r := t.charset.current().Translate(seq.Rune)

    // VS16 (U+FE0F): 前一个 Cell 标记为 emoji presentation
    if r == 0xFE0F {
        prev := t.screen.Cell(t.cursor.Row, t.cursor.Col-1)
        if prev != nil && prev.Width > 0 {
            prev.Attr |= AttrEmojiPresentation
        }
        return // 不写入新 Cell，不移动光标
    }

    // VS15 (U+FE0E): 前一个 Cell 标记为 text presentation
    if r == 0xFE0E {
        prev := t.screen.Cell(t.cursor.Row, t.cursor.Col-1)
        if prev != nil && prev.Width > 0 {
            prev.Attr &^= AttrEmojiPresentation
        }
        return
    }

    // ... 原有逻辑
}
```

### P3 — ZWJ 序列组合（后续迭代）

| 改动 | 文件 | 说明 |
|------|------|------|
| ZWJ 序列收集 | `terminal/terminal.go` | `execPrint` 缓冲 U+200D 连接的多个 rune，组成完整 emoji 序列后写入 Cell |
| Cell 多码点存储 | `screen/cell.go` | 方案一：`EmojiSeq []rune`（变长，内存开销）；方案二：`EmojiID uint32` 引用全局 emoji 序列池（紧凑） |
| `runeWidth` 扩展 | `terminal/rune_width.go` | emoji 序列宽度固定为 2（终端惯例） |
| Emoji 序列池 | `screen/emojipool.go` | 全局 `sync.Map` 或 `map[uint32][]rune`，Cell 用 EmojiID 引用，避免变长数据 |

#### ZWJ 序列处理示意

```
输入: U+1F468 U+200D U+1F469 U+200D U+1F467
         👨       ZWJ     👩       ZWJ     👧

当前（P2 前）: 5 个独立 Cell，每个独立渲染
P3 后:        1 个 Cell（Rune=1F468, EmojiID=42, Width=2）
              + 1 个占位符 Cell（Width=0）
              emojipool[42] = []rune{0x1F468, 0x200D, 0x1F469, 0x200D, 0x1F467}
```

## emoji 宽度策略

**固定 2 列**。符合终端惯例，与大多数终端仿真器一致（iTerm2/kitty/alacritty 均 2 列）。

## Emoji 字体来源策略

**优先系统字体，未找到时降级关闭**。

理由：
- 内嵌 Noto Color Emoji 子集增加 ~3MB 二进制体积
- emoji 字体更新频繁（Unicode 每年新增 emoji），内嵌很快过时
- DRM/KMS 目标环境通常已安装系统字体
- 未找到 emoji 字体时仅影响 emoji 显示，不影响普通字符

后续可按需增加内嵌子集作为回退。

## 依赖变更

```
go get github.com/go-text/typesetting@latest
```

`go-text/typesetting` 依赖 `golang.org/x/image`，与项目现有依赖兼容。

## 配置扩展

```jsonc
// config.jsonc 新增字段
{
    // emoji 字体文件路径，空字符串表示自动搜索系统字体
    "emoji_font": ""
}
```

## 对现有功能的影响

| 功能 | 影响 | 说明 |
|------|------|------|
| 普通字符渲染 | 无 | IsColor=false 走原 alpha 路径 |
| CJK 双宽 | 无 | emoji Width=2 复用双宽机制 |
| GPU 渲染 | 小 | shader 新增 isColor 分支，性能无影响 |
| CPU 渲染 | 小 | blendColorGlyph 仅对 emoji rune 调用 |
| Atlas 缓存 | 小 | RGBA 位图占更多空间，但 emoji 数量有限（LRU 淘汰） |
| 动态缩放 | 中 | EmojiFace 需按 size 重建，FaceCache 需扩展 |
| GPU Atlas | 中 | RGBA 纹理上传路径不同，atlas 空间消耗更大 |
