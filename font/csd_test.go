package font

import "testing"

func TestSynthCsdGlyphMetrics(t *testing.T) {
	m := Metrics{Width: 10, Height: 24, Ascent: 19, Descent: 5}
	for _, r := range []rune{CsdBtnMinRune, CsdBtnMaxRune, CsdBtnCloseRune} {
		g := synthCsdGlyph(r, m)
		if g == nil {
			t.Fatalf("synthCsdGlyph(%U): nil", r)
		}
		// 符号外接正方形 = 栏高的 1/2
		if g.Width != 12 || g.Height != 12 {
			t.Errorf("%U: want 12x12 got %dx%d", r, g.Width, g.Height)
		}
		// XOffset=0（水平由调用方居中）
		if g.XOffset != 0 {
			t.Errorf("%U: XOffset want 0 got %d", r, g.XOffset)
		}
		// 垂直居中：YOffset = -Ascent + (Height-sym)/2
		if want := -19 + (24-12)/2; g.YOffset != want {
			t.Errorf("%U: YOffset want %d got %d", r, want, g.YOffset)
		}
		if g.IsColor {
			t.Errorf("%U: must not be color", r)
		}
	}
}

func TestSynthCsdGlyphShapes(t *testing.T) {
	m := Metrics{Width: 10, Height: 24, Ascent: 19}
	at := func(g *Glyph, x, y int) byte { return g.Bitmap[y*g.Width+x] }

	// 最小化：只有中部的水平横线（垂直居中），上下边界留空
	min := synthCsdGlyph(CsdBtnMinRune, m)
	mid := min.Height / 2
	if at(min, 0, mid) != 255 {
		t.Error("min: middle row should be filled")
	}
	if at(min, 0, 0) != 0 || at(min, 0, min.Height-1) != 0 {
		t.Error("min: top/bottom rows must be empty")
	}

	// 最大化：矩形边框，内部空心
	max := synthCsdGlyph(CsdBtnMaxRune, m)
	if at(max, 0, 0) != 255 || at(max, max.Width-1, max.Height-1) != 255 {
		t.Error("max: border must be filled")
	}
	cx, cy := max.Width/2, max.Height/2
	if at(max, cx, cy) != 0 {
		t.Error("max: interior must be hollow")
	}

	// 关闭：两条对角线交叉
	close := synthCsdGlyph(CsdBtnCloseRune, m)
	if at(close, 0, 0) != 255 || at(close, close.Width-1, 0) != 255 {
		t.Error("close: corners must be filled")
	}
	if at(close, close.Width/2, close.Height/2) != 255 {
		t.Error("close: center crossing must be filled")
	}
}

func TestSynthCsdGlyphSmall(t *testing.T) {
	// 小字号回归：lw=1 时最大化边框曾因 half=0 完全空白
	m := Metrics{Width: 10, Height: 16, Ascent: 12}
	at := func(g *Glyph, x, y int) byte { return g.Bitmap[y*g.Width+x] }

	max := synthCsdGlyph(CsdBtnMaxRune, m)
	if max == nil {
		t.Fatal("synthCsdGlyph(max): nil")
	}
	if max.Width != 8 || max.Height != 8 {
		t.Fatalf("small max: want 8x8 got %dx%d", max.Width, max.Height)
	}
	// 外圈 1px 边框必须可见
	for _, p := range [][2]int{{0, 0}, {7, 0}, {0, 7}, {7, 7}} {
		if at(max, p[0], p[1]) != 255 {
			t.Errorf("small max: border pixel (%d,%d) missing", p[0], p[1])
		}
	}
	// 内部空心
	if at(max, 3, 3) != 0 {
		t.Error("small max: interior must be hollow")
	}

	// 最小化仍有一条居中横线，关闭仍有对角线
	min := synthCsdGlyph(CsdBtnMinRune, m)
	if at(min, 0, 4) != 255 || at(min, 7, 4) != 255 {
		t.Error("small min: center line missing")
	}
	cl := synthCsdGlyph(CsdBtnCloseRune, m)
	if at(cl, 0, 0) != 255 || at(cl, 7, 7) != 255 {
		t.Error("small close: diagonals missing")
	}
}

func TestOpenTypeFaceGlyphCsd(t *testing.T) {
	f, err := NewEmbeddedFace(14, 96)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	defer f.Close()
	for _, r := range []rune{CsdBtnMinRune, CsdBtnMaxRune, CsdBtnCloseRune} {
		g, err := f.Glyph(r)
		if err != nil {
			t.Fatalf("Glyph(%U): %v", r, err)
		}
		if g == nil {
			t.Fatalf("Glyph(%U): nil — CSD rune must be synthesized", r)
		}
		if g.IsColor {
			t.Errorf("Glyph(%U): unexpected color", r)
		}
	}
}
