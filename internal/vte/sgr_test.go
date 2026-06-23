package vte

import "testing"

func TestSGRReset(t *testing.T) {
	sgrs := ParseSGR([]int{0})
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRReset {
		t.Errorf("expected SGRReset, got %d", sgrs[0].Attr)
	}
}

func TestSGRDefault(t *testing.T) {
	sgrs := ParseSGR(nil)
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRReset {
		t.Errorf("expected SGRReset for nil params, got %d", sgrs[0].Attr)
	}
}

func TestSGRBold(t *testing.T) {
	sgrs := ParseSGR([]int{1})
	if len(sgrs) != 1 || sgrs[0].Attr != SGRBold {
		t.Errorf("expected SGRBold, got %d", sgrs[0].Attr)
	}
}

func TestSGR8Color(t *testing.T) {
	sgrs := ParseSGR([]int{31})
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRForegroundColor8 {
		t.Errorf("expected SGRForegroundColor8, got %d", sgrs[0].Attr)
	}
	if sgrs[0].ColorIdx != 1 {
		t.Errorf("expected ColorIdx 1 (red), got %d", sgrs[0].ColorIdx)
	}
}

func TestSGR256Color(t *testing.T) {
	sgrs := ParseSGR([]int{38, 5, 196})
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRForegroundColor256 {
		t.Errorf("expected SGRForegroundColor256, got %d", sgrs[0].Attr)
	}
	if sgrs[0].ColorIdx != 196 {
		t.Errorf("expected ColorIdx 196, got %d", sgrs[0].ColorIdx)
	}
}

func TestSGRRGBColor(t *testing.T) {
	sgrs := ParseSGR([]int{48, 2, 255, 128, 0})
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRBackgroundColorRGB {
		t.Errorf("expected SGRBackgroundColorRGB, got %d", sgrs[0].Attr)
	}
	if sgrs[0].R != 255 || sgrs[0].G != 128 || sgrs[0].B != 0 {
		t.Errorf("expected RGB(255,128,0), got (%d,%d,%d)", sgrs[0].R, sgrs[0].G, sgrs[0].B)
	}
}

func TestSGRMultiple(t *testing.T) {
	sgrs := ParseSGR([]int{1, 3, 31})
	if len(sgrs) != 3 {
		t.Fatalf("expected 3 SGRs, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRBold {
		t.Errorf("expected SGRBold, got %d", sgrs[0].Attr)
	}
	if sgrs[1].Attr != SGRItalic {
		t.Errorf("expected SGRItalic, got %d", sgrs[1].Attr)
	}
	if sgrs[2].Attr != SGRForegroundColor8 || sgrs[2].ColorIdx != 1 {
		t.Errorf("expected SGRForegroundColor8 idx=1, got %d idx=%d", sgrs[2].Attr, sgrs[2].ColorIdx)
	}
}

func TestSGRBrightColors(t *testing.T) {
	sgrs := ParseSGR([]int{91})
	if len(sgrs) != 1 {
		t.Fatalf("expected 1 SGR, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRForegroundColor8 {
		t.Errorf("expected SGRForegroundColor8, got %d", sgrs[0].Attr)
	}
	if sgrs[0].ColorIdx != 9 {
		t.Errorf("expected ColorIdx 9 (bright red), got %d", sgrs[0].ColorIdx)
	}
}

func TestSGRResetAttributes(t *testing.T) {
	sgrs := ParseSGR([]int{22, 23, 24})
	if len(sgrs) != 3 {
		t.Fatalf("expected 3 SGRs, got %d", len(sgrs))
	}
	if sgrs[0].Attr != SGRBoldOff {
		t.Errorf("expected SGRBoldOff, got %d", sgrs[0].Attr)
	}
	if sgrs[1].Attr != SGRItalicOff {
		t.Errorf("expected SGRItalicOff, got %d", sgrs[1].Attr)
	}
	if sgrs[2].Attr != SGRUnderlineOff {
		t.Errorf("expected SGRUnderlineOff, got %d", sgrs[2].Attr)
	}
}
