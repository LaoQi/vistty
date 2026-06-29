package render

import "unsafe"

func FillRect(data []byte, stride int, x, y, w, h int, r, g, b uint8) {
	pixel := uint32(255)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	for row := y; row < y+h; row++ {
		startOff := row*stride + x*4
		endOff := startOff + w*4
		if startOff < 0 {
			continue
		}
		if endOff > len(data) {
			endOff = len(data)
		}
		if endOff <= startOff {
			continue
		}
		for off := startOff; off+4 <= endOff; off += 4 {
			*(*uint32)(unsafe.Pointer(&data[off])) = pixel
		}
	}
}

// FillRectBlend 以 alpha 混合方式将 RGBA 颜色写入帧缓冲。
// alpha=255 时等价 FillRect，alpha=0 时不写入。
func FillRectBlend(data []byte, stride int, x, y, w, h int, r, g, b, a uint8) {
	if a == 0 {
		return
	}
	if a == 255 {
		FillRect(data, stride, x, y, w, h, r, g, b)
		return
	}
	for row := y; row < y+h; row++ {
		startOff := row*stride + x*4
		endOff := startOff + w*4
		if startOff < 0 {
			continue
		}
		if endOff > len(data) {
			endOff = len(data)
		}
		if endOff <= startOff {
			continue
		}
		for off := startOff; off+4 <= endOff; off += 4 {
			data[off+0] = uint8((uint16(b)*uint16(a) + uint16(data[off+0])*uint16(255-a) + 128) >> 8)
			data[off+1] = uint8((uint16(g)*uint16(a) + uint16(data[off+1])*uint16(255-a) + 128) >> 8)
			data[off+2] = uint8((uint16(r)*uint16(a) + uint16(data[off+2])*uint16(255-a) + 128) >> 8)
			data[off+3] = 255
		}
	}
}

// BlendGlyph 将字形位图以指定前景色混合到帧缓冲。
func BlendGlyph(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
	BlendGlyphAlpha(data, stride, x, y, bitmap, glyphW, glyphH, r, g, b, 255)
}

// BlendGlyphAlpha 与 BlendGlyph 相同，但前景色携带 alpha 通道，
// 最终 alpha = glyph_alpha * color_alpha / 255。
func BlendGlyphAlpha(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b, ca uint8) {
	for gy := 0; gy < glyphH; gy++ {
		row := y + gy
		offset := row * stride
		if offset < 0 || offset >= len(data) {
			continue
		}
		for gx := 0; gx < glyphW; gx++ {
			alpha := bitmap[gy*glyphW+gx]
			if alpha == 0 {
				continue
			}
			// 合并字形 alpha 与颜色 alpha
			combined := uint16(alpha) * uint16(ca) / 255
			if combined == 0 {
				continue
			}
			col := x + gx
			px := offset + col*4
			if px < 0 || px+3 >= len(data) {
				continue
			}
			if combined == 255 {
				data[px+0] = b
				data[px+1] = g
				data[px+2] = r
				data[px+3] = 255
			} else {
				inv := uint16(255 - combined)
				data[px+0] = uint8((uint16(b)*combined + uint16(data[px+0])*inv + 128) >> 8)
				data[px+1] = uint8((uint16(g)*combined + uint16(data[px+1])*inv + 128) >> 8)
				data[px+2] = uint8((uint16(r)*combined + uint16(data[px+2])*inv + 128) >> 8)
				data[px+3] = 255
			}
		}
	}
}

func blendGlyphItalic(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
	pixel := uint32(255)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	slopeNum := 2
	slopeDen := 5
	for gy := 0; gy < glyphH; gy++ {
		row := y + gy
		offset := row * stride
		if offset < 0 || offset >= len(data) {
			continue
		}
		shift := gy * slopeNum / slopeDen
		for gx := 0; gx < glyphW; gx++ {
			alpha := bitmap[gy*glyphW+gx]
			if alpha == 0 {
				continue
			}
			col := x + gx + shift
			px := offset + col*4
			if px < 0 || px+3 >= len(data) {
				continue
			}
			if alpha == 255 {
				*(*uint32)(unsafe.Pointer(&data[px])) = pixel
			} else {
				data[px+0] = uint8((uint16(b)*uint16(alpha) + uint16(data[px+0])*uint16(255-alpha) + 128) >> 8)
				data[px+1] = uint8((uint16(g)*uint16(alpha) + uint16(data[px+1])*uint16(255-alpha) + 128) >> 8)
				data[px+2] = uint8((uint16(r)*uint16(alpha) + uint16(data[px+2])*uint16(255-alpha) + 128) >> 8)
				data[px+3] = 255
			}
		}
	}
}
