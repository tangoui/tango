package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// scrollTag marks a scroll area's own hit (id ^ scrollTag); while the
// pointer holds a scrollbar it is also the active (dragging) ID.
const scrollTag ID = 0x5c011

// scrollBarHit is the width of the edge strip that grabs the scrollbar.
const scrollBarHit = 10

// vec64 is a scroll position in content space. Offsets are float64 so a
// virtual list of millions of rows still scrolls by single pixels.
type vec64 struct{ X, Y float64 }

// scrollState is the retained state of a scroll container.
//
// Laid-out children start at content position base (normally 0). Virtual
// containers (VirtualList, Table) build only a window of rows, lay them out
// from 0 and set base to the window's content position plus extent to the
// full content size; child positions then stay small float32 values
// relative to the window, which keeps them exact at huge offsets.
type scrollState struct {
	off     vec64
	base    vec64
	extent  vec64 // virtual content size (0 = measured)
	content vec64 // content size of the last paint
	painted vec64 // offset used by the last paint
	view    geom.Size

	always bool // keep the bars visible
	// Scrollbar dragging: which axis and where the thumb was grabbed.
	dragX, dragY bool
	grab         float32

	// RTL: off.X stays logical (0 = inline start, the right end in RTL);
	// frame/shift convert it to the physical child translation. ln is the
	// container's layout node of the current build (nil for containers
	// that manage their own columns, such as Table); rtl and scrollX cache
	// its flags at the last frame for input handled before the next paint.
	ln      *layout.Node
	rtl     bool
	scrollX bool

	// kbPad is how much of the viewport the soft keyboard covered at the
	// last paint: the content may scroll that much further so its end can
	// be lifted above the keyboard (keyboard.go).
	kbPad float32
}

func (s *scrollState) maxX() float64 { return max(0, s.content.X-float64(s.view.W)) }
func (s *scrollState) maxY() float64 {
	return max(0, s.content.Y-float64(s.view.H)+float64(s.kbPad))
}

func (s *scrollState) clampOffset() {
	s.off.X = max(0, min(s.off.X, s.maxX()))
	s.off.Y = max(0, min(s.off.Y, s.maxY()))
}

// contentSize merges the measured content with the virtual extent.
func (s *scrollState) contentSize(laid geom.Size) vec64 {
	return vec64{
		X: max(s.base.X+float64(laid.W), s.extent.X),
		Y: max(s.base.Y+float64(laid.H), s.extent.Y),
	}
}

// syncDir refreshes the cached direction flags from the layout node.
func (s *scrollState) syncDir() {
	if s.ln != nil {
		s.rtl, s.scrollX = s.ln.RTL, s.ln.ScrollX
	}
}

// physX converts the logical horizontal offset off to the translation
// applied to the children. In RTL the layout mirrored the children inside
// span (the box width, or the laid content width when scrolling on X), so
// logical 0 must show the right end: sx = base + (span - view) - off.
func (s *scrollState) physX(off float64, view, laid geom.Size) float64 {
	if !s.rtl {
		return off - s.base.X
	}
	span := view.W
	if s.scrollX {
		span = max(view.W, laid.W)
	}
	return s.base.X + float64(span-view.W) - off
}

// frame records the viewport and content of this paint, clamps the
// offset and returns the translation applied to the children.
func (s *scrollState) frame(view, laid geom.Size) (sx, sy float32) {
	s.syncDir()
	s.view = view
	s.content = s.contentSize(laid)
	s.clampOffset()
	s.painted = s.off
	return float32(s.physX(s.off.X, view, laid)), float32(s.off.Y - s.base.Y)
}

// shift returns the child translation for a node without mutating the
// state (geometry queries between layout and paint).
func (s *scrollState) shift(view, laid geom.Size) (sx, sy float32) {
	c := s.contentSize(laid)
	x := max(0, min(s.off.X, c.X-float64(view.W)))
	y := max(0, min(s.off.Y, c.Y-float64(view.H)+float64(s.kbPad)))
	s.syncDir()
	return float32(s.physX(x, view, laid)), float32(y - s.base.Y)
}

// physOffX is the horizontal offset measured from the physical left end
// of the content (scrollbar thumb position).
func (s *scrollState) physOffX() float64 {
	if s.rtl {
		return s.maxX() - s.off.X
	}
	return s.off.X
}

// onVBar reports whether p lies on the vertical scrollbar strip of r: the
// inline end edge (right in LTR, left in RTL).
func (s *scrollState) onVBar(p geom.Point, r geom.Rect) bool {
	if s.rtl {
		return p.X <= r.X+scrollBarHit
	}
	return p.X >= r.Right()-scrollBarHit
}

// reveal scrolls the minimum amount that makes the content range
// [y, y+h) visible on the vertical axis; it reports a change.
func (s *scrollState) reveal(y, h float64) bool {
	view := float64(s.view.H)
	if view <= 0 {
		return false
	}
	old := s.off.Y
	switch {
	case h >= view || y < s.off.Y:
		s.off.Y = y
	case y+h > s.off.Y+view:
		s.off.Y = y + h - view
	}
	if s.content.Y > 0 {
		s.off.Y = max(0, min(s.off.Y, s.maxY()))
	}
	return s.off.Y != old
}

// revealX is reveal on the horizontal axis; x is a logical content
// position (measured from the inline start).
func (s *scrollState) revealX(x, w float64) bool {
	view := float64(s.view.W)
	if view <= 0 {
		return false
	}
	old := s.off.X
	switch {
	case w >= view || x < s.off.X:
		s.off.X = x
	case x+w > s.off.X+view:
		s.off.X = x + w - view
	}
	if s.content.X > 0 {
		s.off.X = max(0, min(s.off.X, s.maxX()))
	}
	return s.off.X != old
}

type scrollTarget struct {
	id    ID
	rect  geom.Rect
	layer int
	s     *scrollState
	// hits[from:to] are the scroll area's own hit and its descendants'.
	from, to int
}

// wheelState routes wheel input that no scroll container consumed to
// widgets that asked for it with WheelFor.
type wheelState struct {
	listen  map[ID]uint64 // id -> build pass that last called WheelFor
	target  ID
	dx, dy  float32
	queued  uint64 // pass at which the pending delta was queued
	pending bool
}

var wheelKey = hashID("ui.wheel")

func (c *Context) wheel() *wheelState {
	return State[wheelState](c, wheelKey, func() wheelState { return wheelState{listen: map[ID]uint64{}} })
}

// wheelLayer is the lowest layer that may receive wheel input at p: the
// top-most layer that blocks the pointer there (overlay panels, masks).
// Non-blocking layers (pinned Affix content, floating buttons, toasts) let
// the wheel through to what lies below.
func (c *Context) wheelLayer(p geom.Point) int {
	top := 0
	for _, h := range c.hits {
		if h.blocks && h.layer > top && h.rect.Contains(p) {
			top = h.layer
		}
	}
	return top
}

func (r *Runner) applyScroll(ev input.Scroll) {
	c := r.c
	dx, dy := ev.DX, ev.DY
	if ev.Mods.Has(input.ModShift) && dx == 0 {
		dx, dy = dy, 0
	}
	low := c.wheelLayer(ev.Pos)
	w := c.wheel()
	w.prune(c.pass)
	// Walk the hits top-most/innermost first (paint order reversed): the
	// first scroll area that can move in the wheel direction, or the first
	// WheelFor listener, takes the event.
	for i := len(c.hits) - 1; i >= 0; i-- {
		h := c.hits[i]
		if h.layer < low || !h.rect.Contains(ev.Pos) {
			continue
		}
		if seen, ok := w.listen[h.id]; ok && seen == c.pass {
			if w.pending && w.target != h.id {
				w.dx, w.dy = 0, 0
			}
			w.target, w.pending, w.queued = h.id, true, c.pass
			w.dx += dx
			w.dy += dy
			c.invalid = true
			return
		}
		s := c.scrollTargetOf(h.id)
		if s == nil {
			continue
		}
		ldx := dx // physical wheel delta -> logical offset delta
		if s.rtl {
			ldx = -dx
		}
		canY := dy != 0 && ((dy > 0 && s.off.Y < s.maxY()) || (dy < 0 && s.off.Y > 0))
		canX := ldx != 0 && ((ldx > 0 && s.off.X < s.maxX()) || (ldx < 0 && s.off.X > 0))
		if !canX && !canY {
			continue
		}
		s.off.X += float64(ldx)
		s.off.Y += float64(dy)
		s.clampOffset()
		c.invalid = true
		return
	}
}

func (c *Context) scrollTargetOf(hitID ID) *scrollState {
	for i := len(c.prevScroll) - 1; i >= 0; i-- {
		if t := c.prevScroll[i]; t.id^scrollTag == hitID {
			return t.s
		}
	}
	return nil
}

// WheelFor returns the wheel delta that landed on widget id this frame
// and was not consumed by a scroll container inside it (or by a nested
// WheelFor listener). Use it for wheel-driven widgets that do not scroll:
// zoom in an image preview, a carousel, stepping a number. id must be
// registered for hit testing (any interact/Hovered call); delivery starts
// the frame after the first call. Wheel over a widget that never asks
// keeps scrolling the containers around it.
func (c *Context) WheelFor(id ID) (dx, dy float32, ok bool) {
	w := c.wheel()
	w.listen[id] = c.pass
	if !w.pending || w.target != id {
		return 0, 0, false
	}
	stale := w.queued+1 < c.pass // a later frame: the event is gone
	dx, dy = w.dx, w.dy
	w.pending, w.dx, w.dy = false, 0, 0
	if stale {
		return 0, 0, false
	}
	return dx, dy, true
}

// pruneWheel forgets listeners that stopped asking.
func (w *wheelState) prune(pass uint64) {
	if len(w.listen) < 64 {
		return
	}
	for id, p := range w.listen {
		if pass-p > 2 {
			delete(w.listen, id)
		}
	}
}

// ---------------------------------------------------------------------------
// ScrollBox

// ScrollBox is a scrollable container with overlay scrollbars that appear
// on hover; drag a thumb to scroll, click the track to jump there, wheel or
// trackpad anywhere inside.
type ScrollBox struct {
	*Box
	s *scrollState
}

// Scroll creates a vertically scrolling column. Give it a bounded height
// (Grow inside a sized parent, or layout.H/MaxH).
func Scroll(c *Context, fn func(), opts ...layout.Option) *ScrollBox {
	b := c.box("scroll", layout.Column, opts)
	b.n.ln.ScrollY = true
	return c.finishScroll(b, fn)
}

// ScrollH creates a horizontally scrolling row.
func ScrollH(c *Context, fn func(), opts ...layout.Option) *ScrollBox {
	b := c.box("scrollh", layout.Row, opts)
	b.n.ln.ScrollX = true
	return c.finishScroll(b, fn)
}

func (c *Context) finishScroll(b *Box, fn func()) *ScrollBox {
	s := State[scrollState](c, b.n.id, nil)
	b.n.scroll = s
	s.base, s.extent = vec64{}, vec64{}
	s.ln = &b.n.ln
	id := b.n.id
	navScrollStep(c, id)
	c.dragScrollbars(id, s)
	c.touchScroll(b, s) // scroll_touch.go
	b.n.over = func(l *paint.List, r geom.Rect) { c.paintScrollbars(l, id, s, r) }
	b.Body(fn)
	return &ScrollBox{b, s}
}

// Both scrolls on both axes.
func (s *ScrollBox) Both() *ScrollBox { s.n.ln.ScrollX, s.n.ln.ScrollY = true, true; return s }

// Always keeps the scrollbars visible instead of showing them on hover.
func (s *ScrollBox) Always() *ScrollBox { s.s.always = true; return s }

// Offset returns the scroll offset: the content position at the viewport's
// top-left corner (top-right in RTL, where x counts from the right end).
func (s *ScrollBox) Offset() (x, y float64) { return s.s.off.X, s.s.off.Y }

// ScrollTo sets the scroll offset (clamped at paint). x is logical: 0 is
// the inline start, which is the right end in RTL.
func (s *ScrollBox) ScrollTo(x, y float64) *ScrollBox {
	if s.s.off != (vec64{x, y}) {
		s.s.off = vec64{x, y}
		s.c.invalid = true
	}
	return s
}

// ScrollToSmooth animates the vertical offset to y.
func (s *ScrollBox) ScrollToSmooth(y float32, dur time.Duration) *ScrollBox {
	navScrollTo(s.c, s.n.id, y, dur)
	return s
}

// Reveal scrolls the minimum amount that makes the vertical content range
// [y, y+h) visible (no-op before the first paint). Use it to keep a
// keyboard-selected row in view.
func (s *ScrollBox) Reveal(y, h float64) *ScrollBox {
	if s.s.reveal(y, h) {
		s.c.invalid = true
	}
	return s
}

// ScrollIntoView scrolls container scrollID (a ScrollBox or another
// scroll container's ID) the minimum amount that makes widget id fully
// visible, using both widgets' rectangles from the last painted frame. It
// returns false when either was not painted yet (call it again next frame).
func (c *Context) ScrollIntoView(scrollID, id ID) bool {
	s := scrollStateOf(c, scrollID)
	vr, ok1 := c.rects[scrollID]
	ir, ok2 := c.rects[id]
	if s == nil || !ok1 || !ok2 || s.view.H <= 0 {
		return false
	}
	y := float64(ir.Y-vr.Y) + s.painted.Y
	px := float64(ir.X - vr.X) // physical position inside the viewport
	if s.rtl {
		px = float64(s.view.W) - px - float64(ir.W)
	}
	x := px + s.painted.X
	changed := s.reveal(y, float64(ir.H))
	if s.content.X > float64(s.view.W)+0.5 {
		changed = s.revealX(x, float64(ir.W)) || changed
	}
	if changed {
		c.invalid = true
	}
	return true
}

// ScrollIntoViewInset is ScrollIntoView for a window partly covered by
// system UI: inset (window logical pixels, such as KeyboardInset or
// SafeArea) is removed from the window before it clips the container's
// viewport, so the widget ends up in the part of the container that is
// still visible. A widget taller than that part is aligned to its top.
// While the soft keyboard covers the container it may scroll past its
// content's end by the covered height. Like ScrollIntoView it returns
// false when either widget was not painted yet.
func (c *Context) ScrollIntoViewInset(scrollID, id ID, inset geom.Insets) bool {
	ir, ok := c.rects[id]
	if !ok {
		return false
	}
	_, ok = c.revealInset(scrollID, ir, inset)
	return ok
}

// revealInset scrolls container scrollID the minimum amount that brings
// target (window rect of the last frame) into the part of its viewport
// not covered by inset. It reports whether the offset changed, and ok
// false when the container was not painted yet.
func (c *Context) revealInset(scrollID ID, target geom.Rect, inset geom.Insets) (changed, ok bool) {
	s := scrollStateOf(c, scrollID)
	vr, ok := c.rects[scrollID]
	if s == nil || !ok || s.view.H <= 0 {
		return false, false
	}
	top := max(vr.Y, inset.Top)
	bottom := min(vr.Bottom(), c.size.H-inset.Bottom)
	if bottom-top < 1 {
		return false, true
	}
	y := float64(target.Y-vr.Y) + s.painted.Y // content position
	lo := s.painted.Y + float64(top-vr.Y)     // visible content range
	hi := s.painted.Y + float64(bottom-vr.Y)
	old := s.off.Y
	switch {
	case float64(target.H) >= hi-lo || y < lo:
		s.off.Y = s.painted.Y + y - lo
	case y+float64(target.H) > hi:
		s.off.Y = s.painted.Y + y + float64(target.H) - hi
	}
	if s.content.Y > 0 {
		s.off.Y = max(0, min(s.off.Y, s.maxY()))
	}
	changed = s.off.Y != old
	if s.content.X > float64(s.view.W)+0.5 {
		px := float64(target.X - vr.X)
		if s.rtl {
			px = float64(s.view.W) - px - float64(target.W)
		}
		changed = s.revealX(px+s.painted.X, float64(target.W)) || changed
	}
	if changed {
		c.invalid = true
	}
	return changed, true
}

// scrollStateOf returns the retained state of scroll container id.
func scrollStateOf(c *Context, id ID) *scrollState {
	if e, ok := c.states[id]; ok {
		if s, ok := e.v.(*scrollState); ok {
			e.seen = c.pass
			return s
		}
	}
	return nil
}

// scrollThumb returns the thumb's offset along a track and its length.
func scrollThumb(track, view float32, content, off float64) (pos, size float32) {
	size = min(track, max(24, float32(float64(track)*float64(view)/content)))
	if content > float64(view) {
		pos = (track - size) * float32(off/(content-float64(view)))
	}
	return pos, size
}

func (s *scrollState) canY() bool { return s.content.Y > float64(s.view.H)+0.5 }
func (s *scrollState) canX() bool { return s.content.X > float64(s.view.W)+0.5 }

// dragScrollbars lets the pointer drag the scrollbars of the scroll area
// id (vertical along the inline end edge, horizontal along the bottom). A press
// on the track outside the thumb jumps the thumb under the pointer.
func (c *Context) dragScrollbars(id ID, s *scrollState) {
	r, ok := c.rects[id]
	if !ok {
		return
	}
	bar := id ^ scrollTag
	if c.pressed[bar] {
		p := c.pressPos
		switch {
		case s.canY() && s.onVBar(p, r):
			pos, size := scrollThumb(r.H, s.view.H, s.content.Y, s.off.Y)
			s.dragY, s.grab = true, size/2
			if y := p.Y - r.Y; y >= pos && y <= pos+size {
				s.grab = y - pos
			}
		case s.canX() && p.Y >= r.Bottom()-scrollBarHit:
			pos, size := scrollThumb(r.W, s.view.W, s.content.X, s.physOffX())
			s.dragX, s.grab = true, size/2
			if x := p.X - r.X; x >= pos && x <= pos+size {
				s.grab = x - pos
			}
		}
	}
	held := c.active == bar || c.released[bar]
	if s.dragY {
		if !held {
			s.dragY = false
		} else if _, size := scrollThumb(r.H, s.view.H, s.content.Y, s.off.Y); r.H > size {
			s.off.Y = float64((c.pointer.Y-r.Y-s.grab)/(r.H-size)) * s.maxY()
			s.clampOffset()
			c.invalid = true
		}
	}
	if s.dragX {
		if !held {
			s.dragX = false
		} else if _, size := scrollThumb(r.W, s.view.W, s.content.X, s.off.X); r.W > size {
			s.off.X = float64((c.pointer.X-r.X-s.grab)/(r.W-size)) * s.maxX()
			if s.rtl {
				s.off.X = s.maxX() - s.off.X
			}
			s.clampOffset()
			c.invalid = true
		}
	}
}

// paintScrollbars draws the overlay scrollbars: shown while the pointer is
// over the area (or always), wider and darker while the pointer is on the
// bar strip or dragging.
func (c *Context) paintScrollbars(l *paint.List, id ID, s *scrollState, r geom.Rect) {
	th := c.Theme
	bar, onBar := c.scrollbarAt(c.pointer)
	onBar = onBar && bar == id^scrollTag && c.pointerIn
	drag := s.dragX || s.dragY
	a := c.Anim(id, "sb", b2f(s.always || drag || c.hovered(id^scrollTag) || onBar), th.DurationMid)
	if a <= 0.01 {
		return
	}
	g := c.Anim(id, "sbw", b2f(onBar || drag), th.DurationFast)
	thick := 6 + 2*g
	col := theme.Lerp(theme.Fade(th.TextQuaternary, float64(a)), theme.Fade(th.TextTertiary, float64(a)), g)
	if s.canY() {
		pos, size := scrollThumb(r.H, s.view.H, s.content.Y, s.off.Y)
		x := r.Right() - 2 - thick
		if s.rtl {
			x = r.X + 2
		}
		l.Fill(geom.Rect{X: x, Y: r.Y + pos + 2, W: thick, H: size - 4}, geom.Rad(thick/2), col)
	}
	if s.canX() {
		pos, size := scrollThumb(r.W, s.view.W, s.content.X, s.physOffX())
		l.Fill(geom.Rect{X: r.X + pos + 2, Y: r.Bottom() - 2 - thick, W: size - 4, H: thick}, geom.Rad(thick/2), col)
	}
}

// scrollbarAt returns the scrollbar hit ID (scroll ID ^ scrollTag) when p
// is on a scrollbar strip of the top-most scroll area under p.
func (c *Context) scrollbarAt(p geom.Point) (ID, bool) {
	for i := len(c.prevScroll) - 1; i >= 0; i-- {
		t := c.prevScroll[i]
		if t.layer < c.topLayer || !t.rect.Contains(p) {
			continue
		}
		s := t.s
		r, ok := c.rects[t.id]
		if !ok {
			r = t.rect
		}
		// A widget drawn over the area but outside its content (a custom
		// scrollbar thumb, a floating button) keeps the press.
		if h := c.hotHitIndex(); h >= 0 && (h < t.from || h >= t.to) {
			return 0, false
		}
		if s.canY() && s.onVBar(p, r) {
			return t.id ^ scrollTag, true
		}
		if s.canX() && p.Y >= r.Bottom()-scrollBarHit {
			return t.id ^ scrollTag, true
		}
		return 0, false // inner-most scroll area under p, not on its bars
	}
	return 0, false
}

// hotHitIndex returns the index in c.hits of the hot widget's hit, or -1.
func (c *Context) hotHitIndex() int {
	if c.hot == 0 {
		return -1
	}
	for i := len(c.hits) - 1; i >= 0; i-- {
		if c.hits[i].id == c.hot && c.hits[i].rect.Contains(c.pointer) {
			return i
		}
	}
	return -1
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
