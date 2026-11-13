package ui

// Touch scrolling for Scroll / ScrollH: a finger dragging the content
// scrolls it, the way touch screens expect. The container's takeover only
// applies to presses made by a finger (input.SourceTouch), so mouse drags
// inside scroll areas keep selecting text and dragging widgets as before.
// A drag that starts on a widget inside the area (an Input, a button)
// moves past DragSlop along the scroll axis, the area takes the pointer
// over and the widget sees PointerCancelled instead of a click.

// touchScrollTag derives the key of an area's touch-scroll state from its ID.
const touchScrollTag ID = 0x70c5c

type touchScrollState struct {
	on   bool
	last [2]float32 // last pointer position while scrolling
}

func (c *Context) touchScroll(b *Box, s *scrollState) {
	id := b.n.id
	var axes PanAxis
	if b.n.ln.ScrollX {
		axes |= PanX
	}
	if b.n.ln.ScrollY {
		axes |= PanY
	}
	b.takePointerTouch(axes)
	// Own key: State is keyed by ID, and id already holds the scrollState.
	st := State[touchScrollState](c, id^touchScrollTag, nil)
	// Only finger drags of the content: mouse presses and scrollbar
	// thumb drags (which also make the area the holder) are left alone.
	if !c.Dragging(id) || !c.ptr.cap.touch || s.dragX || s.dragY {
		st.on = false
		return
	}
	p, ok := c.Pointer()
	if !ok {
		return
	}
	if !st.on {
		// The area just took the drag over (or was pressed directly):
		// follow the finger from the takeover point, or from here.
		st.on = true
		if from, taken := c.PointerTaken(id); taken {
			st.last = [2]float32{from.X, from.Y}
		} else {
			st.last = [2]float32{p.X, p.Y}
		}
	}
	dx, dy := p.X-st.last[0], p.Y-st.last[1]
	st.last = [2]float32{p.X, p.Y}
	if dx == 0 && dy == 0 {
		return
	}
	if b.n.ln.ScrollX {
		if s.rtl {
			s.off.X += float64(dx) // logical offset grows leftwards
		} else {
			s.off.X -= float64(dx)
		}
	}
	if b.n.ln.ScrollY {
		s.off.Y -= float64(dy)
	}
	c.invalid = true
}
