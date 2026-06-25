package vte

import (
	"testing"
)

func TestParseOSCSetWindowTitle(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("0;hello world")}
	osc := ParseOSC(seq)
	if osc.Command != OSCSetWindowTitle {
		t.Errorf("expected OSCSetWindowTitle, got %d", osc.Command)
	}
	if osc.Data != "hello world" {
		t.Errorf("expected 'hello world', got %q", osc.Data)
	}
}

func TestParseOSCEmptyCommand(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte(";title")}
	osc := ParseOSC(seq)
	if osc.Command != OSCUnknown {
		t.Errorf("expected OSCUnknown for empty command, got %d", osc.Command)
	}
}

func TestParseOSCNoSemicolon(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("justdata")}
	osc := ParseOSC(seq)
	if osc.Command != OSCUnknown {
		t.Errorf("expected OSCUnknown for no semicolon, got %d", osc.Command)
	}
}

func TestParseOSCNonNumericCommand(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("abc;data")}
	osc := ParseOSC(seq)
	if osc.Command != OSCUnknown {
		t.Errorf("expected OSCUnknown for non-numeric, got %d", osc.Command)
	}
}

func TestParseOSCSetIconTitle(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("1;icon title")}
	osc := ParseOSC(seq)
	if osc.Command != OSCSetIconTitle {
		t.Errorf("expected OSCSetIconTitle, got %d", osc.Command)
	}
}

func TestParseOSCSetClipboard(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("52;clipdata")}
	osc := ParseOSC(seq)
	if osc.Command != OSCSetClipboard {
		t.Errorf("expected OSCSetClipboard, got %d", osc.Command)
	}
}

func TestParseOSCSetWorkingDir(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("7;/home/user")}
	osc := ParseOSC(seq)
	if osc.Command != OSCSetWorkingDir {
		t.Errorf("expected OSCSetWorkingDir, got %d", osc.Command)
	}
}

func TestParseOSCHyperlink(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("8;;https://example.com")}
	osc := ParseOSC(seq)
	if osc.Command != OSCHyperlink {
		t.Errorf("expected OSCHyperlink, got %d", osc.Command)
	}
}

func TestParseOSCFgColor(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("10;?")}
	osc := ParseOSC(seq)
	if osc.Command != OSCFgColor {
		t.Errorf("expected OSCFgColor, got %d", osc.Command)
	}
	if osc.Data != "?" {
		t.Errorf("expected '?', got %q", osc.Data)
	}
}

func TestParseOSCBgColor(t *testing.T) {
	seq := Sequence{Action: ActionOSC, Data: []byte("11;?")}
	osc := ParseOSC(seq)
	if osc.Command != OSCBgColor {
		t.Errorf("expected OSCBgColor, got %d", osc.Command)
	}
	if osc.Data != "?" {
		t.Errorf("expected '?', got %q", osc.Data)
	}
}

func TestOSC2WindowTitle(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, ']', '2', ';', 'w', 'i', 'n', 0x07})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	osc := ParseOSC(seqs[0])
	if osc.Command != OSCSetWindowTitle {
		t.Errorf("expected OSCSetWindowTitle, got %d", osc.Command)
	}
}
