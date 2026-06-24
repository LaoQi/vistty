package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestDECSCSavesAttributes(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[1;31m"))
	term.cursor.Row = 5
	term.cursor.Col = 10
	term.feedBytes([]byte("\x1b7"))
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
	term.feedBytes([]byte("\x1b[1;31m"))
	term.feedBytes([]byte("\x1b7"))
	term.feedBytes([]byte("\x1b[0;34m"))
	term.feedBytes([]byte("\x1b8"))
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
	term.feedBytes([]byte("\x1b[s"))
	if term.saved.row != 3 || term.saved.col != 7 {
		t.Errorf("expected saved (3,7), got (%d,%d)", term.saved.row, term.saved.col)
	}
	term.cursor.Row = 0
	term.cursor.Col = 0
	term.feedBytes([]byte("\x1b[u"))
	if term.cursor.Row != 3 || term.cursor.Col != 7 {
		t.Errorf("expected restored (3,7), got (%d,%d)", term.cursor.Row, term.cursor.Col)
	}
}
