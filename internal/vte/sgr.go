package vte

type SGRAttr int

const (
	SGRReset SGRAttr = iota
	SGRBold
	SGRDim
	SGRItalic
	SGRUnderline
	SGRBlink
	SGRReverse
	SGRCrossedOut
	SGRConceal
	SGRReveal
	SGROverline
	SGROverlineOff
	SGRForegroundColor8
	SGRBackgroundColor8
	SGRForegroundColor256
	SGRBackgroundColor256
	SGRForegroundColorRGB
	SGRBackgroundColorRGB
	SGRBoldOff
	SGRDimOff
	SGRItalicOff
	SGRUnderlineOff
	SGRBlinkOff
	SGRReverseOff
	SGRCrossedOutOff
	SGRForegroundColorReset
	SGRBackgroundColorReset
)

type SGR struct {
	Attr     SGRAttr
	ColorIdx int
	R, G, B  uint8
}

func ParseSGR(params []int) []SGR {
	if len(params) == 0 {
		return []SGR{{Attr: SGRReset}}
	}

	var result []SGR
	result = make([]SGR, 0, 8)
	i := 0
	for i < len(params) {
		p := params[i]
		switch p {
		case 0:
			result = append(result, SGR{Attr: SGRReset})
		case 1:
			result = append(result, SGR{Attr: SGRBold})
		case 2:
			result = append(result, SGR{Attr: SGRDim})
		case 3:
			result = append(result, SGR{Attr: SGRItalic})
		case 4:
			result = append(result, SGR{Attr: SGRUnderline})
		case 5:
			result = append(result, SGR{Attr: SGRBlink})
		case 7:
			result = append(result, SGR{Attr: SGRReverse})
		case 8:
			result = append(result, SGR{Attr: SGRConceal})
		case 9:
			result = append(result, SGR{Attr: SGRCrossedOut})
		case 22:
			result = append(result, SGR{Attr: SGRBoldOff}, SGR{Attr: SGRDimOff})
		case 23:
			result = append(result, SGR{Attr: SGRItalicOff})
		case 24:
			result = append(result, SGR{Attr: SGRUnderlineOff})
		case 25:
			result = append(result, SGR{Attr: SGRBlinkOff})
		case 27:
			result = append(result, SGR{Attr: SGRReverseOff})
		case 28:
			result = append(result, SGR{Attr: SGRReveal})
		case 29:
			result = append(result, SGR{Attr: SGRCrossedOutOff})
		case 30, 31, 32, 33, 34, 35, 36, 37:
			result = append(result, SGR{Attr: SGRForegroundColor8, ColorIdx: p - 30})
		case 39:
			result = append(result, SGR{Attr: SGRForegroundColorReset})
		case 40, 41, 42, 43, 44, 45, 46, 47:
			result = append(result, SGR{Attr: SGRBackgroundColor8, ColorIdx: p - 40})
		case 49:
			result = append(result, SGR{Attr: SGRBackgroundColorReset})
		case 38:
			sgr, adv := parseSGRColor(params, i, true)
			result = append(result, sgr)
			i += adv
			continue
		case 48:
			sgr, adv := parseSGRColor(params, i, false)
			result = append(result, sgr)
			i += adv
			continue
		case 53:
			result = append(result, SGR{Attr: SGROverline})
		case 55:
			result = append(result, SGR{Attr: SGROverlineOff})
		case 90, 91, 92, 93, 94, 95, 96, 97:
			result = append(result, SGR{Attr: SGRForegroundColor8, ColorIdx: p - 90 + 8})
		case 100, 101, 102, 103, 104, 105, 106, 107:
			result = append(result, SGR{Attr: SGRBackgroundColor8, ColorIdx: p - 100 + 8})
		}
		i++
	}

	if len(result) == 0 {
		return []SGR{{Attr: SGRReset}}
	}
	return result
}

func parseSGRColor(params []int, idx int, fg bool) (SGR, int) {
	baseAttr := SGRForegroundColor256
	rgbAttr := SGRForegroundColorRGB
	if !fg {
		baseAttr = SGRBackgroundColor256
		rgbAttr = SGRBackgroundColorRGB
	}

	if idx+1 >= len(params) {
		if fg {
			return SGR{Attr: SGRForegroundColorReset}, 1
		}
		return SGR{Attr: SGRBackgroundColorReset}, 1
	}

	switch params[idx+1] {
	case 5:
		if idx+2 < len(params) {
			return SGR{Attr: baseAttr, ColorIdx: params[idx+2]}, 3
		}
		return SGR{Attr: baseAttr}, 2
	case 2:
		if idx+4 < len(params) {
			r := params[idx+2]
			g := params[idx+3]
			b := params[idx+4]
			if r < 0 {
				r = 0
			}
			if r > 255 {
				r = 255
			}
			if g < 0 {
				g = 0
			}
			if g > 255 {
				g = 255
			}
			if b < 0 {
				b = 0
			}
			if b > 255 {
				b = 255
			}
			return SGR{
				Attr: rgbAttr,
				R:    uint8(r),
				G:    uint8(g),
				B:    uint8(b),
			}, 5
		}
		return SGR{Attr: rgbAttr}, 2
	default:
		if fg {
			return SGR{Attr: SGRForegroundColorReset}, 2
		}
		return SGR{Attr: SGRBackgroundColorReset}, 2
	}
}
