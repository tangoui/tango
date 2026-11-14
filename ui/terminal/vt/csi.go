package vt

import "strconv"

// da1Reply answers primary device attributes like xterm.js: a VT100 with
// advanced video.
const da1Reply = "\x1b[?1;2c"

// csi handles a control sequence. Sequences M1 does not implement are
// ignored.
func (t *Terminal) csi(p *Params, inter []byte, final, private byte) {
	if len(inter) > 0 {
		return
	}
	switch private {
	case 0:
	case '?':
		switch final {
		case 'h':
			t.setModes(p, true)
		case 'l':
			t.setModes(p, false)
		case 'J': // DECSED: no protected cells yet, so as ED
			t.eraseDisplay(p.Get(0, 0))
		case 'K': // DECSEL
			t.eraseLine(p.Get(0, 0))
		}
		return
	default:
		return
	}

	// The count most sequences take, clamped so that pos+n cannot
	// overflow a 32-bit int; no count does more than the screen size.
	n := min(p.Get(0, 1), max(t.cols, t.rows))
	switch final {
	case 'A': // CUU
		t.moveTo(t.cx, t.cy-n)
	case 'B', 'e': // CUD, VPR
		t.moveTo(t.cx, t.cy+n)
	case 'C', 'a': // CUF, HPR
		t.moveTo(t.cx+n, t.cy)
	case 'D': // CUB
		t.moveTo(t.cx-n, t.cy)
	case 'E': // CNL
		t.moveTo(0, t.cy+n)
	case 'F': // CPL
		t.moveTo(0, t.cy-n)
	case 'G', '`': // CHA, HPA
		t.moveTo(n-1, t.cy)
	case 'H', 'f': // CUP, HVP
		t.moveTo(p.Get(1, 1)-1, n-1)
	case 'd': // VPA
		t.moveTo(t.cx, n-1)
	case 'J': // ED
		t.eraseDisplay(p.Get(0, 0))
	case 'K': // EL
		t.eraseLine(p.Get(0, 0))
	case 'X': // ECH
		t.pending = false
		t.eraseRange(t.cy, t.cx, t.cx+min(n, t.cols))
	case 'P': // DCH
		t.deleteChars(n)
	case '@': // ICH
		t.insertChars(n)
	case 'L': // IL
		t.insertLines(n)
	case 'M': // DL
		t.deleteLines(n)
	case 'S': // SU
		t.scrollUpRegion(n)
	case 'T': // SD
		t.scrollDown(0, n)
	case 'm':
		t.sgr(p)
	case 'n': // DSR
		switch p.Get(0, 0) {
		case 5:
			t.sendReply([]byte("\x1b[0n"))
		case 6: // CPR
			b := append(t.buf[:0], "\x1b["...)
			b = strconv.AppendInt(b, int64(t.cy+1), 10)
			b = append(b, ';')
			b = strconv.AppendInt(b, int64(t.cx+1), 10)
			b = append(b, 'R')
			t.buf = b
			t.sendReply(b)
		}
	case 'c': // DA1
		if p.Get(0, 0) == 0 {
			t.sendReply([]byte(da1Reply))
		}
	}
	// SM/RM ('h'/'l') without '?' set ANSI modes M1 does not have.
}

// moveTo puts the cursor at (x, y), clamped to the screen, and cancels a
// pending wrap.
func (t *Terminal) moveTo(x, y int) {
	t.cx = min(max(x, 0), t.cols-1)
	t.cy = min(max(y, 0), t.rows-1)
	t.pending = false
}

// setModes handles DECSET (on) and DECRST.
func (t *Terminal) setModes(p *Params, on bool) {
	for i := range p.Len() {
		switch p.Sub(i)[0] {
		case 1:
			t.modes.AppCursor = on
		case 7:
			t.modes.AutoWrap = on
		case 25:
			t.modes.ShowCursor = on
		case 2004:
			t.modes.BracketedPaste = on
		}
	}
}

// editX is the column ED and EL erase from: as in xterm.js, a pending
// wrap counts as being past the last column.
func (t *Terminal) editX() int {
	if t.pending {
		return t.cols
	}
	return t.cx
}

// unwrapAbove records that active row y no longer continues the line
// above it, which may be in the scrollback.
func (t *Terminal) unwrapAbove(y int) {
	if i := t.s.sbLen + y - 1; i >= 0 {
		t.s.at(i).Wrapped = false
	}
}

// clearRow erases all of active row y.
func (t *Terminal) clearRow(y int) {
	l := t.s.Line(y)
	l.clear(t.cols)
	if e := t.eraseCell(); e != BlankCell {
		for i := range l.Cells {
			l.Cells[i] = e
		}
	}
	t.markRow(y)
}

// eraseDisplay is ED: 0 from the cursor to the end, 1 from the start to
// the cursor, 2 everything, 3 the scrollback.
func (t *Terminal) eraseDisplay(mode int) {
	x := t.editX()
	switch mode {
	case 0:
		t.eraseRange(t.cy, x, t.cols)
		if x == 0 {
			t.unwrapAbove(t.cy)
		}
		if t.cy < t.rows-1 {
			t.s.Line(t.cy).Wrapped = false
		}
		for y := t.cy + 1; y < t.rows; y++ {
			t.clearRow(y)
		}
	case 1:
		for y := range t.cy {
			t.clearRow(y)
		}
		t.unwrapAbove(0)
		t.eraseRange(t.cy, 0, x+1)
		if x+1 >= t.cols {
			t.s.Line(t.cy).Wrapped = false
		}
	case 2:
		for y := range t.rows {
			t.clearRow(y)
		}
		t.unwrapAbove(0)
		t.s.styles.resetBackoff()
		t.markAll()
	case 3:
		t.s.ClearScrollback()
		t.markAll()
	}
}

// eraseLine is EL: 0 from the cursor to the end of the line, 1 from its
// start to the cursor, 2 the whole line.
func (t *Terminal) eraseLine(mode int) {
	x := t.editX()
	switch mode {
	case 0:
		t.eraseRange(t.cy, x, t.cols)
		if x == 0 {
			t.unwrapAbove(t.cy)
		}
	case 1:
		t.eraseRange(t.cy, 0, x+1)
	case 2:
		t.eraseRange(t.cy, 0, t.cols)
		t.unwrapAbove(t.cy)
	}
}

// deleteChars is DCH: it removes n characters at the cursor, moving the
// rest of the line left and filling the end with blanks.
func (t *Terminal) deleteChars(n int) {
	t.pending = false
	x := t.cx
	n = min(n, t.cols-x)
	l := t.s.Line(t.cy)
	t.splitWide(l, x, x+n)
	l.clearCombining(x, x+n)
	l.shiftCombining(x+n, -n, t.cols)
	copy(l.Cells[x:], l.Cells[x+n:])
	t.fillErase(l.Cells[t.cols-n:])
	t.markRow(t.cy)
}

// insertChars is ICH: it inserts n blanks at the cursor, moving the rest
// of the line right; characters pushed past the end are lost.
func (t *Terminal) insertChars(n int) {
	t.pending = false
	x := t.cx
	n = min(n, t.cols-x)
	l := t.s.Line(t.cy)
	t.splitWide(l, x, x)
	l.shiftCombining(x, n, t.cols)
	copy(l.Cells[x+n:], l.Cells[x:t.cols-n])
	t.fillErase(l.Cells[x : x+n])
	fixWideAt(l, t.cols-1)
	t.markRow(t.cy)
}

// fillRowsErase fills active rows y0 through y1-1, freshly blanked by a
// scroll, with the erase cell when it is not the default blank.
func (t *Terminal) fillRowsErase(y0, y1 int) {
	e := t.eraseCell()
	if e == BlankCell {
		return
	}
	for y := y0; y < y1; y++ {
		cells := t.s.Line(y).Cells
		for i := range cells {
			cells[i] = e
		}
	}
}

// insertLines is IL: n blank lines at the cursor row, pushing the rows
// below down and off the bottom. The cursor goes to column 0.
func (t *Terminal) insertLines(n int) {
	n = min(n, t.rows-t.cy)
	t.s.shiftRows(t.cy, t.rows-1, -n)
	t.fillRowsErase(t.cy, t.cy+n)
	t.cx, t.pending = 0, false
	t.markRows(t.cy, t.rows-1)
}

// deleteLines is DL: it removes n lines at the cursor row, pulling the
// rows below up and adding blank rows at the bottom. The cursor goes to
// column 0.
func (t *Terminal) deleteLines(n int) {
	n = min(n, t.rows-t.cy)
	t.s.shiftRows(t.cy, t.rows-1, n)
	t.fillRowsErase(t.rows-n, t.rows)
	t.cx, t.pending = 0, false
	t.markRows(t.cy, t.rows-1)
}

// scrollUpRegion is SU: the screen scrolls up n lines. As in xterm.js,
// the lines scrolled off are discarded, not kept in the scrollback.
func (t *Terminal) scrollUpRegion(n int) {
	n = min(n, t.rows)
	t.s.shiftRows(0, t.rows-1, n)
	t.fillRowsErase(t.rows-n, t.rows)
	if t.s.offset > 0 {
		t.markRows(0, t.rows-1)
		return
	}
	if t.scrolled += n; t.scrolled >= t.rows {
		t.markAll()
		return
	}
	copy(t.dirty, t.dirty[n:])
	for y := t.rows - n; y < t.rows; y++ {
		t.dirty[y] = true
	}
}

// scrollDown scrolls rows top to the bottom down n lines (SD, and RI at
// the top row), adding blank rows at top.
func (t *Terminal) scrollDown(top, n int) {
	n = min(n, t.rows-top)
	t.s.shiftRows(top, t.rows-1, -n)
	t.fillRowsErase(top, top+n)
	t.markRows(top, t.rows-1)
}

// sgr is SGR: it sets the pen's colors and attributes.
func (t *Terminal) sgr(p *Params) {
	st := t.pen
	if p.Len() == 0 {
		st = Style{}
	}
	for i := 0; i < p.Len(); i++ {
		sub := p.Sub(i)
		switch v := sub[0]; v {
		case 0:
			st = Style{}
		case 1:
			st.Attr |= AttrBold
		case 2:
			st.Attr |= AttrFaint
		case 3:
			st.Attr |= AttrItalic
		case 4:
			if len(sub) > 1 && sub[1] == 0 { // 4:0 is "no underline"
				st.Attr &^= AttrUnderline
			} else {
				st.Attr |= AttrUnderline
			}
		case 5:
			st.Attr |= AttrBlink
		case 7:
			st.Attr |= AttrInverse
		case 8:
			st.Attr |= AttrInvisible
		case 9:
			st.Attr |= AttrStrike
		case 21: // double underline, drawn single in M1
			st.Attr |= AttrUnderline
		case 22:
			st.Attr &^= AttrBold | AttrFaint
		case 23:
			st.Attr &^= AttrItalic
		case 24:
			st.Attr &^= AttrUnderline
		case 25:
			st.Attr &^= AttrBlink
		case 27:
			st.Attr &^= AttrInverse
		case 28:
			st.Attr &^= AttrInvisible
		case 29:
			st.Attr &^= AttrStrike
		case 39:
			st.Fg = ColorDefault
		case 49:
			st.Bg = ColorDefault
		case 38, 48, 58:
			c, used, ok := extendedColor(p, i)
			i += used
			switch {
			case !ok:
			case v == 38:
				st.Fg = c
			case v == 48:
				st.Bg = c
			}
			// 58 (underline color) is parsed so its arguments are not
			// taken as attributes; it is drawn from M3.
		default:
			switch {
			case v >= 30 && v <= 37:
				st.Fg = IndexedColor(uint8(v - 30))
			case v >= 40 && v <= 47:
				st.Bg = IndexedColor(uint8(v - 40))
			case v >= 90 && v <= 97:
				st.Fg = IndexedColor(uint8(v - 90 + 8))
			case v >= 100 && v <= 107:
				st.Bg = IndexedColor(uint8(v - 100 + 8))
			}
		}
	}
	t.setPen(st)
}

// extendedColor parses the color of SGR 38, 48 or 58 at parameter i,
// given either as sub-parameters (38:5:n, 38:2:r:g:b, 38:2:cs:r:g:b) or
// as the following parameters (38;5;n, 38;2;r;g;b). used is how many
// following parameters it consumed; ok is false for a malformed or out
// of range color.
func extendedColor(p *Params, i int) (c Color, used int, ok bool) {
	var buf [4]int
	a := buf[:0] // the kind and its values
	if sub := p.Sub(i); len(sub) > 1 {
		a = append(a, sub[1:min(len(sub), 5)]...)
		if a[0] == 2 && len(sub) >= 6 {
			// 38:2:cs:r:g:b; the color space id is ignored.
			a = append(a[:1], sub[3:6]...)
		}
	} else {
		if i+1 >= p.Len() {
			return 0, 0, false
		}
		var want int
		switch p.Sub(i + 1)[0] {
		case 5:
			want = 2
		case 2:
			want = 4
		default:
			return 0, 1, false
		}
		used = min(want, p.Len()-1-i)
		for j := 1; j <= used; j++ {
			a = append(a, p.Sub(i + j)[0])
		}
	}
	inByte := func(v int) bool { return v >= 0 && v <= 255 }
	switch {
	case a[0] == 5 && len(a) >= 2 && inByte(a[1]):
		return IndexedColor(uint8(a[1])), used, true
	case a[0] == 2 && len(a) == 4 && inByte(a[1]) && inByte(a[2]) && inByte(a[3]):
		return RGBColor(uint8(a[1]), uint8(a[2]), uint8(a[3])), used, true
	}
	return 0, used, false
}
