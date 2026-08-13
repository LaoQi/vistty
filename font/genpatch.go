package font

import (
	"bytes"
	"encoding/binary"
	"errors"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// targetUpem is the normalization unitsPerEm used by all vfp files.
const targetUpem = 2048

// GenPatch extracts the requested runes from fontData as simple TrueType glyf
// outlines normalized to 2048 unitsPerEm, returning a complete vfp byte
// stream. Runes not present in the source font are returned in missing.
// Runes whose glyph is a colored (bitmap) glyph that cannot be represented as
// a simple outline are returned in skipped.
func GenPatch(fontData []byte, runes []rune) (vfpData []byte, missing []rune, skipped []rune, err error) {
	f, err := sfnt.Parse(fontData)
	if err != nil {
		return nil, nil, nil, err
	}
	ppem := fixed.Int26_6(targetUpem) << 6
	b := &sfnt.Buffer{}

	var entries []VfpEntry
	var glyphs [][]byte
	seen := make(map[rune]bool)
	for _, r := range runes {
		if seen[r] {
			continue
		}
		seen[r] = true

		gi, err := f.GlyphIndex(b, r)
		if err != nil {
			return nil, nil, nil, err
		}
		if gi == 0 {
			missing = append(missing, r)
			continue
		}

		segs, err := f.LoadGlyph(b, gi, ppem, nil)
		if err != nil {
			if err == sfnt.ErrColoredGlyph {
				skipped = append(skipped, r)
				continue
			}
			return nil, nil, nil, err
		}

		glyf, err := encodeGlyfSimple(segs)
		if err != nil {
			return nil, nil, nil, err
		}

		adv, err := f.GlyphAdvance(b, gi, ppem, font.HintingNone)
		if err != nil {
			return nil, nil, nil, err
		}
		a := adv.Round()
		if a < 0 {
			a = 0
		}
		if a > 0xFFFF {
			a = 0xFFFF
		}

		bounds, _, err := f.GlyphBounds(b, gi, ppem, font.HintingNone)
		if err != nil {
			return nil, nil, nil, err
		}
		lsb := bounds.Min.X.Round()
		if lsb < -0x8000 {
			lsb = -0x8000
		}
		if lsb > 0x7FFF {
			lsb = 0x7FFF
		}

		entries = append(entries, VfpEntry{Rune: r, Advance: uint16(a), Lsb: int16(lsb)})
		glyphs = append(glyphs, glyf)
	}

	vfpData, err = EncodeVFP(entries, glyphs)
	if err != nil {
		return nil, nil, nil, err
	}
	return vfpData, missing, skipped, nil
}

// encodeGlyfSimple encodes a vector path as a simple (non-compound) TrueType
// glyf glyph. The input segments are assumed to be normalized to 2048 units
// per em with the Y axis increasing down; glyf expects Y up, so Y is flipped.
// It returns an empty slice for an empty glyph (no contours).
func encodeGlyfSimple(segs sfnt.Segments) ([]byte, error) {
	if len(segs) == 0 {
		return []byte{}, nil
	}

	type point struct {
		x, y int16
		on   bool
	}
	var contours [][]point
	var cur []point
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if len(cur) > 0 {
				contours = append(contours, cur)
				cur = nil
			}
			cur = append(cur, point{x: coord(s.Args[0].X), y: coord(-s.Args[0].Y), on: true})
		case sfnt.SegmentOpLineTo:
			cur = append(cur, point{x: coord(s.Args[0].X), y: coord(-s.Args[0].Y), on: true})
		case sfnt.SegmentOpQuadTo:
			// Args[0] is the off-curve control point, Args[1] the on-curve end.
			// sfnt already resolves runs of off-curve points (with implicit
			// midpoints) into on-curve quad endpoints, so a simple off+on pair
			// reproduces the shape.
			cur = append(cur, point{x: coord(s.Args[0].X), y: coord(-s.Args[0].Y), on: false})
			cur = append(cur, point{x: coord(s.Args[1].X), y: coord(-s.Args[1].Y), on: true})
		case sfnt.SegmentOpCubeTo:
			return nil, errors.New("genpatch: cubic segment not representable in TrueType glyf")
		}
	}
	if len(cur) > 0 {
		contours = append(contours, cur)
	}

	var pts []point
	for _, c := range contours {
		pts = append(pts, c...)
	}
	if len(pts) == 0 {
		return []byte{}, nil
	}

	// Bounding box.
	xMin, yMin, xMax, yMax := pts[0].x, pts[0].y, pts[0].x, pts[0].y
	for _, p := range pts {
		if p.x < xMin {
			xMin = p.x
		}
		if p.y < yMin {
			yMin = p.y
		}
		if p.x > xMax {
			xMax = p.x
		}
		if p.y > yMax {
			yMax = p.y
		}
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, int16(len(contours)))
	binary.Write(&buf, binary.BigEndian, xMin)
	binary.Write(&buf, binary.BigEndian, yMin)
	binary.Write(&buf, binary.BigEndian, xMax)
	binary.Write(&buf, binary.BigEndian, yMax)

	idx := 0
	for _, c := range contours {
		idx += len(c)
		binary.Write(&buf, binary.BigEndian, uint16(idx-1))
	}
	binary.Write(&buf, binary.BigEndian, uint16(0)) // instructionLength

	// Flags: bit0 = onCurve; all coordinates use 16-bit signed deltas, so the
	// remaining flag bits are 0.
	for _, p := range pts {
		var fl uint8
		if p.on {
			fl = 0x01
		}
		buf.WriteByte(fl)
	}

	prevX := 0
	for _, p := range pts {
		dx := int(p.x) - prevX
		binary.Write(&buf, binary.BigEndian, int16(dx))
		prevX = int(p.x)
	}
	prevY := 0
	for _, p := range pts {
		dy := int(p.y) - prevY
		binary.Write(&buf, binary.BigEndian, int16(dy))
		prevY = int(p.y)
	}

	return buf.Bytes(), nil
}

// coord rounds a fixed.Int26_6 coordinate to the nearest integer font unit.
func coord(v fixed.Int26_6) int16 {
	return int16(v.Round())
}
