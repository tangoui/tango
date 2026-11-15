package editor

import (
	"fmt"
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

// bracketLongLine returns a single line of about n bytes of minified
// TypeScript: one function wrapping statements with nested brackets, tabs
// and CJK comments (a bundle), so a caret between two statements is inside
// a pair whose brackets are the first and the last of the line.
func bracketLongLine(n int) string {
	var sb strings.Builder
	sb.Grow(n + 128)
	sb.WriteString("(function () { ")
	for i := 0; sb.Len() < n-8; i++ {
		fmt.Fprintf(&sb, "const v%d = [a%d, {k: (b + %d) * c[%d]}]; ", i, i, i, i%7)
		if i%5 == 0 {
			sb.WriteString("\t/* 中文注释（全角） */ ")
		}
	}
	sb.WriteString("})();")
	return sb.String()
}

// keystrokeWork is the work of one keystroke typed in the middle of a
// single wrapped line of about n bytes (bracketLongLine), drawn by a View
// with its defaults (bracket pair colorization, matchBrackets "always",
// the minimap), averaged over a few keys.
type keystrokeWork struct {
	bracket, runes, spans, mini int64
	copies                      float64 // the line copied out of the rope (lineMemo)
}

func measureKeystrokeWork(t *testing.T, n int) keystrokeWork {
	return measureKeystrokeWorkIn(t, n, false)
}

// measureKeystrokeWorkIn is measureKeystrokeWork; with sticky the line
// opens a block of 300 lines ("if (a) { " … "}"), so with the caret's rows
// in view the line's start is scrolled out and the line is a sticky
// header too (its chunks from the line start drawn, far from the rows).
func measureKeystrokeWorkIn(t *testing.T, n int, sticky bool) keystrokeWork {
	t.Helper()
	goText(t)
	old := bracketFrameBudget
	bracketFrameBudget = time.Minute // the counts must not depend on the clock
	countWork = true
	defer func() { bracketFrameBudget, countWork = old, false }()
	line := bracketLongLine(n)
	text := line
	if sticky {
		line = "if (a) { " + line
		text = line + "\n" + strings.Repeat("  y(1);\n", 300) + "}\n"
	}
	st := newTestState(text, "bundle.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, 900, 600, pinMac)
	ed.focus(t)
	mid := strings.Index(line[len(line)/2:], "; ") + len(line)/2 + 2
	st.MoveCaret(mid, false)
	st.RevealCaret()
	key := func() {
		ed.Inject(input.TextInput{Text: "x"})
		ed.Frame()
	}
	ed.Frame()
	ed.Frame()
	key()
	key()
	if !st.wrapping() || (st.Snapshot().LineCount() != 1) != sticky {
		t.Fatal("not one wrapped line")
	}
	if sticky && !slices.Contains((*ed.vw).vs.stickyLines, 0) {
		t.Fatal("the long line is not a sticky header")
	}
	const keys = 4
	resetWork()
	for range keys {
		key()
	}
	if got := st.Snapshot().Len(); got != len(text)+keys+2 {
		t.Fatalf("length %d, want %d", got, len(text)+keys+2)
	}
	return keystrokeWork{
		bracket: workBracketBytes.Load() / keys,
		runes:   workRuneBytes.Load() / keys,
		spans:   workOverSpans.Load() / keys,
		mini:    workMiniBytes.Load() / keys,
		copies:  float64(workLineCopies.Load()) / keys,
	}
}

// TestLongLineKeystrokeWork: a keystroke in the middle of a 1 MB wrapped
// line does the same work as in a 64 KB one, up to a constant, in each of
// the paths that used to scan the whole line (Task 5's profile): the
// bracket scanner, the long-line test, the colored brackets' style runs
// and the minimap's prefixes.
func TestLongLineKeystrokeWork(t *testing.T) {
	checkKeystrokeWork(t, false)
}

// TestLongLineKeystrokeWorkSticky is TestLongLineKeystrokeWork with the
// line also a sticky header: its chunks at the line start and the rows in
// view are colored as two windows, not one from the start to the rows.
// TestLongLineBracketColorsDrawn checks the colors drawn in this layout.
func TestLongLineKeystrokeWorkSticky(t *testing.T) {
	checkKeystrokeWork(t, true)
}

func checkKeystrokeWork(t *testing.T, sticky bool) {
	small := measureKeystrokeWorkIn(t, 64<<10, sticky)
	large := measureKeystrokeWorkIn(t, 1<<20, sticky)
	t.Logf("per key, 64 KB line: %+v", small)
	t.Logf("per key, 1 MB line:  %+v", large)
	// The new text's line is copied once (rewrap); every other reader of
	// the key's frame shares it (lineMemo), the old one is the previous
	// frame's.
	if large.copies > 1.5 {
		t.Errorf("the line was copied out of the rope %.1f times per key", large.copies)
	}
	for _, c := range []struct {
		name         string
		small, large int64
		bound        int64 // the most the 1 MB line may cost more
	}{
		{"bracket scan bytes", small.bracket, large.bracket, 32 << 10},
		{"isLong rune-count bytes", small.runes, large.runes, 16 << 10},
		{"overrideBounds spans", small.spans, large.spans, 1024},
		{"minimap bytes", small.mini, large.mini, 64 << 10},
	} {
		if d := c.large - c.small; d > c.bound {
			t.Errorf("%s: %d per key on 1 MB, %d on 64 KB (+%d, at most +%d)", c.name, c.large, c.small, d, c.bound)
		}
	}
}

// TestRewrapReusesRows: typing in a long wrapped line builds each key's
// rows in the slices the previous edit freed (viewLines.spare), so the
// rows of the line alternate between two backing arrays instead of
// allocating new ones (about 330 KB for a 1 MB line) on every key.
func TestRewrapReusesRows(t *testing.T) {
	r := rand.New(rand.NewPCG(4, 4))
	line := genWrapLine(r, 200<<10, "")
	st, ed := wrapTestEditor(t, "first\n"+line+"\nlast\n")
	st.MoveCaret(st.Snapshot().LineStart(1)+fixBoundary(line, len(line)/2), false)
	st.RevealCaret()
	ed.Frame()
	arrays := map[*int]bool{}
	for i := range 12 {
		ed.Type("x")
		lb := st.vl.breaks[1]
		if lb == nil {
			t.Fatalf("key %d: the line's rows were dropped", i)
		}
		if i >= 2 {
			arrays[unsafe.SliceData(lb.starts)] = true
		}
		if !eqBreaks(*lb, computeBreaks(st.Snapshot().Line(1), st.vl.cfg)) {
			t.Fatalf("key %d: rows differ from computeBreaks", i)
		}
	}
	if len(arrays) > 2 {
		t.Errorf("rows in %d backing arrays over 10 keys, want 2", len(arrays))
	}
	// rewrapEdit itself allocates nothing given room.
	cfg := st.vl.cfg
	s := st.Snapshot().Line(1)
	old := computeBreaks(s, cfg)
	ns := s[:len(s)/2] + "y" + s[len(s)/2:]
	dst := lineBreaks{starts: make([]int, 0, 2*len(old.starts)), runes: make([]int, 0, 2*len(old.starts)), reach: make([]int, 0, 2*len(old.starts))}
	if a := testing.AllocsPerRun(5, func() { rewrapEdit(dst, old, s, ns, len(s)/2, 0, 1, cfg) }); a > 1 {
		t.Errorf("rewrapEdit allocates %v times per edit", a)
	}
}

// TestWrapIncrementalLookahead: a break's decision reads the rune after
// the character that overflows (nextGrapheme: is it a combining mark?), so
// an edit right after that character must rescan the break (reach covers
// the lookahead; a reach without it keeps a break that now splits a
// cluster). Random edits of short CJK lines with combining marks at narrow
// columns also keep every reach at least the full computation's.
func TestWrapIncrementalLookahead(t *testing.T) {
	cfg := wrapConfig{col: 1, tab: 4, indent: WrapIndentNone}
	s := "中中中中"
	old := computeBreaks(s, cfg)
	if !slices.Equal(old.starts, []int{3, 6, 9}) {
		t.Fatalf("breaks %v, want each wide character on a row of its own", old.starts)
	}
	ns := s[:3] + "\u0301" + s[3:]
	got, _ := rewrapEdit(lineBreaks{}, old, s, ns, 3, 0, 2, cfg)
	if want := computeBreaks(ns, cfg); !eqBreaks(got, want) {
		t.Fatalf("combining mark after a break: rows at %v, want %v", got.starts, want.starts)
	}
	r := rand.New(rand.NewPCG(8, 1))
	pieces := []string{"中", "a", " ", "\u0301", "\u0302", "，", "(", "x", "\t", "😀"}
	for range 3000 {
		var sb strings.Builder
		for range 4 + r.IntN(40) {
			sb.WriteString(pieces[r.IntN(len(pieces))])
		}
		s := sb.String()
		cfg := wrapConfig{col: 1 + r.IntN(6), tab: 1 + r.IntN(4), indent: WrapIndentNone}
		old := computeBreaks(s, cfg)
		at := fixBoundary(s, r.IntN(len(s)+1))
		end := fixBoundary(s, min(len(s), at+r.IntN(4)))
		if end < at {
			end = at
		}
		ins := pieces[r.IntN(len(pieces))]
		ns := s[:at] + ins + s[end:]
		got, _ := rewrapEdit(lineBreaks{}, old, s, ns, at, end-at, len(ins), cfg)
		want := computeBreaks(ns, cfg)
		if !eqBreaks(got, want) {
			t.Fatalf("%q at col %d: edit at %d: rows %v, want %v", s, cfg.col, at, got.starts, want.starts)
		}
		for k := range want.reach {
			if got.reach[k] < want.reach[k] {
				t.Fatalf("%q: reach[%d] %d below the full computation's %d", ns, k, got.reach[k], want.reach[k])
			}
		}
	}
}

// TestWrapIncrementalSecondViewUndo: a second View of the Model follows
// the keys typed in the first incrementally (State.modelChanged →
// viewLines.syncEdits), and undo through the View rewraps incrementally
// too; every rows equal computeBreaks.
func TestWrapIncrementalSecondViewUndo(t *testing.T) {
	r := rand.New(rand.NewPCG(6, 6))
	line := genWrapLine(r, 120<<10, "  ")
	st, ed := wrapTestEditor(t, "first\n"+line+"\nlast\n")
	st2 := NewView(st.Model, theme.Base("vs"))
	st2.Options.WordWrap = WrapOn
	st2.Options.ColorDecorators = true
	ed2 := newEditDriver(st2, 400, 300, pinMac)
	mid := st.Snapshot().LineStart(1) + fixBoundary(line, len(line)/2)
	st.MoveCaret(mid, false)
	st.RevealCaret()
	st2.MoveCaret(mid, false)
	st2.RevealCaret()
	ed.Frame()
	ed2.Frame()
	check := func(what string, s *State) {
		t.Helper()
		lb := s.vl.breaks[1]
		if lb == nil {
			t.Fatalf("%s: the line's rows were dropped (not rewrapped incrementally)", what)
		}
		if !eqBreaks(*lb, computeBreaks(s.Snapshot().Line(1), s.vl.cfg)) {
			t.Fatalf("%s: rows differ from computeBreaks", what)
		}
	}
	for i := range 6 {
		ed.Type(string(rune('a' + i)))
		check(fmt.Sprintf("key %d, typing view", i), st)
		check(fmt.Sprintf("key %d, second view", i), st2)
	}
	for i := range 3 {
		ed.Key(input.KeyZ, input.ModMeta) // pinMac
		check(fmt.Sprintf("undo %d, typing view", i), st)
		check(fmt.Sprintf("undo %d, second view", i), st2)
		ed2.Frame()
	}
	if st.Snapshot().Len() >= len("first\n"+line+"\nlast\n")+6 {
		t.Fatal("undo did nothing")
	}
}

// TestLongLineBracketColorsDrawn: the bracket colors drawn for a long
// wrapped line, in its rows in view (bracketSpans' windows) and in its
// sticky header (chunks from the line start), are the pixels a View draws
// without the long-line index (whole-line colors), after random edits of
// the line and at several scroll positions.
func TestLongLineBracketColorsDrawn(t *testing.T) {
	goText(t)
	r := rand.New(rand.NewPCG(13, 1))
	var sb strings.Builder
	sb.WriteString("let z = 1;\nif (a) { ")
	for range 3000 {
		sb.WriteString("f(x[1], {k: (v)}); ")
	}
	sb.WriteString(" {\n")
	for i := range 300 {
		fmt.Fprintf(&sb, "  y%d(1);\n", i)
	}
	sb.WriteString("}\n}\n")
	mk := func(text string) (*State, *editDriver) {
		st := newTestState(text, "a.ts", theme.Base("vs"))
		st.Options.WordWrap = WrapOn
		retryDegradedAtOnce(st)
		return st, newEditDriver(st, 700, 420, pinMac)
	}
	st, ed := mk(sb.String())
	bracketLongIndexOff = true
	ref, red := mk(sb.String())
	bracketLongIndexOff = false
	// Drive the actual incremental jobs to completion, not until two
	// images happen to agree: two equal incomplete frames are not done.
	// Precomputing the tree still leaves the View to derive this frame's
	// colors and sticky header; no previous paint supplies those colors.
	settledFrame := func(s *State, d *editDriver) *image.RGBA {
		for frames := 0; frames < maxMatchFrames; frames++ {
			settleTokens(t, s)
			tr := s.Model.brackets()
			tr.beginFrame()
			tr.ensure(modelBrackets{s.Model}, s.Snapshot().LineCount(), time.Now().Add(bracketFrameBudget))
			_, _, degradedBefore := monarch.Stats()
			img := d.Frame()
			_, _, degradedAfter := monarch.Stats()
			// TokenizeSpan (used to paint long-line windows) deliberately
			// stores nothing, so Done alone cannot detect its degraded
			// output. Only compare a frame whose lexer work also finished.
			if tr.complete() && s.Highlighter().Done(s.Snapshot()) && degradedBefore == degradedAfter {
				return img
			}
		}
		t.Fatal("tokens and bracket tree not final within maxMatchFrames")
		return nil
	}
	// The text area's pixels (the minimap's tokens follow the clock).
	frame := func(s *State, d *editDriver, top int, off bool) []byte {
		bracketLongIndexOff = off
		defer func() { bracketLongIndexOff = false }()
		s.Viewport.TopLine = top
		img := settledFrame(s, d)
		tr := (*d.vw).vs.text
		var px []byte
		for y := int(tr.Y); y < int(tr.Y+tr.H); y++ {
			o := img.PixOffset(int(tr.X), y)
			px = append(px, img.Pix[o:o+4*int(tr.W)]...)
		}
		return px
	}
	spans := 0
	compared, sticky := 0, 0
	for step := range 12 {
		if step > 0 {
			// The same edit in both: brackets in line 1.
			l1 := st.Snapshot().LineStart(1)
			n := lineEnd(st.Snapshot(), 1) - l1
			at := l1 + 9 + r.IntN(n-9)
			ins := []string{"(", ")", "{}", "[x]", "x", "{"}[r.IntN(6)]
			for _, s := range []*State{st, ref} {
				s.MoveCaret(at, false)
				s.Insert(ins)
			}
		}
		vl := st.lines()
		v1, rows := vl.viewOf(1), vl.rowsOf(1)
		for _, top := range []int{v1, v1 + r.IntN(rows), v1 + rows - 3, v1 + rows + 40} {
			got := frame(st, ed, top, false)
			want := frame(ref, red, top, true)
			if len((*ed.vw).vs.stickyLines) > 0 {
				sticky++
			}
			spans += len((*ed.vw).vs.brk[1])
			if !slices.Equal(got, want) {
				t.Fatalf("step %d, top view line %d (line 1 at %d, %d rows): pixels differ from whole-line colors", step, top, v1, rows)
			}
			compared++
		}
	}
	if sticky == 0 {
		t.Fatal("line 1 was never a sticky header")
	}
	// A View's first frame, scrolled past line 1: its sticky header has
	// this frame's colors (layoutStickyRows after bracketSpans), not none.
	fresh, fed := mk(st.Snapshot().String())
	vl := fresh.lines()
	top := vl.viewOf(1) + vl.rowsOf(1) + 40
	frameRect := func(s *State, d *editDriver, off bool) []byte {
		bracketLongIndexOff = off
		defer func() { bracketLongIndexOff = false }()
		s.Viewport.TopLine = top
		img := settledFrame(s, d)
		vs := (*d.vw).vs
		if len(vs.stickyClips) == 0 {
			t.Fatal("no sticky header")
		}
		rr := vs.stickyClips[0].Intersect(vs.text)
		var px []byte
		for y := int(rr.Y) + 1; y < int(rr.Y+rr.H)-1; y++ {
			o := img.PixOffset(int(rr.X), y)
			px = append(px, img.Pix[o:o+4*int(rr.W)]...)
		}
		return px
	}
	got := frameRect(fresh, fed, false)
	frameRect(ref, red, true)
	if want := frameRect(ref, red, true); !slices.Equal(got, want) {
		t.Fatal("first frame: the sticky header's pixels differ from whole-line colors")
	}
	t.Logf("%d frames compared, %d with sticky headers", compared, sticky)
}

// TestShortLineNextToLongLineCopies: typing in a short line whose
// brackets pair with brackets in a long line (the long line in view)
// copies the long line out of the rope no more: the bracket queries read
// the index's text (brIndex.text) and the layout's reads carry the line
// to each new version (lineMemo.carry).
func TestShortLineNextToLongLineCopies(t *testing.T) {
	goText(t)
	old := bracketFrameBudget
	bracketFrameBudget = time.Minute
	countWork = true
	defer func() { bracketFrameBudget, countWork = old, false }()
	long := strings.Repeat("g(x[1], {k: (v)}); ", 6000) // 114 KB, balanced
	text := "f(\n  a\n" + long + "),\n[\n  b\n" + long + "]\n"
	st := newTestState(text, "a.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	retryDegradedAtOnce(st)
	ed := newEditDriver(st, 700, 420, pinMac)
	ed.focus(t)
	st.MoveCaret(st.Snapshot().LineStart(1)+3, false) // inside f( … ), whose ")" ends line 2
	st.RevealCaret()
	key := func() {
		ed.Inject(input.TextInput{Text: "x"})
		ed.Frame()
	}
	ed.Frame()
	key()
	key()
	// The bracket tree needs the long lines' tokens final (settleTokens),
	// before the counted keys as after them.
	settleTokens(t, st)
	ed.Frame()
	resetWork()
	const keys = 6
	for range keys {
		key()
	}
	settleTokens(t, st)
	if p, ok := st.enclosingBrackets(st.Primary().Head, far); !ok || st.Snapshot().LineOf(p.close.a) != 2 {
		t.Fatalf("caret's enclosing pair %+v %v, want one closing in line 2", p, ok)
	}
	if copies := workLineCopies.Load(); copies > 0 {
		t.Errorf("long lines copied %d times over %d keys in a short line", copies, keys)
	}
}

// settleTries bounds the Highlight calls of settleTokens: each retries the
// lines Monarch degraded once (retryDegradedAtOnce).
const settleTries = 100

// retryDegradedAtOnce makes st's Highlighter retry a Degraded line at its
// next call, settleTries times before accepting it. Monarch's line budget
// is wall time (monarch.LineBudget): on a loaded machine a long line can
// overrun it and stay not final until the retry RetryDelay later.
func retryDegradedAtOnce(st *State) {
	if hl := st.Highlighter(); hl != nil {
		hl.RetryDelay, hl.MaxRetries = 0, settleTries
	}
}

// settleTokens tokenizes st's text until every line is final (Done).
func settleTokens(t *testing.T, st *State) {
	t.Helper()
	hl := st.Highlighter()
	if hl == nil {
		return
	}
	snap := st.Snapshot()
	for i := 0; !hl.Done(snap); i++ {
		if i == settleTries {
			t.Fatalf("tokens not final after %d Highlight calls", i)
		}
		hl.Highlight(snap, snap.LineCount(), time.Minute)
	}
}

// TestLineMemoRetention: the memo keeps the lines of the newest two
// versions only, at most memoMaxBytes in all.
func TestLineMemoRetention(t *testing.T) {
	b := buffer.New(strings.Repeat("a", 40<<10) + "\n" + strings.Repeat("b", 40<<10) + "\n" + strings.Repeat("c", 40<<10))
	var m lineMemo
	for i := range 6 {
		snap := b.Snapshot()
		for l := range 3 {
			if got := m.line(b, snap, l); got != snap.Line(l) {
				t.Fatalf("version %d line %d: wrong text", i, l)
			}
		}
		if _, err := b.Edit([]buffer.Edit{{Start: 5, End: 5, Text: "z"}}, nil, nil); err != nil {
			t.Fatal(err)
		}
		vers := map[uint64]bool{}
		total := 0
		for _, e := range m.ent {
			if e.s != "" {
				vers[e.ver] = true
				total += len(e.s)
			}
		}
		if len(vers) > 2 || total > memoMaxBytes {
			t.Fatalf("after version %d: %d versions, %d bytes kept", i, len(vers), total)
		}
	}
	// A line the last change did not touch is carried, not copied.
	countWork = true
	defer func() { countWork = false }()
	snap := b.Snapshot()
	m.line(b, snap, 2)
	resetWork()
	b.Edit([]buffer.Edit{{Start: 7, End: 7, Text: "q"}}, nil, nil)
	if got := m.line(b, b.Snapshot(), 2); got != b.Snapshot().Line(2) || workLineCopies.Load() != 0 {
		t.Fatalf("an untouched line was copied (%d) or is wrong", workLineCopies.Load())
	}
	// A line the change touched is copied.
	b.Edit([]buffer.Edit{{Start: b.Snapshot().LineStart(2) + 3, End: b.Snapshot().LineStart(2) + 3, Text: "w"}}, nil, nil)
	if got := m.line(b, b.Snapshot(), 2); got != b.Snapshot().Line(2) || workLineCopies.Load() != 1 {
		t.Fatal("an edited line was not read again")
	}
}
