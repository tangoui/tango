package platform

// SurfaceID is allocated by the core, unique for the app's lifetime, never 0.
type SurfaceID uint64

type AppOptions struct {
	Name string // display name, e.g. "My App"
	ID   string // reverse-DNS identifier, e.g. "com.example.myapp"
}

type SurfaceOptions struct {
	Title         string
	Width, Height int // logical pixels; 0 means backend default (1024x768)
	MinWidth      int
	MinHeight     int
	Resizable     bool
	Visible       bool   // show immediately after creation
	URL           string // absolute; empty means about:blank
	Debug         bool   // allow devtools / inspector

	// BridgeScript is injected at document start in every frame-0 navigation,
	// AFTER the backend's own shim. The backend's shim must define, before
	// any page script runs:
	//
	//   window.__tangoNative = { post(str) { /* native postMessage */ } }
	//
	// The core supplies the rest of the JS runtime through BridgeScript.
	// Go→JS messages are delivered by the core with Eval.
	BridgeScript string
}

type LifecycleEvent int

const (
	LifecycleStarted     LifecycleEvent = iota + 1 // native app finished launching
	LifecycleForeground                            // app became active / visible
	LifecycleBackground                            // app resigned active / went to background
	LifecycleTerminating                           // about to exit; last chance to save
)

type SurfaceEventKind int

// SurfaceClosed timing (round 3): emitted exactly once per surface, when
// the native surface has been destroyed — after Surface.Close, after a
// user close that Host.ShouldClose allowed, or when the platform tore it
// down (an Android activity finishing, a window destroyed by the system).
// It may be emitted from inside Surface.Close. It is not emitted while
// Backend.NewSurface has not returned, nor for surfaces still open when
// Run returns (the core treats those as closed without the event). No
// other event for the id follows it, and the core no longer calls the
// Surface's methods.
const (
	SurfaceClosed  SurfaceEventKind = iota + 1 // native surface destroyed; see above
	SurfaceResized                             // Width/Height set
	SurfaceFocused                             // the window became the key / foreground window
	SurfaceBlurred                             // the window stopped being the key window
	SurfaceLoaded                              // main frame finished loading (DOM ready); URL set

	// Round 2 (ext.go). Backends emit these when they can.
	SurfaceNavigating   // main frame starts a navigation (link, reload, Navigate); URL set
	SurfaceLoadFailed   // main frame navigation failed; URL and Error set
	SurfaceMoved        // X/Y set
	SurfaceStateChanged // State set
	SurfaceFilesDropped // Paths, X/Y set (only when SurfaceExtras.AcceptFiles)
)

type SurfaceEvent struct {
	Kind          SurfaceEventKind
	Width, Height int
	X, Y          int
	URL           string
	Error         string
	State         WindowState
	Paths         []string
}
