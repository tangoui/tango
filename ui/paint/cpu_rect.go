package paint

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// fillRect is the fast path for axis-aligned rectangles without rounded
// corners: it blends directly into the target with exact area coverage on
// the fractional edges instead of running the general path rasterizer
// (whose cost grows with the area). Results match the rasterizer.
func (r *CPU) fillRect(g geom.Rect, c color.NRGBA) {
	s := float64(r.scale)
	x0, y0 := float64(g.X)*s, float64(g.Y)*s
	x1, y1 := float64(g.Right())*s, float64(g.Bottom())*s
	if x1 <= x0 || y1 <= y0 {
		return
	}
	b := r.clip.Intersect(r.cur.Rect)
	ix0 := max(b.Min.X, int(math.Floor(x0)))
	iy0 := max(b.Min.Y, int(math.Floor(y0)))
	ix1 := min(b.Max.X, int(math.Ceil(x1)))
	iy1 := min(b.Max.Y, int(math.Ceil(y1)))
	if ix0 >= ix1 || iy0 >= iy1 {
		return
	}
	cover := func(p int, lo, hi float64) float64 {
		return max(0, min(float64(p+1), hi)-max(float64(p), lo))
	}
	dst := r.cur
	if r.cmask != nil { // inside a rounded clip: multiply its coverage in
		for y := iy0; y < iy1; y++ {
			cy := cover(y, y0, y1)
			for x := ix0; x < ix1; x++ {
				blendCov(dst, x, y, c, cy*cover(x, x0, x1)*float64(r.clipCov(x, y))/255)
			}
		}
		return
	}
	if c.A == 255 {
		// Opaque: copy whole inner rows; only fractional edges blend.
		in0, in1 := max(ix0, int(math.Ceil(x0))), min(ix1, int(math.Floor(x1)))
		px := [4]uint8{c.R, c.G, c.B, 255}
		for y := iy0; y < iy1; y++ {
			cy := cover(y, y0, y1)
			if cy < 1 || in1 <= in0 {
				for x := ix0; x < ix1; x++ {
					blendCov(dst, x, y, c, cy*cover(x, x0, x1))
				}
				continue
			}
			row := dst.Pix[dst.PixOffset(in0, y):dst.PixOffset(in1, y)]
			if len(row) >= 4 {
				copy(row, px[:])
				for k := 4; k < len(row); k *= 2 {
					copy(row[k:], row[:k])
				}
			}
			if ix0 < in0 {
				blendCov(dst, ix0, y, c, cover(ix0, x0, x1))
			}
			if ix1-1 >= in1 {
				blendCov(dst, ix1-1, y, c, cover(ix1-1, x0, x1))
			}
		}
		return
	}
	ca := uint32(c.A)
	cr, cg, cb := uint32(c.R)*ca, uint32(c.G)*ca, uint32(c.B)*ca // premultiplied ×255
	for y := iy0; y < iy1; y++ {
		cy := cover(y, y0, y1)
		row := dst.PixOffset(ix0, y)
		for x := ix0; x < ix1; x, row = x+1, row+4 {
			cv := cy
			if x == ix0 || x == ix1-1 {
				cv *= cover(x, x0, x1)
			}
			if cv <= 0 {
				continue
			}
			m := uint32(cv*255 + 0.5) // coverage 0..255
			a := ca * m / 255
			if a == 0 {
				continue
			}
			inv := 255 - a
			p := dst.Pix[row : row+4 : row+4]
			p[0] = uint8((uint32(p[0])*inv + cr*m/255) / 255)
			p[1] = uint8((uint32(p[1])*inv + cg*m/255) / 255)
			p[2] = uint8((uint32(p[2])*inv + cb*m/255) / 255)
			p[3] = uint8((uint32(p[3])*inv)/255 + a)
		}
	}
}

func blendCov(dst *image.RGBA, x, y int, c color.NRGBA, cv float64) {
	if cv <= 0 {
		return
	}
	blendPixel(dst, x, y, c, uint32(cv*255+0.5))
}

// fillRRect fills a rounded rectangle with the shared RRectCoverage
// definition (the GPU shader's), times the rounded-clip coverage.
func (r *CPU) fillRRect(g geom.Rect, rad geom.Radii, c color.NRGBA) {
	s := r.scale
	dr := geom.Rect{X: g.X * s, Y: g.Y * s, W: g.W * s, H: g.H * s}
	if dr.W <= 0 || dr.H <= 0 {
		return
	}
	drad := geom.Radii{TL: rad.TL * s, TR: rad.TR * s, BR: rad.BR * s, BL: rad.BL * s}
	x0, y0 := float64(dr.X), float64(dr.Y)
	x1, y1 := float64(dr.X+dr.W), float64(dr.Y+dr.H)
	b := r.clip.Intersect(r.cur.Rect).Intersect(image.Rect(
		int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))))
	if b.Empty() {
		return
	}
	// Columns whose pixels lie inside the rect at least a (clamped) radius
	// from both sides are never in a corner quadrant: their coverage is the
	// row's vertical overlap, identical across the band, so it is computed
	// once per row (margins of a pixel keep this exact under rounding).
	rmax := float64(max(0, min(max(drad.TL, drad.TR, drad.BR, drad.BL), min(dr.W, dr.H)/2)))
	bx0 := max(b.Min.X, int(math.Ceil(x0+rmax))+1)
	bx1 := min(b.Max.X, int(math.Floor(x1-rmax))-1)
	dst := r.cur
	opaque := c.A == 255 && r.cmask == nil
	px := [4]uint8{c.R, c.G, c.B, 255}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := float32(y) + 0.5
		pixel := func(x int, cv float32) {
			f := float64(cv)
			if r.cmask != nil {
				f *= float64(r.clipCov(x, y)) / 255
			}
			blendCov(dst, x, y, c, f)
		}
		if bx0 >= bx1 {
			for x := b.Min.X; x < b.Max.X; x++ {
				pixel(x, RRectCoverage(float32(x)+0.5, py, dr, drad))
			}
			continue
		}
		for x := b.Min.X; x < bx0; x++ {
			pixel(x, RRectCoverage(float32(x)+0.5, py, dr, drad))
		}
		band := RRectCoverage(float32(bx0)+0.5, py, dr, drad)
		if opaque && band == 1 {
			row := dst.Pix[dst.PixOffset(bx0, y):dst.PixOffset(bx1, y)]
			copy(row, px[:])
			for k := 4; k < len(row); k *= 2 {
				copy(row[k:], row[:k])
			}
		} else if band > 0 {
			for x := bx0; x < bx1; x++ {
				pixel(x, band)
			}
		}
		for x := bx1; x < b.Max.X; x++ {
			pixel(x, RRectCoverage(float32(x)+0.5, py, dr, drad))
		}
	}
}
