package graph

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// diagramHarness renders one diagram filling an 800×600 window.
type diagramHarness struct {
	d   *ui.Driver
	g   *Graph
	v   *DiagramView
	cfg func(*DiagramW) *DiagramW

	// Events seen during the frames (the app reacts while building).
	clicked, dbl string
	edge         int
	del          bool
}

func newHarness(t *testing.T, g *Graph, cfg func(*DiagramW) *DiagramW) *diagramHarness {
	t.Helper()
	h := &diagramHarness{g: g, cfg: cfg, edge: -1}
	h.d = ui.NewDriver(func(c *ui.Context) {
		w := Diagram(c, h.g).Height(600)
		if h.cfg != nil {
			w = h.cfg(w)
		}
		h.v = w.Show()
		if id, ok := h.v.Clicked(); ok {
			h.clicked = id
		}
		if id, ok := h.v.DoubleClicked(); ok {
			h.dbl = id
		}
		if i, ok := h.v.EdgeClicked(); ok {
			h.edge = i
		}
		if h.v.DeleteRequested() {
			h.del = true
		}
	}, ui.DriverOptions{Width: 800, Height: 600})
	h.d.Settle()
	h.d.Settle()
	return h
}

func (h *diagramHarness) center(t *testing.T, id string) geom.Point {
	t.Helper()
	r, ok := h.v.ScreenRect(id)
	if !ok {
		t.Fatalf("node %s not found", id)
	}
	return r.Center()
}

// drag presses at from with mods, moves to to in steps and releases.
func (h *diagramHarness) drag(from, to geom.Point, mods input.Modifiers) {
	d := h.d
	d.Inject(input.Pointer{Kind: input.Move, Pos: from, Mods: mods})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from, Mods: mods})
	d.Frame()
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		d.Inject(input.Pointer{Kind: input.Move, Pos: lerp(from, to, f), Mods: mods})
		d.Frame()
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: to, Mods: mods})
	d.Settle()
}

func (h *diagramHarness) click(p geom.Point, mods input.Modifiers) {
	d := h.d
	d.Inject(input.Pointer{Kind: input.Move, Pos: p, Mods: mods})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Mods: mods})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p, Mods: mods})
	d.Settle()
}

func threeNodes() *Graph {
	g := mkGraph([]string{"a", "b", "c"}, [2]string{"a", "b"}, [2]string{"b", "c"})
	g.Nodes[0].Pos = geom.Pt(0, 0)
	g.Nodes[1].Pos = geom.Pt(200, 0)
	g.Nodes[2].Pos = geom.Pt(400, 0)
	return g
}

func TestDiagramFitsFirstFrame(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	box := h.v.Rect()
	for _, id := range []string{"a", "b", "c"} {
		r, _ := h.v.ScreenRect(id)
		if r.X < box.X || r.Right() > box.Right() || r.Y < box.Y || r.Bottom() > box.Bottom() {
			t.Fatalf("%s at %v outside canvas %v", id, r, box)
		}
	}
	if z := h.v.Zoom(); z != 1 {
		t.Fatalf("a graph that fits keeps zoom 1, got %v", z)
	}
}

func TestDiagramClickSelects(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	h.click(h.center(t, "a"), 0)
	if s := h.v.Selected(); len(s) != 1 || s[0] != "a" {
		t.Fatalf("selected = %v, want [a]", s)
	}
	h.click(h.center(t, "c"), input.ModShift)
	if s := h.v.Selected(); len(s) != 2 {
		t.Fatalf("shift-click should add: %v", s)
	}
	// Plain click on the background clears.
	r := h.v.Rect()
	h.click(geom.Pt(r.X+20, r.Bottom()-200), 0)
	if s := h.v.Selected(); len(s) != 0 {
		t.Fatalf("background click should clear: %v", s)
	}
}

func TestDiagramClickedAndDoubleClicked(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	p := h.center(t, "b")
	h.click(p, 0)
	d := h.d
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 2})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p, Clicks: 2})
	d.Settle()
	if h.clicked != "b" || h.dbl != "b" {
		t.Fatalf("clicked %q, double-clicked %q", h.clicked, h.dbl)
	}
}

func TestDiagramDragNodeSnaps(t *testing.T) {
	h := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.Grid(20).Snap() })
	k := h.v.Zoom()
	from := h.center(t, "b")
	h.drag(from, add(from, geom.Pt(53*k, 31*k)), 0)
	p := h.g.Node("b").Pos
	if p.X != 260 || p.Y != 40 {
		t.Fatalf("dragged to %v, want snapped (260, 40)", p)
	}
	if h.g.Node("a").Pos != (geom.Point{}) {
		t.Fatalf("unselected node moved")
	}
}

func TestDiagramDragMovesSelection(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	h.click(h.center(t, "a"), 0)
	h.click(h.center(t, "b"), input.ModShift)
	from := h.center(t, "a")
	k := h.v.Zoom()
	h.drag(from, add(from, geom.Pt(0, 50*k)), 0)
	if h.g.Node("a").Pos.Y != 50 || h.g.Node("b").Pos.Y != 50 || h.g.Node("c").Pos.Y != 0 {
		t.Fatalf("selection not moved together: %v %v %v", h.g.Node("a").Pos, h.g.Node("b").Pos, h.g.Node("c").Pos)
	}
}

func TestDiagramReadOnlyAndFixed(t *testing.T) {
	g := threeNodes()
	g.Nodes[2].Fixed = true
	h := newHarness(t, g, nil)
	from := h.center(t, "c")
	h.drag(from, add(from, geom.Pt(40, 40)), 0)
	if g.Node("c").Pos != geom.Pt(400, 0) {
		t.Fatalf("fixed node moved")
	}
	h2 := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.ReadOnly() })
	from = h2.center(t, "a")
	h2.drag(from, add(from, geom.Pt(40, 40)), 0)
	if h2.g.Node("a").Pos != (geom.Point{}) {
		t.Fatalf("read-only diagram moved a node")
	}
}

func TestDiagramPanByDragAndWheel(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	before := h.center(t, "a")
	r := h.v.Rect()
	from := geom.Pt(r.X+300, r.Bottom()-150)
	h.drag(from, add(from, geom.Pt(-60, 25)), 0)
	after := h.center(t, "a")
	if math.Abs(float64(after.X-before.X+60)) > 0.5 || math.Abs(float64(after.Y-before.Y-25)) > 0.5 {
		t.Fatalf("pan moved %v -> %v, want (-60, +25)", before, after)
	}
	if h.g.Node("a").Pos != (geom.Point{}) {
		t.Fatalf("panning changed graph positions")
	}
	h.d.Scroll(from, 40)
	if got := h.center(t, "a"); math.Abs(float64(got.Y-(after.Y-40))) > 0.5 {
		t.Fatalf("wheel pan: %v -> %v", after, got)
	}
}

func TestDiagramCtrlWheelZoomsAboutPointer(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	p := h.center(t, "b")
	z0 := h.v.Zoom()
	h.d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	h.d.Inject(input.Scroll{Pos: p, DY: -40, Mods: input.ModCtrl})
	h.d.Settle()
	if z := h.v.Zoom(); !(z > z0) {
		t.Fatalf("ctrl+wheel up should zoom in: %v -> %v", z0, z)
	}
	if got := h.center(t, "b"); math.Abs(float64(got.X-p.X)) > 0.5 || math.Abs(float64(got.Y-p.Y)) > 0.5 {
		t.Fatalf("point under the pointer moved: %v -> %v", p, got)
	}
}

func TestDiagramMarqueeSelects(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	ra, _ := h.v.ScreenRect("a")
	rb, _ := h.v.ScreenRect("b")
	from := geom.Pt(ra.X-10, ra.Y-10)
	to := geom.Pt(rb.Right()-10, rb.Bottom()+10)
	h.drag(from, to, input.ModShift)
	if s := h.v.Selected(); len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Fatalf("marquee selected %v, want [a b]", s)
	}
	// MarqueeDrag: plain background drag selects.
	h2 := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.MarqueeDrag() })
	rc, _ := h2.v.ScreenRect("c")
	h2.drag(geom.Pt(rc.X-10, rc.Y-10), geom.Pt(rc.Right()+5, rc.Bottom()+5), 0)
	if s := h2.v.Selected(); len(s) != 1 || s[0] != "c" {
		t.Fatalf("marquee drag selected %v, want [c]", s)
	}
}

func TestDiagramKeyboard(t *testing.T) {
	var sel []string
	h := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.Selection(&sel) })
	h.click(h.center(t, "a"), 0)
	if len(sel) != 1 || sel[0] != "a" {
		t.Fatalf("bound selection = %v", sel)
	}
	h.d.Key(input.KeyRight, 0)
	if p := h.g.Node("a").Pos; p.X != 8 {
		t.Fatalf("arrow moved a to %v, want x=8", p)
	}
	h.d.Key(input.KeyDown, input.ModShift)
	if p := h.g.Node("a").Pos; p.Y != 40 {
		t.Fatalf("shift+arrow moved a to %v, want y=40", p)
	}
	h.d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	if len(h.v.Selected()) != 3 || len(sel) != 3 {
		t.Fatalf("select all: %v / %v", h.v.Selected(), sel)
	}
	h.d.Key(input.KeyDelete, 0)
	if !h.del {
		t.Fatalf("Delete with a selection should request removal")
	}
	h.d.Key(input.KeyEscape, 0)
	if len(h.v.Selected()) != 0 {
		t.Fatalf("Esc should clear the selection")
	}
	z := h.v.Zoom()
	h.d.Key(input.KeyMinus, 0)
	if !(h.v.Zoom() < z) {
		t.Fatalf("minus should zoom out")
	}
	h.d.Key(input.Key0, 0)
	h.d.Settle()
	if h.v.Zoom() != z {
		t.Fatalf("0 should fit again: %v, want %v", h.v.Zoom(), z)
	}
	// Bound selection set by the app is picked up.
	sel = []string{"c"}
	h.d.Settle()
	if s := h.v.Selected(); len(s) != 1 || s[0] != "c" {
		t.Fatalf("app selection not read: %v", s)
	}
}

func TestDiagramToolbarZoom(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	z := h.v.Zoom()
	if err := h.d.ClickLabel("放大"); err != nil {
		t.Fatal(err)
	}
	if !(h.v.Zoom() > z) {
		t.Fatalf("zoom-in button: %v -> %v", z, h.v.Zoom())
	}
	if err := h.d.ClickLabel("适应视图"); err != nil {
		t.Fatal(err)
	}
	h.d.Settle()
	if h.v.Zoom() != z {
		t.Fatalf("fit button: zoom %v, want %v", h.v.Zoom(), z)
	}
}

func TestDiagramPinchZooms(t *testing.T) {
	h := newHarness(t, threeNodes(), nil)
	z := h.v.Zoom()
	h.d.Pinch(h.center(t, "b"), 100, 200)
	if !(h.v.Zoom() > z*1.5) {
		t.Fatalf("pinch out should zoom in about 2x: %v -> %v", z, h.v.Zoom())
	}
	if h.g.Node("b").Pos != geom.Pt(200, 0) {
		t.Fatalf("pinch moved a node")
	}
}

func TestDiagramEdgeClick(t *testing.T) {
	h := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.Edges(EdgeStraight) })
	ra, _ := h.v.ScreenRect("a")
	rb, _ := h.v.ScreenRect("b")
	mid := geom.Pt((ra.Right()+rb.X)/2, ra.Center().Y)
	h.click(mid, 0)
	if h.edge != 0 {
		t.Fatalf("edge click = %d", h.edge)
	}
	if e := h.v.SelectedEdges(); len(e) != 1 || e[0] != 0 {
		t.Fatalf("selected edges = %v", e)
	}
}

func TestDiagramMiniMapPans(t *testing.T) {
	g := threeNodes()
	g.Nodes = append(g.Nodes, Node{ID: "far", Label: "far", Pos: geom.Pt(4000, 3000), Size: geom.Size{W: 100, H: 40}})
	h := newHarness(t, g, func(w *DiagramW) *DiagramW { return w.MiniMap().ZoomRange(1, 4) })
	r := h.v.Rect()
	if _, ok := h.v.ScreenRect("far"); !ok {
		t.Fatal("far missing")
	}
	// Press the bottom-right of the minimap: the far node comes into view.
	mr := geom.Rect{X: r.Right() - 212, Y: r.Bottom() - 152, W: 200, H: 140}
	h.click(geom.Pt(mr.Right()-14, mr.Bottom()-14), 0)
	fr, _ := h.v.ScreenRect("far")
	if fr.Right() < r.X || fr.X > r.Right() || fr.Bottom() < r.Y || fr.Y > r.Bottom() {
		t.Fatalf("far node %v not in view %v after minimap click", fr, r)
	}
}

func TestDiagramAutoLayoutOnStructureChange(t *testing.T) {
	g := mkGraph([]string{"a", "b"}, [2]string{"a", "b"})
	h := newHarness(t, g, func(w *DiagramW) *DiagramW { return w.AutoLayout(LayoutOptions{}) })
	if !(g.Node("a").Pos.Y < g.Node("b").Pos.Y) {
		t.Fatalf("not laid out: %v %v", g.Node("a").Pos, g.Node("b").Pos)
	}
	// Moving a node does not trigger a relayout; adding one does.
	from := h.center(t, "a")
	h.drag(from, add(from, geom.Pt(30, 0)), 0)
	moved := g.Node("a").Pos
	h.d.Settle()
	if g.Node("a").Pos != moved {
		t.Fatalf("relaid out without a structure change")
	}
	g.Nodes = append(g.Nodes, Node{ID: "c", Label: "c"})
	g.Edges = append(g.Edges, Edge{From: "b", To: "c"})
	h.d.Settle()
	if c := g.Node("c"); c.Size.W == 0 || !(c.Pos.Y > g.Node("b").Pos.Y) {
		t.Fatalf("new node not laid out: %+v", c)
	}
}

func TestDiagramAccessibility(t *testing.T) {
	h := newHarness(t, threeNodes(), func(w *DiagramW) *DiagramW { return w.MiniMap().AutoLayout(LayoutOptions{}) })
	h.d.EnableAccessibility()
	h.d.Settle()
	n, ok := h.d.A11yFind(a11y.RoleGroup, "关系图")
	if !ok {
		t.Fatal("canvas not exposed")
	}
	if n.Description != "3 个节点，2 条连线" {
		t.Fatalf("description = %q", n.Description)
	}
	for _, name := range []string{"放大", "缩小", "适应视图", "自动布局"} {
		if _, ok := h.d.A11yFind(a11y.RoleButton, name); !ok {
			t.Fatalf("toolbar button %q missing", name)
		}
	}
	rep := h.d.Audit()
	for _, f := range rep.Issues {
		if f.Kind == ui.AuditMissingName || f.Kind == ui.AuditNotKeyboard || f.Kind == ui.AuditNoRole {
			t.Errorf("audit: %+v", f)
		}
	}
}

func TestDiagramLocale(t *testing.T) {
	d := ui.NewDriver(func(c *ui.Context) {
		ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() {
			Diagram(c, threeNodes()).AutoLayout(LayoutOptions{}).Show()
		})
	}, ui.DriverOptions{Width: 800, Height: 600, Accessibility: true})
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleGroup, "Diagram"); !ok {
		t.Fatal("English canvas name missing")
	}
	for _, name := range []string{"Zoom in", "Zoom out", "Fit view", "Auto layout"} {
		if _, ok := d.A11yFind(a11y.RoleButton, name); !ok {
			t.Fatalf("English button %q missing", name)
		}
	}
}

func TestRouteEnds(t *testing.T) {
	a := &Node{ID: "a", Pos: geom.Pt(0, 0), Size: geom.Size{W: 100, H: 40}}
	b := &Node{ID: "b", Pos: geom.Pt(300, 100), Size: geom.Size{W: 100, H: 40}}
	e := &Edge{From: "a", To: "b"}
	for _, k := range []EdgeKind{EdgeBezier, EdgeStraight, EdgeOrthogonal} {
		cv := route(e, a, b, k, SideAuto)
		if !a.Rect().Expand(0.5).Contains(cv.Start) || !b.Rect().Expand(0.5).Contains(cv.end()) {
			t.Fatalf("kind %d: ends %v %v not on the nodes", k, cv.Start, cv.end())
		}
	}
	// Orthogonal routes only turn at right angles.
	cv := route(e, a, b, EdgeOrthogonal, SideHorizontal)
	pts := cv.flatten(nil)
	if pts[0] != geom.Pt(100, 20) || pts[len(pts)-1] != geom.Pt(300, 120) {
		t.Fatalf("orthogonal horizontal ends = %v .. %v", pts[0], pts[len(pts)-1])
	}
	for _, s := range cv.Segs {
		if s.Line {
			continue
		}
	}
	// Ports pin the end to a row.
	b.Ports = []Port{{ID: "id", Side: SideHorizontal, Offset: 30}}
	e.ToPort = "id"
	cv = route(e, a, b, EdgeBezier, SideAuto)
	if end := cv.end(); end != geom.Pt(300, 130) {
		t.Fatalf("port end = %v, want (300,130)", end)
	}
	// Self loops leave and re-enter the right side.
	cv = route(&Edge{From: "a", To: "a"}, a, a, EdgeBezier, SideAuto)
	if cv.Start.X != 100 || cv.end().X != 100 {
		t.Fatalf("self loop = %v .. %v", cv.Start, cv.end())
	}
	// Straight edges stop at the outline of round shapes.
	b.Shape = ShapeEllipse
	b.Ports = nil
	e.ToPort = ""
	cv = route(e, a, b, EdgeStraight, SideAuto)
	c := b.Center()
	d := sub(cv.end(), c)
	if v := d.X*d.X/(50*50) + d.Y*d.Y/(20*20); math.Abs(float64(v-1)) > 0.01 {
		t.Fatalf("ellipse end not on outline: %v", v)
	}
}
