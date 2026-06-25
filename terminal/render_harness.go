package terminal

import (
	"fmt"
	"io"
	"os"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/vte"
)

// NewRenderHarness constructs a Terminal with a fully initialized render
// pipeline (font, compositor, buffer) but without starting a PTY, input
// source, backend event loop, or any goroutine. It is intended for
// performance measurement and offline replay scenarios.
//
// The surface is provided by the caller — typically a fakeSurface in tests
// or a real backend surface for online profiling. hostWriter is set to
// io.Discard so DSR/DA responses do not interfere with measurement.
func NewRenderHarness(surface platform.Surface, opts Options) (*Terminal, error) {
	var fontData []byte
	if opts.FontPath != "" {
		data, err := os.ReadFile(opts.FontPath)
		if err != nil {
			return nil, fmt.Errorf("read font file: %w", err)
		}
		fontData = data
	} else {
		fontData = font.EmbeddedFontData()
	}

	faceCache, err := font.NewFaceCache(fontData, 72)
	if err != nil {
		return nil, fmt.Errorf("load font: %w", err)
	}

	face, err := faceCache.Get(opts.FontSize)
	if err != nil {
		faceCache.Close()
		return nil, fmt.Errorf("get face: %w", err)
	}

	m := face.Metrics()
	w, h := surface.Size()
	cols := w / m.Width
	rows := 0
	if m.Height > 0 {
		rows = h / m.Height
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	buf := screen.NewBuffer(cols, rows)
	altBuf := screen.NewBuffer(cols, rows)
	altBuf.SetAltScreen(true)
	compositor := render.NewCompositor(surface, face)
	parser := vte.NewParser()

	t := &Terminal{
		screen:          buf,
		cursor:          buf.Cursor(),
		parser:          parser,
		hostWriter:      io.Discard,
		compositor:      compositor,
		surface:         surface,
		face:            face,
		faceCache:       faceCache,
		fontData:        fontData,
		initialFontSize: opts.FontSize,
		scaleReqCh:      make(chan scaleReq, 1),
		done:            make(chan struct{}),
		seqCh:           make(chan []vte.Sequence, 64),
		eofCh:           make(chan struct{}, 1),
		opts:            opts,
		mainBuf:         buf,
		altBuf:          altBuf,
		curFg:           screen.Color{IsDefault: true},
		curBg:           screen.Color{IsDefault: true},
		autoWrap:        true,
		charset:         newCharsetState(),
	}
	t.initTabStops()
	return t, nil
}

// FeedBytes is the exported alias of feedBytes for use by external
// measurement tools. It parses data through the VTE parser and executes
// all resulting sequences against the terminal screen.
func (t *Terminal) FeedBytes(data []byte) {
	t.feedBytes(data)
}

// RenderFrame renders the current screen state to the surface and calls
// Swap. It is the measurement entry point for L3 (full render) benchmarks.
func (t *Terminal) RenderFrame() error {
	return t.compositor.Render(t.screen, t.scrollOffset)
}

// EnableFPSLogging enables per-frame timing output to stderr. When enabled,
// the eventLoop prints the Render+Swap duration for each frame.
func (t *Terminal) EnableFPSLogging() {
	t.fpsLogging = true
}
