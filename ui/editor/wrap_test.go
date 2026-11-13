package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// wrapRows returns the rows of s wrapped at col (tab 4, indent mode).
func wrapRows(s string, col int, mode WrappingIndentMode) ([]string, int) {
	lb := computeBreaks(s, wrapConfig{col: col, tab: 4, indent: mode})
	var out []string
	for k := range lb.rows() {
		b0, b1 := lb.rowRange(k, len(s))
		out = append(out, s[b0:b1])
	}
	return out, lb.indent
}

func eqRows(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s: rows %q, want %q", what, got, want)
	}
}

// TestWrapBreakRules checks Monaco's break rules: English breaks after the
// blanks, CJK between any two ideographs, URLs after "/" (and "." "?"
// "&"), before "(" and other break-before characters; kinsoku: no row
// starts with a break-after character such as "，" after CJK.
func TestWrapBreakRules(t *testing.T) {
	for _, c := range []struct {
		name string
		s    string
		col  int
		want []string
	}{
		{"english", "hello world foo bar", 12, []string{"hello world ", "foo bar"}},
		{"english narrow", "hello world foo bar", 8, []string{"hello ", "world ", "foo bar"}},
		{"no break point", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"blank run", "aaaa    bbbb", 6, []string{"aaaa  ", "  bbbb"}},
		{"cjk", "你好世界你好世界", 6, []string{"你好世", "界你好", "世界"}},
		{"cjk mixed", "abc你好def", 6, []string{"abc你", "好def"}},
		{"cjk punctuation", "你好，世界。你好", 6, []string{"你好，", "世界。", "你好"}},
		{"url", "see https://example.com/a/b/c?x=1", 22, []string{"see https://example.", "com/a/b/c?x=1"}},
		{"url slashes", "https://example.com/path/to/file", 12, []string{"https://", "example.com/", "path/to/file"}},
		{"break before", "foo(bar)baz(qux)", 10, []string{"foo(bar)", "baz(qux)"}},
		{"fits", "short", 10, []string{"short"}},
		{"tab", "a\tbcdefgh", 6, []string{"a\t", "bcdefg", "h"}},
		{"emoji", "ab😀😀😀cd", 4, []string{"ab😀", "😀😀", "cd"}},
		{"combining", "ab" + strings.Repeat("é", 5), 4, []string{"abéé", "ééé"}},
	} {
		got, _ := wrapRows(c.s, c.col, WrapIndentNone)
		eqRows(t, c.name, got, c.want)
	}
	// Classes: the ideographic ranges take precedence (Monaco's
	// classifier), the lists hold Monaco's defaults.
	for r, want := range map[rune]uint8{
		'a': wcNone, ' ': wcBreakAfter, '/': wcBreakAfter, '(': wcBreakBefore, '你': wcIdeographic,
		'ー': wcIdeographic, 'ぁ': wcIdeographic, '，': wcBreakAfter, '「': wcBreakBefore, '😀': wcNone,
	} {
		if got := wrapClass(r); got != want {
			t.Errorf("class of %q = %d, want %d", r, got, want)
		}
	}
	// A break never splits a grapheme cluster.
	s := strings.Repeat("👩‍💻", 6)
	lb := computeBreaks(s, wrapConfig{col: 3, tab: 4, indent: WrapIndentNone})
	for _, b := range lb.starts {
		if nextGrapheme(s, prevGrapheme(s, b)) != b {
			t.Errorf("break %d inside a cluster", b)
		}
	}
	if lb.rows() != 6 {
		t.Errorf("%d rows of ZWJ emoji at column 3, want 6", lb.rows())
	}
}

// TestWrapIndentModes checks wrappingIndent: none, same, indent (+1 tab
// stop), deepIndent (+2), and the fallback to no indent when nothing would
// fit after it.
func TestWrapIndentModes(t *testing.T) {
	s := "    aaaa bbbb cccc dddd"
	for _, c := range []struct {
		mode   WrappingIndentMode
		col    int
		indent int
		want   []string
	}{
		{WrapIndentNone, 14, 0, []string{"    aaaa bbbb ", "cccc dddd"}},
		{WrapIndentSame, 14, 4, []string{"    aaaa bbbb ", "cccc dddd"}},
		{WrapIndentIndent, 14, 8, []string{"    aaaa bbbb ", "cccc ", "dddd"}},
		{WrapIndentDeep, 14, 12, []string{"    aaaa bbbb ", "cc", "cc", " ", "dd", "dd"}},
		{WrapIndentDeep, 13, 0, []string{"    aaaa ", "bbbb cccc ", "dddd"}},
	} {
		got, ind := wrapRows(s, c.col, c.mode)
		if ind != c.indent {
			t.Errorf("%s at %d: indent %d, want %d", c.mode, c.col, ind, c.indent)
		}
		eqRows(t, string(c.mode), got, c.want)
	}
	if _, ind := wrapRows("\tx yyyyyyyyyy", 8, WrapIndentSame); ind != 4 {
		t.Errorf("tab indent %d, want 4", ind)
	}
	if got := wrapIndentMode(""); got != WrapIndentSame {
		t.Errorf("default wrapping indent %q", got)
	}
	if o := DefaultOptions(); o.WordWrap != WrapOff || o.WordWrapColumn != 80 || o.WrappingIndent != WrapIndentSame {
		t.Errorf("defaults %q %d %q", o.WordWrap, o.WordWrapColumn, o.WrappingIndent)
	}
	for _, c := range []struct {
		mode WrapMode
		want int
	}{{WrapOff, 0}, {WrapOn, 49}, {WrapColumn, 80}, {WrapBounded, 49}} {
		if got := wrapColumn(c.mode, 80, 500, 10); got != c.want {
			t.Errorf("wrapColumn(%s) = %d, want %d", c.mode, got, c.want)
		}
	}
}

// wrapEditor returns an editor showing text with word wrap on in a view w
// pixels wide.
func wrapEditor(t *testing.T, text string, w, h float32) (*State, *editDriver) {
	t.Helper()
	goText(t)
	st := NewState(text, "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, w, h, pinMac)
	ed.focus(t)
	return st, ed
}

// rowRect returns the window rectangle of the caret before off on its
// row as the View lays it out (up: at the end of the earlier row).
func rowRect(t *testing.T, ed *editDriver, st *State, off int, up bool) geom.Rect {
	t.Helper()
	w := *ed.vw
	x, ok := w.contentXUp(st.Snapshot(), off, up)
	if !ok {
		t.Fatalf("no caret x at %d", off)
	}
	r := st.rowOf(off, up)
	v := st.Viewport
	g := w.vs
	return geom.Rect{X: g.text.X - v.ScrollX + x, Y: g.text.Y - v.OffsetY + float32(r-v.TopLine)*v.LineHeight, W: 2, H: v.LineHeight}
}

// TestWrapMixedScripts drives a wrapped line of CJK, tabs, emoji and
// combining marks: Up/Down move by view line, Home/End go to the row's
// edges first and then to the line's (Monaco), and a click at the caret's
// rectangle puts the caret back there.
func TestWrapMixedScripts(t *testing.T) {
	line := strings.Repeat("ab\t你好世界😀x👩‍💻é̂ word ", 12)
	st, ed := wrapEditor(t, "first\n"+line+"\nlast line\n", 300, 400)
	w := *ed.vw
	snap := st.Snapshot()
	lb := st.breaks(1)
	if lb.rows() < 4 {
		t.Fatalf("line wraps into %d rows at width 300", lb.rows())
	}
	ls := snap.LineStart(1)
	// Every row fits the text area (on the column grid).
	g := w.vs
	for k := range lb.rows() {
		lv := w.rowAt(snap, 1, k)
		if lv.endX > g.text.W+g.colW*2 {
			t.Errorf("row %d ends at %v, text area %v", k, lv.endX, g.text.W)
		}
		if k > 0 && lv.segs[0].x0 != 0 {
			t.Errorf("row %d x0 %v without indentation", k, lv.segs[0].x0)
		}
	}
	// Down from the first line walks the rows one by one, then the last
	// line.
	st.MoveCaret(0, false)
	ed.Frame()
	for k := range lb.rows() {
		ed.Key(input.KeyDown, 0)
		ed.Frame()
		l, r := st.rowPos(st.Primary().Head, st.caretUp())
		if l != 1 || r != k {
			t.Fatalf("down %d: line %d row %d", k+1, l, r)
		}
	}
	ed.Key(input.KeyDown, 0)
	ed.Frame()
	if l := snap.LineOf(st.Primary().Head); l != 2 {
		t.Fatalf("down past the wrapped line: line %d", l)
	}
	// And back up.
	for k := lb.rows() - 1; k >= 0; k-- {
		ed.Key(input.KeyUp, 0)
		ed.Frame()
		if l, r := st.rowPos(st.Primary().Head, st.caretUp()); l != 1 || r != k {
			t.Fatalf("up to row %d: line %d row %d", k, l, r)
		}
	}
	// Home/End on row 2: the row's edges, then the line's.
	b0, b1 := lb.rowRange(2, len(line))
	st.MoveCaret(ls+b0+nextGrapheme(line[b0:], 0), false)
	ed.Frame()
	ed.Key(input.KeyEnd, 0)
	ed.Frame()
	if h := st.Primary().Head; h != ls+b1 || !st.caretUp() {
		t.Fatalf("End: head %d up %v, want the row end %d", h, st.caretUp(), ls+b1)
	}
	if r := st.headRow(); r != st.vl.viewOf(1)+2 {
		t.Fatalf("End: caret on view line %d, want row 2 (%d)", r, st.vl.viewOf(1)+2)
	}
	ed.Key(input.KeyEnd, 0)
	ed.Frame()
	if h := st.Primary().Head; h != ls+len(line) {
		t.Fatalf("End twice: head %d, want the line end %d", h, ls+len(line))
	}
	st.MoveCaret(ls+b0+nextGrapheme(line[b0:], 0), false)
	ed.Key(input.KeyHome, 0)
	ed.Frame()
	if h := st.Primary().Head; h != ls+rowFirstNonBlank(line, b0, b1) {
		t.Fatalf("Home: head %d, want the row start %d", h, ls+b0)
	}
	ed.Key(input.KeyHome, 0)
	ed.Frame()
	if h := st.Primary().Head; h != ls {
		t.Fatalf("Home twice: head %d, want the line start %d", h, ls)
	}
	// Clicks: at each boundary's caret rectangle the caret comes back to
	// that offset and row, and the drawn caret is at the rectangle.
	var offs []int
	for i := 0; i < len(line); i = nextGrapheme(line, i) {
		offs = append(offs, i)
	}
	offs = append(offs, len(line))
	for _, c := range offs {
		off := ls + c
		r := rowRect(t, ed, st, off, false)
		ed.Advance(time.Second)
		ed.Click(geom.Pt(r.X+0.5, r.Y+r.H/2))
		ed.Frame()
		if h := st.Primary().Head; h != off {
			t.Fatalf("click at caret of %d (row %d): head %d", c, st.rowOf(off, false), h-ls)
		}
		if cr := rowRect(t, ed, st, st.Primary().Head, st.caretUp()); abs32(cr.X-r.X) > 1 || cr.Y != r.Y {
			t.Fatalf("click at %d: caret %v, want %v", c, cr, r)
		}
		_, ime := ed.TextInputActive()
		if abs32(ime.X-r.X) > 1 || abs32(ime.Y-r.Y) > 1 {
			t.Fatalf("offset %d: IME caret %v, layout %v", c, ime, r)
		}
	}
	// A click past a row's end puts the caret at that row's end.
	r := rowRect(t, ed, st, ls+b1, true)
	ed.Advance(time.Second)
	ed.Click(geom.Pt(g.text.X+g.text.W-3, r.Y+r.H/2))
	ed.Frame()
	if h := st.Primary().Head; h != ls+b1 || !st.caretUp() || st.headRow() != st.vl.viewOf(1)+2 {
		t.Fatalf("click past row 2's end: head %d up %v row %d", h-ls, st.caretUp(), st.headRow())
	}
}

// TestWrapToggle: editor.action.toggleWordWrap (Alt+Z) turns wrapping on
// and off, keeping the top model line.
func TestWrapToggle(t *testing.T) {
	goText(t)
	var b strings.Builder
	for i := range 200 {
		b.WriteString(strings.Repeat("word ", 30+i%7))
		b.WriteString("\n")
	}
	st := NewState(b.String(), "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 400, 300, pinMac)
	ed.focus(t)
	st.Viewport.TopLine = 50
	ed.Frame()
	if st.WordWrap() != WrapOff || st.wrapping() {
		t.Fatal("wrapping by default")
	}
	if !st.Execute("editor.action.toggleWordWrap", nil) {
		t.Fatal("toggleWordWrap did not run")
	}
	ed.Frame()
	if !st.wrapping() || st.WordWrap() != WrapOn {
		t.Fatal("toggle did not wrap")
	}
	if l := st.topModelLine(); l != 50 {
		t.Fatalf("top model line %d after wrapping, want 50", l)
	}
	if st.Viewport.TopLine <= 50 {
		t.Fatalf("view TopLine %d: rows above not counted", st.Viewport.TopLine)
	}
	ed.Key(input.KeyZ, input.ModAlt)
	ed.Frame()
	if st.wrapping() || st.Viewport.TopLine != 50 {
		t.Fatalf("Alt+Z: wrapping %v, TopLine %d", st.wrapping(), st.Viewport.TopLine)
	}
}

// TestWrapMultiCursorAffinity: each of several cursors keeps its own row
// affinity (Monaco's per-cursor view state): End goes to each row's end
// first, the caret drawn there, then to the line's end; typing at a row's
// end inserts at the break; Up/Down past a row's end stops at that row's
// end.
func TestWrapMultiCursorAffinity(t *testing.T) {
	long := strings.Repeat("lorem ipsum dolor ", 9)
	st, ed := wrapEditor(t, long+"\n"+long+"\n", 500, 300)
	snap := st.Snapshot()
	lb := st.breaks(0)
	if lb.rows() < 3 || st.breaks(1).rows() != lb.rows() {
		t.Fatalf("rows %d / %d", lb.rows(), st.breaks(1).rows())
	}
	b1 := lb.starts[0] // end of row 0
	l1 := snap.LineStart(1)
	st.SetSelections([]Selection{{Anchor: 3, Head: 3}, {Anchor: l1 + 3, Head: l1 + 3}})
	ed.Frame()
	st.Move(Forward, LineEdge, false)
	for k, want := range []int{b1, l1 + b1} {
		if h := st.Sels[k].Head; h != want || !st.headUp(k) {
			t.Fatalf("End: cursor %d head %d up %v, want the row end %d", k, h, st.headUp(k), want)
		}
		if l, r := st.rowPos(st.Sels[k].Head, st.headUp(k)); l != k || r != 0 {
			t.Fatalf("End: cursor %d drawn on line %d row %d, want row 0", k, l, r)
		}
	}
	ed.Frame()
	st.Move(Forward, LineEdge, false)
	for k, want := range []int{len(long), l1 + len(long)} {
		if h := st.Sels[k].Head; h != want {
			t.Fatalf("End twice: cursor %d head %d, want the line end %d", k, h, want)
		}
	}
	// Typing at the row ends inserts at the breaks.
	st.SetSelections([]Selection{{Anchor: 3, Head: 3}, {Anchor: l1 + 3, Head: l1 + 3}})
	st.Move(Forward, LineEdge, false)
	st.Insert("X")
	s := st.Snapshot()
	if got, want := s.Line(0), long[:b1]+"X"+long[b1:]; got != want {
		t.Fatalf("typed at the row end:\n%q\nwant\n%q", got, want)
	}
	if got, want := s.Line(1), long[:b1]+"X"+long[b1:]; got != want {
		t.Fatalf("second cursor typed at:\n%q\nwant\n%q", got, want)
	}
	// Down from the row ends: past the end of the next (shorter or equal)
	// row the carets stop at that row's end, drawn on it.
	st2, ed2 := wrapEditor(t, long+"\n"+long+"\n", 500, 300)
	lb2 := st2.breaks(0)
	l1 = st2.Snapshot().LineStart(1)
	st2.SetSelections([]Selection{{Anchor: 3, Head: 3}, {Anchor: l1 + 3, Head: l1 + 3}})
	st2.Move(Forward, LineEdge, false)
	ed2.Frame()
	for _, d := range []Direction{Forward, Forward, Backward} {
		st2.Move(d, Line, false)
		ed2.Frame()
	}
	for k := range 2 {
		l, r := st2.rowPos(st2.Sels[k].Head, st2.headUp(k))
		if l != k || r != 1 {
			t.Fatalf("Down, Down, Up: cursor %d on line %d row %d, want row 1", k, l, r)
		}
		col := st2.Sels[k].Head - st2.Snapshot().LineStart(l)
		if _, e := lb2.rowRange(1, len(long)); col > e {
			t.Fatalf("cursor %d at col %d past row 1's end %d", k, col, e)
		}
	}
	// The carets keep their goal: one more Down lands on row 2 at the same
	// visual column the End set.
	st2.Move(Forward, Line, false)
	for k := range 2 {
		if _, r := st2.rowPos(st2.Sels[k].Head, st2.headUp(k)); r != 2 {
			t.Fatalf("Down: cursor %d on row %d, want 2", k, r)
		}
	}
}
