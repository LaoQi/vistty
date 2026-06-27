package debug

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func errSnapshot() ([]io.Writer, *os.File) {
	return errWriters, errFile
}

func errRestore(oldWriters []io.Writer, oldFile *os.File) {
	errMu.Lock()
	defer errMu.Unlock()
	if errFile != nil {
		errFile.Close()
	}
	errFile = oldFile
	errWriters = oldWriters
}

func TestErrorfWritesToWriters(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	var buf bytes.Buffer
	errMu.Lock()
	errWriters = []io.Writer{&buf}
	errMu.Unlock()

	Errorf("render error: %v\n", "page flip failed")
	got := buf.String()
	if !strings.Contains(got, "render error: page flip failed\n") {
		t.Fatalf("expected render error in output, got %q", got)
	}
}

func TestErrorfTimestamp(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	var buf bytes.Buffer
	errMu.Lock()
	errWriters = []io.Writer{&buf}
	errMu.Unlock()

	Errorf("boom\n")
	got := buf.String()
	if !strings.HasPrefix(got, "20") {
		t.Fatalf("expected timestamp prefix, got %q", got)
	}
	if !strings.Contains(got, "boom\n") {
		t.Fatalf("expected message, got %q", got)
	}
}

func TestConfigureErrorFile(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	dir := t.TempDir()
	path := filepath.Join(dir, "error.log")
	if err := ConfigureError(path, false); err != nil {
		t.Fatalf("ConfigureError: %v", err)
	}

	Errorf("file only error\n")

	if errFile == nil {
		t.Fatal("expected errFile to be opened")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasSuffix(string(data), "file only error\n") {
		t.Fatalf("expected file content ending with message, got %q", string(data))
	}
}

func TestConfigureErrorStderrOnly(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	if err := ConfigureError("", true); err != nil {
		t.Fatalf("ConfigureError: %v", err)
	}
	if errFile != nil {
		t.Fatal("expected no file when path empty")
	}
	if len(errWriters) != 1 || errWriters[0] != os.Stderr {
		t.Fatalf("expected only stderr, got %v", errWriters)
	}
}

func TestWarningfPrefix(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	var buf bytes.Buffer
	errMu.Lock()
	errWriters = []io.Writer{&buf}
	errMu.Unlock()

	Warningf("vt manager: %v\n", "no such device")
	got := buf.String()
	if !strings.Contains(got, "WARNING: vt manager: no such device\n") {
		t.Fatalf("expected WARNING prefix, got %q", got)
	}
}

func TestDefaultErrorLogPathXDG(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", dir)
	t.Cleanup(func() { os.Unsetenv("XDG_DATA_HOME") })

	got, err := defaultErrorLogPath()
	if err != nil {
		t.Fatalf("defaultErrorLogPath: %v", err)
	}
	want := filepath.Join(dir, "vistty", "error.log")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultErrorLogPathHomeFallback(t *testing.T) {
	os.Unsetenv("XDG_DATA_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}
	got, err := defaultErrorLogPath()
	if err != nil {
		t.Fatalf("defaultErrorLogPath: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "vistty", "error.log")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConfigureErrorStderrDisabled(t *testing.T) {
	oldWriters, oldFile := errSnapshot()
	defer errRestore(oldWriters, oldFile)

	dir := t.TempDir()
	path := filepath.Join(dir, "error.log")
	if err := ConfigureError(path, false); err != nil {
		t.Fatalf("ConfigureError: %v", err)
	}
	if len(errWriters) != 1 || errWriters[0] != errFile {
		t.Fatalf("expected only file writer, got %v", errWriters)
	}
}
