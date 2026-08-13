package font

import (
	"bytes"
	"testing"
)

func TestVfpRoundTrip(t *testing.T) {
	entries := []VfpEntry{
		{Rune: 'c', Advance: 0x1234, Lsb: -5},
		{Rune: 'a', Advance: 0x0100, Lsb: 0},
		{Rune: 'b', Advance: 0x2222, Lsb: 7},
	}
	glyphs := [][]byte{
		{0x10, 0x20, 0x30},
		{}, // zero-length glyph for 'a'
		{0xAA, 0xBB},
	}
	data, err := EncodeVFP(entries, glyphs)
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	f, err := ParseVFP(data)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	if f.Count() != 3 {
		t.Fatalf("count = %d, want 3", f.Count())
	}
	// Verify ascending order and per-entry fields.
	for i, r := range []rune{'a', 'b', 'c'} {
		off, _, adv, lsb, ok := f.Find(r)
		if !ok {
			t.Fatalf("Find(U+%X) miss", r)
		}
		wantAdv := uint16(0)
		wantLsb := int16(0)
		for _, e := range entries {
			if e.Rune == r {
				wantAdv = e.Advance
				wantLsb = e.Lsb
			}
		}
		if adv != wantAdv {
			t.Errorf("entry %d advance = %#x, want %#x", i, adv, wantAdv)
		}
		if lsb != wantLsb {
			t.Errorf("entry %d lsb = %d, want %d", i, lsb, wantLsb)
		}
		if i > 0 {
			prevOff, _, _, _, _ := f.Find([]rune{'a', 'b', 'c'}[i-1])
			if off < prevOff {
				t.Errorf("entry %d off %d not monotonic", i, off)
			}
		}
	}
	// 'a' is the zero-length glyph.
	off, ln, _, _, ok := f.Find('a')
	if !ok {
		t.Fatalf("Find('a') miss")
	}
	if ln != 0 {
		t.Errorf("glyph 'a' len = %d, want 0", ln)
	}
	if g := f.GlyphData(off, ln); len(g) != 0 {
		t.Errorf("glyph 'a' data len = %d, want 0", len(g))
	}
	// 'b' data round-trips.
	off, ln, _, _, _ = f.Find('b')
	if g := f.GlyphData(off, ln); !bytes.Equal(g, []byte{0xAA, 0xBB}) {
		t.Errorf("glyph 'b' data = %v", g)
	}
}

func TestVfpBadMagic(t *testing.T) {
	data, err := EncodeVFP([]VfpEntry{{Rune: 'a'}}, [][]byte{{0x01}})
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	data[0] = 'X'
	if _, err := ParseVFP(data); err == nil {
		t.Fatal("expected error for bad magic")
	}
}

func TestVfpIndexUnsorted(t *testing.T) {
	entries := []VfpEntry{{Rune: 'b', Advance: 1}, {Rune: 'a', Advance: 2}}
	data, err := EncodeVFP(entries, [][]byte{{0x01}, {0x02}})
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	// Hand-corrupt the second index rune so it becomes not strictly ascending.
	// Index entry 1 starts at byte 12+12=24, rune field at 24.
	data[24] = 0
	data[25] = 0
	data[26] = 0
	data[27] = 0 // rune = 0, which is < 'b'(0x62)
	if _, err := ParseVFP(data); err == nil {
		t.Fatal("expected error for unsorted index")
	}
}

func TestVfpBounds(t *testing.T) {
	entries := []VfpEntry{{Rune: 'a', GlyphOff: 0}, {Rune: 'b', GlyphOff: 2}}
	glyphs := [][]byte{{0x01, 0x02}, {0x03}}
	data, err := EncodeVFP(entries, glyphs)
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	f, err := ParseVFP(data)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	_ = f

	// glyphOff out of range: set first entry's glyphOff to a huge value.
	bad := bytes.Clone(data)
	bad[12+4] = 0xFF
	bad[12+5] = 0xFF
	bad[12+6] = 0xFF
	bad[12+7] = 0xFF
	if _, err := ParseVFP(bad); err == nil {
		t.Error("expected error for glyphOff out of range")
	}

	// dataSize truncated: declare more data than present.
	bad2 := bytes.Clone(data)
	bad2[11] = 0xFF // dataSize (bytes 8:12) last byte -> huge
	if _, err := ParseVFP(bad2); err == nil {
		t.Error("expected error for truncated data region")
	}

	// index truncated: count larger than available index bytes.
	bad3 := bytes.Clone(data)
	bad3[7] = 0xFF // count (bytes 4:8) last byte -> huge
	if _, err := ParseVFP(bad3); err == nil {
		t.Error("expected error for truncated index")
	}
}

func TestVfpEmpty(t *testing.T) {
	data, err := EncodeVFP(nil, nil)
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	f, err := ParseVFP(data)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	if f.Count() != 0 {
		t.Fatalf("count = %d, want 0", f.Count())
	}
	for _, r := range []rune{'a', '中', 0x10FFFF} {
		if _, _, _, _, ok := f.Find(r); ok {
			t.Errorf("Find(U+%X) should miss on empty vfp", r)
		}
	}
}

func TestVfpFind(t *testing.T) {
	entries := []VfpEntry{
		{Rune: 0x10},
		{Rune: 0x20},
		{Rune: 0x30},
		{Rune: 0x40},
	}
	data, err := EncodeVFP(entries, [][]byte{{1}, {2}, {3}, {4}})
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	f, err := ParseVFP(data)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	// first / middle / last hit.
	for _, r := range []rune{0x10, 0x30, 0x40} {
		if _, _, _, _, ok := f.Find(r); !ok {
			t.Errorf("Find(U+%X) miss", r)
		}
	}
	// out-of-range miss.
	for _, r := range []rune{0x00, 0x11, 0x41} {
		if _, _, _, _, ok := f.Find(r); ok {
			t.Errorf("Find(U+%X) should miss", r)
		}
	}
	// glyphLen of last glyph reaches dataSize.
	off, ln, _, _, _ := f.Find(0x40)
	if off+ln != uint32(len(data))-12-4*12 {
		t.Errorf("last glyph end = %d, want dataSize", off+ln)
	}
}
