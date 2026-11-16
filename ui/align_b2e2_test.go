package ui

// Batch 2, group E2: Table behaviour and API aligned with antd 6.7
// (es/table/interface.d.ts, hooks/useSelection.js, useSorter.js,
// InternalTable.js) and Element Plus 2.14.8 (el-table, el-table-v2).
// See docs/specs/align/2-table-api.md.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// e2Cols: a name column and a sortable, filterable age column.
func e2Cols(ages []int) []TableColumn {
	return []TableColumn{
		{Key: "name", Title: "姓名", Text: func(i int) string { return "n" + strconv.Itoa(i) }},
		{Key: "age", Title: "年龄", Width: 100, Compare: func(a, b int) int { return ages[a] - ages[b] },
			Text: func(i int) string { return strconv.Itoa(ages[i]) }},
	}
}

func TestTblNextOrderIn(t *testing.T) {
	cases := []struct {
		dirs []SortOrder
		cur  SortOrder
		want SortOrder
	}{
		{nil, SortNone, SortAsc}, {nil, SortAsc, SortDesc}, {nil, SortDesc, SortNone},
		{[]SortOrder{SortDesc}, SortNone, SortDesc}, {[]SortOrder{SortDesc}, SortDesc, SortNone},
		{[]SortOrder{SortDesc, SortAsc}, SortNone, SortDesc}, {[]SortOrder{SortDesc, SortAsc}, SortAsc, SortNone},
		// antd's "no cancel" idiom: ascend, descend, ascend.
		{[]SortOrder{SortAsc, SortDesc, SortAsc}, SortDesc, SortAsc},
		// An order outside the list restarts it.
		{[]SortOrder{SortDesc}, SortAsc, SortDesc},
	}
	for _, k := range cases {
		if got := tblNextOrderIn(k.dirs, k.cur); got != k.want {
			t.Errorf("tblNextOrderIn(%v, %v) = %v, want %v", k.dirs, k.cur, got, k.want)
		}
	}
}

// TestTableSortDirections: column and table sort cycles (antd
// sortDirections, EP sort-orders) and the sorter tooltip that follows them.
func TestTableSortDirections(t *testing.T) {
	ages := []int{30, 10, 20}
	var s SortState
	var tb *TableW
	colDirs := []SortOrder(nil)
	tipOn := true
	d := tblDriver(func(c *Context) {
		cols := e2Cols(ages)
		cols[1].SortDirections = colDirs
		tb = Table(c, cols, 3).Sort(&s).SortDirections(SortDesc, SortAsc).ShowSorterTooltip(tipOn).Show()
	})
	col := tb.fr.colByKey("age")
	if tip, ok := tb.fr.sorterTip(col); !ok || tip != tb.c.Locale().SortDesc {
		t.Fatalf("tip before first click = %q %v, want the descending action", tip, ok)
	}
	d.ClickLabel("年龄")
	if s.Order != SortDesc {
		t.Fatalf("table SortDirections: first click %v, want desc", s.Order)
	}
	d.ClickLabel("年龄")
	d.ClickLabel("年龄")
	if s.Order != SortNone {
		t.Fatalf("after desc, asc: %v, want none", s.Order)
	}
	colDirs = []SortOrder{SortAsc}
	d.Settle()
	d.ClickLabel("年龄")
	if s.Order != SortAsc {
		t.Fatalf("column SortDirections wins: %v", s.Order)
	}
	col = tb.fr.colByKey("age")
	if tip, _ := tb.fr.sorterTip(col); tip != tb.c.Locale().CancelSort {
		t.Fatalf("tip after the last direction = %q, want cancel", tip)
	}
	tipOn = false
	d.Settle()
	if _, ok := tb.fr.sorterTip(tb.fr.colByKey("age")); ok {
		t.Fatal("ShowSorterTooltip(false) still shows the tip")
	}
}

// TestTableDefaultFilter: antd defaultFilteredValue applies on the first
// frame; an entry the bound map already has wins; reset can restore it.
func TestTableDefaultFilter(t *testing.T) {
	ages := []int{30, 10, 20, 10}
	var rows []int
	var tb *TableW
	filters := map[string][]string{}
	bound := false
	d := tblDriver(func(c *Context) {
		cols := e2Cols(ages)
		cols[1].Filters = []Option{{Label: "10", Value: "10"}, {Label: "20", Value: "20"}}
		cols[1].OnFilter = func(r int, v string) bool { return strconv.Itoa(ages[r]) == v }
		cols[1].DefaultFilter = []string{"10"}
		cols[1].FilterResetToDefault = true
		tb = Table(c, cols, len(ages))
		if bound {
			tb.Filters(&filters)
		}
		tb.Show()
		rows = append(rows[:0], tb.DisplayedRows()...)
	})
	if fmt.Sprint(rows) != "[1 3]" {
		t.Fatalf("uncontrolled default filter: rows %v", rows)
	}
	if got := tb.fr.filterResetValues(tb.fr.colByKey("age")); fmt.Sprint(got) != "[10]" {
		t.Fatalf("reset to default = %v", got)
	}
	// Controlled: an existing entry is kept.
	filters["age"] = []string{"20"}
	bound = true
	d2 := tblDriver(func(c *Context) {
		cols := e2Cols(ages)
		cols[1].OnFilter = func(r int, v string) bool { return strconv.Itoa(ages[r]) == v }
		cols[1].DefaultFilter = []string{"10"}
		tb = Table(c, cols, len(ages)).Filters(&filters).Show()
		rows = append(rows[:0], tb.DisplayedRows()...)
	})
	_ = d
	d2.Settle()
	if fmt.Sprint(rows) != "[2]" || fmt.Sprint(filters["age"]) != "[20]" {
		t.Fatalf("controlled entry overwritten: rows %v filters %v", rows, filters)
	}
	if got := tb.fr.filterResetValues(tb.fr.colByKey("age")); got != nil {
		t.Fatalf("reset without FilterResetToDefault = %v, want nil", got)
	}
}

// TestTableCheckStrictlyDefault: antd rowSelection.checkStrictly defaults
// to true (useSelection.js:77); CheckLinked and Defaults turn linkage on.
func TestTableCheckStrictlyDefault(t *testing.T) {
	var sel []int
	var tb *TableW
	defaults := false
	d := tblDriver(func(c *Context) {
		build := func() {
			cols := []TableColumn{{Key: "name", Title: "名称", Text: func(i int) string { return tcNodes[i].name }}}
			tb = Table(c, cols, len(tcNodes)).Tree([]int{0}, func(i int) []int { return tcNodes[i].kids }).
				TreeOpenKeys(&[]string{"0", "1"}).Selection(&sel).Show()
		}
		if defaults {
			ConfigProvider(c).Defaults("Table", Props{"checkStrictly": false}).Body(build)
			return
		}
		build()
	})
	if tb.fr.linked {
		t.Fatal("tree selection is linked by default")
	}
	clickRowCheck(t, d, "1")
	if fmt.Sprint(sel) != "[1]" {
		t.Fatalf("strict click: %v", sel)
	}
	defaults = true
	sel = nil
	d.Settle()
	if !tb.fr.linked {
		t.Fatal(`Defaults("Table", checkStrictly: false) did not link`)
	}
}

// TestTablePreserveSelectedRowKeys: without preserveSelectedRowKeys a
// selection change drops keys of rows that are not in the data
// (useSelection.js setSelectedKeys); unknown keys alone never panic and
// stay until the selection changes.
func TestTablePreserveSelectedRowKeys(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		keys := []string{"zz", "1"}
		var tb *TableW
		d := tblDriver(func(c *Context) {
			tb = Table(c, e2Cols([]int{1, 2, 3}), 3).SelectedKeys(&keys)
			if preserve {
				tb.PreserveSelectedRowKeys()
			}
			tb.Show()
		})
		if len(keys) != 2 {
			t.Fatalf("keys changed without a selection change: %v", keys)
		}
		clickRowCheck(t, d, "2")
		want := "[1 2]"
		if preserve {
			want = "[zz 1 2]"
		}
		if fmt.Sprint(keys) != want {
			t.Fatalf("preserve=%v: keys %v, want %s", preserve, keys, want)
		}
		_ = tb
	}
}

// TestTableShiftRangeSelect: Shift+click checks the rows between the last
// checked row and the clicked one (antd useMultipleSelect); a second
// Shift+click over a fully checked range unchecks it.
func TestTableShiftRangeSelect(t *testing.T) {
	var sel []int
	d := tblDriver(func(c *Context) {
		Table(c, e2Cols([]int{1, 2, 3, 4, 5}), 5).Selection(&sel).SelectDisabled(func(r int) bool { return r == 2 }).Show()
	})
	clickRowCheck(t, d, "0")
	shiftClick := func(key string) {
		r := mustFind(t, d, "row:"+key)
		p := geom.Pt(r.X+16, r.Y+r.H/2)
		d.Inject(input.Pointer{Kind: input.Move, Pos: p})
		d.Inject(input.Pointer{Kind: input.Press, Pos: p, Mods: input.ModShift})
		d.Inject(input.Pointer{Kind: input.Release, Pos: p, Mods: input.ModShift})
		d.Settle()
	}
	shiftClick("3")
	if fmt.Sprint(sel) != "[0 1 3]" {
		t.Fatalf("range 0..3 (row 2 disabled): %v", sel)
	}
	clickRowCheck(t, d, "4") // plain click: 4 becomes the anchor
	shiftClick("0")
	if len(sel) != 0 {
		t.Fatalf("range over checked rows should uncheck: %v", sel)
	}
}

// TestTableExpandRowByClick: antd expandable.expandRowByClick, and
// showExpandColumn false.
func TestTableExpandRowByClick(t *testing.T) {
	var keys []string
	var sel []int
	var tb *TableW
	hide := false
	d := tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{1, 2, 3}), 3).Selection(&sel).
			Expandable(func(r int) { Text(c, "detail"+strconv.Itoa(r)) }).ExpandedKeys(&keys).ExpandRowByClick()
		if hide {
			tb.HideExpandColumn()
		}
		tb.Show()
	})
	d.Click(mustFindText(t, d, "n1").Center())
	if fmt.Sprint(keys) != "[1]" {
		t.Fatalf("row click did not expand: %v", keys)
	}
	if _, ok := d.FindText("detail1"); !ok {
		t.Fatal("detail panel not shown")
	}
	d.Click(mustFindText(t, d, "n1").Center())
	if len(keys) != 0 {
		t.Fatalf("second click did not collapse: %v", keys)
	}
	// The selection checkbox (centred in the 32px column after the 48px expand column) does not
	// toggle the row.
	r0 := mustFind(t, d, "row:0")
	d.Click(geom.Pt(r0.X+48+16, r0.Y+r0.H/2))
	if len(keys) != 0 || fmt.Sprint(sel) != "[0]" {
		t.Fatalf("checkbox click: keys %v sel %v", keys, sel)
	}
	// The expand icon toggles once (the row click does not toggle again).
	d.Click(mustFind(t, d, "expand:2").Center())
	if fmt.Sprint(keys) != "[2]" {
		t.Fatalf("expand icon click: %v", keys)
	}
	hide = true
	d.Settle()
	for _, lf := range tb.fr.leaves {
		if lf.kind == tkExpand {
			t.Fatal("HideExpandColumn kept the expand column")
		}
	}
	// antd order: expand column, then selection.
	hide = false
	d.Settle()
	if tb.fr.leaves[0].kind != tkExpand || tb.fr.leaves[1].kind != tkSelect {
		t.Fatalf("special column order: %d %d", tb.fr.leaves[0].kind, tb.fr.leaves[1].kind)
	}
}

// TestTableDefaultExpandAllRows: antd defaultExpandAllRows / EP
// default-expand-all for detail panels and tree rows, uncontrolled only.
func TestTableDefaultExpandAllRows(t *testing.T) {
	var tb *TableW
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "name", Title: "名称", Text: func(i int) string { return tcNodes[i].name }}}
		tb = Table(c, cols, len(tcNodes)).Tree([]int{0}, func(i int) []int { return tcNodes[i].kids }).DefaultExpandAllRows().Show()
	})
	d.Settle()
	if tb.fr.n != len(tcNodes) {
		t.Fatalf("tree rows shown %d, want all %d", tb.fr.n, len(tcNodes))
	}
	controlled := []string{}
	d2 := tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{1, 2}), 2).Expandable(func(r int) { Text(c, "d"+strconv.Itoa(r)) }).
			ExpandedKeys(&controlled).DefaultExpandAllRows().Show()
	})
	if len(controlled) != 0 {
		t.Fatalf("controlled expansion changed: %v", controlled)
	}
	d3 := tblDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Table", Props{"defaultExpandAllRows": true}).Body(func() {
			tb = Table(c, e2Cols([]int{1, 2}), 2).Expandable(func(r int) { Text(c, "d"+strconv.Itoa(r)) }).
				RowExpandable(func(r int) bool { return r == 1 }).Show()
		})
	})
	if _, ok := d3.FindText("d1"); !ok {
		t.Fatal("Defaults defaultExpandAllRows did not expand row 1")
	}
	if _, ok := d3.FindText("d0"); ok {
		t.Fatal("row 0 is not expandable but expanded")
	}
	_ = d2
}

// TestTableHeaderOptions: showHeader false, header-height (Table V2),
// title / footer bars, table-level show-overflow-tooltip.
func TestTableHeaderOptions(t *testing.T) {
	var tb *TableW
	cols := e2Cols([]int{1, 2})
	d := tblDriver(func(c *Context) {
		tb = Table(c, cols, 2).HideHeader().Title(func() { Text(c, "TitleBar") }).Footer(func() { Text(c, "FooterBar") }).
			ShowOverflowTooltip().Show()
	})
	if tb.fr.depth != 0 {
		t.Fatalf("HideHeader: depth %d", tb.fr.depth)
	}
	if _, ok := d.FindText("姓名"); ok {
		t.Fatal("HideHeader still draws the header")
	}
	title, foot, row := mustFindText(t, d, "TitleBar"), mustFindText(t, d, "FooterBar"), mustFindText(t, d, "n0")
	if !(title.Y < row.Y && row.Y < foot.Y) {
		t.Fatalf("title %v row %v footer %v out of order", title, row, foot)
	}
	for _, li := range tb.fr.data {
		if !tb.fr.leaves[li].col.Ellipsis {
			t.Fatal("ShowOverflowTooltip: column without Ellipsis")
		}
	}
	if cols[0].Ellipsis {
		t.Fatal("ShowOverflowTooltip modified the app's columns")
	}
	tblDriver(func(c *Context) { tb = Table(c, cols, 2).Virtual().RowHeight(50).HeaderHeight(50).ScrollY(200).Show() })
	if tb.fr.hh != 50 || tb.fr.rh != 50 {
		t.Fatalf("Table V2 metrics: header %v row %v", tb.fr.hh, tb.fr.rh)
	}
}

// TestTablePaginationDefaults: antd table pagination has no total by
// default, is hidden without rows, can sit on top (placement) and is
// customizable (PaginationConfig).
func TestTablePaginationDefaults(t *testing.T) {
	page, size := 1, 10
	n := 25
	var place []string
	var cfg func(p *PaginationW)
	d := tblDriver(func(c *Context) {
		Table(c, e2Cols(make([]int, 30)), n).Pagination(&page, &size).PaginationPlacement(place...).PaginationConfig(cfg).Show()
	})
	loc := ZhCN
	if _, ok := d.FindText(loc.Total(25)); ok {
		t.Fatal("total shown by default")
	}
	cfg = func(p *PaginationW) { p.ShowTotal() }
	place = []string{PageTopStart}
	d.Settle()
	tot := mustFindText(t, d, loc.Total(25))
	if row := mustFindText(t, d, "n0"); tot.Y > row.Y {
		t.Fatalf("top pager below the rows: %v vs %v", tot, row)
	}
	place = []string{PageNone}
	d.Settle()
	if _, ok := d.FindText(loc.Total(25)); ok {
		t.Fatal("PageNone still shows the pager")
	}
	place, n = nil, 0
	d.Settle()
	if _, ok := d.FindText(loc.Total(0)); ok {
		t.Fatal("pager shown without rows")
	}
}

// TestTableCurrentRowKey: EP current-row-key / current-change.
func TestTableCurrentRowKey(t *testing.T) {
	cur := ""
	changed := 0
	var tb *TableW
	d := tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{1, 2, 3}), 3).CurrentRowKey(&cur).Show()
		if tb.CurrentChanged() {
			changed++
		}
	})
	d.Click(mustFindText(t, d, "n1").Center())
	if cur != "1" || changed != 1 {
		t.Fatalf("click: cur %q changed %d", cur, changed)
	}
	cur = "2"
	d.Settle()
	if tb.st.cur != "2" || !tb.st.curOn || changed != 1 {
		t.Fatalf("app-set current row: %q %v changed %d", tb.st.cur, tb.st.curOn, changed)
	}
	cur = "zz" // unknown key: nothing highlighted, no panic
	d.Settle()
	cur = ""
	d.Settle()
	if tb.st.curOn {
		t.Fatal(`"" should clear the current row`)
	}
}

// TestTableShowSummary: EP show-summary with the default summary method.
func TestTableShowSummary(t *testing.T) {
	if got := tblSumTexts([]string{"1.5", "x", " 2 ", ""}); got != "3.5" {
		t.Fatalf("sum = %q", got)
	}
	// EP Number(toFixed): trailing zeros drop.
	if got := tblSumTexts([]string{"1.50", "2"}); got != "3.5" {
		t.Fatalf("sum of 1.50 and 2 = %q, want 3.5", got)
	}
	if got := tblSumTexts([]string{"a", ""}); got != "" {
		t.Fatalf("no numbers = %q", got)
	}
	d := tblDriver(func(c *Context) { Table(c, e2Cols([]int{30, 10, 20}), 3).ShowSummary().Show() })
	mustFindText(t, d, ZhCN.Table.SumText)
	mustFindText(t, d, "60")
	d2 := tblDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() { Table(c, e2Cols([]int{30, 10, 20}), 3).ShowSummary().Show() })
	})
	mustFindText(t, d2, "Sum")
	// A custom summary method replaces the default row (EP renders one).
	var tb *TableW
	d3 := tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{30, 10, 20}), 3).ShowSummary().Summary(func(col string, rows []int) string {
			if col == "name" {
				return "custom"
			}
			return ""
		}).Show()
	})
	mustFindText(t, d3, "custom")
	if _, ok := d3.FindText(ZhCN.Table.SumText); ok || len(tb.summary) != 1 {
		t.Fatalf("ShowSummary + Summary: %d summary rows", len(tb.summary))
	}
}

// TestTableCurrentChangedUnbound: EP current-change fires with
// highlight-current-row alone.
func TestTableCurrentChangedUnbound(t *testing.T) {
	changed := 0
	d := tblDriver(func(c *Context) {
		if Table(c, e2Cols([]int{1, 2, 3}), 3).HighlightCurrentRow().Show().CurrentChanged() {
			changed++
		}
	})
	d.Click(mustFindText(t, d, "n1").Center())
	d.Click(mustFindText(t, d, "n1").Center())
	d.Click(mustFindText(t, d, "n2").Center())
	if changed != 2 {
		t.Fatalf("CurrentChanged %d times, want 2", changed)
	}
}

// TestTableThemeMetrics: E1's table tokens drive the selection and expand
// column widths (antd 32, +16 bordered; Element 48), the Element Table V2
// row and header heights (50) and the tree indent (Element 16).
func TestTableThemeMetrics(t *testing.T) {
	var sel []int
	var tb *TableW
	bordered := false
	d := tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{1, 2}), 2).Selection(&sel).Expandable(func(int) {})
		if bordered {
			tb.Bordered()
		}
		tb.Show()
	})
	ex, se := tb.fr.leaves[0], tb.fr.leaves[1]
	if ex.w != tb.fr.tk.ExpandColumnWidth || se.w != 32 {
		t.Fatalf("Ant widths: expand %v selection %v", ex.w, se.w)
	}
	bordered = true
	d.Settle()
	if w := tb.fr.leaves[1].w; w != 48 {
		t.Fatalf("Ant bordered selection width %v, want 48", w)
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if w := tb.fr.leaves[1].w; w != 48 {
		t.Fatalf("Element selection width %v, want 48", w)
	}
	if tb.indent != 16 {
		t.Fatalf("Element indent %v, want 16", tb.indent)
	}
	tblDriver(func(c *Context) {
		ConfigProvider(c).Theme(theme.ElementPlus()).Body(func() { tb = Table(c, e2Cols([]int{1, 2}), 2).Virtual().ScrollY(200).Show() })
	})
	if tb.fr.rh != 50 || tb.fr.hh != 50 {
		t.Fatalf("Element Table V2: row %v header %v", tb.fr.rh, tb.fr.hh)
	}
}

// TestTableSelectionColumnOptions: rowSelection.columnWidth and fixed.
func TestTableSelectionColumnOptions(t *testing.T) {
	var sel []int
	var tb *TableW
	tblDriver(func(c *Context) {
		tb = Table(c, e2Cols([]int{1, 2}), 2).Selection(&sel).SelectionColumnWidth(32).SelectionFixed().Show()
	})
	lf := tb.fr.leaves[0]
	if lf.kind != tkSelect || lf.w != 32 || lf.fixed != tfLeft {
		t.Fatalf("selection leaf %+v", lf)
	}
}

// TestTableScrollToFirstRowOnChange: antd scroll.scrollToFirstRowOnChange
// (default true) brings a bounded body back to the top after sorting.
func TestTableScrollToFirstRowOnChange(t *testing.T) {
	ages := make([]int, 60)
	for i := range ages {
		ages[i] = (i * 7) % 60
	}
	for _, on := range []bool{true, false} {
		var tb *TableW
		d := tblDriver(func(c *Context) {
			tb = Table(c, e2Cols(ages), len(ages)).ScrollY(200).ScrollToFirstRowOnChange(on).Show()
		})
		row := mustFindText(t, d, "n1")
		d.Scroll(row.Center(), 300)
		if tb.fr.scroll.off.Y <= 0 {
			t.Fatal("body did not scroll")
		}
		d.ClickLabel("年龄")
		top := tb.fr.scroll.off.Y == 0
		if top != on {
			t.Fatalf("ScrollToFirstRowOnChange(%v): offset %v", on, tb.fr.scroll.off.Y)
		}
	}
}

// TestTableEmptyAndUnknownKeys (Review Focus 2): no rows, nil bindings and
// keys that match no row render the empty state without panicking.
func TestTableEmptyAndUnknownKeys(t *testing.T) {
	var sel []int
	keys := []string{"ghost"}
	exp := []string{"ghost"}
	open := []string{"ghost"}
	cur := "ghost"
	page, size := 5, 10
	for _, rows := range []int{0, 2} {
		d := tblDriver(func(c *Context) {
			Table(c, e2Cols([]int{1, 2}), rows).Selection(&sel).Expandable(func(int) {}).ExpandedKeys(&exp).
				CurrentRowKey(&cur).Pagination(&page, &size).ShowSummary().DefaultExpandAllRows().Show()
			Table(c, e2Cols([]int{1, 2}), rows).SelectedKeys(&keys).Tree(nil, func(int) []int { return nil }).
				TreeOpenKeys(&open).Show()
			Table(c, nil, rows).Show()
		})
		if rows == 0 {
			mustFindText(t, d, ZhCN.NoData)
		}
		if page != 1 {
			t.Fatalf("rows=%d: page not clamped: %d", rows, page)
		}
	}
	if fmt.Sprint(keys) != "[ghost]" || fmt.Sprint(exp) != "[ghost]" {
		t.Fatalf("unknown keys dropped without a change: %v %v", keys, exp)
	}
}

// TestTableDisabledInteractions (Review Focus 3): under Disabled /
// ConfigProvider.ComponentDisabled sorting, selection and row-click
// expansion do not respond.
func TestTableDisabledInteractions(t *testing.T) {
	var s SortState
	var sel []int
	var keys []string
	var tb *TableW
	for _, viaProvider := range []bool{false, true} {
		d := tblDriver(func(c *Context) {
			build := func() {
				tb = Table(c, e2Cols([]int{3, 1, 2}), 3).Sort(&s).Selection(&sel).
					Expandable(func(int) {}).ExpandedKeys(&keys).ExpandRowByClick().Show()
			}
			if viaProvider {
				ConfigProvider(c).ComponentDisabled(true).Body(build)
			} else {
				Disabled(c, true, build)
			}
		})
		d.ClickLabel("年龄")
		clickRowCheck(t, d, "1")
		d.Click(mustFindText(t, d, "n2").Center())
		if s.Order != SortNone || len(sel) != 0 || len(keys) != 0 || tb.SelectionChanged() || tb.SortChanged() || tb.ExpandChanged() {
			t.Fatalf("provider=%v: disabled table responded: sort %+v sel %v keys %v", viaProvider, s, sel, keys)
		}
	}
}

// TestTableA11ySelection (Review Focus 4): grid, column headers with sort
// state, selection checkboxes named from the Locale.
func TestTableA11ySelection(t *testing.T) {
	var sel []int
	s := SortState{"age", SortAsc}
	d := tblDriver(func(c *Context) {
		Table(c, e2Cols([]int{3, 1, 2}), 3).Selection(&sel).Sort(&s).A11yLabel("people").Show()
	})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleGrid, "people"); !ok {
		t.Fatal("no grid named people")
	}
	h, ok := d.A11yFind(a11y.RoleColumnHeader, "年龄")
	if !ok || h.Sort != a11y.SortAscending {
		t.Fatalf("age header: %v %+v", ok, h)
	}
	if _, ok := d.A11yFind(a11y.RoleCheckbox, ZhCN.SelectAll); !ok {
		t.Fatal("header checkbox not named SelectAll")
	}
	row1 := fmt.Sprintf(ZhCN.Table.SelectRowFmt, 1)
	if _, ok := d.A11yFind(a11y.RoleCheckbox, row1); !ok {
		t.Fatalf("row checkbox %q missing", row1)
	}
	if err := d.A11yPerform(a11y.RoleCheckbox, row1, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if len(sel) != 1 {
		t.Fatalf("Press on the row checkbox: %v", sel)
	}
}
