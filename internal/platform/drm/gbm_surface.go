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

// GPU instanced draw shaders (GLES 3.00)
const gpuVertexSrc = `#version 300 es
layout(location=0) in vec2 a_quadPos;   // 0..1 unit quad
layout(location=1) in vec2 a_quadTex;   // 0..1 unit texcoord
layout(location=2) in vec2 i_cellPos;   // pixel position
layout(location=3) in vec4 i_glyphUV;   // atlas UV (u0,v0,u1,v1)
layout(location=4) in vec2 i_glyphSize; // glyph pixel size
layout(location=5) in vec3 i_fg;        // fg color
layout(location=6) in vec3 i_bg;        // bg color
layout(location=7) in float i_hasBg;
layout(location=8) in float i_attrFlags;
uniform vec2 u_resolution;
out vec2 v_tex;
out vec2 v_cellUV;
out vec3 v_fg;
out vec3 v_bg;
out float v_hasBg;
out float v_attrFlags;
void main() {
    vec2 pixelPos = a_quadPos * i_glyphSize + i_cellPos;
    // italic (bit 2): x 方向 skew
    float hasItalic = mod(floor(i_attrFlags / 4.0), 2.0);
    if (hasItalic > 0.5) {
        pixelPos.x += (a_quadPos.y - 0.5) * i_glyphSize.x * 0.25;
    }
    vec2 ndc = pixelPos / u_resolution * 2.0 - 1.0;
    ndc.y = -ndc.y;
    gl_Position = vec4(ndc, 0.0, 1.0);
    v_tex = mix(i_glyphUV.xy, i_glyphUV.zw, a_quadTex);
    v_cellUV = a_quadTex;
    v_fg = i_fg;
    v_bg = i_bg;
    v_hasBg = i_hasBg;
    v_attrFlags = i_attrFlags;
}
`

const gpuFragmentSrc = `#version 300 es
precision mediump float;
in vec2 v_tex;
in vec2 v_cellUV;
in vec3 v_fg;
in vec3 v_bg;
in float v_hasBg;
in float v_attrFlags;
uniform sampler2D u_atlas;
out vec4 fragColor;
void main() {
    float alpha = texture(u_atlas, v_tex).r;
    vec3 color = mix(v_bg * v_hasBg, v_fg, alpha);
    // underline (bit 0): cell 底部 ~90%
    float hasUL = mod(floor(v_attrFlags), 2.0);
    if (hasUL > 0.5 && v_cellUV.y > 0.85) {
        color = v_fg;
        alpha = 1.0;
    }
    // crossed out (bit 1): cell 中部 ~50%
    float hasCO = floor(v_attrFlags / 2.0);
    if (hasCO > 0.5 && v_cellUV.y > 0.45 && v_cellUV.y < 0.55) {
        color = v_fg;
        alpha = 1.0;
    }
    float a = max(alpha, v_hasBg);
    fragColor = vec4(color, a);
}
`

type atlasEntry struct {
	u0, v0, u1, v1 float32
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

	// GPU instanced draw
	gpuReady     bool
	gpuDrawn     bool
	gpuProgram   uint32
	atlasTex     uint32
	atlasW       int
	atlasH       int
	quadVBO      uint32
	instanceVBO  uint32
	resUni       int32
	atlasCache   map[rune]atlasEntry
	shelfX       int
	shelfY       int
	shelfH       int
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

	// 尝试初始化 GPU instanced draw
	if gl.HasInstancedDraw() {
		if err := s.initGPU(); err != nil {
			if os.Getenv("VISTTY_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "GBM: GPU instanced draw init failed: %v, fallback to CPU\n", err)
			}
		}
	}

	return nil
}

func (s *GBMSurface) initGPU() error {
	gl := s.device.glesLoader

	// 编译 GPU shader
	vs := gl.CreateShader(gbm.GL_VERTEX_SHADER)
	if vs == 0 {
		return fmt.Errorf("glCreateShader(vertex) failed")
	}
	gl.ShaderSource(vs, gpuVertexSrc)
	gl.CompileShader(vs)
	var status [1]int32
	gl.GetShaderiv(vs, gbm.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetShaderInfoLog(vs, 512)
		gl.DeleteShader(vs)
		return fmt.Errorf("GPU vertex shader compile: %s", log)
	}

	fs := gl.CreateShader(gbm.GL_FRAGMENT_SHADER)
	if fs == 0 {
		gl.DeleteShader(vs)
		return fmt.Errorf("glCreateShader(fragment) failed")
	}
	gl.ShaderSource(fs, gpuFragmentSrc)
	gl.CompileShader(fs)
	gl.GetShaderiv(fs, gbm.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetShaderInfoLog(fs, 512)
		gl.DeleteShader(vs)
		gl.DeleteShader(fs)
		return fmt.Errorf("GPU fragment shader compile: %s", log)
	}

	prog := gl.CreateProgram()
	gl.AttachShader(prog, vs)
	gl.AttachShader(prog, fs)
	gl.LinkProgram(prog)
	gl.GetProgramiv(prog, gbm.GL_LINK_STATUS, status[:])
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	if status[0] == 0 {
		log := gl.GetProgramInfoLog(prog, 512)
		gl.DeleteProgram(prog)
		return fmt.Errorf("GPU program link: %s", log)
	}
	s.gpuProgram = prog
	s.resUni = gl.GetUniformLocation(prog, "u_resolution")

	// 创建 atlas 纹理（R8 格式，2048x2048，可存 ~10000 字形）
	s.atlasW = 2048
	s.atlasH = 2048
	var texs [1]uint32
	gl.GenTextures(1, texs[:])
	s.atlasTex = texs[0]
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.atlasTex)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_MIN_FILTER, gbm.GL_LINEAR)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_MAG_FILTER, gbm.GL_LINEAR)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_WRAP_S, gbm.GL_CLAMP_TO_EDGE)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_WRAP_T, gbm.GL_CLAMP_TO_EDGE)
	gl.TexImage2D(gbm.GL_TEXTURE_2D, 0, gbm.GL_R8, int32(s.atlasW), int32(s.atlasH), 0, gbm.GL_RED, gbm.GL_UNSIGNED_BYTE, nil)
	gl.TexParameteri(gbm.GL_TEXTURE_2D, gbm.GL_TEXTURE_MAX_LEVEL, 0)

	s.atlasCache = make(map[rune]atlasEntry)

	// 创建单位 quad VBO（4 顶点：pos.xy + tex.xy）
	var vbos [2]uint32
	gl.GenBuffers(2, vbos[:])
	s.quadVBO = vbos[0]
	s.instanceVBO = vbos[1]

	quadVerts := []float32{
		0, 0, 0, 0,
		1, 0, 1, 0,
		0, 1, 0, 1,
		1, 1, 1, 1,
	}
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.quadVBO)
	gl.BufferData(gbm.GL_ARRAY_BUFFER, float32ToBytes(quadVerts), gbm.GL_STATIC_DRAW)

	// instance VBO（预分配，后续 BufferSubData 更新）
	maxInstances := s.width * s.height // 上限
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.instanceVBO)
	gl.BufferData(gbm.GL_ARRAY_BUFFER, make([]byte, maxInstances*int(unsafe.Sizeof(platform.CellInstance{}))), gbm.GL_DYNAMIC_DRAW)

	s.gpuReady = true
	if os.Getenv("VISTTY_DEBUG") != "" {
		major, minor := gl.GetGLVersion()
		fmt.Fprintf(os.Stderr, "GBM: GPU instanced draw ready (GLES %d.%d, atlas %dx%d R8)\n", major, minor, s.atlasW, s.atlasH)
	}
	return nil
}

// UploadGlyph implements platform.GPURenderer
func (s *GBMSurface) UploadGlyph(r rune, bitmap []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
	if !s.gpuReady {
		return 0, 0, 0, 0, false
	}
	if e, exists := s.atlasCache[r]; exists {
		return e.u0, e.v0, e.u1, e.v1, true
	}

	// Shelf packing
	if s.shelfX+w > s.atlasW {
		s.shelfX = 0
		s.shelfY += s.shelfH
		s.shelfH = 0
	}
	if s.shelfY+h > s.atlasH {
		// atlas 满：重置整个 atlas，下帧重新上传可见字形
		s.shelfX = 0
		s.shelfY = 0
		s.shelfH = 0
		for k := range s.atlasCache {
			delete(s.atlasCache, k)
		}
		gl := s.device.glesLoader
		gl.BindTexture(gbm.GL_TEXTURE_2D, s.atlasTex)
		gl.TexImage2D(gbm.GL_TEXTURE_2D, 0, gbm.GL_R8, int32(s.atlasW), int32(s.atlasH), 0, gbm.GL_RED, gbm.GL_UNSIGNED_BYTE, nil)
		// 重新放置当前字形
		if w > s.atlasW || h > s.atlasH {
			return 0, 0, 0, 0, false
		}
	}

	gl := s.device.glesLoader
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.atlasTex)
	gl.TexSubImage2D(gbm.GL_TEXTURE_2D, 0, int32(s.shelfX), int32(s.shelfY), int32(w), int32(h), gbm.GL_RED, gbm.GL_UNSIGNED_BYTE, bitmap)

	u0 = float32(s.shelfX) / float32(s.atlasW)
	v0 = float32(s.shelfY) / float32(s.atlasH)
	u1 = float32(s.shelfX+w) / float32(s.atlasW)
	v1 = float32(s.shelfY+h) / float32(s.atlasH)

	s.atlasCache[r] = atlasEntry{u0, v0, u1, v1}

	s.shelfX += w
	if h > s.shelfH {
		s.shelfH = h
	}
	return u0, v0, u1, v1, true
}

// DrawInstances implements platform.GPURenderer
func (s *GBMSurface) DrawInstances(instances []platform.CellInstance, screenW, screenH int, bgColor [3]float32) error {
	if !s.gpuReady || len(instances) == 0 {
		return nil
	}

	if err := s.device.eglLoader.MakeCurrent(s.device.eglDisplay, s.eglSurface, s.eglSurface, s.device.eglContext); err != nil {
		return fmt.Errorf("eglMakeCurrent: %w", err)
	}

	gl := s.device.glesLoader

	// 上传 instance data
	instanceBytes := (*[1 << 28]byte)(unsafe.Pointer(&instances[0]))[:len(instances)*int(unsafe.Sizeof(platform.CellInstance{}))]
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.instanceVBO)
	gl.BufferSubData(gbm.GL_ARRAY_BUFFER, 0, instanceBytes)

	gl.Viewport(0, 0, int32(screenW), int32(screenH))
	gl.ClearColor(bgColor[0], bgColor[1], bgColor[2], 1)
	gl.Clear(gbm.GL_COLOR_BUFFER_BIT)

	gl.UseProgram(s.gpuProgram)
	gl.Uniform2f(s.resUni, float32(screenW), float32(screenH))

	gl.ActiveTexture(gbm.GL_TEXTURE0)
	gl.BindTexture(gbm.GL_TEXTURE_2D, s.atlasTex)

	// 绑定 quad VBO (attributes 0,1)
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.quadVBO)
	gl.VertexAttribPointer(0, 2, gbm.GL_FLOAT, false, 16, 0)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(1, 2, gbm.GL_FLOAT, false, 16, 8)
	gl.EnableVertexAttribArray(1)

	// 绑定 instance VBO (attributes 2-7)
	gl.BindBuffer(gbm.GL_ARRAY_BUFFER, s.instanceVBO)
	stride := int32(unsafe.Sizeof(platform.CellInstance{}))

	gl.VertexAttribPointer(2, 2, gbm.GL_FLOAT, false, stride, 0)       // i_cellPos
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribDivisor(2, 1)

	gl.VertexAttribPointer(3, 4, gbm.GL_FLOAT, false, stride, 8)       // i_glyphUV
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribDivisor(3, 1)

	gl.VertexAttribPointer(4, 2, gbm.GL_FLOAT, false, stride, 24)      // i_glyphSize
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribDivisor(4, 1)

	gl.VertexAttribPointer(5, 3, gbm.GL_FLOAT, false, stride, 32)      // i_fg
	gl.EnableVertexAttribArray(5)
	gl.VertexAttribDivisor(5, 1)

	gl.VertexAttribPointer(6, 3, gbm.GL_FLOAT, false, stride, 44)      // i_bg
	gl.EnableVertexAttribArray(6)
	gl.VertexAttribDivisor(6, 1)

	gl.VertexAttribPointer(7, 1, gbm.GL_FLOAT, false, stride, 56)      // i_hasBg
	gl.EnableVertexAttribArray(7)
	gl.VertexAttribDivisor(7, 1)

	gl.VertexAttribPointer(8, 1, gbm.GL_FLOAT, false, stride, 64)      // i_attrFlags
	gl.EnableVertexAttribArray(8)
	gl.VertexAttribDivisor(8, 1)

	gl.DrawArraysInstanced(gbm.GL_TRIANGLE_STRIP, 0, 4, int32(len(instances)))

	// 清理 divisor
	for i := uint32(2); i <= 8; i++ {
		gl.VertexAttribDivisor(i, 0)
	}

	s.gpuDrawn = true
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

	if !s.gpuDrawn {
		s.drawTexturedQuad()
	}
	s.gpuDrawn = false

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

	// 等 上一次 page flip 完成后再提交新帧（避免 EBUSY）
	// 首次 modeset 不发 flip 事件，跳过等待
	flipReceived := true
	if s.info.modesetDone {
		select {
		case <-s.flipCh:
		case <-time.After(200 * time.Millisecond):
			flipReceived = false
		}
	}

	if err := s.commitor.CommitSingle(s.info, fbID, modeset); err != nil {
		drminternal.RmFB(s.device.fd, fbID)
		s.device.gbmLoader.SurfaceReleaseBuffer(s.gbmSurface, bo)
		return fmt.Errorf("atomic commit: %w", err)
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
