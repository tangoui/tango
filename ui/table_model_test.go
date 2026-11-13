package ui

import (
	"reflect"
	"strconv"
	"testing"
)

func TestTblArrangeOrderHiddenFixed(t *testing.T) {
	cols := []TableColumn{
		{Key: "a"}, {Key: "b", Fixed: FixedRight}, {Key: "c"}, {Key: "d", Fixed: FixedLeft}, {Key: "e", Hidden: true},
	}
	got := tblArrange(cols, nil)
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	if want := []string{"d", "a", "c", "b"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("arrange = %v, want %v", keys, want)
	}
	cs := &TableColumnState{Order: []string{"c", "a"}, Hidden: map[string]bool{"e": false, "a": true}}
	got = tblArrange(cols, cs)
	keys = keys[:0]
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	if want := []string{"d", "c", "e", "b"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("arrange with state = %v, want %v", keys, want)
	}
}

func TestTblFlattenGroupedHeader(t *testing.T) {
	cols := []TableColumn{
		{Key: "name"},
		{Key: "g", Children: []TableColumn{
			{Key: "age"},
			{Key: "addr", Children: []TableColumn{{Key: "city"}, {Key: "street"}}},
		}},
	}
	leaves, heads, depth := tblFlatten(cols, []tblLeaf{{kind: tkSelect, w: 48}})
	if depth != 3 {
		t.Fatalf("depth = %d", depth)
	}
	var lk []string
	for _, l := range leaves {
		lk = append(lk, l.key)
	}
	if want := []string{"", "name", "age", "city", "street"}; !reflect.DeepEqual(lk, want) {
		t.Fatalf("leaves = %v", lk)
	}
	find := func(key string) tblHead {
		for _, h := range heads {
			if h.col != nil && h.col.Key == key {
				return h
			}
		}
		t.Fatalf("no head %q", key)
		return tblHead{}
	}
	if h := find("g"); h.first != 2 || h.last != 4 || h.row != 0 || h.rows != 1 {
		t.Fatalf("group g = %+v", h)
	}
	if h := find("addr"); h.first != 3 || h.last != 4 || h.row != 1 {
		t.Fatalf("group addr = %+v", h)
	}
	if h := find("name"); h.rows != 3 {
		t.Fatalf("name spans %d rows, want 3", h.rows)
	}
	if h := find("age"); h.row != 1 || h.rows != 2 {
		t.Fatalf("age = %+v", h)
	}
	if h := find("city"); h.row != 2 || h.rows != 1 {
		t.Fatalf("city = %+v", h)
	}
	if heads[0].kind != tkSelect || heads[0].rows != 3 {
		t.Fatalf("selection head = %+v", heads[0])
	}
}

func TestTblWidths(t *testing.T) {
	mk := func() []tblLeaf {
		leaves, _, _ := tblFlatten([]TableColumn{{Key: "a", Width: 100}, {Key: "b"}, {Key: "c", Grow: 3}}, []tblLeaf{{kind: tkSelect, w: 48}})
		return leaves
	}
	l := mk()
	total := tblWidths(l, 548, nil)
	if total != 548 || l[0].w != 48 || l[1].w != 100 || l[2].w != 100 || l[3].w != 300 {
		t.Fatalf("widths = %v %v %v %v total %v", l[0].w, l[1].w, l[2].w, l[3].w, total)
	}
	if l[3].x != 248 {
		t.Fatalf("x of c = %v", l[3].x)
	}
	// Too narrow: flexible columns keep their minimum (80) and overflow.
	l = mk()
	total = tblWidths(l, 200, nil)
	if l[2].w != 80 || l[3].w != 80 || total != 308 {
		t.Fatalf("min widths: %v %v total %v", l[2].w, l[3].w, total)
	}
	// A user resize wins over Width.
	l = mk()
	tblWidths(l, 548, map[string]float32{"a": 160})
	if l[1].w != 160 || l[2].w+l[3].w != 340 {
		t.Fatalf("resized: %v %v %v", l[1].w, l[2].w, l[3].w)
	}
	// All fixed and narrower than the view: untouched columns widen.
	leaves, _, _ := tblFlatten([]TableColumn{{Key: "a", Width: 100}, {Key: "b", Width: 300}}, nil)
	total = tblWidths(leaves, 800, nil)
	if total != 800 || leaves[0].w != 200 || leaves[1].w != 600 {
		t.Fatalf("stretch: %v %v %v", leaves[0].w, leaves[1].w, total)
	}
}

func TestTblSortAndFilter(t *testing.T) {
	age := []int{30, 20, 30, 10}
	name := []string{"b", "a", "a", "c"}
	ids := []int{0, 1, 2, 3}
	tblSortRows(ids, []tblSortSpec{
		{cmp: func(a, b int) int { return age[a] - age[b] }, desc: true},
		{cmp: func(a, b int) int { return compareStr(name[a], name[b]) }},
	})
	if want := []int{2, 0, 1, 3}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("multi sort = %v, want %v", ids, want)
	}
	f := tblFilterRows([]int{0, 1, 2, 3}, []tblFilterSpec{{fn: func(r int, v string) bool { return name[r] == v }, values: []string{"a", "c"}}})
	if want := []int{1, 2, 3}; !reflect.DeepEqual(f, want) {
		t.Fatalf("filter = %v", f)
	}
	if tblNextOrder(SortNone) != SortAsc || tblNextOrder(SortAsc) != SortDesc || tblNextOrder(SortDesc) != SortNone {
		t.Fatal("sort cycle")
	}
}

func compareStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestTblPage(t *testing.T) {
	for _, c := range []struct{ n, page, size, a, b, p int }{
		{46, 1, 10, 0, 10, 1}, {46, 5, 10, 40, 46, 5}, {46, 9, 10, 40, 46, 5}, {0, 3, 10, 0, 0, 1}, {7, 0, 5, 0, 5, 1},
	} {
		a, b, p := tblPage(c.n, c.page, c.size)
		if a != c.a || b != c.b || p != c.p {
			t.Errorf("tblPage(%d,%d,%d) = %d,%d,%d", c.n, c.page, c.size, a, b, p)
		}
	}
}

func TestTblTreeFlatten(t *testing.T) {
	kids := map[int][]int{0: {1, 2}, 2: {3}}
	open := map[int]bool{0: true}
	rows := tblTree([]int{0, 4}, func(i int) []int { return kids[i] }, func(i int) bool { return i == 4 },
		func(i int) bool { return open[i] }, strconv.Itoa, func(ids []int) []int { return ids })
	var got [][3]int
	for _, r := range rows {
		k := 0
		if r.kids {
			k = 1
		}
		got = append(got, [3]int{r.data, r.depth, k})
	}
	want := [][3]int{{0, 0, 1}, {1, 1, 0}, {2, 1, 1}, {4, 0, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	open[2] = true
	rows = tblTree([]int{0}, func(i int) []int { return kids[i] }, nil, func(i int) bool { return open[i] }, strconv.Itoa, func(ids []int) []int { return ids })
	if len(rows) != 4 || rows[3].data != 3 || rows[3].depth != 2 {
		t.Fatalf("nested open = %+v", rows)
	}
}

func TestTblSpanCells(t *testing.T) {
	// Column 0 merges rows 0-2 and 3-4; row 1 merges columns 1-2.
	f := func(pos, col int) (int, int) {
		if col == 0 {
			switch pos {
			case 0:
				return 3, 1
			case 3:
				return 2, 1
			}
			return 0, 0
		}
		if pos == 1 && col == 1 {
			return 1, 2
		}
		if pos == 1 && col == 2 {
			return 0, 0
		}
		return 1, 1
	}
	cells := tblSpanCells(0, 4, 3, f, 10)
	count := map[[2]int]tblCell{}
	for _, c := range cells {
		count[[2]int{c.pos, c.col}] = c
	}
	if c := count[[2]int{0, 0}]; c.rs != 3 {
		t.Fatalf("owner (0,0) = %+v", c)
	}
	if _, ok := count[[2]int{1, 0}]; ok {
		t.Fatal("covered (1,0) drawn")
	}
	if c := count[[2]int{1, 1}]; c.cs != 2 {
		t.Fatalf("colspan (1,1) = %+v", c)
	}
	if _, ok := count[[2]int{1, 2}]; ok {
		t.Fatal("covered (1,2) drawn")
	}
	if len(cells) != 15-3-1 {
		t.Fatalf("cells = %d", len(cells))
	}
	// A virtual window starting at row 2 still draws the owner at row 0.
	cells = tblSpanCells(2, 4, 3, f, 10)
	if cells[0].pos != 0 || cells[0].col != 0 || cells[0].rs != 3 {
		t.Fatalf("window owner = %+v", cells[0])
	}
	// Starting at row 3: owner row 0 no longer reaches the window.
	cells = tblSpanCells(3, 4, 3, f, 10)
	for _, c := range cells {
		if c.pos < 3 {
			t.Fatalf("stale owner %+v", c)
		}
	}
	// Colspan is clipped to the column count.
	cells = tblSpanCells(0, 0, 2, func(int, int) (int, int) { return 1, 5 }, 0)
	if cells[0].cs != 2 {
		t.Fatalf("clip colspan = %+v", cells[0])
	}
}

func TestTblVisibleRange(t *testing.T) {
	first, last := tblRange(100000, 40, nil, 4000, 400, 0)
	if first != 100 || last != 109 {
		t.Fatalf("range = %d..%d", first, last)
	}
	first, last = tblRange(100000, 40, nil, 4010, 400, 3)
	if first != 97 || last != 113 {
		t.Fatalf("overscan range = %d..%d", first, last)
	}
	first, last = tblRange(100000, 40, nil, 4e6-400, 400, 3)
	if last != 99999 || first != 99987 {
		t.Fatalf("end range = %d..%d", first, last)
	}
	// Variable heights: row 1 has a 100px detail panel.
	offs := []float64{0, 40, 180, 220, 260}
	first, last = tblRange(4, 40, offs, 150, 50, 0)
	if first != 1 || last != 2 {
		t.Fatalf("variable range = %d..%d", first, last)
	}
	if f, l := tblRange(0, 40, nil, 0, 400, 3); f != 0 || l != -1 {
		t.Fatalf("empty range = %d..%d", f, l)
	}
}

func TestTblHelpers(t *testing.T) {
	s := tblToggle(nil, "a", true)
	s = tblToggle(s, "b", true)
	s = tblToggle(s, "a", true)
	s = tblToggle(s, "a", false)
	if !reflect.DeepEqual(s, []string{"b"}) {
		t.Fatalf("toggle = %v", s)
	}
	if all, some := tblSelState(5, 5); !all || some {
		t.Fatal("all")
	}
	if all, some := tblSelState(5, 2); all || !some {
		t.Fatal("half")
	}
	if all, some := tblSelState(0, 0); all || some {
		t.Fatal("empty")
	}
	if got := tblMove([]int{0, 1, 2, 3}, 0, 2); !reflect.DeepEqual(got, []int{1, 2, 0, 3}) {
		t.Fatalf("move = %v", got)
	}
}

func TestProBuildRequest(t *testing.T) {
	req := proBuildRequest(0, 20,
		[]SortState{{"calls", SortDesc}, {"x", SortNone}},
		map[string][]string{"status": {"online", "error"}, "empty": nil},
		map[string]string{"name": "  trade ", "owner": ""})
	if req.Page != 1 || req.PageSize != 20 {
		t.Fatalf("paging = %d/%d", req.Page, req.PageSize)
	}
	if !reflect.DeepEqual(req.Sort, []SortState{{"calls", SortDesc}}) {
		t.Fatalf("sort = %v", req.Sort)
	}
	if !reflect.DeepEqual(req.Filters, map[string][]string{"status": {"online", "error"}}) {
		t.Fatalf("filters = %v", req.Filters)
	}
	if !reflect.DeepEqual(req.Params, map[string]string{"name": "trade"}) {
		t.Fatalf("params = %v", req.Params)
	}
	if req.signature() == proBuildRequest(2, 20, nil, nil, nil).signature() {
		t.Fatal("signature should differ")
	}
}

func TestProFormat(t *testing.T) {
	for _, c := range []struct {
		vt   string
		v    any
		want string
	}{
		{ValueMoney, 1234.5, "¥1,234.50"}, {ValueMoney, -3, "-¥3.00"}, {ValueDigit, 12000, "12,000"},
		{ValuePercent, 12.345, "12.35%"}, {ValueText, nil, "-"}, {ValueText, "x", "x"},
	} {
		if got := ProFormat(c.vt, nil, c.v); got != c.want {
			t.Errorf("ProFormat(%s, %v) = %q, want %q", c.vt, c.v, got, c.want)
		}
	}
	if got := ProFormat(ValueStatus, []ProEnum{{Value: "on", Label: "开启"}}, "on"); got != "开启" {
		t.Errorf("enum = %q", got)
	}
}
