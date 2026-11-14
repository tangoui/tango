package paint

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// ShadowCoverage returns how much of the pixel centered at (px, py) a box
// shadow covers, in 0..1: the rounded rectangle r (corner radii radii)
// blurred by a Gaussian of standard deviation sigma. All values are in the
// same (device) units; ShadowRect uses sigma = Blur*scale/2, with Spread
// already applied to r and radii.
//
// It is the single shadow definition shared by paint.CPU and the GPU quad
// shader (internal/render/gpu/shaders.ShadowCoverage computes exactly the same float32
// formula; a test keeps them equal), a closed-form approximation of the
// true Gaussian blur (within 0.02 of it for sigma >= 0.25):
//
//   - The pixel's box filter is folded into the blur as a Gaussian of
//     variance 1/12: s = sqrt(sigma² + 1/12), so sigma 0 is an anti-aliased
//     rounded rectangle (a negative or NaN sigma counts as 0).
//   - The square-cornered rectangle's blurred coverage is exact and
//     separable: the product over both axes of the Gaussian mass between
//     the two edges, 0.5·(erf((p-lo)/(s√2)) - erf((p-hi)/(s√2))).
//   - Each rounded corner (radius clamped to half the shorter side, as in
//     RRectCoverage) subtracts the Gaussian mass of the region it cuts off
//     the square corner: the corner's rows within 3s of the pixel are cut
//     into 4 strips, each strip's mass along the rows is exact and
//     multiplies the mass across the row between the edge and the arc at
//     the strip's middle. The region is symmetric about the corner's
//     diagonal, so the strips run along whichever edge keeps them off the
//     arc's steep end.
//
// erf is Abramowitz–Stegun 7.1.26 (sl.Erf; absolute error ≤ 1.5e-7).
// An empty r covers nothing.
func ShadowCoverage(px, py float32, r geom.Rect, radii geom.Radii, sigma float32) float32 {
	if !(r.W > 0) || !(r.H > 0) {
		return 0
	}
	sh := newShadowAxes(sigma)
	cx := sh.axis(px, r.X, r.X+r.W)
	cy := sh.axis(py, r.Y, r.Y+r.H)
	return sh.combine(cx, cy, px, py, r, radii)
}

// shadowAxes holds ShadowCoverage's per-sigma constants: k = 1/(s√2) and
// reach = 3s.
type shadowAxes struct{ k, reach float32 }

func newShadowAxes(sigma float32) shadowAxes {
	sg := max32(0, sigma)
	s := sqrt32(sg*sg + 0.083333336)
	return shadowAxes{k: 0.70710677 / s, reach: 3 * s}
}

// axis is the square rectangle's blurred coverage along one axis at p for
// the edges lo, hi (lo + size).
func (sh shadowAxes) axis(p, lo, hi float32) float32 {
	return 0.5 * (erf32((p-lo)*sh.k) - erf32((p-hi)*sh.k))
}

// combine is ShadowCoverage from the two axis coverages: the corners' cut
// subtracted from cx*cy, clamped.
func (sh shadowAxes) combine(cx, cy, px, py float32, r geom.Rect, radii geom.Radii) float32 {
	hiX := r.X + r.W
	hiY := r.Y + r.H
	m := min32(r.W, r.H) * 0.5
	l := px - r.X
	t := py - r.Y
	rt := hiX - px
	b := hiY - py
	cut := shadowCorner(l, t, clamp32(radii.TL, 0, m), sh.k, sh.reach) +
		shadowCorner(rt, t, clamp32(radii.TR, 0, m), sh.k, sh.reach) +
		shadowCorner(rt, b, clamp32(radii.BR, 0, m), sh.k, sh.reach) +
		shadowCorner(l, b, clamp32(radii.BL, 0, m), sh.k, sh.reach)
	return clamp32(cx*cy-cut, 0, 1)
}

// shadowCorner is the Gaussian mass of the region a corner of radius r
// cuts off the square corner, for a pixel u, v from the corner along its
// edges; see ShadowCoverage. It mirrors the shader operation by operation.
func shadowCorner(u, v, r, k, reach float32) float32 {
	if !(r > 0) || u > r+reach || v > r+reach || u < -reach || v < -reach {
		return 0
	}
	a := min32(u, v)
	c := max32(u, v)
	y0 := max32(0, c-reach)
	h := (min32(r, c+reach) - y0) * 0.25
	if !(h > 0) {
		return 0
	}
	ea := erf32(a * k)
	e0 := erf32((c - y0) * k)
	acc := float32(0)
	for i := 0; i < 4; i++ {
		y1 := y0 + h*float32(i+1)
		e1 := erf32((c - y1) * k)
		d := r - (y0 + h*(float32(i)+0.5))
		xi := r - sqrt32(max32(r*r-d*d, 0))
		acc = acc + (e0-e1)*(ea-erf32((a-xi)*k))
		e0 = e1
	}
	return acc * 0.25
}

// erf32 is sl.Erf: Abramowitz–Stegun 7.1.26, operation by operation.
func erf32(x float32) float32 {
	a := abs32(x)
	t := 1 / (1 + 0.3275911*a)
	y := 1 - t*(0.254829592+t*(-0.284496736+t*(1.421413741+t*(-1.453152027+t*1.061405429))))*float32(math.Exp(float64(-a*a)))
	if x < 0 {
		return -y
	}
	return y
}

func sqrt32(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// ShadowGeom is the device geometry paint.CPU draws a ShadowRect with
// (ShadowPlacement); the GPU renderer draws the same.
type ShadowGeom struct {
	// Shape is the shadow's rounded rectangle in logical pixels: the op's
	// Rect offset by (X, Y) and expanded by Spread, with ShapeRadii the
	// radii grown by Spread (not below 0). A shadow without blur is this
	// shape filled as FillRect fills it.
	Shape      geom.Rect
	ShapeRadii geom.Radii
	// Rect is Shape in device pixels, each edge rounded to the nearest
	// pixel (as clip rectangles are), and Radii its corner radii: clamped
	// to half the shorter side of Shape, scaled and truncated to whole
	// device pixels. Sigma is Blur*scale/2 truncated to 1/16 px. These are
	// the shadow mask cache's key, so a mask depends only on its key.
	Rect  image.Rectangle
	Radii geom.Radii
	Sigma float32
	// Margin is how many device pixels the blurred shadow reaches beyond
	// Rect: ceil(3s), s = sqrt(Sigma² + 1/12) as in ShadowCoverage.
	Margin int
}

// ShadowPlacement returns the device geometry of o at scale. blurred is
// false when o has no blur (Blur <= 0 or NaN): then only Shape and
// ShapeRadii are set. A blurred shadow with an empty Rect draws nothing;
// otherwise its pixel (x, y) has coverage ShadowCoverage(x+0.5, y+0.5,
// Rect, Radii, Sigma) within Margin pixels of Rect and 0 beyond.
func ShadowPlacement(o ShadowRect, scale float32) (g ShadowGeom, blurred bool) {
	rect, rad := shadowShape(o)
	if !(o.Shadow.Blur > 0) {
		return ShadowGeom{Shape: rect, ShapeRadii: rad}, false
	}
	s := scale
	dr := image.Rect(roundPx(rect.X*s), roundPx(rect.Y*s), roundPx(rect.Right()*s), roundPx(rect.Bottom()*s))
	g = shadowGeom(dr, rad.Clamp(rect.W, rect.H), s, o.Shadow.Blur*s/2)
	g.Shape, g.ShapeRadii = rect, rad
	return g, true
}

// shadowShape is o's rounded rectangle with offset and spread applied.
func shadowShape(o ShadowRect) (geom.Rect, geom.Radii) {
	sh := o.Shadow
	rect := o.Rect.Offset(sh.X, sh.Y).Expand(sh.Spread)
	rad := o.Radii
	if sh.Spread != 0 {
		rad = rad.Grow(sh.Spread)
	}
	return rect, rad
}

// shadowGeom quantizes a blurred shadow at device rectangle dr, logical
// radii cr (already clamped) at scale s and device sigma.
func shadowGeom(dr image.Rectangle, cr geom.Radii, s, sigma float32) ShadowGeom {
	q := func(v float32) float32 {
		d := v * s
		if !(d > 0) {
			return 0 // also NaN
		}
		return float32(int32(min32(d, 1<<24)))
	}
	sq := float32(0)
	if sigma > 0 {
		sq = float32(int32(min32(sigma*16, 1<<30))) / 16
	}
	return ShadowGeom{
		Rect:   dr,
		Radii:  geom.Radii{TL: q(cr.TL), TR: q(cr.TR), BR: q(cr.BR), BL: q(cr.BL)},
		Sigma:  sq,
		Margin: int(math.Ceil(float64(newShadowAxes(sq).reach))),
	}
}

// roundPx rounds a device coordinate to whole pixels as CPU.devRect does,
// keeping huge, infinite and NaN values well defined.
func roundPx(v float32) int {
	const lim = 1 << 24
	f := math.Round(float64(v))
	if !(f > -lim) {
		return -lim
	}
	return int(min(f, lim))
}

// shadowMaskImage returns the shadow mask of a w×h device rectangle with
// device radii rad and sigma, margin pixels on every side: ShadowCoverage
// at each pixel center, rounded to 8 bits. The axis coverages are computed
// once per column and row, and pixels out of reach of every corner skip
// the corner terms (which are then exactly 0), so the result equals
// ShadowCoverage pixel by pixel.
func shadowMaskImage(w, h int, rad geom.Radii, sigma float32, margin int) *image.Alpha {
	mw, mh := w+2*margin, h+2*margin
	m := image.NewAlpha(image.Rect(0, 0, mw, mh))
	fm := float32(margin)
	r := geom.Rect{X: fm, Y: fm, W: float32(w), H: float32(h)}
	sh := newShadowAxes(sigma)
	cx := make([]float32, mw)
	for u := range cx {
		cx[u] = sh.axis(float32(u)+0.5, r.X, r.X+r.W)
	}
	// Corner terms vanish more than reach beyond a corner's radius from
	// both edges that meet there (see shadowCorner).
	cm := min32(r.W, r.H) * 0.5
	far := max32(max32(clamp32(rad.TL, 0, cm), clamp32(rad.TR, 0, cm)), max32(clamp32(rad.BR, 0, cm), clamp32(rad.BL, 0, cm))) + sh.reach
	for v := 0; v < mh; v++ {
		py := float32(v) + 0.5
		cy := sh.axis(py, r.Y, r.Y+r.H)
		row := m.Pix[v*m.Stride : v*m.Stride+mw]
		rowFar := py-r.Y > far && r.Y+r.H-py > far
		for u := range row {
			px := float32(u) + 0.5
			var c float32
			if rowFar || (px-r.X > far && r.X+r.W-px > far) {
				c = clamp32(cx[u]*cy, 0, 1)
			} else {
				c = sh.combine(cx[u], cy, px, py, r, rad)
			}
			row[u] = uint8(c*255 + 0.5)
		}
	}
	return m
}

// blendMask blends c over the target through mask m placed at at (device
// pixels), times the rounded-clip coverage, within the clip. Each pixel is
// rounded once from the exact source-over result with coverage m/255
// (premultiplied, as a GPU blends into an 8-bit target), so stacked
// translucent shadows do not drift from the GPU's by a level per layer as
// draw.DrawMask's truncating arithmetic does.
func (r *CPU) blendMask(m *image.Alpha, at image.Point, c color.NRGBA) {
	b := image.Rectangle{Min: at, Max: at.Add(m.Rect.Size())}.Intersect(r.clip).Intersect(r.cur.Rect)
	if b.Empty() || c.A == 0 {
		return
	}
	const one = 255 * 255
	ca := uint32(c.A)
	pr, pg, pb := uint32(c.R)*ca, uint32(c.G)*ca, uint32(c.B)*ca // premultiplied ×255
	dst := r.cur
	for y := b.Min.Y; y < b.Max.Y; y++ {
		src := m.Pix[(y-at.Y)*m.Stride:]
		di := dst.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x, di = x+1, di+4 {
			v := uint32(src[x-at.X])
			if v != 0 && r.cmask != nil {
				v = (v*r.clipCov(x, y) + 127) / 255
			}
			if v == 0 {
				continue
			}
			a := ca * v // source alpha ×255²
			inv := one - a
			p := dst.Pix[di : di+4 : di+4]
			p[0] = uint8((uint32(p[0])*inv + pr*v + one/2) / one)
			p[1] = uint8((uint32(p[1])*inv + pg*v + one/2) / one)
			p[2] = uint8((uint32(p[2])*inv + pb*v + one/2) / one)
			p[3] = uint8((uint32(p[3])*inv + 255*a + one/2) / one)
		}
	}
}
