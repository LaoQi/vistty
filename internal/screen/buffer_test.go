package screen

import "testing"

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(80, 24)
	if b.Cols() != 80 || b.Rows() != 24 {
		t.Errorf("expected 80x24, got %dx%d", b.Cols(), b.Rows())
	}
	if b.scrollTop != 0 || b.scrollBot != 23 {
		t.Errorf("expected scroll region [0,23], got [%d,%d]", b.scrollTop, b.scrollBot)
	}
}

func TestBufferCell(t *testing.T) {
	b := NewBuffer(10, 5)
	c := b.Cell(0, 0)
	if c == nil {
		t.Fatal("Cell(0,0) returned nil")
	}
	if c.Rune != ' ' {
		t.Error("expected default cell")
	}
	c.Rune = 'Z'
	if b.Cell(0, 0).Rune != 'Z' {
		t.Error("cell modification not reflected")
	}
	if b.Cell(-1, 0) != nil || b.Cell(5, 0) != nil || b.Cell(0, 10) != nil {
		t.Error("out-of-bounds Cell should return nil")
	}
}

func TestBufferLine(t *testing.T) {
	b := NewBuffer(10, 5)
	if b.Line(-1) != nil || b.Line(5) != nil {
		t.Error("out-of-bounds Line should return nil")
	}
	if b.Line(0) == nil {
		t.Error("Line(0) should not be nil")
	}
}

func TestBufferScrollUp(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		b.Cell(i, 0).Rune = rune('0' + i)
	}
	h := NewHistory(100)
	for i := 0; i < 3; i++ {
		h.Push(b.Line(i))
	}
	b.ScrollUp(3)
	for i := 0; i < 2; i++ {
		if b.Cell(i, 0).Rune != rune('3'+i) {
			t.Errorf("row %d: expected '%c', got '%c'", i, '3'+i, b.Cell(i, 0).Rune)
		}
	}
	for i := 2; i < 5; i++ {
		if b.Cell(i, 0).Rune != ' ' {
			t.Errorf("row %d: expected blank, got '%c'", i, b.Cell(i, 0).Rune)
		}
	}
	if h.Len() != 3 {
		t.Errorf("expected 3 history lines, got %d", h.Len())
	}
}

func TestBufferScrollDown(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		b.Cell(i, 0).Rune = rune('0' + i)
	}
	b.ScrollDown(2)
	if b.Cell(0, 0).Rune != ' ' || b.Cell(1, 0).Rune != ' ' {
		t.Error("top rows should be blank after ScrollDown")
	}
	for i := 2; i < 5; i++ {
		if b.Cell(i, 0).Rune != rune('0'+(i-2)) {
			t.Errorf("row %d: expected '%c', got '%c'", i, '0'+(i-2), b.Cell(i, 0).Rune)
		}
	}
}

func TestBufferScrollRegion(t *testing.T) {
	b := NewBuffer(10, 24)
	for i := 0; i < 24; i++ {
		b.Cell(i, 0).Rune = rune('a' + i%26)
	}
	b.SetScrollRegion(5, 20)
	b.ScrollUp(1)
	for i := 0; i < 5; i++ {
		if b.Cell(i, 0).Rune != rune('a'+i%26) {
			t.Errorf("row %d above scroll region should be unchanged", i)
		}
	}
	for i := 5; i < 20; i++ {
		if b.Cell(i, 0).Rune != rune('a'+(i+1)%26) {
			t.Errorf("row %d in scroll region: expected '%c', got '%c'", i, 'a'+(i+1)%26, b.Cell(i, 0).Rune)
		}
	}
	if b.Cell(20, 0).Rune != ' ' {
		t.Error("row 20 (scrollBot) should be blank after ScrollUp")
	}
	for i := 21; i < 24; i++ {
		if b.Cell(i, 0).Rune != rune('a'+i%26) {
			t.Errorf("row %d below scroll region should be unchanged", i)
		}
	}
}

func TestBufferSetScrollRegionInvalid(t *testing.T) {
	b := NewBuffer(10, 24)
	b.SetScrollRegion(10, 5)
	if b.scrollTop != 0 || b.scrollBot != 23 {
		t.Error("invalid scroll region should not change settings")
	}
}

func TestBufferResizeShrink(t *testing.T) {
	b := NewBuffer(80, 24)
	for i := 0; i < 24; i++ {
		b.Cell(i, 0).Rune = rune('A' + i%26)
	}
	b.Resize(40, 12)
	if b.Cols() != 40 || b.Rows() != 12 {
		t.Errorf("expected 40x12, got %dx%d", b.Cols(), b.Rows())
	}
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("existing content should be preserved after shrink")
	}
	if b.scrollTop != 0 || b.scrollBot != 11 {
		t.Error("scroll region should be reset after resize")
	}
}

func TestBufferResizeGrow(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'X'
	b.Resize(20, 10)
	if b.Cols() != 20 || b.Rows() != 10 {
		t.Errorf("expected 20x10, got %dx%d", b.Cols(), b.Rows())
	}
	if b.Cell(0, 0).Rune != 'X' {
		t.Error("existing content should be preserved after grow")
	}
	if b.Cell(5, 0).Rune != ' ' {
		t.Error("new rows should be default")
	}
}

func TestBufferClear(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'Z'
	b.Clear()
	if b.Cell(0, 0).Rune != ' ' {
		t.Error("Clear should reset all cells")
	}
}

func TestBufferDirtyRegions(t *testing.T) {
	b := NewBuffer(10, 5)
	b.ClearDirty()
	b.Cell(1, 2).MarkDirty()
	b.Cell(1, 3).MarkDirty()
	b.Cell(1, 4).MarkDirty()
	b.Cell(3, 0).MarkDirty()
	regions := b.DirtyRegions()
	if len(regions) < 2 {
		t.Fatalf("expected at least 2 dirty regions, got %d", len(regions))
	}
	found1 := false
	found3 := false
	for _, r := range regions {
		if r.Y == 1 && r.X == 2 && r.W == 3 {
			found1 = true
		}
		if r.Y == 3 && r.X == 0 && r.W == 1 {
			found3 = true
		}
	}
	if !found1 {
		t.Error("expected dirty region at row 1, cols 2-4")
	}
	if !found3 {
		t.Error("expected dirty region at row 3, col 0")
	}
}

func TestBufferClearDirty(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).MarkDirty()
	b.ClearDirty()
	if b.Cell(0, 0).IsDirty() {
		t.Error("cell should not be dirty after ClearDirty")
	}
	regions := b.DirtyRegions()
	if len(regions) != 0 {
		t.Errorf("expected 0 dirty regions, got %d", len(regions))
	}
}

func TestBufferClearRect(t *testing.T) {
	b := NewBuffer(10, 5)
	for i := 0; i < 5; i++ {
		for j := 0; j < 10; j++ {
			b.Cell(i, j).Rune = 'X'
		}
	}
	b.ClearRect(Rect{X: 2, Y: 1, W: 3, H: 2})
	for y := 1; y < 3; y++ {
		for x := 2; x < 5; x++ {
			if b.Cell(y, x).Rune != ' ' {
				t.Errorf("cell (%d,%d) should be cleared", y, x)
			}
		}
	}
	if b.Cell(0, 0).Rune != 'X' {
		t.Error("cell outside rect should be unchanged")
	}
}

func TestBufferScrollUpZero(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'A'
	b.ScrollUp(0)
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("ScrollUp(0) should not change buffer")
	}
}

func TestBufferScrollDownZero(t *testing.T) {
	b := NewBuffer(10, 5)
	b.Cell(0, 0).Rune = 'A'
	b.ScrollDown(0)
	if b.Cell(0, 0).Rune != 'A' {
		t.Error("ScrollDown(0) should not change buffer")
	}
}
