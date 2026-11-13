package ui

// Focus management shared by every widget: Tab order per layer, focus
// traps for modal panels, and returning focus when a popup closes.

// focusEntry is a focusable widget of the last frame, in paint order.
type focusEntry struct {
	id    ID
	layer int
	tab   bool // reachable with Tab (not a pointer-only focus target)
}

// trapEntry is an open focus-trapping panel.
type trapEntry struct {
	panel ID
	layer int
	ret   ID // focus before the panel opened (restored when it closes)
}

// TrapFocus makes the overlay panel (Overlay(...).ID()) a modal focus
// scope for this frame: Tab and Shift+Tab cycle inside it and the layers
// above it, keyboard focus moves into it when it opens and returns to the
// previously focused widget when it closes, and the semantics tree hides
// the layers below. Call it every frame the panel is open; Modal, Drawer
// and the confirm boxes do so already.
func (c *Context) TrapFocus(panel ID) {
	if panel != 0 {
		c.traps = append(c.traps, panel)
	}
}

// trapLayer is the lowest layer Tab may visit (the top open trap).
func (c *Context) trapLayer() int {
	top := 0
	for _, t := range c.trapPrev {
		top = max(top, t.layer)
	}
	return top
}

// focusLayer returns the layer of focusable id in the last frame (-1 if
// it is not focusable).
func (c *Context) focusLayer(id ID) int {
	for _, e := range c.focusList {
		if e.id == id {
			return e.layer
		}
	}
	return -1
}

// tabNext returns the widget Tab (or Shift+Tab when back) moves to from
// cur, staying inside the top focus trap.
func (c *Context) tabNext(cur ID, back bool) (ID, bool) {
	lo := c.trapLayer()
	ok := func(e focusEntry) bool { return e.tab && e.layer >= lo }
	n := len(c.focusList)
	// A pointer-only focus target (not a Tab stop itself) still marks the
	// position Tab moves on from.
	start := -1
	for i, e := range c.focusList {
		if e.id == cur && e.layer >= lo {
			start = i
			break
		}
	}
	step := 1
	if back {
		step = -1
	}
	if start < 0 {
		if back {
			start = 0
		} else {
			start = n - 1
		}
	}
	for k := 1; k <= n; k++ {
		i := ((start+step*k)%n + n) % n
		if ok(c.focusList[i]) {
			return c.focusList[i].id, true
		}
	}
	return 0, false
}

// firstFocusAt returns the first Tab stop in layer l or above.
func (c *Context) firstFocusAt(l int) ID {
	for _, e := range c.focusList {
		if e.tab && e.layer >= l {
			return e.id
		}
	}
	return 0
}

// updateFocus runs after a frame was painted (focusList and overlayLayer
// describe it): it opens and closes focus traps and keeps focus on a live
// widget, sending it back to the trigger when its popup closed. It
// reports whether focus moved.
func (c *Context) updateFocus() bool {
	before := c.focus
	// Traps opened or closed this frame.
	prev := c.trapPrev
	cur := make([]trapEntry, 0, len(c.traps))
	for _, p := range c.traps {
		l, ok := c.overlayLayer[p]
		if !ok {
			continue
		}
		e := trapEntry{panel: p, layer: l}
		found := false
		for _, t := range prev {
			if t.panel == p {
				e.ret, found = t.ret, true
				break
			}
		}
		if !found {
			e.ret = c.focus
			if c.focusLayer(c.focus) < l {
				if id := c.firstFocusAt(l); id != 0 {
					c.focus = id
				}
			}
		}
		cur = append(cur, e)
	}
	for _, t := range prev {
		closed := true
		for _, e := range cur {
			if e.panel == t.panel {
				closed = false
				break
			}
		}
		if closed && t.ret != 0 && (c.focus == 0 || !c.focusOrder.Contains(c.focus) || c.focusLayer(c.focus) >= t.layer) {
			if c.focusOrder.Contains(t.ret) {
				c.focus = t.ret
			}
		}
	}
	c.trapPrev = cur
	// The focused widget disappeared with its popup: back to the anchor.
	if c.focus != 0 && !c.focusOrder.Contains(c.focus) && c.focusReturn != 0 && c.focusOrder.Contains(c.focusReturn) {
		c.focus = c.focusReturn
	}
	c.focusReturn = 0
	if l := c.focusLayer(c.focus); l > 0 && l <= len(c.overlays) {
		if a := c.overlays[l-1].opts.Anchor; a != 0 && c.focusLayer(a) >= 0 && c.focusLayer(a) < l {
			c.focusReturn = a
		}
		for _, t := range cur {
			if t.layer == l && t.ret != 0 {
				c.focusReturn = t.ret
			}
		}
	}
	return c.focus != before
}
