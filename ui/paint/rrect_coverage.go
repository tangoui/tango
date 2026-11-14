package paint

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// RRectCoverage returns how much of the pixel centered at (px, py) the
// rounded rectangle r with corner radii radii covers, in 0..1. All values
// are in the same (device) units.
//
// It is the single coverage definition shared by paint.CPU and the GPU
// quad shader (internal/render/gpu/shaders.RRectCoverage computes exactly the same
// float32 formula; a test keeps them equal):
//
//   - Away from rounded corners it is the exact area overlap of the unit
//     pixel square with the rectangle (covX*covY).
//   - When the pixel center lies in a rounded corner's quadrant (within r of
//     both edges meeting there, r > 0) it is Clamp(0.5 - sdf, 0, 1), sdf
//     being the signed distance to the rounded boundary (negative inside),
//     capped by the box overlap.
//
// Radii are clamped to half the shorter side (as geom.Radii.Clamp does); an
// empty rectangle covers nothing.
func RRectCoverage(px, py float32, r geom.Rect, radii geom.Radii) float32 {
	if r.W <= 0 || r.H <= 0 {
		return 0
	}
	hiX, hiY := r.X+r.W, r.Y+r.H
	covX := clamp32(min32(px+0.5, hiX)-max32(px-0.5, r.X), 0, 1)
	covY := clamp32(min32(py+0.5, hiY)-max32(py-0.5, r.Y), 0, 1)
	box := covX * covY
	halfX, halfY := r.W*0.5, r.H*0.5
	dx, dy := px-(r.X+halfX), py-(r.Y+halfY)
	rad := radii.BL
	if dx < 0 && dy < 0 {
		rad = radii.TL
	} else if dy < 0 {
		rad = radii.TR
	} else if dx >= 0 {
		rad = radii.BR
	}
	rad = clamp32(rad, 0, min32(halfX, halfY))
	qx, qy := abs32(dx)-halfX+rad, abs32(dy)-halfY+rad
	if rad <= 0 || qx <= 0 || qy <= 0 {
		return box
	}
	mx, my := max32(qx, 0), max32(qy, 0)
	sdf := float32(math.Sqrt(float64(mx*mx+my*my))) + min32(max32(qx, qy), 0) - rad
	return min32(clamp32(0.5-sdf, 0, 1), box)
}

// The helpers mirror package sl's semantics (including NaN handling) so
// RRectCoverage stays bit-compatible with the shader on the CPU.

func min32(x, y float32) float32 {
	if y < x {
		return y
	}
	return x
}

func max32(x, y float32) float32 {
	if x < y {
		return y
	}
	return x
}

func clamp32(x, lo, hi float32) float32 { return min32(max32(x, lo), hi) }

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
