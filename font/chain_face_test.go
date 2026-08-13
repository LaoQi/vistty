package font

import (
	"bytes"
	"encoding/binary"
	"testing"
	"testing/fstest"
)

// patchRune is a sentinel rune absent from both the Sarasa primary and the
// NerdFont PUA subset (U+3042 is Hiragana 'あ'), so a chain resolves it only
// via an injected patch level.
const patchRune = 0x3042

// buildSquareGlyf returns a simple TrueType glyf glyph: a single square
// contour from (0,0) to (100,100) in Y-up coordinates.
func buildSquareGlyf() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, int16(1))    // numContours
	binary.Write(&buf, binary.BigEndian, int16(0))     // xMin
	binary.Write(&buf, binary.BigEndian, int16(0))     // yMin
	binary.Write(&buf, binary.BigEndian, int16(100))   // xMax
	binary.Write(&buf, binary.BigEndian, int16(100))   // yMax
	binary.Write(&buf, binary.BigEndian, uint16(3))    // endPtsOfContours[0]
	binary.Write(&buf, binary.BigEndian, uint16(0))    // instructionLength
	buf.Write([]byte{0x01, 0x01, 0x01, 0x01})          // flags: on-curve
	for _, d := range []int16{0, 100, 0, -100} {       // x deltas
		binary.Write(&buf, binary.BigEndian, d)
	}
	for _, d := range []int16{0, 0, 100, 0} { // y deltas
		binary.Write(&buf, binary.BigEndian, d)
	}
	return buf.Bytes()
}

// buildPatchTTF assembles a tiny standalone TTF containing patchRune with the
// given advance, usable as an extra font in a chain/cache.
func buildPatchTTF(t *testing.T, advance uint16) []byte {
	t.Helper()
	g := assembledGlyph{rune: patchRune, glyf: buildSquareGlyf(), advance: advance, lsb: 0}
	data, err := AssembleFont([]assembledGlyph{g})
	if err != nil {
		t.Fatalf("AssembleFont: %v", err)
	}
	return data
}

// newChainTestFace builds a 3-level chain: Sarasa primary, a synthetic patch
// holding patchRune, and the NerdFont fallback. It returns the chain plus the
// standalone patch and nerd faces for baseline comparisons.
func newChainTestFace(t *testing.T, size, dpi float64) (*ChainFace, *OpenTypeFace, *OpenTypeFace, *OpenTypeFace) {
	t.Helper()
	primary, err := NewEmbeddedFace(size, dpi)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	patch, err := NewOpenTypeFace(buildPatchTTF(t, 999), size, dpi)
	if err != nil {
		primary.Close()
		t.Fatalf("NewOpenTypeFace patch: %v", err)
	}
	nerd, err := NewOpenTypeFace(EmbeddedFallbackFontData(), size, dpi)
	if err != nil {
		primary.Close()
		patch.Close()
		t.Fatalf("NewOpenTypeFace nerd: %v", err)
	}
	return NewChainFace(primary, patch, nerd), primary, patch, nerd
}

func TestChainPrimaryHit(t *testing.T) {
	f, primary, _, _ := newChainTestFace(t, 14, 96)
	defer f.Close()

	want, err := primary.Glyph('中')
	if err != nil || want == nil {
		t.Fatalf("primary.Glyph('中') = (%+v, %v)", want, err)
	}
	g, err := f.Glyph('中')
	if err != nil {
		t.Fatalf("chain.Glyph('中') err: %v", err)
	}
	if g == nil {
		t.Fatal("expected primary glyph for '中' through chain, got nil")
	}
	// Level-0 hit must not be YOffset-shifted, so it matches the single face.
	if g.YOffset != want.YOffset || g.Advance != want.Advance ||
		!bytes.Equal(g.Bitmap, want.Bitmap) {
		t.Fatalf("chain primary glyph != single face glyph:\nchain %+v\nsingle %+v", g, want)
	}
}

func TestChainPatchHit(t *testing.T) {
	f, _, patch, _ := newChainTestFace(t, 14, 96)
	defer f.Close()

	// patchRune is absent from primary and nerd, so only the patch resolves it.
	g, err := f.Glyph(patchRune)
	if err != nil {
		t.Fatalf("chain.Glyph(patchRune) err: %v", err)
	}
	if g == nil {
		t.Fatal("expected patch glyph for patchRune through chain, got nil")
	}
	if g.Rune != patchRune {
		t.Fatalf("glyph rune = U+%04X, want U+%04X", g.Rune, patchRune)
	}
	// The advance must match the standalone patch face (sentinel 999 at the
	// same size), proving the glyph came from the patch level, not elsewhere.
	patchG, err := patch.Glyph(patchRune)
	if err != nil || patchG == nil {
		t.Fatalf("patch.Glyph(patchRune) = (%+v, %v)", patchG, err)
	}
	if g.Advance != patchG.Advance {
		t.Fatalf("patch glyph advance = %d, want %d (from patch face)", g.Advance, patchG.Advance)
	}
}

func TestChainNerdHit(t *testing.T) {
	f, _, _, _ := newChainTestFace(t, 14, 96)
	defer f.Close()

	// U+E0B0 (Powerline) is in nerd but not Sarasa or the patch.
	g, err := f.Glyph(0xE0B0)
	if err != nil {
		t.Fatalf("chain.Glyph(U+E0B0) err: %v", err)
	}
	if g == nil {
		t.Fatal("expected nerd glyph for U+E0B0 through chain, got nil")
	}
	if g.Rune != 0xE0B0 {
		t.Fatalf("glyph rune = U+%04X, want U+E0B0", g.Rune)
	}
}

func TestChainMiss(t *testing.T) {
	f, _, _, _ := newChainTestFace(t, 14, 96)
	defer f.Close()

	// U+10FFFF is a reserved/private plane codepoint absent everywhere.
	g, err := f.Glyph(0x10FFFF)
	if err != nil {
		t.Fatalf("chain.Glyph(0x10FFFF) err = %v, want nil", err)
	}
	if g != nil {
		t.Fatalf("expected nil glyph for 0x10FFFF, got %+v", g)
	}
}

func TestChainBaselineAlign(t *testing.T) {
	f, primary, patch, nerd := newChainTestFace(t, 14, 96)
	defer f.Close()

	primaryAscent := primary.Metrics().Ascent

	// Level 2 (patch) hit.
	origPatch, err := patch.Glyph(patchRune)
	if err != nil || origPatch == nil {
		t.Fatalf("patch.Glyph(patchRune) = (%+v, %v)", origPatch, err)
	}
	g, err := f.Glyph(patchRune)
	if err != nil || g == nil {
		t.Fatalf("chain.Glyph(patchRune) = (%+v, %v)", g, err)
	}
	patchAscent := patch.Metrics().Ascent
	if want := origPatch.YOffset + (patchAscent - primaryAscent); g.YOffset != want {
		t.Fatalf("patch YOffset = %d, want %d (orig %d + %d - %d)",
			g.YOffset, want, origPatch.YOffset, patchAscent, primaryAscent)
	}

	// Level 3 (nerd) hit.
	origNerd, err := nerd.Glyph(0xE0B0)
	if err != nil || origNerd == nil {
		t.Fatalf("nerd.Glyph(U+E0B0) = (%+v, %v)", origNerd, err)
	}
	gn, err := f.Glyph(0xE0B0)
	if err != nil || gn == nil {
		t.Fatalf("chain.Glyph(U+E0B0) = (%+v, %v)", gn, err)
	}
	nerdAscent := nerd.Metrics().Ascent
	if want := origNerd.YOffset + (nerdAscent - primaryAscent); gn.YOffset != want {
		t.Fatalf("nerd YOffset = %d, want %d (orig %d + %d - %d)",
			gn.YOffset, want, origNerd.YOffset, nerdAscent, primaryAscent)
	}
}

// TestChainGenPatchPipeline builds a patch via the real GenPatch + LoadPatches
// pipeline (extracting π U+03C0, which Sarasa lacks, from the Nerd source),
// assembles it, and verifies the chain resolves it at level 2.
func TestChainGenPatchPipeline(t *testing.T) {
	vfp, missing, _, err := GenPatch(EmbeddedFallbackFontData(), []rune{0x03C0})
	if err != nil {
		t.Fatalf("GenPatch: %v", err)
	}
	for _, m := range missing {
		if m == 0x03C0 {
			t.Skip("source lacks U+03C0; cannot build patch")
		}
	}
	merged, err := LoadPatches(fstest.MapFS{
		"assets/000-test.vfp": &fstest.MapFile{Data: vfp},
	}, "assets/*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	patchData, err := merged.FontData()
	if err != nil || len(patchData) == 0 {
		t.Fatalf("FontData = (%d bytes, %v)", len(patchData), err)
	}

	c, err := NewChainFaceCache(EmbeddedFontData(), [][]byte{patchData}, 96)
	if err != nil {
		t.Fatalf("NewChainFaceCache: %v", err)
	}
	defer c.Close()
	f, err := c.GetFace(14)
	if err != nil {
		t.Fatalf("GetFace(14): %v", err)
	}
	g, err := f.Glyph(0x03C0)
	if err != nil || g == nil {
		t.Fatalf("chain.Glyph(U+03C0) = (%+v, %v)", g, err)
	}
	if g.Rune != 0x03C0 {
		t.Fatalf("glyph rune = U+%04X, want U+03C0", g.Rune)
	}
}

func TestChainMetrics(t *testing.T) {
	f, primary, _, _ := newChainTestFace(t, 14, 96)
	defer f.Close()

	if fm, pm := f.Metrics(), primary.Metrics(); fm != pm {
		t.Fatalf("chain Metrics = %+v, want primary %+v", fm, pm)
	}
}

func TestChainClose(t *testing.T) {
	mk := func(name string, ascent int) *fakeGlyphFace {
		return &fakeGlyphFace{name: name, ascent: ascent}
	}
	f1, f2, f3 := mk("p", 10), mk("x", 20), mk("y", 30)
	chain := &ChainFace{
		faces:   []glyphFace{f1, f2, f3},
		metrics: Metrics{Ascent: 10},
		ascents: []int{10, 20, 30},
	}
	if err := chain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, f := range []*fakeGlyphFace{f1, f2, f3} {
		if f.closed != 1 {
			t.Errorf("face %s closed %d times, want 1", f.name, f.closed)
		}
	}
	// Close is idempotent with respect to not panicking; each underlying
	// opentype.Face.Close is itself idempotent.
	if err := chain.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestChainSingleLevel(t *testing.T) {
	primary, err := NewEmbeddedFace(14, 96)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	f := NewChainFace(primary)
	defer f.Close()

	if fm, pm := f.Metrics(), primary.Metrics(); fm != pm {
		t.Fatalf("single-level chain Metrics = %+v, want primary %+v", fm, pm)
	}
	want, err := primary.Glyph('中')
	if err != nil || want == nil {
		t.Fatalf("primary.Glyph('中') = (%+v, %v)", want, err)
	}
	g, err := f.Glyph('中')
	if err != nil || g == nil {
		t.Fatalf("single-level chain.Glyph('中') = (%+v, %v)", g, err)
	}
	if g.YOffset != want.YOffset || g.Advance != want.Advance ||
		!bytes.Equal(g.Bitmap, want.Bitmap) {
		t.Fatal("single-level chain not equivalent to OpenTypeFace for '中'")
	}
	// Primary miss stays a miss in the single-level chain.
	if g, _ := f.Glyph(0xE0B0); g != nil {
		t.Fatal("single-level chain should not resolve U+E0B0")
	}
}

// fakeGlyphFace is a test-only glyphFace that records how many times Close was
// invoked.
type fakeGlyphFace struct {
	name   string
	ascent int
	closed int
}

func (f *fakeGlyphFace) Metrics() Metrics              { return Metrics{Ascent: f.ascent} }
func (f *fakeGlyphFace) Glyph(r rune) (*Glyph, error)  { return nil, nil }
func (f *fakeGlyphFace) Close() error {
	f.closed++
	return nil
}

func newChainTestCache(t *testing.T, dpi float64) *ChainFaceCache {
	t.Helper()
	c, err := NewChainFaceCache(EmbeddedFontData(),
		[][]byte{buildPatchTTF(t, 999), EmbeddedFallbackFontData()}, dpi)
	if err != nil {
		t.Fatalf("NewChainFaceCache: %v", err)
	}
	return c
}

func TestChainCacheGetFace(t *testing.T) {
	c := newChainTestCache(t, 96)
	defer c.Close()

	f1, err := c.GetFace(16)
	if err != nil {
		t.Fatalf("GetFace(16): %v", err)
	}
	f2, err := c.GetFace(16)
	if err != nil {
		t.Fatalf("GetFace(16) second: %v", err)
	}
	if f1 != f2 {
		t.Fatal("GetFace should return the same cached instance for the same size")
	}
	f3, err := c.GetFace(20)
	if err != nil {
		t.Fatalf("GetFace(20): %v", err)
	}
	if f3 == f2 {
		t.Fatal("GetFace should return a new instance for a different size")
	}
	// The cached face is a full chain: primary, patch and nerd hits.
	for r, name := range map[rune]string{'中': "primary", patchRune: "patch", 0xE0B0: "nerd"} {
		if g, _ := f1.Glyph(r); g == nil {
			t.Errorf("cached face missing %s glyph U+%04X", name, r)
		}
	}
}

func TestChainCacheCloseCascade(t *testing.T) {
	c := newChainTestCache(t, 96)
	// Touch two sizes so two ChainFace instances (each with primary+2 extras)
	// are cached and must be cascaded closed.
	if _, err := c.GetFace(14); err != nil {
		t.Fatalf("GetFace(14): %v", err)
	}
	if _, err := c.GetFace(20); err != nil {
		t.Fatalf("GetFace(20): %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close must release every cached size level.
	if c.faces != nil {
		t.Fatal("cache Close did not clear the cached face map")
	}
}

func TestZoomPatchGlyph(t *testing.T) {
	c := newChainTestCache(t, 96)
	defer c.Close()

	small, err := c.GetFace(14)
	if err != nil {
		t.Fatalf("GetFace(14): %v", err)
	}
	large, err := c.GetFace(28)
	if err != nil {
		t.Fatalf("GetFace(28): %v", err)
	}
	gs, err := small.Glyph(patchRune)
	if err != nil || gs == nil {
		t.Fatalf("small patch glyph = (%+v, %v)", gs, err)
	}
	gl, err := large.Glyph(patchRune)
	if err != nil || gl == nil {
		t.Fatalf("large patch glyph = (%+v, %v)", gl, err)
	}
	// A larger size must yield a larger rendered patch glyph (zoom compatible).
	if gl.Width <= gs.Width || gl.Height <= gs.Height || gl.Advance <= gs.Advance {
		t.Fatalf("zoomed patch glyph not larger: small %+v, large %+v", gs, gl)
	}
}
