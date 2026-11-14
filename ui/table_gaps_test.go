package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// ---------------------------------------------------------------------------
// Variable row heights.

func TestTblVarHMatchesPrefixSums(t *testing.T) {
	var v tblVarH
	n := 1000
	hs := make([]float32, n)
	for i := range hs {
		hs[i] = float32(20 + (i*7)%50)
	}
	v.build(n, func(p int) float32 { return hs[p] })
	check := func() {
		t.Helper()
		y := 0.0
		for i := 0; i <= n; i++ {
			if got := v.offset(i); math.Abs(got-y) > 1e-6 {
				t.Fatalf("offset(%d) = %v, want %v", i, got, y)
			}
			if i < n {
				if p := v.at(y + float64(hs[i])/2); p != i {
					t.Fatalf("at(mid of %d) = %d", i, p)
				}
				y += float64(hs[i])
			}
		}
	}
	check()
	// Updates re-sum only the following blocks.
	for _, p := range []int{0, 63, 64, 500, 999} {
		hs[p] = 123
		v.set(p, 123)
	}
	check()
	first, last := v.rangeOf(v.offset(500), 100, 0)
	if first != 500 || v.offset(last) >= v.offset(500)+100 || v.offset(last+1) < v.offset(500)+100 {
		t.Fatalf("range %d..%d", first, last)
	}
}

func wrapCols(texts []string) []TableColumn {
	return []TableColumn{
		{Key: "id", Title: "编号", Width: 80, Text: func(i int) string { return "#" + strconv.Itoa(i) }},
		{Key: "note", Title: "说明", Wrap: true, Text: func(i int) string { return texts[i%len(texts)] }},
	}
}

func TestTableWrapRowsGrow(t *testing.T) {
	long := strings.Repeat("很长的备注文字需要换行显示。", 12)
	d := tblDriver(func(c *Context) {
		Table(c, wrapCols([]string{"短", long}), 4).Show()
	})
	short, ok := d.Find("row:0")
	if !ok {
		t.Fatal("row 0 missing")
	}
	tall := mustFind(t, d, "row:1")
	// At least one more line (how many depends on the CJK font: Linux CI
	// has none and wraps the fallback glyphs into two lines).
	if tall.H < short.H*1.5 {
		t.Fatalf("wrapped row not taller: %v vs %v", tall.H, short.H)
	}
	if r2 := mustFind(t, d, "row:2"); math.Abs(float64(r2.Y-tall.Bottom())) > 0.5 || r2.H != short.H {
		t.Fatalf("next row misplaced: %v after %v", r2, tall)
	}
	// Narrower table: the long text needs more lines.
	d.Resize(500, 600)
	for i := 0; i < 4; i++ {
		d.Frame()
	}
	if t2 := mustFind(t, d, "row:1"); t2.H <= tall.H {
		t.Fatalf("row did not grow when narrowed: %v -> %v", tall.H, t2.H)
	}
}

func TestTableWrapRenderCellMeasured(t *testing.T) {
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{
			{Key: "id", Title: "编号", Width: 80, Text: func(i int) string { return strconv.Itoa(i) }},
			{Key: "tags", Title: "标签", Width: 160, Wrap: true, Render: func(i int) {
				for k := 0; k < 1+i*20; k++ {
					Tag(c, "tag-"+strconv.Itoa(k))
				}
			}},
		}
		Table(c, cols, 2).Show()
	})
	a, b := mustFind(t, d, "row:0"), mustFind(t, d, "row:1")
	if b.H <= a.H+10 {
		t.Fatalf("render cell with wrapped tags not measured: %v vs %v", a.H, b.H)
	}
}

func TestTableVirtualWrapScrollsAndKeepsAnchor(t *testing.T) {
	long := strings.Repeat("可变行高", 40)
	var tb *TableW
	d := tblDriver(func(c *Context) {
		tb = Table(c, wrapCols([]string{"一行", long, "一行", "一行"}), 100000).ScrollY(300).Virtual().Show()
	})
	p := mustFind(t, d, "row:0").Center()
	for i := 0; i < 10; i++ {
		d.Inject(input.Scroll{Pos: p, DY: 150000})
		d.Frame()
	}
	d.Settle()
	st := tb.st
	if st.vh.cnt == 0 || st.vh.cnt > 2000 {
		t.Fatalf("measured %d rows (want only the visited ones)", st.vh.cnt)
	}
	if tb.fr.first <= 0 || tb.fr.sy <= 0 {
		t.Fatalf("did not scroll: first %d sy %v", tb.fr.first, tb.fr.sy)
	}
	// The row at the top stays put across frames (no jumping once measured).
	top := tb.fr.vh.at(tb.fr.sy)
	key := "row:" + strconv.Itoa(top)
	r1 := mustFind(t, d, key)
	d.Frame()
	d.Frame()
	if r2 := mustFind(t, d, key); r2.Y != r1.Y {
		t.Fatalf("top row moved: %v -> %v", r1.Y, r2.Y)
	}
	// Every built row has its measured height.
	for pos := tb.fr.first; pos <= tb.fr.last; pos++ {
		n, ok := tblNode(d, "row:"+strconv.Itoa(pos))
		if !ok {
			t.Fatalf("row %d not built", pos)
		}
		if math.Abs(float64(n.ln.Rect.H-st.vh.measured(pos))) > 0.5 || st.vh.measured(pos) == 0 {
			t.Fatalf("row %d height %v, measured %v", pos, n.ln.Rect.H, st.vh.measured(pos))
		}
	}
	// Scrolling to the end reaches the last row.
	d.Inject(input.Scroll{Pos: p, DY: 1e9})
	d.Settle()
	d.Settle()
	if _, ok := d.Find("row:99999"); !ok {
		t.Fatal("last row not reachable")
	}
}

// ---------------------------------------------------------------------------
// Tree selection linkage.

func TestTblTreeCheckModel(t *testing.T) {
	kids := map[int][]int{0: {1, 4}, 1: {2, 3}, 4: {5}}
	k := func(i int) []int { return kids[i] }
	par := tblTreeParents([]int{0}, k)
	sel := map[int]bool{}
	tblTreeCheck(sel, 2, true, k, par, nil)
	if !sel[2] || sel[1] || sel[0] {
		t.Fatalf("one child: %v", sel)
	}
	half := tblTreeHalf([]int{0}, k, sel)
	if !half[1] || !half[0] || half[2] {
		t.Fatalf("half %v", half)
	}
	tblTreeCheck(sel, 3, true, k, par, nil)
	if !sel[1] || sel[0] {
		t.Fatalf("both children: %v", sel)
	}
	tblTreeCheck(sel, 4, true, k, par, nil)
	if !sel[0] || !sel[5] {
		t.Fatalf("all: %v", sel)
	}
	tblTreeCheck(sel, 0, false, k, par, nil)
	if len(sel) != 0 {
		t.Fatalf("uncheck root: %v", sel)
	}
	// Disabled nodes keep their state and do not block the parent.
	dis := func(i int) bool { return i == 3 }
	tblTreeCheck(sel, 1, true, k, par, dis)
	if !sel[1] || !sel[2] || sel[3] {
		t.Fatalf("disabled child: %v", sel)
	}
}

type tcNode struct {
	name string
	kids []int
}

var tcNodes = []tcNode{{"root", []int{1, 2}}, {"a", []int{3, 4}}, {"b", nil}, {"a1", nil}, {"a2", nil}}

func tcTable(sel *[]int, strict bool, out **TableW) func(c *Context) {
	return func(c *Context) {
		cols := []TableColumn{{Key: "name", Title: "名称", Text: func(i int) string { return tcNodes[i].name }}}
		tb := Table(c, cols, len(tcNodes)).Tree([]int{0}, func(i int) []int { return tcNodes[i].kids }).
			TreeOpenKeys(&[]string{"0", "1"}).Selection(sel)
		if strict {
			tb.CheckStrictly()
		}
		*out = tb.Show()
	}
}

// clickRowCheck clicks the selection checkbox of a row.
func clickRowCheck(t *testing.T, d *Driver, key string) {
	t.Helper()
	r := mustFind(t, d, "row:"+key)
	d.Click(geom.Pt(r.X+24, r.Y+r.H/2))
}

func TestTableTreeCheckLinked(t *testing.T) {
	var sel []int
	var tb *TableW
	d := tblDriver(tcTable(&sel, false, &tb))
	clickRowCheck(t, d, "3") // a1
	if fmt.Sprint(sel) != "[3]" {
		t.Fatalf("a1: %v", sel)
	}
	st := tb.fr
	if !st.half[1] || !st.half[0] {
		t.Fatalf("parents not half-checked: %v", st.half)
	}
	clickRowCheck(t, d, "4") // a2: a becomes checked
	if !hasInt(sel, 1) || hasInt(sel, 0) {
		t.Fatalf("a not checked by its children: %v", sel)
	}
	clickRowCheck(t, d, "0") // root checks everything
	if len(sel) != 5 {
		t.Fatalf("root: %v", sel)
	}
	clickRowCheck(t, d, "1") // uncheck a: a, a1, a2 and root go
	if fmt.Sprint(sel) != "[2]" {
		t.Fatalf("uncheck a: %v", sel)
	}
}

func TestTableTreeCheckStrictly(t *testing.T) {
	var sel []int
	var tb *TableW
	d := tblDriver(tcTable(&sel, true, &tb))
	clickRowCheck(t, d, "1")
	if fmt.Sprint(sel) != "[1]" {
		t.Fatalf("strict: %v", sel)
	}
	if tb.fr.linked {
		t.Fatal("linkage active with CheckStrictly")
	}
}

func hasInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sticky header and summary.

func TestTableStickyHeaderAndSummary(t *testing.T) {
	var box *ScrollBox
	d := NewDriver(func(c *Context) {
		box = Scroll(c, func() {
			Div(c, layout.H(100))
			cols := []TableColumn{
				{Key: "name", Title: "姓名", Text: func(i int) string { return "n" + strconv.Itoa(i) }},
				{Key: "v", Title: "数值", Text: func(i int) string { return strconv.Itoa(i) }},
			}
			Table(c, cols, 50).Sticky(10).StickySummary(0).
				Summary(func(col string, rows []int) string {
					if col == "name" {
						return "合计"
					}
					return ""
				}).Show()
			Div(c, layout.H(300))
		}, layout.H(400))
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	view := d.r.c.rects[box.ID()]
	head, _ := d.FindText("姓名")
	if head.Y < view.Y+100-1 {
		t.Fatalf("header moved before scrolling: %v", head)
	}
	sum := painted(d, "合计")
	if sum.Bottom() > view.Bottom()+0.5 {
		t.Fatalf("summary not stuck to the bottom: %v view %v", sum, view)
	}
	d.Scroll(view.Center(), 600)
	d.Settle()
	head, _ = d.FindText("姓名")
	if math.Abs(float64(head.Y-(view.Y+10))) > 1 {
		t.Fatalf("header not stuck: y %v, view top %v", head.Y, view.Y)
	}
	// Clicking a row under the stuck header hits the header, not the row.
	if hit := topHit(d, geom.Pt(view.X+40, view.Y+15)); strings.HasPrefix(hit, "row:") {
		t.Fatalf("row above the sticky header: %s", hit)
	}
	// Scrolled past the table: the header leaves with it.
	d.Scroll(view.Center(), 5000)
	d.Settle()
	if head2, ok := d.FindText("姓名"); ok && head2.Y > view.Y+10 {
		t.Fatalf("header stuck after the table left: %v", head2)
	}
}

// ---------------------------------------------------------------------------
// Drag auto-scroll.

func TestTableDragAutoScroll(t *testing.T) {
	var tb *TableW
	moved := false
	d := tblDriver(func(c *Context) {
		tb = Table(c, ageCols(make([]int, 200)), 200).ScrollY(240).DragSort(func(from, to int) { moved = true }).Show()
	})
	h := mustFind(t, d, "drag:0")
	body := d.r.c.rects[tb.fr.bodyID]
	d.Inject(input.Pointer{Kind: input.Move, Pos: h.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: h.Center()})
	d.Frame()
	edge := geom.Pt(h.Center().X, body.Bottom()-5)
	d.Inject(input.Pointer{Kind: input.Move, Pos: edge})
	d.Frame()
	for i := 0; i < 20; i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	if tb.fr.sy < 100 {
		t.Fatalf("no auto-scroll near the bottom edge: sy %v", tb.fr.sy)
	}
	// Middle of the body: scrolling stops.
	sy := tb.fr.sy
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(edge.X, body.Y+body.H/2)})
	d.Frame()
	for i := 0; i < 5; i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	if math.Abs(tb.fr.sy-sy) > 1 {
		t.Fatalf("kept scrolling in the middle: %v -> %v", sy, tb.fr.sy)
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(edge.X, body.Y+body.H/2)})
	d.Settle()
	if !moved {
		t.Fatal("drop after auto-scroll did not move")
	}
	// Up edge scrolls back.
	h = mustFind(t, d, "drag:"+strconv.Itoa(tb.fr.first+tblOverscan+1))
	d.Inject(input.Pointer{Kind: input.Move, Pos: h.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: h.Center()})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(h.Center().X, body.Y+3)})
	d.Frame()
	before := tb.fr.sy
	for i := 0; i < 10; i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	if tb.fr.sy >= before {
		t.Fatalf("no auto-scroll near the top edge: %v -> %v", before, tb.fr.sy)
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(h.Center().X, body.Y+3)})
	d.Settle()
}

// ---------------------------------------------------------------------------
// Fixed-column shadow and pinning.

func TestTableFixedShadowIsBlurred(t *testing.T) {
	d := tblDriver(func(c *Context) {
		cols := []TableColumn{{Key: "a", Title: "A", Width: 120, Fixed: FixedLeft, Text: func(i int) string { return "a" }}}
		for k := 0; k < 10; k++ {
			cols = append(cols, TableColumn{Key: "c" + strconv.Itoa(k), Title: "C", Width: 150, Text: func(i int) string { return "c" }})
		}
		cols = append(cols, TableColumn{Key: "z", Title: "Z", Width: 100, Fixed: FixedRight, Text: func(i int) string { return "z" }})
		Table(c, cols, 3).Show()
	})
	count := func() int {
		n := 0
		for _, op := range d.r.List().Ops {
			if s, ok := op.(paint.ShadowRect); ok && s.Shadow.Blur > 0 {
				n++
			}
		}
		return n
	}
	if n := count(); n != 1 { // right shadow only (not scrolled yet)
		t.Fatalf("shadows before scrolling: %d", n)
	}
	r := mustFind(t, d, "row:0")
	d.Inject(input.Scroll{Pos: r.Center(), DX: 300})
	d.Settle()
	if n := count(); n != 2 {
		t.Fatalf("shadows while scrolled: %d", n)
	}
}

func TestTblArrangeFixedOverride(t *testing.T) {
	cols := []TableColumn{{Key: "a", Fixed: FixedLeft}, {Key: "b"}, {Key: "c"}, {Key: "d", Fixed: FixedRight}}
	out := tblArrange(cols, &TableColumnState{Fixed: map[string]string{"a": FixedNone, "c": FixedLeft, "b": FixedRight}})
	var got []string
	for _, c := range out {
		got = append(got, c.Key+":"+c.Fixed)
	}
	if strings.Join(got, ",") != "c:left,a:,b:right,d:right" {
		t.Fatalf("arranged %v", got)
	}
}

func TestProTableColumnSettingsPin(t *testing.T) {
	type row struct{ a, b string }
	var pt *ProTableW[row]
	d := NewDriver(func(c *Context) {
		pt = ProTable(c, []ProColumn[row]{
			{Key: "a", Title: "甲", Width: 300, Value: func(r row) any { return r.a }},
			{Key: "b", Title: "乙", Width: 300, Value: func(r row) any { return r.b }},
			{Key: "c", Title: "丙", Width: 300, Value: func(r row) any { return r.b }},
		}).NoSearch().ScrollX(1400).Request(func(req ProRequest, done func(ProResult[row])) {
			done(ProResult[row]{Rows: []row{{"1", "2"}}, Total: 1})
		}).Show()
	}, DriverOptions{Width: 1000, Height: 700})
	d.Settle()
	d.ClickLabel(ZhCN.ProTable.ColumnSetting)
	item := mustFind(t, d, "drag:b")
	d.Move(geom.Pt(item.X+60, item.Center().Y))
	if err := d.ClickLabel("pin-right:b"); err != nil {
		t.Fatal(err)
	}
	if pt.st.colState.Fixed["b"] != FixedRight {
		t.Fatalf("fixed %v", pt.st.colState.Fixed)
	}
	fr := pt.Table().fr
	last := fr.leaves[len(fr.leaves)-1]
	if last.key != "b" || last.fixed != tfRight {
		t.Fatalf("b not pinned right: %+v", last)
	}
	// The pinned column is listed in its own group; unpin it again.
	if err := d.ClickLabel("unpin:b"); err != nil {
		t.Fatal(err)
	}
	if pt.st.colState.Fixed["b"] != FixedNone || pt.Table().fr.anyRight {
		t.Fatalf("unpin: %v", pt.st.colState.Fixed)
	}
}

// topHit returns the label of the top-most hit target at p.
func topHit(d *Driver, p geom.Point) string {
	hs := d.r.c.hits
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].rect.Contains(p) {
			return hs[i].label
		}
	}
	return ""
}

// painted returns the painted (transformed) rect of the node labelled s.
func painted(d *Driver, s string) geom.Rect {
	n, ok := tblNode(d, s)
	if !ok {
		return geom.Rect{}
	}
	return d.r.c.rects[n.id]
}
