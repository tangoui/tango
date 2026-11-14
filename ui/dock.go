package ui

import (
	"slices"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Dock

// DockW is a docking layout (IDE workbench): nested horizontal / vertical
// splits whose leaves are tab groups, edited by the user and kept in a
// DockLayout the application owns.
type DockW struct {
	_ [0]func()

	c   *Context
	th  *theme.Theme
	tk  *theme.DockToken
	sp  *theme.SplitterToken
	loc *Locale
	l   *DockLayout

	tab         func(key string) TabItem
	content     func(key string)
	onTear      func(key string, pos geom.Point)
	beforeClose func(key string) bool
	opts        []layout.Option

	root    *Box
	st      *dockState
	geo     []dockGeo // groups built this frame
	ops     []func() bool
	changed bool
	closed  string
	torn    *tearOff
}

// dockGeo is a group as built: node IDs for the next frame's hit tests
// (last-frame rectangles) and nodes for this frame's paint.
type dockGeo struct {
	group        string
	count        int // tabs in the group
	header, body ID
	tabs         []ID
	hdrN         *node
	bodyN        *node
	tabN         []*node
}

// dockDrop is where a dragged tab would land.
type dockDrop struct {
	ok     bool
	tear   bool // outside the window
	group  string
	insert int // >= 0: index in the group's tab bar; -1: a zone of its body
	side   DockSide
}

type dockDrag struct {
	on     bool
	key    string
	from   string // source group
	ghost  geom.Rect
	target dockDrop
	label  string
	ic     *icon.Icon
}

type dockState struct {
	geo  []dockGeo // last frame
	drag dockDrag
}

type dockGutterState struct {
	on    bool
	start []float64
	from  geom.Point
}

// Dock adds a docking layout editing l: tab groups in nested splits. Drag
// a tab onto another group's tab bar to move it there, onto the left,
// right, top or bottom part of a group's content to split that group, or
// onto its middle to merge; a preview shows where it lands. Drag the bars
// between groups to resize them; the header buttons split the active tab
// to the right and maximize / restore the group. Tabs close with their x
// (or Delete), and a group left empty disappears.
//
// Tab supplies each key's label and icon (TabItem.NoClose keeps a tab
// open), Content builds the active tab of each group. Call Show after the
// options, then read Changed (save l) and Closed. The layout is edited in
// place; the content of a tab keeps its widget state when the tab moves to
// another group.
//
// Experimental: the API may change in any 0.x release.
func Dock(c *Context, l *DockLayout) *DockW {
	th := c.ThemeFor(theme.CompDock)
	return &DockW{c: c, th: th, tk: &th.Components.Dock, sp: &th.Components.Splitter, loc: c.Locale(), l: l}
}

// Tab sets how a tab key is shown (label, icon, NoClose, Disabled; Key
// is ignored). Without it the key is the label.
func (d *DockW) Tab(fn func(key string) TabItem) *DockW { d.tab = fn; return d }

// Content sets the function that builds the content of a group's active
// tab. It runs once per visible group.
func (d *DockW) Content(fn func(key string)) *DockW { d.content = fn; return d }

// OnTearOff lets a tab be dragged out of the window: released outside, the
// tab leaves the layout and fn runs with its key and the release point
// (window coordinates; see TabsW.OnTearOff). The application opens it in
// a new window.
func (d *DockW) OnTearOff(fn func(key string, pos geom.Point)) *DockW { d.onTear = fn; return d }

// BeforeClose is asked before a tab closes; returning false keeps it (an
// unsaved document).
func (d *DockW) BeforeClose(fn func(key string) bool) *DockW { d.beforeClose = fn; return d }

// Style applies layout options to the dock (give it a size: layout.Grow(1),
// layout.H(480) ...).
func (d *DockW) Style(opts ...layout.Option) *DockW { d.opts = append(d.opts, opts...); return d }

// Changed reports an edit of the layout this frame (a move, split, close,
// resize, maximize, focus or active tab change): the moment to save it.
func (d *DockW) Changed() bool { return d.changed }

// Closed returns the key of the tab closed this frame ("" when none).
func (d *DockW) Closed() string { return d.closed }

// TornOff reports the tab torn off this frame (see OnTearOff).
func (d *DockW) TornOff() (key string, pos geom.Point, ok bool) {
	if x := d.torn; x != nil {
		return x.key, x.pos, true
	}
	return "", geom.Point{}, false
}

// Show builds the dock.
func (d *DockW) Show() *DockW {
	c, tk := d.c, d.tk
	d.l.Normalize() // cheap; repairs edits the application made by hand
	d.root = c.box("dock", layout.Column, nil)
	d.root.n.ln.MinW, d.root.n.ln.MinH = 0, 0
	flexbox.Apply(&d.root.n.ln.Style, d.opts...)
	d.root.Bg(tk.Bg).Clip()
	d.st = State[dockState](c, d.root.n.id, nil)
	if c.active == 0 {
		d.st.drag.on = false // a drag whose bar went away
	}
	c.body(d.root.n, func() {
		if g := d.l.Group(d.l.Maximized); g != nil {
			d.group(g, 1)
		} else {
			d.node(d.l.Root, 1)
		}
	})
	d.st.geo = d.geo
	for _, op := range d.ops {
		if op() {
			d.changed = true
		}
	}
	if d.changed {
		c.invalid = true
	}
	d.root.PaintOver(d.paintDrag)
	return d
}

// grow makes n take share of its split's length.
func dockGrow(n *node, share float64) {
	n.ln.Grow, n.ln.Basis = float32(share), 0
	n.ln.MinW, n.ln.MinH = 0, 0
}

// node builds a split or a group taking share of its parent.
func (d *DockW) node(n *DockNode, share float64) {
	if n.IsGroup() {
		d.group(n, share)
		return
	}
	c := d.c
	dir := layout.Row
	if n.Axis == DockColumn {
		dir = layout.Column
	}
	b := c.box("dock-split", dir, nil)
	dockGrow(b.n, share)
	b.n.ln.Align = layout.AlignStretch
	c.body(b.n, func() {
		for i, k := range n.Children {
			if i > 0 {
				d.gutter(n, i-1, b)
			}
			Key(c, dockKey(k), func() { d.node(k, n.Sizes[i]) })
		}
	})
}

// dockKey names a subtree for Key: its first group's ID.
func dockKey(n *DockNode) string {
	for !n.IsGroup() {
		n = n.Children[0]
	}
	return n.ID
}

// gutter builds the bar between children i and i+1 of split s (box b).
func (d *DockW) gutter(s *DockNode, i int, b *Box) {
	c, tk, sp, th := d.c, d.tk, d.sp, d.th
	row := s.Axis == DockRow
	n := c.add("dock-gutter", "dock-gutter-"+strconv.Itoa(i))
	cur := input.CursorResizeNS
	if row {
		n.ln.Width = tk.GutterSize
		cur = input.CursorResizeEW
	} else {
		n.ln.Height = tk.GutterSize
	}
	n.ownKeys = true
	ia := c.interact(n, true, cur)
	n.setRole(a11y.RoleSeparator).setName(d.loc.A11y.Splitter)
	if row {
		n.setState(a11y.Vertical)
	} else {
		n.setState(a11y.Horizontal)
	}
	gs := State[dockGutterState](c, n.id, nil)
	br := c.rects[b.n.id]
	total := br.H
	if row {
		total = br.W
	}
	total -= float32(len(s.Children)-1) * tk.GutterSize
	if total <= 0 {
		total = 1
	}
	// set gives child i the fraction a of the pair, both at least MinPaneSize.
	set := func(a float64, start []float64) {
		pair := start[i] + start[i+1]
		lo := min(float64(tk.MinPaneSize/total), pair/2)
		a = min(max(a, lo), pair-lo)
		if a != s.Sizes[i] {
			s.Sizes[i], s.Sizes[i+1] = a, pair-a
			c.invalid = true
		}
	}
	if ia.PressedNow && !gs.on {
		gs.on, gs.start, gs.from = true, slices.Clone(s.Sizes), c.pointer
	}
	if gs.on && !c.Dragging(n.id) {
		gs.on = false
		d.changed = true
	}
	if gs.on && c.Dragging(n.id) && len(gs.start) == len(s.Sizes) {
		c.KeepPointer(n.id)
		delta := c.pointer.Y - gs.from.Y
		if row {
			delta = c.pointer.X - gs.from.X
			if c.IsRTL() {
				delta = -delta // the first child is on the right
			}
		}
		set(gs.start[i]+float64(delta/total), gs.start)
	}
	if ia.Focused {
		step := float64(10 / total)
		for _, k := range c.KeysFor(n.id) {
			if !k.Down {
				continue
			}
			prev, next := c.inlineKey(k.Key)
			if !row {
				prev, next = k.Key == input.KeyUp, k.Key == input.KeyDown
			}
			switch {
			case prev:
				set(s.Sizes[i]-step, s.Sizes)
				d.changed = true
			case next:
				set(s.Sizes[i]+step, s.Sizes)
				d.changed = true
			}
		}
	}
	if c.a11yOn {
		n.fill(semFunc(func(sm *a11y.Node) {
			pair := s.Sizes[i] + s.Sizes[i+1]
			rangeSem(sm, float64(int(s.Sizes[i]/pair*1000+0.5))/10, 0, 100, 1)
			sm.Value = strconv.FormatFloat(sm.ValueNow, 'f', -1, 64) + "%"
		}))
	}
	id := n.id
	n.paint = func(l *paint.List, r geom.Rect) {
		drag := c.Dragging(id)
		hov := max(c.Anim(id, "hover", b2f(ia.Hovered), th.DurationMid), b2f(drag))
		if hov > 0 {
			col := theme.Lerp(sp.HandleHoverColor, sp.HandleActiveColor, b2f(drag))
			l.Fill(r, geom.Radii{}, theme.Fade(col, float64(hov)))
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Radii{})
		}
	}
}

// tabItem returns how key is shown.
func (d *DockW) tabItem(key string) TabItem {
	if d.tab == nil {
		return TabItem{Key: key, Label: key}
	}
	it := d.tab(key)
	it.Key = key
	if it.Label == "" {
		it.Label = key
	}
	return it
}

// group builds a tab group taking share of its parent.
func (d *DockW) group(g *DockNode, share float64) {
	c, tk := d.c, d.tk
	box := c.box("dock-group", layout.Column, nil)
	dockGrow(box.n, share)
	box.Bg(tk.GroupBg).Clip()
	label := d.loc.Dock.Group
	items := make([]TabItem, len(g.Tabs))
	for i, k := range g.Tabs {
		items[i] = d.tabItem(k)
		if k == g.Active {
			label = items[i].Label
		}
	}
	box.n.setRole(a11y.RoleGroup).setName(label)
	// A press anywhere in the group focuses it.
	if c.pressedAny && d.l.Focus != g.ID && c.pressLayer == c.buildLayer() {
		if r, ok := c.rects[box.n.id]; ok && r.Contains(c.pressPos) {
			d.l.Focus = g.ID
			d.changed = true
		}
	}
	geo := dockGeo{group: g.ID, count: len(g.Tabs)}
	gid := g.ID
	multi := len(d.l.Groups()) > 1
	maxed := d.l.Maximized == gid
	c.body(box.n, func() {
		// Stable identities: the group's widgets by group ID, a tab's
		// content by its key, wherever they sit in the tree.
		c.scope = hashID(d.root.n.id, "dock-group", gid)
		hdr := c.box("dock-header", layout.Row, nil)
		hdr.Bg(tk.HeaderBg)
		hdr.n.ln.Align = layout.AlignEnd
		geo.header, geo.hdrN = hdr.n.id, hdr.n
		var tb *TabsW
		hdr.Body(func() {
			tb = Tabs(c, &g.Active, items).Closable().Size(Small).NoMargin().Draggable().
				TabBarStyle(layout.Grow(1), layout.MinW(0))
			tb.drag.hook = &dockHook{d: d, group: gid}
			tb.drag.dim = tk.HeaderBg
			tb.Extra(func() { d.actions(g, multi, maxed) })
			tb.Show()
		})
		focused := multi && d.l.Focus == gid
		hdr.PaintOver(func(l *paint.List, r geom.Rect) {
			if tk.HeaderBorder.A > 0 {
				l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.HeaderBorder)
			}
			if focused && tk.ActiveBorder.A > 0 {
				l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, geom.Radii{}, tk.ActiveBorder)
			}
		})
		for i := range tb.tabIDs {
			geo.tabs = append(geo.tabs, tb.tabIDs[i])
		}
		geo.tabN = tb.drag.nodes
		if k := tb.Closed(); k != "" {
			d.ops = append(d.ops, func() bool { return d.close(k) })
		}
		if it := tb.Reordered(); it != nil {
			g.Tabs = g.Tabs[:0]
			for _, x := range it {
				g.Tabs = append(g.Tabs, x.Key)
			}
			d.changed = true
		}
		if tb.Changed() {
			d.l.Focus = gid
			d.changed = true
		}
		var body *Box
		if g.Active == "" {
			body = Column(c, func() {
				Text(c, d.loc.Dock.Empty).Color(tk.EmptyText)
			}, layout.Grow(1), layout.MinH(0), layout.MinW(0), layout.Center())
		} else {
			key := g.Active
			body = tb.Panel(func() {
				c.scope = hashID(d.root.n.id, "dock-tab", key)
				if d.content != nil {
					d.content(key)
				}
			})
			body.n.ln.Dir = layout.Column
			body.n.ln.Grow, body.n.ln.MinH, body.n.ln.MinW = 1, 0, 0
			body.Clip()
		}
		geo.body, geo.bodyN = body.n.id, body.n
	})
	d.geo = append(d.geo, geo)
}

// actions builds the header buttons of group g: split the active tab to
// the right, maximize / restore.
func (d *DockW) actions(g *DockNode, multi, maxed bool) {
	c, tk := d.c, d.tk
	gid := g.ID
	box := tk.ActionBoxSize
	Row(c, func() {
		if len(g.Tabs) > 1 && !maxed {
			var b *IconButtonW
			Tooltip(c, d.loc.Dock.Split, func() { b = IconButton(c, icon.SplitPane, d.loc.Dock.Split).Size(box, 16) })
			if b.Clicked() {
				key := g.Active
				d.ops = append(d.ops, func() bool {
					_, ok := d.l.SplitTab(key, gid, DockRight)
					return ok
				})
			}
		}
		if multi || maxed {
			ic, tip := icon.Fullscreen, d.loc.Dock.Maximize
			if maxed {
				ic, tip = icon.FullscreenExit, d.loc.Dock.Restore
			}
			var b *IconButtonW
			Tooltip(c, tip, func() { b = IconButton(c, ic, tip).Size(box, 16) })
			if b.Clicked() {
				d.ops = append(d.ops, func() bool {
					if d.l.Maximized == gid {
						d.l.Maximized = ""
					} else {
						d.l.Maximized, d.l.Focus = gid, gid
					}
					return true
				})
			}
		}
	}, layout.Gap(2), layout.Middle(), layout.Margin(geom.Insets{Right: 4}))
}

// close closes key unless BeforeClose keeps it.
func (d *DockW) close(key string) bool {
	if d.beforeClose != nil && !d.beforeClose(key) {
		return false
	}
	if !d.l.RemoveTab(key) {
		return false
	}
	d.closed = key
	return true
}

// target returns where a tab of group src dragged to p would land, from
// the last frame's geometry.
func (d *DockW) target(src string, p geom.Point) dockDrop {
	c := d.c
	if c.outsideWindow(p) {
		return dockDrop{ok: d.onTear != nil, tear: true}
	}
	for _, g := range d.st.geo {
		if hr, ok := c.rects[g.header]; ok && hr.Contains(p) {
			rs := make([]geom.Rect, len(g.tabs))
			for i, id := range g.tabs {
				rs[i] = c.rects[id]
			}
			return dockDrop{ok: true, group: g.group, insert: tabInsertIndex(c, rs, p, false)}
		}
		br, ok := c.rects[g.body]
		if !ok || !br.Contains(p) {
			continue
		}
		side := d.zone(br, p)
		if g.group == src && (side == DockCenter || g.count <= 1) {
			return dockDrop{} // nothing would change
		}
		return dockDrop{ok: true, group: g.group, insert: -1, side: side}
	}
	return dockDrop{}
}

// zone returns the part of a group body r under p: the edge within a
// quarter of the size, else the middle. Left / Right are logical.
func (d *DockW) zone(r geom.Rect, p geom.Point) DockSide {
	if r.W <= 0 || r.H <= 0 {
		return DockCenter
	}
	fx, fy := (p.X-r.X)/r.W, (p.Y-r.Y)/r.H
	side, m := DockCenter, float32(0.25)
	for _, e := range []struct {
		d float32
		s DockSide
	}{{fx, DockLeft}, {1 - fx, DockRight}, {fy, DockTop}, {1 - fy, DockBottom}} {
		if e.d < m {
			side, m = e.s, e.d
		}
	}
	if d.c.IsRTL() {
		switch side {
		case DockLeft:
			side = DockRight
		case DockRight:
			side = DockLeft
		}
	}
	return side
}

// zoneRect is the preview of a drop on side of body r.
func (d *DockW) zoneRect(r geom.Rect, side DockSide) geom.Rect {
	s := d.c.inline(r)
	switch side {
	case DockLeft:
		return s.Rect(0, r.Y, r.W/2, r.H)
	case DockRight:
		return s.Rect(r.W/2, r.Y, r.W/2, r.H)
	case DockTop:
		return geom.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H / 2}
	case DockBottom:
		return geom.Rect{X: r.X, Y: r.Y + r.H/2, W: r.W, H: r.H / 2}
	}
	return r
}

// apply queues the drop of key from group src.
func (d *DockW) apply(key string, t dockDrop, p geom.Point) {
	if !t.ok {
		return
	}
	d.ops = append(d.ops, func() bool {
		switch {
		case t.tear:
			if !d.l.RemoveTab(key) {
				return false
			}
			d.torn = &tearOff{key: key, pos: p}
			if d.onTear != nil {
				d.onTear(key, p)
			}
			return true
		case t.insert >= 0:
			moved := d.l.MoveTab(key, t.group, t.insert)
			return d.l.Activate(key) || moved
		default:
			_, ok := d.l.SplitTab(key, t.group, t.side)
			return ok
		}
	})
}

// paintDrag draws the drop preview and the dragged tab over the dock.
func (d *DockW) paintDrag(l *paint.List, _ geom.Rect) {
	st := &d.st.drag
	if !st.on {
		return
	}
	c, th, tk := d.c, d.th, d.tk
	t := st.target
	if t.ok && !t.tear {
		for _, g := range d.geo {
			if g.group != t.group {
				continue
			}
			if t.insert >= 0 {
				rs := make([]geom.Rect, len(g.tabN))
				for i, n := range g.tabN {
					rs[i] = n.abs
				}
				if len(rs) > 0 {
					l.Fill(tabInsertMark(c, rs, t.insert, false), geom.Rad(1), tk.DropBorder)
				}
				break
			}
			zr := d.zoneRect(g.bodyN.abs, t.side).Inset(geom.All(2))
			rad := geom.Rad(tk.DropRadius)
			l.Fill(zr, rad, tk.DropBg)
			l.Stroke(zr, rad, 1, tk.DropBorder)
			break
		}
	}
	paintTabGhost(c, l, th, st.ghost, tk.GhostBg, tk.GhostText, tk.GhostRadius, st.label, st.ic, c.style(th.FontSize, text.Regular))
}

// dockHook receives the tab drags of one group's bar.
type dockHook struct {
	d     *DockW
	group string
}

func (h *dockHook) tabDragMove(t *TabsW, key string, p geom.Point, ghost geom.Rect) {
	d := h.d
	it := d.tabItem(key)
	d.st.drag = dockDrag{on: true, key: key, from: h.group, ghost: ghost, target: d.target(h.group, p), label: it.Label, ic: it.Icon}
}

func (h *dockHook) tabDragEnd(t *TabsW, key string, p geom.Point, drop bool) {
	d := h.d
	d.st.drag.on = false
	if drop {
		d.apply(key, d.target(h.group, p), p)
	}
}
