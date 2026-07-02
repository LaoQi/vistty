package pinyin

import (
	"sort"
	"strings"
)

const maxCandidates = 256

type Candidate struct {
	Word string
	Code string
}

var globalDict *dictIndex

func Init() {
	d, err := loadDict()
	if err != nil {
		d = &dictIndex{}
	}
	globalDict = d
}

func Lookup(input string) []Candidate {
	if globalDict == nil {
		return nil
	}
	splits, partial := SplitFuzzy(input)
	if len(splits) == 0 {
		return nil
	}

	const fuzzyWeightFactor = 0.5
	minSyllables := len(splits[0])
	merged := make(map[string]int, 128)
	list := make([]seen, 0, 256)
	for _, split := range splits {
		key := strings.Join(split, "")
		extraSyllables := len(split) - minSyllables
		splitFactor := 1.0
		for i := 0; i < extraSyllables; i++ {
			splitFactor *= 0.1
		}
		start, count, ok := globalDict.findKey(key)
		if ok {
			code := strings.Join(split, " ")
			for i := uint32(0); i < count; i++ {
				wordOff, ew := globalDict.readEntry(start + i)
				word := globalDict.readWord(wordOff)
				w := int(ew)
				if partial != "" {
					w = int(float64(w) * fuzzyWeightFactor)
				}
				if idx, ok := merged[word]; ok {
					if w > list[idx].weight {
						list[idx].weight = w
					}
					continue
				}
				list = append(list, seen{word: word, weight: w, code: code})
				merged[word] = len(list) - 1
			}
		}
		if combos := composeFromSingleChars(split); len(combos) > 0 {
			for _, combo := range combos {
				w := int(float64(combo.weight) * splitFactor)
				if partial != "" {
					w = int(float64(w) * fuzzyWeightFactor)
				}
				if _, exists := merged[combo.word]; !exists {
					list = append(list, seen{word: combo.word, weight: w, code: combo.code})
					merged[combo.word] = len(list) - 1
				}
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		return list[i].word < list[j].word
	})

	if len(list) > maxCandidates {
		list = list[:maxCandidates]
	}
	cands := make([]Candidate, len(list))
	for i, s := range list {
		cands[i] = Candidate{Word: s.word, Code: s.code}
	}
	return cands
}

func FormatPreedit(input string) string {
	if len(input) == 0 {
		return ""
	}
	n := len(input)
	for cut := n; cut >= 1; cut-- {
		if strict := Split(input[:cut]); len(strict) > 0 {
			best := strict[0]
			for _, s := range strict[1:] {
				if len(s) < len(best) {
					best = s
				}
			}
			formatted := strings.Join(best, "'")
			if cut < n {
				formatted += "'" + input[cut:]
			}
			return formatted
		}
	}
	return input
}

type seen struct {
	word   string
	weight int
	code   string
}

func composeFromSingleChars(split []string) []*seen {
	if len(split) < 2 {
		return nil
	}

	n := len(split)
	k := 3
	if n >= 4 {
		k = 2
	}

	type charCand struct {
		word   string
		weight int
	}
	perSyllable := make([][]charCand, n)
	for i, syl := range split {
		start, count, ok := globalDict.findKey(syl)
		if !ok || count == 0 {
			return nil
		}
		topOff, topWeight := globalDict.readEntry(start)
		topWord := globalDict.readWord(topOff)
		for j := uint32(1); j < count; j++ {
			wordOff, w := globalDict.readEntry(start + j)
			if w > topWeight {
				topWeight = w
				topWord = globalDict.readWord(wordOff)
			}
		}
		var cands []charCand
		cands = append(cands, charCand{word: topWord, weight: int(topWeight)})
		for j := uint32(0); j < count; j++ {
			wordOff, w := globalDict.readEntry(start + j)
			word := globalDict.readWord(wordOff)
			if len([]rune(word)) != 1 || word == topWord {
				continue
			}
			cands = append(cands, charCand{word: word, weight: int(w)})
			if len(cands) >= k {
				break
			}
		}
		perSyllable[i] = cands
	}

	const maxCombos = 50
	var results []*seen
	var build func(i int, word []string, minW int)
	build = func(i int, word []string, minW int) {
		if len(results) >= maxCombos {
			return
		}
		if i == n {
			w := strings.Join(word, "")
			if w == "" {
				return
			}
			results = append(results, &seen{
				word:   w,
				weight: minW / 100,
				code:   strings.Join(split, " "),
			})
			return
		}
		for _, c := range perSyllable[i] {
			mw := minW
			if c.weight < mw {
				mw = c.weight
			}
			build(i+1, append(word, c.word), mw)
			if len(results) >= maxCombos {
				return
			}
		}
	}
	build(0, make([]string, 0, n), int(^uint(0)>>1))
	return results
}
