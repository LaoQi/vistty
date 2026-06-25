package render

import "unsafe"

func fillRect(data []byte, stride int, x, y, w, h int, r, g, b uint8) {
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

func blendGlyph(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
	pixel := uint32(255)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
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
			col := x + gx
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
