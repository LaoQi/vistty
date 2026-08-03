# Sixel 图形支持 — 技术探索评估

> **状态：仅技术探索，不实施。** 本文档为后续实施时的参考资料。
> 
> **评估日期：2026-08-04**

## 一、背景

Sixel（SIX pixels）是 DEC VT 系列终端的图形协议，通过 DCS（Device Control String）序列传输位图图像，每个 sixel 字符编码 6 像素列。现代终端（foot、xterm、mintty、wezterm）均有支持。

AGENTS.md 已预留扩展点：

```
| Sixel 图形 | `vte/sixel.go` | 扩展 Parser DCS 处理 |
```

`work_docs/implementation-foot-optimization.md` 阶段 7 也有初步规划。

## 二、现有架构分析

### 2.1 DCS 解析路径（已就绪，但有限制）

**Parser 状态机**（`internal/vte/parser.go`）：

1. `ESC P`（0x1B, 'P'）进入 `stateDCSString`（`parser.go:182-189`）
2. `feedDCSString()` 逐字节累积 DCS payload 到 `p.data`（`parser.go:361-379`）
3. `BEL`（0x07）或 `ESC \`（ST）终止，emit `Sequence{Action: ActionDCS, Data: payload}`
4. Terminal 侧 `executeSequences()` 的 `case vte.ActionDCS:` 是**空 case（no-op）**（`terminal/terminal.go:472`）

**关键限制：64KB 硬上限**

```go
// parser.go:376
if len(p.data) < 65536 {
    p.data = append(p.data, b)
}
```

Sixel 图像轻松超过 64KB（640×480 sixel ≈ 300KB+），超出部分被静默丢弃。**这是实施 Sixel 的首要障碍。**

**DCS payload 格式：** `Sequence.Data` 包含 DCS 参数 + 命令字节 + 数据。对于 Sixel，格式为 `P1;P2;P3 q <sixel-data>`，命令字节 `q` 标识 Sixel。当前 parser 不分离参数与命令字节，全部存在 `Data` 中。

### 2.2 Cell/Line/Buffer 结构

**Cell**（`internal/screen/cell.go:25-31`，48 字节）：

```go
type Cell struct {
    Rune  rune      // 字符（0=宽字符续接）
    Width uint8     // 1=正常, 2=CJK宽, 0=续接
    Fg    Color     // 前景色
    Bg    Color     // 背景色
    Attr  Attributes // 16-bit 位域（Bold/Dim/Italic/Underline/Blink/Reverse/CrossedOut/Clean）
}
```

**Line**（`internal/screen/line.go:3-6`）：`cells []Cell` + `dirty bool`

**Buffer**（`internal/screen/buffer.go:7-20`）：power-of-2 环形缓冲，`ScrollUp` 时旧行进入 `History`。

**问题：** Cell 无图像引用字段。Sixel 图像跨多 cell（如 10×5），每个 cell 需引用同一图像的不同子区域。

### 2.3 渲染管线

#### CPU 路径（DRM dumb buffer / Wayland wl_shm）

`Compositor.Render()`（`internal/render/compositor.go:250`）：

```
for row { for col {
    cell := line.Cell(col)
    FillRect(backBuf, bg)                           // 背景填充
    glyph := getGlyph(cell.Rune)                     // 字形查找
    blendColorGlyph(backBuf, glyph.Bitmap)           // RGBA 混合（emoji 路径）
    或 BlendGlyph(backBuf, glyph.Bitmap, fg)         // alpha 混合（普通文字）
}}
```

像素格式：**BGRA32**（`draw.go:6`：`uint32(255)<<24 | r<<16 | g<<8 | b`）

#### GPU 路径（GBM/EGL/GLES instanced draw）

`Compositor.renderGPU()`（`compositor.go:465`）：

```
for row { for col {
    cell := line.Cell(col)
    inst := CellInstance{X, Y, CellW, CellH, UV, Fg, Bg, BgA, IsColor...}
    gpu.UploadColorGlyph(rune, rgba, w, h)  → UV 坐标   // emoji 上传 atlas
    或 gpu.UploadGlyph(rune, italic, alpha, w, h) → UV
    instances = append(instances, inst)
}}
gpu.DrawInstances(instances, screenW, screenH, bgColor)
```

**GPU Atlas**（`internal/platform/gpu/renderer.go:111-130`）：
- 单张 2048×2048 GL_RGBA 纹理
- Shelf-based bin packing（`atlas.go:22`）
- GL_NEAREST 过滤
- 满时全量重置（清缓存 + 重新初始化纹理）

**CellInstance**（`internal/platform/gpu.go:5-20`，84 字节）：
- `IsColor` 标志（1.0=RGBA 采样，0=alpha 着色）
- Fragment shader（`shader.go:50-99`）已有 `v_isColor` 分支，直接采样 RGBA

### 2.4 Emoji 系统（可直接参考的图像渲染先例）

Emoji 渲染（`font/emoji.go`）是现有架构中最接近 Sixel 的先例：

| 环节 | 实现 | 文件:行 |
|------|------|---------|
| 数据存储 | gzip 内嵌 2.7MB / 1353 emoji PNG | `emoji.go:18-19` |
| 索引 | 紧凑二进制索引 + 二分查找 | `emoji.go:36-91` |
| 解码 | `image/png.Decode` + `draw.BiLinear` 缩放 | `emoji.go:135-156` |
| CPU 渲染 | `blendColorGlyph()` — RGBA alpha 混合到 BGRA | `draw.go:71-107` |
| GPU 渲染 | `UploadColorGlyph()` — RGBA 上传 atlas，`IsColor=1` | `renderer.go:313-359` |

**关键结论：** `UploadColorGlyph(r rune, rgba []byte, w, h int)` 接受任意尺寸 RGBA 数据，`blendColorGlyph()` 做任意 RGBA→BGRA 混合。两者均可直接复用于 Sixel 图像渲染。

## 三、关键技术挑战

| # | 挑战 | 严重度 | 详情 |
|---|------|--------|------|
| 1 | **DCS 64KB 数据上限** | **高** | `parser.go:376` 硬编码 `len(p.data) < 65536`。需改为流式解析或大幅提高上限。 |
| 2 | **Cell 结构无图像支持** | **高** | Cell 只有 `Rune/Width/Fg/Bg/Attr`（48B），无法引用图像。需扩展或引入旁路存储。 |
| 3 | **多 cell 跨度图像** | **高** | Sixel 图像是像素级的，终端网格是 cell 对齐的。一张图可能跨 10×5 个 cell，每个 cell 需引用同一图像的不同子区域。 |
| 4 | **GPU atlas 2048×2048 限制** | **中** | 大图像可能超出 atlas。需独立纹理或更大 atlas。 |
| 5 | **滚动与 History 集成** | **中** | 滚动时图像随行进入 `History`，需引用计数管理图像生命周期。`ScrollUp`（`buffer.go:111`）直接移动 `*Line` 指针，图像引用需随 Line 持久化。 |
| 6 | **纯 Go Sixel 解码器** | **中** | CGO_ENABLED=0 约束，需自研纯 Go Sixel 解码状态机。 |
| 7 | **渲染主线程阻塞** | **中** | 大图像解码耗时，应在 PTY read goroutine 解码而非渲染线程。 |

## 四、实现方案评估

### 4.1 方案对比

#### 方案 A：Cell 内嵌 ImageID（推荐）

```
Cell 新增:
  ImageID  uint32   // 0=无图像，>0 引用 ImageStore
  // Width 标记跨度（Width=5 表示此 cell 是 5 宽图像的起始）
  // 续接 cell 的 Width=0，与 CJK 双宽机制一致
```

- **优点：** 与现有架构一致（类似 CJK 双宽 cell），dirty tracking / scroll / history 自动继承
- **缺点：** Cell 从 48B 增至 52B，与 `todos.md` 阶段5（Cell 紧凑化 16B→12B）方向冲突

#### 方案 B：旁路图像层（Overlay 方式）

不修改 Cell，用独立的 `ImageLayer` 管理图像位置，类似 `FloatingOverlay`。

- **优点：** 不改 Cell 结构，侵入性小
- **缺点：** 不参与滚动/History，图像不随终端滚动移动 — **不符合终端语义**

#### 方案 C：特殊 Rune 映射

用 PUA 区段 rune 映射到图像，复用 `Glyph.IsColor` 路径。

- **优点：** 改动最小
- **缺点：** 无法表达多 cell 跨度图像，每 cell 只能引用整个图像 — **不实用**

**结论：方案 A 是正确选择**，但需配合 DCS 流式解析改造。

### 4.2 Cell 扩展设计

```go
type Cell struct {
    Rune     rune
    Width    uint8     // 1=正常, 2=CJK宽, 0=续接, 5+=图像跨度
    Fg       Color
    Bg       Color
    Attr     Attributes
    ImageID  uint32    // 0=无图像，>0 引用 ImageStore
    ImgX     uint16    // 此 cell 在图像中的像素 X 偏移
    ImgY     uint16    // 此 cell 在图像中的像素 Y 偏移
}
```

> 注意：增加 8 字节（ImageID 4B + ImgX 2B + ImgY 2B），Cell 从 48B→56B。后续如实施 Cell 紧凑化，可用 indirection table 优化（Cell 只存 `ImageIdx uint16` 指向外部表）。

### 4.3 ImageStore 设计

```go
type ImageStore struct {
    images map[uint32]*ImageRef
    nextID uint32
}

type ImageRef struct {
    ID       uint32
    Pix      []byte  // RGBA
    W, H     int     // 像素尺寸
    CellCols int     // 占据的 cell 列数
    CellRows int     // 占据的 cell 行数
    refCount int     // 引用计数（被多少 cell 引用）
}

func (s *ImageStore) Put(pix []byte, w, h, cellCols, cellRows int) uint32
func (s *ImageStore) Get(id uint32) *ImageRef
func (s *ImageStore) Release(id uint32)  // refCount--，为 0 时删除
```

## 五、分阶段实施计划

> 以下为后续实施时的参考计划，当前不执行。

### 阶段 1：DCS 流式解析改造 + Sixel 解码器

**目标：** 解决 64KB 限制，实现纯 Go Sixel 解码

**改动文件：**
- `internal/vte/parser.go` — DCS 状态改为回调式流式解析
- `internal/vte/sixel.go`（新建）— Sixel 状态机

**Parser 改造方案：**

引入 `DCSHandler` 接口，DCS 状态中逐字节喂给 handler，不再累积到 `p.data`：

```go
type DCSHandler interface {
    Start(params []byte)    // DCS 开始，传入参数前缀
    Feed(b byte)            // 逐字节数据
    End()                   // DCS 结束（ST/BEL）
}

type Parser struct {
    // ...
    dcsHandler DCSHandler   // 可选，非 nil 时 DCS 走流式路径
}
```

当 `dcsHandler != nil` 时，`feedDCSString()` 不再累积到 `p.data`，而是：
1. 首字节阶段解析参数（到命令字节 `q` 为止）→ `handler.Start(params)`
2. 后续数据字节 → `handler.Feed(b)`
3. ST/BEL → `handler.End()`

OSC 和其他 DCS 命令不受影响（走原有累积路径）。

**Sixel 状态机**（`internal/vte/sixel.go`，参考 foot `sixel.c`）：

状态：`SixelGround | SixelDECGRA | SixelDECGRI | SixelDECGCI | SixelRepeat | SixelColorDef`

| 字符 | 含义 |
|------|------|
| `"` | DCS raster attributes（宽高像素参数 P1;P2;P3;P4） |
| `!` | repeat 命令（`!n` 重复下一个 sixel n 次） |
| `$` | sixel newline（六角形行前进，回行首） |
| `-` | line feed（六角形行前进 + 新行） |
| `#` | color register 定义（`#N` 选择 register，`#N;P1;P2;P3` HLS/RGB 定义） |
| `?..~` (0x3F-0x7E) | sixel data 字符（6 像素列，每 bit 对应一个像素） |

输出：`image.NRGBA`（或调色板 + 像素索引，最终转 RGBA）

**预估：** 800-1200 行 Go 代码

### 阶段 2：图像存储 + Cell 扩展

**目标：** 图像在终端网格中的表示和管理

**改动文件：**
- `internal/screen/sixel.go`（新建）— ImageStore + ImageRef
- `internal/screen/cell.go` — Cell 新增 ImageID / ImgX / ImgY 字段
- `internal/screen/buffer.go` — ScrollUp/ClearRect/DeleteLines 清除 cell 时调用 ImageStore.Release
- `terminal/terminal.go:472` — 实现 `execDCS`，检测 `q` 命令 → Sixel 解码 → 写入 ImageStore → cursor 位置填充 image cell → 推进 cursor

**Cursor 推进逻辑：**

```
图像像素 W×H → cellCols = ceil(W / cellWidth), cellRows = ceil(H / cellHeight)
在 cursor (row, col) 位置开始：
  for r in 0..cellRows:
    for c in 0..cellCols:
      cell.ImageID = id
      cell.ImgX = c * cellWidth
      cell.ImgY = r * cellHeight
      cell.Width = cellCols（仅起始 cell）或 0（续接）
  cursor 推进到下一行
```

**预估：** 2-3 天

### 阶段 3：CPU 渲染路径

**目标：** DRM dumb buffer + Wayland 路径显示 Sixel

**改动文件：**
- `internal/render/compositor.go:339-419`（CPU cell 循环）
- `internal/render/draw.go` — 新增 `blitImageRegion()`（或复用 `blendColorGlyph` 的子区域版本）

**渲染逻辑：**

```go
if cell.ImageID != 0 {
    imgRef := imageStore.Get(cell.ImageID)
    srcX := int(cell.ImgX)
    srcY := int(cell.ImgY)
    // 从 imgRef.Pix 的 (srcX, srcY) 开始取 cellW×cellH 子区域
    // blit 到 backBuf 的 (px, py)
    blitImageRegion(c.backBuf, c.backStride, px, py,
        imgRef.Pix, imgRef.W, srcX, srcY, cellW, cellH)
    continue  // 跳过正常 glyph 渲染
}
```

`blendColorGlyph`（`draw.go:71-107`）已有逐像素 RGBA→BGRA 混合逻辑，新增子区域偏移参数即可复用。

**预估：** 1-2 天

### 阶段 4：GPU 渲染路径

**目标：** GBM/EGL 路径显示 Sixel

**改动文件：**
- `internal/platform/gpu/renderer.go` — 新增 `UploadImage()` 方法（独立纹理，非 atlas）
- `internal/platform/gpu.go` — `CellInstance` 新增图像纹理标记
- `internal/platform/gpu/shader.go` — fragment shader 支持独立纹理采样
- `internal/render/compositor.go:465-657`（GPU cell 循环）

**方案选择：**

| 图像尺寸 | 方案 | 理由 |
|----------|------|------|
| ≤ 512×512 | 复用 atlas `UploadColorGlyph` | 可放入 2048×2048 atlas |
| > 512×512 | 独立 GL 纹理 | 避免 atlas 溢出 |

**独立纹理方案：**

```go
// renderer.go 新增
type imageTexture struct {
    tex uint32
    w, h int
}

func (c *Renderer) UploadImage(id uint32, rgba []byte, w, h int) (tex uint32, ok bool) {
    // GenTextures + BindTexture + TexImage2D + TexSubImage2D
    // 缓存到 imageTextures[id]
}
```

**CellInstance 扩展：**

```go
type CellInstance struct {
    // ... 现有字段 ...
    ImageTexID float32  // 0=使用 atlas，>0=使用独立纹理 unit 1
}
```

**Shader 改动：**

```glsl
// Fragment shader
uniform sampler2D u_atlas;     // unit 0 — 字形 atlas
uniform sampler2D u_imgTex;    // unit 1 — 图像纹理
// ...
if (v_imageTexID > 0.5) {
    texColor = texture(u_imgTex, v_tex);
} else if (v_isColor > 0.5) {
    texColor = texture(u_atlas, v_tex);
} else {
    float a = texture(u_atlas, v_tex).r;
    texColor = vec4(v_fg, a);
}
```

**预估：** 3-4 天

### 阶段 5：缩放适配 + 边界处理 + 测试

- 字号变更时图像重新缩放（resize 事件触发）
- 图像宽度不是 cell 宽度整数倍时 padding/裁剪
- 终端 resize 时图像截断
- 测试：`img2sixel`、`cat image.sixel`、多图叠加、滚动覆盖

**预估：** 2-3 天

### 工作量汇总

| 阶段 | 估算 | 依赖 |
|------|------|------|
| 1. DCS 流式 + Sixel 解码器 | 3-5 天 | 无 |
| 2. 图像存储 + Cell 扩展 | 2-3 天 | 阶段 1 |
| 3. CPU 渲染路径 | 1-2 天 | 阶段 2 |
| 4. GPU 渲染路径 | 3-4 天 | 阶段 2 |
| 5. 缩放/边界/测试 | 2-3 天 | 阶段 3+4 |
| **总计** | **11-17 天** | |

## 六、风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Sixel 状态机复杂度 | foot 的 `sixel.c` 约 2000 行 C | 先实现最小子集（固定调色板 + 无压缩），逐步完善 |
| Cell 体积增大 | 与 `todos.md` 阶段5（Cell 紧凑化 16B→12B）冲突 | 用 `uint16` ImageID + indirection table，或外部表 |
| GPU shader 改动 | 影响 GBM 后端稳定性 | 保持 atlas 路径不变，独立纹理作为可选扩展 |
| DCS 流式解析改造 | 可能影响 OSC/其他 DCS 命令 | 用 DCSHandler 接口隔离，不影响现有 OSC 路径 |
| 大图像内存占用 | 全分辨率 RGBA 存储可能耗尽内存 | 限制最大图像尺寸（如 1024×1024），超过则降采样 |
| 渲染线程阻塞 | 大图像解码阻塞主渲染循环 | 在 PTY read goroutine 解码，只将解码后的 RGBA 送入 ImageStore |

## 七、关键代码位置索引

> 实施时需修改的文件和行号

| 位置 | 说明 |
|------|------|
| `internal/vte/parser.go:361-379` | `feedDCSString()` — DCS 数据累积，64KB 限制在此 |
| `internal/vte/parser.go:182-189` | DCS 状态入口（`ESC P`） |
| `terminal/terminal.go:472` | `case vte.ActionDCS:` — 空 case，Sixel 入口 |
| `internal/screen/cell.go:25-31` | Cell 结构 — 需新增 ImageID 字段 |
| `internal/screen/buffer.go:111-148` | `ScrollUp()` — 图像引用需随 Line 进入 History |
| `internal/screen/buffer.go:289` | `ClearRect()` — 清除 cell 时 Release 图像引用 |
| `internal/render/compositor.go:339-419` | CPU cell 渲染循环 — 需检测 ImageID |
| `internal/render/compositor.go:517-608` | GPU cell 渲染循环 — 需检测 ImageID |
| `internal/render/draw.go:71-107` | `blendColorGlyph()` — 可复用的 RGBA→BGRA 混合 |
| `internal/platform/gpu/renderer.go:313-359` | `UploadColorGlyph()` — 可复用的 RGBA 纹理上传 |
| `internal/platform/gpu/renderer.go:111-130` | Atlas 纹理初始化 — 2048×2048 限制 |
| `internal/platform/gpu/shader.go:50-99` | Fragment shader — `v_isColor` 分支 |
| `internal/platform/gpu.go:5-20` | `CellInstance` — 需扩展图像纹理标记 |
| `font/emoji.go:104-170` | EmojiFace — 图像渲染先例参考 |

## 八、参考

- **foot `sixel.c`** — Sixel 状态机参考实现（BSD 许可），约 2000 行 C
- **xterm `charproc.c`** — DCS Sixel 处理
- **mintty `sixel.c`** — Sixel 解码 + 缩放
- **wezterm `term/sixel.rs`** — Rust 实现，纯内存安全
- **Sixel 规范** — DEC STD 070 ReGIS/Sixel（文档散射在各终端实现中）
- **本项目** `work_docs/implementation-foot-optimization.md:295-308` — 阶段 7 初步规划
