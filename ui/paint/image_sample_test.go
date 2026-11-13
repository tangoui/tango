package paint

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func sampleTestImage() *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			a := uint8(120 + 20*y)
			m.SetRGBA(x, y, color.RGBA{uint8(x * 30 * int(a) / 255), uint8(y * 50 * int(a) / 255), uint8((6 - x) * 40 * int(a) / 255), a})
		}
	}
	return m
}

// TestSampleCatmullRomTexelCenters: at texel centers the Catmull-Rom
// weights are (0, 1, 0, 0), so the sample is exactly the texel.
func TestSampleCatmullRomTexelCenters(t *testing.T) {
	m := sampleTestImage()
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			u, v := (float32(x)+0.5)/7, (float32(y)+0.5)/5
			if got, want := SampleCatmullRom(m, u, v), m.RGBAAt(x, y); got != want {
				t.Fatalf("texel %d,%d: %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestSampleCatmullRomClamp: edges are clamped (outside samples repeat
// the edge texels), a constant image samples constant everywhere, and the
// result is valid premultiplied color (channels ≤ alpha) even where the
// kernel's negative lobes overshoot.
func TestSampleCatmullRomClamp(t *testing.T) {
	m := sampleTestImage()
	if got, want := SampleCatmullRom(m, -3, -3), m.RGBAAt(0, 0); got != want {
		t.Fatalf("far outside top-left: %v, want %v", got, want)
	}
	if got, want := SampleCatmullRom(m, 5, 0.5/5), m.RGBAAt(6, 0); got != want {
		t.Fatalf("far right: %v, want %v", got, want)
	}
	nan := float32(math.NaN())
	_ = SampleCatmullRom(m, nan, float32(math.Inf(1))) // must not panic

	c := image.NewRGBA(image.Rect(0, 0, 3, 3))
	for i := range c.Pix {
		c.Pix[i] = []uint8{40, 80, 120, 200}[i%4]
	}
	for _, uv := range [][2]float32{{0.1, 0.9}, {0.37, 0.52}, {0.99, 0.01}} {
		if got := SampleCatmullRom(c, uv[0], uv[1]); got != (color.RGBA{40, 80, 120, 200}) {
			t.Fatalf("constant image at %v: %v", uv, got)
		}
	}

	// A hard edge from opaque white to transparent overshoots.
	e := image.NewRGBA(image.Rect(0, 0, 4, 1))
	copy(e.Pix, []uint8{255, 255, 255, 255, 255, 255, 255, 255, 0, 0, 0, 0, 0, 0, 0, 0})
	for i := 0; i <= 100; i++ {
		s := SampleCatmullRomF(e, float32(i)/100, 0.5)
		if s[3] < 0 || s[3] > 1 {
			t.Fatalf("alpha %v at %d", s[3], i)
		}
		for k := 0; k < 3; k++ {
			if s[k] < 0 || s[k] > s[3] {
				t.Fatalf("channel %d = %v > alpha %v at %d", k, s[k], s[3], i)
			}
		}
	}
}

// TestCatmullRomWeights: the weights sum to 1 and are the Catmull-Rom
// spline's (B=0, C=0.5).
func TestCatmullRomWeights(t *testing.T) {
	for i := 0; i <= 64; i++ {
		f := float32(i) / 64
		w := CatmullRomWeights(f)
		if s := w[0] + w[1] + w[2] + w[3]; math.Abs(float64(s-1)) > 1e-6 {
			t.Fatalf("f=%v: sum %v", f, s)
		}
	}
	if w := CatmullRomWeights(0.5); w != [4]float32{-0.0625, 0.5625, 0.5625, -0.0625} {
		t.Fatalf("f=0.5: %v", w)
	}
}

// TestImageReduction: sources more than twice the device size on both
// axes are reduced to the device size first.
func TestImageReduction(t *testing.T) {
	for _, c := range []struct {
		b            image.Rectangle
		dw, dh       float32
		wantW, wantH int
	}{
		{image.Rect(0, 0, 100, 100), 50, 50, 0, 0},
		{image.Rect(0, 0, 4000, 100), 100, 80, 100, 100},
		{image.Rect(0, 0, 50, 900), 40, 30, 50, 30},
		{image.Rect(0, 0, 101, 101), 50, 50, 50, 50},
		{image.Rect(0, 0, 400, 60), 20.5, 40, 21, 60}, // x only
		{image.Rect(0, 0, 400, 300), 20.5, 40, 21, 40},
		{image.Rect(0, 0, 64, 64), 1, 1, 1, 1},
		{image.Rect(0, 0, 1, 1), 16, 16, 0, 0},
	} {
		w, h := ImageReduction(c.b, c.dw, c.dh)
		if w != c.wantW || h != c.wantH {
			t.Errorf("%v into %vx%v: %d×%d, want %d×%d", c.b, c.dw, c.dh, w, h, c.wantW, c.wantH)
		}
	}
}

// TestCPUImageUsesCatmullRom: paint.CPU draws an untransformed image with
// SampleCatmullRomF at device pixel centers, times RRectCoverage, blended
// as blendPremulF does (the GPU's KindImage instance, pixel for pixel).
func TestCPUImageUsesCatmullRom(t *testing.T) {
	src := sampleTestImage()
	g := image.NewGray(image.Rect(0, 0, 3, 2))
	copy(g.Pix, []uint8{0, 128, 255, 60, 200, 30})
	for _, c := range []struct {
		img   image.Image
		rect  geom.Rect
		radii geom.Radii
		scale float32
	}{
		{src, geom.R(3, 2, 30, 20), geom.Radii{}, 1},
		{src, geom.R(2.5, 3.25, 21, 13), geom.Rad(5), 1.5},
		{g, geom.R(1, 1, 40, 30), geom.Radii{TL: 8}, 2},
	} {
		l := &List{Background: color.NRGBA{250, 250, 250, 255}}
		l.DrawImage(c.img, c.rect, c.radii)
		r := NewCPU()
		r.Render(l, geom.Size{W: 50, H: 40}, c.scale)
		got := r.Image()

		want := image.NewRGBA(got.Rect)
		for i := range want.Pix {
			want.Pix[i] = []uint8{250, 250, 250, 255}[i%4]
		}
		s := c.scale
		dr := geom.Rect{X: c.rect.X * s, Y: c.rect.Y * s, W: c.rect.W * s, H: c.rect.H * s}
		drad := geom.Radii{TL: c.radii.TL * s, TR: c.radii.TR * s, BR: c.radii.BR * s, BL: c.radii.BL * s}
		pre := ToRGBA(c.img)
		for y := 0; y < want.Rect.Dy(); y++ {
			for x := 0; x < want.Rect.Dx(); x++ {
				px, py := float32(x)+0.5, float32(y)+0.5
				cov := RRectCoverage(px, py, dr, drad)
				if cov <= 0 {
					continue
				}
				blendPremulF(want, x, y, SampleCatmullRomF(pre, (px-dr.X)/dr.W, (py-dr.Y)/dr.H), cov)
			}
		}
		for i := range want.Pix {
			if want.Pix[i] != got.Pix[i] {
				x, y := (i/4)%want.Rect.Dx(), (i/4)/want.Rect.Dx()
				t.Fatalf("%v at scale %v: pixel %d,%d = %v, want %v", c.rect, s, x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

// TestCPUImageTransformedUsesCatmullRom: under a rotation the image is
// sampled with SampleCatmullRomF too (no bilinear lookup): an image of one
// constant color, rotated, keeps exactly that color inside.
func TestCPUImageTransformedUsesCatmullRom(t *testing.T) {
	c := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range c.Pix {
		c.Pix[i] = []uint8{30, 60, 90, 180}[i%4]
	}
	l := &List{}
	l.Transform(geom.Rotate(0.3))
	l.DrawImage(c, geom.R(20, 0, 30, 30), geom.Radii{})
	l.Pop()
	r := NewCPU()
	r.Render(l, geom.Size{W: 60, H: 60}, 1)
	// The rotated square's center.
	p := geom.Rotate(0.3).Apply(geom.Pt(35, 15))
	if got := r.Image().RGBAAt(int(p.X), int(p.Y)); got != (color.RGBA{30, 60, 90, 180}) {
		t.Fatalf("center %v", got)
	}

	// A two-color image: the sample is the Catmull-Rom value, which
	// overshoots near the edge (bilinear never does), so check against
	// SampleCatmullRomF directly at one interior device pixel.
	src := sampleTestImage()
	m := geom.Rotate(0.2)
	l = &List{}
	l.Transform(m)
	l.DrawImage(src, geom.R(10, 5, 35, 25), geom.Radii{})
	l.Pop()
	r = NewCPU()
	r.Render(l, geom.Size{W: 60, H: 60}, 1)
	inv, _ := m.Invert()
	for _, d := range []image.Point{{25, 22}, {30, 20}, {20, 25}} {
		lp := inv.Apply(geom.Pt(float32(d.X)+0.5, float32(d.Y)+0.5))
		want := image.NewRGBA(image.Rect(d.X, d.Y, d.X+1, d.Y+1))
		blendPremulF(want, d.X, d.Y, SampleCatmullRomF(src, (lp.X-10)/35, (lp.Y-5)/25), 1)
		if got := r.Image().RGBAAt(d.X, d.Y); got != want.RGBAAt(d.X, d.Y) {
			t.Fatalf("pixel %v: %v, want %v", d, got, want.RGBAAt(d.X, d.Y))
		}
	}
}

// TestImageAnisotropicReduction: an image shrunk strongly along one axis
// only (4000×100 alternating black/white columns drawn 100×100) is
// prefiltered along that axis: every pixel is mid gray, as a box filter
// gives, not an aliased stripe pattern.
func TestImageAnisotropicReduction(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4000, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 4000; x++ {
			v := uint8(255 * (x % 2))
			src.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	if w, h := ImageReduction(src.Rect, 100, 100); w != 100 || h != 100 {
		t.Fatalf("reduction %d×%d; want 100×100 (only x reduced)", w, h)
	}
	l := &List{}
	l.DrawImage(src, geom.R(0, 0, 100, 100), geom.Radii{})
	r := NewCPU()
	r.Render(l, geom.Size{W: 100, H: 100}, 1)
	for y := 2; y < 98; y++ {
		for x := 2; x < 98; x++ {
			if c := r.Image().RGBAAt(x, y); c.R < 119 || c.R > 136 {
				t.Fatalf("pixel %d,%d = %v; want ≈127 (box filtered)", x, y, c)
			}
		}
	}
}

// valueImg is an image used by value with an incomparable dynamic value
// in an interface field (map hashing would panic).
type valueImg struct {
	image.Image
	tag any
}

// TestCPUImageValueType: such an image draws without panicking.
func TestCPUImageValueType(t *testing.T) {
	img := valueImg{sampleTestImage(), []int{1}}
	l := &List{}
	l.DrawImage(img, geom.R(2, 2, 20, 15), geom.Radii{})
	r := NewCPU()
	r.Render(l, geom.Size{W: 30, H: 20}, 1)
	r.InvalidateImage(img)
	if r.Image().RGBAAt(10, 8).A == 0 {
		t.Fatal("nothing drawn")
	}
}
