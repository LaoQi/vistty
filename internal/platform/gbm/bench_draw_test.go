//go:build gbm_gl

package gbm

import (
	"testing"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/gl"
	"github.com/LaoQi/vistty/internal/platform/gpu"
)

// BenchmarkGBMDrawStandalone 独立创建 surface 的 DrawInstances benchmark。
// 不依赖共享 setup 的 makeGlyphInstances（避免 58 次 TexSubImage2D 后 context 损坏），
// 仅上传 1 个字形后重复 DrawInstances，模拟稳态渲染。
func BenchmarkGBMDrawStandalone(b *testing.B) {
	getGLEnv()
	if glEnvInst == nil {
		b.Skip(glEnvSkip)
	}
	env := glEnvInst
	_ = env.egl.MakeCurrent(env.disp, gl.EGL_NO_SURFACE, gl.EGL_NO_SURFACE, gl.EGL_NO_CONTEXT)

	const w, h = 640, 384
	gbmFmt := env.nativeVisual
	if gbmFmt == 0 {
		gbmFmt = GBM_FORMAT_XRGB8888
	}
	gbmSurf := env.gbmL.SurfaceCreate(env.gbmDev, uint32(w), uint32(h), gbmFmt, GBM_BO_USE_SCANOUT|GBM_BO_USE_RENDERING)
	if gbmSurf == 0 {
		b.Skip("gbm_surface_create failed")
	}
	eglSurf := env.egl.CreateWindowSurface(env.disp, env.cfg, gbmSurf)
	if eglSurf == 0 || eglSurf == gl.EGL_NO_SURFACE {
		env.gbmL.SurfaceDestroy(gbmSurf)
		b.Skip("eglCreateWindowSurface failed")
	}
	if err := env.egl.MakeCurrent(env.disp, eglSurf, eglSurf, env.ctx); err != nil {
		eglErr := env.egl.GetError()
		env.egl.DestroySurface(env.disp, eglSurf)
		env.gbmL.SurfaceDestroy(gbmSurf)
		b.Skipf("eglMakeCurrent: %v (eglErr=%s)", err, gl.EGLErrorString(eglErr))
	}
	defer func() {
		_ = env.egl.MakeCurrent(env.disp, gl.EGL_NO_SURFACE, gl.EGL_NO_SURFACE, gl.EGL_NO_CONTEXT)
		env.egl.DestroySurface(env.disp, eglSurf)
		env.gbmL.SurfaceDestroy(gbmSurf)
	}()

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
		width:      w,
		height:     h,
		active:     true,
	}
	s.ensureCPUBuf()

	s.gpu = gpu.NewRenderer(env.gles, env.egl, env.disp, eglSurf, env.ctx, w, h)
	if err := s.gpu.Init(); err != nil {
		b.Fatalf("gpu.Init: %v", err)
	}
	if !s.gpu.Ready() {
		b.Skip("GPU renderer not ready")
	}

	// 上传 1 个字形并构造 80x24 个 instance（全部用同一字形，cache 命中）
	face, err := font.NewEmbeddedFace(14, 72)
	if err != nil {
		b.Skip("embedded font: " + err.Error())
	}
	defer face.Close()
	m := face.Metrics()
	g, err := face.Glyph('A')
	if err != nil || g == nil || g.Width == 0 {
		b.Skip("no glyph A")
	}
	u0, v0, u1, v1, ok := s.UploadGlyph('A', false, g.Bitmap, g.Width, g.Height)
	if !ok {
		b.Fatal("UploadGlyph A failed")
	}
	glFinish()
	env.gles.GetError()

	const cellW, cellH = 8, 16
	const cols, rows = 80, 24
	insts := make([]platform.CellInstance, cols*rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			insts[r*cols+c] = platform.CellInstance{
				X:         float32(c * cellW),
				Y:         float32(r * cellH),
				CellW:     float32(cellW),
				CellH:     float32(cellH),
				GlyphOffX: float32(g.XOffset),
				GlyphOffY: float32(m.Ascent + g.YOffset),
				GlyphW:    float32(g.Width),
				GlyphH:    float32(g.Height),
				GlyphU0:   u0, V0: v0, GlyphU1: u1, V1: v1,
				FgR: 1, FgG: 1, FgB: 1,
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 刷新上一帧 GL 命令（驱动在命令未完成时 MakeCurrent 偶发 BAD_ACCESS）
		if i > 0 {
			glFinish()
		}
		if err := s.BeginFrame(); err != nil {
			b.Fatalf("BeginFrame i=%d: %v", i, err)
		}
		if err := s.DrawInstances(insts, w, h, [3]float32{0, 0, 0}); err != nil {
			b.Fatalf("DrawInstances i=%d: %v", i, err)
		}
	}
}
