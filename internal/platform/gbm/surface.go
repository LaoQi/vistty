package gbm

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/drm"
	"github.com/LaoQi/vistty/internal/platform/gl"
	"github.com/LaoQi/vistty/internal/platform/gpu"
)

const vertexShaderSrc = `
attribute vec2 a_pos;
attribute vec2 a_tex;
varying vec2 v_tex;
void main() {
    gl_Position = vec4(a_pos, 0.0, 1.0);
    v_tex = a_tex;
}
`

const fragmentShaderSrc = `
precision mediump float;
varying vec2 v_tex;
uniform sampler2D u_tex;
void main() {
    vec4 c = texture2D(u_tex, v_tex);
    gl_FragColor = c;
}
`

const fragmentShaderSrcSwap = `
precision mediump float;
varying vec2 v_tex;
uniform sampler2D u_tex;
void main() {
    vec4 c = texture2D(u_tex, v_tex);
    gl_FragColor = c.bgra;
}
`

type pendingFrame struct {
	bo     uintptr
	fbID   uint32
	stride uint32
}

type GBMSurface struct {
	device      *GBMDevice
	commitor    *AtomicCommitor
	info        *surfaceAtomicInfo
	gbmSurface  uintptr
	eglSurface  uintptr
	width       int
	height      int
	crtcID      uint32
	connectorID uint32

	mu     sync.Mutex
	active bool
	closed bool

	commitMu    sync.Mutex
	mailbox     *pendingFrame
	committed   *pendingFrame
	releaseBO   *pendingFrame
	flipPending bool
	commitErr   chan error

	frameCount uint64

	cpuBuf    []byte
	cpuStride int

	glInitDone bool
	texture    uint32
	program    uint32
	vbo        uint32
	texUni     int32
	hasBGRA    bool

	gpu      *gpu.Renderer
	gpuDrawn bool
}

func (s *GBMSurface) Size() (int, int) {
	return s.width, s.height
}

func (s *GBMSurface) Data() []byte {
	return s.cpuBuf
}

func (s *GBMSurface) DirectRender() bool {
	return true
}

func (s *GBMSurface) Stride() int {
	return s.cpuStride
}

func (s *GBMSurface) ensureCPUBuf() {
	if s.cpuBuf == nil {
		s.cpuStride = s.width * 4
		s.cpuBuf = make([]byte, s.cpuStride*s.height)
	}
}

func (s *GBMSurface) initGL() error {
	gles := s.device.glesLoader

	s.hasBGRA = gles.HasBGRA()

	var fragSrc string
	if s.hasBGRA {
		fragSrc = fragmentShaderSrc
	} else {
		fragSrc = fragmentShaderSrcSwap
	}

	vs := gles.CreateShader(gl.GL_VERTEX_SHADER)
	if vs == 0 {
		return fmt.Errorf("glCreateShader(vertex) failed (glErr=0x%x)", gles.GetError())
	}
	gles.ShaderSource(vs, vertexShaderSrc)
	gles.CompileShader(vs)
	var status [1]int32
	gles.GetShaderiv(vs, gl.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gles.GetShaderInfoLog(vs, 512)
		gles.DeleteShader(vs)
		return fmt.Errorf("vertex shader compile error: %s", log)
	}

	fs := gles.CreateShader(gl.GL_FRAGMENT_SHADER)
	if fs == 0 {
		gles.DeleteShader(vs)
		return fmt.Errorf("glCreateShader(fragment) failed (glErr=0x%x)", gles.GetError())
	}
	gles.ShaderSource(fs, fragSrc)
	gles.CompileShader(fs)
	gles.GetShaderiv(fs, gl.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gles.GetShaderInfoLog(fs, 512)
		gles.DeleteShader(vs)
		gles.DeleteShader(fs)
		return fmt.Errorf("fragment shader compile error: %s", log)
	}

	prog := gles.CreateProgram()
	if prog == 0 {
		gles.DeleteShader(vs)
		gles.DeleteShader(fs)
		return fmt.Errorf("glCreateProgram failed (glErr=0x%x)", gles.GetError())
	}
	gles.AttachShader(prog, vs)
	gles.AttachShader(prog, fs)
	gles.LinkProgram(prog)
	gles.GetProgramiv(prog, gl.GL_LINK_STATUS, status[:])
	if status[0] == 0 {
		log := gles.GetProgramInfoLog(prog, 512)
		gles.DeleteShader(vs)
		gles.DeleteShader(fs)
		gles.DeleteProgram(prog)
		return fmt.Errorf("program link error: %s", log)
	}
	gles.DeleteShader(vs)
	gles.DeleteShader(fs)
	s.program = prog
	s.texUni = gles.GetUniformLocation(prog, "u_tex")

	var texs [1]uint32
	gles.GenTextures(1, texs[:])
	s.texture = texs[0]
	gles.BindTexture(gl.GL_TEXTURE_2D, s.texture)
	gles.TexParameteri(gl.GL_TEXTURE_2D, gl.GL_TEXTURE_MIN_FILTER, gl.GL_LINEAR)
	gles.TexParameteri(gl.GL_TEXTURE_2D, gl.GL_TEXTURE_MAG_FILTER, gl.GL_LINEAR)
	gles.TexParameteri(gl.GL_TEXTURE_2D, gl.GL_TEXTURE_WRAP_S, gl.GL_CLAMP_TO_EDGE)
	gles.TexParameteri(gl.GL_TEXTURE_2D, gl.GL_TEXTURE_WRAP_T, gl.GL_CLAMP_TO_EDGE)

	uploadFmt := uint32(gl.GL_RGBA)
	if s.hasBGRA {
		uploadFmt = gl.GL_BGRA_EXT
	}
	gles.TexImage2D(gl.GL_TEXTURE_2D, 0, gl.GL_RGBA, int32(s.width), int32(s.height), 0, uploadFmt, gl.GL_UNSIGNED_BYTE, s.cpuBuf)

	var vbos [1]uint32
	gles.GenBuffers(1, vbos[:])
	s.vbo = vbos[0]

	verts := []float32{
		-1, 1, 0, 0,
		1, 1, 1, 0,
		-1, -1, 0, 1,
		1, -1, 1, 1,
	}
	gles.BindBuffer(gl.GL_ARRAY_BUFFER, s.vbo)
	gles.BufferData(gl.GL_ARRAY_BUFFER, float32ToBytes(verts), gl.GL_STATIC_DRAW)

	gles.PixelStorei(gl.GL_UNPACK_ALIGNMENT, 1)

	s.glInitDone = true

	if gles.HasInstancedDraw() {
		s.gpu = gpu.NewRenderer(s.device.glesLoader, s.device.eglLoader, s.device.eglDisplay, s.eglSurface, s.device.eglContext, s.width, s.height)
		if err := s.gpu.Init(); err != nil {
			debug.Warningf("GBM: GPU instanced draw init failed: %v, fallback to CPU\n", err)
			s.gpu = nil
		}
	}

	return nil
}

func (s *GBMSurface) UploadGlyph(r rune, bitmap []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
	if s.gpu == nil {
		return 0, 0, 0, 0, false
	}
	return s.gpu.UploadGlyph(r, bitmap, w, h)
}

func (s *GBMSurface) DrawInstances(instances []platform.CellInstance, screenW, screenH int, bgColor [3]float32) error {
	if s.gpu == nil {
		return nil
	}
	if err := s.gpu.DrawInstances(instances, screenW, screenH, bgColor); err != nil {
		return err
	}
	s.gpuDrawn = true
	return nil
}

func (s *GBMSurface) BeginFrame() error {
	if s.gpu == nil {
		return nil
	}
	return s.gpu.BeginFrame()
}

func (s *GBMSurface) ResetAtlas() {
	if s.gpu == nil {
		return
	}
	s.gpu.ResetAtlas()
}

func (s *GBMSurface) drawTexturedQuad() {
	gles := s.device.glesLoader

	uploadFmt := uint32(gl.GL_RGBA)
	if s.hasBGRA {
		uploadFmt = gl.GL_BGRA_EXT
	}
	gles.BindTexture(gl.GL_TEXTURE_2D, s.texture)
	gles.TexSubImage2D(gl.GL_TEXTURE_2D, 0, 0, 0, int32(s.width), int32(s.height), uploadFmt, gl.GL_UNSIGNED_BYTE, s.cpuBuf)

	gles.Viewport(0, 0, int32(s.width), int32(s.height))
	gles.ClearColor(0, 0, 0, 1)
	gles.Clear(gl.GL_COLOR_BUFFER_BIT)

	gles.UseProgram(s.program)
	gles.ActiveTexture(gl.GL_TEXTURE0)
	gles.BindTexture(gl.GL_TEXTURE_2D, s.texture)
	gles.Uniform1i(s.texUni, 0)

	gles.BindBuffer(gl.GL_ARRAY_BUFFER, s.vbo)
	gles.VertexAttribPointer(0, 2, gl.GL_FLOAT, false, 16, 0)
	gles.EnableVertexAttribArray(0)
	gles.VertexAttribPointer(1, 2, gl.GL_FLOAT, false, 16, 8)
	gles.EnableVertexAttribArray(1)

	gles.DrawArrays(gl.GL_TRIANGLE_STRIP, 0, 4)
}

func (s *GBMSurface) Swap() error {
	select {
	case err := <-s.commitErr:
		return err
	default:
	}

	s.mu.Lock()
	if !s.active || s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.ensureCPUBuf()
	s.frameCount++
	if s.frameCount <= 3 || s.frameCount%100 == 0 {
		debug.Debugf("GBM Swap: crtc=%d frame=%d\n", s.crtcID, s.frameCount)
	}

	if err := s.device.eglLoader.MakeCurrent(s.device.eglDisplay, s.eglSurface, s.eglSurface, s.device.eglContext); err != nil {
		errCode := s.device.eglLoader.GetError()
		return fmt.Errorf("eglMakeCurrent: %w (eglErr=%s)", err, gl.EGLErrorString(errCode))
	}

	if !s.glInitDone {
		if err := s.initGL(); err != nil {
			return fmt.Errorf("initGL: %w", err)
		}
		debug.Debugf("GBM Swap: crtc=%d GL initialized (hasBGRA=%v)\n", s.crtcID, s.hasBGRA)
	}

	if !s.gpuDrawn {
		s.drawTexturedQuad()
	}
	s.gpuDrawn = false

	if err := s.device.eglLoader.SwapBuffers(s.device.eglDisplay, s.eglSurface); err != nil {
		errCode := s.device.eglLoader.GetError()
		return fmt.Errorf("eglSwapBuffers: %w (eglErr=%s)", err, gl.EGLErrorString(errCode))
	}

	bo := s.device.gbmLoader.SurfaceLockFrontBuffer(s.gbmSurface)
	if bo == 0 {
		return fmt.Errorf("gbm_surface_lock_front_buffer returned NULL")
	}

	handle := s.device.gbmLoader.BOGetHandle(bo)
	stride := s.device.gbmLoader.BOGetStride(bo)

	fbID, err := drm.AddFB(
		s.device.fd,
		uint16(s.width), uint16(s.height),
		24, 32,
		stride, handle,
	)
	if err != nil {
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
		return fmt.Errorf("drmModeAddFB: %w", err)
	}

	modeset := !s.info.modesetDone
	if s.frameCount <= 3 || s.frameCount%100 == 0 {
		debug.Debugf("GBM Swap: crtc=%d frame=%d bo=0x%x handle=%d stride=%d fbID=%d modeset=%v\n",
			s.crtcID, s.frameCount, bo, handle, stride, fbID, modeset)
	}

	frame := &pendingFrame{bo, fbID, stride}

	s.commitMu.Lock()
	if s.flipPending {
		if old := s.mailbox; old != nil {
			drm.RmFB(s.device.fd, old.fbID)
			s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, old.bo)
		}
		s.mailbox = frame
		s.commitMu.Unlock()
	} else {
		s.commitMu.Unlock()
		if err := s.commitor.CommitSingle(s.info, fbID, modeset); err != nil {
			drm.RmFB(s.device.fd, fbID)
			s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
			return fmt.Errorf("atomic commit: %w", err)
		}
		s.commitMu.Lock()
		s.committed = frame
		s.flipPending = true
		s.commitMu.Unlock()
	}

	return nil
}

func (s *GBMSurface) onFlipComplete() {
	s.commitMu.Lock()
	if s.closed {
		s.commitMu.Unlock()
		return
	}

	if s.releaseBO != nil {
		drm.RmFB(s.device.fd, s.releaseBO.fbID)
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, s.releaseBO.bo)
		s.releaseBO = nil
	}

	s.releaseBO = s.committed
	s.committed = nil
	s.flipPending = false

	if s.mailbox != nil {
		frame := s.mailbox
		s.mailbox = nil
		modeset := !s.info.modesetDone
		if err := s.commitor.CommitSingle(s.info, frame.fbID, modeset); err != nil {
			drm.RmFB(s.device.fd, frame.fbID)
			s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, frame.bo)
			select {
			case s.commitErr <- fmt.Errorf("atomic commit: %w", err):
			default:
			}
		} else {
			s.committed = frame
			s.flipPending = true
		}
	}

	s.commitMu.Unlock()
}

func (s *GBMSurface) SetActive(active bool) {
	s.mu.Lock()
	s.active = active
	s.mu.Unlock()
}

func (s *GBMSurface) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.active = false
	s.mu.Unlock()

	s.commitMu.Lock()
	for _, frame := range []*pendingFrame{s.mailbox, s.committed, s.releaseBO} {
		if frame != nil {
			if frame.fbID != 0 {
				drm.RmFB(s.device.fd, frame.fbID)
			}
			s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, frame.bo)
		}
	}
	s.mailbox = nil
	s.committed = nil
	s.releaseBO = nil
	s.flipPending = false
	s.commitMu.Unlock()

	_ = s.device.eglLoader.MakeCurrent(s.device.eglDisplay, s.eglSurface, s.eglSurface, s.device.eglContext)
	if s.gpu != nil {
		s.gpu.Close()
		s.gpu = nil
	}
	if s.glInitDone {
		gles := s.device.glesLoader
		if s.texture != 0 {
			texs := [1]uint32{s.texture}
			gles.DeleteTextures(1, texs[:])
			s.texture = 0
		}
		if s.vbo != 0 {
			vbos := [1]uint32{s.vbo}
			gles.DeleteBuffers(1, vbos[:])
			s.vbo = 0
		}
		if s.program != 0 {
			gles.DeleteProgram(s.program)
			s.program = 0
		}
		s.glInitDone = false
	}

	if s.eglSurface != 0 {
		s.device.eglLoader.DestroySurface(s.device.eglDisplay, s.eglSurface)
		s.eglSurface = 0
	}
	if s.gbmSurface != 0 {
		s.device.gbmLoader.SurfaceDestroy(s.gbmSurface)
		s.gbmSurface = 0
	}

	return nil
}

func (s *GBMSurface) ResizeEvents() <-chan platform.ResizeEvent {
	return nil
}

func (s *GBMSurface) OutputID() uint32 {
	return s.connectorID
}

func (s *GBMSurface) CrtcID() uint32 {
	return s.crtcID
}

func float32ToBytes(vals []float32) []byte {
	ret := make([]byte, len(vals)*4)
	for i, v := range vals {
		ui := *(*uint32)(unsafe.Pointer(&v))
		idx := i * 4
		ret[idx] = byte(ui)
		ret[idx+1] = byte(ui >> 8)
		ret[idx+2] = byte(ui >> 16)
		ret[idx+3] = byte(ui >> 24)
	}
	return ret
}

var (
	_ platform.Surface     = (*GBMSurface)(nil)
	_ platform.GPURenderer = (*GBMSurface)(nil)
)
