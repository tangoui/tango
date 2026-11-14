package gpu

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Backdrop blur in the Renderer: the frame it builds (grab, passes,
// composite, kept in order by hoistLayers) and partial redraws around it.

var (
	frostView = geom.R(0, 0, 160, 200)
	frostBar  = geom.R(0, 150, 160, 40)
	frostSize = geom.Size{W: 160, H: 220}
)

// frostList is a list of rows in a viewport scrolled by off, with a
// frosted bar over the viewport's bottom: fixed, or (inside) part of the
// scrolled content. hot recolors row hot (-1: none); extra adds hints.
func frostList(off float32, inside bool, hot int, extra func(*paint.List)) *paint.List {
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
	l.Clip(frostView, geom.Radii{})
	for i := range 40 {
		y := float32(i)*18 - off
		c := color.NRGBA{uint8(40 + i*5), 90, uint8(200 - i*4), 255}
		if i == hot {
			c = color.NRGBA{250, 200, 0, 255}
		}
		l.Fill(geom.R(6+float32(i%3)*20, y+2, 120, 12), geom.Rad(3), c)
	}
	if inside {
		bar := frostBar.Offset(0, -off+20)
		l.BackdropBlur(bar, geom.Rad(10), 6)
		l.Fill(bar, geom.Rad(10), color.NRGBA{255, 255, 255, 110})
	}
	l.Pop()
	if !inside {
		l.BackdropBlur(frostBar, geom.Radii{TL: 10, TR: 10}, 6)
		l.Fill(frostBar, geom.Radii{TL: 10, TR: 10}, color.NRGBA{255, 255, 255, 110})
	}
	if extra != nil {
		extra(l)
	}
	return l
}

// TestBlurFrame: a blur emits a grab of its source, the passes into
// reduced layers and a composite, in that order, after the batches it
// reads (also after hoisting a later group's layer).
func TestBlurFrame(t *testing.T) {
	l := frostList(0, false, -1, nil)
	l.Opacity(0.5)
	l.Fill(geom.R(10, 10, 30, 30), geom.Radii{}, color.NRGBA{255, 0, 0, 255})
	l.Fill(geom.R(20, 20, 30, 30), geom.Radii{}, color.NRGBA{0, 0, 255, 255})
	l.Pop()
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	if err := r.Render(l, frostSize, 2); err != nil {
		t.Fatal(err)
	}
	f := dev.last
	grab := -1
	for i, b := range f.Batches {
		if b.Grab != 0 {
			grab = i
			break
		}
	}
	if grab < 1 {
		t.Fatalf("grab at batch %d", grab)
	}
	// Before the grab: the rows. After it: down, H, V passes (sigma 12 at
	// scale 2 downsamples by 4), then the composite.
	modes := []float32{shaders.BlurDown, shaders.BlurH, shaders.BlurV}
	for k, m := range modes {
		b := f.Batches[grab+1+k]
		if len(b.Instances) != 1 || b.Instances[0].Kind.W != m || b.Target == 0 || b.Layer == 0 {
			t.Fatalf("pass %d: %+v", k, b)
		}
	}
	comp := f.Batches[grab+4]
	if comp.Target != 0 || comp.Instances[0].Kind.W != shaders.BlurComposite || comp.Layer != f.Batches[grab+3].Target {
		t.Fatalf("composite %+v", comp)
	}
	for _, b := range f.Batches[:grab] {
		if b.Target != 0 {
			t.Fatal("a layer batch was hoisted before the grab")
		}
	}
	mustSame(t, "blur frame", dev.img, fullRender(t, l, frostSize, 2))
	ref := paint.NewCPU()
	if err := ref.Render(l, frostSize, 2); err != nil {
		t.Fatal(err)
	}
	if res := compare(dev.img, ref.Image(), 2); res.Frac > 0.005 {
		t.Fatalf("differs from paint.CPU: %.3f%% over 2 (max %d)", res.Frac*100, res.MaxDiff)
	}
}

// dirtyCovers reports whether r lies within the union of rs.
func dirtyCovers(rs []image.Rectangle, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			in := false
			for _, d := range rs {
				if image.Pt(x, y).In(d) {
					in = true
					break
				}
			}
			if !in {
				return false
			}
		}
	}
	return true
}

func frostPlan(scale float32) paint.BlurPlan {
	s := scale
	p, _ := paint.PlanBackdropBlur(geom.Rect{X: frostBar.X * s, Y: frostBar.Y * s, W: frostBar.W * s, H: frostBar.H * s},
		paint.BlurSigma(6, s), image.Rect(0, 0, int(frostSize.W*s), int(frostSize.H*s)), image.Rect(0, 0, int(frostSize.W*s), int(frostSize.H*s)))
	return p
}

// TestBlurPartialDamage: a change under the blur's reach redraws the
// blur's whole source (strict verify mode compares with the full frame);
// a change out of its reach leaves the dirty rectangles alone.
func TestBlurPartialDamage(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		dev := &cpuDevice{}
		r := NewRenderer(dev)
		r.SetVerifyStrict(true)
		if err := r.Render(frostList(0, false, -1, nil), frostSize, scale); err != nil {
			t.Fatal(err)
		}
		// Row 7 (y 128..140) lies just above the bar, within its reach.
		row := geom.R(0, 7*18, 160, 18)
		for _, verify := range []bool{true, false} {
			r.SetVerify(verify)
			for k, hot := range []int{7, -1} {
				if err := r.Render(frostList(0, false, hot, func(l *paint.List) { l.Damage(row) }), frostSize, scale); err != nil {
					t.Fatalf("scale %v verify %v step %d: %v", scale, verify, k, err)
				}
				if !verify {
					f := dev.last
					if len(f.Dirty) == 0 {
						t.Fatalf("scale %v: not partial", scale)
					}
					if p := frostPlan(scale); !dirtyCovers(f.Dirty, p.Src) {
						t.Fatalf("scale %v: dirty %v does not cover the blur's source %v", scale, f.Dirty, p.Src)
					}
					mustSame(t, "damage under blur", dev.img, fullRender(t, frostList(0, false, hot, nil), frostSize, scale))
				}
			}
		}
		// Far from the bar: exactly the damage.
		r.SetVerify(false)
		far := geom.R(0, 0, 160, 18)
		if err := r.Render(frostList(0, false, 0, func(l *paint.List) { l.Damage(far) }), frostSize, scale); err != nil {
			t.Fatal(err)
		}
		want := image.Rect(0, 0, int(160*scale+0.99), int(18*scale+0.99))
		if f := dev.last; len(f.Dirty) != 1 || f.Dirty[0] != want {
			t.Fatalf("scale %v: dirty %v, want %v", scale, f.Dirty, want)
		}
		for _, b := range dev.last.Batches {
			if b.Grab != 0 {
				t.Fatalf("scale %v: the blur was redrawn for a change out of its reach", scale)
			}
		}
	}
}

// TestBlurPartialScroll: rows scrolling under a fixed frosted bar, and a
// frosted bar scrolling with the rows, pass strict verify mode; the
// scrolls still copy.
func TestBlurPartialScroll(t *testing.T) {
	// A fixed bar at scale 2 and a scrolling one at scale 1 (each frame
	// is drawn twice on the CPU executor in verify mode: kept small).
	for _, c := range []struct {
		inside bool
		scale  float32
	}{{false, 2}, {true, 1}} {
		inside, scale := c.inside, c.scale
		{
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerifyStrict(true)
			offs := []float32{0, 18, 54, 40, 0}
			if err := r.Render(frostList(offs[0], inside, -1, nil), frostSize, scale); err != nil {
				t.Fatal(err)
			}
			copied := false
			for _, verify := range []bool{true, false} {
				r.SetVerify(verify)
				for k := 1; k < len(offs); k++ {
					dy := offs[k-1] - offs[k]
					l := frostList(offs[k], inside, -1, func(l *paint.List) { l.Scroll(frostView, geom.Pt(0, dy)) })
					if err := r.Render(l, frostSize, scale); err != nil {
						t.Fatalf("inside %v scale %v verify %v step %d: %v", inside, scale, verify, k, err)
					}
					if !verify {
						copied = copied || len(dev.last.Copies) > 0
						mustSame(t, "scrolled", dev.img, fullRender(t, l, frostSize, scale))
					}
				}
				if verify {
					// Back to the start for the run without verify mode.
					if err := r.Render(frostList(offs[0], inside, -1, nil), frostSize, scale); err != nil {
						t.Fatal(err)
					}
				}
			}
			if !copied {
				t.Errorf("inside %v scale %v: no scroll was copied", inside, scale)
			}
		}
	}
}

// TestBlurPartialInGroup: a blur inside an opacity group redrawn
// partially equals the full frame.
func TestBlurPartialInGroup(t *testing.T) {
	mk := func(c color.NRGBA) *paint.List {
		l := frostList(0, false, -1, nil)
		l.Opacity(0.8)
		l.Fill(geom.R(20, 20, 60, 40), geom.Rad(8), c)
		l.BackdropBlur(geom.R(10, 10, 100, 60), geom.Rad(12), 4)
		l.Pop()
		return l
	}
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	r.SetVerifyStrict(true)
	if err := r.Render(mk(color.NRGBA{255, 0, 0, 255}), frostSize, 2); err != nil {
		t.Fatal(err)
	}
	l := mk(color.NRGBA{0, 160, 0, 255})
	l.Damage(geom.R(20, 20, 60, 40))
	if err := r.Render(l, frostSize, 2); err != nil {
		t.Fatal(err)
	}
}

// TestBlurExactCPU: over an exact backdrop (an opaque checkerboard) the
// GPU pipeline's blur on the CPU executor equals paint.CPU's bit for bit,
// at full resolution and downsampled (the passes round to whole levels
// with the same float32 arithmetic).
func TestBlurExactCPU(t *testing.T) {
	for i, s := range []float32{1, 1.5, 2} {
		for j, radius := range []float32{0.7, 2, 5, 12} {
			raceSkip(t, (i+j)%3 == 0)
			l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
			l.Checker(geom.R(0, 0, 96, 56), geom.Radii{}, 3, color.NRGBA{20, 120, 240, 255}, color.NRGBA{250, 30, 60, 255})
			l.BackdropBlur(geom.R(10, 10, 70, 30), geom.Radii{}, radius)
			ref := paint.NewCPU()
			if err := ref.Render(l, geom.Size{W: 96, H: 56}, s); err != nil {
				t.Fatal(err)
			}
			mustSame(t, "blur", fullRender(t, l, geom.Size{W: 96, H: 56}, s), ref.Image())
		}
	}
}
