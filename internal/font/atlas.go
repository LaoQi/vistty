package font

import (
	"container/list"
	"sync"
)

type Glyph struct {
	Rune    rune
	Bitmap  []byte
	Width   int
	Height  int
	XOffset int
	YOffset int
	Advance int
}

type atlasEntry struct {
	key  rune
	glyph *Glyph
	elem *list.Element
}

type Atlas struct {
	mu       sync.RWMutex
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
	a.mu.RLock()
	entry, ok := a.cache[r]
	a.mu.RUnlock()
	if !ok {
		return nil
	}

	a.mu.Lock()
	a.order.MoveToFront(entry.elem)
	a.mu.Unlock()

	return entry.glyph
}

func (a *Atlas) Put(r rune, g *Glyph) {
	a.mu.Lock()
	defer a.mu.Unlock()

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
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cache = make(map[rune]*atlasEntry)
	a.order.Init()
}
