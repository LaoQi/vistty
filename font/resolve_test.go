package font

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeTemp writes data to a temp file and returns its path (cleaned up by
// the caller's t.Cleanup).
func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// defaultExtras computes the expected default-chain extras for the current
// embedded assets: patch (when present) followed by the Nerd fallback tail.
// The patch layer is queried dynamically rather than hardcoded because assets
// are append-only — a later patch addition must not break this test.
func defaultExtras() ([][]byte, error) {
	extras := make([][]byte, 0, 2)
	patch, err := EmbeddedPatchFontData()
	if err != nil {
		return nil, err
	}
	if len(patch) > 0 {
		extras = append(extras, patch)
	}
	extras = append(extras, EmbeddedFallbackFontData())
	return extras, nil
}

// TestResolveFontChainDefault verifies the default chain (no custom font)
// resolves to the embedded Sarasa primary plus patch (if any) and the embedded
// Nerd fallback tail.
func TestResolveFontChainDefault(t *testing.T) {
	primary, extras, err := ResolveFontChain("", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primary, EmbeddedFontData()) {
		t.Error("default primary != embedded Sarasa")
	}
	want, err := defaultExtras()
	if err != nil {
		t.Fatalf("defaultExtras: %v", err)
	}
	if len(extras) != len(want) {
		t.Fatalf("default extras = %d, want %d (patch + Nerd fallback tail)", len(extras), len(want))
	}
	for i := range want {
		if !bytes.Equal(extras[i], want[i]) {
			t.Errorf("extras[%d] mismatch", i)
		}
	}
}

// TestResolveFontChainCustomFont verifies a custom primary font disables the
// default patch + Nerd fallback layers (no fallback configured).
func TestResolveFontChainCustomFont(t *testing.T) {
	custom := []byte("custom-primary-bytes")
	p := writeTemp(t, custom)
	primary, extras, err := ResolveFontChain(p, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primary, custom) {
		t.Error("custom primary not returned")
	}
	if len(extras) != 0 {
		t.Fatalf("custom-font extras = %d, want 0 (default chain disabled)", len(extras))
	}
}

// TestResolveFontChainCustomFontFallback verifies a custom primary font keeps
// only the user-supplied fallback (no patch, no embedded Nerd).
func TestResolveFontChainCustomFontFallback(t *testing.T) {
	custom := []byte("custom-primary-bytes")
	fallback := []byte("custom-fallback-bytes")
	p := writeTemp(t, custom)
	f := writeTemp(t, fallback)
	primary, extras, err := ResolveFontChain(p, f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primary, custom) {
		t.Error("custom primary not returned")
	}
	if len(extras) != 1 {
		t.Fatalf("extras = %d, want 1 (user fallback only)", len(extras))
	}
	if !bytes.Equal(extras[0], fallback) {
		t.Error("extras[0] != user fallback")
	}
}

// TestResolveFontChainDefaultCustomFallback verifies the default primary with
// a user-supplied fallback keeps the patch layer then the user fallback.
func TestResolveFontChainDefaultCustomFallback(t *testing.T) {
	fallback := []byte("custom-fallback-bytes")
	f := writeTemp(t, fallback)
	primary, extras, err := ResolveFontChain("", f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(primary, EmbeddedFontData()) {
		t.Error("primary != embedded Sarasa")
	}
	// Chain order: patch (if any in assets) then user fallback.
	want, err := defaultExtras()
	if err != nil {
		t.Fatalf("defaultExtras: %v", err)
	}
	want[len(want)-1] = fallback
	if len(extras) != len(want) {
		t.Fatalf("extras = %d, want %d (patch + user fallback)", len(extras), len(want))
	}
	for i := range want {
		if !bytes.Equal(extras[i], want[i]) {
			t.Errorf("extras[%d] mismatch", i)
		}
	}
}

// TestResolveFontChainErrors verifies file read errors are propagated.
func TestResolveFontChainErrors(t *testing.T) {
	if _, _, err := ResolveFontChain("/nonexistent/font.ttf", ""); err == nil {
		t.Error("missing primary font path: want error")
	}
	if _, _, err := ResolveFontChain("", "/nonexistent/fallback.ttf"); err == nil {
		t.Error("missing fallback font path: want error")
	}
}
