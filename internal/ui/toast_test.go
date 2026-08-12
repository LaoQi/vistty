package ui

import (
	"testing"
	"time"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
)

// TestToastRenderGPUBgNotPremultiplied 验证 Toast 背景颜色不预乘 bgAlpha。
// 修复前：bgR = baseR * t.bgAlpha（预乘），导致 GL_BLEND 下颜色被三重衰减。
// 修复后：bgR = baseR（原始值），由 shader mix + GL_BLEND 正确处理。
func TestToastRenderGPUBgNotPremultiplied(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	toast := NewToast(face, "Hello", ToastInfo, 5*time.Second)
	toast.SetGPUGlyphUploader(&fakeUploader{})

	var instances []platform.CellInstance
	toast.RenderGPU(&instances, 320, 200)

	if len(instances) == 0 {
		t.Fatal("RenderGPU produced no instances")
	}

	// ToastInfo bg = (40,40,40), bgAlpha=0.9
	expectedR := float32(40.0 / 255)
	expectedG := float32(40.0 / 255)
	expectedB := float32(40.0 / 255)

	bgCount := 0
	for _, inst := range instances {
		if inst.BgA == toast.bgAlpha {
			bgCount++
			if inst.BgR != expectedR || inst.BgG != expectedG || inst.BgB != expectedB {
				t.Errorf("bg cell color=(%v,%v,%v) want (%v,%v,%v) — should be un-premultiplied",
					inst.BgR, inst.BgG, inst.BgB, expectedR, expectedG, expectedB)
			}
			// 确保不是预乘值
			preMulR := expectedR * toast.bgAlpha
			if inst.BgR == preMulR {
				t.Errorf("bg R=%v == premultiplied %v — should be un-premultiplied", inst.BgR, preMulR)
			}
		}
	}
	if bgCount == 0 {
		t.Fatal("no background cells with BgA == bgAlpha found")
	}
}

// TestToastRenderGPUBgAIsBgAlpha 验证背景 cell 的 BgA 等于 toast.bgAlpha。
func TestToastRenderGPUBgAIsBgAlpha(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	toast := NewToast(face, "Hi", ToastInfo, 5*time.Second)
	toast.SetGPUGlyphUploader(&fakeUploader{})

	var instances []platform.CellInstance
	toast.RenderGPU(&instances, 320, 200)

	bgCount := 0
	for _, inst := range instances {
		if inst.BgA == toast.bgAlpha {
			bgCount++
		}
	}
	if bgCount == 0 {
		t.Fatal("no background cells with BgA == bgAlpha found")
	}
}

// TestToastRenderGPUTextCellsBgAZero 验证文字 cell 的 BgA == 0。
// 修复前：文字 cell 传 t.bgAlpha 作为 BgA，导致 GL_BLEND 下文字区域
// 被半透明背景覆盖。修复后：BgA=0，finalAlpha = glyphAlpha，
// 文字正确叠加到 overlay 背景上。
func TestToastRenderGPUTextCellsBgAZero(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	toast := NewToast(face, "Hello", ToastInfo, 5*time.Second)
	toast.SetGPUGlyphUploader(&fakeUploader{})

	var instances []platform.CellInstance
	toast.RenderGPU(&instances, 320, 200)

	textCount := 0
	for _, inst := range instances {
		if inst.GlyphU1 > inst.GlyphU0 && inst.V1 > inst.V0 {
			textCount++
			if inst.BgA != 0 {
				t.Errorf("text cell at X=%v Y=%v BgA=%v want 0 (text should not draw bg)",
					inst.X, inst.Y, inst.BgA)
			}
		}
	}
	if textCount == 0 {
		t.Fatal("no text cells with glyphs found — uploader may not be set")
	}
}

// TestToastRenderGPUWarnErrorColors 验证不同 ToastLevel 的背景颜色不预乘。
func TestToastRenderGPUWarnErrorColors(t *testing.T) {
	cases := []struct {
		level     ToastLevel
		name      string
		expectR   float32
		expectG   float32
		expectB   float32
	}{
		{ToastWarn, "Warn", 180.0 / 255, 140.0 / 255, 20.0 / 255},
		{ToastError, "Error", 180.0 / 255, 40.0 / 255, 40.0 / 255},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
			toast := NewToast(face, "Msg", tc.level, 5*time.Second)
			toast.SetGPUGlyphUploader(&fakeUploader{})

			var instances []platform.CellInstance
			toast.RenderGPU(&instances, 320, 200)

			found := false
			for _, inst := range instances {
				if inst.BgA == toast.bgAlpha {
					found = true
					if inst.BgR != tc.expectR || inst.BgG != tc.expectG || inst.BgB != tc.expectB {
						t.Errorf("%s bg=(%v,%v,%v) want (%v,%v,%v) — un-premultiplied",
							tc.name, inst.BgR, inst.BgG, inst.BgB,
							tc.expectR, tc.expectG, tc.expectB)
					}
					// 确保不是预乘值
					if inst.BgR == tc.expectR*toast.bgAlpha {
						t.Errorf("%s bg R=%v == premultiplied %v — should be un-premultiplied",
							tc.name, inst.BgR, tc.expectR*toast.bgAlpha)
					}
					break
				}
			}
			if !found {
				t.Fatalf("%s: no bg cell found", tc.name)
			}
		})
	}
}

// TestToastRenderGPUFgColorUnaffected 验证文字前景色不受 bgAlpha 影响。
func TestToastRenderGPUFgColorUnaffected(t *testing.T) {
	face := &fakeFace{m: font.Metrics{Width: 8, Height: 16, Ascent: 12}}
	toast := NewToast(face, "Hi", ToastInfo, 5*time.Second)
	toast.SetGPUGlyphUploader(&fakeUploader{})

	var instances []platform.CellInstance
	toast.RenderGPU(&instances, 320, 200)

	expectedFg := float32(230.0 / 255)
	for _, inst := range instances {
		if inst.GlyphU1 > inst.GlyphU0 {
			if inst.FgR != expectedFg || inst.FgG != expectedFg || inst.FgB != expectedFg {
				t.Errorf("fg=(%v,%v,%v) want (%v,%v,%v)",
					inst.FgR, inst.FgG, inst.FgB, expectedFg, expectedFg, expectedFg)
			}
			break
		}
	}
}
