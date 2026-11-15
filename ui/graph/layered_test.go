package graph

import (
	"fmt"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func mkGraph(nodes []string, edges ...[2]string) *Graph {
	g := &Graph{}
	for _, id := range nodes {
		g.Nodes = append(g.Nodes, Node{ID: id, Label: id, Size: geom.Size{W: 100, H: 40}})
	}
	for _, e := range edges {
		g.Edges = append(g.Edges, Edge{From: e[0], To: e[1]})
	}
	return g
}

func overlaps(a, b geom.Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

func assertNoOverlap(t *testing.T, g *Graph) {
	t.Helper()
	for i := range g.Nodes {
		for j := i + 1; j < len(g.Nodes); j++ {
			if overlaps(g.Nodes[i].Rect(), g.Nodes[j].Rect()) {
				t.Fatalf("nodes %s %v and %s %v overlap", g.Nodes[i].ID, g.Nodes[i].Rect(), g.Nodes[j].ID, g.Nodes[j].Rect())
			}
		}
	}
}

func TestLayoutChainRanksTopDown(t *testing.T) {
	g := mkGraph([]string{"a", "b", "c"}, [2]string{"a", "b"}, [2]string{"b", "c"})
	Layout(g, LayoutOptions{})
	a, b, c := g.Node("a"), g.Node("b"), g.Node("c")
	if !(a.Pos.Y < b.Pos.Y && b.Pos.Y < c.Pos.Y) {
		t.Fatalf("ranks not top-down: %v %v %v", a.Pos, b.Pos, c.Pos)
	}
	if b.Pos.Y-a.Rect().Bottom() != 64 {
		t.Fatalf("rank gap = %v, want 64", b.Pos.Y-a.Rect().Bottom())
	}
	// A chain is drawn straight.
	if a.Center().X != b.Center().X || b.Center().X != c.Center().X {
		t.Fatalf("chain not aligned: %v %v %v", a.Center(), b.Center(), c.Center())
	}
	if r := g.Bounds(); r.X != 0 || r.Y != 0 {
		t.Fatalf("bounds origin = %v", r)
	}
}

func TestLayoutDirections(t *testing.T) {
	for _, tc := range []struct {
		d    Direction
		less func(a, b geom.Point) bool
	}{
		{TopDown, func(a, b geom.Point) bool { return a.Y < b.Y }},
		{BottomUp, func(a, b geom.Point) bool { return a.Y > b.Y }},
		{LeftRight, func(a, b geom.Point) bool { return a.X < b.X }},
		{RightLeft, func(a, b geom.Point) bool { return a.X > b.X }},
	} {
		g := mkGraph([]string{"a", "b"}, [2]string{"a", "b"})
		Layout(g, LayoutOptions{Direction: tc.d})
		if !tc.less(g.Node("a").Center(), g.Node("b").Center()) {
			t.Errorf("direction %d: a %v, b %v", tc.d, g.Node("a").Center(), g.Node("b").Center())
		}
	}
}

func TestLayoutDiamondCentresJoin(t *testing.T) {
	g := mkGraph([]string{"a", "b", "c", "d"},
		[2]string{"a", "b"}, [2]string{"a", "c"}, [2]string{"b", "d"}, [2]string{"c", "d"})
	Layout(g, LayoutOptions{})
	assertNoOverlap(t, g)
	a, b, c, d := g.Node("a"), g.Node("b"), g.Node("c"), g.Node("d")
	if b.Pos.Y != c.Pos.Y {
		t.Fatalf("b and c not on one rank")
	}
	mid := (b.Center().X + c.Center().X) / 2
	if math.Abs(float64(a.Center().X-mid)) > 0.5 || math.Abs(float64(d.Center().X-mid)) > 0.5 {
		t.Fatalf("a %v / d %v not centred over b,c (%v)", a.Center(), d.Center(), mid)
	}
}

func TestLayoutBreaksCycles(t *testing.T) {
	g := mkGraph([]string{"a", "b", "c"}, [2]string{"a", "b"}, [2]string{"b", "c"}, [2]string{"c", "a"})
	Layout(g, LayoutOptions{})
	assertNoOverlap(t, g)
	ys := map[float32]bool{}
	for _, n := range g.Nodes {
		ys[n.Pos.Y] = true
	}
	if len(ys) != 3 {
		t.Fatalf("cycle of 3 should take 3 ranks, got %d", len(ys))
	}
}

func TestLayoutLongEdgeGetsBends(t *testing.T) {
	g := mkGraph([]string{"a", "b", "c", "d"},
		[2]string{"a", "b"}, [2]string{"b", "c"}, [2]string{"c", "d"}, [2]string{"a", "d"})
	Layout(g, LayoutOptions{})
	long := g.Edges[3]
	if len(long.Bends) != 4 {
		t.Fatalf("a->d crosses 2 ranks, want 4 bends (entry and exit of each), got %v", long.Bends)
	}
	for i := 1; i < len(long.Bends); i++ {
		if !(long.Bends[i-1].Y < long.Bends[i].Y) {
			t.Fatalf("bends not ordered from source: %v", long.Bends)
		}
	}
	for _, e := range g.Edges[:3] {
		if e.Bends != nil {
			t.Fatalf("short edge has bends: %+v", e)
		}
	}
	// The bends must not sit inside b or c.
	for _, p := range long.Bends {
		for _, id := range []string{"b", "c"} {
			if g.Node(id).Rect().Expand(1).Contains(p) {
				t.Fatalf("bend %v inside %s", p, id)
			}
		}
	}
}

func TestLayoutReversedEdgeBendsRunFromSource(t *testing.T) {
	// d->a closes a cycle and is reversed internally; its bends must still
	// run from d to a.
	g := mkGraph([]string{"a", "b", "c", "d"},
		[2]string{"a", "b"}, [2]string{"b", "c"}, [2]string{"c", "d"}, [2]string{"d", "a"})
	Layout(g, LayoutOptions{})
	e := g.Edges[3]
	if len(e.Bends) != 4 || !(e.Bends[0].Y > e.Bends[3].Y) {
		t.Fatalf("reversed edge bends = %v, want from bottom (d) to top (a)", e.Bends)
	}
}

func TestLayoutReducesCrossings(t *testing.T) {
	// Two parents each with children listed in crossing order.
	g := mkGraph([]string{"p1", "p2", "x", "y", "z", "w"},
		[2]string{"p1", "z"}, [2]string{"p1", "w"}, [2]string{"p2", "x"}, [2]string{"p2", "y"})
	Layout(g, LayoutOptions{})
	assertNoOverlap(t, g)
	// Children of p1 must be on p1's side.
	p1Left := g.Node("p1").Center().X < g.Node("p2").Center().X
	for _, id := range []string{"z", "w"} {
		for _, o := range []string{"x", "y"} {
			if (g.Node(id).Center().X < g.Node(o).Center().X) != p1Left {
				t.Fatalf("crossing left: %s vs %s", id, o)
			}
		}
	}
}

func TestCrossingCount(t *testing.T) {
	l := &layered{n: 4, ranks: []int{0, 0, 1, 1}}
	l.succ = [][]int{{3}, {2}, nil, nil}
	l.pred = [][]int{nil, nil, {1}, {0}}
	l.layers = [][]int{{0, 1}, {2, 3}}
	l.pos = make([]int, 4)
	l.reindex()
	if c := l.crossings(); c != 1 {
		t.Fatalf("crossings = %d, want 1", c)
	}
	l.layers[1] = []int{3, 2}
	l.reindex()
	if c := l.crossings(); c != 0 {
		t.Fatalf("crossings = %d, want 0", c)
	}
}

func TestIsotonicGap(t *testing.T) {
	// Two nodes wanting the same spot spread symmetrically.
	out := isotonicGap([]float64{10, 10}, []float64{1, 1}, []float64{20})
	if out[0] != 0 || out[1] != 20 {
		t.Fatalf("got %v, want [0 20]", out)
	}
	// Feasible wishes are kept.
	out = isotonicGap([]float64{0, 50, 51}, []float64{1, 1, 1}, []float64{10, 1})
	if out[0] != 0 || out[1] != 50 || out[2] != 51 {
		t.Fatalf("got %v", out)
	}
	// Weights pull the pooled block.
	out = isotonicGap([]float64{0, 0}, []float64{3, 1}, []float64{8})
	if math.Abs(out[0]+2) > 1e-9 || math.Abs(out[1]-6) > 1e-9 {
		t.Fatalf("got %v, want [-2 6]", out)
	}
}

func TestLayoutDisconnectedAndMissing(t *testing.T) {
	g := mkGraph([]string{"a", "b", "c", "lone"}, [2]string{"a", "b"}, [2]string{"a", "ghost"}, [2]string{"c", "c"})
	g.Nodes[3].Size = geom.Size{}
	Layout(g, LayoutOptions{})
	assertNoOverlap(t, g)
	if g.Node("lone").Size != (geom.Size{W: 140, H: 40}) {
		t.Fatalf("zero size not defaulted: %v", g.Node("lone").Size)
	}
}

func TestLayoutRandomNoOverlap(t *testing.T) {
	g := randomDAG(300, 2, 7)
	Layout(g, LayoutOptions{Direction: LeftRight})
	assertNoOverlap(t, g)
	for _, e := range g.Edges {
		a, b := g.Node(e.From), g.Node(e.To)
		if a.Center().X >= b.Center().X {
			t.Fatalf("edge %s->%s does not flow left to right", e.From, e.To)
		}
	}
}

// randomDAG builds n nodes with about deg out-edges each to later nodes
// (deterministic for seed).
func randomDAG(n, deg int, seed uint32) *Graph {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("n%d", i)
	}
	g := mkGraph(ids)
	r := seed
	next := func() uint32 { r ^= r << 13; r ^= r >> 17; r ^= r << 5; return r }
	for i := 0; i < n-1; i++ {
		for k := 0; k < deg; k++ {
			span := min(n-i-1, 12)
			j := i + 1 + int(next()%uint32(span))
			g.Edges = append(g.Edges, Edge{From: ids[i], To: ids[j]})
		}
	}
	return g
}

func BenchmarkLayout1000(b *testing.B) {
	base := randomDAG(1000, 2, 3)
	b.ReportAllocs()
	for b.Loop() {
		g := &Graph{Nodes: append([]Node(nil), base.Nodes...), Edges: append([]Edge(nil), base.Edges...)}
		Layout(g, LayoutOptions{})
	}
}
