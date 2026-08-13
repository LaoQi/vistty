package font

import (
	"sync"

	"golang.org/x/image/font/opentype"
)

// FaceCache caches the parsed font and OpenTypeFace instances by size.
// Parsing a font is expensive (especially for large CJK subsets), so the
// parsed *opentype.Font is created once and shared. Each requested size
// produces a lazily-cached face; subsequent requests for the same size
// return the cached instance with no parsing or NewFace overhead.
//
// Cached faces are owned by the cache: callers borrow references and must
// not Close them individually. Release all faces via Close at shutdown.
type FaceCache struct {
	mu     sync.Mutex
	parsed *opentype.Font
	dpi    float64
	faces  map[float64]*OpenTypeFace
}

// NewFaceCache parses fontData once and returns a cache ready to serve
// faces at arbitrary sizes.
func NewFaceCache(fontData []byte, dpi float64) (*FaceCache, error) {
	parsed, err := opentype.Parse(fontData)
	if err != nil {
		return nil, err
	}
	return &FaceCache{
		parsed: parsed,
		dpi:    dpi,
		faces:  make(map[float64]*OpenTypeFace),
	}, nil
}

// Get returns a cached face for the given size, creating one on first
// request. Repeated calls for the same size return the same instance.
func (c *FaceCache) Get(size float64) (*OpenTypeFace, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if f, ok := c.faces[size]; ok {
		return f, nil
	}
	f, err := newFaceFromParsed(c.parsed, size, c.dpi)
	if err != nil {
		return nil, err
	}
	c.faces[size] = f
	return f, nil
}

// Close releases all cached faces. After Close the cache must not be used.
func (c *FaceCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, f := range c.faces {
		f.Close()
	}
	c.faces = nil
	return nil
}

// GetFace returns a Face (interface) for the given size, adapting *FaceCache
// to satisfy FaceCacheProvider. It is equivalent to Get but returns the Face
// interface so callers using FaceCacheProvider are backend-agnostic.
func (c *FaceCache) GetFace(size float64) (Face, error) {
	f, err := c.Get(size)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// FaceCacheProvider abstracts font caches that serve Face instances by size.
// It is implemented by both FaceCache (single font) and ChainFaceCache
// (primary + ordered fallbacks). Consumers such as the compositor depend on
// this interface rather than a concrete cache, so enabling fallback is a
// drop-in change at the call site.
type FaceCacheProvider interface {
	GetFace(size float64) (Face, error)
	Close() error
}

// ChainFaceCache caches ChainFace instances built from a primary font and an
// ordered list of extra fonts (e.g. merged font patches followed by a Nerd
// fallback). Extras cover glyphs missing from earlier levels. Zero-length
// entries in extraDatas are skipped (not an error), so the chain degrades to a
// shorter one; with no extras each ChainFace is primary-only.
type ChainFaceCache struct {
	mu      sync.Mutex
	primary *opentype.Font
	extras  []*opentype.Font // ordered extras (patches before Nerd), may be empty
	dpi     float64
	faces   map[float64]*ChainFace
}

// NewChainFaceCache parses primaryData (required) and each non-empty
// extraData entry (optional). Empty extraDatas entries are skipped rather than
// treated as errors.
func NewChainFaceCache(primaryData []byte, extraDatas [][]byte, dpi float64) (*ChainFaceCache, error) {
	primary, err := opentype.Parse(primaryData)
	if err != nil {
		return nil, err
	}
	var extras []*opentype.Font
	for _, d := range extraDatas {
		if len(d) == 0 {
			continue
		}
		e, err := opentype.Parse(d)
		if err != nil {
			return nil, err
		}
		extras = append(extras, e)
	}
	return &ChainFaceCache{
		primary: primary,
		extras:  extras,
		dpi:     dpi,
		faces:   make(map[float64]*ChainFace),
	}, nil
}

// NewFallbackFaceCache is a thin two-font wrapper around NewChainFaceCache,
// kept for backward compatibility with existing call sites. When fallbackData
// is empty the returned cache serves primary-only ChainFace instances.
func NewFallbackFaceCache(primaryData, fallbackData []byte, dpi float64) (*ChainFaceCache, error) {
	var extras [][]byte
	if len(fallbackData) > 0 {
		extras = [][]byte{fallbackData}
	}
	return NewChainFaceCache(primaryData, extras, dpi)
}

// GetFace returns a cached ChainFace for the given size, creating one on
// first request. Repeated calls for the same size return the same instance.
// If an extra face fails to create, the already-created faces are closed and
// an error is returned.
func (c *ChainFaceCache) GetFace(size float64) (Face, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if f, ok := c.faces[size]; ok {
		return f, nil
	}
	primary, err := newFaceFromParsed(c.primary, size, c.dpi)
	if err != nil {
		return nil, err
	}
	extras := make([]*OpenTypeFace, 0, len(c.extras))
	for _, e := range c.extras {
		ef, err := newFaceFromParsed(e, size, c.dpi)
		if err != nil {
			primary.Close()
			for _, x := range extras {
				x.Close()
			}
			return nil, err
		}
		extras = append(extras, ef)
	}
	f := NewChainFace(primary, extras...)
	c.faces[size] = f
	return f, nil
}

// Close releases all cached ChainFace instances. After Close the cache must
// not be used.
func (c *ChainFaceCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, f := range c.faces {
		f.Close()
	}
	c.faces = nil
	return nil
}
