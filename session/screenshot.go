package session

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// defaultScreenshotDir 按优先级返回截图保存目录：
//  1. $XDG_PICTURES_DIR
//  2. ~/Pictures（若存在且为目录）
//  3. $HOME
//  4. /tmp（兜底，DRM/tty 场景 HOME 可能为只读）
func defaultScreenshotDir() string {
	if d := os.Getenv("XDG_PICTURES_DIR"); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		pic := filepath.Join(home, "Pictures")
		if st, err := os.Stat(pic); err == nil && st.IsDir() {
			return pic
		}
		return home
	}
	return "/tmp"
}

// defaultScreenshotPath 生成默认截图文件路径。
// 多屏时附加输出名以区分（如 vistty-20260803-214500-HDMI-A-1.png）。
func defaultScreenshotPath(outputName string, multiScreen bool) string {
	ts := time.Now().Format("20060102-150405")
	name := "vistty-" + ts
	if multiScreen && outputName != "" {
		name += "-" + outputName
	}
	return filepath.Join(defaultScreenshotDir(), name+".png")
}

// saveScreenshotPNG 将 BGRA32 像素写入 PNG 文件。
// 内部将 BGRA 转 NRGBA 并强制 alpha=255（Wayland shm buffer alpha
// 可能为 0，会导致 PNG 全透明）。
func saveScreenshotPNG(path string, data []byte, stride, width, height int) (string, error) {
	if width <= 0 || height <= 0 {
		return "", fmt.Errorf("invalid dimensions %dx%d", width, height)
	}
	pix := make([]byte, width*height*4)
	for y := 0; y < height; y++ {
		src := data[y*stride:]
		dst := pix[y*width*4:]
		for x := 0; x < width; x++ {
			dst[x*4+0] = src[x*4+2] // R
			dst[x*4+1] = src[x*4+1] // G
			dst[x*4+2] = src[x*4+0] // B
			dst[x*4+3] = 255        // A
		}
	}
	img := &image.NRGBA{
		Pix:    pix,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", fmt.Errorf("png encode: %w", err)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs, nil
	}
	return path, nil
}
