package font

import (
	"fmt"
	"io/fs"
	"sort"
	"sync"
)

// MergedPatch holds the glyphs merged from all loaded vfp patches, sorted by
// rune ascending with later patches (larger filename) winning conflicts.
type MergedPatch struct {
	glyphs []assembledGlyph
}

// LoadPatches loads and merges every vfp file matched by pattern in fsys.
// Files are processed in lexicographic order, so a rune present in multiple
// patches is taken from the last (largest) filename. A missing match is not an
// error and yields an empty MergedPatch.
func LoadPatches(fsys fs.FS, pattern string) (*MergedPatch, error) {
	names, err := fs.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	merged := &MergedPatch{}
	index := make(map[rune]int)
	for _, name := range names {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("patch %s: %w", name, err)
		}
		f, err := ParseVFP(data)
		if err != nil {
			return nil, fmt.Errorf("patch %s: %w", name, err)
		}
		for i := 0; i < f.Count(); i++ {
			e := f.entries[i]
			var glyphLen uint32
			if i+1 < f.Count() {
				glyphLen = f.entries[i+1].GlyphOff - e.GlyphOff
			} else {
				glyphLen = uint32(len(f.data)) - e.GlyphOff
			}
			g := assembledGlyph{
				rune:    e.Rune,
				glyf:    f.GlyphData(e.GlyphOff, glyphLen),
				advance: e.Advance,
				lsb:     e.Lsb,
			}
			if j, ok := index[e.Rune]; ok {
				merged.glyphs[j] = g
			} else {
				index[e.Rune] = len(merged.glyphs)
				merged.glyphs = append(merged.glyphs, g)
			}
		}
	}
	sort.Slice(merged.glyphs, func(i, j int) bool { return merged.glyphs[i].rune < merged.glyphs[j].rune })
	return merged, nil
}

// FontData assembles the merged glyphs into a TTF byte stream. It returns nil
// when there are no glyphs.
func (m *MergedPatch) FontData() ([]byte, error) {
	if len(m.glyphs) == 0 {
		return nil, nil
	}
	return AssembleFont(m.glyphs)
}

// Empty reports whether the patch set contains no glyphs.
func (m *MergedPatch) Empty() bool {
	return len(m.glyphs) == 0
}

// Runes returns the merged runes in ascending order.
func (m *MergedPatch) Runes() []rune {
	out := make([]rune, len(m.glyphs))
	for i, g := range m.glyphs {
		out[i] = g.rune
	}
	return out
}

var (
	patchOnce sync.Once
	patchData []byte
	patchErr  error
)

// EmbeddedPatchFontData merges all assets/*.vfp and assembles them into a TTF,
// caching the result. It returns nil (with no error) when there are no patches
// in the embedded assets, allowing graceful degradation. A non-nil error
// indicates a corrupt embedded patch file — a build-time defect that should be
// surfaced loudly at startup.
func EmbeddedPatchFontData() ([]byte, error) {
	patchOnce.Do(func() {
		merged, err := LoadPatches(assetsFS, "assets/*.vfp")
		if err != nil {
			patchErr = err
			return
		}
		if merged.Empty() {
			return
		}
		patchData, patchErr = merged.FontData()
	})
	return patchData, patchErr
}
