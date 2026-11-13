//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/gpuconf"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/icon"
	uilayout "github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/shader/sl"
	"github.com/tangoui/tango/ui/theme"
)

func newTestDevice(tb testing.TB) *Device {
	tb.Helper()
	d, err := NewDevice()
	if errors.Is(err, ErrNoDevice) {
		tb.Skipf("no Direct3D 11 device: %v", err)
	}
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(d.Close)
	return d
}

func readPixels(tb testing.TB, d *Device) *image.RGBA {
	tb.Helper()
	img, err := d.ReadPixels()
	if err != nil {
		tb.Fatal(err)
	}
	return img
}

// setInjectHR installs an HRESULT injection hook for the rest of t.
func setInjectHR(t *testing.T, f func(call string) uintptr) {
	injectHR = f
	t.Cleanup(func() { injectHR = nil })
}

// TestPipelineFailure: a pipeline that fails part way returns an error and
// no pipeline. (That it releases the shaders it created first is not
// observable here.)
func TestPipelineFailure(t *testing.T) {
	d := newTestDevice(t)
	setInjectHR(t, func(call string) uintptr {
		if call == "CreateBlendState" {
			return dxgiErrorInvalidCall
		}
		return 0
	})
	p, err := newPipeline(d.dev, d.featureLevel)
	if err == nil {
		t.Fatal("newPipeline with CreateBlendState failing succeeded")
	}
	if p != nil {
		t.Fatalf("newPipeline returned %v with error %v", p, err)
	}
}

// TestD3DConformance runs the device conformance suite shared by every
// GPU backend (parity scenes against paint.CPU, upload order, layers,
// images, gradient ramps, partial frames) on a fresh Direct3D 11 device
// per subtest.
func TestD3DConformance(t *testing.T) {
	gpuconf.Conformance(t, func(t *testing.T) gpu.Device {
		return newTestDevice(t)
	})
}

// TestD3DZeroSize: frames without pixels flush nothing, through the
// Renderer and when executed directly. (That they succeed and draw nothing
// is gpuconf's ZeroSize.)
func TestD3DZeroSize(t *testing.T) {
	d := newTestDevice(t)
	r := gpu.NewRenderer(d)
	l := gputest.Scenes[0].Make(t)
	for _, size := range []geom.Size{{W: 0, H: 100}, {W: 100, H: 0}, {}, {W: -5, H: 10}} {
		if err := r.Render(l, size, 2); err != nil {
			t.Fatalf("%v: %v", size, err)
		}
	}
	for _, sz := range []image.Point{{}, {X: 0, Y: 10}, {X: 10, Y: 0}, {X: -3, Y: 4}} {
		if err := d.Execute(&gpu.Frame{Size: sz, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}); err != nil {
			t.Fatalf("%v: %v", sz, err)
		}
	}
	if d.commits != 0 {
		t.Fatalf("%d frames flushed for empty frames", d.commits)
	}
	if d.target.tex != 0 {
		t.Fatal("an empty frame created the render target")
	}
}

// TestTargetTooLarge: a frame larger than the texture limit fails with
// ErrTargetTooLarge before anything is allocated or flushed, and is not a
// GPU failure: any number in a row never loses the device and the next
// valid frame draws normally. Oversized sampled textures fail the same
// way.
func TestTargetTooLarge(t *testing.T) {
	d := newTestDevice(t)
	r := gpu.NewRenderer(d)
	l := gputest.Scenes[0].Make(t)
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	commits := d.commits
	lim := d.maxTex
	for i := range 2 * maxFailures {
		size := geom.Size{W: float32(lim + 1), H: 10}
		if i%2 == 1 {
			size = geom.Size{W: 10, H: float32(lim/2 + 1000)} // over the limit at 2x
		}
		err := r.Render(l, size, 2)
		if !errors.Is(err, ErrTargetTooLarge) || errors.Is(err, ErrDeviceLost) {
			t.Fatalf("frame %d (%v@2x): %v, want ErrTargetTooLarge", i, size, err)
		}
	}
	for _, f := range []*gpu.Frame{
		{Size: image.Pt(8, 8), MaskSize: image.Pt(4, lim+1), ColorSize: image.Pt(4, 4)},
		{Size: image.Pt(8, 8), MaskSize: image.Pt(4, 4), ColorSize: image.Pt(lim+1, 4)},
		{Size: image.Pt(8, 8), MaskSize: image.Pt(4, 4), RampSize: image.Pt(256, lim+1)},
	} {
		if err := d.Execute(f); !errors.Is(err, ErrTargetTooLarge) {
			t.Fatalf("mask %v color %v ramp %v: %v, want ErrTargetTooLarge", f.MaskSize, f.ColorSize, f.RampSize, err)
		}
	}
	if d.health.failures != 0 || d.health.lost != nil {
		t.Fatalf("after oversized frames: failures %d, lost %v", d.health.failures, d.health.lost)
	}
	if d.commits != commits {
		t.Fatalf("%d frames flushed for oversized frames", d.commits-commits)
	}
	if d.target.size != image.Pt(96, 56) {
		t.Fatalf("an oversized frame replaced the target: %v", d.target.size)
	}
	// The largest valid target still works, and so does the usual size.
	if err := r.Render(l, geom.Size{W: float32(lim), H: 4}, 1); err != nil {
		t.Fatalf("%d px wide: %v", lim, err)
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "after", l, 1, ref.Image(), readPixels(t, d), 2)
}

// solidFrame is an 8×8 frame cleared to c with one solid instance.
func solidFrame(c color.NRGBA) *gpu.Frame {
	return &gpu.Frame{
		Size: image.Pt(8, 8), Clear: c, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4),
		Batches: []gpu.Batch{{Instances: []shaders.Instance{{
			Rect: sl.V4(2, 2, 4, 4), Color: sl.V4(0, 0, 1, 1), Kind: sl.V4(shaders.KindSolid, 0, 0, 0),
		}}}},
	}
}

// TestDeviceRemovedIsLost: a removed reason reported after a frame (here
// DXGI_ERROR_DEVICE_REMOVED injected for GetDeviceRemovedReason) loses
// the device at once: that Execute and every later call return an error
// wrapping ErrDeviceLost.
func TestDeviceRemovedIsLost(t *testing.T) {
	d := newTestDevice(t)
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); err != nil {
		t.Fatal(err)
	}
	readPixels(t, d)
	setInjectHR(t, func(call string) uintptr {
		if call == "GetDeviceRemovedReason" {
			return dxgiErrorDeviceRemoved
		}
		return 0
	})
	err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255}))
	if !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Execute on a removed device: %v, want ErrDeviceLost", err)
	}
	injectHR = nil // the device stays lost without the hook
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Execute after the loss: %v", err)
	}
	if _, err := d.ReadPixels(); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("ReadPixels after the loss: %v", err)
	}
	if err := d.Execute(&gpu.Frame{Size: image.Pt(16384000, 1)}); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("oversized Execute after the loss: %v", err)
	}
}

// TestDeviceLostHRESULT: a call returning DXGI_ERROR_DEVICE_HUNG (here
// Map of the instance buffer) loses the device at once.
func TestDeviceLostHRESULT(t *testing.T) {
	d := newTestDevice(t)
	setInjectHR(t, func(call string) uintptr {
		if call == "Map" {
			return dxgiErrorDeviceHung
		}
		return 0
	})
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Execute with Map hung: %v, want ErrDeviceLost", err)
	}
}

// TestFailuresLoseDevice: failed frames (Map failing with E_FAIL) are
// counted; a successful frame resets the count, and maxFailures in a row
// lose the device.
func TestFailuresLoseDevice(t *testing.T) {
	d := newTestDevice(t)
	fail := false
	setInjectHR(t, func(call string) uintptr {
		if fail && call == "Map" {
			return eFail
		}
		return 0
	})
	white := color.NRGBA{255, 255, 255, 255}
	fail = true
	for i := range maxFailures - 1 {
		err := d.Execute(solidFrame(white))
		if err == nil || errors.Is(err, ErrDeviceLost) {
			t.Fatalf("failure %d: %v, want a plain error", i+1, err)
		}
	}
	fail = false
	if err := d.Execute(solidFrame(white)); err != nil {
		t.Fatalf("frame after the failures: %v", err)
	}
	if got := readPixels(t, d).RGBAAt(3, 3); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("pixel after recovery %v", got)
	}
	fail = true
	var err error
	for range maxFailures {
		err = d.Execute(solidFrame(white))
	}
	if !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("after %d failures in a row: %v, want ErrDeviceLost", maxFailures, err)
	}
}

// TestD3DInvalidFramesNotCounted: invalid frames (an unknown upload
// texture, Copies without Dirty, a layer sampled before it is drawn, an
// image the device does not hold, a bad image upload) fail without
// counting as GPU failures.
func TestD3DInvalidFramesNotCounted(t *testing.T) {
	d := newTestDevice(t)
	ld := gpu.LayerDesc{Size: image.Pt(4, 4), Bounds: image.Rect(0, 0, 4, 4)}
	frames := []*gpu.Frame{
		{Size: image.Pt(8, 8), Uploads: []gpu.Upload{{Tex: 9, Rect: image.Rect(0, 0, 1, 1), Pix: []byte{0}, Stride: 1}}},
		{Size: image.Pt(8, 8), Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 1, 1), Dst: image.Rect(2, 2, 3, 3)}}},
		{Size: image.Pt(8, 8), Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Layer: 1, Instances: make([]shaders.Instance, 1)}}},
		{Size: image.Pt(8, 8), Batches: []gpu.Batch{{Image: 1, Instances: make([]shaders.Instance, 1)}}},
		{Size: image.Pt(8, 8), ImageUploads: []gpu.ImageUpload{{ID: 1, Size: image.Pt(2, 2), Pix: make([]byte, 3), Stride: 8}}},
	}
	for range maxFailures {
		for i, f := range frames {
			if err := d.Execute(f); err == nil || errors.Is(err, ErrDeviceLost) {
				t.Fatalf("frame %d: %v", i, err)
			}
		}
	}
	if d.health.failures != 0 {
		t.Fatalf("invalid frames counted as %d failures", d.health.failures)
	}
}

// TestFailureAsksRemovedReason: a failed call whose HRESULT is not itself
// a device-lost code (Map with E_FAIL) loses the device at once when the
// device then reports a removed reason.
func TestFailureAsksRemovedReason(t *testing.T) {
	d := newTestDevice(t)
	setInjectHR(t, func(call string) uintptr {
		switch call {
		case "Map":
			return eFail
		case "GetDeviceRemovedReason":
			return dxgiErrorDeviceReset
		}
		return 0
	})
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Execute: %v, want ErrDeviceLost after one failure", err)
	}
}

// TestD3DImageCache: the device holds one texture per cached image across
// frames, and an invalidated image's old texture is released. (That the
// frames match paint.CPU is gpuconf's ImageCache.)
func TestD3DImageCache(t *testing.T) {
	d := newTestDevice(t)
	r := gpu.NewRenderer(d)
	a := image.NewRGBA(image.Rect(0, 0, 10, 8))
	for i := range a.Pix {
		a.Pix[i] = uint8(i * 3)
	}
	b := gputest.TestImage()
	size := geom.Size{W: 96, H: 40}
	draw := func(name string, imgs ...image.Image) {
		t.Helper()
		l := &paint.List{Background: gputest.White}
		for i, img := range imgs {
			l.DrawImage(img, geom.R(float32(2+i*18), 4.5, 16, 30), geom.Rad(3))
		}
		if err := r.Render(l, size, 2); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	draw("first", a, b, a, b)
	draw("cached", a, b, a, b)
	if n := len(d.images); n != 2 {
		t.Fatalf("device holds %d image textures; want 2", n)
	}
	for i := range a.Pix {
		a.Pix[i] = 255 - uint8(i)
	}
	r.InvalidateImage(a)
	draw("invalidated", b, a)
	if n := len(d.images); n != 2 {
		t.Fatalf("after invalidation the device holds %d image textures; want 2 (old A freed)", n)
	}
}

// TestD3DClosed: every call after Close fails with ErrClosed; Close is
// idempotent.
func TestD3DClosed(t *testing.T) {
	d := newTestDevice(t)
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d.Close()
	if err := d.Execute(&gpu.Frame{Size: image.Pt(4, 4)}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Execute after Close: %v", err)
	}
	if _, err := d.ReadPixels(); !errors.Is(err, ErrClosed) {
		t.Fatalf("ReadPixels after Close: %v", err)
	}
}

// TestInstanceRing: frames with growing and shrinking instance counts
// (wrapping the ring, growing the buffer past its first size) each draw
// exactly their own instances.
func TestInstanceRing(t *testing.T) {
	d := newTestDevice(t)
	const w, h = 64, 64
	for frame, n := range []int{1, 300, 300, 455, 456, 1000, 3, 4096, 2, 4096} {
		f := &gpu.Frame{Size: image.Pt(w, h), Clear: color.NRGBA{255, 255, 255, 255}, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}
		ins := make([]shaders.Instance, n)
		for i := range ins {
			// One pixel per instance, row-major; the frame's last instance
			// is red, the others blue.
			x, y := i%w, i/w
			c := sl.V4(0, 0, 1, 1)
			if i == n-1 {
				c = sl.V4(1, 0, 0, 1)
			}
			ins[i] = shaders.Instance{Rect: sl.V4(float32(x), float32(y), 1, 1), Color: c, Kind: sl.V4(shaders.KindSolid, 0, 0, 0)}
		}
		// Split into batches of at most 100 so offsets inside the ring are
		// exercised too.
		for len(ins) > 0 {
			k := min(len(ins), 100)
			f.Batches = append(f.Batches, gpu.Batch{Instances: ins[:k]})
			ins = ins[k:]
		}
		if err := d.Execute(f); err != nil {
			t.Fatalf("frame %d (%d instances): %v", frame, n, err)
		}
		img := readPixels(t, d)
		for i := range w * h {
			want := color.RGBA{255, 255, 255, 255}
			switch {
			case i == n-1:
				want = color.RGBA{255, 0, 0, 255}
			case i < n:
				want = color.RGBA{0, 0, 255, 255}
			}
			if got := img.RGBAAt(i%w, i/w); got != want {
				t.Fatalf("frame %d (%d instances): pixel %d = %v, want %v", frame, n, i, got, want)
			}
		}
	}
}

// TestNewDeviceDebugEnv: TANGO_D3D_DEBUG=1 still yields a working device,
// with the debug layer if it is installed and without it otherwise.
func TestNewDeviceDebugEnv(t *testing.T) {
	t.Setenv(debugEnv, "1")
	d := newTestDevice(t)
	if err := d.Execute(solidFrame(color.NRGBA{255, 255, 255, 255})); err != nil {
		t.Fatal(err)
	}
	if got := readPixels(t, d).RGBAAt(3, 3); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("pixel %v", got)
	}
}

// galleryPage is ui's golden test page, rebuilt through the public ui API
// (as ui/gpu/metal's gallery test does).
func galleryPage(c *ui.Context) {
	open := true
	val, empty := "Tango", ""
	ck := true
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Button(c, "主按钮").Primary()
			ui.Button(c, "默认")
			ui.Button(c, "虚线").Dashed()
			ui.Button(c, "危险").Danger()
			ui.Button(c, "搜索").Icon(icon.Search)
			ui.Button(c, "禁用").Disabled(true)
		}, uilayout.Gap(8))
		ui.Row(c, func() {
			ui.Input(c, &val).Width(160)
			ui.Input(c, &empty).Placeholder("请输入").Status("error").Width(160)
			ui.Checkbox(c, "复选框", &ck)
			ui.Switch(c, &open)
		}, uilayout.Gap(12), uilayout.Middle())
		ui.Row(c, func() {
			ui.Tag(c, "默认")
			ui.Tag(c, "成功").Color("success")
			ui.Badge(c).Status("processing", "运行中")
			ui.Badge(c).Count(5).Body(func() { ui.Avatar(c, "U").Square() })
		}, uilayout.Gap(12), uilayout.Middle())
		ui.Alert(c, "info", "这是一条提示信息，中文渲染与断行。").Show()
		ui.Progress(c, 60).Layout(uilayout.W(300))
	}, uilayout.Pad(16), uilayout.Gap(16))
}

// TestD3DGallery: the gallery page matches paint.CPU (text threshold), so
// the benchmark measures a correct frame.
func TestD3DGallery(t *testing.T) {
	d := newTestDevice(t)
	size := geom.Size{W: 640, H: 300}
	drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: size.W, Height: size.H, Scale: 1, Theme: theme.AntDesign()})
	drv.Settle()
	l := drv.Runner().List()
	r := gpu.NewRenderer(d)
	for _, scale := range []float32{1, 2} {
		ref := paint.NewCPU()
		ref.Render(l, size, scale)
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, "gallery", l, scale, ref.Image(), readPixels(t, d), 4)
	}
}

// BenchmarkD3DGalleryFrame renders the gallery page (640×300 logical) one
// frame per iteration. submit-ms is the CPU time of Renderer.Render
// including Execute up to the flush, without waiting for the GPU; exec-ms
// is the Execute part of it (validation, uploads, instance writes,
// draws, flush). Every 16 frames the benchmark waits for the GPU (a
// read-back, excluded from both) so queued frames do not pile up.
func BenchmarkD3DGalleryFrame(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newTestDevice(b)
			size := geom.Size{W: 640, H: 300}
			drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: size.W, Height: size.H, Scale: scale, Theme: theme.AntDesign()})
			drv.Settle()
			l := drv.Runner().List()
			td := &timedDevice{d: d}
			r := gpu.NewRenderer(td)
			if err := r.Render(l, size, scale); err != nil { // warm the atlas
				b.Fatal(err)
			}
			var submitT time.Duration
			td.t = 0
			i := 0
			b.ResetTimer()
			for b.Loop() {
				start := time.Now()
				if err := r.Render(l, size, scale); err != nil {
					b.Fatal(err)
				}
				submitT += time.Since(start)
				if i++; i%16 == 0 {
					b.StopTimer()
					if _, err := d.ReadPixels(); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
				}
			}
			n := float64(b.N)
			b.ReportMetric(float64(submitT.Microseconds())/1000/n, "submit-ms")
			b.ReportMetric(float64(td.t.Microseconds())/1000/n, "exec-ms")
		})
	}
}

// timedDevice adds up the time spent in Execute.
type timedDevice struct {
	d *Device
	t time.Duration
}

func (td *timedDevice) Execute(f *gpu.Frame) error {
	start := time.Now()
	err := td.d.Execute(f)
	td.t += time.Since(start)
	return err
}
