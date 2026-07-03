package font

import "testing"

func TestEmojiFaceGlyph(t *testing.T) {
	cellW, cellH, ascent := 8, 16, 13
	face, err := NewEmojiFace(cellW, cellH, ascent)
	if err != nil {
		t.Fatalf("NewEmojiFace: %v", err)
	}
	g, err := face.Glyph(0x1F600)
	if err != nil {
		t.Fatalf("Glyph U+1F600: %v", err)
	}
	if g == nil {
		t.Fatal("Glyph U+1F600 returned nil")
	}
	if !g.IsColor {
		t.Error("Glyph.IsColor = false, want true")
	}
	expectedW := cellW * 2
	expectedH := ascent
	if g.Width != expectedW {
		t.Errorf("Glyph.Width = %d, want %d", g.Width, expectedW)
	}
	if g.Height != expectedH {
		t.Errorf("Glyph.Height = %d, want %d", g.Height, expectedH)
	}
	if g.YOffset != -ascent {
		t.Errorf("Glyph.YOffset = %d, want %d", g.YOffset, -ascent)
	}
	expectedLen := expectedW * expectedH * 4
	if len(g.Bitmap) != expectedLen {
		t.Errorf("len(Bitmap) = %d, want %d", len(g.Bitmap), expectedLen)
	}
	hasContent := false
	for _, b := range g.Bitmap {
		if b != 0 {
			hasContent = true
			break
		}
	}
	if !hasContent {
		t.Error("Bitmap is all zeros")
	}
}

func TestEmojiFaceMultipleRunes(t *testing.T) {
	face, _ := NewEmojiFace(8, 16, 13)
	runes := []rune{0x1F600, 0x1F389, 0x2764, 0x1F680}
	for _, r := range runes {
		g, err := face.Glyph(r)
		if err != nil {
			t.Errorf("Glyph U+%X: %v", r, err)
			continue
		}
		if g == nil {
			t.Errorf("Glyph U+%X returned nil", r)
			continue
		}
		if !g.IsColor {
			t.Errorf("Glyph U+%X IsColor = false, want true", r)
		}
	}
}

func TestEmojiFaceCacheHit(t *testing.T) {
	face, _ := NewEmojiFace(8, 16, 13)
	g1, _ := face.Glyph(0x1F600)
	if g1 == nil {
		t.Fatal("first Glyph returned nil")
	}
	g2, _ := face.Glyph(0x1F600)
	if g2 != g1 {
		t.Error("cache miss: second Glyph returned different pointer")
	}
}

func TestEmojiFaceResize(t *testing.T) {
	face, _ := NewEmojiFace(8, 16, 13)
	face.Glyph(0x1F600)
	if len(face.cache) != 1 {
		t.Fatalf("cache size = %d, want 1", len(face.cache))
	}
	face.Resize(10, 20, 17)
	if len(face.cache) != 0 {
		t.Errorf("cache size after resize = %d, want 0", len(face.cache))
	}
	if face.cellW != 10 || face.cellH != 20 || face.ascent != 17 {
		t.Errorf("cell dims = %dx%d ascent=%d, want 10x20x17", face.cellW, face.cellH, face.ascent)
	}
	g, _ := face.Glyph(0x1F600)
	if g == nil {
		t.Fatal("Glyph after resize returned nil")
	}
	if g.Width != 20 {
		t.Errorf("Glyph.Width after resize = %d, want 20", g.Width)
	}
	if g.Height != 17 {
		t.Errorf("Glyph.Height after resize = %d, want 17", g.Height)
	}
	if g.YOffset != -17 {
		t.Errorf("Glyph.YOffset after resize = %d, want -17", g.YOffset)
	}
}

func TestEmojiFaceMissingRune(t *testing.T) {
	face, _ := NewEmojiFace(8, 16, 13)
	g, err := face.Glyph('A')
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if g != nil {
		t.Error("expected nil for non-emoji rune, got non-nil")
	}
}
