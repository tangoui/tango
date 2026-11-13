package editor

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// naiveRows checks vl against the plain sums of its row counts.
func checkViewLines(t *testing.T, vl *viewLines) {
	t.Helper()
	v := 0
	for l := range vl.n {
		if got := vl.viewOf(l); got != v {
			t.Fatalf("viewOf(%d) = %d, want %d", l, got, v)
		}
		for r := range vl.rowsOf(l) {
			if gl, gr := vl.find(v + r); gl != l || gr != r {
				t.Fatalf("find(%d) = %d,%d, want %d,%d", v+r, gl, gr, l, r)
			}
		}
		v += vl.rowsOf(l)
	}
	if vl.Count() != v {
		t.Fatalf("Count %d, want %d", vl.Count(), v)
	}
}

// TestViewLinesMapping checks the Fenwick mapping against plain sums while
// lines are measured in random order and the text is edited (incremental
// updates, line count changes, several edits on one line).
func TestViewLinesMapping(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	for i := range 300 {
		b.WriteString(strings.Repeat("word ", rng.IntN(12)))
		if i < 299 {
			b.WriteString("\n")
		}
	}
	buf := buffer.New(b.String())
	vl := &viewLines{}
	vl.configure(wrapConfig{col: 10, tab: 4, indent: WrapIndentNone}, 7, buf, buf.Snapshot())
	checkViewLines(t, vl)
	for range 200 {
		snap := buf.Snapshot()
		vl.breaksOf(snap, rng.IntN(snap.LineCount()))
	}
	checkViewLines(t, vl)
	// Rows of measured lines match their breaks.
	snap := buf.Snapshot()
	for l := range snap.LineCount() {
		if vl.rows[l] != 0 && int(vl.rows[l]) != computeBreaks(snap.Line(l), vl.cfg).rows() {
			t.Fatalf("line %d: %d rows", l, vl.rows[l])
		}
	}
	for step := range 60 {
		snap := buf.Snapshot()
		var edits []buffer.Edit
		at := 0
		for range 1 + rng.IntN(3) {
			a := at + rng.IntN(max(1, (snap.Len()-at)/3))
			if a > snap.Len() {
				break
			}
			e := min(snap.Len(), a+rng.IntN(30))
			txt := []string{"x", "word word word ", "\n", "a\nb\nc", ""}[rng.IntN(5)]
			edits = append(edits, buffer.Edit{Start: a, End: e, Text: txt})
			at = e + 1
		}
		if _, err := buf.Edit(edits, nil, nil); err != nil {
			continue
		}
		vl.sync(buf, buf.Snapshot())
		if vl.n != buf.Snapshot().LineCount() {
			t.Fatalf("step %d: %d lines, want %d", step, vl.n, buf.Snapshot().LineCount())
		}
		checkViewLines(t, vl)
		for range 20 {
			s := buf.Snapshot()
			vl.breaksOf(s, rng.IntN(s.LineCount()))
		}
		checkViewLines(t, vl)
		s := buf.Snapshot()
		for l := range s.LineCount() {
			if vl.rows[l] != 0 && int(vl.rows[l]) != computeBreaks(s.Line(l), vl.cfg).rows() {
				t.Fatalf("step %d: line %d: %d rows, want %d", step, l, vl.rows[l], computeBreaks(s.Line(l), vl.cfg).rows())
			}
		}
	}
	// ModelToView and ViewToModel are inverse on the column grid.
	s := buf.Snapshot()
	for l := range s.LineCount() {
		text := s.Line(l)
		for c := 0; c <= len(text); c = nextGrapheme(text, c) {
			v, x := vl.ModelToView(l, c)
			gl, gc := vl.ViewToModel(v, x)
			if gl != l || gc != c {
				t.Fatalf("line %d col %d: view %d x %v back to %d,%d", l, c, v, x, gl, gc)
			}
			if c == len(text) {
				break
			}
		}
	}
}

// bigWrapDoc is n lines, every fifth one long enough to wrap a few times.
func bigWrapDoc(n int) string {
	var b strings.Builder
	long := strings.Repeat("lorem ipsum dolor ", 9)
	for i := range n {
		if i%5 == 0 {
			b.WriteString(long)
		} else {
			b.WriteString("short line")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// TestWrapResizeLazy: a 1M-line document with word wrap on wraps only the
// lines around the viewport on its first frame, after a resize and after
// jumps; the top model line stays put on resizes, and jumps reveal the
// caret.
func TestWrapResizeLazy(t *testing.T) {
	goText(t)
	n := 1_000_000
	if raceEnabled {
		n = 200_000
	}
	text := bigWrapDoc(n)
	start := time.Now()
	st := NewState(text, "big.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, 600, 400, pinMac)
	open := time.Since(start)
	t.Logf("%d lines: open and two frames with wrap on in %v", n, open)
	if open > 5*time.Second {
		t.Errorf("opening took %v", open)
	}
	vl := &st.vl
	if !vl.on() {
		t.Fatal("not wrapping")
	}
	rows := int(st.Viewport.Height / st.Viewport.LineHeight)
	if vl.computed > 4*rows+16 {
		t.Fatalf("first frames wrapped %d lines for %d visible rows", vl.computed, rows)
	}
	// Scroll to the middle, then resize: the top model line stays and only
	// the lines near it are wrapped again.
	st.Viewport.TopLine = vl.viewOf(n / 2)
	ed.Frame()
	top, row := st.topAnchor()
	if top != n/2 || row != 0 {
		t.Fatalf("top %d row %d, want %d", top, row, n/2)
	}
	before := vl.computed
	ed.Resize(420, 400)
	ed.Frame()
	if l, _ := st.topAnchor(); l != n/2 {
		t.Fatalf("top model line %d after the resize, want %d", l, n/2)
	}
	if d := vl.computed - before; d > 4*rows+16 {
		t.Fatalf("resize wrapped %d lines", d)
	}
	// The visible rows are exact: each shows its line's row.
	first, last := st.VisibleLines()
	snap := st.Snapshot()
	for v := first; v <= last; v++ {
		l, r := vl.find(v)
		if lb := computeBreaks(snap.Line(l), vl.cfg); r >= lb.rows() {
			t.Fatalf("view line %d: row %d of line %d with %d rows", v, r, l, lb.rows())
		}
	}
	// Ctrl+End and Ctrl+Home (Cmd on the Mac keymap) reveal the caret.
	ed.focus(t)
	for _, k := range []input.Key{input.KeyDown, input.KeyUp} {
		before := vl.computed
		ed.Key(k, input.ModMeta)
		ed.Frame()
		first, last := st.VisibleLines()
		if r := st.headRow(); r < first || r > last {
			t.Fatalf("after Cmd+%v: caret row %d outside %d..%d", k, r, first, last)
		}
		if d := vl.computed - before; d > 4*rows+16 {
			t.Fatalf("jump wrapped %d lines", d)
		}
	}
	if st.Primary().Head != 0 {
		t.Fatalf("Cmd+Up: head %d", st.Primary().Head)
	}
}

// TestWrapScrollStable: scrolling a wrapped document up from the middle
// (rows above the top becoming known) never moves the picture: the
// anchored top model line changes only by the scrolled rows.
func TestWrapScrollStable(t *testing.T) {
	goText(t)
	st := NewState(bigWrapDoc(5000), "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, 500, 300, pinMac)
	st.Viewport.TopLine = st.vl.viewOf(4000)
	ed.Frame()
	lh := st.Viewport.LineHeight
	prevLine, prevRow := st.topAnchor()
	p := geom.Pt(200, 150)
	for range 30 {
		ed.Inject(input.Scroll{Pos: p, DY: -lh})
		ed.Frame()
		l, r := st.topAnchor()
		// One row up from (prevLine, prevRow).
		wl, wr, _ := st.stepRows(prevLine, prevRow, Backward, 1)
		if l != wl || r != wr {
			t.Fatalf("scrolled one row up from %d/%d to %d/%d, want %d/%d", prevLine, prevRow, l, r, wl, wr)
		}
		prevLine, prevRow = l, r
	}
}

// TestWrapOtherViewEdit: an edit through another view above the top keeps
// the top model line's text at the top of a wrapped view.
func TestWrapOtherViewEdit(t *testing.T) {
	goText(t)
	m := NewModel("a.txt", bigWrapDoc(400), DefaultRegistry())
	st := NewView(m, theme.Base("vs"))
	defer st.Close()
	st.Options.WordWrap = WrapOn
	other := NewView(m, theme.Base("vs"))
	defer other.Close()
	ed := newEditDriver(st, 500, 300, pinMac)
	st.Viewport.TopLine = st.vl.viewOf(200)
	ed.Frame()
	want := st.Snapshot().Line(200)
	other.MoveCaret(other.Snapshot().LineStart(10), false)
	other.Insert("one\ntwo\nthree lorem ipsum dolor lorem ipsum dolor lorem ipsum dolor lorem ipsum dolor\n")
	ed.Frame()
	l, r := st.topAnchor()
	if got := st.Snapshot().Line(l); got != want || r != 0 {
		t.Fatalf("top line %d row %d %q, want %q", l, r, got, want)
	}
}

// TestWrapIndentView: continuation rows start at the indentation x.
func TestWrapIndentView(t *testing.T) {
	goText(t)
	st := NewState("\t\t"+strings.Repeat("word ", 40)+"\n", "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	st.Options.WrappingIndent = WrapIndentIndent
	ed := newEditDriver(st, 400, 300, pinMac)
	w := *ed.vw
	snap := st.Snapshot()
	lb := st.breaks(0)
	if lb.rows() < 3 || lb.indent != 12 {
		t.Fatalf("%d rows, indent %d", lb.rows(), lb.indent)
	}
	for k := 1; k < lb.rows(); k++ {
		lv := w.rowAt(snap, 0, k)
		if want := 12 * w.vs.colW; lv.segs[0].x0 != want {
			t.Fatalf("row %d at x %v, want %v", k, lv.segs[0].x0, want)
		}
	}
}

// TestWrapViewState: SaveViewState records the top model line (not the
// view line) and RestoreViewState converts it back.
func TestWrapViewState(t *testing.T) {
	goText(t)
	st := NewState(bigWrapDoc(300), "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, 500, 300, pinMac)
	st.breaks(100) // line 100 wraps
	st.Viewport.TopLine = st.vl.viewOf(100) + 1
	ed.Frame()
	if l, r := st.topAnchor(); l != 100 || r != 1 {
		t.Fatalf("top %d/%d", l, r)
	}
	vs := st.SaveViewState()
	if vs.TopLine != 100 {
		t.Fatalf("saved TopLine %d, want model line 100", vs.TopLine)
	}
	st.Viewport.TopLine = 0
	ed.Frame()
	st.RestoreViewState(vs)
	ed.Frame()
	if l, r := st.topAnchor(); l != 100 || r != 0 {
		t.Fatalf("restored top %d/%d, want 100/0", l, r)
	}
}
