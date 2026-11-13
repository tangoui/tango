package corehook

import "github.com/tangoui/tango/internal/platform"

// Proxy surfaces: tan dev's hot-reload workers show their canvas windows in
// another process (docs/specs/ui-hotreload.md). The app still gets an
// ordinary *tango.Surface for each; its native part forwards to the host.

// ProxyEvents feeds a proxy surface's events to the core, as a backend
// does through platform.Host. Main thread.
type ProxyEvents interface {
	// Event delivers a surface event (SurfaceClosed after Close, focus,
	// geometry, state).
	Event(ev platform.SurfaceEvent)
	// ShouldClose asks the surface's OnClose handlers.
	ShouldClose() bool
}

// NewProxySurface creates a *tango.Surface (returned as any) for app (a
// *tango.App) with opts (a tango.SurfaceOptions; only the window state the
// core records is used) whose native surface is the one create returns.
// create gets the new surface's id and its event sink. Main thread. Set by
// package tango in builds with tango_dev.
var NewProxySurface func(app any, opts any, create func(id platform.SurfaceID, ev ProxyEvents) (platform.Surface, error)) (surface any, err error)

// AppMenuProxy, when set, lets the menu package's SetAppMenu and
// SetSurfaceMenu for app (a *tango.App) and parent (0: the application
// menu) go somewhere else than the backend: it returns ok false to use the
// backend. Set by package uihost in tan dev workers.
var AppMenuProxy func(app any, parent platform.SurfaceID) (am platform.AppMenu, ok bool)
