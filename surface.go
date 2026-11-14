package tango

import (
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

// SurfaceOptions configures a new Surface.
type SurfaceOptions struct {
	_ [0]func()

	Title               string
	Width, Height       int // logical pixels; 0 means the backend default
	MinWidth, MinHeight int
	Resizable           bool
	Hidden              bool // create without showing; shown by default
	// URL is the page to load. Relative paths ("/index.html",
	// "index.html") resolve against the backend's asset origin; empty means
	// "/". Absolute URLs (http://localhost:5173/) load as is.
	URL   string
	Debug bool // allow the inspector

	// Window options (round 2; see surface_ext.go). Platforms ignore what
	// they cannot do.
	Kind   SurfaceKind // KindModal and KindSheet need Parent
	Parent *Surface
	// X, Y place the window's top-left corner (logical pixels, desktop
	// coordinates). When both are 0, or Centered is set, the window is
	// centered.
	X, Y                int
	Centered            bool
	MaxWidth, MaxHeight int         // 0 means no limit
	State               WindowState // initial state
	Appearance          Appearance
	AcceptFiles         bool // report dropped files (OnFilesDropped) instead of letting the page open them

	// WebView settings: user agent and extra request headers (webview.go).
	WebView WebViewOptions
}

// Surface is one native window (desktop) or page (mobile) hosting a
// WebView. Its methods are safe from any goroutine; off the main thread
// they run there (Send and Eval asynchronously, the rest waiting for it).
// After the surface has closed they do nothing.
type Surface struct {
	app    *App
	id     platform.SurfaceID
	closed atomic.Bool

	// Main thread only.
	ns        platform.Surface
	loaded    bool
	pending   []string // Send messages waiting for Loaded
	navOrigin string   // origin of the URL the core last navigated to
	title     string   // last title set (creation or SetTitle)
	visible   bool     // bookkeeping for surfaces without platform.SurfaceStatus
	focused   bool     // likewise, from SurfaceFocused / SurfaceBlurred

	mu        sync.Mutex // guards the handlers
	onClose   []func() bool
	onClosed  []func()
	onLoaded  []func()
	onFocused []func()
	onBlurred []func()

	ext surfaceExt // round-2 window state and handlers; surface_ext.go
	web surfaceWeb // round-4 WebView state and handlers; webview.go
}

// NewSurface creates a surface. Safe from any goroutine once the app runs;
// from other goroutines before Run it waits for Run to start. It returns
// ErrNotRunning on the main goroutine before Run and after the app quit.
func (a *App) NewSurface(opts SurfaceOptions) (*Surface, error) {
	if a.b == nil {
		return nil, ErrNoBackend
	}
	if !a.isMain() && a.mainBeforeRun() {
		return nil, ErrNotRunning
	}
	type result struct {
		s   *Surface
		err error
	}
	r, ok := await(a, func() result {
		s, err := a.createSurface(opts)
		return result{s, err}
	})
	if !ok {
		return nil, ErrNotRunning
	}
	return r.s, r.err
}

// createSurface runs on the main thread.
func (a *App) createSurface(opts SurfaceOptions) (*Surface, error) {
	opts = a.prepareOptions(opts) // hooks.go
	u := a.resolveURL(opts.URL)
	_, extrasTaken := a.b.(platform.CreatorEx)
	return a.addSurface(opts, originOf(u), extrasTaken, func(s *Surface) (platform.Surface, error) {
		return a.newNative(s, opts, platform.SurfaceOptions{
			Title:        opts.Title,
			Width:        opts.Width,
			Height:       opts.Height,
			MinWidth:     opts.MinWidth,
			MinHeight:    opts.MinHeight,
			Resizable:    opts.Resizable,
			Visible:      !opts.Hidden,
			URL:          u,
			Debug:        opts.Debug,
			BridgeScript: a.bridgeScript(),
		})
	})
}

// addSurface allocates and registers a surface and calls create to make
// its native part. extrasTaken tells afterCreate whether create passed the
// creation extras to the backend. Main thread.
func (a *App) addSurface(opts SurfaceOptions, navOrigin string, extrasTaken bool, create func(s *Surface) (platform.Surface, error)) (*Surface, error) {
	a.mu.Lock()
	a.nextID++
	s := &Surface{app: a, id: a.nextID, navOrigin: navOrigin, title: opts.Title, visible: !opts.Hidden}
	// Registered before the backend call: a backend may emit events for
	// the surface while creating it.
	a.surfaces[s.id] = s
	a.mu.Unlock()
	ns, err := create(s)
	if err != nil {
		a.mu.Lock()
		delete(a.surfaces, s.id)
		a.mu.Unlock()
		s.closed.Store(true)
		return nil, err
	}
	if s.closed.Load() { // closed while being created
		return s, nil
	}
	s.ns = ns
	s.afterCreate(opts, extrasTaken)
	s.flush()
	a.surfaceCreated(s)
	return s, nil
}

// resolveURL makes u absolute against the asset origin.
func (a *App) resolveURL(u string) string {
	origin := a.assetOrigin()
	if u == "" {
		return origin + "/"
	}
	if p, err := url.Parse(u); err == nil && p.Scheme != "" {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return origin + u
	}
	return origin + "/" + u
}

// originOf returns scheme://host[:port] of an absolute URL, or "" for URLs
// without a host (about:blank, data:, ...).
func originOf(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Scheme == "" || p.Host == "" {
		return ""
	}
	return strings.ToLower(p.Scheme) + "://" + strings.ToLower(p.Host)
}

// trusts reports whether a page at origin may talk to Go: the asset origin
// or the origin the core last navigated the surface to. Main thread.
func (s *Surface) trusts(origin string) bool {
	origin = strings.ToLower(strings.TrimSuffix(origin, "/"))
	if origin == "" {
		return false
	}
	return origin == strings.ToLower(s.app.assetOrigin()) || origin == s.navOrigin
}

// ID returns the surface's id, unique for the app's lifetime.
func (s *Surface) ID() platform.SurfaceID { return s.id }

// App returns the app the surface belongs to.
func (s *Surface) App() *App { return s.app }

// do runs fn with the native surface on the main thread and waits.
func (s *Surface) do(fn func(ns platform.Surface)) {
	if s.closed.Load() {
		return
	}
	s.app.awaitVoid(func() {
		if !s.closed.Load() && s.ns != nil {
			fn(s.ns)
		}
	})
}

func (s *Surface) Show()            { s.do(func(ns platform.Surface) { s.visible = true; ns.Show() }) }
func (s *Surface) Hide()            { s.do(func(ns platform.Surface) { s.visible = false; ns.Hide() }) }
func (s *Surface) Focus()           { s.do(platform.Surface.Focus) }
func (s *Surface) SetSize(w, h int) { s.do(func(ns platform.Surface) { ns.SetSize(w, h) }) }

// OpenDevTools opens the WebView inspector where the platform has one and
// the surface was created with SurfaceOptions.Debug (some platforms open
// it regardless). It does nothing otherwise.
func (s *Surface) OpenDevTools() { s.do(platform.Surface.OpenDevTools) }

// SetTitle sets the window title.
func (s *Surface) SetTitle(title string) {
	s.do(func(ns platform.Surface) { s.title = title; ns.SetTitle(title) })
}

// Title returns the title last set (SurfaceOptions.Title or SetTitle), ""
// once closed.
func (s *Surface) Title() (t string) {
	s.do(func(platform.Surface) { t = s.title })
	return
}

// IsVisible reports whether the window is shown (a minimized window still
// counts as shown). It asks the window system where the platform can tell,
// so hiding the whole app (appctl.Hide) is seen too; elsewhere it follows
// SurfaceOptions.Hidden, Show and Hide. False once closed.
func (s *Surface) IsVisible() (v bool) {
	s.do(func(ns platform.Surface) {
		if st, ok := ns.(platform.SurfaceStatus); ok {
			v = st.IsVisible()
			return
		}
		v = s.visible
	})
	return
}

// IsFocused reports whether the window has keyboard focus (is the key /
// foreground window). False once closed.
func (s *Surface) IsFocused() (f bool) {
	s.do(func(ns platform.Surface) {
		if st, ok := ns.(platform.SurfaceStatus); ok {
			f = st.IsFocused()
			return
		}
		f = s.focused
	})
	return
}

// Close closes the surface without asking OnClose handlers.
func (s *Surface) Close() { s.do(platform.Surface.Close) }

// Size returns the logical size, or 0, 0 once closed.
func (s *Surface) Size() (w, h int) {
	s.do(func(ns platform.Surface) { w, h = ns.Size() })
	return
}

// Native returns the native handle (NSWindow*, HWND, GtkWindow*, ...), or 0
// once closed.
func (s *Surface) Native() (p uintptr) {
	s.do(func(ns platform.Surface) { p = ns.Native() })
	return
}

// Navigate loads url (resolved like SurfaceOptions.URL). Messages sent
// afterwards wait for the new page's Loaded.
func (s *Surface) Navigate(u string) {
	if s.closed.Load() {
		return
	}
	s.app.awaitVoid(func() {
		if s.closed.Load() || s.ns == nil {
			return
		}
		abs := s.app.resolveURL(u)
		s.loaded = false
		s.navOrigin = originOf(abs)
		s.noteCoreNav(abs)
		s.ns.Navigate(abs)
	})
}

// Eval runs js in the page, fire-and-forget, in order with Send.
func (s *Surface) Eval(js string) {
	if s.closed.Load() {
		return
	}
	s.app.run(func() {
		if !s.closed.Load() && s.ns != nil {
			s.ns.Eval(js)
		}
	})
}

// Send delivers msg to the page's window.__tango.onMessage subscribers.
// Before the page has loaded (and after Navigate until it loads again)
// messages are kept and delivered in order on Loaded.
func (s *Surface) Send(msg string) {
	if s.closed.Load() {
		return
	}
	s.app.run(func() {
		if s.closed.Load() {
			return
		}
		s.pending = append(s.pending, msg)
		s.flush()
	})
}

// flush delivers pending messages if the page is ready. Main thread.
func (s *Surface) flush() {
	for s.loaded && s.ns != nil && !s.closed.Load() && len(s.pending) > 0 {
		msg := s.pending[0]
		s.pending[0] = ""
		s.pending = s.pending[1:]
		s.ns.Eval(recvJS(msg))
	}
	if len(s.pending) == 0 {
		s.pending = nil
	}
}

// OnClose registers fn to be asked when the user tries to close the
// surface (on the main thread, in order); if one returns false the
// surface stays open and later handlers are not asked.
func (s *Surface) OnClose(fn func() bool) {
	s.mu.Lock()
	s.onClose = append(s.onClose, fn)
	s.mu.Unlock()
}

// OnClosed registers fn to run on the main thread once the surface closed.
func (s *Surface) OnClosed(fn func()) {
	s.mu.Lock()
	s.onClosed = append(s.onClosed, fn)
	s.mu.Unlock()
}

// OnLoaded registers fn to run on the main thread each time a page
// finishes loading, after pending messages were delivered.
func (s *Surface) OnLoaded(fn func()) {
	s.mu.Lock()
	s.onLoaded = append(s.onLoaded, fn)
	s.mu.Unlock()
}

// OnFocused registers fn to run on the main thread when the window gains
// keyboard focus.
func (s *Surface) OnFocused(fn func()) {
	s.mu.Lock()
	s.onFocused = append(s.onFocused, fn)
	s.mu.Unlock()
}

// OnBlurred registers fn to run on the main thread when the window loses
// keyboard focus.
func (s *Surface) OnBlurred(fn func()) {
	s.mu.Lock()
	s.onBlurred = append(s.onBlurred, fn)
	s.mu.Unlock()
}

// handleFocus handles SurfaceFocused / SurfaceBlurred. Main thread.
func (s *Surface) handleFocus(focused bool) {
	if s.closed.Load() {
		return
	}
	s.focused = focused
	list := &s.onBlurred
	if focused {
		list = &s.onFocused
	}
	fire(s, list, func(fn func()) { fn() })
}

func (s *Surface) shouldClose() bool {
	s.mu.Lock()
	fns := slices.Clone(s.onClose)
	s.mu.Unlock()
	for _, fn := range fns {
		// A panicking handler does not keep the window open.
		if !guardCall(s.app, whereEvent, true, fn) {
			return false
		}
	}
	return true
}

func (s *Surface) handleClosed() {
	if s.closed.Swap(true) {
		return
	}
	s.pending = nil
	s.loaded = false
	a := s.app
	a.mu.Lock()
	delete(a.surfaces, s.id)
	a.mu.Unlock()
	s.mu.Lock()
	fns := slices.Clone(s.onClosed)
	s.onClose, s.onClosed, s.onLoaded = nil, nil, nil
	s.onFocused, s.onBlurred = nil, nil
	s.clearExtHandlers()
	s.clearWebHandlers()
	s.mu.Unlock()
	for _, fn := range fns {
		a.guard(whereEvent, fn)
	}
}

func (s *Surface) handleLoaded(u string) {
	if s.closed.Load() {
		return
	}
	// A page the core did not navigate to (the user followed a link
	// elsewhere) gets no messages; they wait for a trusted page.
	s.loaded = u == "" || s.trusts(originOf(u))
	s.notePageLoaded(u) // webview.go: source of downloads and permission requests
	s.onLoadedExt()
	s.flush()
	s.mu.Lock()
	fns := slices.Clone(s.onLoaded)
	s.mu.Unlock()
	for _, fn := range fns {
		s.app.guard(whereEvent, fn)
	}
}
