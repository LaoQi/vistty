package render

import (
	"testing"
)

const (
	testW      = 100
	testH      = 100
	testStride = testW * 4
)

func newFrameBuf() []byte {
	return make([]byte, testStride*testH)
}

func pixelAt(data []byte, stride, x, y int) (b, g, r, a uint8) {
	off := y*stride + x*4
	return data[off], data[off+1], data[off+2], data[off+3]
}

func TestFillRect(t *testing.T) {
	data := newFrameBuf()

	FillRect(data, testStride, 10, 10, 20, 20, 0xFF, 0x00, 0x80)

	b, g, r, a := pixelAt(data, testStride, 15, 15)
	if r != 0xFF || g != 0x00 || b != 0x80 || a != 255 {
		t.Errorf("filled pixel = (R=%02X,G=%02X,B=%02X,A=%02X), want (FF,00,80,FF)", r, g, b, a)
	}

	b, g, r, a = pixelAt(data, testStride, 0, 0)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("unfilled pixel = (R=%02X,G=%02X,B=%02X,A=%02X), want (00,00,00,00)", r, g, b, a)
	}

	b, g, r, a = pixelAt(data, testStride, 9, 10)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("pixel outside rect = (R=%02X,G=%02X,B=%02X,A=%02X), want (00,00,00,00)", r, g, b, a)
	}

	b, g, r, a = pixelAt(data, testStride, 30, 10)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("pixel outside rect = (R=%02X,G=%02X,B=%02X,A=%02X), want (00,00,00,00)", r, g, b, a)
	}

	FillRect(data, testStride, -5, -5, 3, 3, 0xFF, 0xFF, 0xFF)
	b, g, r, a = pixelAt(data, testStride, 0, 0)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("negative coords should not write, got (R=%02X,G=%02X,B=%02X,A=%02X)", r, g, b, a)
	}

	FillRect(data, testStride, 95, 95, 20, 20, 0xFF, 0xFF, 0xFF)
	b, g, r, a = pixelAt(data, testStride, 99, 99)
	if r != 0xFF || g != 0xFF || b != 0xFF || a != 255 {
		t.Errorf("clipped rect edge = (R=%02X,G=%02X,B=%02X,A=%02X), want (FF,FF,FF,FF)", r, g, b, a)
	}
}

func TestFillRectRowBoundaryClamp(t *testing.T) {
	data := newFrameBuf()
	// Fill starting at col 95 with width 10 -- should NOT bleed into next row.
	// Row 10 should only have pixels 95..99 filled; (10,0) must remain zero.
	FillRect(data, testStride, 95, 10, 10, 1, 0xAA, 0xBB, 0xCC)
	b, g, r, a := pixelAt(data, testStride, 99, 10)
	if r != 0xAA || g != 0xBB || b != 0xCC || a != 255 {
		t.Errorf("last pixel of row = (R=%02X,G=%02X,B=%02X,A=%02X), want (AA,BB,CC,FF)", r, g, b, a)
	}
	b, g, r, a = pixelAt(data, testStride, 0, 11)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("pixel at next row start should be zero (no bleed), got (R=%02X,G=%02X,B=%02X,A=%02X)", r, g, b, a)
	}
}

func TestBlendGlyph(t *testing.T) {
	data := newFrameBuf()

	bitmap := make([]byte, 4*4)
	for i := range bitmap {
		bitmap[i] = 128
	}

	BlendGlyph(data, testStride, 10, 10, bitmap, 4, 4, 0xFF, 0xFF, 0xFF)

	b, g, r, a := pixelAt(data, testStride, 11, 11)
	if r == 0 || g == 0 || b == 0 {
		t.Errorf("blended pixel = (R=%02X,G=%02X,B=%02X,A=%02X), expected non-zero", r, g, b, a)
	}
	if a != 255 {
		t.Errorf("blended alpha = %02X, want FF", a)
	}

	var expected uint8 = uint8((uint16(0xFF)*128 + uint16(0)*127) / 255)
	if r != expected || g != expected || b != expected {
		t.Errorf("blended value = (R=%02X,G=%02X,B=%02X), expected %02X for all channels", r, g, b, expected)
	}

	b, g, r, a = pixelAt(data, testStride, 0, 0)
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("unblended pixel = (R=%02X,G=%02X,B=%02X), expected zero", r, g, b)
	}

	zeroBitmap := make([]byte, 4*4)
	BlendGlyph(data, testStride, 20, 20, zeroBitmap, 4, 4, 0xFF, 0x00, 0x00)
	b, g, r, a = pixelAt(data, testStride, 21, 21)
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("zero alpha blend = (R=%02X,G=%02X,B=%02X), expected zero", r, g, b)
	}

	fullBitmap := make([]byte, 2*2)
	for i := range fullBitmap {
		fullBitmap[i] = 255
	}
	BlendGlyph(data, testStride, 30, 30, fullBitmap, 2, 2, 0x80, 0x40, 0x20)
	b, g, r, a = pixelAt(data, testStride, 30, 30)
	if r != 0x80 || g != 0x40 || b != 0x20 || a != 255 {
		t.Errorf("full alpha blend = (R=%02X,G=%02X,B=%02X,A=%02X), want (80,40,20,FF)", r, g, b, a)
	}
}
