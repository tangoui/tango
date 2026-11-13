package paint

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestPathSkewed(t *testing.T) {
	p := NewPath().MoveTo(0, 0).LineTo(0, -10).LineTo(4, -10).Close()
	q := p.Skewed(0.2)
	if q.Segs[1].P[0] != (geom.Point{X: 2, Y: -10}) || q.Segs[0].P[0] != (geom.Point{}) {
		t.Fatalf("skew %+v", q.Segs)
	}
	if p.Segs[1].P[0].X != 0 {
		t.Fatal("original modified")
	}
}

type barSrc struct{}

func (barSrc) GlyphPath(id uint32, size float32) *Path {
	return NewPath().MoveTo(0, 0).LineTo(0, -size).LineTo(2, -size).LineTo(2, 0).Close()
}

func TestCPUDrawsSkewedGlyph(t *testing.T) {
	draw := func(skew float32) (top, bottom int) {
		var l List
		l.Glyphs(geom.Pt(10, 30), []Glyph{{Src: barSrc{}, ID: 1, Size: 20, Skew: skew}}, color.NRGBA{A: 255})
		c := NewCPU()
		if err := c.Render(&l, geom.Size{W: 40, H: 40}, 1); err != nil {
			t.Fatal(err)
		}
		img := c.Image()
		first := func(y int) int {
			for x := 0; x < 40; x++ {
				if img.RGBAAt(x, y).A > 128 && img.RGBAAt(x, y).R < 128 {
					return x
				}
			}
			return -1
		}
		return first(12), first(29)
	}
	t0, b0 := draw(0)
	t1, b1 := draw(0.2)
	if t0 != b0 || b1 != b0 || t1 <= b1+2 {
		t.Fatalf("upright %d/%d skewed %d/%d", t0, b0, t1, b1)
	}
}
