package wayland

import (
	"unicode/utf8"

	"github.com/LaoQi/vistty/internal/platform"
	"golang.org/x/sys/unix"
)

type keymapEntry struct {
	level0 rune
	level1 rune
}

type keymap []keymapEntry

func parseKeymap(fd int, size uint32) (keymap, error) {
	data, err := unix.Mmap(fd, 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	defer unix.Munmap(data)

	km := make(keymap, 256)

	state := kmStateKeycodes
	var currentKeycode uint32
	var currentLevel int

	lines := splitLines(data)
	for _, line := range lines {
		switch state {
		case kmStateKeycodes:
			if contains(line, keycodesEnd) {
				state = kmStateTypes
			} else if contains(line, keycodesBegin) {
				state = kmStateKeycodesInner
			}
		case kmStateKeycodesInner:
			if contains(line, keycodesEnd) {
				state = kmStateTypes
			} else {
				kc, ks := parseKeycodeLine(line)
				if kc > 0 && int(kc) < len(km) {
					km[kc].level0 = xkbKeysymToRune(ks)
					km[kc].level1 = xkbKeysymToRune(shiftKeysym(ks))
				}
			}
		case kmStateTypes:
			if contains(line, typesEnd) {
				state = kmStateCompat
			}
		case kmStateCompat:
			if contains(line, compatEnd) {
				state = kmStateSymbols
			}
		case kmStateSymbols:
			if contains(line, symbolsBegin) {
				state = kmStateSymbolsInner
			} else if contains(line, symbolsEnd) {
				state = kmStateGeometry
			}
		case kmStateSymbolsInner:
			if contains(line, symbolsEnd) {
				state = kmStateGeometry
				break
			}
			kc, l0, l1 := parseSymbolLine(line)
			if kc > 0 && int(kc) < len(km) {
				if l0 != 0 {
					km[kc].level0 = xkbKeysymToRune(l0)
				}
				if l1 != 0 {
					km[kc].level1 = xkbKeysymToRune(l1)
				}
			}
			_ = currentKeycode
			_ = currentLevel
		}
	}

	return km, nil
}

func (km keymap) lookup(key uint32, mods platform.Modifiers) rune {
	if int(key) >= len(km) {
		return platform.FallbackKeyRune(key, mods)
	}
	entry := km[key]
	var r rune
	if mods&platform.ModShift != 0 && entry.level1 != 0 {
		r = entry.level1
	} else {
		r = entry.level0
	}
	if r == 0 {
		return platform.FallbackKeyRune(key, mods)
	}
	if mods&platform.ModCtrl != 0 && r >= 'a' && r <= 'z' {
		r = r - 'a' + 1
	}
	return r
}

type kmParseState int

const (
	kmStateKeycodes kmParseState = iota
	kmStateKeycodesInner
	kmStateTypes
	kmStateCompat
	kmStateSymbols
	kmStateSymbolsInner
	kmStateGeometry
)

const (
	keycodesBegin = "xkb_keycodes"
	keycodesEnd   = "};"
	typesEnd      = "};"
	compatEnd    = "};"
	symbolsBegin = "xkb_symbols"
	symbolsEnd   = "};"
)

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func contains(line []byte, substr string) bool {
	return indexOf(line, substr) >= 0
}

func indexOf(line []byte, substr string) int {
	sub := []byte(substr)
	n := len(line) - len(sub)
	for i := 0; i <= n; i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if line[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func parseKeycodeLine(line []byte) (uint32, xkbKeysym) {
	idx := indexOf(line, "<")
	if idx < 0 {
		return 0, 0
	}
	remaining := line[idx+1:]
	end := indexOf(remaining, ">")
	if end < 0 {
		return 0, 0
	}
	name := string(remaining[:end])

	ks := keyNameToKeysym(name)
	if ks == 0 {
		return 0, 0
	}

	eqIdx := indexOf(remaining[end:], "=")
	if eqIdx < 0 {
		return 0, 0
	}
	afterEq := remaining[end+eqIdx+1:]
	kc := parseUint(afterEq)
	return kc, ks
}

func parseSymbolLine(line []byte) (uint32, xkbKeysym, xkbKeysym) {
	idx := indexOf(line, "[")
	if idx < 0 {
		return 0, 0, 0
	}

	keyIdx := indexOf(line, "key")
	if keyIdx < 0 {
		return 0, 0, 0
	}

	kcStart := indexOf(line[keyIdx:], "<")
	if kcStart < 0 {
		return 0, 0, 0
	}
	kcRemaining := line[keyIdx+kcStart+1:]
	kcEnd := indexOf(kcRemaining, ">")
	if kcEnd < 0 {
		return 0, 0, 0
	}
	keyName := string(kcRemaining[:kcEnd])

	remaining := line[idx+1:]
	var syms [2]xkbKeysym
	for level := 0; level < 2; level++ {
		trimmed := trimLeft(remaining)
		comma := indexOf(trimmed, ",")
		bracket := indexOf(trimmed, "]")
		end := comma
		if bracket >= 0 && (comma < 0 || bracket < comma) {
			end = bracket
		}
		if end < 0 {
			end = len(trimmed)
		}
		token := trim(trimmed[:end])
		if len(token) > 0 {
			syms[level] = tokenToKeysym(token)
		}
		if comma >= 0 {
			remaining = trimmed[comma+1:]
		} else {
			break
		}
	}

	kc := keyNameToKeycode(keyName)
	return kc, syms[0], syms[1]
}

func trimLeft(data []byte) []byte {
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	return data[i:]
}

func trim(data []byte) []byte {
	start := 0
	for start < len(data) && (data[start] == ' ' || data[start] == '\t') {
		start++
	}
	end := len(data)
	for end > start && (data[end-1] == ' ' || data[end-1] == '\t') {
		end--
	}
	return data[start:end]
}

func parseUint(data []byte) uint32 {
	var n uint32
	for _, b := range data {
		if b >= '0' && b <= '9' {
			n = n*10 + uint32(b-'0')
		} else {
			break
		}
	}
	return n
}

type xkbKeysym uint32

func keyNameToKeysym(name string) xkbKeysym {
	if len(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return xkbKeysym(r)
	}
	switch name {
	case "ESC":
		return 0xff1b
	case "AE01":
		return '1'
	case "AE02":
		return '2'
	case "AE03":
		return '3'
	case "AE04":
		return '4'
	case "AE05":
		return '5'
	case "AE06":
		return '6'
	case "AE07":
		return '7'
	case "AE08":
		return '8'
	case "AE09":
		return '9'
	case "AE10":
		return '0'
	case "AE11":
		return '-'
	case "AE12":
		return '='
	case "BKSP":
		return 0xff08
	case "TAB":
		return 0xff09
	case "AD01":
		return 'q'
	case "AD02":
		return 'w'
	case "AD03":
		return 'e'
	case "AD04":
		return 'r'
	case "AD05":
		return 't'
	case "AD06":
		return 'y'
	case "AD07":
		return 'u'
	case "AD08":
		return 'i'
	case "AD09":
		return 'o'
	case "AD10":
		return 'p'
	case "AD11":
		return '['
	case "AD12":
		return ']'
	case "RTRN":
		return 0xff0d
	case "AC01":
		return 'a'
	case "AC02":
		return 's'
	case "AC03":
		return 'd'
	case "AC04":
		return 'f'
	case "AC05":
		return 'g'
	case "AC06":
		return 'h'
	case "AC07":
		return 'j'
	case "AC08":
		return 'k'
	case "AC09":
		return 'l'
	case "AC10":
		return ';'
	case "AC11":
		return '\''
	case "TLDE":
		return '`'
	case "BKSL":
		return '\\'
	case "AB01":
		return 'z'
	case "AB02":
		return 'x'
	case "AB03":
		return 'c'
	case "AB04":
		return 'v'
	case "AB05":
		return 'b'
	case "AB06":
		return 'n'
	case "AB07":
		return 'm'
	case "AB08":
		return ','
	case "AB09":
		return '.'
	case "AB10":
		return '/'
	case "SPCE":
		return ' '
	case "LFSH":
		return 0xffe1
	case "RTSH":
		return 0xffe2
	case "LCTL":
		return 0xffe3
	case "RCTL":
		return 0xffe4
	case "LALT":
		return 0xffe9
	case "RALT":
		return 0xffea
	case "LWIN":
		return 0xffeb
	case "RWIN":
		return 0xffec
	case "UP":
		return 0xff52
	case "DOWN":
		return 0xff54
	case "LEFT":
		return 0xff51
	case "RGHT":
		return 0xff53
	case "HOME":
		return 0xff50
	case "END":
		return 0xff57
	case "PGUP":
		return 0xff55
	case "PGDN":
		return 0xff56
	case "DELE":
		return 0xffff
	case "INS":
		return 0xff63
	}
	return 0
}

func keyNameToKeycode(name string) uint32 {
	mapping := map[string]uint32{
		"ESC":  9, "AE01": 10, "AE02": 11, "AE03": 12, "AE04": 13, "AE05": 14,
		"AE06": 15, "AE07": 16, "AE08": 17, "AE09": 18, "AE10": 19, "AE11": 20,
		"AE12": 21, "BKSP": 22, "TAB": 23, "AD01": 24, "AD02": 25, "AD03": 26,
		"AD04": 27, "AD05": 28, "AD06": 29, "AD07": 30, "AD08": 31, "AD09": 32,
		"AD10": 33, "AD11": 34, "AD12": 35, "RTRN": 36, "AC01": 38, "AC02": 39,
		"AC03": 40, "AC04": 41, "AC05": 42, "AC06": 43, "AC07": 44, "AC08": 45,
		"AC09": 46, "AC10": 47, "AC11": 48, "TLDE": 49, "BKSL": 51, "AB01": 52,
		"AB02": 53, "AB03": 54, "AB04": 55, "AB05": 56, "AB06": 57, "AB07": 58,
		"AB08": 59, "AB09": 60, "AB10": 61, "SPCE": 65, "LFSH": 50, "RTSH": 62,
		"LCTL": 37, "RCTL": 105, "LALT": 64, "RALT": 108, "LWIN": 133, "RWIN": 134,
		"UP": 111, "DOWN": 116, "LEFT": 113, "RGHT": 114, "HOME": 110, "END": 115,
		"PGUP": 112, "PGDN": 117, "DELE": 119, "INS": 118,
	}
	if kc, ok := mapping[name]; ok {
		return kc
	}
	return 0
}

func tokenToKeysym(token []byte) xkbKeysym {
	s := string(token)
	if len(s) >= 4 && s[:4] == "XK_" {
		return namedKeysym(s[4:])
	}
	if len(s) > 0 && s[0] == 'U' {
		var n uint32
		for _, c := range s[1:] {
			if c >= '0' && c <= '9' {
				n = n*16 + uint32(c-'0')
			} else if c >= 'a' && c <= 'f' {
				n = n*16 + uint32(c-'a'+10)
			} else if c >= 'A' && c <= 'F' {
				n = n*16 + uint32(c-'A'+10)
			} else {
				break
			}
		}
		if n > 0 {
			return xkbKeysym(n)
		}
	}
	if len(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return xkbKeysym(r)
	}
	return 0
}

func namedKeysym(name string) xkbKeysym {
	switch name {
	case "Escape":
		return 0xff1b
	case "Return":
		return 0xff0d
	case "BackSpace":
		return 0xff08
	case "Tab":
		return 0xff09
	case "space":
		return ' '
	case "Shift_L":
		return 0xffe1
	case "Shift_R":
		return 0xffe2
	case "Control_L":
		return 0xffe3
	case "Control_R":
		return 0xffe4
	case "Alt_L":
		return 0xffe9
	case "Alt_R":
		return 0xffea
	case "Super_L":
		return 0xffeb
	case "Super_R":
		return 0xffec
	case "Up":
		return 0xff52
	case "Down":
		return 0xff54
	case "Left":
		return 0xff51
	case "Right":
		return 0xff53
	case "Home":
		return 0xff50
	case "End":
		return 0xff57
	case "Page_Up":
		return 0xff55
	case "Page_Down":
		return 0xff56
	case "Delete":
		return 0xffff
	case "Insert":
		return 0xff63
	}
	return 0
}

func shiftKeysym(ks xkbKeysym) xkbKeysym {
	r := xkbKeysymToRune(ks)
	if r == 0 {
		return 0
	}
	sr := platform.ShiftRune(r)
	if sr == r {
		return 0
	}
	return xkbKeysym(sr)
}

func xkbKeysymToRune(ks xkbKeysym) rune {
	if ks == 0 {
		return 0
	}
	if ks >= 0x20 && ks <= 0x7e {
		return rune(ks)
	}
	switch ks {
	case 0xff08:
		return 0x08
	case 0xff09:
		return '\t'
	case 0xff0d:
		return '\r'
	case 0xff1b:
		return 0x1b
	case 0xff50:
		return 0x1b
	case 0xff51:
		return 0x1b
	case 0xff52:
		return 0x1b
	case 0xff53:
		return 0x1b
	case 0xff55:
		return 0x1b
	case 0xff56:
		return 0x1b
	case 0xff57:
		return 0x1b
	case 0xff63:
		return 0x1b
	case 0xffff:
		return 0x7f
	}
	if ks >= 0x100 {
		return rune(ks)
	}
	return 0
}
