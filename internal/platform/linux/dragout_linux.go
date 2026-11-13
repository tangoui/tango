//go:build linux && !android

package linux

// platform.FileDragger: dragging files out of a surface with GTK drag and
// drop. The surface's GtkWindow is the drag source (so the drag never
// mixes with WebKit's own HTML5 drags on the web view); the files are
// offered as text/uri-list, which file managers, mail clients and
// browsers accept.
//
// gtk_drag_begin_with_coordinates is given no trigger event and falls
// back to the current one. On X11 that works from a relayed mousedown
// while the button is still held; on Wayland the compositor needs the
// serial of the press, so a drag started from an idle callback may be
// refused (drag-end then reports DragNone).

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.FileDragger = (*surface)(nil)

const (
	gdkActionCopy = 1 << 1
	gdkActionMove = 1 << 2
	gdkActionLink = 1 << 3
)

var dgo struct {
	once sync.Once
	ok   bool

	targetListNew     func(targets, n ptr) ptr
	targetListAddURIs func(list ptr, info uint32)
	targetListUnref   func(list ptr)
	dragBegin         func(widget, targets ptr, actions, button int32, event ptr, x, y int32) ptr
	selectedAction    func(ctx ptr) int32
	setIconPixbuf     func(ctx, pixbuf ptr, hotX, hotY int32)
	selectionSetURIs  func(sd ptr, uris unsafe.Pointer) bool
}

func bindDragOut() bool {
	dgo.once.Do(func() {
		if loadSystemLibs() != nil {
			return
		}
		var bd binder
		t, d := libGTK, libGDK
		bd.req(&dgo.targetListNew, "gtk_target_list_new", t)
		bd.req(&dgo.targetListAddURIs, "gtk_target_list_add_uri_targets", t)
		bd.req(&dgo.targetListUnref, "gtk_target_list_unref", t)
		bd.req(&dgo.dragBegin, "gtk_drag_begin_with_coordinates", t)
		bd.req(&dgo.selectedAction, "gdk_drag_context_get_selected_action", d, t)
		bd.req(&dgo.setIconPixbuf, "gtk_drag_set_icon_pixbuf", t)
		bd.req(&dgo.selectionSetURIs, "gtk_selection_data_set_uris", t)
		dgo.ok = len(bd.missing) == 0
	})
	return dgo.ok
}

// dragOut is a surface's drag-out state (main thread).
type dragOut struct {
	connected bool
	ctx       ptr // GdkDragContext of the drag in progress, 0 if none
	uris      []string
	done      func(platform.DragOperation)
}

var dragOuts = map[*surface]*dragOut{} // main thread

func (s *surface) StartFileDrag(paths []string, icon []byte, done func(platform.DragOperation)) error {
	if s.closed {
		return fmt.Errorf("tango/linux: surface closed")
	}
	if !bindDragOut() {
		return fmt.Errorf("%w: GTK drag-and-drop symbols missing", platform.ErrUnsupported)
	}
	d := dragOuts[s]
	if d == nil {
		d = &dragOut{}
		dragOuts[s] = d
		connect(s.win, "destroy", 0, func(_, _ ptr) ptr {
			if d.done != nil {
				fn := d.done
				d.done = nil
				fn(platform.DragNone)
			}
			delete(dragOuts, s)
			return 0
		})
	}
	if d.ctx != 0 {
		return fmt.Errorf("tango/linux: a drag is already in progress")
	}
	if !d.connected {
		d.connected = true
		// drag-data-get(widget, context, selection_data, info, time)
		connectExt(s.win, "drag-data-get", 4, func(a [7]uintptr) uintptr {
			if a[1] != d.ctx || a[2] == 0 {
				return 0
			}
			setURIs(a[2], d.uris)
			return 0
		})
		// drag-end(widget, context)
		connect(s.win, "drag-end", 1, func(_, ctx ptr) ptr {
			if ctx != d.ctx {
				return 0
			}
			op := dragOp(dgo.selectedAction(ctx))
			d.ctx, d.uris = 0, nil
			if fn := d.done; fn != nil {
				d.done = nil
				fn(op)
			}
			return 0
		})
	}
	uris := make([]string, len(paths))
	for i, p := range paths {
		uris[i] = fileURI(p)
	}
	tl := dgo.targetListNew(0, 0)
	dgo.targetListAddURIs(tl, 0)
	ctx := dgo.dragBegin(s.win, tl, gdkActionCopy|gdkActionMove|gdkActionLink, 1, 0, -1, -1)
	dgo.targetListUnref(tl)
	if ctx == 0 {
		later(0, func() { done(platform.DragNone) })
		return nil
	}
	d.ctx, d.uris, d.done = ctx, uris, done
	if pb := pixbufFromPNG(icon); pb != 0 {
		dgo.setIconPixbuf(ctx, pb, 0, 0)
		gObjectUnref(pb)
	}
	return nil
}

func dragOp(action int32) platform.DragOperation {
	switch {
	case action&gdkActionMove != 0:
		return platform.DragMove
	case action&gdkActionCopy != 0:
		return platform.DragCopy
	case action&gdkActionLink != 0:
		return platform.DragLink
	}
	return platform.DragNone
}

// setURIs answers drag-data-get with a NULL-terminated URI array.
func setURIs(sd ptr, uris []string) {
	bufs := make([][]byte, len(uris))
	arr := make([]*byte, len(uris)+1)
	for i, u := range uris {
		bufs[i] = append([]byte(u), 0)
		arr[i] = &bufs[i][0]
	}
	dgo.selectionSetURIs(sd, unsafe.Pointer(&arr[0]))
	runtime.KeepAlive(bufs)
	runtime.KeepAlive(arr)
}

// pixbufFromPNG decodes png into a new GdkPixbuf (0 if empty or broken).
func pixbufFromPNG(png []byte) ptr {
	if len(png) == 0 {
		return 0
	}
	var gerr ptr
	l := gdkPixbufLoaderNewWithType("png", &gerr)
	if l == 0 {
		gErrorMessage(gerr)
		return 0
	}
	defer gObjectUnref(l)
	ok := gdkPixbufLoaderWrite(l, unsafe.Pointer(&png[0]), uintptr(len(png)), &gerr)
	runtime.KeepAlive(png)
	var e2 ptr
	closed := gdkPixbufLoaderClose(l, &e2)
	if !ok || !closed {
		gErrorMessage(gerr)
		gErrorMessage(e2)
		return 0
	}
	pb := gdkPixbufLoaderGetPixbuf(l)
	if pb == 0 {
		return 0
	}
	bindMenuExt()
	if mx.objectRefSink == nil {
		return 0
	}
	mx.objectRefSink(pb) // GdkPixbuf is not floating: this adds our reference
	return pb
}
