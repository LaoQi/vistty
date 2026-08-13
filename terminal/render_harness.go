package terminal

import (
	"fmt"
	"io"

	"github.com/LaoQi/vistty/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/render"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/vte"
)

type RenderHarness struct {
	*Terminal
	compositor *render.Compositor
	faceCache  font.FaceCacheProvider
	surface    platform.Surface
}

func NewRenderHarness(surface platform.Surface, opts Options) (*RenderHarness, error) {
	fontData, extraDatas, err := font.ResolveFontChain(opts.FontPath, opts.FallbackFontPath)
	if err != nil {
		return nil, err
	}

	var faceCache font.FaceCacheProvider
	if len(extraDatas) > 0 {
		fc, err := font.NewChainFaceCache(fontData, extraDatas, 72)
		if err != nil {
			return nil, fmt.Errorf("load font chain: %w", err)
		}
		faceCache = fc
	} else {
		fc, err := font.NewFaceCache(fontData, 72)
		if err != nil {
			return nil, fmt.Errorf("load font: %w", err)
		}
		faceCache = fc
	}

	face, err := faceCache.GetFace(opts.FontSize)
	if err != nil {
		faceCache.Close()
		return nil, fmt.Errorf("get face: %w", err)
	}

	m := face.Metrics()
	if m.Width <= 0 {
		m.Width = 8
	}
	if m.Height <= 0 {
		m.Height = 16
	}
	w, h := surface.Size()
	cols := w / m.Width
	rows := h / m.Height
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	buf := screen.NewBuffer(cols, rows, 1000)
	altBuf := screen.NewBuffer(cols, rows, 0)
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
