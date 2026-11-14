package vt

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// newTerm returns a cols×rows terminal whose replies are appended to
// *reply when reply is not nil.
func newTerm(cols, rows int, reply *[]byte) *Terminal {
	o := Options{Cols: cols, Rows: rows, Scrollback: 100}
	if reply != nil {
		o.Reply = func(b []byte) { *reply = append(*reply, b...) }
	}
	return New(o)
}

// screenText returns the active rows with trailing spaces removed.
func screenText(t *Terminal) []string {
	s := t.Screen()
	_, rows := s.Size()
	out := make([]string, rows)
	for y := range rows {
		out[y] = strings.TrimRight(lineText(s.Line(y)), " ")
	}
	return out
}

// trimmedText is screenText without trailing empty rows.
func trimmedText(t *Terminal) []string {
	out := screenText(t)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func cellStyle(t *Terminal, x, y int) Style {
	s := t.Screen()
	return s.Styles().Get(s.Line(y).Cells[x].Style)
}

func write(t *Terminal, s string) { t.Write([]byte(s)) }

func wantText(tb testing.TB, term *Terminal, want ...string) {
	tb.Helper()
	if got := trimmedText(term); !slices.Equal(got, want) {
		tb.Fatalf("screen = %q, want %q", got, want)
	}
}

func wantCursor(tb testing.TB, term *Terminal, x, y int) {
	tb.Helper()
	if gx, gy, _ := term.Cursor(); gx != x || gy != y {
		tb.Fatalf("cursor = (%d,%d), want (%d,%d)", gx, gy, x, y)
	}
}

func TestTerminalPrintWrap(t *testing.T) {
	term := newTerm(5, 3, nil)
	write(term, "abcdefg")
	wantText(t, term, "abcde", "fg")
	if !term.Screen().Line(0).Wrapped {
		t.Error("row 0 not Wrapped")
	}
	if term.Screen().Line(1).Wrapped {
		t.Error("row 1 Wrapped")
	}
	wantCursor(t, term, 2, 1)
}

func TestTerminalPendingWrap(t *testing.T) {
	term := newTerm(5, 3, nil)
	write(term, "abcde")
	wantCursor(t, term, 4, 0)
	write(term, "\r\nx")
	wantText(t, term, "abcde", "x")
	if term.Screen().Line(0).Wrapped {
		t.Error("CR LF set Wrapped")
	}

	// Autowrap off: the last column is overwritten.
	term = newTerm(5, 3, nil)
	write(term, "\x1b[?7labcdefg")
	wantText(t, term, "abcdg")
	wantCursor(t, term, 4, 0)
	write(term, "\x1b[?7h")
	if !term.Modes().AutoWrap {
		t.Error("DECAWM not restored")
	}
}

func TestWideCharAtRightEdge(t *testing.T) {
	term := newTerm(5, 3, nil)
	write(term, "abcd中")
	wantText(t, term, "abcd", "中")
	l0 := term.Screen().Line(0)
	if l0.Cells[4] != BlankCell {
		t.Errorf("cell (4,0) = %+v, want blank", l0.Cells[4])
	}
	if !l0.Wrapped {
		t.Error("row 0 not Wrapped")
	}
	l1 := term.Screen().Line(1)
	if l1.Cells[0].Rune != '中' || l1.Cells[0].Width != 2 || l1.Cells[1].Width != 0 {
		t.Errorf("row 1 cells = %+v", l1.Cells[:2])
	}
	wantCursor(t, term, 2, 1)

	// A wide character filling the last two columns leaves a pending wrap.
	term = newTerm(5, 3, nil)
	write(term, "abc中")
	wantText(t, term, "abc中")
	wantCursor(t, term, 4, 0)
	write(term, "x")
	wantText(t, term, "abc中", "x")
}

func TestWideCharOverwrite(t *testing.T) {
	term := newTerm(6, 2, nil)
	write(term, "中文\x1b[1;2Hx")
	// Overwriting the right half of 中 blanks its left half.
	wantText(t, term, " x文")
	write(term, "\x1b[1;3Hy")
	// Overwriting the left half of 文 blanks its right half.
	wantText(t, term, " xy")
	for x, c := range term.Screen().Line(0).Cells {
		if c.Width != 1 {
			t.Errorf("cell %d width %d, want 1", x, c.Width)
		}
	}
}

func TestTerminalCombining(t *testing.T) {
	term := newTerm(5, 2, nil)
	write(term, "éx")
	l := term.Screen().Line(0)
	if got := l.Combining(0); !slices.Equal(got, []rune{0x301}) {
		t.Errorf("Combining(0) = %U", got)
	}
	if l.Combining(1) != nil {
		t.Errorf("Combining(1) = %U", l.Combining(1))
	}
	wantText(t, term, "ex")
	// On a wide character, the mark goes to its left cell.
	write(term, "\r\n中̂")
	if got := term.Screen().Line(1).Combining(0); !slices.Equal(got, []rune{0x302}) {
		t.Errorf("wide Combining(0) = %U", got)
	}
	// With a pending wrap, the mark joins the last column.
	write(term, "\x1b[1;1Habcdẽ")
	if got := term.Screen().Line(0).Combining(4); !slices.Equal(got, []rune{0x303}) {
		t.Errorf("pending-wrap Combining(4) = %U", got)
	}
	// Overwriting the base drops the mark.
	write(term, "\x1b[1;1Hz")
	if got := term.Screen().Line(0).Combining(0); got != nil {
		t.Errorf("Combining(0) after overwrite = %U", got)
	}
	// A mark at column 0 has no base and is dropped.
	write(term, "\r̄")
	if got := term.Screen().Line(0).Combining(0); got != nil {
		t.Errorf("Combining(0) without base = %U", got)
	}
	// C1 code points are not printed.
	write(term, "\x1b[2J\x1b[H\u0085\u009b")
	wantText(t, term)
	wantCursor(t, term, 0, 0)
}

func TestTerminalCUPAndErase(t *testing.T) {
	term := newTerm(10, 4, nil)
	write(term, "\x1b[2;3Hx")
	if c := term.Screen().Line(1).Cells[2]; c.Rune != 'x' {
		t.Fatalf("cell (2,1) = %+v, want x", c)
	}
	wantCursor(t, term, 3, 1)

	write(term, "\x1b[2J")
	wantText(t, term)
	wantCursor(t, term, 3, 1)

	write(term, "\x1b[1;1Habcdefghij\x1b[1;4H\x1b[K")
	wantText(t, term, "abc")
	write(term, "\x1b[2;1H0123456789\x1b[2;4H\x1b[1K")
	wantText(t, term, "abc", "    456789")
	write(term, "\x1b[2K")
	wantText(t, term, "abc")

	write(term, "\x1b[H\x1b[2J1111\r\n2222\r\n3333\r\n4444\x1b[2;3H\x1b[J")
	wantText(t, term, "1111", "22")
	write(term, "\x1b[H\x1b[2J1111\r\n2222\r\n3333\r\n4444\x1b[3;2H\x1b[1J")
	wantText(t, term, "", "", "  33", "4444")
}

func TestTerminalEditChars(t *testing.T) {
	term := newTerm(8, 2, nil)
	write(term, "abcdefgh\x1b[1;3H\x1b[2P")
	wantText(t, term, "abefgh")
	write(term, "\x1b[1;2H\x1b[3@")
	wantText(t, term, "a   befg")
	write(term, "\x1b[1;1H\x1b[2X")
	wantText(t, term, "    befg")
	wantCursor(t, term, 0, 0)

	// ICH pushing a wide character past the edge blanks it.
	term = newTerm(6, 1, nil)
	write(term, "abcd中\x1b[1;1H\x1b[@")
	wantText(t, term, " abcd")
	// DCH removing a wide character's left half blanks the right half.
	term = newTerm(6, 1, nil)
	write(term, "a中bc\x1b[1;2H\x1b[P")
	wantText(t, term, "a bc")
}

func TestTerminalLines(t *testing.T) {
	term := newTerm(4, 4, nil)
	write(term, "1\r\n2\r\n3\r\n4\x1b[2;3H\x1b[L")
	wantText(t, term, "1", "", "2", "3")
	wantCursor(t, term, 0, 1)
	write(term, "\x1b[2M")
	wantText(t, term, "1", "3")
	write(term, "\x1b[H\x1b[2J1\r\n2\r\n3\r\n4\x1b[2S")
	wantText(t, term, "3", "4")
	if n := term.Screen().ScrollbackLen(); n != 0 {
		t.Errorf("SU put %d lines in the scrollback", n)
	}
	write(term, "\x1b[T")
	wantText(t, term, "", "3", "4")
	// RI at the top scrolls down; elsewhere it moves up.
	write(term, "\x1b[2;1H\x1bMa\x1bMb")
	wantText(t, term, " b", "a", "3", "4")
	// IND and NEL.
	write(term, "\x1b[1;2H\x1bDc\x1bEd")
	wantText(t, term, " b", "ac", "d", "4")
	wantCursor(t, term, 1, 2)
	// Counts beyond the screen clear the affected rows.
	write(term, "\x1b[2;1H\x1b[9999L")
	wantText(t, term, " b")
	write(term, "\x1b[H\x1b[2J1\r\n2\r\n3\x1b[2;1H\x1b[9999M")
	wantText(t, term, "1")
	write(term, "\x1b[H\x1b[2J1\r\n2\x1b[9999T")
	wantText(t, term)
	write(term, "1\r\n2\x1b[9999S")
	wantText(t, term)
}

func TestTerminalCursorMoves(t *testing.T) {
	term := newTerm(10, 5, nil)
	steps := []struct {
		in   string
		x, y int
	}{
		{"\x1b[3;4H", 3, 2},
		{"\x1b[A", 3, 1},
		{"\x1b[9A", 3, 0},
		{"\x1b[2B", 3, 2},
		{"\x1b[99B", 3, 4},
		{"\x1b[3C", 6, 4},
		{"\x1b[99C", 9, 4},
		{"\x1b[4D", 5, 4},
		{"\x1b[2F", 0, 2},
		{"\x1b[3G", 2, 2},
		{"\x1b[E", 0, 3},
		{"\x1b[2d", 0, 1},
		{"\x1b[5;7f", 6, 4},
		{"\x1b[H", 0, 0},
		{"\x1b[0;0H", 0, 0},
		{"\b", 0, 0},
		{"abc\b", 2, 0},
		{"\t", 8, 0},
		{"\t", 9, 0},
		{"\r", 0, 0},
		{"\n\v\f", 0, 3},
		{"\x1b7\x1b[H\x1b8", 0, 3},
	}
	for _, s := range steps {
		write(term, s.in)
		if x, y, _ := term.Cursor(); x != s.x || y != s.y {
			t.Fatalf("after %q: cursor (%d,%d), want (%d,%d)", s.in, x, y, s.x, s.y)
		}
	}
	// BS from a pending wrap goes to the second-to-last column.
	write(term, "\x1b[1;1H0123456789\b")
	wantCursor(t, term, 8, 0)
}

func TestTerminalSaveRestoreStyle(t *testing.T) {
	term := newTerm(10, 3, nil)
	write(term, "\x1b[2;3H\x1b[1;32m\x1b7\x1b[0m\x1b[H\x1b8X")
	if c := term.Screen().Line(1).Cells[2]; c.Rune != 'X' {
		t.Fatalf("cell (2,1) = %+v", c)
	}
	if st := cellStyle(term, 2, 1); st != (Style{Fg: IndexedColor(2), Attr: AttrBold}) {
		t.Errorf("restored style = %+v", st)
	}
}

func TestTerminalSGR(t *testing.T) {
	term := newTerm(10, 2, nil)
	write(term, "\x1b[1;31;48;2;10;20;30mX")
	want := Style{Fg: IndexedColor(1), Bg: RGBColor(10, 20, 30), Attr: AttrBold}
	if st := cellStyle(term, 0, 0); st != want {
		t.Errorf("X style = %+v, want %+v", st, want)
	}
	write(term, "\x1b[38:5:200mY")
	want.Fg = IndexedColor(200)
	if st := cellStyle(term, 1, 0); st != want {
		t.Errorf("Y style = %+v, want %+v", st, want)
	}
	write(term, "\x1b[0mZ")
	if st := cellStyle(term, 2, 0); st != (Style{}) {
		t.Errorf("Z style = %+v, want default", st)
	}

	cases := []struct {
		sgr  string
		want Style
	}{
		{"1;2;3;4;5;7;8;9", Style{Attr: AttrBold | AttrFaint | AttrItalic | AttrUnderline | AttrBlink | AttrInverse | AttrInvisible | AttrStrike}},
		{"1;2;3;4;5;7;8;9;22;23;24;25;27;28;29", Style{}},
		{"21", Style{Attr: AttrUnderline}},
		{"4:3", Style{Attr: AttrUnderline}},
		{"4;4:0", Style{}},
		{"37;47", Style{Fg: IndexedColor(7), Bg: IndexedColor(7)}},
		{"90;107", Style{Fg: IndexedColor(8), Bg: IndexedColor(15)}},
		{"97;100", Style{Fg: IndexedColor(15), Bg: IndexedColor(8)}},
		{"31;41;39;49", Style{}},
		{"38;5;17;48;5;232", Style{Fg: IndexedColor(17), Bg: IndexedColor(232)}},
		{"38:2:1:2:3", Style{Fg: RGBColor(1, 2, 3)}},
		{"38:2::1:2:3", Style{Fg: RGBColor(1, 2, 3)}},
		{"48:2:0:4:5:6", Style{Bg: RGBColor(4, 5, 6)}},
		{"48:5:9", Style{Bg: IndexedColor(9)}},
		{"38;2;300;0;0;1", Style{Attr: AttrBold}},
		{"58;2;1;2;3;1", Style{Attr: AttrBold}},
		{"58;5;3;3", Style{Attr: AttrItalic}},
		{"38;5", Style{}},
		{"", Style{}},
		{"1;;3", Style{Attr: AttrItalic}},
	}
	for _, c := range cases {
		write(term, "\x1b[0m\x1b[H\x1b["+c.sgr+"mQ")
		if st := cellStyle(term, 0, 0); st != c.want {
			t.Errorf("SGR %q: style %+v, want %+v", c.sgr, st, c.want)
		}
	}
}

func TestTerminalEraseUsesBackground(t *testing.T) {
	term := newTerm(4, 2, nil)
	write(term, "abcd\x1b[44m\x1b[1;2H\x1b[K")
	if st := cellStyle(term, 3, 0); st != (Style{Bg: IndexedColor(4)}) {
		t.Errorf("erased style = %+v", st)
	}
	if st := cellStyle(term, 0, 0); st != (Style{}) {
		t.Errorf("kept style = %+v", st)
	}
	// Only the background carries over to erased cells.
	write(term, "\x1b[1;31m\x1b[2;1H\x1b[2K")
	if st := cellStyle(term, 0, 1); st != (Style{Bg: IndexedColor(4)}) {
		t.Errorf("EL 2 style = %+v", st)
	}
	// Lines scrolled in take the background too.
	write(term, "\n")
	if st := cellStyle(term, 2, 1); st != (Style{Bg: IndexedColor(4)}) {
		t.Errorf("scrolled-in style = %+v", st)
	}
}

func TestTerminalScrollIntoScrollback(t *testing.T) {
	term := newTerm(5, 3, nil)
	write(term, "l1\r\nl2\r\nl3\r\nl4\r\nl5")
	wantText(t, term, "l3", "l4", "l5")
	s := term.Screen()
	if n := s.ScrollbackLen(); n != 2 {
		t.Fatalf("ScrollbackLen = %d, want 2", n)
	}
	s.SetViewOffset(2)
	for y, want := range []string{"l1", "l2", "l3"} {
		if got := strings.TrimRight(lineText(s.ViewLine(y)), " "); got != want {
			t.Errorf("view row %d = %q, want %q", y, got, want)
		}
	}
	s.SetViewOffset(0)

	write(term, "\x1b[3J")
	if n := s.ScrollbackLen(); n != 0 {
		t.Errorf("ED 3 left %d scrollback lines", n)
	}
	wantText(t, term, "l3", "l4", "l5")
}

func TestTerminalSplitUTF8(t *testing.T) {
	term := newTerm(5, 2, nil)
	for _, b := range []byte("中") {
		term.Write([]byte{b})
	}
	l := term.Screen().Line(0)
	if l.Cells[0].Rune != '中' || l.Cells[0].Width != 2 || l.Cells[1].Width != 0 {
		t.Fatalf("cells = %+v", l.Cells[:2])
	}
	wantCursor(t, term, 2, 0)
}

func TestTerminalDSR(t *testing.T) {
	var reply []byte
	term := newTerm(10, 5, &reply)
	write(term, "\x1b[2;3H\x1b[6n")
	if string(reply) != "\x1b[2;3R" {
		t.Errorf("CPR = %q", reply)
	}
	reply = nil
	write(term, "\x1b[5n")
	if string(reply) != "\x1b[0n" {
		t.Errorf("DSR 5 = %q", reply)
	}
	reply = nil
	write(term, "\x1b[c\x1b[0c")
	if string(reply) != "\x1b[?1;2c\x1b[?1;2c" {
		t.Errorf("DA1 = %q", reply)
	}
	// A nil Reply is fine.
	write(newTerm(5, 2, nil), "\x1b[6n\x1b[c")
}

func TestTerminalTitle(t *testing.T) {
	term := newTerm(5, 2, nil)
	write(term, "\x1b]2;hi\x07")
	if got := term.Title(); got != "hi" {
		t.Errorf("Title = %q", got)
	}
	write(term, "\x1b]0;both\x1b\\")
	if got := term.Title(); got != "both" {
		t.Errorf("Title = %q", got)
	}
	write(term, "\x1b]1;icon\x07\x1b]7;file:///x\x07")
	if got := term.Title(); got != "both" {
		t.Errorf("OSC 1/7 changed Title to %q", got)
	}
	long := strings.Repeat("中", 2000) // 6000 bytes
	write(term, "\x1b]2;"+long+"\x07")
	if got := term.Title(); len(got) > 4096 || !strings.HasPrefix(long, got) || len(got) < 4093 {
		t.Errorf("long title: %d bytes", len(got))
	}
}

func TestTerminalModes(t *testing.T) {
	term := newTerm(5, 2, nil)
	if m := term.Modes(); m != (Modes{ShowCursor: true, AutoWrap: true}) {
		t.Fatalf("default modes = %+v", m)
	}
	write(term, "\x1b[?1;2004h\x1b[?25l")
	if m := term.Modes(); m != (Modes{AppCursor: true, BracketedPaste: true, AutoWrap: true}) {
		t.Errorf("modes = %+v", m)
	}
	if _, _, vis := term.Cursor(); vis {
		t.Error("cursor visible after DECTCEM reset")
	}
	write(term, "\x1b[?1l\x1b[?2004l\x1b[?25h\x1b[4h")
	if m := term.Modes(); m != (Modes{ShowCursor: true, AutoWrap: true}) {
		t.Errorf("modes = %+v", m)
	}
}

func TestTerminalRIS(t *testing.T) {
	term := newTerm(5, 2, nil)
	write(term, "\x1b]2;t\x07\x1b[31m\x1b[?25la\r\nb\r\nc\x1bc")
	wantText(t, term)
	wantCursor(t, term, 0, 0)
	if term.Screen().ScrollbackLen() != 0 {
		t.Error("RIS kept the scrollback")
	}
	if m := term.Modes(); !m.ShowCursor {
		t.Error("RIS kept DECTCEM reset")
	}
	write(term, "x")
	if st := cellStyle(term, 0, 0); st != (Style{}) {
		t.Errorf("style after RIS = %+v", st)
	}
}

func TestTerminalIgnoresUnknown(t *testing.T) {
	term := newTerm(5, 2, nil)
	write(term, "\x1b[?1049h\x1b[>c\x1b[!p\x1b[5 q\x1b(0\x1bP1$qm\x1b\\\x1b_x\x1b\\\x07\x0e\x0fa\x1b[99z")
	wantText(t, term, "a")
}

func TestTerminalDirty(t *testing.T) {
	term := newTerm(5, 3, nil)
	term.TakeDirty() // the initial state
	term.TakeScroll()
	write(term, "\x1b[2;1Hx")
	rows, all := term.TakeDirty()
	if all || !slices.Contains(rows, 1) {
		t.Errorf("TakeDirty = %v, %v; want row 1", rows, all)
	}
	if rows, all := term.TakeDirty(); all || len(rows) != 0 {
		t.Errorf("second TakeDirty = %v, %v; want none", rows, all)
	}

	write(term, "\x1b[3;1H\n")
	if n := term.TakeScroll(); n != 1 {
		t.Errorf("TakeScroll = %d, want 1", n)
	}
	if n := term.TakeScroll(); n != 0 {
		t.Errorf("second TakeScroll = %d, want 0", n)
	}
	// After the shift only the new bottom row needs painting.
	rows, all = term.TakeDirty()
	if all || !slices.Equal(rows, []int{2}) {
		t.Errorf("TakeDirty after scroll = %v, %v; want [2]", rows, all)
	}

	// A row dirtied before a scroll moves up with it.
	write(term, "\x1b[2;1Hy\x1b[3;1H\n")
	term.TakeScroll()
	rows, _ = term.TakeDirty()
	if !slices.Equal(rows, []int{0, 2}) {
		t.Errorf("TakeDirty = %v, want [0 2]", rows)
	}

	for _, seq := range []string{"\x1b[2J", "\x1bc"} {
		write(term, seq)
		if _, all := term.TakeDirty(); !all {
			t.Errorf("%q: all = false", seq)
		}
	}
	term.Resize(6, 4)
	if _, all := term.TakeDirty(); !all {
		t.Error("Resize: all = false")
	}
	write(term, "1\r\n2\r\n3\r\n4\r\n5")
	term.TakeScroll()
	term.TakeDirty()
	term.Screen().SetViewOffset(1)
	if _, all := term.TakeDirty(); !all {
		t.Error("viewport change: all = false")
	}
	// Output while scrolled back leaves the viewport alone.
	write(term, "\r\n6")
	if n := term.TakeScroll(); n != 0 {
		t.Errorf("TakeScroll while scrolled back = %d", n)
	}
	if rows, all := term.TakeDirty(); all || len(rows) != 0 {
		t.Errorf("TakeDirty while scrolled back = %v, %v", rows, all)
	}
	// A visible active row maps to its viewport row.
	write(term, "\x1b[1;1Hz")
	if rows, _ := term.TakeDirty(); !slices.Equal(rows, []int{2}) {
		t.Errorf("TakeDirty scrolled back by 2 = %v, want [2]", rows)
	}
}

func TestTerminalResizeClampsCursor(t *testing.T) {
	term := newTerm(80, 24, nil)
	write(term, "\x1b[24;80H")
	wantCursor(t, term, 79, 23)
	term.Resize(40, 10)
	wantCursor(t, term, 39, 9)
	if c, r := term.Screen().Size(); c != 40 || r != 10 {
		t.Errorf("Size = %d×%d", c, r)
	}
}

func TestTerminalResize(t *testing.T) {
	term := newTerm(6, 4, nil)
	write(term, "1\r\n2\r\n3\r\nab中")
	// Shrinking keeps the cursor's line: the top rows go to the scrollback.
	term.Resize(4, 2)
	wantText(t, term, "3", "ab中")
	wantCursor(t, term, 3, 1)
	s := term.Screen()
	if n := s.ScrollbackLen(); n != 2 {
		t.Errorf("ScrollbackLen = %d, want 2", n)
	}
	s.SetViewOffset(2)
	if n := len(s.ViewLine(0).Cells); n != 4 {
		t.Errorf("scrollback line width %d, want 4", n)
	}
	s.SetViewOffset(0)
	// A wide character cut by the new edge becomes a blank.
	term.Resize(3, 2)
	wantText(t, term, "3", "ab")
	// Growing pulls them back.
	term.Resize(8, 4)
	wantText(t, term, "1", "2", "3", "ab") // no reflow: the cut 中 stays lost
	if n := s.ScrollbackLen(); n != 0 {
		t.Errorf("ScrollbackLen = %d, want 0", n)
	}
	// Lines below the cursor are dropped first.
	term = newTerm(5, 4, nil)
	write(term, "a\r\nb\r\nc\r\nd\x1b[2;1H")
	term.Resize(5, 2)
	wantText(t, term, "a", "b")
	wantCursor(t, term, 0, 1)
	// Growing with the cursor above the bottom pads.
	term.Resize(5, 3)
	wantText(t, term, "a", "b")
	wantCursor(t, term, 0, 1)
	// The saved cursor is clamped as well.
	write(term, "\x1b[3;5H\x1b7")
	term.Resize(3, 2)
	write(term, "\x1b8")
	if x, y, _ := term.Cursor(); x > 2 || y > 1 {
		t.Errorf("restored cursor (%d,%d) out of range", x, y)
	}
}

func TestTerminalStyleTableFull(t *testing.T) {
	term := newTerm(4, 2, nil)
	tbl := term.Screen().Styles()
	tbl.limit = 4
	// Fill the table, then wipe the screen; the pen must re-intern after
	// the compaction renumbers.
	write(term, "\x1b[31mA\x1b[32mB\x1b[33mC")
	write(term, "\x1b[2J\x1b[H\x1b[34mD")
	if st := cellStyle(term, 0, 0); st != (Style{Fg: IndexedColor(4)}) {
		t.Errorf("after compaction style = %+v", st)
	}
	write(term, "\x1b[31mE")
	if st := cellStyle(term, 1, 0); st != (Style{Fg: IndexedColor(1)}) {
		t.Errorf("E style = %+v", st)
	}
	if st := cellStyle(term, 0, 0); st != (Style{Fg: IndexedColor(4)}) {
		t.Errorf("D style after renumber = %+v", st)
	}
}

// checkInvariants fails if the terminal's state breaks the rules every
// operation must keep.
func checkInvariants(tb testing.TB, term *Terminal) {
	tb.Helper()
	s := term.Screen()
	cols, rows := s.Size()
	x, y, _ := term.Cursor()
	if x < 0 || x >= cols || y < 0 || y >= rows {
		tb.Fatalf("cursor (%d,%d) outside %d×%d", x, y, cols, rows)
	}
	for i := -s.ScrollbackLen(); i < rows; i++ {
		var l *Line
		if i < 0 {
			s.SetViewOffset(-i)
			l = s.ViewLine(0)
			s.SetViewOffset(0)
		} else {
			l = s.Line(i)
		}
		if len(l.Cells) != cols {
			tb.Fatalf("line %d has %d cells, want %d", i, len(l.Cells), cols)
		}
		for j, c := range l.Cells {
			switch c.Width {
			case 0:
				if j == 0 || l.Cells[j-1].Width != 2 {
					tb.Fatalf("line %d: orphan right half at %d", i, j)
				}
			case 2:
				if j+1 >= cols || l.Cells[j+1].Width != 0 {
					tb.Fatalf("line %d: wide char at %d without right half", i, j)
				}
			}
			if int(c.Style) >= s.Styles().Len() {
				tb.Fatalf("line %d: style %d out of range", i, c.Style)
			}
		}
		for _, m := range l.comb {
			if m.x >= cols || l.Cells[m.x].Width == 0 {
				tb.Fatalf("line %d: combining mark at %d", i, m.x)
			}
		}
	}
}

func FuzzTerminal(f *testing.F) {
	for _, s := range []string{
		"abc\x1b[2;3H中文\x1b[K\x1b[2@\x1b[P",
		"é̂\x1b[3L\x1b[2M\x1b[S\x1b[T\x1bM\x1bD\x1bE",
		"\x1b[31;48;2;1;2;3mX\x1b[0m\x1b[J\x1b[1J\x1b[3J\x1bc",
		"\x1b[?7labcdefghij中\x1b[?7h\x1b7\x1b[99;99H\x1b8",
	} {
		f.Add([]byte(s), uint8(5), uint8(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, w, h uint8) {
		term := New(Options{Cols: 6, Rows: 4, Scrollback: 5, Reply: func([]byte) {}})
		term.Screen().Styles().limit = 8
		// Resize at a point chosen by the input, then keep writing.
		cut := len(data) / 2
		term.Write(data[:cut])
		checkInvariants(t, term)
		term.Resize(int(w%12)+1, int(h%8)+1)
		checkInvariants(t, term)
		term.Write(data[cut:])
		checkInvariants(t, term)
		term.TakeScroll()
		term.TakeDirty()
	})
}

// catInput returns about n bytes of `ls -l --color` style output.
func catInput(n int) []byte {
	var b bytes.Buffer
	colors := []string{"01;34", "01;32", "00", "01;36", "38;5;208", "38;2;200;100;50"}
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "-rw-r--r--  1 user  staff  %8d Nov 14 10:%02d \x1b[%sm%s_%d.go\x1b[0m\r\n",
			i*37%100000, i%60, colors[i%len(colors)], "file_name", i)
	}
	return b.Bytes()
}

func BenchmarkTerminalCat(b *testing.B) {
	in := catInput(1 << 20)
	term := New(Options{Cols: 120, Rows: 40})
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		term.Write(in)
	}
}
