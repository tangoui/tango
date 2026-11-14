package shaders

import (
	"testing"

	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestPaintRRectCoverageMatchesShader: paint.CPU and the GPU shader share
// one rounded-rectangle coverage definition, so paint.RRectCoverage must
// equal RRectCoverage at every pixel center.
func TestPaintRRectCoverageMatchesShader(t *testing.T) {
	cases := []struct {
		r   geom.Rect
		rad geom.Radii
	}{
		{geom.R(4, 4, 40, 30), geom.Rad(6)},
		{geom.R(10.3, 6.7, 36.5, 24.25), geom.Radii{TL: 12, TR: 2, BR: 8, BL: 0}},
		{geom.R(2.5, 3.5, 70, 8), geom.Rad(20)},                // radii > half size
		{geom.R(1.25, 0.75, 9, 30), geom.Radii{TL: 40, BR: 3}}, // one huge radius
		{geom.R(5, 5, 0.6, 0.4), geom.Rad(1)},                  // sub-pixel
		{geom.R(15.75, 12.125, 61.5, 44.875), geom.Radii{TL: 0.5, TR: 7.3, BR: 15.1, BL: 22}},
		{geom.R(3, 3, 20, 20), geom.Radii{}}, // plain
		{geom.R(3, 3, 0, 20), geom.Rad(4)},   // empty
	}
	for _, c := range cases {
		n := 0
		for y := -2; y < 64; y++ {
			for x := -2; x < 84; x++ {
				// Pixel centers plus off-center points.
				for _, o := range [][2]float32{{0.5, 0.5}, {0.13, 0.77}, {0.91, 0.02}} {
					px, py := float32(x)+o[0], float32(y)+o[1]
					got := paint.RRectCoverage(px, py, c.r, c.rad)
					want := RRectCoverage(sl.V2(px, py), sl.V2(c.r.X, c.r.Y), sl.V2(c.r.W, c.r.H),
						sl.V4(c.rad.TL, c.rad.TR, c.rad.BR, c.rad.BL))
					if d := got - want; d > 1e-6 || d < -1e-6 {
						t.Fatalf("rect %v radii %v at (%v,%v): paint %v, shader %v", c.r, c.rad, px, py, got, want)
					}
					if got > 0 {
						n++
					}
				}
			}
		}
		if n == 0 && !c.r.Empty() {
			t.Fatalf("rect %v: no coverage at all", c.r)
		}
	}
}
