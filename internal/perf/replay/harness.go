package replay

import (
	"fmt"
	"runtime"
	"time"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/vte"
	"github.com/LaoQi/vistty/terminal"
)

type fakeSurface struct {
	w, h   int
	data   []byte
	stride int
}

func newFakeSurface(w, h int) *fakeSurface {
	return &fakeSurface{
		w:      w,
		h:      h,
		stride: w * 4,
		data:   make([]byte, w*4*h),
	}
}

func (s *fakeSurface) Size() (int, int)                   { return s.w, s.h }
func (s *fakeSurface) Data() []byte                       { return s.data }
func (s *fakeSurface) Stride() int                        { return s.stride }
func (s *fakeSurface) Swap() error                        { return nil }
func (s *fakeSurface) Close() error                       { return nil }
func (s *fakeSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return nil
}
func (s *fakeSurface) OutputID() uint32 { return 0 }

var _ platform.Surface = (*fakeSurface)(nil)

type Layer int

const (
	LayerParser Layer = iota
	LayerScreen
	LayerRender
)

func (l Layer) String() string {
	switch l {
	case LayerParser:
		return "L1-parser"
	case LayerScreen:
		return "L2-screen"
	case LayerRender:
		return "L3-render"
	default:
		return "unknown"
	}
}

type Result struct {
	Layer      Layer
	Workload   string
	Iterations int
	TotalTime  time.Duration
	PerOp      time.Duration
	AllocsPerOp float64
	BytesPerOp  int64
	FrameTimes []time.Duration
}

func (r Result) Header() string {
	return fmt.Sprintf("%-12s %-16s %8s %16s %16s %10s %12s",
		"layer", "workload", "iters", "total", "per-op", "allocs/op", "bytes/op")
}

func (r Result) String() string {
	return fmt.Sprintf("%-12s %-16s %8d %16s %16s %10.1f %12d",
		r.Layer.String(),
		r.Workload,
		r.Iterations,
		r.TotalTime.Round(time.Microsecond),
		r.PerOp.Round(time.Nanosecond),
		r.AllocsPerOp,
		r.BytesPerOp,
	)
}

type Config struct {
	Layer     Layer
	Data      []byte
	Iterations int
	Cols      int
	Rows      int
	FontSize  float64
	Workload  string
}

func Bench(cfg Config) Result {
	switch cfg.Layer {
	case LayerParser:
		return benchParser(cfg)
	case LayerScreen:
		return benchScreen(cfg)
	case LayerRender:
		return benchRender(cfg)
	default:
		return Result{Layer: cfg.Layer, Workload: cfg.Workload}
	}
}

func benchParser(cfg Config) Result {
	data := cfg.Data
	iters := cfg.Iterations

	var totalAllocs uint64
	var totalBytes uint64
	start := time.Now()

	for i := 0; i < iters; i++ {
		p := vte.NewParser()
		allocsBefore := uint64(runtime.NumGoroutine())
		_ = allocsBefore
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		allocsBefore = m.Mallocs
		bytesBefore := m.TotalAlloc

		p.FeedAll(data)

		runtime.ReadMemStats(&m)
		totalAllocs += m.Mallocs - allocsBefore
		totalBytes += m.TotalAlloc - bytesBefore
	}

	elapsed := time.Since(start)
	return Result{
		Layer:       LayerParser,
		Workload:    cfg.Workload,
		Iterations:  iters,
		TotalTime:   elapsed,
		PerOp:       elapsed / time.Duration(max(iters, 1)),
		AllocsPerOp: float64(totalAllocs) / float64(max(iters, 1)),
		BytesPerOp:  int64(totalBytes) / int64(max(iters, 1)),
	}
}

func benchScreen(cfg Config) Result {
	data := cfg.Data
	iters := cfg.Iterations

	surf := newFakeSurface(cfg.Cols*8, cfg.Rows*16)
	opts := terminal.DefaultOptions()
	opts.FontSize = cfg.FontSize
	opts.Width = cfg.Cols * 8
	opts.Height = cfg.Rows * 16

	var totalAllocs uint64
	var totalBytes uint64
	start := time.Now()

	for i := 0; i < iters; i++ {
		term, err := terminal.NewRenderHarness(surf, opts)
		if err != nil {
			return Result{Layer: LayerScreen, Workload: cfg.Workload, TotalTime: time.Since(start)}
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		allocsBefore := m.Mallocs
		bytesBefore := m.TotalAlloc

		term.FeedBytes(data)

		runtime.ReadMemStats(&m)
		totalAllocs += m.Mallocs - allocsBefore
		totalBytes += m.TotalAlloc - bytesBefore

		term.Close()
		runtime.GC()
	}

	elapsed := time.Since(start)
	return Result{
		Layer:       LayerScreen,
		Workload:    cfg.Workload,
		Iterations:  iters,
		TotalTime:   elapsed,
		PerOp:       elapsed / time.Duration(max(iters, 1)),
		AllocsPerOp: float64(totalAllocs) / float64(max(iters, 1)),
		BytesPerOp:  int64(totalBytes) / int64(max(iters, 1)),
	}
}

func benchRender(cfg Config) Result {
	data := cfg.Data
	iters := cfg.Iterations

	surf := newFakeSurface(cfg.Cols*8, cfg.Rows*16)
	opts := terminal.DefaultOptions()
	opts.FontSize = cfg.FontSize
	opts.Width = cfg.Cols * 8
	opts.Height = cfg.Rows * 16

	term, err := terminal.NewRenderHarness(surf, opts)
	if err != nil {
		return Result{Layer: LayerRender, Workload: cfg.Workload}
	}
	defer term.Close()

	term.FeedBytes(data)

	var totalAllocs uint64
	var totalBytes uint64
	frameTimes := make([]time.Duration, 0, iters)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	allocsBefore := m.Mallocs
	bytesBefore := m.TotalAlloc

	start := time.Now()

	for i := 0; i < iters; i++ {
		frameStart := time.Now()
		term.RenderFrame()
		frameTimes = append(frameTimes, time.Since(frameStart))
	}

	elapsed := time.Since(start)

	runtime.ReadMemStats(&m)
	totalAllocs = m.Mallocs - allocsBefore
	totalBytes = m.TotalAlloc - bytesBefore

	return Result{
		Layer:       LayerRender,
		Workload:    cfg.Workload,
		Iterations:  iters,
		TotalTime:   elapsed,
		PerOp:       elapsed / time.Duration(max(iters, 1)),
		AllocsPerOp: float64(totalAllocs) / float64(max(iters, 1)),
		BytesPerOp:  int64(totalBytes) / int64(max(iters, 1)),
		FrameTimes:  frameTimes,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
