package font

import (
	"encoding/binary"
	"testing"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// glyphIsComposite reports whether gid in data is a composite (numContours<0)
// glyf glyph.
func glyphIsComposite(data []byte, gid uint32) bool {
	headOff := tableOffset(data, "head")
	locOff := tableOffset(data, "loca")
	glyfOff := tableOffset(data, "glyf")
	indexToLoc := binary.BigEndian.Uint16(data[headOff+50 : headOff+52])
	var loc func(uint32) int
	if indexToLoc == 1 {
		loc = func(i uint32) int {
			return int(binary.BigEndian.Uint32(data[locOff+4*i : locOff+4*i+4]))
		}
	} else {
		loc = func(i uint32) int {
			return int(binary.BigEndian.Uint16(data[locOff+2*i : locOff+2*i+2])) * 2
		}
	}
	o, e := loc(gid), loc(gid+1)
	if o == e {
		return false
	}
	return int16(binary.BigEndian.Uint16(data[int(glyfOff)+o : int(glyfOff)+o+2])) < 0
}

func tableOffset(data []byte, tag string) uint32 {
	n := binary.BigEndian.Uint16(data[4:6])
	for i := 0; i < int(n); i++ {
		rec := data[12+i*16 : 12+i*16+16]
		if string(rec[0:4]) == tag {
			return binary.BigEndian.Uint32(rec[8:12])
		}
	}
	return 0
}

func TestGenSimpleGlyph(t *testing.T) {
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), []rune{'A'})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	off, ln, adv, _, ok := f.Find('A')
	if !ok {
		t.Fatal("Find('A') miss")
	}
	if adv == 0 {
		t.Error("advance = 0, want > 0")
	}
	if ln == 0 {
		t.Fatal("glyf empty for 'A'")
	}
	g := f.GlyphData(off, ln)
	if nc := int16(binary.BigEndian.Uint16(g)); nc <= 0 {
		t.Errorf("numContours = %d, want > 0", nc)
	}
}

func TestGenCompositeGlyph(t *testing.T) {
	f, err := sfnt.Parse(EmbeddedFontData())
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	b := &sfnt.Buffer{}
	var runeWithComposite rune
	for r := 0x20; r <= 0x2FF && runeWithComposite == 0; r++ {
		gi, err := f.GlyphIndex(b, rune(r))
		if err != nil || gi == 0 {
			continue
		}
		if glyphIsComposite(EmbeddedFontData(), uint32(gi)) {
			runeWithComposite = rune(r)
		}
	}
	if runeWithComposite == 0 {
		t.Skip("no composite glyph found in Sarasa")
	}

	vfp, _, _, err := GenPatch(EmbeddedFontData(), []rune{runeWithComposite})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	pf, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	off, ln, _, _, ok := pf.Find(runeWithComposite)
	if !ok {
		t.Fatalf("Find(U+%X) miss", runeWithComposite)
	}
	if ln == 0 {
		t.Fatal("glyf empty for composite-extracted rune")
	}
	g := pf.GlyphData(off, ln)
	if nc := int16(binary.BigEndian.Uint16(g)); nc <= 0 {
		t.Errorf("numContours = %d, want > 0 (composite must be expanded to simple)", nc)
	}
}

func TestGenMissingRune(t *testing.T) {
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), []rune{0x3040})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	if len(missing) != 1 || missing[0] != 0x3040 {
		t.Errorf("missing = %v, want [U+3040]", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	if _, _, _, _, ok := f.Find(0x3040); ok {
		t.Error("missing rune should not be in vfp")
	}
}

func TestGenSpaceGlyph(t *testing.T) {
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), []rune{' '})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	off, ln, adv, _, ok := f.Find(' ')
	if !ok {
		t.Fatal("Find(' ') miss")
	}
	if adv == 0 {
		t.Error("space advance = 0, want > 0")
	}
	if ln != 0 {
		t.Errorf("space glyf len = %d, want 0", ln)
	}
	if g := f.GlyphData(off, ln); len(g) != 0 {
		t.Errorf("space glyf data len = %d, want 0", len(g))
	}
}

func TestGenUpemNormalize(t *testing.T) {
	x := normalizeCoord(fixed.I(500), 1000, 2048)
	if x != 1024 {
		t.Errorf("normalizeCoord(500,1000,2048) = %d, want 1024", x)
	}
	y := normalizeCoord(fixed.I(500), 1000, 2048)
	if y != 1024 {
		t.Errorf("normalizeCoord(500,1000,2048) = %d, want 1024", y)
	}
	// Rounding: 1000 upem -> 2048, a 700-unit coord.
	if v := normalizeCoord(fixed.I(700), 1000, 2048); v != 1434 && v != 1433 {
		t.Errorf("normalizeCoord(700,1000,2048) = %d, want ~1434", v)
	}
}

func TestGenIndexSorted(t *testing.T) {
	// Deliberately shuffled runes.
	runes := []rune{'z', 'a', 'm', 'b', 'A', 'Z'}
	vfp, _, _, err := GenPatch(EmbeddedFontData(), runes)
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	prev := rune(-1)
	for i := 0; i < f.Count(); i++ {
		off := 12 + i*12
		r := rune(binary.LittleEndian.Uint32(vfp[off : off+4]))
		if r <= prev {
			t.Fatalf("index not ascending at %d: %x <= %x", i, r, prev)
		}
		prev = r
	}
}

func TestGenRoundTrip(t *testing.T) {
	runes := []rune{'A', 'B', 'C', 0x4E2D, ' '}
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), runes)
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	for _, r := range runes {
		if _, _, _, _, ok := f.Find(r); !ok {
			t.Errorf("Find(U+%X) miss after round trip", r)
		}
	}
}

// normalizeCoord scales a fixed.Int26_6 coordinate from srcUpem font units to
// dstUpem font units, returning the nearest integer. It mirrors the scaling
// applied by sfnt.LoadGlyph (used for manual calculation and testing).
func normalizeCoord(v fixed.Int26_6, srcUpem, dstUpem int) int16 {
	scaled := v * fixed.Int26_6(dstUpem) / fixed.Int26_6(srcUpem)
	return int16(scaled.Round())
}

func TestGenPatchFitForcesAdvance(t *testing.T) {
	const fit = 2048
	vfp, missing, _, err := GenPatchFit(EmbeddedFontData(), []rune{'A'}, fit)
	if err != nil {
		t.Fatalf("GenPatchFit: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	off, ln, adv, lsb, ok := f.Find('A')
	if !ok {
		t.Fatal("Find('A') miss")
	}
	if adv != fit {
		t.Errorf("fit advance = %d, want %d", adv, fit)
	}
	if lsb != 0 {
		t.Errorf("fit lsb = %d, want 0 (outline should start at x=0)", lsb)
	}
	if ln == 0 {
		t.Fatal("fit glyph has no outline")
	}
	g := f.GlyphData(off, ln)
	xMin := int16(binary.BigEndian.Uint16(g[2:4]))
	xMax := int16(binary.BigEndian.Uint16(g[6:8]))
	if xMin != 0 {
		t.Errorf("fit xMin = %d, want 0", xMin)
	}
	if xMax != fit {
		t.Errorf("fit xMax = %d, want %d (outline fills [0,fit])", xMax, fit)
	}
}

func TestGenPatchFitEmptyOutline(t *testing.T) {
	// Space (empty outline) must not crash and must still get the forced advance.
	vfp, missing, _, err := GenPatchFit(EmbeddedFontData(), []rune{' '}, 1024)
	if err != nil {
		t.Fatalf("GenPatchFit(space): %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v", missing)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	_, _, adv, _, ok := f.Find(' ')
	if !ok {
		t.Fatal("Find(' ') miss")
	}
	if adv != 1024 {
		t.Errorf("space advance = %d, want 1024", adv)
	}
}
