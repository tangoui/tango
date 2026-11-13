//go:build linux && !android

package linux

// Files dropped on a canvas surface created with SurfaceExtras.AcceptFiles.
// The canvas widget becomes a GTK drag destination for URI lists
// (gtk_drag_dest_set with GTK_DEST_DEFAULT_MOTION | GTK_DEST_DEFAULT_DROP
// and gtk_drag_dest_add_uri_targets): GTK answers the drag status, asks
// for the data on drop and finishes the drag itself; "drag-data-received"
// turns the file:// URIs into paths (localPath) and reports
// SurfaceFilesDropped with the drop point in widget coordinates (logical
// pixels; the canvas fills the content area). No highlight is drawn by GTK
// (no GTK_DEST_DEFAULT_HIGHLIGHT): a sink implementing
// platform.CanvasDropTarget draws its own from "drag-motion" (CanvasDragOver)
// and "drag-leave" (CanvasDragLeave, which GTK also emits right before a
// drop).
//
// The destination is the GtkOverlay around the drawing area when there is
// one (wrapGL), not the drawing area: GTK finds the drop target by walking
// up from the widget under the pointer, and once UseGLArea has put a
// GtkGLArea above the drawing area that widget is the GLArea, whose
// parent is the overlay (the drawing area, a sibling, would never be
// asked). The overlay has the drawing area's allocation, so the drag
// coordinates are the same.

import (
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

const (
	canvasGtkDestDefaultMotion = 1 << 0 // GtkDestDefaults
	canvasGtkDestDefaultDrop   = 1 << 2
	canvasGdkActionCopy        = 1 << 1 // GdkDragAction
)

var canvasDropFFI struct {
	once sync.Once
	ok   bool

	destSet           func(w ptr, flags int32, targets ptr, n int32, actions int32)
	destAddURITargets func(w ptr)
}

func loadCanvasDropFFI() bool {
	canvasDropFFI.once.Do(func() {
		var bd binder
		canvasDropFFI.ok = wx.dropOK &&
			bd.opt(&canvasDropFFI.destSet, "gtk_drag_dest_set", libGTK) &&
			bd.opt(&canvasDropFFI.destAddURITargets, "gtk_drag_dest_add_uri_targets", libGTK)
	})
	return canvasDropFFI.ok
}

// enableFileDrop makes the canvas widget (its overlay, when wrapped)
// accept dropped files; onDrop runs on the main thread with the paths and
// the drop point (logical pixels relative to the widget). Main thread.
func (c *canvas) enableFileDrop(onDrop func(paths []string, x, y int)) {
	if c.closed || c.dropOn || !loadCanvasDropFFI() {
		return
	}
	c.dropOn = true
	dest := c.widget
	if c.glOverlay != 0 {
		dest = c.glOverlay // see the file comment: the GLArea would hide the drawing area
	}
	canvasDropFFI.destSet(dest, canvasGtkDestDefaultMotion|canvasGtkDestDefaultDrop, 0, 0, canvasGdkActionCopy)
	canvasDropFFI.destAddURITargets(dest)

	// drag-motion(widget, context, x, y, time) -> gboolean
	connectExt(dest, "drag-motion", 4, func(a [7]uintptr) uintptr {
		if c.closed || !canvasDragOffersURIs(a[1]) {
			return 0
		}
		c.dragOver(float32(int32(a[2])), float32(int32(a[3])))
		return 0 // GTK_DEST_DEFAULT_MOTION answers the drag status
	})
	// drag-leave(widget, context, time)
	connectExt2(dest, "drag-leave", func(a [7]uintptr) uintptr {
		c.dragLeave()
		return 0
	})
	// drag-data-received(widget, context, x, y, selection_data, info, time)
	connectExt(dest, "drag-data-received", 6, func(a [7]uintptr) uintptr {
		sd := a[4]
		if c.closed || sd == 0 || wx.selectionGetTarget(sd) != wx.atomIntern("text/uri-list", false) {
			return 0
		}
		c.dragLeave() // in case GTK did not emit drag-leave first
		var paths []string
		if v := wx.selectionGetURIs(sd); v != 0 {
			for i := uintptr(0); ; i++ {
				u := *(*ptr)(unsafe.Add(toPtr(v), i*unsafe.Sizeof(ptr(0))))
				if u == 0 {
					break
				}
				if p, ok := localPath(goStr(u)); ok {
					paths = append(paths, p)
				}
			}
			wx.strfreev(v)
		}
		if len(paths) > 0 {
			onDrop(paths, int(int32(a[2])), int(int32(a[3])))
		}
		return 0 // GTK_DEST_DEFAULT_DROP calls gtk_drag_finish
	})
}

// canvasDragOffersURIs reports whether a drag offers text/uri-list.
func canvasDragOffersURIs(ctx ptr) bool {
	uriList := wx.atomIntern("text/uri-list", false)
	// struct GList { gpointer data; GList *next; GList *prev; }
	for l := wx.dragContextTargets(ctx); l != 0; l = *(*ptr)(unsafe.Add(toPtr(l), unsafe.Sizeof(ptr(0)))) {
		if *(*ptr)(toPtr(l)) == uriList {
			return true
		}
	}
	return false
}

// dragOver reports the hover position to a sink that wants it.
func (c *canvas) dragOver(x, y float32) {
	t, ok := c.sink.(platform.CanvasDropTarget)
	if !ok || !c.ready || c.closed {
		return
	}
	c.dropHover = true
	t.CanvasDragOver(x, y)
}

// dragLeave ends the hover feedback.
func (c *canvas) dragLeave() {
	if !c.dropHover {
		return
	}
	c.dropHover = false
	if t, ok := c.sink.(platform.CanvasDropTarget); ok && c.ready && !c.closed {
		t.CanvasDragLeave()
	}
}
