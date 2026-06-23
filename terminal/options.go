package terminal

import "time"

type Options struct {
	Shell       string
	FontPath    string
	FontSize    float64
	Width       int
	Height      int
	RepeatDelay time.Duration
	RepeatRate  time.Duration
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
	}
}
