package graph

import (
	"hash/fnv"
	"math"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Table is a database table of an ER diagram.
type Table struct {
	Name    string
	Comment string // shown under the name when set
	Columns []Column
}

// Column is one column of a Table.
type Column struct {
	Name string
	Type string
	PK   bool // part of the primary key
	FK   bool // references another table (set automatically for Relation.FromColumn)
}

// Relation is a foreign key: From.FromColumn references To.ToColumn.
type Relation struct {
	From, FromColumn string
	To, ToColumn     string
	Label            string
}

// ER table metrics in graph units.
const (
	erHeader  = 36
	erComment = 18
	erRow     = 28
	erPadB    = 4
	erMinW    = 200
	erMaxW    = 380
)

// ER starts an entity-relationship diagram of tables. Each table is a node
// with a name header and one row per column (key marks, name, type);
// relations run from the referencing column row to the referenced one.
// The graph is kept between frames and laid out left to right whenever
// the tables or relations change; dragged positions are kept otherwise.
// All Diagram modifiers apply (Height, MiniMap, Edges ...); the selection
// holds table names.
func ER(c *ui.Context, tables []Table, rels []Relation) *DiagramW {
	w := Diagram(c, nil)
	w.er = &erData{tables: tables, rels: rels}
	w.edgeKind = EdgeOrthogonal
	w.auto = &LayoutOptions{Direction: LeftRight, RankSep: 96, NodeSep: 40}
	w.height = 560
	return w
}

type erData struct {
	tables []Table
	rels   []Relation
}

// erState keeps the graph built from the tables.
type erState struct {
	sig uint64
	g   Graph
}

// erSalt derives the ER state ID from the canvas ID.
const erSalt ui.ID = 0x9e3779b97f4a7c15

// graphFor returns the retained graph for the tables, rebuilt when they
// change (positions of tables that stay are kept).
func (d *erData) graphFor(c *ui.Context, id ui.ID, sty *Style) *Graph {
	st := ui.State[erState](c, id^erSalt, nil)
	sig := d.sig()
	if st.sig == sig && len(st.g.Nodes) == len(d.tables) {
		return &st.g
	}
	st.sig = sig
	old := map[string]geom.Point{}
	for i := range st.g.Nodes {
		old[st.g.Nodes[i].ID] = st.g.Nodes[i].Pos
	}
	fk := map[[2]string]bool{}
	for _, r := range d.rels {
		fk[[2]string{r.From, r.FromColumn}] = true
	}
	g := Graph{Nodes: make([]Node, 0, len(d.tables))}
	for ti := range d.tables {
		t := d.tables[ti]
		cols := make([]Column, len(t.Columns))
		copy(cols, t.Columns)
		for i := range cols {
			if fk[[2]string{t.Name, cols[i].Name}] {
				cols[i].FK = true
			}
		}
		t.Columns = cols
		top := float32(erHeader)
		if t.Comment != "" {
			top += erComment
		}
		ports := make([]Port, len(cols))
		for i, col := range cols {
			ports[i] = Port{ID: col.Name, Side: SideHorizontal, Offset: top + float32(i)*erRow + erRow/2}
		}
		tp := &erTable{t: t, top: top}
		n := Node{ID: t.Name, Label: t.Name, Detail: t.Comment, Size: tp.size(sty), Ports: ports, Draw: tp.draw(c), Data: &d.tables[ti]}
		n.Pos = old[t.Name]
		g.Nodes = append(g.Nodes, n)
	}
	for _, r := range d.rels {
		g.Edges = append(g.Edges, Edge{From: r.From, FromPort: r.FromColumn, To: r.To, ToPort: r.ToColumn, Label: r.Label})
	}
	st.g = g
	return &st.g
}

// sig hashes everything the graph is built from.
func (d *erData) sig() uint64 {
	h := fnv.New64a()
	w := func(s string) { h.Write([]byte(s)); h.Write([]byte{0}) }
	for _, t := range d.tables {
		w(t.Name)
		w(t.Comment)
		for _, c := range t.Columns {
			w(c.Name)
			w(c.Type)
			h.Write([]byte{b2u(c.PK), b2u(c.FK)})
		}
		h.Write([]byte{1})
	}
	for _, r := range d.rels {
		w(r.From)
		w(r.FromColumn)
		w(r.To)
		w(r.ToColumn)
		w(r.Label)
	}
	return h.Sum64()
}

func b2u(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// erTable draws one table node.
type erTable struct {
	t   Table
	top float32
}

func (tp *erTable) size(sty *Style) geom.Size {
	w := text.Width(tp.t.Name, text.Style{Size: sty.Font, Weight: text.Semibold}) + 24 + 32
	if tp.t.Comment != "" {
		w = max(w, text.Width(tp.t.Comment, text.Style{Size: sty.FontSM})+32)
	}
	for _, c := range tp.t.Columns {
		cw := 24 + text.Width(c.Name, text.Style{Size: sty.FontSM}) + 24 + text.Width(c.Type, text.Style{Size: sty.FontSM, Mono: true}) + 24
		w = max(w, cw)
	}
	w = max(erMinW, min(erMaxW, float32(math.Ceil(float64(w)))))
	h := tp.top + float32(len(tp.t.Columns))*erRow + erPadB
	return geom.Size{W: w, H: h}
}

func (tp *erTable) draw(c *ui.Context) NodePainter {
	return func(l *paint.List, r geom.Rect, s NodeState) {
		sty, k := s.Style, s.Zoom
		rad := sty.NodeRadius * k
		if k >= 0.5 && len(sty.NodeShadow) > 0 {
			l.Shadows(r, geom.Rad(rad), sty.NodeShadow)
		}
		l.Fill(r, geom.Rad(rad), sty.NodeBg)
		hr := geom.Rect{X: r.X, Y: r.Y, W: r.W, H: tp.top * k}
		l.Fill(hr, geom.Radii{TL: rad, TR: rad}, sty.HeaderBg)
		l.Line(geom.Pt(r.X, hr.Bottom()), geom.Pt(r.Right(), hr.Bottom()), 1, sty.NodeBorder)
		pad := 12 * k
		is := 16 * k
		if is >= 4 {
			c.PaintIcon(l, icon.Table, geom.Rect{X: r.X + pad, Y: r.Y + (erHeader*k-is)/2, W: is, H: is}, sty.Selection, 0)
		}
		textBox(l, tp.t.Name, text.Style{Size: sty.Font, Weight: text.Semibold},
			geom.Rect{X: r.X + pad + 22*k, Y: r.Y, W: r.W - 2*pad - 22*k, H: erHeader * k}, k, sty.HeaderText, false)
		if tp.t.Comment != "" {
			textBox(l, tp.t.Comment, text.Style{Size: sty.FontSM},
				geom.Rect{X: r.X + pad, Y: r.Y + (erHeader-8)*k, W: r.W - 2*pad, H: erComment * k}, k, sty.NodeDetail, false)
		}
		fs := text.Style{Size: sty.FontSM}
		ts := text.Style{Size: sty.FontSM, Mono: true}
		for i, col := range tp.t.Columns {
			y := r.Y + (tp.top+float32(i)*erRow)*k
			if i > 0 {
				l.Fill(geom.Rect{X: r.X + pad, Y: y, W: r.W - 2*pad, H: max(0.5, k*0.5)}, geom.Radii{}, sty.MiniBorder)
			}
			x := r.X + pad
			ks := 14 * k
			if ks >= 4 {
				kr := geom.Rect{X: x, Y: y + (erRow*k-ks)/2, W: ks, H: ks}
				switch {
				case col.PK:
					c.PaintIcon(l, icon.Key, kr, sty.KeyPK, 0)
				case col.FK:
					c.PaintIcon(l, icon.Link, kr, sty.KeyFK, 0)
				}
			}
			x += 22 * k
			tw := text.Width(col.Type, ts) * k
			tw = min(tw, r.W-2*pad-(22+48)*k)
			nameCol := sty.NodeText
			if col.PK {
				nameCol = sty.HeaderText
			}
			textBox(l, col.Name, fs, geom.Rect{X: x, Y: y, W: r.Right() - pad - tw - 8*k - x, H: erRow * k}, k, nameCol, false)
			textBox(l, col.Type, ts, geom.Rect{X: r.Right() - pad - tw, Y: y, W: tw, H: erRow * k}, k, sty.NodeDetail, false)
		}
		border := sty.NodeBorder
		if s.Hovered || s.Selected {
			border = sty.Hover // the diagram draws the selection outline
		}
		l.Stroke(r, geom.Rad(rad), 1, border)
	}
}
