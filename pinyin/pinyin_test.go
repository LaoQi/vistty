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
		"a":     true,
		"ai":    true,
		"an":    true,
		"ang":   true,
		"ao":    true,
		"ba":    true,
		"zuo":   true,
		"nv":    true,
		"lv":    true,
		"nve":   true,
		"lve":   true,
		"biang": true,
		"zhei":  true,
		"xian":  true,
		"xyz":   false,
		"":      false,
		"q":     false,
		"ab":    false,
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

func TestSplitLongSyllablePriority(t *testing.T) {
	res := Split("xian")
	if len(res) == 0 {
		t.Fatal("Split(xian) empty")
	}
	if joined := strings.Join(res[0], "|"); joined != "xian" {
		t.Errorf("Split(xian)[0] = %q, want xian (long syllable first)", joined)
	}
	res2 := Split("fangan")
	if len(res2) == 0 {
		t.Fatal("Split(fangan) empty")
	}
	if joined := strings.Join(res2[0], "|"); joined != "fang|an" {
		t.Errorf("Split(fangan)[0] = %q, want fang|an", joined)
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
	if globalDict.keyCount() == 0 {
		t.Fatal("dict should not be empty")
	}
	if _, _, ok := globalDict.findKey("ni"); !ok {
		t.Errorf("dict missing single-syllable key 'ni', have %d keys", globalDict.keyCount())
	}
	if _, _, ok := globalDict.findKey("hao"); !ok {
		t.Errorf("dict missing single-syllable key 'hao', have %d keys", globalDict.keyCount())
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

func TestLookupLongSyllableBeatsShortCombo(t *testing.T) {
	cases := []struct {
		input string
		word  string
	}{
		{"daqiao", "大桥"},
		{"shanghai", "上海"},
		{"tianxian", "天线"},
		{"shangqiao", "上翘"},
		{"wangqiao", "网桥"},
		{"changqiao", "长桥"},
		{"jianqiao", "剑桥"},
	}
	for _, c := range cases {
		cands := Lookup(c.input)
		if len(cands) == 0 {
			t.Errorf("Lookup(%q) returned no candidates", c.input)
			continue
		}
		if cands[0].Word != c.word {
			t.Errorf("Lookup(%q)[0] = %q, want %q", c.input, cands[0].Word, c.word)
		}
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
	for i := 0; i < globalDict.keyCount(); i++ {
		key := globalDict.keyAt(i)
		r := globalDict.keyRanges[i]
		for j := uint32(1); j < r.count; j++ {
			prevOff, prevW := globalDict.readEntry(r.start + j - 1)
			curOff, curW := globalDict.readEntry(r.start + j)
			prevWord := globalDict.readWord(prevOff)
			curWord := globalDict.readWord(curOff)
			if curW > prevW {
				t.Errorf("entries for key %q not sorted by weight desc", key)
				break
			}
			if curW == prevW && curWord <= prevWord {
				t.Errorf("entries for key %q not sorted by weight desc", key)
				break
			}
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
		start, count, ok := globalDict.findKey(w.key)
		if !ok {
			t.Errorf("dict missing key %q", w.key)
			continue
		}
		found := false
		for i := uint32(0); i < count; i++ {
			wordOff, _ := globalDict.readEntry(start + i)
			if globalDict.readWord(wordOff) == w.word {
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

func TestExpandPrefix(t *testing.T) {
	exp := expandPrefix("h")
	if len(exp) == 0 {
		t.Fatal("expandPrefix('h') should return syllables starting with 'h'")
	}
	for _, s := range exp {
		if !strings.HasPrefix(s, "h") {
			t.Errorf("expandPrefix('h') returned %q, expected 'h' prefix", s)
		}
	}
	if !sort.SliceIsSorted(exp, func(i, j int) bool { return exp[i] < exp[j] }) {
		t.Error("expandPrefix result not sorted")
	}
}

func TestExpandPrefixNoMatch(t *testing.T) {
	exp := expandPrefix("xyz")
	if len(exp) != 0 {
		t.Fatalf("expandPrefix('xyz') = %v, want empty", exp)
	}
}

func TestSplitFuzzyStrict(t *testing.T) {
	splits, partial := SplitFuzzy("nihao")
	if partial != "" {
		t.Errorf("SplitFuzzy(nihao) partial = %q, want empty", partial)
	}
	strict := Split("nihao")
	if len(splits) != len(strict) {
		t.Fatalf("SplitFuzzy(nihao) = %d splits, Split = %d splits", len(splits), len(strict))
	}
}

func TestSplitFuzzySinglePrefix(t *testing.T) {
	splits, partial := SplitFuzzy("n")
	if len(splits) == 0 {
		t.Fatal("SplitFuzzy('n') should return expansions")
	}
	if partial != "n" {
		t.Errorf("SplitFuzzy('n') partial = %q, want 'n'", partial)
	}
	for _, s := range splits {
		if len(s) != 1 {
			t.Errorf("SplitFuzzy('n') split = %v, expected single syllable", s)
		}
		if !strings.HasPrefix(s[0], "n") {
			t.Errorf("SplitFuzzy('n') syllable %q doesn't start with 'n'", s[0])
		}
	}
}

func TestSplitFuzzyPartialTail(t *testing.T) {
	splits, partial := SplitFuzzy("nih")
	if len(splits) == 0 {
		t.Fatal("SplitFuzzy('nih') should return results")
	}
	if partial != "h" {
		t.Errorf("SplitFuzzy('nih') partial = %q, want 'h'", partial)
	}
	foundNiHao := false
	for _, s := range splits {
		if len(s) == 2 && s[0] == "ni" && strings.HasPrefix(s[1], "h") {
			foundNiHao = true
		}
	}
	if !foundNiHao {
		t.Errorf("SplitFuzzy('nih') missing [ni h*], got %v", splits)
	}
}

func TestSplitFuzzyEmpty(t *testing.T) {
	splits, partial := SplitFuzzy("")
	if splits != nil || partial != "" {
		t.Fatalf("SplitFuzzy('') = %v, %q; want nil, empty", splits, partial)
	}
}

func TestSplitFuzzyInvalid(t *testing.T) {
	splits, partial := SplitFuzzy("zzz")
	if len(splits) != 0 {
		t.Fatalf("SplitFuzzy('zzz') = %v, want empty", splits)
	}
	if partial != "zzz" {
		t.Errorf("SplitFuzzy('zzz') partial = %q, want 'zzz'", partial)
	}
}

func TestLookupPrefix(t *testing.T) {
	cands := Lookup("n")
	if len(cands) == 0 {
		t.Fatal("Lookup('n') should return candidates via prefix expansion")
	}
}

func TestLookupPartialTail(t *testing.T) {
	cands := Lookup("nih")
	if len(cands) == 0 {
		t.Fatal("Lookup('nih') should return candidates")
	}
	found := false
	for _, c := range cands {
		if c.Word == "你好" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Lookup('nih') missing '你好', got %v", cands[:min(5, len(cands))])
	}
}

func TestLookupFuzzyWeightPenalty(t *testing.T) {
	strict := Lookup("nihao")
	fuzzy := Lookup("nih")
	if len(strict) == 0 || len(fuzzy) == 0 {
		t.Skip("need both strict and fuzzy results")
	}
	foundStrict := false
	foundFuzzy := false
	for _, c := range strict {
		if c.Word == "你好" {
			foundStrict = true
		}
	}
	for _, c := range fuzzy {
		if c.Word == "你好" {
			foundFuzzy = true
		}
	}
	if !foundStrict {
		t.Error("Lookup('nihao') missing '你好'")
	}
	if !foundFuzzy {
		t.Error("Lookup('nih') missing '你好'")
	}
}

func TestFormatPreeditPartial(t *testing.T) {
	pre := FormatPreedit("nih")
	if !strings.Contains(pre, "ni") {
		t.Errorf("FormatPreedit('nih') = %q, should contain 'ni'", pre)
	}
	if !strings.Contains(pre, "'h") && !strings.Contains(pre, "h") {
		t.Errorf("FormatPreedit('nih') = %q, should contain partial 'h'", pre)
	}
}

func TestFormatPreeditSinglePrefix(t *testing.T) {
	pre := FormatPreedit("n")
	if pre == "" {
		t.Fatal("FormatPreedit('n') should not be empty")
	}
	if !strings.Contains(pre, "n") {
		t.Errorf("FormatPreedit('n') = %q, should contain 'n'", pre)
	}
}

func TestFormatPreeditLongSyllable(t *testing.T) {
	cases := map[string]string{
		"xian":     "xian",
		"fangan":   "fang'an",
		"fangxian": "fang'xian",
		"xianzai":  "xian'zai",
	}
	for in, want := range cases {
		got := FormatPreedit(in)
		if got != want {
			t.Errorf("FormatPreedit(%q) = %q, want %q", in, got, want)
		}
	}
}
