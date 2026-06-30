//go:build gbm_gl

package gbm

import (
	"sync"
	"testing"

	"github.com/LaoQi/vistty/internal/font"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/LaoQi/vistty/internal/platform/gl"
	"github.com/LaoQi/vistty/internal/platform/gpu"
	"github.com/ebitengine/purego"
)

// glFinish 刷新 GL 命令队列。render node 上 GBM surface 在 TexSubImage2D 后
// 若不刷新，后续 MakeCurrent 偶发 BAD_ACCESS（Mesa/i915 驱动问题）。
var (
	glFinishFn   func()
	glFinishOnce sync.Once
)

func glFinish() {
	glFinishOnce.Do(func() {
		lib, err := purego.Dlopen("libGLESv2.so.2", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			lib, _ = purego.Dlopen("libGLESv2.so", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		}
		if addr, err := purego.Dlsym(lib, "glFinish"); err == nil {
			purego.RegisterFunc(&glFinishFn, addr)
		}
	})
	if glFinishFn != nil {
		glFinishFn()
	}
}

// benchEnv 持有一次 benchmark 所需的 GBM surface + EGL surface + GPU Renderer。
type benchEnv struct {
	surf       *GBMSurface
	eglSurface uintptr
	gbmSurface uintptr
	cellW      int
	cellH      int
	cols       int
	rows       int
}

// setupBenchSurface 创建独立 GBM+EGL surface 并初始化 GPU Renderer。
// 注意：render node 上多次创建/销毁 GBM surface 会触发驱动竞态（EGL_BAD_ACCESS），
// 故整个 benchmark 套件只调用一次（由 BenchmarkGBM 持有），子测试共享。
func setupBenchSurface(b *testing.B, cols, rows, cellW, cellH int) *benchEnv {
	getGLEnv()
	if glEnvInst == nil {
		b.Skip(glEnvSkip)
	}
	env := glEnvInst

	w := cols * cellW
	h := rows * cellH

	_ = env.egl.MakeCurrent(env.disp, gl.EGL_NO_SURFACE, gl.EGL_NO_SURFACE, gl.EGL_NO_CONTEXT)

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
		b.Skip("GPU renderer not ready (no instanced draw)")
	}

	return &benchEnv{
		surf:       s,
		eglSurface: eglSurf,
		gbmSurface: gbmSurf,
		cellW:      cellW,
		cellH:      cellH,
		cols:       cols,
		rows:       rows,
	}
}

func (be *benchEnv) close() {
	env := glEnvInst
	if be.surf.gpu != nil {
		_ = env.egl.MakeCurrent(env.disp, be.eglSurface, be.eglSurface, env.ctx)
		be.surf.gpu.Close()
	}
	_ = env.egl.MakeCurrent(env.disp, gl.EGL_NO_SURFACE, gl.EGL_NO_SURFACE, gl.EGL_NO_CONTEXT)
	if be.eglSurface != 0 {
		env.egl.DestroySurface(env.disp, be.eglSurface)
	}
	if be.gbmSurface != 0 {
		env.gbmL.SurfaceDestroy(be.gbmSurface)
	}
}

// makeGlyphInstances 构造覆盖终端全部 cell 的 instances，字形已上传到 atlas。
func (be *benchEnv) makeGlyphInstances(b *testing.B, s *GBMSurface, runeSet []rune) []platform.CellInstance {
	face, err := font.NewEmbeddedFace(14, 72)
	if err != nil {
		b.Skip("embedded font: " + err.Error())
	}
	defer face.Close()
	m := face.Metrics()

	var insts []platform.CellInstance
	for r := 0; r < be.rows; r++ {
		for c := 0; c < be.cols; c++ {
			ru := runeSet[(r*be.cols+c)%len(runeSet)]
			g, err := face.Glyph(ru)
			if err != nil || g == nil || g.Width == 0 || g.Height == 0 {
				continue
			}
			u0, v0, u1, v1, ok := s.UploadGlyph(ru, g.Bitmap, g.Width, g.Height)
			if !ok {
				continue
			}
			if (r*be.cols+c)%10 == 0 {
				glFinish()
			}
			insts = append(insts, platform.CellInstance{
				X:         float32(c * be.cellW),
				Y:         float32(r * be.cellH),
				CellW:     float32(be.cellW),
				CellH:     float32(be.cellH),
				GlyphOffX: float32(g.XOffset),
				GlyphOffY: float32(m.Ascent + g.YOffset),
				GlyphW:    float32(g.Width),
				GlyphH:    float32(g.Height),
				GlyphU0:   u0, V0: v0, GlyphU1: u1, V1: v1,
				FgR: 1, FgG: 1, FgB: 1,
			})
		}
	}
	return insts
}

func asciiRunes(n int) []rune {
	out := make([]rune, n)
	for i := 0; i < n; i++ {
		out[i] = rune('A' + (i % 58))
	}
	return out
}

// BenchmarkGBM 是 GBM 后端性能套件入口。一次 setup 共享 surface，
// 子测试覆盖各路径。运行：
//
//	go test -tags gbm_gl -run=^$ -bench=BenchmarkGBM -benchmem ./internal/platform/gbm/
func BenchmarkGBM(b *testing.B) {
	be := setupBenchSurface(b, 80, 24, 8, 16)
	defer be.close()
	s := be.surf

	runes := asciiRunes(58)
	insts := be.makeGlyphInstances(b, s, runes)
	if len(insts) == 0 {
		b.Skip("no instances")
	}
	// 刷新 makeGlyphInstances 的 GL 命令 + 清理错误
	glFinish()
	glEnvInst.gles.GetError()

	face, err := font.NewEmbeddedFace(14, 72)
	if err != nil {
		b.Skip("embedded font: " + err.Error())
	}
	defer face.Close()
	glyphA, _ := face.Glyph('A')

	// CJK 字形池（用于 cold upload 测试）
	const pool = 600
	type gm struct {
		r rune
		g *font.Glyph
	}
	cjkPool := make([]gm, 0, pool)
	for i := 0; i < pool; i++ {
		ru := rune(0x4E00 + i)
		g, err := face.Glyph(ru)
		if err == nil && g != nil && g.Width > 0 {
			cjkPool = append(cjkPool, gm{ru, g})
		}
	}

	b.Run("UploadGlyphCold", func(b *testing.B) {
		if len(cjkPool) == 0 {
			b.Skip("no CJK glyphs")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			gm := cjkPool[i%len(cjkPool)]
			s.UploadGlyph(gm.r, gm.g.Bitmap, gm.g.Width, gm.g.Height)
			if i%100 == 0 {
				glFinish()
				glEnvInst.gles.GetError()
			}
		}
	})

	b.Run("UploadGlyphWarm", func(b *testing.B) {
		if glyphA == nil {
			b.Skip("no glyph A")
		}
		s.UploadGlyph('A', glyphA.Bitmap, glyphA.Width, glyphA.Height)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.UploadGlyph('A', glyphA.Bitmap, glyphA.Width, glyphA.Height)
		}
		glFinish()
	})

	b.Run("DrawInstances", func(b *testing.B) {
		// 刷新 UploadGlyph 残留的 GL 命令（否则 MakeCurrent 偶发 BAD_ACCESS）
		glFinish()
		glEnvInst.gles.GetError()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := s.BeginFrame(); err != nil {
				b.Fatalf("BeginFrame i=%d: %v", i, err)
			}
			if err := s.DrawInstances(insts, s.width, s.height, [3]float32{0, 0, 0}); err != nil {
				b.Fatalf("DrawInstances i=%d: %v", i, err)
			}
		}
	})

	b.Run("FullFrame", func(b *testing.B) {
		var coldG *font.Glyph
		for i := 0; i < 200; i++ {
			g, err := face.Glyph(rune(0x5000 + i))
			if err == nil && g != nil && g.Width > 0 {
				coldG = g
				break
			}
		}
		glFinish()
		glEnvInst.gles.GetError()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := s.BeginFrame(); err != nil {
				b.Fatalf("BeginFrame: %v", err)
			}
			if coldG != nil && i%10 == 0 {
				s.UploadGlyph(rune(0x6000+i%50), coldG.Bitmap, coldG.Width, coldG.Height)
				glFinish()
				glEnvInst.gles.GetError()
			}
			if err := s.DrawInstances(insts, s.width, s.height, [3]float32{0, 0, 0}); err != nil {
				b.Fatalf("DrawInstances: %v", err)
			}
		}
	})
}
