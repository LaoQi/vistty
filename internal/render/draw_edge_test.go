package render

// draw_edge_test.go — draw.go 像素级正确性测试体系。
//
// 结构：
//  1. ref* 参考实现：当前 draw.go 实现的逐字拷贝，作为行为基线。
//     #1（行级裁剪）/#2（ca=255 特化）/#5（SWAR）等优化必须
//     在参考实现定义的行为域内产生逐字节一致的结果。
//  2. 表驱动边缘用例：裁剪、alpha 极值、零尺寸、角落、stride padding。
//  3. 随机化差异化对拍：固定种子、可复现，覆盖实现与参考一致的定义域。
//  4. 舍入精确性穷举：混合公式 (src*a + dst*ia + 128)>>8 必须逐位保持。
//  5. quirk 测试：显式固定当前已知的越行 bleed 行为（见各 TestQuirk*）。
//
// 已知行为域限制（quirk，见文件末尾）：
// 当前实现水平方向只按 [0, len(data)) 检查，不做行内裁剪，
// x<0 或 x+w 超出行宽时会 bleed 到上一行末尾/下一行开头。
// 随机对拍因此只生成 x∈[0, bufW-w] 的用例（垂直裁剪当前实现是正确的）。

import (
	"fmt"
	"math/rand"
	"testing"
)

// ============================================================
// 参考实现（draw.go 当前语义的逐字拷贝，优化期不得修改）
// ============================================================

func refFillRect(data []byte, stride int, x, y, w, h int, r, g, b uint8) {
	pixel := uint32(255)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	rowEnd := func(row int) int { return row*stride + stride }
	for row := y; row < y+h; row++ {
		startOff := row*stride + x*4
		endOff := startOff + w*4
		if startOff < 0 || row < 0 {
			continue
		}
		if re := rowEnd(row); endOff > re {
			endOff = re
		}
		if endOff > len(data) {
			endOff = len(data)
		}
		if endOff <= startOff {
			continue
		}
		for off := startOff; off+4 <= endOff; off += 4 {
			data[off+0] = byte(pixel)
			data[off+1] = byte(pixel >> 8)
			data[off+2] = byte(pixel >> 16)
			data[off+3] = byte(pixel >> 24)
		}
	}
}

func refFillRectBlend(data []byte, stride int, x, y, w, h int, r, g, b, a uint8) {
	if a == 0 {
		return
	}
	if a == 255 {
		refFillRect(data, stride, x, y, w, h, r, g, b)
		return
	}
	rowEnd := func(row int) int { return row*stride + stride }
	for row := y; row < y+h; row++ {
		startOff := row*stride + x*4
		endOff := startOff + w*4
		if startOff < 0 || row < 0 {
			continue
		}
		if re := rowEnd(row); endOff > re {
			endOff = re
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

func refBlendGlyphAlpha(data []byte, stride int, x, y int, bitmap []byte, glyphW, glyphH int, r, g, b, ca uint8) {
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

func refBlendColorGlyph(data []byte, stride int, x, y int, rgba []byte, glyphW, glyphH int) {
	for gy := 0; gy < glyphH; gy++ {
		row := y + gy
		offset := row * stride
		if offset < 0 || offset >= len(data) {
			continue
		}
		for gx := 0; gx < glyphW; gx++ {
			srcOff := (gy*glyphW + gx) * 4
			sr := rgba[srcOff]
			sg := rgba[srcOff+1]
			sb := rgba[srcOff+2]
			sa := rgba[srcOff+3]
			if sa == 0 {
				continue
			}
			col := x + gx
			px := offset + col*4
			if px < 0 || px+3 >= len(data) {
				continue
			}
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

// ============================================================
// 工具
// ============================================================

type fb struct {
	data         []byte
	stride, w, h int
}

// newFB 创建 w×h 可见区域 + 每行 pad 字节 padding 的帧缓冲。
func newFB(w, h, pad int) fb {
	stride := w*4 + pad
	return fb{data: make([]byte, stride*h), stride: stride, w: w, h: h}
}

func (f fb) clone() fb {
	d := make([]byte, len(f.data))
	copy(d, f.data)
	return fb{data: d, stride: f.stride, w: f.w, h: f.h}
}

// fillRandom 用确定性随机内容填充（混合依赖 dst 初值，必须覆盖）。
func (f fb) fillRandom(rng *rand.Rand) {
	rng.Read(f.data)
}

func (f fb) at(x, y int) (b, g, r, a uint8) {
	off := y*f.stride + x*4
	return f.data[off], f.data[off+1], f.data[off+2], f.data[off+3]
}

func assertSameFB(t *testing.T, name string, got, want fb) {
	t.Helper()
	if len(got.data) != len(want.data) {
		t.Fatalf("%s: buffer len %d != %d", name, len(got.data), len(want.data))
	}
	for i := range got.data {
		if got.data[i] != want.data[i] {
			row := i / want.stride
			col := (i % want.stride) / 4
			ch := i % 4
			t.Fatalf("%s: first diff at byte %d (row=%d col=%d ch=%d): got %02x want %02x",
				name, i, row, col, ch, got.data[i], want.data[i])
		}
	}
}

// ============================================================
// 表驱动边缘用例
// ============================================================

func TestFillRectEdgeCases(t *testing.T) {
	cases := []struct {
		name         string
		w, h, pad    int // 帧缓冲几何
		x, y, rw, rh int // 矩形
	}{
		{"full", 16, 8, 0, 0, 0, 16, 8},
		{"zero_size", 16, 8, 0, 4, 4, 0, 0},
		{"zero_w", 16, 8, 0, 4, 4, 0, 4},
		{"zero_h", 16, 8, 0, 4, 4, 4, 0},
		{"right_edge_exact", 16, 8, 0, 14, 3, 2, 1},
		{"bottom_edge_exact", 16, 8, 0, 3, 6, 1, 2},
		{"right_overflow", 16, 8, 0, 14, 3, 8, 1},
		{"bottom_overflow", 16, 8, 0, 3, 6, 1, 8},
		{"y_negative_partial", 16, 8, 0, 2, -3, 4, 5},
		{"y_fully_above", 16, 8, 0, 2, -5, 4, 3},
		{"y_fully_below", 16, 8, 0, 2, 100, 4, 3},
		{"x_fully_right", 16, 8, 0, 100, 2, 4, 3},
		{"single_pixel_origin", 16, 8, 0, 0, 0, 1, 1},
		{"single_pixel_last", 16, 8, 0, 15, 7, 1, 1},
		{"tiny_buffer", 1, 1, 0, 0, 0, 1, 1},
		{"tiny_buffer_overflow", 1, 1, 0, 0, 0, 5, 5},
		{"pad_exact_visible", 16, 8, 16, 0, 0, 16, 8},
		{"pad_into_padding", 16, 8, 16, 14, 2, 6, 2}, // x+w 超出可见宽，写入 padding（rowEnd=stride 语义）
		{"pad_bottom_overflow", 16, 8, 16, 0, 6, 4, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := newFB(tc.w, tc.h, tc.pad)
			base.fillRandom(rand.New(rand.NewSource(42)))
			got := base.clone()
			want := base.clone()
			FillRect(got.data, got.stride, tc.x, tc.y, tc.rw, tc.rh, 0x11, 0x22, 0x33)
			refFillRect(want.data, want.stride, tc.x, tc.y, tc.rw, tc.rh, 0x11, 0x22, 0x33)
			assertSameFB(t, "FillRect", got, want)
		})
	}
}

func TestFillRectBlendEdgeCases(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		x, y, rw, rh int
		a            uint8
	}{
		{"alpha0_nowrite", 16, 8, 2, 2, 4, 4, 0},
		{"alpha1", 16, 8, 2, 2, 4, 4, 1},
		{"alpha128", 16, 8, 2, 2, 4, 4, 128},
		{"alpha254", 16, 8, 2, 2, 4, 4, 254},
		{"alpha255_opaque", 16, 8, 2, 2, 4, 4, 255},
		{"right_overflow", 16, 8, 13, 2, 8, 2, 100},
		{"bottom_overflow", 16, 8, 2, 6, 4, 8, 100},
		{"y_negative_partial", 16, 8, 2, -2, 4, 4, 100},
		{"y_fully_below", 16, 8, 2, 50, 4, 4, 100},
		{"zero_size", 16, 8, 2, 2, 0, 0, 100},
		{"tiny_buffer", 1, 1, 0, 0, 3, 3, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := newFB(tc.w, tc.h, 0)
			base.fillRandom(rand.New(rand.NewSource(7)))
			got := base.clone()
			want := base.clone()
			FillRectBlend(got.data, got.stride, tc.x, tc.y, tc.rw, tc.rh, 0x11, 0x22, 0x33, tc.a)
			refFillRectBlend(want.data, want.stride, tc.x, tc.y, tc.rw, tc.rh, 0x11, 0x22, 0x33, tc.a)
			assertSameFB(t, "FillRectBlend", got, want)
		})
	}
}

// FillRectBlend alpha=0 必须完全不动帧缓冲（显式断言，不依赖参考）。
func TestFillRectBlendAlpha0NoWrite(t *testing.T) {
	f := newFB(8, 4, 0)
	f.fillRandom(rand.New(rand.NewSource(1)))
	snapshot := f.clone()
	FillRectBlend(f.data, f.stride, 0, 0, 8, 4, 0xFF, 0xFF, 0xFF, 0)
	assertSameFB(t, "alpha0", f, snapshot)
}

func TestBlendGlyphAlphaEdgeCases(t *testing.T) {
	// bitmap 内容：混合 0/半透/全透，覆盖 alpha==0 continue、combined==255 快路径
	mkBitmap := func(w, h int, fill func(x, y int) uint8) []byte {
		bm := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				bm[y*w+x] = fill(x, y)
			}
		}
		return bm
	}
	grad := func(x, y int) uint8 { return uint8((x*16 + y*7) % 256) }

	cases := []struct {
		name   string
		w, h   int // 帧缓冲
		gw, gh int // 字形
		x, y   int
		ca     uint8
		bitmap []byte
	}{
		{"center", 16, 16, 4, 4, 6, 6, 255, mkBitmap(4, 4, grad)},
		{"all_zero_alpha", 16, 16, 4, 4, 6, 6, 255, make([]byte, 16)},
		{"all_full_alpha", 16, 16, 4, 4, 6, 6, 255, mkBitmap(4, 4, func(x, y int) uint8 { return 255 })},
		{"ca0_nowrite", 16, 16, 4, 4, 6, 6, 0, mkBitmap(4, 4, grad)},
		{"ca1", 16, 16, 4, 4, 6, 6, 1, mkBitmap(4, 4, grad)},
		{"ca128", 16, 16, 4, 4, 6, 6, 128, mkBitmap(4, 4, grad)},
		{"ca254", 16, 16, 4, 4, 6, 6, 254, mkBitmap(4, 4, grad)},
		{"origin", 16, 16, 4, 4, 0, 0, 255, mkBitmap(4, 4, grad)},
		{"top_clip_partial", 16, 16, 4, 4, 6, -2, 255, mkBitmap(4, 4, grad)},
		{"top_clip_fully", 16, 16, 4, 4, 6, -10, 255, mkBitmap(4, 4, grad)},
		{"bottom_clip_partial", 16, 16, 4, 4, 6, 14, 255, mkBitmap(4, 4, grad)},
		{"bottom_clip_fully", 16, 16, 4, 4, 6, 100, 255, mkBitmap(4, 4, grad)},
		{"right_edge_exact", 16, 16, 4, 4, 12, 6, 255, mkBitmap(4, 4, grad)},
		{"glyph_1x1", 16, 16, 1, 1, 15, 15, 255, []byte{200}},
		{"glyph_zero_w", 16, 16, 0, 4, 6, 6, 255, nil},
		{"glyph_zero_h", 16, 16, 4, 0, 6, 6, 255, nil},
		{"glyph_wider_than_half", 8, 8, 6, 6, 2, 1, 200, mkBitmap(6, 6, grad)},
		{"tiny_buffer", 1, 1, 1, 1, 0, 0, 255, []byte{255}},
		{"buffer_1px_wide", 1, 8, 1, 4, 0, 2, 255, mkBitmap(1, 4, grad)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := newFB(tc.w, tc.h, 0)
			base.fillRandom(rand.New(rand.NewSource(99)))
			got := base.clone()
			want := base.clone()
			BlendGlyphAlpha(got.data, got.stride, tc.x, tc.y, tc.bitmap, tc.gw, tc.gh, 0xAA, 0xBB, 0xCC, tc.ca)
			refBlendGlyphAlpha(want.data, want.stride, tc.x, tc.y, tc.bitmap, tc.gw, tc.gh, 0xAA, 0xBB, 0xCC, tc.ca)
			assertSameFB(t, "BlendGlyphAlpha", got, want)
		})
	}
}

func TestBlendColorGlyphEdgeCases(t *testing.T) {
	mkRGBA := func(w, h int, fill func(x, y int) (r, g, b, a uint8)) []byte {
		p := make([]byte, w*h*4)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, a := fill(x, y)
				off := (y*w + x) * 4
				p[off], p[off+1], p[off+2], p[off+3] = r, g, b, a
			}
		}
		return p
	}
	mixed := func(x, y int) (uint8, uint8, uint8, uint8) {
		return uint8(x * 40), uint8(y * 30), uint8((x + y) * 17), uint8((x*50 + y*23) % 256)
	}

	cases := []struct {
		name   string
		w, h   int
		gw, gh int
		x, y   int
		rgba   []byte
	}{
		{"center_mixed_alpha", 16, 16, 4, 4, 6, 6, mkRGBA(4, 4, mixed)},
		{"all_sa0", 16, 16, 4, 4, 6, 6, make([]byte, 4*4*4)},
		{"all_sa255", 16, 16, 4, 4, 6, 6, mkRGBA(4, 4, func(x, y int) (uint8, uint8, uint8, uint8) {
			return uint8(x * 60), uint8(y * 60), 0x80, 255
		})},
		{"top_clip_partial", 16, 16, 4, 4, 6, -2, mkRGBA(4, 4, mixed)},
		{"top_clip_fully", 16, 16, 4, 4, 6, -10, mkRGBA(4, 4, mixed)},
		{"bottom_clip_partial", 16, 16, 4, 4, 6, 14, mkRGBA(4, 4, mixed)},
		{"right_edge_exact", 16, 16, 4, 4, 12, 6, mkRGBA(4, 4, mixed)},
		{"glyph_1x1", 16, 16, 1, 1, 15, 15, []byte{10, 20, 30, 128}},
		{"glyph_zero_size", 16, 16, 0, 0, 6, 6, nil},
		{"tiny_buffer", 1, 1, 1, 1, 0, 0, []byte{1, 2, 3, 200}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := newFB(tc.w, tc.h, 0)
			base.fillRandom(rand.New(rand.NewSource(55)))
			got := base.clone()
			want := base.clone()
			blendColorGlyph(got.data, got.stride, tc.x, tc.y, tc.rgba, tc.gw, tc.gh)
			refBlendColorGlyph(want.data, want.stride, tc.x, tc.y, tc.rgba, tc.gw, tc.gh)
			assertSameFB(t, "blendColorGlyph", got, want)
		})
	}
}

// ============================================================
// 舍入精确性穷举
// 混合公式 (src*a + dst*ia + 128) >> 8 必须逐位保持——
// SWAR/查表等优化最容易在舍入上产生 ±1 偏差。
// ============================================================

// TestBlendGlyphRoundingExhaustive 穷举 alpha∈[0,255] ×
// src/dst 代表值，验证 BlendGlyph（ca=255 路径）每个通道
// 与混合公式严格一致。
func TestBlendGlyphRoundingExhaustive(t *testing.T) {
	vals := []uint8{0, 1, 2, 127, 128, 129, 253, 254, 255}
	for alpha := 0; alpha <= 255; alpha++ {
		for _, src := range vals {
			for _, dst := range vals {
				f := newFB(1, 1, 0)
				f.data[0], f.data[1], f.data[2], f.data[3] = dst, dst, dst, dst
				BlendGlyph(f.data, f.stride, 0, 0, []byte{uint8(alpha)}, 1, 1, src, src, src)
				a := uint16(alpha)
				ia := uint16(255 - alpha)
				var want uint8
				if alpha == 0 {
					want = dst
				} else if alpha == 255 {
					want = src
				} else {
					want = uint8((uint16(src)*a + uint16(dst)*ia + 128) >> 8)
				}
				b, g, r, _ := f.at(0, 0)
				if b != want || g != want || r != want {
					t.Fatalf("alpha=%d src=%d dst=%d: got (%d,%d,%d) want %d",
						alpha, src, dst, r, g, b, want)
				}
			}
		}
	}
}

// TestBlendGlyphAlphaCombinedRounding 验证 combined = alpha*ca/255
// 的边界：alpha×ca 最大值 65025 不溢出 uint16，combined==255 快路径
// 仅在 alpha==255 && ca==255 时触发。
func TestBlendGlyphAlphaCombinedRounding(t *testing.T) {
	// alpha=255, ca=255 → combined=255 → 快路径写死 fg
	f := newFB(1, 1, 0)
	f.data[0], f.data[1], f.data[2], f.data[3] = 9, 9, 9, 9
	BlendGlyphAlpha(f.data, f.stride, 0, 0, []byte{255}, 1, 1, 0x11, 0x22, 0x33, 255)
	b, g, r, a := f.at(0, 0)
	if b != 0x33 || g != 0x22 || r != 0x11 || a != 255 {
		t.Fatalf("combined=255 fast path = (%02x,%02x,%02x,%02x), want (33,22,11,ff)", b, g, r, a)
	}

	// alpha=255, ca=254 → combined=254（非快路径）
	f2 := newFB(1, 1, 0)
	BlendGlyphAlpha(f2.data, f2.stride, 0, 0, []byte{255}, 1, 1, 0xFF, 0xFF, 0xFF, 254)
	combined := uint16(255) * 254 / 255
	want := uint8((255*combined + 0*(255-combined) + 128) >> 8)
	b, _, _, _ = f2.at(0, 0)
	if b != want {
		t.Fatalf("combined=%d: got %d want %d", combined, b, want)
	}

	// alpha=1, ca=1 → combined=0 → 不写
	f3 := newFB(1, 1, 0)
	f3.data[2] = 0x77
	BlendGlyphAlpha(f3.data, f3.stride, 0, 0, []byte{1}, 1, 1, 0xFF, 0xFF, 0xFF, 1)
	if f3.data[2] != 0x77 {
		t.Fatalf("combined=0 should not write, got %02x", f3.data[2])
	}
}

// ============================================================
// 随机化差异化对拍（固定种子，可复现）
// 定义域限制：x∈[0, bufW-gw]（避开水平 bleed quirk）；
// y 全域（垂直裁剪当前实现正确）。
// ============================================================

func TestDifferentialFillRect(t *testing.T) {
	rng := rand.New(rand.NewSource(20260818))
	for i := 0; i < 300; i++ {
		w := 1 + rng.Intn(48)
		h := 1 + rng.Intn(24)
		pad := []int{0, 0, 0, 8, 16}[rng.Intn(5)]
		stride := w*4 + pad
		maxW := stride/4 - rng.Intn(w) // x+w 允许进入 padding 但不超 stride
		x := rng.Intn(maxW)
		rw := rng.Intn(stride/4 - x + 1)
		y := -2 - rng.Intn(4) + rng.Intn(h+8)
		rh := rng.Intn(h + 6)

		base := newFB(w, h, pad)
		base.fillRandom(rng)
		got, want := base.clone(), base.clone()
		r, g, b := uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))

		FillRect(got.data, got.stride, x, y, rw, rh, r, g, b)
		refFillRect(want.data, want.stride, x, y, rw, rh, r, g, b)
		assertSameFB(t, fmt.Sprintf("iter%d FillRect(x=%d,y=%d,w=%d,h=%d,buf=%dx%d pad=%d)", i, x, y, rw, rh, w, h, pad), got, want)

		a := uint8(rng.Intn(256))
		got, want = base.clone(), base.clone()
		FillRectBlend(got.data, got.stride, x, y, rw, rh, r, g, b, a)
		refFillRectBlend(want.data, want.stride, x, y, rw, rh, r, g, b, a)
		assertSameFB(t, fmt.Sprintf("iter%d FillRectBlend(a=%d)", i, a), got, want)
	}
}

func TestDifferentialBlendGlyphAlpha(t *testing.T) {
	rng := rand.New(rand.NewSource(20260819))
	for i := 0; i < 300; i++ {
		w := 1 + rng.Intn(48)
		h := 1 + rng.Intn(24)
		gw := rng.Intn(w + 1)
		gh := rng.Intn(24)
		x := 0
		if w-gw > 0 {
			x = rng.Intn(w - gw + 1)
		}
		y := -2 - rng.Intn(4) + rng.Intn(h+8)

		bitmap := make([]byte, gw*gh)
		rng.Read(bitmap)
		// 掺入 alpha 极值
		if len(bitmap) > 0 {
			bitmap[0] = 0
			if len(bitmap) > 1 {
				bitmap[len(bitmap)-1] = 255
			}
		}
		ca := []uint8{0, 1, 128, 254, 255, uint8(rng.Intn(256))}[rng.Intn(6)]
		r, g, b := uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))

		base := newFB(w, h, 0)
		base.fillRandom(rng)
		got, want := base.clone(), base.clone()

		BlendGlyphAlpha(got.data, got.stride, x, y, bitmap, gw, gh, r, g, b, ca)
		refBlendGlyphAlpha(want.data, want.stride, x, y, bitmap, gw, gh, r, g, b, ca)
		assertSameFB(t, fmt.Sprintf("iter%d BlendGlyphAlpha(x=%d,y=%d,g=%dx%d,ca=%d,buf=%dx%d)", i, x, y, gw, gh, ca, w, h), got, want)
	}
}

func TestDifferentialBlendColorGlyph(t *testing.T) {
	rng := rand.New(rand.NewSource(20260820))
	for i := 0; i < 300; i++ {
		w := 1 + rng.Intn(48)
		h := 1 + rng.Intn(24)
		gw := rng.Intn(w + 1)
		gh := rng.Intn(24)
		x := 0
		if w-gw > 0 {
			x = rng.Intn(w - gw + 1)
		}
		y := -2 - rng.Intn(4) + rng.Intn(h+8)

		rgba := make([]byte, gw*gh*4)
		rng.Read(rgba)
		if len(rgba) >= 4 {
			rgba[3] = 0 // sa=0
			rgba[len(rgba)-1] = 255
		}

		base := newFB(w, h, 0)
		base.fillRandom(rng)
		got, want := base.clone(), base.clone()

		blendColorGlyph(got.data, got.stride, x, y, rgba, gw, gh)
		refBlendColorGlyph(want.data, want.stride, x, y, rgba, gw, gh)
		assertSameFB(t, fmt.Sprintf("iter%d blendColorGlyph(x=%d,y=%d,g=%dx%d,buf=%dx%d)", i, x, y, gw, gh, w, h), got, want)
	}
}

// ============================================================
// 行级裁剪测试（#1 优化后的正确行为）。
// 原 quirk 测试（bleed 到上一行末尾/下一行开头）随 #1 优化翻转：
// 负 x 与右侧越界现在按行正确裁剪，只写本行合法区域。
// 任何在此产生像素的回归都会被这些断言捕获。
// ============================================================

// TestFillRectNegativeXClipsToRow FillRect x<0：只写本行 [0, x+w)，
// 不再写入上一行末尾像素。
func TestFillRectNegativeXClipsToRow(t *testing.T) {
	f := newFB(10, 4, 0)
	FillRect(f.data, f.stride, -1, 1, 2, 1, 0xFF, 0xFF, 0xFF)
	// 本行 [0, x+w) = [0,1)：row1 pixel0 写入
	if got := f.data[1*f.stride+2]; got != 0xFF {
		t.Fatalf("row1 pixel0 = %02x, want FF (in-row clip)", got)
	}
	// x+w = 1 → row1 pixel1 不写
	if got := f.data[1*f.stride+6]; got != 0 {
		t.Fatalf("row1 pixel1 = %02x, want 00 (outside clipped rect)", got)
	}
	// 上一行末尾不得被写入（无 bleed）
	if got := f.data[0*f.stride+9*4+2]; got != 0 {
		t.Fatalf("row0 last pixel = %02x, want 00 (no bleed)", got)
	}
}

// TestBlendGlyphNegativeXClipsToRow BlendGlyph x<0：col<0 的像素跳过，
// 可见列正常绘制，不写入上一行末尾。
func TestBlendGlyphNegativeXClipsToRow(t *testing.T) {
	f := newFB(10, 4, 0)
	bitmap := []byte{255, 255} // 2x1 字形，gx=0 在屏外、gx=1 可见
	BlendGlyph(f.data, f.stride, -1, 1, bitmap, 2, 1, 0xFF, 0xFF, 0xFF)
	// gx=1 → col=0 → row1 pixel0 正常绘制
	if got := f.data[1*f.stride+2]; got != 0xFF {
		t.Fatalf("row1 pixel0 = %02x, want FF (visible column)", got)
	}
	// gx=0 → col=-1 → 跳过（无 bleed 到上一行末尾）
	if got := f.data[0*f.stride+9*4+2]; got != 0 {
		t.Fatalf("row0 last pixel = %02x, want 00 (no bleed)", got)
	}

	// y=0 时同样不写上一行（负 x 全屏外也安全）
	f2 := newFB(10, 4, 0)
	BlendGlyph(f2.data, f2.stride, -1, 0, bitmap, 2, 1, 0xFF, 0xFF, 0xFF)
	if got := f2.data[9*4+2]; got != 0 {
		t.Fatalf("y=0 row0 pixel9 = %02x, want 00 (no bleed)", got)
	}
}

// TestBlendGlyphRightOverflowClipsToRow x+w 超出行宽：越界像素跳过，
// 不写入下一行开头。
func TestBlendGlyphRightOverflowClipsToRow(t *testing.T) {
	f := newFB(10, 4, 0)
	bitmap := []byte{255, 255}
	BlendGlyph(f.data, f.stride, 9, 0, bitmap, 2, 1, 0xFF, 0xFF, 0xFF)
	// gx=0 → col=9：最后可见列正常绘制
	if got := f.data[0*f.stride+9*4+2]; got != 0xFF {
		t.Fatalf("row0 pixel9 = %02x, want FF (visible column)", got)
	}
	// gx=1 → col=10 → 裁剪，不写入下一行开头（无 bleed）
	if got := f.data[1*f.stride+2]; got != 0 {
		t.Fatalf("row1 pixel0 = %02x, want 00 (no bleed)", got)
	}
}
