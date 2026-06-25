package replay

import (
	"math/rand"
	"strconv"
	"strings"
)

// PlainText generates n bytes of ASCII text with line breaks, simulating
// output from `cat` or `yes`. Each line is 60-78 chars wide.
func PlainText(n int) []byte {
	var b strings.Builder
	b.Grow(n)
	words := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog",
		"hello", "world", "foo", "bar", "baz", "test", "data", "line",
		"vistty", "terminal", "render", "parser", "screen", "buffer"}
	rng := rand.New(rand.NewSource(42))
	for b.Len() < n {
		lineLen := 60 + rng.Intn(19)
		for i := 0; i < lineLen; i++ {
			b.WriteString(words[rng.Intn(len(words))])
			b.WriteByte(' ')
		}
		b.WriteString("\r\n")
	}
	return []byte(b.String()[:min(n, b.Len())])
}

// CJKScroll generates n bytes of Chinese text with line breaks, simulating
// a Chinese log stream. Tests rune_width, glyph atlas LRU, and double-width
// rendering.
func CJKScroll(n int) []byte {
	chars := []rune("你好世界终端渲染解析缓冲区字体光栅化性能测试中文日志输出滚动屏幕光标颜色背景前景混合双宽字符等宽布局合成器页面翻转帧缓冲区分辨率刷新率微秒毫秒纳秒分配垃圾回收栈堆逃逸")
	var b strings.Builder
	b.Grow(n)
	rng := rand.New(rand.NewSource(42))
	for b.Len() < n {
		count := 20 + rng.Intn(30)
		for i := 0; i < count; i++ {
			b.WriteRune(chars[rng.Intn(len(chars))])
		}
		b.WriteString("\r\n")
	}
	return []byte(b.String()[:min(n, b.Len())])
}

// SGRCursor generates n bytes of alternating SGR color sequences and cursor
// movement, simulating tmux status bar refresh and colored prompts.
func SGRCursor(n int) []byte {
	var b strings.Builder
	b.Grow(n)
	rng := rand.New(rand.NewSource(42))
	for b.Len() < n {
		color := rng.Intn(255)
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(color))
		b.WriteString("m")
		b.WriteString("text\x1b[0m")
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(1 + rng.Intn(24)))
		b.WriteString(";")
		b.WriteString(strconv.Itoa(1 + rng.Intn(80)))
		b.WriteString("H")
	}
	return []byte(b.String()[:min(n, b.Len())])
}

// TUIRedraw returns the embedded nvim_full.bin recording for full-screen
// TUI redraw testing. Falls back to a synthetic screen-clear + grid if the
// recording is not available.
func TUIRedraw() []byte {
	if data := loadEmbeddedRecording(); data != nil {
		return data
	}
	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	for row := 0; row < 54; row++ {
		for col := 0; col < 80; col++ {
			b.WriteRune(rune('A' + (row+col)%26))
		}
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

// ScrollStress generates output that fills and scrolls the screen multiple
// times, testing history Push + Clone performance.
func ScrollStress() []byte {
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("line ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(": ")
		for j := 0; j < 60; j++ {
			b.WriteByte(byte('a' + (i+j)%26))
		}
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
