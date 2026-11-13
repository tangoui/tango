package paint

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// TestShadowRectUsesShadowCoverage: a ShadowRect draws its color through
// ShadowCoverage of its device rectangle (snapped to whole pixels, radii
// truncated to whole device pixels, sigma to 1/16 px — the cache key's
// quantization, which the GPU renderer applies too), within the 8-bit
// mask rounding, at several scales, with spread and offset.
func TestShadowRectUsesShadowCoverage(t *testing.T) {
	for _, c := range []struct {
		rect  geom.Rect
		rad   geom.Radii
		sh    Shadow
		scale float32
	}{
		{geom.R(20, 15, 40, 30), geom.Rad(8), Shadow{Y: 4, Blur: 12, Color: color.NRGBA{0, 0, 0, 255}}, 1},
		{geom.R(20.3, 15.6, 40.2, 30), geom.Radii{TL: 12, BR: 3}, Shadow{X: -2, Y: 3, Blur: 6, Spread: 2, Color: color.NRGBA{0, 0, 0, 255}}, 2},
		{geom.R(20, 15, 40, 30), geom.Rad(6), Shadow{Blur: 3, Spread: -4, Color: color.NRGBA{0, 0, 0, 255}}, 1.5},
		{geom.R(25, 20, 30, 20), geom.Rad(4), Shadow{Blur: 40, Color: color.NRGBA{0, 0, 0, 255}}, 1},
	} {
		l := &List{Background: color.NRGBA{255, 255, 255, 255}}
		l.Shadows(c.rect, c.rad, []Shadow{c.sh})
		cpu := NewCPU()
		if err := cpu.Render(l, geom.Size{W: 100, H: 80}, c.scale); err != nil {
			t.Fatal(err)
		}
		img := cpu.Image()
		s := c.scale
		g := c.rect.Offset(c.sh.X, c.sh.Y).Expand(c.sh.Spread)
		rad := c.rad.Grow(c.sh.Spread).Clamp(g.W, g.H)
		dr := image.Rect(int(math.Round(float64(g.X*s))), int(math.Round(float64(g.Y*s))),
			int(math.Round(float64(g.Right()*s))), int(math.Round(float64(g.Bottom()*s))))
		dev := geom.Rect{X: float32(dr.Min.X), Y: float32(dr.Min.Y), W: float32(dr.Dx()), H: float32(dr.Dy())}
		q := func(v float32) float32 { return float32(int32(v * s)) }
		drad := geom.Radii{TL: q(rad.TL), TR: q(rad.TR), BR: q(rad.BR), BL: q(rad.BL)}
		sigma := float32(int32(c.sh.Blur*s/2*16)) / 16
		maxDiff := 0
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				cov := ShadowCoverage(float32(x)+0.5, float32(y)+0.5, dev, drad, sigma)
				want := 255 - int(math.Round(float64(cov)*255))
				got := int(img.RGBAAt(x, y).R)
				d := got - want
				if d < 0 {
					d = -d
				}
				maxDiff = max(maxDiff, d)
			}
		}
		if maxDiff > 1 {
			t.Errorf("rect %v radii %v shadow %+v scale %v: max diff %d from ShadowCoverage", c.rect, c.rad, c.sh, s, maxDiff)
		}
	}
}

// TestShadowMaskCached: equal shadows at different positions share one
// cached mask (the cache keyed by device size, radii and sigma), and the
// mask does not depend on which shadow filled the cache first.
func TestShadowMaskCached(t *testing.T) {
	cpu := NewCPU()
	l := &List{Background: color.NRGBA{255, 255, 255, 255}}
	sh := []Shadow{{Y: 2, Blur: 8, Color: color.NRGBA{0, 0, 0, 255}}}
	l.Shadows(geom.R(30, 30, 30, 20), geom.Rad(6), sh)
	l.Shadows(geom.R(130, 30, 30, 20), geom.Rad(6), sh)
	l.Shadows(geom.R(30, 130, 30, 20), geom.Rad(6.4), sh) // same key (radii truncated)
	if err := cpu.Render(l, geom.Size{W: 200, H: 200}, 1); err != nil {
		t.Fatal(err)
	}
	if n := len(cpu.shadows); n != 1 {
		t.Fatalf("%d cached shadow masks, want 1", n)
	}
	img := cpu.Image()
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			a, b := img.RGBAAt(x, y), img.RGBAAt(x+100, y)
			if a != b {
				t.Fatalf("(%d,%d) %v differs from the same shadow moved: %v", x, y, a, b)
			}
		}
	}
	// A fresh renderer drawing only the 6.4 shadow gets the same pixels.
	l2 := &List{Background: color.NRGBA{255, 255, 255, 255}}
	l2.Shadows(geom.R(30, 130, 30, 20), geom.Rad(6.4), sh)
	cpu2 := NewCPU()
	if err := cpu2.Render(l2, geom.Size{W: 200, H: 200}, 1); err != nil {
		t.Fatal(err)
	}
	for y := 100; y < 200; y++ {
		for x := 0; x < 100; x++ {
			if a, b := img.RGBAAt(x, y), cpu2.Image().RGBAAt(x, y); a != b {
				t.Fatalf("(%d,%d): %v with a shared cache, %v alone", x, y, a, b)
			}
		}
	}
}
