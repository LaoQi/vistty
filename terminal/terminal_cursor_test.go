package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestDECSCUSRSteadyBlock(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[2 q"))
	if term.cursor.Style != screen.CursorBlock {
		t.Errorf("expected Block, got %d", term.cursor.Style)
	}
	if term.cursor.Blinking {
		t.Error("expected Blinking=false")
	}
}

func TestDECSCUSRBlinkingUnderline(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[3 q"))
	if term.cursor.Style != screen.CursorUnderline {
		t.Errorf("expected Underline, got %d", term.cursor.Style)
	}
	if !term.cursor.Blinking {
		t.Error("expected Blinking=true")
	}
}

func TestDECSCUSRSteadyBar(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[6 q"))
	if term.cursor.Style != screen.CursorBar {
		t.Errorf("expected Bar, got %d", term.cursor.Style)
	}
	if term.cursor.Blinking {
		t.Error("expected Blinking=false")
	}
}

func TestDECSCUSRDefault(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.feedBytes([]byte("\x1b[0 q"))
	if term.cursor.Style != screen.CursorBlock {
		t.Errorf("expected Block, got %d", term.cursor.Style)
	}
	if !term.cursor.Blinking {
		t.Error("expected Blinking=true")
	}
}
