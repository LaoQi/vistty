package vte

import (
	"testing"
)

func TestPrintCharacters(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte("Hello"))
	if len(seqs) != 5 {
		t.Fatalf("expected 5 sequences, got %d", len(seqs))
	}
	expected := []rune{'H', 'e', 'l', 'l', 'o'}
	for i, seq := range seqs {
		if seq.Action != ActionPrint {
			t.Errorf("seq[%d]: expected ActionPrint, got %d", i, seq.Action)
		}
		if seq.Rune != expected[i] {
			t.Errorf("seq[%d]: expected rune %c, got %c", i, expected[i], seq.Rune)
		}
	}
}

func TestControlCharacters(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x0D, 0x0A})
	if len(seqs) != 2 {
		t.Fatalf("expected 2 sequences, got %d", len(seqs))
	}
	if seqs[0].Action != ActionExecute || seqs[0].Command != 0x0D {
		t.Errorf("expected CR execute, got action=%d command=0x%02X", seqs[0].Action, seqs[0].Command)
	}
	if seqs[1].Action != ActionExecute || seqs[1].Command != 0x0A {
		t.Errorf("expected LF execute, got action=%d command=0x%02X", seqs[1].Action, seqs[1].Command)
	}
}

func TestESCSequence(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '7'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionESC {
		t.Fatalf("expected ActionESC, got %d", seqs[0].Action)
	}
	if seqs[0].Command != '7' {
		t.Errorf("expected command '7', got %c", seqs[0].Command)
	}
}

func TestCSICursorPosition(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '1', ';', '2', 'H'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionCSI {
		t.Fatalf("expected ActionCSI, got %d", seqs[0].Action)
	}
	if seqs[0].Command != 'H' {
		t.Errorf("expected command 'H', got %c", seqs[0].Command)
	}
	if len(seqs[0].Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(seqs[0].Params))
	}
	if seqs[0].Params[0] != 1 || seqs[0].Params[1] != 2 {
		t.Errorf("expected params [1,2], got %v", seqs[0].Params)
	}
}

func TestCSINoParams(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', 'H'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionCSI {
		t.Fatalf("expected ActionCSI, got %d", seqs[0].Action)
	}
	if seqs[0].Command != 'H' {
		t.Errorf("expected command 'H', got %c", seqs[0].Command)
	}
	if len(seqs[0].Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(seqs[0].Params))
	}
}

func TestCSIPrivateMode(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '?', '2', '5', 'l'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionCSI {
		t.Fatalf("expected ActionCSI, got %d", seqs[0].Action)
	}
	if seqs[0].Command != 'l' {
		t.Errorf("expected command 'l', got %c", seqs[0].Command)
	}
	if len(seqs[0].Params) != 1 || seqs[0].Params[0] != 25 {
		t.Errorf("expected params [25], got %v", seqs[0].Params)
	}
}

func TestOSCBelTerminated(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, ']', '0', ';', 't', 'i', 't', 'l', 'e', 0x07})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionOSC {
		t.Fatalf("expected ActionOSC, got %d", seqs[0].Action)
	}
	if string(seqs[0].Data) != "0;title" {
		t.Errorf("expected data '0;title', got %q", string(seqs[0].Data))
	}
}

func TestOSCStringTerminated(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, ']', '0', ';', 't', 'i', 't', 0x1B, '\\'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionOSC {
		t.Fatalf("expected ActionOSC, got %d", seqs[0].Action)
	}
	if string(seqs[0].Data) != "0;tit" {
		t.Errorf("expected data '0;tit', got %q", string(seqs[0].Data))
	}
}

func TestMixedInput(t *testing.T) {
	p := NewParser()
	input := []byte("A" + "\x1B[1;1H" + "B" + "\x0D\x0A")
	seqs := p.FeedAll(input)

	var actions []Action
	for _, s := range seqs {
		actions = append(actions, s.Action)
	}

	expected := []Action{ActionPrint, ActionCSI, ActionPrint, ActionExecute, ActionExecute}
	if len(actions) != len(expected) {
		t.Fatalf("expected %d actions, got %d: %v", len(expected), len(actions), actions)
	}
	for i, a := range actions {
		if a != expected[i] {
			t.Errorf("action[%d]: expected %d, got %d", i, expected[i], a)
		}
	}
}

func TestCSIMultipleParams(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '3', '8', ';', '5', ';', '1', '9', '6', 'm'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Command != 'm' {
		t.Errorf("expected command 'm', got %c", seqs[0].Command)
	}
	expected := []int{38, 5, 196}
	if len(seqs[0].Params) != len(expected) {
		t.Fatalf("expected %d params, got %d", len(expected), len(seqs[0].Params))
	}
	for i, v := range expected {
		if seqs[0].Params[i] != v {
			t.Errorf("param[%d]: expected %d, got %d", i, v, seqs[0].Params[i])
		}
	}
}

func TestParserReset(t *testing.T) {
	p := NewParser()
	p.FeedAll([]byte{0x1B, '[', '1', ';'})
	p.Reset()
	seqs := p.FeedAll([]byte("A"))
	if len(seqs) != 1 || seqs[0].Action != ActionPrint {
		t.Errorf("expected Print after reset, got %v", seqs)
	}
}

func TestDCS(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, 'P', 'd', 'a', 't', 'a', 0x1B, '\\'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionDCS {
		t.Fatalf("expected ActionDCS, got %d", seqs[0].Action)
	}
}

func TestESCIntermediate(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, ' ', 'F'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	if seqs[0].Action != ActionESC {
		t.Fatalf("expected ActionESC, got %d", seqs[0].Action)
	}
	if len(seqs[0].Intermed) != 1 || seqs[0].Intermed[0] != ' ' {
		t.Errorf("expected intermediate [' '], got %v", seqs[0].Intermed)
	}
	if seqs[0].Command != 'F' {
		t.Errorf("expected command 'F', got %c", seqs[0].Command)
	}
}
