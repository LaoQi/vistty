package runeutil

import "golang.org/x/text/width"

// RuneWidth 返回 rune 的显示列宽（1 或 2）。
// ASCII 走快路径；East Asian Fullwidth/Wide 返回 2，其余返回 1。
func RuneWidth(r rune) int {
	if r < 0x80 {
		return 1
	}
	kind := width.LookupRune(r).Kind()
	if kind == width.EastAsianFullwidth || kind == width.EastAsianWide {
		return 2
	}
	return 1
}

// IsWide 判断 rune 是否为双宽字符。
func IsWide(r rune) bool {
	return RuneWidth(r) == 2
}
