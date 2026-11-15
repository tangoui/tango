package highlight

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
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
	// Establish complete results before edits. Checks below also validate
	// after Tokens reads, which can re-tokenize missing window slots.
	clean := func() { cleanCutHighlight(t, h, s) }
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
	checkCutComplete(t, h, lx, s, "after injected degradation", h.Tokens)
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
			checkCutComplete(t, h, lx, s, fmt.Sprintf("after edit %d", n), h.Tokens)
		}
	}
	clean()
	checkCutComplete(t, h, lx, s, "end", h.Tokens)
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

// The previous checker cleaned the highlighter and then read Tokens. An
// evicted slot makes Tokens tokenize again: the read itself can introduce
// an accepted degraded result AFTER the clean check. Exercise that exact
// path deterministically with a timeout grammar on just the first read.
func TestLongCutComparisonRetriesTokenRead(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(strings.Repeat("a", 200) + "c\nx")
	h := New(lx)
	h.TokenWindow = 0
	h.MaxRetries = 0
	forced := grammarLexer(t, "cut-read-timeout", `{"tokenPostfix":"","tokenizer":{"root":[["(a+)+(?=b)","bad"],[".","other"]]}}`)
	injected := false
	reads := 0
	read := func(snap buffer.Snapshot, line int, dst []monarch.Token) ([]monarch.Token, bool) {
		reads++
		if injected || line != 0 {
			return h.Tokens(snap, line, dst)
		}
		injected = true
		h.DropView(DefaultView) // force the real TokensFor cache-miss path
		h.lx = forced
		toks, final := h.Tokens(snap, line, dst)
		h.lx = lx
		if !h.st.acceptedDegraded(line) || !final {
			t.Fatal("forced Tokens cache miss did not yield an accepted degraded result")
		}
		return toks, final
	}
	checkCutComplete(t, h, lx, s, "forced token-read degradation", read)
	if !injected || reads <= s.LineCount() {
		t.Fatalf("token-read degradation was not retried: injected=%v reads=%d", injected, reads)
	}
}

// cleanCutHighlight obtains complete state/token results, not merely final
// ones: MaxRetries=0 also accepts scheduler-degraded results as final.
// This is a test-only explicit retry of known degradation, never of an
// unexplained mismatch, and is called after the no-tokenization assertions.
func cleanCutHighlight(t testing.TB, h *Highlighter, s buffer.Snapshot) {
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
	t.Fatal("could not obtain non-degraded highlight results in 50 attempts")
}

// checkCutComplete compares only known-complete results on BOTH sides.
// The older fullTokenize helper silently accepts degradation at its retry
// limit; checkAllFinal can in turn degrade while reading an evicted slot.
// Neither behavior is suitable for this truncation differential test.
func checkCutComplete(t testing.TB, h *Highlighter, lx *monarch.Lexer, s buffer.Snapshot, ctx string, read func(buffer.Snapshot, int, []monarch.Token) ([]monarch.Token, bool)) {
	t.Helper()
	want := make([][]monarch.Token, s.LineCount())
	state := lx.Initial()
	for i := range want {
		line := s.Line(i)
		for try := 0; ; try++ {
			if try == 50 {
				t.Fatalf("%s: reference line %d (%d bytes) degraded in all 50 attempts", ctx, i, len(line))
			}
			toks, end, info := lx.TokenizeLineInfo(line, state, nil)
			if !info.Degraded {
				want[i], state = toks, end
				break
			}
		}
	}
	for try := 0; try < 50; try++ {
		cleanCutHighlight(t, h, s)
		retry := false
		for i := range want {
			got, final := read(s, i, nil)
			// Check AFTER the read: Tokens may have just re-tokenized
			// a missing slot and accepted a scheduler-degraded prefix.
			h.mu.Lock()
			degraded := len(h.st.deg) != 0
			h.mu.Unlock()
			if degraded {
				retry = true
				break
			}
			if !final {
				t.Fatalf("%s: line %d not final without degradation", ctx, i)
			}
			if !slices.Equal(got, want[i]) {
				k := 0
				for k < min(len(got), len(want[i])) && got[k] == want[i][k] {
					k++
				}
				var g, w any = "<end>", "<end>"
				if k < len(got) {
					g = got[k]
				}
				if k < len(want[i]) {
					w = want[i][k]
				}
				t.Fatalf("%s: line %d (%d bytes), token counts got=%d want=%d; first mismatch %d: got=%+v want=%+v", ctx, i, lineLen(s, i), len(got), len(want[i]), k, g, w)
			}
		}
		if !retry {
			return
		}
	}
	t.Fatalf("%s: Tokens reads degraded in all 50 attempts", ctx)
}
