package pinyin

import (
	"sort"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	Init()
	m.Run()
}

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
	if globalDict == nil {
		t.Fatal("globalDict should be initialized after Init()")
	}
	if len(globalDict) == 0 {
		t.Fatal("dict should not be empty")
	}
	if _, ok := globalDict["ni"]; !ok {
		t.Errorf("dict missing single-syllable key 'ni', have %d keys", len(globalDict))
	}
	if _, ok := globalDict["hao"]; !ok {
		t.Errorf("dict missing single-syllable key 'hao', have %d keys", len(globalDict))
	}
}

func TestLookupBasic(t *testing.T) {
	cands := Lookup("nihao")
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'nihao'")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first candidate = %q, want 你好", cands[0].Word)
	}
}

func TestLookupShijie(t *testing.T) {
	cands := Lookup("shijie")
	if len(cands) == 0 {
		t.Fatal("expected candidates for 'shijie'")
	}
	if cands[0].Word != "世界" {
		t.Fatalf("first candidate = %q, want 世界", cands[0].Word)
	}
}

func TestLookupEmpty(t *testing.T) {
	cands := Lookup("")
	if cands != nil {
		t.Fatalf("Lookup('') = %v, want nil", cands)
	}
}

func TestLookupInvalid(t *testing.T) {
	cands := Lookup("zzz")
	if cands != nil {
		t.Fatalf("Lookup('zzz') = %v, want nil", cands)
	}
}

func TestLookupDedup(t *testing.T) {
	cands := Lookup("xian")
	words := map[string]bool{}
	for _, c := range cands {
		if words[c.Word] {
			t.Errorf("duplicate candidate word: %q", c.Word)
		}
		words[c.Word] = true
	}
}

func TestLookupMaxCandidates(t *testing.T) {
	cands := Lookup("a")
	if len(cands) > maxCandidates {
		t.Fatalf("Lookup('a') returned %d, should be <= %d", len(cands), maxCandidates)
	}
}

func TestLookupRealPhraseBeatsCombo(t *testing.T) {
	cands := Lookup("nihao")
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	if cands[0].Word != "你好" {
		t.Fatalf("first = %q, want 你好 (real phrase should beat combos)", cands[0].Word)
	}
}

func TestFormatPreedit(t *testing.T) {
	pre := FormatPreedit("nihao")
	if !strings.Contains(pre, "ni") || !strings.Contains(pre, "hao") {
		t.Fatalf("FormatPreedit(nihao) = %q", pre)
	}
	if !strings.Contains(pre, "'") {
		t.Logf("FormatPreedit(nihao) = %q (note: expected separator)", pre)
	}
}

func TestFormatPreeditEmpty(t *testing.T) {
	pre := FormatPreedit("")
	if pre != "" {
		t.Fatalf("FormatPreedit('') = %q, want empty", pre)
	}
}

func TestFormatPreeditInvalid(t *testing.T) {
	pre := FormatPreedit("zzz")
	if pre != "zzz" {
		t.Fatalf("FormatPreedit('zzz') = %q, want zzz", pre)
	}
}

func TestFormatPreeditXian(t *testing.T) {
	pre := FormatPreedit("xian")
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
	for key, entries := range globalDict {
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
		entries, ok := globalDict[w.key]
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
	combos := composeFromSingleChars([]string{"nin", "hao"})
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
	combos := composeFromSingleChars([]string{"ni", "hao", "ma", "a"})
	if len(combos) > 50 {
		t.Fatalf("combos = %d, should be <= 50", len(combos))
	}
	if len(combos) == 0 {
		t.Fatal("expected at least 1 combo")
	}
}

func TestComposeFromSingleCharsSingleSyllable(t *testing.T) {
	combos := composeFromSingleChars([]string{"ni"})
	if len(combos) != 0 {
		t.Fatalf("single syllable should produce no combos, got %d", len(combos))
	}
}
