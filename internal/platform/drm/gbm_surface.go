package drm

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/LaoQi/vistty/internal/platform"
	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
	"github.com/LaoQi/vistty/internal/platform/drm/internal/gbm"
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

	mu          sync.Mutex
	active      bool
	flipCh      chan struct{}

	currentBO    uintptr
	currentFB    uint32
	currentStride uint32
	closed       bool
	frameCount   uint64

	cpuBuf    []byte
	cpuStride int

	glInitDone bool
	texture   uint32
	program   uint32
	vbo       uint32
	texUni    int32
	hasBGRA   bool
}

func (s *GBMSurface) Size() (int, int) {
	return s.width, s.height
}

func (s *GBMSurface) Data() []byte {
	return s.cpuBuf
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
	gl := s.device.glesLoader

	s.hasBGRA = gl.HasBGRA()

	var fragSrc string
	if s.hasBGRA {
		fragSrc = fragmentShaderSrc
	} else {
		fragSrc = fragmentShaderSrcSwap
	}

	vs := gl.CreateShader(gbm.GL_VERTEX_SHADER)
	if vs == 0 {
		return fmt.Errorf("glCreateShader(vertex) failed (glErr=0x%x)", gl.GetError())
	}
	gl.ShaderSource(vs, vertexShaderSrc)
	gl.CompileShader(vs)
	var status [1]int32
	gl.GetShaderiv(vs, gbm.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetShaderInfoLog(vs, 512)
		gl.DeleteShader(vs)
		return fmt.Errorf("vertex shader compile error: %s", log)
	}

	fs := gl.CreateShader(gbm.GL_FRAGMENT_SHADER)
	if fs == 0 {
		gl.DeleteShader(vs)
		return fmt.Errorf("glCreateShader(fragment) failed (glErr=0x%x)", gl.GetError())
	}
	gl.ShaderSource(fs, fragSrc)
	gl.CompileShader(fs)
	gl.GetShaderiv(fs, gbm.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetShaderInfoLog(fs, 512)
		gl.DeleteShader(vs)
		gl.DeleteShader(fs)
		return fmt.Errorf("fragment shader compile error: %s", log)
	}

	prog := gl.CreateProgram()
	if prog == 0 {
		gl.DeleteShader(vs)
		gl.DeleteShader(fs)
		return fmt.Errorf("glCreateProgram failed (glErr=0x%x)", gl.GetError())
	}
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)
	gl.GetProgramiv(prog, gbm.GL_LINK_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetProgramInfoLog(prog, 512)
		gl.DeleteShader(vs)
		gl.DeleteShader(fs)
		gl.DeleteProgram(prog)
		return fmt.Errorf("program link error: %s", log)
	}
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	s.program = prog
	s.texUni = gl.GetUniformLocation(prog, "u_tex")

	var texs [1]uint32
	gl.GenTextures(1, texs[:])
	s.texture = texs[0]
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.texture)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_MIN_FILTER, gbm.GL_LINEAR)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_MAG_FILTER, gbm.GL_LINEAR)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_WRAP_S, gbm.GL_CLAMP_TO_EDGE)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_WRAP_T, gbm.GL_CLAMP_TO_EDGE)

	uploadFmt := uint32(gbm.GL_RGBA)
	if s.hasBGRA {
		uploadFmt = gbm.GL_BGRA_EXT
	}
	gl.TexImage2D(gbm.GL_TEXTURE_2D, 0, gbm.GL_RGBA, int32(s.width), int32(s.height), 0, uploadFmt, gbm.GL_UNSIGNED_BYTE, s.cpuBuf)

	var vbos [1]uint32
	gl.GenBuffers(1, vbos[:])
	s.vbo = vbos[0]

	verts := []float32{
		-1, 1, 0, 0,
		1, 1, 1, 0,
		-1, -1, 0, 1,
		1, -1, 1, 1,
	}
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.vbo)
	gl.BufferData(gbm.GL_ARRAY_BUFFER, float32ToBytes(verts), gbm.GL_STATIC_DRAW)

	gl.PixelStorei(gbm.GL_UNPACK_ALIGNMENT, 1)

	s.glInitDone = true
	return nil
}

func (s *GBMSurface) drawTexturedQuad() {
	gl := s.device.glesLoader

	uploadFmt := uint32(gbm.GL_RGBA)
	if s.hasBGRA {
		uploadFmt = gbm.GL_BGRA_EXT
	}
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.texture)
	gl.TexSubImage2D(gbm.GL_TEXTURE_2D, 0, 0, 0, int32(s.width), int32(s.height), uploadFmt, gbm.GL_UNSIGNED_BYTE, s.cpuBuf)

	gl.Viewport(0, 0, int32(s.width), int32(s.height))
	gl.ClearColor(0, 0, 0, 1)
	gl.Clear(gbm.GL_COLOR_BUFFER_BIT)

	gl.UseProgram(s.program)
	gl.ActiveTexture(gbm.GL_TEXTURE0)
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.texture)
	gl.Uniform1i(s.texUni, 0)

	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.vbo)
	gl.VertexAttribPointer(0, 2, gbm.GL_FLOAT, false, 16, 0)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(1, 2, gbm.GL_FLOAT, false, 16, 8)
	gl.EnableVertexAttribArray(1)

	gl.DrawArrays(gbm.GL_TRIANGLE_STRIP, 0, 4)
}

func (s *GBMSurface) Swap() error {
	s.mu.Lock()
	if !s.active || s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.ensureCPUBuf()

	s.frameCount++
	debugLog := os.Getenv("VISTTY_DEBUG") != ""
	if debugLog && (s.frameCount <= 3 || s.frameCount%100 == 0) {
		fmt.Fprintf(os.Stderr, "GBM Swap: crtc=%d frame=%d\n", s.crtcID, s.frameCount)
	}

	if err := s.device.eglLoader.MakeCurrent(s.device.eglDisplay, s.eglSurface, s.eglSurface, s.device.eglContext); err != nil {
		errCode := s.device.eglLoader.GetError()
		return fmt.Errorf("eglMakeCurrent: %w (eglErr=%s)", err, gbm.EGLErrorString(errCode))
	}

	if !s.glInitDone {
		if err := s.initGL(); err != nil {
			return fmt.Errorf("initGL: %w", err)
		}
		if debugLog {
			fmt.Fprintf(os.Stderr, "GBM Swap: crtc=%d GL initialized (hasBGRA=%v)\n", s.crtcID, s.hasBGRA)
		}
	}

	s.drawTexturedQuad()

	if err := s.device.eglLoader.SwapBuffers(s.device.eglDisplay, s.eglSurface); err != nil {
		errCode := s.device.eglLoader.GetError()
		return fmt.Errorf("eglSwapBuffers: %w (eglErr=%s)", err, gbm.EGLErrorString(errCode))
	}

	bo := s.device.gbmLoader.SurfaceLockFrontBuffer(s.gbmSurface)
	if bo == 0 {
		return fmt.Errorf("gbm_surface_lock_front_buffer returned NULL")
	}

	handle := s.device.gbmLoader.BOGetHandle(bo)
	stride := s.device.gbmLoader.BOGetStride(bo)

	fbID, err := drminternal.AddFB(
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
	if debugLog && (s.frameCount <= 3 || s.frameCount%100 == 0) {
		fmt.Fprintf(os.Stderr, "GBM Swap: crtc=%d frame=%d bo=0x%x handle=%d stride=%d fbID=%d modeset=%v\n",
			s.crtcID, s.frameCount, bo, handle, stride, fbID, modeset)
	}

	if err := s.commitor.CommitSingle(s.info, fbID, modeset); err != nil {
		drminternal.RmFB(s.device.fd, fbID)
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
		return fmt.Errorf("atomic commit: %w", err)
	}

	flipReceived := true
	select {
	case <-s.flipCh:
	case <-time.After(200 * time.Millisecond):
		flipReceived = false
	}

	if debugLog && (s.frameCount <= 3 || s.frameCount%100 == 0) {
		fmt.Fprintf(os.Stderr, "GBM Swap: crtc=%d frame=%d flipReceived=%v\n",
			s.crtcID, s.frameCount, flipReceived)
	}

	oldBO := s.currentBO
	oldFB := s.currentFB

	s.mu.Lock()
	s.currentBO = bo
	s.currentFB = fbID
	s.currentStride = stride
	s.mu.Unlock()

	if oldBO != 0 {
		if oldFB != 0 {
			drminternal.RmFB(s.device.fd, oldFB)
		}
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, oldBO)
	}

	return nil
}

func (s *GBMSurface) notifyFlip() {
	select {
	case s.flipCh <- struct{}{}:
	default:
	}
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

	if s.currentBO != 0 {
		if s.currentFB != 0 {
			drminternal.RmFB(s.device.fd, s.currentFB)
			s.currentFB = 0
		}
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, s.currentBO)
		s.currentBO = 0
	}

	if s.glInitDone {
		gl := s.device.glesLoader
		if s.texture != 0 {
			texs := [1]uint32{s.texture}
			gl.DeleteTextures(1, texs[:])
			s.texture = 0
		}
		if s.vbo != 0 {
			vbos := [1]uint32{s.vbo}
			gl.DeleteBuffers(1, vbos[:])
			s.vbo = 0
		}
		if s.program != 0 {
			gl.DeleteProgram(s.program)
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

var _ platform.Surface = (*GBMSurface)(nil)
