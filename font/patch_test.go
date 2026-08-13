package font

import (
	"strings"
	"testing"
	"testing/fstest"
)

func mkVFP(t *testing.T, entries []VfpEntry, glyphs [][]byte) []byte {
	t.Helper()
	d, err := EncodeVFP(entries, glyphs)
	if err != nil {
		t.Fatalf("EncodeVFP: %v", err)
	}
	return d
}

func TestLoadSorted(t *testing.T) {
	fsys := fstest.MapFS{
		"000-a.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'c', Advance: 1}}, [][]byte{{1}})},
		"001-b.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'a', Advance: 1}}, [][]byte{{1}})},
		"002-c.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'b', Advance: 1}}, [][]byte{{1}})},
	}
	m, err := LoadPatches(fsys, "*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	want := []rune{'a', 'b', 'c'}
	got := m.Runes()
	if len(got) != len(want) {
		t.Fatalf("runes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("runes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMergeNoConflict(t *testing.T) {
	fsys := fstest.MapFS{
		"001.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'A', Advance: 10}, {Rune: 'B', Advance: 11}}, [][]byte{{1}, {2}})},
		"002.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'C', Advance: 12}, {Rune: 'D', Advance: 13}}, [][]byte{{3}, {4}})},
	}
	m, err := LoadPatches(fsys, "*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	runes := m.Runes()
	for _, r := range []rune{'A', 'B', 'C', 'D'} {
		found := false
		for _, got := range runes {
			if got == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rune %q missing from union", r)
		}
	}
	if m.Empty() {
		t.Error("union of 4 glyphs should not be empty")
	}
}

func TestMergeConflict(t *testing.T) {
	fsys := fstest.MapFS{
		"001.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'A', Advance: 100}}, [][]byte{{1}})},
		"002.vfp": {Data: mkVFP(t, []VfpEntry{{Rune: 'A', Advance: 200}}, [][]byte{{2}})},
	}
	m, err := LoadPatches(fsys, "*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	runes := m.Runes()
	if len(runes) != 1 || runes[0] != 'A' {
		t.Fatalf("runes = %v, want [A]", runes)
	}
	if m.glyphs[0].advance != 200 {
		t.Errorf("conflict advance = %d, want 200 (later patch wins)", m.glyphs[0].advance)
	}
}

func TestLoadNone(t *testing.T) {
	m, err := LoadPatches(fstest.MapFS{}, "*.vfp")
	if err != nil {
		t.Fatalf("LoadPatches: %v", err)
	}
	if !m.Empty() {
		t.Error("empty patch set should report Empty()==true")
	}
	if data, err := m.FontData(); err != nil || data != nil {
		t.Errorf("FontData() = (%d bytes, %v), want (nil, nil)", len(data), err)
	}
}

func TestLoadCorrupt(t *testing.T) {
	fsys := fstest.MapFS{
		"000-bad.vfp": {Data: []byte("NOPE...")},
	}
	_, err := LoadPatches(fsys, "*.vfp")
	if err == nil {
		t.Fatal("expected error for corrupt vfp")
	}
	if !strings.Contains(err.Error(), "000-bad.vfp") {
		t.Errorf("error %q does not contain filename", err)
	}
}
