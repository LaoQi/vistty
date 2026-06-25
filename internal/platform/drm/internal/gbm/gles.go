package gbm

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	GL_TEXTURE_2D                   = 0x0DE1
	GL_TEXTURE_WRAP_S               = 0x2802
	GL_TEXTURE_WRAP_T               = 0x2803
	GL_TEXTURE_MIN_FILTER           = 0x2801
	GL_TEXTURE_MAG_FILTER           = 0x2800
	GL_NEAREST                      = 0x2600
	GL_LINEAR                       = 0x2601
	GL_CLAMP_TO_EDGE                = 0x812F
	GL_RGBA                         = 0x1908
	GL_BGRA_EXT                     = 0x80E1
	GL_UNSIGNED_BYTE                = 0x1401
	GL_FLOAT                       = 0x1406
	GL_RGB                          = 0x1907
	GL_STATIC_DRAW                  = 0x88E4
	GL_ARRAY_BUFFER                 = 0x8892
	GL_VERTEX_SHADER                = 0x8B31
	GL_FRAGMENT_SHADER              = 0x8B30
	GL_COMPILE_STATUS               = 0x8B81
	GL_LINK_STATUS                  = 0x8B82
	GL_TEXTURE0                     = 0x84C0
	GL_NO_ERROR                     = 0
	GL_EXTENSIONS                   = 0x1F03
	GL_RENDERER                     = 0x1F01
	GL_VERSION                      = 0x1F02
	GL_NUM_EXTENSIONS               = 0x821D
	GL_COLOR_BUFFER_BIT             = 0x4000
	GL_TEXTURE_WIDTH                = 0x1000
	GL_TEXTURE_HEIGHT               = 0x1001
	GL_UNPACK_ALIGNMENT             = 0x0CF5

	GL_TRIANGLES                    = 0x0004
	GL_TRIANGLE_STRIP               = 0x0005

	GL_EXT_texture_format_BGRA8888 = "GL_EXT_texture_format_BGRA8888"
)

type GLESLoader struct {
	lib                 uintptr
	genTextures         func(n int32, textures unsafe.Pointer)
	deleteTextures      func(n int32, textures unsafe.Pointer)
	bindTexture         func(target uint32, texture uint32)
	activeTexture      func(unit uint32)
	texImage2D         func(target uint32, level int32, internalFormat int32, w, h int32, border int32, format, typ uint32, data unsafe.Pointer)
	texSubImage2D      func(target uint32, level int32, x, y int32, w, h int32, format, typ uint32, data unsafe.Pointer)
	texParameteri      func(target uint32, pname uint32, param int32)
	getTexLevelParameteriv func(target uint32, level int32, pname uint32, params unsafe.Pointer)
	createShader        func(shaderType uint32) uint32
	deleteShader        func(shader uint32)
	shaderSource        func(shader uint32, count int32, srcs unsafe.Pointer, lengths unsafe.Pointer)
	compileShader       func(shader uint32)
	getShaderiv         func(shader uint32, pname uint32, params unsafe.Pointer)
	getShaderInfoLog    func(shader uint32, maxLength int32, length unsafe.Pointer, infoLog unsafe.Pointer)
	createProgram       func() uint32
	deleteProgram       func(program uint32)
	attachShader        func(program, shader uint32)
	linkProgram         func(program uint32)
	getProgramiv        func(program uint32, pname uint32, params unsafe.Pointer)
	getProgramInfoLog   func(program uint32, maxLength int32, length unsafe.Pointer, infoLog unsafe.Pointer)
	useProgram          func(program uint32)
	getUniformLocation  func(program uint32, name unsafe.Pointer) int32
	genBuffers          func(n int32, buffers unsafe.Pointer)
	deleteBuffers       func(n int32, buffers unsafe.Pointer)
	bindBuffer          func(target uint32, buffer uint32)
	bufferData          func(target uint32, size uintptr, data unsafe.Pointer, usage uint32)
	vertexAttribPointer func(index uint32, size int32, typ uint32, normalized bool, stride int32, pointer unsafe.Pointer)
	enableVertexAttribArray func(index uint32)
	drawArrays          func(mode uint32, first int32, count int32)
	viewport            func(x, y int32, w, h int32)
	clear               func(mask uint32)
	clearColor          func(r, g, b, a float32)
	uniform1i           func(location int32, v0 int32)
	getString           func(name uint32) unsafe.Pointer
	getIntegerv         func(name uint32, params unsafe.Pointer)
	getError           func() uint32
	pixelStorei         func(pname uint32, param int32)
}

func LoadGLES() (*GLESLoader, error) {
	lib, err := purego.Dlopen("libGLESv2.so.2", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		lib, err = purego.Dlopen("libGLESv2.so", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("load libGLESv2: %w", err)
		}
	}

	l := &GLESLoader{lib: lib}

	type symDef struct {
		name string
		fptr any
	}
	syms := []symDef{
		{"glGenTextures", &l.genTextures},
		{"glDeleteTextures", &l.deleteTextures},
		{"glBindTexture", &l.bindTexture},
		{"glActiveTexture", &l.activeTexture},
		{"glTexImage2D", &l.texImage2D},
		{"glTexSubImage2D", &l.texSubImage2D},
		{"glTexParameteri", &l.texParameteri},
		{"glGetTexLevelParameteriv", &l.getTexLevelParameteriv},
		{"glCreateShader", &l.createShader},
		{"glDeleteShader", &l.deleteShader},
		{"glShaderSource", &l.shaderSource},
		{"glCompileShader", &l.compileShader},
		{"glGetShaderiv", &l.getShaderiv},
		{"glGetShaderInfoLog", &l.getShaderInfoLog},
		{"glCreateProgram", &l.createProgram},
		{"glDeleteProgram", &l.deleteProgram},
		{"glAttachShader", &l.attachShader},
		{"glLinkProgram", &l.linkProgram},
		{"glGetProgramiv", &l.getProgramiv},
		{"glGetProgramInfoLog", &l.getProgramInfoLog},
		{"glUseProgram", &l.useProgram},
		{"glGetUniformLocation", &l.getUniformLocation},
		{"glGenBuffers", &l.genBuffers},
		{"glDeleteBuffers", &l.deleteBuffers},
		{"glBindBuffer", &l.bindBuffer},
		{"glBufferData", &l.bufferData},
		{"glVertexAttribPointer", &l.vertexAttribPointer},
		{"glEnableVertexAttribArray", &l.enableVertexAttribArray},
		{"glDrawArrays", &l.drawArrays},
		{"glViewport", &l.viewport},
		{"glClear", &l.clear},
		{"glClearColor", &l.clearColor},
		{"glUniform1i", &l.uniform1i},
		{"glGetString", &l.getString},
		{"glGetIntegerv", &l.getIntegerv},
		{"glGetError", &l.getError},
		{"glPixelStorei", &l.pixelStorei},
	}

	var errs []error
	for _, s := range syms {
		addr, err := purego.Dlsym(lib, s.name)
		if err != nil {
			errs = append(errs, fmt.Errorf("missing GLES symbol %s: %w", s.name, err))
			continue
		}
		purego.RegisterFunc(s.fptr, addr)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("GLES symbol resolution: %w", errors.Join(errs...))
	}

	return l, nil
}

func (l *GLESLoader) GenTextures(n int32, textures []uint32) {
	l.genTextures(n, unsafe.Pointer(&textures[0]))
}

func (l *GLESLoader) DeleteTextures(n int32, textures []uint32) {
	l.deleteTextures(n, unsafe.Pointer(&textures[0]))
}

func (l *GLESLoader) BindTexture(target, texture uint32) {
	l.bindTexture(target, texture)
}

func (l *GLESLoader) ActiveTexture(unit uint32) {
	l.activeTexture(unit)
}

func (l *GLESLoader) TexImage2D(target uint32, level, internalFormat int32, w, h, border int32, format, typ uint32, data []byte) {
	var ptr unsafe.Pointer
	if data != nil {
		ptr = unsafe.Pointer(&data[0])
	}
	l.texImage2D(target, level, internalFormat, w, h, border, format, typ, ptr)
}

func (l *GLESLoader) TexSubImage2D(target uint32, level, x, y, w, h int32, format, typ uint32, data []byte) {
	var ptr unsafe.Pointer
	if data != nil {
		ptr = unsafe.Pointer(&data[0])
	}
	l.texSubImage2D(target, level, x, y, w, h, format, typ, ptr)
}

func (l *GLESLoader) TexParameteri(target, pname uint32, param int32) {
	l.texParameteri(target, pname, param)
}

func (l *GLESLoader) PixelStorei(pname uint32, param int32) {
	l.pixelStorei(pname, param)
}

func (l *GLESLoader) CreateShader(shaderType uint32) uint32 {
	return l.createShader(shaderType)
}

func (l *GLESLoader) DeleteShader(shader uint32) {
	l.deleteShader(shader)
}

func (l *GLESLoader) ShaderSource(shader uint32, source string) error {
	src := []byte(source)
	srcs := [1]unsafe.Pointer{unsafe.Pointer(&src[0])}
	lengths := [1]int32{int32(len(src))}
	l.shaderSource(shader, 1, unsafe.Pointer(&srcs[0]), unsafe.Pointer(&lengths[0]))
	return nil
}

func (l *GLESLoader) CompileShader(shader uint32) {
	l.compileShader(shader)
}

func (l *GLESLoader) GetShaderiv(shader, pname uint32, params []int32) {
	l.getShaderiv(shader, pname, unsafe.Pointer(&params[0]))
}

func (l *GLESLoader) GetShaderInfoLog(shader uint32, maxLen int32) string {
	buf := make([]byte, maxLen)
	var length int32
	l.getShaderInfoLog(shader, maxLen, unsafe.Pointer(&length), unsafe.Pointer(&buf[0]))
	if length < 0 {
		length = 0
	}
	if length > int32(len(buf)) {
		length = int32(len(buf))
	}
	return string(buf[:length])
}

func (l *GLESLoader) CreateProgram() uint32 {
	return l.createProgram()
}

func (l *GLESLoader) DeleteProgram(program uint32) {
	l.deleteProgram(program)
}

func (l *GLESLoader) AttachShader(program, shader uint32) {
	l.attachShader(program, shader)
}

func (l *GLESLoader) LinkProgram(program uint32) {
	l.linkProgram(program)
}

func (l *GLESLoader) GetProgramiv(program, pname uint32, params []int32) {
	l.getProgramiv(program, pname, unsafe.Pointer(&params[0]))
}

func (l *GLESLoader) GetProgramInfoLog(program uint32, maxLen int32) string {
	buf := make([]byte, maxLen)
	var length int32
	l.getProgramInfoLog(program, maxLen, unsafe.Pointer(&length), unsafe.Pointer(&buf[0]))
	if length < 0 {
		length = 0
	}
	if length > int32(len(buf)) {
		length = int32(len(buf))
	}
	return string(buf[:length])
}

func (l *GLESLoader) UseProgram(program uint32) {
	l.useProgram(program)
}

func (l *GLESLoader) GetUniformLocation(program uint32, name string) int32 {
	cname := []byte(name + "\x00")
	return l.getUniformLocation(program, unsafe.Pointer(&cname[0]))
}

func (l *GLESLoader) GenBuffers(n int32, buffers []uint32) {
	l.genBuffers(n, unsafe.Pointer(&buffers[0]))
}

func (l *GLESLoader) DeleteBuffers(n int32, buffers []uint32) {
	l.deleteBuffers(n, unsafe.Pointer(&buffers[0]))
}

func (l *GLESLoader) BindBuffer(target, buffer uint32) {
	l.bindBuffer(target, buffer)
}

func (l *GLESLoader) BufferData(target uint32, data []byte, usage uint32) {
	var ptr unsafe.Pointer
	if data != nil {
		ptr = unsafe.Pointer(&data[0])
	}
	l.bufferData(target, uintptr(len(data)), ptr, usage)
}

func (l *GLESLoader) VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr) {
	l.vertexAttribPointer(index, size, typ, normalized, stride, unsafe.Add(unsafe.Pointer(nil), offset))
}

func (l *GLESLoader) EnableVertexAttribArray(index uint32) {
	l.enableVertexAttribArray(index)
}

func (l *GLESLoader) DrawArrays(mode uint32, first, count int32) {
	l.drawArrays(mode, first, count)
}

func (l *GLESLoader) Viewport(x, y, w, h int32) {
	l.viewport(x, y, w, h)
}

func (l *GLESLoader) Clear(mask uint32) {
	l.clear(mask)
}

func (l *GLESLoader) ClearColor(r, g, b, a float32) {
	l.clearColor(r, g, b, a)
}

func (l *GLESLoader) Uniform1i(location int32, v0 int32) {
	l.uniform1i(location, v0)
}

func (l *GLESLoader) GetString(name uint32) string {
	ptr := l.getString(name)
	if ptr == nil {
		return ""
	}
	return cGoString(ptr)
}

func (l *GLESLoader) GetExtensions() string {
	return l.GetString(GL_EXTENSIONS)
}

func (l *GLESLoader) GetError() uint32 {
	return l.getError()
}

func (l *GLESLoader) HasBGRA() bool {
	extensions := l.GetExtensions()
	for len(extensions) > 0 {
		idx := -1
		for i, c := range extensions {
			if c == ' ' {
				idx = i
				break
			}
		}
		var ext string
		if idx >= 0 {
			ext = extensions[:idx]
			extensions = extensions[idx+1:]
		} else {
			ext = extensions
			extensions = ""
		}
		if ext == GL_EXT_texture_format_BGRA8888 {
			return true
		}
	}
	return false
}

func cGoString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	var bs []byte
	for i := uintptr(0); ; i++ {
		b := *(*byte)(unsafe.Pointer(uintptr(ptr) + i))
		if b == 0 {
			break
		}
		bs = append(bs, b)
	}
	return string(bs)
}
