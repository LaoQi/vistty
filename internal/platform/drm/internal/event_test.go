package internal

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestModeInfoRoundTrip(t *testing.T) {
	original := ModeInfo{
		Clock:      148500,
		HDisplay:   1920,
		HSyncStart: 2008,
		HSyncEnd:   2052,
		HTotal:     2200,
		VDisplay:   1080,
		VSyncStart: 1084,
		VSyncEnd:   1089,
		VTotal:     1125,
		VRefresh:   60,
		Flags:      0x5,
		Type:       0x40,
	}
	name := "1920x1080"
	copy(original.Name[:], name)

	pub := modeInfoToPublic(&original)
	if pub.Clock != original.Clock {
		t.Errorf("Clock mismatch: %d vs %d", pub.Clock, original.Clock)
	}
	if pub.HDisplay != original.HDisplay {
		t.Errorf("HDisplay mismatch: %d vs %d", pub.HDisplay, original.HDisplay)
	}
	if pub.Name != name {
		t.Errorf("Name mismatch: %q vs %q", pub.Name, name)
	}

	roundTrip := publicToModeInfo(&pub)
	if roundTrip.Clock != original.Clock {
		t.Errorf("round-trip Clock mismatch: %d vs %d", roundTrip.Clock, original.Clock)
	}
	if roundTrip.HDisplay != original.HDisplay {
		t.Errorf("round-trip HDisplay mismatch: %d vs %d", roundTrip.HDisplay, original.HDisplay)
	}
	if roundTrip.VRefresh != original.VRefresh {
		t.Errorf("round-trip VRefresh mismatch: %d vs %d", roundTrip.VRefresh, original.VRefresh)
	}
}

func TestModeInfoPublicNameTrim(t *testing.T) {
	m := ModeInfo{}
	copy(m.Name[:], "test\x00\x00")
	pub := modeInfoToPublic(&m)
	if pub.Name != "test" {
		t.Errorf("expected trimmed name 'test', got %q", pub.Name)
	}
}

func TestReadEventFlipComplete(t *testing.T) {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[0:4], EventFlipComplete)
	binary.LittleEndian.PutUint32(buf[4:8], 32)
	binary.LittleEndian.PutUint32(buf[16:20], 1000)
	binary.LittleEndian.PutUint32(buf[20:24], 500)
	binary.LittleEndian.PutUint32(buf[24:28], 42)
	binary.LittleEndian.PutUint32(buf[28:32], 7)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("osPipe: %v", err)
	}
	defer r.Close()
	if _, err := w.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	ev, err := ReadEvent(int(r.Fd()))
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil event")
	}
	if ev.Type != EventFlipComplete {
		t.Errorf("expected type %d, got %d", EventFlipComplete, ev.Type)
	}
	if ev.TVSec != 1000 {
		t.Errorf("expected TVSec 1000, got %d", ev.TVSec)
	}
	if ev.Sequence != 42 {
		t.Errorf("expected Sequence 42, got %d", ev.Sequence)
	}
	if ev.CrtcID != 7 {
		t.Errorf("expected CrtcID 7, got %d", ev.CrtcID)
	}
}

func TestReadEventTooShort(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	if _, err := w.Write([]byte{1, 2, 3}); err != nil {
		t.Fatalf("write: %v", err)
	}
	w.Close()

	_, err = ReadEvent(int(r.Fd()))
	if err == nil {
		t.Error("expected error for short read")
	}
}
