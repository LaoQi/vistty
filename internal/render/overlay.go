package render

import (
	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
)

// GlyphProvider resolves rune bitmaps for CPU overlay rendering.
// Implemented by Compositor to share its glyph atlas cache.
type GlyphProvider interface {
	OverlayGlyph(r rune) *font.Glyph
}

// GPUGlyphUploader uploads a glyph to the GPU atlas and returns UV coords and metrics.
// Implemented by Compositor to share its GPU atlas texture.
type GPUGlyphUploader interface {
	OverlayUploadGlyph(r rune) (u0, v0, u1, v1 float32, gw, gh, xoff, yoff int, ok bool)
}

// Overlay is the compositor's extension point for persistent on-screen UI
// (tab bar, status regions) drawn in border areas around terminal content.
type Overlay interface {
	Insets() (top, bottom, left, right int)
	SetGlyphProvider(GlyphProvider)
	SetGPUGlyphUploader(GPUGlyphUploader)
	RenderCPU(buf []byte, stride, width, height int)
	RenderGPU(instances *[]platform.CellInstance, width, height int)
}
