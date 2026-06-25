package terminal

import "testing"

func TestEraseCharsECH(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("ABCDE"))
	term.cursor.Col = 1
	term.FeedBytes([]byte("\x1b[2X"))
	cell := term.screen.Cell(0, 0)
	if cell.Rune != 'A' {
		t.Errorf("expected 'A' at col 0, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 1)
	if cell.Rune != ' ' {
		t.Errorf("expected ' ' at col 1, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 2)
	if cell.Rune != ' ' {
		t.Errorf("expected ' ' at col 2, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 3)
	if cell.Rune != 'D' {
		t.Errorf("expected 'D' at col 3, got %c", cell.Rune)
	}
	cell = term.screen.Cell(0, 4)
	if cell.Rune != 'E' {
		t.Errorf("expected 'E' at col 4, got %c", cell.Rune)
	}
	if term.cursor.Col != 1 {
		t.Errorf("cursor should not move, expected col 1, got %d", term.cursor.Col)
	}
}

func TestEraseDisplayScrollback(t *testing.T) {
	term, _ := newTerminalForTest(80, 3)
	term.FeedBytes([]byte("line1\nline2\nline3\nline4"))
	if term.screen.History().Len() == 0 {
		t.Fatal("expected history to have content after scroll")
	}
	term.FeedBytes([]byte("\x1b[3J"))
	if term.screen.History().Len() != 0 {
		t.Errorf("expected history cleared, got len %d", term.screen.History().Len())
	}
}

func TestTabClearAll(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 8 {
		t.Fatalf("expected col 8 after tab, got %d", term.cursor.Col)
	}
	term.FeedBytes([]byte("\x1b[3g"))
	term.cursor.Col = 0
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 79 {
		t.Errorf("expected col 79 (no tab stops), got %d", term.cursor.Col)
	}
}

func TestTabClearCurrent(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[g"))
	term.cursor.Col = 0
	term.FeedBytes([]byte("\t"))
	if term.cursor.Col != 8 {
		t.Errorf("expected col 8 (col 0 cleared but 8 remains), got %d", term.cursor.Col)
	}
}

func TestCursorClamp(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 5
	term.cursor.Col = 5
	term.FeedBytes([]byte("\x1b[999A"))
	if term.cursor.Row != 0 {
		t.Errorf("expected row 0, got %d", term.cursor.Row)
	}
	if term.cursor.Col != 5 {
		t.Errorf("col should not change, expected 5, got %d", term.cursor.Col)
	}
}
