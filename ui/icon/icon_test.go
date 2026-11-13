package icon

import (
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
