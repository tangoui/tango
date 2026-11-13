package platform

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Optional Canvas interfaces for phones and tablets (iOS, Android). See
// docs/specs/mobile-canvas.md §3.1. Desktop canvases implement none of
// them; the core discovers them with type assertions on the Canvas.

// CanvasInsets is implemented by canvases that fill the screen behind
// system UI. The core polls it every frame, so both methods return cached
// values and must be cheap. When a value changes the platform calls
// Invalidate (or reports CanvasResized when the size changed too). Logical
// pixels; main thread.
type CanvasInsets interface {
	// SafeAreaInsets returns the parts of the content area covered by the
	// status bar, a notch or camera cut-out, rounded corners, the home
	// indicator or the navigation bar.
	SafeAreaInsets() geom.Insets
	// KeyboardInset returns the part covered by the soft keyboard, usually
	// only Bottom; zero while the keyboard is hidden. The keyboard is not
	// part of the safe area.
	KeyboardInset() geom.Insets
}

// CanvasTextInputState is implemented by canvases whose input method can
// use the text around the caret (autocorrection, prediction, surrounding
// text deletion). The core calls SetTextInputState after SetTextInput
// whenever the focused field's state changes. Without it, or until the
// first call, the platform runs its input method on a shadow document that
// holds only the composition. Main thread.
type CanvasTextInputState interface {
	SetTextInputState(st input.TextInputState)
}

// CanvasNativeWindow is implemented by canvases that can be drawn by a GPU
// API bound to a native window (Android EGL). UseNativeWindow switches the
// canvas to GPU presentation and returns a stable platform handle to the
// window holder, not the window itself: the native window is recreated
// whenever the app returns from the background, so the presenter reads the
// current window from the holder on every present and skips the frame while
// there is none. After it succeeds, Present(nil) means the GPU presented
// the frame. It returns ErrUnsupported when the canvas cannot do it (for
// example canvases placed inside a WebView surface). Main thread.
type CanvasNativeWindow interface {
	UseNativeWindow() (target uintptr, err error)
}

// CanvasBackHandler is implemented by canvases on platforms with a system
// Back action (Android's back button and back gesture). The core calls
// SetBackHandling(true) while the UI has something Back should close (an
// overlay, a drawer, a dialog: ui.Context.WantBack) and false otherwise.
// While handling is on, the platform intercepts Back and delivers it to the
// canvas as an input.KeyEvent for input.KeyEscape (down then up) instead of
// closing the page. Main thread.
type CanvasBackHandler interface {
	SetBackHandling(on bool)
}

// AppMemoryWarning: the system is low on memory (iOS memory warning,
// Android onTrimMemory at TRIM_MEMORY_RUNNING_LOW or above). No payload.
// Receivers release caches they can rebuild (GPU atlases and textures,
// glyph caches). 201 is AppContrastChanged, 202 is reserved for a future
// AppTextScaleChanged (docs/specs/mobile-canvas.md §3.3–3.4).
const AppMemoryWarning AppEventKind = 203
