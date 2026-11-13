package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Pointer takeover: a light form of event bubbling for drags.
//
// The ui layer gives a press to the top-most hit widget, which then holds
// the pointer until release (Dragging). A scroll container whose content
// is made of ordinary widgets (inputs, buttons) would never see a drag
// that starts on them. Instead of a full capture/bubble dispatch, a
// container declares which drag directions it handles (Box.TakePointer,
// the equivalent of CSS touch-action on a scroller): when a press that
// landed inside its subtree moves more than DragSlop, mostly along one of
// those directions, the innermost such container takes the pointer over.
// It becomes the holder (Dragging reports it, PointerTaken reports the
// switch and where the press started), the widget that held the pointer
// sees PointerCancelled for one frame and gets no click on release, and
// keyboard focus the press gave to that widget goes back where it was.
//
// A widget that drags by itself in the same direction (a slider inside a
// horizontally swiping cell) keeps the pointer by calling KeepPointer
// while it holds it; containers holding the pointer are never taken over
// by an outer one. Containers that do not declare anything behave as
// before, so existing widgets are unaffected.

// DragSlop is how far (logical px) a press may travel before an enclosing
// container that accepts the movement direction takes the pointer over.
const DragSlop = 8

// PanAxis is a set of drag directions.
type PanAxis uint8

const (
	PanX  PanAxis = 1 << iota // horizontal drags
	PanY                      // vertical drags
	PanXY = PanX | PanY
)

// ptrFlags are a node's declarations for touch routing and pointer
// takeover.
type ptrFlags uint8

const (
	ptrPanX     ptrFlags = ptrFlags(PanX) // take over horizontal drags
	ptrPanY     ptrFlags = ptrFlags(PanY) // take over vertical drags
	ptrTouch    ptrFlags = 1 << 2         // receives TouchEvents (Box.Touches)
	ptrRawTouch ptrFlags = 1 << 3         // fingers starting here are not turned into pointer events
	ptrPanTouch ptrFlags = 1 << 4         // take over only presses made by a finger (scroll areas)
)

// ptrRegion is a node with declarations as painted in one frame: its
// visible screen rectangle and the range of hits its subtree produced.
type ptrRegion struct {
	id       ID
	flags    ptrFlags
	rect     geom.Rect
	layer    int
	from, to int // c.hits indices of the subtree, [from, to)
}

// capture is the takeover bookkeeping of the current press.
type capture struct {
	down     bool       // primary button held (since a Press seen here)
	from     geom.Point // press position
	hitIdx   int        // index in c.hits of the pressed widget, -1 none
	focus    ID         // keyboard focus before the press
	taken    bool       // the pointer was taken over during this press
	keep     ID         // KeepPointer during the last build
	keepNext ID         // KeepPointer during this build
	takenBy  ID         // took over this frame (PointerTaken)
	lost     ID         // lost the pointer this frame (PointerCancelled)
	touch    bool       // the press came from a finger (input.SourceTouch)
}

// ptrState is the Context's touch / takeover state.
type ptrState struct {
	regions, next []ptrRegion
	cap           capture
	touch         touchState
}

// openRegion records n (painted with visible rect vis) when it carries
// declarations; it returns the region index, closed by closeRegion after
// the children are painted.
func (c *Context) openRegion(n *node, vis geom.Rect, layer int) int {
	if n.ptr == 0 {
		return -1 // the common case stays inlined in paintNode
	}
	return c.addRegion(n, vis, layer)
}

func (c *Context) addRegion(n *node, vis geom.Rect, layer int) int {
	if n.disabled {
		return -1
	}
	c.ptr.next = append(c.ptr.next, ptrRegion{id: n.id, flags: n.ptr, rect: c.screenRect(vis), layer: layer, from: len(c.nextHits), to: -1})
	return len(c.ptr.next) - 1
}

func (c *Context) closeRegion(i int) {
	if i >= 0 {
		c.ptr.next[i].to = len(c.nextHits)
	}
}

// swapRegions makes this frame's regions current (after paint).
func (c *Context) swapRegions() {
	c.ptr.regions, c.ptr.next = c.ptr.next, c.ptr.regions[:0]
	c.ptr.cap.keep, c.ptr.cap.keepNext = c.ptr.cap.keepNext, 0
}

// hitAt returns the index in c.hits of the widget a press at p reaches
// (the same rule as updateHot) and the top layer at p; -1 when none.
func (c *Context) hitAt(p geom.Point) (idx, top int) {
	top = 0
	for _, h := range c.hits {
		if h.rect.Contains(p) && h.layer > top {
			top = h.layer
		}
	}
	for i := len(c.hits) - 1; i >= 0; i-- {
		h := c.hits[i]
		if h.layer >= top && h.rect.Contains(p) {
			return i, top
		}
	}
	return -1, top
}

// covers reports whether region g receives a contact at p whose hit is
// idx (top layer top): p is inside g and nothing painted after g's
// subtree, or on a higher layer, is hit there.
func (g *ptrRegion) covers(p geom.Point, idx, top int) bool {
	if g.to < 0 || !g.rect.Contains(p) || g.layer < top {
		return false
	}
	return idx < g.to
}

// ---------------------------------------------------------------------------
// Takeover

// TakePointer makes b take over drags that start on widgets inside it and
// move along axes (see the package notes above on pointer takeover). b
// should register a hit (Interact, Focusable) so that it can also be
// dragged directly. Declare it every frame, like other modifiers.
func (b *Box) TakePointer(axes PanAxis) *Box {
	b.n.ptr = b.n.ptr&^(ptrFlags(PanXY)|ptrPanTouch) | ptrFlags(axes&PanXY)
	return b
}

// takePointerTouch is TakePointer for finger presses only: mouse drags
// that start inside keep going to the pressed widget (text selection,
// sliders, drag and drop). Desktop scroll areas use it.
func (b *Box) takePointerTouch(axes PanAxis) *Box {
	b.TakePointer(axes)
	if axes&PanXY != 0 {
		b.n.ptr |= ptrPanTouch
	}
	return b
}

// PointerTaken reports whether id took the pointer over from a widget
// inside it this frame, and where that press started (id is now the
// holder: Dragging(id) is true).
func (c *Context) PointerTaken(id ID) (from geom.Point, ok bool) {
	if id != 0 && c.ptr.cap.takenBy == id {
		return c.ptr.cap.from, true
	}
	return geom.Point{}, false
}

// PointerCancelled reports whether id lost the pointer this frame without
// a release: an enclosing container took the drag over, or the system
// cancelled the gesture (input.Cancel, a cancelled touch, the window lost
// focus). The widget should drop any drag in progress without
// committing it.
func (c *Context) PointerCancelled(id ID) bool { return id != 0 && c.ptr.cap.lost == id }

// KeepPointer stops enclosing containers from taking the pointer over
// from id while it holds it (CSS touch-action: none). Call it during the
// build of each frame in which id drags by itself.
func (c *Context) KeepPointer(id ID) {
	if id != 0 && c.active == id {
		c.ptr.cap.keepNext = id
	}
}

// capPress starts takeover tracking for a primary press at p.
func (c *Context) capPress(p geom.Point, src input.PointerSource) {
	cp := &c.ptr.cap
	cp.down, cp.from, cp.taken, cp.focus = true, p, false, c.focus
	cp.touch = src == input.SourceTouch
	cp.hitIdx = -1
	if c.active == c.hot && c.hot != 0 {
		cp.hitIdx, _ = c.hitAt(p)
	}
}

// capMove hands the pointer to the innermost enclosing container that
// accepts the drag direction once the press has moved past DragSlop.
func (c *Context) capMove(p geom.Point) {
	cp := &c.ptr.cap
	if cp.down && !cp.taken && cp.touch && c.active == 0 {
		c.capMoveUnheld(p)
		return
	}
	if !cp.down || cp.taken || cp.hitIdx < 0 || c.active == 0 || cp.keep == c.active {
		return
	}
	d := p.Sub(cp.from)
	if d.X*d.X+d.Y*d.Y <= DragSlop*DragSlop {
		return
	}
	axis := PanY
	if abs32(d.X) >= abs32(d.Y) {
		axis = PanX
	}
	_, top := c.hitAt(cp.from)
	cp.taken = true // decided once per press, taken over or not
	for i := len(c.ptr.regions) - 1; i >= 0; i-- {
		g := &c.ptr.regions[i]
		if g.flags&ptrFlags(PanXY) == 0 || !g.covers(cp.from, cp.hitIdx, top) || cp.hitIdx < g.from {
			continue
		}
		if g.id == c.active {
			break // the holder is itself a container: it arbitrates its own content
		}
		if g.flags&ptrFlags(axis) == 0 || (g.flags&ptrPanTouch != 0 && !cp.touch) {
			continue
		}
		lost := c.active
		c.active = g.id
		cp.takenBy, cp.lost = g.id, lost
		// Focus the press gave (to the widget or a part of it, such as an
		// input's editor) goes back: a scroll is not a tap on the field.
		if c.focus != cp.focus {
			c.focus = cp.focus
		}
		return
	}
	cp.taken = false
	cp.hitIdx = -1 // nothing to take it: stop looking for this press
}

// capRelease ends tracking; it reports whether the press was taken over
// (the release must not click).
func (c *Context) capRelease() bool {
	cp := &c.ptr.cap
	t := cp.taken
	cp.down, cp.taken, cp.hitIdx = false, false, -1
	return t
}

// capCancel records that the pointer holder lost it to the system.
func (c *Context) capCancel() {
	cp := &c.ptr.cap
	if c.active != 0 {
		cp.lost = c.active
	}
	cp.down, cp.taken, cp.hitIdx = false, false, -1
}

// clearCapFrame forgets the per-frame takeover reports.
func (c *Context) clearCapFrame() {
	c.ptr.cap.takenBy, c.ptr.cap.lost = 0, 0
}

// capMoveUnheld handles a finger press that landed on no widget (plain text,
// background): no one holds the pointer, so the innermost finger-only
// scroll area (takePointerTouch) under the press takes the drag once it
// moves past DragSlop along one of its axes.
func (c *Context) capMoveUnheld(p geom.Point) {
	cp := &c.ptr.cap
	d := p.Sub(cp.from)
	if d.X*d.X+d.Y*d.Y <= DragSlop*DragSlop {
		return
	}
	axis := PanY
	if abs32(d.X) >= abs32(d.Y) {
		axis = PanX
	}
	cp.taken = true // decided once per press
	_, top := c.hitAt(cp.from)
	for i := len(c.ptr.regions) - 1; i >= 0; i-- {
		g := &c.ptr.regions[i]
		if g.flags&ptrPanTouch == 0 || g.flags&ptrFlags(axis) == 0 {
			continue
		}
		if g.to < 0 || !g.rect.Contains(cp.from) || g.layer < top {
			continue
		}
		c.active = g.id
		cp.takenBy = g.id
		return
	}
	cp.taken = false
}
