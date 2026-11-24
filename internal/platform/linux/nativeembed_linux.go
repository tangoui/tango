//go:build linux && !android

package linux

// App-owned native views embedded in a canvas surface
// (platform.NativeViewEmbedder). Each view is put in its own GtkLayout,
// added to the canvas's GtkOverlay above the canvas and the embedded web
// views and placed like them (webembed_linux.go): the layout is the clip
// rectangle (get-child-position), the application's widget its frame.
// With PassInput the layout is an overlay pass-through child and every
// GdkWindow inside it is made pass-through when it maps, so pointer events
// reach the canvas (passThroughNative; not verified on a GTK desktop yet). The application keeps its reference to the widget;
// it is taken out of the layout before the layout is destroyed.

import (
	"errors"
	"fmt"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.NativeViewEmbedder = (*backend)(nil)

var nativeByLayout = map[ptr]*embeddedNative{}

// embeddedNative is an application's GtkWidget placed in a canvas surface.
type embeddedNative struct {
	h      *embedHost
	layout ptr
	view   ptr
	p      platform.WebPlacement
	track  embedTracker
	closed bool
	onGone func() // the parent closed (platform.NativeViewEmbedder)
}

// NewEmbeddedNativeView implements platform.NativeViewEmbedder. Main thread.
func (b *backend) NewEmbeddedNativeView(parent platform.SurfaceID, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error) {
	if !b.loaded.Load() {
		return nil, errors.New("tango/linux: NewEmbeddedNativeView before Init")
	}
	ps := b.surfaces[parent]
	if ps == nil || ps.closed {
		return nil, fmt.Errorf("tango/linux: NewEmbeddedNativeView: no open surface %d", parent)
	}
	if ps.canvas == nil || ps.canvas.glOverlay == 0 {
		return nil, fmt.Errorf("%w: the canvas has no GtkOverlay", platform.ErrUnsupported)
	}
	if view == 0 {
		return nil, errors.New("tango/linux: NewEmbeddedNativeView: no view")
	}
	if err := loadEmbedFFI(); err != nil {
		return nil, fmt.Errorf("%w: %v", platform.ErrUnsupported, err)
	}
	f := &embedFFI
	layout := f.layoutNew(0, 0)
	if layout == 0 {
		return nil, errors.New("tango/linux: gtk_layout_new failed")
	}
	f.layoutPut(layout, ptr(view), 0, 0)
	gtkWidgetShow(ptr(view)) // the layout stays hidden until a visible placement
	h := embedHostFor(ps, ps.canvas.glOverlay, nil)
	n := &embeddedNative{h: h, layout: layout, view: ptr(view), onGone: closed}
	h.natives = append(h.natives, n)
	nativeByLayout[layout] = n
	f.overlayAddOverlay(h.overlay, layout) // last: above the canvas and earlier views
	if opts.PassInput {
		passThroughNative(h.overlay, layout)
	}
	return n, nil
}

// passThroughNative makes the layout transparent to the pointer: an
// overlay pass-through child, and every GdkWindow inside it (the layout's
// own windows and those of the application's widget) pass-through, on
// every map, as the GL path does for the GLArea (canvas_gl_linux.go).
// Needs GTK 3.18; on older GTK the view takes the input over its area.
func passThroughNative(overlay, layout ptr) {
	if f := &embedFFI; f.overlayPassThru != nil {
		f.overlayPassThru(overlay, layout, true)
	}
	if loadCanvasGLFFI() != nil {
		return
	}
	connect(layout, "map", 0, func(_, _ ptr) ptr {
		g := &canvasGLFFI
		if parent := g.widgetGetParentWindow(layout); parent != 0 {
			passThroughWindows(parent)
		}
		return 0
	})
}

// passThroughWindows makes win's descendants pass-through.
func passThroughWindows(win ptr) {
	g := &canvasGLFFI
	list := g.windowGetChildren(win)
	for l := list; l != 0; l = (*gListNode)(toPtr(l)).next {
		child := (*gListNode)(toPtr(l)).data
		g.windowSetPassThrough(child, true)
		passThroughWindows(child)
	}
	g.listFree(list)
}

// SetPlacement implements platform.EmbeddedNativeView.
func (n *embeddedNative) SetPlacement(p platform.WebPlacement) {
	if n.closed || p == n.p {
		return
	}
	n.p = p
	f := &embedFFI
	st := n.track.update(p.Geometry(1, false)) // GTK positions in logical pixels
	if st.Hide {
		gtkWidgetHide(n.layout)
	}
	if st.Frame {
		fr := n.track.frame
		gtkWidgetSetSizeRequest(n.view, int32(fr.Dx()), int32(fr.Dy()))
		f.layoutMove(n.layout, n.view, int32(fr.Min.X), int32(fr.Min.Y))
	}
	if st.Clip {
		f.widgetQueueResize(n.layout) // get-child-position runs again
	}
	if st.Show {
		gtkWidgetShow(n.layout)
	}
}

// Close implements platform.EmbeddedNativeView.
func (n *embeddedNative) Close() {
	if n.closed {
		return
	}
	n.remove()
	h := n.h
	for i, x := range h.natives {
		if x == n {
			h.natives = append(h.natives[:i:i], h.natives[i+1:]...)
			break
		}
	}
}

// remove takes the application's widget out (it keeps its own reference)
// and destroys the layout.
func (n *embeddedNative) remove() {
	n.closed = true
	delete(nativeByLayout, n.layout)
	gtkWidgetHide(n.view)
	embedFFI.containerRemove(n.layout, n.view)
	gtkWidgetDestroy(n.layout)
}

// closeNatives gives back the native views of a closing parent and tells
// their owners (closeEmbedded, from the parent's "destroy").
func (h *embedHost) closeNatives() {
	list := h.natives
	h.natives = nil
	for i := len(list) - 1; i >= 0; i-- {
		n := list[i]
		if n.closed {
			continue
		}
		n.remove()
		if n.onGone != nil {
			n.onGone()
		}
	}
}
