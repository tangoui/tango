//go:build darwin && !ios

package darwin

import (
	"errors"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// App-owned native views embedded in a canvas surface
// (platform.NativeViewEmbedder). They share the parent's TangoEmbedHost
// with embedded WebViews (webembed.go): each sits in its own clip view,
// which TangoEmbedClipPass makes transparent to the mouse when the views
// pass input through.

// embeddedNative is an application's NSView placed in a canvas surface.
type embeddedNative struct {
	parent *surface
	clip   id // TangoEmbedClip or TangoEmbedClipPass
	view   id // the application's NSView
	place  platform.WebPlacement
	geo    platform.EmbedGeometry // last applied
	scale  float64                // scale geo was computed for
	closed bool
	onGone func() // the parent closed (platform.NativeViewEmbedder)
}

var (
	nativesOf     = map[*surface][]*embeddedNative{} // parent → its native views, bottom to top
	clipPassClass bool
)

func registerClipPass() {
	if clipPassClass {
		return
	}
	clipPassClass = true
	// A clip view that, with its subtree, never takes the mouse: points
	// over it fall through to the canvas below.
	classDef("TangoEmbedClipPass", "NSView", nil, []objc.MethodDef{
		method("isFlipped", func(_ id, _ objc.SEL) bool { return true }),
		method("hitTest:", func(_ id, _ objc.SEL, _ nsPoint) id { return 0 }),
		method("acceptsFirstResponder", func(_ id, _ objc.SEL) bool { return false }),
	})
}

var errNoNativeView = errors.New("tango/darwin: NewEmbeddedNativeView: no view")

// NewEmbeddedNativeView implements platform.NativeViewEmbedder.
func (b *backend) NewEmbeddedNativeView(parent platform.SurfaceID, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error) {
	if !isMainThread() {
		return nil, errors.New("tango/darwin: NewEmbeddedNativeView must run on the main thread")
	}
	p := b.surfaces[parent]
	if p == nil || p.closed || p.view == 0 {
		return nil, errEmbedParent
	}
	if view == 0 {
		return nil, errNoNativeView
	}
	registerEmbedClasses()
	registerClipPass()
	n := &embeddedNative{parent: p, view: id(view), onGone: closed}
	var err error
	withPool(func() {
		host := embedHost(p)
		if host == 0 {
			err = errEmbedParent
			return
		}
		cls := "TangoEmbedClip"
		if opts.PassInput {
			cls = "TangoEmbedClipPass"
		}
		n.clip = msgInitRect(send(class(cls), "alloc"), sel("initWithFrame:"), nsRect{Size: nsSize{Width: 1, Height: 1}})
		send(n.clip, "setWantsLayer:", 1)
		if respondsTo(n.clip, "setClipsToBounds:") { // macOS 14+: not clipping by default
			send(n.clip, "setClipsToBounds:", 1)
		}
		if l := send(n.clip, "layer"); l != 0 {
			send(l, "setMasksToBounds:", 1)
		}
		send(n.clip, "setHidden:", 1)
		send(n.clip, "addSubview:", uintptr(n.view)) // retained by the clip view until Close
		send(host, "addSubview:", uintptr(n.clip))   // above earlier views
	})
	if err != nil {
		return nil, err
	}
	nativesOf[p] = append(nativesOf[p], n)
	return n, nil
}

// SetPlacement implements platform.EmbeddedNativeView.
func (n *embeddedNative) SetPlacement(pl platform.WebPlacement) {
	if n.closed {
		return
	}
	n.place = pl
	scale := 1.0
	if w := n.parent.win; w != 0 {
		if sc := msgGetFloat(w, sel("backingScaleFactor")); sc > 0 {
			scale = sc
		}
	}
	g := pl.Geometry(scale, false)
	if g == n.geo && scale == n.scale {
		return
	}
	wasVisible := n.geo.Visible
	n.geo, n.scale = g, scale
	withPool(func() {
		if !g.Visible {
			if wasVisible {
				send(n.clip, "setHidden:", 1)
			}
			return
		}
		msgSetRect(n.clip, sel("setFrame:"), devRect(g.Clip, scale))  // the host view is flipped
		msgSetRect(n.view, sel("setFrame:"), devRect(g.Frame, scale)) // the clip view is flipped
		if !wasVisible {
			send(n.clip, "setHidden:", 0)
		}
	})
}

// Close implements platform.EmbeddedNativeView.
func (n *embeddedNative) Close() {
	if n.closed {
		return
	}
	n.remove()
	list := nativesOf[n.parent]
	for i, x := range list {
		if x == n {
			nativesOf[n.parent] = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(nativesOf[n.parent]) == 0 {
		delete(nativesOf, n.parent)
	}
}

// remove takes the application's view out of the window and drops the
// clip view; the application's own reference keeps its view alive.
func (n *embeddedNative) remove() {
	n.closed = true
	withPool(func() {
		send(n.view, "setHidden:", 1)
		send(n.view, "removeFromSuperview")
		send(n.clip, "removeFromSuperview")
	})
	autorelease(n.clip)
}

// destroyNatives removes the native views of a closing parent and tells
// their owners (called from surface.destroy with destroyEmbeds).
func (s *surface) destroyNatives() {
	list := nativesOf[s]
	delete(nativesOf, s)
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
