package editor

import (
	"math"
	"math/bits"
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// View lines (Monaco's viewModelLines).
//
// The View shows view lines (rows): a model line that wraps is several
// rows (wrap.go), a model line hidden inside a folded region none
// (folding.go). Viewport.TopLine is a view line; selections stay model
// offsets.
//
// Wrapping is computed lazily: a model line's rows are only known once its
// breaks have been computed, which happens for the lines around the
// viewport (and for lines the caret or a reveal moves to). Unknown lines
// count as one row. The row counts live in a Fenwick tree over the model
// lines, so the view line of a model line and the model line of a view
// line are O(log n) on a document of millions of lines; opening a large
// file with wrapping on allocates the tree (zeroed: every line one row)
// and wraps only the visible lines.
//
// Learning a line's real row count changes the view line numbers of every
// line below it. The State keeps what is shown stable: whenever row counts
// change (lines measured, the text edited, the wrapping column changed,
// regions folded or unfolded) it keeps the first visible model line and
// its row at the top (Viewport.TopLine is moved, the picture is not); the
// View's redraw hints compare frames through that anchor (damage.go).
// Count is therefore an estimate until every line has been seen, as is the
// view line of a line far from the viewport.
//
// The mapping API: ModelToView and ViewToModel convert between model
// positions and view lines (with the content x on the column grid); the
// State's rowOf (measuring) and peekRowOf (not measuring, for paint after
// layout) and the find / viewOf primitives are the same mapping
// (viewLines.locate). A position on a hidden line maps to the end of the
// visible line before it (Monaco's convertModelPositionToViewPosition).
//
// Without wrapping and folding the mapping is the identity and holds no
// per-line state; with folding alone the tree holds the hidden lines only
// (no row counts).
type viewLines struct {
	cfg  wrapConfig
	colW float32 // column width of the last paint, for x

	buf  *buffer.Buffer
	snap buffer.Snapshot // the synced text
	ver  uint64
	n    int // model lines

	// rows[l] is the row count of model line l, 0 when not measured yet
	// (counted as one row); allocated only while wrapping. tree is a
	// Fenwick tree over the lines' extra rows (rows-1; -1 for a hidden
	// line), 1-based, and total the sum of those; allocated while wrapping
	// or folding (mapped).
	rows  []int32
	tree  []int32
	total int

	// hid are the hidden model lines (sorted, disjoint, in the synced
	// text): the folded regions' bodies (folding.go).
	hid []lineSpan

	// breaks of recently measured lines, by model line; cleared when the
	// text or the configuration changes.
	breaks map[int]*lineBreaks

	// computed counts the lines whose breaks were computed (tests).
	computed int
}

// lineSpan is the model lines [a, b] (inclusive).
type lineSpan struct{ a, b int }

// breaksCacheMax bounds viewLines.breaks: a few screens of lines.
const breaksCacheMax = 4096

// on reports whether lines wrap.
func (vl *viewLines) on() bool { return vl.cfg.on() }

// mapped reports whether the mapping is not the identity: lines wrap or
// some are hidden.
func (vl *viewLines) mapped() bool { return vl.cfg.on() || len(vl.hid) > 0 }

// Count returns the number of view lines (see viewLines on estimates).
func (vl *viewLines) Count() int {
	if !vl.mapped() {
		return vl.n
	}
	return vl.n + vl.total
}

// hiddenAt returns the index in hid of the range holding model line l, -1
// when l is shown.
func (vl *viewLines) hiddenAt(l int) int {
	h := vl.hid
	if len(h) == 0 || l < h[0].a {
		return -1
	}
	i, _ := slices.BinarySearchFunc(h, l, func(r lineSpan, l int) int {
		switch {
		case r.b < l:
			return -1
		case r.a > l:
			return 1
		}
		return 0
	})
	if i < len(h) && h[i].a <= l && l <= h[i].b {
		return i
	}
	return -1
}

// hidden reports whether model line l is hidden.
func (vl *viewLines) hidden(l int) bool { return vl.hiddenAt(l) >= 0 }

// visibleLine returns l, or for a hidden line the visible line before its
// hidden range (a folded region's start line).
func (vl *viewLines) visibleLine(l int) (int, bool) {
	if i := vl.hiddenAt(l); i >= 0 {
		return max(0, vl.hid[i].a-1), true
	}
	return l, false
}

// nextVisible returns the first shown model line after l (n when none).
func (vl *viewLines) nextVisible(l int) int {
	l++
	if i := vl.hiddenAt(l); i >= 0 {
		l = vl.hid[i].b + 1
	}
	return l
}

// prevVisible returns the last shown model line before l (-1 when none).
func (vl *viewLines) prevVisible(l int) int {
	l--
	if i := vl.hiddenAt(l); i >= 0 {
		l = vl.hid[i].a - 1
	}
	return l
}

// wrapExtra returns the extra rows of model line l from wrapping (rows-1,
// 0 when unknown or not wrapping).
func (vl *viewLines) wrapExtra(l int) int32 {
	if l < len(vl.rows) {
		if r := vl.rows[l]; r > 0 {
			return r - 1
		}
	}
	return 0
}

// extra returns the extra rows of model line l in the tree: -1 for a
// hidden line, else wrapExtra.
func (vl *viewLines) extra(l int) int32 {
	if vl.hidden(l) {
		return -1
	}
	return vl.wrapExtra(l)
}

// rowsOf returns the row count of model line l: 0 when hidden, 1 when
// unknown or not wrapping.
func (vl *viewLines) rowsOf(l int) int {
	if l < 0 || l >= vl.n {
		return 1
	}
	if vl.hidden(l) {
		return 0
	}
	return int(vl.wrapExtra(l)) + 1
}

// add adds d to the extra rows of model line l in the tree.
func (vl *viewLines) add(l int, d int32) {
	vl.total += int(d)
	for i := l + 1; i < len(vl.tree); i += i & -i {
		vl.tree[i] += d
	}
}

// prefix returns the extra rows of model lines [0, l).
func (vl *viewLines) prefix(l int) int {
	s := 0
	for i := min(l, vl.n, len(vl.tree)-1); i > 0; i -= i & -i {
		s += int(vl.tree[i])
	}
	return s
}

// rebuild recomputes the tree from rows and hid in linear time (it frees
// the tree when the mapping is the identity).
func (vl *viewLines) rebuild() {
	n := vl.n
	if !vl.mapped() {
		vl.tree, vl.total = nil, 0
		return
	}
	if cap(vl.tree) >= n+1 {
		vl.tree = vl.tree[:n+1]
		clear(vl.tree)
	} else {
		vl.tree = make([]int32, n+1, n+1+n/8) // room for added lines
	}
	vl.total = 0
	h := 0 // next range of hid
	for i := 1; i <= n; i++ {
		l := i - 1
		for h < len(vl.hid) && vl.hid[h].b < l {
			h++
		}
		e := vl.wrapExtra(l)
		if h < len(vl.hid) && vl.hid[h].a <= l {
			e = -1
		}
		vl.total += int(e)
		vl.tree[i] += e
		if j := i + i&-i; j <= n {
			vl.tree[j] += vl.tree[i]
		}
	}
}

// setHidden sets the hidden lines (sorted, disjoint, within the synced
// text), updating the tree.
func (vl *viewLines) setHidden(h []lineSpan) {
	if slices.Equal(h, vl.hid) {
		return
	}
	size := 0
	for _, r := range vl.hid {
		size += r.b - r.a + 1
	}
	for _, r := range h {
		size += r.b - r.a + 1
	}
	if len(vl.tree) != vl.n+1 || len(h) == 0 && !vl.on() || size > vl.n/16 {
		vl.hid = h
		vl.rebuild()
		return
	}
	// Incrementally: show the old ranges, hide the new ones.
	for _, r := range vl.hid {
		for l := r.a; l <= r.b; l++ {
			vl.add(l, vl.wrapExtra(l)+1)
		}
	}
	vl.hid = h
	for _, r := range h {
		for l := r.a; l <= r.b; l++ {
			vl.add(l, -vl.wrapExtra(l)-1)
		}
	}
}

// viewOf returns the first view line of model line l (for a hidden line,
// the view line after the visible line before it).
func (vl *viewLines) viewOf(l int) int {
	l = max(0, min(l, vl.n))
	if !vl.mapped() {
		return l
	}
	return l + vl.prefix(l)
}

// find returns the model line showing view line v and the row of v in it
// (v clamped to the view lines). The line is never hidden.
func (vl *viewLines) find(v int) (line, row int) {
	v = max(0, min(v, vl.Count()-1))
	if !vl.mapped() {
		return v, 0
	}
	// The largest pos with sum(rows[0:pos]) <= v: the line holding v
	// (rows are >= 0, so hidden lines before it are skipped).
	pos, rem := 0, v
	for step := 1 << (bits.Len(uint(vl.n)) - 1); step > 0; step >>= 1 {
		if p := pos + step; p <= vl.n && int(vl.tree[p])+step <= rem {
			pos = p
			rem -= int(vl.tree[p]) + step
		}
	}
	if pos >= vl.n {
		l := vl.n - 1
		if vl.hidden(l) {
			l = max(0, vl.prevVisible(l))
		}
		return l, max(0, vl.rowsOf(l)-1)
	}
	return pos, rem
}

// locate is the model-to-view mapping: the view line showing byte column
// col of model line l (up places a row boundary at the end of the earlier
// row), with the visible line and column it is drawn at (a hidden line's
// position is the end of the visible line before it). breaks returns the
// rows of a line, nil when they are not known (the first row is used).
func (vl *viewLines) locate(snap buffer.Snapshot, l, col int, up bool, breaks func(int) *lineBreaks) (vline, line, c int) {
	if !vl.mapped() {
		return l, l, col
	}
	if vis, hidden := vl.visibleLine(l); hidden {
		l, col, up = vis, len(snap.Line(vis)), false
	}
	v := vl.viewOf(l)
	if !vl.on() {
		return v, l, col
	}
	if lb := breaks(l); lb != nil {
		v += lb.rowOf(col, up)
	}
	return v, l, col
}

// reset forgets every measured line (n model lines), keeping the hidden
// lines.
func (vl *viewLines) reset(n int) {
	vl.n = n
	clear(vl.breaks)
	if !vl.on() {
		vl.rows = nil
		vl.rebuild()
		return
	}
	if cap(vl.rows) >= n {
		vl.rows = vl.rows[:n]
		clear(vl.rows)
	} else {
		vl.rows = make([]int32, n)
	}
	vl.rebuild()
}

// dropHidden forgets the hidden lines before the text changes under them
// (the State hides the lines of the new text afterwards: State.lines).
func (vl *viewLines) dropHidden() {
	if len(vl.hid) == 0 {
		return
	}
	vl.setHidden(nil)
}

// sync follows the text to snap of buffer b: incrementally after the
// buffer's last change when it started from the synced version, else by
// forgetting every measured line. Hidden lines are dropped.
func (vl *viewLines) sync(b *buffer.Buffer, snap buffer.Snapshot) {
	if vl.buf == b && vl.ver == snap.Version() && vl.n == snap.LineCount() {
		return
	}
	vl.dropHidden()
	same, prev := vl.buf == b, vl.ver
	vl.buf, vl.ver, vl.snap = b, snap.Version(), snap
	if !vl.on() {
		vl.n = snap.LineCount()
		vl.rebuild()
		return
	}
	clear(vl.breaks)
	if same {
		// Incrementally when the buffer's last change is the only one
		// since the synced version.
		if old, edits := b.LastChange(); old.Version() == prev && old.LineCount() == vl.n && vl.applied(old, edits, snap) {
			return
		}
	}
	vl.reset(snap.LineCount())
}

// syncEdits follows edits (in old's coordinates) from old to snap. Hidden
// lines are dropped.
func (vl *viewLines) syncEdits(b *buffer.Buffer, old buffer.Snapshot, edits []buffer.Edit, snap buffer.Snapshot) {
	if !vl.on() || vl.buf != b || old.LineCount() != vl.n || old.Version() != vl.ver {
		vl.sync(b, snap)
		return
	}
	vl.dropHidden()
	vl.ver, vl.snap = snap.Version(), snap
	clear(vl.breaks)
	if !vl.applied(old, edits, snap) {
		vl.reset(snap.LineCount())
	}
}

// applied applies edits from old to snap to the measured lines: the lines
// an edit touches become unknown. It reports false when the edits do not
// account for snap's line count. There are no hidden lines.
func (vl *viewLines) applied(old buffer.Snapshot, edits []buffer.Edit, snap buffer.Snapshot) bool {
	if len(edits) == 0 {
		return snap.LineCount() == vl.n
	}
	type span struct{ l0, l1, nl int } // old lines [l0, l1] become nl lines
	spans := make([]span, 0, len(edits))
	same := true
	d := 0
	for _, e := range edits {
		if e.Start < 0 || e.End > old.Len() || e.Start > e.End {
			return false
		}
		sp := span{old.LineOf(e.Start), old.LineOf(e.End), lineBreakCount(e.Text) + 1}
		if k := len(spans); k > 0 && spans[k-1].l1 >= sp.l0 {
			// Two edits on one line (sorted, so the previous one ends on
			// the line this one starts on): the shared line counts once.
			p := &spans[k-1]
			p.nl += sp.nl - 1
			p.l1 = sp.l1
		} else {
			spans = append(spans, sp)
		}
	}
	for _, sp := range spans {
		d += sp.nl - (sp.l1 - sp.l0 + 1)
		if sp.nl != sp.l1-sp.l0+1 {
			same = false
		}
	}
	if vl.n+d != snap.LineCount() {
		return false
	}
	if same {
		for _, sp := range spans {
			for l := sp.l0; l <= sp.l1; l++ {
				if e := vl.wrapExtra(l); e != 0 {
					vl.add(l, -e)
				}
				vl.rows[l] = 0
			}
		}
		return true
	}
	if len(spans) <= 8 {
		// In place (a memmove per span; growing by append): an Enter in a
		// document of millions of lines allocates nothing.
		var zero [64]int32
		for i := len(spans) - 1; i >= 0; i-- {
			sp := spans[i]
			ins := zero[:min(sp.nl, len(zero))]
			if sp.nl > len(zero) {
				ins = make([]int32, sp.nl)
			}
			vl.rows = slices.Replace(vl.rows, sp.l0, sp.l1+1, ins...)
		}
	} else {
		rows := make([]int32, 0, snap.LineCount())
		at := 0
		for _, sp := range spans {
			rows = append(rows, vl.rows[at:sp.l0]...)
			rows = append(rows, make([]int32, sp.nl)...)
			at = sp.l1 + 1
		}
		vl.rows = append(rows, vl.rows[at:]...)
	}
	vl.n = len(vl.rows)
	vl.rebuild()
	return true
}

// lineBreakCount returns the number of line breaks in s ("\r\n", "\r"
// and "\n" are one each, as buffer.Snapshot.Apply counts them).
func lineBreakCount(s string) int {
	return strings.Count(s, "\n") + strings.Count(s, "\r") - strings.Count(s, "\r\n")
}

// configure sets the wrapping and the column width; a change forgets every
// measured line. It reports whether the configuration changed.
func (vl *viewLines) configure(cfg wrapConfig, colW float32, b *buffer.Buffer, snap buffer.Snapshot) bool {
	if vl.cfg == cfg && vl.colW == colW && vl.buf == b && vl.ver == snap.Version() {
		return false
	}
	changed := vl.cfg != cfg || vl.colW != colW
	if !changed {
		vl.sync(b, snap)
		return false
	}
	if vl.buf != b || vl.ver != snap.Version() {
		vl.dropHidden() // of another text
	}
	vl.cfg, vl.colW = cfg, colW
	vl.buf, vl.ver, vl.snap = b, snap.Version(), snap
	vl.reset(snap.LineCount())
	return true
}

// breaksOf returns the rows of model line l of snap (synced), computing
// them when needed; it reports whether l's row count changed (the line was
// not measured or its rows differ). Without wrapping it returns one row.
func (vl *viewLines) breaksOf(snap buffer.Snapshot, l int) (*lineBreaks, bool) {
	if !vl.on() || l < 0 || l >= vl.n {
		return &lineBreaks{}, false
	}
	if lb := vl.breaks[l]; lb != nil {
		return lb, false
	}
	return vl.measureText(l, snap.Line(l))
}

// measureText is breaksOf for line l whose text is s.
func (vl *viewLines) measureText(l int, s string) (*lineBreaks, bool) {
	if lb := vl.breaks[l]; lb != nil {
		return lb, false
	}
	lb := computeBreaks(s, vl.cfg)
	vl.computed++
	if vl.breaks == nil || len(vl.breaks) >= breaksCacheMax {
		vl.breaks = make(map[int]*lineBreaks)
	}
	vl.breaks[l] = &lb
	r := int32(lb.rows())
	if vl.rows[l] == r {
		return &lb, false
	}
	old := vl.extra(l)
	vl.rows[l] = r
	d := vl.extra(l) - old // 0 for a hidden line
	if d != 0 {
		vl.add(l, d)
	}
	return &lb, d != 0
}

// ModelToView returns the view line and the content x (on the column
// grid) of byte column col of model line line, in the synced text. A
// column at a row boundary is at the start of the later row; a column on a
// hidden line is at the end of the visible line before it.
func (vl *viewLines) ModelToView(line, col int) (vline int, x float32) {
	snap, tab := vl.snap, max(vl.cfg.tab, 1)
	line = max(0, min(line, snap.LineCount()-1))
	col = max(0, min(col, len(snap.Line(line))))
	measure := func(l int) *lineBreaks { lb, _ := vl.breaksOf(snap, l); return lb }
	vline, line, col = vl.locate(snap, line, col, false, measure)
	s := snap.Line(line)
	if !vl.on() {
		return vline, float32(lineColumnTab(s, col, tab)) * vl.colW
	}
	lb := measure(line)
	k := lb.rowOf(col, false)
	b0, _ := lb.rowRange(k, len(s))
	c := lb.rowIndent(k) + lineColumnTab(s[b0:col], col-b0, tab)
	return vline, float32(c) * vl.colW
}

// ViewToModel returns the model line and the byte column (a grapheme
// boundary, the nearest one on the column grid) at content x of view line
// vline (clamped to the document), in the synced text. Past the end of a
// row that continues on the next one, it returns the row end, which is
// that next row's start. The line is never hidden.
func (vl *viewLines) ViewToModel(vline int, x float32) (line, col int) {
	snap, tab := vl.snap, max(vl.cfg.tab, 1)
	line, row := vl.find(vline)
	s := snap.Line(line)
	goal := 0
	if vl.colW > 0 {
		goal = int(math.Round(float64(x / vl.colW)))
	}
	if !vl.on() {
		return line, nearestColumn(s, goal, tab)
	}
	lb, _ := vl.breaksOf(snap, line)
	row = min(row, lb.rows()-1)
	b0, b1 := lb.rowRange(row, len(s))
	return line, b0 + nearestColumn(s[b0:b1], max(0, goal-lb.rowIndent(row)), tab)
}

// lineColumnTab returns the visual column of byte end of s with tab stops
// every tab columns (State.lineColumn).
func lineColumnTab(s string, end, tab int) int {
	col := 0
	for i := 0; i < end && i < len(s); {
		j := nextGrapheme(s, i)
		col = advanceCol(col, s[i:j], tab)
		i = j
	}
	return col
}

// nearestColumn returns the grapheme boundary of s whose visual column is
// nearest goal (ties before it).
func nearestColumn(s string, goal, tab int) int {
	col := 0
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		next := advanceCol(col, s[i:j], tab)
		if next > goal {
			if goal-col <= next-goal {
				return i
			}
			return j
		}
		col, i = next, j
	}
	return len(s)
}

// --- State ---

// lines returns the State's view lines synced to the current text. When
// the text changed it keeps the top model line and its row at the top
// (see viewLines).
func (s *State) lines() *viewLines {
	vl := &s.vl
	snap := s.Snapshot()
	b := s.Model.Buffer()
	if vl.buf == b && vl.ver == snap.Version() {
		return vl
	}
	if !vl.mapped() && len(s.fold.folds) == 0 {
		vl.sync(b, snap)
		return vl
	}
	line, row := vl.find(s.Viewport.TopLine)
	vl.sync(b, snap)
	s.syncFolds()
	s.anchorTop(line, row)
	return vl
}

// anchorTop sets Viewport.TopLine to row of model line (both clamped),
// measuring the line; a hidden line anchors the visible line before it.
func (s *State) anchorTop(line, row int) {
	vl := s.lines()
	if !vl.mapped() {
		s.Viewport.TopLine = line
		return
	}
	line = max(0, min(line, vl.n-1))
	if vis, hidden := vl.visibleLine(line); hidden {
		line, row = vis, 0
	}
	if !vl.on() {
		s.Viewport.TopLine = vl.viewOf(line)
		return
	}
	lb, _ := vl.breaksOf(s.Snapshot(), line)
	s.Viewport.TopLine = vl.viewOf(line) + max(0, min(row, lb.rows()-1))
}

// topAnchor returns the model line and row at the top of the viewport.
func (s *State) topAnchor() (line, row int) { return s.lines().find(s.Viewport.TopLine) }

// viewCount returns the number of view lines.
func (s *State) viewCount() int { return s.lines().Count() }

// wrapping reports whether lines wrap (as of the last paint).
func (s *State) wrapping() bool { return s.vl.on() }

// setWrap sets the wrapping (from the View's paint), keeping the top model
// line and its row at the top.
func (s *State) setWrap(cfg wrapConfig, colW float32) {
	vl := s.lines()
	line, row := vl.find(s.Viewport.TopLine)
	if vl.configure(cfg, colW, s.Model.Buffer(), s.Snapshot()) {
		s.anchorTop(line, row)
	}
}

// breaks returns the rows of model line l, measuring it (and keeping the
// top of the viewport in place when its row count changed).
func (s *State) breaks(l int) *lineBreaks {
	vl := s.lines()
	if !vl.on() {
		return &lineBreaks{}
	}
	if lb := vl.breaks[l]; lb != nil {
		return lb
	}
	line, row := vl.find(s.Viewport.TopLine)
	lb, changed := vl.breaksOf(s.Snapshot(), l)
	if changed && l < line {
		s.Viewport.TopLine = vl.viewOf(line) + row
	}
	return lb
}

// measure measures model lines [l0, l1] (see breaks).
func (s *State) measure(l0, l1 int) {
	vl := s.lines()
	if !vl.on() {
		return
	}
	snap := s.Snapshot()
	l0, l1 = max(0, l0), min(l1, vl.n-1)
	line, row := vl.find(s.Viewport.TopLine)
	moved := false
	for l := l0; l <= l1; l++ {
		if vl.rows[l] != 0 && vl.breaks[l] != nil {
			continue
		}
		if _, changed := vl.breaksOf(snap, l); changed && l < line {
			moved = true
		}
	}
	if moved {
		s.Viewport.TopLine = vl.viewOf(line) + row
	}
}

// measureViewport measures the lines around the viewport: two lines above
// the top one and enough below it to fill the viewport and two more rows.
func (s *State) measureViewport() {
	vl := s.lines()
	if !vl.on() {
		return
	}
	v := &s.Viewport
	line, _ := vl.find(v.TopLine)
	s.measure(line-2, line)
	line, row := vl.find(v.TopLine)
	need := 3
	if v.LineHeight > 0 && v.Height > 0 {
		need += int(math.Ceil(float64(v.OffsetY+v.Height) / float64(v.LineHeight)))
	}
	snap := s.Snapshot()
	for l, got := line, -row; l < vl.n && got < need; l = vl.nextVisible(l) {
		lb, _ := vl.breaksOf(snap, l)
		got += lb.rows()
	}
}

// headUp reports whether caret k (of Sels), at the boundary between two
// rows of a wrapped line, is drawn at the end of the earlier row (after
// End, or a click or vertical move past that row's end) rather than at the
// start of the later one, as Monaco's view cursor state keeps it. It holds
// while that caret stays there in the same text.
func (s *State) headUp(k int) bool {
	u := &s.ups
	return k >= 0 && k < len(u.heads) && k < len(s.Sels) && u.heads[k] >= 0 && u.heads[k] == s.Sels[k].Head &&
		u.ver == s.Snapshot().Version() && s.vl.on()
}

// caretUp is headUp of the primary caret.
func (s *State) caretUp() bool { return s.headUp(0) }

// setCaretUp sets the primary caret's row affinity (see headUp), keeping
// the others'.
func (s *State) setCaretUp(on bool) {
	u := &s.ups
	if ver := s.Snapshot().Version(); u.ver != ver || len(u.heads) != len(s.Sels) {
		u.ver = ver
		u.heads = make([]int, len(s.Sels))
		for i := range u.heads {
			u.heads[i] = -1
		}
	}
	if len(u.heads) == 0 {
		return
	}
	u.heads[0] = -1
	if on {
		u.heads[0] = s.Sels[0].Head
	}
}

// setUps sets the row affinities after setSels(out): the selections of
// out whose up is true keep it (also when they merged with others).
func (s *State) setUps(out []Selection, up []bool) {
	any := false
	for _, v := range up {
		any = any || v
	}
	if !any {
		return
	}
	m := make(map[Selection]bool, len(out))
	for k, sel := range out {
		if up[k] {
			m[sel] = true
		}
	}
	s.ups = caretAffinity{ver: s.Snapshot().Version(), heads: make([]int, len(s.Sels))}
	for i, sel := range s.Sels {
		s.ups.heads[i] = -1
		if m[sel] {
			s.ups.heads[i] = sel.Head
		}
	}
}

// caretAffinity are the carets' row affinities (State.headUp): heads[k] is
// the head of Sels[k] when it is drawn at the end of the earlier row, -1
// otherwise, in text version ver.
type caretAffinity struct {
	ver   uint64
	heads []int
}

// peekRowOf is rowOf without measuring: a line whose rows are not known
// yet counts as one row. Paint uses it after laying out the rows, so the
// top cannot move under the drawn rows; every line it draws is measured,
// and a line it has not measured is outside the drawn rows anyway.
func (s *State) peekRowOf(off int, up bool) int {
	snap := s.Snapshot()
	off = clampOff(snap, off)
	l := snap.LineOf(off)
	vl := s.lines()
	v, _, _ := vl.locate(snap, l, off-snap.LineStart(l), up, func(l int) *lineBreaks { return vl.breaks[l] })
	return v
}

// rowOf returns the view line showing offset off; up places a row
// boundary at the end of the earlier row. An offset on a hidden line is
// on the last row of the visible line before it.
func (s *State) rowOf(off int, up bool) int {
	snap := s.Snapshot()
	off = clampOff(snap, off)
	l := snap.LineOf(off)
	v, _, _ := s.lines().locate(snap, l, off-snap.LineStart(l), up, s.breaks)
	return v
}

// headRow returns the view line of the primary caret.
func (s *State) headRow() int { return s.rowOf(s.Primary().Head, s.caretUp()) }

// rowPos returns the model line and the row of offset off (for an offset
// on a hidden line, the visible line before it and its last row).
func (s *State) rowPos(off int, up bool) (line, row int) {
	snap := s.Snapshot()
	off = clampOff(snap, off)
	line = snap.LineOf(off)
	vl := s.lines()
	if !vl.mapped() {
		return line, 0
	}
	col := off - snap.LineStart(line)
	if vis, hidden := vl.visibleLine(line); hidden {
		line, col, up = vis, len(snap.Line(vis)), false
	}
	if !vl.on() {
		return line, 0
	}
	return line, s.breaks(line).rowOf(col, up)
}

// stepRows moves (line, row) n rows up (Backward) or down, over the
// hidden lines; it reports false when that leaves the document (above the
// first row or below the last). Rows are measured on the way, so the moves
// are exact.
func (s *State) stepRows(line, row int, dir Direction, n int) (int, int, bool) {
	snap := s.Snapshot()
	nl := snap.LineCount()
	vl := s.lines()
	if !vl.mapped() {
		if dir == Backward {
			line -= n
		} else {
			line += n
		}
		return line, 0, line >= 0 && line < nl
	}
	rows := func(l int) int {
		if !vl.on() {
			return 1
		}
		return s.breaks(l).rows()
	}
	for range n {
		if dir == Backward {
			if row > 0 {
				row--
				continue
			}
			p := vl.prevVisible(line)
			if p < 0 {
				return 0, 0, false
			}
			line = p
			row = rows(line) - 1
			continue
		}
		if row+1 < rows(line) {
			row++
			continue
		}
		nx := vl.nextVisible(line)
		if nx >= nl {
			return line, row, false
		}
		line, row = nx, 0
	}
	return line, row, true
}

// rowText returns the byte range [b0, b1) of row of model line l (whose
// text is text) and the row's indentation in columns.
func (s *State) rowText(l, row int, text string) (b0, b1, indent int) {
	if !s.lines().on() {
		return 0, len(text), 0
	}
	lb := s.breaks(l)
	row = max(0, min(row, lb.rows()-1))
	b0, b1 = lb.rowRange(row, len(text))
	return b0, b1, lb.rowIndent(row)
}

// viewColumn returns the visual column of off within its row (the row's
// indentation included): the goal column of vertical moves by column.
func (s *State) viewColumn(snap buffer.Snapshot, off int, up bool) int {
	if !s.lines().on() {
		return s.column(snap, off)
	}
	line, row := s.rowPos(off, up)
	text := snap.Line(line)
	b0, _, ind := s.rowText(line, row, text)
	col := clampOff(snap, off) - snap.LineStart(line)
	return ind + s.lineColumn(text[b0:], col-b0)
}

// peekViewColumn is viewColumn without measuring (see peekRowOf): the
// column in the model line when its rows are not known.
func (s *State) peekViewColumn(snap buffer.Snapshot, off int, up bool) int {
	vl := s.lines()
	if !vl.on() || vl.breaks[snap.LineOf(clampOff(snap, off))] == nil {
		return s.column(snap, off)
	}
	return s.viewColumn(snap, off, up)
}

// offsetInRow returns the offset at visual column goal (indentation
// included) of row of model line l: the last cluster boundary at or before
// it. up reports that it is the end of a row that continues on the next
// one (the caret drawn there: headUp).
func (s *State) offsetInRow(snap buffer.Snapshot, l, row, goal int) (off int, up bool) {
	text := snap.Line(l)
	b0, b1, ind := s.rowText(l, row, text)
	rt := text[b0:b1]
	c := s.offsetForColumn(rt, max(0, goal-ind))
	return snap.LineStart(l) + b0 + c, b1 < len(text) && c == len(rt) && c > 0
}

// rowFirstNonBlank returns the byte column of the first non-blank of row
// (its start when it is blank) of the line text.
func rowFirstNonBlank(text string, b0, b1 int) int {
	t := text[b0:b1]
	if i := len(t) - len(strings.TrimLeft(t, " \t")); i < len(t) {
		return b0 + i
	}
	return b0
}

// lineEdgeTarget is Home / End from off with wrapping (Monaco's
// moveToBeginningOfLine / moveToEndOfLine): on a continuation row Home
// goes to the row's first non-blank first, End on a row that continues
// goes to the row's end first (the caret drawn there, up), then both to
// the model line's edges. ok is false when the model line's edges apply.
func (s *State) lineEdgeTarget(dir Direction, off int, up bool) (to int, toUp, ok bool) {
	snap := s.Snapshot()
	if !s.lines().on() {
		return 0, false, false
	}
	l := snap.LineOf(off)
	lb := s.breaks(l)
	if lb.rows() == 1 {
		return 0, false, false
	}
	ls := snap.LineStart(l)
	text := snap.Line(l)
	col := off - ls
	row := lb.rowOf(col, up)
	b0, b1 := lb.rowRange(row, len(text))
	if dir == Backward {
		if row == 0 {
			return 0, false, false
		}
		fnb := rowFirstNonBlank(text, b0, b1)
		if col == fnb {
			return 0, false, false
		}
		return ls + fnb, false, true
	}
	if row == lb.rows()-1 || col == b1 {
		return 0, false, false
	}
	return ls + b1, true, true
}
