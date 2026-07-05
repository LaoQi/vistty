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
// It is implemented by both FaceCache (single font) and FallbackFaceCache
// (primary + optional fallback). Consumers such as the compositor depend on
// this interface rather than a concrete cache, so enabling fallback is a
// drop-in change at the call site.
type FaceCacheProvider interface {
	GetFace(size float64) (Face, error)
	Close() error
}

// FallbackFaceCache caches FallbackFace instances built from a primary font
// and an optional fallback font. The fallback covers glyphs missing from
// primary. When fallbackData is empty the cache degrades to primary-only
// (each FallbackFace carries a nil fallback).
type FallbackFaceCache struct {
	mu       sync.Mutex
	primary  *opentype.Font
	fallback *opentype.Font
	dpi      float64
	faces    map[float64]*FallbackFace
}

// NewFallbackFaceCache parses primaryData (required) and fallbackData
// (optional). When fallbackData is empty the returned cache serves
// primary-only FallbackFace instances.
func NewFallbackFaceCache(primaryData, fallbackData []byte, dpi float64) (*FallbackFaceCache, error) {
	primary, err := opentype.Parse(primaryData)
	if err != nil {
		return nil, err
	}
	var fallback *opentype.Font
	if len(fallbackData) > 0 {
		fallback, err = opentype.Parse(fallbackData)
		if err != nil {
			return nil, err
		}
	}
	return &FallbackFaceCache{
		primary:  primary,
		fallback: fallback,
		dpi:      dpi,
		faces:    make(map[float64]*FallbackFace),
	}, nil
}

// GetFace returns a cached FallbackFace for the given size, creating one on
// first request. Repeated calls for the same size return the same instance.
func (c *FallbackFaceCache) GetFace(size float64) (Face, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if f, ok := c.faces[size]; ok {
		return f, nil
	}
	primary, err := newFaceFromParsed(c.primary, size, c.dpi)
	if err != nil {
		return nil, err
	}
	var fallback *OpenTypeFace
	if c.fallback != nil {
		fallback, err = newFaceFromParsed(c.fallback, size, c.dpi)
		if err != nil {
			primary.Close()
			return nil, err
		}
	}
	f := NewFallbackFace(primary, fallback)
	c.faces[size] = f
	return f, nil
}

// Close releases all cached FallbackFace instances. After Close the cache
// must not be used.
func (c *FallbackFaceCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, f := range c.faces {
		f.Close()
	}
	c.faces = nil
	return nil
}
