package icon

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func TestAllIconsParse(t *testing.T) {
	for _, n := range Names() {
		ic := ByName(n)
		var l paint.List
		Draw(&l, ic, geom.R(0, 0, 16, 16), color.NRGBA{A: 255}, 0)
		if len(l.Ops) == 0 {
			t.Errorf("%s drew nothing", n)
		}
	}
	for _, n := range []string{"paper-clip", "picture", "cloud-upload", "swap"} {
		if ByName(n) == nil {
			t.Errorf("missing %s", n)
		}
	}
}

func TestNew(t *testing.T) {
	ic := New("diamond", "M12 3L21 12L12 21L3 12Z", "M11 11H13V13H11Z")
	if ByName("diamond") != nil {
		t.Fatal("New must not register")
	}
	var l paint.List
	Draw(&l, ic, geom.R(0, 0, 24, 24), color.NRGBA{A: 255}, 0)
	if n := len(paths(l)); n != 2 {
		t.Fatalf("want fill + stroke, got %d path fills in %d ops", n, len(l.Ops))
	}
	r := paint.NewCPU()
	if err := r.Render(&l, geom.Size{W: 24, H: 24}, 1); err != nil {
		t.Fatal(err)
	}
	if r.Image().RGBAAt(12, 12).A == 0 || r.Image().RGBAAt(12, 3).A == 0 || r.Image().RGBAAt(1, 1).A != 0 {
		t.Fatal("unexpected pixels")
	}
}

func TestDrawReusesPaths(t *testing.T) {
	ic := ByName(Names()[0])
	var a, b paint.List
	Draw(&a, ic, geom.Rect{X: 10, Y: 20, W: 16, H: 16}, color.NRGBA{A: 255}, 0)
	Draw(&b, ic, geom.Rect{X: 300, Y: 7.5, W: 16, H: 16}, color.NRGBA{A: 255}, 0)
	pa, pb := paths(a), paths(b)
	if len(pa) == 0 || len(pa) != len(pb) {
		t.Fatalf("paths %d vs %d", len(pa), len(pb))
	}
	for i := range pa {
		if pa[i] != pb[i] {
			t.Fatal("same icon and size at another position must reuse the same *paint.Path")
		}
	}
}

func paths(l paint.List) []*paint.Path {
	var out []*paint.Path
	for _, op := range l.Ops {
		if f, ok := op.(paint.FillPath); ok {
			out = append(out, f.Path)
		}
	}
	return out
}

func TestFilledStatusIconsHaveHoles(t *testing.T) {
	cases := []struct {
		ic         *Icon
		hole, body [2]int // pixels (48px render) inside the glyph / the disc
	}{
		{CheckFilled, [2]int{21, 31}, [2]int{24, 8}},
		{CloseFilled, [2]int{24, 24}, [2]int{24, 8}},
		{InfoFilled, [2]int{24, 28}, [2]int{12, 24}},
		{WarningFilled, [2]int{24, 20}, [2]int{12, 24}},
	}
	for _, tc := range cases {
		var l paint.List
		Draw(&l, tc.ic, geom.R(0, 0, 48, 48), color.NRGBA{A: 255}, 0)
		r := paint.NewCPU()
		if err := r.Render(&l, geom.Size{W: 48, H: 48}, 1); err != nil {
			t.Fatal(err)
		}
		img := r.Image()
		if a := img.RGBAAt(tc.hole[0], tc.hole[1]).A; a != 0 {
			t.Errorf("%s: glyph pixel %v alpha %d, want a hole", tc.ic.Name, tc.hole, a)
		}
		if a := img.RGBAAt(tc.body[0], tc.body[1]).A; a != 255 {
			t.Errorf("%s: disc pixel %v alpha %d, want filled", tc.ic.Name, tc.body, a)
		}
	}
}

func TestFromImageMask(t *testing.T) {
	src := image.NewAlpha(image.Rect(0, 0, 8, 4))
	src.SetAlpha(1, 1, color.Alpha{A: 255})
	ic := FromImage("m", src, true)
	var l paint.List
	red := color.NRGBA{R: 255, A: 255}
	Draw(&l, ic, geom.R(10, 10, 16, 16), red, 0)
	Draw(&l, ic, geom.R(40, 10, 16, 16), red, 0)
	if len(l.Ops) != 2 {
		t.Fatalf("ops %d", len(l.Ops))
	}
	a, b := l.Ops[0].(paint.Image), l.Ops[1].(paint.Image)
	if a.Img != b.Img {
		t.Error("same color must reuse the tinted image")
	}
	if a.Rect != geom.R(10, 14, 16, 8) {
		t.Errorf("aspect fit: %v", a.Rect)
	}
	if c := a.Img.(*image.NRGBA).NRGBAAt(1, 1); c != red {
		t.Errorf("tinted pixel %v", c)
	}
	if c := a.Img.(*image.NRGBA).NRGBAAt(0, 0); c.A != 0 {
		t.Errorf("transparent pixel %v", c)
	}
}

func TestStyleReplace(t *testing.T) {
	alt := New("alt", "M4 4H20", "")
	st := &Style{Replace: map[*Icon]*Icon{Search: alt}, StrokeWidth: 3}
	var names []string
	l := paint.List{IconStyle: st, IconNote: func(n string, _, _ bool, _ geom.Rect) { names = append(names, n) }}
	Draw(&l, Search, geom.R(0, 0, 24, 24), color.NRGBA{A: 255}, 0)
	want := alt.sized(1, 0, false, 3)
	if f := l.Ops[1].(paint.FillPath); f.Path != want[0] {
		t.Error("style must draw the replacement with its stroke width")
	}
	if len(names) != 1 || names[0] != "search" {
		t.Errorf("IconNote names %v, want the icon asked for", names)
	}
	if st.Resolve(Close) != Close || (*Style)(nil).Resolve(Close) != Close {
		t.Error("Resolve passthrough")
	}
}
