package shaders

import "github.com/tangoui/tango/internal/render/shader/sl"

// StrokeCoverage returns the coverage of the pixel centered at p by a
// border of the given width drawn inside the rectangle at origin with the
// given size and corner radii (top-left, top-right, bottom-right,
// bottom-left), optionally dashed. It is paint.StrokeCoverage's formula
// (a test keeps them equal):
//
//   - The ring is RRectCoverage of the rectangle minus RRectCoverage of the
//     rectangle inset by width with radii reduced by width (not below 0),
//     clamped to 0..1. A width of at least half the shorter side fills the
//     rectangle; a width <= 0 (or NaN) or an empty rectangle draws nothing.
//   - The dash (strokeDash) is evaluated at the pixel's arc length along the
//     stroke centerline plus dashPhase. Arc length starts where the top-left
//     corner ends on the top edge and runs clockwise; on a rounded corner it
//     is measured along the centerline's circular arc.
func StrokeCoverage(p, origin, size sl.Vec2, radii sl.Vec4, width float32, dash sl.Vec4, dashPhase float32) float32 {
	if !(width > 0) || size.X <= 0 || size.Y <= 0 {
		return 0
	}
	outer := RRectCoverage(p, origin, size, radii)
	inner := RRectCoverage(p, origin.Add(sl.V2(width, width)), size.Sub(sl.V2(2*width, 2*width)),
		sl.V4(sl.Max(radii.X-width, 0), sl.Max(radii.Y-width, 0), sl.Max(radii.Z-width, 0), sl.Max(radii.W-width, 0)))
	ring := sl.Clamp(outer-inner, 0, 1)
	if ring <= 0 || !(dash.X > 0) {
		return ring
	}
	s := strokeArcLength(p, origin, size, radii, width)
	return ring * strokeDash(s+dashPhase, dash)
}

// halfPi is π/2, the arc length of a unit-radius quarter circle.
const halfPi = 1.5707964

// strokeArcLength returns the arc length position of p along the
// centerline of a stroke of the given width inside the rounded rectangle:
// the rectangle inset by half the width (at most half the shorter side),
// radii reduced by as much (not below 0) and clamped to the inset
// rectangle. Zero is where the top-left corner's arc ends on the top edge;
// the outline runs clockwise. p maps to the corner arc when it lies beyond
// the arc's center along both axes in its corner's quadrant (the quadrants
// as in RRectCoverage), else to the nearer of the quadrant's two edges.
func strokeArcLength(p, origin, size sl.Vec2, radii sl.Vec4, width float32) float32 {
	half := size.Scale(0.5)
	hw := sl.Min(width*0.5, sl.Min(half.X, half.Y))
	x0 := origin.X + hw
	y0 := origin.Y + hw
	cw := size.X - 2*hw
	ch := size.Y - 2*hw
	x1 := x0 + cw
	y1 := y0 + ch
	m := sl.Min(cw, ch) * 0.5
	cTL := sl.Clamp(radii.X-hw, 0, m)
	cTR := sl.Clamp(radii.Y-hw, 0, m)
	cBR := sl.Clamp(radii.Z-hw, 0, m)
	cBL := sl.Clamp(radii.W-hw, 0, m)
	// Where each piece starts: top edge at 0, then the top-right arc, right
	// edge, bottom-right arc, bottom edge, bottom-left arc, left edge and
	// top-left arc.
	aTR := cw - cTL - cTR
	eR := aTR + halfPi*cTR
	aBR := eR + ch - cTR - cBR
	eB := aBR + halfPi*cBR
	aBL := eB + cw - cBR - cBL
	eL := aBL + halfPi*cBL
	aTL := eL + ch - cBL - cTL
	// Edge positions.
	top := p.X - (x0 + cTL)
	right := eR + (p.Y - (y0 + cTR))
	bottom := eB + ((x1 - cBR) - p.X)
	left := eL + ((y1 - cBL) - p.Y)
	// Distances to the outer edges, to pick the nearer edge.
	dTop := p.Y - origin.Y
	dRight := origin.X + size.X - p.X
	dBottom := origin.Y + size.Y - p.Y
	dLeft := p.X - origin.X
	dx := p.X - (origin.X + half.X)
	dy := p.Y - (origin.Y + half.Y)
	if dx < 0 && dy < 0 { // top-left
		ax := x0 + cTL
		ay := y0 + cTL
		if p.X <= ax && p.Y <= ay {
			return aTL + cTL*quarterAngle(ay-p.Y, ax-p.X)
		}
		if dTop <= dLeft {
			return top
		}
		return left
	}
	if dy < 0 { // top-right
		ax := x1 - cTR
		ay := y0 + cTR
		if p.X >= ax && p.Y <= ay {
			return aTR + cTR*quarterAngle(p.X-ax, ay-p.Y)
		}
		if dTop <= dRight {
			return top
		}
		return right
	}
	if dx >= 0 { // bottom-right
		ax := x1 - cBR
		ay := y1 - cBR
		if p.X >= ax && p.Y >= ay {
			return aBR + cBR*quarterAngle(p.Y-ay, p.X-ax)
		}
		if dRight <= dBottom {
			return right
		}
		return bottom
	}
	// bottom-left
	ax := x0 + cBL
	ay := y1 - cBL
	if p.X <= ax && p.Y >= ay {
		return aBL + cBL*quarterAngle(ax-p.X, p.Y-ay)
	}
	if dBottom <= dLeft {
		return bottom
	}
	return left
}

// quarterAngle returns atan2(y, x) for x, y >= 0, in 0..π/2, with a
// polynomial for atan on 0..1 (error below 1e-5 rad) so that every backend
// computes it with the same arithmetic. It is 0 when both are 0.
func quarterAngle(y, x float32) float32 {
	lo := sl.Min(x, y)
	hi := sl.Max(x, y)
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

// strokeDash returns how much of the unit window [s-0.5, s+0.5] along the
// outline the dash pattern d covers. d holds up to 4 alternating on/off
// lengths; the first entry <= 0 (or NaN) ends it, and an empty pattern is
// solid (1). An odd count repeats the pattern twice (as SVG does), so its
// on and off roles alternate: {a} is on a, off a; {a, b, c} is on a, off b,
// on c, off a, on b, off c. The pattern repeats every period; a period
// shorter than one pixel gives its average coverage.
func strokeDash(s float32, d sl.Vec4) float32 {
	if !(d.X > 0) {
		return 1
	}
	// On runs: (start, length) ×3 within one period.
	l1 := d.X
	o2 := float32(0)
	l2 := float32(0)
	o3 := float32(0)
	l3 := float32(0)
	period := 2 * d.X
	if d.Y > 0 {
		period = d.X + d.Y
		if d.Z > 0 {
			o2 = d.X + d.Y
			l2 = d.Z
			if d.W > 0 {
				period = d.X + d.Y + d.Z + d.W
			} else {
				period = 2 * (d.X + d.Y + d.Z)
				o3 = 2*d.X + d.Y + d.Z
				l3 = d.Y
			}
		}
	}
	if period < 1 {
		return sl.Clamp((l1+l2+l3)/period, 0, 1)
	}
	u := s - period*sl.Floor(s/period)
	lo := u - 0.5
	hi := u + 0.5
	c := dashRun(lo, hi, 0, l1, period) + dashRun(lo, hi, o2, l2, period) + dashRun(lo, hi, o3, l3, period)
	return sl.Clamp(c, 0, 1)
}

// dashRun returns how much of [lo, hi] (within a period of the pattern
// position, plus half a pixel either side) the on run [o, o+l) covers,
// counting its copies one period before and after.
func dashRun(lo, hi, o, l, period float32) float32 {
	a := sl.Max(sl.Min(hi, o-period+l)-sl.Max(lo, o-period), 0)
	b := sl.Max(sl.Min(hi, o+l)-sl.Max(lo, o), 0)
	c := sl.Max(sl.Min(hi, o+period+l)-sl.Max(lo, o+period), 0)
	return a + b + c
}
