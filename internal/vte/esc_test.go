package vte

import (
	"testing"
)

func TestParseESCSaveCursor(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: '7'}
	esc := ParseESC(seq)
	if esc.Command != ESCResetState {
		t.Errorf("expected ESCResetState, got %d", esc.Command)
	}
}

func TestParseESCRestoreCursor(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: '8'}
	esc := ParseESC(seq)
	if esc.Command != ESCRestoreState {
		t.Errorf("expected ESCRestoreState, got %d", esc.Command)
	}
}

func TestParseESCIndex(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'D'}
	esc := ParseESC(seq)
	if esc.Command != ESCIndex {
		t.Errorf("expected ESCIndex, got %d", esc.Command)
	}
}

func TestParseESCNextLine(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'E'}
	esc := ParseESC(seq)
	if esc.Command != ESCNextLine {
		t.Errorf("expected ESCNextLine, got %d", esc.Command)
	}
}

func TestParseESCReverseIndex(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'M'}
	esc := ParseESC(seq)
	if esc.Command != ESCReverseIndex {
		t.Errorf("expected ESCReverseIndex, got %d", esc.Command)
	}
}

func TestParseESCTabSet(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'H'}
	esc := ParseESC(seq)
	if esc.Command != ESCTabSet {
		t.Errorf("expected ESCTabSet, got %d", esc.Command)
	}
}

func TestParseESCDeckpam(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: '='}
	esc := ParseESC(seq)
	if esc.Command != ESCDeckpam {
		t.Errorf("expected ESCDeckpam, got %d", esc.Command)
	}
}

func TestParseESCDeckpnm(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: '>'}
	esc := ParseESC(seq)
	if esc.Command != ESCDeckpnm {
		t.Errorf("expected ESCDeckpnm, got %d", esc.Command)
	}
}

func TestParseESCFullReset(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'c'}
	esc := ParseESC(seq)
	if esc.Command != ESCFullReset {
		t.Errorf("expected ESCFullReset, got %d", esc.Command)
	}
}

func TestParseESCUnknown(t *testing.T) {
	seq := Sequence{Action: ActionESC, Command: 'Z'}
	esc := ParseESC(seq)
	if esc.Command != ESCUnknown {
		t.Errorf("expected ESCUnknown, got %d", esc.Command)
	}
}

func TestESCG0CharsetUS(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '(', 'B'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	esc := ParseESC(seqs[0])
	if esc.Command != ESCDesignateG0 {
		t.Errorf("expected ESCDesignateG0, got %d", esc.Command)
	}
	if esc.Charset != 'B' {
		t.Errorf("expected charset 'B', got %c", esc.Charset)
	}
}

func TestESCG0CharsetDEC(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '(', '0'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	esc := ParseESC(seqs[0])
	if esc.Command != ESCDesignateG0 {
		t.Errorf("expected ESCDesignateG0, got %d", esc.Command)
	}
	if esc.Charset != '0' {
		t.Errorf("expected charset '0', got %c", esc.Charset)
	}
}

func TestESCG1Charset(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, ')', '0'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	esc := ParseESC(seqs[0])
	if esc.Command != ESCDesignateG1 {
		t.Errorf("expected ESCDesignateG1, got %d", esc.Command)
	}
	if esc.Charset != '0' {
		t.Errorf("expected charset '0', got %c", esc.Charset)
	}
}
