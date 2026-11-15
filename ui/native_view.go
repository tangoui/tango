package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
)

// Native views: boxes of the layout reserved for a platform view (an
// embedded WebView) that the host places over the canvas each frame
// (docs/specs/webview-embed.md). The ui package only reserves the box,
// takes part in focus and reports, after each frame, where every native
// view is, how much of it ancestors leave visible and whether a layer
// (popup, tooltip, dialog) covers it; package uihost does the rest.
//
// Experimental: see docs/release.md.

// NativeViewW is the builder of NativeView. Box modifiers (Bg, Border,
// Radius, Paint, ...) style what the canvas draws under the native view,
// which shows wherever the native view is hidden (before it exists,
// while it is frozen as a snapshot, when the platform has none).
type NativeViewW struct {
	*Box
	ia Interaction
}

// nativeState lists the native views of the latest build. Main thread.
type nativeState struct {
	pass  uint64
	nodes []*node
}

// NativeView reserves a box for a platform view. key identifies the view
// among its siblings (it is part of the ID: views with the same key in
// the same scope share an ID). The box is a Tab stop like a focusable
// control (Focusable(false) removes it from the Tab order); Enter and
// Space are left to the native view.
func NativeView(c *Context, key string, opts ...layout.Option) *NativeViewW {
	n := c.add("nativeview", key)
	n.ln.Dir = layout.Column
	flexbox.Apply(&n.ln.Style, opts...)
	b := &Box{c: c, n: n}
	n.paint = b.paintBox
	n.ownKeys = true
	n.setRole(a11y.RoleGroup)
	n.aKbd = true
	if c.natives.pass != c.pass {
		c.natives.pass = c.pass
		c.natives.nodes = c.natives.nodes[:0]
	}
	c.natives.nodes = append(c.natives.nodes, n)
	w := &NativeViewW{Box: b}
	w.ia = c.interact(n, true, input.CursorDefault)
	return w
}

// Focusable puts the box in the Tab order (the default) or takes it out.
func (w *NativeViewW) Focusable(on bool) *NativeViewW {
	w.n.focus = on && !w.n.disabled
	return w
}

// A11yLabel names the region for assistive technologies.
func (w *NativeViewW) A11yLabel(s string) *NativeViewW { w.n.setName(s); return w }

// Focused reports whether the box has the UI's keyboard focus.
func (w *NativeViewW) Focused() bool { return w.ia.Focused }

// Clicked reports a click on the box itself (the canvas only sees clicks
// where the native view is hidden).
func (w *NativeViewW) Clicked() bool { return w.ia.Clicked }

// NativeRegion is where a native view sits in the frame just painted.
// Rectangles are window logical pixels, like Box.Rect.
type NativeRegion struct {
	ID ID
	// Rect is the whole box.
	Rect geom.Rect
	// Visible is the part of Rect that ancestors (scroll containers,
	// clipping boxes, the window) leave visible; empty when the box is
	// scrolled or clipped out of view, or culled.
	Visible geom.Rect
	// Covered reports that a layer above the box (an Overlay: dropdown,
	// tooltip, popover, dialog, message), including a margin for its
	// shadow, overlaps Visible.
	Covered bool
	// Focused reports the UI's keyboard focus on the box.
	Focused bool
}

// coverMargin widens layer rectangles for the soft shadows drawn around
// popups, which the native view would otherwise cut.
const coverMargin = 16

// NativeRegions returns the native views of the latest frame in build
// order (later ones stack above earlier ones). Call it after Frame (or
// from Window.Present); it allocates a new slice.
func (r *Runner) NativeRegions() []NativeRegion {
	c := r.c
	if c.natives.pass != c.pass || len(c.natives.nodes) == 0 {
		return nil
	}
	out := make([]NativeRegion, 0, len(c.natives.nodes))
	for _, n := range c.natives.nodes {
		reg := NativeRegion{ID: n.id, Rect: c.rects[n.id], Focused: c.focus == n.id}
		layer := -1
		for i := range c.hits {
			if h := &c.hits[i]; h.id == n.id {
				reg.Visible, layer = h.rect, h.layer
				break
			}
		}
		if layer >= 0 && !reg.Visible.Empty() {
			for i, o := range c.overlays {
				if i+1 <= layer {
					continue
				}
				or, ok := c.rects[o.root.id]
				if !ok {
					continue
				}
				or = geom.Rect{X: or.X - coverMargin, Y: or.Y - coverMargin, W: or.W + 2*coverMargin, H: or.H + 2*coverMargin}
				if !or.Intersect(reg.Visible).Empty() {
					reg.Covered = true
					break
				}
			}
		}
		out = append(out, reg)
	}
	return out
}
