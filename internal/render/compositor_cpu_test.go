package render

// compositor_cpu_test.go — Compositor CPU 渲染路径的像素级正确性测试。
//
// 与 compositor_test.go（GPU instance 构建 + dirty 行为回归）互补：
// 本文件用确定性 patternFace + 参考实现（draw_edge_test.go 的 ref*）
// 独立重建期望帧缓冲，与 Compositor 输出逐字节对比，
// 覆盖 cell 定位/属性/宽字符/历史滚动/copyAllToSurface/双帧稳定性。
//
// 这些测试是 #1-#6 优化的验收网：任何渲染语义变化都会在此暴露。

import (
	"fmt"
	"testing"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
)

// ============================================================
// 确定性字体：alpha 由 (rune,x,y) 纯函数生成，
// 带非零 XOffset/YOffset 以覆盖定位逻辑，字形完全落在 cell 内。
// ============================================================

const (
	pfCellW  = 8
	pfCellH  = 16
	pfAscent = 12
	pfGlyphW = 6
	pfGlyphH = 10
	// gx = px + 1, gy = py + ascent + (-10) = py + 2 → 字形占 cell 内 (1..7, 2..12)
	pfXOff = 1
	pfYOff = -10
)

type patternFace struct{}

func patternAlpha(r rune, x, y int) uint8 {
	return uint8((int(r)*7 + x*13 + y*29) % 256)
}

func patternGlyph(r rune) *font.Glyph {
	bmp := make([]byte, pfGlyphW*pfGlyphH)
	for y := 0; y < pfGlyphH; y++ {
		for x := 0; x < pfGlyphW; x++ {
			bmp[y*pfGlyphW+x] = patternAlpha(r, x, y)
		}
	}
	return &font.Glyph{
		Rune:    r,
		Bitmap:  bmp,
		Width:   pfGlyphW,
		Height:  pfGlyphH,
		XOffset: pfXOff,
		YOffset: pfYOff,
		Advance: pfCellW,
	}
}

func (f *patternFace) Glyph(r rune) (*font.Glyph, error) { return patternGlyph(r), nil }
func (f *patternFace) Metrics() font.Metrics {
	return font.Metrics{Width: pfCellW, Height: pfCellH, Ascent: pfAscent, Descent: 4}
}
func (f *patternFace) Close() error { return nil }

// ============================================================
// 期望帧缓冲构建器：独立复刻 Compositor CPU 路径语义
// （draw_edge_test.go 的 ref* 参考实现 + compositor.go 的 per-cell 流程）
// ============================================================

var (
	pfDefFg = screen.Color{R: 255, G: 255, B: 255}
	pfDefBg = screen.Color{R: 0, G: 0, B: 0}
)

// expectedCellOp 记录单个 cell 的渲染操作，按 Compositor 顺序应用到帧缓冲。
func applyCell(data []byte, stride, px, py int, cell *screen.Cell, italic bool) {
	fg := cell.Fg
	if fg.IsDefault {
		fg = pfDefFg
	}
	bg := cell.Bg
	if bg.IsDefault {
		bg = pfDefBg
	}
	fgR, fgG, fgB := fg.R, fg.G, fg.B
	bgR, bgG, bgB := bg.R, bg.G, bg.B
	if cell.Attr&screen.AttrReverse != 0 {
		fgR, fgG, fgB, bgR, bgG, bgB = bgR, bgG, bgB, fgR, fgG, fgB
	}
	if cell.Attr&screen.AttrDim != 0 {
		fgR, fgG, fgB = fgR/2, fgG/2, fgB/2
	}
	cellW := int(cell.Width) * pfCellW
	refFillRect(data, stride, px, py, cellW, pfCellH, bgR, bgG, bgB)

	if cell.Rune == 0 {
		return
	}
	g := patternGlyph(cell.Rune)
	if italic {
		g = font.ShearGlyph(g, 0.1, 0.5)
		if g == nil {
			return
		}
	}
	gx := px + g.XOffset
	gy := py + pfAscent + g.YOffset
	if cell.Attr&screen.AttrBold != 0 && !g.IsColor {
		refBlendGlyphAlpha(data, stride, px+1, gy, g.Bitmap, g.Width, g.Height, fgR, fgG, fgB, 255)
	}
	refBlendGlyphAlpha(data, stride, gx, gy, g.Bitmap, g.Width, g.Height, fgR, fgG, fgB, 255)

	if cell.Attr&screen.AttrUnderline != 0 {
		underlineY := py + pfAscent + 1
		if underlineY < py+pfCellH {
			for x := px; x < px+cellW; x++ {
				off := underlineY*stride + x*4
				if off+3 < len(data) {
					data[off] = fgB
					data[off+1] = fgG
					data[off+2] = fgR
					data[off+3] = 255
				}
			}
		}
	}
	if cell.Attr&screen.AttrCrossedOut != 0 {
		midY := py + pfCellH/2
		for x := px; x < px+cellW; x++ {
			off := midY*stride + x*4
			if off+3 < len(data) {
				data[off] = fgB
				data[off+1] = fgG
				data[off+2] = fgR
				data[off+3] = 255
			}
		}
	}
}

// buildExpected 按 Compositor CPU 路径语义（backBuf 模式，origin=(0,0)，
// 无滚动）独立构建期望帧缓冲。
func buildExpected(buf *screen.Buffer, cols, rows int) []byte {
	stride := cols * pfCellW * 4
	data := make([]byte, stride*rows*pfCellH)
	for row := 0; row < rows; row++ {
		line := buf.Line(row)
		if line == nil {
			continue
		}
		for col := 0; col < cols; col++ {
			cell := line.Cell(col)
			if cell == nil || cell.Width == 0 {
				continue
			}
			applyCell(data, stride, col*pfCellW, row*pfCellH, cell,
				cell.Attr&screen.AttrItalic != 0)
		}
	}
	return data
}

// newCPUTestCompositor 创建 backBuf 模式（DirectRender=false）compositor，
// 网格 cols×rows cell。
func newCPUTestCompositor(cols, rows int) (*Compositor, *dirtyTestSurface) {
	surf := &dirtyTestSurface{
		data:   make([]byte, cols*pfCellW*4*rows*pfCellH),
		stride: cols * pfCellW * 4,
		width:  cols * pfCellW,
		height: rows * pfCellH,
	}
	return NewCompositor(surf, &patternFace{}), surf
}

func newCalmBuffer(cols, rows, scrollback int) *screen.Buffer {
	buf := screen.NewBuffer(cols, rows, scrollback)
	// 光标闪烁相位依赖墙钟，为保证逐字节确定性统一禁用
	buf.Cursor().Visible = false
	buf.Cursor().Blinking = false
	return buf
}

// ============================================================
// 黄金帧测试：混合内容全帧逐字节对比
// ============================================================

func TestCompositorCPUGoldenFrame(t *testing.T) {
	c, surf := newCPUTestCompositor(6, 3)
	buf := newCalmBuffer(6, 3, 0)

	set := func(row, col int, r rune, attr screen.Attributes) {
		cell := buf.Cell(row, col)
		cell.Rune = r
		cell.Attr = attr
	}
	set(0, 0, 'A', 0)
	set(0, 1, 'B', screen.AttrBold)
	set(0, 2, 'C', screen.AttrUnderline)
	set(0, 4, 'D', screen.AttrItalic)
	// 宽字符：cell(1,0) Width=2，延续 cell(1,1) Width=0
	wide := buf.Cell(1, 0)
	wide.Rune = '中'
	wide.Width = 2
	cont := buf.Cell(1, 1)
	cont.Width = 0
	cont.Bg = screen.Color{R: 99} // 延续 cell 的 bg 必须被忽略（不渲染）
	set(1, 2, 'R', screen.AttrReverse)
	set(1, 3, 'M', screen.AttrDim)
	set(2, 0, 'X', screen.AttrCrossedOut)
	custom := buf.Cell(2, 2)
	custom.Rune = 'Q'
	custom.Fg = screen.Color{R: 10, G: 20, B: 30}
	custom.Bg = screen.Color{R: 40, G: 50, B: 60}
	set(2, 4, 'Z', screen.AttrBold|screen.AttrUnderline|screen.AttrDim|screen.AttrReverse)

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := buildExpected(buf, 6, 3)

	// backBuf 逐字节一致
	gotFB := fb{data: c.backBuf, stride: c.backStride, w: 6 * pfCellW, h: 3 * pfCellH}
	wantFB := fb{data: want, stride: c.backStride, w: 6 * pfCellW, h: 3 * pfCellH}
	assertSameFB(t, "golden backBuf", gotFB, wantFB)

	// copyAllToSurface 后 surface 数据一致（等 stride 全拷贝）
	surfFB := fb{data: surf.data, stride: surf.stride, w: 6 * pfCellW, h: 3 * pfCellH}
	assertSameFB(t, "golden surface", surfFB, wantFB)
}

// TestCompositorCPUWideCharBg 宽字符背景必须横跨两个 cell，
// 延续 cell（Width=0）完全跳过（bg 不绘制）。
func TestCompositorCPUWideCharBg(t *testing.T) {
	c, _ := newCPUTestCompositor(4, 1)
	buf := newCalmBuffer(4, 1, 0)

	wide := buf.Cell(0, 1)
	wide.Rune = 0 // 无字形，纯背景
	wide.Width = 2
	wide.Bg = screen.Color{R: 200, G: 0, B: 0}
	cont := buf.Cell(0, 2)
	cont.Rune = 0
	cont.Width = 0
	cont.Bg = screen.Color{R: 0, G: 0, B: 255} // 必须被忽略

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// 第二 cell 区域（宽字符右半）= 宽字符 bg，延续 cell 的蓝色 bg 不得出现
	cb, cg, cr, _ := pixelAt(c.backBuf, c.backStride, 2*pfCellW+2, 5)
	if cr != 200 || cg != 0 || cb != 0 {
		t.Fatalf("wide char right half = (%d,%d,%d), want (0,0,200) spanned bg", cr, cg, cb)
	}
}

// TestCompositorCPUAttrExact 各属性的精确像素值断言。
func TestCompositorCPUAttrExact(t *testing.T) {
	c, _ := newCPUTestCompositor(4, 1)
	buf := newCalmBuffer(4, 1, 0)

	// underline：行 py+ascent+1 = 13，颜色 = fg
	buf.Cell(0, 0).Rune = 'U'
	buf.Cell(0, 0).Attr = screen.AttrUnderline
	// crossedout：行 py+cellH/2 = 8
	buf.Cell(0, 1).Rune = 'S'
	buf.Cell(0, 1).Attr = screen.AttrCrossedOut
	// dim：fg 减半（整数除）；注意 underline 仅在 Rune != 0 时绘制
	dimCell := buf.Cell(0, 2)
	dimCell.Rune = 'M'
	dimCell.Fg = screen.Color{R: 101, G: 51, B: 25}
	dimCell.Bg = screen.Color{R: 7, G: 7, B: 7}
	dimCell.Attr = screen.AttrDim | screen.AttrUnderline

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// underline 行（cell 0）：y=13 整行为 fg（白）
	b, g, r, a := pixelAt(c.backBuf, c.backStride, 3, 13)
	if r != 255 || g != 255 || b != 255 || a != 255 {
		t.Fatalf("underline row pixel = (%d,%d,%d,%d), want white", r, g, b, a)
	}
	// underline 行上一行（y=12）不是 underline（是字形区/背景）
	// crossedout 行（cell 1）：y=8
	b, g, r, _ = pixelAt(c.backBuf, c.backStride, pfCellW+3, 8)
	if r != 255 || g != 255 || b != 255 {
		t.Fatalf("crossedout row pixel = (%d,%d,%d), want white", r, g, b)
	}
	// dim underline 行（cell 2, y=13）：fg/2 = (50, 25, 12)
	b, g, r, _ = pixelAt(c.backBuf, c.backStride, 2*pfCellW+3, 13)
	if r != 50 || g != 25 || b != 12 {
		t.Fatalf("dim underline pixel = (%d,%d,%d), want (50,25,12)", r, g, b)
	}
}

// TestCompositorCPUBoldAccumulates 粗体 = 同一字形混合两次（px+1 与 gx），
// 与参考实现逐字节一致（ golden 已覆盖，这里单独固化语义防止"优化"掉第二次混合）。
func TestCompositorCPUBoldAccumulates(t *testing.T) {
	c, _ := newCPUTestCompositor(1, 1)
	buf := newCalmBuffer(1, 1, 0)

	cell := buf.Cell(0, 0)
	cell.Rune = 'K'
	cell.Attr = screen.AttrBold

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := make([]byte, pfCellW*4*pfCellH)
	g := patternGlyph('K')
	gy := pfAscent + g.YOffset
	refFillRect(want, pfCellW*4, 0, 0, pfCellW, pfCellH, 0, 0, 0)
	refBlendGlyphAlpha(want, pfCellW*4, 1, gy, g.Bitmap, g.Width, g.Height, 255, 255, 255, 255)
	refBlendGlyphAlpha(want, pfCellW*4, g.XOffset, gy, g.Bitmap, g.Width, g.Height, 255, 255, 255, 255)

	gotFB := fb{data: c.backBuf, stride: c.backStride, w: pfCellW, h: pfCellH}
	wantFB := fb{data: want, stride: pfCellW * 4, w: pfCellW, h: pfCellH}
	assertSameFB(t, "bold double blend", gotFB, wantFB)
}

// ============================================================
// 双帧稳定性：幂等性是 dirty 优化（#4 run-length、#6 脏行拷贝）的前提
// ============================================================

// TestCompositorCPUSecondFrameStable 无 damage 第二帧 + DamageAll 第三帧，
// 三者必须逐字节一致（bg 重填使重混合幂等）。
func TestCompositorCPUSecondFrameStable(t *testing.T) {
	c, _ := newCPUTestCompositor(4, 2)
	buf := newCalmBuffer(4, 2, 0)
	buf.Cell(0, 0).Rune = 'A'
	buf.Cell(0, 1).Rune = 'B'
	buf.Cell(0, 1).Attr = screen.AttrBold
	buf.Cell(1, 2).Rune = 'C'
	buf.Cell(1, 2).Attr = screen.AttrUnderline | screen.AttrReverse

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}
	frame1 := append([]byte(nil), c.backBuf...)

	// 第二帧：全部 clean（光标行除外），不得有任何像素变化
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 2: %v", err)
	}
	assertSameFB(t, "frame2 vs frame1",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: frame1, stride: c.backStride})

	// 第三帧：DamageAll 强制全量重绘，结果仍须一致（幂等）
	buf.DamageAll()
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 3: %v", err)
	}
	assertSameFB(t, "frame3(full redraw) vs frame1",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: frame1, stride: c.backStride})
}

// TestCompositorCPUStableAcrossBlinkPhase 光标行每帧重绘，
// 即使光标不可见也重绘（cursorRow 逻辑）——重绘必须幂等，
// 否则光标闪烁相位切换会造成非光标像素抖动。
func TestCompositorCPUStableAcrossBlinkPhase(t *testing.T) {
	c, _ := newCPUTestCompositor(2, 2)
	buf := newCalmBuffer(2, 2, 0)
	buf.Cell(0, 0).Rune = 'A'
	buf.Cell(0, 0).Attr = screen.AttrBold | screen.AttrItalic // 光标行重混合最复杂的 cell

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}
	prev := append([]byte(nil), c.backBuf...)
	// 连续多帧（光标行每帧重绘），任何一帧与前帧不同即说明重绘不幂等
	for i := 0; i < 5; i++ {
		if err := c.Render(buf, 0); err != nil {
			t.Fatalf("Render %d: %v", i+2, err)
		}
		assertSameFB(t, fmt.Sprintf("frame %d stability", i+2),
			fb{data: c.backBuf, stride: c.backStride},
			fb{data: prev, stride: c.backStride})
	}
}

// ============================================================
// 历史滚动
// ============================================================

// TestCompositorCPUHistoryScroll 验证 scrollOffset 的历史行渲染、
// 超界钳制、以及 scrollChanged 触发的全量重绘。
func TestCompositorCPUHistoryScroll(t *testing.T) {
	c, _ := newCPUTestCompositor(4, 2)
	buf := newCalmBuffer(4, 2, 10)

	buf.Cell(0, 0).Rune = 'H' // 将进历史
	buf.Cell(1, 0).Rune = 'V' // 留在可视区
	buf.ScrollUp(1)           // row0('H') → history；row0 ← 旧 row1('V')

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render offset=0: %v", err)
	}
	// offset=0：row0 = 'V'
	want0 := buildExpected(buf, 4, 2)
	assertSameFB(t, "offset=0",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: want0, stride: c.backStride})

	// offset=1：row0 显示历史行（'H'），row1 显示 buf row0（'V'）
	if err := c.Render(buf, 1); err != nil {
		t.Fatalf("Render offset=1: %v", err)
	}
	hist := buf.History()
	if hist.Len() != 1 {
		t.Fatalf("history len = %d, want 1", hist.Len())
	}
	stride := 4 * pfCellW * 4
	want1 := make([]byte, stride*2*pfCellH)
	// row0 ← history line（'H' 在 col0）
	histLine := hist.Line(0)
	for col := 0; col < 4; col++ {
		cell := histLine.Cell(col)
		if cell == nil || cell.Width == 0 {
			continue
		}
		applyCell(want1, stride, col*pfCellW, 0, cell, cell.Attr&screen.AttrItalic != 0)
	}
	// row1 ← buf row0（'V'）
	for col := 0; col < 4; col++ {
		cell := buf.Line(0).Cell(col)
		if cell == nil || cell.Width == 0 {
			continue
		}
		applyCell(want1, stride, col*pfCellW, pfCellH, cell, cell.Attr&screen.AttrItalic != 0)
	}
	assertSameFB(t, "offset=1",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: want1, stride: c.backStride})

	// offset=999 → 钳制到 histLen=1，与 offset=1 一致
	if err := c.Render(buf, 999); err != nil {
		t.Fatalf("Render offset=999: %v", err)
	}
	assertSameFB(t, "offset=999 clamped",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: want1, stride: c.backStride})

	// 回到 offset=0 → scrollChanged 全量重绘，恢复可视区内容
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render back to 0: %v", err)
	}
	assertSameFB(t, "back to offset=0",
		fb{data: c.backBuf, stride: c.backStride},
		fb{data: want0, stride: c.backStride})
}

// ============================================================
// copyAllToSurface：stride 映射与裁剪
// ============================================================

// TestCopyAllToSurfaceEqualStride 等 stride：整帧一次拷贝。
func TestCopyAllToSurfaceEqualStride(t *testing.T) {
	c, surf := newCPUTestCompositor(4, 2)
	for i := range c.backBuf {
		c.backBuf[i] = uint8(i * 31)
	}
	c.copyAllToSurface()
	assertSameFB(t, "equal stride copy",
		fb{data: surf.data, stride: surf.stride},
		fb{data: c.backBuf, stride: c.backStride})
}

// TestCopyAllToSurfacePaddedStride surface stride > backBuf stride：
// 逐行拷贝，padding 字节保持原值（预填 sentinel）。
func TestCopyAllToSurfacePaddedStride(t *testing.T) {
	w, h := 4*pfCellW, 2*pfCellH
	pad := 16
	surf := &dirtyTestSurface{
		stride: w*4 + pad,
		width:  w,
		height: h,
	}
	surf.data = make([]byte, surf.stride*h)
	for i := range surf.data {
		surf.data[i] = 0xEE // sentinel
	}
	c := NewCompositor(surf, &patternFace{})
	for i := range c.backBuf {
		c.backBuf[i] = uint8(i*31 + 7)
	}

	c.copyAllToSurface()

	backStride := c.backStride
	for y := 0; y < h; y++ {
		for x := 0; x < backStride; x++ {
			got := surf.data[y*surf.stride+x]
			want := c.backBuf[y*backStride+x]
			if got != want {
				t.Fatalf("row %d byte %d: got %02x want %02x", y, x, got, want)
			}
		}
		// padding 未被触碰
		for x := backStride; x < surf.stride; x++ {
			if got := surf.data[y*surf.stride+x]; got != 0xEE {
				t.Fatalf("padding row %d byte %d overwritten: %02x", y, x, got)
			}
		}
	}
}

// TestCopyAllToSurfaceSmallerSurface surface 行数不足：裁剪不越界。
func TestCopyAllToSurfaceSmallerSurface(t *testing.T) {
	w, h := 4*pfCellW, 2*pfCellH
	surf := &dirtyTestSurface{
		stride: w * 4,
		width:  w,
		height: h,
	}
	// 只给一半行数的数据
	surfRows := h / 2
	surf.data = make([]byte, surf.stride*surfRows)
	c := NewCompositor(surf, &patternFace{})
	for i := range c.backBuf {
		c.backBuf[i] = uint8(i * 13)
	}

	c.copyAllToSurface() // 不得 panic

	backStride := c.backStride
	for i := range surf.data {
		y := i / surf.stride
		want := c.backBuf[y*backStride+i%surf.stride]
		if surf.data[i] != want {
			t.Fatalf("byte %d: got %02x want %02x", i, surf.data[i], want)
		}
	}
}

// TestCopyAllToSurfaceNilData surface 无数据：安全返回。
func TestCopyAllToSurfaceNilData(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   nil,
		stride: 0,
		width:  4 * pfCellW,
		height: 2 * pfCellH,
	}
	c := NewCompositor(surf, &patternFace{})
	c.copyAllToSurface() // 不得 panic
}

// ============================================================
// directRender 路径（wl_shm 直写）
// ============================================================

// TestCompositorCPUDirectPath DirectRender=true：直接写 surface，
// 每帧全屏 bg 填充 + 全量 cell 重绘（无 dirty 逻辑），两帧一致。
func TestCompositorCPUDirectPath(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 4*pfCellW*4*2*pfCellH),
		stride: 4 * pfCellW * 4,
		width:  4 * pfCellW,
		height: 2 * pfCellH,
	}
	c := NewCompositor(surf, &patternFace{})
	buf := newCalmBuffer(4, 2, 0)
	buf.Cell(0, 0).Rune = 'A'
	buf.Cell(1, 1).Rune = 'B'
	buf.Cell(1, 1).Attr = screen.AttrBold

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}
	frame1 := append([]byte(nil), surf.data...)

	want := buildExpected(buf, 4, 2)
	assertSameFB(t, "direct path frame",
		fb{data: surf.data, stride: surf.stride},
		fb{data: want, stride: surf.stride})

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 2: %v", err)
	}
	assertSameFB(t, "direct path frame2 stable",
		fb{data: surf.data, stride: surf.stride},
		fb{data: frame1, stride: surf.stride})
}

// ============================================================
// Overlay insets：origin 偏移
// ============================================================

type insetOverlay struct {
	top, bottom, left, right int
	rendered                 bool
}

func (o *insetOverlay) Insets() (int, int, int, int)         { return o.top, o.bottom, o.left, o.right }
func (o *insetOverlay) SetGlyphProvider(GlyphProvider)       {}
func (o *insetOverlay) SetGPUGlyphUploader(GPUGlyphUploader) {}
func (o *insetOverlay) RenderCPU(buf []byte, stride, width, height int) {
	o.rendered = true
}
func (o *insetOverlay) RenderGPU(instances *[]platform.CellInstance, width, height int) {}

// TestCompositorCPUOverlayInsets 顶部 16px + 左侧 8px inset：
// cell(0,0) 的字形必须偏移到 (8+1, 16+2)，原位保持 defBg。
func TestCompositorCPUOverlayInsets(t *testing.T) {
	c, _ := newCPUTestCompositor(4, 3)
	ov := &insetOverlay{top: pfCellH, left: pfCellW}
	c.AddOverlay(ov)

	buf := newCalmBuffer(4, 3, 0)
	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !ov.rendered {
		t.Fatal("overlay RenderCPU not called")
	}

	// 偏移后的字形区（origin + XOffset.. ）应有非 bg 像素：
	// cell(0,0) → px=8, py=16；glyph gx=9, gy=18，bitmap 6x10 全 alpha 由 pattern 决定
	g := patternGlyph('A')
	// 在字形区域找一个 alpha>0 的像素验证偏移生效
	found := false
	for y := 0; y < g.Height && !found; y++ {
		for x := 0; x < g.Width; x++ {
			if g.Bitmap[y*g.Width+x] > 128 {
				bb, gg, rr, _ := pixelAt(c.backBuf, c.backStride, 9+x, 18+y)
				if rr != 0 || gg != 0 || bb != 0 {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatal("glyph pixels not found at inset-shifted position")
	}
	// 未偏移位置（无 inset 时 glyph 会在 (1,2) 起）保持 defBg
	bb, gg, rr, _ := pixelAt(c.backBuf, c.backStride, 1, 2)
	if rr != 0 || gg != 0 || bb != 0 {
		t.Fatalf("unshifted position has glyph pixels (%d,%d,%d) - insets not applied", rr, gg, bb)
	}
}
