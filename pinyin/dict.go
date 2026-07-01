package pinyin

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
)

//go:embed data/dict.bin.gz
var dictData []byte

type dictEntry struct {
	word   string
	weight int
}

func loadDict() (map[string][]dictEntry, error) {
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
	off := 12
	if uint32(len(data))-12 < keyPoolSize+wordPoolSize+count*12 {
		return nil, fmt.Errorf("dict.bin truncated: need %d bytes, got %d",
			12+keyPoolSize+wordPoolSize+count*12, len(data))
	}
	keyPool := data[off : off+int(keyPoolSize)]
	off += int(keyPoolSize)
	wordPool := data[off : off+int(wordPoolSize)]
	off += int(wordPoolSize)

	result := make(map[string][]dictEntry, count)
	for i := uint32(0); i < count; i++ {
		base := off + int(i*12)
		keyOff := binary.LittleEndian.Uint32(data[base : base+4])
		wordOff := binary.LittleEndian.Uint32(data[base+4 : base+8])
		weight := int(binary.LittleEndian.Uint32(data[base+8 : base+12]))

		key := readCString(keyPool, int(keyOff))
		word := readWord(wordPool, int(wordOff))
		result[key] = append(result[key], dictEntry{word: word, weight: weight})
	}
	return result, nil
}

func readCString(pool []byte, off int) string {
	end := off
	for end < len(pool) && pool[end] != 0 {
		end++
	}
	return string(pool[off:end])
}

func readWord(pool []byte, off int) string {
	if off+2 > len(pool) {
		return ""
	}
	length := int(binary.LittleEndian.Uint16(pool[off : off+2]))
	start := off + 2
	if start+length > len(pool) {
		return ""
	}
	return string(pool[start : start+length])
}
