package corehook

import "github.com/tangoui/tango/internal/platform"

// AttachCanvas puts a platform.Canvas inside the WebView surface surface (a
// *tango.Surface), placed by layer (docs/specs/canvas-surface.md, step D).
// sink receives the canvas callbacks on the main thread, never before
// AttachCanvas returns. The returned Canvas also implements
// platform.AttachedCanvas when the backend can move or detach it. It must
// be called on the main thread. It returns an error wrapping
// platform.ErrUnsupported when the backend cannot mix a canvas with a
// WebView. Set by package tango.
var AttachCanvas func(surface any, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error)
