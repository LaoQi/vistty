package main

import "testing"

func TestResolveTtyPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"2", "/dev/tty2"},
		{"12", "/dev/tty12"},
		{"/dev/tty3", "/dev/tty3"},
		{"/dev/console", "/dev/console"},
		{"tty4", "tty4"},
		{"abc", "abc"},
	}
	for _, c := range cases {
		if got := resolveTtyPath(c.in); got != c.want {
			t.Errorf("resolveTtyPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
