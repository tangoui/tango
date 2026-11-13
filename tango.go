// Package tango is the core of the Tango application framework: one Go
// program that runs on desktop (macOS, Windows, Linux) and mobile, showing
// its interface in the system WebView.
//
// An App owns the native event loop and a queue of work for the main (UI)
// thread. A Surface is one window (desktop) or page (mobile) hosting a
// WebView. Pages and Go exchange raw string messages: pages call
// window.__tango.post(str) and window.__tango.onMessage(fn); Go calls
// Surface.Send and App.OnMessage. Higher layers (ipc, bind) build on that.
//
// # Threading
//
// App.Run must be called from the main goroutine (from main()); the package
// locks it to the process' main thread in init on desktop. Everything else
// is safe from any goroutine. Callbacks (OnReady, OnLifecycle, OnMessage,
// Surface.OnClose, ...) run on the main thread; keep them short.
//
// Post queues work for the main thread. Await runs a function there and
// waits for its result; AwaitAsync starts asynchronous work there (a
// dialog, say) and waits until it calls done. Called on the main thread,
// Await runs directly and AwaitAsync keeps pumping native events while it
// waits, so nested use does not deadlock.
//
// After the app has quit (Run returned), Post drops its function, Await and
// AwaitAsync return the zero value without running anything, Surface
// methods do nothing and NewSurface returns ErrNotRunning. Calling Await,
// AwaitAsync or NewSurface on the main goroutine before Run would deadlock;
// Await and AwaitAsync panic and NewSurface returns ErrNotRunning instead.
// Create surfaces in OnReady.
package tango

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/devfeat"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/apphook"
	"github.com/tangoui/tango/internal/platform/native"
)

// ErrNotRunning is returned by calls that need the running app when it has
// not started yet (on the main goroutine) or has already quit.
var ErrNotRunning = errors.New("tango: app is not running")

// ErrNoBackend is returned by Run when no platform backend is built in for
// this GOOS.
var ErrNoBackend = errors.New("tango: no platform backend for this platform")

// Options configures an App.
type Options struct {
	Name, ID string
	// Assets serves requests under the backend's asset origin; nil
	// answers 404. Requests other than GET and HEAD whose Origin header
	// names another origin (a remote page posting a form to /api, say)
	// are answered 403 before they reach it; see assets.go.
	Assets http.Handler
	// AllowCrossOriginAssets turns that Origin check off, for apps whose
	// asset handler does its own (CORS, tokens).
	AllowCrossOriginAssets bool

	// QuitOnLastClose quits the app when its last surface closes. Nil
	// means the platform default: false on macOS (apps stay in the Dock),
	// true on Windows and Linux. Ignored on mobile. See lifecycle.go.
	QuitOnLastClose *bool
	// URLSchemes are the custom URL schemes the app handles ("myapp" for
	// myapp://...). Launch arguments using them are delivered as
	// EventOpenURL; see lifecycle.go. Registering them with the OS is done
	// when packaging.
	URLSchemes []string

	// OnPanic is called when application code the framework runs panics:
	// main-thread work (Post, Await, AwaitAsync), OnReady, event handlers,
	// App.Go goroutines, ipc bindings (see docs/panics.md). It may run on
	// any goroutine. Returning false exits the process with status 2;
	// for ipc bindings the result is ignored (they only report). Nil
	// means: log the panic with log/slog, keep running in development
	// builds (-tags tango_dev), and in production builds (the default)
	// show an error dialog and exit with status 2.
	OnPanic func(p Panic) (keepRunning bool)
}

// Lifecycle is an application lifecycle event.
type Lifecycle = platform.LifecycleEvent

const (
	Started     = platform.LifecycleStarted
	Foreground  = platform.LifecycleForeground
	Background  = platform.LifecycleBackground
	Terminating = platform.LifecycleTerminating
)

const (
	stateNew int32 = iota
	stateRunning
	stateClosed
)

// App is a Tango application. Create it with New.
type App struct {
	opts Options
	b    platform.Backend

	state    atomic.Int32
	ran      atomic.Bool
	mainG    atomic.Uint64 // goroutine currently acting as the main thread; see enterMain
	closedCh chan struct{}
	dying    atomic.Bool // a fatal panic is ending the process; see panic.go

	loop mainLoop

	mu       sync.Mutex // guards the fields below
	started  bool
	onReady  []func()
	onLife   []func(Lifecycle)
	onMsg    []func(*Surface, string)
	onNew    []func(*Surface)
	onPrep   []func(*SurfaceOptions) // internal: adjust options before a surface is created (hooks.go)
	scripts  []string
	surfaces map[platform.SurfaceID]*Surface
	nextID   platform.SurfaceID
	origin   string

	ext appExt // round-2 app events; see events.go
	web appWeb // round-4 WebView handlers; see webview.go

	intl internalRoutes // internal page→Go messages; see hooks.go
}

func init() {
	apphook.NewApp = func(b platform.Backend, opts any) any {
		o, _ := opts.(Options)
		return newWithBackend(o, b)
	}
}

// New returns an App on the platform's default backend.
func New(opts Options) *App {
	return newWithBackend(opts, native.NewBackend())
}

func newWithBackend(opts Options, b platform.Backend) *App {
	a := &App{
		opts:     opts,
		b:        b,
		closedCh: make(chan struct{}),
		surfaces: map[platform.SurfaceID]*Surface{},
	}
	a.loop.app = a
	devfeat.AttachWindowState(a) // tan dev window placement; internal/devfeat
	devfeat.AttachAutomation(a)  // TANGO_AUTOMATION control channel; internal/devfeat
	return a
}

// Backend returns the platform backend, for capability packages to
// type-assert optional interfaces on. Nil if none is built in.
func (a *App) Backend() platform.Backend { return a.b }

// Run initializes the backend and runs the native event loop until the app
// quits. Call it once, from the main goroutine.
func (a *App) Run() error {
	if a.b == nil {
		a.shutdown()
		return ErrNoBackend
	}
	if !a.ran.CompareAndSwap(false, true) {
		return errors.New("tango: Run called twice")
	}
	if native.RunOwnsMainThread {
		defer a.enterMain()()
	}
	if err := a.b.Init(host{a}, platform.AppOptions{Name: a.opts.Name, ID: a.opts.ID}); err != nil {
		a.shutdown()
		return err
	}
	a.mu.Lock()
	a.origin = strings.TrimSuffix(a.b.AssetOrigin(), "/")
	a.mu.Unlock()
	a.loop.start()
	a.OnReady(a.dispatchLaunchArgs)
	err := a.b.Run()
	a.shutdown()
	return err
}

// shutdown closes the main-thread queue, drops whatever is still queued,
// releases every waiter and marks remaining surfaces closed. Main thread
// (or before Run).
func (a *App) shutdown() {
	if a.state.Swap(stateClosed) == stateClosed {
		return
	}
	a.loop.close()
	close(a.closedCh)
	a.mu.Lock()
	ss := a.surfaces
	a.surfaces = map[platform.SurfaceID]*Surface{}
	a.mu.Unlock()
	for _, s := range ss {
		s.closed.Store(true)
	}
}

// Quit asks the app to quit. Safe from any goroutine; before Run it makes
// Run quit right after starting.
func (a *App) Quit() {
	if a.b == nil {
		return
	}
	a.run(a.b.Quit)
}

// OnReady registers fn to run on the main thread once the app has started.
// Registered after that, fn is posted to run soon.
func (a *App) OnReady(fn func()) {
	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		a.Post(func() { a.guard(whereReady, fn) })
		return
	}
	a.onReady = append(a.onReady, fn)
	a.mu.Unlock()
}

// OnLifecycle registers fn for every lifecycle event, on the main thread.
func (a *App) OnLifecycle(fn func(Lifecycle)) {
	a.mu.Lock()
	a.onLife = append(a.onLife, fn)
	a.mu.Unlock()
}

// OnMessage registers fn for raw messages any surface's page sends with
// window.__tango.post. Messages from origins other than the asset origin or
// the origin the core last navigated the surface to are dropped.
func (a *App) OnMessage(fn func(s *Surface, msg string)) {
	a.mu.Lock()
	a.onMsg = append(a.onMsg, fn)
	a.mu.Unlock()
}

// OnSurfaceCreated registers fn to run on the main thread each time a
// surface (WebView or canvas) has been created, before NewSurface returns
// it; the surface's page may not have loaded yet. Surfaces that exist when
// fn is registered are not reported: see Surfaces. Packages use it to set
// up per-window state (badges on new taskbar buttons, page sync, window
// state persistence).
func (a *App) OnSurfaceCreated(fn func(*Surface)) {
	a.mu.Lock()
	a.onNew = append(a.onNew, fn)
	a.mu.Unlock()
}

// surfaceCreated runs the OnSurfaceCreated handlers. Main thread.
func (a *App) surfaceCreated(s *Surface) {
	a.mu.Lock()
	fns := slices.Clone(a.onNew)
	a.mu.Unlock()
	for _, fn := range fns {
		if s.closed.Load() {
			return
		}
		a.guard(whereEvent, func() { fn(s) })
	}
}

// Name returns Options.Name.
func (a *App) Name() string { return a.opts.Name }

// AddBridgeScript appends js to the bridge script of surfaces created after
// the call, after the core shim (window.__tango) and earlier scripts.
func (a *App) AddBridgeScript(js string) {
	a.mu.Lock()
	a.scripts = append(a.scripts, js)
	a.mu.Unlock()
}

func (a *App) bridgeScript() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var sb strings.Builder
	sb.WriteString(coreShim)
	for _, s := range a.scripts {
		sb.WriteString("\n;")
		sb.WriteString(s)
	}
	return sb.String()
}

// Surfaces returns the open surfaces in creation order.
func (a *App) Surfaces() []*Surface {
	a.mu.Lock()
	ss := make([]*Surface, 0, len(a.surfaces))
	for _, s := range a.surfaces {
		ss = append(ss, s)
	}
	a.mu.Unlock()
	slices.SortFunc(ss, func(x, y *Surface) int { return int(x.id) - int(y.id) })
	return ss
}

func (a *App) surface(id platform.SurfaceID) *Surface {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.surfaces[id]
}

func (a *App) assetOrigin() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.origin
}

// host implements platform.Host for an App, keeping Drain and friends off
// App's public method set.
type host struct{ a *App }

func (h host) Drain() {
	defer h.a.enterMain()()
	h.a.loop.drain()
}

func (h host) OnLifecycle(ev platform.LifecycleEvent) {
	a := h.a
	defer a.enterMain()()
	a.mu.Lock()
	life := slices.Clone(a.onLife)
	var ready []func()
	if ev == platform.LifecycleStarted && !a.started {
		a.started = true
		ready, a.onReady = a.onReady, nil
	}
	a.mu.Unlock()
	for _, fn := range life {
		a.guard(whereEvent, func() { fn(ev) })
	}
	for _, fn := range ready {
		a.guard(whereReady, fn)
	}
}

func (h host) ShouldClose(id platform.SurfaceID) bool {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil {
		return true
	}
	return s.shouldClose()
}

func (h host) OnSurfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent) {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil {
		return
	}
	switch ev.Kind {
	case platform.SurfaceClosed:
		s.handleClosed()
		h.a.afterSurfaceClosed()
	case platform.SurfaceLoaded:
		s.handleLoaded(ev.URL)
	case platform.SurfaceFocused, platform.SurfaceBlurred:
		s.handleFocus(ev.Kind == platform.SurfaceFocused)
	default:
		s.handleExtEvent(ev) // surface_ext.go
	}
}

func (h host) OnMessage(id platform.SurfaceID, origin, msg string) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || !s.trusts(origin) {
		return
	}
	if a.dispatchInternal(s, msg) {
		return
	}
	a.mu.Lock()
	fns := slices.Clone(a.onMsg)
	a.mu.Unlock()
	for _, fn := range fns {
		a.guard(whereEvent, func() { fn(s, msg) })
	}
}

// ServeAsset may run on any thread (contract round 2), so it must not claim
// the main thread via enterMain.
func (h host) ServeAsset(id platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	if h.a.opts.Assets == nil {
		http.NotFound(w, r)
		return
	}
	if !h.a.opts.AllowCrossOriginAssets && !h.a.assetRequestAllowed(r) {
		http.Error(w, "tango: cross-origin request to the asset origin refused", http.StatusForbidden)
		return
	}
	serveFollowingRedirects(h.a.opts.Assets, w, h.a.assetRequest(id, r))
}
