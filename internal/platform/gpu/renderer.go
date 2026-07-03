package gpu

import (
	"fmt"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	glLib "github.com/LaoQi/vistty/internal/platform/gl"
)

// Renderer 是后端无关的 GPU instanced draw 渲染核心。
// 持有 GLES atlas 纹理、quad/instance VBO、shader program 与 shelf packing 状态。
// GBM 与 Wayland 后端通过组合本结构复用同一套 GPU 渲染管线。
type Renderer struct {
	gles    *glLib.GLESLoader
	egl     *glLib.EGLLoader
	display uintptr
	surface uintptr
	context uintptr
	width   int
	height  int

	gpuReady    bool
	frameCount  uint64
	gpuProgram  uint32
	atlasTex    uint32
	atlasW      int
	atlasH      int
	quadVBO     uint32
	instanceVBO uint32
	vao         uint32
	vaoReady    bool
	resUni      int32
	defBgUni    int32
	atlasUni    int32
	atlasCache  map[glyphKey]atlasEntry
	shelfX      int
	shelfY      int
	shelfH      int
	rgbaBuf     []byte
}

// NewRenderer 创建渲染核心。width/height 用于预分配 instance VBO 上限。
// 调用 Init 后才可用。
func NewRenderer(gles *glLib.GLESLoader, egl *glLib.EGLLoader, display, surface, context uintptr, width, height int) *Renderer {
	return &Renderer{
		gles:    gles,
		egl:     egl,
		display: display,
		surface: surface,
		context: context,
		width:   width,
		height:  height,
	}
}

// Ready 返回 GPU 管线是否已成功初始化。
func (c *Renderer) Ready() bool { return c.gpuReady }

// Init 编译 shader、创建 atlas 纹理与 VBO。失败时返回错误，调用方应回退 CPU 路径。
func (c *Renderer) Init() error {
	gl := c.gles

	vs := gl.CreateShader(glLib.GL_VERTEX_SHADER)
	if vs == 0 {
		return fmt.Errorf("glCreateShader(vertex) failed")
	}
	gl.ShaderSource(vs, gpuVertexSrc)
	gl.CompileShader(vs)
	var status [1]int32
	gl.GetShaderiv(vs, glLib.GL_COMPILE_STATUS, status[:])
	if status[0] == 0 {
		log := gl.GetShaderInfoLog(vs, 512)
		gl.DeleteShader(vs)
		return fmt.Errorf("GPU vertex shader compile: %s", log)
	}

	fs := gl.CreateShader(glLib.GL_FRAGMENT_SHADER)
	if fs == 0 {
		gl.DeleteShader(vs)
		return fmt.Errorf("glCreateShader(fragment) failed")
	}
	gl.ShaderSource(fs, gpuFragmentSrc)
	gl.CompileShader(fs)
	gl.GetShaderiv(fs, glLib.GL_COMPILE_STATUS, status[:])
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
	gl.GetProgramiv(prog, glLib.GL_LINK_STATUS, status[:])
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	if status[0] == 0 {
		log := gl.GetProgramInfoLog(prog, 512)
		gl.DeleteProgram(prog)
		return fmt.Errorf("GPU program link: %s", log)
	}
	c.gpuProgram = prog
	c.resUni = gl.GetUniformLocation(prog, "u_resolution")
	c.defBgUni = gl.GetUniformLocation(prog, "u_defBg")
	c.atlasUni = gl.GetUniformLocation(prog, "u_atlas")

	c.atlasW = 2048
	c.atlasH = 2048
	var texs [1]uint32
	gl.GenTextures(1, texs[:])
	c.atlasTex = texs[0]
	gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
	gl.TexParameteri(glLib.GL_TEXTURE_2D, glLib.GL_TEXTURE_MIN_FILTER, glLib.GL_NEAREST)
	gl.TexParameteri(glLib.GL_TEXTURE_2D, glLib.GL_TEXTURE_MAG_FILTER, glLib.GL_NEAREST)
	gl.TexParameteri(glLib.GL_TEXTURE_2D, glLib.GL_TEXTURE_WRAP_S, glLib.GL_CLAMP_TO_EDGE)
	gl.TexParameteri(glLib.GL_TEXTURE_2D, glLib.GL_TEXTURE_WRAP_T, glLib.GL_CLAMP_TO_EDGE)
	gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
	gl.GetError()
	gl.TexImage2D(glLib.GL_TEXTURE_2D, 0, glLib.GL_RGBA, int32(c.atlasW), int32(c.atlasH), 0, glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, nil)
	texImgErr := gl.GetError()
	gl.TexParameteri(glLib.GL_TEXTURE_2D, glLib.GL_TEXTURE_MAX_LEVEL, 0)

	if texImgErr != 0 {
		gl.DeleteTextures(1, []uint32{c.atlasTex})
		return fmt.Errorf("TexImage2D failed: glErr=0x%x", texImgErr)
	}

	c.atlasCache = make(map[glyphKey]atlasEntry)

	var vbos [2]uint32
	gl.GenBuffers(2, vbos[:])
	c.quadVBO = vbos[0]
	c.instanceVBO = vbos[1]

	quadVerts := []float32{
		0, 0, 0, 0,
		1, 0, 1, 0,
		0, 1, 0, 1,
		1, 1, 1, 1,
	}
	gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.quadVBO)
	gl.BufferData(glLib.GL_ARRAY_BUFFER, float32ToBytes(quadVerts), glLib.GL_STATIC_DRAW)

	maxInstances := 65536
	if maxInstances < 1 {
		maxInstances = 1
	}
	gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.instanceVBO)
	gl.BufferDataEmpty(glLib.GL_ARRAY_BUFFER, maxInstances*int(unsafe.Sizeof(platform.CellInstance{})), glLib.GL_DYNAMIC_DRAW)

	if gl.HasVAO() {
		var vaos [1]uint32
		gl.GenVertexArrays(1, vaos[:])
		c.vao = vaos[0]
		gl.BindVertexArray(c.vao)

		gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.quadVBO)
		gl.VertexAttribPointer(0, 2, glLib.GL_FLOAT, false, 16, 0)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(1, 2, glLib.GL_FLOAT, false, 16, 8)
		gl.EnableVertexAttribArray(1)

		gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.instanceVBO)
		stride := int32(unsafe.Sizeof(platform.CellInstance{}))
		gl.VertexAttribPointer(2, 2, glLib.GL_FLOAT, false, stride, 0)
		gl.EnableVertexAttribArray(2)
		gl.VertexAttribDivisor(2, 1)
		gl.VertexAttribPointer(3, 2, glLib.GL_FLOAT, false, stride, 8)
		gl.EnableVertexAttribArray(3)
		gl.VertexAttribDivisor(3, 1)
		gl.VertexAttribPointer(4, 2, glLib.GL_FLOAT, false, stride, 16)
		gl.EnableVertexAttribArray(4)
		gl.VertexAttribDivisor(4, 1)
		gl.VertexAttribPointer(5, 2, glLib.GL_FLOAT, false, stride, 24)
		gl.EnableVertexAttribArray(5)
		gl.VertexAttribDivisor(5, 1)
		gl.VertexAttribPointer(6, 4, glLib.GL_FLOAT, false, stride, 32)
		gl.EnableVertexAttribArray(6)
		gl.VertexAttribDivisor(6, 1)
		gl.VertexAttribPointer(7, 3, glLib.GL_FLOAT, false, stride, 48)
		gl.EnableVertexAttribArray(7)
		gl.VertexAttribDivisor(7, 1)
		gl.VertexAttribPointer(8, 3, glLib.GL_FLOAT, false, stride, 60)
		gl.EnableVertexAttribArray(8)
		gl.VertexAttribDivisor(8, 1)
		gl.VertexAttribPointer(9, 1, glLib.GL_FLOAT, false, stride, 72)
		gl.EnableVertexAttribArray(9)
		gl.VertexAttribDivisor(9, 1)
		gl.VertexAttribPointer(10, 1, glLib.GL_FLOAT, false, stride, 76)
		gl.EnableVertexAttribArray(10)
		gl.VertexAttribDivisor(10, 1)
		gl.VertexAttribPointer(11, 1, glLib.GL_FLOAT, false, stride, 80)
		gl.EnableVertexAttribArray(11)
		gl.VertexAttribDivisor(11, 1)

		gl.BindVertexArray(0)
		c.vaoReady = true
	}

	c.gpuReady = true
	major, minor := gl.GetGLVersion()
	debug.Debugf("GPU: instanced draw ready (GLES %d.%d, atlas %dx%d)\n", major, minor, c.atlasW, c.atlasH)
	return nil
}

// BeginFrame implements platform.GPURenderer
func (c *Renderer) BeginFrame() error {
	if !c.gpuReady {
		return nil
	}
	if err := c.egl.MakeCurrent(c.display, c.surface, c.surface, c.context); err != nil {
		return fmt.Errorf("eglMakeCurrent: %w", err)
	}
	return nil
}

// ResetAtlas implements platform.GPURenderer
func (c *Renderer) ResetAtlas() {
	if !c.gpuReady {
		return
	}
	if err := c.egl.MakeCurrent(c.display, c.surface, c.surface, c.context); err != nil {
		return
	}
	c.shelfX = 0
	c.shelfY = 0
	c.shelfH = 0
	for k := range c.atlasCache {
		delete(c.atlasCache, k)
	}
	gl := c.gles
	gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
	gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(glLib.GL_TEXTURE_2D, 0, glLib.GL_RGBA, int32(c.atlasW), int32(c.atlasH), 0, glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, nil)
}

// UploadGlyph implements platform.GPURenderer
func (c *Renderer) UploadGlyph(r rune, italic bool, bitmap []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
	if !c.gpuReady {
		return 0, 0, 0, 0, false
	}
	if w <= 0 || h <= 0 || len(bitmap) < w*h {
		return 0, 0, 0, 0, false
	}
	key := glyphKey{Rune: r, Italic: italic}
	if e, exists := c.atlasCache[key]; exists {
		return e.u0, e.v0, e.u1, e.v1, true
	}

	placeX, placeY, nextShelfX, nextShelfY, nextShelfH, gu0, gv0, gu1, gv1, reset, pok := packGlyph(c.shelfX, c.shelfY, c.shelfH, c.atlasW, c.atlasH, w, h)
	if !pok {
		return 0, 0, 0, 0, false
	}

	if reset {
		c.shelfX = 0
		c.shelfY = 0
		c.shelfH = 0
		for k := range c.atlasCache {
			delete(c.atlasCache, k)
		}
		gl := c.gles
		gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
		gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
		gl.TexImage2D(glLib.GL_TEXTURE_2D, 0, glLib.GL_RGBA, int32(c.atlasW), int32(c.atlasH), 0, glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, nil)
	}

	need := w * h * 4
	if cap(c.rgbaBuf) < need {
		c.rgbaBuf = make([]byte, need)
	} else {
		c.rgbaBuf = c.rgbaBuf[:need]
	}
	for i := 0; i < w*h; i++ {
		c.rgbaBuf[i*4] = bitmap[i]
		c.rgbaBuf[i*4+3] = 255
	}

	gl := c.gles
	gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
	gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
	gl.GetError()
	gl.TexSubImage2D(glLib.GL_TEXTURE_2D, 0, int32(placeX), int32(placeY), int32(w), int32(h), glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, c.rgbaBuf)
	subErr := gl.GetError()

	if debug.Enabled() && c.frameCount <= 3 {
		maxA := 0
		for _, b := range bitmap {
			if int(b) > maxA {
				maxA = int(b)
			}
		}
		debug.Debugf("UploadGlyph: rune=%q atlasTex=%d place=%d,%d w=%d h=%d maxAlpha=%d rgbaLen=%d glErr=0x%x\n",
			r, c.atlasTex, placeX, placeY, w, h, maxA, len(c.rgbaBuf), subErr)
	}

	if subErr != glLib.GL_NO_ERROR {
		return 0, 0, 0, 0, false
	}

	c.atlasCache[key] = atlasEntry{gu0, gv0, gu1, gv1}
	c.shelfX = nextShelfX
	c.shelfY = nextShelfY
	c.shelfH = nextShelfH
	return gu0, gv0, gu1, gv1, true
}

// UploadColorGlyph implements platform.GPURenderer
func (c *Renderer) UploadColorGlyph(r rune, rgba []byte, w, h int) (u0, v0, u1, v1 float32, ok bool) {
	if !c.gpuReady {
		return 0, 0, 0, 0, false
	}
	if w <= 0 || h <= 0 || len(rgba) < w*h*4 {
		return 0, 0, 0, 0, false
	}
	key := glyphKey{Rune: r, Italic: false, IsColor: true}
	if e, exists := c.atlasCache[key]; exists {
		return e.u0, e.v0, e.u1, e.v1, true
	}

	placeX, placeY, nextShelfX, nextShelfY, nextShelfH, gu0, gv0, gu1, gv1, reset, pok := packGlyph(c.shelfX, c.shelfY, c.shelfH, c.atlasW, c.atlasH, w, h)
	if !pok {
		return 0, 0, 0, 0, false
	}

	if reset {
		c.shelfX = 0
		c.shelfY = 0
		c.shelfH = 0
		for k := range c.atlasCache {
			delete(c.atlasCache, k)
		}
		gl := c.gles
		gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
		gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
		gl.TexImage2D(glLib.GL_TEXTURE_2D, 0, glLib.GL_RGBA, int32(c.atlasW), int32(c.atlasH), 0, glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, nil)
	}

	gl := c.gles
	gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)
	gl.PixelStorei(glLib.GL_UNPACK_ALIGNMENT, 1)
	gl.GetError()
	gl.TexSubImage2D(glLib.GL_TEXTURE_2D, 0, int32(placeX), int32(placeY), int32(w), int32(h), glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, rgba)
	subErr := gl.GetError()

	if subErr != glLib.GL_NO_ERROR {
		return 0, 0, 0, 0, false
	}

	c.atlasCache[key] = atlasEntry{gu0, gv0, gu1, gv1}
	c.shelfX = nextShelfX
	c.shelfY = nextShelfY
	c.shelfH = nextShelfH
	return gu0, gv0, gu1, gv1, true
}

// DrawInstances implements platform.GPURenderer
func (c *Renderer) DrawInstances(instances []platform.CellInstance, screenW, screenH int, bgColor [3]float32) error {
	if !c.gpuReady {
		return nil
	}

	gl := c.gles
	gl.Viewport(0, 0, int32(screenW), int32(screenH))
	gl.ClearColor(bgColor[0], bgColor[1], bgColor[2], 1)
	gl.Clear(glLib.GL_COLOR_BUFFER_BIT)

	if len(instances) == 0 {
		return nil
	}

	const maxInstances = 65536
	if len(instances) > maxInstances {
		instances = instances[:maxInstances]
	}

	instanceBytes := (*[1 << 28]byte)(unsafe.Pointer(&instances[0]))[:len(instances)*int(unsafe.Sizeof(platform.CellInstance{}))]
	gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.instanceVBO)
	gl.BufferSubData(glLib.GL_ARRAY_BUFFER, 0, instanceBytes)

	gl.UseProgram(c.gpuProgram)
	gl.Uniform2f(c.resUni, float32(screenW), float32(screenH))
	if c.defBgUni >= 0 {
		gl.Uniform3fv(c.defBgUni, 1, bgColor[:])
	}
	if c.atlasUni >= 0 {
		gl.Uniform1i(c.atlasUni, 0)
	}

	gl.ActiveTexture(glLib.GL_TEXTURE0)
	gl.BindTexture(glLib.GL_TEXTURE_2D, c.atlasTex)

	if c.vaoReady {
		gl.BindVertexArray(c.vao)
	} else {
		gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.quadVBO)
		gl.VertexAttribPointer(0, 2, glLib.GL_FLOAT, false, 16, 0)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointer(1, 2, glLib.GL_FLOAT, false, 16, 8)
		gl.EnableVertexAttribArray(1)

		gl.BindBuffer(glLib.GL_ARRAY_BUFFER, c.instanceVBO)
		stride := int32(unsafe.Sizeof(platform.CellInstance{}))

		gl.VertexAttribPointer(2, 2, glLib.GL_FLOAT, false, stride, 0)
		gl.EnableVertexAttribArray(2)
		gl.VertexAttribDivisor(2, 1)

		gl.VertexAttribPointer(3, 2, glLib.GL_FLOAT, false, stride, 8)
		gl.EnableVertexAttribArray(3)
		gl.VertexAttribDivisor(3, 1)

		gl.VertexAttribPointer(4, 2, glLib.GL_FLOAT, false, stride, 16)
		gl.EnableVertexAttribArray(4)
		gl.VertexAttribDivisor(4, 1)

		gl.VertexAttribPointer(5, 2, glLib.GL_FLOAT, false, stride, 24)
		gl.EnableVertexAttribArray(5)
		gl.VertexAttribDivisor(5, 1)

		gl.VertexAttribPointer(6, 4, glLib.GL_FLOAT, false, stride, 32)
		gl.EnableVertexAttribArray(6)
		gl.VertexAttribDivisor(6, 1)

		gl.VertexAttribPointer(7, 3, glLib.GL_FLOAT, false, stride, 48)
		gl.EnableVertexAttribArray(7)
		gl.VertexAttribDivisor(7, 1)

		gl.VertexAttribPointer(8, 3, glLib.GL_FLOAT, false, stride, 60)
		gl.EnableVertexAttribArray(8)
		gl.VertexAttribDivisor(8, 1)

		gl.VertexAttribPointer(9, 1, glLib.GL_FLOAT, false, stride, 72)
		gl.EnableVertexAttribArray(9)
		gl.VertexAttribDivisor(9, 1)

		gl.VertexAttribPointer(10, 1, glLib.GL_FLOAT, false, stride, 76)
		gl.EnableVertexAttribArray(10)
		gl.VertexAttribDivisor(10, 1)

		gl.VertexAttribPointer(11, 1, glLib.GL_FLOAT, false, stride, 80)
		gl.EnableVertexAttribArray(11)
		gl.VertexAttribDivisor(11, 1)
	}

	gl.DrawArraysInstanced(glLib.GL_TRIANGLE_STRIP, 0, 4, int32(len(instances)))
	drawErr := gl.GetError()

	if c.vaoReady {
		gl.BindVertexArray(0)
	} else {
		for i := uint32(2); i <= 11; i++ {
			gl.VertexAttribDivisor(i, 0)
		}
	}

	c.frameCount++

	if debug.Enabled() && c.frameCount <= 3 && len(instances) > 0 {
		inst := instances[0]
		px := make([]byte, 4)
		gl.ReadPixels(int32(screenW/2), int32(screenH/2), 1, 1, glLib.GL_RGBA, glLib.GL_UNSIGNED_BYTE, px)
		debug.Debugf("DrawInstances: count=%d atlasUni=%d atlasTex=%d glErr=0x%x inst[0]: X=%v Y=%v CW=%v CH=%v OffX=%v OffY=%v GW=%v GH=%v UV=(%v,%v,%v,%v) fg=(%v,%v,%v) hasBg=%v centerPx=(%d,%d,%d,%d)\n",
			len(instances), c.atlasUni, c.atlasTex, drawErr,
			inst.X, inst.Y, inst.CellW, inst.CellH, inst.GlyphOffX, inst.GlyphOffY,
			inst.GlyphW, inst.GlyphH, inst.GlyphU0, inst.V0, inst.GlyphU1, inst.V1,
			inst.FgR, inst.FgG, inst.FgB, inst.HasBg, px[0], px[1], px[2], px[3])
	}
	return nil
}

// Close 释放 GPU 资源（program/atlas/VBO）。调用方需保证 EGL context 已 current。
func (c *Renderer) Close() {
	if !c.gpuReady {
		return
	}
	gl := c.gles
	if c.gpuProgram != 0 {
		gl.DeleteProgram(c.gpuProgram)
		c.gpuProgram = 0
	}
	if c.atlasTex != 0 {
		texs := [1]uint32{c.atlasTex}
		gl.DeleteTextures(1, texs[:])
		c.atlasTex = 0
	}
	if c.quadVBO != 0 || c.instanceVBO != 0 {
		vbos := [2]uint32{c.quadVBO, c.instanceVBO}
		gl.DeleteBuffers(2, vbos[:])
		c.quadVBO = 0
		c.instanceVBO = 0
	}
	if c.vao != 0 {
		vaos := [1]uint32{c.vao}
		gl.DeleteVertexArrays(1, vaos[:])
		c.vao = 0
	}
	c.gpuReady = false
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

var _ platform.GPURenderer = (*Renderer)(nil)
