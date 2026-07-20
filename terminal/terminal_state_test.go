package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestDECSCSavesAttributes(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[1;31m"))
	term.cursor.Row = 5
	term.cursor.Col = 10
	term.FeedBytes([]byte("\x1b7"))
	if term.saved.row != 5 || term.saved.col != 10 {
		t.Errorf("expected saved cursor (5,10), got (%d,%d)", term.saved.row, term.saved.col)
	}
	if term.saved.attr&screen.AttrBold == 0 {
		t.Error("expected saved attr to have Bold")
	}
	if term.saved.fg.R != 205 || term.saved.fg.G != 0 || term.saved.fg.B != 0 {
		t.Errorf("expected saved fg red (205,0,0), got (%d,%d,%d)", term.saved.fg.R, term.saved.fg.G, term.saved.fg.B)
	}
}

func TestDECRCRestoresAttributes(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[1;31m"))
	term.FeedBytes([]byte("\x1b7"))
	term.FeedBytes([]byte("\x1b[0;34m"))
	term.FeedBytes([]byte("\x1b8"))
	if term.curAttr&screen.AttrBold == 0 {
		t.Error("expected bold restored")
	}
	if term.curFg.R != 205 {
		t.Errorf("expected fg red restored (R=205), got R=%d", term.curFg.R)
	}
}

func TestSCOSCEquivalence(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.cursor.Row = 3
	term.cursor.Col = 7
	term.FeedBytes([]byte("\x1b[s"))
	if term.saved.row != 3 || term.saved.col != 7 {
		t.Errorf("expected saved (3,7), got (%d,%d)", term.saved.row, term.saved.col)
	}
	term.cursor.Row = 0
	term.cursor.Col = 0
	term.FeedBytes([]byte("\x1b[u"))
	if term.cursor.Row != 3 || term.cursor.Col != 7 {
		t.Errorf("expected restored (3,7), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}

func TestFullResetResetsModes(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[?1h\x1b[?2004h\x1b[?1004h\x1b[?2026h\x1b[?25l\x1b[4 q"))
	term.title = "test-title"
	if !term.cursorKeysApp || !term.bracketedPaste || !term.focusReporting {
		t.Fatal("setup failed: expected modes enabled")
	}
	if !term.IsSyncUpdates() {
		t.Fatal("setup failed: expected appSyncUpdates=true")
	}
	if term.cursor.Visible {
		t.Fatal("setup failed: expected cursor hidden")
	}
	if term.cursor.Blinking {
		t.Fatal("setup failed: expected cursor not blinking")
	}
	term.FeedBytes([]byte("\x1bc"))
	if term.cursorKeysApp {
		t.Error("expected cursorKeysApp=false after full reset")
	}
	if term.bracketedPaste {
		t.Error("expected bracketedPaste=false after full reset")
	}
	if term.focusReporting {
		t.Error("expected focusReporting=false after full reset")
	}
	if term.IsSyncUpdates() {
		t.Error("expected appSyncUpdates=false after full reset")
	}
	if !term.cursor.Visible {
		t.Error("expected cursor.Visible=true after full reset")
	}
	if !term.cursor.Blinking {
		t.Error("expected cursor.Blinking=true after full reset")
	}
	if term.cursor.Style != screen.CursorBlock {
		t.Error("expected cursor.Style=CursorBlock after full reset")
	}
	if term.title != "" {
		t.Errorf("expected title cleared, got %q", term.title)
	}
}
