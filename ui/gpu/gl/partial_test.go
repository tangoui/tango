//go:build linux || darwin

package gl

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

// TestGLLayerPoolReuse: layer textures come from a pool reused by size:
// ten frames of the nested-groups scene (three layers each) and of the
// two-group gradient scene leave the number of layer textures the device
// holds from growing after the first frame. (That the frames stay correct
// is gpuconf's LayerPoolReuse.)
func TestGLLayerPoolReuse(t *testing.T) {
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

// TestGLScrollOverlap: a partial frame that moves the target's pixels up
// or down by 40 (source and destination overlapping) and redraws the
// exposed strip gives exactly the full frame at the new offset; so does
// the Renderer's scroll path (verify mode compares it with the full frame
// on the device).
func TestGLScrollOverlap(t *testing.T) {
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
				lf.PartialCopies = lf.PartialCopies[:0]
				if err := r.Render(l, gpuconf.ScrollSize, scale); err != nil {
					t.Fatalf("scroll %v: %v", dy, err)
				}
				// Verify mode follows the partial frame with a full one:
				// the copies are those of the partial frame.
				if len(lf.PartialCopies) != 1 {
					t.Fatalf("scroll %v: copies %v, want one", dy, lf.PartialCopies)
				}
				prev = off
			}
		})
	}
}

// TestGLCopiesWithoutDirtyRejected: a frame with Copies but no Dirty is
// refused (not executed as a full frame, which would clear the target),
// leaves the target as it was and is not a GPU failure.
func TestGLCopiesWithoutDirtyRejected(t *testing.T) {
	d := newTestDevice(t)
	if err := d.Execute(stripes(0, nil, nil)); err != nil {
		t.Fatal(err)
	}
	before := readPixels(t, d)
	f := stripes(40, nil, []gpu.CopyRegion{{Src: image.Rect(0, 40, 64, 192), Dst: image.Rect(0, 0, 64, 152)}})
	f.Clear = color.NRGBA{255, 0, 0, 255}
	err := d.Execute(f)
	if err == nil || errors.Is(err, gpu.ErrDeviceLost) || errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("Copies without Dirty: %v, want an invalid-frame error", err)
	}
	after := readPixels(t, d)
	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			t.Fatalf("the refused frame changed the target at byte %d", i)
		}
	}
	if d.failures != 0 || !d.Retained(image.Pt(64, 192)) {
		t.Fatalf("failures %d, retained %v", d.failures, d.Retained(image.Pt(64, 192)))
	}
}

// TestGLScratchLimits: the copy scratch texture is created for copies,
// replaced by a smaller one when it is more than four times the area
// needed, released after scratchIdleFrames frames without copies, and
// copies whose stacked sources exceed the texture limit are refused with
// ErrNotRetained (drawn in full by the Renderer), not as a failure.
func TestGLScratchLimits(t *testing.T) {
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

	// Two full-height copies stack to twice the texture limit (lowered
	// to 256 so the test needs no 16384-pixel target).
	const lim = 256
	maxTextureOverride = lim
	t.Cleanup(func() { maxTextureOverride = 0 })
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
	if !errors.Is(err, gpu.ErrNotRetained) || errors.Is(err, gpu.ErrDeviceLost) {
		t.Fatalf("copies stacking past the limit: %v, want ErrNotRetained", err)
	}
	if d.failures != 0 || d.scratchBytes() != 0 {
		t.Fatalf("failures %d, scratch %d bytes", d.failures, d.scratchBytes())
	}
	if !d.Retained(image.Pt(16, lim)) {
		t.Fatal("a refused partial frame dropped the retained frame")
	}
}

// TestGLFailedFrameNotRetained: a frame ending with a GL error may have
// drawn part of itself into the target, so the target holds no frame:
// Retained is false and a partial frame is refused with
// gpu.ErrNotRetained (Metal's gpuFailed), until a full frame succeeds.
// A recreated target is not retained either.
func TestGLFailedFrameNotRetained(t *testing.T) {
	d := newTestDevice(t)
	size := image.Pt(64, 192)
	if d.Retained(size) {
		t.Fatal("retained before any frame")
	}
	if err := d.Execute(stripes(0, nil, nil)); err != nil {
		t.Fatal(err)
	}
	if !d.Retained(size) || d.Retained(image.Pt(64, 191)) {
		t.Fatalf("retained %v at the frame's size, %v at another", d.Retained(size), d.Retained(image.Pt(64, 191)))
	}
	fail := true
	setInjectGLError(t, func() uint32 {
		if fail {
			return glINVALID_OPERATION
		}
		return glNO_ERROR
	})
	if err := d.Execute(stripes(0, []image.Rectangle{image.Rect(0, 0, 8, 8)}, nil)); err == nil {
		t.Fatal("frame with a GL error succeeded")
	}
	fail = false
	if d.Retained(size) {
		t.Fatal("retained after a failed frame")
	}
	if err := d.Execute(stripes(0, []image.Rectangle{image.Rect(0, 0, 8, 8)}, nil)); !errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("partial frame after a failed frame: %v, want ErrNotRetained", err)
	}
	if err := d.Execute(stripes(0, nil, nil)); err != nil {
		t.Fatal(err)
	}
	if !d.Retained(size) {
		t.Fatal("not retained after a successful full frame")
	}
	if err := d.Execute(&gpu.Frame{Size: image.Pt(8, 8)}); err != nil {
		t.Fatal(err)
	}
	if d.Retained(size) {
		t.Fatal("retained after the target was recreated")
	}
	d.Close()
	if d.Retained(image.Pt(8, 8)) {
		t.Fatal("retained after Close")
	}
}

// TestGLLayerTargetSwitches: a frame alternating between the main target
// and two layers several times (each layer sampled by the main target,
// the second layer reusing the first layer's pooled texture) composites
// each layer's own contents: switching targets never samples a texture
// that is being rendered to. Each layer holds a solid 8×8 square and is
// composited opaque; the result is compared pixel by pixel.
func TestGLLayerTargetSwitches(t *testing.T) {
	d := newTestDevice(t)
	solid := func(x, y float32, c sl.Vec4) shaders.Instance {
		return shaders.Instance{Rect: sl.V4(x, y, 8, 8), Color: c, Kind: sl.V4(shaders.KindSolid, 0, 0, 0)}
	}
	red, green := sl.V4(1, 0, 0, 1), sl.V4(0, 1, 0, 1)
	// Each layer has a transparent margin of 2 pixels right of and below
	// its square: a composite reaches a pixel past its bounds (its edge
	// antialiasing samples the clamped edge texel).
	l1 := gpu.LayerDesc{Size: image.Pt(10, 10), Bounds: image.Rect(0, 0, 10, 10)}
	l2 := gpu.LayerDesc{Size: image.Pt(10, 10), Bounds: image.Rect(16, 0, 26, 10)}
	f := &gpu.Frame{
		Size: image.Pt(32, 12), Clear: white, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4),
		Layers: []gpu.LayerDesc{l1, l2},
		Batches: []gpu.Batch{
			{Target: 1, Instances: []shaders.Instance{solid(0, 0, red)}},
			{Layer: 1, Instances: []shaders.Instance{gpuconf.LayerComposite(l1, 1)}},
			{Target: 2, Instances: []shaders.Instance{solid(16, 0, green)}},
			{Layer: 2, Instances: []shaders.Instance{gpuconf.LayerComposite(l2, 1)}},
		},
	}
	for frame := range 3 {
		if err := d.Execute(f); err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
		img := readPixels(t, d)
		for x := range 32 {
			for y := range 12 {
				want := color.RGBA(white)
				switch {
				case y >= 8:
				case x < 8:
					want = color.RGBA{255, 0, 0, 255}
				case x >= 16 && x < 24:
					want = color.RGBA{0, 255, 0, 255}
				}
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("frame %d: pixel %d,%d = %v, want %v", frame, x, y, got, want)
				}
			}
		}
	}
	if n := d.layerTextures(); n != 1 {
		t.Fatalf("%d layer textures, want 1 (the second layer reuses the first's)", n)
	}
}
