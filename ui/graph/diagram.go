package graph

import (
	"fmt"
	"hash/fnv"
	"math"
	"slices"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// DiagramW builds a node-link diagram canvas; configure it with the
// modifiers and finish with Show.
type DiagramW struct {
	c        *ui.Context
	g        *Graph
	height   float32
	opts     []layout.Option
	grid     float32
	snap     bool
	mini     bool
	noTools  bool
	edgeKind EdgeKind
	auto     *LayoutOptions
	readOnly bool
	marquee  bool
	sel      *[]string
	minZ     float32
	maxZ     float32
	tweaks   []func(*Style)
	name     string
	key      any
	sides    Side
	er       *erData
}

// Diagram starts a diagram of g. The graph is owned by the caller; the
// diagram writes node positions (dragging, AutoLayout, keyboard moves) and
// the measured sizes of nodes with a zero Size back into it.
func Diagram(c *ui.Context, g *Graph) *DiagramW {
	return &DiagramW{c: c, g: g, height: 480, minZ: 0.1, maxZ: 4}
}

// Height sets the canvas height (default 480; the width follows the parent).
func (w *DiagramW) Height(h float32) *DiagramW { w.height = h; return w }

// Layout applies layout options to the canvas box (Grow, Margin ...).
func (w *DiagramW) Layout(o ...layout.Option) *DiagramW { w.opts = append(w.opts, o...); return w }

// Grid draws a dot grid every size graph units (0: none).
func (w *DiagramW) Grid(size float32) *DiagramW { w.grid = size; return w }

// Snap snaps dragged nodes to the grid (Grid sets the step; 16 if unset).
func (w *DiagramW) Snap() *DiagramW { w.snap = true; return w }

// MiniMap shows an overview of the whole graph in the bottom end corner;
// pressing or dragging in it moves the view.
func (w *DiagramW) MiniMap() *DiagramW { w.mini = true; return w }

// NoToolbar hides the zoom / fit / layout buttons.
func (w *DiagramW) NoToolbar() *DiagramW { w.noTools = true; return w }

// Edges sets how edges with Kind EdgeDefault are routed (default Bézier).
func (w *DiagramW) Edges(k EdgeKind) *DiagramW { w.edgeKind = k; return w }

// Sides sets where edge ends without a fixed port attach: SideHorizontal
// (left / right), SideVertical (top / bottom) or SideAuto (whichever side
// faces the other end). The default follows the AutoLayout direction.
func (w *DiagramW) Sides(s Side) *DiagramW { w.sides = s; return w }

// AutoLayout lays the graph out with Layout whenever its set of nodes and
// edges changes (and on the first Show), then fits the view. The toolbar
// gets a button that lays it out again.
func (w *DiagramW) AutoLayout(o LayoutOptions) *DiagramW { w.auto = &o; return w }

// ReadOnly disables node dragging and keyboard moves (panning, zooming and
// selection still work).
func (w *DiagramW) ReadOnly() *DiagramW { w.readOnly = true; return w }

// MarqueeDrag makes a drag on the background draw a selection marquee
// (Space+drag or a two-finger drag pans). By default the background drag
// pans and Shift+drag draws the marquee.
func (w *DiagramW) MarqueeDrag() *DiagramW { w.marquee = true; return w }

// Selection binds the selected node IDs to *ids: the diagram reads it
// every frame and writes it on change.
func (w *DiagramW) Selection(ids *[]string) *DiagramW { w.sel = ids; return w }

// ZoomRange limits the zoom factor (default 0.1 – 4).
func (w *DiagramW) ZoomRange(lo, hi float32) *DiagramW {
	if lo > 0 && hi >= lo {
		w.minZ, w.maxZ = lo, hi
	}
	return w
}

// Style tweaks the resolved style for this diagram only.
func (w *DiagramW) Style(fn func(*Style)) *DiagramW { w.tweaks = append(w.tweaks, fn); return w }

// A11yLabel sets the accessible name of the canvas (default
// Locale.Graph.Diagram).
func (w *DiagramW) A11yLabel(s string) *DiagramW { w.name = s; return w }

// Key gives the diagram a stable identity (viewport, selection) when
// widgets are added or removed around it.
func (w *DiagramW) Key(k any) *DiagramW { w.key = k; return w }

// Interaction modes of a pointer gesture.
const (
	modeNone = iota
	modePan
	modeNodes
	modeMarquee
	modeMini
	modePinch
)

type dragItem struct {
	idx int
	pos geom.Point
}

// diagState is retained per diagram.
type diagState struct {
	pan     geom.Point // screen offset of the graph origin from the box origin
	zoom    float32
	fitted  bool
	fitReq  bool
	touched bool      // the user moved the view: no refit on resize
	size    geom.Size // canvas size of the last paint

	sel      map[string]bool // node IDs
	selEdges map[string]bool // edge IDs (Graph.edgeID)

	mode      int
	start     geom.Point // pointer at press (screen)
	startPan  geom.Point
	moved     bool
	additive  bool
	pressNode int
	items     []dragItem
	marquee   geom.Rect // screen

	space   bool
	fingers map[int64]geom.Point
	pinchD  float32
	pinchC  geom.Point

	layoutSig uint64
	grid      gridCache
	mini      miniCache
	hoverNode int
	hoverEdge int
}

// DiagramView is a shown diagram: its box plus what happened this frame.
// The action methods (FitView, ZoomTo, CenterOn, Select) take effect on the
// next frame.
type DiagramView struct {
	*ui.Box
	c           *ui.Context
	st          *diagState
	g           *Graph
	w           *DiagramW
	clicked     int
	dbl         int
	edgeClicked int
	ctxNode     int
	ctxPos      geom.Point
	ctx         bool
	moved       bool
	del         bool
}

// Show builds the diagram and handles this frame's input.
func (w *DiagramW) Show() *DiagramView {
	var v *DiagramView
	if w.key != nil {
		ui.Key(w.c, w.key, func() { v = w.show() })
	} else {
		v = w.show()
	}
	return v
}

func (w *DiagramW) show() *DiagramView {
	c := w.c
	th := c.Theme
	sty := StyleOf(th)
	for _, f := range w.tweaks {
		f(sty)
	}
	loc := c.Locale()
	opts := append([]layout.Option{layout.H(w.height)}, w.opts...)
	box := ui.Div(c, opts...).Clip()
	if w.er != nil {
		w.g = w.er.graphFor(c, box.ID(), sty)
		if w.name == "" {
			w.name = loc.Graph.ER
		}
	}
	g := w.g
	g.stale()
	w.measure(sty)
	st := ui.State[diagState](c, box.ID(), func() diagState {
		return diagState{zoom: 1, sel: map[string]bool{}, selEdges: map[string]bool{}, fingers: map[int64]geom.Point{}, hoverNode: -1, hoverEdge: -1, pressNode: -1}
	})
	v := &DiagramView{Box: box, c: c, st: st, g: g, w: w, clicked: -1, dbl: -1, edgeClicked: -1, ctxNode: -1}
	if w.sel != nil {
		w.syncSelIn(st)
	}

	if w.auto != nil {
		if sig := structureSig(g); sig != st.layoutSig {
			st.layoutSig = sig
			Layout(g, *w.auto)
			st.fitReq = true
		}
	}

	r := box.Rect()
	m := st.xform(r)
	p, pin := c.Pointer()

	// Hover (the pointer over the canvas, no gesture running).
	st.hoverNode, st.hoverEdge = -1, -1
	if pin && r.Contains(p) && st.mode == modeNone && !w.inMini(r, p) {
		if i := nodeAt(g, m, p); i >= 0 {
			st.hoverNode = i
		} else {
			st.hoverEdge = edgeAt(g, m, p, w.edgeKind, w.pref())
		}
	}
	cursor := input.CursorDefault
	switch {
	case st.mode == modePan || (st.space && st.mode == modeNone):
		cursor = input.CursorGrabbing
		if st.mode == modeNone {
			cursor = input.CursorGrab
		}
	case st.mode == modeNodes && st.moved:
		cursor = input.CursorMove
	case st.hoverNode >= 0 && !w.readOnly && !g.Nodes[st.hoverNode].Fixed:
		cursor = input.CursorMove
	case st.hoverNode >= 0 || st.hoverEdge >= 0:
		cursor = input.CursorPointer
	case !w.marquee && st.mode == modeNone:
		cursor = input.CursorGrab
	}
	ia := box.Focusable(cursor)
	name := w.name
	if name == "" {
		name = loc.Graph.Diagram
	}
	box.Semantics(a11y.RoleGroup, name).A11y(func(s *a11y.Node) {
		s.RoleDescription = name
		s.Description = fmt.Sprintf(loc.Graph.SummaryFmt, len(g.Nodes), len(g.Edges))
		if n := len(st.sel); n > 0 {
			s.Value = loc.Selected(n)
		}
	})

	// Two-finger pinch / pan.
	w.touches(box, st, r)

	// Pointer gestures.
	if ia.PressedNow && pin && st.mode == modeNone {
		w.press(st, r, m, p, c.PressMods())
	}
	if st.mode != modeNone && st.mode != modePinch && c.Dragging(box.ID()) && pin {
		if w.drag(st, r, m, p) {
			v.moved = true
		}
	}
	if st.mode != modeNone && !c.Dragging(box.ID()) {
		if st.mode == modePinch && len(st.fingers) > 0 {
			// Wait for the fingers to lift.
		} else {
			w.release(st, m, v)
		}
	}
	if ia.Clicked && ia.Clicks >= 2 && pin {
		if i := nodeAt(g, m, p); i >= 0 {
			v.dbl = i
		}
	}
	if pos, ok := c.ContextMenuFor(box.ID()); ok {
		v.ctx, v.ctxPos = true, pos
		v.ctxNode = nodeAt(g, m, pos)
	}

	// Wheel: pan, or zoom with Ctrl / ⌘ (trackpad pinch arrives so too).
	if dx, dy, ok := c.WheelFor(box.ID()); ok && (dx != 0 || dy != 0) {
		mods := c.PointerMods()
		if mods.Has(input.ModCtrl) || mods.Has(input.ModMeta) {
			k := float32(math.Pow(1.25, float64(-dy)/40))
			k = max(1/1.5, min(k, 1.5))
			at := p
			if !pin {
				at = r.Center()
			}
			st.zoomAbout(r, at, st.zoom*k, w.minZ, w.maxZ)
		} else {
			st.pan.X -= dx
			st.pan.Y -= dy
			st.touched = true
		}
	}

	// Keyboard.
	if ia.Focused {
		if w.keys(st, r, v) {
			v.moved = true
		}
	} else {
		st.space = false
	}

	// Toolbar.
	box.Body(func() {
		if !w.noTools {
			w.toolbar(st, r, sty, loc)
		}
	})

	if w.sel != nil {
		w.syncSelOut(st)
	}

	focusVisible := ia.FocusVisible
	box.Paint(func(l *paint.List, rr geom.Rect) {
		if sz := rr.Size(); sz != st.size {
			if !st.touched {
				st.fitted = false // follow the canvas size until the user moves the view
			}
			st.size = sz
		}
		if !st.fitted || st.fitReq {
			if rr.W > 0 && rr.H > 0 {
				st.fit(g, rr, w.minZ, w.maxZ)
				st.fitted, st.fitReq = true, false
			}
		}
		w.paint(l, rr, st, sty, th)
		if focusVisible {
			l.Stroke(rr.Expand(-1), geom.Rad(th.Radius), max(th.FocusRingWidth, 2), th.FocusOutline)
		}
	})
	return v
}

// measure gives nodes with a zero size the built-in template's size.
func (w *DiagramW) measure(sty *Style) {
	for i := range w.g.Nodes {
		n := &w.g.Nodes[i]
		if n.Size.W > 0 && n.Size.H > 0 {
			continue
		}
		n.Size = templateSize(n, sty)
	}
}

// templateSize is the size of the built-in node template for n.
func templateSize(n *Node, sty *Style) geom.Size {
	tw := text.Width(n.Label, text.Style{Size: sty.Font, Weight: text.Medium})
	if n.Detail != "" {
		tw = max(tw, text.Width(n.Detail, text.Style{Size: sty.FontSM}))
	}
	if n.Icon != nil {
		tw += 24
	}
	wd := max(96, min(tw+32, 320))
	h := float32(40)
	if n.Detail != "" {
		h = 56
	}
	switch n.Shape {
	case ShapeEllipse:
		wd, h = wd*1.25, h+8
	case ShapeDiamond:
		wd, h = max(wd*1.6, 120), max(h*1.6, 64)
	case ShapePill:
		wd += h / 2
	}
	return geom.Size{W: float32(math.Ceil(float64(wd))), H: h}
}

// structureSig hashes the node IDs and edge ends: auto layout runs again
// when it changes.
func structureSig(g *Graph) uint64 {
	h := fnv.New64a()
	for i := range g.Nodes {
		h.Write([]byte(g.Nodes[i].ID))
		h.Write([]byte{0})
	}
	h.Write([]byte{1})
	for i := range g.Edges {
		e := &g.Edges[i]
		h.Write([]byte(e.From))
		h.Write([]byte{0})
		h.Write([]byte(e.To))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func (st *diagState) xform(r geom.Rect) xform {
	return xform{Origin: geom.Pt(r.X+st.pan.X, r.Y+st.pan.Y), K: st.zoom}
}

// zoomAbout sets the zoom to k keeping the graph point under screen point
// at fixed.
func (st *diagState) zoomAbout(r geom.Rect, at geom.Point, k, lo, hi float32) {
	st.touched = true
	k = max(lo, min(k, hi))
	if k == st.zoom || st.zoom == 0 {
		return
	}
	m := st.xform(r)
	gp := m.inv(at)
	st.pan = geom.Pt(at.X-r.X-gp.X*k, at.Y-r.Y-gp.Y*k)
	st.zoom = k
}

// fit zooms (never beyond 1) and pans so the whole graph is in view.
func (st *diagState) fit(g *Graph, r geom.Rect, lo, hi float32) {
	b := g.Bounds()
	if b.W <= 0 && b.H <= 0 {
		st.pan, st.zoom = geom.Point{}, 1
		return
	}
	st.touched = false
	const pad = 32
	k := min((r.W-2*pad)/max(b.W, 1), (r.H-2*pad)/max(b.H, 1))
	k = max(lo, min(k, min(hi, 1)))
	st.zoom = k
	st.pan = geom.Pt(r.W/2-(b.X+b.W/2)*k, r.H/2-(b.Y+b.H/2)*k)
}

// nodeAt returns the top-most node under screen point p (-1: none).
func nodeAt(g *Graph, m xform, p geom.Point) int {
	gp := m.inv(p)
	for i := len(g.Nodes) - 1; i >= 0; i-- {
		if g.Nodes[i].Rect().Contains(gp) {
			return i
		}
	}
	return -1
}

// edgeHitPx is the pointer tolerance around an edge, in screen pixels.
const edgeHitPx = 5

// edgeAt returns the edge nearest to screen point p within edgeHitPx.
func edgeAt(g *Graph, m xform, p geom.Point, def EdgeKind, pref Side) int {
	gp := m.inv(p)
	tol := edgeHitPx / m.K
	best, bestD := -1, tol
	var buf []geom.Point
	for i := range g.Edges {
		e := &g.Edges[i]
		a, b := g.Node(e.From), g.Node(e.To)
		if a == nil || b == nil {
			continue
		}
		if !edgeReach(e, a, b).Expand(tol).Contains(gp) {
			continue
		}
		cv := route(e, a, b, def, pref)
		buf = cv.flatten(buf[:0])
		if d := polyDist(buf, gp); d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}

// edgeReach is a cheap box that contains any route of e (the Bézier
// handles reach up to 160 units out of the sides).
func edgeReach(e *Edge, a, b *Node) geom.Rect {
	r := a.Rect().Union(b.Rect())
	for _, p := range e.Bends {
		r = r.Union(geom.Rect{X: p.X, Y: p.Y})
	}
	return r.Expand(170)
}

// pref is the side family of edge ends without a port: the flow of the
// auto layout, else whichever side faces the other end.
func (w *DiagramW) pref() Side {
	if w.sides != SideAuto {
		return w.sides
	}
	if w.auto == nil {
		return SideAuto
	}
	if w.auto.Direction.horizontal() {
		return SideHorizontal
	}
	return SideVertical
}

func (w *DiagramW) miniRect(r geom.Rect) geom.Rect {
	const mw, mh, inset = 200, 140, 12
	s := w.c.Inline(r)
	return s.Rect(r.W-mw-inset, r.Y+r.H-mh-inset, mw, mh)
}

func (w *DiagramW) inMini(r geom.Rect, p geom.Point) bool {
	return w.mini && w.miniRect(r).Contains(p)
}

func (w *DiagramW) press(st *diagState, r geom.Rect, m xform, p geom.Point, mods input.Modifiers) {
	g := w.g
	st.start, st.startPan, st.moved = p, st.pan, false
	st.pressNode = -1
	shift := mods.Has(input.ModShift)
	toggle := shift || mods.Shortcut()
	st.additive = toggle
	switch {
	case w.inMini(r, p):
		st.mode = modeMini
		w.miniMove(st, r, p)
		return
	case st.space:
		st.mode = modePan
		return
	}
	if i := nodeAt(g, m, p); i >= 0 {
		id := g.Nodes[i].ID
		st.pressNode = i
		if toggle {
			if st.sel[id] {
				delete(st.sel, id)
			} else {
				st.sel[id] = true
			}
		} else if !st.sel[id] {
			clear(st.sel)
			clear(st.selEdges)
			st.sel[id] = true
		}
		st.mode = modeNone
		if !w.readOnly && st.sel[id] {
			st.mode = modeNodes
			st.items = st.items[:0]
			for j := range g.Nodes {
				if st.sel[g.Nodes[j].ID] && !g.Nodes[j].Fixed {
					st.items = append(st.items, dragItem{j, g.Nodes[j].Pos})
				}
			}
		}
		if st.mode == modeNone {
			st.mode = modeMarquee // a no-op gesture that ends on release
			st.marquee = geom.Rect{}
		}
		return
	}
	if e := edgeAt(g, m, p, w.edgeKind, w.pref()); e >= 0 {
		id := g.edgeID(e)
		if !toggle {
			clear(st.sel)
			clear(st.selEdges)
		}
		if st.selEdges[id] && toggle {
			delete(st.selEdges, id)
		} else {
			st.selEdges[id] = true
		}
		st.mode = modeMarquee
		st.marquee = geom.Rect{}
		st.pressNode = -2 - e
		return
	}
	if w.marquee || shift {
		st.mode = modeMarquee
		st.marquee = geom.Rect{X: p.X, Y: p.Y, W: 0.001}
		if !toggle {
			clear(st.sel)
			clear(st.selEdges)
		}
		return
	}
	// A click on the background clears the selection on release (unless
	// it turned into a pan).
	st.mode = modePan
}

// dragSlop is how far the pointer must move before a press on a node
// becomes a drag.
const dragSlop = 3

// drag updates the running gesture; it reports node moves.
func (w *DiagramW) drag(st *diagState, r geom.Rect, m xform, p geom.Point) bool {
	d := sub(p, st.start)
	if !st.moved && math.Abs(float64(d.X)) < dragSlop && math.Abs(float64(d.Y)) < dragSlop {
		return false
	}
	first := !st.moved
	st.moved = true
	switch st.mode {
	case modePan:
		st.pan = add(st.startPan, d)
		st.touched = true
	case modeMini:
		st.touched = true
		w.miniMove(st, r, p)
	case modeMarquee:
		if st.marquee.W == 0 {
			return false // click on a node or an edge: nothing to draw
		}
		x0, y0 := min(st.start.X, p.X), min(st.start.Y, p.Y)
		st.marquee = geom.Rect{X: x0, Y: y0, W: float32(math.Abs(float64(d.X))), H: float32(math.Abs(float64(d.Y)))}
	case modeNodes:
		if first {
			w.clearBends(st)
		}
		gd := scale(d, 1/m.K)
		step := w.snapStep()
		for _, it := range st.items {
			np := add(it.pos, gd)
			if step > 0 {
				np = geom.Pt(snapTo(np.X, step), snapTo(np.Y, step))
			}
			w.g.Nodes[it.idx].Pos = np
		}
		return true
	}
	return false
}

func snapTo(v, step float32) float32 { return float32(math.Round(float64(v/step))) * step }

func (w *DiagramW) snapStep() float32 {
	if !w.snap {
		return 0
	}
	if w.grid > 0 {
		return w.grid
	}
	return 16
}

// clearBends drops the layout bends of edges touching the dragged nodes.
func (w *DiagramW) clearBends(st *diagState) {
	moving := make(map[string]bool, len(st.items))
	for _, it := range st.items {
		moving[w.g.Nodes[it.idx].ID] = true
	}
	for i := range w.g.Edges {
		e := &w.g.Edges[i]
		if moving[e.From] || moving[e.To] {
			e.Bends = nil
		}
	}
}

func (w *DiagramW) release(st *diagState, m xform, v *DiagramView) {
	g := w.g
	switch st.mode {
	case modeMarquee:
		if st.marquee.W > 0.01 || st.marquee.H > 0.01 {
			gr := m.invRect(st.marquee)
			for i := range g.Nodes {
				n := &g.Nodes[i]
				if overlapsRect(gr, n.Rect()) {
					st.sel[n.ID] = true
				}
			}
		} else if st.pressNode >= 0 {
			v.clicked = st.pressNode
		} else if st.pressNode <= -2 {
			v.edgeClicked = -2 - st.pressNode
		}
	case modeNodes:
		if !st.moved {
			v.clicked = st.pressNode
			// A plain click on a node of a multi-selection selects it alone.
			if !st.additive && len(st.sel) > 1 {
				clear(st.sel)
				st.sel[g.Nodes[st.pressNode].ID] = true
			}
		}
	case modePan:
		if !st.moved && !st.additive && !st.space {
			clear(st.sel)
			clear(st.selEdges)
		}
	}
	st.mode = modeNone
	st.marquee = geom.Rect{}
	st.items = st.items[:0]
	st.additive = false
}

func overlapsRect(a, b geom.Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

// miniMove centres the view on the graph point under minimap point p.
func (w *DiagramW) miniMove(st *diagState, r geom.Rect, p geom.Point) {
	mr := w.miniRect(r)
	mm, ok := miniXform(w.g, st, r, mr)
	if !ok {
		return
	}
	gp := mm.inv(p)
	st.pan = geom.Pt(r.W/2-gp.X*st.zoom, r.H/2-gp.Y*st.zoom)
}

// miniXform maps graph units into the minimap: the graph bounds plus the
// visible area, fitted with a margin.
func miniXform(g *Graph, st *diagState, r, mr geom.Rect) (xform, bool) {
	b := g.Bounds()
	vis := st.xform(r).invRect(r)
	if len(g.Nodes) == 0 {
		b = vis
	} else {
		b = b.Union(vis)
	}
	if b.W <= 0 || b.H <= 0 {
		return xform{}, false
	}
	const pad = 8
	k := min((mr.W-2*pad)/b.W, (mr.H-2*pad)/b.H)
	ox := mr.X + (mr.W-b.W*k)/2 - b.X*k
	oy := mr.Y + (mr.H-b.H*k)/2 - b.Y*k
	return xform{Origin: geom.Pt(ox, oy), K: k}, true
}

// touches handles two-finger pinch zoom and pan.
func (w *DiagramW) touches(box *ui.Box, st *diagState, r geom.Rect) {
	for _, t := range box.Touches() {
		if t.Done() {
			delete(st.fingers, t.ID)
		} else {
			st.fingers[t.ID] = t.Pos
		}
	}
	if len(st.fingers) < 2 {
		st.pinchD = 0
		return
	}
	ids := make([]int64, 0, len(st.fingers))
	for id := range st.fingers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	a, b := st.fingers[ids[0]], st.fingers[ids[1]]
	ctr := lerp(a, b, 0.5)
	d := length(sub(b, a))
	if st.pinchD > 0 && d > 0 {
		st.touched = true
		st.pan = add(st.pan, sub(ctr, st.pinchC))
		st.zoomAbout(r, ctr, st.zoom*d/st.pinchD, w.minZ, w.maxZ)
	} else if st.mode == modeNodes {
		// A second finger cancels a node drag.
		for _, it := range st.items {
			w.g.Nodes[it.idx].Pos = it.pos
		}
	}
	st.mode = modePinch
	st.pinchD, st.pinchC = d, ctr
}

// keys handles the keyboard: arrows move the selection (or pan), + / -
// zoom, 0 fits, ⌘/Ctrl+A selects all, Esc clears the selection, Delete /
// Backspace requests removal, Space held pans with the pointer.
func (w *DiagramW) keys(st *diagState, r geom.Rect, v *DiagramView) bool {
	g := w.g
	moved := false
	for _, k := range w.c.KeysFor(v.ID()) {
		if k.Key == input.KeySpace {
			st.space = k.Down
			continue
		}
		if !k.Down {
			continue
		}
		var dx, dy float32
		switch k.Key {
		case input.KeyLeft:
			dx = -1
		case input.KeyRight:
			dx = 1
		case input.KeyUp:
			dy = -1
		case input.KeyDown:
			dy = 1
		case input.KeyEqual:
			st.zoomAbout(r, r.Center(), st.zoom*1.2, w.minZ, w.maxZ)
		case input.KeyMinus:
			st.zoomAbout(r, r.Center(), st.zoom/1.2, w.minZ, w.maxZ)
		case input.Key0:
			st.fitReq = true
		case input.KeyA:
			if k.Mods.Shortcut() {
				for i := range g.Nodes {
					st.sel[g.Nodes[i].ID] = true
				}
			}
		case input.KeyDelete, input.KeyBackspace:
			if len(st.sel) > 0 || len(st.selEdges) > 0 {
				v.del = true
			}
		}
		if dx == 0 && dy == 0 {
			continue
		}
		if len(st.sel) > 0 && !w.readOnly {
			step := w.snapStep()
			if step == 0 {
				step = 8
			}
			if k.Mods.Has(input.ModShift) {
				step *= 5
			}
			st.items = st.items[:0]
			for i := range g.Nodes {
				if st.sel[g.Nodes[i].ID] && !g.Nodes[i].Fixed {
					st.items = append(st.items, dragItem{i, g.Nodes[i].Pos})
				}
			}
			w.clearBends(st)
			for _, it := range st.items {
				n := &g.Nodes[it.idx]
				n.Pos = geom.Pt(n.Pos.X+dx*step, n.Pos.Y+dy*step)
			}
			st.items = st.items[:0]
			moved = true
		} else {
			step := float32(48)
			if k.Mods.Has(input.ModShift) {
				step *= 4
			}
			st.pan = geom.Pt(st.pan.X-dx*step, st.pan.Y-dy*step)
			st.touched = true
		}
	}
	if (len(st.sel) > 0 || len(st.selEdges) > 0) && w.c.Escape() {
		clear(st.sel)
		clear(st.selEdges)
	}
	return moved
}

func (w *DiagramW) toolbar(st *diagState, r geom.Rect, sty *Style, loc *ui.Locale) {
	c := w.c
	th := c.Theme
	U := layout.Unset
	tb := ui.Column(c, func() {
		btn := func(ic *icon.Icon, label string) bool {
			return ui.IconButton(c, ic, label).Size(28, 16).Clicked()
		}
		if btn(icon.ZoomIn, loc.Graph.ZoomIn) {
			st.zoomAbout(r, r.Center(), st.zoom*1.25, w.minZ, w.maxZ)
		}
		if btn(icon.ZoomOut, loc.Graph.ZoomOut) {
			st.zoomAbout(r, r.Center(), st.zoom/1.25, w.minZ, w.maxZ)
		}
		if btn(icon.FitView, loc.Graph.FitView) {
			st.fitReq = true
		}
		if w.auto != nil && btn(icon.AutoLayout, loc.Graph.AutoLayout) {
			Layout(w.g, *w.auto)
			st.fitReq = true
		}
	}, layout.Abs(12, U, U, 12), layout.Pad(2), layout.Gap(2))
	tb.Bg(sty.ToolbarBg).Radius(th.Radius).Border(1, sty.MiniBorder).Shadow(sty.ToolbarShadow)
	tb.Semantics(a11y.RoleToolbar, loc.Graph.Diagram)
}

func (w *DiagramW) syncSelIn(st *diagState) {
	ids := *w.sel
	same := len(ids) == len(st.sel)
	if same {
		for _, id := range ids {
			if !st.sel[id] {
				same = false
				break
			}
		}
	}
	if same {
		return
	}
	clear(st.sel)
	for _, id := range ids {
		st.sel[id] = true
	}
}

func (w *DiagramW) syncSelOut(st *diagState) {
	if w.sel == nil {
		return
	}
	if cur := *w.sel; len(cur) == len(st.sel) {
		same := true
		for _, id := range cur {
			if !st.sel[id] {
				same = false
				break
			}
		}
		if same {
			return
		}
	}
	out := (*w.sel)[:0:0]
	for i := range w.g.Nodes {
		if st.sel[w.g.Nodes[i].ID] {
			out = append(out, w.g.Nodes[i].ID)
		}
	}
	*w.sel = out
}

// Selected returns the selected node IDs in graph order.
func (v *DiagramView) Selected() []string {
	var out []string
	for i := range v.g.Nodes {
		if v.st.sel[v.g.Nodes[i].ID] {
			out = append(out, v.g.Nodes[i].ID)
		}
	}
	return out
}

// SelectedEdges returns the indices of the selected edges.
func (v *DiagramView) SelectedEdges() []int {
	var out []int
	for i := range v.g.Edges {
		if v.st.selEdges[v.g.edgeID(i)] {
			out = append(out, i)
		}
	}
	return out
}

func (v *DiagramView) nodeID(i int) (string, bool) {
	if i < 0 || i >= len(v.g.Nodes) {
		return "", false
	}
	return v.g.Nodes[i].ID, true
}

// Clicked returns the node clicked (pressed and released without
// dragging) this frame.
func (v *DiagramView) Clicked() (string, bool) { return v.nodeID(v.clicked) }

// DoubleClicked returns the node double-clicked this frame.
func (v *DiagramView) DoubleClicked() (string, bool) { return v.nodeID(v.dbl) }

// Hovered returns the node under the pointer.
func (v *DiagramView) Hovered() (string, bool) { return v.nodeID(v.st.hoverNode) }

// EdgeClicked returns the index of the edge clicked this frame.
func (v *DiagramView) EdgeClicked() (int, bool) { return v.edgeClicked, v.edgeClicked >= 0 }

// HoveredEdge returns the index of the edge under the pointer.
func (v *DiagramView) HoveredEdge() (int, bool) { return v.st.hoverEdge, v.st.hoverEdge >= 0 }

// Moved reports whether nodes were dragged or moved by keys this frame.
func (v *DiagramView) Moved() bool { return v.moved }

// DeleteRequested reports Delete / Backspace with a selection this frame;
// removing the selected nodes and edges is up to the application.
func (v *DiagramView) DeleteRequested() bool { return v.del }

// ContextMenu reports a secondary click this frame: the node under it
// (empty for the background) and the window position.
func (v *DiagramView) ContextMenu() (node string, pos geom.Point, ok bool) {
	id, _ := v.nodeID(v.ctxNode)
	return id, v.ctxPos, v.ctx
}

// Zoom returns the zoom factor (screen pixels per graph unit).
func (v *DiagramView) Zoom() float32 { return v.st.zoom }

// ScreenRect returns where node id was drawn in the last frame (window
// coordinates), for anchoring popovers and menus.
func (v *DiagramView) ScreenRect(id string) (geom.Rect, bool) {
	n := v.g.Node(id)
	if n == nil {
		return geom.Rect{}, false
	}
	return v.st.xform(v.Rect()).rect(n.Rect()), true
}

// FitView fits the whole graph into the canvas.
func (v *DiagramView) FitView() { v.st.fitReq = true; v.c.Invalidate() }

// ZoomTo sets the zoom factor about the canvas centre.
func (v *DiagramView) ZoomTo(k float32) {
	r := v.Rect()
	v.st.zoomAbout(r, r.Center(), k, v.w.minZ, v.w.maxZ)
	v.c.Invalidate()
}

// CenterOn pans so node id is in the middle of the canvas.
func (v *DiagramView) CenterOn(id string) {
	n := v.g.Node(id)
	if n == nil {
		return
	}
	r := v.Rect()
	ctr := n.Center()
	v.st.pan = geom.Pt(r.W/2-ctr.X*v.st.zoom, r.H/2-ctr.Y*v.st.zoom)
	v.st.touched = true
	v.c.Invalidate()
}

// Select replaces the node selection.
func (v *DiagramView) Select(ids ...string) {
	clear(v.st.sel)
	clear(v.st.selEdges)
	for _, id := range ids {
		v.st.sel[id] = true
	}
	v.w.syncSelOut(v.st)
	v.c.Invalidate()
}

// statusColor returns the accent of a status string (zero: none).
func statusColor(th *theme.Theme, status string) theme.Color {
	if status == "" {
		return theme.Color{}
	}
	return th.ColorOf(status).Base
}
