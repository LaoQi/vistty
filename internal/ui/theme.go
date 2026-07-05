package ui

type OSDTheme struct {
	BarBg      [3]uint8
	ActiveBg   [3]uint8
	InactiveBg [3]uint8
	ActiveFg   [3]uint8
	InactiveFg [3]uint8
	CsdBtnBg   [3]uint8
	CsdCloseBg [3]uint8
	CsdBtnFg   [3]uint8
}

var DefaultOSDTheme = OSDTheme{
	BarBg:      [3]uint8{24, 24, 24},
	ActiveBg:   [3]uint8{56, 56, 56},
	InactiveBg: [3]uint8{32, 32, 32},
	ActiveFg:   [3]uint8{230, 230, 230},
	InactiveFg: [3]uint8{150, 150, 150},
	CsdBtnBg:   [3]uint8{40, 40, 40},
	CsdCloseBg: [3]uint8{200, 50, 50},
	CsdBtnFg:   [3]uint8{200, 200, 200},
}
