package debug

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshot() (bool, []io.Writer, *os.File) {
	return on, writers, file
}

func restore(oldOn bool, oldWriters []io.Writer, oldFile *os.File) {
	mu.Lock()
	defer mu.Unlock()
	on = oldOn
	writers = oldWriters
	if file != nil {
		file.Close()
	}
	file = oldFile
}

func TestDebugfDisabled(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	var buf bytes.Buffer
	mu.Lock()
	on = false
	writers = []io.Writer{&buf}
	mu.Unlock()

	Debugf("should not appear %d\n", 42)
	if buf.Len() > 0 {
		t.Fatalf("expected no output when disabled, got %q", buf.String())
	}
}

func TestDebugfEnabled(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	var buf bytes.Buffer
	mu.Lock()
	on = true
	writers = []io.Writer{&buf}
	mu.Unlock()

	Debugf("hello %s %d\n", "world", 7)
	if got := buf.String(); got != "hello world 7\n" {
		t.Fatalf("expected %q, got %q", "hello world 7\n", got)
	}
}

func TestConfigureFileOnly(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")
	if err := Configure(path, false); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	mu.Lock()
	on = true
	mu.Unlock()

	Debugf("file only line\n")

	if file == nil {
		t.Fatal("expected file to be opened")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "file only line\n" {
		t.Fatalf("expected file content %q, got %q", "file only line\n", string(data))
	}
}

func TestConfigureFileAndStderr(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	var stderr bytes.Buffer
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")
	if err := Configure(path, true); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	mu.Lock()
	on = true
	writers = []io.Writer{&stderr, file}
	mu.Unlock()

	Debugf("both sinks\n")

	if got := stderr.String(); got != "both sinks\n" {
		t.Fatalf("stderr expected %q, got %q", "both sinks\n", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "both sinks\n" {
		t.Fatalf("file expected %q, got %q", "both sinks\n", string(data))
	}
}

func TestConfigureAppendOnReopen(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")
	if err := Configure(path, false); err != nil {
		t.Fatalf("Configure first: %v", err)
	}
	mu.Lock()
	on = true
	mu.Unlock()
	Debugf("first line\n")

	if err := Configure(path, false); err != nil {
		t.Fatalf("Configure second: %v", err)
	}
	Debugf("second line\n")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasSuffix(string(data), "second line\n") {
		t.Fatalf("expected append, got %q", string(data))
	}
}

func TestEnabledReflectsEnv(t *testing.T) {
	oldOn, oldWriters, oldFile := snapshot()
	defer restore(oldOn, oldWriters, oldFile)

	os.Setenv("VISTTY_DEBUG", "1")
	t.Cleanup(func() { os.Unsetenv("VISTTY_DEBUG") })
	mu.Lock()
	on = os.Getenv("VISTTY_DEBUG") != ""
	mu.Unlock()
	if !Enabled() {
		t.Fatal("expected Enabled() true after VISTTY_DEBUG=1")
	}
}
