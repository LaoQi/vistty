package terminal

import "testing"

func TestOSCTitleCallback(t *testing.T) {
	var title string
	term, _ := newTerminalForTest(80, 24)
	term.opts.OnTitle = func(s string) { title = s }
	term.FeedBytes([]byte("\x1b]0;hello\x07"))
	if title != "hello" {
		t.Errorf("expected title 'hello', got %q", title)
	}
}

func TestOSC2WindowTitle(t *testing.T) {
	var title string
	term, _ := newTerminalForTest(80, 24)
	term.opts.OnTitle = func(s string) { title = s }
	term.FeedBytes([]byte("\x1b]2;win\x07"))
	if title != "win" {
		t.Errorf("expected title 'win', got %q", title)
	}
}
