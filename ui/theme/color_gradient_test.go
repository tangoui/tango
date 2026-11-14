package theme

import "testing"

func TestGradientParseCSSAt(t *testing.T) {
	g, err := ParseGradient("linear-gradient(90deg, #1677ff 0%, rgb(114, 46, 209) 100%)")
	if err != nil {
		t.Fatal(err)
	}
	if g.Angle != 90 || len(g.Stops) != 2 || g.Stops[1].Color != Hex("#722ed1") || g.Stops[1].Offset != 1 {
		t.Fatalf("parse: %+v", g)
	}
	if s := g.CSS(); s != "linear-gradient(90deg, #1677ff 0%, #722ed1 100%)" {
		t.Fatalf("css %q", s)
	}
	if c := g.At(0.5); c != Lerp(Hex("#1677ff"), Hex("#722ed1"), 0.5) {
		t.Fatalf("at 0.5 = %v", c)
	}
	if g.At(-1) != Hex("#1677ff") || g.At(2) != Hex("#722ed1") {
		t.Fatal("ends clamp")
	}

	// Direction keywords, unpositioned stops spread evenly, unsorted
	// stops sorted for CSS / At.
	g2, err := ParseGradient("linear-gradient(to left, red, #00ff00, blue 80%)")
	if err == nil {
		t.Fatal("named colors are not supported")
	}
	g2, err = ParseGradient("linear-gradient(to left, #f00, #0f0, #00f 80%)")
	if err != nil {
		t.Fatal(err)
	}
	if g2.Angle != 270 || g2.Stops[1].Offset != 0.4 || g2.Stops[2].Offset != 0.8 {
		t.Fatalf("spread: %+v", g2)
	}
	g3, _ := ParseGradient("linear-gradient(#000, #fff)")
	if g3.Angle != 180 {
		t.Fatalf("default angle %v", g3.Angle)
	}
	u := Gradient{Angle: 45, Stops: []GradientStop{{Offset: 1, Color: Hex("#ffffff")}, {Offset: 0, Color: Hex("#000000")}}}
	if s := u.CSS(); s != "linear-gradient(45deg, #000000 0%, #ffffff 100%)" {
		t.Fatalf("sorted css %q", s)
	}
	if u.Stops[0].Offset != 1 {
		t.Fatal("Sorted must not reorder the original")
	}
	if (Gradient{}).CSS() != "" || !(Gradient{}).IsZero() || (Gradient{}).At(0.3) != (Color{}) {
		t.Fatal("zero gradient")
	}
	for _, bad := range []string{"", "radial-gradient(#000, #fff)", "linear-gradient(90deg, #000)", "linear-gradient(90deg, nope, #fff)"} {
		if _, err := ParseGradient(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
