package corehook

import "github.com/tangoui/tango/internal/platform"

// Hooks for the development features of package internal/devfeat (window
// placement across `tan dev` restarts, the automation channel), which only
// do something in builds with -tags tango_dev. All are set by package
// tango.

// SurfacePlacement is the part of a tango.SurfaceOptions OnPrepareSurface
// handlers see. Changes to X, Y, Width, Height, Centered and State go into
// the options of the surface being created; Title is read only.
type SurfacePlacement struct {
	Title               string
	X, Y, Width, Height int
	Centered            bool
	State               platform.WindowState
}

// OnPrepareSurface registers fn to adjust the placement of every surface
// (WebView or canvas) app (a *tango.App) is about to create, before the
// native surface exists. fn runs on the main thread.
var OnPrepareSurface func(app any, fn func(p *SurfacePlacement))

// Await runs fn on the main thread of app (a *tango.App) and waits,
// reporting whether it ran (false after the app has quit). On the main
// thread it calls fn directly.
var Await func(app any, fn func()) bool

// Done returns a channel closed once app (a *tango.App) has quit.
var Done func(app any) <-chan struct{}

// Surfaces returns the open surfaces of app (a *tango.App) in creation
// order, each a *tango.Surface (App.Surfaces).
var Surfaces func(app any) []any

// SurfaceOf returns the surface with id of app (a *tango.App) as a
// *tango.Surface, or nil if there is none.
var SurfaceOf func(app any, id platform.SurfaceID) any

// SurfaceClosed reports whether surface (a *tango.Surface) has closed.
var SurfaceClosed func(surface any) bool

// SurfaceLoaded reports whether the page of surface (a *tango.Surface) has
// finished loading. Main thread.
var SurfaceLoaded func(surface any) bool
