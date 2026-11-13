package paint

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// TestPathMaskMatchesFill: PathDeviceBounds is the rectangle paint.CPU
// fills a path in, and PathMask's coverage is what FillPath draws: an
// opaque black FillPath on a transparent region has the mask's alpha (±1
// for the rasterizer's 16-bit blend), and nothing outside the bounds.
func TestPathMaskMatchesFill(t *testing.T) {
	paths := []*Path{
		NewPath().MoveTo(4.3, 3.7).LineTo(40, 10).LineTo(12, 30.5).Close(),
		NewPath().Circle(20.5, 18.25, 12.3),
		NewPath().MoveTo(20, 4).LineTo(26, 22).LineTo(10, 10).LineTo(30, 10).LineTo(14, 22).Close(),
	}
	for _, scale := range []float32{1, 1.5, 2} {
		for i, p := range paths {
			cpu := NewCPU()
			cpu.BeginFrame(scale)
			if b := PathDeviceBounds(p, scale); b != cpu.devBounds(p) {
				t.Fatalf("path %d @%v: PathDeviceBounds %v, devBounds %v", i, scale, b, cpu.devBounds(p))
			}
			m, at := cpu.PathMask(p)
			mb := image.Rectangle{Min: at, Max: at.Add(m.Rect.Size())}
			if mb != PathDeviceBounds(p, scale) {
				t.Fatalf("path %d @%v: mask at %v, want %v", i, scale, mb, PathDeviceBounds(p, scale))
			}
			region := mb.Inset(-4)
			img := cpu.RenderRegion([]Op{FillPath{p, color.NRGBA{A: 255}}}, region, scale)
			for y := region.Min.Y; y < region.Max.Y; y++ {
				for x := region.Min.X; x < region.Max.X; x++ {
					got := int(img.RGBAAt(x, y).A)
					want := 0
					if (image.Point{x, y}).In(mb) {
						want = int(m.AlphaAt(x-at.X, y-at.Y).A)
					}
					if d := got - want; d < -1 || d > 1 {
						t.Fatalf("path %d @%v: pixel %d,%d alpha %d, mask %d", i, scale, x, y, got, want)
					}
				}
			}
		}
	}
}

// TestPathDeviceBoundsNonFinite: clampPx maps NaN to -2^24 (like -Inf;
// Go's min and max would pass NaN through to an implementation-defined
// int conversion) and ±Inf to ∓2^24, so a path with non-finite
// coordinates gets well-defined bounds far outside any target.
func TestPathDeviceBoundsNonFinite(t *testing.T) {
	const lim = 1 << 24
	nan, inf := math.NaN(), math.Inf(1)
	for _, c := range []struct {
		f    float64
		want int
	}{{nan, -lim}, {-nan, -lim}, {inf, lim}, {-inf, -lim}, {3e9, lim}, {-3e9, -lim}, {12.7, 12}, {-5, -5}} {
		if got := clampPx(c.f); got != c.want {
			t.Errorf("clampPx(%v) = %d, want %d", c.f, got, c.want)
		}
	}
	n := float32(nan)
	for _, p := range []*Path{
		NewPath().MoveTo(n, n).LineTo(n, 4).LineTo(3, n).Close(),
		NewPath().MoveTo(n, 2).LineTo(10, 10).LineTo(2, 10).Close(),
	} {
		b := PathDeviceBounds(p, 2)
		if b.Min.X < -lim-1 || b.Max.X > lim+1 || b.Min.Y < -lim-1 || b.Max.Y > lim+1 {
			t.Fatalf("bounds %v outside ±2^24", b)
		}
		if b.Overlaps(image.Rect(-4096, -4096, 4096, 4096)) && b.Dx() > 1<<20 {
			t.Fatalf("bounds %v of a NaN path cover the target", b)
		}
	}
}
