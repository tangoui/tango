package ui

import (
	"time"

	"github.com/tangoui/tango/ui/input"
)

// ---------------------------------------------------------------------------
// Hover intent: open after the pointer rests, close after it leaves.

type hoverDelayState struct {
	open     bool
	hovering bool
	since    time.Time
}

// HoverDelay turns a raw hover signal into a debounced open state: it
// becomes true after hovering has held for enter and false after it has
// been off for leave (Ant mouseEnterDelay/mouseLeaveDelay, Element
// show-after/hide-after). key identifies the trigger across frames.
func HoverDelay(c *Context, key ID, hovering bool, enter, leave time.Duration) bool {
	st := State[hoverDelayState](c, hashID(key, "hover-delay"), nil)
	if hovering != st.hovering {
		st.hovering, st.since = hovering, c.now
	}
	want := st.hovering
	if want == st.open {
		return st.open
	}
	d := leave
	if want {
		d = enter
	}
	if c.noAnim || d <= 0 || c.now.Sub(st.since) >= d {
		st.open = want
		c.invalid = true
		return st.open
	}
	c.WakeAt(st.since.Add(d))
	return st.open
}

// SetHoverOpen forces the debounced state (used when a click closes a
// hover-triggered popup).
func SetHoverOpen(c *Context, key ID, open bool) {
	st := State[hoverDelayState](c, hashID(key, "hover-delay"), nil)
	st.open = open
	st.hovering = open
	st.since = c.now
}

// pointerOver reports whether the pointer is inside any of the rects of
// the given widgets in the last frame.
func (c *Context) pointerOver(ids ...ID) bool {
	if !c.pointerIn {
		return false
	}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if r, ok := c.rects[id]; ok && r.Contains(c.pointer) {
			return true
		}
	}
	return false
}

// clickedWithin reports a click this frame on n or any of its descendants
// (a trigger wrapping arbitrary content: Dropdown, Popover, Popconfirm).
func (c *Context) clickedWithin(n *node) bool {
	if c.clicked[n.id] > 0 {
		return true
	}
	for _, k := range n.kids {
		if c.clickedWithin(k) {
			return true
		}
	}
	return false
}

// focusWithin reports whether keyboard focus is on n or a descendant.
func (c *Context) focusWithin(n *node) bool {
	if c.focus == 0 {
		return false
	}
	if n.id == c.focus {
		return true
	}
	for _, k := range n.kids {
		if c.focusWithin(k) {
			return true
		}
	}
	return false
}

// keyActivatedWithin reports Enter or Space pressed this frame on the
// focused widget when it lies inside n and does not handle those keys
// itself: the keyboard twin of clickedWithin (a Button wrapped by a
// Popconfirm trigger).
func (c *Context) keyActivatedWithin(n *node) bool {
	f := findFocused(n, c.focus)
	if f == nil || !f.focus || f.ownKeys {
		return false
	}
	for _, k := range c.keys {
		if k.Down && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
			return true
		}
	}
	return false
}

// findFocused returns the node id in n's subtree (nil when absent or 0).
func findFocused(n *node, id ID) *node {
	if id == 0 {
		return nil
	}
	if n.id == id {
		return n
	}
	for _, k := range n.kids {
		if f := findFocused(k, id); f != nil {
			return f
		}
	}
	return nil
}
