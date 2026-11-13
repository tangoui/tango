package paint

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// StrokeRect uses the shared StrokeCoverage definition (device units: the
// rect, radii, width and dash scaled), including the skipped interior of
// a large stroke.
func TestStrokeRectUsesStrokeCoverage(t *testing.T) {
	cases := []struct {
		r     geom.Rect
		rad   geom.Radii
		width float32
		dash  []float32
	}{
		{geom.R(3.3, 2.6, 30.5, 20.2), geom.Radii{TL: 8, TR: 2.5, BR: 12, BL: 0}, 1, nil},
		{geom.R(1, 1, 37, 27), geom.Rad(4), 2.5, []float32{4, 2}},
		{geom.R(2.5, 2.5, 30, 22), geom.Radii{TL: 10}, 1.5, []float32{1, 1, 3, 1, 9}}, // fifth entry dropped
		{geom.R(5, 5, 12, 8), geom.Rad(2), 6, nil},
	}
	for _, c := range cases {
		for _, s := range []float32{1, 1.5, 2} {
			l := &List{}
			l.StrokeDashed(c.r, c.rad, c.width, color.NRGBA{0, 0, 0, 255}, c.dash)
			img := render(t, l, 40, 30, s).Image()
			dr := geom.Rect{X: c.r.X * s, Y: c.r.Y * s, W: c.r.W * s, H: c.r.H * s}
			drad := geom.Radii{TL: c.rad.TL * s, TR: c.rad.TR * s, BR: c.rad.BR * s, BL: c.rad.BL * s}
			var dd []float32
			for i, v := range c.dash {
				if i < 4 {
					dd = append(dd, v*s)
				}
			}
			for y := 0; y < img.Rect.Dy(); y++ {
				for x := 0; x < img.Rect.Dx(); x++ {
					want := covByte(StrokeCoverage(float32(x)+0.5, float32(y)+0.5, dr, drad, c.width*s, dd, 0))
					if got := int(img.RGBAAt(x, y).A); !near(got, want, 1) {
						t.Fatalf("%v scale %v (%d,%d): alpha %d, want %d", c.r, s, x, y, got, want)
					}
				}
			}
		}
	}
}

// The coverage-based StrokeRect looks like the path-based outline it
// replaced (strokeRectPath, still used under non-uniform transforms): the
// same pixels inked up to anti-aliasing at rounded corners and dash ends.
func TestStrokeRectMatchesPathOutline(t *testing.T) {
	cases := []StrokeRect{
		{Rect: geom.R(3.3, 2.6, 30.5, 20.2), Radii: geom.Radii{TL: 8, TR: 2.5, BR: 12}, Width: 1},
		{Rect: geom.R(2, 2, 34, 24), Radii: geom.Rad(6), Width: 3},
		{Rect: geom.R(5, 5, 12, 8), Radii: geom.Rad(2), Width: 6},
		{Rect: geom.R(1, 1, 37, 27), Radii: geom.Rad(4), Width: 1, Dash: []float32{4, 2}},
		{Rect: geom.R(2.5, 2.5, 34, 22), Radii: geom.Radii{TL: 10}, Width: 2, Dash: []float32{1, 1, 3, 1}},
	}
	for _, o := range cases {
		o.Color = color.NRGBA{0, 0, 0, 255}
		for _, s := range []float32{1, 2} {
			l := &List{}
			l.Add(o)
			got := render(t, l, 40, 30, s).Image()
			ref := NewCPU()
			ref.BeginFrame(s)
			ref.Render(&List{}, geom.Size{W: 40, H: 30}, s)
			ref.fill(strokeRectPath(o), o.Color)
			want := ref.Image()
			var sumGot, sumWant, maxD int
			for i := 3; i < len(got.Pix); i += 4 {
				a, b := int(got.Pix[i]), int(want.Pix[i])
				sumGot, sumWant = sumGot+a, sumWant+b
				maxD = max(maxD, a-b, b-a)
			}
			t.Logf("%v width %v dash %v scale %v: ink %d vs %d, max diff %d", o.Rect, o.Width, o.Dash, s, sumGot, sumWant, maxD)
			// RRectCoverage's corner formula (Clamp(0.5-sdf)) inks a thin
			// stroke's rounded corners a little more than exact area.
			if d := sumGot - sumWant; d*20 > sumWant || -d*20 > sumWant {
				t.Errorf("%v dash %v scale %v: total ink %d, path outline %d (more than 5%% apart)", o.Rect, o.Dash, s, sumGot, sumWant)
			}
			lim := 64
			if len(o.Dash) > 0 {
				lim = 255 // dash ends at corners: radial (arc) vs butt cap after a round join
			}
			if maxD > lim {
				t.Errorf("%v dash %v scale %v: max alpha difference %d", o.Rect, o.Dash, s, maxD)
			}
		}
	}
}
