package terminal

import "golang.org/x/text/width"

func runeWidth(r rune) int {
	kind := width.LookupRune(r).Kind()
	if kind == width.EastAsianFullwidth || kind == width.EastAsianWide {
		return 2
	}
	return 1
}
