package gpu

type glyphKey struct {
	Rune   rune
	Italic bool
}

type atlasEntry struct {
	u0, v0, u1, v1 float32
}

// packGlyph 计算字形在 atlas 中的放置位置与 UV（含 0.5 纹素 inset，
// 避免 GL_NEAREST 边界越界采样到相邻字形）。
//
// 输入当前 shelf 状态 (shelfX, shelfY, shelfH) 与字形尺寸 (w,h)，
// 返回放置左上角 (placeX,placeY)、更新后的 shelf 状态、UV、reset 与 ok。
//
// reset=true 表示 atlas 已满，调用方需清空 cache 并重新 TexImage2D，
// 此时返回的 placeX/placeY 为重置后的 (0,0) 起点。
// ok=false 表示 w/h 非法或超过 atlas 尺寸。
func packGlyph(shelfX, shelfY, shelfH, atlasW, atlasH, w, h int) (
	placeX, placeY, nextShelfX, nextShelfY, nextShelfH int,
	u0, v0, u1, v1 float32, reset, ok bool,
) {
	if w <= 0 || h <= 0 || w > atlasW || h > atlasH {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, false, false
	}
	px, py := shelfX, shelfY
	curShelfH := shelfH
	// 当前行放不下 → 换行
	if shelfX+w > atlasW {
		px = 0
		py = shelfY + shelfH
		curShelfH = 0
	}
	// atlas 纵向已满 → 重置
	if py+h > atlasH {
		px = 0
		py = 0
		curShelfH = 0
		reset = true
	}
	aw := float32(atlasW)
	ah := float32(atlasH)
	u0 = (float32(px) + 0.5) / aw
	v0 = (float32(py) + 0.5) / ah
	u1 = (float32(px+w) - 0.5) / aw
	v1 = (float32(py+h) - 0.5) / ah
	nextShelfX = px + w
	nextShelfY = py
	if h > curShelfH {
		nextShelfH = h
	} else {
		nextShelfH = curShelfH
	}
	return px, py, nextShelfX, nextShelfY, nextShelfH, u0, v0, u1, v1, reset, true
}
