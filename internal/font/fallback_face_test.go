package font

import (
	"testing"
)

// newFallbackTestFace builds a FallbackFace pairing the embedded Sarasa
// primary with the embedded NerdFont fallback at a fixed size/dpi.
func newFallbackTestFace(t *testing.T) (*FallbackFace, *OpenTypeFace, *OpenTypeFace) {
	t.Helper()
	primary, err := NewEmbeddedFace(14, 96)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	fallback, err := NewOpenTypeFace(EmbeddedFallbackFontData(), 14, 96)
	if err != nil {
		primary.Close()
		t.Fatalf("NewOpenTypeFace fallback: %v", err)
	}
	return NewFallbackFace(primary, fallback), primary, fallback
}

func TestFallbackFacePrimaryHit(t *testing.T) {
	f, primary, fallback := newFallbackTestFace(t)
	defer f.Close()
	_ = primary
	_ = fallback

	g, err := f.Glyph('o')
	if err != nil {
		t.Fatalf("Glyph('o') err: %v", err)
	}
	if g == nil {
		t.Fatal("expected glyph for 'o' from primary, got nil")
	}
	if g.Rune != 'o' {
		t.Fatalf("glyph rune = %q, want 'o'", g.Rune)
	}
}

func TestFallbackFaceFallbackHit(t *testing.T) {
	f, _, _ := newFallbackTestFace(t)
	defer f.Close()

	// U+E0B0 is a Powerline separator present in NerdFont but not Sarasa.
	g, err := f.Glyph(0xE0B0)
	if err != nil {
		t.Fatalf("Glyph(U+E0B0) err: %v", err)
	}
	if g == nil {
		t.Fatal("expected glyph for U+E0B0 from fallback, got nil")
	}
	if g.Rune != 0xE0B0 {
		t.Fatalf("glyph rune = U+%04X, want U+E0B0", g.Rune)
	}
}

func TestFallbackFaceBothMiss(t *testing.T) {
	f, _, _ := newFallbackTestFace(t)
	defer f.Close()

	// U+1F000 (Mahjong tile) is absent from both the Sarasa subset and the
	// NerdFont PUA subset.
	g, err := f.Glyph(0x1F000)
	if err != nil {
		t.Fatalf("Glyph(U+1F000) err: %v", err)
	}
	if g != nil {
		t.Fatalf("expected nil glyph for U+1F000, got %+v", g)
	}
}

func TestFallbackFaceNilFallback(t *testing.T) {
	primary, err := NewEmbeddedFace(14, 96)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	f := NewFallbackFace(primary, nil)
	defer f.Close()

	if g, _ := f.Glyph('o'); g == nil {
		t.Fatal("expected glyph for 'o' from primary with nil fallback")
	}
	// With no fallback a primary miss must yield nil, not an error.
	if g, err := f.Glyph(0xE0B0); g != nil || err != nil {
		t.Fatalf("Glyph(U+E0B0) with nil fallback = (%+v, %v), want (nil, nil)", g, err)
	}
}

func TestFallbackFaceMetricsMatchesPrimary(t *testing.T) {
	f, primary, _ := newFallbackTestFace(t)
	defer f.Close()

	pm := primary.Metrics()
	fm := f.Metrics()
	if fm != pm {
		t.Fatalf("FallbackFace Metrics = %+v, want primary %+v", fm, pm)
	}
}

func TestFallbackFaceBaselineAlignment(t *testing.T) {
	f, primary, fallback := newFallbackTestFace(t)
	defer f.Close()

	const r = 0xE0B0

	// Original glyph straight from the fallback face (relative to the
	// fallback baseline).
	orig, err := fallback.Glyph(r)
	if err != nil {
		t.Fatalf("fallback.Glyph err: %v", err)
	}
	if orig == nil {
		t.Fatal("fallback lacks U+E0B0; cannot verify baseline alignment")
	}

	// Glyph routed through FallbackFace must be baseline-aligned to primary.
	g, err := f.Glyph(r)
	if err != nil {
		t.Fatalf("FallbackFace.Glyph err: %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil glyph through FallbackFace")
	}

	primaryAscent := primary.Metrics().Ascent
	fallbackAscent := fallback.Metrics().Ascent
	want := orig.YOffset + (fallbackAscent - primaryAscent)
	if g.YOffset != want {
		t.Fatalf("YOffset = %d, want %d (orig %d + fbAsc %d - primAsc %d)",
			g.YOffset, want, orig.YOffset, fallbackAscent, primaryAscent)
	}
}

func TestFallbackFaceCacheGetFace(t *testing.T) {
	cache, err := NewFallbackFaceCache(EmbeddedFontData(), EmbeddedFallbackFontData(), 96)
	if err != nil {
		t.Fatalf("NewFallbackFaceCache: %v", err)
	}
	defer cache.Close()

	f1, err := cache.GetFace(16)
	if err != nil {
		t.Fatalf("GetFace(16): %v", err)
	}
	f2, err := cache.GetFace(16)
	if err != nil {
		t.Fatalf("GetFace(16) second: %v", err)
	}
	if f1 != f2 {
		t.Fatal("GetFace should return the same cached instance for the same size")
	}

	// Different size yields a different instance.
	f3, err := cache.GetFace(20)
	if err != nil {
		t.Fatalf("GetFace(20): %v", err)
	}
	if f3 == f2 {
		t.Fatal("GetFace should return a new instance for a different size")
	}

	// Cached face behaves as a FallbackFace: primary hit + fallback hit.
	if g, _ := f1.Glyph('o'); g == nil {
		t.Error("cached face missing primary glyph 'o'")
	}
	if g, _ := f1.Glyph(0xE0B0); g == nil {
		t.Error("cached face missing fallback glyph U+E0B0")
	}
}

func TestFallbackFaceCacheNilFallback(t *testing.T) {
	cache, err := NewFallbackFaceCache(EmbeddedFontData(), nil, 96)
	if err != nil {
		t.Fatalf("NewFallbackFaceCache: %v", err)
	}
	defer cache.Close()

	f, err := cache.GetFace(14)
	if err != nil {
		t.Fatalf("GetFace(14): %v", err)
	}
	if g, _ := f.Glyph('o'); g == nil {
		t.Error("primary-only cache missing 'o'")
	}
	if g, _ := f.Glyph(0xE0B0); g != nil {
		t.Error("primary-only cache should not resolve U+E0B0")
	}
}

// Verify *FaceCache satisfies FaceCacheProvider via GetFace/Close.
func TestFaceCacheImplementsProvider(t *testing.T) {
	var p FaceCacheProvider = &FaceCache{}
	_ = p
}

// Verify *FallbackFaceCache satisfies FaceCacheProvider.
func TestFallbackFaceCacheImplementsProvider(t *testing.T) {
	cache, err := NewFallbackFaceCache(EmbeddedFontData(), EmbeddedFallbackFontData(), 96)
	if err != nil {
		t.Fatalf("NewFallbackFaceCache: %v", err)
	}
	defer cache.Close()
	var p FaceCacheProvider = cache
	_ = p
}
