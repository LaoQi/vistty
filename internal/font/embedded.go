package font

import _ "embed"

//go:embed assets/SarasaFixedSC-Regular.ttf
var embeddedFont []byte

func NewEmbeddedFace(size float64, dpi float64) (*OpenTypeFace, error) {
	return NewOpenTypeFace(embeddedFont, size, dpi)
}

// EmbeddedFontData returns the raw bytes of the embedded font. It allows
// callers (e.g. FaceCache) to share a single copy without re-reading disk.
func EmbeddedFontData() []byte {
	return embeddedFont
}
