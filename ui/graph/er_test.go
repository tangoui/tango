package graph

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

func TestERBuildsTablesAndForeignKeys(t *testing.T) {
	tables := []Table{
		{Name: "users", Columns: []Column{{Name: "id", Type: "bigint", PK: true}, {Name: "name", Type: "text"}}},
		{Name: "orders", Comment: "订单", Columns: []Column{{Name: "id", Type: "bigint", PK: true}, {Name: "user_id", Type: "bigint"}}},
		{Name: "tree", Columns: []Column{{Name: "id", Type: "int", PK: true}, {Name: "parent_id", Type: "int"}}},
	}
	rels := []Relation{
		{From: "orders", FromColumn: "user_id", To: "users", ToColumn: "id"},
		{From: "tree", FromColumn: "parent_id", To: "tree", ToColumn: "id"},
	}
	var v *DiagramView
	d := ui.NewDriver(func(c *ui.Context) {
		v = ER(c, tables, rels).Height(500).Show()
	}, ui.DriverOptions{Width: 900, Height: 520, Accessibility: true})
	d.Settle()
	g := v.g
	if len(g.Nodes) != 3 || len(g.Edges) != 2 {
		t.Fatalf("graph = %d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
	orders := g.Node("orders")
	tp := orders.Data.(*Table)
	if tp.Name != "orders" {
		t.Fatalf("Data = %+v", tp)
	}
	// Header, comment line and two rows.
	if want := float32(erHeader + erComment + 2*erRow + erPadB); orders.Size.H != want {
		t.Fatalf("orders height = %v, want %v", orders.Size.H, want)
	}
	// Laid out left to right: the referencing table first.
	if !(orders.Pos.X < g.Node("users").Pos.X) {
		t.Fatalf("orders %v should be left of users %v", orders.Pos, g.Node("users").Pos)
	}
	// The FK edge leaves the user_id row and enters the id row.
	cv := route(&g.Edges[0], orders, g.Node("users"), EdgeOrthogonal, SideHorizontal)
	if want := orders.Pos.Y + erHeader + erComment + erRow + erRow/2; cv.Start.Y != want {
		t.Fatalf("edge starts at y %v, want the user_id row %v", cv.Start.Y, want)
	}
	if want := g.Node("users").Pos.Y + erHeader + erRow/2; cv.end().Y != want {
		t.Fatalf("edge ends at y %v, want the id row %v", cv.end().Y, want)
	}
	if _, ok := d.A11yFind(a11y.RoleGroup, "ER 图"); !ok {
		t.Fatal("ER canvas not exposed with its name")
	}
	// Dragged positions survive frames; a schema change relays out but
	// keeps the graph identity.
	from := v.st.xform(v.Rect()).rect(orders.Rect()).Center()
	d.Drag(from, geom.Pt(from.X, from.Y+40))
	moved := v.g.Node("orders").Pos
	d.Settle()
	if v.g.Node("orders").Pos != moved {
		t.Fatal("drag position lost")
	}
	tables = append(tables, Table{Name: "audit", Columns: []Column{{Name: "id", Type: "int", PK: true}}})
	d.Settle()
	if v.g.Node("audit") == nil {
		t.Fatal("new table missing")
	}
}

func TestERSelfReferenceDrawn(t *testing.T) {
	tables := []Table{{Name: "tree", Columns: []Column{{Name: "id", Type: "int", PK: true}, {Name: "parent_id", Type: "int"}}}}
	rels := []Relation{{From: "tree", FromColumn: "parent_id", To: "tree", ToColumn: "id"}}
	var v *DiagramView
	d := ui.NewDriver(func(c *ui.Context) { v = ER(c, tables, rels).Show() }, ui.DriverOptions{Width: 600, Height: 600})
	d.Settle()
	n := v.g.Node("tree")
	if tables[0].Columns[1].FK {
		t.Fatal("ER must not modify the caller's tables")
	}
	cv := route(&v.g.Edges[0], n, n, EdgeOrthogonal, SideHorizontal)
	if cv.Start.X != n.Rect().Right() || len(cv.Segs) < 3 {
		t.Fatalf("self loop = %+v", cv)
	}
}

// bigGraph returns n nodes on a grid with edges to the right and below.
func bigGraph(n int) *Graph {
	g := &Graph{}
	cols := 80
	for i := range n {
		g.Nodes = append(g.Nodes, Node{ID: fmt.Sprintf("n%d", i), Label: fmt.Sprintf("节点 %d", i),
			Pos: geom.Pt(float32(i%cols)*200, float32(i/cols)*100), Size: geom.Size{W: 140, H: 40}})
	}
	for i := range n {
		if i%cols != cols-1 && i+1 < n {
			g.Edges = append(g.Edges, Edge{From: g.Nodes[i].ID, To: g.Nodes[i+1].ID})
		}
		if i%3 == 0 && i+cols < n {
			g.Edges = append(g.Edges, Edge{From: g.Nodes[i].ID, To: g.Nodes[i+cols].ID, Label: "下"})
		}
	}
	return g
}

func benchPaint(b *testing.B, n int, zoom float32) {
	g := bigGraph(n)
	d := ui.NewDriver(func(c *ui.Context) {}, ui.DriverOptions{Width: 1200, Height: 800})
	d.Frame()
	c := d.Runner().Context()
	w := Diagram(c, g).Grid(16).MiniMap()
	sty := StyleOf(c.Theme)
	st := &diagState{zoom: zoom, sel: map[string]bool{}, selEdges: map[string]bool{}, hoverNode: -1, hoverEdge: -1}
	r := geom.Rect{W: 1200, H: 800}
	var l paint.List
	w.paint(&l, r, st, sty, c.Theme) // warm the font registry and text caches
	b.ReportAllocs()
	for b.Loop() {
		l.Reset()
		g.stale()
		w.paint(&l, r, st, sty, c.Theme)
	}
	b.ReportMetric(float64(len(l.Ops)), "ops")
}

// BenchmarkDiagramPaint5000 draws a 5000-node / ~6600-edge graph at zoom
// 1 (a few dozen nodes in view) and zoomed out to show it all.
func BenchmarkDiagramPaint5000Zoom1(b *testing.B)   { benchPaint(b, 5000, 1) }
func BenchmarkDiagramPaint5000ZoomOut(b *testing.B) { benchPaint(b, 5000, 0.07) }

// BenchmarkDiagramFrame5000 runs whole driver frames (build, input, layout,
// paint and CPU rasterisation) of a 5000-node diagram while panning.
func BenchmarkDiagramFrame5000(b *testing.B) {
	g := bigGraph(5000)
	d := ui.NewDriver(func(c *ui.Context) {
		Diagram(c, g).Height(780).Grid(16).MiniMap().ZoomRange(1, 4).Show()
	}, ui.DriverOptions{Width: 1200, Height: 800})
	d.Settle()
	d.Frame()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		d.Inject(inputScroll(float32(i%7) - 3))
		d.Frame()
	}
}

// BenchmarkGitGraphFrame10k runs driver frames of a 10 000-commit graph
// while scrolling.
func BenchmarkGitGraphFrame10k(b *testing.B) {
	cs := synthHistory(10000)
	d := ui.NewDriver(func(c *ui.Context) {
		GitGraph(c, cs).Height(780).Show()
	}, ui.DriverOptions{Width: 1200, Height: 800})
	d.Settle()
	b.ReportAllocs()
	for b.Loop() {
		d.Inject(inputScroll(90))
		d.Frame()
	}
}

func inputScroll(dy float32) input.Event {
	return input.Scroll{Pos: geom.Pt(600, 400), DY: dy}
}
