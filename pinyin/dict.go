package pinyin

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"unsafe"
)

//go:embed data/dict.bin.gz
var dictData []byte

type entryRange struct {
	start uint32
	count uint32
}

type dictIndex struct {
	buf        []byte
	wordPool   []byte
	entries    []byte
	entryBase  int
	entryCount uint32
	keyOffsets []uint32
	keyRanges  []entryRange
}

func loadDict() (*dictIndex, error) {
	gz, err := gzip.NewReader(bytes.NewReader(dictData))
	if err != nil {
		return nil, fmt.Errorf("dict.bin.gz: %w", err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read dict.bin.gz: %w", err)
	}
	if len(data) < 12 {
		return nil, fmt.Errorf("dict.bin too small: %d bytes", len(data))
	}
	count := binary.LittleEndian.Uint32(data[0:4])
	keyPoolSize := binary.LittleEndian.Uint32(data[4:8])
	wordPoolSize := binary.LittleEndian.Uint32(data[8:12])
	if uint32(len(data))-12 < keyPoolSize+wordPoolSize+count*12 {
		return nil, fmt.Errorf("dict.bin truncated: need %d bytes, got %d",
			12+keyPoolSize+wordPoolSize+count*12, len(data))
	}

	off := 12
	keyPoolStart := off
	keyPoolEnd := off + int(keyPoolSize)
	wordPoolEnd := keyPoolEnd + int(wordPoolSize)
	entryBase := wordPoolEnd
	entryBytes := int(count) * 12

	d := &dictIndex{
		buf:        data,
		wordPool:   data[keyPoolEnd:wordPoolEnd],
		entries:    data[entryBase : entryBase+entryBytes],
		entryBase:  entryBase,
		entryCount: count,
	}

	if count > 0 {
		keyCap := 0
		for _, b := range data[keyPoolStart:keyPoolEnd] {
			if b == 0 {
				keyCap++
			}
		}
		d.keyOffsets = make([]uint32, 0, keyCap)
		d.keyRanges = make([]entryRange, 0, keyCap)

		var lastKeyOff uint32
		for i := uint32(0); i < count; i++ {
			base := entryBase + int(i*12)
			keyOff := binary.LittleEndian.Uint32(data[base : base+4])
			if i == 0 || keyOff != lastKeyOff {
				if len(d.keyRanges) > 0 {
					last := &d.keyRanges[len(d.keyRanges)-1]
					last.count = i - last.start
				}
				d.keyOffsets = append(d.keyOffsets, keyOff+uint32(keyPoolStart))
				d.keyRanges = append(d.keyRanges, entryRange{start: i})
				lastKeyOff = keyOff
			}
		}
		if len(d.keyRanges) > 0 {
			last := &d.keyRanges[len(d.keyRanges)-1]
			last.count = count - last.start
		}
	}

	return d, nil
}

func (d *dictIndex) keyString(i int) string {
	off := int(d.keyOffsets[i])
	end := off
	for end < len(d.buf) && d.buf[end] != 0 {
		end++
	}
	return unsafe.String(&d.buf[off], end-off)
}

func (d *dictIndex) keyAt(i int) string {
	return d.keyString(i)
}

func (d *dictIndex) findKey(key string) (start, count uint32, ok bool) {
	n := len(d.keyOffsets)
	i := sort.Search(n, func(i int) bool {
		return d.keyString(i) >= key
	})
	if i < n && d.keyString(i) == key {
		return d.keyRanges[i].start, d.keyRanges[i].count, true
	}
	return 0, 0, false
}

func (d *dictIndex) readEntry(idx uint32) (wordOff, weight uint32) {
	base := d.entryBase + int(idx*12)
	wordOff = binary.LittleEndian.Uint32(d.buf[base+4 : base+8])
	weight = binary.LittleEndian.Uint32(d.buf[base+8 : base+12])
	return
}

func (d *dictIndex) readWord(wordOff uint32) string {
	off := int(wordOff)
	if off+2 > len(d.wordPool) {
		return ""
	}
	length := int(binary.LittleEndian.Uint16(d.wordPool[off : off+2]))
	start := off + 2
	if start+length > len(d.wordPool) {
		return ""
	}
	return unsafe.String(&d.wordPool[start], length)
}

func (d *dictIndex) keyCount() int {
	return len(d.keyOffsets)
}
