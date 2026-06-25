package terminal

import "testing"

func TestDSRCursorPosition(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.cursor.Row = 3
	term.cursor.Col = 5
	term.FeedBytes([]byte("\x1b[6n"))
	if resp.String() != "\x1b[4;6R" {
		t.Errorf("expected '\\x1b[4;6R', got %q", resp.String())
	}
}

func TestDSRStatus(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[5n"))
	if resp.String() != "\x1b[0n" {
		t.Errorf("expected '\\x1b[0n', got %q", resp.String())
	}
}

func TestDA1Response(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[c"))
	if resp.String() != "\x1b[?62;4c" {
		t.Errorf("expected '\\x1b[?62;4c', got %q", resp.String())
	}
}

func TestDA1ResponseExplicit(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[0c"))
	if resp.String() != "\x1b[?62;4c" {
		t.Errorf("expected '\\x1b[?62;4c', got %q", resp.String())
	}
}

func TestDA2Response(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b[>c"))
	if resp.String() != "\x1b[>0;0;0c" {
		t.Errorf("expected '\\x1b[>0;0;0c', got %q", resp.String())
	}
}

func TestDECXCPR(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.cursor.Row = 2
	term.cursor.Col = 3
	term.FeedBytes([]byte("\x1b[?6n"))
	if resp.String() != "\x1b[?3;4;1R" {
		t.Errorf("expected '\\x1b[?3;4;1R', got %q", resp.String())
	}
}
