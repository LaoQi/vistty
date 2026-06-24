package font

import (
	"testing"
)

func TestFaceCacheSameSizeReturnsSameFace(t *testing.T) {
	c, err := NewFaceCache(EmbeddedFontData(), 72)
	if err != nil {
		t.Fatalf("NewFaceCache failed: %v", err)
	}
	defer c.Close()

	f1, err := c.Get(14)
	if err != nil {
		t.Fatalf("Get(14) failed: %v", err)
	}
	f2, err := c.Get(14)
	if err != nil {
		t.Fatalf("second Get(14) failed: %v", err)
	}
	if f1 != f2 {
		t.Error("expected same face instance for repeated Get(14)")
	}
}

func TestFaceCacheDifferentSizesDifferentMetrics(t *testing.T) {
	c, err := NewFaceCache(EmbeddedFontData(), 72)
	if err != nil {
		t.Fatalf("NewFaceCache failed: %v", err)
	}
	defer c.Close()

	small, err := c.Get(8)
	if err != nil {
		t.Fatalf("Get(8) failed: %v", err)
	}
	large, err := c.Get(32)
	if err != nil {
		t.Fatalf("Get(32) failed: %v", err)
	}
	if small == large {
		t.Fatal("expected distinct face instances for different sizes")
	}

	sm := small.Metrics()
	lm := large.Metrics()
	if lm.Width <= sm.Width {
		t.Errorf("expected larger font to have greater width, got small=%d large=%d", sm.Width, lm.Width)
	}
	if lm.Height <= sm.Height {
		t.Errorf("expected larger font to have greater height, got small=%d large=%d", sm.Height, lm.Height)
	}
}

func TestFaceCacheInvalidFontData(t *testing.T) {
	_, err := NewFaceCache([]byte("not a font"), 72)
	if err == nil {
		t.Error("expected error for invalid font data")
	}
}

func TestFaceCacheCloseReleasesFaces(t *testing.T) {
	c, err := NewFaceCache(EmbeddedFontData(), 72)
	if err != nil {
		t.Fatalf("NewFaceCache failed: %v", err)
	}
	if _, err := c.Get(14); err != nil {
		t.Fatalf("Get(14) failed: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
