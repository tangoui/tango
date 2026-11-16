//go:build darwin

package macoslite

import (
	"image"
	"math"
)

// Canvas draws into the current frame's bitmap. Coordinates are in points
// with the origin at the top-left; the canvas scales them to pixels. A Canvas
// is valid only during the View.Draw call that received it.
type Canvas struct {
	img   *image.RGBA // premultiplied RGBA, device pixels
	scale float64
	fonts *fontCache
}

func newCanvas(img *image.RGBA, scale float64) *Canvas {
	return &Canvas{img: img, scale: scale}
}

// pixelBounds converts r to a pixel rectangle clipped to the bitmap.
func (c *Canvas) pixelBounds(r Rect) image.Rectangle {
	s := c.scale
	pr := image.Rect(
		int(math.Round(r.X*s)), int(math.Round(r.Y*s)),
		int(math.Round((r.X+r.W)*s)), int(math.Round((r.Y+r.H)*s)))
	return pr.Intersect(c.img.Rect)
}

// blend composites col at coverage cov (0..1) over pixel (x, y), source-over.
func (c *Canvas) blend(x, y int, col Color, cov float64) {
	a := float64(col.A) * cov / 255
	if a <= 0 {
		return
	}
	i := c.img.PixOffset(x, y)
	p := c.img.Pix[i : i+4 : i+4]
	inv := 1 - a
	p[0] = uint8(float64(col.R)*a + float64(p[0])*inv + 0.5)
	p[1] = uint8(float64(col.G)*a + float64(p[1])*inv + 0.5)
	p[2] = uint8(float64(col.B)*a + float64(p[2])*inv + 0.5)
	p[3] = uint8(255*a + float64(p[3])*inv + 0.5)
}

// Fill paints r with col.
func (c *Canvas) Fill(r Rect, col Color) {
	if col.A == 0 {
		return
	}
	b := c.pixelBounds(r)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c.blend(x, y, col, 1)
		}
	}
}

// RoundRect paints r with corners of the given radius (points), antialiased.
func (c *Canvas) RoundRect(r Rect, radius float64, col Color) {
	if col.A == 0 {
		return
	}
	s := c.scale
	rad := math.Max(0, math.Min(radius, math.Min(r.W, r.H)/2)) * s
	x0, y0 := r.X*s, r.Y*s
	x1, y1 := (r.X+r.W)*s, (r.Y+r.H)*s
	b := c.pixelBounds(r)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := float64(y) + 0.5
		for x := b.Min.X; x < b.Max.X; x++ {
			px := float64(x) + 0.5
			cx := math.Max(x0+rad, math.Min(px, x1-rad))
			cy := math.Max(y0+rad, math.Min(py, y1-rad))
			cov := 1.0
			if dx, dy := px-cx, py-cy; dx != 0 || dy != 0 {
				cov = math.Max(0, math.Min(1, rad-math.Hypot(dx, dy)+0.5))
			}
			c.blend(x, y, col, cov)
		}
	}
}

// Text draws s with its baseline-left at origin. Implemented in Task 4.
func (c *Canvas) Text(s string, origin Point, size float64, col Color) {}

// fontCache is filled in by the CoreText task.
type fontCache struct{}

func (f *fontCache) clear() {}
