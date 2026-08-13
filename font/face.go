package font

import (
	"image"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type Face interface {
	Metrics() Metrics
	Glyph(r rune) (*Glyph, error)
	Close() error
}

type OpenTypeFace struct {
	face    font.Face
	metrics Metrics
}

func NewOpenTypeFace(fontData []byte, size float64, dpi float64) (*OpenTypeFace, error) {
	parsed, err := opentype.Parse(fontData)
	if err != nil {
		return nil, err
	}
	return newFaceFromParsed(parsed, size, dpi)
}

func newFaceFromParsed(parsed *opentype.Font, size float64, dpi float64) (*OpenTypeFace, error) {
	f, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: size,
		DPI:  dpi,
	})
	if err != nil {
		return nil, err
	}

	m := f.Metrics()
	adv, _ := f.GlyphAdvance('M')
	charWidth := adv.Ceil()
	if charWidth <= 0 {
		charWidth = int(size * dpi / 72)
	}

	return &OpenTypeFace{
		face: f,
		metrics: Metrics{
			Height:  m.Height.Ceil(),
			Width:   charWidth,
			Ascent:  m.Ascent.Ceil(),
			Descent: m.Descent.Ceil(),
			XHeight: m.XHeight.Ceil(),
		},
	}, nil
}

func NewOpenTypeFaceFromFile(path string, size float64, dpi float64) (*OpenTypeFace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return NewOpenTypeFace(data, size, dpi)
}

func (f *OpenTypeFace) Metrics() Metrics {
	return f.metrics
}

func (f *OpenTypeFace) Glyph(r rune) (*Glyph, error) {
	if isCsdRune(r) {
		if g := synthCsdGlyph(r, f.metrics); g != nil {
			return g, nil
		}
	}
	adv, ok := f.face.GlyphAdvance(r)
	if !ok {
		if g := synthBlockElement(r, f.metrics); g != nil {
			return g, nil
		}
		return nil, nil
	}

	dr, mask, maskp, _, ok := f.face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		if g := synthBlockElement(r, f.metrics); g != nil {
			return g, nil
		}
		return nil, nil
	}

	bitmap := extractAlpha(mask, maskp, dr.Dx(), dr.Dy())

	return &Glyph{
		Rune:    r,
		Bitmap:  bitmap,
		Width:   dr.Dx(),
		Height:  dr.Dy(),
		XOffset: dr.Min.X,
		YOffset: dr.Min.Y,
		Advance: adv.Ceil(),
	}, nil
}

func (f *OpenTypeFace) Close() error {
	f.face.Close()
	return nil
}

// glyphFace is the subset of OpenTypeFace needed by ChainFace. It is an
// interface so tests can inject counting/stub faces, but in production it is
// always satisfied by *OpenTypeFace.
type glyphFace interface {
	Metrics() Metrics
	Glyph(r rune) (*Glyph, error)
	Close() error
}

// ChainFace composes an ordered chain of faces (primary, then any number of
// fallbacks such as font patches and the Nerd fallback). It implements Face so
// it can be used wherever a single Face is expected. Glyph lookup walks the
// chain in order and returns the first hit; glyphs from level i>0 have their
// YOffset adjusted by (levelAscent - primaryAscent) so they align to the
// primary baseline.
type ChainFace struct {
	faces   []glyphFace // ordered: primary, then fallbacks (may be 1..N)
	metrics Metrics     // primary's metrics
	ascents []int       // per-level ascent, cached to avoid hot-path queries
}

// NewChainFace returns a ChainFace serving primary, with fallbacks consulted
// in order for glyphs missing from earlier levels. Nil fallbacks are skipped.
// With no fallbacks the result behaves as primary-only.
func NewChainFace(primary *OpenTypeFace, fallbacks ...*OpenTypeFace) *ChainFace {
	faces := make([]glyphFace, 0, 1+len(fallbacks))
	faces = append(faces, primary)
	for _, f := range fallbacks {
		if f != nil {
			faces = append(faces, f)
		}
	}
	ascents := make([]int, len(faces))
	for i, f := range faces {
		ascents[i] = f.Metrics().Ascent
	}
	return &ChainFace{
		faces:   faces,
		metrics: primary.Metrics(),
		ascents: ascents,
	}
}

// NewFallbackFace is a thin two-level wrapper around NewChainFace, kept for
// backward compatibility with existing call sites.
func NewFallbackFace(primary, fallback *OpenTypeFace) *ChainFace {
	return NewChainFace(primary, fallback)
}

func (f *ChainFace) Metrics() Metrics {
	return f.metrics
}

// Glyph returns the glyph for r from the first chain level that provides it.
// Glyphs from level i>0 are YOffset-shifted so they render on the primary
// baseline: compositor draws gy = py + primaryAscent + glyph.YOffset, so the
// original level YOffset (relative to that level's baseline) is offset by
// (levelAscent - primaryAscent).
func (f *ChainFace) Glyph(r rune) (*Glyph, error) {
	base := f.ascents[0]
	for i, face := range f.faces {
		g, err := face.Glyph(r)
		if err != nil || g == nil {
			continue
		}
		if i > 0 {
			g.YOffset += f.ascents[i] - base
		}
		return g, nil
	}
	return nil, nil
}

func (f *ChainFace) Close() error {
	var err error
	for _, face := range f.faces {
		if face != nil {
			if e := face.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	return err
}

func extractAlpha(mask image.Image, offset image.Point, w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}

	buf := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := mask.At(offset.X+x, offset.Y+y).RGBA()
			buf[y*w+x] = byte(a >> 8)
		}
	}
	return buf
}

// synthBlockElement 为字体中缺失的 Block Elements (U+2580-U+259F) 合成 alpha 字形。
// 终端块字符是几何形状，硬边渲染符合预期。坐标基于 cell：渲染时 gy = py + Ascent + YOffset，
// 故 YOffset = -Ascent 使 bitmap 起始于 cell 顶部 py。
func synthBlockElement(r rune, m Metrics) *Glyph {
	w, h := m.Width, m.Height
	if w <= 0 || h <= 0 {
		return nil
	}
	ascent := m.Ascent

	full := func(bw, bh int) []byte {
		b := make([]byte, bw*bh)
		for i := range b {
			b[i] = 255
		}
		return b
	}

	var bmp []byte
	var bw, bh, xoff, yoff int

	switch {
	case r == 0x2580: // ▀ upper half
		bw, bh = w, h/2
		bmp = full(bw, bh)
		yoff = -ascent
	case r == 0x2588: // █ full
		bw, bh = w, h
		bmp = full(bw, bh)
		yoff = -ascent
	case r >= 0x2581 && r <= 0x2587: // ▁▂▃▄▅▆▇ lower N/8
		n := int(r - 0x2580)
		bh = h * n / 8
		bw = w
		bmp = full(bw, bh)
		yoff = -ascent + (h - bh)
	case r >= 0x2589 && r <= 0x258F: // ▉▊▋▌▍▎▏ left N/8
		n := 8 - int(r-0x2588)
		bw = w * n / 8
		bh = h
		bmp = full(bw, bh)
		yoff = -ascent
	case r == 0x2590: // ▐ right half
		bw, bh = w/2, h
		bmp = full(bw, bh)
		xoff = w - bw
		yoff = -ascent
	case r == 0x2594: // ▔ upper one eighth
		bw, bh = w, h/8
		bmp = full(bw, bh)
		yoff = -ascent
	case r == 0x2595: // ▕ right one eighth
		bw, bh = w/8, h
		bmp = full(bw, bh)
		xoff = w - bw
		yoff = -ascent
	case r >= 0x2591 && r <= 0x2593: // ░▒▓ shades
		bw, bh = w, h
		bmp = make([]byte, bw*bh)
		for y := 0; y < bh; y++ {
			for x := 0; x < bw; x++ {
				on := false
				switch r {
				case 0x2591:
					on = x%2 == 0 && y%2 == 0
				case 0x2592:
					on = (x+y)%2 == 0
				case 0x2593:
					on = !(x%2 == 1 && y%2 == 1)
				}
				if on {
					bmp[y*bw+x] = 255
				}
			}
		}
		yoff = -ascent
	default:
		return nil
	}

	return &Glyph{
		Rune:    r,
		Bitmap:  bmp,
		Width:   bw,
		Height:  bh,
		XOffset: xoff,
		YOffset: yoff,
		Advance: w,
	}
}
