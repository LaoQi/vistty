package font

// CSD（客户端窗口装饰）按钮符号专用 rune。
// 使用 SMP 私用区（Plane 15，U+F8000-U+F8002）未分配码点，Sarasa/Nerd 均
// 无覆盖，避免与终端正文字形及 nerd fallback 冲突。OpenTypeFace.Glyph 对
// 这些 rune 直接返回几何合成字形，不查询字体。
const (
	CsdBtnMinRune  rune = 0xF8000
	CsdBtnMaxRune  rune = 0xF8001
	CsdBtnCloseRune rune = 0xF8002
)

func isCsdRune(r rune) bool {
	return r >= CsdBtnMinRune && r <= CsdBtnCloseRune
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// synthCsdGlyph 为 CSD 按钮符号合成几何 alpha 字形（不依赖字体字形，规避
// 等宽字体中符号过小或缺失的问题）。符号外接正方形边长为栏高的 1/2，垂直
// 居中于标签栏（YOffset=-Ascent+(Height-sym)/2），水平由调用方在按钮内居中。
// 最小化=水平横线，最大化=矩形边框，关闭=✕ 对角交叉线，均为硬边填充。
func synthCsdGlyph(r rune, m Metrics) *Glyph {
	if !isCsdRune(r) || m.Height <= 0 || m.Width <= 0 {
		return nil
	}
	sym := m.Height / 2
	if sym < 8 {
		sym = 8
	}
	lw := sym / 6
	if lw < 1 {
		lw = 1
	}
	half := lw / 2
	mid := sym / 2

	bmp := make([]byte, sym*sym)
	for y := 0; y < sym; y++ {
		for x := 0; x < sym; x++ {
			on := false
			switch {
			case r == CsdBtnMinRune:
				// 水平横线，垂直居中
				if y >= mid-half && y < mid-half+lw {
					on = true
				}
			case r == CsdBtnMaxRune:
				// 矩形边框，厚度 lw（从边缘向内；lw=1 时仍须画 1px 外圈，
				// 不能用 half=lw/2，否则 half=0 导致边框条件永假而空白）
				if x < lw || x >= sym-lw || y < lw || y >= sym-lw {
					on = true
				}
			case r == CsdBtnCloseRune:
				// 两条对角交叉线
				if absInt(x-y) <= lw || absInt((x+y)-(sym-1)) <= lw {
					on = true
				}
			}
			if on {
				bmp[y*sym+x] = 255
			}
		}
	}

	return &Glyph{
		Rune:    r,
		Bitmap:  bmp,
		Width:   sym,
		Height:  sym,
		XOffset: 0,
		YOffset: -m.Ascent + (m.Height-sym)/2,
		Advance: m.Width,
	}
}
