package replay

import (
	"fmt"
	"testing"
)

type workloadGen struct {
	name string
	fn   func(int) []byte
	size int
}

var workloads = []workloadGen{
	{"plain_text_4k", PlainText, 4096},
	{"plain_text_64k", PlainText, 65536},
	{"cjk_scroll_4k", CJKScroll, 4096},
	{"cjk_scroll_64k", CJKScroll, 65536},
	{"sgr_cursor_4k", SGRCursor, 4096},
	{"sgr_cursor_64k", SGRCursor, 65536},
	{"scroll_stress", func(_ int) []byte { return ScrollStress() }, 0},
	{"tui_redraw", func(_ int) []byte { return TUIRedraw() }, 0},
}

func BenchmarkLayers(b *testing.B) {
	layers := []Layer{LayerParser, LayerScreen, LayerRender}

	for _, wl := range workloads {
		data := wl.fn(wl.size)
		for _, layer := range layers {
			name := fmt.Sprintf("%s/%s", layer.String(), wl.name)
			b.Run(name, func(b *testing.B) {
				cfg := Config{
					Layer:      layer,
					Data:       data,
					Iterations: b.N,
					Cols:       80,
					Rows:       24,
					FontSize:   14,
					Workload:   wl.name,
				}
				b.ReportAllocs()
				b.ResetTimer()
				Bench(cfg)
			})
		}
	}
}

func boolPtr(v bool) *bool { return &v }

// BenchmarkRenderPaths 渲染路径矩阵 benchmark。
//
// 覆盖 Compositor 的两条 CPU 路径（GPU 路径不在此测量）：
//
//	direct        DirectRender=true（wl_shm 堆内存直写），逐帧全量重绘
//	backbuf_full  DirectRender=false + 每帧 DamageAll（DRM dumb buffer 滚动场景最坏情况：
//	              dirty 逐 cell 重绘 + copyAllToSurface 全帧拷贝）
//	backbuf_steady DirectRender=false 稳态（仅光标行重绘 + copyAllToSurface 全帧拷贝，
//	              用于度量 copy 成本本身，即 #6 脏行区间拷贝的收益上限）
//
// 网格：80x24（开发基准）与 240x67（1920x1072 像素，模拟 1080p 全屏）。
func BenchmarkRenderPaths(b *testing.B) {
	type pathMode struct {
		name       string
		direct     bool
		fullRedraw bool
	}
	modes := []pathMode{
		{"direct", true, false},
		{"backbuf_full", false, true},
		{"backbuf_steady", false, false},
	}
	grids := []struct {
		name       string
		cols, rows int
	}{
		{"80x24", 80, 24},
		{"240x67", 240, 67},
	}
	benchWorkloads := []workloadGen{
		{"plain_text_64k", PlainText, 65536},
		{"tui_redraw", func(_ int) []byte { return TUIRedraw() }, 0},
	}

	for _, wl := range benchWorkloads {
		data := wl.fn(wl.size)
		for _, grid := range grids {
			for _, mode := range modes {
				name := fmt.Sprintf("%s/%s/%s", mode.name, grid.name, wl.name)
				b.Run(name, func(b *testing.B) {
					cfg := Config{
						Layer:        LayerRender,
						Data:         data,
						Iterations:   b.N,
						Cols:         grid.cols,
						Rows:         grid.rows,
						FontSize:     14,
						Workload:     wl.name,
						FullRedraw:   mode.fullRedraw,
						DirectRender: boolPtr(mode.direct),
					}
					b.ReportAllocs()
					b.ResetTimer()
					Bench(cfg)
				})
			}
		}
	}
}

func TestBenchParserPlainText(t *testing.T) {
	data := PlainText(1024)
	cfg := Config{
		Layer:      LayerParser,
		Data:       data,
		Iterations: 10,
		Cols:       80,
		Rows:       24,
		FontSize:   14,
		Workload:   "test_plain",
	}
	r := Bench(cfg)
	if r.Iterations != 10 {
		t.Errorf("iterations = %d, want 10", r.Iterations)
	}
	if r.AllocsPerOp == 0 {
		t.Error("expected non-zero allocs for parser")
	}
}

func TestBenchScreenPlainText(t *testing.T) {
	data := PlainText(1024)
	cfg := Config{
		Layer:      LayerScreen,
		Data:       data,
		Iterations: 5,
		Cols:       80,
		Rows:       24,
		FontSize:   14,
		Workload:   "test_screen",
	}
	r := Bench(cfg)
	if r.Iterations != 5 {
		t.Errorf("iterations = %d, want 5", r.Iterations)
	}
	if r.AllocsPerOp == 0 {
		t.Error("expected non-zero allocs for screen")
	}
}

func TestBenchRenderPlainText(t *testing.T) {
	data := PlainText(256)
	cfg := Config{
		Layer:      LayerRender,
		Data:       data,
		Iterations: 3,
		Cols:       80,
		Rows:       24,
		FontSize:   14,
		Workload:   "test_render",
	}
	r := Bench(cfg)
	if r.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", r.Iterations)
	}
	if len(r.FrameTimes) != 3 {
		t.Errorf("frame times len = %d, want 3", len(r.FrameTimes))
	}
}

func TestWorkloadGenerators(t *testing.T) {
	wls := []struct {
		name string
		fn   func(int) []byte
		min  int
	}{
		{"PlainText", PlainText, 100},
		{"CJKScroll", CJKScroll, 100},
		{"SGRCursor", SGRCursor, 100},
	}
	for _, wl := range wls {
		t.Run(wl.name, func(t *testing.T) {
			data := wl.fn(wl.min)
			if len(data) == 0 {
				t.Errorf("%s returned empty", wl.name)
			}
		})
	}
	data := ScrollStress()
	if len(data) == 0 {
		t.Error("ScrollStress returned empty")
	}
	data = TUIRedraw()
	if len(data) == 0 {
		t.Error("TUIRedraw returned empty")
	}
}
