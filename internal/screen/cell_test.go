package screen

import "testing"

func TestNewCell(t *testing.T) {
	c := NewCell()
	if c.Rune != ' ' {
		t.Errorf("expected Rune ' ', got %q", c.Rune)
	}
	if c.Width != 1 {
		t.Errorf("expected Width 1, got %d", c.Width)
	}
	if !c.Fg.IsDefault {
		t.Error("expected Fg.IsDefault true")
	}
	if !c.Bg.IsDefault {
		t.Error("expected Bg.IsDefault true")
	}
	if c.Attr != 0 {
		t.Errorf("expected Attr 0, got %d", c.Attr)
	}
	if c.Dirty {
		t.Error("expected Dirty false")
	}
}

func TestCellClear(t *testing.T) {
	c := Cell{Rune: 'A', Width: 2, Fg: Color{R: 255, IsDefault: false}, Attr: AttrBold}
	c.Clear()
	if c.Rune != ' ' || c.Width != 1 || !c.Fg.IsDefault || c.Attr != 0 {
		t.Error("Clear did not reset cell to defaults")
	}
}

func TestCellDirty(t *testing.T) {
	c := NewCell()
	if c.IsDirty() {
		t.Error("new cell should not be dirty")
	}
	c.MarkDirty()
	if !c.IsDirty() {
		t.Error("cell should be dirty after MarkDirty")
	}
	c.ClearDirty()
	if c.IsDirty() {
		t.Error("cell should not be dirty after ClearDirty")
	}
}

func TestCellEqual(t *testing.T) {
	a := Cell{Rune: 'X', Width: 1, Fg: Color{R: 10, G: 20, B: 30}, Bg: Color{IsDefault: true}, Attr: AttrBold}
	b := Cell{Rune: 'X', Width: 1, Fg: Color{R: 10, G: 20, B: 30}, Bg: Color{IsDefault: true}, Attr: AttrBold}
	if !a.Equal(b) {
		t.Error("identical cells should be equal")
	}

	b.Rune = 'Y'
	if a.Equal(b) {
		t.Error("cells with different Rune should not be equal")
	}

	b.Rune = 'X'
	b.Attr = AttrItalic
	if a.Equal(b) {
		t.Error("cells with different Attr should not be equal")
	}
}

func TestColorEqual(t *testing.T) {
	a := Color{R: 1, G: 2, B: 3, IsDefault: false}
	b := Color{R: 1, G: 2, B: 3, IsDefault: false}
	if a != b {
		t.Error("identical colors should be equal")
	}
	b.IsDefault = true
	if a == b {
		t.Error("colors with different IsDefault should not be equal")
	}
}
