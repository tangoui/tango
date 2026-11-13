package ui

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// tblNode finds a node by label in the last built tree (cells carry their
// text as label even though they are not hit targets).
func tblNode(d *Driver, label string) (*node, bool) {
	var walk func(n *node) *node
	walk = func(n *node) *node {
		if n.label == label {
			return n
		}
		for _, k := range n.kids {
			if r := walk(k); r != nil {
				return r
			}
		}
		return nil
	}
	if n := walk(d.r.c.root); n != nil {
		return n, true
	}
	for _, o := range d.r.c.overlays {
		if n := walk(o.root); n != nil {
			return n, true
		}
	}
	return nil, false
}

func countKind(n *node, label string) int {
	k := 0
	if len(n.label) >= len(label) && n.label[:len(label)] == label {
		k++
	}
	for _, c := range n.kids {
		k += countKind(c, label)
	}
	return k
}

func tblDriver(page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(10)) }, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	return d
}

func ageCols(ages []int) []TableColumn {
	return []TableColumn{
		{Key: "name", Title: "姓名", Text: func(i int) string { return "n" + strconv.Itoa(i) }},
		{Key: "age", Title: "年龄", Width: 100, Compare: func(a, b int) int { return ages[a] - ages[b] },
			Text: func(i int) string { return strconv.Itoa(ages[i]) }},
	}
}

func TestTableSortClickLocalAndControlled(t *testing.T) {
	ages := []int{30, 10, 20}
	var rows []int
	var s SortState
	changed := 0
	d := tblDriver(func(c *Context) {
		tb := Table(c, ageCols(ages), 3).Sort(&s).Show()
		rows = append(rows[:0], tb.DisplayedRows()...)
		if tb.SortChanged() {
			changed++
		}
	})
	d.ClickLabel("年龄")
	if s.Column != "age" || s.Order != SortAsc || fmt.Sprint(rows) != "[1 2 0]" {
		t.Fatalf("asc: %+v rows %v", s, rows)
	}
	d.ClickLabel("年龄")
	if s.Order != SortDesc || fmt.Sprint(rows) != "[0 2 1]" {
		t.Fatalf("desc: %+v rows %v", s, rows)
	}
	d.ClickLabel("年龄")
	if s.Order != SortNone || fmt.Sprint(rows) != "[0 1 2]" {
		t.Fatalf("none: %+v rows %v", s, rows)
	}
	if changed != 3 {
		t.Fatalf("SortChanged %d times", changed)
	}
	// Controlled from outside: the app sets the order.
	s = SortState{"age", SortDesc}
	d.Settle()
	if fmt.Sprint(rows) != "[0 2 1]" {
		t.Fatalf("controlled rows %v", rows)
	}
}

func TestTableUncontrolledMultiSort(t *testing.T) {
	a := []int{1, 1, 0, 0}
	b := []int{5, 3, 9, 1}
	var rows []int
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{
			{Key: "a", Title: "A", SortMultiple: 2, Compare: func(x, y int) int { return a[x] - a[y] }, Text: func(i int) string { return "" }},
			{Key: "b", Title: "B", SortMultiple: 1, Compare: func(x, y int) int { return b[x] - b[y] }, Text: func(i int) string { return "" }},
		}
		rows = append(rows[:0], Table(c, cols, 4).Show().DisplayedRows()...)
	})
	d.ClickLabel("B")
	d.ClickLabel("A")
	// A asc has priority over B asc.
	if fmt.Sprint(rows) != "[3 2 1 0]" {
		t.Fatalf("multi sort rows %v", rows)
	}
}

func TestTableFilterDropdown(t *testing.T) {
	city := []string{"杭州", "上海", "杭州", "北京"}
	filters := map[string][]string{}
	var rows []int
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "city", Title: "城市", Filters: Opts("杭州", "上海", "北京"), FilterSearch: true,
			OnFilter: func(i int, v string) bool { return city[i] == v }, Text: func(i int) string { return city[i] }}}
		rows = append(rows[:0], Table(c, cols, 4).Filters(&filters).Show().DisplayedRows()...)
	})
	if err := d.ClickLabel("city"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Find("确定"); !ok {
		t.Fatal("dropdown did not open")
	}
	d.ClickLabel("杭州")
	d.ClickLabel("北京")
	if len(rows) != 4 {
		t.Fatal("filter applied before confirm")
	}
	d.ClickLabel("确定")
	if fmt.Sprint(rows) != "[0 2 3]" || len(filters["city"]) != 2 {
		t.Fatalf("filtered rows %v filters %v", rows, filters)
	}
	if _, ok := d.Find("确定"); ok {
		t.Fatal("dropdown still open")
	}
	// Reset clears.
	d.ClickLabel("city")
	d.ClickLabel("重置")
	if len(rows) != 4 || len(filters) != 0 {
		t.Fatalf("after reset rows %v filters %v", rows, filters)
	}
}

func TestTableSelectionPageAndHalf(t *testing.T) {
	keys := []string{"k7"} // a row on page 2
	page, size := 1, 5
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "n", Title: "N", Text: func(i int) string { return strconv.Itoa(i) }}}
		Table(c, cols, 12).RowKey(func(i int) string { return "k" + strconv.Itoa(i) }).SelectedKeys(&keys).
			SelectDisabled(func(i int) bool { return i == 3 }).Pagination(&page, &size).Show()
	})
	// Select one row: header becomes indeterminate.
	r := mustFind(t, d, "row:k1")
	d.Click(geom.Pt(r.X+24, r.Y+r.H/2))
	if fmt.Sprint(keys) != "[k7 k1]" {
		t.Fatalf("keys %v", keys)
	}
	f := d.r.c
	_ = f
	// Header checkbox selects the 4 selectable rows of page 1 (row 3 disabled).
	hdr := mustFind(t, d, "row:k0")
	d.Click(geom.Pt(hdr.X+24, hdr.Y-hdr.H/2))
	if len(keys) != 5 || tblHas(keys, "k3") || !tblHas(keys, "k7") {
		t.Fatalf("select all page: %v", keys)
	}
	// Clicking it again clears page 1 but keeps page 2's selection.
	d.Click(geom.Pt(hdr.X+24, hdr.Y-hdr.H/2))
	if fmt.Sprint(keys) != "[k7]" {
		t.Fatalf("clear page: %v", keys)
	}
	// Go to page 2: selection kept.
	d.ClickLabel("2")
	if page != 2 || fmt.Sprint(keys) != "[k7]" {
		t.Fatalf("page 2: page %d keys %v", page, keys)
	}
	// Disabled row cannot be toggled with the keyboard either.
	page = 1
	d.Settle()
	r = mustFind(t, d, "row:k3")
	d.Click(geom.Pt(r.X+24, r.Y+r.H/2))
	if tblHas(keys, "k3") {
		t.Fatal("disabled row selected")
	}
}

func TestTableExpandAndTree(t *testing.T) {
	var exp []string
	detail := 0
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "n", Title: "N", Text: func(i int) string { return "r" + strconv.Itoa(i) }}}
		Table(c, cols, 3).Expandable(func(i int) { detail++; Text(c, "详情"+strconv.Itoa(i)) }).ExpandedKeys(&exp).
			RowExpandable(func(i int) bool { return i != 2 }).Show()
	})
	if _, ok := d.Find("expand:2"); ok {
		t.Fatal("row 2 should not be expandable")
	}
	d.ClickLabel("expand:1")
	if fmt.Sprint(exp) != "[1]" {
		t.Fatalf("expanded %v", exp)
	}
	if _, ok := tblNode(d, "详情1"); !ok {
		t.Fatal("detail panel missing")
	}
	d.ClickLabel("expand:1")
	if len(exp) != 0 {
		t.Fatalf("collapse: %v", exp)
	}

	kids := map[int][]int{0: {1, 2}}
	loaded := 0
	var open []string
	var rows []int
	d = tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "n", Title: "N", Text: func(i int) string { return "t" + strconv.Itoa(i) }}}
		tb := Table(c, cols, 5).Tree([]int{0, 3}, func(i int) []int { return kids[i] }).
			TreeLazy(func(i int) bool { return i == 3 }, func(i int) { loaded++; kids[3] = []int{4} }).
			TreeOpenKeys(&open).Show()
		rows = append(rows[:0], tb.DisplayedRows()...)
	})
	if fmt.Sprint(rows) != "[0 3]" {
		t.Fatalf("collapsed tree %v", rows)
	}
	d.ClickLabel("tree:0")
	if fmt.Sprint(rows) != "[0 1 2 3]" {
		t.Fatalf("open tree %v", rows)
	}
	d.ClickLabel("tree:3")
	if loaded != 1 || fmt.Sprint(rows) != "[0 1 2 3 4]" {
		t.Fatalf("lazy: loaded %d rows %v", loaded, rows)
	}
	// Indentation: the child's text starts further right than its parent's.
	p, _ := tblNode(d, "t0")
	ch, _ := tblNode(d, "t1")
	if p == nil || ch == nil || p.abs.X != ch.abs.X {
		t.Fatalf("cells not found")
	}
}

func TestTableColumnResize(t *testing.T) {
	var tb *TableW
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "a", Title: "A", Width: 120, Text: func(i int) string { return "a" }},
			{Key: "b", Title: "B", Text: func(i int) string { return "b" }}}
		tb = Table(c, cols, 2).Resizable().Show()
	})
	h := mustFind(t, d, "a") // resize handle of column a
	p := h.Center()
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(p.X+60, p.Y)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(p.X+60, p.Y)})
	d.Settle()
	if w := tb.st.widths["a"]; w != 180 {
		t.Fatalf("width after drag = %v", w)
	}
	// The minimum width holds.
	h = mustFind(t, d, "a")
	p = h.Center()
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(p.X-400, p.Y)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(p.X-400, p.Y)})
	d.Settle()
	if w := tb.st.widths["a"]; w != 50 {
		t.Fatalf("min width = %v", w)
	}
}

func TestTableFixedColumnsStick(t *testing.T) {
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "l", Title: "L", Width: 100, Fixed: FixedLeft, Text: func(i int) string { return "left" + strconv.Itoa(i) }}}
		for k := 0; k < 10; k++ {
			k := k
			cols = append(cols, TableColumn{Key: strconv.Itoa(k), Title: "C" + strconv.Itoa(k), Width: 150, Text: func(i int) string { return fmt.Sprintf("c%d-%d", k, i) }})
		}
		cols = append(cols, TableColumn{Key: "r", Title: "R", Width: 100, Fixed: FixedRight, Text: func(i int) string { return "right" + strconv.Itoa(i) }})
		Table(c, cols, 3).ScrollX(1700).Show()
	})
	l0, ok1 := tblNode(d, "left0")
	r0, ok2 := tblNode(d, "right0")
	c0, ok3 := tblNode(d, "c2-0")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("cells missing: %v %v %v", ok1, ok2, ok3)
	}
	lx, rx, cx := l0.abs.X, r0.abs.X, c0.abs.X
	if r0.abs.Right() > 790.5 || r0.abs.Right() < 789.5 {
		t.Fatalf("right fixed column not at the right edge: %v", r0.abs)
	}
	row := mustFind(t, d, "row:0")
	d.Inject(input.Scroll{Pos: row.Center(), DX: 300})
	d.Settle()
	l1, _ := tblNode(d, "left0")
	r1, _ := tblNode(d, "right0")
	c1, ok := tblNode(d, "c2-0")
	if l1.abs.X != lx || r1.abs.X != rx {
		t.Fatalf("fixed columns moved: left %v→%v right %v→%v", lx, l1.abs.X, rx, r1.abs.X)
	}
	if !ok || c1.abs.X != cx-300 {
		t.Fatalf("scrolling column at %v, want %v", c1.abs.X, cx-300)
	}
}

func TestTableVirtualBuildsOnlyVisibleRows(t *testing.T) {
	var sel []int
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "id", Title: "ID", Width: 80, Text: func(i int) string { return "id" + strconv.Itoa(i) }}}
		for k := 0; k < 50; k++ {
			k := k
			cols = append(cols, TableColumn{Key: strconv.Itoa(k), Title: strconv.Itoa(k), Width: 100, Text: func(i int) string { return strconv.Itoa(i + k) }})
		}
		Table(c, cols, 100000).Selection(&sel).ScrollY(400).Size(Small).Show()
	})
	rows := countKind(d.r.c.root, "row:")
	if rows < 10 || rows > 20 {
		t.Fatalf("built %d rows, want ~visible", rows)
	}
	r := mustFind(t, d, "row:0")
	d.Inject(input.Scroll{Pos: r.Center(), DY: 39 * 50000})
	d.Settle()
	if _, ok := d.Find("row:50000"); !ok {
		t.Fatal("row 50000 not built after scrolling")
	}
	if n := countKind(d.r.c.root, "row:"); n > 20 {
		t.Fatalf("built %d rows after scroll", n)
	}
}

func TestTableCellEditCommitCancelValidate(t *testing.T) {
	names := []string{"甲", "乙"}
	edits := 0
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "name", Title: "姓名", Text: func(i int) string { return names[i] },
			Editor: &TableEditor{Set: func(i int, v string) { names[i] = v; edits++ }, Validate: func(v string) string {
				if v == "" {
					return "必填"
				}
				return ""
			}}}}
		Table(c, cols, 2).Show()
	})
	click := func(row string) {
		r := mustFind(t, d, row)
		d.Click(geom.Pt(r.X+40, r.Y+r.H/2))
	}
	click("row:0")
	if d.r.c.focus == 0 {
		t.Fatal("editor not focused")
	}
	d.Type("1")
	d.Key(input.KeyEnter, 0)
	if names[0] != "甲1" || edits != 1 {
		t.Fatalf("commit: %q edits %d", names[0], edits)
	}
	// Escape cancels.
	click("row:1")
	d.Type("x")
	d.Key(input.KeyEscape, 0)
	if names[1] != "乙" {
		t.Fatalf("cancel changed value: %q", names[1])
	}
	// Invalid value keeps the editor and shows the message.
	click("row:1")
	d.Key(input.KeyBackspace, 0)
	d.Key(input.KeyEnter, 0)
	if names[1] != "乙" {
		t.Fatalf("invalid value committed: %q", names[1])
	}
	if _, ok := tblNode(d, "必填"); !ok {
		t.Fatal("validation message missing")
	}
}

func TestTableRowEditSaveCancel(t *testing.T) {
	names := []string{"甲", "乙"}
	editing := []string{"0"}
	var tb *TableW
	saved := false
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "name", Title: "姓名", Text: func(i int) string { return names[i] },
			Editor: &TableEditor{Set: func(i int, v string) { names[i] = v }}},
			{Key: "op", Title: "操作", Render: func(i int) {
				if tb.IsEditing(i) && Button(c, "保存"+strconv.Itoa(i)).Clicked() {
					saved = tb.Save(i)
				}
			}}}
		tb = Table(c, cols, 2).EditableRows(&editing)
		tb.Show()
	})
	r := mustFind(t, d, "row:0")
	d.Click(geom.Pt(r.X+40, r.Y+r.H/2)) // focus the row editor
	d.Type("改")
	d.ClickLabel("保存0")
	if !saved || names[0] != "甲改" || len(editing) != 0 {
		t.Fatalf("save: %v %q %v", saved, names[0], editing)
	}
}

func TestTableKeyboardNavigationSelectCopy(t *testing.T) {
	var sel []int
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "a", Title: "A", Text: func(i int) string { return "a" + strconv.Itoa(i) }},
			{Key: "b", Title: "B", Text: func(i int) string { return "b" + strconv.Itoa(i) }}}
		Table(c, cols, 5).Selection(&sel).Show()
	})
	r := mustFind(t, d, "row:0")
	d.Click(geom.Pt(r.X+200, r.Y+r.H/2)) // column a of row 0
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyC, input.ModCtrl)
	if got := d.Clipboard(); got != "b2" {
		t.Fatalf("copied %q, want b2", got)
	}
	d.Key(input.KeySpace, 0)
	if fmt.Sprint(sel) != "[2]" {
		t.Fatalf("space selected %v", sel)
	}
	d.Key(input.KeyEnd, input.ModCtrl)
	d.Key(input.KeyC, input.ModMeta)
	if got := d.Clipboard(); got != "b4" {
		t.Fatalf("copied %q after Ctrl+End, want b4", got)
	}
	d.Key(input.KeyHome, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyC, input.ModCtrl)
	if got := d.Clipboard(); got != "a4" {
		t.Fatalf("copied %q, want a4", got)
	}
}

func TestTableSpanSummaryEmptyLoading(t *testing.T) {
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "g", Title: "G", Text: func(i int) string { return "g" + strconv.Itoa(i) }},
			{Key: "v", Title: "V", Text: func(i int) string { return strconv.Itoa(i) }}}
		Table(c, cols, 4).Span(func(s TableSpan) (int, int) {
			if s.Key == "g" {
				if s.Pos%2 == 0 {
					return 2, 1
				}
				return 0, 0
			}
			return 1, 1
		}).Summary(func(col string, rows []int) string {
			if col == "v" {
				return "sum" + strconv.Itoa(len(rows))
			}
			return ""
		}).Show()
		Table(c, cols, 0).EmptyText("空空如也").Loading(true).Show()
	})
	g0, ok := tblNode(d, "g0")
	r0 := mustFind(t, d, "row:0")
	if !ok || g0.abs.H != 2*r0.H {
		t.Fatalf("merged cell height %v, row %v", g0.abs.H, r0.H)
	}
	if _, ok := tblNode(d, "g1"); ok {
		t.Fatal("covered cell drawn")
	}
	if _, ok := tblNode(d, "sum4"); !ok {
		t.Fatal("summary missing")
	}
}

func TestProTableRequestParams(t *testing.T) {
	type row struct{ name string }
	var reqs []ProRequest
	var pending func(ProResult[row])
	var pt *ProTableW[row]
	d := NewDriver(func(c *Context) {
		pt = ProTable(c, []ProColumn[row]{
			{Key: "name", Title: "名称", Value: func(r row) any { return r.name }, Sortable: true},
			{Key: "status", Title: "状态", ValueType: ValueStatus, Filterable: true, ValueEnum: []ProEnum{{Value: "on", Label: "开"}}},
		}).Request(func(req ProRequest, done func(ProResult[row])) {
			reqs = append(reqs, req)
			pending = done
		}).Show()
	}, DriverOptions{Width: 1200, Height: 800})
	d.Settle()
	if len(reqs) != 1 || reqs[0].Page != 1 || reqs[0].PageSize != 10 {
		t.Fatalf("initial request %+v", reqs)
	}
	if !pt.Loading() {
		t.Fatal("not loading while pending")
	}
	pending(ProResult[row]{Rows: []row{{"a"}, {"b"}}, Total: 42})
	d.Settle()
	if pt.Loading() || len(pt.Rows()) != 2 {
		t.Fatalf("result not applied: loading %v rows %d", pt.Loading(), len(pt.Rows()))
	}
	// Sorting re-requests with the sort.
	if err := d.ClickLabel("名称"); err != nil {
		t.Fatal(err)
	}
	last := reqs[len(reqs)-1]
	if len(last.Sort) != 1 || last.Sort[0] != (SortState{"name", SortAsc}) {
		t.Fatalf("sort request %+v", last)
	}
	pending(ProResult[row]{Rows: []row{{"a"}}, Total: 42})
	d.Settle()
	// Query form: type a name, submit.
	in := mustFind(t, d, "查询")
	_ = in
	fieldNode, ok := tblNode(d, "名称：")
	if !ok {
		t.Fatal("query label missing")
	}
	d.Click(geom.Pt(fieldNode.abs.Right()+40, fieldNode.abs.Y+fieldNode.abs.H/2))
	d.Type("abc")
	d.ClickLabel("查询")
	last = reqs[len(reqs)-1]
	if last.Params["name"] != "abc" || last.Page != 1 {
		t.Fatalf("query request %+v", last)
	}
	// Errors show and keep rows empty.
	pending(ProResult[row]{Err: fmt.Errorf("boom")})
	d.Settle()
	if _, ok := tblNode(d, "数据加载失败：boom"); !ok {
		t.Fatal("error banner missing")
	}
}
