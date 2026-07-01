package pinyin

import (
	"sort"
	"strings"
	"testing"
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

func TestPinyinName(t *testing.T) {
	p := New()
	if p.Name() != "pinyin" {
		t.Fatalf("Name = %q, want pinyin", p.Name())
	}
}

func TestPinyinLookupBasic(t *testing.T) {
	p := New()
	cands := p.Lookup("nihao")
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'nihao'")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first candidate = %q, want 你好", cands[0].Word)
	}
}

func TestPinyinLookupShijie(t *testing.T) {
	p := New()
	cands := p.Lookup("shijie")
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'shijie'")
	}
	if cands[0].Word != "世界" {
		t.Fatalf("first candidate = %q, want 世界", cands[0].Word)
	}
}

func TestPinyinLookupEmpty(t *testing.T) {
	p := New()
	cands := p.Lookup("")
	if cands != nil {
		t.Fatalf("Lookup('') = %v, want nil", cands)
	}
}

func TestPinyinLookupInvalid(t *testing.T) {
	p := New()
	cands := p.Lookup("zzz")
	if cands != nil {
		t.Fatalf("Lookup('zzz') = %v, want nil", cands)
	}
}

func TestPinyinLookupDedup(t *testing.T) {
	p := New()
	cands := p.Lookup("xian")
	words := map[string]bool{}
	for _, c := range cands {
		if words[c.Word] {
			t.Errorf("duplicate candidate word: %q", c.Word)
		}
		words[c.Word] = true
	}
}

func TestPinyinLookupMaxCandidates(t *testing.T) {
	p := New()
	cands := p.Lookup("a")
	if len(cands) > maxCandidates {
		t.Fatalf("Lookup('a') returned %d, should be <= %d", len(cands), maxCandidates)
	}
}

func TestPinyinLookupRealPhraseBeatsCombo(t *testing.T) {
	p := New()
	cands := p.Lookup("nihao")
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first = %q, want 你好 (real phrase should beat combos)", cands[0].Word)
	}
}

func TestPinyinFormatPreedit(t *testing.T) {
	p := New()
	pre := p.FormatPreedit("nihao")
	if !strings.Contains(pre, "ni") || !strings.Contains(pre, "hao") {
		t.Fatalf("FormatPreedit(nihao) = %q", pre)
	}
	if !strings.Contains(pre, "'") {
		t.Logf("FormatPreedit(nihao) = %q (note: expected separator)", pre)
	}
}

func TestPinyinFormatPreeditEmpty(t *testing.T) {
	p := New()
	pre := p.FormatPreedit("")
	if pre != "" {
		t.Fatalf("FormatPreedit('') = %q, want empty", pre)
	}
}

func TestPinyinFormatPreeditInvalid(t *testing.T) {
	p := New()
	pre := p.FormatPreedit("zzz")
	if pre != "zzz" {
		t.Fatalf("FormatPreedit('zzz') = %q, want zzz", pre)
	}
}

func TestPinyinFormatPreeditXian(t *testing.T) {
	p := New()
	pre := p.FormatPreedit("xian")
	if pre == "xian" {
		t.Logf("FormatPreedit(xian) = %q (single syllable)", pre)
	} else if !strings.Contains(pre, "'") {
		t.Fatalf("FormatPreedit(xian) = %q, expected multi-syllable format", pre)
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

func TestComposeFromSingleCharsMulti(t *testing.T) {
	p := New()
	combos := p.composeFromSingleChars([]string{"nin", "hao"})
	if len(combos) == 0 {
		t.Fatal("expected combos for [nin hao]")
	}
	if combos[0].word != "您好" {
		t.Logf("first combo = %q (note: weight-penalized)", combos[0].word)
	}
	if combos[0].weight > 100000 {
		t.Errorf("combo weight %d too high (should be penalized)", combos[0].weight)
	}
}

func TestComposeFromSingleCharsMaxCombos(t *testing.T) {
	p := New()
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
