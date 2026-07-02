package font

import "math"

// ShearGlyph 对字形做 shear（剪切）变换生成斜体字形。
// slope 表示顶部向右偏移 slope*(Height-1) 像素，底部不动，形成 / 形（标准 italic）。
// align 控制整体水平偏移以平衡左右溢出：XOffset -= round(align*maxShift)。
//   align=0  → 底部对齐（右侧溢出 maxShift，左侧 0）
//   align=0.5→ 居中（左右各溢出 0.5*maxShift）
//   align=1  → 顶部对齐（左侧溢出 maxShift，右侧 0）
// 使用双线性插值抗锯齿。返回新 Glyph（不修改原 g）：
//   - 新 Bitmap 宽度 = Width + ceil(slope*(Height-1))
//   - 新 Height = 原 Height
//   - XOffset = 原 XOffset - round(align*maxShift)
//   - YOffset, Advance, Rune 不变
func ShearGlyph(g *Glyph, slope, align float64) *Glyph {
	if g == nil {
		return nil
	}
	if g.Width <= 0 || g.Height <= 0 {
		return nil
	}
	if slope == 0 {
		return &Glyph{
			Rune:    g.Rune,
			Bitmap:  append([]byte(nil), g.Bitmap...),
			Width:   g.Width,
			Height:  g.Height,
			XOffset: g.XOffset,
			YOffset: g.YOffset,
			Advance: g.Advance,
		}
	}

	srcW := g.Width
	srcH := g.Height
	src := g.Bitmap

	maxShift := slope * float64(srcH-1)
	newW := srcW + int(math.Ceil(maxShift))
	newH := srcH
	dst := make([]byte, newW*newH)

	for oy := 0; oy < newH; oy++ {
		shift := slope * float64(newH-1-oy)
		rowOff := oy * newW
		srcRowOff := oy * srcW
		for ox := 0; ox < newW; ox++ {
			sx := float64(ox) - shift
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			var a0, a1 float64
			if x0 >= 0 && x0 < srcW {
				a0 = float64(src[srcRowOff+x0])
			}
			x1 := x0 + 1
			if x1 >= 0 && x1 < srcW {
				a1 = float64(src[srcRowOff+x1])
			}
			alpha := a0*(1-fx) + a1*fx
			if alpha < 0 {
				alpha = 0
			} else if alpha > 255 {
				alpha = 255
			}
			dst[rowOff+ox] = byte(alpha + 0.5)
		}
	}

	return &Glyph{
		Rune:    g.Rune,
		Bitmap:  dst,
		Width:   newW,
		Height:  newH,
		XOffset: g.XOffset - int(math.Round(align*maxShift)),
		YOffset: g.YOffset,
		Advance: g.Advance,
	}
}
