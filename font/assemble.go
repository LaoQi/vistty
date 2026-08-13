package font

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// patchAscent/patchDescent are the primary font's hhea metrics normalized to
// targetUpem (2048). Sarasa Fixed SC has upem=1000, ascent=970, descent=215,
// which scale to 1976/440 at 2048 upem. ChainFace baseline alignment
// (YOffset += levelAscent - primaryAscent) requires these to match the
// primary font's actual metrics. If the embedded primary font changes, these
// constants must be updated (TestAssembleMetricsMatchPrimary guards against
// drift). Long-term: vfp header v2 could carry per-patch source metrics.
const (
	patchAscent  = 1976
	patchDescent = -440
)

// assembledGlyph is a single glyph ready to be assembled into a TTF font. The
// glyf bytes are simple TrueType outlines normalized to 2048 unitsPerEm (as
// stored in a vfp data region).
type assembledGlyph struct {
	rune    rune
	glyf    []byte // 2048-upem simple glyf (may be empty)
	advance uint16
	lsb     int16
}

// ttTable holds the name, checksum, offset and length of one table in the
// assembled TTF.
type ttTable struct {
	tag      uint32
	length   uint32
	offset   uint32
	checksum uint32
	data     []byte
}

// AssembleFont reassembles a set of glyphs into a valid in-memory TrueType
// font. Glyph 0 is .notdef (empty); glyph i>0 is the i'th patch glyph ordered
// by rune ascending. The returned bytes can be parsed by opentype.Parse and
// rendered. Glyphs with duplicate runes keep the last occurrence.
func AssembleFont(glyphs []assembledGlyph) ([]byte, error) {
	sorted := dedupeSorted(glyphs)
	numGlyphs := len(sorted) + 1

	// ---- glyf + loca ----------------------------------------------------
	glyfBuf := &bytes.Buffer{}
	loca := make([]uint32, numGlyphs+1)
	loca[0] = 0
	off := 0
	for i, g := range sorted {
		for off%4 != 0 {
			glyfBuf.WriteByte(0)
			off++
		}
		loca[i+1] = uint32(off)
		glyfBuf.Write(g.glyf)
		off += len(g.glyf)
	}
	loca[numGlyphs] = uint32(off)
	glyf := glyfBuf.Bytes()

	// ---- hmtx -----------------------------------------------------------
	// Glyph 0 (.notdef) gets a zero advance.
	hmtx := make([]byte, 0, numGlyphs*4)
	var maxAdvance uint16
	minLsb := int16(0x7FFF)
	for i := 0; i < numGlyphs; i++ {
		adv, lsb := uint16(0), int16(0)
		if i > 0 {
			g := sorted[i-1]
			adv, lsb = g.advance, g.lsb
			if adv > maxAdvance {
				maxAdvance = adv
			}
			if lsb < minLsb {
				minLsb = lsb
			}
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint16(buf[0:2], adv)
		binary.BigEndian.PutUint16(buf[2:4], uint16(lsb))
		hmtx = append(hmtx, buf...)
	}
	if minLsb == 0x7FFF {
		minLsb = 0
	}

	// ---- cmap format 12 -------------------------------------------------
	cmap := buildCmap(sorted)

	// ---- head bounds (union of glyph bounding boxes) ---------------------
	xMin, yMin, xMax, yMax := int16(0), int16(0), int16(0), int16(0)
	for _, g := range sorted {
		if len(g.glyf) < 10 {
			continue
		}
		// Simple glyph header: numContours, xMin, yMin, xMax, yMax.
		gxMin := int16(binary.BigEndian.Uint16(g.glyf[2:4]))
		gyMin := int16(binary.BigEndian.Uint16(g.glyf[4:6]))
		gxMax := int16(binary.BigEndian.Uint16(g.glyf[6:8]))
		gyMax := int16(binary.BigEndian.Uint16(g.glyf[8:10]))
		if gxMin < xMin {
			xMin = gxMin
		}
		if gyMin < yMin {
			yMin = gyMin
		}
		if gxMax > xMax {
			xMax = gxMax
		}
		if gyMax > yMax {
			yMax = gyMax
		}
	}

	// ---- build all tables ------------------------------------------------
	tables := []*ttTable{
		{tag: tagOf("OS/2"), data: buildOS2()},
		{tag: tagOf("cmap"), data: cmap},
		{tag: tagOf("glyf"), data: glyf},
		{tag: tagOf("head"), data: buildHead(xMin, yMin, xMax, yMax)},
		{tag: tagOf("hhea"), data: buildHhea(numGlyphs, maxAdvance, minLsb)},
		{tag: tagOf("hmtx"), data: hmtx},
		{tag: tagOf("loca"), data: buildLoca(loca)},
		{tag: tagOf("maxp"), data: buildMaxp(numGlyphs)},
		{tag: tagOf("name"), data: buildName()},
		{tag: tagOf("post"), data: buildPost()},
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].tag < tables[j].tag })

	return assembleFile(tables), nil
}

// dedupeSorted sorts glyphs by rune ascending, keeping only the last
// occurrence of a duplicate rune (later patch wins).
func dedupeSorted(glyphs []assembledGlyph) []assembledGlyph {
	if len(glyphs) == 0 {
		return nil
	}
	sorted := make([]assembledGlyph, len(glyphs))
	copy(sorted, glyphs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].rune < sorted[j].rune })
	out := sorted[:0]
	for _, g := range sorted {
		if len(out) > 0 && out[len(out)-1].rune == g.rune {
			out[len(out)-1] = g
			continue
		}
		out = append(out, g)
	}
	return out
}

// buildCmap builds a cmap table with a single format 12 subtable (Windows
// platform, Unicode full repertoire encoding 10), covering both BMP and
// non-BMP runes. Glyph IDs are the glyph's index in the assembled font.
func buildCmap(sorted []assembledGlyph) []byte {
	type group struct{ start, end, gid uint32 }
	var groups []group
	for i := 0; i < len(sorted); {
		start := uint32(sorted[i].rune)
		gid := uint32(i + 1)
		j := i + 1
		for j < len(sorted) && uint32(sorted[j].rune) == uint32(sorted[j-1].rune)+1 {
			j++
		}
		groups = append(groups, group{start: start, end: uint32(sorted[j-1].rune), gid: gid})
		i = j
	}

	sub := &bytes.Buffer{}
	n := uint32(len(groups))
	sub.Write([]byte{0, 12, 0, 0}) // format=12, reserved=0
	writeU32(sub, 16+12*n)          // length
	writeU32(sub, 0)                // language
	writeU32(sub, n)                // nGroups
	for _, g := range groups {
		writeU32(sub, g.start)
		writeU32(sub, g.end)
		writeU32(sub, g.gid)
	}

	out := &bytes.Buffer{}
	writeU16(out, 0) // version
	writeU16(out, 1) // numTables
	writeU16(out, 3) // platformID = Windows
	writeU16(out, 10)
	writeU32(out, 12) // offset to subtable
	out.Write(sub.Bytes())
	return out.Bytes()
}

// buildHead builds a 54-byte head table with indexToLocFormat=1 (long) and a
// zero checkSumAdjustment placeholder (filled in assembleFile).
func buildHead(xMin, yMin, xMax, yMax int16) []byte {
	b := &bytes.Buffer{}
	writeU32(b, 0x00010000)  // version
	writeU32(b, 0)           // fontRevision
	writeU32(b, 0)           // checkSumAdjustment (placeholder)
	writeU32(b, 0x5F0F3CF5)  // magicNumber
	writeU16(b, 0)           // flags
	writeU16(b, targetUpem)  // unitsPerEm
	writeU64(b, 0)           // created
	writeU64(b, 0)           // modified
	writeI16(b, xMin)        // xMin
	writeI16(b, yMin)        // yMin
	writeI16(b, xMax)        // xMax
	writeI16(b, yMax)        // yMax
	writeU16(b, 0)           // macStyle
	writeU16(b, 0)           // lowestRecPPEM
	writeI16(b, 0)           // fontDirectionHint
	writeI16(b, 1)           // indexToLocFormat (long)
	writeI16(b, 0)           // glyphDataFormat
	return b.Bytes()
}

func buildHhea(numGlyphs int, maxAdvance uint16, minLsb int16) []byte {
	b := &bytes.Buffer{}
	writeU32(b, 0x00010000)   // version
	writeI16(b, patchAscent)  // ascent
	writeI16(b, patchDescent) // descent
	writeI16(b, 0)          // lineGap
	writeU16(b, maxAdvance) // advanceWidthMax
	writeI16(b, minLsb)     // minLeftSideBearing
	writeI16(b, 0)          // minRightSideBearing
	writeI16(b, 0)          // xMaxExtent
	writeI16(b, 1)          // caretSlopeRise
	writeI16(b, 0)          // caretSlopeRun
	writeI16(b, 0)          // caretOffset
	writeI16(b, 0)          // reserved
	writeI16(b, 0)          // reserved
	writeI16(b, 0)          // reserved
	writeI16(b, 0)          // reserved
	writeI16(b, 0)          // metricDataFormat
	writeU16(b, uint16(numGlyphs))
	return b.Bytes()
}

func buildLoca(loca []uint32) []byte {
	b := &bytes.Buffer{}
	for _, l := range loca {
		writeU32(b, l)
	}
	return b.Bytes()
}

// buildMaxp builds a 32-byte TrueType maxp table (version 0x00010000).
func buildMaxp(numGlyphs int) []byte {
	b := &bytes.Buffer{}
	writeU32(b, 0x00010000)
	writeU16(b, uint16(numGlyphs))
	for i := 0; i < 13; i++ { // maxPoints .. maxComponentDepth
		writeU16(b, 0)
	}
	return b.Bytes()
}

// buildName builds a minimal valid name table (one empty record).
func buildName() []byte {
	b := &bytes.Buffer{}
	writeU16(b, 0) // format
	writeU16(b, 1) // count
	writeU16(b, 18) // stringOffset
	// One record: platform 3, encoding 1, language 0x409, nameID 0.
	writeU16(b, 3)
	writeU16(b, 1)
	writeU16(b, 0x409)
	writeU16(b, 0)
	writeU16(b, 0) // length
	writeU16(b, 0) // stringOffset
	return b.Bytes()
}

// buildPost builds a 32-byte post table, format 3.0 (no glyph names).
func buildPost() []byte {
	b := &bytes.Buffer{}
	writeU32(b, 0x00030000)
	for i := 0; i < 7; i++ {
		writeU32(b, 0)
	}
	return b.Bytes()
}

// buildOS2 builds a minimal OS/2 version 0 table (78 bytes).
func buildOS2() []byte {
	b := &bytes.Buffer{}
	writeU16(b, 0)   // version
	writeU16(b, 0)   // xAvgCharWidth
	writeU16(b, 400) // usWeightClass
	writeU16(b, 5)   // usWidthClass
	writeU16(b, 0)   // fsType
	for i := 0; i < 11; i++ {
		writeU16(b, 0) // ySubscriptXSize .. sFamilyClass
	}
	for i := 0; i < 10; i++ {
		b.WriteByte(0) // panose[10]
	}
	for i := 0; i < 4; i++ {
		writeU32(b, 0) // ulUnicodeRange1..4
	}
	b.WriteString("vist") // achVendID
	writeU16(b, 0)        // fsSelection
	writeU16(b, 0x20)     // usFirstCharIndex
	writeU16(b, 0xFFFF)        // usLastCharIndex
	writeI16(b, patchAscent)   // sTypoAscender
	writeI16(b, patchDescent)  // sTypoDescender
	writeI16(b, 0)             // sTypoLineGap
	writeU16(b, uint16(patchAscent))  // usWinAscent
	writeU16(b, uint16(-patchDescent)) // usWinDescent
	return b.Bytes()
}

// assembleFile lays out tables 4-byte aligned, builds the table directory
// (with per-table checksums) and finally computes checkSumAdjustment.
func assembleFile(tables []*ttTable) []byte {
	numTables := len(tables)
	// Header 12 bytes + 16 bytes per record.
	cur := 12 + 16*numTables
	for _, t := range tables {
		t.length = uint32(len(t.data))
	}
	// Assign 4-byte-aligned offsets. The first table follows the directory.
	for _, t := range tables {
		for cur%4 != 0 {
			cur++
		}
		t.offset = uint32(cur)
		cur += len(t.data)
	}
	// Compute per-table checksums (padded to 4 bytes). head is computed with
	// its checkSumAdjustment field still 0, which is the standard practice.
	for _, t := range tables {
		t.checksum = tableChecksum(t.data)
	}

	// Build the file bytes: offset table + directory + table data.
	file := make([]byte, 0, cur)
	file = appendOffsetTable(file, tables)
	var headOff uint32
	for _, t := range tables {
		for uint32(len(file)) < t.offset {
			file = append(file, 0)
		}
		file = append(file, t.data...)
		if t.tag == tagOf("head") {
			headOff = t.offset
		}
	}

	// checkSumAdjustment: sum all uint32 words (head field still 0), then
	// 0xB1B0AFBA - sum, written into head[8:12].
	sum := tableChecksum(file)
	adj := 0xB1B0AFBA - sum
	binary.BigEndian.PutUint32(file[headOff+8:headOff+12], adj)
	return file
}

// appendOffsetTable writes the offset table (scaler type, table count and the
// directory) into dst. It must be called before the table data is appended so
// the per-table offsets match the final layout.
func appendOffsetTable(dst []byte, tables []*ttTable) []byte {
	numTables := len(tables)
	// Offset table header.
	var hdr [12]byte
	binary.BigEndian.PutUint32(hdr[0:4], 0x00010000)
	binary.BigEndian.PutUint16(hdr[4:6], uint16(numTables))
	entrySelector := 0
	for (1 << uint(entrySelector+1)) <= numTables {
		entrySelector++
	}
	searchRange := uint16(1) << uint(entrySelector)
	binary.BigEndian.PutUint16(hdr[6:8], uint16(searchRange*16))
	binary.BigEndian.PutUint16(hdr[8:10], uint16(entrySelector))
	binary.BigEndian.PutUint16(hdr[10:12], uint16(numTables*16-int(searchRange)*16))
	dst = append(dst, hdr[:]...)

	for _, t := range tables {
		var rec [16]byte
		binary.BigEndian.PutUint32(rec[0:4], t.tag)
		binary.BigEndian.PutUint32(rec[4:8], t.checksum)
		binary.BigEndian.PutUint32(rec[8:12], uint32(t.offset))
		binary.BigEndian.PutUint32(rec[12:16], t.length)
		dst = append(dst, rec[:]...)
	}
	return dst
}

// tableChecksum returns the sum of all 4-byte big-endian words of data,
// padding the final partial word with zeros.
func tableChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i+4 <= len(data); i += 4 {
		sum += binary.BigEndian.Uint32(data[i : i+4])
	}
	if r := len(data) % 4; r != 0 {
		var pad [4]byte
		copy(pad[:], data[len(data)-r:])
		sum += binary.BigEndian.Uint32(pad[:])
	}
	return sum
}

// tagOf converts a 4-character table tag to its uint32 big-endian value.
func tagOf(s string) uint32 {
	return binary.BigEndian.Uint32([]byte(s))
}

func writeU16(b *bytes.Buffer, v uint16) {
	binary.Write(b, binary.BigEndian, v)
}

func writeI16(b *bytes.Buffer, v int16) {
	binary.Write(b, binary.BigEndian, v)
}

func writeU32(b *bytes.Buffer, v uint32) {
	binary.Write(b, binary.BigEndian, v)
}

func writeU64(b *bytes.Buffer, v uint64) {
	binary.Write(b, binary.BigEndian, v)
}
