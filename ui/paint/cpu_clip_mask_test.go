package paint

import (
	"image"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// Fully covered intersections should not allocate a viewport-sized mask or
// disturb an enclosing rounded clip. Reintroducing per-pixel masks here fails
// the allocation assertion even when the rendered pixels are unchanged.
func TestRoundedClipOpaqueIntersectionDoesNotAllocate(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "nested"}[parent], func(t *testing.T) {
			r := NewCPU()
			r.prepare(1.5)
			if parent {
				r.cmask = image.NewAlpha(image.Rect(0, 0, 240, 160))
				r.cmaskAt = image.Pt(-12, 4)
				for i := range r.cmask.Pix {
					r.cmask.Pix[i] = uint8(i % 256)
				}
			}
			want, at := r.cmask, r.cmaskAt
			var got *image.Alpha
			var gotAt image.Point
			allocs := testing.AllocsPerRun(10, func() {
				got, gotAt = r.rrectMask(geom.R(0.25, 0.5, 180, 120), geom.Rad(12), image.Rect(40, 35, 200, 140))
			})
			if allocs != 0 {
				t.Fatalf("opaque clip allocated %.0f times; want no new mask", allocs)
			}
			if got != want || gotAt != at {
				t.Fatal("opaque clip did not preserve the enclosing coverage mask")
			}
		})
	}
}

// RRectCoverage is the scalar pixel definition shared with GPU shaders. It
// provides a separate oracle for fast row bands, including nested-mask rounding.
func TestRoundedClipMaskMatchesScalarCoverage(t *testing.T) {
	for _, scale := range []float32{0.75, 1, 1.5, 2} {
		for _, rad := range []geom.Radii{geom.Rad(6), {TL: 18, TR: 2, BR: 35, BL: 0}, geom.Rad(500)} {
			for _, parent := range []bool{false, true} {
				r := NewCPU()
				r.prepare(scale)
				g := geom.R(-3.3, 2.6, 76.5, 40.2)
				bounds := image.Rect(-12, -6, 180, 100)
				if parent {
					r.cmask = image.NewAlpha(image.Rect(0, 0, 180, 100))
					r.cmaskAt = image.Pt(-7, -2)
					for i := range r.cmask.Pix {
						r.cmask.Pix[i] = uint8(i % 256)
					}
				}
				m, at := r.rrectMask(g, rad, bounds)
				dr := geom.Rect{X: g.X * scale, Y: g.Y * scale, W: g.W * scale, H: g.H * scale}
				drad := geom.Radii{TL: rad.TL * scale, TR: rad.TR * scale, BR: rad.BR * scale, BL: rad.BL * scale}
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						coverage := float64(RRectCoverage(float32(x)+0.5, float32(y)+0.5, dr, drad))
						if parent {
							coverage *= float64(r.clipCov(x, y)) / 255
						}
						want := uint8(coverage*255 + 0.5)
						if got := m.AlphaAt(x-at.X, y-at.Y).A; got != want {
							t.Fatalf("scale %v radii %+v parent %v pixel (%d,%d): %d, want %d", scale, rad, parent, x, y, got, want)
						}
					}
				}
			}
		}
	}
}

func BenchmarkRoundedClipMaskViewport(b *testing.B) {
	r := NewCPU()
	r.prepare(1)
	g, rad := geom.R(0, 0, 1000, 600), geom.Rad(6)
	bounds := image.Rect(0, 0, 1000, 600)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.rrectMask(g, rad, bounds)
	}
}
