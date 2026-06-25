package terminal

import "testing"

func TestDECLineDrawing(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b(0lqk"))
	cell := term.screen.Cell(0, 0)
	if cell.Rune != '\u250C' {
		t.Errorf("expected ┌ (U+250C), got U+%04X", cell.Rune)
	}
	cell = term.screen.Cell(0, 1)
	if cell.Rune != '\u2500' {
		t.Errorf("expected ─ (U+2500), got U+%04X", cell.Rune)
	}
	cell = term.screen.Cell(0, 2)
	if cell.Rune != '\u2510' {
		t.Errorf("expected ┐ (U+2510), got U+%04X", cell.Rune)
	}
}

func TestCharsetRestore(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b(0a"))
	term.FeedBytes([]byte("\x1b(B"))
	term.FeedBytes([]byte("a"))
	cell := term.screen.Cell(0, 0)
	if cell.Rune != '\u2592' {
		t.Errorf("expected ▒ (U+2592), got U+%04X", cell.Rune)
	}
	cell = term.screen.Cell(0, 1)
	if cell.Rune != 'a' {
		t.Errorf("expected 'a', got %c", cell.Rune)
	}
}

func TestSOSI(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b)0"))
	term.FeedBytes([]byte("\x0e"))
	term.FeedBytes([]byte("q"))
	term.FeedBytes([]byte("\x0f"))
	term.FeedBytes([]byte("q"))
	cell := term.screen.Cell(0, 0)
	if cell.Rune != '\u2500' {
		t.Errorf("expected ─ (U+2500) after SO, got U+%04X", cell.Rune)
	}
	cell = term.screen.Cell(0, 1)
	if cell.Rune != 'q' {
		t.Errorf("expected 'q' after SI, got %c", cell.Rune)
	}
}
