package font

import (
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
