package paint

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// shade fills the logical path p (before the current transform) by
// evaluating fn, a premultiplied color in 0..1, at the logical position of
// every covered device pixel centre (dx, dy is the device pixel).
func (r *CPU) shade(p *Path, fn func(x, y float32, dx, dy int) [4]float32) {
	if !r.ident {
		p = p.Transform(r.m)
	}
	inv, ok := r.device().Invert()
	if !ok {
		return
	}
	m, at := r.coverage(p, r.clip)
	dst := r.cur
	for y := 0; y < m.Rect.Dy(); y++ {
		for x := 0; x < m.Rect.Dx(); x++ {
			cov := uint32(m.Pix[y*m.Stride+x])
			if cov != 0 && r.cmask != nil {
				cov = (cov*r.clipCov(at.X+x, at.Y+y) + 127) / 255
			}
			if cov == 0 {
				continue
			}
			lp := inv.Apply(geom.Pt(float32(at.X+x)+0.5, float32(at.Y+y)+0.5))
			blendPremulF(dst, at.X+x, at.Y+y, fn(lp.X, lp.Y, at.X+x, at.Y+y), float32(cov)/255)
		}
	}
}

// shadeRect fills the rounded rectangle g (logical, no transform active)
// with the premultiplied color fn returns at each device pixel center
// p (device coordinates) of pixel (x, y), times RRectCoverage of the
// device rectangle and the rounded-clip coverage: the GPU renderer's
// KindGradient and KindChecker instances, pixel for pixel.
func (r *CPU) shadeRect(g geom.Rect, rad geom.Radii, fn func(p geom.Point, x, y int) [4]float32) {
	s := r.scale
	dr := geom.Rect{X: g.X * s, Y: g.Y * s, W: g.W * s, H: g.H * s}
	if !(dr.W > 0) || !(dr.H > 0) {
		return
	}
	drad := geom.Radii{TL: rad.TL * s, TR: rad.TR * s, BR: rad.BR * s, BL: rad.BL * s}
	b := r.clip.Intersect(r.cur.Rect).Intersect(image.Rect(
		clampPx(math.Floor(float64(dr.X))), clampPx(math.Floor(float64(dr.Y))),
		clampPx(math.Ceil(float64(dr.X+dr.W))), clampPx(math.Ceil(float64(dr.Y+dr.H)))))
	dst := r.cur
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := float32(y) + 0.5
		for x := b.Min.X; x < b.Max.X; x++ {
			px := float32(x) + 0.5
			cov := RRectCoverage(px, py, dr, drad)
			if cov > 0 && r.cmask != nil {
				cov = cov * float32(r.clipCov(x, y)) / 255
			}
			if cov <= 0 {
				continue
			}
			blendPremulF(dst, x, y, fn(geom.Pt(px, py), x, y), cov)
		}
	}
}

// clampPx converts a device coordinate to int, keeping huge values defined.
func clampPx(f float64) int {
	const lim = 1 << 24
	if !(f > -lim) {
		return -lim
	}
	return int(min(f, lim))
}

// blendPremulF composites the premultiplied color c (0..1) with coverage
// cov "source over" onto dst's pixel (x, y), rounding the result to 8 bits
// once: the arithmetic of the GPU's float blend into an 8-bit target (and
// of internal/render/gpu/cpuexec).
func blendPremulF(dst *image.RGBA, x, y int, c [4]float32, cov float32) {
	if !(image.Point{x, y}.In(dst.Rect)) {
		return
	}
	i := dst.PixOffset(x, y)
	p := dst.Pix[i : i+4 : i+4]
	inv := 1 - c[3]*cov
	for k := range 4 {
		p[k] = unorm8(c[k]*cov + float32(p[k])/255*inv)
	}
}

// unorm8 converts a 0..1 channel to 8 bits, rounding to nearest.
func unorm8(f float32) uint8 {
	if !(f > 0) {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return uint8(f*255 + 0.5)
}

// premulF is c premultiplied, in 0..1 (the GPU renderer's premul).
func premulF(c color.NRGBA) [4]float32 {
	a := float32(c.A) / 255
	return [4]float32{float32(c.R) / 255 * a, float32(c.G) / 255 * a, float32(c.B) / 255 * a, a}
}

func (o *Gradient) shape() *Path {
	if o.Path != nil {
		return o.Path
	}
	return NewPath().RRect(o.Rect, o.Radii)
}

// gradient draws o through its color ramp (GradientRamp, RampPremul) at
// GradientT of each pixel, sampled as the GPU's linear filter samples the
// ramp texture (RampSample) and dithered (DitherPremul with the pixel's
// Dither4x4 threshold) like the GPU's KindGradient. A rect gradient without a transform is
// evaluated in device pixels like the GPU's KindGradient instance; a Path
// gradient or one under a transform goes through the path rasterizer's
// coverage and is evaluated at logical pixel positions (the GPU renderer
// draws those cases with this code, as patches).
func (r *CPU) gradient(o Gradient) {
	if !gradientDrawable(o.Stops, o.Start, o.End) {
		return
	}
	ramp := GradientRamp(o.Stops)
	row := RampPremul(&ramp)
	kind, start, end := o.Kind, o.Start, o.End
	if o.Path == nil && r.ident {
		s := r.scale
		ds, de := geom.Pt(start.X*s, start.Y*s), geom.Pt(end.X*s, end.Y*s)
		r.shadeRect(o.Rect, o.Radii, func(p geom.Point, x, y int) [4]float32 {
			return DitherPremul(RampSample(&row, GradientT(kind, p, ds, de)), Dither4x4(x, y))
		})
		return
	}
	r.shade(o.shape(), func(lx, ly float32, x, y int) [4]float32 {
		return DitherPremul(RampSample(&row, GradientT(kind, geom.Pt(lx, ly), start, end)), Dither4x4(x, y))
	})
}

// transformedBy maps p through the renderer's current transform.
func (p *Path) transformedBy(r *CPU) *Path {
	if r.ident {
		return p
	}
	return p.Transform(r.m)
}

// checker draws o: CheckerSelect picks A or B per pixel. Without a
// transform it is evaluated at device pixel centers with the device cell
// size and anchor, like the GPU's KindChecker instance.
func (r *CPU) checker(o Checker) {
	if !(o.Cell > 0) {
		return
	}
	ca, cb := CheckerColors(o)
	a, b := premulF(ca), premulF(cb)
	pick := func(sel float32) [4]float32 {
		if sel > 0.5 {
			return b
		}
		return a
	}
	if r.ident {
		s := r.scale
		anchor, cell := geom.Pt(o.Rect.X*s, o.Rect.Y*s), o.Cell*s
		r.shadeRect(o.Rect, o.Radii, func(p geom.Point, _, _ int) [4]float32 {
			return pick(CheckerSelect(p, anchor, cell))
		})
		return
	}
	anchor := geom.Pt(o.Rect.X, o.Rect.Y)
	r.shade(NewPath().RRect(o.Rect, o.Radii), func(x, y float32, _, _ int) [4]float32 {
		return pick(CheckerSelect(geom.Pt(x, y), anchor, o.Cell))
	})
}
