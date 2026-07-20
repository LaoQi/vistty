package render

import (
	"fmt"
	"testing"

	"github.com/LaoQi/vistty/font"
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
	beginFrameErr error
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

func (f *fakeGPUSurface) UploadColorGlyph(r rune, rgba []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
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

func (f *fakeGPUSurface) BeginFrame() error {
	if f.beginFrameErr != nil {
		return f.beginFrameErr
	}
	return nil
}

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
	// 默认前景白 (255,255,255)/255
	if inst.FgR != 255.0/255 || inst.FgG != 255.0/255 || inst.FgB != 255.0/255 {
		t.Errorf("fg=(%v,%v,%v) want default white", inst.FgR, inst.FgG, inst.FgB)
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

// TestCompositorGPUDisabledPermanent 验证 GPU BeginFrame 失败后永久禁用，
// 不会每帧重新尝试 GPU 路径（P2-27）。
func TestCompositorGPUDisabledPermanent(t *testing.T) {
	surf := newFakeGPUSurface(80, 32)
	surf.beginFrameErr = fmt.Errorf("synthetic EGL failure")
	c := NewCompositor(surf, &testFace{})
	buf := screen.NewBuffer(10, 2)
	clearBuffer(buf)
	buf.Cell(0, 0).Rune = 'A'

	// 第一帧：GPU BeginFrame 失败 -> 降级 CPU
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render frame 1: %v", err)
	}
	if !c.gpuDisabled {
		t.Fatal("gpuDisabled should be true after BeginFrame failure")
	}
	if c.gpu != nil {
		t.Fatal("gpu should be nil after failure")
	}
	firstDrawCount := surf.drawCount

	// 第二帧：不应重新尝试 GPU（gpuDisabled=true）
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render frame 2: %v", err)
	}
	if surf.drawCount != firstDrawCount {
		t.Errorf("GPU drawCount should not increase (gpuDisabled), got %d -> %d", firstDrawCount, surf.drawCount)
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
	// Reverse: fg/bg 交换。原 fg=白(255), bg=黑(0) → 交换后 fg=黑, bg=白, HasBg=1
	if inst.FgR != 0 {
		t.Errorf("Reverse fg.R=%v want 0 (swapped from bg)", inst.FgR)
	}
	if inst.BgR != 255.0/255 {
		t.Errorf("Reverse bg.R=%v want 255/255 (swapped from fg)", inst.BgR)
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

func TestRenderGPUCursor(t *testing.T) {
	c, surf := newGPUCompositor()
	c.SetCursorColor(screen.Color{R: 255, G: 0, B: 0})
	buf := screen.NewBuffer(10, 2)
	cur := buf.Cursor()
	cur.Row = 0
	cur.Col = 0
	cur.Visible = true
	cur.Blinking = false
	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render: %v", err)
	}
	inst, ok := findInst(surf, 0, 0)
	if !ok {
		t.Fatal("no instance at cursor cell")
	}
	if inst.HasBg != 1 {
		t.Errorf("cursor cell HasBg=%v want 1", inst.HasBg)
	}
	if inst.BgR != 1.0 {
		t.Errorf("cursor bg.R=%v want 1.0 (cursor color red)", inst.BgR)
	}
	if inst.FgR != 255.0/255 {
		t.Errorf("cursor fg.R=%v want 255/255 (original fg unchanged)", inst.FgR)
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

// ============================================================
// L3: CPU dirty 路径光标移动保留字形正确性
// 验证 useDirty=true（DirectRender=false）路径下，光标移动时
// DamageCursor 仅标光标 cell dirty，非光标列 Clean cell 不被
// 整行 FillRect 误擦。回归 P0-1 缺陷。
// ============================================================

// dirtyTestSurface 与 testSurface 类似但 DirectRender=false，
// 触发 compositor CPU dirty 渲染路径（backBuf 在 compositor 内分配）。
type dirtyTestSurface struct {
	data   []byte
	stride int
	width  int
	height int
}

func (s *dirtyTestSurface) Size() (int, int)                          { return s.width, s.height }
func (s *dirtyTestSurface) Data() []byte                              { return s.data }
func (s *dirtyTestSurface) Stride() int                               { return s.stride }
func (s *dirtyTestSurface) Swap() error                               { return nil }
func (s *dirtyTestSurface) Close() error                              { return nil }
func (s *dirtyTestSurface) ResizeEvents() <-chan platform.ResizeEvent { return nil }
func (s *dirtyTestSurface) OutputID() uint32                          { return 0 }
func (s *dirtyTestSurface) DirectRender() bool                        { return false }
func (s *dirtyTestSurface) DecoMode() uint32                          { return 2 }

// isPixelBlack 判断 BGRA32 像素是否为纯黑（defBg）。
func isPixelBlack(data []byte, stride, x, y int) bool {
	off := y*stride + x*4
	if off+3 >= len(data) {
		return true
	}
	return data[off] == 0 && data[off+1] == 0 && data[off+2] == 0
}

// TestCompositorDirtyPathPreservesGlyphOnCursorMove 验证 dirty 路径下
// 光标移动后非光标列的字形像素保留（不被整行 FillRect 擦除）。
// testFace 字形：bitmap 8x16，center 4x8 alpha=255（y:4..12, x:2..6）。
// 字形绘制位置：gx=px+XOffset(0)=col*8, gy=py+Ascent(12)+YOffset(-4)=row*16+8。
// 有效像素范围：gx+2..gx+6, gy+4..gy+12。
// 对于 cell(row=0, col=1)='B'：px=8, py=0, gx=8, gy=8。
// 有效像素：x=10..14, y=12..19。但 y=16..19 会被 row 1 的 FillRect 覆盖
// （row 1 py=16, FillRect rows 16..31），所以检测 y=12..15 范围内的像素。
// 像素 (12, 12)：gx=4, gy=4，bitmap[36]=255，前景白+defBg黑 -> 白色。
func TestCompositorDirtyPathPreservesGlyphOnCursorMove(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   make([]byte, 80*80*4),
		stride: 80 * 4,
		width:  80,
		height: 80,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	cur := buf.Cursor()
	cur.Visible = true
	cur.Blinking = false

	buf.Cell(0, 0).Rune = 'A'
	buf.Cell(0, 1).Rune = 'B'
	buf.Cell(0, 2).Rune = 'C'

	// 首帧渲染：所有 cell 渲染后 SetClean
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}
	// 验证 'B' 字形已绘制（像素非黑），使用 y=12 避开 row 1 FillRect 覆盖区
	if isPixelBlack(surf.data, surf.stride, 12, 12) {
		t.Fatal("setup failed: 'B' glyph not rendered in first frame")
	}

	// 移动光标 (0,0) -> (1,0)，DamageCursor 旧+新位置
	buf.DamageCursor(0, 0)
	buf.DamageCursor(1, 0)
	cur.Row = 1
	cur.Col = 0

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 2: %v", err)
	}

	// 断言：'B' 字形像素保留（未被整行 FillRect 擦成 defBg）
	if isPixelBlack(surf.data, surf.stride, 12, 12) {
		t.Error("BUG: 'B' glyph erased after cursor move - dirty path FillRect cleared entire row but Clean cell was skipped")
	}
	// 同样验证 'C' 字形（col=2, px=16, gx=16, gy=8, 有效像素 x=18..22 y=12..15）
	if isPixelBlack(surf.data, surf.stride, 20, 12) {
		t.Error("BUG: 'C' glyph erased after cursor move")
	}
}

// TestCompositorDirtyPathRedrawsCursorCell 验证 dirty 路径下
// 旧光标 cell 被标 dirty 后会重新渲染（恢复字形，不残留光标像素）。
func TestCompositorDirtyPathRedrawsCursorCell(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   make([]byte, 80*80*4),
		stride: 80 * 4,
		width:  80,
		height: 80,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	cur := buf.Cursor()
	cur.Visible = false
	cur.Blinking = false

	buf.Cell(0, 0).Rune = 'A'

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}

	// 'A' 在 (0,0)：px=0, gx=0, gy=8, 有效像素 x=2..6 y=12..15（避开 row 1 覆盖）
	if isPixelBlack(surf.data, surf.stride, 4, 12) {
		t.Fatal("setup failed: 'A' glyph not rendered")
	}

	// 将 cell(0,0) 标 dirty，重新渲染
	buf.DamageCell(0, 0)
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 2: %v", err)
	}
	// 'A' 应重新绘制（dirty cell 总是清 bg + 重绘字形）
	if isPixelBlack(surf.data, surf.stride, 4, 12) {
		t.Error("dirty cell should be re-rendered with glyph")
	}
}

// TestCompositorDirtyPathCleanCellSkipped 验证 dirty 路径下
// Clean cell（非光标行）被跳过不重绘，保留上一帧像素。这是 P0-1 修复的核心：
// 不再整行 FillRect 清背景，而是逐 cell 清 bg，Clean cell 跳过。
func TestCompositorDirtyPathCleanCellSkipped(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   make([]byte, 80*80*4),
		stride: 80 * 4,
		width:  80,
		height: 80,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	cur := buf.Cursor()
	cur.Visible = false
	cur.Blinking = false
	// 光标放在 row 1，使 row 0 不是 cursorRow，Clean cell 会被跳过
	cur.Row = 1
	cur.Col = 0

	// 在 cell(0,0) 写 'A'
	buf.Cell(0, 0).Rune = 'A'

	// 首帧渲染：所有 cell SetClean
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 1: %v", err)
	}

	// 手动在 backBuf 的 cell(0,1) 位置写入一个红色像素（模拟上一帧残留）
	// cell(0,1): px=8, py=0. 在 (10, 12) 写红色像素（避开 row 1 FillRect 覆盖区 y=16..）。
	off := 12*surf.stride + 10*4
	c.backBuf[off] = 0     // B
	c.backBuf[off+1] = 0   // G
	c.backBuf[off+2] = 255 // R (red)
	c.backBuf[off+3] = 255

	// 标 cell(0,0) dirty（模拟光标移动 DamageCursor），cell(0,1) 保持 Clean
	buf.DamageCell(0, 0)

	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render 2: %v", err)
	}

	// cell(0,1) 是 Clean 且非光标行（cursorRow=1）-> 应被跳过，红色像素保留
	if c.backBuf[off+2] != 255 {
		t.Error("BUG: Clean cell pixel was overwritten - dirty path should skip Clean cells to preserve previous frame")
	}
}

// ============================================================
// P1-12: copyAllToSurface 切片越界保护
// 验证 surfData 小于 backBuf 时不 panic（同 stride 与不同 stride 两路径）
// ============================================================

func TestCopyAllToSurfaceSmallSurfaceSameStride(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   make([]byte, 100),
		stride: 80 * 4,
		width:  80,
		height: 80,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render same-stride small surface failed: %v", err)
	}
}

func TestCopyAllToSurfaceSmallSurfaceDiffStride(t *testing.T) {
	surf := &dirtyTestSurface{
		data:   make([]byte, 500),
		stride: 60 * 4,
		width:  80,
		height: 80,
	}
	face := &testFace{}
	c := NewCompositor(surf, face)

	buf := screen.NewBuffer(10, 5)
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render diff-stride small surface failed: %v", err)
	}
}

// ============================================================
// P1-13: metrics.Width==0 除零保护
// 验证 face 返回 Width=0/Height=0 时 compositor 设置默认值不 panic
// ============================================================

type zeroFace struct{}

func (f *zeroFace) Glyph(r rune) (*font.Glyph, error) {
	return &font.Glyph{Rune: r, Bitmap: make([]byte, 1), Width: 1, Height: 1}, nil
}
func (f *zeroFace) Metrics() font.Metrics {
	return font.Metrics{Width: 0, Height: 0, Ascent: 0, Descent: 0}
}
func (f *zeroFace) Close() error { return nil }

func TestCompositorZeroMetricsWidth(t *testing.T) {
	surf := &testSurface{
		data:   make([]byte, 800*600*4),
		stride: 800 * 4,
		width:  800,
		height: 600,
	}
	c := NewCompositor(surf, &zeroFace{})
	if c.metrics.Width != 8 {
		t.Errorf("metrics.Width=%d want 8 (guard should set default)", c.metrics.Width)
	}
	if c.metrics.Height != 16 {
		t.Errorf("metrics.Height=%d want 16 (guard should set default)", c.metrics.Height)
	}
	buf := screen.NewBuffer(10, 5)
	if err := c.Render(buf, 0); err != nil {
		t.Fatalf("Render with zero metrics failed: %v", err)
	}
}
