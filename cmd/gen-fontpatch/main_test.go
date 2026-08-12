package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoNumber(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "000-x.vfp"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "002-y.vfp"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("z"), 0o644)
	n, err := nextPatchNumber(dir)
	if err != nil {
		t.Fatalf("nextPatchNumber: %v", err)
	}
	if n != 3 {
		t.Fatalf("nextPatchNumber = %d, want 3", n)
	}

	// Empty dir -> 0.
	empty := t.TempDir()
	n, err = nextPatchNumber(empty)
	if err != nil {
		t.Fatalf("nextPatchNumber(empty): %v", err)
	}
	if n != 0 {
		t.Fatalf("nextPatchNumber(empty) = %d, want 0", n)
	}
}

func TestParseRange(t *testing.T) {
	lo, hi, err := parseRange("U+3040-30FF")
	if err != nil || lo != 0x3040 || hi != 0x30FF {
		t.Fatalf("parseRange(U+3040-30FF) = %x,%x,%v", lo, hi, err)
	}
	lo, hi, err = parseRange("U+31F0")
	if err != nil || lo != 0x31F0 || hi != 0x31F0 {
		t.Fatalf("parseRange(U+31F0) = %x,%x,%v", lo, hi, err)
	}
	if _, _, err := parseRange("zzz"); err == nil {
		t.Fatal("expected error for bad range")
	}
}
