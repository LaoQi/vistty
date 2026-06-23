package screen

import "testing"

func TestNewHistory(t *testing.T) {
	h := NewHistory(100)
	if h.Len() != 0 {
		t.Errorf("expected 0, got %d", h.Len())
	}
	if h.maxLines != 100 {
		t.Errorf("expected maxLines 100, got %d", h.maxLines)
	}
}

func TestHistoryPush(t *testing.T) {
	h := NewHistory(3)
	l := NewLine(5)
	l.Cell(0).Rune = 'A'
	h.Push(l)
	if h.Len() != 1 {
		t.Errorf("expected 1, got %d", h.Len())
	}
	if h.Line(0).Cell(0).Rune != 'A' {
		t.Error("first history line content mismatch")
	}
}

func TestHistoryPushOverflow(t *testing.T) {
	h := NewHistory(2)
	for i := 0; i < 5; i++ {
		l := NewLine(5)
		l.Cell(0).Rune = rune('0' + i)
		h.Push(l)
	}
	if h.Len() != 2 {
		t.Errorf("expected 2, got %d", h.Len())
	}
	if h.Line(0).Cell(0).Rune != '3' {
		t.Errorf("oldest should be '3', got '%c'", h.Line(0).Cell(0).Rune)
	}
	if h.Line(1).Cell(0).Rune != '4' {
		t.Errorf("newest should be '4', got '%c'", h.Line(1).Cell(0).Rune)
	}
}

func TestHistoryPushClone(t *testing.T) {
	h := NewHistory(10)
	l := NewLine(5)
	l.Cell(0).Rune = 'X'
	h.Push(l)
	l.Cell(0).Rune = 'Y'
	if h.Line(0).Cell(0).Rune != 'X' {
		t.Error("Push should clone the line")
	}
}

func TestHistoryLineOutOfBounds(t *testing.T) {
	h := NewHistory(10)
	if h.Line(-1) != nil || h.Line(0) != nil {
		t.Error("out-of-bounds Line should return nil")
	}
}

func TestHistoryClear(t *testing.T) {
	h := NewHistory(10)
	l := NewLine(5)
	h.Push(l)
	h.Clear()
	if h.Len() != 0 {
		t.Errorf("expected 0 after Clear, got %d", h.Len())
	}
}
