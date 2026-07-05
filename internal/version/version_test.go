package version

import (
	"strings"
	"testing"
)

func TestGetNonEmpty(t *testing.T) {
	i := Get()
	if i.Version == "" {
		t.Fatal("Version empty")
	}
	if i.GoVersion == "" {
		t.Fatal("GoVersion empty")
	}
}

func TestStringFormat(t *testing.T) {
	s := String()
	if !strings.HasPrefix(s, "vistty ") {
		t.Fatalf("String() = %q, want prefix %q", s, "vistty ")
	}
	if !strings.Contains(s, "go:") {
		t.Fatalf("String() = %q, want go: line", s)
	}
}
