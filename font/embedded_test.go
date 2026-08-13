package font

import (
	"io/fs"
	"testing"
)

// L0: 用嵌入字体（Sarasa Fixed SC）验证字形位图非空非零。
// dumb buffer 路径正常即说明 font 层 OK，此测试做回归覆盖，且不依赖系统字体。
// 定位：若 GBM 字形不显示但此测试通过 → font 层排除，问题在 GPU 上传/采样。

func newEmbeddedFaceForTest(t *testing.T) *OpenTypeFace {
	t.Helper()
	face, err := NewEmbeddedFace(14, 72)
	if err != nil {
		t.Fatalf("NewEmbeddedFace: %v", err)
	}
	return face
}

func maxAlpha(b []byte) int {
	m := 0
	for _, v := range b {
		if int(v) > m {
			m = int(v)
		}
	}
	return m
}

func TestEmbeddedFaceGlyphASCIINonZero(t *testing.T) {
	face := newEmbeddedFaceForTest(t)
	defer face.Close()

	g, err := face.Glyph('A')
	if err != nil {
		t.Fatalf("Glyph('A'): %v", err)
	}
	if g == nil {
		t.Fatal("Glyph('A') returned nil")
	}
	if g.Width <= 0 || g.Height <= 0 {
		t.Fatalf("dimensions %dx%d", g.Width, g.Height)
	}
	if len(g.Bitmap) != g.Width*g.Height {
		t.Fatalf("bitmap len=%d want %d", len(g.Bitmap), g.Width*g.Height)
	}
	if ma := maxAlpha(g.Bitmap); ma == 0 {
		t.Error("Glyph('A') bitmap is all zero — 字形位图无笔画，渲染必不可能显示")
	}
}

func TestEmbeddedFaceGlyphCJKNonZero(t *testing.T) {
	face := newEmbeddedFaceForTest(t)
	defer face.Close()

	g, err := face.Glyph('中')
	if err != nil {
		t.Fatalf("Glyph('中'): %v", err)
	}
	if g == nil {
		t.Skip("embedded font has no glyph for '中'")
	}
	if g.Width <= 0 || g.Height <= 0 {
		t.Fatalf("dimensions %dx%d", g.Width, g.Height)
	}
	if len(g.Bitmap) != g.Width*g.Height {
		t.Fatalf("bitmap len=%d want %d", len(g.Bitmap), g.Width*g.Height)
	}
	if ma := maxAlpha(g.Bitmap); ma == 0 {
		t.Error("Glyph('中') bitmap is all zero")
	}
}

func TestEmbeddedFaceGlyphDimensionsConsistent(t *testing.T) {
	face := newEmbeddedFaceForTest(t)
	defer face.Close()

	for _, r := range []rune{'A', 'M', '0', '#', ' '} {
		g, err := face.Glyph(r)
		if err != nil {
			t.Errorf("Glyph(%q): %v", r, err)
			continue
		}
		if g == nil {
			continue
		}
		if len(g.Bitmap) != g.Width*g.Height {
			t.Errorf("rune %q: bitmap len=%d != %d*%d", r, len(g.Bitmap), g.Width, g.Height)
		}
	}
}

func TestEmbeddedFaceMetricsPositive(t *testing.T) {
	face := newEmbeddedFaceForTest(t)
	defer face.Close()

	m := face.Metrics()
	if m.Width <= 0 || m.Height <= 0 || m.Ascent <= 0 {
		t.Errorf("metrics invalid: %+v", m)
	}
}

// TestEmbeddedPatchesValid guards against a bad patch being committed: every
// assets/*.vfp must parse as a valid vfp file. With no patches the loop is a
// vacuous pass.
func TestEmbeddedPatchesValid(t *testing.T) {
	names, err := fs.Glob(assetsFS, "assets/*.vfp")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	for _, name := range names {
		data, err := fs.ReadFile(assetsFS, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := ParseVFP(data); err != nil {
			t.Fatalf("ParseVFP(%s): %v", name, err)
		}
	}
}

// TestBraillePatchRenders verifies the embedded Braille patch (if present) is
// served by the default chain: a Braille rune must resolve to a non-empty
// glyph whose bitmap width matches the Sarasa cell width. With no embedded
// patch the test skips.
func TestBraillePatchRenders(t *testing.T) {
	patchData, err := EmbeddedPatchFontData()
	if err != nil {
		t.Fatalf("EmbeddedPatchFontData: %v", err)
	}
	if patchData == nil {
		t.Skip("no embedded patch; Braille not covered")
	}
	cache, err := NewChainFaceCache(EmbeddedFontData(), [][]byte{patchData}, 96)
	if err != nil {
		t.Fatalf("NewChainFaceCache: %v", err)
	}
	defer cache.Close()

	face, err := cache.GetFace(16)
	if err != nil {
		t.Fatalf("GetFace(16): %v", err)
	}
	m := face.Metrics()
	for _, r := range []rune{0x2801, 0x2807, 0x28FF} {
		g, err := face.Glyph(r)
		if err != nil {
			t.Errorf("Glyph(U+%X): %v", r, err)
			continue
		}
		if g == nil {
			t.Errorf("Glyph(U+%X) resolved to nil — Braille patch not served", r)
			continue
		}
		if g.Width <= 0 || g.Height <= 0 {
			t.Errorf("U+%X dims %dx%d", r, g.Width, g.Height)
			continue
		}
		if ma := maxAlpha(g.Bitmap); ma == 0 {
			t.Errorf("U+%X bitmap all zero", r)
		}
		switch r {
		case 0x28FF:
			// The all-dots glyph fills the full cell width (fitted to 1024 units).
			if d := g.Width - m.Width; d < -1 || d > 2 {
				t.Errorf("U+%X width=%d cell=%d (unexpected slack %d)", r, g.Width, m.Width, d)
			}
		default:
			// Single/partial dot glyphs stay point-sized, inside the cell.
			if g.Width >= m.Width {
				t.Errorf("U+%X width=%d should be smaller than cell=%d", r, g.Width, m.Width)
			}
		}
	}
}
