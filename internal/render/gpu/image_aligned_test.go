package gpu

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestImageAlignedSingleSample: an image drawn at its own size on whole
// device pixels (each pixel center on a texel center, as the minimap's
// strips) is marked to read one texel per pixel instead of the
// Catmull-Rom filter's 16 (Params.Z); one at a fractional position or
// another size is not. Both draw the CPU renderer's pixels exactly.
func TestImageAlignedSingleSample(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 37)
	}
	for y := range 16 {
		for x := range 24 {
			c := img.RGBAAt(x, y)
			a := c.A
			img.SetRGBA(x, y, color.RGBA{min(c.R, a), min(c.G, a), min(c.B, a), a})
		}
	}
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			s := scale
			l := &paint.List{Background: color.NRGBA{R: 17, G: 43, B: 71, A: 255}}
			l.DrawImage(img, geom.R(4, 6, 24/s, 16/s), geom.Radii{})         // aligned, 1:1
			l.DrawImage(img, geom.R(40+0.25/s, 6, 24/s, 16/s), geom.Radii{}) // a quarter pixel off
			l.DrawImage(img, geom.R(4, 40, 30/s, 16/s), geom.Radii{})        // stretched
			l.DrawImage(img, geom.R(40, 40, 24/s, 16/s), geom.Rad(3))        // aligned, rounded
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			var aligned []bool
			for _, b := range dev.last.Batches {
				for _, in := range b.Instances {
					if in.Kind.X == shaders.KindImage {
						aligned = append(aligned, in.Params.Z > 0.5)
					}
				}
			}
			if fmt.Sprint(aligned) != "[true false false true]" {
				t.Fatalf("one-texel images %v, want [true false false true]", aligned)
			}
			if diff := gputest.Compare(want, got, 0); diff.Over != 0 {
				t.Fatalf("%d pixels differ, max channel difference %d", diff.Over, diff.MaxDiff)
			}
		})
	}
}
