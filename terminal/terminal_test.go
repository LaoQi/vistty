package terminal

import (
	"bytes"
	"io"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/screen"
	"github.com/LaoQi/vistty/internal/vte"
)

type fakeSurface struct {
	w, h    int
	data    []byte
	stride  int
	resizeC chan platform.ResizeEvent
}

func newFakeSurface(w, h int) *fakeSurface {
	return &fakeSurface{
		w: w, h: h, stride: w * 4,
		data:    make([]byte, w*4*h),
		resizeC: make(chan platform.ResizeEvent, 4),
	}
}
func (s *fakeSurface) Size() (int, int)             { return s.w, s.h }
func (s *fakeSurface) Data() []byte                 { return s.data }
func (s *fakeSurface) Stride() int                  { return s.stride }
func (s *fakeSurface) Swap() error                  { return nil }
func (s *fakeSurface) Close() error                 { return nil }
func (s *fakeSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return s.resizeC
}
func (s *fakeSurface) OutputID() uint32 { return 0 }
func (s *fakeSurface) DirectRender() bool { return true }
func (s *fakeSurface) DecoMode() uint32   { return 2 }

type fakeInput struct {
	keyCh   chan platform.KeyEvent
	mouseCh chan platform.MouseEvent
}

func newFakeInput() *fakeInput {
	return &fakeInput{
		keyCh:   make(chan platform.KeyEvent, 16),
		mouseCh: make(chan platform.MouseEvent, 16),
	}
}
func (i *fakeInput) KeyEvents() <-chan platform.KeyEvent   { return i.keyCh }
func (i *fakeInput) MouseEvents() <-chan platform.MouseEvent { return i.mouseCh }
func (i *fakeInput) Close() error                          { return nil }

type fakeBackend struct {
	surface *fakeSurface
	input   *fakeInput
	doneCh  chan struct{}
}

func newFakeBackend(w, h int) *fakeBackend {
	return &fakeBackend{
		surface: newFakeSurface(w, h),
		input:   newFakeInput(),
		doneCh:  make(chan struct{}),
	}
}
func (b *fakeBackend) CreateSurface(int, int) (platform.Surface, error) { return b.surface, nil }
func (b *fakeBackend) CreateSurfaceFor(platform.Output) (platform.Surface, error) {
	return b.surface, nil
}
func (b *fakeBackend) ListOutputs() ([]platform.Output, error) {
	return nil, nil
}
func (b *fakeBackend) CreateInputSource() (platform.InputSource, error) { return b.input, nil }
func (b *fakeBackend) Run(func())                                       {}
func (b *fakeBackend) Done() <-chan struct{}                            { return b.doneCh }
func (b *fakeBackend) Stop() error                                      { return nil }
func (b *fakeBackend) Close() error {
	select {
	case <-b.doneCh:
	default:
		close(b.doneCh)
	}
	return nil
}

var _ io.Closer = (*fakeSurface)(nil)

func newTerminalForTest(cols, rows int) (*Terminal, *bytes.Buffer) {
	buf := screen.NewBuffer(cols, rows)
	altBuf := screen.NewBuffer(cols, rows)
	altBuf.SetAltScreen(true)
	resp := &bytes.Buffer{}
	t := &Terminal{
		screen:     buf,
		cursor:     buf.Cursor(),
		parser:     vte.NewParser(),
		mainBuf:    buf,
		altBuf:     altBuf,
		hostWriter: resp,
		done:       make(chan struct{}),
		curFg:      screen.Color{IsDefault: true},
		curBg:      screen.Color{IsDefault: true},
		theme:      &DefaultTheme,
		defFg:      DefaultTheme.DefFg,
		defBg:      DefaultTheme.DefBg,
		cursorColor: DefaultTheme.CursorColor,
		autoWrap:   true,
		charset:    newCharsetState(),
		active:     true,
		cols:       cols,
		rows:       rows,
	}
	t.initTabStops()
	return t, resp
}
