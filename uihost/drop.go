package uihost

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
)

// File drag feedback: the host is a platform.CanvasDropTarget, so backends
// report files dragged over a canvas surface created with
// SurfaceOptions.AcceptFiles. The position goes to the runner
// (ui.Runner.FileDragOver), where ui.DropZone and Context.FileDrag read it.
//
// The drop itself arrives as SurfaceFilesDropped on the surface
// (tango.Surface.OnFilesDropped) after CanvasDragLeave; AcceptDrops
// (upload.go) forwards it to Runner.DropFiles, and the DropZone under the
// point (Upload's dragger) receives the paths.

var _ platform.CanvasDropTarget = (*host)(nil)

// CanvasDragOver reports files dragged over the canvas at (x, y), logical
// pixels from its top-left corner (the runner's window coordinates).
func (h *host) CanvasDragOver(x, y float32) {
	if h.closed || h.r == nil {
		return
	}
	h.r.FileDragOver(geom.Pt(x, y))
	h.invalidate()
}

// CanvasDragLeave reports that the drag left the canvas, was cancelled or
// dropped.
func (h *host) CanvasDragLeave() {
	if h.closed || h.r == nil {
		return
	}
	h.r.FileDragLeave()
	h.invalidate()
}
