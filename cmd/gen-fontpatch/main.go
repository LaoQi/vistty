package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/LaoQi/vistty/font"
)

func main() {
	fontPath := flag.String("font", "", "input .ttf")
	unicodes := flag.String("unicodes", "", "unicode ranges, e.g. U+3040-30FF,U+31F0-31FF")
	fitWidth := flag.Uint("fit-width", 0, "force advance width and horizontally fit outlines to this many 2048-upem units (e.g. 1024 = Sarasa cell width)")
	out := flag.String("o", "", "output .vfp path (default: auto-assign font/assets/NNN-name.vfp)")
	name := flag.String("name", "", "patch semantic name (used for auto output path)")
	assets := flag.String("assets", "font/assets", "assets directory root for auto numbering")
	flag.Parse()

	if *fontPath == "" {
		fmt.Fprintln(os.Stderr, "usage: gen-fontpatch -font <input.ttf> [-unicodes U+3040-30FF,U+31F0-31FF] [-o output.vfp] [-name jp-kana]")
		os.Exit(1)
	}

	data, err := os.ReadFile(*fontPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *fontPath, err)
		os.Exit(1)
	}

	runes, err := resolveRunes(*unicodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve runes: %v\n", err)
		os.Exit(1)
	}
	if len(runes) == 0 {
		fmt.Fprintln(os.Stderr, "no runes specified; provide -unicodes or pipe runes on stdin")
		os.Exit(1)
	}

	var vfpData []byte
	var missing, skipped []rune
	if *fitWidth > 0 {
		vfpData, missing, skipped, err = font.GenPatchFit(data, runes, uint16(*fitWidth))
	} else {
		vfpData, missing, skipped, err = font.GenPatch(data, runes)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "genpatch: %v\n", err)
		os.Exit(1)
	}

	outPath := *out
	if outPath == "" {
		if *name == "" {
			fmt.Fprintln(os.Stderr, "no -o or -name given; cannot determine output path")
			os.Exit(1)
		}
		num, err := nextPatchNumber(*assets)
		if err != nil {
			fmt.Fprintf(os.Stderr, "next patch number: %v\n", err)
			os.Exit(1)
		}
		outPath = filepath.Join(*assets, fmt.Sprintf("%03d-%s.vfp", num, *name))
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, vfpData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
		os.Exit(1)
	}

	generated := len(runes) - len(missing) - len(skipped)
	fmt.Printf("generated %d glyphs, %d missing, %d skipped (colored)\n", generated, len(missing), len(skipped))
	if len(missing) > 0 {
		n := 20
		if len(missing) < n {
			n = len(missing)
		}
		var hex []string
		for _, r := range missing[:n] {
			hex = append(hex, fmt.Sprintf("U+%X", r))
		}
		fmt.Printf("missing (%d shown of %d): %s\n", n, len(missing), strings.Join(hex, " "))
	}
	if len(skipped) > 0 {
		n := 20
		if len(skipped) < n {
			n = len(skipped)
		}
		var hex []string
		for _, r := range skipped[:n] {
			hex = append(hex, fmt.Sprintf("U+%X", r))
		}
		fmt.Printf("skipped colored (%d shown of %d): %s\n", n, len(skipped), strings.Join(hex, " "))
	}
	fmt.Printf("wrote %s (%d bytes)\n", outPath, len(vfpData))
}

// resolveRunes parses the -unicodes argument (comma separated ranges like
// U+3040-30FF or U+31F0). If empty, it reads all unique runes from stdin.
func resolveRunes(unicodes string) ([]rune, error) {
	var runes []rune
	if unicodes != "" {
		for _, part := range strings.Split(unicodes, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lo, hi, err := parseRange(part)
			if err != nil {
				return nil, err
			}
			for r := lo; r <= hi; r++ {
				runes = append(runes, r)
			}
		}
		sortRunes(runes)
		return runes, nil
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	for _, r := range string(raw) {
		runes = append(runes, r)
	}
	sortRunes(runes)
	return runes, nil
}

func parseRange(s string) (lo, hi rune, err error) {
	s = strings.TrimPrefix(s, "U+")
	s = strings.TrimPrefix(s, "u+")
	var a, b string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		a, b = s[:i], s[i+1:]
	} else {
		a = s
	}
	ai, err := strconv.ParseInt(a, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("bad range %q", s)
	}
	lo = rune(ai)
	if b == "" {
		return lo, lo, nil
	}
	bi, err := strconv.ParseInt(b, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("bad range %q", s)
	}
	hi = rune(bi)
	if hi < lo {
		return 0, 0, fmt.Errorf("range %q reversed", s)
	}
	return lo, hi, nil
}

func sortRunes(r []rune) {
	sort.Slice(r, func(i, j int) bool { return r[i] < r[j] })
}

// nextPatchNumber scans dir for existing *.vfp files, parses the 3-digit
// prefix of each, and returns the max+1. Returns 0 if no patches exist.
func nextPatchNumber(dir string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.vfp"))
	if err != nil {
		return 0, err
	}
	max := -1
	for _, m := range matches {
		base := filepath.Base(m)
		idx := strings.IndexByte(base, '-')
		if idx < 0 {
			continue
		}
		n, err := strconv.Atoi(base[:idx])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}
