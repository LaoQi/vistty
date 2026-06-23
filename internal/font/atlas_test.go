package font

import (
	"testing"
)

func TestAtlasPutAndGet(t *testing.T) {
	a := NewAtlas(10)
	glyph := &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16}
	a.Put('A', glyph)

	g := a.Get('A')
	if g == nil {
		t.Fatal("expected to find 'A' in atlas")
	}
	if g.Width != 8 || g.Height != 16 {
		t.Errorf("expected 8x16, got %dx%d", g.Width, g.Height)
	}
	if len(g.Bitmap) != 64 {
		t.Errorf("expected glyph length 64, got %d", len(g.Bitmap))
	}
}

func TestAtlasGetMissing(t *testing.T) {
	a := NewAtlas(10)
	g := a.Get('Z')
	if g != nil {
		t.Error("expected nil for missing glyph")
	}
}

func TestAtlasLRUEviction(t *testing.T) {
	a := NewAtlas(3)
	a.Put('A', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('B', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('C', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('D', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})

	g := a.Get('A')
	if g != nil {
		t.Error("expected 'A' to be evicted")
	}
	g = a.Get('D')
	if g == nil {
		t.Error("expected 'D' to be present")
	}
}

func TestAtlasLRUReorder(t *testing.T) {
	a := NewAtlas(3)
	a.Put('A', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('B', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('C', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})

	a.Get('A')
	a.Put('D', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})

	g := a.Get('A')
	if g == nil {
		t.Error("expected 'A' to be present after LRU reorder")
	}
	g = a.Get('B')
	if g != nil {
		t.Error("expected 'B' to be evicted")
	}
}

func TestAtlasUpdate(t *testing.T) {
	a := NewAtlas(3)
	a.Put('A', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('A', &Glyph{Bitmap: make([]byte, 128), Width: 10, Height: 20})

	g := a.Get('A')
	if g == nil {
		t.Fatal("expected 'A' to be present after update")
	}
	if g.Width != 10 || g.Height != 20 {
		t.Errorf("expected 10x20, got %dx%d", g.Width, g.Height)
	}
	if len(g.Bitmap) != 128 {
		t.Errorf("expected bitmap length 128, got %d", len(g.Bitmap))
	}
}

func TestAtlasClear(t *testing.T) {
	a := NewAtlas(10)
	a.Put('A', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Put('B', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	a.Clear()

	if g := a.Get('A'); g != nil {
		t.Error("expected 'A' to be cleared")
	}
	if g := a.Get('B'); g != nil {
		t.Error("expected 'B' to be cleared")
	}
}

func TestAtlasZeroCapacity(t *testing.T) {
	a := NewAtlas(0)
	a.Put('A', &Glyph{Bitmap: make([]byte, 64), Width: 8, Height: 16})
	if g := a.Get('A'); g == nil {
		t.Error("expected 'A' to be present with zero capacity (defaults to 4096)")
	}
}
