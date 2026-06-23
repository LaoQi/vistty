package screen

import "testing"

func TestNewSelection(t *testing.T) {
	s := NewSelection()
	if s.Active {
		t.Error("new selection should not be active")
	}
	if !s.IsEmpty() {
		t.Error("new selection should be empty")
	}
}

func TestSelectionSetStartEnd(t *testing.T) {
	s := NewSelection()
	s.SetStart(1, 2)
	s.SetEnd(3, 4)
	if s.StartRow != 1 || s.StartCol != 2 {
		t.Errorf("start mismatch: (%d,%d)", s.StartRow, s.StartCol)
	}
	if s.EndRow != 3 || s.EndCol != 4 {
		t.Errorf("end mismatch: (%d,%d)", s.EndRow, s.EndCol)
	}
}

func TestSelectionContains(t *testing.T) {
	s := NewSelection()
	s.SetStart(1, 2)
	s.SetEnd(3, 4)
	s.Active = true

	if !s.Contains(1, 2) {
		t.Error("should contain start point")
	}
	if !s.Contains(3, 4) {
		t.Error("should contain end point")
	}
	if !s.Contains(2, 0) {
		t.Error("should contain middle row")
	}
	if s.Contains(0, 5) {
		t.Error("should not contain point before start")
	}
	if s.Contains(4, 0) {
		t.Error("should not contain point after end")
	}
	if s.Contains(1, 1) {
		t.Error("should not contain point before start col on start row")
	}
	if s.Contains(3, 5) {
		t.Error("should not contain point after end col on end row")
	}
}

func TestSelectionContainsInactive(t *testing.T) {
	s := NewSelection()
	s.SetStart(1, 2)
	s.SetEnd(3, 4)
	if s.Contains(2, 0) {
		t.Error("inactive selection should not contain any point")
	}
}

func TestSelectionContainsReversed(t *testing.T) {
	s := NewSelection()
	s.SetStart(3, 4)
	s.SetEnd(1, 2)
	s.Active = true
	if !s.Contains(2, 0) {
		t.Error("reversed selection should still contain middle row")
	}
	if s.Contains(1, 1) {
		t.Error("reversed: should not contain point before normalized start col")
	}
}

func TestSelectionClear(t *testing.T) {
	s := NewSelection()
	s.SetStart(1, 2)
	s.SetEnd(3, 4)
	s.Active = true
	s.Clear()
	if s.Active {
		t.Error("cleared selection should not be active")
	}
	if !s.IsEmpty() {
		t.Error("cleared selection should be empty")
	}
}

func TestSelectionIsEmpty(t *testing.T) {
	s := NewSelection()
	if !s.IsEmpty() {
		t.Error("zero selection should be empty")
	}
	s.SetStart(1, 1)
	s.SetEnd(1, 1)
	if !s.IsEmpty() {
		t.Error("same start/end should be empty")
	}
	s.SetEnd(1, 2)
	if s.IsEmpty() {
		t.Error("different start/end should not be empty")
	}
}

func TestSelectionNormalized(t *testing.T) {
	s := NewSelection()
	s.SetStart(3, 4)
	s.SetEnd(1, 2)
	sr, sc, er, ec := s.Normalized()
	if sr != 1 || sc != 2 || er != 3 || ec != 4 {
		t.Errorf("expected (1,2,3,4), got (%d,%d,%d,%d)", sr, sc, er, ec)
	}
}

func TestSelectionNormalizedAlready(t *testing.T) {
	s := NewSelection()
	s.SetStart(1, 2)
	s.SetEnd(3, 4)
	sr, sc, er, ec := s.Normalized()
	if sr != 1 || sc != 2 || er != 3 || ec != 4 {
		t.Errorf("expected (1,2,3,4), got (%d,%d,%d,%d)", sr, sc, er, ec)
	}
}

func TestSelectionNormalizedSameRow(t *testing.T) {
	s := NewSelection()
	s.SetStart(2, 5)
	s.SetEnd(2, 3)
	sr, sc, er, ec := s.Normalized()
	if sr != 2 || sc != 3 || er != 2 || ec != 5 {
		t.Errorf("expected (2,3,2,5), got (%d,%d,%d,%d)", sr, sc, er, ec)
	}
}
