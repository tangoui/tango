// Package platform defines the contract between Tango's core (the root tango
// package) and each native backend (darwin, windows, linux, ios, android,
// fake).
//
// Rules every backend follows:
//
//   - Every method on Backend and Surface is called on the main (UI) thread,
//     except Backend.Wake, which is safe from any goroutine, and — on
//     mobile — Init and Run (see below).
//   - Mobile (round 3, documenting what ios / android do): the native host
//     owns the main thread and starts main.main on a goroutine of its own,
//     so Init and Run are called on the goroutine that called App.Run, not
//     on the UI thread. Init only records the Host; Run blocks that
//     goroutine until the app terminates. LifecycleStarted, and later
//     LifecycleTerminating, are delivered on the main thread like every
//     other Host call; Run returns only after Terminating was delivered.
//   - Every Host method is called by the backend on the main thread.
//   - Backends translate; they hold no policy. Queues, ID allocation,
//     message routing and lifecycle state machines live in the core.
//   - Desktop backends never use cgo (purego / syscall only). Mobile backends
//     may use cgo and live behind ios / android build tags.
//
// This file is the shared contract. Backends must not change it; propose
// changes instead.
package platform

import "net/http"

// Backend is the minimum a platform must implement to run a Tango app.
// Optional features (menus, dialogs, clipboard, ...) are separate interfaces
// in caps.go that a backend may additionally implement.
type Backend interface {
	// Init prepares the native application object. Called once on the main
	// thread before Run. The backend keeps h and calls it on the main thread.
	Init(h Host, opts AppOptions) error

	// Run blocks on the main thread running the native event loop until Quit
	// is called or the platform terminates the app. Before returning it calls
	// h.OnLifecycle(LifecycleTerminating).
	Run() error

	// Quit asks the native loop to stop. Main thread only.
	Quit()

	// Wake schedules h.Drain() to run soon on the main thread. Safe from any
	// goroutine, may be called many times; coalescing is allowed but at least
	// one Drain must follow every Wake.
	Wake()

	// Step processes native events once. If block is true it waits for at
	// least one event (or a Wake). Returns false once the loop is quitting.
	// Used by the core to keep the UI responsive while the main thread waits
	// on something (nested loops). Main thread only.
	Step(block bool) bool

	// NewSurface creates a native surface (a desktop window or a mobile
	// page) hosting a WebView. It is created hidden unless opts.Visible.
	NewSurface(id SurfaceID, opts SurfaceOptions) (Surface, error)

	// AssetOrigin is the origin the backend serves Host.ServeAsset under,
	// without trailing slash, e.g. "tango://app" (WebKit custom scheme) or
	// "https://tango.localhost" (WebView2 / Android request interception).
	AssetOrigin() string
}

// Surface is one native window/page containing a WebView.
type Surface interface {
	Show()
	Hide()
	// Close destroys the surface without asking Host.ShouldClose. The backend
	// then emits SurfaceClosed.
	Close()
	SetTitle(title string)
	SetSize(width, height int) // logical (DIP) pixels
	Size() (width, height int)
	Focus()

	// Navigate loads an absolute URL. The core resolves app-relative paths
	// against AssetOrigin before calling this.
	Navigate(url string)

	// Eval runs JavaScript in the page's main frame, fire-and-forget.
	Eval(js string)

	// OpenDevTools opens the inspector if the platform supports it; no-op
	// otherwise.
	OpenDevTools()

	// Native returns the native handle (NSWindow*, HWND, GtkWindow*, ...).
	Native() uintptr
}

// Host is implemented by the core. Backends call it on the main thread.
type Host interface {
	// Drain runs work queued with App.Post. Called after Wake.
	Drain()

	OnLifecycle(ev LifecycleEvent)

	// ShouldClose is asked when the user tries to close a surface (window
	// close button, Android back on the root page). Returning false keeps it
	// open. If true the backend closes it and emits SurfaceClosed.
	ShouldClose(id SurfaceID) bool

	OnSurfaceEvent(id SurfaceID, ev SurfaceEvent)

	// OnMessage delivers a string the page sent with
	// window.__tangoNative.post(str). origin is the page origin as reported by
	// the WebView (scheme://host[:port]) so the core can reject untrusted
	// frames. Only main-frame messages are delivered. Where the platform
	// cannot tell which frame sent a message (WebKitGTK 4.1, Android
	// JavascriptInterface), the backend reports the main frame's current
	// origin and the bridge shim must refuse to post from sub-frames.
	OnMessage(id SurfaceID, origin string, msg string)

	// ServeAsset serves a request made to AssetOrigin. Backends adapt their
	// native scheme handler / request interception to net/http. id is the
	// surface that issued the request, or 0 if unknown.
	//
	// EXCEPTION to the main-thread rule (revised round 2): ServeAsset may be
	// called from any goroutine/thread, and backends SHOULD call it off the
	// main thread so slow or streaming handlers never block the UI. Writes to
	// w may happen after ServeAsset returns only if the backend documents
	// it; by default the response is complete when ServeAsset returns.
	ServeAsset(id SurfaceID, w http.ResponseWriter, r *http.Request)
}
