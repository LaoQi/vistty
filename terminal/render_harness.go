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

type RenderHarness struct {
	*Terminal
	compositor *render.Compositor
	faceCache  *font.FaceCache
	surface    platform.Surface
}

func NewRenderHarness(surface platform.Surface, opts Options) (*RenderHarness, error) {
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
		screen:     buf,
		cursor:     buf.Cursor(),
		parser:     parser,
		hostWriter: io.Discard,
		done:       make(chan struct{}),
		seqCh:      make(chan []vte.Sequence, 64),
		eofCh:      make(chan struct{}, 1),
		opts:       opts,
		mainBuf:    buf,
		altBuf:     altBuf,
		curFg:      screen.Color{IsDefault: true},
		curBg:      screen.Color{IsDefault: true},
		defFg:      screen.Color{R: 204, G: 204, B: 204},
		defBg:      screen.Color{R: 0, G: 0, B: 0},
		autoWrap:   true,
		charset:    newCharsetState(),
		active:     true,
		cols:       cols,
		rows:       rows,
	}
	t.initTabStops()

	return &RenderHarness{
		Terminal:   t,
		compositor: compositor,
		faceCache:  faceCache,
		surface:    surface,
	}, nil
}

func (h *RenderHarness) RenderFrame() error {
	return h.compositor.Render(h.Screen(), h.ScrollOffset())
}

func (h *RenderHarness) Close() error {
	h.Terminal.Close()
	if h.compositor != nil {
		h.compositor.Close()
	}
	if h.faceCache != nil {
		h.faceCache.Close()
	}
	return nil
}
