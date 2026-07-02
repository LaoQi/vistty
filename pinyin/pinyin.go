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

var globalDict map[string][]dictEntry

func Init() {
	d, err := loadDict()
	if err != nil {
		d = make(map[string][]dictEntry)
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
	merged := make(map[string]*seen)
	for _, split := range splits {
		key := strings.Join(split, "")
		extraSyllables := len(split) - minSyllables
		splitFactor := 1.0
		for i := 0; i < extraSyllables; i++ {
			splitFactor *= 0.1
		}
		entries, ok := globalDict[key]
		if ok {
			code := strings.Join(split, " ")
			for _, e := range entries {
				w := e.weight
				if partial != "" {
					w = int(float64(w) * fuzzyWeightFactor)
				}
				if s, ok := merged[e.word]; ok {
					if w > s.weight {
						s.weight = w
					}
					continue
				}
				merged[e.word] = &seen{word: e.word, weight: w, code: code}
			}
		}
		if combos := composeFromSingleChars(split); len(combos) > 0 {
			for _, combo := range combos {
				w := int(float64(combo.weight) * splitFactor)
				if partial != "" {
					w = int(float64(w) * fuzzyWeightFactor)
				}
				if _, exists := merged[combo.word]; !exists {
					combo.weight = w
					merged[combo.word] = combo
				}
			}
		}
	}

	list := make([]*seen, 0, len(merged))
	for _, s := range merged {
		list = append(list, s)
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
		entries, ok := globalDict[syl]
		if !ok || len(entries) == 0 {
			return nil
		}
		top := entries[0]
		for _, e := range entries[1:] {
			if e.weight > top.weight {
				top = e
			}
		}
		var cands []charCand
		cands = append(cands, charCand{word: top.word, weight: top.weight})
		for _, e := range entries {
			if len([]rune(e.word)) != 1 || e.word == top.word {
				continue
			}
			cands = append(cands, charCand{word: e.word, weight: e.weight})
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
