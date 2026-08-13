package font

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// vfp is an append-only font patch format storing simple TrueType glyf
// outlines normalized to 2048 unitsPerEm.
//
// Header (12B):
//
//	magic     4B   "VFP1"
//	count     4B   number of glyphs
//	dataSize  4B   size of the data region in bytes
//
// Index[count] (12B each, sorted by rune ascending):
//
//	rune      4B
//	glyphOff  4B   offset relative to the start of the data region
//	advance   2B   font units (uint16)
//	lsb       2B   int16
//
// Data (dataSize bytes):
//
//	concatenated simple glyf bytes (empty glyphs occupy 0 bytes; glyph length
//	is derived from the difference of adjacent glyphOff values).
const vfpMagic = "VFP1"

const (
	vfpHeaderSize = 12
	vfpEntrySize  = 12
)

// VfpEntry describes a single glyph in a vfp file.
type VfpEntry struct {
	Rune     rune
	GlyphOff uint32
	Advance  uint16
	Lsb      int16
}

// VfpFile is a parsed vfp font patch.
type VfpFile struct {
	entries []VfpEntry // sorted by rune ascending
	data    []byte     // data region
}

// EncodeVFP encodes entries and their glyph byte slices into a vfp byte
// stream. The input entries may be in any order; they are sorted by rune and
// glyphOff is assigned in that sorted order. glyphs[i] is the glyf byte slice
// of entries[i] (may be empty). Duplicate runes are rejected.
func EncodeVFP(entries []VfpEntry, glyphs [][]byte) ([]byte, error) {
	if len(entries) != len(glyphs) {
		return nil, fmt.Errorf("vfp: %d entries but %d glyphs", len(entries), len(glyphs))
	}
	seen := make(map[rune]bool, len(entries))
	for _, e := range entries {
		if seen[e.Rune] {
			return nil, fmt.Errorf("vfp: duplicate rune U+%04X", e.Rune)
		}
		seen[e.Rune] = true
	}

	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return entries[order[i]].Rune < entries[order[j]].Rune })

	var data []byte
	var dataSize uint32
	idx := make([]VfpEntry, len(entries))
	for k, o := range order {
		idx[k] = entries[o]
		idx[k].GlyphOff = dataSize
		data = append(data, glyphs[o]...)
		dataSize += uint32(len(glyphs[o]))
	}

	var buf bytes.Buffer
	var hdr [vfpHeaderSize]byte
	copy(hdr[0:4], vfpMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(idx)))
	binary.LittleEndian.PutUint32(hdr[8:12], dataSize)
	buf.Write(hdr[:])

	var e [vfpEntrySize]byte
	for _, ent := range idx {
		binary.LittleEndian.PutUint32(e[0:4], uint32(ent.Rune))
		binary.LittleEndian.PutUint32(e[4:8], ent.GlyphOff)
		binary.LittleEndian.PutUint16(e[8:10], ent.Advance)
		binary.LittleEndian.PutUint16(e[10:12], uint16(ent.Lsb))
		buf.Write(e[:])
	}
	buf.Write(data)
	return buf.Bytes(), nil
}

// ParseVFP parses and validates a vfp byte stream.
func ParseVFP(data []byte) (*VfpFile, error) {
	if len(data) < vfpHeaderSize {
		return nil, errors.New("vfp: truncated header")
	}
	if string(data[0:4]) != vfpMagic {
		return nil, errors.New("vfp: bad magic")
	}
	count := binary.LittleEndian.Uint32(data[4:8])
	dataSize := binary.LittleEndian.Uint32(data[8:12])
	indexEnd := vfpHeaderSize + int(count)*vfpEntrySize
	if indexEnd > len(data) {
		return nil, errors.New("vfp: truncated index")
	}
	dataStart := indexEnd
	if uint32(len(data)-dataStart) < dataSize {
		return nil, errors.New("vfp: truncated data region")
	}

	entries := make([]VfpEntry, 0, count)
	for i := 0; i < int(count); i++ {
		off := vfpHeaderSize + i*vfpEntrySize
		e := VfpEntry{
			Rune:     rune(binary.LittleEndian.Uint32(data[off : off+4])),
			GlyphOff: binary.LittleEndian.Uint32(data[off+4 : off+8]),
			Advance:  binary.LittleEndian.Uint16(data[off+8 : off+10]),
			Lsb:      int16(binary.LittleEndian.Uint16(data[off+10 : off+12])),
		}
		if len(entries) > 0 && e.Rune <= entries[len(entries)-1].Rune {
			return nil, errors.New("vfp: index not strictly ascending")
		}
		if e.GlyphOff > dataSize {
			return nil, errors.New("vfp: glyphOff out of range")
		}
		if len(entries) > 0 && e.GlyphOff < entries[len(entries)-1].GlyphOff {
			return nil, errors.New("vfp: glyphOff not monotonic")
		}
		entries = append(entries, e)
	}

	return &VfpFile{
		entries: entries,
		data:    data[dataStart : dataStart+int(dataSize)],
	}, nil
}

// Find returns the glyf offset/length, advance and lsb of the glyph for r.
// glyphLen is derived from the next entry's glyphOff (or dataSize for the
// last glyph).
func (f *VfpFile) Find(r rune) (glyphOff, glyphLen uint32, advance uint16, lsb int16, ok bool) {
	i := sort.Search(len(f.entries), func(i int) bool { return f.entries[i].Rune >= r })
	if i >= len(f.entries) || f.entries[i].Rune != r {
		return 0, 0, 0, 0, false
	}
	e := f.entries[i]
	var end uint32
	if i+1 < len(f.entries) {
		end = f.entries[i+1].GlyphOff
	} else {
		end = uint32(len(f.data))
	}
	return e.GlyphOff, end - e.GlyphOff, e.Advance, e.Lsb, true
}

// GlyphData returns the glyf bytes of a glyph located at glyphOff of length
// glyphLen.
func (f *VfpFile) GlyphData(glyphOff, glyphLen uint32) []byte {
	return f.data[glyphOff : glyphOff+glyphLen]
}

// Count returns the number of glyph entries.
func (f *VfpFile) Count() int {
	return len(f.entries)
}
