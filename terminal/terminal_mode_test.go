package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestDECAWMEnable(t *testing.T) {
	term, _ := newTerminalForTest(10, 24)
	term.FeedBytes([]byte("1234567890AB"))
	if term.cursor.Row != 1 || term.cursor.Col != 2 {
		t.Errorf("expected (1,2), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestDECAWMDisable(t *testing.T) {
	term, _ := newTerminalForTest(10, 24)
	term.FeedBytes([]byte("\x1b[?7l"))
	term.FeedBytes([]byte("1234567890AB"))
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
	term.FeedBytes([]byte("\x1b[?1h"))
	if !term.cursorKeysApp {
		t.Error("expected cursorKeysApp=true")
	}
}

func TestDECCKMDisable(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[?1l"))
	if term.cursorKeysApp {
		t.Error("expected cursorKeysApp=false")
	}
}

func TestBracketedPasteFlag(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[?2004h"))
	if !term.bracketedPaste {
		t.Error("expected bracketedPaste=true")
	}
	term.FeedBytes([]byte("\x1b[?2004l"))
	if term.bracketedPaste {
		t.Error("expected bracketedPaste=false")
	}
}

func TestAltScreen47(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("main"))
	term.FeedBytes([]byte("\x1b[?47h"))
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
	term.FeedBytes([]byte("\x1b[?1047h"))
	if term.screen != term.altBuf {
		t.Error("expected screen == altBuf")
	}
	term.FeedBytes([]byte("alt"))
	term.FeedBytes([]byte("\x1b[?1047l"))
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
	term.FeedBytes([]byte("\x1b[?1048h"))
	if term.saved.row != 5 || term.saved.col != 10 {
		t.Errorf("expected saved (5,10), got (%d,%d)", term.saved.row, term.saved.col)
	}
	term.cursor.Row = 0
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[?1048l"))
	if term.cursor.Row != 5 || term.cursor.Col != 10 {
		t.Errorf("expected restored (5,10), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestFocusReportingFlag(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[?1004h"))
	if !term.focusReporting {
		t.Error("expected focusReporting=true")
	}
	term.FeedBytes([]byte("\x1b[?1004l"))
	if term.focusReporting {
		t.Error("expected focusReporting=false")
	}
}

func TestDECSCUSRDoesNotBreakMode1049(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 3
	term.cursor.Col = 5
	term.FeedBytes([]byte("\x1b[?1049h"))
	if term.screen != term.altBuf {
		t.Error("expected alt screen after ?1049h")
	}
	term.FeedBytes([]byte("\x1b[?1049l"))
	if term.screen != term.mainBuf {
		t.Error("expected main screen after ?1049l")
	}
	if term.cursor.Row != 3 || term.cursor.Col != 5 {
		t.Errorf("expected cursor restored to (3,5), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestScrollRegionLineFeedScrolls(t *testing.T) {
	term, _ := newTerminalForTest(10, 4)
	term.FeedBytes([]byte("\x1b[?1049h"))
	term.FeedBytes([]byte("\x1b[1;3r"))
	term.FeedBytes([]byte("AAA\r\nBBB\r\nCCC"))
	term.FeedBytes([]byte("\r\n"))
	if term.screen.Cell(0, 0).Rune != 'B' {
		t.Errorf("row0: expected 'B' after scroll, got %c", term.screen.Cell(0, 0).Rune)
	}
	if term.screen.Cell(1, 0).Rune != 'C' {
		t.Errorf("row1: expected 'C' after scroll, got %c", term.screen.Cell(1, 0).Rune)
	}
	if term.screen.Cell(2, 0).Rune != ' ' {
		t.Errorf("row2: expected blank after scroll, got %c", term.screen.Cell(2, 0).Rune)
	}
	if term.cursor.Row != 2 {
		t.Errorf("cursor: expected at scrollBot row 2, got %d", term.cursor.Row)
	}
	if term.screen.Cell(3, 0).Rune != ' ' {
		t.Errorf("status row 3 should be untouched, got %c", term.screen.Cell(3, 0).Rune)
	}
}

func TestScrollRegionAutoWrapScrolls(t *testing.T) {
	term, _ := newTerminalForTest(3, 4)
	term.FeedBytes([]byte("\x1b[?1049h"))
	term.FeedBytes([]byte("\x1b[1;3r"))
	// With deferred wrap, writing "ABC" fills row 0 but doesn't scroll yet.
	// The wrap happens when the NEXT printable char arrives.
	// ABC->row0, DEF->row1, GHI->row2(scrollBot), J triggers wrap+scroll.
	term.FeedBytes([]byte("ABCDEFGHIJ"))
	if term.screen.Cell(0, 0).Rune != 'D' {
		t.Errorf("row0: expected 'D' after scroll, got %c", term.screen.Cell(0, 0).Rune)
	}
	if term.screen.Cell(1, 0).Rune != 'G' {
		t.Errorf("row1: expected 'G' after scroll, got %c", term.screen.Cell(1, 0).Rune)
	}
	if term.screen.Cell(2, 0).Rune != 'J' {
		t.Errorf("row2: expected 'J' after scroll, got %c", term.screen.Cell(2, 0).Rune)
	}
	if term.cursor.Row != 2 {
		t.Errorf("cursor: expected row 2, got %d", term.cursor.Row)
	}
}

func TestAltScreenNoScrollback(t *testing.T) {
	term, _ := newTerminalForTest(10, 5)
	term.FeedBytes([]byte("\x1b[?1049h"))
	term.FeedBytes([]byte("AAAAA\r\nBBBBB\r\nCCCCC\r\nDDDD\r\n"))
	if term.altBuf.History().Len() != 0 {
		t.Errorf("alt screen should have no scrollback, got %d", term.altBuf.History().Len())
	}
}

var _ screen.Attributes = 0
