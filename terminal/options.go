package terminal

import (
	"io"
	"time"

	"github.com/LaoQi/vistty/internal/screen"
)

type Options struct {
	Shell         string
	FontPath      string
	FontSize      float64
	Width         int
	Height        int
	RepeatDelay   time.Duration
	RepeatRate    time.Duration
	OnTitle       func(string)
	OnDefaultColor func(fg, bg screen.Color)
	RecordWriter  io.Writer
	Primary       string
	Mode          string
}

func DefaultOptions() Options {
	return Options{
		Shell:       "/bin/bash",
		FontPath:    "",
		FontSize:    14,
		Width:       800,
		Height:      600,
		RepeatDelay: 250 * time.Millisecond,
		RepeatRate:  33 * time.Millisecond,
		Mode:        "independent",
	}
}
