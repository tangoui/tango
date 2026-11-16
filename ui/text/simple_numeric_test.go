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
