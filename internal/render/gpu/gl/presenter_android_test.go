//go:build android

package gl

import (
	"image"
	"runtime"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/gl/nativewindow"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
)

// Run on a device or emulator (go test -c for android, adb push, run):
// the presenter's context without a window (its pbuffer), the parity of
// the ES device, and the rebuild after a context loss.

func newTestPresenter(t *testing.T) (*androidPresenter, *nativewindow.Holder) {
	t.Helper()
	runtime.LockOSThread() // EGL contexts are current per thread
	t.Cleanup(runtime.UnlockOSThread)
	h := nativewindow.New()
	t.Cleanup(h.Close)
	r, _, err := NewPresenterAndroid(h.Handle())
	if err != nil {
		t.Fatalf("NewPresenterAndroid: %v", err)
	}
	p := r.(*androidRenderer).p
	t.Cleanup(p.Close)
	return p, h
}

func renderCheck(t *testing.T, p *androidPresenter, name string) {
	t.Helper()
	sc := gputest.SceneNamed(name)
	l := sc.Make(t)
	ref := paint.NewCPU()
	if err := ref.Render(l, gputest.SceneSize, 2); err != nil {
		t.Fatal(err)
	}
	if err := p.r.Render(l, gputest.SceneSize, 2); err != nil {
		t.Fatalf("%s: Render: %v", name, err)
	}
	got, err := p.dev.ReadPixels()
	if err != nil {
		t.Fatalf("%s: ReadPixels: %v", name, err)
	}
	gputest.Check(t, name, l, 2, ref.Image(), got, sc.Thr)
}

var esScenes = []string{"card", "text", "gradients", "gradstops", "images", "opacitynest", "pathgrad"}

// TestAndroidPresenterParity: the ES device (ramp as GL_RGBA16F unless the
// driver has EXT_texture_norm16) matches paint.CPU; present without a
// window skips.
func TestAndroidPresenterParity(t *testing.T) {
	p, _ := newTestPresenter(t)
	t.Logf("GL_RENDERER %q, ES %v, ramp half %v, visual %d", p.glRenderer, p.dev.es, p.dev.formats[gpu.TexRamp].half, p.visual)
	if !p.dev.es {
		t.Fatal("device not on an ES context")
	}
	for _, name := range esScenes {
		renderCheck(t, p, name)
	}
	if err := p.present(image.Pt(100, 100), 2); err != nil || p.surf != 0 || p.presented != 0 {
		t.Fatalf("present without a window: %v, surface %#x, presented %d", err, p.surf, p.presented)
	}
}

// TestAndroidPresenterRebuild: a context destroyed under the presenter
// (as a loss would) is rebuilt by the next Render, which renders the
// frame on a new Device with a new gpu.Renderer (everything uploaded
// again) and matches paint.CPU; maxRebuilds in a row lose the device.
func TestAndroidPresenterRebuild(t *testing.T) {
	p, _ := newTestPresenter(t)
	renderCheck(t, p, "text")
	oldDev, oldR := p.dev, p.r.Renderer
	for i := 1; i <= maxRebuilds; i++ {
		// Simulate the loss: the context goes away behind the Device.
		p.e.MakeCurrent(p.dpy, 0, 0, 0)
		p.e.DestroyContext(p.dpy, p.ctx)
		p.e.DestroySurface(p.dpy, p.pb)
		p.pb = 0
		renderCheck(t, p, "text")
		if p.rebuilds != i || p.lost != nil {
			t.Fatalf("after loss %d: rebuilds %d, lost %v", i, p.rebuilds, p.lost)
		}
	}
	if p.dev == oldDev || p.r.Renderer == oldR {
		t.Fatal("Device or Renderer not replaced")
	}
	if !oldDev.closed {
		t.Fatal("old Device not closed")
	}
	// One more without a present in between: lost for good.
	p.e.MakeCurrent(p.dpy, 0, 0, 0)
	p.e.DestroyContext(p.dpy, p.ctx)
	l := gputest.SceneNamed("card").Make(t)
	if err := p.r.Render(l, gputest.SceneSize, 2); err == nil || p.lost == nil {
		t.Fatalf("loss %d: Render %v, lost %v; want ErrDeviceLost", maxRebuilds+1, err, p.lost)
	}
}
