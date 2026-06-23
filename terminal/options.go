package terminal

type Options struct {
	Shell    string
	FontPath string
	FontSize float64
	Width    int
	Height   int
}

func DefaultOptions() Options {
	return Options{
		Shell:    "/bin/bash",
		FontPath: "",
		FontSize: 14,
		Width:    800,
		Height:   600,
	}
}
