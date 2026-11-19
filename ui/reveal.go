package ui

// rowFollow is the retained state of followRow.
type rowFollow struct {
	seen    int  // active row after the last build (hover included)
	pending bool // a reveal waits for the list's first layout
}

// followRow keeps the active row of a fixed-row-height list inside sb in
// view while it is moved with the keyboard. idx is the active index before
// the rows were built (after key handling); cur is the index after the
// build (hovering a row may have changed it). Only changes that did not
// come from the pointer scroll, so hovering a half-visible row never moves
// the list under the mouse. top is the content y of row 0. The first call
// for a list (opening a dropdown) also reveals idx.
func followRow(c *Context, sb *ScrollBox, idx, cur int, top, rowH float32) {
	followRowBy(c, sb, idx, cur, func(i int) (y, h float64) {
		return float64(top) + float64(i)*float64(rowH), float64(rowH)
	})
}

// followRowBy is followRow for rows of any height: span returns the
// content y and height of row i.
func followRowBy(c *Context, sb *ScrollBox, idx, cur int, span func(i int) (y, h float64)) {
	if sb == nil {
		return
	}
	st := State[rowFollow](c, sb.n.id^0x7e5ea1, func() rowFollow { return rowFollow{seen: -1, pending: true} })
	if idx != st.seen {
		st.pending = true
	}
	st.seen = cur
	if !st.pending || idx < 0 {
		st.pending = false
		return
	}
	if sb.s.view.H <= 0 {
		c.WakeAt(c.now) // not laid out yet: reveal next frame
		return
	}
	st.pending = false
	sb.Reveal(span(idx))
}

// scrollAncestor returns the nearest vertically scrolling ancestor of n
// (Table sticky headers, Form scroll-to-first-error).
func scrollAncestor(n *node) *node {
	for p := n.parent; p != nil; p = p.parent {
		if p.scroll != nil && p.ln.ScrollY {
			return p
		}
	}
	return nil
}
