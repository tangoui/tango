package paint

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// StrokeCoverage returns how much of the pixel centered at (px, py) a
// StrokeRect border covers, in 0..1: a border of the given width inside the
// rounded rectangle r (corner radii radii), dashed with dash starting at
// dashPhase. All values are in the same (device) units.
//
// It is the single stroke definition shared by paint.CPU and the GPU quad
// shader (internal/render/gpu/shaders.StrokeCoverage computes exactly the same float32
// formula; a test keeps them equal):
//
//   - The ring is RRectCoverage(r, radii) minus the RRectCoverage of r
//     inset by width with every radius reduced by width (not below 0),
//     clamped to 0..1 — the border StrokeRect always drew (CSS
//     border-box). A width of at least half the shorter side fills r; a
//     width <= 0 (or NaN) or an empty r draws nothing.
//   - The dash is evaluated at the pixel's arc length along the stroke
//     centerline (r inset by half the width, radii reduced by as much and
//     clamped to it) plus dashPhase. Arc length 0 is where the top-left
//     corner ends on the top edge, and it grows clockwise; on a rounded
//     corner it is measured along the centerline's circular arc. Pixels map
//     to a corner's arc beyond its center along both axes, elsewhere to the
//     nearer edge. Dash ends are anti-aliased over one pixel of arc length.
//   - dash holds alternating on/off lengths. Only the first 4 entries are
//     used (the GPU carries 4; longer patterns are truncated), and the
//     first entry <= 0 (or NaN) ends the pattern. An empty pattern is solid.
//     An odd count repeats the pattern twice as in SVG ({a} is on a, off
//     a). A period shorter than one pixel draws its average coverage.
//
// Where the outline closes (the top-left corner) the pattern restarts, as a
// dash along a path does.
func StrokeCoverage(px, py float32, r geom.Rect, radii geom.Radii, width float32, dash []float32, dashPhase float32) float32 {
	var d [4]float32
	copy(d[:], dash)
	return strokeCoverage(px, py, r, radii, width, d, dashPhase)
}

// strokeCoverage is StrokeCoverage with the pattern in a fixed array (the
// shader's Vec4).
func strokeCoverage(px, py float32, r geom.Rect, radii geom.Radii, width float32, d [4]float32, dashPhase float32) float32 {
	if !(width > 0) || r.W <= 0 || r.H <= 0 {
		return 0
	}
	outer := RRectCoverage(px, py, r, radii)
	inner := RRectCoverage(px, py, geom.Rect{X: r.X + width, Y: r.Y + width, W: r.W - 2*width, H: r.H - 2*width},
		geom.Radii{TL: max32(radii.TL-width, 0), TR: max32(radii.TR-width, 0), BR: max32(radii.BR-width, 0), BL: max32(radii.BL-width, 0)})
	ring := clamp32(outer-inner, 0, 1)
	if ring <= 0 || !(d[0] > 0) {
		return ring
	}
	s := strokeArcLength(px, py, r, radii, width)
	return ring * strokeDash(s+dashPhase, d)
}

// halfPi is π/2 as float32 (the shader's constant).
const halfPi = 1.5707964

// strokeArcLength is the arc length position of (px, py) along the stroke
// centerline; see StrokeCoverage. It mirrors the shader operation by
// operation.
func strokeArcLength(px, py float32, r geom.Rect, radii geom.Radii, width float32) float32 {
	halfX, halfY := r.W*0.5, r.H*0.5
	hw := min32(width*0.5, min32(halfX, halfY))
	x0 := r.X + hw
	y0 := r.Y + hw
	cw := r.W - 2*hw
	ch := r.H - 2*hw
	x1 := x0 + cw
	y1 := y0 + ch
	m := min32(cw, ch) * 0.5
	cTL := clamp32(radii.TL-hw, 0, m)
	cTR := clamp32(radii.TR-hw, 0, m)
	cBR := clamp32(radii.BR-hw, 0, m)
	cBL := clamp32(radii.BL-hw, 0, m)
	aTR := cw - cTL - cTR
	eR := aTR + halfPi*cTR
	aBR := eR + ch - cTR - cBR
	eB := aBR + halfPi*cBR
	aBL := eB + cw - cBR - cBL
	eL := aBL + halfPi*cBL
	aTL := eL + ch - cBL - cTL
	top := px - (x0 + cTL)
	right := eR + (py - (y0 + cTR))
	bottom := eB + ((x1 - cBR) - px)
	left := eL + ((y1 - cBL) - py)
	dTop := py - r.Y
	dRight := r.X + r.W - px
	dBottom := r.Y + r.H - py
	dLeft := px - r.X
	dx := px - (r.X + halfX)
	dy := py - (r.Y + halfY)
	if dx < 0 && dy < 0 {
		ax := x0 + cTL
		ay := y0 + cTL
		if px <= ax && py <= ay {
			return aTL + cTL*quarterAngle(ay-py, ax-px)
		}
		if dTop <= dLeft {
			return top
		}
		return left
	}
	if dy < 0 {
		ax := x1 - cTR
		ay := y0 + cTR
		if px >= ax && py <= ay {
			return aTR + cTR*quarterAngle(px-ax, ay-py)
		}
		if dTop <= dRight {
			return top
		}
		return right
	}
	if dx >= 0 {
		ax := x1 - cBR
		ay := y1 - cBR
		if px >= ax && py >= ay {
			return aBR + cBR*quarterAngle(py-ay, px-ax)
		}
		if dRight <= dBottom {
			return right
		}
		return bottom
	}
	ax := x0 + cBL
	ay := y1 - cBL
	if px <= ax && py >= ay {
		return aBL + cBL*quarterAngle(ax-px, py-ay)
	}
	if dBottom <= dLeft {
		return bottom
	}
	return left
}

// quarterAngle is atan2(y, x) for x, y >= 0 by the shader's polynomial
// (error below 1e-5 rad); 0 when both are 0.
func quarterAngle(y, x float32) float32 {
	lo := min32(x, y)
	hi := max32(x, y)
	if !(hi > 0) {
		return 0
	}
	z := lo / hi
	z2 := z * z
	a := z * (0.99997726 + z2*(-0.33262347+z2*(0.19354346+z2*(-0.11643287+z2*(0.05265332-z2*0.0117212)))))
	if y > x {
		a = halfPi - a
	}
	return a
}

// strokeDash is the dash pattern's coverage of the unit window around arc
// length s; see StrokeCoverage.
func strokeDash(s float32, d [4]float32) float32 {
	if !(d[0] > 0) {
		return 1
	}
	l1 := d[0]
	o2 := float32(0)
	l2 := float32(0)
	o3 := float32(0)
	l3 := float32(0)
	period := 2 * d[0]
	if d[1] > 0 {
		period = d[0] + d[1]
		if d[2] > 0 {
			o2 = d[0] + d[1]
			l2 = d[2]
			if d[3] > 0 {
				period = d[0] + d[1] + d[2] + d[3]
			} else {
				period = 2 * (d[0] + d[1] + d[2])
				o3 = 2*d[0] + d[1] + d[2]
				l3 = d[1]
			}
		}
	}
	if period < 1 {
		return clamp32((l1+l2+l3)/period, 0, 1)
	}
	u := s - period*floor32(s/period)
	lo := u - 0.5
	hi := u + 0.5
	c := dashRun(lo, hi, 0, l1, period) + dashRun(lo, hi, o2, l2, period) + dashRun(lo, hi, o3, l3, period)
	return clamp32(c, 0, 1)
}

// dashRun is how much of [lo, hi] the on run [o, o+l) and its copies one
// period either side cover.
func dashRun(lo, hi, o, l, period float32) float32 {
	a := max32(min32(hi, o-period+l)-max32(lo, o-period), 0)
	b := max32(min32(hi, o+l)-max32(lo, o), 0)
	c := max32(min32(hi, o+period+l)-max32(lo, o+period), 0)
	return a + b + c
}

func floor32(x float32) float32 { return float32(math.Floor(float64(x))) }
