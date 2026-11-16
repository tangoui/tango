//go:build darwin

package metal

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gpuconf"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func copyTargetSeed(size image.Point) gpu.Frame {
	f := gpu.Frame{Size: size, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4), Clear: color.NRGBA{R: 39, G: 73, B: 101, A: 137}}
	var instances []shaders.Instance
	for y := 0; y < size.Y; y += 8 {
		for x := 0; x < size.X; x += 8 {
			instances = append(instances, shaders.Instance{
				Rect:  sl.V4(float32(x), float32(y), 8, 8),
				Color: sl.V4(float32(x%96+1)/256, float32(y%64+1)/256, .125, .5),
				Kind:  sl.V4(shaders.KindSolid, 0, 0, 0),
			})
		}
	}
	f.Batches = []gpu.Batch{{Instances: instances}}
	return f
}

func copyTargetFrame(size image.Point) gpu.Frame {
	f := copyTargetSeed(size)
	f.Copies = []gpu.CopyRegion{{Src: image.Rect(0, 8, size.X-8, size.Y), Dst: image.Rect(0, 0, size.X-8, size.Y-8)}}
	f.Dirty = []image.Rectangle{image.Rect(1, size.Y-7, 31, size.Y-1), image.Rect(size.X-13, 1, size.X-1, 7)}
	f.Clear = color.NRGBA{R: 190, G: 80, B: 40, A: 83}
	f.Batches = []gpu.Batch{{Instances: []shaders.Instance{{Rect: sl.V4(2, 2, float32(size.X-6), float32(size.Y-4)), Color: sl.V4(.125, .25, .375, .5), Kind: sl.V4(shaders.KindSolid, 0, 0, 0)}}}}
	return f
}

// executeScratchReference preserves the old all-sources-first algorithm
// independently of Execute's copy-strategy choice. Its extra CB is ONLY a
// correctness reference, never a performance measurement or completion trick.
func executeScratchReference(t *testing.T, d *Device, f *gpu.Frame) {
	t.Helper()
	if len(f.Copies) > 0 {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer autoreleasePool()()
		if err := d.Wait(); err != nil {
			t.Fatal(err)
		}
		if err := d.ensureScratch(f.Copies); err != nil {
			t.Fatal(err)
		}
		cb := d.queue.Send(sel("commandBuffer"))
		if cb == 0 {
			t.Fatal("reference command buffer")
		}
		d.copyRegions(cb, f.Copies)
		d.commit(cb)
	}
	frame := *f
	frame.Copies = nil
	if err := d.Execute(&frame); err != nil {
		t.Fatal(err)
	}
}

func checkCopyTargetPixels(t *testing.T, d, ref *Device) {
	t.Helper()
	got, want := readPixels(t, d), readPixels(t, ref)
	if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("production copy target differs from all-old-sources scratch reference")
	}
}

// This test exercises production Execute, not the opt-in diagnostic replay.
// In addition to exact pixels, it must reject a two-blit implementation for
// large moves by requiring the destination target identity to change.
func TestMetalCopyTargetPixels(t *testing.T) {
	size := image.Pt(96, 64)
	cases := []struct {
		name   string
		copies []gpu.CopyRegion
	}{
		{"up", []gpu.CopyRegion{{Src: image.Rect(0, 8, 80, 62), Dst: image.Rect(0, 0, 80, 54)}}},
		{"down", []gpu.CopyRegion{{Src: image.Rect(4, 0, 90, 56), Dst: image.Rect(4, 8, 90, 64)}}},
		{"horizontal", []gpu.CopyRegion{{Src: image.Rect(7, 4, 96, 60), Dst: image.Rect(0, 4, 89, 60)}}},
		{"multi", []gpu.CopyRegion{
			{Src: image.Rect(0, 4, 40, 60), Dst: image.Rect(0, 0, 40, 56)},
			{Src: image.Rect(48, 0, 90, 56), Dst: image.Rect(48, 8, 90, 64)},
		}},
		// Stronger than the renderer's non-intersecting-copy contract:
		// cross-copy cycles still read ALL sources from the old target.
		{"cyclic", []gpu.CopyRegion{
			{Src: image.Rect(0, 0, 40, 64), Dst: image.Rect(48, 0, 88, 64)},
			{Src: image.Rect(48, 0, 88, 64), Dst: image.Rect(0, 0, 40, 64)},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, ref := newDevice(t), newDevice(t)
			d.copyStats = &copyStats{}
			seed := copyTargetSeed(size)
			if err := d.Execute(&seed); err != nil {
				t.Fatal(err)
			}
			executeScratchReference(t, ref, &seed)
			f := copyTargetFrame(size)
			f.Copies = c.copies
			for range 4 {
				before := d.target
				if err := d.Execute(&f); err != nil {
					t.Fatal(err)
				}
				if d.target == before {
					t.Fatal("large copy must write a distinct target, not double-blit back")
				}
				if s := d.copyStats; s.pixels != int64(size.X)*int64(size.Y) || s.encoders != 1 || s.blits != len(d.copyKeep)+len(f.Copies) {
					t.Fatalf("copy work %+v, keep=%v", s, d.copyKeep)
				}
				executeScratchReference(t, ref, &f)
				checkCopyTargetPixels(t, d, ref)
			}
		})
	}
}

func TestMetalCopyTargetPlan(t *testing.T) {
	d := &Device{}
	f := copyTargetFrame(image.Pt(96, 64))
	if !d.planCopyTarget(&f) {
		t.Fatal("large copy not planned")
	}
	f.Copies = []gpu.CopyRegion{{Src: image.Rect(0, 0, 48, 64), Dst: image.Rect(48, 0, 96, 64)}}
	if d.planCopyTarget(&f) {
		t.Fatal("equal cost must keep scratch")
	}
	f.Copies[0].Src.Max.X--
	f.Copies[0].Dst.Max.X--
	if d.planCopyTarget(&f) {
		t.Fatal("small copy would copy more pixels with ping-pong")
	}
	f.Copies = []gpu.CopyRegion{
		{Src: image.Rect(0, 0, 90, 60), Dst: image.Rect(0, 0, 90, 60)},
		{Src: image.Rect(0, 0, 90, 60), Dst: image.Rect(1, 1, 91, 61)},
	}
	if d.planCopyTarget(&f) {
		t.Fatal("overlapping destinations must use snapshot scratch")
	}
	f = copyTargetFrame(image.Pt(128, 128))
	f.Copies = []gpu.CopyRegion{{Src: image.Rect(0, 0, 64, 128), Dst: image.Rect(0, 0, 64, 128)}}
	for i := range 63 {
		r := image.Rect(65+(i%31)*2, 2+(i/31)*3, 66+(i%31)*2, 3+(i/31)*3)
		f.Copies = append(f.Copies, gpu.CopyRegion{Src: r, Dst: r})
	}
	if d.planCopyTarget(&f) {
		t.Fatal("fragmented complement must hit the hard planning bound")
	}
	f.Copies = append(f.Copies, gpu.CopyRegion{Src: image.Rect(0, 0, 1, 1), Dst: image.Rect(0, 0, 1, 1)})
	if d.planCopyTarget(&f) {
		t.Fatal("unbounded copy count must not enter planner")
	}
	f = copyTargetFrame(image.Pt(96, 64))
	f.Size.X = int(^uint(0) >> 1)
	if d.planCopyTarget(&f) {
		t.Fatal("oversized dimensions must not overflow the cost comparison")
	}
}

func TestMetalCopyTargetStackLimit(t *testing.T) {
	d := newDevice(t)
	d.copyStats = &copyStats{}
	size := image.Pt(64, 9000) // small allocation, but sources stack past 16384
	seed := copyTargetSeed(size)
	if err := d.Execute(&seed); err != nil {
		t.Fatal(err)
	}
	old := readPixels(t, d)
	f := gpu.Frame{Size: size, MaskSize: seed.MaskSize, ColorSize: seed.ColorSize, Clear: color.NRGBA{R: 70, G: 20, B: 90, A: 255},
		Dirty: []image.Rectangle{image.Rect(0, 8992, 64, 9000)},
		Copies: []gpu.CopyRegion{
			{Src: image.Rect(0, 400, 32, 8600), Dst: image.Rect(0, 0, 32, 8200)},
			{Src: image.Rect(32, 400, 64, 8600), Dst: image.Rect(32, 0, 64, 8200)},
		}}
	want := image.NewRGBA(old.Rect)
	copy(want.Pix, old.Pix)
	for _, c := range f.Copies {
		draw.Draw(want, c.Dst, old, c.Src.Min, draw.Src)
	}
	for _, r := range f.Dirty {
		draw.Draw(want, r, image.NewUniform(f.Clear), image.Point{}, draw.Src)
	}
	if err := d.Execute(&f); err != nil {
		t.Fatalf("legal large copies must not hit legacy stack limit: %v", err)
	}
	if got := readPixels(t, d); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("large-copy source/retained strip mismatch")
	}
	if d.copyStats.pixels != int64(size.X)*int64(size.Y) || d.copyStats.encoders != 1 {
		t.Fatal("large legal copies did not use bounded one-target work")
	}
}

func TestMetalCopyTargetRetinaCost(t *testing.T) {
	d := newDevice(t)
	d.copyStats = &copyStats{}
	f := gpu.Frame{Size: image.Pt(3840, 2160), MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	f.Copies = []gpu.CopyRegion{{Src: image.Rect(0, 84, 3816, 2158), Dst: image.Rect(0, 0, 3816, 2074)}}
	f.Dirty = []image.Rectangle{image.Rect(0, 2074, 3816, 2158)}
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	if s := d.copyStats; s.pixels != 3840*2160 || s.blits != 3 || s.encoders != 1 {
		t.Fatalf("Retina scrolling copy work %+v, want 8294400 pixels / 3 blits / 1 encoder", s)
	}
	t.Logf("production Retina scroll: %+v, copied bytes=%d, spare bytes=%d", d.copyStats, d.copyStats.pixels*4, texBytes(d.copyTargetSize))
}

func TestMetalCopyTargetTransitions(t *testing.T) {
	d, ref := newDevice(t), newDevice(t)
	d.copyStats = &copyStats{}
	run := func(f gpu.Frame, wantSwap bool) {
		t.Helper()
		old := d.target
		if err := d.Execute(&f); err != nil {
			t.Fatal(err)
		}
		executeScratchReference(t, ref, &f)
		if len(f.Dirty) > 0 && (d.target != old) != wantSwap {
			t.Fatalf("unexpected swap=%v", d.target != old)
		}
		if len(f.Copies) == 0 && d.copyStats.pixels != 0 {
			t.Fatal("ordinary frame must not copy the whole target")
		}
		checkCopyTargetPixels(t, d, ref)
	}
	size := image.Pt(96, 64)
	run(copyTargetSeed(size), false)
	f := copyTargetFrame(size)
	run(f, true)
	spare := d.copyTarget
	normal := f
	normal.Copies = nil
	run(normal, false)
	run(copyTargetSeed(size), false)
	before, commits := d.target, d.commits
	if err := d.Execute(&gpu.Frame{}); err != nil {
		t.Fatal(err)
	}
	if d.target != before || d.commits != commits || d.copyTarget != spare {
		t.Fatal("zero frame changed targets or submitted")
	}
	run(f, true)
	run(copyTargetSeed(image.Pt(128, 80)), false)
	if d.copyTarget != 0 || d.copyTargetSize != (image.Point{}) {
		t.Fatal("resize retained an obsolete spare")
	}
	before, commits = d.target, d.commits
	if err := d.Execute(&f); !errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("old-size partial: %v", err)
	}
	if d.target != before || d.commits != commits {
		t.Fatal("invalid partial swapped/submitted")
	}
	f = copyTargetFrame(image.Pt(128, 80))
	run(f, true)
	// Destination overlap is outside the Frame contract, but legacy's
	// all-sources snapshot and destination order must not be weakened.
	overlap := f
	overlap.Copies = []gpu.CopyRegion{
		{Src: image.Rect(0, 0, 110, 70), Dst: image.Rect(0, 0, 110, 70)},
		{Src: image.Rect(4, 0, 114, 70), Dst: image.Rect(8, 4, 118, 74)},
	}
	run(overlap, false)
	if d.copyStats.encoders != 2 || d.copyStats.blits != 4 {
		t.Fatal("overlap fallback did not keep two-pass snapshot order")
	}
	small := f
	small.Copies = []gpu.CopyRegion{{Src: image.Rect(0, 0, 4, 4), Dst: image.Rect(2, 2, 6, 6)}}
	run(small, false)
	if d.copyStats.pixels != 32 || d.copyStats.encoders != 2 {
		t.Fatal("small copy did not retain cheaper scratch path")
	}
	normal = f
	normal.Copies = nil
	for range scratchIdleFrames + 1 {
		if err := d.Execute(&normal); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	if d.copyTarget != 0 || d.scratch != 0 {
		t.Fatal("idle copy textures were not reclaimed")
	}
	run(copyTargetSeed(size), false)
	run(copyTargetFrame(size), true)
}

func TestMetalCopyTargetFailureLifecycle(t *testing.T) {
	d, ref := newDevice(t), newDevice(t)
	seed := copyTargetSeed(image.Pt(96, 64))
	if err := d.Execute(&seed); err != nil {
		t.Fatal(err)
	}
	executeScratchReference(t, ref, &seed)
	f := copyTargetFrame(seed.Size)
	warm := f
	warm.Copies = nil
	if err := d.Execute(&warm); err != nil {
		t.Fatal(err)
	}
	executeScratchReference(t, ref, &warm)
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	if err := d.ensureScratch(f.Copies); err != nil {
		t.Fatal(err)
	}
	// Objective-C messages to nil return nil. All other resources are
	// warm: temporarily hiding only the allocator's device injects a real
	// newTexture allocation failure without a production test hook or GPU fault.
	failAllocation := func() error {
		dev := d.dev
		d.dev = 0
		defer func() { d.dev = dev }()
		return d.Execute(&f)
	}
	old := d.target
	if err := failAllocation(); err != nil {
		t.Fatalf("spare allocation must fall back to existing scratch: %v", err)
	}
	if d.target != old || d.copyTarget != 0 || d.failures != 0 {
		t.Fatal("allocation fallback swapped/lost the target")
	}
	executeScratchReference(t, ref, &f)
	checkCopyTargetPixels(t, d, ref)
	// A retry may create the spare. Invalid uploads must fail before any
	// swap, even after a spare was successfully allocated for the attempt.
	bad := f
	bad.Uploads = []gpu.Upload{{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 9, 9)}}
	commits := d.commits
	if err := d.Execute(&bad); err == nil {
		t.Fatal("invalid upload accepted")
	}
	if d.target != old || d.commits != commits {
		t.Fatal("validation failure swapped/submitted")
	}
	checkCopyTargetPixels(t, d, ref)
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	if d.target == old || d.copyTarget == 0 {
		t.Fatal("allocation recovery did not use ping-pong")
	}
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("injected removed GPU")
	d.gpuFailed(boom, true)
	old, spare := d.target, d.copyTarget
	if err := d.Execute(&f); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("lost Execute: %v", err)
	}
	if d.target != old || d.copyTarget != spare || d.Retained(f.Size) {
		t.Fatal("lost device changed or retained targets")
	}
	d.Close()
	d.Close()
	if d.target != 0 || d.copyTarget != 0 || d.scratch != 0 || d.dev != 0 || d.queue != 0 || len(d.inflight.items) != 0 {
		t.Fatal("Close did not drain/release copy targets and device")
	}
	if err := d.Wait(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Wait: %v", err)
	}
}

func TestMetalCopyTargetBothAllocationsFail(t *testing.T) {
	d := newDevice(t)
	seed := copyTargetSeed(image.Pt(96, 64))
	if err := d.Execute(&seed); err != nil {
		t.Fatal(err)
	}
	f := copyTargetFrame(seed.Size)
	warm := f
	warm.Copies = nil
	if err := d.Execute(&warm); err != nil {
		t.Fatal(err)
	}
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	old, commits := d.target, d.commits
	for i := 1; i <= maxFailures; i++ {
		dev := d.dev
		err := func() error { d.dev = 0; defer func() { d.dev = dev }(); return d.Execute(&f) }()
		if err == nil || d.target != old || d.commits != commits || d.copyTarget != 0 || d.scratch != 0 {
			t.Fatalf("allocation failure #%d: err=%v", i, err)
		}
		if d.failures != i {
			t.Fatalf("failure count %d, want %d", d.failures, i)
		}
		if i == maxFailures && !errors.Is(err, ErrDeviceLost) {
			t.Fatalf("repeated allocation failure: %v", err)
		}
	}
}

func TestMetalCopyTargetOffscreenPresent(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	d, ref := newDevice(t), newDevice(t)
	pipe, err := d.newPresentPipeline() // Metal only: no CAMetalLayer/NSWindow
	if err != nil {
		t.Fatal(err)
	}
	d.presentPipeline = pipe
	seed := copyTargetSeed(image.Pt(96, 64))
	if err := d.Execute(&seed); err != nil {
		t.Fatal(err)
	}
	executeScratchReference(t, ref, &seed)
	f := copyTargetFrame(seed.Size)
	for range 3 {
		if err := d.Execute(&f); err != nil {
			t.Fatal(err)
		}
		executeScratchReference(t, ref, &f)
		want := readPixels(t, ref)
		// Do not drain d first: same-queue ordering must make the present
		// helper sample the new target after its pending frame completes.
		got := d.copyToBGRAForTest(t, seed.Size)
		for y := range seed.Size.Y {
			for x := range seed.Size.X {
				p := (y*seed.Size.X + x) * 4
				w := want.RGBAAt(x, y)
				if (color.RGBA{R: got[p+2], G: got[p+1], B: got[p], A: got[p+3]}) != w {
					t.Fatalf("present helper read stale target at %d,%d", x, y)
				}
			}
		}
		if err := d.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

type copyTargetCapture struct {
	*Device
	copies, imageUploads int
	recorded             bool
	stats                copyStats
}

func (c *copyTargetCapture) Execute(f *gpu.Frame) error {
	err := c.Device.Execute(f)
	if !c.recorded {
		// Keep the primary frame, not verify mode's subsequent full draw.
		c.copies, c.imageUploads = len(f.Copies), len(f.ImageUploads)
		c.stats = *c.Device.copyStats
		c.recorded = true
	}
	return err
}

// A real Renderer builds the copy frame, uploads fresh images/atlas masks,
// transforms and clips it, then grabs the NEW target for backdrop blur.
func TestMetalCopyTargetRenderer(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			d := newDevice(t)
			d.copyStats = &copyStats{}
			c := &copyTargetCapture{Device: d}
			r := gpu.NewRenderer(c)
			r.SetVerify(true)
			r.SetVerifyStrict(true)
			size := geom.Size{W: 328, H: 280}
			previous := float32(0)
			// Keep both directions within the planner's partial-area limit;
			// a larger reverse jump plus blur correctly falls back to full.
			for step, off := range []float32{0, 24, 48, 24, 0} {
				img := gputest.TestImage() // fresh image upload on a copy frame
				body := gpuconf.ScrollList(t, off, func(l *paint.List) {
					l.BackdropBlur(gpuconf.FrostedFooter, geom.Radii{TL: 8, TR: 8}, 8)
					l.Fill(gpuconf.FrostedFooter, geom.Radii{TL: 8, TR: 8}, color.NRGBA{R: 255, G: 255, B: 255, A: 140})
					l.Transform(geom.Translate(240, 0))
					l.Clip(geom.R(0, 0, 80, 24), geom.Radii{})
					l.DrawImage(img, geom.R(0, 0, 20, 20), geom.Radii{})
					l.Glyphs(geom.Pt(22, 16), gputest.Shape(t, fmt.Sprintf("new %d", step), 12), gputest.White)
					l.Pop()
					l.Pop()
					if step > 0 {
						l.Scroll(gpuconf.ScrollViewport, geom.Pt(0, previous-off))
						l.Damage(geom.R(240, 0, 80, 24))
					}
				})
				l := &paint.List{Background: body.Background}
				l.Transform(geom.Translate(4, 4))
				l.Ops = append(l.Ops, body.Ops...)
				l.Pop()
				old := d.target
				c.recorded = false
				if err := r.Render(l, size, scale); err != nil {
					t.Fatal(err)
				}
				if step > 0 && (c.copies != 1 || d.target == old || c.stats.encoders != 1) {
					t.Fatalf("renderer did not exercise ping-pong + blur: copies=%d swapped=%v stats=%+v", c.copies, d.target != old, c.stats)
				}
				if c.imageUploads == 0 || len(d.images) == 0 || d.mask == 0 || d.maskSize.X <= 1 {
					t.Fatal("image/atlas upload not exercised")
				}
				ref := paint.NewCPU()
				if err := ref.Render(l, size, scale); err != nil {
					t.Fatal(err)
				}
				gputest.Check(t, fmt.Sprintf("copy-blur-%d", step), l, scale, ref.Image(), readPixels(t, d), 4)
				previous = off
			}
		})
	}
}
