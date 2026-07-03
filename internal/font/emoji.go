package font

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"sort"
	"sync"

	"golang.org/x/image/draw"
)

//go:embed data/emoji.bin.gz
var emojiData []byte

var (
	emojiIdxOnce sync.Once
	emojiIdx     *emojiIndex
	emojiIdxErr  error
)

type emojiEntry struct {
	r       rune
	pngOff  uint32
	pngLen  uint32
	xOffset int8
	yOffset int8
	advance uint8
}

type emojiIndex struct {
	buf     []byte
	entries []emojiEntry
	pngBase int
	srcSize int
}

func loadEmojiIndex() (*emojiIndex, error) {
	gz, err := gzip.NewReader(bytes.NewReader(emojiData))
	if err != nil {
		return nil, fmt.Errorf("emoji.bin.gz: %w", err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read emoji.bin.gz: %w", err)
	}
	if len(data) < 12 {
		return nil, fmt.Errorf("emoji.bin too small: %d bytes", len(data))
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	pngDataSize := binary.LittleEndian.Uint32(data[4:8])
	srcSize := int(binary.LittleEndian.Uint32(data[8:12]))

	indexEnd := 12 + int(count)*16
	if indexEnd > len(data) {
		return nil, fmt.Errorf("emoji.bin index truncated: need %d, got %d", indexEnd, len(data))
	}
	pngBase := indexEnd
	if pngBase+int(pngDataSize) > len(data) {
		return nil, fmt.Errorf("emoji.bin png data truncated: need %d, got %d", pngBase+int(pngDataSize), len(data))
	}

	entries := make([]emojiEntry, count)
	for i := uint32(0); i < count; i++ {
		base := 12 + int(i)*16
		entries[i] = emojiEntry{
			r:       rune(binary.LittleEndian.Uint32(data[base : base+4])),
			pngOff:  binary.LittleEndian.Uint32(data[base+4 : base+8]),
			pngLen:  binary.LittleEndian.Uint32(data[base+8 : base+12]),
			xOffset: int8(data[base+12]),
			yOffset: int8(data[base+13]),
			advance: data[base+14],
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].r < entries[j].r
	})

	return &emojiIndex{
		buf:     data,
		entries: entries,
		pngBase: pngBase,
		srcSize: srcSize,
	}, nil
}

func (idx *emojiIndex) find(r rune) (emojiEntry, bool) {
	n := len(idx.entries)
	i := sort.Search(n, func(i int) bool {
		return idx.entries[i].r >= r
	})
	if i < n && idx.entries[i].r == r {
		return idx.entries[i], true
	}
	return emojiEntry{}, false
}

type EmojiFace struct {
	index *emojiIndex
	cellW int
	cellH int
	cache map[rune]*Glyph
}

func NewEmojiFace(cellW, cellH int) (*EmojiFace, error) {
	emojiIdxOnce.Do(func() {
		emojiIdx, emojiIdxErr = loadEmojiIndex()
	})
	if emojiIdxErr != nil {
		return nil, emojiIdxErr
	}
	return &EmojiFace{
		index: emojiIdx,
		cellW: cellW,
		cellH: cellH,
		cache: make(map[rune]*Glyph),
	}, nil
}

func (e *EmojiFace) Resize(cellW, cellH int) {
	e.cellW = cellW
	e.cellH = cellH
	e.cache = make(map[rune]*Glyph)
}

func (e *EmojiFace) Glyph(r rune) (*Glyph, error) {
	if g, ok := e.cache[r]; ok {
		return g, nil
	}
	entry, ok := e.index.find(r)
	if !ok {
		return nil, nil
	}
	pngStart := e.index.pngBase + int(entry.pngOff)
	pngEnd := pngStart + int(entry.pngLen)
	if pngEnd > len(e.index.buf) {
		return nil, nil
	}
	img, err := png.Decode(bytes.NewReader(e.index.buf[pngStart:pngEnd]))
	if err != nil {
		return nil, fmt.Errorf("emoji U+%X png decode: %w", r, err)
	}

	dstW := e.cellW * 2
	dstH := e.cellH
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)

	g := &Glyph{
		Rune:    r,
		Bitmap:  dst.Pix,
		Width:   dstW,
		Height:  dstH,
		XOffset: 0,
		YOffset: 0,
		Advance: dstW,
		IsColor: true,
	}
	e.cache[r] = g
	return g, nil
}
