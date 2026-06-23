package screen

import "testing"

func TestNewCursor(t *testing.T) {
	c := NewCursor()
	if c.Row != 0 || c.Col != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", c.Row, c.Col)
	}
	if c.Style != CursorBlock {
		t.Errorf("expected CursorBlock, got %d", c.Style)
	}
	if !c.Visible {
		t.Error("expected Visible true")
	}
	if !c.Blinking {
		t.Error("expected Blinking true")
	}
}

func TestCursorMove(t *testing.T) {
	c := NewCursor()
	c.Move(5, 10)
	if c.Row != 5 || c.Col != 10 {
		t.Errorf("expected (5,10), got (%d,%d)", c.Row, c.Col)
	}
}

func TestCursorClamp(t *testing.T) {
	c := NewCursor()
	c.Move(100, 200)
	c.Clamp(23, 79)
	if c.Row != 23 || c.Col != 79 {
		t.Errorf("expected (23,79), got (%d,%d)", c.Row, c.Col)
	}
	c.Move(-5, -10)
	c.Clamp(23, 79)
	if c.Row != 0 || c.Col != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", c.Row, c.Col)
	}
}

func TestCursorClampExact(t *testing.T) {
	c := NewCursor()
	c.Move(10, 40)
	c.Clamp(23, 79)
	if c.Row != 10 || c.Col != 40 {
		t.Error("in-range position should not change")
	}
}

func TestCursorHideShow(t *testing.T) {
	c := NewCursor()
	c.Hide()
	if c.Visible {
		t.Error("cursor should be hidden")
	}
	c.Show()
	if !c.Visible {
		t.Error("cursor should be visible")
	}
}

func TestCursorSetStyle(t *testing.T) {
	c := NewCursor()
	c.SetStyle(CursorUnderline)
	if c.Style != CursorUnderline {
		t.Errorf("expected CursorUnderline, got %d", c.Style)
	}
	c.SetStyle(CursorBar)
	if c.Style != CursorBar {
		t.Errorf("expected CursorBar, got %d", c.Style)
	}
}
