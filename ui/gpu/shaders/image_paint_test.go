package shaders

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/shader/sl"
)

// TestPaintCatmullRomMatchesShader: paint.CPU and the GPU shader share one
// image filter, so paint.SampleCatmullRomF on a premultiplied RGBA image
// must equal SampleCatmullRom reading the same pixels from the nearest-
// filtered Img texture, over a grid of sample points that covers texel
// centers, fractions, the clamped borders and far outside.
func TestPaintCatmullRomMatchesShader(t *testing.T) {
	imgs := []*image.RGBA{image.NewRGBA(image.Rect(0, 0, 9, 6)), image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 3, 17))}
	for k, m := range imgs {
		b := m.Rect
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				a := uint8((x*53 + y*31 + k*17) % 256)
				m.SetRGBA(x, y, color.RGBA{uint8(int(a) * (x * 29 % 256) / 255), uint8(int(a) * (y * 71 % 256) / 255), uint8(int(a) * ((x + y) * 13 % 256) / 255), a})
			}
		}
	}
	defer func() { Img = sl.Texture2D{} }()
	for _, m := range imgs {
		Img = sl.NewTexture(m, sl.Nearest)
		w, h := float32(m.Rect.Dx()), float32(m.Rect.Dy())
		for j := -12; j <= 112; j++ {
			for i := -12; i <= 112; i++ {
				for _, o := range []float32{0, 0.37} {
					u, v := (float32(i)+o)/100, (float32(j)+o)/100
					got := paint.SampleCatmullRomF(m, u, v)
					want := SampleCatmullRom(sl.V2(u, v), sl.V2(w, h))
					for c, wv := range [4]float32{want.X, want.Y, want.Z, want.W} {
						if d := got[c] - wv; d > 1e-6 || d < -1e-6 || got[c] != got[c] {
							t.Fatalf("%v at %v,%v channel %d: paint %v, shader %v", m.Rect, u, v, c, got, want)
						}
					}
				}
			}
		}
	}
}

// TestCatmullRomWeightsMatchShader: the per-axis weights are shared.
func TestCatmullRomWeightsMatchShader(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		f := float32(i) / 1000
		got := paint.CatmullRomWeights(f)
		want := CatmullRomWeights(f)
		if got != [4]float32{want.X, want.Y, want.Z, want.W} {
			t.Fatalf("f=%v: paint %v, shader %v", f, got, want)
		}
	}
}
