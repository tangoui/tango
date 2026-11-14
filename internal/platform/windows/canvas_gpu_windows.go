//go:build windows && (amd64 || arm64)

package windows

// GPU presentation (platform.CanvasHWND). UseGPUWindow hands the canvas's
// own child HWND to the caller, which puts a DXGI flip-model swap chain on
// it (d3d11.NewPresenter). The HWND keeps everything else: input, IME,
// cursor, focus, UI Automation, frame scheduling and the synchronous frame
// after WM_SIZE / WM_DPICHANGED_AFTERPARENT (canvas_windows.go). Only
// drawing changes:
//
//	CPU: WM_PAINT → frame (when due) → CanvasFrame → Present(img) → DIB; WM_PAINT blits it
//	GPU: WM_PAINT → frame (when due) → CanvasFrame → (GPU present) → Present(nil)
//
// A flip-model swap chain and GDI must not draw on the same window, so on
// the GPU path nothing is drawn on the canvas HWND with GDI: WM_PAINT only
// validates it (BeginPaint/EndPaint) and runs a frame that is due
// (canvasGPU.paint, canvas_gpu_plan.go). CPU frames that still come (the
// presenter could not start, a frame beyond a device limit, a lost
// device) are shown by a sibling child window of the surface placed
// directly above the canvas, sized like it: Present(img) copies the frame
// into the canvas's DIB section as before and shows or repaints the
// sibling, Present(nil) hides it again (canvasGPU.present). The sibling is
// inert: WM_NCHITTEST answers HTTRANSPARENT, so the system hit-tests the
// canvas below it (same thread) and pointer input, WM_SETCURSOR and
// capture stay on the canvas; it never activates and passes focus on to
// the canvas. Frames are not scheduled through WM_PAINT on the GPU path
// (gpuFrameNow): the sibling may cover the canvas's whole visible region.
//
// Only opaque top-level canvases switch: attached canvases (layered,
// pass-through, below a WebView) and see-through ones (Transparent or a
// Material: the surface's DWM sheet of glass needs per-pixel alpha from
// GDI) report ErrUnsupported and keep the GDI path.

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasHWND = (*canvas)(nil)
	_ platform.CanvasHWND = (*attached)(nil)
)

const canvasCPUClass = "TangoCanvasCPU"

const (
	canvasGPUWmMouseActivate = 0x0021
	canvasGPUMANoActivate    = 3
	canvasGPUSwpShowWindow   = 0x0040
	canvasGPUSwpHideWindow   = 0x0080
	canvasGPUSiblingID       = canvasChildID + 1 // control ID of the sibling window
)

var (
	canvasGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	canvasGetCurrentThreadID       = kernel32.export("GetCurrentThreadId")
	canvasIsWindowVisible          = user32.NewProc("IsWindowVisible")
)

// Sibling windows by HWND; the canvas whose sibling is being created.
var (
	siblingByHwnd   = map[uintptr]*canvas{}
	siblingCreating *canvas

	siblingClassOnce sync.Once
	siblingClassErr  error
	siblingProcCB    uintptr
)

func registerSiblingClass() error {
	siblingClassOnce.Do(func() {
		siblingProcCB = syscall.NewCallback(canvasSiblingWndProc)
		wc := wndClassEx{
			WndProc:   siblingProcCB,
			Instance:  instance(),
			ClassName: u16(canvasCPUClass),
			// No cursor (the canvas below sets it), no background brush
			// (WM_PAINT covers everything; WM_ERASEBKGND is eaten).
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			siblingClassErr = fmt.Errorf("tango: RegisterClassExW(%s): %v", canvasCPUClass, err)
		}
	})
	return siblingClassErr
}

// onUIThread reports whether the caller runs on the thread that owns the
// canvas window (the backend's UI thread).
func (c *canvas) onUIThread() bool {
	owner, _, _ := canvasGetWindowThreadProcessID.Call(c.hwnd, 0)
	cur, _, _ := canvasGetCurrentThreadID.Call()
	return owner != 0 && owner == cur
}

// UseGPUWindow implements platform.CanvasHWND.
func (c *canvas) UseGPUWindow() (uintptr, error) {
	if c.closed {
		return 0, errCanvasClosed
	}
	if !c.onUIThread() {
		return 0, errors.New("tango/windows: UseGPUWindow must run on the main thread")
	}
	if c.gpu.on {
		return c.hwnd, nil
	}
	if !canvasGPUSupport(attachedByCanvas[c] != nil, c.transparent) {
		return 0, platform.ErrUnsupported
	}
	if err := c.createSibling(); err != nil {
		return 0, err // stay on the GDI path: there would be no CPU fallback
	}
	c.gpu.on = true
	// From now on nothing is drawn on the HWND. Ask for the first GPU
	// frame from the message loop (never re-entrantly).
	c.frameWanted = true
	c.enqueue(func() { c.qFrame = true })
	return c.hwnd, nil
}

// gpuFrameNow is paintNow on the GPU path: it runs the frame directly
// instead of through a synchronous WM_PAINT. While the sibling covers the
// canvas the canvas's visible region (WS_CLIPSIBLINGS) can be empty, and
// the system sends no WM_PAINT for an empty update region, so scheduling
// through WM_PAINT would stop the frames that could bring the GPU back.
// Inside a sink callback or a paint the frame is posted instead (no
// re-entrant frames). WM_PAINT still runs a frame that is due.
func (c *canvas) gpuFrameNow() {
	if c.inPaint || c.sinkDepth > 0 {
		c.frameWanted = true
		c.enqueue(func() { c.qFrame = true })
		return
	}
	if c.closed {
		return
	}
	if !c.shown() {
		// As on the CPU path, where a hidden or minimized window gets no
		// WM_PAINT: keep the request; WM_PAINT or the next request after
		// the window comes back runs it.
		c.frameWanted = true
		return
	}
	changed := c.ready && c.checkGeometry()
	if changed {
		c.fitSibling()
	}
	if frame, _ := c.gpu.paint(c.ready && !c.closed, changed, c.frameWanted, c.dib != 0); frame {
		c.frame()
	}
}

// UseGPUWindow implements platform.CanvasHWND: an attached canvas shares
// its window with a WebView (and may be layered or pass-through), so it
// stays on the GDI path.
func (a *attached) UseGPUWindow() (uintptr, error) { return 0, platform.ErrUnsupported }

// createSibling creates the (hidden) CPU-fallback sibling window above
// the canvas. UI thread.
func (c *canvas) createSibling() error {
	if c.sibling != 0 {
		return nil
	}
	if err := registerSiblingClass(); err != nil {
		return err
	}
	siblingCreating = c
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(canvasCPUClass))), 0,
		canvasWsChild|canvasWsClipSiblings, // not visible until a CPU frame comes
		0, 0, uintptr(max(c.pxW, 1)), uintptr(max(c.pxH, 1)),
		c.parent, canvasGPUSiblingID, instance(), 0)
	siblingCreating = nil
	if hwnd == 0 {
		for h, sc := range siblingByHwnd {
			if sc == c {
				delete(siblingByHwnd, h)
			}
		}
		return fmt.Errorf("tango: CreateWindowExW(%s): %v", canvasCPUClass, err)
	}
	c.sibling = hwnd
	siblingByHwnd[hwnd] = c
	procSetWindowPos.Call(hwnd, attachHwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	return nil
}

// presentGPU applies Present(img) (cpu true, the frame is in the DIB
// already) or Present(nil) on the GPU path to the sibling window.
func (c *canvas) presentGPU(cpu bool) {
	if c.sibling == 0 {
		return
	}
	switch c.gpu.present(cpu) {
	case siblingShow:
		// On top of the canvas, sized like it, then painted at once so a
		// live resize shows no stale frame.
		procSetWindowPos.Call(c.sibling, attachHwndTop, 0, 0, uintptr(c.pxW), uintptr(c.pxH),
			swpNoActivate|canvasGPUSwpShowWindow)
		restackEmbeddedIn(c.parent) // webembed_windows.go: embedded views stay on top
		canvasRedrawWindow.Call(c.sibling, 0, 0, canvasRdwInvalidate|canvasRdwUpdateNow)
	case siblingRedraw:
		canvasRedrawWindow.Call(c.sibling, 0, 0, canvasRdwInvalidate|canvasRdwUpdateNow)
	case siblingHide:
		procSetWindowPos.Call(c.sibling, 0, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|canvasGPUSwpHideWindow)
	}
}

// shown reports whether the canvas can be seen: visible and its surface
// not minimized.
func (c *canvas) shown() bool {
	if v, _, _ := canvasIsWindowVisible.Call(c.hwnd); v == 0 {
		return false
	}
	iconic, _, _ := canvasIsIconic.Call(c.parent)
	return iconic == 0
}

// fitSibling keeps the sibling the canvas's size. Call after a geometry
// change, before the frame that follows it.
func (c *canvas) fitSibling() {
	if c.sibling == 0 || c.closed {
		return
	}
	procSetWindowPos.Call(c.sibling, 0, 0, 0, uintptr(c.pxW), uintptr(c.pxH), swpNoMove|swpNoZOrder|swpNoActivate)
}

// canvasSiblingWndProc is the window procedure of the sibling class.
func canvasSiblingWndProc(hwnd, m, wp, lp uintptr) uintptr {
	c := siblingByHwnd[hwnd]
	if c == nil {
		if c = siblingCreating; c != nil && c.sibling == 0 {
			// First messages, inside CreateWindowExW.
			siblingByHwnd[hwnd] = c
		}
	}
	if c != nil {
		switch uint32(m) {
		case attachWmNCHitTest:
			return attachHTTransparent // the canvas below takes pointer input
		case canvasGPUWmMouseActivate:
			return canvasGPUMANoActivate
		case canvasWmEraseBkgnd:
			return 1 // WM_PAINT covers the whole client area
		case canvasWmPaint:
			var ps canvasPaintStruct
			if hdc, _, _ := canvasBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps))); hdc != 0 {
				if !c.closed {
					c.blit(hdc)
				}
				canvasEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			}
			return 0
		case canvasWmSetFocus:
			if !c.closed {
				canvasSetFocus.Call(c.hwnd)
			}
			return 0
		case canvasWmNCDestroy:
			delete(siblingByHwnd, hwnd)
			if c.sibling == hwnd {
				c.sibling = 0
			}
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}
