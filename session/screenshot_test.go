package session

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveScreenshotPNG(t *testing.T) {
	w, h := 4, 2
	stride := w * 4
	// BGRA: 第一行红色，第二行绿色
	data := make([]byte, stride*h)
	// row0: B=0 G=0 R=255
	for i := 0; i < w; i++ {
		data[i*4+0] = 0
		data[i*4+1] = 0
		data[i*4+2] = 255
		data[i*4+3] = 255
	}
	// row1: B=0 G=255 R=0
	for i := 0; i < w; i++ {
		data[stride+i*4+0] = 0
		data[stride+i*4+1] = 255
		data[stride+i*4+2] = 0
		data[stride+i*4+3] = 255
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "shot.png")
	if err := saveScreenshotPNG(path, data, stride, w, h); err != nil {
		t.Fatalf("saveScreenshotPNG: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("dims=%dx%d want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), w, h)
	}
	r0, g0, b0, a0 := img.At(0, 0).RGBA()
	if r0 != 0xffff || g0 != 0 || b0 != 0 || a0 != 0xffff {
		t.Fatalf("px(0,0)=r%d g%d b%d a%d want red", r0, g0, b0, a0)
	}
	r1, g1, b1, a1 := img.At(0, 1).RGBA()
	if r1 != 0 || g1 != 0xffff || b1 != 0 || a1 != 0xffff {
		t.Fatalf("px(0,1)=r%d g%d b%d a%d want green", r1, g1, b1, a1)
	}
}

func TestDefaultScreenshotPath(t *testing.T) {
	t.Setenv("XDG_PICTURES_DIR", "")
	p := defaultScreenshotPath("HDMI-A-1", true)
	if filepath.Ext(p) != ".png" {
		t.Fatalf("ext=%s want .png", filepath.Ext(p))
	}
	base := filepath.Base(p)
	if base[:7] != "vistty-" {
		t.Fatalf("prefix=%s want vistty-", base[:7])
	}
	// multi-screen 应包含输出名
	if !contains(base, "HDMI-A-1") {
		t.Fatalf("multi-screen path should contain output name: %s", base)
	}

	p2 := defaultScreenshotPath("", false)
	b2 := filepath.Base(p2)
	if contains(b2, "HDMI-A-1") {
		t.Fatalf("single-screen path should not contain output name: %s", b2)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
