package editor

import (
	"fmt"
	"image/color"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

// highlighted returns a State for text in a file named name, fully
// tokenized.
func highlighted(t *testing.T, text, name string, th *theme.Theme) *State {
	t.Helper()
	st := NewState(text, name, th)
	if hl := st.Highlighter(); hl != nil {
		hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Minute)
	}
	return st
}

var far = time.Now().Add(time.Hour)

// spanAt returns the color of the colored bracket at byte b of line i.
func spanAt(spans map[int][]brSpan, i, b int) (color.NRGBA, bool) {
	for _, s := range spans[i] {
		if s.b0 == b {
			return s.c, true
		}
	}
	return color.NRGBA{}, false
}

func TestBracketPairColors(t *testing.T) {
	th := theme.Base("vs-dark")
	text := "package p\n" +
		"var x = (((((((1)))))))\n" + // 7 levels
		"var s = \"(([\" // ({\n" + // in a string and a comment: not brackets
		"var y = f(a[1)\n" + // "]" never comes: "[" stays unclosed
		"}\n" + // unexpected
		"func g() {\n" // never closed
	st := highlighted(t, text, "a.go", th)
	tr := st.Model.brackets()
	if tr == nil {
		t.Fatal("no bracket tree")
	}
	tr.ensure(modelBrackets{st.Model}, 100, time.Time{})
	pal := paletteOf(th)
	if len(pal.levels) != 3 {
		t.Fatalf("vs-dark palette has %d levels, want 3 (foreground4-6 are transparent)", len(pal.levels))
	}
	spans, sure := st.bracketColors(tr, []int{0, 1, 2, 3, 4, 5}, pal, far, nil)
	if !sure {
		t.Error("colors not sure with every line known")
	}
	line := st.Snapshot().Line(1)
	open := strings.Index(line, "(")
	close := strings.Index(line, ")")
	for lvl := range 7 {
		want := pal.levels[lvl%3]
		if c, ok := spanAt(spans, 1, open+lvl); !ok || c != want {
			t.Errorf("opening bracket at level %d: %v %v, want %v", lvl, c, ok, want)
		}
		if c, ok := spanAt(spans, 1, close+6-lvl); !ok || c != want {
			t.Errorf("closing bracket at level %d: %v %v, want %v", lvl, c, ok, want)
		}
	}
	if s := spans[2]; len(s) != 0 {
		t.Errorf("brackets in a string or comment colored: %v", s)
	}
	l3 := st.Snapshot().Line(3)
	if c, _ := spanAt(spans, 3, strings.Index(l3, "[")); c != pal.invalid {
		t.Errorf("unclosed [ = %v, want the unexpected color %v", c, pal.invalid)
	}
	if c, _ := spanAt(spans, 3, strings.Index(l3, "(")); c != pal.levels[0] {
		t.Errorf("( closed past the unclosed [ = %v, want level 1", c)
	}
	if c, _ := spanAt(spans, 4, 0); c != pal.invalid {
		t.Errorf("unexpected } = %v, want %v", c, pal.invalid)
	}
	if c, _ := spanAt(spans, 5, strings.Index(st.Snapshot().Line(5), "{")); c != pal.invalid {
		t.Errorf("{ never closed = %v, want %v", c, pal.invalid)
	}
	if pal.invalid != (color.NRGBA{0xff, 0x12, 0x12, 0xcc}) {
		t.Errorf("unexpected bracket color %v", pal.invalid)
	}

	// colorizedBracketPairs: plain text has brackets but colorizes none.
	pt := NewState("((x))", "a.unknownext", th)
	if bc := bracketConfOf(pt.Model.Conf()); len(bc.defs) == 0 || bc.colorized[0] {
		t.Errorf("plain text brackets: %d defs, colorized %v", len(bc.defs), bc.colorized)
	}
	ptr := pt.Model.brackets()
	ptr.ensure(modelBrackets{pt.Model}, 1, time.Time{})
	if sp, _ := pt.bracketColors(ptr, []int{0}, pal, far, nil); len(sp) != 0 {
		t.Errorf("plain text colored: %v", sp)
	}
}

// plainSource is a bracketSource without tokens.
type plainSource struct{}

func (plainSource) tokens(buffer.Snapshot, int, string, []monarch.Token) []monarch.Token { return nil }
func (plainSource) finalLines(s buffer.Snapshot) int                                     { return s.LineCount() }
func (plainSource) startState(buffer.Snapshot, int) monarch.State                        { return 0 }
func (plainSource) advance(buffer.Snapshot, int, time.Time)                              {}

func sameTree(t *testing.T, got, want *bracketTree, upTo int) {
	t.Helper()
	for i := 0; i < upTo; i++ {
		if !stackEqual(got.start[i], want.start[i]) || got.minD[i] != want.minD[i] {
			t.Fatalf("line %d: start depth %d minD %d, full computation %d %d", i,
				stackDepth(got.start[i]), got.minD[i], stackDepth(want.start[i]), want.minD[i])
		}
	}
}

func TestBracketDepthIncremental(t *testing.T) {
	const n = 1_000_000
	var sb strings.Builder
	for i := range n {
		switch i % 4 {
		case 0:
			sb.WriteString("f(a, b) {\n")
		case 3:
			sb.WriteString("}\n")
		default:
			sb.WriteString("  x[1]\n")
		}
	}
	snap := buffer.New(sb.String()).Snapshot()
	conf := bracketConfOf(nil)
	tr := newBracketTree(conf, snap)
	if !tr.ensure(plainSource{}, snap.LineCount(), time.Time{}) || !tr.complete() {
		t.Fatal("full computation incomplete")
	}

	// Insert "{" at the start of line 10: every later line's depth grows.
	ns, err := snap.Apply([]buffer.Edit{{Start: snap.LineStart(10), End: snap.LineStart(10), Text: "{"}})
	if err != nil {
		t.Fatal(err)
	}
	tr.edit(snap, ns, []buffer.Edit{{Start: snap.LineStart(10), End: snap.LineStart(10), Text: "{"}})
	tr.scanned = 0
	tr.ensure(plainSource{}, 60, time.Time{})
	if tr.scanned > 60 {
		t.Errorf("scanned %d lines to know the first 60", tr.scanned)
	}
	full := newBracketTree(conf, ns)
	full.ensure(plainSource{}, ns.LineCount(), time.Time{})
	sameTree(t, tr, full, 60)
	tr.ensure(plainSource{}, ns.LineCount(), time.Time{})
	sameTree(t, tr, full, ns.LineCount())

	// A balanced edit stops at once.
	ns2, _ := ns.Apply([]buffer.Edit{{Start: ns.LineStart(500), End: ns.LineStart(500), Text: "zz"}})
	tr.edit(ns, ns2, []buffer.Edit{{Start: ns.LineStart(500), End: ns.LineStart(500), Text: "zz"}})
	tr.scanned = 0
	tr.ensure(plainSource{}, ns2.LineCount(), time.Time{})
	if tr.scanned > 2 || !tr.complete() {
		t.Errorf("a balanced edit rescanned %d lines", tr.scanned)
	}

	// Inserting and removing lines (the arrays are rebuilt).
	e := []buffer.Edit{{Start: ns2.LineStart(20), End: ns2.LineStart(24), Text: "g() {\n{\n\n"}}
	ns3, _ := ns2.Apply(e)
	tr.edit(ns2, ns3, e)
	tr.ensure(plainSource{}, ns3.LineCount(), time.Time{})
	full = newBracketTree(conf, ns3)
	full.ensure(plainSource{}, ns3.LineCount(), time.Time{})
	if len(tr.start) != ns3.LineCount() {
		t.Fatalf("%d lines, want %d", len(tr.start), ns3.LineCount())
	}
	sameTree(t, tr, full, ns3.LineCount())
}

func TestMatchBrackets(t *testing.T) {
	text := "func f() {\n" + //  0
		"\tif (a) {\n" + //       1
		"\t\tx := \"}\"\n" + //   2: a bracket in a string
		"\t\tx = y\n" + //        3
		"\t}\n" + //              4
		"}\n" //                  5
	st := highlighted(t, text, "a.go", theme.Base("vs"))
	snap := st.Snapshot()
	at := func(line int, sub string) int { return snap.LineStart(line) + strings.Index(snap.Line(line), sub) }
	open0, close5 := at(0, "{"), at(5, "}")
	open1, close4 := at(1, "{"), at(4, "}")

	for _, legacy := range []bool{false, true} {
		match := st.matchBracket
		encl := st.enclosingBrackets
		if legacy {
			match, encl = st.legacyMatch, st.legacyEnclosing
		}
		// After "{": the pair of line 0.
		if p, ok := match(open0+1, far); !ok || p.open.a != open0 || p.close.a != close5 {
			t.Errorf("legacy %v: match after { = %+v %v", legacy, p, ok)
		}
		// Before "}" of line 4.
		if p, ok := match(close4, far); !ok || p.open.a != open1 || p.close.a != close4 {
			t.Errorf("legacy %v: match before } = %+v %v", legacy, p, ok)
		}
		// Inside: no bracket touches the caret; the enclosing pair.
		x := at(3, "x")
		if _, ok := match(x, far); ok {
			t.Errorf("legacy %v: match inside text", legacy)
		}
		if p, ok := encl(x, far); !ok || p.open.a != open1 || p.close.a != close4 {
			t.Errorf("legacy %v: enclosing = %+v %v", legacy, p, ok)
		}
	}

	// Highlighting: "near" only when touching, "always" also enclosing.
	x := at(3, "x")
	st.MoveCaret(x, false)
	if got := st.matchedBrackets(MatchBracketsNear, far); len(got) != 0 {
		t.Errorf("near inside text: %v", got)
	}
	if got := st.matchedBrackets(MatchBracketsAlways, far); len(got) != 2 || got[0].a != open1 || got[1].a != close4 {
		t.Errorf("always inside text: %v", got)
	}
	st.SetSelections([]Selection{{Anchor: x, Head: x + 1}})
	if got := st.matchedBrackets(MatchBracketsAlways, far); len(got) != 0 {
		t.Errorf("a selection matched brackets: %v", got)
	}

	// jumpToBracket: from inside to the enclosing closing bracket, then
	// back to its opening bracket.
	st.MoveCaret(x, false)
	if !st.Execute("editor.action.jumpToBracket", nil) {
		t.Fatal("jumpToBracket did not run")
	}
	if h := st.Primary().Head; h != close4 {
		t.Fatalf("jump from inside: %d, want %d", h, close4)
	}
	st.Execute("editor.action.jumpToBracket", nil)
	if h := st.Primary().Head; h != open1 {
		t.Fatalf("jump back: %d, want %d", h, open1)
	}
	// selectToBracket: with and without the brackets.
	st.MoveCaret(x, false)
	st.Execute("editor.action.selectToBracket", nil)
	if s := st.Primary(); s.Anchor != open1 || s.Head != close4+1 {
		t.Errorf("selectToBracket = %+v, want %d..%d", s, open1, close4+1)
	}
	st.MoveCaret(close4, false)
	st.Execute("editor.action.selectToBracket", map[string]any{"selectBrackets": false})
	if s := st.Primary(); s.Anchor != close4 || s.Head != open1+1 {
		t.Errorf("selectToBracket from the closing bracket = %+v, want %d..%d", s, close4, open1+1)
	}
}

// TestBracketTokenChanges: an edit that changes the tokens of the lines
// after it (an opened comment) without changing their text rescans them
// (Monaco's handleDidChangeTokens).
func TestBracketTokenChanges(t *testing.T) {
	text := "func f() {\n" + // 0
		"\tx := 1\n" + //       1
		"\tif x {\n" + //       2
		"\t\ty() */ + 1\n" + // 3
		"\t}\n" + //            4
		"}\n" //                5
	th := theme.Base("vs-dark")
	st := highlighted(t, text, "a.go", th)
	src := modelBrackets{st.Model}
	tr := st.Model.brackets()
	tr.ensure(src, 100, time.Time{})
	// On a line without brackets: its end stack does not change.
	st.MoveCaret(st.Snapshot().LineStart(1), false)
	st.Insert("/*")
	snap := st.Snapshot()
	st.Highlighter().Highlight(snap, snap.LineCount(), time.Minute)
	tr = st.Model.brackets()
	tr.ensure(src, snap.LineCount(), time.Time{})
	full := newBracketTree(tr.conf, snap)
	full.ensure(src, snap.LineCount(), time.Time{})
	sameTree(t, tr, full, snap.LineCount())
	if d := stackDepth(tr.start[4]); d != 1 {
		t.Errorf("line 5 starts at depth %d, want 1 (the if's { is commented out)", d)
	}
	pal := paletteOf(th)
	spans, _ := st.bracketColors(tr, []int{4, 5}, pal, far, nil)
	if c, _ := spanAt(spans, 4, 1); c != pal.levels[0] {
		t.Errorf("line 5's } = %v, want level 1 %v", c, pal.levels[0])
	}
	if c, _ := spanAt(spans, 5, 0); c != pal.invalid {
		t.Errorf("line 6's } = %v, want unexpected %v", c, pal.invalid)
	}
	if p, ok := st.matchBracket(snap.LineStart(5), far); ok {
		t.Errorf("line 6's } matched %+v", p)
	}
}

// TestBracketCommandsTokenize: without a background Highlighter, the
// bracket commands tokenize the lines they need themselves.
func TestBracketCommandsTokenize(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("func f() {\n")
	for i := range 300 {
		fmt.Fprintf(&sb, "\tx%d := \"}\" // }\n", i)
	}
	sb.WriteString("}\n")
	st := NewState(sb.String(), "a.go", theme.Base("vs")) // nothing tokenized
	snap := st.Snapshot()
	open := strings.Index(snap.Line(0), "{")
	st.MoveCaret(open, false)
	st.Execute("editor.action.jumpToBracket", nil)
	if h, want := st.Primary().Head, snap.LineStart(301); h != want {
		t.Fatalf("jumpToBracket: caret %d, want %d (line 302's })", h, want)
	}
	st.MoveCaret(snap.LineStart(150)+1, false)
	st.Execute("editor.action.selectToBracket", nil)
	if s := st.Primary(); s.Anchor != open || s.Head != snap.LineStart(301)+1 {
		t.Errorf("selectToBracket = %+v", s)
	}
}

// TestMatchBracketsFarNoBackground: with no background Highlighter, the
// View's match of a bracket whose pair is far below keeps requesting
// frames (the query tokenizes the lines itself) until it is found.
func TestMatchBracketsFarNoBackground(t *testing.T) {
	goText(t)
	var sb strings.Builder
	sb.WriteString("func f() {\n")
	for i := range 20000 {
		fmt.Fprintf(&sb, "\tx%d := g(%d)\n", i, i)
	}
	sb.WriteString("}\n")
	st := NewState(sb.String(), "a.go", theme.Base("vs"))
	// A small frame budget, often spent at the moment the tree has caught
	// up with the tokenized lines: the query still tokenizes further and
	// asks for the next frame.
	defer func(b time.Duration) { bracketFrameBudget = b }(bracketFrameBudget)
	bracketFrameBudget = 300 * time.Microsecond
	if raceEnabled {
		bracketFrameBudget *= 10 // the same work per frame, 10× slower
	}
	d, vw := viewDriver(st, 400, 300, nil)
	d.Frame()
	d.Click(geom.Pt(300, 250)) // focus
	st.MoveCaret(strings.Index(sb.String(), "{"), false)
	close := st.Snapshot().LineStart(20001)
	for frames := 1; ; frames++ {
		d.Frame()
		if m := (*vw).vs.match; len(m) == 2 {
			if m[1].a != close {
				t.Fatalf("matched %v, want the } at %d", m, close)
			}
			t.Logf("matched after %d frames", frames)
			break
		}
		if !d.NeedsFrame() {
			t.Fatalf("frame %d: no match and no frame requested", frames)
		}
		if frames > maxMatchFrames {
			t.Fatal("no match within maxMatchFrames frames")
		}
	}
}

// TestBracketColorsLanguageSwitch: the colors cached for a text follow a
// language change (the snapshot is the same).
func TestBracketColorsLanguageSwitch(t *testing.T) {
	goText(t)
	st := NewState("x := ((1))\n", "a.go", theme.Base("vs"))
	d, vw := viewDriver(st, 400, 100, nil)
	d.Frame()
	d.Frame()
	if len((*vw).vs.brk[0]) == 0 {
		t.Fatal("go: brackets not colored")
	}
	st.Model.SetLanguage(PlainText)
	d.Frame()
	if sp := (*vw).vs.brk[0]; len(sp) != 0 {
		t.Errorf("plain text: brackets still colored %v", sp)
	}
}

// TestMatchManyCursorsBounded: carets far apart in a long function, with
// no background Highlighter: however many carets query, a frame's bracket
// work overruns its budget at most once (minOverrun), and in a document
// the frames can tokenize in time every caret gets matched, the undecided
// carets going first in the next frame.
//
// A frame's work is its thread's CPU time where there is a thread clock
// (threadCPUClock): its wall time also counts the time the OS runs other
// threads instead, tens of milliseconds on a loaded machine, though the
// work stopped at its deadline.
func TestMatchManyCursorsBounded(t *testing.T) {
	runtime.LockOSThread() // workClock is this thread's clock
	defer runtime.UnlockOSThread()
	for _, c := range []struct {
		lines, carets, frames int
		all                   bool
	}{{100_000, 100, 40, false}, {6_000, 30, maxMatchFrames, true}} {
		var sb strings.Builder
		sb.WriteString("func f() {\n")
		for i := range c.lines {
			fmt.Fprintf(&sb, "\tx%d := g(%d)\n", i, i)
		}
		sb.WriteString("}\n")
		st := NewState(sb.String(), "a.go", theme.Base("vs"))
		snap := st.Snapshot()
		var sels []Selection
		for k := range c.carets {
			off := snap.LineStart(1+k*(c.lines/c.carets)) + 2
			sels = append(sels, Selection{Anchor: off, Head: off})
		}
		st.SetSelections(sels)
		tr := st.Model.brackets()
		budget := bracketFrameBudget
		if raceEnabled {
			budget *= 10 // the same work per frame, 10× slower
		}
		// The budget, the work allowed past it, a slow line; relaxed under
		// the race detector and on shared CI runners (gateLimit), which are
		// slower and, without a thread clock, preempt the frame.
		limit := gateLimit(bracketFrameBudget + minOverrun + time.Millisecond)
		// Thread-clock platforms and Windows keep the original per-frame
		// thresholds. Windows uses wall time, so OS preemption still counts:
		// a single far overrun must fail, not disappear behind a median.
		// Other fallback platforms retain the median/far-overrun policy.
		hardLimit, overLimit := bracketFrameTimingLimits(runtime.GOOS, threadCPUClock, limit)
		over := 0
		from := 0
		var walls []time.Duration
		done := false
		res := make([]caretMatch, len(sels))
		for frame := 0; frame < c.frames && !done; frame++ {
			tr.beginFrame()
			tr.unsure = false
			start := time.Now()
			w0 := workClock()
			unsure := st.matchCarets(MatchBracketsAlways, start.Add(budget), from, res)
			work := workClock() - w0
			wall := time.Since(start)
			if runtime.GOOS == "windows" {
				work = wall // original start-to-return wall interval, not a CPU estimate
			}
			out := matchRanges(res)
			walls = append(walls, wall)
			if hardLimit > 0 && work > hardLimit {
				t.Fatalf("%d lines, %d carets, frame %d: bracket work %v (wall %v), want at most %v", c.lines, c.carets, frame, work, wall, limit)
			}
			if work > overLimit {
				over++ // a collection or a slow line; not frame after frame
				t.Logf("%d lines, %d carets, frame %d: bracket work %v (wall %v), want at most %v", c.lines, c.carets, frame, work, wall, limit)
			}
			if unsure < 0 {
				if len(out) != 2*len(sels) {
					t.Fatalf("frame %d: %d brackets matched, want %d", frame, len(out), 2*len(sels))
				}
				t.Logf("%d lines: all %d carets matched after %d frames", c.lines, c.carets, frame+1)
				done = true
			}
			from = max(unsure, 0)
		}
		if over > c.frames/20+1 {
			t.Errorf("%d lines: %d frames over %v", c.lines, over, overLimit)
		}
		slices.Sort(walls)
		if med := walls[len(walls)/2]; hardLimit == 0 && len(walls) >= 10 && med > limit { // a few frames have no stable median
			t.Errorf("%d lines: median frame %v, want at most %v", c.lines, med, limit)
		}
		if c.all && !done {
			t.Errorf("%d lines: carets not all matched within %d frames", c.lines, c.frames)
		}
	}
}

// TestMatchManyCursorsConverge: 100 carets in a 30 000-line function, each
// enclosed by the function's braces: once the tree is complete each caret
// still costs a scan, more than one frame's budget for all. The View keeps
// the carets decided in earlier frames, so the shown matches only grow,
// every caret is matched within a bounded number of frames and the frame
// requests then stop.
func TestMatchManyCursorsConverge(t *testing.T) {
	goText(t)
	const lines, carets = 30_000, 100
	var sb strings.Builder
	sb.WriteString("func f() {\n")
	for i := range lines {
		fmt.Fprintf(&sb, "\tx%d := g(%d)\n", i, i)
	}
	sb.WriteString("}\n")
	st := NewState(sb.String(), "a.go", theme.Base("vs"))
	if raceEnabled {
		defer func(b time.Duration) { bracketFrameBudget = b }(bracketFrameBudget)
		bracketFrameBudget *= 10 // the same work per frame, 10× slower
	}
	d, vw := viewDriver(st, 400, 300, nil)
	d.Frame()
	d.Click(geom.Pt(300, 250)) // focus
	snap := st.Snapshot()
	var sels []Selection
	for k := range carets {
		off := snap.LineStart(1+k*(lines/carets)) + 2
		sels = append(sels, Selection{Anchor: off, Head: off})
	}
	st.SetSelections(sels)
	open, close := strings.Index(sb.String(), "{"), snap.LineStart(lines+1)
	// About 110 frames on an idle machine: tokenizing for the tree, then
	// one for the carets. A frame's budget is wall time, so a loaded
	// machine (or -race) needs more frames; minOverrun makes every frame
	// progress, and the View gives up after maxMatchFrames.
	bound := maxMatchFrames
	shown := 0
	for frames := 1; ; frames++ {
		d.Frame()
		m := (*vw).vs.match
		if len(m) < shown {
			t.Fatalf("frame %d: %d brackets shown, %d the frame before", frames, len(m), shown)
		}
		shown = len(m)
		for k := 0; k < len(m); k += 2 {
			if m[k].a != open || m[k+1].a != close {
				t.Fatalf("frame %d: matched %v %v, want the function's braces", frames, m[k], m[k+1])
			}
		}
		if len(m) == 2*carets {
			t.Logf("all carets matched after %d frames", frames)
			break
		}
		if !d.NeedsFrame() {
			t.Fatalf("frame %d: %d of %d carets matched and no frame requested", frames, len(m)/2, carets)
		}
		if frames > bound {
			t.Fatalf("%d of %d carets matched after %d frames", len(m)/2, carets, frames)
		}
	}
	// Converged: the match no longer asks for frames.
	d.Frame()
	if d.NeedsFrame() {
		t.Error("frames still requested after every caret matched")
	}
	if len((*vw).vs.match) != 2*carets {
		t.Errorf("%d brackets shown after converging, want %d", len((*vw).vs.match), 2*carets)
	}
}
