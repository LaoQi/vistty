package terminal

import (
	"testing"

	"github.com/LaoQi/vistty/internal/screen"
)

func TestColor256(t *testing.T) {
	tests := []struct {
		idx  int
		want screen.Color
	}{
		{16, screen.Color{R: 0, G: 0, B: 0}},
		{17, screen.Color{R: 0, G: 0, B: 95}},
		{21, screen.Color{R: 0, G: 0, B: 255}},
		{22, screen.Color{R: 0, G: 95, B: 0}},
		{52, screen.Color{R: 95, G: 0, B: 0}},
		{88, screen.Color{R: 135, G: 0, B: 0}},
		{124, screen.Color{R: 175, G: 0, B: 0}},
		{160, screen.Color{R: 215, G: 0, B: 0}},
		{196, screen.Color{R: 255, G: 0, B: 0}},
		{231, screen.Color{R: 255, G: 255, B: 255}},
		{232, screen.Color{R: 8, G: 8, B: 8}},
		{243, screen.Color{R: 118, G: 118, B: 118}},
		{255, screen.Color{R: 238, G: 238, B: 238}},
	}
	for _, tc := range tests {
		got := color256(tc.idx)
		if got != tc.want {
			t.Errorf("color256(%d) = {%d,%d,%d} IsDefault=%v, want {%d,%d,%d}",
				tc.idx, got.R, got.G, got.B, got.IsDefault, tc.want.R, tc.want.G, tc.want.B)
		}
	}
}
