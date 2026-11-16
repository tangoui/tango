package gpu

import (
	"image"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Solid-only windows must not allocate or upload a gradient table. The
// first later gradient must still upload its pixels even after empty frames.
func TestRampResourcesDeferredUntilGradient(t *testing.T) {
	dev := &cpuDevice{noRetain: true}
	r := NewRenderer(dev)
	solid := &paint.List{Background: white}
	solid.Fill(geom.R(0, 0, 16, 16), geom.Radii{}, blue)
	for range 2 {
		if err := r.Render(solid, geom.Size{W: 32, H: 32}, 1); err != nil {
			t.Fatal(err)
		}
		if len(r.ramps.pix) != 0 || dev.last.RampSize != (image.Point{}) || len(rampUploads(dev.last)) != 0 {
			t.Fatalf("unused ramp retained %d CPU bytes, texture %v, uploads %d", len(r.ramps.pix), dev.last.RampSize, len(rampUploads(dev.last)))
		}
	}
	gradient := &paint.List{Background: white}
	gradient.Add(paint.LinearGradient{Rect: geom.R(0, 0, 32, 32), P0: geom.Pt(0, 0), P1: geom.Pt(32, 0), From: blue, To: red})
	if err := r.Render(gradient, geom.Size{W: 32, H: 32}, 1); err != nil {
		t.Fatal(err)
	}
	if len(rampUploads(dev.last)) == 0 {
		t.Fatal("first gradient did not upload its table")
	}
	ref := paint.NewCPU()
	if err := ref.Render(gradient, geom.Size{W: 32, H: 32}, 1); err != nil {
		t.Fatal(err)
	}
	if diff := compare(dev.img, ref.Image(), 2); diff.Frac > 0.005 {
		t.Fatalf("first gradient differs in %.3f%% pixels", diff.Frac*100)
	}
}

func TestPatchResourcesDeferredUntilUse(t *testing.T) {
	dev := &cpuDevice{noRetain: true}
	r := NewRenderer(dev)
	if err := r.Render(&paint.List{Background: white}, geom.Size{W: 32, H: 32}, 1); err != nil {
		t.Fatal(err)
	}
	if dev.last.ColorSize != (image.Point{}) {
		t.Fatalf("unused patch texture %v", dev.last.ColorSize)
	}
}

// A single complex frame must not pin its enlarged patch texture for the
// remaining lifetime of a window that returns to native drawing.
func TestPatchResourcesReleasedAfterIdle(t *testing.T) {
	dev := &cpuDevice{noRetain: true}
	r := NewRenderer(dev)
	solid := &paint.List{Background: white}
	size := geom.Size{W: 800, H: 800}
	patch := &paint.List{Background: white}
	patch.Transform(geom.RotateAround(0.03, geom.Pt(400, 400)))
	patch.Fill(geom.R(50, 50, 700, 700), geom.Radii{}, blue)
	patch.Pop()
	for range 4 {
		if err := r.Render(patch, size, 1); err != nil {
			t.Fatal(err)
		}
	}
	grown := dev.last.ColorSize
	if grown.X < 512 {
		t.Fatalf("fixture did not grow patch page: %v", grown)
	}
	if err := r.Render(solid, size, 1); err != nil {
		t.Fatal(err)
	}
	if dev.last.ColorSize != grown {
		t.Fatalf("patch texture released before idle grace period: %v -> %v", grown, dev.last.ColorSize)
	}
	for range 120 {
		if err := r.Render(solid, size, 1); err != nil {
			t.Fatal(err)
		}
	}
	if dev.last.ColorSize != (image.Point{}) {
		t.Fatalf("idle patch texture retained: %v", dev.last.ColorSize)
	}
	if err := r.Render(patch, size, 1); err != nil {
		t.Fatal(err)
	}
	ref := paint.NewCPU()
	if err := ref.Render(patch, size, 1); err != nil {
		t.Fatal(err)
	}
	if diff := compare(dev.img, ref.Image(), 2); diff.Frac > 0.005 {
		t.Fatalf("rebuilt patch differs in %.3f%% pixels", diff.Frac*100)
	}
}

func TestTrimDropsOptionalResources(t *testing.T) {
	dev := &cpuDevice{noRetain: true}
	r := NewRenderer(dev)
	l := &paint.List{Background: white}
	l.Add(paint.LinearGradient{Rect: geom.R(0, 0, 32, 32), P0: geom.Pt(0, 0), P1: geom.Pt(32, 0), From: blue, To: red})
	l.Transform(geom.RotateAround(0.1, geom.Pt(16, 16)))
	l.Fill(geom.R(4, 4, 24, 24), geom.Radii{}, blue)
	l.Pop()
	size := geom.Size{W: 32, H: 32}
	if err := r.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	before := trimClone(dev.img)
	r.Trim()
	if len(r.ramps.pix) != 0 {
		t.Fatalf("Trim retained %d ramp bytes", len(r.ramps.pix))
	}
	if err := r.Render(&paint.List{Background: white}, size, 1); err != nil {
		t.Fatal(err)
	}
	if dev.last.ColorSize != (image.Point{}) || dev.last.RampSize != (image.Point{}) {
		t.Fatalf("Trim retained optional textures: color %v ramp %v", dev.last.ColorSize, dev.last.RampSize)
	}
	if err := r.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	if diff := trimDiff(before, dev.img); diff != 0 {
		t.Fatalf("Trim rebuild changed %d pixels", diff)
	}
}
