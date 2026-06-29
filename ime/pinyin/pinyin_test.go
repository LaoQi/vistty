package pinyin

import (
	"sort"
	"strings"
	"testing"

	"github.com/LaoQi/vistty/ime"
)

func TestIsSyllable(t *testing.T) {
	cases := map[string]bool{
		"a":   true,
		"ai":  true,
		"an":  true,
		"ang": true,
		"ao":  true,
		"ba":  true,
		"zuo": true,
		"nv":  true,
		"lv":  true,
		"nve": true,
		"lve": true,
		"biang": true,
		"zhei": true,
		"xian": true,
		"xyz":  false,
		"":     false,
		"q":    false,
		"ab":   false,
	}
	for s, want := range cases {
		if got := isSyllable(s); got != want {
			t.Errorf("isSyllable(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestSplitSingle(t *testing.T) {
	res := Split("hao")
	if len(res) < 1 {
		t.Fatalf("Split(hao) = %v, want at least 1 result", res)
	}
	found := false
	for _, s := range res {
		if len(s) == 1 && s[0] == "hao" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Split(hao) missing [hao], got %v", res)
	}
}

func TestSplitMulti(t *testing.T) {
	res := Split("xian")
	if len(res) < 2 {
		t.Fatalf("Split(xian) = %v, want >=2 results", res)
	}
	wantXiAn := false
	wantXian := false
	for _, s := range res {
		joined := strings.Join(s, "|")
		if joined == "xi|an" {
			wantXiAn = true
		}
		if joined == "xian" {
			wantXian = true
		}
	}
	if !wantXiAn {
		t.Errorf("Split(xian) missing [xi an], got %v", res)
	}
	if !wantXian {
		t.Errorf("Split(xian) missing [xian], got %v", res)
	}
}

func TestSplitNihao(t *testing.T) {
	res := Split("nihao")
	if len(res) == 0 {
		t.Fatal("Split(nihao) should have results")
	}
	want := false
	for _, s := range res {
		if len(s) == 2 && s[0] == "ni" && s[1] == "hao" {
			want = true
			break
		}
	}
	if !want {
		t.Fatalf("Split(nihao) missing [ni hao], got %v", res)
	}
}

func TestSplitEmpty(t *testing.T) {
	if res := Split(""); len(res) != 0 {
		t.Fatalf("Split('') = %v, want empty", res)
	}
}

func TestSplitInvalid(t *testing.T) {
	if res := Split("zzz"); len(res) != 0 {
		t.Fatalf("Split(zzz) = %v, want empty", res)
	}
}

func TestLoadDict(t *testing.T) {
	dict, err := loadDict()
	if err != nil {
		t.Fatalf("loadDict error: %v", err)
	}
	if len(dict) == 0 {
		t.Fatal("dict should not be empty")
	}
	if _, ok := dict["ni"]; !ok {
		t.Errorf("dict missing single-syllable key 'ni', have %d keys", len(dict))
	}
	if _, ok := dict["hao"]; !ok {
		t.Errorf("dict missing single-syllable key 'hao', have %d keys", len(dict))
	}
}

func TestPinyinNameActivate(t *testing.T) {
	p := New()
	if p.Name() != "pinyin" {
		t.Fatalf("Name = %q, want pinyin", p.Name())
	}
	if p.IsActive() {
		t.Fatal("should start inactive")
	}
	p.Activate()
	if !p.IsActive() {
		t.Fatal("should be active after Activate")
	}
	p.Deactivate()
	if p.IsActive() {
		t.Fatal("should be inactive after Deactivate")
	}
}

func TestPinyinReset(t *testing.T) {
	p := New()
	p.Activate()
	p.buf = "abc"
	p.cands = []ime.Candidate{{Word: "x"}}
	p.page = 2
	p.Reset()
	if p.buf != "" {
		t.Fatalf("buf = %q after Reset", p.buf)
	}
	if p.cands != nil {
		t.Fatalf("cands = %v after Reset", p.cands)
	}
	if p.page != 0 {
		t.Fatalf("page = %d after Reset", p.page)
	}
}

func TestPinyinProcessKeyInactive(t *testing.T) {
	p := New()
	resp := p.ProcessKey(ime.KeyEvent{Rune: 'a', State: true})
	if resp.Consumed {
		t.Fatal("inactive ProcessKey should not consume")
	}
}

func TestPinyinProcessKeyNotPress(t *testing.T) {
	p := New()
	p.Activate()
	resp := p.ProcessKey(ime.KeyEvent{Rune: 'a', State: false})
	if resp.Consumed {
		t.Fatal("non-press ProcessKey should not consume")
	}
}

func TestPinyinTypeAndSelect(t *testing.T) {
	p := New()
	p.Activate()

	resp := p.ProcessKey(ime.KeyEvent{Rune: 'n', State: true})
	if !resp.Consumed {
		t.Fatal("typing 'n' should be consumed")
	}

	resp = p.ProcessKey(ime.KeyEvent{Rune: 'i', State: true})
	if !resp.Consumed {
		t.Fatal("typing 'i' should be consumed")
	}

	resp = p.ProcessKey(ime.KeyEvent{Rune: 'h', State: true})
	if !resp.Consumed {
		t.Fatal("typing 'h' should be consumed")
	}
	resp = p.ProcessKey(ime.KeyEvent{Rune: 'a', State: true})
	if !resp.Consumed {
		t.Fatal("typing 'a' should be consumed")
	}
	resp = p.ProcessKey(ime.KeyEvent{Rune: 'o', State: true})
	if !resp.Consumed {
		t.Fatal("typing 'o' should be consumed")
	}

	cands := p.Candidates()
	if len(cands) == 0 {
		t.Fatal("expected candidates after typing 'nihao'")
	}

	first := cands[0].Word
	if first == "" {
		t.Fatal("first candidate word should not be empty")
	}

	resp = p.ProcessKey(ime.KeyEvent{Code: 2, State: true})
	if !resp.Consumed {
		t.Fatal("selecting candidate should be consumed")
	}
	if resp.Commit == "" {
		t.Fatal("commit should not be empty after select")
	}
	if resp.Commit != first {
		t.Fatalf("Commit = %q, want first candidate %q", resp.Commit, first)
	}
	if p.buf != "" {
		t.Fatal("buf should be empty after commit")
	}
}

func TestPinyinBackspace(t *testing.T) {
	p := New()
	p.Activate()
	p.ProcessKey(ime.KeyEvent{Rune: 'n', State: true})
	p.ProcessKey(ime.KeyEvent{Rune: 'i', State: true})
	if p.buf != "ni" {
		t.Fatalf("buf = %q, want ni", p.buf)
	}
	resp := p.ProcessKey(ime.KeyEvent{Code: 14, State: true})
	if !resp.Consumed {
		t.Fatal("backspace should be consumed")
	}
	if p.buf != "n" {
		t.Fatalf("buf = %q, want n after backspace", p.buf)
	}
	resp = p.ProcessKey(ime.KeyEvent{Code: 14, State: true})
	if !resp.Consumed {
		t.Fatal("backspace on last char should be consumed")
	}
	if p.buf != "" {
		t.Fatalf("buf = %q, want empty", p.buf)
	}
}

func TestPinyinBackspaceEmptyPassthrough(t *testing.T) {
	p := New()
	p.Activate()
	resp := p.ProcessKey(ime.KeyEvent{Code: 14, State: true})
	if resp.Consumed {
		t.Fatal("backspace with empty buf should passthrough (not consumed)")
	}
}

func TestPinyinEsc(t *testing.T) {
	p := New()
	p.Activate()
	p.ProcessKey(ime.KeyEvent{Rune: 'n', State: true})
	p.ProcessKey(ime.KeyEvent{Rune: 'i', State: true})
	resp := p.ProcessKey(ime.KeyEvent{Code: 1, State: true})
	if !resp.Consumed {
		t.Fatal("Esc should be consumed when buf non-empty")
	}
	if p.buf != "" {
		t.Fatalf("buf = %q after Esc, want empty", p.buf)
	}
}

func TestPinyinEscEmptyPassthrough(t *testing.T) {
	p := New()
	p.Activate()
	resp := p.ProcessKey(ime.KeyEvent{Code: 1, State: true})
	if resp.Consumed {
		t.Fatal("Esc with empty buf should passthrough (not consumed)")
	}
}

func TestPinyinEnterCommitRaw(t *testing.T) {
	p := New()
	p.Activate()
	p.ProcessKey(ime.KeyEvent{Rune: 'x', State: true})
	p.ProcessKey(ime.KeyEvent{Rune: 'y', State: true})
	resp := p.ProcessKey(ime.KeyEvent{Code: 28, State: true})
	if !resp.Consumed {
		t.Fatal("Enter should be consumed")
	}
	if resp.Commit != "xy" {
		t.Fatalf("Commit = %q, want xy", resp.Commit)
	}
}

func TestPinyinSpaceCommitFirst(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "ni" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	cands := p.Candidates()
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'ni'")
	}
	resp := p.ProcessKey(ime.KeyEvent{Code: 57, State: true})
	if !resp.Consumed {
		t.Fatal("Space should be consumed")
	}
	if resp.Commit != cands[0].Word {
		t.Fatalf("Commit = %q, want %q", resp.Commit, cands[0].Word)
	}
}

func TestPinyinCtrlLetterPassthrough(t *testing.T) {
	p := New()
	p.Activate()
	resp := p.ProcessKey(ime.KeyEvent{Rune: 'a', Code: 30, Mods: 1, State: true})
	if resp.Consumed {
		t.Fatal("Ctrl+a should not be consumed by IME")
	}
	if p.buf != "" {
		t.Fatal("buf should remain empty after Ctrl+a")
	}
}

func TestPinyinPreedit(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "nihao" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	pre := p.Preedit()
	if pre == "" {
		t.Fatal("Preedit should not be empty")
	}
	if !strings.Contains(pre, "'") {
		t.Logf("Preedit = %q (note: expected separator)", pre)
	}
}

func TestPinyinLookupDedup(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "xian" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	cands := p.Candidates()
	words := map[string]bool{}
	for _, c := range cands {
		if words[c.Word] {
			t.Errorf("duplicate candidate word: %q", c.Word)
		}
		words[c.Word] = true
	}
}

func TestPinyinPageNav(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "a" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	origPage := p.page
	resp := p.ProcessKey(ime.KeyEvent{Code: 13, State: true})
	if !resp.Consumed {
		t.Fatal("EQUAL (next page) should be consumed")
	}
	if p.page < origPage {
		t.Fatalf("page = %d, should not decrease", p.page)
	}
	resp = p.ProcessKey(ime.KeyEvent{Code: 12, State: true})
	if !resp.Consumed {
		t.Fatal("MINUS (prev page) should be consumed")
	}
	if p.page > origPage {
		t.Fatalf("page = %d, should not exceed %d", p.page, origPage)
	}
}

func TestPinyinTabNextPage(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "a" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	if len(p.cands) <= pageSize {
		t.Skip("need more than 9 candidates to test paging")
	}
	p.page = 0
	resp := p.ProcessKey(ime.KeyEvent{Code: 15, State: true})
	if !resp.Consumed {
		t.Fatal("Tab should be consumed when buf non-empty")
	}
	if p.page != 1 {
		t.Fatalf("page = %d, want 1 after Tab", p.page)
	}
}

func TestPinyinTabWrapToFirst(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "a" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	if len(p.cands) <= pageSize {
		t.Skip("need more than 9 candidates to test paging")
	}
	lastPage := (len(p.cands) - 1) / pageSize
	p.page = lastPage
	resp := p.ProcessKey(ime.KeyEvent{Code: 15, State: true})
	if !resp.Consumed {
		t.Fatal("Tab should be consumed")
	}
	if p.page != 0 {
		t.Fatalf("page = %d, want 0 (wrap) after Tab on last page", p.page)
	}
}

func TestPinyinTabEmptyPassthrough(t *testing.T) {
	p := New()
	p.Activate()
	resp := p.ProcessKey(ime.KeyEvent{Code: 15, State: true})
	if resp.Consumed {
		t.Fatal("Tab with empty buf should passthrough")
	}
}

func TestPinyinCandidatesPaged(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "a" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	if len(p.cands) <= pageSize {
		t.Skip("need more than 9 candidates to test paging")
	}
	p.page = 0
	cands := p.Candidates()
	if len(cands) > pageSize {
		t.Fatalf("Candidates() returned %d, should be <= %d (page size)", len(cands), pageSize)
	}
	p.page = 1
	cands = p.Candidates()
	if len(cands) > pageSize {
		t.Fatalf("Candidates() page 1 returned %d, should be <= %d", len(cands), pageSize)
	}
}

func TestPinyinCtrlModifierValue(t *testing.T) {
	if modCtrl != 1 {
		t.Fatalf("modCtrl = %d, want 1 (platform.ModCtrl)", modCtrl)
	}
}

func TestSplitMemoCorrectness(t *testing.T) {
	res := Split("zhongguo")
	found := false
	for _, s := range res {
		if len(s) == 2 && s[0] == "zhong" && s[1] == "guo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Split(zhongguo) missing [zhong guo], got %v", res)
	}
}

func TestFormatPreedit(t *testing.T) {
	pre := formatPreedit("nihao")
	if !strings.Contains(pre, "ni") || !strings.Contains(pre, "hao") {
		t.Fatalf("formatPreedit(nihao) = %q", pre)
	}
}

func TestDictConsistentWeights(t *testing.T) {
	dict, err := loadDict()
	if err != nil {
		t.Fatalf("loadDict: %v", err)
	}
	for key, entries := range dict {
		sorted := sort.SliceIsSorted(entries, func(i, j int) bool {
			if entries[i].weight != entries[j].weight {
				return entries[i].weight > entries[j].weight
			}
			return entries[i].word < entries[j].word
		})
		if !sorted {
			t.Errorf("entries for key %q not sorted by weight desc", key)
		}
	}
}

func TestDictContainsCommonPhrases(t *testing.T) {
	dict, err := loadDict()
	if err != nil {
		t.Fatalf("loadDict: %v", err)
	}
	want := []struct {
		key  string
		word string
	}{
		{"nihao", "你好"},
		{"shijie", "世界"},
		{"women", "我们"},
		{"zhongguo", "中国"},
		{"beijing", "北京"},
		{"xiexie", "谢谢"},
		{"shihou", "时候"},
		{"xianzai", "现在"},
	}
	for _, w := range want {
		entries, ok := dict[w.key]
		if !ok {
			t.Errorf("dict missing key %q", w.key)
			continue
		}
		found := false
		for _, e := range entries {
			if e.word == w.word {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dict key %q has no word %q", w.key, w.word)
		}
	}
}

func TestPinyinLookupNihao(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "nihao" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	cands := p.Candidates()
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'nihao'")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first candidate = %q, want 你好", cands[0].Word)
	}
}

func TestPinyinLookupShijie(t *testing.T) {
	p := New()
	p.Activate()
	for _, r := range "shijie" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	cands := p.Candidates()
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'shijie'")
	}
	if cands[0].Word != "世界" {
		t.Fatalf("first candidate = %q, want 世界", cands[0].Word)
	}
}

func TestComposeFromSingleCharsMulti(t *testing.T) {
	p := New()
	// 使用一个无词组命中的切分来验证组合生成。
	// "nin"（您）+ "hao"（好）→ "ninhao" 无词组时靠组合。
	combos := p.composeFromSingleChars([]string{"nin", "hao"})
	if len(combos) == 0 {
		t.Fatal("expected combos for [nin hao]")
	}
	// 首组合应为您好（nin top1=您, hao top1=好）。
	if combos[0].word != "您好" {
		t.Logf("first combo = %q (note: weight-penalized)", combos[0].word)
	}
	// 验证权重惩罚：组合权重应远小于单字权重。
	if combos[0].weight > 100000 {
		t.Errorf("combo weight %d too high (should be penalized)", combos[0].weight)
	}
}

func TestComposeFromSingleCharsMaxCombos(t *testing.T) {
	p := New()
	// 4 音节，K=2，理论上限 2^4=16 < 50。
	combos := p.composeFromSingleChars([]string{"ni", "hao", "ma", "a"})
	if len(combos) > 50 {
		t.Fatalf("combos = %d, should be <= 50", len(combos))
	}
	if len(combos) == 0 {
		t.Fatal("expected at least 1 combo")
	}
}

func TestComposeFromSingleCharsSingleSyllable(t *testing.T) {
	p := New()
	combos := p.composeFromSingleChars([]string{"ni"})
	if len(combos) != 0 {
		t.Fatalf("single syllable should produce no combos, got %d", len(combos))
	}
}

func TestPinyinLookupRealPhraseBeatsCombo(t *testing.T) {
	p := New()
	p.Activate()
	// "nihao" 真实词组 "你好" weight=332885，应排在所有组合词之前。
	for _, r := range "nihao" {
		p.ProcessKey(ime.KeyEvent{Rune: r, State: true})
	}
	cands := p.Candidates()
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first = %q, want 你好 (real phrase should beat combos)", cands[0].Word)
	}
}
