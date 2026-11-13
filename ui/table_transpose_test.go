package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// tpCols are three fields: plain text, an end-aligned number and a Render
// cell (a Text widget).
func tpCols(c *Context) []TableColumn {
	return []TableColumn{
		{Key: "name", Title: "Name", Text: func(r int) string { return "n" + strconv.Itoa(r) }},
		{Key: "price", Title: "Price", Align: AlignRight, Text: func(r int) string { return "p" + strconv.Itoa(r) }},
		{Key: "tag", Title: "Tag", Render: func(r int) { Text(c, "t"+strconv.Itoa(r)) }},
		{Key: "hidden", Title: "Hidden", Hidden: true, Text: func(r int) string { return "h" + strconv.Itoa(r) }},
	}
}

func tpRecTitle(r int) string { return "R" + strconv.Itoa(r) }

// TestTableTransposeGrid: fields are rows, records are columns, the header
// column holds the titles, record titles head the columns, cells call the
// field callbacks with the record index and keep the field's alignment.
func TestTableTransposeGrid(t *testing.T) {
	page := func(c *Context) {
		Table(c, tpCols(c), 3).Transposed().TransposedHeader(tpRecTitle).Show()
	}
	d := g6Driver(LTR, page)
	if g6Node(d, "Hidden") != nil || g6Node(d, "h0") != nil {
		t.Fatal("hidden field shown")
	}
	rows := []string{"Name", "Price", "Tag"}
	cell := func(fi, r int) geom.Rect {
		return g6Rect(t, d, []string{"n", "p", "t"}[fi]+strconv.Itoa(r))
	}
	for fi, title := range rows {
		h := g6Rect(t, d, title)
		for r := 0; r < 3; r++ {
			cr := cell(fi, r)
			head := g6Rect(t, d, tpRecTitle(r))
			// Same row as the field title, same column as the record title.
			if cr.Y < h.Y-1 || cr.Bottom() > h.Bottom()+1 {
				t.Fatalf("%s/R%d: cell %+v not in row %+v", title, r, cr, h)
			}
			if cr.X < head.X-1 || cr.Right() > head.Right()+1 {
				t.Fatalf("%s/R%d: cell %+v not under %+v", title, r, cr, head)
			}
			if h.X >= cr.X {
				t.Fatalf("%s: header column %+v not before cell %+v", title, h, cr)
			}
		}
	}
	// The record header row sits above the first field row.
	if r0, n := g6Rect(t, d, "R0"), g6Rect(t, d, "Name"); r0.Bottom() > n.Y+1 {
		t.Fatalf("header row %+v not above %+v", r0, n)
	}
	// The Render cell's widget lies inside its record column.
	tag, head := g6Rect(t, d, "t2"), g6Rect(t, d, "R2")
	if tag.X < head.X || tag.Right() > head.Right() {
		t.Fatalf("render cell %+v outside column %+v", tag, head)
	}
}

// TestTableTransposeAlign: the cell alignment follows the field's Align.
func TestTableTransposeAlign(t *testing.T) {
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, func(c *Context) { Table(c, tpCols(c), 2).Transposed().Show() })
		price := g6Node(d, "p1")
		name := g6Node(d, "n1")
		if price == nil || name == nil {
			t.Fatalf("dir %v: cells not drawn", dir)
		}
		// Measure where the glyphs were drawn: find text ops inside each cell.
		textX := func(r geom.Rect) float32 {
			for _, o := range d.r.List().Ops {
				if g, ok := o.(paint.Text); ok && r.Contains(g.Origin) {
					return g.Origin.X
				}
			}
			t.Fatalf("dir %v: no text in %+v", dir, r)
			return 0
		}
		px, nx := textX(price.abs)-price.abs.X, textX(name.abs)-name.abs.X
		w := price.abs.W
		startAligned := func(off float32) bool {
			if dir == RTL {
				return off > w/2
			}
			return off < w/2
		}
		if !startAligned(nx) {
			t.Fatalf("dir %v: Name text offset %v of %v not at the start", dir, nx, w)
		}
		if startAligned(px) {
			t.Fatalf("dir %v: Price (AlignRight = end) text offset %v of %v at the start", dir, px, w)
		}
	}
}

// TestTableTransposeNoHeader: without TransposedHeader there is no header
// row; the first field row starts at the top of the table.
func TestTableTransposeNoHeader(t *testing.T) {
	var tw *TableW
	d := g6Driver(LTR, func(c *Context) { tw = Table(c, tpCols(c), 2).Transposed().Show() })
	d.EnableAccessibility()
	if _, ok := d.A11yFind(a11y.RoleColumnHeader, ""); ok {
		t.Fatal("column header without TransposedHeader")
	}
	name := g6Rect(t, d, "Name")
	frame := d.r.c.rects[tw.fr.frameID]
	if !near(name.Y, frame.Y) {
		t.Fatalf("first row %+v not at the top of %+v", name, frame)
	}
	g, ok := d.A11yFind(a11y.RoleGrid, "")
	if !ok || g.RowCount != 3 || g.ColCount != 3 {
		t.Fatalf("grid %+v: want 3 rows (fields) x 3 cols", g)
	}
	rh, ok := d.A11yFind(a11y.RoleRowHeader, "Name")
	if !ok || rh.RowIndex != 1 || rh.ColIndex != 1 {
		t.Fatalf("row header Name %+v: want row 1 col 1", rh)
	}
}

// TestTableTransposeA11y: header-column cells are row headers, record
// titles column headers, and indices follow the transposed grid.
func TestTableTransposeA11y(t *testing.T) {
	d := g6Driver(LTR, func(c *Context) {
		Table(c, tpCols(c), 3).Transposed().TransposedHeader(tpRecTitle).A11yLabel("Plans").Show()
	})
	d.EnableAccessibility()
	g, ok := d.A11yFind(a11y.RoleGrid, "Plans")
	if !ok || g.RowCount != 4 || g.ColCount != 4 {
		t.Fatalf("grid %+v: want 4 rows (header + 3 fields) x 4 cols (header + 3 records)", g)
	}
	for i, title := range []string{"Name", "Price", "Tag"} {
		n, ok := d.A11yFind(a11y.RoleRowHeader, title)
		if !ok || n.RowIndex != i+2 || n.ColIndex != 1 {
			t.Fatalf("row header %s: %+v want row %d col 1", title, n, i+2)
		}
		if _, ok := d.A11yFind(a11y.RoleColumnHeader, title); ok {
			t.Fatalf("%s is a column header", title)
		}
	}
	for r := 0; r < 3; r++ {
		n, ok := d.A11yFind(a11y.RoleColumnHeader, tpRecTitle(r))
		if !ok || n.RowIndex != 1 || n.ColIndex != r+2 {
			t.Fatalf("column header R%d: %+v want row 1 col %d", r, n, r+2)
		}
	}
	c, ok := d.A11yFind(a11y.RoleCell, "p1")
	if !ok || c.RowIndex != 3 || c.ColIndex != 3 {
		t.Fatalf("cell p1 %+v: want row 3 col 3", c)
	}
	rep := d.AuditNoContrast()
	for _, is := range rep.Issues {
		if is.Kind == AuditMissingName || is.Kind == AuditNoRole || is.Kind == AuditNotKeyboard {
			t.Errorf("audit: %+v", is)
		}
	}
}

// TestTableTransposeDisabled: features addressing records as rows render
// nothing on a transposed table and report no change.
func TestTableTransposeDisabled(t *testing.T) {
	var sel []int
	var sort SortState
	filt := map[string][]string{}
	exp := []string{"0"}
	page, size := 1, 2
	cs := TableColumnState{}
	var tw *TableW
	d := g6Driver(LTR, func(c *Context) {
		cols := tpCols(c)
		cols[0].Sortable, cols[0].Compare = true, func(a, b int) int { return a - b }
		cols[1].Filters = []Option{{Label: "A", Value: "a"}}
		cols[1].Editor = &TableEditor{Set: func(int, string) {}}
		cols[0].Wrap = true
		tw = Table(c, cols, 5).Transposed().TransposedHeader(tpRecTitle).
			Selection(&sel).Sort(&sort).Filters(&filt).
			Expandable(func(int) { Text(c, "detail") }).ExpandedKeys(&exp).
			Tree([]int{0}, func(int) []int { return nil }).
			Span(func(TableSpan) (int, int) { return 2, 2 }).
			Summary(func(string, []int) string { return "SUM" }).
			DragSort(func(int, int) {}).Resizable().ColumnState(&cs).
			Pagination(&page, &size).Virtual().ScrollY(300).Show()
	})
	d.EnableAccessibility()
	for _, role := range []a11y.Role{a11y.RoleCheckbox, a11y.RoleRadio, a11y.RoleTextbox} {
		if n, ok := d.A11yFind(role, ""); ok {
			t.Fatalf("role %v rendered: %+v", role, n)
		}
	}
	for _, label := range []string{"detail", "SUM"} {
		if g6Node(d, label) != nil {
			t.Fatalf("%q rendered", label)
		}
	}
	// No pager: all five records are columns.
	for r := 0; r < 5; r++ {
		if g6Node(d, tpRecTitle(r)) == nil {
			t.Fatalf("record %d column missing (paginated?)", r)
		}
	}
	if b, ok := d.A11yFind(a11y.RoleButton, ""); ok {
		t.Fatalf("button rendered (sorter / filter / pager): %+v", b)
	}
	// No merged cells: every field x record cell exists.
	if g6Node(d, "p4") == nil || g6Node(d, "n3") == nil {
		t.Fatal("cells merged or missing")
	}
	// Clicking a header title does not sort.
	d.Click(g6Rect(t, d, "R1").Center())
	d.Settle()
	if tw.SortChanged() || sort.Order != SortNone || tw.SelectionChanged() || tw.PageChanged() {
		t.Fatal("disabled feature reported a change")
	}
	if got := tw.DisplayedRows(); len(got) != 5 || got[4] != 4 {
		t.Fatalf("DisplayedRows %v: want every record", got)
	}
}

// TestTableTransposeRowClicked: RowClicked reports the record whose column
// was clicked, -1 on the header column.
func TestTableTransposeRowClicked(t *testing.T) {
	got := -2
	d := g6Driver(LTR, func(c *Context) {
		tw := Table(c, tpCols(c), 3).Transposed().Show()
		if r := tw.RowClicked(); r != -1 || got == -2 {
			got = r
		}
	})
	d.Click(g6Rect(t, d, "p2").Center())
	d.Settle()
	if got != 2 {
		t.Fatalf("RowClicked = %d, want record 2", got)
	}
	got = -2
	d.Click(g6Rect(t, d, "Price").Center())
	d.Settle()
	if got != -1 {
		t.Fatalf("header column click: RowClicked = %d, want -1", got)
	}
}

// tpWide is a transposed table wider than the window.
func tpWide(c *Context) {
	Table(c, tpCols(c), 12).Transposed().TransposedHeader(tpRecTitle).TransposedColumnWidth(150).ScrollX(1900).Show()
}

// TestTableTransposeFixedScroll: the header column stays pinned at the
// inline start (right in RTL) while ScrollX scrolls the records; RTL
// starts at the right end.
func TestTableTransposeFixedScroll(t *testing.T) {
	for _, dir := range []Direction{LTR, RTL} {
		d := g6Driver(dir, tpWide)
		body := d.r.c.rects[g6Node(d, "row:name").parent.id]
		name, r0 := g6Rect(t, d, "Name"), g6Rect(t, d, "n0")
		if dir == LTR {
			if !near(name.X, body.X) || !near(r0.X, name.Right()) {
				t.Fatalf("ltr: Name %+v R0 %+v body %+v", name, r0, body)
			}
		} else {
			if !near(name.Right(), body.Right()) || !near(r0.Right(), name.X) {
				t.Fatalf("rtl: Name %+v R0 %+v body %+v: header column not at the right", name, r0, body)
			}
		}
		if g6Node(d, "n11") != nil && g6Rect(t, d, "n11").Intersect(body).W > 1 {
			t.Fatalf("dir %v: last record visible before scrolling", dir)
		}
		dx := float32(300)
		if dir == RTL {
			dx = -300
		}
		d.Inject(input.Scroll{Pos: geom.Pt(body.X+400, body.Y+20), DX: dx})
		d.Settle()
		d.Settle()
		name1 := g6Rect(t, d, "Name")
		if !near(name1.X, name.X) {
			t.Fatalf("dir %v: header column moved %+v -> %+v", dir, name, name1)
		}
		r1 := g6Rect(t, d, "n1")
		want := float32(-300)
		if dir == RTL {
			want = 300
		}
		before := r0.X + r0.W // n1 starts where n0 ended before scrolling
		if dir == RTL {
			before = r0.X - r0.W
		}
		if !near(r1.X, before+want) {
			t.Fatalf("dir %v: record 1 at %v want %v", dir, r1.X, before+want)
		}
		// The pinned group casts its shadow toward the end once scrolled.
		strips := 0
		for _, o := range d.r.List().Ops {
			if cl, ok := o.(paint.PushClip); ok && cl.Rect.W == 14 {
				strips++
				end := name1.Right()
				if dir == RTL {
					end = name1.X - 14
				}
				if !near(cl.Rect.X, end) {
					t.Fatalf("dir %v: shadow strip %+v want x=%v", dir, cl.Rect, end)
				}
			}
		}
		if strips != 1 {
			t.Fatalf("dir %v: %d shadow strips", dir, strips)
		}
	}
}

// TestTableTransposeUnfixed: TransposedHeaderFixed(false) scrolls the
// header column away with the records.
func TestTableTransposeUnfixed(t *testing.T) {
	d := g6Driver(LTR, func(c *Context) {
		Table(c, tpCols(c), 12).Transposed().TransposedHeaderFixed(false).ScrollX(1900).Show()
	})
	body := d.r.c.rects[g6Node(d, "row:name").parent.id]
	name := g6Rect(t, d, "Name")
	d.Inject(input.Scroll{Pos: geom.Pt(body.X+400, body.Y+20), DX: 300})
	d.Settle()
	d.Settle()
	if n := g6Node(d, "Name"); n != nil && n.drawn == d.r.c.pass && near(n.abs.X, name.X) {
		t.Fatalf("unfixed header column stayed at %+v", n.abs)
	}
}

// TestTableTransposeMirror: the transposed table mirrors in RTL.
func TestTableTransposeMirror(t *testing.T) {
	pages := map[string]func(c *Context){
		"header": func(c *Context) {
			Table(c, tpCols(c), 3).Transposed().TransposedHeader(tpRecTitle).Bordered().Show()
		},
		"plain": func(c *Context) { Table(c, tpCols(c), 3).Transposed().Show() },
		"wide":  tpWide,
	}
	for name, page := range pages {
		d := NewDriver(page, DriverOptions{Width: g6W, Height: 700})
		if rep := d.MirrorCheck(page); !rep.OK() || rep.Checked == 0 {
			t.Errorf("%s: %s", name, rep)
		}
	}
}

// TestTableTransposeTokens: the header column paints with the Table header
// tokens (HeaderBg, HeaderColor) of its scope.
func TestTableTransposeTokens(t *testing.T) {
	hb, hc := theme.Hex("#f0f5ff"), theme.Hex("#123456")
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Components(theme.Components{Table: theme.TableToken{HeaderBg: hb, HeaderColor: hc}}).Body(func() {
				Table(c, tpCols(c), 2).Transposed().Show()
			})
		}, layout.Gap(16))
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	d.Settle()
	name := g6Rect(t, d, "Name")
	filled, colored := false, false
	for _, o := range d.r.List().Ops {
		switch op := o.(type) {
		case paint.FillRect:
			if op.Color == hb && op.Rect.Intersect(name).W > name.W-1 {
				filled = true
			}
		case paint.Text:
			if op.Color == hc && name.Contains(op.Origin) {
				colored = true
			}
		}
	}
	if !filled || !colored {
		t.Fatalf("header column: HeaderBg painted %v, HeaderColor text %v", filled, colored)
	}
	// Record cells keep the body look.
	p := g6Rect(t, d, "p0")
	for _, o := range d.r.List().Ops {
		if op, ok := o.(paint.FillRect); ok && op.Color == hb && op.Rect.Intersect(p).W > 1 {
			t.Fatalf("record cell painted with HeaderBg: %+v", op.Rect)
		}
	}
}
