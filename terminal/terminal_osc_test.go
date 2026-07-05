package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

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

func TestOSCBgColorQuery(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]11;?\x07"))
	want := "\x1b]11;rgb:0000/0000/0000\x07"
	if resp.String() != want {
		t.Errorf("expected %q, got %q", want, resp.String())
	}
}

func TestOSCFgColorQuery(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]10;?\x07"))
	want := "\x1b]10;rgb:ffff/ffff/ffff\x07"
	if resp.String() != want {
		t.Errorf("expected %q, got %q", want, resp.String())
	}
}

func TestOSCFgColorSet(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	var cbFg, cbBg screen.Color
	var cbCalled bool
	term.opts.OnDefaultColor = func(fg, bg screen.Color) {
		cbFg, cbBg = fg, bg
		cbCalled = true
	}
	term.FeedBytes([]byte("\x1b]10;rgb:ff/00/00\x07"))
	if !cbCalled {
		t.Fatal("expected OnDefaultColor callback to be called")
	}
	if term.defFg.R != 255 || term.defFg.G != 0 || term.defFg.B != 0 {
		t.Errorf("expected defFg red=255,0,0 got %+v", term.defFg)
	}
	if cbFg.R != 255 || cbFg.G != 0 || cbFg.B != 0 {
		t.Errorf("expected callback fg red=255,0,0 got %+v", cbFg)
	}
	if cbBg.R != 0 || cbBg.G != 0 || cbBg.B != 0 {
		t.Errorf("expected callback bg unchanged 0,0,0 got %+v", cbBg)
	}
}

func TestOSCBgColorSet(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	var cbFg, cbBg screen.Color
	var cbCalled bool
	term.opts.OnDefaultColor = func(fg, bg screen.Color) {
		cbFg, cbBg = fg, bg
		cbCalled = true
	}
	term.FeedBytes([]byte("\x1b]11;#1e1e2e\x07"))
	if !cbCalled {
		t.Fatal("expected OnDefaultColor callback to be called")
	}
	if term.defBg.R != 0x1e || term.defBg.G != 0x1e || term.defBg.B != 0x2e {
		t.Errorf("expected defBg 1e,1e,2e got %+v", term.defBg)
	}
	if cbBg.R != 0x1e || cbBg.G != 0x1e || cbBg.B != 0x2e {
		t.Errorf("expected callback bg 1e,1e,2e got %+v", cbBg)
	}
	if cbFg.R != 255 || cbFg.G != 255 || cbFg.B != 255 {
		t.Errorf("expected callback fg unchanged 255,255,255 got %+v", cbFg)
	}
}

func TestOSCBgColorQueryAfterSet(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]11;rgb:3030/ffff/0000\x07"))
	resp.Reset()
	term.FeedBytes([]byte("\x1b]11;?\x07"))
	want := "\x1b]11;rgb:3030/ffff/0000\x07"
	if resp.String() != want {
		t.Errorf("expected %q, got %q", want, resp.String())
	}
}

func TestOSCFgColorSetHex(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]10;#aabbcc\x07"))
	if term.defFg.R != 0xaa || term.defFg.G != 0xbb || term.defFg.B != 0xcc {
		t.Errorf("expected defFg aa,bb,cc got %+v", term.defFg)
	}
}

func TestOSCFgColorSetShortHex(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]10;#abc\x07"))
	if term.defFg.R != 0xaa || term.defFg.G != 0xbb || term.defFg.B != 0xcc {
		t.Errorf("expected defFg aa,bb,cc got %+v", term.defFg)
	}
}

func TestOSCCursorColorQuery(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]12;?\x07"))
	want := "\x1b]12;rgb:ffff/ffff/ffff\x07"
	if resp.String() != want {
		t.Errorf("expected %q, got %q", want, resp.String())
	}
}

func TestOSCCursorColorSet(t *testing.T) {
	term, _ := newTerminalForTest(80, 24)
	var cbColor screen.Color
	var cbCalled bool
	term.opts.OnCursorColor = func(c screen.Color) {
		cbColor = c
		cbCalled = true
	}
	term.FeedBytes([]byte("\x1b]12;rgb:ff/00/00\x07"))
	if !cbCalled {
		t.Fatal("expected OnCursorColor callback to be called")
	}
	if term.cursorColor.R != 255 || term.cursorColor.G != 0 || term.cursorColor.B != 0 {
		t.Errorf("expected cursorColor red=255,0,0 got %+v", term.cursorColor)
	}
	if cbColor.R != 255 || cbColor.G != 0 || cbColor.B != 0 {
		t.Errorf("expected callback cursorColor red=255,0,0 got %+v", cbColor)
	}
}

func TestOSCCursorColorQueryAfterSet(t *testing.T) {
	term, resp := newTerminalForTest(80, 24)
	term.FeedBytes([]byte("\x1b]12;#1e1e2e\x07"))
	resp.Reset()
	term.FeedBytes([]byte("\x1b]12;?\x07"))
	want := "\x1b]12;rgb:1e1e/1e1e/2e2e\x07"
	if resp.String() != want {
		t.Errorf("expected %q, got %q", want, resp.String())
	}
}
