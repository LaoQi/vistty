package debug

import (
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	mu      sync.Mutex
	writers []io.Writer
	file    *os.File
	on      bool
)

func init() {
	on = os.Getenv("VISTTY_DEBUG") != ""
	configureFromEnv()
}

func configureFromEnv() {
	mu.Lock()
	defer mu.Unlock()
	stderr := os.Getenv("VISTTY_DEBUG_STDERR") != "0"
	path := os.Getenv("VISTTY_DEBUG_FILE")
	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "debug: cannot open VISTTY_DEBUG_FILE %q: %v\n", path, err)
			if stderr {
				writers = []io.Writer{os.Stderr}
			} else {
				writers = nil
			}
			return
		}
		file = f
		if stderr {
			writers = []io.Writer{os.Stderr, file}
		} else {
			writers = []io.Writer{file}
		}
		return
	}
	if stderr {
		writers = []io.Writer{os.Stderr}
	} else {
		writers = nil
	}
}

func Enabled() bool { return on }

func Debugf(format string, args ...any) {
	if !on {
		return
	}
	s := fmt.Sprintf(format, args...)
	mu.Lock()
	defer mu.Unlock()
	for _, w := range writers {
		io.WriteString(w, s)
	}
}

func Configure(path string, stderr bool) error {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			if stderr {
				writers = []io.Writer{os.Stderr}
			} else {
				writers = nil
			}
			return err
		}
		file = f
		if stderr {
			writers = []io.Writer{os.Stderr, file}
		} else {
			writers = []io.Writer{file}
		}
		return nil
	}
	if stderr {
		writers = []io.Writer{os.Stderr}
	} else {
		writers = nil
	}
	return nil
}
