package pinyin

import (
	"sort"
	"strings"

	"github.com/LaoQi/vistty/ime"
)

const pageSize = 9

const (
	codeBackspace = 14
	codeEnter     = 28
	codeEsc       = 1
	codeSpace     = 57
	codeTab       = 15
	codeMinus     = 12
	codeEqual     = 13
	codeLeft      = 105
	codeRight     = 106
)

const modCtrl = 1

type Pinyin struct {
	dict   map[string][]dictEntry
	buf    string
	cands  []ime.Candidate
	page   int
	active bool
}

func New() *Pinyin {
	dict, err := loadDict()
	if err != nil {
		dict = make(map[string][]dictEntry)
	}
	return &Pinyin{dict: dict}
}

func (p *Pinyin) Name() string { return "pinyin" }

func (p *Pinyin) Activate() {
	p.active = true
	p.Reset()
}

func (p *Pinyin) Deactivate() {
	p.active = false
	p.Reset()
}

func (p *Pinyin) IsActive() bool { return p.active }

func (p *Pinyin) Reset() {
	p.buf = ""
	p.cands = nil
	p.page = 0
}

func (p *Pinyin) Preedit() string {
	if p.buf == "" {
		return ""
	}
	return formatPreedit(p.buf)
}

func (p *Pinyin) Candidates() []ime.Candidate {
	return p.pageCandidates()
}

func (p *Pinyin) pageCandidates() []ime.Candidate {
	if len(p.cands) == 0 {
		return nil
	}
	start := p.page * pageSize
	if start >= len(p.cands) {
		return nil
	}
	end := start + pageSize
	if end > len(p.cands) {
		end = len(p.cands)
	}
	return p.cands[start:end]
}

func (p *Pinyin) ProcessKey(ev ime.KeyEvent) ime.Response {
	if !p.active || !ev.State {
		return ime.Response{}
	}

	if isLowerLetter(ev.Rune) && ev.Mods&modCtrl == 0 {
		p.buf += string(ev.Rune)
		p.page = 0
		p.lookup()
		return ime.Response{Consumed: true, Preedit: p.Preedit(), Candidates: p.pageCandidates()}
	}

	switch ev.Code {
	case codeBackspace:
		if len(p.buf) > 0 {
			r := []rune(p.buf)
			p.buf = string(r[:len(r)-1])
			p.page = 0
			if p.buf == "" {
				p.cands = nil
				return ime.Response{Consumed: true}
			}
			p.lookup()
			return ime.Response{Consumed: true, Preedit: p.Preedit(), Candidates: p.pageCandidates()}
		}
		return ime.Response{}

	case codeEnter:
		if p.buf == "" {
			return ime.Response{}
		}
		commit := p.buf
		p.Reset()
		return ime.Response{Consumed: true, Commit: commit}

	case codeEsc:
		if p.buf == "" {
			return ime.Response{}
		}
		p.Reset()
		return ime.Response{Consumed: true}

	case codeSpace:
		if p.buf == "" {
			return ime.Response{}
		}
		commit := p.firstCandidateWord()
		if commit == "" {
			commit = p.buf
		}
		p.Reset()
		return ime.Response{Consumed: true, Commit: commit}

	case 2, 3, 4, 5, 6, 7, 8, 9, 10:
		if len(p.buf) == 0 {
			return ime.Response{}
		}
		n := int(ev.Code) - 2
		idx := p.page*pageSize + n
		if idx >= len(p.cands) {
			return ime.Response{Consumed: true}
		}
		commit := p.cands[idx].Word
		p.Reset()
		return ime.Response{Consumed: true, Commit: commit}

	case codeMinus, codeLeft:
		if p.buf == "" {
			return ime.Response{}
		}
		if p.page > 0 {
			p.page--
		}
		return ime.Response{Consumed: true, Preedit: p.Preedit(), Candidates: p.pageCandidates()}

	case codeEqual, codeRight, codeTab:
		if p.buf == "" {
			return ime.Response{}
		}
		if (p.page+1)*pageSize < len(p.cands) {
			p.page++
		} else {
			p.page = 0
		}
		return ime.Response{Consumed: true, Preedit: p.Preedit(), Candidates: p.pageCandidates()}

	default:
		if p.buf == "" {
			return ime.Response{}
		}
		commit := p.firstCandidateWord()
		p.Reset()
		return ime.Response{Consumed: false, Commit: commit}
	}
}

func (p *Pinyin) firstCandidateWord() string {
	if len(p.cands) == 0 {
		return ""
	}
	return p.cands[0].Word
}

type seen struct {
	word   string
	weight int
	code   string
}

func (p *Pinyin) lookup() {
	splits := Split(p.buf)
	if len(splits) == 0 {
		p.cands = nil
		return
	}

	merged := make(map[string]*seen)
	for _, split := range splits {
		key := strings.Join(split, "")
		entries, ok := p.dict[key]
		if ok {
			code := strings.Join(split, " ")
			for _, e := range entries {
				if s, ok := merged[e.word]; ok {
					if e.weight > s.weight {
						s.weight = e.weight
					}
					continue
				}
				merged[e.word] = &seen{word: e.word, weight: e.weight, code: code}
			}
		}
		if combos := p.composeFromSingleChars(split); len(combos) > 0 {
			for _, combo := range combos {
				if _, exists := merged[combo.word]; !exists {
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

	if len(list) > pageSize*4 {
		list = list[:pageSize*4]
	}
	p.cands = make([]ime.Candidate, len(list))
	for i, s := range list {
		p.cands[i] = ime.Candidate{Word: s.word, Code: s.code}
	}
}

func formatPreedit(buf string) string {
	splits := Split(buf)
	if len(splits) == 0 {
		return buf
	}
	best := splits[0]
	for _, s := range splits[1:] {
		if len(s) < len(best) {
			best = s
		}
	}
	return strings.Join(best, "'")
}

func (p *Pinyin) composeFromSingleChars(split []string) []*seen {
	if len(split) < 2 {
		return nil
	}

	// 每音节取 top-K 单字，K 随音节数递减防止组合爆炸。
	n := len(split)
	k := 3
	if n >= 4 {
		k = 2
	}

	// 每音节的 top-K 单字候选。
	type charCand struct {
		word   string
		weight int
	}
	perSyllable := make([][]charCand, n)
	for i, syl := range split {
		entries, ok := p.dict[syl]
		if !ok || len(entries) == 0 {
			return nil
		}
		top := entries[0]
		for _, e := range entries[1:] {
			if e.weight > top.weight {
				top = e
			}
		}
		// 严格单字（UTF-8 rune 数 == 1）。
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

	// 笛卡尔积生成组合，总上限 50。
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
				weight: minW / 10,
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

func isLowerLetter(r rune) bool {
	return r >= 'a' && r <= 'z'
}
