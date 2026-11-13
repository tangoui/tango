package paint

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

var (
	white = color.NRGBA{255, 255, 255, 255}
	blue  = color.NRGBA{0x16, 0x77, 0xff, 255}
)

func render(t *testing.T, l *List, w, h, scale float32) *CPU {
	t.Helper()
	r := NewCPU()
	if err := r.Render(l, geom.Size{W: w, H: h}, scale); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFillRectAndScale(t *testing.T) {
	l := &List{Background: white}
	l.Fill(geom.R(10, 10, 20, 10), geom.Radii{}, blue)
	r := render(t, l, 50, 40, 2)
	img := r.Image()
	if img.Rect.Dx() != 100 || img.Rect.Dy() != 80 {
		t.Fatalf("size %v", img.Rect)
	}
	if c := img.RGBAAt(40, 30); c.B != 0xff || c.R != 0x16 {
		t.Fatalf("inside = %v", c)
	}
	if c := img.RGBAAt(5, 5); c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("outside = %v", c)
	}
}

func TestRoundedCornerIsAntialiased(t *testing.T) {
	l := &List{Background: white}
	l.Fill(geom.R(0, 0, 40, 40), geom.Rad(10), blue)
	img := render(t, l, 40, 40, 1).Image()
	if c := img.RGBAAt(0, 0); c.R != 255 {
		t.Fatalf("corner should be background, got %v", c)
	}
	if c := img.RGBAAt(20, 20); c.R != 0x16 {
		t.Fatalf("center %v", c)
	}
	// Somewhere along the arc a partially covered pixel exists.
	partial := false
	for i := 0; i < 10; i++ {
		c := img.RGBAAt(i, 3)
		if c.R > 0x16 && c.R < 255 {
			partial = true
		}
	}
	if !partial {
		t.Fatal("no antialiasing on rounded corner")
	}
}

func TestStrokeLeavesInteriorEmpty(t *testing.T) {
	l := &List{Background: white}
	l.Stroke(geom.R(0, 0, 30, 30), geom.Rad(4), 1, blue)
	img := render(t, l, 30, 30, 1).Image()
	if c := img.RGBAAt(15, 15); c.R != 255 {
		t.Fatalf("interior painted %v", c)
	}
	if c := img.RGBAAt(15, 0); c.R == 255 {
		t.Fatalf("border missing %v", c)
	}
}

func TestShadowBlursOutside(t *testing.T) {
	l := &List{Background: white}
	l.Shadows(geom.R(20, 20, 40, 40), geom.Rad(6), []Shadow{{Y: 4, Blur: 12, Color: color.NRGBA{0, 0, 0, 80}}})
	img := render(t, l, 80, 90, 1).Image()
	below := img.RGBAAt(40, 66)
	far := img.RGBAAt(2, 2)
	if below.R >= 255 || far.R != 255 {
		t.Fatalf("shadow below=%v far=%v", below, far)
	}
}

func TestClipAndOpacity(t *testing.T) {
	l := &List{Background: white}
	l.Clip(geom.R(0, 0, 10, 10), geom.Radii{})
	l.Fill(geom.R(0, 0, 20, 20), geom.Radii{}, blue)
	l.Pop()
	l.Opacity(0.5)
	l.Fill(geom.R(20, 0, 10, 10), geom.Radii{}, color.NRGBA{0, 0, 0, 255})
	l.Pop()
	img := render(t, l, 40, 20, 1).Image()
	if img.RGBAAt(15, 5).R != 255 {
		t.Fatal("clip leaked")
	}
	if g := img.RGBAAt(25, 5).R; g < 120 || g > 135 {
		t.Fatalf("opacity blend = %d", g)
	}
}

func TestSVGPathAndStroke(t *testing.T) {
	p, err := ParseSVGPath("M2 2h10v10H2zM20 7a5 5 0 1 0 10 0a5 5 0 1 0 -10 0")
	if err != nil {
		t.Fatal(err)
	}
	b := p.Bounds()
	if b.X != 2 || b.Right() < 29 || b.Bottom() < 11 {
		t.Fatalf("bounds %+v", b)
	}
	if _, err := ParseSVGPath("X1"); err == nil {
		t.Fatal("expected error")
	}
	st := StrokeToFill(NewPath().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10), StrokeStyle{Width: 2, Cap: CapRound})
	if len(st.Segs) == 0 {
		t.Fatal("empty stroke")
	}
	l := &List{Background: white}
	l.StrokePath(NewPath().MoveTo(2, 5).LineTo(28, 5), StrokeStyle{Width: 2}, blue)
	img := render(t, l, 30, 10, 1).Image()
	if img.RGBAAt(15, 5).R == 255 || img.RGBAAt(15, 8).R != 255 {
		t.Fatal("stroked line")
	}
}
