package vte

import "testing"

func TestCSICursorUp(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'A', Params: [16]int{5}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorUp {
		t.Errorf("expected CSICursorUp, got %d", csi.Command)
	}
	if csi.Params[0] != 5 {
		t.Errorf("expected param 5, got %d", csi.Params[0])
	}
}

func TestCSICursorPosCmd(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'H', Params: [16]int{10, 20}, NParams: 2}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorPosition {
		t.Errorf("expected CSICursorPosition, got %d", csi.Command)
	}
	if csi.Params[0] != 10 || csi.Params[1] != 20 {
		t.Errorf("expected params [10,20], got %v", csi.Params[:csi.NParams])
	}
}

func TestCSIDefaultParams(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'A'}
	csi := ParseCSI(seq)
	if csi.Command != CSICursorUp {
		t.Errorf("expected CSICursorUp, got %d", csi.Command)
	}
	if csi.Params[0] != 1 {
		t.Errorf("expected default param 1, got %d", csi.Params[0])
	}
}

func TestCSIEraseInDisplay(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'J', Params: [16]int{2}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIEraseInDisplay {
		t.Errorf("expected CSIEraseInDisplay, got %d", csi.Command)
	}
	if csi.Params[0] != 2 {
		t.Errorf("expected param 2, got %d", csi.Params[0])
	}
}

func TestCSISGR(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'm', Params: [16]int{1, 31}, NParams: 2}
	csi := ParseCSI(seq)
	if csi.Command != CSISGR {
		t.Errorf("expected CSISGR, got %d", csi.Command)
	}
	if csi.NParams != 2 || csi.Params[0] != 1 || csi.Params[1] != 31 {
		t.Errorf("expected params [1,31], got %v", csi.Params[:csi.NParams])
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
	seq := Sequence{Action: ActionCSI, Command: 'P', Params: [16]int{3}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIDeleteChars {
		t.Errorf("expected CSIDeleteChars, got %d", csi.Command)
	}
	if csi.Params[0] != 3 {
		t.Errorf("expected param 3, got %d", csi.Params[0])
	}
}

func TestCSIMargins(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'r', Params: [16]int{1, 24}, NParams: 2}
	csi := ParseCSI(seq)
	if csi.Command != CSISetTopBottomMargin {
		t.Errorf("expected CSISetTopBottomMargin, got %d", csi.Command)
	}
	if csi.Params[0] != 1 || csi.Params[1] != 24 {
		t.Errorf("expected params [1,24], got %v", csi.Params[:csi.NParams])
	}
}

func TestCSIEraseChars(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'X', Params: [16]int{3}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIEraseChars {
		t.Errorf("expected CSIEraseChars, got %d", csi.Command)
	}
	if csi.Params[0] != 3 {
		t.Errorf("expected param 3, got %d", csi.Params[0])
	}
}

func TestCSIDeviceStatusReport(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'n', Params: [16]int{6}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIDeviceStatusReport {
		t.Errorf("expected CSIDeviceStatusReport, got %d", csi.Command)
	}
}

func TestCSIDeviceAttributes(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'c', Params: [16]int{0}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIDeviceAttributes {
		t.Errorf("expected CSIDeviceAttributes, got %d", csi.Command)
	}
}

func TestCSIDECSCUSR(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '3', ' ', 'q'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	csi := ParseCSI(seqs[0])
	if csi.Command != CSICursorStyle {
		t.Errorf("expected CSICursorStyle, got %d", csi.Command)
	}
	if csi.Params[0] != 3 {
		t.Errorf("expected param 3, got %d", csi.Params[0])
	}
}

func TestCSIDECSCA(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'q', Intermed: []byte{'"'}, Params: [16]int{1}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSISetCharProtection {
		t.Errorf("expected CSISetCharProtection, got %d", csi.Command)
	}
}

func TestCSIBareQUnknown(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'q', Params: [16]int{1}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSIUnknown {
		t.Errorf("expected CSIUnknown for bare q, got %d", csi.Command)
	}
}

func TestCSIDA2(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '>', 'c'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	csi := ParseCSI(seqs[0])
	if csi.Command != CSIDeviceAttributes2 {
		t.Errorf("expected CSIDeviceAttributes2, got %d", csi.Command)
	}
	if !csi.Private {
		t.Error("expected Private=true")
	}
}

func TestCSITabClear(t *testing.T) {
	seq := Sequence{Action: ActionCSI, Command: 'g', Params: [16]int{3}, NParams: 1}
	csi := ParseCSI(seq)
	if csi.Command != CSITabClear {
		t.Errorf("expected CSITabClear, got %d", csi.Command)
	}
}

func TestCSIDECPrivateDSR(t *testing.T) {
	p := NewParser()
	seqs := p.FeedAll([]byte{0x1B, '[', '?', '6', 'n'})
	if len(seqs) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(seqs))
	}
	csi := ParseCSI(seqs[0])
	if csi.Command != CSIDeviceStatusReport {
		t.Errorf("expected CSIDeviceStatusReport, got %d", csi.Command)
	}
	if !csi.Private {
		t.Error("expected Private=true")
	}
}
