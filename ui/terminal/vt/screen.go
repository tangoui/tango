package vt

// Screen is a grid of rows lines plus up to scrollback lines that have
// scrolled off its top. Lines live in a ring buffer, so scrolling moves
// an index instead of copying, and a line evicted from a full scrollback
// is reused, cells and all, as the new bottom row.
//
// The active area is rows 0 to rows-1, read with [Screen.Line]. The
// viewport is what a view shows: rows lines ending ViewOffset lines
// above the bottom of the active area, read with [Screen.ViewLine].
//
// A Screen is not safe for concurrent use.
type Screen struct {
	cols, rows int
	scrollback int // capacity of the scrollback, in lines

	ring   []Line // len rows+scrollback; grows lazily up to that
	head   int    // ring index of the oldest line (scrollback or row 0)
	sbLen  int    // lines in the scrollback
	offset int    // viewport offset from the bottom, in [0, sbLen]

	styles StyleTable
}

// NewScreen returns a blank screen of cols×rows cells keeping up to
// scrollback lines of history. A screen with scrollback 0, such as the
// alternate screen, drops lines that scroll off the top. cols and rows
// are raised to at least 1 and scrollback to at least 0.
func NewScreen(cols, rows, scrollback int) *Screen {
	cols, rows, scrollback = max(cols, 1), max(rows, 1), max(scrollback, 0)
	s := &Screen{cols: cols, rows: rows, scrollback: scrollback}
	// Scrollback lines are allocated as they are first needed.
	s.ring = make([]Line, rows, rows+min(scrollback, 1024))
	for i := range s.ring {
		s.ring[i].clear(cols)
	}
	s.styles.init(s)
	return s
}

// Size returns the screen's size in cells.
func (s *Screen) Size() (cols, rows int) { return s.cols, s.rows }

// Styles returns the table that cells' Style fields index.
func (s *Screen) Styles() *StyleTable { return &s.styles }

// ScrollbackLen returns the number of lines in the scrollback.
func (s *Screen) ScrollbackLen() int { return s.sbLen }

// at returns the line at absolute position i, where 0 is the oldest
// scrollback line and sbLen is active row 0.
func (s *Screen) at(i int) *Line {
	i += s.head
	if i >= len(s.ring) {
		i -= len(s.ring)
	}
	return &s.ring[i]
}

// Line returns active row y, 0 ≤ y < rows. It panics if y is out of range.
// The pointer is valid until the next scroll, which moves lines between
// rows.
func (s *Screen) Line(y int) *Line {
	if uint(y) >= uint(s.rows) {
		panic("vt: Screen.Line row out of range")
	}
	return s.at(s.sbLen + y)
}

// ViewLine returns row y of the viewport, 0 ≤ y < rows. It panics if y is
// out of range. As with [Screen.Line], the pointer is valid until the
// next scroll.
func (s *Screen) ViewLine(y int) *Line {
	if uint(y) >= uint(s.rows) {
		panic("vt: Screen.ViewLine row out of range")
	}
	return s.at(s.sbLen - s.offset + y)
}

// ViewOffset returns how many lines the viewport is scrolled up from the
// bottom; 0 means it shows the active area.
func (s *Screen) ViewOffset() int { return s.offset }

// SetViewOffset scrolls the viewport n lines up from the bottom, clamped
// to [0, ScrollbackLen].
func (s *Screen) SetViewOffset(n int) { s.offset = min(max(n, 0), s.sbLen) }

// ScrollUp scrolls active rows top through bottom (inclusive) up one line
// and blanks row bottom. If top is 0 and the screen keeps scrollback, the
// line scrolled off goes into the scrollback (evicting the oldest line
// when it is full) and a viewport scrolled up keeps showing the same
// content; otherwise it is discarded. Rows outside the region do not
// move. Out-of-range or empty regions are ignored.
func (s *Screen) ScrollUp(top, bottom int) {
	if top < 0 || bottom >= s.rows || top > bottom {
		return
	}
	if top > 0 || s.scrollback == 0 {
		s.rotateUp(top, bottom)
		return
	}

	// Row 0 joins the scrollback by advancing the active area one slot.
	if s.sbLen < s.scrollback {
		s.sbLen++
		if s.sbLen+s.rows > len(s.ring) {
			// The slot is new: grow the ring. Until the ring is full,
			// head is 0, so appending keeps the order.
			s.ring = append(s.ring, Line{})
		}
	} else {
		s.head++ // evict the oldest line; its slot becomes the new last row
		s.styles.evicted++
		if s.head == len(s.ring) {
			s.head = 0
		}
	}
	if s.offset > 0 {
		s.offset = min(s.offset+1, s.sbLen)
	}
	// The slot now at the last row is free. Rows below the region keep
	// their place, so move the free slot up to row bottom.
	free := *s.Line(s.rows - 1)
	for y := s.rows - 1; y > bottom; y-- {
		*s.Line(y) = *s.Line(y - 1)
		s.Line(y).Dirty = true
	}
	free.clear(s.cols)
	*s.Line(bottom) = free
	for y := 0; y < bottom; y++ {
		s.Line(y).Dirty = true
	}
}

// rotateUp moves rows top+1..bottom up one row and reuses row top's line
// as a blank row bottom.
func (s *Screen) rotateUp(top, bottom int) {
	free := *s.Line(top)
	for y := top; y < bottom; y++ {
		*s.Line(y) = *s.Line(y + 1)
		s.Line(y).Dirty = true
	}
	free.clear(s.cols)
	*s.Line(bottom) = free
}

// shiftRows moves rows top through bottom up n rows (down for negative
// n), in place and in time proportional to the region. The |n| rows
// freed at the other end are blanked. 0 < |n| ≤ bottom-top+1.
func (s *Screen) shiftRows(top, bottom, n int) {
	reverse := func(a, b int) {
		for ; a < b; a, b = a+1, b-1 {
			la, lb := s.Line(a), s.Line(b)
			*la, *lb = *lb, *la
		}
	}
	// Rotating left by k is three reversals.
	k := n
	if n < 0 {
		k = bottom - top + 1 + n
	}
	reverse(top, top+k-1)
	reverse(top+k, bottom)
	reverse(top, bottom)
	for y := top; y <= bottom; y++ {
		s.Line(y).Dirty = true
	}
	if n > 0 {
		for y := bottom - n + 1; y <= bottom; y++ {
			s.Line(y).clear(s.cols)
		}
	} else {
		for y := top; y < top-n; y++ {
			s.Line(y).clear(s.cols)
		}
	}
}

// ScrollDown scrolls active rows top through bottom (inclusive) down one
// line and blanks row top. Nothing enters or leaves the scrollback. Out
// of range or empty regions are ignored.
func (s *Screen) ScrollDown(top, bottom int) {
	if top < 0 || bottom >= s.rows || top > bottom {
		return
	}
	s.shiftRows(top, bottom, -1)
}

// setLines replaces the ring with lines, the scrollback first, of which
// the first sb are scrollback.
func (s *Screen) setLines(lines []Line, sb int) {
	ring := make([]Line, len(lines), max(len(lines), s.rows+min(s.scrollback, 1024)))
	copy(ring, lines)
	s.ring, s.head, s.sbLen = ring, 0, sb
	s.offset = min(s.offset, sb)
}

// lines returns a copy of the line headers in order, scrollback first.
func (s *Screen) lines() []Line {
	out := make([]Line, s.sbLen+s.rows)
	for i := range out {
		out[i] = *s.at(i)
	}
	return out
}

// ClearScrollback drops every scrollback line and returns the viewport
// to the bottom.
func (s *Screen) ClearScrollback() {
	if s.sbLen == 0 {
		return
	}
	all := s.lines()
	s.setLines(all[s.sbLen:], 0)
	s.offset = 0
	s.styles.resetBackoff()
}

// reset blanks the screen, drops the scrollback and empties the style
// table.
func (s *Screen) reset() {
	s.ClearScrollback()
	for y := range s.rows {
		s.Line(y).clear(s.cols)
	}
	s.offset = 0
	s.styles.reset()
}

// resize changes the screen to cols×rows and returns the new row of the
// line the cursor was on (cursorY). Lines are not reflowed: each is
// truncated or padded with blanks, scrollback included.
//
// As in xterm.js, losing rows first drops lines below the cursor and then
// moves lines from the top into the scrollback, so the cursor's line stays
// on screen; gaining rows pulls lines back from the scrollback while the
// cursor is on the last row and otherwise adds blank rows at the bottom.
func (s *Screen) resize(cols, rows, cursorY int) int {
	cols, rows = max(cols, 1), max(rows, 1)
	cursorY = min(max(cursorY, 0), s.rows-1)
	if cols == s.cols && rows == s.rows {
		return cursorY
	}
	all, sb := s.lines(), s.sbLen
	for r := s.rows; r > rows; r-- {
		if len(all)-sb-1 > cursorY {
			all = all[:len(all)-1]
		} else {
			sb++
			cursorY--
		}
	}
	for r := s.rows; r < rows; r++ {
		if sb > 0 && cursorY == r-1 {
			sb--
			cursorY++
		} else {
			var l Line
			l.clear(cols)
			all = append(all, l)
		}
	}
	if sb > s.scrollback {
		all = all[sb-s.scrollback:]
		sb = s.scrollback
	}
	for i := range all {
		all[i].resize(cols)
		all[i].Dirty = true
	}
	s.cols, s.rows = cols, rows
	s.setLines(all, sb)
	s.styles.resetBackoff()
	return cursorY
}

// maxStyles is the number of entries a StyleTable holds before it
// compacts: every uint16 index except 0xFFFF.
const maxStyles = 0xFFFF

// StyleTable stores each distinct [Style] once and hands out 16-bit
// indexes for cells to refer to. Index 0 is always the default style.
// When the table is full, interning a new style first drops the entries
// no cell of the owning screen uses, renumbering the cells that remain.
//
// Compacting scans every cell of the screen, so it is rate-limited: after
// a compaction that freed less than an eighth of the table, the next one
// waits until an eighth of the table's worth of styles have been refused
// (interned as 0) or an eighth of the stored lines have been evicted from
// the scrollback. Until then a new style on a full table is the default.
type StyleTable struct {
	styles []Style
	index  map[Style]uint16
	limit  int     // capacity; maxStyles except in tests
	screen *Screen // whose cells to scan when compacting

	backoff  bool // the last compaction freed too little to retry at once
	refused  int  // Intern calls answered 0 since the last compaction
	evicted  int  // scrollback lines evicted since the last compaction
	compacts int  // compactions run; read by tests

	// gen changes whenever indexes may have been renumbered, so a cached
	// index is valid only while gen is the value it was cached with.
	gen uint32
}

func (t *StyleTable) init(s *Screen) {
	t.styles = []Style{{}}
	t.index = map[Style]uint16{{}: 0}
	t.limit = maxStyles
	t.screen = s
}

// Len returns the number of styles in the table, including the default.
func (t *StyleTable) Len() int { return len(t.styles) }

// Get returns the style at index i. An index the table does not hold
// returns the default style.
func (t *StyleTable) Get(i uint16) Style {
	if int(i) < len(t.styles) {
		return t.styles[i]
	}
	return Style{}
}

// Intern returns st's index, adding it if needed. Adding to a full table
// compacts it first, which renumbers the screen's cells; indexes obtained
// earlier and not stored in a cell are then invalid. If every entry is in
// use, Intern returns 0 (the default style).
func (t *StyleTable) Intern(st Style) uint16 {
	if i, ok := t.index[st]; ok {
		return i
	}
	if len(t.styles) >= t.limit {
		if t.mayCompact() {
			t.Compact()
		}
		if len(t.styles) >= t.limit {
			t.refused++
			return 0
		}
	}
	i := uint16(len(t.styles))
	t.styles = append(t.styles, st)
	t.index[st] = i
	return i
}

// resetBackoff lets the next Intern on a full table compact at once.
// Callers use it after freeing many cells in bulk (erasing the screen or
// the scrollback, resizing), when a compaction is likely to pay off.
func (t *StyleTable) resetBackoff() {
	t.backoff = false
	t.refused, t.evicted = 0, 0
}

// reset empties the table down to the default style. Only valid when no
// cell refers to another index.
func (t *StyleTable) reset() {
	clear(t.styles[1:])
	t.styles = t.styles[:1]
	clear(t.index)
	t.index[Style{}] = 0
	t.resetBackoff()
	t.gen++
}

// mayCompact reports whether a full table should try compacting now.
func (t *StyleTable) mayCompact() bool {
	if !t.backoff {
		return true
	}
	step := max(t.limit/8, 1)
	lines := max((t.screen.sbLen+t.screen.rows)/8, 1)
	return t.refused >= step || t.evicted >= lines
}

// Compact removes the styles no cell of the screen uses and renumbers the
// remaining ones, updating the cells. The default style stays at index 0.
func (t *StyleTable) Compact() {
	t.compacts++
	t.gen++
	before := len(t.styles)
	defer func() {
		t.backoff = before-len(t.styles) < max(t.limit/8, 1)
		t.refused, t.evicted = 0, 0
	}()
	used := make([]bool, len(t.styles))
	used[0] = true
	s := t.screen
	n := s.sbLen + s.rows
	for i := range n {
		for _, c := range s.at(i).Cells {
			if int(c.Style) < len(used) {
				used[c.Style] = true
			}
		}
	}
	remap := make([]uint16, len(t.styles))
	kept := t.styles[:1]
	clear(t.index)
	t.index[Style{}] = 0
	for i := 1; i < len(t.styles); i++ {
		if !used[i] {
			continue
		}
		remap[i] = uint16(len(kept))
		t.index[t.styles[i]] = uint16(len(kept))
		kept = append(kept, t.styles[i])
	}
	if len(kept) == len(t.styles) {
		return
	}
	clear(t.styles[len(kept):])
	t.styles = kept
	for i := range n {
		cells := s.at(i).Cells
		for j := range cells {
			if int(cells[j].Style) < len(remap) {
				cells[j].Style = remap[cells[j].Style]
			}
		}
	}
}
