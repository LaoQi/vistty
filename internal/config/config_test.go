package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Backend != "auto" {
		t.Errorf("Backend = %q, want %q", cfg.Backend, "auto")
	}
	if cfg.Shell != "/bin/bash" {
		t.Errorf("Shell = %q, want %q", cfg.Shell, "/bin/bash")
	}
	if cfg.FontSize != 14 {
		t.Errorf("FontSize = %v, want 14", cfg.FontSize)
	}
	if cfg.Mode != "independent" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "independent")
	}
}

func TestLoadNotFound(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.jsonc"))
	if err != nil {
		t.Fatalf("Load nonexistent should not error: %v", err)
	}
	def := Default()
	if cfg.Shell != def.Shell {
		t.Errorf("Shell = %q, want default %q", cfg.Shell, def.Shell)
	}
	if cfg.FontSize != def.FontSize {
		t.Errorf("FontSize = %v, want default %v", cfg.FontSize, def.FontSize)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")

	original := Default()
	original.Shell = "/bin/zsh"
	original.FontSize = 18
	original.Backend = "drm"
	original.NoGBM = true

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want /bin/zsh", loaded.Shell)
	}
	if loaded.FontSize != 18 {
		t.Errorf("FontSize = %v, want 18", loaded.FontSize)
	}
	if loaded.Backend != "drm" {
		t.Errorf("Backend = %q, want drm", loaded.Backend)
	}
	if !loaded.NoGBM {
		t.Errorf("NoGBM = false, want true")
	}
}

func TestGenerateHasComments(t *testing.T) {
	out := Default().Generate()
	if !strings.Contains(out, "//") {
		t.Error("Generate output should contain JSONC comments")
	}
	for _, field := range []string{"backend", "shell", "font", "fontsize", "primary", "mode", "nogbm", "record", "error_log"} {
		if !strings.Contains(out, `"`+field+`"`) {
			t.Errorf("Generate output missing field %q", field)
		}
	}
}

func TestLoadJSONCComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	data := `{
  // line comment
  "shell": "/bin/zsh",
  /* block comment
     spanning lines */
  "fontsize": 20,
  "backend": "wayland" // trailing comment
}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want /bin/zsh", cfg.Shell)
	}
	if cfg.FontSize != 20 {
		t.Errorf("FontSize = %v, want 20", cfg.FontSize)
	}
	if cfg.Backend != "wayland" {
		t.Errorf("Backend = %q, want wayland", cfg.Backend)
	}
}

func TestDefaultPathXDG(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("XDG_CONFIG_HOME", dir)
	t.Cleanup(func() { os.Unsetenv("XDG_CONFIG_HOME") })

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	want := filepath.Join(dir, "vistty", "config.jsonc")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPathHomeFallback(t *testing.T) {
	os.Unsetenv("XDG_CONFIG_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	want := filepath.Join(home, ".config", "vistty", "config.jsonc")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestLoadMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonc")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("error should contain 'parse config', got %v", err)
	}
}

func TestLoadPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.jsonc")
	if err := os.WriteFile(path, []byte(`{"shell": "/bin/zsh", "fontsize": 20}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want /bin/zsh", cfg.Shell)
	}
	if cfg.FontSize != 20 {
		t.Errorf("FontSize = %v, want 20", cfg.FontSize)
	}
	if cfg.Backend != "auto" {
		t.Errorf("Backend = %q, want default 'auto'", cfg.Backend)
	}
	if cfg.Mode != "independent" {
		t.Errorf("Mode = %q, want default 'independent'", cfg.Mode)
	}
}
