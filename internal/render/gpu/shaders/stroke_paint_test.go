package shaders

import (
	"testing"

	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestPaintStrokeCoverageMatchesShader: paint.CPU and the GPU shader share
// one stroke coverage definition (ring and dash pattern), so
// paint.StrokeCoverage must equal StrokeCoverage at every sample point.
func TestPaintStrokeCoverageMatchesShader(t *testing.T) {
	cases := []struct {
		r     geom.Rect
		rad   geom.Radii
		width float32
		dash  []float32
		phase float32
	}{
		{geom.R(4, 4, 40, 30), geom.Rad(6), 1, nil, 0},
		{geom.R(10.3, 6.7, 36.5, 24.25), geom.Radii{TL: 12, TR: 2, BR: 8, BL: 0}, 2.5, []float32{4, 2}, 0},
		{geom.R(2.5, 3.5, 30, 18), geom.Rad(5), 12, nil, 0}, // width > half size
		{geom.R(1.25, 0.75, 60, 40), geom.Radii{TL: 20, BR: 3}, 1.5, []float32{1, 1, 3, 1}, 0},
		{geom.R(15.75, 12.125, 61.5, 44.875), geom.Radii{TL: 0.5, TR: 7.3, BR: 15.1, BL: 22}, 3, []float32{4, 2}, 1.5},
		{geom.R(3, 3, 50, 20), geom.Radii{}, 0.6, []float32{3}, 1.5},    // odd dash, sub-pixel width
		{geom.R(5, 5, 30, 30), geom.Rad(8), 2, []float32{2, 1, 0.5}, 0}, // odd count: pattern doubles
		{geom.R(5, 5, 30, 30), geom.Rad(4), 1, []float32{0.2, 0.1}, 0},  // period below a pixel
		{geom.R(3, 3, 0, 20), geom.Rad(4), 2, nil, 0},                   // empty
		{geom.R(3, 3, 20, 20), geom.Rad(4), 0, nil, 0},                  // zero width
	}
	for _, c := range cases {
		var d sl.Vec4
		for i, v := range c.dash {
			switch i {
			case 0:
				d.X = v
			case 1:
				d.Y = v
			case 2:
				d.Z = v
			case 3:
				d.W = v
			}
		}
		n := 0
		for y := -2; y < 64; y++ {
			for x := -2; x < 84; x++ {
				for _, o := range [][2]float32{{0.5, 0.5}, {0.13, 0.77}, {0.91, 0.02}} {
					px, py := float32(x)+o[0], float32(y)+o[1]
					got := paint.StrokeCoverage(px, py, c.r, c.rad, c.width, c.dash, c.phase)
					want := StrokeCoverage(sl.V2(px, py), sl.V2(c.r.X, c.r.Y), sl.V2(c.r.W, c.r.H),
						sl.V4(c.rad.TL, c.rad.TR, c.rad.BR, c.rad.BL), c.width, d, c.phase)
					if dd := got - want; dd > 1e-6 || dd < -1e-6 {
						t.Fatalf("rect %v radii %v width %v dash %v phase %v at (%v,%v): paint %v, shader %v",
							c.r, c.rad, c.width, c.dash, c.phase, px, py, got, want)
					}
					if got < 0 || got > 1 {
						t.Fatalf("coverage %v out of range at (%v,%v)", got, px, py)
					}
					if got > 0 {
						n++
					}
				}
			}
		}
		if (n == 0) != (c.r.Empty() || c.width <= 0) {
			t.Fatalf("rect %v width %v: %d covered samples", c.r, c.width, n)
		}
	}
}

// TestStrokeCoverageDash checks the dash pattern's placement: arc length
// from the top-left corner's end on the top edge, clockwise, on the stroke
// centerline.
func TestStrokeCoverageDash(t *testing.T) {
	o, size := sl.V2(10, 10), sl.V2(40, 20)
	cov := func(x, y float32, dash sl.Vec4, phase float32) float32 {
		return StrokeCoverage(sl.V2(x, y), o, size, sl.V4(0, 0, 0, 0), 2, dash, phase)
	}
	// Square corners, width 2: the centerline starts at (11, 11). Dash
	// {4,2}: on for s in [0,4), off in [4,6), on in [6,10) ...
	on, off := float32(1), float32(0)
	for _, c := range []struct {
		x, y  float32
		phase float32
		want  float32
	}{
		{13.5, 10.5, 0, on},  // s = 2.5
		{16.5, 10.5, 0, off}, // s = 5.5
		{18.5, 10.5, 0, on},  // s = 7.5
		{15.0, 10.5, 0, 0.5}, // s = 4: half the pixel on the dash end
		{16.5, 10.5, 2, on},  // phase shifts the pattern: s+2 = 7.5
		// Right edge: the centerline is x = 49 from (49, 11); s = 38 + (y-11).
		// s = 38 + 3.5 = 41.5 → 41.5 mod 6 = 5.5: off.
		{49.5, 14.5, 0, off},
		{49.5, 16.5, 0, on}, // s = 43.5 → 1.5
	} {
		if got := cov(c.x, c.y, sl.V4(4, 2, 0, 0), c.phase); sl.Abs(got-c.want) > 1e-4 {
			t.Errorf("(%v,%v) phase %v: coverage %v, want %v", c.x, c.y, c.phase, got, c.want)
		}
	}
	// No dash: solid ring.
	if got := cov(16.5, 10.5, sl.Vec4{}, 0); got != 1 {
		t.Errorf("solid: %v", got)
	}
	// Inside the ring: nothing.
	if got := cov(30, 20, sl.V4(4, 2, 0, 0), 0); got != 0 {
		t.Errorf("interior: %v", got)
	}
	// Rounded corner: the arc's arc length counts. Radius 6, width 2:
	// centerline radius 5, the top edge runs s in [0, 28), the arc in
	// [28, 28+5π/2). The arc's midpoint (45°) is s = 28 + 5π/4 ≈ 31.93.
	rad := sl.V4(6, 6, 6, 6)
	ax, ay := float32(50-6), float32(10+6)
	mx, my := ax+5*0.70710677, ay-5*0.70710677
	// dash {31.93-1, 2}: the midpoint lies 1 into the 2-long gap... use a
	// pattern whose on-run ends exactly at the midpoint.
	m := float32(28 + 5*3.14159265/4)
	if got := StrokeCoverage(sl.V2(mx, my), o, size, rad, 2, sl.V4(m-1.5, 3, 0, 0), 0); got > 1e-3 {
		t.Errorf("arc midpoint inside the gap: coverage %v", got)
	}
	if got := StrokeCoverage(sl.V2(mx, my), o, size, rad, 2, sl.V4(m+1.5, 3, 0, 0), 0); got < 0.999 {
		t.Errorf("arc midpoint inside the dash: coverage %v", got)
	}
}
