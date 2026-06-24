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
