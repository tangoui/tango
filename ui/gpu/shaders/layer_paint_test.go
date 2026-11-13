package shaders

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/shader/sl"
)

// TestPaintLayerCompositeMatchesShader: an opacity group's layer texel is
// composited by the GPU with LayerComposite's color, which must be exactly
// what paint.CPU composites onto a transparent target (its integer
// arithmetic, alpha quantized to 8 bits), for every alpha and a grid of
// premultiplied texels. The clip coverage path (an 8-bit mask) is checked
// on its own: coverage cov must act as the alpha ia*round(cov*255)/255
// truncated.
func TestPaintLayerCompositeMatchesShader(t *testing.T) {
	cpu := paint.NewCPU()
	region := image.Rect(0, 0, 1, 1)
	px := func(ops ...paint.Op) color.RGBA {
		return cpu.RenderRegion(ops, region, 1).RGBAAt(0, 0)
	}
	for _, c := range []color.NRGBA{{255, 255, 255, 255}, {0x16, 0x77, 0xff, 255}, {200, 100, 50, 128}, {255, 0, 0, 1}, {30, 230, 90, 37}, {255, 255, 255, 254}} {
		fill := paint.FillRect{Rect: geom.R(0, 0, 1, 1), Color: c}
		lp := px(fill) // the layer texel paint.CPU composites
		tex := sl.V4(float32(lp.R)/255, float32(lp.G)/255, float32(lp.B)/255, float32(lp.A)/255)
		for ia := 1; ia < 255; ia++ {
			a := float32(ia) / 255
			got := px(paint.PushOpacity{Alpha: a}, fill, paint.Pop{})
			w := LayerComposite(tex, a, 1)
			want := color.RGBA{uint8(w.X*255 + 0.5), uint8(w.Y*255 + 0.5), uint8(w.Z*255 + 0.5), uint8(w.W*255 + 0.5)}
			if got != want {
				t.Fatalf("texel %v alpha %d/255: paint.CPU %v, LayerComposite %v (%v)", lp, ia, got, want, w)
			}
			// Exact multiples of 1/255.
			for _, f := range []float32{w.X, w.Y, w.Z, w.W} {
				if d := f*255 - float32(int(f*255+0.5)); d > 1e-4 || d < -1e-4 {
					t.Fatalf("LayerComposite %v is not a multiple of 1/255", w)
				}
			}
		}
	}
	// Coverage: cov quantized to an 8-bit mask, then m = ia*mask/255.
	for _, ia := range []int{1, 77, 128, 200, 254, 255} {
		for mask := 0; mask <= 255; mask++ {
			cov := float32(mask) / 255
			m := ia * mask / 255
			for k := 0; k <= 255; k += 17 {
				tex := sl.V4(float32(k)/255, float32(k)/255, float32(k)/255, float32(k)/255)
				w := LayerComposite(tex, float32(ia)/255, cov)
				if got, want := int(w.W*255+0.5), k*m/255; got != want {
					t.Fatalf("ia %d mask %d texel %d: %d, want %d", ia, mask, k, got, want)
				}
			}
		}
	}
}
