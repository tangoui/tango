package ui

import "github.com/tangoui/tango/ui/input"

// Keyboard navigation shared by composite widgets (WAI-ARIA "roving
// tabindex"): only the current item of a radio group, tab list or toolbar
// is a Tab stop; arrow keys move between the items.

// rovingAxis selects the arrow keys a roving group answers to.
type rovingAxis uint8

const (
	rovingBoth rovingAxis = iota // Left/Up = previous, Right/Down = next (Left/Right swap in RTL)
	rovingHorizontal
	rovingVertical
)

// rovingStep consumes the keys of whichever item of ids has focus and
// returns the index arrow keys (and Home / End) move to, skipping items for
// which enabled returns false (nil: all enabled), wrapping around. It
// returns -1 when no item has focus or no navigation key was pressed.
func rovingStep(c *Context, ids []ID, enabled func(i int) bool, axis rovingAxis) int {
	cur := -1
	for i, id := range ids {
		if id != 0 && c.focus == id {
			cur = i
			break
		}
	}
	if cur < 0 {
		return -1
	}
	ok := func(i int) bool { return enabled == nil || enabled(i) }
	step := func(from, d int) int {
		n := len(ids)
		for k := 1; k <= n; k++ {
			j := ((from+d*k)%n + n) % n
			if ok(j) {
				return j
			}
		}
		return -1
	}
	to := -1
	for _, k := range c.KeysFor(ids[cur]) {
		if !k.Down {
			continue
		}
		prev, next := false, false
		switch k.Key {
		case input.KeyLeft, input.KeyRight:
			// Visual arrows: swapped in RTL scopes (inline.go).
			if axis != rovingVertical {
				prev, next = c.inlineKey(k.Key)
			}
		case input.KeyUp:
			prev = axis != rovingHorizontal
		case input.KeyDown:
			next = axis != rovingHorizontal
		case input.KeyHome:
			to = step(-1, 1)
			cur = to
			continue
		case input.KeyEnd:
			to = step(len(ids), -1)
			cur = to
			continue
		}
		if prev {
			to = step(cur, -1)
		} else if next {
			to = step(cur, 1)
		}
		if to >= 0 {
			cur = to
		}
	}
	return to
}

// rovingTabStop makes only item cur (or the first enabled item when cur <
// 0) of a roving group reachable with Tab; the other items stay
// focusable with the pointer.
func rovingTabStop(nodes []*node, cur int) {
	if cur < 0 || cur >= len(nodes) || nodes[cur].disabled || !nodes[cur].focus {
		cur = -1
		for i, n := range nodes {
			if n.focus && !n.disabled {
				cur = i
				break
			}
		}
	}
	for i, n := range nodes {
		n.aKbd = true // arrow keys reach every item
		if i != cur {
			n.noTab = true
		}
	}
}
