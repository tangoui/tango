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

// shapedNumericWidth shapes s with HarfBuzz exactly as the general path
// does, bypassing the numeric fast path: the reference the fast path must
// reproduce.
func shapedNumericWidth(t *testing.T, s string, st Style) float32 {
	t.Helper()
	st = st.norm()
	l := layout(s, st, Options{NoWrap: true}, reg.chain(st.Mono), resolveLang(st))
	return l.Width
}

// TestNumericFastPathMatchesShaping: the fast path is an optimization, so a
// numeric label must measure and place exactly as HarfBuzz shaping does —
// 26.6 advances and pair kerning included ("12.34" kerns around the dot) —
// and therefore the same in LTR and RTL.
func TestNumericFastPathMatchesShaping(t *testing.T) {
	st := Style{Size: 14, LineHeight: 22}
	for _, s := range []string{"42", "421", "1", "11", "1.1", "12.34", "-0.5", "+7"} {
		want := shapedNumericWidth(t, s, st)
		if got := Shape(s, st, Options{NoWrap: true}).Width; got != want {
			t.Errorf("%q: fast path width %v, shaped %v (diff %v)", s, got, want, got-want)
		}
		if rtl := Shape(s, st, Options{NoWrap: true, Direction: DirRTL}).Width; rtl != want {
			t.Errorf("%q: RTL width %v, shaped %v", s, rtl, want)
		}
		var l Layout
		if got := ShapeInto(&l, s, st, Options{NoWrap: true}).Width; got != want {
			t.Errorf("%q: ShapeInto width %v, shaped %v", s, got, want)
		}
	}
}

// TestNumericFastPathGlyphsMatchShaping: not only the width — every glyph's
// id and x position on the fast path equals full shaping, and the fast path
// is really taken (a silent fallback to shaping would also match).
func TestNumericFastPathGlyphsMatchShaping(t *testing.T) {
	for _, st := range []Style{{Size: 11}, {Size: 14, LineHeight: 22}, {Size: 13, Weight: Bold}} {
		st = st.norm()
		for _, s := range []string{"452.79", "12.34", "-0.5", "7.7.7", "100.00", "1"} {
			var fl Layout
			fast := directNumericLayoutInto(&fl, s, st, Options{NoWrap: true}, reg.version)
			if fast == nil {
				t.Fatalf("size %v %q: fast path declined", st.Size, s)
			}
			ref := layout(s, st, Options{NoWrap: true}, reg.chain(st.Mono), resolveLang(st))
			if len(fast.Glyphs) != len(ref.Glyphs) {
				t.Fatalf("size %v %q: %d glyphs, shaped %d", st.Size, s, len(fast.Glyphs), len(ref.Glyphs))
			}
			for i := range ref.Glyphs {
				if fast.Glyphs[i].ID != ref.Glyphs[i].ID || fast.Glyphs[i].X != ref.Glyphs[i].X {
					t.Errorf("size %v %q glyph %d: id %d x %v, shaped id %d x %v", st.Size, s, i, fast.Glyphs[i].ID, fast.Glyphs[i].X, ref.Glyphs[i].ID, ref.Glyphs[i].X)
				}
			}
		}
	}
}
