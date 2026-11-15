package ui

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// txView: the virtualized monospace text viewport of CodeBlock and LogView.
//
// Lines come from callbacks, so the viewer holds no copy of the text. Only
// the rows inside the viewport are shaped (through a text.LineCache) and
// painted, in one node. Geometry works in character cells: a line's width
// is its cell count (wide CJK and emoji characters take two cells, a tab
// advances to the next multiple of txvTab), which makes the horizontal
// extent and soft wrapping of a million lines a matter of integers. With
// wrapping on, a rune belongs to visual row floor(startCell / cols), so a
// line of n cells takes exactly ceil(n / cols) rows; a wide character at a
// row end may overhang it by one cell.
//
// Selection is (line, rune column); it is plain text and copies with
// Cmd/Ctrl+C. The viewer is one Tab stop: arrows, Page Up/Down and
// Home/End scroll it.

const (
	txvTab      = 4     // tab stop in cells
	txvMaxRunes = 10000 // runes of one line shown without wrapping
	txvOverscan = 2     // rows built beyond the viewport
	txvMinCols  = 8     // narrowest wrap width in cells
)

// txvRun styles bytes [start, end) of a line. Zero colors use the viewer's
// defaults.
type txvRun struct {
	start, end                      int
	fg, bg                          theme.Color
	bold, italic, underline, strike bool
	faint                           bool
}

// txvMark is a search match: bytes [start, end) of a line.
type txvMark struct {
	start, end int
	current    bool
}

// txvPos is a caret position: a line and a rune index in it.
type txvPos struct{ line, col int }

func (a txvPos) less(b txvPos) bool { return a.line < b.line || (a.line == b.line && a.col < b.col) }

type txvState struct {
	anc, cur txvPos // selection; anc == cur: none

	// Soft wrap: rowStart[i] is the first visual row of line i (len
	// lines+1), valid for wrapCols / wrapVer and the first wrapLines lines.
	rowStart  []int32
	wrapLines int
	wrapCols  int
	wrapVer   uint64

	// Widest line in cells over the first cellLines lines (cellVer).
	maxCells  int
	cellLines int
	cellVer   uint64

	cache    *text.LineCache
	charW    float32 // advance of one cell
	charSize float32 // font size charW was measured at

	// Geometry of the last paint (window coordinates).
	originY float32   // content y 0 (row 0 is at originY + padY)
	textX   float32   // x of cell 0 of the text
	view    geom.Rect // viewport
	gutterW float32
	painted bool

	shift int // rows removed from the top since the last frame (dropFront)
}

type txView struct {
	c       *Context
	st      *txvState
	sb      *ScrollBox
	content *node
	ia      Interaction

	count   int                                  // lines
	line    func(i int) string                   // text of line i (no "\n")
	runs    func(i int, dst []txvRun) []txvRun   // style runs of line i (nil: plain)
	cells   func(i int) int                      // display cells of line i
	marks   func(i int, dst []txvMark) []txvMark // search matches of line i (nil: none)
	lineBg  func(i int) theme.Color              // tint of line i (nil: none)
	numbers func(i int) int                      // gutter number of line i (nil: no gutter)
	maxNo   int                                  // widest gutter number
	ver     uint64                               // bumps when lines change other than by appending
	wrap    bool
	size    float32
	lineH   float32
	padX    float32
	padY    float32
	fg      theme.Color
	gutter  theme.Color
	gutterB theme.Color // gutter rule (zero: none)
	selBg   theme.Color
	match   theme.Color
	matchC  theme.Color
	matchB  theme.Color // outline of the current match
	name    string      // accessible name

	first, end int // visual rows built this frame
	selChanged bool
}

// newTxView prepares a viewer; set its fields, then call show.
func newTxView(c *Context, id ID) *txView {
	st := State(c, id, func() txvState { return txvState{cache: text.NewLineCache(512)} })
	return &txView{c: c, st: st, size: 13, lineH: 20, padX: 8, padY: 4}
}

func (v *txView) mono() text.Style {
	return text.Style{Size: v.size, LineHeight: v.lineH, Mono: true}
}

// cellW is the advance of one character cell.
func (v *txView) cellW() float32 {
	st := v.st
	if st.charW == 0 || st.charSize != v.size {
		st.charW = max(1, text.Width("0000000000", v.mono())/10)
		st.charSize = v.size
	}
	return st.charW
}

// txvCells returns the display cells of s.
func txvCells(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < utf8.RuneSelf {
			if b == '\t' {
				n += txvTab - n%txvTab
			} else {
				n++
			}
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		n += txvRuneCells(r)
		i += sz - 1
	}
	return n
}

// txvRuneCells is the cell width of a non-tab rune: 2 for East Asian wide
// and fullwidth characters and most emoji, 0 for combining marks.
func txvRuneCells(r rune) int {
	switch {
	case r < 0x1100:
		if r >= 0x300 && r < 0x370 {
			return 0
		}
		return 1
	case r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf && r != 0x303f, r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe4f, r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6, r >= 0x1f300 && r <= 0x1f64f, r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	case unicode.Is(unicode.Mn, r), r == 0x200b, r == 0x200d, r >= 0xfe00 && r <= 0xfe0f:
		return 0
	}
	return 1
}

// cols is the wrap width in cells for the current viewport.
func (v *txView) cols() int {
	w := v.st.view.W
	if w <= 0 {
		w = 600
	}
	w -= v.gutterWidth() + 2*v.padX
	return max(txvMinCols, int(w/v.cellW()))
}

func (v *txView) gutterWidth() float32 {
	if v.numbers == nil {
		return 0
	}
	digits := max(2, len(itoaEnt(max(v.maxNo, 1))))
	return float32(digits)*v.cellW() + 2*v.padX
}

// syncWrap brings rowStart up to date (appended lines are added; other
// changes rebuild it).
func (v *txView) syncWrap() {
	st := v.st
	cols := v.cols()
	if st.wrapCols != cols || st.wrapVer != v.ver || st.wrapLines > v.count || len(st.rowStart) == 0 {
		st.wrapCols, st.wrapVer, st.wrapLines = cols, v.ver, 0
		st.rowStart = append(st.rowStart[:0], 0)
	}
	for i := st.wrapLines; i < v.count; i++ {
		n := v.cells(i)
		rows := 1
		if n > 0 {
			rows = (n + cols - 1) / cols
		}
		st.rowStart = append(st.rowStart, st.rowStart[len(st.rowStart)-1]+int32(rows))
	}
	st.wrapLines = v.count
}

// syncCells keeps the widest line up to date.
func (v *txView) syncCells() {
	st := v.st
	if st.cellVer != v.ver || st.cellLines > v.count {
		st.cellVer, st.cellLines, st.maxCells = v.ver, 0, 0
	}
	for i := st.cellLines; i < v.count; i++ {
		st.maxCells = max(st.maxCells, min(v.cells(i), txvMaxRunes+2))
	}
	st.cellLines = v.count
}

func (v *txView) rows() int {
	if v.wrap {
		rs := v.st.rowStart
		return int(rs[v.count] - rs[0])
	}
	return v.count
}

// dropFront tells the viewer that its first k lines were removed (a log
// buffer at its limit): cached geometry and the selection move up.
func (st *txvState) dropFront(k int) {
	if k <= 0 {
		return
	}
	rows := k
	if k >= st.wrapLines {
		st.wrapLines, st.wrapCols = 0, 0
	} else {
		if len(st.rowStart) > k {
			rows = int(st.rowStart[k] - st.rowStart[0])
		}
		st.rowStart = st.rowStart[k:]
		st.wrapLines -= k
	}
	st.shift += rows
	st.cellLines = max(0, st.cellLines-k) // the widest line may be gone: harmless
	st.anc.line -= k
	st.cur.line -= k
	if st.cur.line < 0 && st.anc.line < 0 {
		st.anc, st.cur = txvPos{}, txvPos{}
	}
	st.anc.line, st.cur.line = max(0, st.anc.line), max(0, st.cur.line)
}

// rowLine returns the line and wrap segment of visual row r.
func (v *txView) rowLine(r int) (line, seg int) {
	if !v.wrap {
		return r, 0
	}
	rs := v.st.rowStart[:v.count+1]
	r += int(rs[0])
	i := sort.Search(v.count, func(i int) bool { return int(rs[i+1]) > r })
	i = min(i, v.count-1)
	return i, r - int(rs[i])
}

// lineRow returns the first visual row of line i.
func (v *txView) lineRow(i int) int {
	if !v.wrap {
		return i
	}
	rs := v.st.rowStart
	return int(rs[max(0, min(i, v.count))] - rs[0])
}

// segment returns the bytes [b0, b1) and first rune index of wrap segment
// seg of s (the whole line, capped at txvMaxRunes, without wrapping).
func (v *txView) segment(s string, seg int) (b0, b1, r0 int) {
	if !v.wrap {
		if len(s) <= txvMaxRunes {
			return 0, len(s), 0
		}
		n := 0
		for i := range s {
			if n == txvMaxRunes {
				return 0, i, 0
			}
			n++
		}
		return 0, len(s), 0
	}
	cols := v.st.wrapCols
	lo, hi := seg*cols, (seg+1)*cols
	cell, ri := 0, 0
	b0, b1 = -1, len(s)
	for i, r := range s {
		w := 1
		if r == '\t' {
			w = txvTab - cell%txvTab
		} else if r >= utf8.RuneSelf {
			w = txvRuneCells(r)
		}
		if cell >= lo && b0 < 0 {
			b0, r0 = i, ri
		}
		if cell >= hi {
			b1 = i
			break
		}
		cell += w
		ri++
	}
	if b0 < 0 {
		return len(s), len(s), ri
	}
	return b0, b1, r0
}

// show builds the viewport into the current parent.
func (v *txView) show(kind string, opts []layout.Option) {
	c, st := v.c, v.st
	if v.wrap {
		v.syncWrap()
	} else {
		v.syncCells()
	}
	// Clamp the selection to the text.
	if v.count == 0 {
		st.anc, st.cur = txvPos{}, txvPos{}
	} else {
		st.anc.line, st.cur.line = min(st.anc.line, v.count-1), min(st.cur.line, v.count-1)
	}
	b := c.box(kind, layout.Column, opts)
	b.n.ln.ScrollY = true
	b.n.ln.ScrollX = !v.wrap
	v.sb = c.finishScroll(b, nil)
	s := v.sb.s
	rowH := float64(v.lineH)
	rows := v.rows()
	total := float64(rows)*rowH + 2*float64(v.padY)
	cw := v.cellW()
	gw := v.gutterWidth()
	contentW := float32(0)
	if !v.wrap {
		contentW = gw + 2*v.padX + float32(st.maxCells)*cw + 2
	}
	s.extent = vec64{X: float64(contentW), Y: total}
	if st.shift > 0 {
		// Keep the visible lines in place when lines above them went away.
		s.off.Y = max(0, s.off.Y-float64(st.shift)*float64(v.lineH))
		st.shift = 0
	}
	view := fillViewH(v.sb, 200)
	off := max(0, min(s.off.Y, total-view))
	v.first, v.end = fillSpan(off-float64(v.padY), view, rowH, rows, txvOverscan)
	s.base.Y = float64(v.first) * rowH
	v.sb.Body(func() {
		n := c.add(kind+"-text", "")
		v.content = n
		n.ln.Width = max(contentW, st.view.W)
		n.ln.Height = max(float32(v.end-v.first)*v.lineH+2*v.padY, float32(view))
		v.ia = c.interact(n, true, input.CursorText)
		n.ownKeys = true
		n.setRole(a11y.RoleTextbox).setState(a11y.ReadOnly | a11y.Multiline)
		n.setName(v.name)
		n.fill(v)
		v.handlePointer()
		n.paint = v.paint
		n.over = func(l *paint.List, r geom.Rect) {
			if v.ia.FocusVisible {
				drawFocusRing(l, c.Theme, st.view, geom.Rad(c.Theme.RadiusSM))
			}
		}
	})
	v.handleKeys()
}

// semantics exposes the visible lines as the value.
func (v *txView) semantics(s *a11y.Node) {
	var b strings.Builder
	for r := v.first; r < v.end; r++ {
		line, seg := v.rowLine(r)
		if seg > 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(v.line(line))
		if b.Len() > 4096 {
			break
		}
	}
	s.Value = b.String()
	s.RowCount = v.count
}

// posAt maps a window point to a caret position (clamped to the text).
func (v *txView) posAt(p geom.Point) txvPos {
	st := v.st
	if v.count == 0 {
		return txvPos{}
	}
	y := p.Y - st.originY - v.padY
	r := int(math.Floor(float64(y / v.lineH)))
	rows := v.rows()
	if r < 0 {
		return txvPos{0, 0}
	}
	if r >= rows {
		last := v.count - 1
		return txvPos{last, utf8.RuneCountInString(v.line(last))}
	}
	line, seg := v.rowLine(r)
	s := v.line(line)
	b0, b1, r0 := v.segment(s, seg)
	x := p.X - st.textX
	if x <= 0 {
		return txvPos{line, r0}
	}
	lay := st.cache.Shape(s[b0:b1], []text.Run{{Start: 0, End: b1 - b0, Style: v.mono()}}, text.LineOptions{TabSize: txvTab})
	idx := lay.IndexAt(x, 0)
	return txvPos{line, r0 + idx}
}

// handlePointer runs selection by pointer (press, drag with auto-scroll,
// double click word, triple click line, press in the gutter: line).
func (v *txView) handlePointer() {
	c, st := v.c, v.st
	if !st.painted || v.count == 0 {
		return
	}
	cid := v.content.id
	p := c.pointer
	if v.ia.PressedNow {
		pos := v.posAt(p)
		inGutter := v.numbers != nil && p.X < st.view.X+st.gutterW
		switch {
		case c.clickCount >= 3 || inGutter:
			st.anc = txvPos{pos.line, 0}
			st.cur = txvPos{pos.line, utf8.RuneCountInString(v.line(pos.line))}
		case c.clickCount == 2:
			a, b := txvWord(v.line(pos.line), pos.col)
			st.anc, st.cur = txvPos{pos.line, a}, txvPos{pos.line, b}
		default:
			if c.pointerMods&input.ModShift != 0 && st.anc != st.cur {
				st.cur = pos
			} else {
				st.anc, st.cur = pos, pos
			}
		}
		v.selChanged = true
		c.invalid = true
		return
	}
	if !c.Dragging(cid) {
		return
	}
	// Auto-scroll while the pointer is beyond the viewport.
	s := v.sb.s
	var dy, dx float64
	switch {
	case p.Y < st.view.Y:
		dy = float64(p.Y - st.view.Y)
	case p.Y > st.view.Bottom():
		dy = float64(p.Y - st.view.Bottom())
	}
	if !v.wrap {
		switch {
		case p.X < st.view.X+st.gutterW:
			dx = float64(p.X - st.view.X - st.gutterW)
		case p.X > st.view.Right():
			dx = float64(p.X - st.view.Right())
		}
	}
	if dy != 0 || dx != 0 {
		s.off.Y = max(0, s.off.Y+dy/2)
		s.off.X = max(0, s.off.X+dx/2)
		c.WakeAt(c.now.Add(16 * time.Millisecond))
	}
	if pos := v.posAt(p); pos != st.cur {
		st.cur = pos
		v.selChanged = true
		c.invalid = true
	}
}

func (v *txView) handleKeys() {
	c, st := v.c, v.st
	if c.focus != v.content.id {
		return
	}
	s := v.sb.s
	rowH := float64(v.lineH)
	page := max(rowH, float64(st.view.H)-rowH)
	for _, k := range c.KeysFor(v.content.id) {
		if !k.Down {
			continue
		}
		if k.Mods.Shortcut() {
			switch k.Key {
			case input.KeyA:
				v.selectAll()
			case input.KeyC:
				if t := v.selection(); t != "" && c.win != nil {
					c.win.WriteClipboard(t)
				}
			case input.KeyHome, input.KeyUp:
				s.off.Y = 0
			case input.KeyEnd, input.KeyDown:
				s.off.Y = math.MaxFloat32
			}
			c.invalid = true
			continue
		}
		switch k.Key {
		case input.KeyUp:
			s.off.Y -= rowH
		case input.KeyDown:
			s.off.Y += rowH
		case input.KeyPageUp:
			s.off.Y -= page
		case input.KeyPageDown, input.KeySpace:
			s.off.Y += page
		case input.KeyHome:
			s.off.Y, s.off.X = 0, 0
		case input.KeyEnd:
			s.off.Y = math.MaxFloat32
		case input.KeyLeft:
			s.off.X -= float64(4 * v.cellW())
		case input.KeyRight:
			s.off.X += float64(4 * v.cellW())
		case input.KeyEscape:
			st.cur = st.anc
		default:
			continue
		}
		s.off.X = max(0, s.off.X)
		s.off.Y = max(0, s.off.Y)
		c.invalid = true
	}
}

func (v *txView) selectAll() {
	if v.count == 0 {
		return
	}
	last := v.count - 1
	v.st.anc, v.st.cur = txvPos{}, txvPos{last, utf8.RuneCountInString(v.line(last))}
	v.selChanged = true
}

// selRange returns the ordered selection.
func (v *txView) selRange() (a, b txvPos, ok bool) {
	a, b = v.st.anc, v.st.cur
	if b.less(a) {
		a, b = b, a
	}
	return a, b, a != b
}

// selection returns the selected text, lines joined with "\n".
func (v *txView) selection() string {
	a, b, ok := v.selRange()
	if !ok {
		return ""
	}
	var sb strings.Builder
	for i := a.line; i <= b.line && i < v.count; i++ {
		s := v.line(i)
		lo, hi := 0, len(s)
		if i == a.line {
			lo = txvByteOf(s, a.col)
		}
		if i == b.line {
			hi = txvByteOf(s, b.col)
		}
		if i > a.line {
			sb.WriteByte('\n')
		}
		if lo < hi {
			sb.WriteString(s[lo:hi])
		}
		if sb.Len() > 64<<20 {
			break // copying more than 64 MB is not useful
		}
	}
	return sb.String()
}

// txvByteOf returns the byte offset of rune index col in s.
func txvByteOf(s string, col int) int {
	if col <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == col {
			return i
		}
		n++
	}
	return len(s)
}

// txvWord returns the rune range of the word around rune col.
func txvWord(s string, col int) (int, int) {
	rs := []rune(s)
	col = max(0, min(col, len(rs)))
	word := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	if col == len(rs) || !word(rs[col]) {
		if col > 0 && word(rs[col-1]) {
			col--
		} else {
			return col, min(col+1, len(rs))
		}
	}
	a, b := col, col
	for a > 0 && word(rs[a-1]) {
		a--
	}
	for b < len(rs) && word(rs[b]) {
		b++
	}
	return a, b
}

// revealLine scrolls so line i is visible (centered when far away).
func (v *txView) revealLine(i int) {
	if v.sb == nil || v.count == 0 {
		return
	}
	s := v.sb.s
	top := float64(v.lineRow(i))*float64(v.lineH) + float64(v.padY)
	view := fillViewH(v.sb, 200)
	if top < s.off.Y || top+float64(v.lineH) > s.off.Y+view {
		s.off.Y = max(0, top-view/2)
		v.c.invalid = true
	}
}

// revealCol scrolls horizontally so bytes [b0, b1) of line i are visible.
func (v *txView) revealCol(i, b0 int) {
	if v.wrap || v.sb == nil || i >= v.count {
		return
	}
	s := v.sb.s
	x := float64(float32(txvCells(v.line(i)[:b0])) * v.cellW())
	w := float64(v.st.view.W - v.st.gutterW - 2*v.padX)
	if w <= 0 {
		return
	}
	if x < s.off.X || x > s.off.X+w-float64(8*v.cellW()) {
		s.off.X = max(0, x-w/3)
		v.c.invalid = true
	}
}

// atBottom reports whether the viewport shows the last row (or nothing
// was painted yet).
func (v *txView) atBottom() bool {
	s := v.sb.s
	if s.view.H <= 0 {
		return true
	}
	return s.off.Y >= s.maxY()-float64(v.lineH)/2
}

// paint draws the visible rows; r is the text node (it starts at the
// first built row and spans the content width).
func (v *txView) paint(l *paint.List, r geom.Rect) {
	st := v.st
	s := v.sb.s
	vx := r.X + float32(s.painted.X)
	vy := r.Y + float32(s.painted.Y-s.base.Y)
	st.view = geom.Rect{X: vx, Y: vy, W: s.view.W, H: s.view.H}
	st.originY = r.Y - float32(s.base.Y)
	st.gutterW = v.gutterWidth()
	st.textX = r.X + st.gutterW + v.padX
	st.painted = true
	mono := v.mono()
	selA, selB, hasSel := v.selRange()
	textClip := geom.Rect{X: vx + st.gutterW, Y: vy, W: max(0, s.view.W-st.gutterW), H: s.view.H}
	l.Clip(textClip, geom.Radii{})
	var runs []txvRun
	var marks []txvMark
	var trs []text.Run
	var attrs []txvRun
	for row := v.first; row < v.end; row++ {
		line, seg := v.rowLine(row)
		y := st.originY + v.padY + float32(row)*v.lineH
		if y > vy+s.view.H || y+v.lineH < vy {
			continue
		}
		str := v.line(line)
		b0, b1, r0 := v.segment(str, seg)
		segStr := str[b0:b1]
		if v.lineBg != nil {
			if bg := v.lineBg(line); bg.A > 0 {
				l.Fill(geom.Rect{X: textClip.X, Y: y, W: textClip.W, H: v.lineH}, geom.Radii{}, bg)
			}
		}
		if v.runs != nil {
			runs = v.runs(line, runs[:0])
		} else {
			runs = runs[:0]
		}
		trs, attrs = v.styleRuns(segStr, b0, runs, mono, trs[:0], attrs[:0])
		lay := st.cache.Shape(segStr, trs, text.LineOptions{TabSize: txvTab})
		x := st.textX
		caret := func(rr int) float32 { cx, _, _ := lay.Caret(max(0, min(rr, len(lay.Runes)))); return x + cx }
		conv := txvRuneConv{s: segStr}
		// Backgrounds of styled runs.
		for k, a := range attrs {
			if a.bg.A > 0 {
				x0, x1 := caret(trs[k].Start), caret(trs[k].End)
				l.Fill(geom.Rect{X: x0, Y: y, W: x1 - x0, H: v.lineH}, geom.Radii{}, a.bg)
			}
		}
		// Search matches.
		if v.marks != nil {
			marks = v.marks(line, marks[:0])
			for _, m := range marks {
				ms, me := max(m.start, b0), min(m.end, b1)
				if ms >= me && !(m.start == m.end) {
					continue
				}
				x0, x1 := caret(conv.at(ms-b0)), caret(conv.at(me-b0))
				mr := geom.Rect{X: x0, Y: y + 1, W: max(x1-x0, 2), H: v.lineH - 2}
				if m.current {
					l.Fill(mr, geom.Rad(2), v.matchC)
					if v.matchB.A > 0 {
						l.Stroke(mr, geom.Rad(2), 1, v.matchB)
					}
				} else {
					l.Fill(mr, geom.Rad(2), v.match)
				}
			}
		}
		// Selection.
		if hasSel && line >= selA.line && line <= selB.line {
			n := utf8.RuneCountInString(segStr)
			lo, hi := 0, n
			if line == selA.line {
				lo = max(0, selA.col-r0)
			}
			if line == selB.line {
				hi = min(n, selB.col-r0)
			}
			eol := line < selB.line && (b1 == len(str) || !v.wrap)
			if lo < hi || (eol && lo <= n) {
				x0, x1 := caret(lo), caret(max(lo, hi))
				if eol && hi >= n {
					x1 += v.cellW() / 2 // the line break
				}
				if x1 > x0 {
					l.Fill(geom.Rect{X: x0, Y: y, W: x1 - x0, H: v.lineH}, geom.Radii{}, v.selBg)
				}
			}
		}
		// Glyphs, by style run.
		gl := lay.Glyphs
		origin := geom.Pt(x, y)
		for i := 0; i < len(gl); {
			run := int32(0)
			if i < len(lay.GlyphRun) {
				run = lay.GlyphRun[i]
			}
			j := i + 1
			for j < len(gl) && j < len(lay.GlyphRun) && lay.GlyphRun[j] == run {
				j++
			}
			col := v.fg
			if int(run) < len(attrs) {
				a := attrs[run]
				if a.fg.A > 0 {
					col = a.fg
				}
				if a.faint {
					col = theme.Fade(col, 0.6)
				}
			}
			l.Glyphs(origin, gl[i:j], col)
			i = j
		}
		// Decorations.
		for k, a := range attrs {
			if !a.underline && !a.strike {
				continue
			}
			x0, x1 := caret(trs[k].Start), caret(trs[k].End)
			col := v.fg
			if a.fg.A > 0 {
				col = a.fg
			}
			if a.underline {
				uy := y + lay.Ascent + (v.lineH-lay.LineHeight)/2 + 2
				l.Fill(geom.Rect{X: x0, Y: uy, W: x1 - x0, H: 1}, geom.Radii{}, col)
			}
			if a.strike {
				l.Fill(geom.Rect{X: x0, Y: y + v.lineH/2, W: x1 - x0, H: 1}, geom.Radii{}, col)
			}
		}
		if !v.wrap && b1 < len(str) { // capped line
			l.Glyphs(geom.Pt(caret(len(lay.Runes))+2, y), text.Shape("…", mono, text.Options{NoWrap: true}).Glyphs, v.gutter)
		}
	}
	l.Pop()
	// Gutter: line numbers stay put while the text scrolls sideways.
	if v.numbers != nil {
		if v.gutterB.A > 0 {
			l.Fill(geom.Rect{X: vx + st.gutterW - 1, Y: vy, W: 1, H: s.view.H}, geom.Radii{}, v.gutterB)
		}
		l.Clip(geom.Rect{X: vx, Y: vy, W: st.gutterW, H: s.view.H}, geom.Radii{})
		for row := v.first; row < v.end; row++ {
			line, seg := v.rowLine(row)
			if seg > 0 {
				continue
			}
			y := st.originY + v.padY + float32(row)*v.lineH
			num := itoaEnt(v.numbers(line))
			lay := text.Shape(num, mono, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(vx+st.gutterW-v.padX-lay.Width, y+(v.lineH-lay.Height)/2), lay.Glyphs, v.gutter)
		}
		l.Pop()
	}
}

// styleRuns converts the byte runs of a line to text runs over the
// segment starting at byte b0 (rune ranges of seg), filling gaps with
// the default style; attrs[k] holds the paint attributes of trs[k].
func (v *txView) styleRuns(seg string, b0 int, runs []txvRun, mono text.Style, trs []text.Run, attrs []txvRun) ([]text.Run, []txvRun) {
	conv := txvRuneConv{s: seg}
	n := conv.at(len(seg))
	pos := 0 // rune
	add := func(rs, re int, a txvRun) {
		if re <= rs {
			return
		}
		st := mono
		if a.bold {
			st.Weight = text.Bold
		}
		st.Italic = a.italic
		trs = append(trs, text.Run{Start: rs, End: re, Style: st})
		attrs = append(attrs, a)
	}
	for _, r := range runs {
		s, e := r.start-b0, r.end-b0
		if e <= 0 || s >= len(seg) {
			continue
		}
		s, e = max(s, 0), min(e, len(seg))
		rs, re := conv.at(s), conv.at(e)
		if rs < pos {
			rs = pos
		}
		add(pos, rs, txvRun{})
		add(rs, re, r)
		pos = max(pos, re)
	}
	add(pos, n, txvRun{})
	if len(trs) == 0 {
		trs = append(trs, text.Run{Start: 0, End: 0, Style: mono})
		attrs = append(attrs, txvRun{})
	}
	return trs, attrs
}

// txvRuneConv converts byte offsets of s to rune indexes, cheaply for
// increasing offsets.
type txvRuneConv struct {
	s    string
	b, r int
}

func (c *txvRuneConv) at(b int) int {
	b = max(0, min(b, len(c.s)))
	if b < c.b {
		c.b, c.r = 0, 0
	}
	c.r += utf8.RuneCountInString(c.s[c.b:b])
	c.b = b
	return c.r
}
