package ui

import (
	"testing"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
)

// fakeUploader 是 render.GPUGlyphUploader 的测试 mock。
type fakeUploader struct{}

func (f *fakeUploader) OverlayUploadGlyph(r rune) (u0, v0, u1, v1 float32, gw, gh, xoff, yoff int, ok bool) {
	return 0.1, 0.1, 0.2, 0.2, 8, 16, 0, 0, true
}

func newTestDialog() *Dialog {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	d := NewDialog(face, "Title", "Content", nil, []string{"OK"})
	d.SetGPUGlyphUploader(&fakeUploader{})
	return d
}

// TestDialogRenderGPUBgNotPremultiplied 验证 Dialog 背景颜色不预乘 bgAlpha。
// 修复前：bgR = 30.0/255 * bgAlpha（预乘），导致 GL_BLEND 下颜色被三重衰减。
// 修复后：bgR = 30.0/255（原始值），由 shader mix + GL_BLEND 正确处理。
func TestDialogRenderGPUBgNotPremultiplied(t *testing.T) {
	d := newTestDialog()
	var instances []platform.CellInstance
	d.RenderGPU(&instances, 400, 200)

	if len(instances) == 0 {
		t.Fatal("RenderGPU produced no instances")
	}

	expectedBg := float32(30.0 / 255)
	expectedBorder := float32(55.0 / 255)
	expectedBorderB := float32(68.0 / 255)

	for _, inst := range instances {
		if inst.BgA != d.bgAlpha {
			continue // 非背景 cell（文字），跳过
		}
		// 背景区域有两种颜色：inner (30,30,30) 和 border (55,55,68)
		isBorder := inst.BgR == expectedBorder && inst.BgG == expectedBorder && inst.BgB == expectedBorderB
		isInner := inst.BgR == expectedBg && inst.BgG == expectedBg && inst.BgB == expectedBg
		if !isBorder && !isInner {
			t.Errorf("bg cell color=(%v,%v,%v) is neither inner(%.3f,%.3f,%.3f) nor border(%.3f,%.3f,%.3f) — may be premultiplied",
				inst.BgR, inst.BgG, inst.BgB, expectedBg, expectedBg, expectedBg, expectedBorder, expectedBorder, expectedBorderB)
		}
		// 确保不是预乘值
		preMul := float32(30.0/255) * d.bgAlpha
		if inst.BgR == preMul {
			t.Errorf("bg R=%v == premultiplied %v — should be un-premultiplied", inst.BgR, preMul)
		}
	}
}

// TestDialogRenderGPUBgAIsBgAlpha 验证背景 cell 的 BgA 等于 d.bgAlpha。
func TestDialogRenderGPUBgAIsBgAlpha(t *testing.T) {
	d := newTestDialog()
	var instances []platform.CellInstance
	d.RenderGPU(&instances, 400, 200)

	bgCount := 0
	for _, inst := range instances {
		if inst.BgA == d.bgAlpha {
			bgCount++
		}
	}
	if bgCount == 0 {
		t.Fatal("no background cells with BgA == bgAlpha found")
	}
}

// TestDialogRenderGPUTextCellsBgAZero 验证文字 cell 的 BgA == 0。
// 修复前：文字 cell 传 d.bgAlpha 作为 BgA，导致 GL_BLEND 下文字区域
// 被不透明背景覆盖。修复后：BgA=0，finalAlpha = glyphAlpha，
// 文字正确叠加到 overlay 背景上。
func TestDialogRenderGPUTextCellsBgAZero(t *testing.T) {
	d := newTestDialog()
	var instances []platform.CellInstance
	d.RenderGPU(&instances, 400, 200)

	// 文字 cell 有非零 UV（有 glyph），BgA 应为 0
	textCount := 0
	for _, inst := range instances {
		if inst.GlyphU1 > inst.GlyphU0 && inst.V1 > inst.V0 {
			// 这是一个有字形的 cell
			textCount++
			if inst.BgA != 0 {
				t.Errorf("text cell at X=%v Y=%v BgA=%v want 0 (text should not draw opaque bg)",
					inst.X, inst.Y, inst.BgA)
			}
		}
	}
	if textCount == 0 {
		t.Fatal("no text cells with glyphs found — uploader may not be set")
	}
}

// TestDialogRenderGPUButtonBgA 验证按钮背景 cell 的 BgA 值。
// Selected button: BgA=1.0, unselected: BgA=0.8。
// 按钮文字 cell BgA 应为 0。
func TestDialogRenderGPUButtonBgA(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	d := NewDialog(face, "", "", nil, []string{"OK", "Cancel"})
	d.SetGPUGlyphUploader(&fakeUploader{})
	// 默认 selectedBtn=0
	var instances []platform.CellInstance
	d.RenderGPU(&instances, 400, 200)

	foundSelected := false
	foundUnselected := false
	for _, inst := range instances {
		if inst.BgA == 1.0 && inst.BgR == float32(60.0/255) {
			foundSelected = true
		}
		if inst.BgA == 0.8 && inst.BgR == float32(45.0/255) {
			foundUnselected = true
		}
		// 按钮文字 cell（有 glyph）BgA 应为 0
		if inst.GlyphU1 > inst.GlyphU0 && inst.V1 > inst.V0 {
			if inst.BgA != 0 {
				t.Errorf("button text cell BgA=%v want 0", inst.BgA)
			}
		}
	}
	if !foundSelected {
		t.Error("selected button bg cell (BgA=1.0, BgR=60/255) not found")
	}
	if !foundUnselected {
		t.Error("unselected button bg cell (BgA=0.8, BgR=45/255) not found")
	}
}

// TestDialogRenderGPUButtonBgNotPremultiplied 验证按钮背景颜色不预乘。
func TestDialogRenderGPUButtonBgNotPremultiplied(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	d := NewDialog(face, "", "", nil, []string{"OK"})
	d.SetGPUGlyphUploader(&fakeUploader{})
	var instances []platform.CellInstance
	d.RenderGPU(&instances, 400, 200)

	for _, inst := range instances {
		// Selected button bg: (60,120,200), BgA=1.0 — 不预乘
		if inst.BgA == 1.0 {
			if inst.BgR != float32(60.0/255) || inst.BgG != float32(120.0/255) || inst.BgB != float32(200.0/255) {
				t.Errorf("selected button bg=(%v,%v,%v) want (60/255,120/255,200/255) — not premultiplied",
					inst.BgR, inst.BgG, inst.BgB)
			}
		}
		// Unselected button bg: (45,45,52), BgA=0.8 — 不预乘
		if inst.BgA == 0.8 {
			if inst.BgR != float32(45.0/255) || inst.BgG != float32(45.0/255) || inst.BgB != float32(52.0/255) {
				t.Errorf("unselected button bg=(%v,%v,%v) want (45/255,45/255,52/255) — not premultiplied",
					inst.BgR, inst.BgG, inst.BgB)
			}
		}
	}
}
