package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// RTL geometry of Table (docs/specs/rtl-and-component-tokens.md §3.5):
// columns mirror, FixedLeft / FixedRight are start / end, horizontal
// scrolling starts at the right end, header icons, tree indent and keys
// follow the inline direction.

const g6W, g6H = 900, 500

func g6TableCols() []TableColumn {
	return []TableColumn{
		{Key: "name", Title: "Name", Width: 140, Fixed: FixedLeft, Sortable: true, Compare: func(a, b int) int { return a - b }, Text: func(i int) string { return "Row " + strconv.Itoa(i) }},
		{Key: "age", Title: "Age", Width: 90, Align: AlignRight, Filters: []Option{{Label: "A", Value: "a"}}, Text: func(i int) string { return strconv.Itoa(20 + i) }},
		{Key: "addr", Title: "Address", Width: 260, Text: func(i int) string { return "Street " + strconv.Itoa(i) }},
		{Key: "mid", Title: "Center", Width: 120, Align: AlignCenter, Text: func(i int) string { return "c" + strconv.Itoa(i) }},
		{Key: "op", Title: "Action", Width: 100, Fixed: FixedRight, Text: func(int) string { return "Edit" }},
	}
}

// g6Driver renders page in the given direction.
func g6Driver(dir Direction, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { ConfigProvider(c).Direction(dir).Body(func() { page(c) }) }, DriverOptions{Width: g6W, Height: g6H})
	d.Settle()
	d.Settle()
	return d
}

// g6Node returns the first drawn node with kind-less label s.
func g6Node(d *Driver, label string) *node {
	c := d.r.c
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil {
			return
		}
		if n.label == label && n.drawn == c.pass {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	return found
}

func g6Rect(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	n := g6Node(d, label)
	if n == nil {
		t.Fatalf("node %q not drawn", label)
	}
	return n.abs
}

// TestG6TableMirrorCheck: every node of several table configurations sits
// at the mirror image of its LTR position inside its parent.
func TestG6TableMirrorCheck(t *testing.T) {
	open := []string{"0"}
	exp := []string{"1"}
	pages := map[string]func(c *Context){
		"plain": func(c *Context) {
			cols := g6TableCols()
			for i := range cols {
				cols[i].Fixed = ""
			}
			Table(c, cols, 4).Bordered().Show()
		},
		"fixed-scroll": func(c *Context) {
			sel := []int{1}
			Table(c, g6TableCols(), 8).ScrollX(1400).ScrollY(200).Selection(&sel).Resizable().Show()
		},
		"tree-expand-summary": func(c *Context) {
			cols := g6TableCols()[:3]
			cols[0].Fixed = ""
			Table(c, cols, 4).Tree([]int{0, 3}, func(r int) []int {
				if r == 0 {
					return []int{1, 2}
				}
				return nil
			}).TreeOpenKeys(&open).Summary(func(col string, _ []int) string { return "S" + col }).Show()
			Table(c, cols, 3).Expandable(func(int) { Text(c, "detail") }).ExpandedKeys(&exp).DragSort(func(int, int) {}).Show()
		},
		"grouped": func(c *Context) {
			Table(c, []TableColumn{
				{Key: "a", Title: "A", Width: 100, Text: func(int) string { return "a" }},
				{Title: "G", Children: []TableColumn{
					{Key: "b", Title: "B", Width: 100, Text: func(int) string { return "b" }},
					{Key: "cc", Title: "C", Width: 100, Text: func(int) string { return "c" }},
				}},
			}, 3).Show()
		},
	}
	for name, page := range pages {
		d := NewDriver(page, DriverOptions{Width: g6W, Height: 700})
		rep := d.MirrorCheck(page)
		if !rep.OK() || rep.Checked == 0 {
			t.Errorf("%s: %s", name, rep)
		}
	}
}

// TestG6TableRTLColumns: the first (start) column sits at the right edge,
// FixedLeft sticks to the right and FixedRight to the left, and the body
// scroll starts at the right end.
func TestG6TableRTLColumns(t *testing.T) {
	page := func(c *Context) { Table(c, g6TableCols(), 5).ScrollX(1400).Show() }
	ltr, rtl := g6Driver(LTR, page), g6Driver(RTL, page)
	bodyOf := func(d *Driver) geom.Rect { return d.r.c.rects[g6Node(d, "row:0").parent.id] }
	lb, rb := bodyOf(ltr), bodyOf(rtl)
	lname, lop, laddr := g6Rect(t, ltr, "Name"), g6Rect(t, ltr, "Action"), g6Rect(t, ltr, "Address")
	if !near(lname.X, lb.X) || !near(lop.Right(), lb.Right()) || laddr.X <= lname.Right()-1 {
		t.Fatalf("ltr: Name %+v Action %+v Address %+v body %+v", lname, lop, laddr, lb)
	}
	name, op, addr := g6Rect(t, rtl, "Name"), g6Rect(t, rtl, "Action"), g6Rect(t, rtl, "Address")
	if !near(name.Right(), rb.Right()) || !near(op.X, rb.X) || !near(name.W, lname.W) || !near(op.W, lop.W) {
		t.Fatalf("rtl: Name %+v Action %+v body %+v: fixed columns not at start=right / end=left", name, op, rb)
	}
	// Offset 0 shows the start (right) end: Address sits as far from the
	// right edge as it does from the left edge in LTR.
	if off := laddr.X - lb.X; !near(addr.Right(), rb.Right()-off) || !near(addr.W, laddr.W) {
		t.Fatalf("rtl: Address %+v want right=%v", addr, rb.Right()-off)
	}
	row := g6Rect(t, rtl, "row:0")
	if !near(row.Right(), rb.Right()) || row.X > rb.X-400 {
		t.Fatalf("rtl: row %+v does not start at the right end of %+v", row, rb)
	}
	// A physical wheel to the left scrolls toward the end in RTL: the
	// scrolled cells move right, the fixed ones stay.
	rtl.Inject(input.Scroll{Pos: geom.Pt(rb.X+300, rb.Y+20), DX: -200})
	rtl.Settle()
	rtl.Settle()
	addr1, name1, op1 := g6Rect(t, rtl, "Address"), g6Rect(t, rtl, "Name"), g6Rect(t, rtl, "Action")
	if !near(addr1.X, addr.X+200) {
		t.Fatalf("rtl wheel: Address %+v want x=%v", addr1, addr.X+200)
	}
	if !near(name1.Right(), rb.Right()) || !near(op1.X, rb.X) {
		t.Fatalf("rtl wheel: fixed columns moved: Name %+v Action %+v", name1, op1)
	}
	// Body cells follow the header.
	td := g6Rect(t, rtl, "Street 0")
	if !near(td.X, addr1.X) || !near(td.W, addr1.W) {
		t.Fatalf("rtl wheel: body cell %+v not under header %+v", td, addr1)
	}
}

// TestG6TableRTLFixedShadow: the start group casts its shadow toward the
// end (left in RTL), the end group toward the start.
func TestG6TableRTLFixedShadow(t *testing.T) {
	page := func(c *Context) { Table(c, g6TableCols(), 3).ScrollX(1400).Show() }
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, page)
		body := d.r.c.rects[g6Node(d, "row:0").parent.id]
		dx := float32(200)
		if dir == RTL {
			dx = -200
		}
		d.Inject(input.Scroll{Pos: geom.Pt(body.X+300, body.Y+20), DX: dx})
		d.Settle()
		d.Settle()
		name, op := g6Rect(t, d, "Name"), g6Rect(t, d, "Action")
		var strips []geom.Rect
		for _, o := range d.r.List().Ops {
			if cl, ok := o.(paint.PushClip); ok && cl.Rect.W == 14 {
				strips = append(strips, cl.Rect)
			}
		}
		if len(strips) != 2 {
			t.Fatalf("dir %v: %d shadow strips", dir, len(strips))
		}
		startStrip, endStrip := name.Right(), op.X-14
		if dir == RTL {
			startStrip, endStrip = name.X-14, op.Right()
		}
		if !near(strips[0].X, startStrip) || !near(strips[1].X, endStrip) {
			t.Fatalf("dir %v: strips %+v want x=%v, %v", dir, strips, startStrip, endStrip)
		}
	}
}

// g6PathsIn returns the mean x of the filled triangles lying inside r.
func g6PathsIn(d *Driver, r geom.Rect) (float32, int) {
	var sum float32
	n := 0
	for _, o := range d.r.List().Ops {
		fp, ok := o.(paint.FillPath)
		if !ok || len(fp.Path.Segs) != 4 {
			continue
		}
		in := true
		var sx float32
		k := 0
		for _, sg := range fp.Path.Segs {
			if sg.Op == paint.SegClose {
				continue
			}
			p := sg.P[0]
			if !r.Contains(p) {
				in = false
				break
			}
			sx += p.X
			k++
		}
		if in && k > 0 {
			sum += sx / float32(k)
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float32(n), n
}

// TestG6TableRTLHeaderIcons: the sorter of a start-aligned title sits at
// the cell's end (left in RTL); the title starts at the right; an
// end-aligned (numeric) column's text sits at the left in RTL.
func TestG6TableRTLHeaderIcons(t *testing.T) {
	page := func(c *Context) { Table(c, g6TableCols(), 2).Show() }
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, page)
		name := g6Rect(t, d, "Name")
		x, n := g6PathsIn(d, name)
		if n != 2 { // the two sorter triangles
			t.Fatalf("dir %v: %d sorter paths in %+v", dir, n, name)
		}
		mid := name.X + name.W/2
		if (dir == LTR) != (x > mid) {
			t.Fatalf("dir %v: sorter at x=%v in %+v", dir, x, name)
		}
		// Title glyphs: near the start edge.
		title := g6TextIn(d, name)
		if dir == RTL && !(title.Right() > name.Right()-30) || dir == LTR && !(title.X < name.X+30) {
			t.Fatalf("dir %v: title %+v in %+v", dir, title, name)
		}
		// Numeric column (AlignRight = end): text at the left in RTL.
		age := g6Rect(t, d, "20")
		txt := g6TextIn(d, age)
		if dir == RTL && !(txt.X < age.X+30) || dir == LTR && !(txt.Right() > age.Right()-30) {
			t.Fatalf("dir %v: numeric text %+v in %+v", dir, txt, age)
		}
	}
}

// g6TextIn returns the extent of the glyph runs whose origin is inside r.
func g6TextIn(d *Driver, r geom.Rect) geom.Rect {
	var out geom.Rect
	for _, o := range d.r.List().Ops {
		tx, ok := o.(paint.Text)
		if !ok || !r.Contains(tx.Origin) || len(tx.Glyphs) == 0 {
			continue
		}
		x0, x1 := tx.Origin.X+tx.Glyphs[0].X, tx.Origin.X
		for _, g := range tx.Glyphs {
			x0 = min(x0, tx.Origin.X+g.X)
			x1 = max(x1, tx.Origin.X+g.X+g.Size*0.5) // approximate advance
		}
		out = geom.Rect{X: x0, Y: tx.Origin.Y, W: x1 - x0, H: 1}
	}
	return out
}

// TestG6TableRTLTreeIndent: tree rows indent from the right in RTL.
func TestG6TableRTLTreeIndent(t *testing.T) {
	open := []string{"0"}
	page := func(c *Context) {
		cols := g6TableCols()[:2]
		cols[0].Fixed = ""
		Table(c, cols, 3).Tree([]int{0}, func(r int) []int {
			if r == 0 {
				return []int{1}
			}
			return nil
		}).TreeOpenKeys(&open).Show()
	}
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, page)
		parent, child := g6Rect(t, d, "tree:0"), g6Rect(t, d, "tree:1")
		if dir == RTL && !(child.X < parent.X-10) || dir == LTR && !(child.X > parent.X+10) {
			t.Fatalf("dir %v: toggles parent %+v child %+v", dir, parent, child)
		}
		cell := g6Rect(t, d, "Row 0")
		if dir == RTL && !near(parent.Right(), cell.Right()-16) {
			t.Fatalf("rtl: root toggle %+v not at the start of %+v", parent, cell)
		}
	}
}

// TestG6TableRTLKeys: Left / Right move the active cell visually.
func TestG6TableRTLKeys(t *testing.T) {
	var tw *TableW
	page := func(c *Context) { tw = Table(c, g6TableCols(), 3).Show() }
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, page)
		addr := g6Rect(t, d, "Street 1")
		d.Click(geom.Pt(addr.X+addr.W/2, addr.Y+addr.H/2))
		d.Settle()
		st := tw.st
		if !st.actOn || st.actCol != 2 {
			t.Fatalf("dir %v: active cell %d (on=%v)", dir, st.actCol, st.actOn)
		}
		d.Key(input.KeyRight, 0)
		d.Settle()
		want := 3 // the column to the right: the next one in LTR
		if dir == RTL {
			want = 1 // ... the previous one in RTL
		}
		if st := tw.st; st.actCol != want {
			t.Fatalf("dir %v: Right moved to column %d, want %d", dir, st.actCol, want)
		}
		d.Key(input.KeyLeft, 0)
		d.Key(input.KeyLeft, 0)
		d.Settle()
		want = 1
		if dir == RTL {
			want = 3
		}
		if st := tw.st; st.actCol != want {
			t.Fatalf("dir %v: Left×2 moved to column %d, want %d", dir, st.actCol, want)
		}
	}
}

// TestG6TableRTLResize: the resize handle sits on the column's end edge
// (left in RTL); dragging it outward (left) widens the column.
func TestG6TableRTLResize(t *testing.T) {
	var tw *TableW
	page := func(c *Context) {
		cols := g6TableCols()
		for i := range cols {
			cols[i].Fixed = ""
		}
		tw = Table(c, cols, 2).Resizable().Show()
	}
	d := g6Driver(RTL, page)
	h, addr := g6Rect(t, d, "addr"), g6Rect(t, d, "Address")
	w0 := tw.st.widths["addr"]
	if w0 == 0 {
		w0 = addr.W
	}
	if !near(h.X+h.W/2, addr.X) {
		t.Fatalf("handle %+v not on the left edge of %+v", h, addr)
	}
	p := geom.Pt(h.X+h.W/2, h.Y+h.H/2)
	d.Drag(p, geom.Pt(p.X-30, p.Y))
	d.Settle()
	if w := tw.st.widths["addr"]; !near(w, w0+30) {
		t.Fatalf("dragging left by 30 in RTL: width %v, want %v", w, w0+30)
	}
}
