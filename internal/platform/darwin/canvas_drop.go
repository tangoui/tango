//go:build darwin && !ios

package darwin

// Files dropped on a canvas (SurfaceExtras.AcceptFiles). The canvas view
// of a canvas surface, and a canvas attached to a WebView surface that
// accepts files, register for file URLs and act as NSDraggingDestination:
// a drag carrying files shows the copy cursor, the sink hears
// CanvasDragOver / CanvasDragLeave if it implements
// platform.CanvasDropTarget, and the drop is reported to the Host as
// SurfaceFilesDropped with X/Y in the surface's content coordinates (the
// same as WebView surfaces, surface_ext.go). Drags without files are
// refused.

import (
	"math"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

const pasteboardTypeFileURL = "public.file-url" // NSPasteboardTypeFileURL

// registerDrop makes the view a drop target for files. Main thread.
func (c *canvas) registerDrop() {
	withPool(func() {
		send(c.view, "registerForDraggedTypes:", uintptr(nsArray(nsString(pasteboardTypeFileURL))))
	})
}

func canvasDropMethods() []objc.MethodDef {
	over := func(c *canvas, info id) uint {
		if !c.dropping {
			return nsDragOperationNone
		}
		if t, ok := c.sink.(platform.CanvasDropTarget); ok && c.ready {
			p := c.viewPointOf(msgGetPoint(info, sel("draggingLocation")))
			t.CanvasDragOver(float32(p.X), float32(p.Y))
		}
		return nsDragOperationCopy
	}
	return []objc.MethodDef{
		method("draggingEntered:", func(self id, _ objc.SEL, info id) uint {
			c := canvasOf(self)
			if c == nil {
				return nsDragOperationNone
			}
			c.dropping = c.s.ext.acceptFiles && len(draggedFiles(info)) > 0
			return over(c, info)
		}),
		method("draggingUpdated:", func(self id, _ objc.SEL, info id) uint {
			if c := canvasOf(self); c != nil {
				return over(c, info)
			}
			return nsDragOperationNone
		}),
		method("draggingExited:", func(self id, _ objc.SEL, info id) {
			if c := canvasOf(self); c != nil {
				c.dropLeave()
			}
		}),
		method("draggingEnded:", func(self id, _ objc.SEL, info id) {
			if c := canvasOf(self); c != nil {
				c.dropLeave()
			}
		}),
		method("prepareForDragOperation:", func(self id, _ objc.SEL, info id) bool {
			c := canvasOf(self)
			return c != nil && c.dropping
		}),
		method("performDragOperation:", func(self id, _ objc.SEL, info id) bool {
			c := canvasOf(self)
			if c == nil || !c.dropping {
				return false
			}
			paths := draggedFiles(info)
			c.dropLeave()
			if len(paths) == 0 || c.s.closed {
				return false
			}
			x, y := c.contentPoint(msgGetPoint(info, sel("draggingLocation")))
			c.s.b.h.OnSurfaceEvent(c.s.id, platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: x, Y: y})
			return true
		}),
		method("concludeDragOperation:", func(self id, _ objc.SEL, info id) {}),
	}
}

// dropLeave ends a drag over the view and tells the sink once.
func (c *canvas) dropLeave() {
	if !c.dropping {
		return
	}
	c.dropping = false
	if t, ok := c.sink.(platform.CanvasDropTarget); ok && c.ready && !c.closed {
		t.CanvasDragLeave()
	}
}

// viewPointOf converts a window point to view coordinates (logical,
// top-left: the view is flipped).
func (c *canvas) viewPointOf(win nsPoint) nsPoint {
	return msgConvertPoint(c.view, sel("convertPoint:fromView:"), win, 0)
}

// contentPoint converts a window point to the surface's content
// coordinates (top-left of the content area), rounded.
func (c *canvas) contentPoint(win nsPoint) (int, int) {
	content := send(c.s.win, "contentView")
	if content == 0 {
		p := c.viewPointOf(win)
		return int(math.Round(p.X)), int(math.Round(p.Y))
	}
	p := msgConvertPoint(content, sel("convertPoint:fromView:"), win, 0)
	if !sendBool(content, "isFlipped") {
		p.Y = msgRect(content, sel("bounds")).Size.Height - p.Y
	}
	return int(math.Round(p.X)), int(math.Round(p.Y))
}
