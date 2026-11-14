package vt

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Options configures a [Terminal].
type Options struct {
	Cols, Rows int // size in cells; raised to at least 1
	// Scrollback is the number of lines kept after they scroll off the
	// top. 0 means the default, 10 000; a negative value keeps none.
	Scrollback int
	// Reply receives the bytes the terminal sends back to the program,
	// such as cursor position and device attribute reports. It is called
	// during Write and may be nil. b is only valid during the call.
	Reply func(b []byte)
}

// DefaultScrollback is the scrollback length used when
// [Options.Scrollback] is 0.
const DefaultScrollback = 10000

// maxTitle bounds the window title, in bytes.
const maxTitle = 4096

// Modes are the terminal modes a view or input encoder needs to know.
type Modes struct {
	AppCursor      bool // DECCKM (?1): cursor keys send application sequences
	BracketedPaste bool // ?2004: wrap pasted text in ESC [200~ … ESC [201~
	ShowCursor     bool // DECTCEM (?25): the cursor is visible
	AutoWrap       bool // DECAWM (?7): printing past the last column wraps
}

func defaultModes() Modes { return Modes{ShowCursor: true, AutoWrap: true} }

// savedCursor is the state ESC 7 saves and ESC 8 restores.
type savedCursor struct {
	x, y    int
	pending bool
	pen     Style
	gl      uint8
}

// Terminal is a terminal emulator: it parses the bytes a program writes
// and keeps the resulting screen, cursor and modes.
//
// A Terminal is not safe for concurrent use; the caller serializes Write
// with reads of its state.
type Terminal struct {
	p     *Parser
	s     *Screen
	reply func([]byte)

	cols, rows int
	cx, cy     int
	// pending is DECAWM's "wrap on next character" state: the last
	// column was just written and cx stays on it until the next
	// printable character wraps.
	pending bool

	// pen is the style new characters get. penIdx caches its index in
	// the style table while penOK is set and the table's generation is
	// penGen.
	pen    Style
	penIdx uint16
	penGen uint32
	penOK  bool

	saved savedCursor
	modes Modes
	title string
	gl    uint8 // 0 after SI (G0), 1 after SO (G1); mapping is M2

	// Rows of the viewport changed since TakeDirty; see TakeDirty and
	// TakeScroll.
	dirty      []bool
	allDirty   bool
	scrolled   int
	lastOffset int // the viewport offset dirty is relative to

	buf []byte // scratch for replies
	out []int  // reused result of TakeDirty
}

// New returns a terminal with a blank screen and the cursor at the top
// left.
func New(o Options) *Terminal {
	sb := o.Scrollback
	if sb == 0 {
		sb = DefaultScrollback
	}
	t := &Terminal{reply: o.Reply, modes: defaultModes()}
	t.s = NewScreen(o.Cols, o.Rows, max(sb, 0))
	t.cols, t.rows = t.s.Size()
	t.dirty = make([]bool, t.rows)
	t.allDirty = true
	t.p = NewParser((*handler)(t))
	return t
}

// Write feeds program output to the terminal. Input may be split across
// calls at any byte, including inside a UTF-8 sequence.
func (t *Terminal) Write(b []byte) { t.p.Write(b) }

// Screen returns the terminal's screen. The pointer stays valid for the
// terminal's lifetime.
func (t *Terminal) Screen() *Screen { return t.s }

// Cursor returns the cursor's active-area position and whether it is
// shown (DECTCEM). After the last column is written x stays on it until
// the next character wraps.
func (t *Terminal) Cursor() (x, y int, visible bool) {
	return t.cx, t.cy, t.modes.ShowCursor
}

// Title returns the window title last set with OSC 0 or OSC 2.
func (t *Terminal) Title() string { return t.title }

// Modes returns the current modes.
func (t *Terminal) Modes() Modes { return t.modes }

// Resize changes the terminal to cols×rows cells (each at least 1). Lines
// are truncated or padded, not reflowed. Losing rows keeps the cursor's
// line on screen by moving lines into the scrollback; gaining rows brings
// them back while the cursor is on the last row. The cursor and the saved
// cursor are clamped to the new size.
func (t *Terminal) Resize(cols, rows int) {
	if max(cols, 1) == t.cols && max(rows, 1) == t.rows {
		return // keep the pending wrap and skip the redraw, as xterm.js
	}
	t.cy = t.s.resize(cols, rows, t.cy)
	t.cols, t.rows = t.s.Size()
	t.cx = min(t.cx, t.cols-1)
	t.pending = false
	t.saved.x = min(t.saved.x, t.cols-1)
	t.saved.y = min(t.saved.y, t.rows-1)
	t.saved.pending = false
	if len(t.dirty) != t.rows {
		t.dirty = make([]bool, t.rows)
	}
	t.markAll()
}

// TakeDirty returns the viewport rows that changed since the last call,
// in ascending order, and clears the record. all is true, and rows nil,
// when the whole viewport must be redrawn: after a resize, ED 2, RIS, a
// change of the viewport offset or more scrolling than the viewport has
// rows. The slice is reused by the next call.
//
// Rows are relative to the viewport after the scroll TakeScroll reports:
// a view shifts its old image up by TakeScroll() rows, then repaints rows.
func (t *Terminal) TakeDirty() (rows []int, all bool) {
	if t.allDirty || t.s.ViewOffset() != t.lastOffset {
		t.allDirty = false
		t.scrolled = 0
		t.lastOffset = t.s.ViewOffset()
		clear(t.dirty)
		return nil, true
	}
	t.out = t.out[:0]
	for y, d := range t.dirty {
		if d {
			t.out = append(t.out, y)
		}
	}
	clear(t.dirty)
	return t.out, false
}

// TakeScroll returns how many lines the viewport content moved up since
// the last call and clears the count. It is 0 while a full redraw is
// pending (see TakeDirty), so a view calls it before TakeDirty.
func (t *Terminal) TakeScroll() int {
	if t.allDirty || t.s.ViewOffset() != t.lastOffset {
		return 0
	}
	n := t.scrolled
	t.scrolled = 0
	return n
}

// markRow records that active row y changed.
func (t *Terminal) markRow(y int) {
	if vy := y + t.s.offset; vy < t.rows {
		t.dirty[vy] = true
	}
}

// markRows records that active rows y0 through y1 (inclusive) changed.
func (t *Terminal) markRows(y0, y1 int) {
	for y := max(y0, 0); y <= y1; y++ {
		t.markRow(y)
	}
}

func (t *Terminal) markAll() { t.allDirty = true }

// penIndex returns the pen's style index, interning it if the cached one
// may be stale: after the pen changed, after the table renumbered and
// after the table was full and handed out the default instead.
func (t *Terminal) penIndex() uint16 {
	tb := &t.s.styles
	if t.penOK && t.penGen == tb.gen {
		return t.penIdx
	}
	i := tb.Intern(t.pen)
	t.penIdx, t.penGen = i, tb.gen
	t.penOK = i != 0 || t.pen == Style{}
	return i
}

// setPen changes the pen.
func (t *Terminal) setPen(st Style) {
	if st != t.pen {
		t.pen = st
		t.penOK = false
	}
}

// eraseCell is the cell erased and inserted areas are filled with: a
// blank in the pen's background color, as xterm does.
func (t *Terminal) eraseCell() Cell {
	if t.pen.Bg == ColorDefault {
		return BlankCell
	}
	return Cell{Style: t.s.styles.Intern(Style{Bg: t.pen.Bg}), Width: 1}
}

// fillErase fills cells with the erase cell.
func (t *Terminal) fillErase(cells []Cell) {
	if e := t.eraseCell(); e == BlankCell {
		FillBlank(cells)
	} else {
		for i := range cells {
			cells[i] = e
		}
	}
}

// eraseRange erases columns x0 through x1-1 of active row y, blanking
// any wide character cut in half at either end.
func (t *Terminal) eraseRange(y, x0, x1 int) {
	x0, x1 = max(x0, 0), min(x1, t.cols)
	if x0 >= x1 {
		return
	}
	l := t.s.Line(y)
	t.splitWide(l, x0, x1)
	l.clearCombining(x0, x1)
	t.fillErase(l.Cells[x0:x1])
	t.markRow(y)
}

// splitWide prepares cells x0 through x1-1 of l to be overwritten: a wide
// character with only one half inside the range becomes a blank so no
// half-character is left outside it.
func (t *Terminal) splitWide(l *Line, x0, x1 int) {
	c := l.Cells
	if x0 > 0 && x0 < len(c) && c[x0].Width == 0 {
		c[x0-1] = Cell{Style: c[x0-1].Style, Width: 1}
		l.clearCombining(x0-1, x0)
	}
	if x1 > 0 && x1 < len(c) && c[x1].Width == 0 {
		c[x1] = Cell{Style: c[x1].Style, Width: 1}
	}
}

// fixWideAt blanks a broken wide character half at column x of l after
// cells were shifted: a right half without its left, or a left half
// without its right.
func fixWideAt(l *Line, x int) {
	c := l.Cells
	if x < 0 || x >= len(c) {
		return
	}
	switch c[x].Width {
	case 0:
		if x == 0 || c[x-1].Width != 2 {
			c[x] = Cell{Style: c[x].Style, Width: 1}
		}
	case 2:
		if x+1 >= len(c) || c[x+1].Width != 0 {
			c[x] = Cell{Style: c[x].Style, Width: 1}
			l.clearCombining(x, x+1)
		}
	}
}

// index moves the cursor down a line, scrolling the screen up at the
// bottom row (LF, IND).
func (t *Terminal) index() {
	if t.cy < t.rows-1 {
		t.cy++
		return
	}
	t.scrollUp()
}

// scrollUp scrolls the whole screen up a line into the scrollback.
func (t *Terminal) scrollUp() {
	s := t.s
	o := s.offset
	s.ScrollUp(0, t.rows-1)
	if e := t.eraseCell(); e != BlankCell {
		cells := s.Line(t.rows - 1).Cells
		for i := range cells {
			cells[i] = e
		}
	}
	switch {
	case o == 0:
		// The viewport shows the active area: its content moved up.
		if t.scrolled++; t.scrolled >= t.rows {
			t.markAll()
		} else {
			copy(t.dirty, t.dirty[1:])
			t.dirty[t.rows-1] = true
		}
	case s.offset == o+1:
		// The viewport kept its lines; the new row is below it.
	default:
		t.markAll() // the viewport's top line was evicted
	}
	if t.lastOffset == o {
		t.lastOffset = s.offset
	}
}

// wrap performs the pending wrap: it marks the current line as continued
// and moves to the start of the next.
func (t *Terminal) wrap() {
	t.s.Line(t.cy).Wrapped = true
	t.pending = false
	t.cx = 0
	t.index()
}

// startPrint resolves a pending wrap before a character of width w is
// printed and reports whether it can be printed.
func (t *Terminal) startPrint(w int) bool {
	if t.pending {
		if t.modes.AutoWrap {
			t.wrap()
		} else {
			t.pending = false
		}
	}
	if w == 2 && t.cx == t.cols-1 {
		if !t.modes.AutoWrap || t.cols < 2 {
			return false // as xterm.js: dropped
		}
		// It does not fit: blank the last column and wrap.
		l := t.s.Line(t.cy)
		t.splitWide(l, t.cx, t.cx+1)
		l.clearCombining(t.cx, t.cx+1)
		l.Cells[t.cx] = Cell{Style: t.penIndex(), Width: 1}
		t.markRow(t.cy)
		t.wrap()
	}
	return true
}

// printASCII writes a run of printable ASCII at the cursor.
func (t *Terminal) printASCII(b []byte) {
	for len(b) > 0 {
		t.startPrint(1)
		if !t.modes.AutoWrap && t.cx == t.cols-1 && len(b) > 1 {
			// Without autowrap only the last byte survives in the last
			// column.
			b = b[len(b)-1:]
		}
		st := t.penIndex()
		l := t.s.Line(t.cy)
		x := t.cx
		n := min(len(b), t.cols-x)
		t.splitWide(l, x, x+n)
		if len(l.comb) > 0 {
			l.clearCombining(x, x+n)
		}
		cells := l.Cells[x : x+n]
		for i, c := range b[:n] {
			cells[i] = Cell{Rune: rune(c), Style: st, Width: 1}
		}
		if vy := t.cy + t.s.offset; vy < t.rows {
			t.dirty[vy] = true
		}
		b = b[n:]
		if x+n >= t.cols {
			t.cx = t.cols - 1
			t.pending = true
		} else {
			t.cx = x + n
		}
	}
}

// print writes one non-ASCII code point at the cursor.
func (t *Terminal) print(r rune) {
	if r >= 0x80 && r <= 0x9F {
		return // C1 controls are not printable
	}
	w := Width(r)
	if w == 0 {
		t.combine(r)
		return
	}
	if !t.startPrint(w) {
		return
	}
	st := t.penIndex()
	l := t.s.Line(t.cy)
	x := t.cx
	t.splitWide(l, x, x+w)
	l.clearCombining(x, x+w)
	l.Cells[x] = Cell{Rune: r, Style: st, Width: uint8(w)}
	if w == 2 {
		l.Cells[x+1] = Cell{Style: st}
	}
	t.markRow(t.cy)
	if x+w >= t.cols {
		t.cx = t.cols - 1
		t.pending = true
	} else {
		t.cx = x + w
	}
}

// combine attaches a zero-width code point to the character before the
// cursor. With no character before it on the line, it is dropped.
func (t *Terminal) combine(r rune) {
	x := t.cx - 1
	if t.pending {
		x = t.cx
	}
	l := t.s.Line(t.cy)
	if x >= 0 && l.Cells[x].Width == 0 {
		x--
	}
	if x < 0 {
		return
	}
	l.addCombining(x, r)
	t.markRow(t.cy)
}

// sendReply sends b to the program.
func (t *Terminal) sendReply(b []byte) {
	if t.reply != nil {
		t.reply(b)
	}
}

// execute handles a C0 control.
func (t *Terminal) execute(c byte) {
	switch c {
	case 0x08: // BS
		t.pending = false
		if t.cx > 0 {
			t.cx--
		}
	case 0x09: // HT: next multiple of 8, stopping at the last column
		t.pending = false
		t.cx = min((t.cx/8+1)*8, t.cols-1)
	case 0x0A, 0x0B, 0x0C: // LF, VT, FF
		t.pending = false
		t.index()
	case 0x0D: // CR
		t.pending = false
		t.cx = 0
	case 0x0E: // SO
		t.gl = 1
	case 0x0F: // SI
		t.gl = 0
	}
	// BEL and the rest are ignored.
}

// esc handles an escape sequence.
func (t *Terminal) esc(inter []byte, final byte) {
	if len(inter) > 0 {
		return // charset designations and the rest are M2
	}
	switch final {
	case '7': // DECSC
		t.saved = savedCursor{x: t.cx, y: t.cy, pending: t.pending, pen: t.pen, gl: t.gl}
	case '8': // DECRC
		sv := t.saved
		t.cx, t.cy = min(sv.x, t.cols-1), min(sv.y, t.rows-1)
		t.pending = sv.pending && sv.x == t.cols-1
		t.setPen(sv.pen)
		t.gl = sv.gl
	case 'D': // IND
		t.pending = false
		t.index()
	case 'E': // NEL
		t.pending = false
		t.cx = 0
		t.index()
	case 'M': // RI
		t.pending = false
		if t.cy > 0 {
			t.cy--
			return
		}
		t.scrollDown(0, 1)
	case 'c': // RIS
		t.resetAll()
	}
}

// resetAll is RIS: everything but the title and size back to the state
// New leaves.
func (t *Terminal) resetAll() {
	t.s.reset()
	t.cx, t.cy, t.pending = 0, 0, false
	t.pen, t.penOK = Style{}, false
	t.saved = savedCursor{}
	t.modes = defaultModes()
	t.gl = 0
	t.markAll()
}

// osc handles an operating system command.
func (t *Terminal) osc(data []byte) {
	// Look at the command before converting: ignored payloads can be
	// up to 1 MB.
	cmd, rest, ok := bytes.Cut(data, []byte(";"))
	if !ok {
		return
	}
	switch string(cmd) {
	case "0", "2":
		arg := string(rest[:min(len(rest), maxTitle+utf8.UTFMax)])
		if len(arg) > maxTitle {
			// Cut before the character straddling the limit.
			i := maxTitle
			for i > maxTitle-utf8.UTFMax && !utf8.RuneStart(arg[i]) {
				i--
			}
			arg = arg[:i]
		}
		t.title = strings.ToValidUTF8(arg, "�")
	}
}

// handler is the [Handler] the terminal's parser reports to. It is a
// separate type so the Handler methods stay out of Terminal's API.
type handler Terminal

func (h *handler) Print(r rune)        { (*Terminal)(h).print(r) }
func (h *handler) PrintASCII(b []byte) { (*Terminal)(h).printASCII(b) }
func (h *handler) Execute(b byte)      { (*Terminal)(h).execute(b) }
func (h *handler) ESC(inter []byte, final byte) {
	(*Terminal)(h).esc(inter, final)
}
func (h *handler) OSC(data []byte) { (*Terminal)(h).osc(data) }
func (h *handler) CSI(p *Params, inter []byte, final, private byte) {
	(*Terminal)(h).csi(p, inter, final, private)
}
func (h *handler) DCS(*Params, []byte, byte, []byte) {}
