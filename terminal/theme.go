package terminal

import "github.com/LaoQi/vistty/internal/screen"

type Theme struct {
	DefFg       screen.Color
	DefBg       screen.Color
	CursorColor screen.Color
	Palette     [16]screen.Color
}

var DefaultTheme = Theme{
	DefFg:       screen.Color{R: 255, G: 255, B: 255},
	DefBg:       screen.Color{R: 0, G: 0, B: 0},
	CursorColor: screen.Color{R: 255, G: 255, B: 255},
	Palette: [16]screen.Color{
		{R: 0, G: 0, B: 0},
		{R: 205, G: 0, B: 0},
		{R: 0, G: 205, B: 0},
		{R: 205, G: 205, B: 0},
		{R: 0, G: 0, B: 238},
		{R: 205, G: 0, B: 205},
		{R: 0, G: 205, B: 205},
		{R: 229, G: 229, B: 229},
		{R: 127, G: 127, B: 127},
		{R: 255, G: 0, B: 0},
		{R: 0, G: 255, B: 0},
		{R: 255, G: 255, B: 0},
		{R: 92, G: 92, B: 255},
		{R: 255, G: 0, B: 255},
		{R: 0, G: 255, B: 255},
		{R: 255, G: 255, B: 255},
	},
}
