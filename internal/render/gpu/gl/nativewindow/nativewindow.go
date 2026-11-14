// Package nativewindow is the hand-over point between a platform canvas
// that owns a native window (an Android ANativeWindow) and the OpenGL ES
// presenter that draws into it (gl.NewPresenterAndroid).
//
// The window itself cannot be the presenter's target: Android replaces it
// whenever the app comes back from the background, a covered page is
// shown again or the Surface is recreated. So the canvas creates one
// Holder per GPU canvas and gives the presenter its Handle
// (platform.CanvasNativeWindow.UseNativeWindow); the canvas stores the
// current window in it and the presenter reads it on every present.
//
// The presenter binds an EGL window surface to the window. An Android
// window can be connected to one producer at a time, and it must be let go
// before surfaceDestroyed returns, so the canvas tells the Holder
// synchronously before it gives a window up (ClearWindow, SetWindow with
// another window, Close) or locks it for a CPU frame (UseCPU): the Holder
// calls its User's DetachWindow first, which destroys the EGL surface.
//
// Every method runs on the platform's main thread (Android's Looper
// thread: the canvas's JNI callbacks and the frames uihost draws there).
// The package is plain Go, with no platform code, so both sides can
// import it and it is tested on any host.
package nativewindow

import "sync"

// User is the side drawing into a Holder's window (the presenter).
type User interface {
	// DetachWindow releases everything bound to the current window (the
	// EGL surface) before it returns: the window is about to go away or
	// to be drawn by the CPU. The Holder's window is still the old one
	// while it runs. Main thread.
	DetachWindow()
}

// Holder holds the current native window of one canvas. The zero value
// is not usable: create holders with New.
type Holder struct {
	handle uintptr
	win    uintptr // ANativeWindow*; 0 while there is none
	w, h   int     // buffer size in device pixels
	gen    uint64  // bumped whenever win changes to another window
	cpu    bool    // win was locked for a CPU frame: no EGL surface can attach to it
	closed bool
	user   User
}

// reg maps handles to live holders. Handles are small integers, never
// reused, so a stale handle finds nothing instead of another canvas.
var reg = struct {
	mu   sync.Mutex
	next uintptr
	m    map[uintptr]*Holder
}{m: map[uintptr]*Holder{}}

// New creates a holder without a window and registers it.
func New() *Holder {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.next++
	h := &Holder{handle: reg.next}
	reg.m[h.handle] = h
	return h
}

// Lookup returns the live holder of handle, or nil (unknown, or closed).
func Lookup(handle uintptr) *Holder {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.m[handle]
}

// Handle returns the holder's handle (never 0).
func (h *Holder) Handle() uintptr { return h.handle }

// SetWindow records the current window win (not 0) and its buffer size
// in device pixels. A different window than before detaches the user from
// the old one first and starts a new generation (the next present makes a
// new EGL surface); the same window only updates the size.
func (h *Holder) SetWindow(win uintptr, w, hgt int) {
	if h.closed || win == 0 {
		return
	}
	if win != h.win {
		h.detach()
		h.win = win
		h.gen++
		h.cpu = false
	}
	h.w, h.h = w, hgt
}

// ClearWindow forgets the window, detaching the user first (Android's
// surfaceDestroyed: call it before releasing the window).
func (h *Holder) ClearWindow() {
	if h.closed {
		return
	}
	h.detach()
	if h.win != 0 {
		h.win = 0
		h.gen++
	}
	h.cpu = false
}

// UseCPU is called before the canvas locks the window for a CPU frame. It
// detaches the user and marks the window as CPU-drawn: Android connects a
// locked window to the CPU until the Surface is destroyed, so no EGL
// surface can attach to it again. It reports whether a user was attached
// (the GPU was drawing into this window).
func (h *Holder) UseCPU() bool {
	if h.closed || h.win == 0 {
		return false
	}
	had := h.user != nil
	h.detach()
	h.cpu = true
	return had
}

// Close detaches the user, forgets the window and unregisters the holder:
// Lookup no longer finds it and every method does nothing. Idempotent.
func (h *Holder) Close() {
	if h.closed {
		return
	}
	h.detach()
	h.closed = true
	h.win = 0
	h.user = nil
	reg.mu.Lock()
	delete(reg.m, h.handle)
	reg.mu.Unlock()
}

// detach tells the user, if any, to let the window go.
func (h *Holder) detach() {
	if h.user != nil {
		h.user.DetachWindow()
	}
}

// Window returns the current window (0: none), its buffer size and its
// generation, which changes whenever the window is replaced or cleared.
func (h *Holder) Window() (win uintptr, w, hgt int, gen uint64) {
	return h.win, h.w, h.h, h.gen
}

// CPU reports whether the current window was locked for a CPU frame.
func (h *Holder) CPU() bool { return h.cpu }

// Closed reports whether Close was called.
func (h *Holder) Closed() bool { return h.closed }

// SetUser sets the user DetachWindow is called on (nil: none). It does
// not call the previous user.
func (h *Holder) SetUser(u User) {
	if h.closed {
		return
	}
	h.user = u
}
