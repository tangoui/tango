package paint

import (
	"image"
	"math"
)

// PathDeviceBounds returns the device pixels FillPath of p may touch at
// scale: p's bounds scaled, floored / ceiled to whole pixels and grown by
// one pixel on each side (the rectangle paint.CPU rasterizes FillPath in;
// its outermost rows and columns are always transparent). Coordinates
// beyond ±2^24 pixels are clamped.
func PathDeviceBounds(p *Path, scale float32) image.Rectangle {
	b := p.Bounds()
	return image.Rect(clampPx(math.Floor(float64(b.X*scale)))-1, clampPx(math.Floor(float64(b.Y*scale)))-1,
		clampPx(math.Ceil(float64(b.Right()*scale)))+1, clampPx(math.Ceil(float64(b.Bottom()*scale)))+1)
}

// PathMask returns the coverage of p (logical coordinates, non-zero rule)
// at the device scale of the current frame (BeginFrame) as an 8-bit mask
// with origin 0,0 covering PathDeviceBounds(p, scale), placed at at: the
// coverage paint.CPU draws FillPath and Path-shaped gradients with. GPU
// renderers cache it to draw FillPath without a patch. The caller must keep
// the mask small: it allocates the whole bounds.
func (r *CPU) PathMask(p *Path) (mask *image.Alpha, at image.Point) {
	if r.scale == 0 {
		r.prepare(1)
	}
	return r.coverage(p, PathDeviceBounds(p, r.scale))
}
