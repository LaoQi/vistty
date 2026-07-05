package runeutil

import "testing"

func TestRuneWidthEmoji(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want int
	}{
		{"grinning U+1F600", 0x1F600, 2},
		{"heart U+2764", 0x2764, 2},
		{"star U+2B50", 0x2B50, 2},
		{"sun U+2600", 0x2600, 2},
		{"scissors U+2702", 0x2702, 2},
		{"watch U+231A", 0x231A, 2},
		{"ZWJ U+200D", 0x200D, 1},
		{"VS16 U+FE0F", 0xFE0F, 1},
		{"ascii A", 'A', 1},
		{"cjk 中", '中', 2},
		{"mult-x U+2715 ✕", 0x2715, 1},
		{"mult-sign U+00D7 ×", 0x00D7, 1},
		{"pi U+03C0 π", 0x03C0, 1},
		{"heavy-x U+2716 ✖", 0x2716, 2},
	}
	for _, c := range cases {
		if got := RuneWidth(c.r); got != c.want {
			t.Errorf("%s: RuneWidth=%d, want %d", c.name, got, c.want)
		}
	}
}
