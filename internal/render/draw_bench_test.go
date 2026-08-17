package render

// draw_bench_test.go — draw.go 各原语的微基准。
// 为 #1（行级裁剪）/#2（ca=255 特化）/#5（SWAR）提供 A/B 对比基线。
// 帧缓冲固定 1920x1080，模拟生产环境尺寸。

import "testing"

var benchFB = func() []byte {
	return make([]byte, 1920*4*1080)
}()

const benchStride = 1920 * 4

// 典型 ASCII 字形：8x16，~60% 覆盖率（模拟抗锯齿边缘的混合 alpha）
func benchGlyphBitmap() []byte {
	bm := make([]byte, 8*16)
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			switch {
			case x == 0 || x == 7 || y == 0 || y == 15:
				bm[y*8+x] = 0
			case x == 1 || x == 6 || y == 1 || y == 14:
				bm[y*8+x] = 128 // 抗锯齿边缘
			default:
				bm[y*8+x] = 255
			}
		}
	}
	return bm
}

// emoji 彩色字形：18x16（2*cellW x cellH），混合 sa
func benchColorBitmap() []byte {
	p := make([]byte, 18*16*4)
	for i := 0; i < 18*16; i++ {
		p[i*4+0] = uint8(i * 3)
		p[i*4+1] = uint8(i * 5)
		p[i*4+2] = uint8(i * 7)
		switch i % 4 {
		case 0:
			p[i*4+3] = 255
		case 1:
			p[i*4+3] = 0
		default:
			p[i*4+3] = uint8(i)
		}
	}
	return p
}

func BenchmarkBlendGlyph(b *testing.B) {
	bm := benchGlyphBitmap()
	b.ReportAllocs()
	b.SetBytes(int64(len(bm) * 4)) // 输出像素字节数
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlendGlyph(benchFB, benchStride, 100, 100, bm, 8, 16, 0xCC, 0xCC, 0xCC)
	}
}

func BenchmarkBlendGlyphAlphaTranslucent(b *testing.B) {
	bm := benchGlyphBitmap()
	b.ReportAllocs()
	b.SetBytes(int64(len(bm) * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// ca=200：overlay/半透明场景，走 combined 计算路径
		BlendGlyphAlpha(benchFB, benchStride, 100, 100, bm, 8, 16, 0xCC, 0xCC, 0xCC, 200)
	}
}

func BenchmarkBlendColorGlyph(b *testing.B) {
	rgba := benchColorBitmap()
	b.ReportAllocs()
	b.SetBytes(int64(len(rgba)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blendColorGlyph(benchFB, benchStride, 100, 100, rgba, 18, 16)
	}
}

func BenchmarkFillRectCell(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 16 * 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FillRect(benchFB, benchStride, 100, 100, 8, 16, 0x20, 0x20, 0x20)
	}
}

func BenchmarkFillRectFullScreen(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchFB)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FillRect(benchFB, benchStride, 0, 0, 1920, 1080, 0x20, 0x20, 0x20)
	}
}

func BenchmarkFillRectBlendHalf(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 16 * 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FillRectBlend(benchFB, benchStride, 100, 100, 8, 16, 0x20, 0x20, 0x20, 128)
	}
}
