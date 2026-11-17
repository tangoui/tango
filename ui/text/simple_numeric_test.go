package text

import "testing"

func TestShapeIntoReusesNumericLayout(t *testing.T) {
	var l Layout
	a := ShapeInto(&l, "12.34", Style{Size: 11}, Options{NoWrap: true})
	if a != &l {
		t.Fatal("numeric ShapeInto did not return destination")
	}
	glyphs := &l.Glyphs[0]
	a = ShapeInto(&l, "87.01", Style{Size: 11}, Options{NoWrap: true})
	if a != &l || &l.Glyphs[0] != glyphs {
		t.Fatal("numeric layout buffers were replaced")
	}
}

func TestDirectNumericLayoutPreservesMetrics(t *testing.T) {
	st := Style{Size: 11}
	opt := Options{NoWrap: true}
	got := directNumericLayout("123.45", st, opt, 0)
	if got == nil || len(got.Glyphs) != 6 {
		t.Fatalf("direct numeric layout = %#v", got)
	}
	want := Shape("123.45", st, opt)
	if absFloat(got.Width-want.Width) > 0.01 || got.Height != want.Height {
		t.Fatalf("direct metrics width=%v height=%v; shaped width=%v height=%v", got.Width, got.Height, want.Width, want.Height)
	}
}

func absFloat(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// TestDirectNumericLayoutCachesGlyphMetrics: once a numeric face is known,
// laying out another number must not re-evaluate variable-font advances (HVAR
// deltas dominate a 600-cell grid frame); glyph IDs and advances come from a
// per-face table, and the result matches a fresh layout exactly.
func TestDirectNumericLayoutCachesGlyphMetrics(t *testing.T) {
	clearNumericFaces()
	st := Style{Size: 11}
	opt := Options{NoWrap: true}
	var l Layout
	directNumericLayoutInto(&l, "0123456789.-+", st, opt, 0)
	nf, ok := numericFaceFor(st.norm(), resolveLang(st.norm()), 0)
	if !ok || !nf.table.ok {
		t.Fatal("numeric face has no cached glyph table")
	}
	fresh := directNumericLayout("-45.67", st, opt, 0)
	got := directNumericLayoutInto(&l, "-45.67", st, opt, 0)
	if got.Width != fresh.Width || len(got.Glyphs) != len(fresh.Glyphs) {
		t.Fatalf("cached layout width %v (%d glyphs), fresh %v (%d)", got.Width, len(got.Glyphs), fresh.Width, len(fresh.Glyphs))
	}
	for i := range got.Glyphs {
		if got.Glyphs[i] != fresh.Glyphs[i] {
			t.Fatalf("glyph %d differs: %+v vs %+v", i, got.Glyphs[i], fresh.Glyphs[i])
		}
	}
	if a := testing.AllocsPerRun(50, func() { directNumericLayoutInto(&l, "987.65", st, opt, 0) }); a > 1 {
		t.Fatalf("reused numeric layout allocated %.0f times", a)
	}
}

// TestDirectNumericLayoutHasCarets: a numeric label stays selectable — every
// rune boundary has a caret, matching the shaped layout.
func TestDirectNumericLayoutHasCarets(t *testing.T) {
	st := Style{Size: 13}
	opt := Options{NoWrap: true}
	got := directNumericLayout("12.5", st, opt, 0)
	if got == nil || len(got.Lines) != 1 || len(got.Lines[0].carets) != 5 {
		t.Fatalf("direct numeric layout carets = %+v", got)
	}
	c := got.Lines[0].carets
	if c[0] != 0 || c[4] != got.Width {
		t.Fatalf("carets %v do not span 0..%v", c, got.Width)
	}
	for i := 1; i < len(c); i++ {
		if c[i] <= c[i-1] {
			t.Fatalf("carets not increasing: %v", c)
		}
	}
}
