package render

import "unsafe"

func FillRect(data []byte, stride int, x, y, w, h int, r, g, b uint8) {
	if w <= 0 || h <= 0 || len(data) == 0 || stride <= 0 {
		return
	}
	pixel := uint32(255)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	rowsTotal := (len(data) + stride - 1) / stride
	rowLo := max(0, y)
	rowHi := min(y+h, rowsTotal)
	rowPix := stride / 4
	colLo := max(0, x)
	colHi := min(x+w, rowPix)
	if colHi <= colLo {
		return
	}
	fullRows := len(data)%stride == 0
	for row := rowLo; row < rowHi; row++ {
		offset := row * stride
		endCol := colHi
		if !fullRows {
			L := len(data) - 3 - offset
			if L <= 0 {
				continue
			}
			endCol = min(colHi, (L-1)/4+1)
			if endCol <= colLo {
				continue
			}
		}
		startOff := offset + colLo*4
		endOff := offset + endCol*4
		for off := startOff; off+4 <= endOff; off += 4 {
			*(*uint32)(unsafe.Pointer(&data[off])) = pixel
		}
	}
}

// FillRectBlend 以 alpha 混合方式将 RGBA 颜色写入帧缓冲。
// alpha=255 时等价 FillRect，alpha=0 时不写入。
func FillRectBlend(data []byte, stride int, x, y, w, h int, r, g, b, a uint8) {
	if a == 0 || w <= 0 || h <= 0 || len(data) == 0 || stride <= 0 {
		return
	}
	if a == 255 {
		FillRect(data, stride, x, y, w, h, r, g, b)
		return
	}
	rowsTotal := (len(data) + stride - 1) / stride
	rowLo := max(0, y)
	rowHi := min(y+h, rowsTotal)
	rowPix := stride / 4
	colLo := max(0, x)
	colHi := min(x+w, rowPix)
	if colHi <= colLo {
		return
	}
	fullRows := len(data)%stride == 0
	for row := rowLo; row < rowHi; row++ {
		offset := row * stride
		endCol := colHi
		if !fullRows {
			L := len(data) - 3 - offset
			if L <= 0 {
				continue
			}
			endCol = min(colHi, (L-1)/4+1)
			if endCol <= colLo {
				continue
			}
		}
		startOff := offset + colLo*4
		endOff := offset + endCol*4
		for off := startOff; off+4 <= endOff; off += 4 {
			data[off+0] = uint8((uint16(b)*uint16(a) + uint16(data[off+0])*uint16(255-a) + 128) >> 8)
			data[off+1] = uint8((uint16(g)*uint16(a) + uint16(data[off+1])*uint16(255-a) + 128) >> 8)
			data[off+2] = uint8((uint16(r)*uint16(a) + uint16(data[off+2])*uint16(255-a) + 128) >> 8)
			data[off+3] = 255
		}
	}
}

// BlendGlyph 将字形位图以指定前景色混合到帧缓冲。
// ca=255 特化：combined = alpha*255/255 恒等于 alpha，
// 因此省去每像素的乘法/除法路径，仅保留 alpha 分支。
func BlendGlyph(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
	blendGlyph(data, stride, x, y, bitmap, glyphW, glyphH, r, g, b)
}

func blendGlyph(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
	if glyphW <= 0 || glyphH <= 0 || len(data) == 0 || stride <= 0 {
		return
	}
	rowsTotal := (len(data) + stride - 1) / stride
	rowLo := max(0, -y)
	rowHi := min(glyphH, rowsTotal-y)
	rowPix := stride / 4
	colLo := max(0, -x)
	colHi := min(glyphW, rowPix-x)
	if rowHi <= rowLo || colHi <= colLo {
		return
	}
	ri, gi, bi := uint16(r), uint16(g), uint16(b)
	fullRows := len(data)%stride == 0
	for gy := rowLo; gy < rowHi; gy++ {
		row := y + gy
		offset := row * stride
		colEnd := colHi
		if !fullRows {
			L := len(data) - 3 - offset
			if L <= 0 {
				continue
			}
			colEnd = min(colHi, (L-1)/4+1-x)
			if colEnd <= colLo {
				continue
			}
		}
		srcRow := bitmap[gy*glyphW+colLo : gy*glyphW+colEnd]
		dstBase := offset + (x+colLo)*4
		for i, alpha := range srcRow {
			if alpha == 0 {
				continue
			}
			px := dstBase + i*4
			if alpha == 255 {
				data[px+0] = b
				data[px+1] = g
				data[px+2] = r
				data[px+3] = 255
			} else {
				a := uint16(alpha)
				inv := uint16(255 - alpha)
				data[px+0] = uint8((bi*a + uint16(data[px+0])*inv + 128) >> 8)
				data[px+1] = uint8((gi*a + uint16(data[px+1])*inv + 128) >> 8)
				data[px+2] = uint8((ri*a + uint16(data[px+2])*inv + 128) >> 8)
				data[px+3] = 255
			}
		}
	}
}

// blendColorGlyph 将 RGBA 彩色字形（如 emoji）混合到 BGRA32 帧缓冲。
// 忽略前景色（彩色字形自带颜色），alpha 预乘混合。
func blendColorGlyph(data []byte, stride int, x, y int, rgba []byte, glyphW, glyphH int) {
	if glyphW <= 0 || glyphH <= 0 || len(data) == 0 || stride <= 0 {
		return
	}
	rowsTotal := (len(data) + stride - 1) / stride
	rowLo := max(0, -y)
	rowHi := min(glyphH, rowsTotal-y)
	rowPix := stride / 4
	colLo := max(0, -x)
	colHi := min(glyphW, rowPix-x)
	if rowHi <= rowLo || colHi <= colLo {
		return
	}
	for gy := rowLo; gy < rowHi; gy++ {
		row := y + gy
		offset := row * stride
		L := len(data) - 3 - offset
		if L <= 0 {
			continue
		}
		colEnd := min(colHi, (L-1)/4+1-x)
		if colEnd <= colLo {
			continue
		}
		srcRow := rgba[(gy*glyphW+colLo)*4 : (gy*glyphW+colEnd)*4]
		dstBase := offset + (x+colLo)*4
		for i := 0; i < len(srcRow); i += 4 {
			sr, sg, sb := srcRow[i], srcRow[i+1], srcRow[i+2]
			sa := srcRow[i+3]
			if sa == 0 {
				continue
			}
			px := dstBase + i
			if sa == 255 {
				data[px+0] = sb
				data[px+1] = sg
				data[px+2] = sr
				data[px+3] = 255
				continue
			}
			a := uint16(sa)
			ia := uint16(255 - sa)
			data[px+0] = uint8((uint16(sb)*a + uint16(data[px+0])*ia + 128) >> 8)
			data[px+1] = uint8((uint16(sg)*a + uint16(data[px+1])*ia + 128) >> 8)
			data[px+2] = uint8((uint16(sr)*a + uint16(data[px+2])*ia + 128) >> 8)
			data[px+3] = uint8((a + uint16(data[px+3])*ia + 128) >> 8)
		}
	}
}

// BlendGlyphAlpha 与 BlendGlyph 相同，但前景色携带 alpha 通道，
// 最终 alpha = glyph_alpha * color_alpha / 255。
// ca=255 时委托给 BlendGlyph 的特化路径。
func BlendGlyphAlpha(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b, ca uint8) {
	if ca == 0 {
		return
	}
	if ca == 255 {
		blendGlyph(data, stride, x, y, bitmap, glyphW, glyphH, r, g, b)
		return
	}
	if glyphW <= 0 || glyphH <= 0 || len(data) == 0 || stride <= 0 {
		return
	}
	rowsTotal := (len(data) + stride - 1) / stride
	rowLo := max(0, -y)
	rowHi := min(glyphH, rowsTotal-y)
	rowPix := stride / 4
	colLo := max(0, -x)
	colHi := min(glyphW, rowPix-x)
	if rowHi <= rowLo || colHi <= colLo {
		return
	}
	ri, gi, bi := uint16(r), uint16(g), uint16(b)
	caU := uint16(ca)
	fullRows := len(data)%stride == 0
	for gy := rowLo; gy < rowHi; gy++ {
		row := y + gy
		offset := row * stride
		colEnd := colHi
		if !fullRows {
			L := len(data) - 3 - offset
			if L <= 0 {
				continue
			}
			colEnd = min(colHi, (L-1)/4+1-x)
			if colEnd <= colLo {
				continue
			}
		}
		srcRow := bitmap[gy*glyphW+colLo : gy*glyphW+colEnd]
		dstBase := offset + (x+colLo)*4
		for i, alpha := range srcRow {
			if alpha == 0 {
				continue
			}
			combined := uint16(alpha) * caU / 255
			if combined == 0 {
				continue
			}
			px := dstBase + i*4
			if combined == 255 {
				data[px+0] = b
				data[px+1] = g
				data[px+2] = r
				data[px+3] = 255
			} else {
				inv := uint16(255 - combined)
				data[px+0] = uint8((bi*combined + uint16(data[px+0])*inv + 128) >> 8)
				data[px+1] = uint8((gi*combined + uint16(data[px+1])*inv + 128) >> 8)
				data[px+2] = uint8((ri*combined + uint16(data[px+2])*inv + 128) >> 8)
				data[px+3] = 255
			}
		}
	}
}
