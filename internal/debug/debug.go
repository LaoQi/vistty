package debug

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	writers []io.Writer
	file    *os.File
	on      bool

	errMu      sync.Mutex
	errWriters []io.Writer
	errFile    *os.File
)

func init() {
	on = os.Getenv("VISTTY_DEBUG") != ""
	configureFromEnv()
	configureErrorFromEnv()
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

func configureErrorFromEnv() {
	path := os.Getenv("VISTTY_ERROR_LOG")
	stderr := os.Getenv("VISTTY_ERROR_STDERR") != "0"
	if path == "" {
		p, err := defaultErrorLogPath()
		if err != nil {
			errMu.Lock()
			errWriters = []io.Writer{os.Stderr}
			errMu.Unlock()
			return
		}
		path = p
	}
	if err := configureErrorLocked(path, stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error log: cannot open %q: %v\n", path, err)
		errMu.Lock()
		errWriters = []io.Writer{os.Stderr}
		errMu.Unlock()
	}
}

func defaultErrorLogPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "vistty", "error.log"), nil
}

func ConfigureError(path string, stderr bool) error {
	errMu.Lock()
	defer errMu.Unlock()
	return configureErrorLocked(path, stderr)
}

func configureErrorLocked(path string, stderr bool) error {
	if errFile != nil {
		errFile.Close()
		errFile = nil
	}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			if stderr {
				errWriters = []io.Writer{os.Stderr}
			} else {
				errWriters = nil
			}
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			if stderr {
				errWriters = []io.Writer{os.Stderr}
			} else {
				errWriters = nil
			}
			return err
		}
		errFile = f
		if stderr {
			errWriters = []io.Writer{os.Stderr, errFile}
		} else {
			errWriters = []io.Writer{errFile}
		}
		return nil
	}
	if stderr {
		errWriters = []io.Writer{os.Stderr}
	} else {
		errWriters = nil
	}
	return nil
}

func Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02 15:04:05.000 ")
	errMu.Lock()
	defer errMu.Unlock()
	for _, w := range errWriters {
		io.WriteString(w, ts+msg)
	}
}

func Warningf(format string, args ...any) {
	Errorf("WARNING: "+format, args...)
}
