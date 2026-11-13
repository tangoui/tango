package ui

import "github.com/tangoui/tango/ui/geom"

// Layer stack: overlays form a stack in build order (page = layer 0, the
// n-th overlay of the frame = layer n). Escape and outside presses are
// routed by that stack so stacked popups close one at a time, top first —
// even though a lower popup (a Modal built by the page) is built before a
// higher one (a Confirm opened on top of it). Routing uses the previous
// frame's stack, like all pointer interaction.

// layerOf returns the layer id was painted in during the last frame: the
// layer of an overlay root, or of an interactive widget. It returns -1 for
// unknown IDs. Overlays built earlier in this build count with their
// current position when they did not exist last frame.
func (c *Context) layerOf(id ID) int {
	if id == 0 {
		return -1
	}
	if l, ok := c.overlayLayer[id]; ok {
		return l
	}
	for i := len(c.hits) - 1; i >= 0; i-- {
		if c.hits[i].id == id {
			return c.hits[i].layer
		}
	}
	for i, o := range c.overlays {
		if o.root.id == id {
			return i + 1
		}
	}
	return -1
}

// buildLayer is the layer of the content being built right now.
func (c *Context) buildLayer() int {
	if c.curOverlay == 0 {
		return 0
	}
	return max(0, c.layerOf(c.curOverlay))
}

func (c *Context) escapeAt(layer int) bool {
	if layer > c.escNext {
		c.escNext = layer
	}
	if !c.escape || layer < c.escTop {
		return false
	}
	c.escape = false
	return true
}

// EscapeAt reports (and consumes) an Escape press meant for the overlay
// panel: it is true only when panel is the top-most layer that listened for
// Escape in the last frame, so Escape closes stacked popups one at a time.
// Call it every frame the panel is open (calling it is what registers the
// panel as a listener). panel is the overlay's box ID (Overlay(...).ID());
// before the panel was first painted the caller's own layer is used.
func (c *Context) EscapeAt(panel ID) bool {
	l := c.layerOf(panel)
	if l < 0 {
		// Not painted yet (opened this frame, or built before its panel in
		// this pass): resolve its layer after layout so the next frame
		// routes Escape to it.
		c.escPending = append(c.escPending, panel)
		l = c.buildLayer()
	}
	return c.escapeAt(l)
}

// Escape reports (and consumes) an Escape press not handled by a focused
// widget, routed like EscapeAt using the layer the caller is being built
// in. Overlays that check Escape outside their own content (after building
// the panel) must use EscapeAt with the panel ID instead.
func (c *Context) Escape() bool { return c.escapeAt(c.buildLayer()) }

// PressedOutside reports whether the primary button went down this frame
// outside the last-frame rectangles of all ids and not on a layer above
// them: a press inside a popup stacked over the panel (a Select list opened
// from a Modal) is not "outside" the panel. The highest layer among ids
// (usually the panel; anchors on the page do not raise it) is the panel's.
func (c *Context) PressedOutside(ids ...ID) bool {
	if !c.pressedAny {
		return false
	}
	top := -1
	for _, id := range ids {
		top = max(top, c.layerOf(id))
		if r, ok := c.rects[id]; ok && r.Contains(c.pressPos) {
			return false
		}
	}
	if top < 0 {
		top = c.buildLayer()
	}
	return c.pressLayer <= top
}

// Dismissed reports whether an open overlay panel should close this frame:
// Escape routed to it (EscapeAt) or a press outside it and the extra
// widgets, typically its anchor (PressedOutside). Call it every frame the
// panel is open.
func (c *Context) Dismissed(panel ID, also ...ID) bool {
	esc := c.EscapeAt(panel)
	out := c.PressedOutside(append([]ID{panel}, also...)...)
	return esc || out
}

// PointerDownOutside reports whether a press happened this frame outside
// all of the given rectangles. It ignores layering; popups should use
// PressedOutside or Dismissed with widget IDs instead.
func (c *Context) PointerDownOutside(rs ...geom.Rect) bool {
	if !c.pressedAny {
		return false
	}
	for _, r := range rs {
		if r.Contains(c.pressPos) {
			return false
		}
	}
	return true
}

// ContextMenu reports a secondary-button (right) press this frame and its
// position.
func (c *Context) ContextMenu() (geom.Point, bool) { return c.ctxPos, c.ctxPressed }

// ContextMenuFor reports a secondary-button press this frame inside id's
// last-frame rectangle that did not land on a layer above id.
func (c *Context) ContextMenuFor(id ID) (geom.Point, bool) {
	if !c.ctxPressed {
		return geom.Point{}, false
	}
	r, ok := c.rects[id]
	if !ok || !r.Contains(c.ctxPos) {
		return geom.Point{}, false
	}
	l := c.layerOf(id)
	if l < 0 {
		l = c.buildLayer()
	}
	if c.ctxLayer > l {
		return geom.Point{}, false
	}
	return c.ctxPos, true
}

// ContextMenuSource is an extra context-menu gesture for platforms
// without a secondary button (e.g. a two-finger tap): when set, context-menu
// triggers (Dropdown with TriggerContextMenu) also open when it reports a
// point inside them. Right-button presses always work (they are read with
// Context.ContextMenuFor, which respects layers). Nil by default.
var ContextMenuSource func(c *Context) (geom.Point, bool)
