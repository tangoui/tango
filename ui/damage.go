package ui

import (
	"image"
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Redraw hints. Widgets declare what changed with Context.Damage and
// Context.ScrollShift; the Runner adds the changes it can see itself
// (animations, overlays, focus ring, hover/press) and writes the result
// at the end of the frame's paint.List as paint.ScrollRegion and
// paint.Damage ops. A frame in which no widget declared anything carries
// no hints, so renderers redraw everything (the safe default), and so do
// frames the Runner knows changed as a whole (first frame, theme, size,
// scale and reduced-motion changes).

// Padding added around node rects that the Runner damages on its own:
// box shadows, focus rings and hover halos are drawn outside the layout
// rect of their node.
const (
	damagePadNode    = 24 // animations, hover, press, focus ring
	damagePadOverlay = 48 // popup shadows (Ant's largest reaches ~45px)
	damagePadShift   = 12 // non-scrolling nodes inside a ScrollShift region
	damageGutter     = 12 // scrollbar strip of a scroll container (6..8px thumb + 2px inset)
)

type scrollShift struct {
	r       geom.Rect
	d       geom.Point
	content *node
}

// damageState is what the Runner remembers between frames to detect
// changes.
type damageState struct {
	started  bool // a frame was presented (false: next frame is full)
	now      time.Time
	timed    bool      // last frame animated (c.animating at paint time)
	wakeAt   time.Time // last frame's WakeAt request
	theme    *theme.Theme
	reduce   bool
	size     geom.Size
	scale    float32
	anims    map[animKey]float32
	overlays map[ID]geom.Rect
	next     map[ID]geom.Rect
	focus    ID
	focusVis bool
	hot      ID
	active   ID
}

// Damage tells the renderer that the pixels inside r (window logical
// pixels, the space of RectOf and Box.Rect) change this frame. Once any
// widget declares damage (or a ScrollShift) in a frame, the renderer may
// redraw only the declared regions plus what the Runner detects itself:
// running Anim/AnimFrom transitions (old and new rect of the animation's
// widget ID), overlays opening, closing or moving, the focus ring and the
// hovered/pressed widget. Everything else that looks different must be
// covered by a declaration. Frames without declarations are redrawn
// completely, and so are frames in which time-driven drawing may have
// changed (the clock passed a WakeAt timer of the last frame, such as a
// text caret's blink, or the last frame was animating; WakeAtDamaged
// timers excepted).
//
// UI thread only, during the page build or a paint function (before the
// Runner collects the hints at the end of the frame).
func (c *Context) Damage(r geom.Rect) {
	c.declared = true
	if !r.Empty() {
		c.damage = append(c.damage, r)
	}
}

// ScrollShift declares that the content being built moved by d inside r
// (window logical pixels; a pixel at p is now at p+d) since the last
// frame, so the renderer may copy those pixels instead of redrawing them.
// Call it inside the Body of the scrolled content: the current parent
// node is recorded as the scrolled content (for a Scroll container, its
// children). The Runner damages, inside r, the strip the copy uncovers and
// every node that does not move with the content (sticky headers, summary
// rows, Affix, overlays, fixed-column shadows), at its new place and where
// the copy carried its old pixels. The backgrounds of the content's
// ancestors must look the same everywhere in r. When the nodes in r cannot
// be classified (the content was not laid out, a node straddles r's edge,
// an ancestor paints over r or is transformed) the shift is withdrawn and r
// is redrawn as a whole.
//
// UI thread only, during the page build.
func (c *Context) ScrollShift(r geom.Rect, d geom.Point) {
	c.declared = true
	if r.Empty() {
		return
	}
	if !c.running || c.cur == nil {
		c.damage = append(c.damage, r)
		return
	}
	c.shifts = append(c.shifts, scrollShift{r: r, d: d, content: c.cur})
}

// ScrollShift is Context.ScrollShift for a widget that paints scrolled
// content itself (the lines of a code editor): it declares that what b
// paints, with b's subtree, moved by d inside r. Unlike Context.ScrollShift
// it may also be called from b's paint function, once the widget knows
// where it scrolled to; the rules are the same (the backgrounds behind r
// must look the same everywhere in r, nodes that do not move with b are
// damaged by the Runner, and an unclassifiable region is redrawn whole).
//
// UI thread only, during the page build or b's paint.
func (b *Box) ScrollShift(r geom.Rect, d geom.Point) {
	c := b.c
	c.declared = true
	if r.Empty() {
		return
	}
	c.shifts = append(c.shifts, scrollShift{r: r, d: d, content: b.n})
}

// WakeAtDamaged asks for a frame at t like WakeAt, for a time-driven
// change the widget declares itself, with Damage, in the frame it wakes
// (a text caret's blink): reaching t does not make that frame a full
// redraw, as reaching a WakeAt timer does (see Damage). The widget must
// declare everything the clock changed in its drawing.
//
// UI thread only, during the page build or paint.
func (c *Context) WakeAtDamaged(t time.Time) {
	w := c.declaredWake()
	if w.at.IsZero() || t.Before(w.at) {
		w.at = t
	}
}

// declaredWake holds the frame's WakeAtDamaged requests. It lives in
// c.states under declaredWakeID: Context's fields are declared in
// context.go, which this file does not own.
type declaredWake struct{ at time.Time }

var declaredWakeID = hashID("ui.damage", "WakeAtDamaged")

func (c *Context) declaredWake() *declaredWake {
	e := c.states[declaredWakeID]
	if e == nil {
		e = &stateEntry{v: &declaredWake{}}
		c.states[declaredWakeID] = e
	}
	e.seen = c.pass
	return e.v.(*declaredWake)
}

// beginDamage resets the hints at the start of a frame.
func (c *Context) beginDamage() {
	c.damage = c.damage[:0]
	c.shifts = c.shifts[:0]
	c.staleShifts = c.staleShifts[:0]
	c.declared = false
	if e := c.states[declaredWakeID]; e != nil {
		e.v.(*declaredWake).at = time.Time{}
	}
}

// stashShifts sets aside the ScrollShift calls of a build pass that is
// about to be rebuilt: their nodes are discarded, so the next pass must
// declare them again (or their regions are redrawn).
func (c *Context) stashShifts() {
	c.staleShifts = append(c.staleShifts, c.shifts...)
	c.shifts = c.shifts[:0]
}

// collectDamage runs after paint, before afterFrame swaps the per-frame
// maps (c.rects is the last frame, c.nextRects this one). It appends the
// frame's hints to the list and returns the dirty rects for a
// DamagePresenter (device pixels), or nil when the frame must be presented
// whole.
func (r *Runner) collectDamage(scale float32) []image.Rectangle {
	c := r.c
	s := &r.dmg
	view := geom.Rect{W: c.size.W, H: c.size.H}
	full := !s.started || s.theme != c.Theme || s.reduce != c.reduceMotion || s.size != c.size || s.scale != scale
	// Time-driven visuals (caret blink, shimmer, auto-dismiss, anything
	// drawn from c.Now) are not tracked: once the clock has moved past a
	// WakeAt timer the last frame asked for, or the last frame was
	// animating, redraw everything. WakeAtDamaged timers do not count:
	// their widgets declare what the clock changed.
	if c.now != s.now && (s.timed || (!s.wakeAt.IsZero() && !c.now.Before(s.wakeAt))) {
		full = true
	}
	s.started, s.theme, s.reduce, s.size, s.scale = true, c.Theme, c.reduceMotion, c.size, scale
	// c.wakeAt holds the WakeAt timers only: the WakeAtDamaged ones join
	// it now, for the host, after the check above left them out.
	s.now, s.timed, s.wakeAt = c.now, c.animating, c.wakeAt
	if e := c.states[declaredWakeID]; e != nil {
		e.seen = c.pass // kept while in use (afterFrame drops idle states)
		if w := e.v.(*declaredWake); !w.at.IsZero() {
			c.WakeAt(w.at)
		}
	}

	var auto []geom.Rect
	node := func(id ID, pad float32) bool {
		a, okA := c.rects[id]
		b, okB := c.nextRects[id]
		if okA {
			auto = append(auto, a.Expand(pad))
		}
		if okB {
			auto = append(auto, b.Expand(pad))
		}
		return okA || okB
	}

	// Animations whose value moved (or is moving) this frame.
	moved := false
	if s.anims == nil {
		s.anims = map[animKey]float32{}
	}
	for k, a := range c.anims {
		prev, seen := s.anims[k]
		if a.seen != c.pass {
			continue
		}
		s.anims[k] = a.val
		if seen && prev == a.val && a.val == a.to {
			continue
		}
		moved = true
		if !node(k.id, damagePadNode) {
			full = true // the value drives something we cannot locate
		}
	}
	for k := range s.anims {
		if a, ok := c.anims[k]; !ok || a.seen != c.pass {
			delete(s.anims, k)
		}
	}
	// An animated value may resize its node and so move others (siblings
	// below a growing box, an auto-sized parent). While anything animates
	// (moved also covers a value that jumped to its target in one frame:
	// reduced motion, NoAnimations), every node whose box changed is
	// damaged at its old and new place (layoutShiftDamage, run below once
	// the frame is known to be partial).
	shifted := moved || c.animating

	// Overlays opened, closed or moved.
	if s.next == nil {
		s.overlays, s.next = map[ID]geom.Rect{}, map[ID]geom.Rect{}
	}
	clear(s.next)
	for _, o := range c.overlays {
		if o.root.drawn == c.pass {
			s.next[o.root.id] = o.root.scr
		}
	}
	for id, nr := range s.next {
		if or, ok := s.overlays[id]; !ok || or != nr {
			auto = append(auto, nr.Expand(damagePadOverlay))
			if ok {
				auto = append(auto, or.Expand(damagePadOverlay))
			}
		}
	}
	for id, or := range s.overlays {
		if _, ok := s.next[id]; !ok {
			auto = append(auto, or.Expand(damagePadOverlay))
		}
	}
	s.overlays, s.next = s.next, s.overlays

	// Focus ring shown, hidden or moved; hover and press changes.
	ring, prevRing := ID(0), ID(0)
	if c.focusVisible {
		ring = c.focus
	}
	if s.focusVis {
		prevRing = s.focus
	}
	if ring != prevRing {
		node(ring, damagePadNode)
		node(prevRing, damagePadNode)
	}
	s.focus, s.focusVis = c.focus, c.focusVisible
	if c.hot != s.hot {
		node(c.hot, damagePadNode)
		node(s.hot, damagePadNode)
		s.hot = c.hot
	}
	if c.active != s.active {
		node(c.active, damagePadNode)
		node(s.active, damagePadNode)
		s.active = c.active
	}

	paintHints := listHasHints(&r.list)
	if !c.declared && !paintHints {
		c.fp.fpEnd(false)
		return nil
	}
	if full {
		if paintHints {
			r.list.Damage(view) // hints from paint functions: widen to the frame
		}
		c.fp.fpEnd(false)
		return nil
	}

	dam := append([]geom.Rect(nil), c.damage...)
	regions := c.resolveShifts(&dam)
	dam = append(dam, auto...)
	// Whatever changed without a declaration (damage_fp.go).
	dam = c.fingerprintDamage(dam, r.list.Ops)
	c.fp.fpEnd(true)
	if shifted {
		dam = c.layoutShiftDamage(dam)
	}
	for _, sr := range regions {
		r.list.Scroll(sr.Rect, sr.Delta)
	}
	for _, d := range mergeDamage(dam, view) {
		r.list.Damage(d)
	}
	return dirtyRects(&r.list, scale, c.size)
}

// layoutShiftDamage appends to dam, padded by damagePadNode, the old and
// new boxes (c.rects, c.nextRects) of the nodes whose box changed since the
// last frame, the boxes of nodes that appeared and those of nodes that
// disappeared. Boxes are compared field by field. A node whose padded boxes
// lie inside the ones already added for an ancestor adds nothing, so a
// moved subtree usually costs one pair of rects. Nodes inside a
// ScrollShift region that move with its content by exactly its delta are
// left to the shift (which redraws its whole region when withdrawn), except
// for the parts of their boxes outside the region, which the copy does not
// reach. Any other node whose box changed or that vanished, inside a shift's
// content or not, also gets the place the copy carries its old pixels to
// (appendShiftGhosts).
func (c *Context) layoutShiftDamage(dam []geom.Rect) []geom.Rect {
	// sh is the ScrollShift whose content n moves with (nil: none).
	// pa, pb and pg are the padded old, new and copied-old boxes last added
	// for an ancestor.
	var walk func(n *node, sh *scrollShift, pa, pb, pg geom.Rect)
	walk = func(n *node, sh *scrollShift, pa, pb, pg geom.Rect) {
		b, ok := c.nextRects[n.id]
		if !ok {
			return // not laid out this frame (neither are its kids)
		}
		var own *scrollShift
		for i := range c.shifts {
			if c.shifts[i].content == n {
				own = &c.shifts[i]
				break
			}
		}
		if own != nil && n.scroll == nil {
			sh = own // the content moves itself
		}
		a, okA := c.rects[n.id]
		if okA && sh != nil && a.Offset(sh.d.X, sh.d.Y) == b {
			if a != b {
				dam = appendOutside(dam, a.Expand(damagePadNode), sh.r)
				dam = appendOutside(dam, b.Expand(damagePadNode), sh.r)
			}
		} else if !okA || a != b || sh != nil {
			eb := b.Expand(damagePadNode)
			if pb.Intersect(eb) != eb {
				dam = append(dam, eb)
				pb = eb
			}
			if okA {
				ea := a.Expand(damagePadNode)
				if pa.Intersect(ea) != ea {
					dam = append(dam, ea)
					pa = ea
				}
				dam, pg = c.appendShiftGhosts(dam, ea, pg)
			}
		}
		if own != nil && n.scroll != nil {
			sh = own // a scroll container's kids move
		}
		for _, k := range n.kids {
			walk(k, sh, pa, pb, pg)
		}
	}
	walk(c.root, nil, geom.Rect{}, geom.Rect{}, geom.Rect{})
	for _, o := range c.overlays {
		walk(o.root, nil, geom.Rect{}, geom.Rect{}, geom.Rect{})
	}
	for id, a := range c.rects {
		if _, ok := c.nextRects[id]; !ok {
			ea := a.Expand(damagePadNode) // disappeared
			dam = append(dam, ea)
			dam, _ = c.appendShiftGhosts(dam, ea, geom.Rect{})
		}
	}
	return dam
}

// appendShiftGhosts appends, for every ScrollShift of the frame whose region
// r ea (a padded old box) reaches, (ea + delta) ∩ r: the copy carried the
// old pixels there, whether or not the node belongs to that shift's content
// (a moved or vanished sticky header, overlay or content row). A ghost
// inside skip (the one last added for an ancestor) is left out; it returns
// the last ghost added (or skip).
func (c *Context) appendShiftGhosts(dam []geom.Rect, ea, skip geom.Rect) ([]geom.Rect, geom.Rect) {
	for _, sh := range c.shifts {
		if sh.r.Intersect(ea).Empty() {
			continue
		}
		eg := ea.Offset(sh.d.X, sh.d.Y).Intersect(sh.r)
		if !eg.Empty() && skip.Intersect(eg) != eg {
			dam = append(dam, eg)
			skip = eg
		}
	}
	return dam, skip
}

// appendOutside appends to dam the parts of x outside r (up to four bands).
func appendOutside(dam []geom.Rect, x, r geom.Rect) []geom.Rect {
	in := x.Intersect(r)
	if in.Empty() {
		return append(dam, x)
	}
	if in == x {
		return dam
	}
	if in.Y > x.Y {
		dam = append(dam, geom.Rect{X: x.X, Y: x.Y, W: x.W, H: in.Y - x.Y})
	}
	if in.Bottom() < x.Bottom() {
		dam = append(dam, geom.Rect{X: x.X, Y: in.Bottom(), W: x.W, H: x.Bottom() - in.Bottom()})
	}
	if in.X > x.X {
		dam = append(dam, geom.Rect{X: x.X, Y: in.Y, W: in.X - x.X, H: in.H})
	}
	if in.Right() < x.Right() {
		dam = append(dam, geom.Rect{X: in.Right(), Y: in.Y, W: x.Right() - in.Right(), H: in.H})
	}
	return dam
}

// resolveShifts validates the frame's ScrollShift calls, appends the damage
// each one implies to dam (or the whole region when it is withdrawn) and
// returns the regions renderers may copy.
func (c *Context) resolveShifts(dam *[]geom.Rect) []paint.ScrollRegion {
	var out []paint.ScrollRegion
	final := c.shifts[:0:0]
	for _, sh := range c.shifts {
		dup := false
		for _, f := range final {
			if f.r == sh.r && f.d == sh.d && f.content == sh.content {
				dup = true
			}
		}
		if !dup {
			final = append(final, sh)
		}
	}
	for _, st := range c.staleShifts {
		again := false
		for _, f := range final {
			if f.r == st.r && f.d == st.d {
				again = true
			}
		}
		if !again {
			*dam = append(*dam, st.r) // declared by a discarded pass only
		}
	}
	for i, sh := range final {
		ok := sh.d != (geom.Point{}) && abs32(sh.d.X) < sh.r.W && abs32(sh.d.Y) < sh.r.H
		for j, o := range final {
			if i != j && !o.r.Intersect(sh.r).Empty() {
				ok = false // overlapping shifts: order is ambiguous
			}
		}
		var extra []geom.Rect
		if ok {
			extra, ok = c.shiftDamage(sh)
		}
		if !ok {
			*dam = append(*dam, sh.r)
			continue
		}
		*dam = append(*dam, extra...)
		out = append(out, paint.ScrollRegion{Rect: sh.r, Delta: sh.d})
	}
	return out
}

// shiftDamage classifies the nodes inside a ScrollShift region. It returns
// the damage the shift needs, or false when the region must be redrawn.
func (c *Context) shiftDamage(sh scrollShift) ([]geom.Rect, bool) {
	r, d, content := sh.r, sh.d, sh.content
	// The scrolled pixels are the content (for a scroll container, its
	// children); it must have been laid out and painted in this pass and
	// overlap the region.
	if content.drawn != c.pass {
		return nil, false
	}
	ancestor := map[*node]bool{}
	for a := content; a != nil; a = a.parent {
		ancestor[a] = true
		if a.xf != nil {
			return nil, false // shift in screen space through a transform: not a translation
		}
	}
	if content.scroll != nil {
		seen := false
		for _, k := range content.kids {
			if k.drawn == c.pass && !k.scr.Intersect(r).Empty() {
				seen = true
			}
		}
		if !seen && len(content.kids) > 0 {
			return nil, false
		}
	} else if content.scr.Intersect(r).Empty() {
		return nil, false
	}

	var dam []geom.Rect
	// Uncovered strip(s): r minus r shifted by d.
	if d.Y > 0 {
		dam = append(dam, geom.Rect{X: r.X, Y: r.Y, W: r.W, H: d.Y})
	} else if d.Y < 0 {
		dam = append(dam, geom.Rect{X: r.X, Y: r.Bottom() + d.Y, W: r.W, H: -d.Y})
	}
	if d.X > 0 {
		dam = append(dam, geom.Rect{X: r.X, Y: r.Y, W: d.X, H: r.H})
	} else if d.X < 0 {
		dam = append(dam, geom.Rect{X: r.Right() + d.X, Y: r.Y, W: -d.X, H: r.H})
	}
	ok := true
	var walk func(n *node)
	walk = func(n *node) {
		if !ok || n.drawn != c.pass {
			return // culled subtrees paint nothing
		}
		switch {
		case n == content && content.scroll == nil, n.parent == content:
			return // moves with the content (with its own PaintOver)
		case ancestor[n]:
			if s := n.scroll; s != nil && n.over != nil {
				// Scrollbars over the inline end edge (right in LTR,
				// left in RTL: s.rtl is what paintScrollbars used this
				// frame) and the bottom edge.
				v := n.scr
				if s.canY() {
					x := v.Right() - damageGutter
					if s.rtl {
						x = v.X
					}
					dam = append(dam, geom.Rect{X: x, Y: v.Y, W: damageGutter, H: v.H}.Intersect(r))
				}
				if s.canX() {
					dam = append(dam, geom.Rect{X: v.X, Y: v.Bottom() - damageGutter, W: v.W, H: damageGutter}.Intersect(r))
				}
			} else if n.over != nil && !n.scr.Intersect(r).Empty() {
				ok = false // paints over the content with unknown extent
				return
			}
		default:
			if !n.scr.Intersect(r).Empty() {
				if n.scr.Intersect(r) != n.scr {
					ok = false // straddles the region edge
					return
				}
				b := n.scr.Expand(damagePadShift)
				dam = append(dam, b.Intersect(r), b.Offset(d.X, d.Y).Intersect(r))
			}
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return dam, ok
}

// mergeDamage clips rects to the view and drops empty ones and those
// inside another rect, keeping declaration order.
func mergeDamage(rs []geom.Rect, view geom.Rect) []geom.Rect {
	out := rs[:0]
	for _, r := range rs {
		if r = r.Intersect(view); !r.Empty() {
			out = append(out, r)
		}
	}
	keep := out[:0]
	for i, r := range out {
		inside := false
		for j, o := range out {
			if i != j && o.Intersect(r) == r && (o != r || j < i) {
				inside = true
				break
			}
		}
		if !inside {
			keep = append(keep, r)
		}
	}
	return keep
}

// listHasHints reports whether l carries any Damage or ScrollRegion op.
func listHasHints(l *paint.List) bool {
	for _, op := range l.Ops {
		switch op.(type) {
		case paint.Damage, paint.ScrollRegion:
			return true
		}
	}
	return false
}

// dirtyRects maps every hint of l through its transform stack to device
// pixels (rounded outwards, clipped to the frame).
func dirtyRects(l *paint.List, scale float32, size geom.Size) []image.Rectangle {
	bounds := image.Rect(0, 0, int(math.Ceil(float64(size.W*scale))), int(math.Ceil(float64(size.H*scale))))
	stack := []geom.Affine{geom.Identity()}
	var out []image.Rectangle
	add := func(r geom.Rect) {
		r = stack[len(stack)-1].Bounds(r)
		ir := image.Rect(
			int(math.Floor(float64(r.X*scale))), int(math.Floor(float64(r.Y*scale))),
			int(math.Ceil(float64(r.Right()*scale))), int(math.Ceil(float64(r.Bottom()*scale)))).Intersect(bounds)
		if !ir.Empty() {
			out = append(out, ir)
		}
	}
	for _, op := range l.Ops {
		switch o := op.(type) {
		case paint.PushTransform:
			stack = append(stack, stack[len(stack)-1].Mul(o.M))
		case paint.PushClip, paint.PushOpacity:
			stack = append(stack, stack[len(stack)-1])
		case paint.Pop:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case paint.Damage:
			add(o.Rect)
		case paint.ScrollRegion:
			add(o.Rect)
		}
	}
	return out
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
