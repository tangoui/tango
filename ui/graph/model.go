package graph

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Direction is the flow direction of a layered layout: the side ranks
// follow each other towards.
type Direction uint8

const (
	TopDown   Direction = iota // ranks run top to bottom (the default)
	LeftRight                  // ranks run left to right
	BottomUp                   // ranks run bottom to top
	RightLeft                  // ranks run right to left
)

func (d Direction) horizontal() bool { return d == LeftRight || d == RightLeft }

// Side is the side of a node a port (or an edge end) sits on.
type Side uint8

const (
	SideAuto       Side = iota // the side facing the other end of the edge
	SideTop                    // top edge, Offset from the left
	SideRight                  // right edge, Offset from the top
	SideBottom                 // bottom edge, Offset from the left
	SideLeft                   // left edge, Offset from the top
	SideHorizontal             // left or right, whichever faces the other end; Offset from the top
	SideVertical               // top or bottom, whichever faces the other end; Offset from the left
)

// Port is a named anchor on a node's border that edges attach to.
type Port struct {
	ID   string
	Side Side
	// Offset is the distance in graph units from the start of the side
	// (its left end for top / bottom, its top for left / right); zero or
	// less means the middle of the side.
	Offset float32
}

// Shape is the outline of a built-in node template.
type Shape uint8

const (
	ShapeRect    Shape = iota // rounded rectangle (the default)
	ShapePill                 // fully rounded ends
	ShapeEllipse              // ellipse inscribed in the node box
	ShapeDiamond              // rhombus (decisions)
)

// NodeState is what a node painter needs besides its rectangle.
type NodeState struct {
	Zoom     float32 // screen pixels per graph unit
	Selected bool
	Hovered  bool
	Theme    *theme.Theme
	Style    *Style
}

// NodePainter draws a node in screen space: r is the node box already
// scaled by s.Zoom, so sizes in graph units multiply by s.Zoom.
type NodePainter func(l *paint.List, r geom.Rect, s NodeState)

// Node is one vertex of a Graph. Positions and sizes are in graph units
// (one unit is one logical pixel at zoom 1).
type Node struct {
	ID     string
	Label  string
	Detail string     // second line of the built-in template
	Icon   *icon.Icon // leading icon of the built-in template
	Status string     // "success" | "warning" | "error" | "info" | "processing": accent colour
	Shape  Shape

	Pos  geom.Point // top-left corner
	Size geom.Size  // zero: measured from the label by the built-in template (stored back on Show)

	Ports []Port
	// Draw replaces the built-in template. The selection outline is still
	// drawn around the node.
	Draw NodePainter
	// Fixed nodes cannot be dragged (they can still be selected).
	Fixed bool
	Data  any
}

// Rect returns the node box in graph units.
func (n *Node) Rect() geom.Rect { return geom.Rect{X: n.Pos.X, Y: n.Pos.Y, W: n.Size.W, H: n.Size.H} }

// Center returns the centre of the node box.
func (n *Node) Center() geom.Point { return n.Rect().Center() }

func (n *Node) port(id string) (Port, bool) {
	if id == "" {
		return Port{}, false
	}
	for _, p := range n.Ports {
		if p.ID == id {
			return p, true
		}
	}
	return Port{}, false
}

// EdgeKind selects how an edge is routed.
type EdgeKind uint8

const (
	EdgeDefault    EdgeKind = iota // the diagram's default (DiagramW.Edges; Bézier unless set)
	EdgeBezier                     // cubic curve leaving and entering perpendicular to the sides
	EdgeStraight                   // straight segments (through Bends)
	EdgeOrthogonal                 // horizontal and vertical segments with rounded corners
)

// Arrow selects the arrowheads of an edge.
type Arrow uint8

const (
	ArrowEnd   Arrow = iota // at the target (the default)
	ArrowNone               // no arrowheads
	ArrowStart              // at the source
	ArrowBoth               // at both ends
)

func (a Arrow) end() bool   { return a == ArrowEnd || a == ArrowBoth }
func (a Arrow) start() bool { return a == ArrowStart || a == ArrowBoth }

// Edge joins two nodes, optionally at named ports.
type Edge struct {
	ID       string // optional; used for edge selection (defaults to "From->To#index")
	From, To string
	FromPort string
	ToPort   string
	Label    string
	Kind     EdgeKind
	Arrow    Arrow
	Dashed   bool
	Status   string // accent colour like Node.Status
	// Bends are intermediate points in graph units. Layout sets them for
	// edges that span several ranks; dragging an end node clears them.
	// Orthogonal edges jog through them along the flow.
	Bends []geom.Point
	Data  any
}

// Graph is the data a Diagram draws. The application owns it; the diagram
// writes node positions (dragging, auto layout) and measured sizes back.
type Graph struct {
	Nodes []Node
	Edges []Edge

	index map[string]int
	fresh bool
}

// Node returns the node with id (nil when absent). The lookup index is
// rebuilt on demand, so nodes may be appended, removed or reordered
// between calls.
func (g *Graph) Node(id string) *Node {
	if i, ok := g.Index(id); ok {
		return &g.Nodes[i]
	}
	return nil
}

// Index returns the position of node id in Nodes.
func (g *Graph) Index(id string) (int, bool) {
	if i, ok := g.index[id]; ok && i < len(g.Nodes) && g.Nodes[i].ID == id {
		return i, true
	}
	if g.fresh && len(g.index) == len(g.Nodes) {
		return 0, false // index built since the last change check: id is absent
	}
	g.reindex()
	i, ok := g.index[id]
	return i, ok
}

// stale marks the index for a re-check on the next miss (Diagram does this
// once per frame, so edits between frames are picked up).
func (g *Graph) stale() { g.fresh = false }

func (g *Graph) reindex() {
	if g.index == nil {
		g.index = make(map[string]int, len(g.Nodes))
	} else {
		clear(g.index)
	}
	for i := range g.Nodes {
		g.index[g.Nodes[i].ID] = i
	}
	g.fresh = true
}

// Bounds returns the union of the node boxes and edge bend points.
func (g *Graph) Bounds() geom.Rect {
	var b geom.Rect
	first := true
	add := func(r geom.Rect) {
		if first {
			b, first = r, false
			return
		}
		b = b.Union(r)
	}
	for i := range g.Nodes {
		add(g.Nodes[i].Rect())
	}
	for i := range g.Edges {
		for _, p := range g.Edges[i].Bends {
			add(geom.Rect{X: p.X, Y: p.Y})
		}
	}
	return b
}

// edgeID returns the identity of edge i used by the selection.
func (g *Graph) edgeID(i int) string {
	e := &g.Edges[i]
	if e.ID != "" {
		return e.ID
	}
	return e.From + "->" + e.To + "#" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
