package paint

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// stripes draws vertical black/white stripes over a white frame.
func stripesList(w float32) *List {
	l := &List{Background: color.NRGBA{255, 255, 255, 255}}
	for x := float32(0); x < w; x += 8 {
		l.Fill(geom.R(x, 0, 4, 200), geom.Radii{}, color.NRGBA{20, 40, 200, 255})
	}
	return l
}

func TestPlanBackdropBlur(t *testing.T) {
	frame := image.Rect(0, 0, 400, 300)
	for _, c := range []struct {
		radius, scale float32
		down, taps    int
	}{{1, 1, 1, 3}, {4, 1, 1, 12}, {5, 1, 2, 8}, {12, 2, 8, 9}, {64, 2, 32, 12}, {64, 4, 32, 24}, {1000, 4, 32, 24}} {
		sigma := BlurSigma(c.radius, c.scale)
		p, ok := PlanBackdropBlur(geom.Rect{X: 10.5, Y: 200.25, W: 300, H: 60}, sigma, frame, frame)
		if !ok {
			t.Fatalf("radius %v: no plan", c.radius)
		}
		if p.Down != c.down || p.Taps != c.taps {
			t.Errorf("radius %v scale %v: down %d taps %d, want %d %d", c.radius, c.scale, p.Down, p.Taps, c.down, c.taps)
		}
		if p.Out != image.Rect(10, 200, 311, 261) {
			t.Errorf("out %v", p.Out)
		}
		if !p.Src.In(frame) || !p.Out.In(p.Src) {
			t.Errorf("src %v out %v", p.Src, p.Out)
		}
		if p.Src.Min.X%p.Down != 0 || p.Src.Min.Y%p.Down != 0 {
			t.Errorf("src %v not aligned to %d", p.Src, p.Down)
		}
		if p.Small != image.Pt((p.Src.Dx()+p.Down-1)/p.Down, (p.Src.Dy()+p.Down-1)/p.Down) {
			t.Errorf("small %v for %v", p.Small, p.Src)
		}
	}
	if _, ok := PlanBackdropBlur(geom.Rect{W: 10, H: 10}, 0, frame, frame); ok {
		t.Error("zero sigma planned")
	}
	if BlurSigma(-1, 2) != 0 || BlurSigma(10, 2) != 20 || BlurSigma(100, 2) != 128 {
		t.Error("BlurSigma")
	}
}

// TestBackdropBlurUniform: blurring a uniform opaque area changes nothing,
// and Radius 0 draws nothing.
func TestBackdropBlurUniform(t *testing.T) {
	l := &List{Background: color.NRGBA{30, 60, 90, 255}}
	l.BackdropBlur(geom.R(10, 10, 80, 40), geom.Rad(8), 6)
	l.BackdropBlur(geom.R(0, 0, 100, 60), geom.Radii{}, 0)
	if len(l.Ops) != 1 {
		t.Fatalf("%d ops", len(l.Ops))
	}
	r := NewCPU()
	for _, s := range []float32{1, 1.5, 2} {
		if err := r.Render(l, geom.Size{W: 100, H: 60}, s); err != nil {
			t.Fatal(err)
		}
		img := r.Image()
		for i := 0; i < len(img.Pix); i += 4 {
			if c := (color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}); c != (color.RGBA{30, 60, 90, 255}) {
				t.Fatalf("scale %v: pixel %d = %v", s, i/4, c)
			}
		}
	}
}

// TestBackdropBlurSmooths: stripes under the blur become an even mix;
// pixels outside the rectangle stay as they were.
func TestBackdropBlurSmooths(t *testing.T) {
	l := stripesList(200)
	base := NewCPU()
	if err := base.Render(l, geom.Size{W: 200, H: 100}, 2); err != nil {
		t.Fatal(err)
	}
	l.BackdropBlur(geom.R(40, 30, 120, 40), geom.Radii{}, 6)
	r := NewCPU()
	if err := r.Render(l, geom.Size{W: 200, H: 100}, 2); err != nil {
		t.Fatal(err)
	}
	a, b := base.Image(), r.Image()
	out := image.Rect(80, 60, 320, 140)
	minR, maxR := 255, 0
	for y := 0; y < b.Rect.Dy(); y++ {
		for x := 0; x < b.Rect.Dx(); x++ {
			i := b.PixOffset(x, y)
			if !image.Pt(x, y).In(out) {
				if a.Pix[i] != b.Pix[i] || a.Pix[i+2] != b.Pix[i+2] {
					t.Fatalf("pixel %d,%d outside the blur changed", x, y)
				}
				continue
			}
			minR, maxR = min(minR, int(b.Pix[i])), max(maxR, int(b.Pix[i]))
			if b.Pix[i+3] != 255 {
				t.Fatalf("pixel %d,%d alpha %d", x, y, b.Pix[i+3])
			}
		}
	}
	// The stripes alternate 20 and 255 every 8 device pixels; sigma 12
	// leaves well under a tenth of that contrast.
	if maxR-minR > 20 {
		t.Errorf("blurred red spans %d..%d", minR, maxR)
	}
}

// TestBackdropBlurClipIndependent: a pixel's blurred value depends only
// on the target, not on how much of the rectangle a clip leaves (a partial
// redraw relies on this).
func TestBackdropBlurClipIndependent(t *testing.T) {
	for _, s := range []float32{1, 1.5, 2} {
		full := stripesList(200)
		full.BackdropBlur(geom.R(20.5, 10, 150, 70), geom.Rad(10), 9)
		clipped := stripesList(200)
		clipped.Clip(geom.R(60, 30, 40, 25), geom.Radii{})
		clipped.BackdropBlur(geom.R(20.5, 10, 150, 70), geom.Rad(10), 9)
		clipped.Pop()
		a, b := NewCPU(), NewCPU()
		if err := a.Render(full, geom.Size{W: 200, H: 100}, s); err != nil {
			t.Fatal(err)
		}
		if err := b.Render(clipped, geom.Size{W: 200, H: 100}, s); err != nil {
			t.Fatal(err)
		}
		in := image.Rect(int(60*s)+1, int(30*s)+1, int((60+40)*s)-1, int((30+25)*s)-1)
		for y := in.Min.Y; y < in.Max.Y; y++ {
			for x := in.Min.X; x < in.Max.X; x++ {
				if a.Image().RGBAAt(x, y) != b.Image().RGBAAt(x, y) {
					t.Fatalf("scale %v: pixel %d,%d %v clipped, %v whole", s, x, y, b.Image().RGBAAt(x, y), a.Image().RGBAAt(x, y))
				}
			}
		}
	}
}

// TestBackdropBlurTransform: under a translate + uniform scale the blur
// equals the mapped blur; under a rotation it draws nothing.
func TestBackdropBlurTransform(t *testing.T) {
	a := stripesList(200)
	a.BackdropBlur(geom.R(30, 20, 100, 40), geom.Rad(6), 8)
	b := stripesList(200)
	b.Transform(geom.Translate(10, 5).Mul(geom.Scale(2, 2)))
	b.BackdropBlur(geom.R(10, 7.5, 50, 20), geom.Rad(3), 4)
	b.Pop()
	ra, rb := NewCPU(), NewCPU()
	if err := ra.Render(a, geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	if err := rb.Render(b, geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	if string(ra.Image().Pix) != string(rb.Image().Pix) {
		t.Error("blur under a uniform transform differs from the mapped blur")
	}
	c := stripesList(200)
	c.Transform(geom.RotateAround(0.3, geom.Pt(100, 50)))
	c.BackdropBlur(geom.R(30, 20, 100, 40), geom.Radii{}, 8)
	c.Pop()
	rc, base := NewCPU(), NewCPU()
	if err := rc.Render(c, geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	if err := base.Render(stripesList(200), geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	if string(rc.Image().Pix) != string(base.Image().Pix) {
		t.Error("blur under a rotation drew something")
	}
}

// TestBackdropBlurInGroup: inside an opacity group only the group's own
// content is blurred (what lies under the group is not).
func TestBackdropBlurInGroup(t *testing.T) {
	l := stripesList(200)
	l.Opacity(0.5)
	l.Fill(geom.R(40, 20, 60, 40), geom.Radii{}, color.NRGBA{255, 0, 0, 255})
	l.BackdropBlur(geom.R(20, 10, 120, 60), geom.Radii{}, 3)
	l.Pop()
	r := NewCPU()
	if err := r.Render(l, geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	// Far from the red square, inside the blur: the group is transparent
	// there, so the stripes show unblurred.
	base := NewCPU()
	if err := base.Render(stripesList(200), geom.Size{W: 200, H: 100}, 1); err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{22, 12}, {25, 12}, {130, 65}} {
		if r.Image().RGBAAt(p.X, p.Y) != base.Image().RGBAAt(p.X, p.Y) {
			t.Errorf("pixel %v = %v, want the unblurred %v", p, r.Image().RGBAAt(p.X, p.Y), base.Image().RGBAAt(p.X, p.Y))
		}
	}
	// The red square's edge is soft.
	if c := r.Image().RGBAAt(40, 40); c.R == 0 {
		t.Errorf("edge pixel %v", c)
	}
}

// BenchmarkBackdropBlur measures paint.CPU drawing a FooterToolbar-sized
// frosted bar (1280×56 logical, radius 8, at scale 1 and 2) over stripes:
// the blur alone (BlurBackdrop + composite), without the rest of the frame.
func BenchmarkBackdropBlur(b *testing.B) {
	for _, s := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", s), func(b *testing.B) {
			r := NewCPU()
			if err := r.Render(stripesList(1280), geom.Size{W: 1280, H: 200}, s); err != nil {
				b.Fatal(err)
			}
			r.prepare(s)
			r.cur, r.clip, r.frame = r.img, r.img.Rect, r.img.Rect
			op := BackdropBlur{Rect: geom.R(0, 144, 1280, 56), Radius: 8}
			b.ResetTimer()
			for b.Loop() {
				r.backdropBlur(op)
			}
		})
	}
}
