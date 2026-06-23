package vte

import (
	"testing"
)

func TestParseControlLF(t *testing.T) {
	cc, ok := ParseControl(0x0A)
	if !ok || cc != ControlLF {
		t.Errorf("expected ControlLF, got %d, ok=%v", cc, ok)
	}
}

func TestParseControlCR(t *testing.T) {
	cc, ok := ParseControl(0x0D)
	if !ok || cc != ControlCR {
		t.Errorf("expected ControlCR, got %d, ok=%v", cc, ok)
	}
}

func TestParseControlBS(t *testing.T) {
	cc, ok := ParseControl(0x08)
	if !ok || cc != ControlBS {
		t.Errorf("expected ControlBS, got %d, ok=%v", cc, ok)
	}
}

func TestParseControlHT(t *testing.T) {
	cc, ok := ParseControl(0x09)
	if !ok || cc != ControlHT {
		t.Errorf("expected ControlHT, got %d, ok=%v", cc, ok)
	}
}

func TestParseControlBEL(t *testing.T) {
	cc, ok := ParseControl(0x07)
	if !ok || cc != ControlBEL {
		t.Errorf("expected ControlBEL, got %d, ok=%v", cc, ok)
	}
}

func TestParseControlInvalid(t *testing.T) {
	_, ok := ParseControl(0x20)
	if ok {
		t.Error("expected false for non-control character")
	}
}
