package render

import (
	"testing"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
)

type testSurface struct {
	data   []byte
	stride int
	width  int
	height int
}

func (s *testSurface) Size() (int, int)                          { return s.width, s.height }
func (s *testSurface) Data() []byte                              { return s.data }
func (s *testSurface) Stride() int                               { return s.stride }
func (s *testSurface) Swap() error                               { return nil }
func (s *testSurface) Close() error                              { return nil }
func (s *testSurface) ResizeEvents() <-chan platform.ResizeEvent { return nil }
func (s *testSurface) OutputID() uint32                          { return 0 }
func (s *testSurface) DirectRender() bool                        { return true }
func (s *testSurface) DecoMode() uint32                          { return 2 }

type testFace struct{}

// Glyph 返回带可见 alpha 的字形位图（修复原 make([]byte,8*16) 全 0 导致字形本就不可能显示的缺陷）。
// 中心 4×8 区域 alpha=255；YOffset=-4 使 GlyphOffY = Ascent(12)+(-4) = 8。
func (f *testFace) Glyph(r rune) (*font.Glyph, error) {
	bmp := make([]byte, 8*16)
	for y := 4; y < 12; y++ {
		for x := 2; x < 6; x++ {
			bmp[y*8+x] = 255
		}
	}
	return &font.Glyph{
		Rune:    r,
		Bitmap:  bmp,
		Width:   8,
		Height:  16,
		XOffset: 0,
		YOffset: -4,
		Advance: 8,
	}, nil
}
func (f *testFace) Metrics() font.Metrics {
	return font.Metrics{Width: 8, Height: 16, Ascent: 12, Descent: 4}
}
func (f *testFace) Close() error { return nil }

func TestCompositorRenderNoDirty(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 800*600*4),
		stride: 800 * 4,
		width:  800,
		height: 600,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	err := c.Render(buf, 0)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

func TestCompositorRenderWithScrollOffset(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 800*600*4),
		stride: 800 * 4,
		width:  800,
		height: 600,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	err := c.Render(buf, 0)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
}

// ============================================================
// L2: GPU 路径 CellInstance 构建正确性（fake GPURenderer）
// 验证 compositor.renderGPU 传给 DrawInstances 的 instance 字段，
// 定位 instance 构建层是否正确填充 UV/偏移/尺寸/属性。
// ============================================================

// fakeGPUSurface 同时实现 platform.Surface 与 platform.GPURenderer，
// 记录 UploadGlyph 调用与 DrawInstances 收到的 instances。
type fakeGPUSurface struct {
	*testSurface
	uploadedRunes []rune
	uploadOK      bool
	uv            [4]float32
	drawnInst     []platform.CellInstance
	drawCount     int
}

func newFakeGPUSurface(w, h int) *fakeGPUSurface {
	return &fakeGPUSurface{
		testSurface: &testSurface{
			data:   make([]byte, w*h*4),
			stride: w * 4,
			width:  w,
			height: h,
		},
		uploadOK: true,
		uv:       [4]float32{0.1, 0.1, 0.2, 0.2},
	}
}

func (f *fakeGPUSurface) UploadGlyph(r rune, italic bool, bitmap []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
	f.uploadedRunes = append(f.uploadedRunes, r)
	if !f.uploadOK {
		return 0, 0, 0, 0, false
	}
	return f.uv[0], f.uv[1], f.uv[2], f.uv[3], true
}

func (f *fakeGPUSurface) DrawInstances(instances []platform.CellInstance, screenW, screenH int, bgColor [3]float32) error {
	f.drawnInst = append(f.drawnInst[:0], instances...)
	f.drawCount++
	return nil
}

func (f *fakeGPUSurface) BeginFrame() error { return nil }

func (f *fakeGPUSurface) ResetAtlas() {}

func newGPUCompositor() (*Compositor, *fakeGPUSurface) {
	// 80×32 → cols=10, rows=2 (metrics 8×16)
	surf := newFakeGPUSurface(80, 32)
	c := NewCompositor(surf, &testFace{})
	return c, surf
}

func findInst(surf *fakeGPUSurface, x, y float32) (platform.CellInstance, bool) {
	for _, inst := range surf.drawnInst {
		if inst.X == x && inst.Y == y {
			return inst, true
		}
	}
	return platform.CellInstance{}, false
}

// clearBuffer 把所有 cell 的 Rune 清零（NewBuffer 默认 Rune=' '，空格≠0 也会触发 UploadGlyph）。
func clearBuffer(buf *screen.Buffer) {
	for r := 0; r < buf.Rows(); r++ {
		for c := 0; c < buf.Cols(); c++ {
			cell := buf.Cell(r, c)
			cell.Rune = 0
			cell.Width = 1
		}
	}
}

func TestRenderGPUASCIICell(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	clearBuffer(buf)
	// 光标默认在 (0,0) 且 Blinking=true → 首帧不可见，不干扰断言
	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if surf.drawCount != 1 {
		t.Fatalf("drawCount=%d want 1", surf.drawCount)
	}
	if len(surf.uploadedRunes) != 1 || surf.uploadedRunes[0] != 'A' {
		t.Errorf("uploaded=%v want [A]", surf.uploadedRunes)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance at (0,0)")
	}
	// UV 来自 fake
	if inst.GlyphU0 != 0.1 || inst.V0 != 0.1 || inst.GlyphU1 != 0.2 || inst.V1 != 0.2 {
		t.Errorf("UV=(%v,%v,%v,%v) want fake uv", inst.GlyphU0, inst.V0, inst.GlyphU1, inst.V1)
	}
	// 字形偏移/尺寸来自 glyph
	if inst.GlyphOffX != 0 {
		t.Errorf("GlyphOffX=%v want 0 (glyph.XOffset)", inst.GlyphOffX)
	}
	if inst.GlyphOffY != 8 {
		t.Errorf("GlyphOffY=%v want 8 (Ascent12+YOffset-4)", inst.GlyphOffY)
	}
	if inst.GlyphW != 8 || inst.GlyphH != 16 {
		t.Errorf("Glyph size=%v×%v want 8×16", inst.GlyphW, inst.GlyphH)
	}
	// 默认前景灰 (204,204,204)/255
	if inst.FgR != 204.0/255 || inst.FgG != 204.0/255 || inst.FgB != 204.0/255 {
		t.Errorf("fg=(%v,%v,%v) want default gray", inst.FgR, inst.FgG, inst.FgB)
	}
	// bg=default black → HasBg=0
	if inst.HasBg != 0 {
		t.Errorf("HasBg=%v want 0 (default bg)", inst.HasBg)
	}
	if inst.CellW != 8 || inst.CellH != 16 {
		t.Errorf("cell size=%v×%v want 8×16", inst.CellW, inst.CellH)
	}
}

func TestRenderGPUEmptyRuneNoUpload(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	clearBuffer(buf)
	cell := buf.Cell(0, 0)
	cell.Rune = 0 // 无字符
	cell.Width = 1

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(surf.uploadedRunes) != 0 {
		t.Errorf("expected no UploadGlyph calls, got %v", surf.uploadedRunes)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("empty cell should still produce an instance (background quad)")
	}
	if inst.GlyphU0 != 0 || inst.GlyphU1 != 0 || inst.V0 != 0 || inst.V1 != 0 {
		t.Errorf("empty rune UV should be 0, got (%v,%v,%v,%v)", inst.GlyphU0, inst.V0, inst.GlyphU1, inst.V1)
	}
}

func TestRenderGPUUploadFailDegraded(t *testing.T) {
	c, surf := newGPUCompositor()
	surf.uploadOK = false // UploadGlyph 恒返回 ok=false
	buf := screen.NewBuffer(10, 2)
	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("expected instance even when upload fails")
	}
	// 降级：UV=0、GlyphOffX=0（修复后默认值）、GlyphW/H=metrics 默认
	if inst.GlyphU0 != 0 || inst.GlyphU1 != 0 {
		t.Errorf("degraded UV should be 0, got u0=%v u1=%v", inst.GlyphU0, inst.GlyphU1)
	}
	if inst.GlyphOffX != 0 {
		t.Errorf("degraded GlyphOffX=%v want 0 (修复后默认值，原为 metrics.Width)", inst.GlyphOffX)
	}
	if inst.GlyphW != 8 || inst.GlyphH != 16 {
		t.Errorf("degraded GlyphW/H=%v×%v want metrics 8×16", inst.GlyphW, inst.GlyphH)
	}
}

func TestRenderGPUAttrsAndBold(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	cell := buf.Cell(0, 0)
	cell.Rune = 'A'
	cell.Attr = screen.AttrUnderline | screen.AttrCrossedOut | screen.AttrItalic | screen.AttrBold | screen.AttrReverse

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance")
	}
	// AttrFlags: underline(1)+crossedOut(2)=3；italic 不计入 AttrFlags（由 font 层 shear 预生成字形），Bold 不计入
	if inst.AttrFlags != 3 {
		t.Errorf("AttrFlags=%v want 3 (underline|crossed)", inst.AttrFlags)
	}
	// Bold → GlyphOffX += 1。但 italic 字形经 ShearGlyph(0.1,0.5) 后 XOffset=-1
	// (round(0.5*0.1*15)=1 左移)，Bold 再 +1 → GlyphOffX=0
	if inst.GlyphOffX != 0 {
		t.Errorf("Bold+Italic GlyphOffX=%v want 0 (italic XOffset=-1 + bold +1)", inst.GlyphOffX)
	}
	// Reverse: fg/bg 交换。原 fg=灰(204), bg=黑(0) → 交换后 fg=黑, bg=灰, HasBg=1
	if inst.FgR != 0 {
		t.Errorf("Reverse fg.R=%v want 0 (swapped from bg)", inst.FgR)
	}
	if inst.BgR != 204.0/255 {
		t.Errorf("Reverse bg.R=%v want 204/255 (swapped from fg)", inst.BgR)
	}
	if inst.HasBg != 1 {
		t.Errorf("Reverse HasBg=%v want 1", inst.HasBg)
	}
}

func TestRenderGPUDoubleWidth(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	cell := buf.Cell(0, 0)
	cell.Rune = '中'
	cell.Width = 2
	buf.Cell(0, 1).Width = 0 // 占位符

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance for double-width cell")
	}
	if inst.CellW != 16 {
		t.Errorf("double-width CellW=%v want 16 (2*metrics.Width)", inst.CellW)
	}
	// 占位符 cell(0,1) 应被跳过，无 instance
	if _, found := findInst(surf, 8, 0); found {
		t.Error("placeholder cell at x=8 should not produce an instance")
	}
}

func TestRenderGPUCursorSwap(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	cur := buf.Cursor()
	cur.Row = 0
	cur.Col = 0
	cur.Visible = true
	cur.Blinking = false // 关闭闪烁，确保可见
	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance at cursor cell")
	}
	// 光标反转 fg/bg 并强制 HasBg=1
	if inst.HasBg != 1 {
		t.Errorf("cursor cell HasBg=%v want 1", inst.HasBg)
	}
	// 原 fg=灰 bg=黑 → 交换 fg=黑 bg=灰
	if inst.FgR != 0 || inst.BgR != 204.0/255 {
		t.Errorf("cursor swap fg.R=%v bg.R=%v want (0, 204/255)", inst.FgR, inst.BgR)
	}
}

func TestRenderGPUHasBgLogic(t *testing.T) {
	c, surf := newGPUCompositor()
	buf := screen.NewBuffer(10, 2)
	// cell(0,0): 非默认背景 → HasBg=1
	c0 := buf.Cell(0, 0)
	c0.Rune = 'A'
	c0.Bg = screen.Color{R: 255, G: 0, B: 0}
	// cell(0,1): 默认背景 → HasBg=0
	c1 := buf.Cell(0, 1)
	c1.Rune = 'B'
	c1.Bg = screen.Color{IsDefault: true}

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst0, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance at (0,0)")
	}
	if inst0.HasBg != 1 {
		t.Errorf("non-default bg: HasBg=%v want 1", inst0.HasBg)
	}
	inst1, ok := findInst(surf, 8, 0)
	if !ok {
		t.Fatal("no instance at (8,0)")
	}
	if inst1.HasBg != 0 {
		t.Errorf("default bg: HasBg=%v want 0", inst1.HasBg)
	}
}
