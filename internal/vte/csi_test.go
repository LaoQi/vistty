package vte

import "testing"

func TestCSICursorUp(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'A', Params: []int{5}}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorUp {
		t.Errorf("expected CSICursorUp, got %d", csi.Command)
	}
	if csi.Params[0] != 5 {
		t.Errorf("expected param 5, got %d", csi.Params[0])
	}
}

func TestCSICursorPosCmd(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'H', Params: []int{10, 20}}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorPosition {
		t.Errorf("expected CSICursorPosition, got %d", csi.Command)
	}
	if csi.Params[0] != 10 || csi.Params[1] != 20 {
		t.Errorf("expected params [10,20], got %v", csi.Params)
	}
}

func TestCSIDefaultParams(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'A', Params: nil}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorUp {
		t.Errorf("expected CSICursorUp, got %d", csi.Command)
	}
	if csi.Params[0] != 1 {
		t.Errorf("expected default param 1, got %d", csi.Params[0])
	}
}

func TestCSIEraseInDisplay(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'J', Params: []int{2}}
	csi := ParseCSI(seq)
	if csi.Command != CSIEraseInDisplay {
		t.Errorf("expected CSIEraseInDisplay, got %d", csi.Command)
	}
	if csi.Params[0] != 2 {
		t.Errorf("expected param 2, got %d", csi.Params[0])
	}
}

func TestCSISGR(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'm', Params: []int{1, 31}}
	csi := ParseCSI(seq)
	if csi.Command != CSISGR {
		t.Errorf("expected CSISGR, got %d", csi.Command)
	}
	if len(csi.Params) != 2 || csi.Params[0] != 1 || csi.Params[1] != 31 {
		t.Errorf("expected params [1,31], got %v", csi.Params)
	}
}

func TestCSICursorHide(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '?', '2', '5', 'l'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	csi := ParseCSI(seqs[0])
	if csi.Command != CSICursorHide {
		t.Errorf("expected CSICursorHide, got %d", csi.Command)
	}
	if !csi.Private {
		t.Error("expected Private=true")
	}
}

func TestCSICursorShow(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '?', '2', '5', 'h'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	csi := ParseCSI(seqs[0])
	if csi.Command != CSICursorShow {
		t.Errorf("expected CSICursorShow, got %d", csi.Command)
	}
}

func TestCSIDeleteChars(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'P', Params: []int{3}}
	csi := ParseCSI(seq)
	if csi.Command != CSIDeleteChars {
		t.Errorf("expected CSIDeleteChars, got %d", csi.Command)
	}
	if csi.Params[0] != 3 {
		t.Errorf("expected param 3, got %d", csi.Params[0])
	}
}

func TestCSIMargins(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'r', Params: []int{1, 24}}
	csi := ParseCSI(seq)
	if csi.Command != CSISetTopBottomMargin {
		t.Errorf("expected CSISetTopBottomMargin, got %d", csi.Command)
	}
	if csi.Params[0] != 1 || csi.Params[1] != 24 {
		t.Errorf("expected params [1,24], got %v", csi.Params)
	}
}
