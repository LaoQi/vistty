package font

import (
	"reflect"
	"testing"
)

func TestShearGlyph_NilAndZero(t *testing.T) {
	if got := ShearGlyph(nil, 0.25, 0.5); got != nil {
		t.Errorf("nil input should return nil, got %+v", got)
	}
	if got := ShearGlyph(&Glyph{Width: 0, Height: 4}, 0.25, 0.5); got != nil {
		t.Errorf("Width=0 should return nil, got %+v", got)
	}
	if got := ShearGlyph(&Glyph{Width: 4, Height: 0}, 0.25, 0.5); got != nil {
		t.Errorf("Height=0 should return nil, got %+v", got)
	}

	g := &Glyph{
		Rune:    'A',
		Bitmap:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Width:   4,
		Height:  2,
		XOffset: 1,
		YOffset: 2,
		Advance: 9,
	}
	out := ShearGlyph(g, 0, 0.5)
	if out == nil {
		t.Fatal("slope=0 should return non-nil copy")
	}
	if out == g {
		t.Error("slope=0 should return a different instance, not the same pointer")
	}
	if len(out.Bitmap) == 0 || &out.Bitmap[0] == &g.Bitmap[0] {
		t.Error("slope=0 should copy the bitmap into a new backing array")
	}
	if !reflect.DeepEqual(out, g) {
		t.Errorf("slope=0 copy content mismatch:\n got %+v\nwant %+v", out, g)
	}
}

func TestShearGlyph_Direction(t *testing.T) {
	src := []byte{
		255, 255, 255, 255,
		0, 0, 0, 0,
		0, 0, 0, 0,
		255, 255, 255, 255,
	}
	g := &Glyph{Bitmap: src, Width: 4, Height: 4}

	out := ShearGlyph(g, 1.0, 0)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out.Width != 7 {
		t.Fatalf("Width=%d want 7 (4 + ceil(1.0*3))", out.Width)
	}
	if out.Height != 4 {
		t.Fatalf("Height=%d want 4", out.Height)
	}

	bottom := out.Height - 1
	for ox := 0; ox < 4; ox++ {
		if v := out.Bitmap[bottom*out.Width+ox]; v != 255 {
			t.Errorf("bottom row ox=%d: got %d want 255 (bottom must stay put)", ox, v)
		}
	}
	for ox := 4; ox < out.Width; ox++ {
		if v := out.Bitmap[bottom*out.Width+ox]; v != 0 {
			t.Errorf("bottom row ox=%d: got %d want 0", ox, v)
		}
	}

	top := 0
	for ox := 0; ox < 3; ox++ {
		if v := out.Bitmap[top*out.Width+ox]; v != 0 {
			t.Errorf("top row ox=%d: got %d want 0 (top shifted right by maxShift)", ox, v)
		}
	}
	for ox := 3; ox < out.Width; ox++ {
		if v := out.Bitmap[top*out.Width+ox]; v != 255 {
			t.Errorf("top row ox=%d: got %d want 255 (top shifted right)", ox, v)
		}
	}
}

func TestShearGlyph_WidthAndSize(t *testing.T) {
	g := &Glyph{
		Rune:    'Q',
		Bitmap:  make([]byte, 8*16),
		Width:   8,
		Height:  16,
		XOffset: 1,
		YOffset: 2,
		Advance: 10,
	}
	out := ShearGlyph(g, 0.25, 0)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	wantW := 8 + 4
	if out.Width != wantW {
		t.Errorf("Width=%d want %d (8 + ceil(0.25*15)=8+4)", out.Width, wantW)
	}
	if out.Height != 16 {
		t.Errorf("Height=%d want 16", out.Height)
	}
	if out.XOffset != 1 || out.YOffset != 2 || out.Advance != 10 || out.Rune != 'Q' {
		t.Errorf("metadata changed: got %+v", out)
	}
	if len(out.Bitmap) != out.Width*out.Height {
		t.Errorf("bitmap len=%d want %d", len(out.Bitmap), out.Width*out.Height)
	}
}

func TestShearGlyph_Antialiasing(t *testing.T) {
	src := []byte{
		255, 0,
		0, 0,
	}
	g := &Glyph{Bitmap: src, Width: 2, Height: 2}
	out := ShearGlyph(g, 0.5, 0)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out.Width != 3 {
		t.Fatalf("Width=%d want 3 (2 + ceil(0.5*1)=2+1)", out.Width)
	}
	mid := out.Bitmap[0*out.Width+1]
	if mid < 120 || mid > 135 {
		t.Errorf("antialiased middle pixel=%d want in [120,135] (255*0.5=127.5)", mid)
	}
	if v := out.Bitmap[0*out.Width+2]; v != 0 {
		t.Errorf("top ox=2: got %d want 0", v)
	}
}

func TestShearGlyph_DoesNotMutateOriginal(t *testing.T) {
	orig := []byte{
		255, 0, 255, 0,
		0, 255, 0, 255,
		255, 255, 0, 0,
		0, 0, 255, 255,
	}
	g := &Glyph{Bitmap: orig, Width: 4, Height: 4}
	snapshot := append([]byte(nil), orig...)

	_ = ShearGlyph(g, 0.25, 0)
	if !reflect.DeepEqual(g.Bitmap, snapshot) {
		t.Errorf("original bitmap mutated:\n got %v\nwant %v", g.Bitmap, snapshot)
	}
}

func TestShearGlyph_RealFont(t *testing.T) {
	face, err := NewEmbeddedFace(14, 72)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	defer face.Close()

	g, err := face.Glyph('A')
	if err != nil {
		t.Fatalf("Glyph('A'): %v", err)
	}
	if g == nil {
		t.Skip("embedded font has no glyph for 'A'")
	}

	out := ShearGlyph(g, 0.25, 0)
	if out == nil {
		t.Fatal("ShearGlyph returned nil for real glyph")
	}
	if out.Height != g.Height {
		t.Errorf("Height changed: got %d want %d", out.Height, g.Height)
	}
	if g.Height > 1 && out.Width <= g.Width {
		t.Errorf("Width did not increase: got %d from %d", out.Width, g.Width)
	}
	if len(out.Bitmap) != out.Width*out.Height {
		t.Errorf("bitmap len=%d want %d", len(out.Bitmap), out.Width*out.Height)
	}
}

func TestShearGlyph_AlignCenter(t *testing.T) {
	// slope=1.0, H=4 → maxShift=3。align=0.5 → XOffset 应减少 round(0.5*3)=2
	g := &Glyph{
		Bitmap:  []byte{255, 255, 255, 255, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 255},
		Width:   4,
		Height:  4,
		XOffset: 5,
		YOffset: 0,
		Advance: 8,
	}
	out := ShearGlyph(g, 1.0, 0.5)
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	if out.XOffset != 3 {
		t.Errorf("XOffset=%d want 3 (5 - round(0.5*3)=5-2)", out.XOffset)
	}
	// align=0 底部对齐：XOffset 不变
	out0 := ShearGlyph(g, 1.0, 0)
	if out0.XOffset != 5 {
		t.Errorf("align=0 XOffset=%d want 5 (unchanged)", out0.XOffset)
	}
	// align=1 顶部对齐：XOffset 减少 round(1.0*3)=3
	out1 := ShearGlyph(g, 1.0, 1.0)
	if out1.XOffset != 2 {
		t.Errorf("align=1 XOffset=%d want 2 (5 - round(1.0*3)=5-3)", out1.XOffset)
	}
}
