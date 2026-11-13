//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/gpuconf"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/shader/sl"
)

// TestD3DLayerPoolReuse: layer textures come from a pool reused by size:
// ten frames of the nested-groups scene (three layers each) and of the
// two-group gradient scene leave the number of layer textures the device
// holds from growing after the first frame. (That the frames stay correct
// is gpuconf's LayerPoolReuse.)
func TestD3DLayerPoolReuse(t *testing.T) {
	d := newTestDevice(t)
	r := gpu.NewRenderer(d)
	for _, name := range []string{"opacitynest", "opacitygrad"} {
		sc := gputest.SceneNamed(name)
		l := sc.Make(t)
		first := -1
		for frame := range 10 {
			if err := r.Render(l, gputest.SceneSize, 2); err != nil {
				t.Fatal(err)
			}
			n := d.layerTextures()
			if first < 0 {
				first = n
			}
			if n == 0 || n > first {
				t.Fatalf("%s frame %d: %d layer textures (first frame %d)", name, frame, n, first)
			}
		}
		t.Logf("%s: %d layer textures", name, first)
	}
}

// stripes is a 64×192 frame of 8-pixel horizontal stripes of distinct
// colors, scrolled by off pixels (stripe k at y = 8k - off), drawn whole
// or, with dirty set, as a partial frame copying copies first.
func stripes(off int, dirty []image.Rectangle, copies []gpu.CopyRegion) *gpu.Frame {
	f := &gpu.Frame{Size: image.Pt(64, 192), Clear: color.NRGBA{255, 255, 255, 255}, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4), Dirty: dirty, Copies: copies}
	var ins []shaders.Instance
	for k := range 40 {
		y := 8*k - off
		c := sl.V4(float32(k*37%256)/255, float32(k*91%256)/255, float32(k*13%256)/255, 1)
		ins = append(ins, shaders.Instance{Rect: sl.V4(0, float32(y), 64, 8), Color: c, Kind: sl.V4(shaders.KindSolid, 0, 0, 0)})
	}
	f.Batches = []gpu.Batch{{Instances: ins[:20]}, {Instances: ins[20:], Scissor: image.Rect(0, 0, 64, 192)}}
	return f
}

// TestD3DScrollOverlap: a partial frame that moves the target's pixels up
// or down by 40 (source and destination overlapping) and redraws the
// exposed strip gives exactly the full frame at the new offset; so does
// the Renderer's scroll path (verify mode compares it with the full frame
// on the device).
func TestD3DScrollOverlap(t *testing.T) {
	d := newTestDevice(t)
	ref := newTestDevice(t)
	const h = 192
	for _, dy := range []int{40, -40} { // content moves up (scroll down), then down
		if err := d.Execute(stripes(80, nil, nil)); err != nil {
			t.Fatal(err)
		}
		off := 80 + dy
		var cp gpu.CopyRegion
		var dirty image.Rectangle
		if dy > 0 {
			cp = gpu.CopyRegion{Src: image.Rect(0, dy, 64, h), Dst: image.Rect(0, 0, 64, h-dy)}
			dirty = image.Rect(0, h-dy, 64, h)
		} else {
			cp = gpu.CopyRegion{Src: image.Rect(0, 0, 64, h+dy), Dst: image.Rect(0, -dy, 64, h)}
			dirty = image.Rect(0, 0, 64, -dy)
		}
		if !d.Retained(image.Pt(64, h)) {
			t.Fatal("target not retained after a full frame")
		}
		if err := d.Execute(stripes(off, []image.Rectangle{dirty}, []gpu.CopyRegion{cp})); err != nil {
			t.Fatalf("dy %d: %v", dy, err)
		}
		if err := ref.Execute(stripes(off, nil, nil)); err != nil {
			t.Fatal(err)
		}
		got, want := readPixels(t, d), readPixels(t, ref)
		for y := range h {
			for x := range 64 {
				if g, w := got.RGBAAt(x, y), want.RGBAAt(x, y); g != w {
					t.Fatalf("dy %d: pixel %d,%d = %v, want %v", dy, x, y, g, w)
				}
			}
		}
	}

	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			d := newTestDevice(t)
			lf := &gpuconf.LastFrame{Device: d}
			r := gpu.NewRenderer(lf)
			r.SetVerify(true)
			prev := float32(0)
			if err := r.Render(gpuconf.ScrollList(t, prev, nil), gpuconf.ScrollSize, scale); err != nil {
				t.Fatal(err)
			}
			for _, off := range []float32{40, 0} {
				dy := prev - off
				l := gpuconf.ScrollList(t, off, func(l *paint.List) { l.Scroll(gpuconf.ScrollViewport, geom.Pt(0, dy)) })
				if err := r.Render(l, gpuconf.ScrollSize, scale); err != nil {
					t.Fatalf("scroll %v: %v", dy, err)
				}
				// Verify mode follows the partial frame with a full one:
				// check the copies of the partial frame.
				if len(lf.PartialCopies) != 1 {
					t.Fatalf("scroll %v: copies %v, want one", dy, lf.PartialCopies)
				}
				prev = off
			}
		})
	}
}

// TestD3DCopiesWithoutDirtyRejected: a frame with Copies but no Dirty is
// refused (not executed as a full frame, which would clear the target),
// leaves the target as it was and is not a GPU failure.
func TestD3DCopiesWithoutDirtyRejected(t *testing.T) {
	d := newTestDevice(t)
	if err := d.Execute(stripes(0, nil, nil)); err != nil {
		t.Fatal(err)
	}
	before := readPixels(t, d)
	f := stripes(40, nil, []gpu.CopyRegion{{Src: image.Rect(0, 40, 64, 192), Dst: image.Rect(0, 0, 64, 152)}})
	f.Clear = color.NRGBA{255, 0, 0, 255}
	err := d.Execute(f)
	if err == nil || errors.Is(err, ErrDeviceLost) || errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("Copies without Dirty: %v, want an invalid-frame error", err)
	}
	after := readPixels(t, d)
	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			t.Fatalf("the refused frame changed the target at byte %d", i)
		}
	}
	if d.health.failures != 0 || !d.Retained(image.Pt(64, 192)) {
		t.Fatalf("failures %d, retained %v", d.health.failures, d.Retained(image.Pt(64, 192)))
	}
}

// TestD3DScratchLimits: the copy scratch texture is created for copies,
// replaced by a smaller one when it is more than four times the area
// needed, released after scratchIdleFrames frames without copies, and
// copies whose stacked sources exceed the texture limit are refused with
// ErrNotRetained (drawn in full by the Renderer), not as a failure.
func TestD3DScratchLimits(t *testing.T) {
	d := newTestDevice(t)
	full := stripes(0, nil, nil)
	if err := d.Execute(full); err != nil {
		t.Fatal(err)
	}
	partial := func(cp gpu.CopyRegion) *gpu.Frame {
		return stripes(0, []image.Rectangle{image.Rect(0, 0, 1, 1)}, []gpu.CopyRegion{cp})
	}
	if err := d.Execute(partial(gpu.CopyRegion{Src: image.Rect(0, 40, 64, 192), Dst: image.Rect(0, 0, 64, 152)})); err != nil {
		t.Fatal(err)
	}
	if got := d.scratchBytes(); got != texBytes(image.Pt(64, 152)) {
		t.Fatalf("scratch %d bytes, want %d", got, texBytes(image.Pt(64, 152)))
	}
	if err := d.Execute(partial(gpu.CopyRegion{Src: image.Rect(0, 10, 16, 20), Dst: image.Rect(0, 30, 16, 40)})); err != nil {
		t.Fatal(err)
	}
	if got := d.scratchBytes(); got != texBytes(image.Pt(16, 10)) {
		t.Fatalf("after a small copy the scratch is %d bytes, want %d", got, texBytes(image.Pt(16, 10)))
	}
	for i := range scratchIdleFrames + 1 {
		if err := d.Execute(full); err != nil {
			t.Fatal(err)
		}
		if i < scratchIdleFrames-1 && d.scratchBytes() == 0 {
			t.Fatalf("scratch released after %d idle frames", i+1)
		}
	}
	if d.scratchBytes() != 0 {
		t.Fatalf("scratch of %d bytes kept after %d idle frames", d.scratchBytes(), scratchIdleFrames+1)
	}

	// Two full-height copies stack to twice the texture limit.
	lim := d.maxTex
	tall := &gpu.Frame{Size: image.Pt(16, lim), Clear: color.NRGBA{255, 255, 255, 255}, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}
	if err := d.Execute(tall); err != nil {
		t.Fatal(err)
	}
	tall.Dirty = []image.Rectangle{image.Rect(0, 0, 1, 1)}
	tall.Copies = []gpu.CopyRegion{
		{Src: image.Rect(0, 0, 4, lim), Dst: image.Rect(4, 0, 8, lim)},
		{Src: image.Rect(8, 0, 12, lim), Dst: image.Rect(12, 0, 16, lim)},
	}
	err := d.Execute(tall)
	if !errors.Is(err, gpu.ErrNotRetained) || errors.Is(err, ErrDeviceLost) {
		t.Fatalf("copies stacking past the limit: %v, want ErrNotRetained", err)
	}
	if d.health.failures != 0 || d.scratchBytes() != 0 {
		t.Fatalf("failures %d, scratch %d bytes", d.health.failures, d.scratchBytes())
	}
}
