package screen

import "testing"

func TestNewLine(t *testing.T) {
	l := NewLine(10)
	if l.Width() != 10 {
		t.Errorf("expected width 10, got %d", l.Width())
	}
	for i := 0; i < l.Width(); i++ {
		c := l.Cell(i)
		if c == nil {
			t.Errorf("cell %d is nil", i)
			continue
		}
		if c.Rune != ' ' {
			t.Errorf("cell %d: expected ' ', got %q", i, c.Rune)
		}
	}
}

func TestLineCellOutOfBounds(t *testing.T) {
	l := NewLine(5)
	if l.Cell(-1) != nil {
		t.Error("expected nil for negative col")
	}
	if l.Cell(5) != nil {
		t.Error("expected nil for col >= width")
	}
}

func TestLineResizeGrow(t *testing.T) {
	l := NewLine(3)
	l.Cell(0).Rune = 'A'
	l.Cell(1).Rune = 'B'
	l.Cell(2).Rune = 'C'
	l.Resize(5)
	if l.Width() != 5 {
		t.Errorf("expected width 5, got %d", l.Width())
	}
	if l.Cell(0).Rune != 'A' || l.Cell(1).Rune != 'B' || l.Cell(2).Rune != 'C' {
		t.Error("existing cells should be preserved after grow")
	}
	if l.Cell(3).Rune != ' ' || l.Cell(4).Rune != ' ' {
		t.Error("new cells should be default")
	}
}

func TestLineResizeShrink(t *testing.T) {
	l := NewLine(5)
	l.Cell(0).Rune = 'A'
	l.Cell(4).Rune = 'Z'
	l.Resize(3)
	if l.Width() != 3 {
		t.Errorf("expected width 3, got %d", l.Width())
	}
	if l.Cell(0).Rune != 'A' {
		t.Error("first cell should be preserved")
	}
	if l.Cell(2).Rune != ' ' {
		t.Error("third cell should be default")
	}
}

func TestLineClear(t *testing.T) {
	l := NewLine(3)
	l.Cell(0).Rune = 'X'
	l.Cell(1).Rune = 'Y'
	l.Clear()
	for i := 0; i < l.Width(); i++ {
		if l.Cell(i).Rune != ' ' {
			t.Errorf("cell %d not cleared", i)
		}
	}
}

func TestLineClone(t *testing.T) {
	l := NewLine(3)
	l.Cell(0).Rune = 'A'
	l.Cell(1).Rune = 'B'
	c := l.Clone()
	if c.Width() != l.Width() {
		t.Error("clone width mismatch")
	}
	if c.Cell(0).Rune != 'A' || c.Cell(1).Rune != 'B' {
		t.Error("clone cell content mismatch")
	}
	c.Cell(0).Rune = 'Z'
	if l.Cell(0).Rune != 'A' {
		t.Error("clone should be deep copy")
	}
}
