package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// Batch 3 V3: opt-in virtual ProTable, EditableProTable, CellEditorTable
// and RowEditorTable (ScrollY + Virtual, passed through to TableW).

func v3Rows(n int) []b4dRow {
	out := make([]b4dRow, n)
	for i := range out {
		s := strconv.Itoa(i)
		out[i] = b4dRow{ID: "r" + s, Name: "N" + s, Note: "note" + s, Age: float64(i % 100)}
	}
	return out
}

// v3Node returns the first node whose label (or kind) is label.
func v3Node(n *node, label string) *node {
	if n.label == label || n.label == "" && n.kind == label {
		return n
	}
	for _, k := range n.kids {
		if f := v3Node(k, label); f != nil {
			return f
		}
	}
	return nil
}

// v3InView reports whether the row with key k is built and fully inside
// the table body's viewport.
func v3InView(d *Driver, k string) bool {
	root := d.r.c.root
	row, body := v3Node(root, "row:"+k), v3Node(root, "tbody")
	if row == nil || body == nil {
		return false
	}
	rr, br := d.r.c.rects[row.id], d.r.c.rects[body.id]
	return rr.H > 0 && rr.Y >= br.Y-0.5 && rr.Bottom() <= br.Bottom()+0.5
}

// v3FocusIn reports whether keyboard focus is inside the row with key k.
func v3FocusIn(d *Driver, k string) bool {
	row := v3Node(d.r.c.root, "row:"+k)
	return row != nil && d.r.c.focusWithin(row)
}

func v3Wheel(d *Driver, dy float32) {
	body := v3Node(d.r.c.root, "tbody")
	d.Inject(input.Scroll{Pos: d.r.c.rects[body.id].Center(), DY: dy})
	d.Settle()
}

func TestProTableVirtualPassthrough(t *testing.T) {
	const n = 5000
	rows := v3Rows(n)
	cols := []ProColumn[b4dRow]{
		{Key: "name", Title: "Name", Value: func(r b4dRow) any { return r.Name }},
		{Key: "note", Title: "Note", Value: func(r b4dRow) any { return r.Note }},
	}
	var scrollY, rowH float32
	virtual := true
	var p *ProTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		p = ProTable(c, cols).RowKey(func(r b4dRow) string { return r.ID }).PageSize(n).NoSearch().
			Request(func(req ProRequest, done func(ProResult[b4dRow])) { done(ProResult[b4dRow]{Rows: rows, Total: n}) })
		if scrollY > 0 {
			p.ScrollY(scrollY)
		}
		if virtual {
			p.Virtual()
		}
		if rowH > 0 {
			p.RowHeight(rowH)
		}
		p.Show()
	})
	// Without ScrollY nothing changes: every row of the page is built.
	if got := countKind(d.r.c.root, "row:"); got != n {
		t.Fatalf("no ScrollY: built %d rows, want %d", got, n)
	}
	full := mustFind(t, d, "row:r0")
	scrollY = 300
	d.r.Invalidate()
	d.Settle()
	// Plain TableW.ScrollY turned on at run time keeps the last
	// unbounded viewport for one frame (unchanged by batch 3).
	d.r.Invalidate()
	d.Settle()
	got := countKind(d.r.c.root, "row:")
	if got < 5 || got > 20 {
		t.Fatalf("ScrollY+Virtual: built %d rows, want the visible ones", got)
	}
	if !p.Table().virtual || p.Table().maxH != 300 {
		t.Fatal("ScrollY / Virtual not passed to the table of the same Show")
	}
	// The row height follows the density (same as the unbounded table).
	if r := mustFind(t, d, "row:r0"); r.H != full.H {
		t.Fatalf("virtual row height %v, unbounded %v", r.H, full.H)
	}
	// Scrolling moves the window.
	v3Wheel(d, 4000)
	if _, ok := d.Find("row:r0"); ok {
		t.Fatal("row 0 still built after scrolling")
	}
	if got := countKind(d.r.c.root, "row:"); got > 20 {
		t.Fatalf("built %d rows after scrolling", got)
	}
	// The grid reports every row of the page.
	d.EnableAccessibility()
	g, ok := d.A11yFind(a11y.RoleGrid, "")
	if !ok || g.RowCount != n+1 {
		t.Fatalf("grid RowCount %v, want %d", g, n+1)
	}
	rowH = 60
	d.r.Invalidate()
	d.Settle()
	var tallest float32
	for i := 0; i < n; i++ {
		if r, ok := d.Find("row:r" + strconv.Itoa(i)); ok {
			tallest = max(tallest, r.H)
		}
	}
	if tallest != 60 {
		t.Fatalf("RowHeight: rows %v tall, want 60", tallest)
	}
}

func TestEditableProTableVirtual(t *testing.T) {
	rows := v3Rows(2000)
	var e *EditableProTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		e = EditableProTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	})
	if got := countKind(d.r.c.root, "row:"); got < 5 || got > 20 {
		t.Fatalf("built %d rows, want the visible ones", got)
	}
	// Data is read live: an in-place edit shows on the next frame.
	rows[1].Name = "Changed"
	d.Settle()
	if !pro3HasText(d, "Changed") {
		t.Fatal("in-place edit of a row not shown")
	}
	// Editing a row far down keeps it built and scrolls it in.
	e.Edit("r1500")
	d.Settle()
	d.Settle()
	if !v3InView(d, "r1500") {
		t.Fatal("edited row not revealed")
	}
	if e.Table().virtual != true {
		t.Fatal("Virtual not passed through")
	}
}

func TestCellEditorTableVirtualKeepsEditedCell(t *testing.T) {
	rows := v3Rows(3000)
	var w *CellEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		w = CellEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	})
	if got := countKind(d.r.c.root, "row:"); got < 5 || got > 20 {
		t.Fatalf("built %d rows, want the visible ones", got)
	}
	// Start editing an unbuilt row: it is scrolled in and its editor
	// takes the focus.
	w.Edit("r2000", "name")
	d.Settle()
	d.Settle()
	if k, col := w.EditingCell(); k != "r2000" || col != "name" {
		t.Fatalf("editing %q %q", k, col)
	}
	if !v3InView(d, "r2000") || !v3FocusIn(d, "r2000") {
		t.Fatalf("edited row in view %v, focused %v", v3InView(d, "r2000"), v3FocusIn(d, "r2000"))
	}
	d.Key(input.KeyEnd, 0)
	d.Type("X")
	// Scrolling away keeps the edited row built (keep-row): the draft and
	// the focus survive.
	v3Wheel(d, -1e7)
	if _, ok := d.Find("row:r0"); !ok {
		t.Fatal("did not scroll to the top")
	}
	if v3Node(d.r.c.root, "row:r2000") == nil {
		t.Fatal("edited row not kept while scrolled away")
	}
	if v3InView(d, "r2000") {
		t.Fatal("kept row drawn inside the viewport")
	}
	if !v3FocusIn(d, "r2000") {
		t.Fatal("focus lost while the edited row is offscreen")
	}
	if got := countKind(d.r.c.root, "row:"); got > 21 {
		t.Fatalf("built %d rows", got)
	}
	d.Type("Y")
	d.Key(input.KeyEnter, 0)
	if k, _ := w.EditingCell(); k != "" || rows[2000].Name != "N2000XY" {
		t.Fatalf("save offscreen: editing %q name %q", k, rows[2000].Name)
	}
	// Default path: without ScrollY every row is built.
	rows = v3Rows(50)
	d2 := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		CellEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).Show()
	})
	if got := countKind(d2.r.c.root, "row:"); got != 50 {
		t.Fatalf("default form built %d rows", got)
	}
}

func TestRowEditorTableVirtualTabAndErrors(t *testing.T) {
	rows := v3Rows(3000)
	var w *RowEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	})
	b4dDoubleClick(d, mustFindText(t, d, "N2").Center())
	if w.EditingKey() != "r2" || !v3FocusIn(d, "r2") {
		t.Fatalf("editing %q", w.EditingKey())
	}
	// Scroll the edited row away, then Tab to its next editor: the row
	// scrolls back into view with the focus in the age editor.
	v3Wheel(d, 20000)
	if v3InView(d, "r2") {
		t.Fatal("row still in view")
	}
	before := d.r.c.focus
	d.Key(input.KeyTab, 0)
	d.Settle()
	if d.r.c.focus == before || !v3FocusIn(d, "r2") {
		t.Fatal("Tab did not move to the next editor of the row")
	}
	if !v3InView(d, "r2") {
		t.Fatal("Tab into the offscreen edited row did not scroll it in")
	}
	// A validation error on an offscreen edited row scrolls it back in.
	v3Wheel(d, 20000)
	w.e.st.drafts["r2"]["name"] = "bad"
	w.Save()
	d.Settle()
	d.Settle()
	if w.Errors("r2")["name"] != "Bad name" || w.EditingKey() != "r2" {
		t.Fatalf("errors %v editing %q", w.Errors("r2"), w.EditingKey())
	}
	if !v3InView(d, "r2") {
		t.Fatal("row with a validation error not revealed")
	}
	// The kept row follows its key when rows are inserted above it.
	rows = append([]b4dRow{{ID: "new", Name: "New"}}, rows...)
	d.Settle()
	v3Wheel(d, 20000)
	row := v3Node(d.r.c.root, "row:r2")
	if row == nil {
		t.Fatal("kept row lost after an insert above it")
	}
	if got := countKind(d.r.c.root, "row:"); got > 21 {
		t.Fatalf("built %d rows", got)
	}
}

func TestRowEditorTableVirtualAddRowAndKeyboard(t *testing.T) {
	rows := v3Rows(3000)
	var w *RowEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 800, func(c *Context) {
		w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).
			NewRow(func() b4dRow { return b4dRow{ID: "fresh"} }).ScrollY(300).Virtual().Show()
	})
	// Keyboard navigation past the window scrolls (Table's active cell).
	d.Click(mustFindText(t, d, "note0").Center())
	for range 40 {
		d.Key(input.KeyDown, 0)
	}
	if !v3InView(d, "r40") {
		t.Fatal("keyboard navigation did not scroll the active row in")
	}
	// A new row is added after the last of 3000 rows: it is scrolled in
	// with the focus in its first editor.
	k := w.AddRow()
	for range 4 {
		d.Settle()
	}
	if k == "" || w.EditingKey() != k || !v3InView(d, k) || !v3FocusIn(d, k) {
		t.Fatalf("new row %q editing %q in view %v focused %v", k, w.EditingKey(), v3InView(d, k), v3FocusIn(d, k))
	}
}

// Plain TableW.ScrollY is unchanged by batch 3 V3 (keepEdit is only for
// EditableProTable and its presets): an edited cell whose row leaves the
// window is not built any more and loses the focus, as before.
func TestTableScrollYDropsEditedCell(t *testing.T) {
	names := make([]string, 2000)
	for i := range names {
		names[i] = "n" + strconv.Itoa(i)
	}
	var tw *TableW
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "name", Title: "Name", Width: 200, Text: func(i int) string { return names[i] },
			Editor: &TableEditor{Set: func(i int, v string) { names[i] = v }}}}
		tw = Table(c, cols, len(names)).ScrollY(300).Show()
	})
	b4dDoubleClick(d, mustFindText(t, d, "n1").Center())
	if !v3FocusIn(d, "1") {
		t.Fatal("cell editor not focused")
	}
	v3Wheel(d, 30000)
	if v3Node(d.r.c.root, "row:1") != nil || d.r.c.focus != 0 {
		t.Fatalf("plain ScrollY kept the edited row: built %v focus %v", v3Node(d.r.c.root, "row:1") != nil, d.r.c.focus)
	}
	if tw.keepEdit || tw.st.revOn {
		t.Fatal("plain table in keep-edit mode")
	}
}

// v3PtrRow tables use a pointer row type: a zero T handed to Value or
// Render would be a nil dereference.
type v3PtrRow struct{ ID, Name string }

func v3PtrCols() []ProColumn[*v3PtrRow] {
	return []ProColumn[*v3PtrRow]{
		{Key: "name", Title: "Name", Width: 200, Value: func(r *v3PtrRow) any { return r.Name },
			Editor: &ProField{}, Set: func(r **v3PtrRow, v any) { (*r).Name, _ = v.(string) }},
		{Key: "id", Title: "ID", Width: 120, Value: func(r *v3PtrRow) any { return "id-" + r.ID }},
		{Key: "op", Title: "Actions", ValueType: ValueOption},
	}
}

// Deleting a row or cancelling a new row changes the rows while the body
// is being built (the action runs inside a cell); the rest of that build
// must not see a missing row (C1), in the default and the bounded form.
func TestEditableProTablePointerRowsDeleteCancel(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "bounded"}[bounded], func(t *testing.T) {
			for _, top := range []bool{false, true} {
				rows := []*v3PtrRow{{"a", "Alice"}, {"b", "Bob"}, {"c", "Carol"}}
				seq := 0
				var e *EditableProTableW[*v3PtrRow]
				d := pro3Driver(b4dAnt, 900, 600, func(c *Context) {
					e = EditableProTable(c, v3PtrCols(), &rows).RowKey(func(r *v3PtrRow) string { return r.ID }).
						NewRow(func() *v3PtrRow { seq++; return &v3PtrRow{ID: "n" + strconv.Itoa(seq)} })
					if top {
						e.AddAtTop()
					}
					if bounded {
						e.ScrollY(300)
					}
					e.Show()
				})
				p2eClickInRow(t, d, "a", ZhCN.Delete)
				if err := d.ClickLabel(ZhCN.OK); err != nil {
					t.Fatal(err)
				}
				d.Settle()
				if len(rows) != 2 || rows[0].ID != "b" || pro3HasText(d, "Alice") || !pro3HasText(d, "id-c") {
					t.Fatalf("delete: rows %v", rows)
				}
				// A new row, cancelled: dropped while its row is built.
				k := e.AddRow()
				d.Settle()
				p2eClickInRow(t, d, k, ZhCN.Cancel)
				d.Settle()
				if e.Editing(k) || len(rows) != 2 || !pro3HasText(d, "Bob") {
					t.Fatalf("cancel new row: editing %v rows %v", e.Editing(k), rows)
				}
				// A new row saved (top: it joins the data at index 0).
				k = e.AddRow()
				d.Settle()
				e.st.drafts[k]["name"] = "Dan"
				p2eClickInRow(t, d, k, ZhCN.Save)
				d.Settle()
				if len(rows) != 3 || e.Editing(k) || !pro3HasText(d, "Dan") {
					t.Fatalf("save new row (top %v): rows %v", top, rows)
				}
			}
		})
	}
}

func TestRowEditorTableVirtualTypingReveals(t *testing.T) {
	rows := v3Rows(3000)
	var w *RowEditorTableW[b4dRow]
	d := pro3Driver(b4dAnt, 900, 700, func(c *Context) {
		w = RowEditorTable(c, b4dCols(), &rows).RowKey(func(r b4dRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	})
	b4dDoubleClick(d, mustFindText(t, d, "N2").Center())
	if w.EditingKey() != "r2" {
		t.Fatalf("editing %q", w.EditingKey())
	}
	v3Wheel(d, 20000)
	if v3InView(d, "r2") || !v3FocusIn(d, "r2") {
		t.Fatal("setup: row in view or focus lost")
	}
	// Typing into the editor scrolled out of view brings its row back.
	d.Type("Q")
	d.Settle()
	if !v3InView(d, "r2") || w.e.st.drafts["r2"]["name"] != "N2Q" && w.e.st.drafts["r2"]["name"] != "Q" {
		t.Fatalf("typing: in view %v draft %v", v3InView(d, "r2"), w.e.st.drafts["r2"]["name"])
	}
	// Wheel scrolling alone does not pull it back.
	v3Wheel(d, 20000)
	d.Settle()
	if v3InView(d, "r2") {
		t.Fatal("scrolling pulled the edited row back")
	}
}
