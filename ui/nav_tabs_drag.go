package ui

import (
	"slices"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Tabs: drag to reorder, drag out of the window to tear off

// tearOff is a tab dragged out of the window and released there: its key
// and the release point in the window's logical coordinates (outside the
// window's bounds).
type tearOff struct {
	key string
	pos geom.Point
}

// tabsDrag holds the Draggable / OnTearOff options and results of a TabsW.
type tabsDrag struct {
	on        bool
	onTear    func(key string, pos geom.Point)
	hook      tabDragHook // Dock: takes over drop targets, previews and drops
	reordered []TabItem
	torn      *tearOff
	barID     ID          // the bar's outer box
	nodes     []*node     // the tab nodes built this frame
	dim       theme.Color // fill over the dragged tab (zero: th.BgContainer)
}

// tabDragHook lets a container (Dock) handle a tab drag started in a bar:
// it gets the pointer every frame of the drag and the release, and draws
// the previews itself. The bar only dims the dragged tab.
type tabDragHook interface {
	tabDragMove(t *TabsW, key string, p geom.Point, ghost geom.Rect)
	tabDragEnd(t *TabsW, key string, p geom.Point, drop bool)
}

// tabDragState is the drag in progress of one bar.
type tabDragState struct {
	on, cancel bool
	src        ID
	key        string
	from, to   int        // dragged index; insertion index (0..len), -1 when none
	grab       geom.Point // pointer offset inside the tab at the press
	size       geom.Size
}

// Draggable lets the user reorder the tabs by dragging them (the moved
// tab becomes active); read the new order with Reordered. With the
// keyboard, Shift plus the shortcut modifier (Ctrl, Cmd on macOS) and the
// arrow keys move the focused tab.
func (t *TabsW) Draggable() *TabsW { t.drag.on = true; return t }

// OnTearOff makes the tabs draggable and lets a tab be dragged out of the
// window: released outside, fn runs with its key and the release point,
// and TornOff reports them. pos is in the window's logical coordinates, so
// it lies outside the window's bounds (negative, or past its size); package
// ui does not know where the window is on the screen, so add the window's
// position (for a uihost window, tango.Surface.Bounds) to place a new
// window under the pointer. The bar does not remove the tab: the
// application opens it in a new window and removes it from the items.
func (t *TabsW) OnTearOff(fn func(key string, pos geom.Point)) *TabsW {
	t.drag.on, t.drag.onTear = true, fn
	return t
}

// Reordered returns the items in their new order after a drag or keyboard
// move this frame (nil when none). Assign it to the slice the bar is built
// from.
func (t *TabsW) Reordered() []TabItem { return t.drag.reordered }

// TornOff reports the tab torn off this frame and its release point (see
// OnTearOff).
func (t *TabsW) TornOff() (key string, pos geom.Point, ok bool) {
	if x := t.drag.torn; x != nil {
		return x.key, x.pos, true
	}
	return "", geom.Point{}, false
}

// moveItems returns a copy of items with the item at from moved to index to.
func moveItems[T any](items []T, from, to int) []T {
	out := slices.Delete(slices.Clone(items), from, from+1)
	return slices.Insert(out, to, items[from])
}

// reorder records a move of tab from to index to (already adjusted for the
// tab itself) and activates it.
func (t *TabsW) reorder(from, to int) {
	if to < 0 || to >= len(t.items) || from == to {
		return
	}
	t.drag.reordered = moveItems(t.items, from, to)
	t.c.invalid = true
}

// dragKeys moves the focused tab with Shift + shortcut + arrow (or Page
// Up / Page Down), consuming those keys before the roving focus sees them.
func (t *TabsW) dragKeys(nodes []*node) {
	if !t.drag.on {
		return
	}
	c := t.c
	cur := -1
	for i, n := range nodes {
		if n.id == c.focus {
			cur = i
		}
	}
	if cur < 0 || t.items[cur].Disabled {
		return
	}
	keep := c.keys[:0:0]
	for _, k := range c.keys {
		d := 0
		if k.Mods.Has(input.ModShift) && k.Mods.Shortcut() {
			prev, next := c.inlineKey(k.Key)
			if t.vertical() {
				prev, next = k.Key == input.KeyUp, k.Key == input.KeyDown
			}
			switch {
			case prev || k.Key == input.KeyPageUp:
				d = -1
			case next || k.Key == input.KeyPageDown:
				d = 1
			}
		}
		if d == 0 {
			keep = append(keep, k)
			continue
		}
		if k.Down {
			t.reorder(cur, min(max(cur+d, 0), len(t.items)-1))
		}
	}
	c.keys = keep
}

// tabInsertIndex returns where a tab dropped at p goes among tabs laid out
// in rects (0..len): after every tab whose middle p has passed.
func tabInsertIndex(c *Context, rects []geom.Rect, p geom.Point, vert bool) int {
	n := 0
	for _, r := range rects {
		if vert {
			if p.Y > r.Y+r.H/2 {
				n++
			}
		} else if c.inline(r).Off(p.X) > r.W/2 {
			n++
		}
	}
	return n
}

// tabInsertMark returns the 2px mark drawn where a tab dropped at index to
// goes: in the gap between tabs to-1 and to, inside the first / last tab
// at the ends.
func tabInsertMark(c *Context, rects []geom.Rect, to int, vert bool) geom.Rect {
	n := len(rects)
	if n == 0 {
		return geom.Rect{}
	}
	to = min(max(to, 0), n)
	if vert {
		ref := rects[min(to, n-1)]
		var y float32
		switch {
		case to == 0:
			y = rects[0].Y + 1
		case to == n:
			y = rects[n-1].Bottom() - 1
		default:
			y = (rects[to-1].Bottom() + rects[to].Y) / 2
		}
		return geom.Rect{X: ref.X, Y: y - 1, W: ref.W, H: 2}
	}
	ref := rects[min(to, n-1)]
	var x float32
	switch {
	case to == 0:
		s := c.inline(rects[0])
		x = s.Start() + s.Sign()
	case to == n:
		s := c.inline(rects[n-1])
		x = s.End() - s.Sign()
	default:
		x = (c.inline(rects[to-1]).End() + c.inline(rects[to]).Start()) / 2
	}
	return geom.Rect{X: x - 1, Y: ref.Y, W: 2, H: ref.H}
}

// outsideWindow reports whether p lies outside the window's content area.
func (c *Context) outsideWindow(p geom.Point) bool {
	return p.X < 0 || p.Y < 0 || p.X >= c.size.W || p.Y >= c.size.H
}

// paintTabGhost draws a dragged tab following the pointer: an elevated
// chip with the tab's icon and label.
func paintTabGhost(c *Context, l *paint.List, th *theme.Theme, r geom.Rect, bg, fg theme.Color, rad float32, label string, ic *icon.Icon, st text.Style) {
	rr := geom.Rad(rad)
	l.Opacity(0.92)
	l.Shadows(r, rr, th.ShadowSecondary)
	l.Fill(r, rr, bg)
	l.Stroke(r, rr, 1, th.BorderSecondary)
	lay := text.Shape(label, st, c.textOpts(text.Options{NoWrap: true}))
	w := lay.Width
	fs := st.Size
	if ic != nil {
		w += fs + 6
	}
	x := r.X + max(8, (r.W-w)/2)
	if ic != nil {
		c.drawIconAt(l, ic, x, r.Y+(r.H-fs)/2, fs, fg)
		x += fs + 6
	}
	l.Clip(r, rr)
	l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	l.Pop()
	l.Pop()
}

// dragTabs runs the pointer drag of a Draggable bar, built after the tabs
// (nodes) and the bar's own paint callbacks: a press on a tab that moves
// past DragSlop starts it, the insertion mark and a ghost of the tab follow
// the pointer, and the release reorders the tabs or, outside the window,
// tears the tab off. Escape cancels. Inside a Dock the hook does the
// targeting and drawing.
func (t *TabsW) dragTabs(outer *Box, nodes []*node) {
	if !t.drag.on {
		return
	}
	c := t.c
	t.drag.barID, t.drag.nodes = outer.n.id, nodes
	st := State[tabDragState](c, hashID(outer.n.id, "tab-drag"), nil)
	hook := t.drag.hook
	if st.on && c.active != st.src {
		// Released (or cancelled by the system).
		drop := !st.cancel && c.released[st.src] && !c.PointerCancelled(st.src)
		p := c.releasePos[st.src]
		if hook != nil {
			if !st.cancel {
				hook.tabDragEnd(t, st.key, p, drop)
			}
		} else if drop {
			t.dropAt(st, nodes, p)
		}
		st.on = false
		c.invalid = true
	}
	if !st.on && c.active != 0 {
		for i, n := range nodes {
			if n.id != c.active || t.items[i].Disabled {
				continue
			}
			dx, dy := c.pointer.X-c.pressPos.X, c.pointer.Y-c.pressPos.Y
			if dx*dx+dy*dy >= DragSlop*DragSlop {
				r := c.rects[n.id]
				*st = tabDragState{on: true, src: n.id, key: t.items[i].Key, from: i, to: i,
					grab: geom.Pt(c.pressPos.X-r.X, c.pressPos.Y-r.Y), size: r.Size()}
			}
			break
		}
	}
	if !st.on {
		return
	}
	c.KeepPointer(st.src)
	if !st.cancel && c.Escape() {
		st.cancel = true
		if hook != nil {
			hook.tabDragEnd(t, st.key, c.pointer, false)
		}
	}
	if st.cancel {
		return
	}
	n := len(nodes)
	if st.from >= n {
		return
	}
	src := nodes[st.from]
	if hook != nil {
		ghost := geom.Rect{X: c.pointer.X - st.grab.X, Y: c.pointer.Y - st.grab.Y, W: st.size.W, H: st.size.H}
		hook.tabDragMove(t, st.key, c.pointer, ghost)
	} else {
		st.to = -1
		if !c.outsideWindow(c.pointer) {
			st.to = tabInsertIndex(c, t.rectsOf(nodes), c.pointer, t.vertical())
		}
	}
	th := t.th
	dim := t.drag.dim
	if dim == (theme.Color{}) {
		dim = th.BgContainer
	}
	it := t.items[st.from]
	_, style, _ := t.tabMetrics()
	vert := t.vertical()
	ink := th.Components.Tabs.InkBarColor
	prev := outer.n.over
	outer.n.over = func(l *paint.List, r geom.Rect) {
		if prev != nil {
			prev(l, r)
		}
		l.Fill(src.abs, geom.Rad(th.RadiusSM), theme.Fade(dim, 0.55))
		if hook != nil {
			return
		}
		if st.to >= 0 {
			rects := make([]geom.Rect, n)
			for i, k := range nodes {
				rects[i] = k.abs
			}
			l.Fill(tabInsertMark(c, rects, st.to, vert), geom.Rad(1), ink)
		}
		gr := geom.Rect{X: c.pointer.X - st.grab.X, Y: c.pointer.Y - st.grab.Y, W: st.size.W, H: st.size.H}
		paintTabGhost(c, l, th, gr, th.BgElevated, th.Text, th.RadiusSM, it.Label, it.Icon, style)
	}
}

// rectsOf returns the last-frame rectangles of the tab nodes.
func (t *TabsW) rectsOf(nodes []*node) []geom.Rect {
	rs := make([]geom.Rect, len(nodes))
	for i, n := range nodes {
		rs[i] = t.c.rects[n.id]
	}
	return rs
}

// dropAt finishes a drag of a standalone bar released at p.
func (t *TabsW) dropAt(st *tabDragState, nodes []*node, p geom.Point) {
	c := t.c
	if c.outsideWindow(p) {
		if t.drag.onTear != nil {
			t.drag.torn = &tearOff{key: st.key, pos: p}
			t.drag.onTear(st.key, p)
		}
		return
	}
	to := tabInsertIndex(c, t.rectsOf(nodes), p, t.vertical())
	if to > st.from {
		to--
	}
	t.reorder(st.from, to)
	if *t.active != st.key {
		*t.active = st.key
		t.changed = true
	}
}
