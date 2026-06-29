package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type entry struct {
	word   string
	pinyin string
	weight int
}

type fileSpec struct {
	path      string
	topN      int
	annotate  string
}

func main() {
	out := flag.String("o", "", "output path")
	annotate := flag.String("annotate", "", "char dict yaml for annotating 2-column files")
	flag.Parse()

	args := flag.Args()
	if *out == "" || len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gen-dict -o <output> [-annotate char.yaml] file=topN [file=topN ...] or just files")
		fmt.Fprintln(os.Stderr, "  file=topN  e.g. /tmp/ext.dict.yaml=20000")
		fmt.Fprintln(os.Stderr, "  -annotate char dict for 2-column (text,weight) files")
		os.Exit(1)
	}

	var specs []fileSpec
	for _, a := range args {
		spec := fileSpec{path: a}
		if idx := strings.LastIndex(a, "="); idx > 0 {
			if n, err := strconv.Atoi(a[idx+1:]); err == nil {
				spec.path = a[:idx]
				spec.topN = n
			}
		}
		if *annotate != "" {
			spec.annotate = *annotate
		}
		specs = append(specs, spec)
	}

	var charMap map[string]string
	for _, s := range specs {
		if s.annotate != "" {
			if charMap == nil {
				cm, err := loadCharMap(s.annotate)
				if err != nil {
					fmt.Fprintf(os.Stderr, "load annotate %s: %v\n", s.annotate, err)
					os.Exit(1)
				}
				charMap = cm
			}
			break
		}
	}

	all := []entry{}
	for _, s := range specs {
		es, err := parseFile(s, charMap)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", s.path, err)
			os.Exit(1)
		}
		all = append(all, es...)
	}

	merged := mergeByKey(all)

	if err := writeDict(*out, merged); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d entries to %s\n", len(merged), *out)
}

func loadCharMap(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	inHeader := true
	type cand struct {
		pinyin string
		weight int
	}
	tmp := make(map[string][]cand)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimRight(line, "\r\n")
			if inHeader {
				if trimmed == "..." {
					inHeader = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "#") || trimmed == "" {
				continue
			}
			parts := strings.Split(trimmed, "\t")
			if len(parts) != 3 {
				continue
			}
			word, py, ws := parts[0], parts[1], parts[2]
			if len(word) != 1 || strings.Contains(py, " ") {
				continue
			}
			w, err := strconv.Atoi(strings.TrimSpace(ws))
			if err != nil {
				continue
			}
			tmp[word] = append(tmp[word], cand{pinyin: py, weight: w})
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	result := make(map[string]string, len(tmp))
	for c, cs := range tmp {
		best := cs[0]
		for _, x := range cs[1:] {
			if x.weight > best.weight {
				best = x
			}
		}
		result[c] = best.pinyin
	}
	return result, nil
}

func parseFile(s fileSpec, charMap map[string]string) ([]entry, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	inHeader := true
	var entries []entry
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimRight(line, "\r\n")
			if inHeader {
				if trimmed == "..." {
					inHeader = false
				}
				continue
			}
			if strings.HasPrefix(trimmed, "#") || trimmed == "" {
				continue
			}
			if e, ok := parseLine(trimmed, charMap); ok {
				entries = append(entries, e)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	if s.topN > 0 && s.topN < len(entries) {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].weight != entries[j].weight {
				return entries[i].weight > entries[j].weight
			}
			return entries[i].word < entries[j].word
		})
		entries = entries[:s.topN]
	}
	return entries, nil
}

func parseLine(line string, charMap map[string]string) (entry, bool) {
	parts := strings.Split(line, "\t")
	var word, pinyin string
	var weight int

	switch len(parts) {
	case 3:
		word = parts[0]
		pinyin = parts[1]
		w, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return entry{}, false
		}
		weight = w
	case 2:
		word = parts[0]
		w, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return entry{}, false
		}
		weight = w
		if charMap != nil {
			pys := make([]string, 0, len(word))
			ok := true
			for _, c := range word {
				py, found := charMap[string(c)]
				if !found {
					ok = false
					break
				}
				pys = append(pys, py)
			}
			if !ok {
				return entry{}, false
			}
			pinyin = strings.Join(pys, " ")
		}
	default:
		return entry{}, false
	}

	if word == "" {
		return entry{}, false
	}
	return entry{word: word, pinyin: pinyin, weight: weight}, true
}

type keyEntries struct {
	key     string
	entries []entry
}

func mergeByKey(all []entry) []keyEntries {
	m := make(map[string][]entry)
	keys := []string{}
	seen := make(map[string]bool)
	for _, e := range all {
		key := strings.ReplaceAll(e.pinyin, " ", "")
		if key == "" {
			continue
		}
		dedupKey := key + "\x00" + e.word
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		if _, ok := m[key]; !ok {
			keys = append(keys, key)
		}
		m[key] = append(m[key], e)
	}

	result := make([]keyEntries, 0, len(keys))
	for _, key := range keys {
		es := m[key]
		sort.Slice(es, func(i, j int) bool {
			if es[i].weight != es[j].weight {
				return es[i].weight > es[j].weight
			}
			return es[i].word < es[j].word
		})
		result = append(result, keyEntries{key: key, entries: es})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].key < result[j].key
	})
	return result
}

func writeDict(path string, merged []keyEntries) error {
	var totalEntries int
	for _, ke := range merged {
		totalEntries += len(ke.entries)
	}

	keyPool := []byte{}
	keyOffsets := make(map[string]uint32)
	for _, ke := range merged {
		keyOffsets[ke.key] = uint32(len(keyPool))
		keyPool = append(keyPool, []byte(ke.key)...)
		keyPool = append(keyPool, 0)
	}

	wordPool := []byte{}
	wordOffsets := make(map[string]uint32)
	getWordOff := func(word string) uint32 {
		if off, ok := wordOffsets[word]; ok {
			return off
		}
		off := uint32(len(wordPool))
		wordOffsets[word] = off
		var l [2]byte
		binary.LittleEndian.PutUint16(l[:], uint16(len(word)))
		wordPool = append(wordPool, l[:]...)
		wordPool = append(wordPool, []byte(word)...)
		return off
	}

	type flatEntry struct {
		keyOff  uint32
		wordOff uint32
		weight  uint32
	}
	flat := make([]flatEntry, 0, totalEntries)
	for _, ke := range merged {
		keyOff := keyOffsets[ke.key]
		for _, e := range ke.entries {
			flat = append(flat, flatEntry{
				keyOff:  keyOff,
				wordOff: getWordOff(e.word),
				weight:  uint32(e.weight),
			})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(flat)))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(keyPool)))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(len(wordPool)))
	if _, err := f.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := f.Write(keyPool); err != nil {
		return err
	}
	if _, err := f.Write(wordPool); err != nil {
		return err
	}
	buf := make([]byte, 12)
	for _, fe := range flat {
		binary.LittleEndian.PutUint32(buf[0:4], fe.keyOff)
		binary.LittleEndian.PutUint32(buf[4:8], fe.wordOff)
		binary.LittleEndian.PutUint32(buf[8:12], fe.weight)
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
