package highlight

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// An edit of a long line after the part the lexer reads (cutSafe) leaves
// the line's tokens and end state as they were: Edit does not make the
// line dirty, nothing is tokenized again, and every line's tokens still
// equal a full tokenization of the new text. Edits before cutSafe (and
// around the real cut) re-tokenize as before.
func TestEditPastTokenizationCut(t *testing.T) {
	lx := tsLexer(t)
	rng := rand.New(rand.NewPCG(7, 11))
	var sb strings.Builder
	sb.WriteString("const a = `x${1}`;\n")
	for i := 0; sb.Len() < 150_000; i++ {
		switch i % 7 {
		case 0:
			sb.WriteString("/* 中文注释😀 */ ")
		case 3:
			sb.WriteString("let s = \"str\"; ")
		default:
			sb.WriteString("f(x[1], {k: (v)}); ")
		}
	}
	sb.WriteString("\n/* open\nconst b = 2;\n")
	s := buffer.NewSnapshot(sb.String())
	h := New(lx)
	h.MaxRetries = 0
	// This test compares complete tokens, not accepted timeout previews.
	// MaxRetries=0 makes a scheduler-degraded line final too; unlike the
	// reference's fullTokenize, Highlight will not retry that line. Obtain
	// a clean result explicitly before edits/comparisons. Crucially, this
	// runs AFTER the no-tokenization assertion for an edit past the cut.
	clean := func() {
		t.Helper()
		for try := 0; try < 50; try++ {
			highlightAll(t, h, s)
			h.mu.Lock()
			if len(h.st.deg) == 0 {
				h.mu.Unlock()
				return
			}
			for _, d := range h.st.deg {
				h.st.markDirty(d.line)
			}
			h.st.deg = h.st.deg[:0]
			h.mu.Unlock()
		}
		t.Fatal("could not obtain non-degraded tokens in 50 attempts")
	}
	clean()
	// Deterministically exercise the accepted-degraded case that caused
	// the contention failure: final does not mean fully tokenized.
	h.mu.Lock()
	h.apply(1, h.startState(1), h.startState(1), true, nil, true, time.Now())
	h.mu.Unlock()
	highlightAll(t, h, s) // degradation also dirties the following line
	if !h.FinalBefore(s, s.LineCount()) {
		t.Fatal("MaxRetries=0 did not accept the injected degradation")
	}
	clean()
	checkAllFinal(t, h, lx, s, "after injected degradation")
	inserts := []string{"x", "/*", "*/", "`", "\"", "中", "😀", "é", ""}
	past := 0
	for n := range 300 {
		ls := s.LineStart(1)
		ll := lineLen(s, 1)
		var off int
		switch rng.IntN(3) {
		case 0: // around the real cut and cutSafe
			off = 19_000 + rng.IntN(cutSafe-19_000+2_000)
		case 1:
			off = rng.IntN(ll + 1)
		default:
			off = cutSafe + rng.IntN(ll-cutSafe+1)
		}
		start := ls + min(off, ll)
		end := start
		if rng.IntN(3) == 0 {
			end = min(ls+ll, start+rng.IntN(40))
		}
		txt := inserts[rng.IntN(len(inserts))]
		if txt == "" && end == start {
			txt = "y"
		}
		e := buffer.Edit{Start: start, End: end, Text: txt}
		ns, err := s.Apply([]buffer.Edit{e})
		if err != nil {
			continue // inside a multi-byte character
		}
		isPast := start-ls >= cutSafe
		h.Edit(s, ns, []buffer.Edit{e})
		s = ns
		n0 := h.tokenized.Load()
		if isPast {
			past++
			if !h.FinalBefore(s, s.LineCount()) {
				t.Fatalf("edit %d %+v at %d of the line (past the cut): the document is not final", n, e, start-ls)
			}
		}
		highlightAll(t, h, s)
		if d := h.tokenized.Load() - n0; isPast && d != 0 {
			t.Fatalf("edit %d past the cut: %d lines tokenized again", n, d)
		}
		if n%10 == 9 || !isPast {
			clean()
			checkAllFinal(t, h, lx, s, "after edit")
		}
	}
	clean()
	checkAllFinal(t, h, lx, s, "end")
	if past < 50 {
		t.Fatalf("only %d edits past the cut", past)
	}
}

func lineLen(s buffer.Snapshot, i int) int {
	end := s.Len()
	if i+1 < s.LineCount() {
		end = s.LineStart(i+1) - 1
	}
	return end - s.LineStart(i)
}
