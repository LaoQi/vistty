package render

func fillRect(data []byte, stride int, x, y, w, h int, r, g, b uint8) {
	for row := y; row < y+h; row++ {
		offset := row * stride
		if offset < 0 || offset >= len(data) {
			continue
		}
		for col := x; col < x+w; col++ {
			px := offset + col*4
			if px < 0 || px+3 >= len(data) {
				continue
			}
			data[px+0] = b
			data[px+1] = g
			data[px+2] = r
			data[px+3] = 255
		}
	}
}

func blendGlyph(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
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
				data[px+0] = b
				data[px+1] = g
				data[px+2] = r
				data[px+3] = 255
			} else {
				data[px+0] = uint8((uint16(b)*uint16(alpha) + uint16(data[px+0])*uint16(255-alpha)) / 255)
				data[px+1] = uint8((uint16(g)*uint16(alpha) + uint16(data[px+1])*uint16(255-alpha)) / 255)
				data[px+2] = uint8((uint16(r)*uint16(alpha) + uint16(data[px+2])*uint16(255-alpha)) / 255)
				data[px+3] = 255
			}
		}
	}
}

func blendGlyphItalic(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b uint8) {
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
				data[px+0] = b
				data[px+1] = g
				data[px+2] = r
				data[px+3] = 255
			} else {
				data[px+0] = uint8((uint16(b)*uint16(alpha) + uint16(data[px+0])*uint16(255-alpha)) / 255)
				data[px+1] = uint8((uint16(g)*uint16(alpha) + uint16(data[px+1])*uint16(255-alpha)) / 255)
				data[px+2] = uint8((uint16(r)*uint16(alpha) + uint16(data[px+2])*uint16(255-alpha)) / 255)
				data[px+3] = 255
			}
		}
	}
}
