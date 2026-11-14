package highlight

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// The caller's 1ms budget is checked between lines. An individual short line
// with repeated sub-timeout backtracking must still have its own protection.
func TestShortBacktrackingHighlightBounded(t *testing.T) {
	lx := grammarLexer(t, "short-backtracking", `{"tokenPostfix":"","tokenizer":{"root":[["(a+)+(?=b)","a"],[".","c"]]}}`)
	s := buffer.NewSnapshot(strings.Repeat(strings.Repeat("a", 19)+"c", 20))
	h := New(lx)
	start := time.Now()
	if h.Highlight(s, 1, time.Millisecond) {
		t.Fatal("expensive line unexpectedly final")
	}
	if elapsed := time.Since(start); !raceEnabled && elapsed > 500*time.Millisecond {
		t.Fatalf("1ms Highlight blocked for %v", elapsed)
	}
	if h.st.minPending() != 0 {
		t.Fatal("expensive short line was not degraded")
	}
}

// Inject the result of a transient timeout, not an actual slow regex. A clean
// retry must remove minPending even when it is the last allowed retry and even
// when its end state is unchanged. No sleeps or background goroutines are used.
func TestCleanRetryClearsTransientDegradation(t *testing.T) {
	lx := grammarLexer(t, "pause", `{"tokenPostfix":"","tokenizer":{"root":[["x","x"],["y","y"]]}}`)
	s := buffer.NewSnapshot("xy\nyx")
	h := New(lx)
	h.RetryDelay = 0
	h.MaxRetries = 1
	highlightAll(t, h, s)
	h.apply(0, lx.Initial(), lx.Initial(), true, []monarch.Token{{}}, true, time.Time{})
	if h.Done(s) || h.FinalBefore(s, 1) || h.st.minPending() != 0 {
		t.Fatal("injected degraded line was final")
	}
	if !h.Highlight(s, s.LineCount(), time.Hour) || !h.Done(s) || len(h.st.deg) != 0 {
		t.Fatal("clean retry left degradation pending or accepted")
	}
	for i := range s.LineCount() {
		got, final := h.Tokens(s, i, nil)
		want, _ := lx.TokenizeLine(s.Line(i), lx.Initial(), nil)
		if !final || !slices.Equal(got, want) {
			t.Fatalf("line %d final=%v got=%+v want=%+v", i, final, got, want)
		}
	}
}
