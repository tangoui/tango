package corehook

import "github.com/tangoui/tango/internal/platform"

// NewCanvasSurface creates a surface whose content is a platform.Canvas
// (see docs/specs/canvas-surface.md). app is a *tango.App, opts a
// tango.SurfaceOptions (URL and Debug are ignored); surface is the new
// *tango.Surface. The surface is an ordinary one — closing, events,
// appearance and OnClose/OnClosed work as for NewSurface — and sink
// receives the canvas callbacks on the main thread. It must be called on
// the main thread. It returns an error wrapping platform.ErrUnsupported
// when the backend cannot create canvas surfaces. Set by package tango.
var NewCanvasSurface func(app any, opts any, sink platform.CanvasSink) (surface any, canvas platform.Canvas, err error)
