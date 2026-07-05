package font

import "testing"

func TestSynthBlockElements(t *testing.T) {
	face, err := NewEmbeddedFace(14, 96)
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	m := face.Metrics()

	for _, r := range []rune{0x2580, 0x2584, 0x2588, 0x2592, 0x258C, 0x2590, 0x2581, 0x2587, 0x2594} {
		g, err := face.Glyph(r)
		if err != nil {
			t.Errorf("U+%04X err=%v", r, err)
			continue
		}
		if g == nil {
			t.Errorf("U+%04X %c: synth returned nil", r, r)
			continue
		}
		t.Logf("U+%04X %c: w=%d h=%d xoff=%d yoff=%d", r, r, g.Width, g.Height, g.XOffset, g.YOffset)
	}

	if g, _ := face.Glyph(0x2588); g != nil {
		if g.Width != m.Width || g.Height != m.Height {
			t.Errorf("█ want %dx%d got %dx%d", m.Width, m.Height, g.Width, g.Height)
		}
		if g.YOffset != -m.Ascent {
			t.Errorf("█ yoff want %d got %d", -m.Ascent, g.YOffset)
		}
	}

	if g, _ := face.Glyph(0x2580); g != nil {
		if g.Height != m.Height/2 {
			t.Errorf("▀ h want %d got %d", m.Height/2, g.Height)
		}
		if g.YOffset != -m.Ascent {
			t.Errorf("▀ yoff want %d got %d", -m.Ascent, g.YOffset)
		}
	}

	if g, _ := face.Glyph(0x2584); g != nil {
		if g.YOffset != -m.Ascent+m.Height/2 {
			t.Errorf("▄ yoff want %d got %d", -m.Ascent+m.Height/2, g.YOffset)
		}
	}

	if g, _ := face.Glyph(0x2590); g != nil {
		want := m.Width - m.Width/2
		if g.XOffset != want {
			t.Errorf("▐ xoff want %d got %d", want, g.XOffset)
		}
	}

	if g, _ := face.Glyph('o'); g == nil {
		t.Error("'o' should be present from font")
	}
}
