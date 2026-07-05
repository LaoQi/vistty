package terminal

import (
	"io"
	"time"

	"github.com/LaoQi/vistty/internal/screen"
)

type Options struct {
	Shell            string
	FontPath         string
	FallbackFontPath string // path to fallback font; empty uses built-in NerdFont subset
	FontSize         float64
	RepeatDelay    time.Duration
	RepeatRate     time.Duration
	OnTitle        func(string)
	OnDefaultColor func(fg, bg screen.Color)
	OnCursorColor  func(screen.Color)
	Theme          *Theme
	RecordWriter   io.Writer
	Primary        string
}

func DefaultOptions() Options {
	return Options{
		Shell:            "/bin/bash",
		FontPath:         "",
		FallbackFontPath: "",
		FontSize:         14,
		RepeatDelay:      250 * time.Millisecond,
		RepeatRate:       33 * time.Millisecond,
	}
}
