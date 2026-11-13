//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"image"
	"image/color"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
)

// user32 is loaded by name: it is a KnownDLL, always from System32.
var (
	user32            = syscall.NewLazyDLL("user32.dll")
	procCreateWindowW = user32.NewProc("CreateWindowExW")
	procDestroyWindow = user32.NewProc("DestroyWindow")
)

// newTestWindow creates a hidden top-level window (the predefined STATIC
// class, so no window class is registered) with a client area of about
// w×h, destroyed at the end of the test. The test stays on its OS thread,
// which owns the window.
func newTestWindow(t *testing.T, w, h int) uintptr {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	class, _ := syscall.UTF16PtrFromString("STATIC")
	title, _ := syscall.UTF16PtrFromString("tango d3d11 test")
	const wsOverlappedWindow = 0x00CF0000 // not WS_VISIBLE: hidden
	hwnd, _, err := procCreateWindowW.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, 0, 0, uintptr(w), uintptr(h), 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW: %v", err)
	}
	t.Cleanup(func() { procDestroyWindow.Call(hwnd) })
	return hwnd
}

// startPresenter runs newPresenter on hwnd, skipping without a Direct3D
// device, and closes it at the end of the test.
func startPresenter(t *testing.T, hwnd uintptr) *presenter {
	t.Helper()
	p, err := newPresenter(hwnd)
	if errors.Is(err, ErrNoDevice) {
		t.Skipf("no Direct3D 11 device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// renderScene renders the solid parity scene at size (logical) and
// scale through the presenter's Renderer.
func renderScene(t *testing.T, p *presenter, size geom.Size, scale float32) *paint.List {
	t.Helper()
	l := gputest.Scenes[0].Make(t)
	if err := p.r.Render(l, size, scale); err != nil {
		t.Fatal(err)
	}
	return l
}

// TestPresenterBasic: three frames rendered and presented without error;
// the presenter's Renderer is closable and Close is idempotent.
func TestPresenterBasic(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	if _, ok := p.r.(interface{ Close() error }); !ok {
		t.Fatal("the presenter's Renderer has no Close")
	}
	for i := range 3 {
		renderScene(t, p, gputest.SceneSize, 2)
		if err := p.present(image.Pt(192, 112), 2); err != nil {
			t.Fatalf("present %d: %v", i, err)
		}
	}
	if p.presented != 3 {
		t.Fatalf("%d presents, want 3", p.presented)
	}
	if p.bufSize != image.Pt(192, 112) {
		t.Fatalf("back buffers %v, want 192×112", p.bufSize)
	}
}

// TestPresenterCopy: the back buffer holds the target's bytes (RGBA →
// BGRA), and a back buffer larger than the target repeats its last row
// and column.
func TestPresenterCopy(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1) // 96×56
	want, err := p.d.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []image.Point{{96, 56}, {100, 60}, {50, 30}} {
		var got []byte
		var pitch int
		p.beforePresent = func(back uintptr) { got, pitch = readBGRA(t, p.d, back, size) }
		if err := p.present(size, 1); err != nil {
			t.Fatal(err)
		}
		if got == nil {
			t.Fatalf("%v: back buffer not read", size)
		}
		for y := range size.Y {
			for x := range size.X {
				c := want.RGBAAt(min(x, 95), min(y, 55))
				o := y*pitch + 4*x
				if b := (color.RGBA{got[o+2], got[o+1], got[o], got[o+3]}); b != c {
					t.Fatalf("%v: pixel %d,%d = %v, want %v", size, x, y, b, c)
				}
			}
		}
	}
}

// readBGRA copies the back buffer back to the CPU (the Device's mutex is
// held by present): its rows and their pitch.
func readBGRA(t *testing.T, d *Device, back uintptr, size image.Point) ([]byte, int) {
	stage, err := newTexture(d.dev, size, textureSpec{format: formatB8G8R8A8Unorm, usage: usageStaging, cpu: cpuAccessRead}, nil, 0)
	if err != nil {
		t.Error(err)
		return nil, 0
	}
	defer stage.release()
	comCall(d.ctx, ctxCopyResource, stage.tex, back)
	var m mappedSubresource
	if err := hrError("Map", comCall(d.ctx, ctxMap, stage.tex, 0, mapRead, 0, uintptr(unsafe.Pointer(&m)))); err != nil {
		t.Error(err)
		return nil, 0
	}
	defer comCall(d.ctx, ctxUnmap, stage.tex, 0)
	pitch := int(m.RowPitch)
	n := (size.Y-1)*pitch + 4*size.X // the last row may end before its pitch
	return append([]byte(nil), unsafe.Slice((*byte)(foreign(m.Data)), n)...), pitch
}

// TestPresenterResize: a new size resizes the back buffers once (not on
// every present), also before the first frame, and frames keep being
// presented.
func TestPresenterResize(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	r0 := p.resizes
	if err := p.present(image.Pt(64, 48), 1); err != nil { // no frame yet
		t.Fatal(err)
	}
	if p.resizes != r0+1 || p.bufSize != image.Pt(64, 48) || p.presented != 0 {
		t.Fatalf("before the first frame: %d resizes, buffers %v, %d presents", p.resizes-r0, p.bufSize, p.presented)
	}
	for i, size := range []image.Point{{192, 112}, {192, 112}, {300, 150}, {96, 56}, {96, 56}} {
		renderScene(t, p, geom.Size{W: float32(size.X) / 2, H: float32(size.Y) / 2}, 2)
		if err := p.present(size, 2); err != nil {
			t.Fatalf("present %d (%v): %v", i, size, err)
		}
		if p.bufSize != size {
			t.Fatalf("present %d: buffers %v, want %v", i, p.bufSize, size)
		}
	}
	if got := p.resizes - r0; got != 4 {
		t.Fatalf("%d ResizeBuffers calls, want 4 (64×48, 192×112, 300×150, 96×56)", got)
	}
	// Larger than the texture limit: clamped.
	huge := image.Pt(p.d.maxTex+100, 20)
	if err := p.present(huge, 1); err != nil {
		t.Fatal(err)
	}
	if p.bufSize != image.Pt(p.d.maxTex, 20) {
		t.Fatalf("huge present: buffers %v", p.bufSize)
	}
}

// TestPresenterZeroSize: an empty size (minimized window) does nothing:
// no ResizeBuffers, no present, no error.
func TestPresenterZeroSize(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1)
	if err := p.present(image.Pt(96, 56), 1); err != nil {
		t.Fatal(err)
	}
	resizes, presented := p.resizes, p.presented
	for _, size := range []image.Point{{}, {0, 56}, {96, 0}, {-1, 5}} {
		if err := p.present(size, 1); err != nil {
			t.Fatalf("%v: %v", size, err)
		}
	}
	if p.resizes != resizes || p.presented != presented {
		t.Fatalf("empty sizes: %d resizes, %d presents", p.resizes-resizes, p.presented-presented)
	}
}

// TestPresenterDeviceRemoved: Present1 reporting DXGI_ERROR_DEVICE_REMOVED
// loses the device: present and the Renderer fail with ErrDeviceLost from
// then on. DXGI_STATUS_OCCLUDED is a success.
func TestPresenterDeviceRemoved(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1)
	setInjectHR(t, func(call string) uintptr {
		if call == "Present1" {
			return dxgiStatusOccluded
		}
		return 0
	})
	if err := p.present(image.Pt(96, 56), 1); err != nil {
		t.Fatalf("occluded present: %v", err)
	}
	injectHR = func(call string) uintptr {
		if call == "Present1" {
			return dxgiErrorDeviceRemoved
		}
		return 0
	}
	if err := p.present(image.Pt(96, 56), 1); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("present on a removed device: %v, want ErrDeviceLost", err)
	}
	injectHR = nil
	if err := p.present(image.Pt(96, 56), 1); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("present after the loss: %v", err)
	}
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Render after the loss: %v", err)
	}
}

// TestPresenterFailuresCounted: failed presents (Present1 E_FAIL) count
// on their own counter: a successful present resets it, maxFailures in a
// row lose the device.
func TestPresenterFailuresCounted(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1)
	fail := true
	setInjectHR(t, func(call string) uintptr {
		if fail && call == "Present1" {
			return eFail
		}
		return 0
	})
	for range maxFailures - 1 {
		if err := p.present(image.Pt(96, 56), 1); err == nil || errors.Is(err, ErrDeviceLost) {
			t.Fatalf("failed present: %v", err)
		}
	}
	fail = false
	if err := p.present(image.Pt(96, 56), 1); err != nil {
		t.Fatal(err)
	}
	fail = true
	var err error
	for range maxFailures {
		err = p.present(image.Pt(96, 56), 1)
	}
	if !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("after %d failed presents: %v", maxFailures, err)
	}
}

// TestPresenterFailsBetweenFrames: the platform's loop — Render succeeds,
// then present fails (here ResizeBuffers with DXGI_ERROR_INVALID_CALL, as
// with an outstanding back-buffer reference) — loses the device after
// maxFailures cycles instead of spinning forever: successful frames do not
// reset the present failure count.
func TestPresenterFailsBetweenFrames(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	setInjectHR(t, func(call string) uintptr {
		if call == "ResizeBuffers" {
			return dxgiErrorInvalidCall
		}
		return 0
	})
	for i := range maxFailures {
		renderScene(t, p, gputest.SceneSize, 1) // Execute succeeds
		err := p.present(image.Pt(96+i, 56), 1) // a new size each time: ResizeBuffers
		if i < maxFailures-1 {
			if err == nil || errors.Is(err, ErrDeviceLost) {
				t.Fatalf("cycle %d: %v, want a plain error", i+1, err)
			}
			continue
		}
		if !errors.Is(err, ErrDeviceLost) {
			t.Fatalf("cycle %d: %v, want ErrDeviceLost", i+1, err)
		}
	}
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("Render after the loss: %v, want ErrDeviceLost (CPU fallback)", err)
	}
}

// TestPresenterDirtyHint: dirty rectangles are passed to Present1 only on
// a FLIP_SEQUENTIAL swap chain, for a partial frame right after the shown
// full frame; never on FLIP_DISCARD (the Windows 10+ swap chain), which
// rejects partial presentation. Either way the frame is presented.
func TestPresenterDirtyHint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		effect uint32
	}{{"discard", swapEffectFlipDiscard}, {"sequential", swapEffectFlipSequential}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.effect == swapEffectFlipSequential {
				saved := swapChainModes
				swapChainModes = saved[1:] // FLIP_SEQUENTIAL first
				t.Cleanup(func() { swapChainModes = saved })
			}
			p := startPresenter(t, newTestWindow(t, 320, 200))
			if p.effect != tc.effect {
				t.Fatalf("swap effect %d, want %d", p.effect, tc.effect)
			}
			r := p.r.(*presenterRenderer).Renderer
			l := gputest.Scenes[0].Make(t)
			if err := r.Render(l, gputest.SceneSize, 1); err != nil {
				t.Fatal(err)
			}
			if err := p.present(image.Pt(96, 56), 1); err != nil {
				t.Fatal(err)
			}
			l2 := &paint.List{Background: l.Background, Ops: append(append([]paint.Op(nil), l.Ops...), paint.Damage{Rect: geom.R(5, 5, 20, 10)})}
			if err := r.Render(l2, gputest.SceneSize, 1); err != nil {
				t.Fatal(err)
			}
			if p.d.lastFull {
				t.Skip("the Renderer drew the hinted frame in full")
			}
			hint := presentDirtyRects(p.effect, p.d.frameSeq, p.presentedSeq, p.d.lastFull, false, p.d.lastDirty, p.d.target.size, p.bufSize)
			switch {
			case tc.effect == swapEffectFlipDiscard && hint != nil:
				t.Fatalf("dirty rectangles %v on a FLIP_DISCARD swap chain", hint)
			case tc.effect == swapEffectFlipSequential && p.presentedSeq == 0:
				t.Log("the full frame was occluded (hidden window): no hint expected")
			case tc.effect == swapEffectFlipSequential && len(hint) == 0:
				t.Fatal("no dirty rectangles for a partial frame after the shown one")
			}
			if err := p.present(image.Pt(96, 56), 1); err != nil {
				t.Fatalf("present: %v", err)
			}
		})
	}
}

// TestPresenterOccludedNoHint: an occluded present (DXGI_STATUS_OCCLUDED)
// succeeds but does not count as shown, so the next frame gets no
// dirty-rectangle hint.
func TestPresenterOccludedNoHint(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1)
	setInjectHR(t, func(call string) uintptr {
		if call == "Present1" {
			return dxgiStatusOccluded
		}
		return 0
	})
	if err := p.present(image.Pt(96, 56), 1); err != nil {
		t.Fatal(err)
	}
	if p.presentedSeq != 0 {
		t.Fatalf("occluded present recorded frame %d as shown", p.presentedSeq)
	}
}

// TestPresenterClose: Close twice is fine; present and Render fail with
// ErrClosed afterwards.
func TestPresenterClose(t *testing.T) {
	p := startPresenter(t, newTestWindow(t, 320, 200))
	renderScene(t, p, gputest.SceneSize, 1)
	c := p.r.(interface{ Close() error })
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.present(image.Pt(96, 56), 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("present after Close: %v", err)
	}
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("Render after Close: %v", err)
	}
}

// TestPresenterBadHWND: no window, no presenter; a swap chain failing to
// be created leaves nothing behind.
func TestPresenterBadHWND(t *testing.T) {
	if _, _, err := NewPresenter(0); err == nil {
		t.Fatal("NewPresenter(0) succeeded")
	}
	hwnd := newTestWindow(t, 64, 64)
	setInjectHR(t, func(call string) uintptr {
		if call == "CreateSwapChainForHwnd" {
			return dxgiErrorInvalidCall
		}
		return 0
	})
	if _, err := newPresenter(hwnd); err == nil {
		t.Fatal("newPresenter with every swap chain failing succeeded")
	} else if errors.Is(err, ErrNoDevice) {
		t.Skip(err)
	}
}
