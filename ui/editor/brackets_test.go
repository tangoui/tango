package editor

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
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
	spans, sure := st.bracketColors(tr, []int{0, 1, 2, 3, 4, 5}, pal, far)
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
	if sp, _ := pt.bracketColors(ptr, []int{0}, pal, far); len(sp) != 0 {
		t.Errorf("plain text colored: %v", sp)
	}
}

// plainSource is a bracketSource without tokens.
type plainSource struct{}

func (plainSource) tokens(buffer.Snapshot, int, string, []monarch.Token) []monarch.Token { return nil }
func (plainSource) finalLines(s buffer.Snapshot) int                                     { return s.LineCount() }

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
