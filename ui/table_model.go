package ui

import (
	"math"
	"sort"
	"strconv"
)

// Pure table logic, independent of the widget tree so it can be unit
// tested: column flattening and widths, sort/filter/paging pipelines, tree
// flattening, merged-cell resolution and virtual-scroll ranges.

// Leaf column kinds.
const (
	tkData int8 = iota
	tkSelect
	tkExpand
	tkDrag
)

// Fixed sides.
const (
	tfNone int8 = iota
	tfLeft
	tfRight
)

// tblLeaf is a resolved body column.
type tblLeaf struct {
	col   *TableColumn // nil for special columns
	kind  int8
	di    int // index among data leaves (-1 for special columns)
	key   string
	w, x  float32
	fixed int8
	min   float32
	set   bool // width is explicit (Width or resized)
	grow  float32
	user  bool // resized by the user
}

// tblHead is a header cell spanning leaves [first,last] and header rows
// [row,row+rows).
type tblHead struct {
	col         *TableColumn
	kind        int8
	first, last int
	row, rows   int
}

func fixedOf(col *TableColumn) int8 {
	if col.Fixed == FixedLeft {
		return tfLeft
	}
	if col.Fixed == FixedRight {
		return tfRight
	}
	if len(col.Children) > 0 {
		return fixedOf(&col.Children[0])
	}
	return tfNone
}

// tblArrange applies a column state (order and visibility) to cols and puts
// left-fixed columns first and right-fixed columns last.
func tblArrange(cols []TableColumn, cs *TableColumnState) []TableColumn {
	out := make([]TableColumn, 0, len(cols))
	for _, col := range cols {
		hidden := col.Hidden
		if cs != nil && cs.Hidden != nil {
			if h, ok := cs.Hidden[col.Key]; ok {
				hidden = h
			}
		}
		if hidden {
			continue
		}
		if cs != nil && cs.Fixed != nil {
			if fx, ok := cs.Fixed[col.Key]; ok {
				col.Fixed = fx
				if fx == FixedNone {
					col.Fixed = ""
					col.Children = tblUnpin(col.Children)
				}
			}
		}
		if len(col.Children) > 0 {
			col.Children = tblArrange(col.Children, cs)
			if len(col.Children) == 0 {
				continue
			}
		}
		out = append(out, col)
	}
	if cs != nil && len(cs.Order) > 0 {
		rank := make(map[string]int, len(cs.Order))
		for i, k := range cs.Order {
			rank[k] = i
		}
		sort.SliceStable(out, func(i, j int) bool {
			ri, oki := rank[out[i].Key]
			rj, okj := rank[out[j].Key]
			if !oki || !okj {
				return false
			}
			return ri < rj
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return fixedRank(fixedOf(&out[i])) < fixedRank(fixedOf(&out[j]))
	})
	return out
}

// tblUnpin clears Fixed in a column group (copies the slice).
func tblUnpin(cols []TableColumn) []TableColumn {
	if len(cols) == 0 {
		return cols
	}
	out := append([]TableColumn(nil), cols...)
	for i := range out {
		out[i].Fixed = ""
		out[i].Children = tblUnpin(out[i].Children)
	}
	return out
}

func fixedRank(f int8) int {
	switch f {
	case tfLeft:
		return 0
	case tfRight:
		return 2
	}
	return 1
}

func colDepth(cols []TableColumn) int {
	d := 0
	for i := range cols {
		k := 1
		if len(cols[i].Children) > 0 {
			k += colDepth(cols[i].Children)
		}
		d = max(d, k)
	}
	return d
}

// tblFlatten turns arranged columns into data leaves and header cells.
// specials are prepended leaves (selection, expand, drag) that span every
// header row. It returns the header row count.
func tblFlatten(cols []TableColumn, specials []tblLeaf) ([]tblLeaf, []tblHead, int) {
	depth := max(1, colDepth(cols))
	leaves := append([]tblLeaf(nil), specials...)
	var heads []tblHead
	anyLeft := false
	for i := range cols {
		if fixedOf(&cols[i]) == tfLeft {
			anyLeft = true
		}
	}
	for i := range leaves {
		leaves[i].di = -1
		if anyLeft {
			leaves[i].fixed = tfLeft
		}
		heads = append(heads, tblHead{kind: leaves[i].kind, first: i, last: i, row: 0, rows: depth})
	}
	di := 0
	var walk func(cols []TableColumn, row int, fixed int8)
	walk = func(cols []TableColumn, row int, fixed int8) {
		for i := range cols {
			col := &cols[i]
			f := fixed
			if row == 0 {
				f = fixedOf(col)
			}
			if len(col.Children) > 0 {
				first := len(leaves)
				hi := len(heads)
				heads = append(heads, tblHead{col: col, kind: tkData, first: first, row: row, rows: 1})
				walk(col.Children, row+1, f)
				heads[hi].last = len(leaves) - 1
				continue
			}
			lf := tblLeaf{col: col, kind: tkData, di: di, key: col.Key, fixed: f, grow: col.Grow, min: col.MinWidth}
			if lf.grow <= 0 {
				lf.grow = 1
			}
			if lf.min <= 0 {
				lf.min = 80
			}
			if col.Width > 0 {
				lf.w, lf.set = col.Width, true
				lf.min = min(lf.min, col.Width)
			}
			di++
			heads = append(heads, tblHead{col: col, kind: tkData, first: len(leaves), last: len(leaves), row: row, rows: depth - row})
			leaves = append(leaves, lf)
		}
	}
	walk(cols, 0, tfNone)
	return leaves, heads, depth
}

// tblWidths resolves leaf widths and x offsets for an available width and
// returns the total width. Resized widths come from user.
func tblWidths(leaves []tblLeaf, avail float32, user map[string]float32) float32 {
	fixedSum, grow := float32(0), float32(0)
	for i := range leaves {
		lf := &leaves[i]
		if lf.kind == tkData && lf.key != "" {
			if w, ok := user[lf.key]; ok && w > 0 {
				lf.w, lf.set, lf.user = w, true, true
			}
		}
		if lf.set || lf.kind != tkData {
			fixedSum += lf.w
		} else {
			grow += lf.grow
		}
	}
	free := avail - fixedSum
	if grow > 0 {
		// Flexible columns share the free space, never below their minimum.
		// Columns clamped to their minimum are taken out and the rest
		// re-shared, so the result is stable.
		left := map[int]bool{}
		for i := range leaves {
			if !leaves[i].set && leaves[i].kind == tkData {
				left[i] = true
			}
		}
		for iter := 0; iter < len(leaves)+1 && len(left) > 0; iter++ {
			g := float32(0)
			for i := range left {
				g += leaves[i].grow
			}
			share := max(0, free)
			clamped := false
			for i := range left {
				w := share * leaves[i].grow / g
				if w < leaves[i].min {
					leaves[i].w = leaves[i].min
					free -= leaves[i].min
					delete(left, i)
					clamped = true
				}
			}
			if !clamped {
				for i := range left {
					leaves[i].w = share * leaves[i].grow / g
				}
				break
			}
		}
	} else if free > 0.5 {
		// Every width is explicit and the table is wider: widen the
		// untouched data columns proportionally (like a 100% wide HTML table).
		base := float32(0)
		for i := range leaves {
			if leaves[i].kind == tkData && !leaves[i].user {
				base += leaves[i].w
			}
		}
		if base > 0 {
			for i := range leaves {
				if leaves[i].kind == tkData && !leaves[i].user {
					leaves[i].w += free * leaves[i].w / base
				}
			}
		}
	}
	// Snap to whole pixels, carrying the rounding so the total is exact.
	x, acc := float32(0), float32(0)
	for i := range leaves {
		acc += leaves[i].w
		end := float32(math.Round(float64(acc)))
		leaves[i].x = x
		leaves[i].w = end - x
		x = end
	}
	return x
}

// tblNextOrder cycles none → ascending → descending → none.
func tblNextOrder(o SortOrder) SortOrder { return (o + 1) % 3 }

// tblSortSpec is one comparator of a (multi-column) sort.
type tblSortSpec struct {
	cmp  func(a, b int) int
	desc bool
}

// tblSortRows stable-sorts ids by the specs in priority order.
func tblSortRows(ids []int, specs []tblSortSpec) {
	if len(specs) == 0 {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		for _, s := range specs {
			r := s.cmp(a, b)
			if s.desc {
				r = -r
			}
			if r != 0 {
				return r < 0
			}
		}
		return false
	})
}

// tblFilterSpec is an active local filter.
type tblFilterSpec struct {
	fn     func(row int, v string) bool
	values []string
}

func (f tblFilterSpec) match(row int) bool {
	for _, v := range f.values {
		if f.fn(row, v) {
			return true
		}
	}
	return false
}

// tblFilterRows keeps the ids that pass every filter (in place).
func tblFilterRows(ids []int, specs []tblFilterSpec) []int {
	if len(specs) == 0 {
		return ids
	}
	out := ids[:0]
	for _, id := range ids {
		ok := true
		for _, f := range specs {
			if !f.match(id) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, id)
		}
	}
	return out
}

// tblPage clamps page (1-based) and returns the [start,end) slice of n
// items shown on it.
func tblPage(n, page, size int) (start, end, clamped int) {
	if size <= 0 {
		return 0, n, 1
	}
	pages := max(1, (n+size-1)/size)
	page = max(1, min(page, pages))
	start = (page - 1) * size
	end = min(n, start+size)
	return start, end, page
}

// tblRow is a displayed row.
type tblRow struct {
	data  int
	key   string
	depth int
	kids  bool // has (or may lazily have) children
	open  bool
}

// tblTree flattens a tree: prep filters and sorts each sibling list, open
// reports expanded rows.
func tblTree(roots []int, kids func(int) []int, has func(int) bool, open func(int) bool, key func(int) string, prep func([]int) []int) []tblRow {
	var out []tblRow
	var walk func(ids []int, depth int)
	walk = func(ids []int, depth int) {
		ids = prep(append([]int(nil), ids...))
		for _, id := range ids {
			ch := kids(id)
			r := tblRow{data: id, key: key(id), depth: depth, kids: len(ch) > 0 || (has != nil && has(id))}
			r.open = r.kids && open(id)
			out = append(out, r)
			if r.open && len(ch) > 0 {
				walk(ch, depth+1)
			}
		}
	}
	walk(roots, 0)
	return out
}

// tblCell is a body cell to draw for a merged-cell window: the cell owned by
// position pos and leaf col spans rs rows and cs columns.
type tblCell struct{ pos, col, rs, cs int }

// tblSpanCells resolves merged cells for positions [first,last] over ncols
// columns. f returns (rowSpan, colSpan) of a cell. Cells covered by a
// neighbour are skipped; cells owned by rows above first that reach into
// the window are included (pos < first) so virtual scrolling keeps them.
func tblSpanCells(first, last, ncols int, f func(pos, col int) (int, int), lookback int) []tblCell {
	var out []tblCell
	if f == nil {
		for p := first; p <= last; p++ {
			for col := 0; col < ncols; col++ {
				out = append(out, tblCell{p, col, 1, 1})
			}
		}
		return out
	}
	if first <= last {
		for col := 0; col < ncols; col++ {
			for p := first - 1; p >= 0 && p >= first-lookback; p-- {
				rs, cs := f(p, col)
				if rs <= 0 || cs <= 0 {
					continue
				}
				if p+rs-1 >= first {
					out = append(out, tblCell{p, col, rs, cs})
				}
				break
			}
		}
	}
	for p := first; p <= last; p++ {
		for col := 0; col < ncols; col++ {
			rs, cs := f(p, col)
			if rs <= 0 || cs <= 0 {
				continue
			}
			out = append(out, tblCell{p, col, rs, min(cs, ncols-col)})
		}
	}
	return out
}

// tblRange returns the rows [first,last] intersecting the viewport
// [sy, sy+viewH) plus overscan rows on each side. offs, if not nil, holds
// n+1 prefix offsets (variable heights); otherwise rows are rowH tall.
func tblRange(n int, rowH float32, offs []float64, sy float64, viewH float32, overscan int) (first, last int) {
	if n == 0 {
		return 0, -1
	}
	if offs == nil {
		first = int(sy / float64(rowH))
		last = int(math.Ceil((sy+float64(viewH))/float64(rowH))) - 1
	} else {
		first = sort.Search(n, func(i int) bool { return offs[i+1] > sy }) // first row ending below sy
		last = sort.Search(n, func(i int) bool { return offs[i] >= sy+float64(viewH) }) - 1
	}
	first = max(0, min(first-overscan, n-1))
	last = max(first, min(last+overscan, n-1))
	return first, last
}

// tblOffset returns the top of row i.
func tblOffset(i int, rowH float32, offs []float64) float64 {
	if offs != nil {
		return offs[i]
	}
	return float64(i) * float64(rowH)
}

// tblKey returns the default key of a data row.
func tblKey(f func(int) string, row int) string {
	if f != nil {
		return f(row)
	}
	return strconv.Itoa(row)
}

// tblSelState summarizes the header checkbox.
func tblSelState(n, selected int) (all, some bool) {
	return n > 0 && selected == n, selected > 0 && selected < n
}

// tblToggle adds or removes v.
func tblToggle(s []string, v string, on bool) []string {
	for i, x := range s {
		if x == v {
			if on {
				return s
			}
			return append(s[:i:i], s[i+1:]...)
		}
	}
	if on {
		return append(s, v)
	}
	return s
}

func tblHas(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// tblMove returns the order of n items after moving item from to the index
// of item to (both indices into the current order).
func tblMove(order []int, from, to int) []int {
	if from == to || from < 0 || to < 0 || from >= len(order) || to >= len(order) {
		return order
	}
	v := order[from]
	out := append(order[:from:from], order[from+1:]...)
	out = append(out[:to], append([]int{v}, out[to:]...)...)
	return out
}

// ---------------------------------------------------------------------------
// Variable row heights.

// tblBlock is the number of positions per prefix-sum block of tblVarH.
const tblBlock = 64

// tblVarH holds the slot heights of variable-height rows (wrapped cells).
// Measured heights are cached by data row; rows not measured yet use an
// estimate (the mean of the measured ones). Slots are summed per block of
// tblBlock positions with a lazily updated prefix over the blocks, so an
// offset or a lookup costs O(n/B + B) and a new measurement O(1) until the
// next lookup re-sums the blocks after it.
type tblVarH struct {
	sig string    // what measured heights depend on (widths, data revision)
	ord string    // what the position → row mapping depends on
	h   []float32 // measured row height by data row (0: not measured)
	rnd []float32 // measured Render-cell content height by data row
	sum float64   // of measured heights
	cnt int

	slot  []float32 // height per position (row + detail panel)
	bs    []float64 // sum per block
	pre   []float64 // pre[b] = sum of blocks < b (len nb+1)
	dirty int       // first block whose prefix is stale
}

// est is the height assumed for rows not measured yet.
func (v *tblVarH) est(min float32) float32 {
	if v.cnt == 0 {
		return min
	}
	return max(min, float32(v.sum/float64(v.cnt)))
}

// reset drops all measurements for n data rows.
func (v *tblVarH) reset(rows int) {
	v.h = make([]float32, rows)
	v.rnd = nil
	v.sum, v.cnt = 0, 0
	v.ord = ""
}

// grow makes room for data row d.
func (v *tblVarH) grow(d int) {
	if d >= len(v.h) {
		v.h = append(v.h, make([]float32, d+1-len(v.h))...)
	}
}

// measured returns the cached height of data row d (0 if unknown).
func (v *tblVarH) measured(d int) float32 {
	if d < 0 || d >= len(v.h) {
		return 0
	}
	return v.h[d]
}

// record stores the height of data row d.
func (v *tblVarH) record(d int, h float32) {
	v.grow(d)
	if old := v.h[d]; old > 0 {
		v.sum -= float64(old)
		v.cnt--
	}
	v.h[d] = h
	v.sum += float64(h)
	v.cnt++
}

// build lays out n slots; slotH returns each position's height.
func (v *tblVarH) build(n int, slotH func(pos int) float32) {
	if cap(v.slot) >= n {
		v.slot = v.slot[:n]
	} else {
		v.slot = make([]float32, n)
	}
	nb := (n + tblBlock - 1) / tblBlock
	v.bs = make([]float64, nb)
	v.pre = make([]float64, nb+1)
	for p := 0; p < n; p++ {
		h := slotH(p)
		v.slot[p] = h
		v.bs[p/tblBlock] += float64(h)
	}
	v.dirty = 0
}

// set changes the slot height of position p.
func (v *tblVarH) set(p int, h float32) {
	if p < 0 || p >= len(v.slot) || v.slot[p] == h {
		return
	}
	b := p / tblBlock
	v.bs[b] += float64(h - v.slot[p])
	v.slot[p] = h
	if b < v.dirty {
		v.dirty = b
	}
}

func (v *tblVarH) fix() {
	nb := len(v.bs)
	if v.dirty >= nb {
		return
	}
	for b := v.dirty; b < nb; b++ {
		v.pre[b+1] = v.pre[b] + v.bs[b]
	}
	v.dirty = nb
}

// offset returns the top of position i (i may be n: the total height).
func (v *tblVarH) offset(i int) float64 {
	v.fix()
	n := len(v.slot)
	if i >= n {
		return v.pre[len(v.bs)]
	}
	if i <= 0 {
		return 0
	}
	b := i / tblBlock
	y := v.pre[b]
	for p := b * tblBlock; p < i; p++ {
		y += float64(v.slot[p])
	}
	return y
}

// at returns the position whose slot contains y (clamped).
func (v *tblVarH) at(y float64) int {
	v.fix()
	n := len(v.slot)
	if n == 0 {
		return -1
	}
	nb := len(v.bs)
	// Last block starting at or above y.
	b := sort.Search(nb, func(b int) bool { return v.pre[b+1] > y })
	if b >= nb {
		return n - 1
	}
	top := v.pre[b]
	for p := b * tblBlock; p < min(n, (b+1)*tblBlock); p++ {
		top += float64(v.slot[p])
		if top > y {
			return p
		}
	}
	return min(n-1, (b+1)*tblBlock-1)
}

// rangeOf returns the positions [first,last] intersecting [sy, sy+viewH)
// plus overscan rows on each side.
func (v *tblVarH) rangeOf(sy float64, viewH float32, overscan int) (first, last int) {
	n := len(v.slot)
	if n == 0 {
		return 0, -1
	}
	first = v.at(sy)
	last = v.at(sy + float64(viewH) - 0.001)
	first = max(0, min(first-overscan, n-1))
	last = max(first, min(last+overscan, n-1))
	return first, last
}

// ---------------------------------------------------------------------------
// Tree selection linkage.

// tblTreeParents maps every node of a tree to its parent (roots: -1).
func tblTreeParents(roots []int, kids func(int) []int) map[int]int {
	par := map[int]int{}
	var walk func(ids []int, p int)
	walk = func(ids []int, p int) {
		for _, id := range ids {
			if _, seen := par[id]; seen {
				continue // guard against cycles
			}
			par[id] = p
			walk(kids(id), id)
		}
	}
	walk(roots, -1)
	return par
}

// tblTreeCheck checks or unchecks id with its descendants, then updates
// every ancestor: a parent is checked when all its enabled children are.
// Disabled nodes (dis) keep their state and do not block their parent.
func tblTreeCheck(sel map[int]bool, id int, on bool, kids func(int) []int, par map[int]int, dis func(int) bool) {
	seen := map[int]bool{}
	var down func(id int)
	down = func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		if dis == nil || !dis(id) {
			if on {
				sel[id] = true
			} else {
				delete(sel, id)
			}
		}
		for _, k := range kids(id) {
			down(k)
		}
	}
	down(id)
	for p, ok := par[id]; ok && p >= 0; p, ok = par[p] {
		if dis != nil && dis(p) {
			continue
		}
		all, any := true, false
		for _, k := range kids(p) {
			if dis != nil && dis(k) {
				continue
			}
			any = true
			if !sel[k] {
				all = false
			}
		}
		if all && any {
			sel[p] = true
		} else {
			delete(sel, p)
		}
	}
}

// tblTreeHalf returns the nodes shown half-checked: not checked themselves
// but with a checked descendant.
func tblTreeHalf(roots []int, kids func(int) []int, sel map[int]bool) map[int]bool {
	half := map[int]bool{}
	seen := map[int]bool{}
	var walk func(id int) bool // reports a checked node in the subtree
	walk = func(id int) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		some := false
		for _, k := range kids(id) {
			if walk(k) {
				some = true
			}
		}
		if some && !sel[id] {
			half[id] = true
		}
		return some || sel[id]
	}
	for _, r := range roots {
		walk(r)
	}
	return half
}
