package paint

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestRenderRegionIntoReusesBufferAndMatches(t *testing.T) {
	r := NewCPU()
	region := image.Rect(0, 0, 64, 48)
	ops := []Op{FillRect{Rect: geom.Rect{X: 4, Y: 4, W: 20, H: 10}, Color: color.NRGBA{R: 200, A: 255}}}
	want := r.RenderRegion(ops, region, 1)

	dst := image.NewRGBA(region)
	for i := range dst.Pix {
		dst.Pix[i] = 0x7f // stale pixels must be cleared
	}
	got := r.RenderRegionInto(dst, ops, region, 1)
	if got != dst {
		t.Fatal("RenderRegionInto did not reuse a buffer of the right size")
	}
	if string(got.Pix) != string(want.Pix) {
		t.Fatal("RenderRegionInto pixels differ from RenderRegion")
	}
	if a := testing.AllocsPerRun(20, func() { r.RenderRegionInto(dst, ops, region, 1) }); a > 2 {
		t.Fatalf("RenderRegionInto allocated %.0f times with a reusable buffer", a)
	}
	other := r.RenderRegionInto(dst, ops, image.Rect(0, 0, 32, 32), 1)
	if other == dst || other.Bounds() != image.Rect(0, 0, 32, 32) {
		t.Fatal("a different region must get a buffer of its own bounds")
	}
}
