//go:build android

package android

/*
#include <stdlib.h>
#include <sys/system_properties.h>

// tg_gl_renderer_prop copies the system property debug.tango.renderer into
// out (PROP_VALUE_MAX bytes) and returns its length (0: unset).
static int tg_gl_renderer_prop(char *out) {
	return __system_property_get("debug.tango.renderer", out);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/gpu/gl/nativewindow"
)

// GPU presentation of canvas pages (docs/specs/mobile-canvas.md §1.3.5,
// M5): platform.CanvasNativeWindow. UseNativeWindow gives uihost the
// handle of a nativewindow.Holder, which the OpenGL ES presenter
// (gl.NewPresenterAndroid) reads the current ANativeWindow from on every
// present; the canvas keeps the holder up to date from the Surface
// callbacks:
//
//   - nativeSurfaceChanged → gpuSetWindow: the window (the same one on a
//     resize or rotation; a new one after the Surface was recreated) and
//     its buffer size.
//   - releaseWindow (surfaceDestroyed, the page closing) → gpuClearWindow,
//     before the window is released: the presenter destroys its EGL
//     surface synchronously, as Android requires before surfaceDestroyed
//     returns. No frame is drawn in the background (the scheduler is in
//     Background until the next Surface), and none could be: the holder
//     has no window.
//   - present (a CPU frame: an oversized frame, or every frame after uihost
//     left the GPU) → gpuYieldToCPU, before ANativeWindow_lock: an
//     EGL-connected window cannot be locked, so the EGL surface goes first.
//     A locked window stays connected to the CPU until its Surface is
//     destroyed, so the presenter then loses the device and uihost stays on
//     the CPU for the window's life.
//   - teardown → the holder closes.
//
// With the GPU on, uihost presents with Present(nil), which copies
// nothing. Overlays (canvases attached to WebView pages, TextureView) stay
// on the CPU: UseNativeWindow reports ErrUnsupported for them.

var _ platform.CanvasNativeWindow = (*canvas)(nil)

// UseNativeWindow implements platform.CanvasNativeWindow. Calling it
// again returns the same handle. Main thread.
func (c *canvas) UseNativeWindow() (uintptr, error) {
	if c.closed {
		return 0, errors.New("tango/android: canvas closed")
	}
	if c.ov != nil {
		return 0, fmt.Errorf("tango/android: attached canvases draw on the CPU: %w", platform.ErrUnsupported)
	}
	if c.gl == nil {
		c.gl = nativewindow.New()
		c.gpuSetWindow()
		if canvasDebug > 0 {
			logf(logInfo, "canvas %d: GPU presentation (native window holder %d)", c.id, c.gl.Handle())
		}
	}
	return c.gl.Handle(), nil
}

// ReleaseNativeWindow undoes UseNativeWindow when the presenter could not
// start (uihost's gpuRelease): the holder goes and the canvas is on the
// plain CPU path. Main thread; idempotent.
func (c *canvas) ReleaseNativeWindow() {
	if c.gl != nil {
		c.gl.Close()
		c.gl = nil
	}
}

// gpuSetWindow publishes the current window and buffer size to the
// holder (nativeSurfaceChanged).
func (c *canvas) gpuSetWindow() {
	if c.gl != nil && c.win != nil {
		c.gl.SetWindow(uintptr(c.win), c.wpx, c.hpx)
	}
}

// gpuClearWindow detaches the presenter from the window before the
// canvas releases it (releaseWindow).
func (c *canvas) gpuClearWindow() {
	if c.gl != nil {
		c.gl.ClearWindow()
	}
}

// gpuYieldToCPU detaches the presenter before a CPU frame locks the
// window (present).
func (c *canvas) gpuYieldToCPU() {
	if c.gl != nil && c.gl.UseCPU() {
		logf(logWarn, "canvas %d: a CPU frame replaces GPU presentation in this Surface", c.id)
	}
}

// gpuClose closes the holder (teardown).
func (c *canvas) gpuClose() { c.ReleaseNativeWindow() }

// rendererFromProperty sets TANGO_RENDERER from the system property
// debug.tango.renderer (adb shell setprop debug.tango.renderer cpu, read
// when the app starts), unless the environment sets it already: apps have
// no other way to receive an environment variable on Android. uihost reads
// it for each window ("cpu" keeps windows off the GPU, "gpu" also accepts
// a software renderer). Main thread, from lookupCanvas.
func rendererFromProperty() {
	if os.Getenv("TANGO_RENDERER") != "" {
		return
	}
	buf := (*C.char)(C.malloc(C.PROP_VALUE_MAX))
	defer C.free(unsafe.Pointer(buf))
	if n := C.tg_gl_renderer_prop(buf); n > 0 {
		v := C.GoStringN(buf, n)
		os.Setenv("TANGO_RENDERER", v)
		logf(logInfo, "TANGO_RENDERER=%s (debug.tango.renderer)", v)
	}
}
