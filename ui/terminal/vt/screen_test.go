package vt

import (
	"strconv"
	"testing"
)

func TestWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'中', 2},
		{'😀', 2},
		{0x0301, 0}, // combining acute accent
		{'é', 1},
		{' ', 1},
		{'~', 1},
		{0x00, 0},
		{0x1B, 0},
		{0x7F, 0},
		{0x9F, 0},
		{0x00AD, 0}, // soft hyphen (Cf)
		{0x200B, 0}, // zero width space
		{0x200D, 0}, // zero width joiner
		{0x2028, 0},
		{0x2060, 0},
		{0xFEFF, 0},
		{0x1100, 2}, // Hangul choseong (W)
		{0x1160, 0}, // Hangul jungseong
		{0x11FF, 0},
		{'가', 2},
		{0xFF21, 2}, // fullwidth A (F)
		{0xFF61, 1}, // halfwidth (H)
		{0x3400, 2},
		{0x2A6E0, 2}, // unassigned in plane 2 defaults to W
		{0x231A, 2},  // watch, Emoji_Presentation
		{0x2764, 1},  // heavy heart: emoji but text presentation by default
		{0xFE0F, 0},  // variation selector 16 (Mn)
		{0x10FFFF, 1},
		{-1, 1},
	}
	for _, c := range cases {
		if got := Width(c.r); got != c.want {
			t.Errorf("Width(%U) = %d, want %d", c.r, got, c.want)
		}
	}
}

// lineText returns the line's runes with blank cells as spaces, trailing
// spaces kept.
func lineText(l *Line) string {
	b := make([]rune, 0, len(l.Cells))
	for _, c := range l.Cells {
		switch {
		case c.Width == 0:
		case c.Rune == 0:
			b = append(b, ' ')
		default:
			b = append(b, c.Rune)
		}
	}
	return string(b)
}

// putLine writes s at the start of active row y.
func putLine(s *Screen, y int, text string) {
	l := s.Line(y)
	for i, r := range []rune(text) {
		l.Cells[i] = Cell{Rune: r, Width: 1}
	}
}

// emit simulates a program printing one numbered line per call: the
// cursor sits on the last row, so every line after the first rows lines
// scrolls the screen first.
type emitter struct {
	s *Screen
	n int
}

func (e *emitter) line() {
	_, rows := e.s.Size()
	y := e.n
	if y >= rows {
		e.s.ScrollUp(0, rows-1)
		y = rows - 1
	}
	e.n++
	putLine(e.s, y, "L"+strconv.Itoa(e.n))
}

func scrollbackText(t *testing.T, s *Screen) []string {
	t.Helper()
	old := s.ViewOffset()
	defer s.SetViewOffset(old)
	n := s.ScrollbackLen()
	out := make([]string, n)
	for i := range n {
		// At offset n-i the viewport's top row is scrollback line i.
		s.SetViewOffset(n - i)
		if s.ViewOffset() != n-i {
			t.Fatalf("SetViewOffset(%d) gave %d", n-i, s.ViewOffset())
		}
		out[i] = trimRight(lineText(s.ViewLine(0)))
	}
	return out
}

func activeText(s *Screen) []string {
	_, rows := s.Size()
	out := make([]string, rows)
	for y := range rows {
		out[y] = trimRight(lineText(s.Line(y)))
	}
	return out
}

func trimRight(s string) string {
	i := len(s)
	for i > 0 && s[i-1] == ' ' {
		i--
	}
	return s[:i]
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewScreenBlank(t *testing.T) {
	s := NewScreen(4, 2, 5)
	if c, r := s.Size(); c != 4 || r != 2 {
		t.Fatalf("Size() = %d,%d", c, r)
	}
	for y := range 2 {
		l := s.Line(y)
		if len(l.Cells) != 4 {
			t.Fatalf("row %d has %d cells", y, len(l.Cells))
		}
		for x, c := range l.Cells {
			if c != (Cell{Width: 1}) {
				t.Fatalf("cell (%d,%d) = %+v, want blank", x, y, c)
			}
		}
	}
	if s.ScrollbackLen() != 0 || s.ViewOffset() != 0 {
		t.Fatal("new screen has scrollback or view offset")
	}
}

func TestScrollUpIntoScrollback(t *testing.T) {
	s := NewScreen(10, 3, 10)
	e := &emitter{s: s}
	for range 5 {
		e.line()
	}
	if got := s.ScrollbackLen(); got != 2 {
		t.Fatalf("ScrollbackLen() = %d, want 2", got)
	}
	if got, want := scrollbackText(t, s), []string{"L1", "L2"}; !equalStrings(got, want) {
		t.Fatalf("scrollback = %q, want %q", got, want)
	}
	if got, want := activeText(s), []string{"L3", "L4", "L5"}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
	// The viewport is at the bottom, so it shows the active area.
	for y := range 3 {
		if s.ViewLine(y) != s.Line(y) {
			t.Fatalf("ViewLine(%d) != Line(%d) at offset 0", y, y)
		}
	}
}

func TestScrollUpClearsNewLine(t *testing.T) {
	s := NewScreen(4, 2, 1)
	putLine(s, 0, "aaaa")
	putLine(s, 1, "bbbb")
	s.Line(1).Wrapped = true
	s.ScrollUp(0, 1) // "aaaa" into scrollback
	s.ScrollUp(0, 1) // evicts "aaaa"; its line is reused as the new bottom row
	l := s.Line(1)
	if lineText(l) != "    " || l.Wrapped {
		t.Fatalf("new bottom row = %q wrapped=%v, want blank", lineText(l), l.Wrapped)
	}
	if !l.Dirty {
		t.Fatal("new bottom row not dirty")
	}
	if got := scrollbackText(t, s); !equalStrings(got, []string{"bbbb"}) {
		t.Fatalf("scrollback = %q", got)
	}
	s.SetViewOffset(1)
	if !s.ViewLine(0).Wrapped {
		t.Fatal("scrollback line lost its Wrapped flag")
	}
}

func TestScrollbackEvicts(t *testing.T) {
	s := NewScreen(10, 3, 3)
	e := &emitter{s: s}
	for range 13 { // 3 to fill the screen, then 10 scrolls
		e.line()
	}
	if got := s.ScrollbackLen(); got != 3 {
		t.Fatalf("ScrollbackLen() = %d, want 3", got)
	}
	if got, want := scrollbackText(t, s), []string{"L8", "L9", "L10"}; !equalStrings(got, want) {
		t.Fatalf("scrollback = %q, want %q", got, want)
	}
	if got, want := activeText(s), []string{"L11", "L12", "L13"}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
}

func TestScrollbackViewportStableOnOutput(t *testing.T) {
	s := NewScreen(10, 3, 5)
	e := &emitter{s: s}
	for range 6 { // scrollback L1..L3
		e.line()
	}
	s.SetViewOffset(2)
	if s.ViewOffset() != 2 {
		t.Fatalf("ViewOffset() = %d, want 2", s.ViewOffset())
	}
	top := trimRight(lineText(s.ViewLine(0)))
	if top != "L2" {
		t.Fatalf("view top = %q, want L2", top)
	}
	e.line() // one more line scrolls into scrollback
	if s.ViewOffset() != 3 {
		t.Fatalf("after scroll ViewOffset() = %d, want 3", s.ViewOffset())
	}
	if got := trimRight(lineText(s.ViewLine(0))); got != top {
		t.Fatalf("view top changed to %q, want %q", got, top)
	}

	// Fill the scrollback (5) and keep scrolling: the view keeps its
	// content until that content is evicted, then sticks to the oldest line.
	e.line() // scrollback L1..L5, offset 4, top L2
	if s.ScrollbackLen() != 5 || s.ViewOffset() != 4 {
		t.Fatalf("len=%d offset=%d, want 5,4", s.ScrollbackLen(), s.ViewOffset())
	}
	e.line() // evicts L1; scrollback L2..L6; top still L2 at offset 5
	if s.ViewOffset() != 5 {
		t.Fatalf("offset = %d, want 5", s.ViewOffset())
	}
	if got := trimRight(lineText(s.ViewLine(0))); got != "L2" {
		t.Fatalf("view top = %q, want L2", got)
	}
	for range 4 {
		e.line()
		if s.ViewOffset() > s.ScrollbackLen() {
			t.Fatalf("ViewOffset %d > ScrollbackLen %d", s.ViewOffset(), s.ScrollbackLen())
		}
	}
	if s.ViewOffset() != 5 {
		t.Fatalf("offset = %d, want clamped 5", s.ViewOffset())
	}
	if got, want := trimRight(lineText(s.ViewLine(0))), scrollbackText(t, s)[0]; got != want {
		t.Fatalf("view top = %q, want oldest line %q", got, want)
	}

	// At the bottom the view follows output.
	s.SetViewOffset(0)
	e.line()
	if s.ViewOffset() != 0 {
		t.Fatalf("offset at bottom moved to %d", s.ViewOffset())
	}
}

func TestSetViewOffsetClamps(t *testing.T) {
	s := NewScreen(10, 3, 5)
	e := &emitter{s: s}
	for range 5 {
		e.line()
	}
	s.SetViewOffset(-4)
	if s.ViewOffset() != 0 {
		t.Fatalf("offset = %d, want 0", s.ViewOffset())
	}
	s.SetViewOffset(100)
	if s.ViewOffset() != 2 {
		t.Fatalf("offset = %d, want 2", s.ViewOffset())
	}
	// Viewport rows below the scrollback continue into the active area.
	s.SetViewOffset(1)
	want := []string{"L2", "L3", "L4"}
	for y, w := range want {
		if got := trimRight(lineText(s.ViewLine(y))); got != w {
			t.Fatalf("ViewLine(%d) = %q, want %q", y, got, w)
		}
	}
}

func TestScrollRegionNoScrollback(t *testing.T) {
	s := NewScreen(10, 4, 10)
	for y, txt := range []string{"A", "B", "C", "D"} {
		putLine(s, y, txt)
	}
	s.ScrollUp(1, 2)
	if s.ScrollbackLen() != 0 {
		t.Fatalf("ScrollbackLen() = %d, want 0", s.ScrollbackLen())
	}
	if got, want := activeText(s), []string{"A", "C", "", "D"}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
}

func TestScrollRegionAtTopFeedsScrollback(t *testing.T) {
	// As in xterm.js, a region that starts at the top row still pushes
	// its top line into the scrollback; rows below the region stay put.
	s := NewScreen(10, 4, 10)
	for y, txt := range []string{"A", "B", "C", "D"} {
		putLine(s, y, txt)
	}
	s.ScrollUp(0, 1)
	if got := scrollbackText(t, s); !equalStrings(got, []string{"A"}) {
		t.Fatalf("scrollback = %q, want [A]", got)
	}
	if got, want := activeText(s), []string{"B", "", "C", "D"}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
}

func TestScrollRegionAtTopEvicting(t *testing.T) {
	// Partial top region with a full scrollback: the ring wraps while the
	// rows below the region keep their content.
	s := NewScreen(10, 4, 2)
	for y, txt := range []string{"A", "B", "C", "D"} {
		putLine(s, y, txt)
	}
	for i := range 7 {
		putLine(s, 1, "N"+strconv.Itoa(i))
		s.ScrollUp(0, 1)
	}
	if got, want := scrollbackText(t, s), []string{"N4", "N5"}; !equalStrings(got, want) {
		t.Fatalf("scrollback = %q, want %q", got, want)
	}
	if got, want := activeText(s), []string{"N6", "", "C", "D"}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
}

func TestScrollOneRowScreen(t *testing.T) {
	s := NewScreen(3, 1, 2)
	putLine(s, 0, "a")
	s.ScrollUp(0, 0)
	if got := scrollbackText(t, s); !equalStrings(got, []string{"a"}) {
		t.Fatalf("scrollback = %q", got)
	}
	if got := activeText(s); !equalStrings(got, []string{""}) {
		t.Fatalf("active = %q", got)
	}
}

func TestScrollNoScrollbackScreen(t *testing.T) {
	// A screen with no scrollback (the alternate screen) drops lines.
	s := NewScreen(10, 2, 0)
	putLine(s, 0, "A")
	putLine(s, 1, "B")
	s.ScrollUp(0, 1)
	if s.ScrollbackLen() != 0 {
		t.Fatalf("ScrollbackLen() = %d", s.ScrollbackLen())
	}
	if got, want := activeText(s), []string{"B", ""}; !equalStrings(got, want) {
		t.Fatalf("active = %q, want %q", got, want)
	}
}

func TestStyleTableIntern(t *testing.T) {
	s := NewScreen(4, 2, 0)
	tab := s.Styles()
	if got := tab.Get(0); got != (Style{}) {
		t.Fatalf("Get(0) = %+v, want default", got)
	}
	if got := tab.Intern(Style{}); got != 0 {
		t.Fatalf("Intern(default) = %d, want 0", got)
	}
	bold := Style{Attr: AttrBold}
	red := Style{Fg: IndexedColor(1)}
	rgb := Style{Bg: RGBColor(10, 20, 30), Attr: AttrItalic | AttrUnderline}
	i1, i2, i3 := tab.Intern(bold), tab.Intern(red), tab.Intern(rgb)
	if i1 == 0 || i2 == 0 || i3 == 0 || i1 == i2 || i2 == i3 || i1 == i3 {
		t.Fatalf("indexes %d %d %d not distinct and non-zero", i1, i2, i3)
	}
	if tab.Intern(Style{Attr: AttrBold}) != i1 || tab.Intern(red) != i2 || tab.Intern(rgb) != i3 {
		t.Fatal("same style interned to a different index")
	}
	if tab.Get(i3) != rgb {
		t.Fatalf("Get(%d) = %+v, want %+v", i3, tab.Get(i3), rgb)
	}
}

func TestColorEncoding(t *testing.T) {
	if c := IndexedColor(200); c != Color(1<<24|200) || c.Kind() != ColorIndexed || c.Index() != 200 {
		t.Fatalf("IndexedColor(200) = %#x", uint32(c))
	}
	c := RGBColor(10, 20, 30)
	if c != Color(2<<24|10<<16|20<<8|30) || c.Kind() != ColorRGB {
		t.Fatalf("RGBColor = %#x", uint32(c))
	}
	if r, g, b := c.RGB(); r != 10 || g != 20 || b != 30 {
		t.Fatalf("RGB() = %d,%d,%d", r, g, b)
	}
	if ColorDefault.Kind() != ColorKindDefault {
		t.Fatal("ColorDefault kind")
	}
}

func TestStyleTableCompact(t *testing.T) {
	s := NewScreen(4, 2, 2)
	tab := s.Styles()
	tab.limit = 8 // small table so the test can fill it
	keep := tab.Intern(Style{Fg: IndexedColor(7)})
	s.Line(0).Cells[0] = Cell{Rune: 'x', Style: keep, Width: 1}
	s.ScrollUp(0, 1) // the styled cell moves to the scrollback
	for i := range 6 {
		tab.Intern(Style{Fg: IndexedColor(uint8(100 + i))})
	}
	if tab.Len() != 8 {
		t.Fatalf("Len() = %d, want 8", tab.Len())
	}
	// Table is full: the next new style compacts away unused entries.
	ni := tab.Intern(Style{Fg: IndexedColor(50)})
	if tab.Len() != 3 {
		t.Fatalf("after compact Len() = %d, want 3", tab.Len())
	}
	if tab.Get(ni) != (Style{Fg: IndexedColor(50)}) {
		t.Fatal("new style not stored")
	}
	s.SetViewOffset(1)
	c := s.ViewLine(0)
	if got := tab.Get(c.Cells[0].Style); got != (Style{Fg: IndexedColor(7)}) {
		t.Fatalf("scrollback cell style = %+v after compact", got)
	}
	// A full table with every entry in use falls back to the default style.
	tab.limit = 3
	s.Line(1).Cells[1].Style = ni
	if got := tab.Intern(Style{Fg: IndexedColor(51)}); got != 0 {
		t.Fatalf("Intern on a full, fully used table = %d, want 0", got)
	}
	if tab.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", tab.Len())
	}
	if tab.Get(ni) != (Style{Fg: IndexedColor(50)}) {
		t.Fatal("in-use style lost")
	}
}

func BenchmarkScrollUp(b *testing.B) {
	const cols, rows, lines = 200, 50, 1_000_000
	s := NewScreen(cols, rows, 10_000)
	for b.Loop() {
		for range lines {
			s.ScrollUp(0, rows-1)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*lines), "ns/line")
}
