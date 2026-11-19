package editor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// bracketCatchUpDoc is Go text with a bracket pair on every line.
func bracketCatchUpDoc(n int) string {
	var b strings.Builder
	b.WriteString("package p\n\nfunc f() {\n")
	for i := range n {
		b.WriteString("\tv := g(h(a[1]), m{k: 2}) // ")
		b.WriteString(strings.Repeat("x", i%11))
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// behindSource is the Model's bracket source whose background tokenizer
// is reported behind or not, counting the tree's own tokenizing.
type behindSource struct {
	modelBrackets
	behind   bool
	advanced int
}

func (s *behindSource) catchingUp(buffer.Snapshot, int) bool { return s.behind }

func (s *behindSource) advance(snap buffer.Snapshot, upTo int, deadline time.Time) {
	s.advanced++
	s.modelBrackets.advance(snap, upTo, deadline)
}

// TestFrameTreeLeavesTokenizingToBackground: lines not final below the
// known ones (a paste): while a background tokenizer is behind, a frame's
// queries do not tokenize to reach them (the tree stops at the first line
// not final); a bracket command, which has no later frame, does; without a
// background tokenizer the frame does too.
func TestFrameTreeLeavesTokenizingToBackground(t *testing.T) {
	goText(t)
	st := NewState(bracketCatchUpDoc(3000), "a.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	snap := st.Snapshot()
	hl.HighlightFor(st.hlKey, snap, snap.LineCount(), time.Minute)
	st.Model.beginBracketFrame()
	if st.bracketTreeFor(snap.LineCount()-1, time.Now().Add(time.Minute)) == nil {
		t.Fatal("tree not built")
	}
	reset := func() *bracketTree {
		// A paste at line 100: the lines from there on not final.
		snap := st.Snapshot()
		at := snap.LineStart(100)
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: strings.Repeat("\tq := r(s[0])\n", 200)}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		tr := st.Model.brackets()
		tr.tokenize = true
		return tr
	}
	far := time.Now().Add(time.Minute)
	line := 2000

	tr := reset()
	src := &behindSource{modelBrackets: modelBrackets{st.Model}, behind: true}
	st.Model.beginBracketFrame()
	if tr.command {
		t.Fatal("a frame's queries marked as a command's")
	}
	if tr.ensure(src, line, far) || src.advanced != 0 {
		t.Fatalf("a frame tokenized %d times to reach line %d while the background tokenizer is behind", src.advanced, line)
	}

	st.beginBracketCommand()
	if !tr.command {
		t.Fatal("a bracket command's queries not marked as such")
	}
	if !tr.ensure(src, line, far) || src.advanced == 0 {
		t.Fatal("a bracket command did not tokenize to reach its line")
	}

	tr = reset()
	src = &behindSource{modelBrackets: modelBrackets{st.Model}}
	st.Model.beginBracketFrame()
	if !tr.ensure(src, line, far) || src.advanced == 0 {
		t.Fatal("without a background tokenizer the frame did not tokenize to reach its line")
	}
}

// TestModelBracketsBehindFollowsHighlighter: the Model's bracket source
// reports the Highlighter's background goroutine behind exactly while one
// runs and the line asked for is not final.
func TestModelBracketsBehindFollowsHighlighter(t *testing.T) {
	goText(t)
	st := NewState(bracketCatchUpDoc(300), "a.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	snap := st.Snapshot()
	src := modelBrackets{st.Model}
	hl.Highlight(snap, 1, time.Hour)
	if src.catchingUp(snap, 200) {
		t.Fatal("behind without a goroutine")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hl.Start(ctx, make(chan buffer.Snapshot), func() {})
	if !src.catchingUp(snap, 200) {
		t.Fatal("not behind with the lines before 200 not final and a goroutine running")
	}
	hl.Highlight(snap, snap.LineCount(), time.Hour)
	if src.catchingUp(snap, 200) {
		t.Fatal("behind with every line final")
	}
}
