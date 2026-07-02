//go:build gbm_gl

package gbm

import (
	"os"
	"sync"
	"testing"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/gl"
	"github.com/LaoQi/vistty/internal/platform/gpu"
)

type glEnv struct {
	egl          *gl.EGLLoader
	gles         *gl.GLESLoader
	gbmL         *GBMLoader
	disp         uintptr
	ctx          uintptr
	cfg          uintptr
	gbmDev       uintptr
	nativeVisual uint32
}

var (
	glEnvOnce sync.Once
	glEnvInst *glEnv
	glEnvSkip string
)

func getGLEnv() {
	glEnvOnce.Do(func() {
		egl, err := gl.LoadEGL()
		if err != nil {
			glEnvSkip = "LoadEGL: " + err.Error()
			return
		}
		gbmL, err := LoadGBM()
		if err != nil {
			glEnvSkip = "LoadGBM: " + err.Error()
			return
		}
		gles, err := gl.LoadGLES()
		if err != nil {
			glEnvSkip = "LoadGLES: " + err.Error()
			return
		}
		fd, err := os.OpenFile("/dev/dri/renderD128", os.O_RDWR, 0)
		if err != nil {
			glEnvSkip = "open renderD128: " + err.Error() + "（需 GPU render node 权限）"
			return
		}
		gbmDev := gbmL.CreateDevice(int(fd.Fd()))
		if gbmDev == 0 {
			glEnvSkip = "gbm_create_device failed"
			return
		}
		disp := egl.GetPlatformDisplay(gl.EGL_PLATFORM_GBM_KHR, gbmDev)
		if disp == 0 || disp == gl.EGL_NO_DISPLAY {
			disp = egl.GetDisplay(gbmDev)
		}
		if disp == 0 || disp == gl.EGL_NO_DISPLAY {
			glEnvSkip = "eglGetDisplay(GBM) failed"
			return
		}
		if _, _, err := egl.Initialize(disp); err != nil {
			glEnvSkip = "eglInitialize: " + err.Error()
			return
		}
		if err := egl.BindAPI(gl.EGL_OPENGL_ES_API); err != nil {
			glEnvSkip = "eglBindAPI: " + err.Error()
			return
		}
		cfg, err := egl.ChooseConfig(disp, gl.EGLAttribList(
			gl.EGL_SURFACE_TYPE, gl.EGL_WINDOW_BIT,
			gl.EGL_RED_SIZE, 8, gl.EGL_GREEN_SIZE, 8, gl.EGL_BLUE_SIZE, 8, gl.EGL_ALPHA_SIZE, 8,
			gl.EGL_RENDERABLE_TYPE, gl.EGL_OPENGL_ES2_BIT,
		))
		if err != nil {
			glEnvSkip = "eglChooseConfig: " + err.Error()
			return
		}
		ctx := egl.CreateContext(disp, cfg, gl.EGL_NO_CONTEXT, gl.EGLAttribList(gl.EGL_CONTEXT_CLIENT_VERSION, 3))
		if ctx == 0 {
			ctx = egl.CreateContext(disp, cfg, gl.EGL_NO_CONTEXT, gl.EGLAttribList(gl.EGL_CONTEXT_CLIENT_VERSION, 2))
		}
		if ctx == 0 {
			glEnvSkip = "eglCreateContext: " + gl.EGLErrorString(egl.GetError())
			return
		}
		nv, _ := egl.GetConfigAttrib(disp, cfg, gl.EGL_NATIVE_VISUAL_ID)
		glEnvInst = &glEnv{
			egl: egl, gles: gles, gbmL: gbmL,
			disp: disp, ctx: ctx, cfg: cfg,
			gbmDev: gbmDev, nativeVisual: uint32(nv),
		}
	})
}

func readPixel(gles *gl.GLESLoader, x, y int) (r, g, b, a byte) {
	px := make([]byte, 4)
	gles.ReadPixels(int32(x), int32(y), 1, 1, gl.GL_RGBA, gl.GL_UNSIGNED_BYTE, px)
	return px[0], px[1], px[2], px[3]
}

func glyphCenterY(height, compY int) int { return height - 1 - compY }

func fullAlphaBitmap(w, h int) []byte {
	b := make([]byte, w*h)
	for i := range b {
		b[i] = 255
	}
	return b
}

func TestGBMGL(t *testing.T) {
	getGLEnv()
	if glEnvInst == nil {
		t.Skip(glEnvSkip)
	}
	env := glEnvInst

	gbmFmt := env.nativeVisual
	if gbmFmt == 0 {
		gbmFmt = GBM_FORMAT_XRGB8888
	}
	gbmSurf := env.gbmL.SurfaceCreate(env.gbmDev, 80, 32, gbmFmt, GBM_BO_USE_SCANOUT|GBM_BO_USE_RENDERING)
	if gbmSurf == 0 {
		t.Skip("gbm_surface_create failed")
	}
	eglSurf := env.egl.CreateWindowSurface(env.disp, env.cfg, gbmSurf)
	if eglSurf == 0 || eglSurf == gl.EGL_NO_SURFACE {
		env.gbmL.SurfaceDestroy(gbmSurf)
		t.Skipf("eglCreateWindowSurface: %s", gl.EGLErrorString(env.egl.GetError()))
	}
	if err := env.egl.MakeCurrent(env.disp, eglSurf, eglSurf, env.ctx); err != nil {
		env.egl.DestroySurface(env.disp, eglSurf)
		env.gbmL.SurfaceDestroy(gbmSurf)
		t.Skipf("eglMakeCurrent: %v", err)
	}
	t.Cleanup(func() {
		env.egl.MakeCurrent(env.disp, gl.EGL_NO_SURFACE, gl.EGL_NO_SURFACE, gl.EGL_NO_CONTEXT)
		env.egl.DestroySurface(env.disp, eglSurf)
		env.gbmL.SurfaceDestroy(gbmSurf)
	})

	dev := &GBMDevice{
		gbmLoader:  env.gbmL,
		eglLoader:  env.egl,
		glesLoader: env.gles,
		eglDisplay: env.disp,
		eglConfig:  env.cfg,
	}
	s := &GBMSurface{
		device:     dev,
		eglSurface: eglSurf,
		eglContext: env.ctx,
		width:      80,
		height:     32,
		active:     true,
	}
	s.ensureCPUBuf()

	s.gpu = gpu.NewRenderer(env.gles, env.egl, env.disp, eglSurf, env.ctx, 80, 32)
	if err := s.gpu.Init(); err != nil {
		t.Fatalf("initGPU 失败: %v\nshader 编译/链接失败将导致字形无法显示", err)
	}
	if !s.gpu.Ready() {
		t.Fatal("gpuReady=false（HasInstancedDraw 不支持或 initGPU 失败）")
	}
	if !env.gles.HasInstancedDraw() {
		t.Fatal("GLES 不支持 instanced draw，GPU 路径不可用")
	}

	t.Run("UploadGlyphNoError", func(t *testing.T) {
		bmp := make([]byte, 8*16)
		for i := range bmp {
			bmp[i] = 255
		}
		u0, v0, u1, v1, ok := s.UploadGlyph('A', false, bmp, 8, 16)
		if !ok {
			t.Fatal("UploadGlyph 返回 ok=false（TexSubImage2D glErr≠0 或上传失败）")
		}
		if u0 == 0 && v0 == 0 && u1 == 0 && v1 == 0 {
			t.Error("UV 全零")
		}
		if !(u0 < u1 && v0 < v1) {
			t.Errorf("UV 未有序: u0=%v u1=%v v0=%v v1=%v", u0, u1, v0, v1)
		}
	})

	t.Run("DrawInstancesRendersGlyph", func(t *testing.T) {
		bmp := make([]byte, 8*16)
		for i := range bmp {
			bmp[i] = 255
		}
		u0, v0, u1, v1, ok := s.UploadGlyph('A', false, bmp, 8, 16)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 8, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 0,
			GlyphW: 8, GlyphH: 16,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0,
			HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, g, b, _ := readPixel(env.gles, 4, glyphCenterY(32, 8))
		if r < 200 {
			t.Errorf("字形中心像素 R=%d want >=200（红色前景；字形未绘制或 alpha 采样为0）", r)
		}
		if g > 50 || b > 50 {
			t.Errorf("字形中心 G=%d B=%d want 接近0（应纯红）", g, b)
		}
		r2, _, _, _ := readPixel(env.gles, 40, 16)
		if r2 > 20 {
			t.Errorf("cell 外像素 R=%d want 接近0（Clear 背景）", r2)
		}
	})

	t.Run("DrawInstancesZeroAlpha", func(t *testing.T) {
		bmp := make([]byte, 8*16)
		u0, v0, u1, v1, ok := s.UploadGlyph('B', false, bmp, 8, 16)
		if !ok {
			t.Fatal("UploadGlyph failed for zero-alpha bitmap")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 8, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 0, GlyphW: 8, GlyphH: 16,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0,
			HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 4, glyphCenterY(32, 8))
		if r > 20 {
			t.Errorf("alpha=0 字形中心 R=%d want 接近0（不应显示前景）", r)
		}
	})

	t.Run("DrawInstancesNoUVDegraded", func(t *testing.T) {
		u0 := float32(1000) / 2048
		v0 := float32(1000) / 2048
		u1 := float32(1001) / 2048
		v1 := float32(1001) / 2048
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 8, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 0, GlyphW: 8, GlyphH: 16,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0,
			HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 4, glyphCenterY(32, 8))
		if r > 20 {
			t.Errorf("空白 UV 降级像素 R=%d want 接近0（采样空白 atlas 显示背景）", r)
		}
	})

	t.Run("DrawInstancesRealGlyph", func(t *testing.T) {
		face, err := font.NewEmbeddedFace(14, 72)
		if err != nil {
			t.Skip("embedded font: " + err.Error())
		}
		defer face.Close()
		m := face.Metrics()
		glyph, err := face.Glyph('A')
		if err != nil || glyph == nil {
			t.Skip("no glyph 'A'")
		}
		t.Logf("metrics W=%d H=%d Ascent=%d; glyph %dx%d Off=(%d,%d)",
			m.Width, m.Height, m.Ascent, glyph.Width, glyph.Height, glyph.XOffset, glyph.YOffset)

		u0, v0, u1, v1, ok := s.UploadGlyph('R', false, glyph.Bitmap, glyph.Width, glyph.Height)
		if !ok {
			t.Fatal("UploadGlyph real glyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0,
			CellW:     float32(m.Width),
			CellH:     float32(m.Height),
			GlyphOffX: float32(glyph.XOffset),
			GlyphOffY: float32(m.Ascent + glyph.YOffset),
			GlyphW:    float32(glyph.Width),
			GlyphH:    float32(glyph.Height),
			GlyphU0:   u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0,
			HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		checked := 0
		for y := 0; y < glyph.Height; y++ {
			for x := 0; x < glyph.Width; x++ {
				if glyph.Bitmap[y*glyph.Width+x] <= 128 {
					continue
				}
				cx := int(float32(glyph.XOffset)) + x
				cy := int(float32(m.Ascent+glyph.YOffset)) + y
				if cx < 0 || cy < 0 || cx >= 80 || cy >= 32 {
					continue
				}
				r, _, _, _ := readPixel(env.gles, cx, glyphCenterY(32, cy))
				checked++
				if r < 100 {
					t.Errorf("字形笔画 cell(%d,%d) R=%d want >=100（v_inGlyph 误裁或采样失败）", cx, cy, r)
				}
				if checked >= 8 {
					return
				}
			}
		}
		if checked == 0 {
			t.Skip("字形笔画均落在 cell 外，无法验证 v_inGlyph")
		}
	})

	t.Run("FullAlpha8x16OffY3", func(t *testing.T) {
		u0, v0, u1, v1, ok := s.UploadGlyph('F', false, fullAlphaBitmap(8, 16), 8, 16)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 8, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 3,
			GlyphW: 8, GlyphH: 16,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0, HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 4, glyphCenterY(32, 10))
		if r < 200 {
			t.Errorf("8x16 OffY=3 字形 R=%d want >=200（OffY=3 导致 v_inGlyph 裁剪或采样失败）", r)
		}
	})

	t.Run("FullAlpha7x11OffY0", func(t *testing.T) {
		u0, v0, u1, v1, ok := s.UploadGlyph('G', false, fullAlphaBitmap(7, 11), 7, 11)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 7, CellH: 18,
			GlyphOffX: 0, GlyphOffY: 0,
			GlyphW: 7, GlyphH: 11,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0, HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 3, glyphCenterY(32, 5))
		if r < 200 {
			t.Errorf("7x11 OffY=0 字形 R=%d want >=200（7×11 尺寸导致采样失败）", r)
		}
	})

	t.Run("FullAlpha7x11OffY3Rows", func(t *testing.T) {
		u0, v0, u1, v1, ok := s.UploadGlyph('I', false, fullAlphaBitmap(7, 11), 7, 11)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 7, CellH: 18,
			GlyphOffX: 0, GlyphOffY: 3,
			GlyphW: 7, GlyphH: 11,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0, HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r0, _, _, _ := readPixel(env.gles, 3, glyphCenterY(32, 3))
		r5, _, _, _ := readPixel(env.gles, 3, glyphCenterY(32, 8))
		rMid, _, _, _ := readPixel(env.gles, 3, glyphCenterY(32, 13))
		t.Logf("7x11 OffY=3: row0 R=%d row5 R=%d row10 R=%d (全255应均>=200)", r0, r5, rMid)
		foundRed := false
		for yy := 15; yy <= 31 && !foundRed; yy++ {
			for xx := 0; xx <= 10 && !foundRed; xx++ {
				rr, _, _, _ := readPixel(env.gles, xx, yy)
				if rr > 100 {
					t.Logf("找到红色像素 GL(%d,%d) R=%d", xx, yy, rr)
					foundRed = true
				}
			}
		}
		if !foundRed {
			t.Error("整个区域无红色像素，7×11+OffY=3 字形完全未绘制（inst 数据正确但 DrawArraysInstanced 未生效）")
		}
	})

	t.Run("FullAlpha7x11OffY3CellH16", func(t *testing.T) {
		u0, v0, u1, v1, ok := s.UploadGlyph('J', false, fullAlphaBitmap(7, 11), 7, 11)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 7, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 3,
			GlyphW: 7, GlyphH: 11,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0, HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 3, glyphCenterY(32, 8))
		t.Logf("7x11 OffY=3 CellH=16: row5 R=%d", r)
		if r < 200 {
			t.Errorf("CellH=16 字形 R=%d want >=200", r)
		}
	})

	t.Run("FullAlpha8x11OffY3", func(t *testing.T) {
		u0, v0, u1, v1, ok := s.UploadGlyph('K', false, fullAlphaBitmap(8, 11), 8, 11)
		if !ok {
			t.Fatal("UploadGlyph failed")
		}
		inst := platform.CellInstance{
			X: 0, Y: 0, CellW: 8, CellH: 16,
			GlyphOffX: 0, GlyphOffY: 3,
			GlyphW: 8, GlyphH: 11,
			GlyphU0: u0, V0: v0, GlyphU1: u1, V1: v1,
			FgR: 1.0, FgG: 0, FgB: 0,
			BgR: 0, BgG: 0, BgB: 0, HasBg: 0,
		}
		if err := s.DrawInstances([]platform.CellInstance{inst}, 80, 32, [3]float32{0, 0, 0}); err != nil {
			t.Fatalf("DrawInstances: %v", err)
		}
		r, _, _, _ := readPixel(env.gles, 4, glyphCenterY(32, 8))
		t.Logf("8x11 OffY=3: row5 R=%d", r)
		if r < 200 {
			t.Errorf("8x11 OffY=3 字形 R=%d want >=200（GH=11 导致失败）", r)
		}
	})
}
