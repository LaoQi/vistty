package font

import (
	"encoding/binary"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// vfpToGlyphs extracts the glyphs of a parsed vfp stream as assembledGlyph,
// mirroring LoadPatches' extraction logic.
func vfpToGlyphs(t testing.TB, vfpData []byte) []assembledGlyph {
	t.Helper()
	f, err := ParseVFP(vfpData)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	var out []assembledGlyph
	for i := 0; i < f.Count(); i++ {
		e := f.entries[i]
		var ln uint32
		if i+1 < f.Count() {
			ln = f.entries[i+1].GlyphOff - e.GlyphOff
		} else {
			ln = uint32(len(f.data)) - e.GlyphOff
		}
		out = append(out, assembledGlyph{rune: e.Rune, glyf: f.GlyphData(e.GlyphOff, ln), advance: e.Advance, lsb: e.Lsb})
	}
	return out
}

// assembleVFP generates a patch for runes and assembles it into a TTF.
func assembleVFP(t *testing.T, runes []rune) []byte {
	t.Helper()
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), runes)
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("GenPatch missing runes: %v", missing)
	}
	ttf, err := AssembleFont(vfpToGlyphs(t, vfp))
	if err != nil {
		t.Fatalf("AssembleFont: %v", err)
	}
	return ttf
}

func TestAssembleParseable(t *testing.T) {
	runes := []rune{'A', 'B', 'C'}
	ttf := assembleVFP(t, runes)
	sf, err := sfnt.Parse(ttf)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	if got := sf.NumGlyphs(); got != len(runes)+1 {
		t.Errorf("NumGlyphs = %d, want %d", got, len(runes)+1)
	}
}

func TestAssembleCmap(t *testing.T) {
	runes := []rune{'A', '中', 'Z', ' '}
	ttf := assembleVFP(t, runes)
	sf, err := sfnt.Parse(ttf)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	b := &sfnt.Buffer{}
	for _, r := range runes {
		gi, err := sf.GlyphIndex(b, r)
		if err != nil {
			t.Fatalf("GlyphIndex(%q): %v", r, err)
		}
		if gi == 0 {
			t.Errorf("GlyphIndex(%q) = 0, want != 0", r)
		}
	}
	// A rune not in the font must map to 0.
	if gi, _ := sf.GlyphIndex(b, '\u3040'); gi != 0 {
		t.Errorf("GlyphIndex(missing) = %d, want 0", gi)
	}
}

func TestAssembleAdvance(t *testing.T) {
	runes := []rune{'A', '中', 'B'}
	ttf := assembleVFP(t, runes)
	sf, err := sfnt.Parse(ttf)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	src, err := sfnt.Parse(EmbeddedFontData())
	if err != nil {
		t.Fatalf("sfnt.Parse(Sarasa): %v", err)
	}
	b := &sfnt.Buffer{}
	for _, r := range runes {
		gi, err := sf.GlyphIndex(b, r)
		if err != nil || gi == 0 {
			t.Fatalf("GlyphIndex(%q): gi=%d err=%v", r, gi, err)
		}
		// fixed.I(targetUpem) yields ppem = 2048.0 in 26.6, so the returned
		// advance equals the normalized font-unit advance (matches vfp).
		adv, err := sf.GlyphAdvance(b, gi, fixed.I(targetUpem), font.HintingNone)
		if err != nil {
			t.Fatalf("GlyphAdvance(%q): %v", r, err)
		}
		// Expected advance from the source font at normalized upem.
		sgi, _ := src.GlyphIndex(b, r)
		sadv, _ := src.GlyphAdvance(b, sgi, fixed.I(targetUpem), font.HintingNone)
		want := sadv.Round()
		if d := int(adv.Round()) - want; d < -1 || d > 1 {
			t.Errorf("advance(%q) = %d, want ~%d", r, adv.Round(), want)
		}
	}
}

func TestAssembleRender(t *testing.T) {
	runes := []rune{'A'}
	ttf := assembleVFP(t, runes)

	// Render from the assembled font.
	face, err := NewOpenTypeFace(ttf, 32, 72)
	if err != nil {
		t.Fatalf("NewOpenTypeFace(assembled): %v", err)
	}
	g, err := face.Glyph('A')
	if err != nil {
		t.Fatalf("Glyph('A'): %v", err)
	}
	if g == nil {
		t.Fatal("Glyph('A') = nil")
	}
	if g.Width <= 0 || g.Height <= 0 {
		t.Errorf("bitmap %dx%d, want both > 0", g.Width, g.Height)
	}
	if len(g.Bitmap) > 0 && maxAlpha(g.Bitmap) == 0 {
		t.Error("bitmap all zero")
	}

	// Compare advance with Sarasa at the same size.
	sface, err := NewOpenTypeFace(EmbeddedFontData(), 32, 72)
	if err != nil {
		t.Fatalf("NewOpenTypeFace(Sarasa): %v", err)
	}
	sg, err := sface.Glyph('A')
	if err != nil || sg == nil {
		t.Fatalf("Sarasa Glyph('A'): %v", err)
	}
	if g.Advance != sg.Advance {
		t.Errorf("advance %d != Sarasa %d", g.Advance, sg.Advance)
	}
	face.Close()
	sface.Close()
}

func TestAssembleCompositeExpanded(t *testing.T) {
	sf, err := sfnt.Parse(EmbeddedFontData())
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	b := &sfnt.Buffer{}
	var compositeRune rune
	for r := 0x20; r <= 0x2FF && compositeRune == 0; r++ {
		gi, err := sf.GlyphIndex(b, rune(r))
		if err != nil || gi == 0 {
			continue
		}
		if glyphIsComposite(EmbeddedFontData(), uint32(gi)) {
			compositeRune = rune(r)
		}
	}
	if compositeRune == 0 {
		t.Skip("no composite glyph in Sarasa")
	}

	ttf := assembleVFP(t, []rune{compositeRune})
	face, err := NewOpenTypeFace(ttf, 32, 72)
	if err != nil {
		t.Fatalf("NewOpenTypeFace: %v", err)
	}
	g, err := face.Glyph(compositeRune)
	if err != nil {
		t.Fatalf("Glyph(U+%X): %v", compositeRune, err)
	}
	if g == nil || len(g.Bitmap) == 0 {
		t.Errorf("composite-expanded rune U+%X rendered empty bitmap", compositeRune)
	}
	face.Close()
}

func TestAssembleNonBMP(t *testing.T) {
	// Neither embedded font contains a >0xFFFF rune, so synthesize one by
	// reusing the 'A' outline mapped to a non-BMP code point. This exercises
	// the format 12 cmap path (single format 12 subtable).
	vfp, _, _, err := GenPatch(EmbeddedFontData(), []rune{'A'})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	f, err := ParseVFP(vfp)
	if err != nil {
		t.Fatalf("ParseVFP: %v", err)
	}
	off, ln, adv, lsb, _ := f.Find('A')
	const nonBMP = 0x1F600
	ttf, err := AssembleFont([]assembledGlyph{
		{rune: nonBMP, glyf: f.GlyphData(off, ln), advance: adv, lsb: lsb},
	})
	if err != nil {
		t.Fatalf("AssembleFont: %v", err)
	}
	sf, err := sfnt.Parse(ttf)
	if err != nil {
		t.Fatalf("sfnt.Parse: %v", err)
	}
	b := &sfnt.Buffer{}
	gi, err := sf.GlyphIndex(b, nonBMP)
	if err != nil {
		t.Fatalf("GlyphIndex(nonBMP): %v", err)
	}
	if gi == 0 {
		t.Error("non-BMP rune should be reachable via format 12 cmap")
	}
}

func TestAssembleEmptyGlyph(t *testing.T) {
	ttf := assembleVFP(t, []rune{' '})
	face, err := NewOpenTypeFace(ttf, 32, 72)
	if err != nil {
		t.Fatalf("NewOpenTypeFace: %v", err)
	}
	g, err := face.Glyph(' ')
	if err != nil {
		t.Fatalf("Glyph(' '): %v", err)
	}
	if g == nil {
		t.Fatal("Glyph(' ') = nil")
	}
	if len(g.Bitmap) != 0 {
		t.Errorf("space bitmap len = %d, want 0", len(g.Bitmap))
	}
	if g.Advance <= 0 {
		t.Errorf("space advance = %d, want > 0", g.Advance)
	}
	face.Close()
}

func TestAssembleChecksum(t *testing.T) {
	ttf := assembleVFP(t, []rune{'A', 'B', 'C', '中'})
	var sum uint32
	for i := 0; i+4 <= len(ttf); i += 4 {
		sum += binary.BigEndian.Uint32(ttf[i : i+4])
	}
	if r := len(ttf) % 4; r != 0 {
		var pad [4]byte
		copy(pad[:], ttf[len(ttf)-r:])
		sum += binary.BigEndian.Uint32(pad[:])
	}
	if sum != 0xB1B0AFBA {
		t.Errorf("whole-file checksum sum = %#08x, want 0xB1B0AFBA", sum)
	}
}

func TestAssembleMultiPatch(t *testing.T) {
	// Build three separate patches then merge via LoadPatches.
	vfpA, _, _, err := GenPatch(EmbeddedFontData(), []rune{'A', 'B'})
	if err != nil {
		t.Fatalf("GenPatch A: %v", err)
	}
	vfpB, _, _, err := GenPatch(EmbeddedFontData(), []rune{'C', '中'})
	if err != nil {
		t.Fatalf("GenPatch B: %v", err)
	}
	vfpC, _, _, err := GenPatch(EmbeddedFontData(), []rune{'D', 'E'})
	if err != nil {
		t.Fatalf("GenPatch C: %v", err)
	}
	fsys := fstest.MapFS{
		"000-a.vfp": {Data: vfpA},
		"001-b.vfp": {Data: vfpB},
		"002-c.vfp": {Data: vfpC},
	}
	m, err := LoadPatches(fsys, "*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	ttf, err := m.FontData()
	if err != nil {
		t.Fatalf("FontData: %v", err)
	}
	face, err := NewOpenTypeFace(ttf, 32, 72)
	if err != nil {
		t.Fatalf("NewOpenTypeFace: %v", err)
	}
	for _, r := range []rune{'A', 'B', 'C', '中', 'D', 'E'} {
		g, err := face.Glyph(r)
		if err != nil {
			t.Fatalf("Glyph(%q): %v", r, err)
		}
		if g == nil {
			t.Errorf("Glyph(%q) = nil after multi-patch merge", r)
		}
	}
	face.Close()
}

func TestEmbeddedPatchEmpty(t *testing.T) {
	// When assets/ has no .vfp the embedded patch font must be nil with no
	// error; when patches exist it must be non-nil and parseable. Either way
	// the call must not error.
	data, err := EmbeddedPatchFontData()
	if err != nil {
		t.Fatalf("EmbeddedPatchFontData error: %v", err)
	}
	if data != nil {
		if _, err := sfnt.Parse(data); err != nil {
			t.Errorf("EmbeddedPatchFontData() returned %d bytes but sfnt.Parse failed: %v", len(data), err)
		}
	}
}

// TestAssembleMetricsMatchPrimary guards against the patchAscent/patchDescent
// constants drifting from the embedded primary font's actual hhea values
// (normalized to targetUpem). A mismatch would silently shift all patch
// glyphs vertically via ChainFace's baseline alignment formula.
func TestAssembleMetricsMatchPrimary(t *testing.T) {
	sf, err := sfnt.Parse(EmbeddedFontData())
	if err != nil {
		t.Fatalf("sfnt.Parse(EmbeddedFontData): %v", err)
	}
	b := &sfnt.Buffer{}
	ppem := fixed.Int26_6(targetUpem) << 6
	m, err := sf.Metrics(b, ppem, 0)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	wantAscent := m.Ascent.Round()
	wantDescent := m.Descent.Round()
	if wantAscent != patchAscent {
		t.Errorf("primary ascent (normalized to %d upem) = %d, patchAscent constant = %d; update patchAscent in assemble.go", targetUpem, wantAscent, patchAscent)
	}
	if wantDescent != -patchDescent {
		t.Errorf("primary descent (normalized to %d upem) = %d, patchDescent constant = %d; update patchDescent in assemble.go", targetUpem, wantDescent, patchDescent)
	}
}

func BenchmarkAssemble(b *testing.B) {
	// A realistic patch set: a few hundred runes.
	var runes []rune
	for r := 0x2500; r <= 0x257F; r++ {
		runes = append(runes, rune(r))
	}
	vfp, missing, _, err := GenPatch(EmbeddedFontData(), runes)
	if err != nil {
		b.Fatalf("GenPatch: %v", err)
	}
	_ = missing
	glyphs := vfpToGlyphs(b, vfp)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := AssembleFont(glyphs); err != nil {
			b.Fatalf("AssembleFont: %v", err)
		}
	}
}
