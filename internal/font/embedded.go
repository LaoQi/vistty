package font

import _ "embed"

//go:embed assets/SarasaFixedSC-Regular.ttf
var embeddedFont []byte

func NewEmbeddedFace(size float64, dpi float64) (*OpenTypeFace, error) {
	return NewOpenTypeFace(embeddedFont, size, dpi)
}
