package cpuexec_test

import (
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestDeviceMatchesGPUTests: Device is the device ui/gpu's own tests run
// on (their cpuDevice, through cpuexec_bridge_test.go), so the renderer's
// partial-frame tests there exercise it. This test runs the same kind of
// frames through it from outside: a full frame, a partial frame from a
// Damage hint, a partial frame with a copy from a ScrollRegion, all in
// strict verify mode (each partial frame compared with a full redraw) and
// against paint.CPU, and the frame contract checks.
func TestDeviceMatchesGPUTests(t *testing.T) {
	size := geom.Size{W: 64, H: 48}
	bg := color.NRGBA{250, 250, 250, 255}
	blue := color.NRGBA{20, 60, 200, 255}
	red := color.NRGBA{200, 30, 30, 255}
	// scene draws rows of boxes scrolled up by off pixels, with a box at
	// mark that does not scroll.
	scene := func(off float32, mark geom.Rect, hints func(l *paint.List)) *paint.List {
		l := &paint.List{Background: bg}
		l.Clip(geom.Rect{W: size.W, H: size.H}, geom.Radii{})
		for i := range 12 {
			l.Fill(geom.Rect{X: 4, Y: float32(i*8) - off, W: float32(10 + 2*i), H: 6}, geom.Radii{}, blue)
		}
		l.Pop()
		l.Fill(mark, geom.Radii{}, red)
		if hints != nil {
			hints(l)
		}
		return l
	}
	for _, scale := range []float32{1, 2} {
		dev := cpuexec.NewDevice()
		r := gpu.NewRenderer(dev)
		r.SetVerify(true)
		r.SetVerifyStrict(true)
		render := func(l *paint.List) {
			t.Helper()
			if err := r.Render(l, size, scale); err != nil {
				t.Fatalf("scale %v: %v", scale, err)
			}
			ref := paint.NewCPU()
			if err := ref.Render(l, size, scale); err != nil {
				t.Fatal(err)
			}
			got, err := dev.ReadPixels()
			if err != nil {
				t.Fatal(err)
			}
			if d := maxDiff(got, ref.Image()); d > 2 {
				t.Fatalf("scale %v: differs from paint.CPU by %d", scale, d)
			}
		}
		mark := geom.Rect{X: 40, Y: 4, W: 8, H: 8}
		render(scene(0, mark, nil))
		if s := dev.Stats(); s.Executed != 1 || s.Partial != 0 || len(dev.Last().Dirty) != 0 {
			t.Fatalf("first frame: %+v", s)
		}
		// The mark moves: a partial frame redrawing its old and new place.
		moved := geom.Rect{X: 40, Y: 20, W: 8, H: 8}
		render(scene(0, moved, func(l *paint.List) { l.Damage(mark); l.Damage(moved) }))
		if s := dev.Stats(); s.Partial != 1 {
			t.Fatalf("damage frame not partial: %+v", s)
		}
		// The rows scroll by 8 pixels: a copy and the exposed strip, and the
		// fixed mark redrawn where the copy moved it.
		render(scene(8, moved, func(l *paint.List) {
			l.Scroll(geom.Rect{W: 36, H: size.H}, geom.Pt(0, -8))
			l.Damage(geom.Rect{Y: size.H - 8, W: 36, H: 8})
		}))
		if s := dev.Stats(); s.Partial < 2 {
			t.Fatalf("scroll frame not partial: %+v", s)
		}
		if n, last := r.VerifyFailures(); n != 0 {
			t.Fatalf("scale %v: verify failures %d, last %v", scale, n, last)
		}
	}

	// Contract checks.
	dev := cpuexec.NewDevice()
	f := &gpu.Frame{Size: image.Pt(8, 8), Clear: bg, Dirty: []image.Rectangle{image.Rect(0, 0, 2, 2)}}
	if err := dev.Execute(f); !errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("partial frame without a target: %v", err)
	}
	full := &gpu.Frame{Size: image.Pt(8, 8), Clear: bg}
	if err := dev.Execute(full); err != nil {
		t.Fatal(err)
	}
	if !dev.Retained(image.Pt(8, 8)) || dev.Retained(image.Pt(9, 8)) {
		t.Fatal("Retained")
	}
	dev.NoRetain = true
	if dev.Retained(image.Pt(8, 8)) {
		t.Fatal("NoRetain: still retained")
	}
	dev.NoRetain = false
	bad := []*gpu.Frame{
		{Size: image.Pt(8, 8), Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 2, 2), Dst: image.Rect(2, 2, 4, 4)}}},
		{Size: image.Pt(8, 8), Dirty: []image.Rectangle{image.Rect(0, 0, 4, 4), image.Rect(2, 2, 6, 6)}},
		{Size: image.Pt(8, 8), Dirty: []image.Rectangle{image.Rect(0, 0, 9, 4)}},
		{Size: image.Pt(8, 8), Dirty: []image.Rectangle{image.Rect(0, 0, 2, 2)}, Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 2, 2), Dst: image.Rect(2, 2, 5, 5)}}},
	}
	for i, f := range bad {
		if err := dev.Execute(f); err == nil {
			t.Errorf("bad frame %d accepted", i)
		}
	}
}

func maxDiff(a, b *image.RGBA) int {
	if a.Rect != b.Rect {
		return 256
	}
	worst := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		worst = max(worst, d, -d)
	}
	return worst
}
