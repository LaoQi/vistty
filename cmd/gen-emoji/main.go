package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"sort"

	"github.com/LaoQi/vistty/internal/runeutil"
)

type tableRecord struct {
	offset uint32
	length uint32
}

type indexSubTable struct {
	first           uint16
	last            uint16
	imageFormat     uint16
	imageDataOffset uint32
	offsets         []uint32
}

type strike struct {
	ppem      int
	subtables []indexSubTable
}

type glyphMetrics struct {
	height   uint8
	width    uint8
	bearingX int8
	bearingY int8
	advance  uint8
}

type emojiEntry struct {
	r       rune
	pngOff  uint32
	pngLen  uint32
	xOffset int8
	yOffset int8
	advance uint8
}

func main() {
	in := flag.String("in", "", "input NotoColorEmoji.ttf")
	out := flag.String("out", "", "output emoji.bin.gz path")
	srcSize := flag.Int("src-size", 128, "preferred source ppem size")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: gen-emoji -in <input.ttf> -out <output.bin.gz> [-src-size 128]")
		os.Exit(1)
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *in, err)
		os.Exit(1)
	}

	tables, err := parseSFNT(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse sfnt: %v\n", err)
		os.Exit(1)
	}
	cmapRec, ok := tables["cmap"]
	if !ok {
		fmt.Fprintln(os.Stderr, "missing cmap table")
		os.Exit(1)
	}
	cblcRec, ok := tables["CBLC"]
	if !ok {
		fmt.Fprintln(os.Stderr, "missing CBLC table")
		os.Exit(1)
	}
	cbdtRec, ok := tables["CBDT"]
	if !ok {
		fmt.Fprintln(os.Stderr, "missing CBDT table")
		os.Exit(1)
	}

	cmap, err := parseCmap(data, cmapRec.offset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse cmap: %v\n", err)
		os.Exit(1)
	}

	st, err := parseCBLC(data, cblcRec.offset, *srcSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse CBLC: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("selected strike ppem=%d (requested %d), %d subtables\n", st.ppem, *srcSize, len(st.subtables))

	var runes []rune
	for r := range cmap {
		if runeutil.IsEmojiRune(r) && !isControlRune(r) {
			runes = append(runes, r)
		}
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })

	var entries []emojiEntry
	var pngs [][]byte
	var pngOff uint32
	skipped := 0
	for _, r := range runes {
		gid := uint16(cmap[r])
		m, png, ok := extractGlyph(data, cbdtRec.offset, st, gid)
		if !ok || len(png) == 0 {
			skipped++
			continue
		}
		entries = append(entries, emojiEntry{
			r:       r,
			pngOff:  pngOff,
			pngLen:  uint32(len(png)),
			xOffset: m.bearingX,
			yOffset: m.bearingY,
			advance: m.advance,
		})
		pngs = append(pngs, png)
		pngOff += uint32(len(png))
	}
	fmt.Printf("collected %d emoji (%d skipped, no bitmap)\n", len(entries), skipped)

	if err := writeEmoji(*out, entries, pngs, st.ppem); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
		os.Exit(1)
	}

	if err := verify(*out, entries, pngs); err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(1)
	}
}

func isControlRune(r rune) bool {
	switch {
	case r == 0x200D:
		return true
	case r == 0x20E3:
		return true
	case r == 0xFE0E, r == 0xFE0F:
		return true
	case 0xE0020 <= r && r <= 0xE007F:
		return true
	case 0xFE00 <= r && r <= 0xFE0F:
		return true
	}
	return false
}

func parseSFNT(data []byte) (map[string]tableRecord, error) {
	if len(data) < 12 {
		return nil, errors.New("sfnt too small")
	}
	numTables := binary.BigEndian.Uint16(data[4:6])
	tables := make(map[string]tableRecord)
	off := 12
	for i := 0; i < int(numTables); i++ {
		if off+16 > len(data) {
			return nil, errors.New("truncated table records")
		}
		rec := data[off : off+16]
		tag := string(rec[0:4])
		offset := binary.BigEndian.Uint32(rec[8:12])
		length := binary.BigEndian.Uint32(rec[12:16])
		tables[tag] = tableRecord{offset: offset, length: length}
		off += 16
	}
	return tables, nil
}

func parseCmap(data []byte, cmapOff uint32) (map[rune]uint32, error) {
	base := int(cmapOff)
	if base+4 > len(data) {
		return nil, errors.New("cmap truncated")
	}
	numTables := binary.BigEndian.Uint16(data[base+2 : base+4])
	var bestSubOff uint32
	bestFormat := uint16(0)
	for i := 0; i < int(numTables); i++ {
		recOff := base + 4 + i*8
		if recOff+8 > len(data) {
			break
		}
		platformID := binary.BigEndian.Uint16(data[recOff : recOff+2])
		encodingID := binary.BigEndian.Uint16(data[recOff+2 : recOff+4])
		subOff := binary.BigEndian.Uint32(data[recOff+4 : recOff+8])
		subBase := base + int(subOff)
		if subBase+2 > len(data) {
			continue
		}
		fmtVal := binary.BigEndian.Uint16(data[subBase : subBase+2])
		if fmtVal == 12 && (platformID == 0 || (platformID == 3 && encodingID == 10)) {
			bestSubOff = subOff
			bestFormat = 12
			break
		}
		if bestFormat != 12 && fmtVal == 12 {
			bestSubOff = subOff
			bestFormat = 12
		}
	}
	if bestFormat != 12 {
		return nil, errors.New("no cmap format 12 subtable (SMP support required for emoji)")
	}
	subBase := base + int(bestSubOff)
	if subBase+16 > len(data) {
		return nil, errors.New("cmap format 12 header truncated")
	}
	numGroups := binary.BigEndian.Uint32(data[subBase+12 : subBase+16])
	m := make(map[rune]uint32)
	gOff := subBase + 16
	for i := uint32(0); i < numGroups; i++ {
		if gOff+12 > len(data) {
			break
		}
		startChar := binary.BigEndian.Uint32(data[gOff : gOff+4])
		endChar := binary.BigEndian.Uint32(data[gOff+4 : gOff+8])
		startGID := binary.BigEndian.Uint32(data[gOff+8 : gOff+12])
		for c := startChar; c <= endChar; c++ {
			m[rune(c)] = startGID + (c - startChar)
		}
		gOff += 12
	}
	return m, nil
}

func parseCBLC(data []byte, cblcOff uint32, srcSize int) (*strike, error) {
	base := int(cblcOff)
	if base+8 > len(data) {
		return nil, errors.New("CBLC truncated")
	}
	numSizes := binary.BigEndian.Uint32(data[base+4 : base+8])
	var chosen *strike
	bestDiff := -1
	for i := uint32(0); i < numSizes; i++ {
		recOff := base + 8 + int(i)*48
		if recOff+48 > len(data) {
			return nil, errors.New("CBLC BitmapSize record truncated")
		}
		idxArrayOff := binary.BigEndian.Uint32(data[recOff : recOff+4])
		numIdxSub := binary.BigEndian.Uint32(data[recOff+8 : recOff+12])
		ppemX := int(data[recOff+44])
		diff := ppemX - srcSize
		if diff < 0 {
			diff = -diff
		}
		if bestDiff < 0 || diff < bestDiff {
			subs := parseStrikeSubtables(data, base, idxArrayOff, numIdxSub)
			chosen = &strike{ppem: ppemX, subtables: subs}
			bestDiff = diff
		}
	}
	if chosen == nil {
		return nil, errors.New("no CBLC strike found")
	}
	return chosen, nil
}

func parseStrikeSubtables(data []byte, cblcBase int, idxArrayOff uint32, numIdxSub uint32) []indexSubTable {
	var subs []indexSubTable
	for i := uint32(0); i < numIdxSub; i++ {
		arrOff := cblcBase + int(idxArrayOff) + int(i)*8
		if arrOff+8 > len(data) {
			break
		}
		first := binary.BigEndian.Uint16(data[arrOff : arrOff+2])
		last := binary.BigEndian.Uint16(data[arrOff+2 : arrOff+4])
		addOff := binary.BigEndian.Uint32(data[arrOff+4 : arrOff+8])
		subOff := cblcBase + int(idxArrayOff) + int(addOff)
		if subOff+8 > len(data) {
			continue
		}
		indexFormat := binary.BigEndian.Uint16(data[subOff : subOff+2])
		imageFormat := binary.BigEndian.Uint16(data[subOff+2 : subOff+4])
		imageDataOffset := binary.BigEndian.Uint32(data[subOff+4 : subOff+8])
		if indexFormat != 1 {
			continue
		}
		n := int(last-first) + 2
		oOff := subOff + 8
		if oOff+n*4 > len(data) {
			n = (len(data) - oOff) / 4
		}
		if n < 2 {
			continue
		}
		offsets := make([]uint32, n)
		for k := 0; k < n; k++ {
			offsets[k] = binary.BigEndian.Uint32(data[oOff+k*4 : oOff+k*4+4])
		}
		subs = append(subs, indexSubTable{
			first:           first,
			last:            last,
			imageFormat:     imageFormat,
			imageDataOffset: imageDataOffset,
			offsets:         offsets,
		})
	}
	return subs
}

func extractGlyph(data []byte, cbdtOff uint32, st *strike, gid uint16) (glyphMetrics, []byte, bool) {
	for _, sub := range st.subtables {
		if gid < sub.first || gid > sub.last {
			continue
		}
		idx := int(gid - sub.first)
		if idx+1 >= len(sub.offsets) {
			return glyphMetrics{}, nil, false
		}
		dataOff := sub.imageDataOffset + sub.offsets[idx]
		dataLen := sub.offsets[idx+1] - sub.offsets[idx]
		if dataLen == 0 {
			return glyphMetrics{}, nil, false
		}
		abs := int(cbdtOff) + int(dataOff)
		if abs < 0 || abs+int(dataLen) > len(data) {
			return glyphMetrics{}, nil, false
		}
		blob := data[abs : abs+int(dataLen)]
		switch sub.imageFormat {
		case 17:
			if len(blob) < 9 {
				return glyphMetrics{}, nil, false
			}
			m := glyphMetrics{
				height:   blob[0],
				width:    blob[1],
				bearingX: int8(blob[2]),
				bearingY: int8(blob[3]),
				advance:  blob[4],
			}
			pngLen := binary.BigEndian.Uint32(blob[5:9])
			if 9+int(pngLen) > len(blob) {
				return glyphMetrics{}, nil, false
			}
			return m, blob[9 : 9+int(pngLen)], true
		case 18:
			if len(blob) < 12 {
				return glyphMetrics{}, nil, false
			}
			m := glyphMetrics{
				height:   blob[0],
				width:    blob[1],
				bearingX: int8(blob[2]),
				bearingY: int8(blob[3]),
				advance:  blob[4],
			}
			pngLen := binary.BigEndian.Uint32(blob[8:12])
			if 12+int(pngLen) > len(blob) {
				return glyphMetrics{}, nil, false
			}
			return m, blob[12 : 12+int(pngLen)], true
		}
	}
	return glyphMetrics{}, nil, false
}

func writeEmoji(path string, entries []emojiEntry, pngs [][]byte, srcSize int) error {
	var pngDataSize uint32
	for _, p := range pngs {
		pngDataSize += uint32(len(p))
	}
	var buf bytes.Buffer
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(entries)))
	binary.LittleEndian.PutUint32(hdr[4:8], pngDataSize)
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(srcSize))
	buf.Write(hdr[:])

	var idx [16]byte
	for _, e := range entries {
		binary.LittleEndian.PutUint32(idx[0:4], uint32(e.r))
		binary.LittleEndian.PutUint32(idx[4:8], e.pngOff)
		binary.LittleEndian.PutUint32(idx[8:12], e.pngLen)
		idx[12] = byte(e.xOffset)
		idx[13] = byte(e.yOffset)
		idx[14] = e.advance
		idx[15] = 0
		buf.Write(idx[:])
	}
	for _, p := range pngs {
		buf.Write(p)
	}

	if err := os.MkdirAll(parentDir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(buf.Bytes()); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}

func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func verify(path string, entries []emojiEntry, pngs [][]byte) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	if len(data) < 12 {
		return errors.New("emoji.bin too small")
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	pngDataSize := binary.LittleEndian.Uint32(data[4:8])
	srcSize := binary.LittleEndian.Uint32(data[8:12])
	fmt.Printf("verify: count=%d pngDataSize=%d srcSize=%d\n", count, pngDataSize, srcSize)
	if count == 0 {
		return errors.New("no emoji entries")
	}
	if 12+int(count)*16 > len(data) {
		return errors.New("index truncated")
	}
	firstOff := binary.LittleEndian.Uint32(data[12+4 : 12+8])
	firstLen := binary.LittleEndian.Uint32(data[12+8 : 12+12])
	pngBase := 12 + int(count)*16
	pngStart := pngBase + int(firstOff)
	if pngStart+int(firstLen) > len(data) {
		return errors.New("first png out of range")
	}
	if _, err := png.Decode(bytes.NewReader(data[pngStart : pngStart+int(firstLen)])); err != nil {
		return fmt.Errorf("decode first png (rune U+%X): %w", entries[0].r, err)
	}
	fmt.Printf("verify: first png (U+%X, %d bytes) decoded ok\n", entries[0].r, firstLen)

	n := 5
	if n > len(entries) {
		n = len(entries)
	}
	fmt.Print("verify: first emoji -> ")
	for i := 0; i < n; i++ {
		fmt.Printf("U+%X(%dB) ", entries[i].r, entries[i].pngLen)
	}
	fmt.Println()
	return nil
}
