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
	adv, ok := f.face.GlyphAdvance(r)
	if !ok {
		return nil, nil
	}

	dr, mask, maskp, _, ok := f.face.Glyph(fixed.Point26_6{}, r)
	if !ok {
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
