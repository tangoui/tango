package ui

import (
	"image"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Window is what the UI runtime needs from a platform window. A platform
// backend (macOS NSView, Win32 HWND, GTK widget, mobile view) implements it;
// so does the headless Driver used by tests.
//
// Threading: every method except Invalidate is called on the UI thread
// (the thread that calls Runner.Frame / Runner.HandleEvent). Invalidate may
// be called from any goroutine and must only schedule a frame.
type Window interface {
	// Size is the drawable content size in logical pixels.
	Size() geom.Size
	// Scale is the device-pixel ratio (2 on Retina).
	Scale() float32
	// Invalidate requests that the host call Runner.Frame soon (ideally on
	// the next vsync). Calls coalesce.
	Invalidate()
	// InvalidateAt requests a frame no later than t (timers, animations
	// paused between steps, caret blink). A later call with an earlier time
	// wins.
	InvalidateAt(t time.Time)
	// SetCursor changes the pointer shape over the content area.
	SetCursor(c input.Cursor)
	// ReadClipboard / WriteClipboard access the plain-text clipboard.
	ReadClipboard() string
	WriteClipboard(s string)
	// SetTextInput turns the platform text-input client (IME) on or off and
	// reports the caret rectangle, in logical pixels, used to position the
	// candidate window. While enabled the host delivers input.TextInput and
	// input.Composition events instead of raw characters.
	SetTextInput(enabled bool, caret geom.Rect)
	// Present shows a finished CPU frame. img has Size()*Scale() pixels,
	// is premultiplied RGBA (image.RGBA semantics) and is owned by the
	// runner: copy it or upload it before returning. Hosts using a GPU
	// renderer (RunnerOptions.Renderer) receive a nil image.
	Present(img *image.RGBA) error
}

// DamagePresenter is implemented by windows that can present only the
// changed parts of a CPU frame. Runner calls it instead of Present when
// the frame carries damage and the renderer is the CPU renderer.
//
// img is the complete frame (the CPU renderer still rasterizes every
// pixel); dirty lists the device-pixel rectangles, within img.Bounds(),
// that differ from the previous frame presented through this window, so a
// host that keeps its own copy of the last frame may copy only those.
// Ownership of img is the same as for Present. Frames without damage
// hints (the first frame, resizes, scale/theme/reduced-motion changes,
// frames whose widgets declared nothing) still go through Present.
type DamagePresenter interface {
	PresentDamage(img *image.RGBA, dirty []image.Rectangle) error
}

// WindowOptions describe a window to open.
type WindowOptions struct {
	_ [0]func()

	Title                 string
	Width, Height         float32 // logical
	MinWidth, MinHeight   float32
	Resizable             bool
	BackgroundTransparent bool
}

// Host is implemented by a platform backend that can open windows whose
// content is drawn by the Tango UI. The host owns the event loop and must:
//
//   - forward input as input.Event values to Runner.HandleEvent (logical
//     coordinates relative to the content area; positive Scroll.DY reveals
//     content below; Pointer.Clicks if the OS tracks double clicks);
//   - call Runner.Frame on the UI thread after Window.Invalidate /
//     InvalidateAt, after a resize or scale change, and when the window is
//     first shown;
//   - map input.Cursor to native cursors in Window.SetCursor;
//   - route IME through Window.SetTextInput (NSTextInputClient,
//     IMM32/TSF, GTK IM context) and deliver input.Composition +
//     input.TextInput;
//   - display the image passed to Window.Present (e.g. CGImage into a
//     CALayer, StretchDIBits / D2D bitmap, cairo surface);
//   - call Runner.Close when the window is destroyed.
type Host interface {
	OpenWindow(opts WindowOptions, r *Runner) (Window, error)
}

// Open creates a Runner for page and asks host to put it in a window.
func Open(h Host, opts WindowOptions, page func(*Context), ropts RunnerOptions) (*Runner, error) {
	r := NewRunner(page, ropts)
	w, err := h.OpenWindow(opts, r)
	if err != nil {
		return nil, err
	}
	r.Attach(w)
	return r, nil
}
