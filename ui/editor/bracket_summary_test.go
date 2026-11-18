package editor

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

// treeLines returns every line's start stack and lowest depth of the
// Model's bracket tree, built to the end.
func treeLines(t *testing.T, st *State) ([]*bnode, []int32) {
	t.Helper()
	snap := st.Snapshot()
	st.Model.beginBracketFrame()
	tr := st.bracketTreeFor(snap.LineCount()-1, time.Now().Add(time.Minute))
	if tr == nil || tr.known() < snap.LineCount() {
		t.Fatal("tree not built")
	}
	return append([]*bnode(nil), tr.start...), append([]int32(nil), tr.minD...)
}

// TestBracketTreeFromSummaries: the tree built from the Highlighter's line
// summaries (no tokenizing) has the stacks and depths of the tree built
// from the lines' tokens, over edits of every kind, and builds without
// tokenizing lines outside the token windows.
func TestBracketTreeFromSummaries(t *testing.T) {
	goText(t)
	var doc strings.Builder
	doc.WriteString("package p\n\nfunc f() {\n")
	for i := range 600 {
		switch i % 7 {
		case 0:
			doc.WriteString("\tif g(a[1], \"(\") {\n")
		case 3:
			doc.WriteString("\t} // ) ] }\n")
		case 5:
			doc.WriteString("\ts := `[{(` + x[(1)]\n")
		default:
			doc.WriteString("\tv := m{k: h(a[2])} /* ( */\n")
		}
	}
	doc.WriteString("}\n")
	st := NewState(doc.String(), "a.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	hl.TokenWindow = 5
	countWork = true
	defer func() { countWork = false }()
	r := rand.New(rand.NewPCG(4, 4))
	inserts := []string{"(", ")", "{", "}", "[", "]", "\"", "`", "//", "/*", "*/", "\n", "x"}
	for step := range 40 {
		if step > 0 {
			snap := st.Snapshot()
			l := 3 + r.IntN(snap.LineCount()-4)
			at := snap.LineStart(l) + r.IntN(len(snap.Line(l))+1)
			if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: inserts[r.IntN(len(inserts))]}}, Selection{}); err != nil {
				continue
			}
		}
		snap := st.Snapshot()
		// The tree from tokens: no summaries.
		hl.SetLineSummary(nil, nil)
		hl.HighlightFor(st.hlKey, snap, snap.LineCount(), time.Minute)
		st.Model.brk = nil
		st.Model.brGrammar = ^uint64(0) // brackets makes a new tree
		ws, wd := treeLines(t, st)
		// The tree from summaries: every line tokenized again with the
		// summary function set (the Model sets it with its tree).
		hl.Reset(hl.LexerIdentity())
		hl.HighlightFor(st.hlKey, snap, snap.LineCount(), time.Minute)
		st.Model.brk = nil
		resetWork()
		gs, gd := treeLines(t, st)
		if n := workBracketToks.Load(); n > 0 {
			t.Fatalf("step %d: the tree tokenized %d lines, want the summaries", step, n)
		}
		for i := range ws {
			if !stackEqual(gs[i], ws[i]) || gd[i] != wd[i] {
				t.Fatalf("step %d line %d: stack/depth from summaries differ from tokens", step, i)
			}
		}
	}
}

func TestBracketTreeIgnoresForeignLineSummary(t *testing.T) {
	st := NewState("package p\nvar x = (\n  1\n)\n", "main.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	want, _ := treeLines(t, st)
	hl.SetLineSummary("external", func(_ string, _ []monarch.Token, dst []uint64) []uint64 { return dst })
	off := st.Snapshot().LineStart(1)
	if err := st.Edit([]buffer.Edit{{Start: off, End: off, Text: " "}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	got, _ := treeLines(t, st)
	if !stackEqual(got[2], want[2]) || stackDepth(got[2]) != 1 {
		t.Fatal("foreign summary hid the opening bracket")
	}
}

func TestBracketSummariesFollowDocumentSizeWithoutTreeQuery(t *testing.T) {
	st := NewState("var x = (1)\n", "main.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	key := bracketConfOf(st.Model.Conf())
	hasSummary := func() bool {
		snap := st.Snapshot()
		hl.Highlight(snap, 1, time.Second)
		_, ok := hl.LineSummary(key, snap, 0, nil)
		return ok
	}
	if !hasSummary() {
		t.Fatal("missing summary below the size limit")
	}
	end := st.Snapshot().Len()
	if err := st.Edit([]buffer.Edit{{Start: end, End: end, Text: strings.Repeat(" ", maxBracketTreeLen)}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	if hasSummary() {
		t.Fatal("retained bracket summaries above the size limit")
	}
	if !st.Undo() {
		t.Fatal("undo failed")
	}
	if err := st.Edit([]buffer.Edit{{Start: 0, End: 0, Text: " "}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	if !hasSummary() {
		t.Fatal("did not resume summarizing below the size limit")
	}
}
