package terminal

import (
	"io"
	"time"

	"github.com/LaoQi/vistty/internal/screen"
)

type Options struct {
	Shell            string
	FontPath         string
	FallbackFontPath string
	FontSize         float64
	Scrollback       int
	RepeatDelay      time.Duration
	RepeatRate       time.Duration
	OnTitle          func(string)
	OnDefaultColor   func(fg, bg screen.Color)
	OnCursorColor    func(screen.Color)
	OnRenderRequest  func()
	Theme            *Theme
	RecordWriter     io.Writer
	Primary          string
}

func DefaultOptions() Options {
	return Options{
		Shell:            "/bin/bash",
		FontPath:         "",
		FallbackFontPath: "",
		FontSize:         14,
		Scrollback:       10000,
		RepeatDelay:      250 * time.Millisecond,
		RepeatRate:       33 * time.Millisecond,
	}
}
