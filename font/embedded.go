package font

import (
	"embed"
	"io/fs"
	"sync"
)

// assetsFS is the single embedded view of the assets directory. It holds the
// base fonts (Sarasa, NerdFont fallback), the LICENSE and any append-only
// font patches (NNN-name.vfp). Embedding the whole directory once avoids
// duplicating the large font files across multiple go:embed directives.
//
//go:embed assets
var assetsFS embed.FS

// Font assets are read from assetsFS on demand so there is a single copy in
// the binary (rather than one per go:embed directive). The two base fonts are
// cached so repeated callers share one copy (zero-copy after first load).
var (
	assetCache  = map[string][]byte{}
	assetCacheM sync.Mutex
)

func readAsset(name string) []byte {
	assetCacheM.Lock()
	defer assetCacheM.Unlock()
	if d, ok := assetCache[name]; ok {
		return d
	}
	data, err := fs.ReadFile(assetsFS, name)
	if err != nil {
		return nil
	}
	assetCache[name] = data
	return data
}

func NewEmbeddedFace(size float64, dpi float64) (*OpenTypeFace, error) {
	return NewOpenTypeFace(EmbeddedFontData(), size, dpi)
}

// EmbeddedFontData returns the raw bytes of the primary embedded font
// (Sarasa Fixed SC). It allows callers (e.g. FaceCache) to share a single
// copy without re-reading disk.
func EmbeddedFontData() []byte {
	return readAsset("assets/SarasaFixedSC-Regular.ttf")
}

// EmbeddedFallbackFontData returns the raw bytes of the embedded fallback
// font (NerdFont PUA subset). It allows callers to share a single copy.
func EmbeddedFallbackFontData() []byte {
	return readAsset("assets/NerdFontFallback.ttf")
}
