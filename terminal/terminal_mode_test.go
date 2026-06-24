package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestDECAWMEnable(t *testing.T) {
	term, _ := newTerminalForTest(10, 24)
	term.feedBytes([]byte("1234567890AB"))
	if term.cursor.Row != 1 || term.cursor.Col != 2 {
		t.Errorf("expected (1,2), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestDECAWMDisable(t *testing.T) {
	term, _ := newTerminalForTest(10, 24)
	term.feedBytes([]byte("\x1b[?7l"))
	term.feedBytes([]byte("1234567890AB"))
	if term.cursor.Row != 0 || term.cursor.Col != 9 {
		t.Errorf("expected (0,9), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
	cell := term.screen.Cell(0, 9)
	if cell.Rune != 'B' {
		t.Errorf("expected 'B' at (0,9), got %c", cell.Rune)
	}
}

func TestDECCKMEnable(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[?1h"))
	if !term.cursorKeysApp {
		t.Error("expected cursorKeysApp=true")
	}
}

func TestDECCKMDisable(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[?1l"))
	if term.cursorKeysApp {
		t.Error("expected cursorKeysApp=false")
	}
}

func TestBracketedPasteFlag(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[?2004h"))
	if !term.bracketedPaste {
		t.Error("expected bracketedPaste=true")
	}
	term.feedBytes([]byte("\x1b[?2004l"))
	if term.bracketedPaste {
		t.Error("expected bracketedPaste=false")
	}
}

func TestAltScreen47(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("main"))
	term.feedBytes([]byte("\x1b[?47h"))
	if term.screen != term.altBuf {
		t.Error("expected screen == altBuf")
	}
	cell := term.mainBuf.Cell(0, 0)
	if cell.Rune != 'm' {
		t.Errorf("expected 'm' in mainBuf, got %c", cell.Rune)
	}
}

func TestAltScreen1047(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[?1047h"))
	if term.screen != term.altBuf {
		t.Error("expected screen == altBuf")
	}
	term.feedBytes([]byte("alt"))
	term.feedBytes([]byte("\x1b[?1047l"))
	if term.screen != term.mainBuf {
		t.Error("expected screen == mainBuf")
	}
	cell := term.altBuf.Cell(0, 0)
	if cell.Rune != ' ' {
		t.Errorf("expected altBuf cleared, got %c", cell.Rune)
	}
}

func TestSaveCursor1048(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 5
	term.cursor.Col = 10
	term.feedBytes([]byte("\x1b[?1048h"))
	if term.saved.row != 5 || term.saved.col != 10 {
		t.Errorf("expected saved (5,10), got (%d,%d)", term.saved.row, term.saved.col)
	}
	term.cursor.Row = 0
	term.cursor.Col = 0
	term.feedBytes([]byte("\x1b[?1048l"))
	if term.cursor.Row != 5 || term.cursor.Col != 10 {
		t.Errorf("expected restored (5,10), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestFocusReportingFlag(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[?1004h"))
	if !term.focusReporting {
		t.Error("expected focusReporting=true")
	}
	term.feedBytes([]byte("\x1b[?1004l"))
	if term.focusReporting {
		t.Error("expected focusReporting=false")
	}
}

func TestDECSCUSRDoesNotBreakMode1049(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 3
	term.cursor.Col = 5
	term.feedBytes([]byte("\x1b[?1049h"))
	if term.screen != term.altBuf {
		t.Error("expected alt screen after ?1049h")
	}
	term.feedBytes([]byte("\x1b[?1049l"))
	if term.screen != term.mainBuf {
		t.Error("expected main screen after ?1049l")
	}
	if term.cursor.Row != 3 || term.cursor.Col != 5 {
		t.Errorf("expected cursor restored to (3,5), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

var _ screen.Attributes = 0
