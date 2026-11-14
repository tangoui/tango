package graph

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// minTextPx is the smallest scaled font size still drawn as text; below
// it labels become placeholder bars (level of detail for zoomed-out views).
const minTextPx = 5

// scaledGlyphs draws lay (shaped at zoom 1) scaled by k at origin.
func scaledGlyphs(l *paint.List, lay *text.Layout, origin geom.Point, k float32, col theme.Color) {
	if len(lay.Glyphs) == 0 {
		return
	}
	if k == 1 {
		l.Glyphs(origin, lay.Glyphs, col)
		return
	}
	gs := make([]paint.Glyph, len(lay.Glyphs))
	for i, g := range lay.Glyphs {
		g.X *= k
		g.Y *= k
		g.Size *= k
		gs[i] = g
	}
	l.Glyphs(origin, gs, col)
}

// textBox draws s shaped at size (graph units) inside the screen
// rectangle r, vertically centred, start-aligned (or centred), cut with an
// ellipsis to r's width. Zoomed far out it draws a bar instead.
func textBox(l *paint.List, s string, st text.Style, r geom.Rect, k float32, col theme.Color, center bool) {
	if s == "" || r.W <= 0 {
		return
	}
	if st.Size*k < minTextPx {
		w := min(r.W, text.Width(s, st)*k)
		h := max(1, st.Size*k*0.5)
		x := r.X
		if center {
			x = r.X + (r.W-w)/2
		}
		l.Fill(geom.Rect{X: x, Y: r.Y + (r.H-h)/2, W: w, H: h}, geom.Rad(h/2), theme.Fade(col, 0.35))
		return
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true, MaxWidth: r.W / k, Ellipsis: true})
	x := r.X
	if center {
		x = r.X + (r.W-lay.Width*k)/2
	}
	scaledGlyphs(l, lay, geom.Pt(x, r.Y+(r.H-lay.Height*k)/2), k, col)
}

// paint draws the whole canvas: background, grid, edges, nodes, the
// marquee and the minimap. Only the parts inside the canvas are drawn.
func (w *DiagramW) paint(l *paint.List, r geom.Rect, st *diagState, sty *Style, th *theme.Theme) {
	m := st.xform(r)
	l.Fill(r, geom.Rad(th.Radius), sty.Canvas)
	l.Stroke(r, geom.Rad(th.Radius), 1, th.BorderSecondary)
	l.Clip(r, geom.Rad(th.Radius))
	if w.grid > 0 {
		paintGrid(l, &st.grid, r, m, w.grid, sty.GridDot)
	}
	vis := m.invRect(r).Expand(4 / m.K)
	if m.K < sketchZoom {
		w.paintSketch(l, m, vis, st, sty)
	} else {
		w.paintFull(l, m, vis, st, sty, th)
	}
	if st.mode == modeMarquee && (st.marquee.W > 0.01 || st.marquee.H > 0.01) {
		l.Fill(st.marquee, geom.Radii{}, sty.Marquee)
		l.Stroke(st.marquee, geom.Radii{}, 1, sty.MarqueeBorder)
	}
	if w.mini {
		w.paintMini(l, r, st, sty, th)
	}
	l.Pop()
}

// sketchZoom is the zoom below which the canvas draws the sketch level of
// detail: nodes as plain blocks and edges as thin centre-to-centre lines,
// each batched into one path (text would be unreadable anyway).
const sketchZoom = 0.3

func (w *DiagramW) paintSketch(l *paint.List, m xform, vis geom.Rect, st *diagState, sty *Style) {
	g := w.g
	edges, hot := &paint.Path{}, &paint.Path{}
	ne, nh := 0, 0
	for i := range g.Edges {
		e := &g.Edges[i]
		a, b := g.Node(e.From), g.Node(e.To)
		if a == nil || b == nil || a == b {
			continue
		}
		if !overlapsRect(edgeReach(e, a, b), vis) {
			continue
		}
		p := edges
		if st.selEdges[g.edgeID(i)] || st.sel[e.From] || st.sel[e.To] {
			p, nh = hot, nh+1
		} else {
			ne++
		}
		q := m.pt(a.Center())
		p.MoveTo(q.X, q.Y)
		for _, bp := range e.Bends {
			q = m.pt(bp)
			p.LineTo(q.X, q.Y)
		}
		q = m.pt(b.Center())
		p.LineTo(q.X, q.Y)
	}
	if ne > 0 {
		l.StrokePath(edges, paint.StrokeStyle{Width: 1}, sty.Edge)
	}
	if nh > 0 {
		l.StrokePath(hot, paint.StrokeStyle{Width: 1.5}, sty.Selection)
	}
	nodes, sel := &paint.Path{}, &paint.Path{}
	nn, ns := 0, 0
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if !overlapsRect(n.Rect(), vis) {
			continue
		}
		sr := m.rect(n.Rect())
		sr.W, sr.H = max(sr.W, 1), max(sr.H, 1)
		if st.sel[n.ID] {
			rectPath(sel, sr)
			ns++
		} else {
			rectPath(nodes, sr)
			nn++
		}
	}
	if nn > 0 {
		l.Path(nodes, sty.NodeBorder)
	}
	if ns > 0 {
		l.Path(sel, sty.Selection)
	}
}

func (w *DiagramW) paintFull(l *paint.List, m xform, vis geom.Rect, st *diagState, sty *Style, th *theme.Theme) {
	g := w.g

	// Edges below nodes.
	var labels []edgeLabel
	var buf []geom.Point
	for i := range g.Edges {
		e := &g.Edges[i]
		a, b := g.Node(e.From), g.Node(e.To)
		if a == nil || b == nil {
			continue
		}
		if !overlapsRect(edgeReach(e, a, b), vis) {
			continue
		}
		cv := route(e, a, b, w.edgeKind, w.pref())
		if !overlapsRect(cv.bounds().Expand(8/m.K), vis) {
			continue
		}
		sel := st.selEdges[g.edgeID(i)]
		hov := st.hoverEdge == i
		col := sty.Edge
		if c := statusColor(th, e.Status); c.A > 0 {
			col = c
		}
		width := sty.EdgeWidth
		if sel || hov {
			col = sty.Selection
			if hov && !sel {
				col = sty.Hover
			}
			width += 1
		}
		// Edges touching a selected node are highlighted too.
		if !sel && (st.sel[e.From] || st.sel[e.To]) {
			col = sty.Selection
		}
		as := max(4, min(10, 9*m.K))
		trimS, trimE := float32(0), float32(0)
		if e.Arrow.end() {
			trimE = as * 0.8
		}
		if e.Arrow.start() {
			trimS = as * 0.8
		}
		ss := paint.StrokeStyle{Width: width}
		if e.Dashed {
			ss.Dash = []float32{5, 4}
		}
		l.StrokePath(cv.path(m, trimS, trimE), ss, col)
		if e.Arrow.end() {
			arrowHead(l, m.pt(cv.end()), cv.endDir(), as, col)
		}
		if e.Arrow.start() {
			arrowHead(l, m.pt(cv.Start), scale(cv.startDir(), -1), as, col)
		}
		if e.Label != "" {
			buf = cv.flatten(buf[:0])
			labels = append(labels, edgeLabel{e.Label, m.pt(polyMid(buf)), col, sel || hov})
		}
	}
	for _, lb := range labels {
		paintEdgeLabel(l, lb, m.K, sty, th)
	}

	// Nodes.
	for i := range g.Nodes {
		n := &g.Nodes[i]
		nr := n.Rect()
		if !overlapsRect(nr, vis) {
			continue
		}
		sr := m.rect(nr)
		ns := NodeState{Zoom: m.K, Selected: st.sel[n.ID], Hovered: st.hoverNode == i, Theme: th, Style: sty}
		if n.Draw != nil {
			n.Draw(l, sr, ns)
			if ns.Selected {
				l.Stroke(sr.Expand(3), geom.Rad(sty.NodeRadius*m.K+3), 2, sty.Selection)
			}
			continue
		}
		w.paintNode(l, n, sr, ns)
	}
}

type edgeLabel struct {
	s   string
	at  geom.Point
	col theme.Color
	hot bool
}

func paintEdgeLabel(l *paint.List, lb edgeLabel, k float32, sty *Style, th *theme.Theme) {
	ts := text.Style{Size: sty.FontSM}
	if ts.Size*k < minTextPx {
		return
	}
	lay := text.Shape(lb.s, ts, text.Options{NoWrap: true, MaxWidth: 200, Ellipsis: true})
	w, h := lay.Width*k+12, lay.Height*k+2
	r := geom.Rect{X: lb.at.X - w/2, Y: lb.at.Y - h/2, W: w, H: h}
	l.Fill(r, geom.Rad(h/2), sty.LabelBg)
	bc := th.BorderSecondary
	if lb.hot {
		bc = lb.col
	}
	l.Stroke(r, geom.Rad(h/2), 1, bc)
	col := sty.LabelText
	if lb.hot {
		col = lb.col
	}
	scaledGlyphs(l, lay, geom.Pt(r.X+6, r.Y+1), k, col)
}

// arrowHead draws a filled arrowhead with its tip at tip pointing along dir.
func arrowHead(l *paint.List, tip, dir geom.Point, size float32, col theme.Color) {
	n := geom.Pt(-dir.Y, dir.X)
	base := sub(tip, scale(dir, size))
	a := add(base, scale(n, size*0.5))
	b := sub(base, scale(n, size*0.5))
	p := paint.NewPath().MoveTo(tip.X, tip.Y).LineTo(a.X, a.Y).LineTo(b.X, b.Y).Close()
	l.Path(p, col)
}

// rectPath appends a closed rectangle (no rounded-corner handling).
func rectPath(p *paint.Path, r geom.Rect) {
	p.MoveTo(r.X, r.Y).LineTo(r.Right(), r.Y).LineTo(r.Right(), r.Bottom()).LineTo(r.X, r.Bottom()).Close()
}

// gridCache keeps the dot pattern of one step and canvas size; panning
// only translates it.
type gridCache struct {
	step, w, h float32
	path       *paint.Path
	dots       int
}

// paintGrid draws one dot per grid step (coarser when zoomed out).
func paintGrid(l *paint.List, gc *gridCache, r geom.Rect, m xform, step float32, col theme.Color) {
	s := step * m.K
	for s < 10 {
		s *= 2
	}
	if gc.path == nil || gc.step != s || gc.w != r.W || gc.h != r.H {
		d := max(1, min(2, 1.5*m.K))
		p := &paint.Path{Segs: make([]paint.Seg, 0, int((r.W/s+2)*(r.H/s+2))*5)}
		n := 0
		for y := float32(0); y <= r.H+s; y += s {
			for x := float32(0); x <= r.W+s; x += s {
				rectPath(p, geom.Rect{X: x - d/2, Y: y - d/2, W: d, H: d})
				n++
			}
		}
		*gc = gridCache{step: s, w: r.W, h: r.H, path: p, dots: n}
	}
	if gc.dots == 0 {
		return
	}
	x0 := m.Origin.X + float32(math.Floor(float64((r.X-m.Origin.X)/s)))*s
	y0 := m.Origin.Y + float32(math.Floor(float64((r.Y-m.Origin.Y)/s)))*s
	l.Transform(geom.Translate(x0, y0))
	l.Path(gc.path, col)
	l.Pop()
}

// shapePath returns the outline of a non-rectangular node shape.
func shapePath(s Shape, r geom.Rect) *paint.Path {
	p := paint.NewPath()
	switch s {
	case ShapeDiamond:
		c := r.Center()
		p.MoveTo(c.X, r.Y).LineTo(r.Right(), c.Y).LineTo(c.X, r.Bottom()).LineTo(r.X, c.Y).Close()
	default: // ellipse: four cubic quarter arcs
		const kappa = 0.5523
		c := r.Center()
		rx, ry := r.W/2, r.H/2
		ox, oy := rx*kappa, ry*kappa
		p.MoveTo(c.X+rx, c.Y)
		p.CubeTo(c.X+rx, c.Y+oy, c.X+ox, c.Y+ry, c.X, c.Y+ry)
		p.CubeTo(c.X-ox, c.Y+ry, c.X-rx, c.Y+oy, c.X-rx, c.Y)
		p.CubeTo(c.X-rx, c.Y-oy, c.X-ox, c.Y-ry, c.X, c.Y-ry)
		p.CubeTo(c.X+ox, c.Y-ry, c.X+rx, c.Y-oy, c.X+rx, c.Y)
		p.Close()
	}
	return p
}

// paintNode draws the built-in node template in screen rectangle sr.
func (w *DiagramW) paintNode(l *paint.List, n *Node, sr geom.Rect, s NodeState) {
	sty, th, k := s.Style, s.Theme, s.Zoom
	border, bw := sty.NodeBorder, float32(1)
	if s.Hovered {
		border = sty.Hover
	}
	if s.Selected {
		border, bw = sty.Selection, 2
	}
	accent := statusColor(th, n.Status)
	rad := sty.NodeRadius * k
	switch n.Shape {
	case ShapeEllipse, ShapeDiamond:
		p := shapePath(n.Shape, sr)
		l.Path(p, sty.NodeBg)
		if accent.A > 0 {
			l.Path(p, theme.Fade(accent, 0.12))
		}
		l.StrokePath(p, paint.StrokeStyle{Width: bw}, border)
		inner := sr
		if n.Shape == ShapeDiamond {
			inner = geom.Rect{X: sr.X + sr.W/4, Y: sr.Y, W: sr.W / 2, H: sr.H}
		} else {
			inner = geom.Rect{X: sr.X + sr.W*0.15, Y: sr.Y, W: sr.W * 0.7, H: sr.H}
		}
		w.nodeText(l, n, inner, s, true)
		return
	case ShapePill:
		rad = sr.H / 2
	}
	if k >= 0.5 && len(sty.NodeShadow) > 0 {
		l.Shadows(sr, geom.Rad(rad), sty.NodeShadow)
	}
	l.Fill(sr, geom.Rad(rad), sty.NodeBg)
	if accent.A > 0 && n.Shape == ShapePill {
		// Status dot inside the rounded start.
		d := max(3, 8*k)
		l.Path(paint.NewPath().Circle(sr.X+sr.H/2, sr.Y+sr.H/2, d/2), accent)
	} else if accent.A > 0 {
		// Accent bar on the left (the canvas keeps its geometry in RTL).
		l.Clip(sr, geom.Rad(rad))
		l.Fill(geom.Rect{X: sr.X, Y: sr.Y, W: max(2, 4*k), H: sr.H}, geom.Radii{}, accent)
		l.Pop()
	}
	l.Stroke(sr, geom.Rad(rad), bw, border)
	pad := 16 * k
	if n.Shape == ShapePill {
		pad = sr.H / 2
		if accent.A > 0 {
			pad += 6 * k
		}
	}
	inner := geom.Rect{X: sr.X + pad, Y: sr.Y, W: sr.W - 2*pad, H: sr.H}
	w.nodeText(l, n, inner, s, n.Icon == nil && n.Detail == "")
}

// nodeText draws the icon, label and detail of the built-in template.
func (w *DiagramW) nodeText(l *paint.List, n *Node, r geom.Rect, s NodeState, center bool) {
	sty, k := s.Style, s.Zoom
	if n.Icon != nil {
		is := 16 * k
		col := sty.NodeDetail
		if c := statusColor(s.Theme, n.Status); c.A > 0 {
			col = c
		}
		if is >= 4 {
			w.c.PaintIcon(l, n.Icon, geom.Rect{X: r.X, Y: r.Y + (r.H-is)/2, W: is, H: is}, col, 0)
		}
		r.X += 24 * k
		r.W -= 24 * k
	}
	ls := text.Style{Size: sty.Font, Weight: text.Medium}
	if n.Detail == "" {
		textBox(l, n.Label, ls, r, k, sty.NodeText, center)
		return
	}
	half := r.H / 2
	textBox(l, n.Label, ls, geom.Rect{X: r.X, Y: r.Y + 6*k, W: r.W, H: half - 6*k}, k, sty.NodeText, center)
	textBox(l, n.Detail, text.Style{Size: sty.FontSM}, geom.Rect{X: r.X, Y: r.Y + half, W: r.W, H: half - 6*k}, k, sty.NodeDetail, center)
}

// paintMini draws the minimap: every node as a small block (one path) and
// the visible area.
func (w *DiagramW) paintMini(l *paint.List, r geom.Rect, st *diagState, sty *Style, th *theme.Theme) {
	mr := w.miniRect(r)
	l.Shadows(mr, geom.Rad(th.Radius), sty.ToolbarShadow)
	l.Fill(mr, geom.Rad(th.Radius), sty.MiniBg)
	l.Stroke(mr, geom.Rad(th.Radius), 1, sty.MiniBorder)
	mm, ok := miniXform(w.g, st, r, mr)
	if !ok {
		return
	}
	l.Clip(mr, geom.Rad(th.Radius))
	mc := &st.mini
	if key := miniKey(w.g, st.sel); mc.nodes == nil || mc.key != key {
		*mc = miniCache{key: key, nodes: &paint.Path{}, sel: &paint.Path{}}
		for i := range w.g.Nodes {
			n := &w.g.Nodes[i]
			if st.sel[n.ID] {
				rectPath(mc.sel, n.Rect())
				mc.ns++
			} else {
				rectPath(mc.nodes, n.Rect())
				mc.nn++
			}
		}
	}
	// The cached outlines are in graph units: map them into the minimap.
	l.Transform(geom.Affine{A: mm.K, D: mm.K, E: mm.Origin.X, F: mm.Origin.Y})
	if mc.nn > 0 {
		l.Path(mc.nodes, sty.MiniNode)
	}
	if mc.ns > 0 {
		l.Path(mc.sel, sty.Selection)
	}
	l.Pop()
	vr := mm.rect(st.xform(r).invRect(r))
	l.Fill(vr, geom.Rad(2), sty.MiniView)
	l.Stroke(vr, geom.Rad(2), 1, sty.MiniViewBorder)
	l.Pop()
}

// miniCache keeps the minimap outlines (graph units) until a node moves,
// appears or changes selection.
type miniCache struct {
	key        uint64
	nodes, sel *paint.Path
	nn, ns     int
}

// miniKey fingerprints node boxes and the selection.
func miniKey(g *Graph, sel map[string]bool) uint64 {
	h := uint64(14695981039346656037)
	mix := func(v uint64) { h = (h ^ v) * 1099511628211 }
	mix(uint64(len(g.Nodes)))
	for i := range g.Nodes {
		r := g.Nodes[i].Rect()
		mix(uint64(math.Float32bits(r.X)) | uint64(math.Float32bits(r.Y))<<32)
		mix(uint64(math.Float32bits(r.W)) | uint64(math.Float32bits(r.H))<<32)
		if sel[g.Nodes[i].ID] {
			mix(uint64(i) + 1)
		}
	}
	return h
}
