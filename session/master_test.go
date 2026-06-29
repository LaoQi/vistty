package session

import (
	"testing"
	"time"

	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/ui"
	"github.com/LaoQi/vistty/terminal"
)

type fakeOutput struct {
	id   uint32
	name string
	w, h int
}

func (o *fakeOutput) ID() uint32          { return o.id }
func (o *fakeOutput) ConnectorID() uint32 { return o.id }
func (o *fakeOutput) CrtcID() uint32      { return o.id }
func (o *fakeOutput) Name() string        { return o.name }
func (o *fakeOutput) Size() (int, int)    { return o.w, o.h }

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
func (s *fakeSurface) Size() (int, int) { return s.w, s.h }
func (s *fakeSurface) Data() []byte     { return s.data }
func (s *fakeSurface) Stride() int      { return s.stride }
func (s *fakeSurface) Swap() error      { return nil }
func (s *fakeSurface) Close() error     { return nil }
func (s *fakeSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return s.resizeC
}
func (s *fakeSurface) OutputID() uint32   { return 0 }
func (s *fakeSurface) DirectRender() bool { return true }

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
func (i *fakeInput) KeyEvents() <-chan platform.KeyEvent     { return i.keyCh }
func (i *fakeInput) MouseEvents() <-chan platform.MouseEvent { return i.mouseCh }
func (i *fakeInput) Close() error                            { return nil }

type fakeBackend struct {
	surface *fakeSurface
	input   *fakeInput
	doneCh  chan struct{}
	out     platform.Output
}

func newFakeBackend(w, h int) *fakeBackend {
	return &fakeBackend{
		surface: newFakeSurface(w, h),
		input:   newFakeInput(),
		doneCh:  make(chan struct{}),
		out:     &fakeOutput{id: 1, name: "fake", w: w, h: h},
	}
}
func (b *fakeBackend) CreateSurface(int, int) (platform.Surface, error) { return b.surface, nil }
func (b *fakeBackend) CreateSurfaceFor(platform.Output) (platform.Surface, error) {
	return b.surface, nil
}
func (b *fakeBackend) ListOutputs() ([]platform.Output, error) {
	return []platform.Output{b.out}, nil
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

func TestMasterCloseIdempotent(t *testing.T) {
	b := newFakeBackend(400, 300)
	opts := terminal.DefaultOptions()
	opts.Shell = "/bin/cat"
	opts.FontPath = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
	opts.FontSize = 14
	keybinds := defaultTestKeybinds(platform.ModSuper)
	m, err := NewMaster(b, opts, ui.Config{}, keybinds)
	if err != nil {
		t.Skipf("skip: cannot create master: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = m.Run()
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 5; i++ {
		if err := m.Close(); err != nil {
			t.Errorf("Close attempt %d failed: %v", i, err)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Close")
	}
}

func TestMasterPtyExit(t *testing.T) {
	b := newFakeBackend(400, 300)
	opts := terminal.DefaultOptions()
	opts.Shell = "/bin/true"
	opts.FontPath = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
	opts.FontSize = 14
	keybinds := defaultTestKeybinds(platform.ModSuper)
	m, err := NewMaster(b, opts, ui.Config{}, keybinds)
	if err != nil {
		t.Skipf("skip: cannot create master: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = m.Run()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after pty exit")
	}
}

func TestMasterInputNoDeadlock(t *testing.T) {
	b := newFakeBackend(400, 300)
	opts := terminal.DefaultOptions()
	opts.Shell = "/bin/cat"
	opts.FontPath = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
	opts.FontSize = 14
	keybinds := defaultTestKeybinds(platform.ModSuper)
	m, err := NewMaster(b, opts, ui.Config{}, keybinds)
	if err != nil {
		t.Skipf("skip: cannot create master: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = m.Run()
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 100; i++ {
		b.input.keyCh <- platform.KeyEvent{Rune: 'a', State: platform.KeyPress}
	}

	time.Sleep(100 * time.Millisecond)
	_ = m.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Close")
	}
}
