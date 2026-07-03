package font

import (
	"container/list"
)

type Glyph struct {
	Rune    rune
	Bitmap  []byte
	Width   int
	Height  int
	XOffset int
	YOffset int
	Advance int
	IsColor bool // true=Bitmap 是 RGBA(w*h*4)，false=alpha(w*h)
}

type atlasEntry struct {
	key   rune
	glyph *Glyph
	elem  *list.Element
}

type Atlas struct {
	capacity int
	cache    map[rune]*atlasEntry
	order    *list.List
}

func NewAtlas(capacity int) *Atlas {
	if capacity <= 0 {
		capacity = 4096
	}
	return &Atlas{
		capacity: capacity,
		cache:    make(map[rune]*atlasEntry),
		order:    list.New(),
	}
}

func (a *Atlas) Get(r rune) *Glyph {
	entry, ok := a.cache[r]
	if !ok {
		return nil
	}
	return entry.glyph
}

func (a *Atlas) Put(r rune, g *Glyph) {
	if entry, ok := a.cache[r]; ok {
		entry.glyph = g
		a.order.MoveToFront(entry.elem)
		return
	}

	if a.order.Len() >= a.capacity {
		oldest := a.order.Back()
		if oldest != nil {
			oldEntry := a.order.Remove(oldest).(*atlasEntry)
			delete(a.cache, oldEntry.key)
		}
	}

	elem := a.order.PushFront(&atlasEntry{key: r})
	a.cache[r] = &atlasEntry{key: r, glyph: g, elem: elem}
}

func (a *Atlas) Clear() {
	a.cache = make(map[rune]*atlasEntry)
	a.order.Init()
}
