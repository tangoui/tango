package highlight

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// lenSummary summarizes a line as its length and token count.
func lenSummary(text string, toks []monarch.Token, dst []uint64) []uint64 {
	return append(dst, uint64(len(text)), uint64(len(toks)))
}

func wantSummary(t *testing.T, lx *monarch.Lexer, h *Highlighter, s buffer.Snapshot, line int) {
	t.Helper()
	got, ok := h.LineSummary("len", s, line, nil)
	if !ok {
		t.Fatalf("line %d: no summary", line)
	}
	st, _ := h.StartState(s, line)
	toks, _, _ := lx.TokenizeLineInfo(s.Line(line), st, nil)
	if want := lenSummary(s.Line(line), toks, nil); !slices.Equal(got, want) {
		t.Fatalf("line %d: summary %v, want %v", line, got, want)
	}
}

// TestLineSummaryFollowsFinalLines: lines tokenized to their final tokens
// (in the foreground and by the background goroutine) carry their summary;
// an edited line has none until it is tokenized again; lines moved by an
// edit keep theirs; another summary function drops them all.
func TestLineSummaryFollowsFinalLines(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 400))
	h := New(lx)
	h.SetLineSummary("len", lenSummary)
	if _, ok := h.LineSummary("len", s, 10, nil); ok {
		t.Fatal("a summary before tokenizing")
	}
	highlightAll(t, h, s)
	for _, l := range []int{0, 10, 399} {
		wantSummary(t, lx, h, s, l)
	}
	// Edit line 100 in place: it loses its summary until tokenized.
	at := s.LineStart(100)
	edits := []buffer.Edit{{Start: at, End: at, Text: "q"}}
	ns, _ := s.Apply(edits)
	h.Edit(s, ns, edits)
	if _, ok := h.LineSummary("len", ns, 100, nil); ok {
		t.Fatal("the edited line kept its summary")
	}
	wantSummary(t, lx, h, ns, 99)
	highlightAll(t, h, ns)
	wantSummary(t, lx, h, ns, 100)
	// Open a block comment on line 120: the lines below are no longer
	// final (their start states change) and give no summary until tokenized.
	s = ns
	at = s.LineStart(120)
	edits = []buffer.Edit{{Start: at, End: at, Text: "/*"}}
	ns, _ = s.Apply(edits)
	h.Edit(s, ns, edits)
	h.Highlight(ns, 121, time.Hour) // line 120 only: its end state changed
	if _, ok := h.LineSummary("len", ns, 125, nil); ok {
		t.Fatal("a line below a new block comment kept its summary before it was tokenized")
	}
	highlightAll(t, h, ns)
	wantSummary(t, lx, h, ns, 125)
	s = ns
	at = s.LineStart(120)
	edits = []buffer.Edit{{Start: at, End: at + 2}}
	ns, _ = s.Apply(edits)
	h.Edit(s, ns, edits)
	highlightAll(t, h, ns)
	// Insert two lines above line 200: the lines below keep their
	// summaries at their new numbers.
	s = ns
	at = s.LineStart(200)
	edits = []buffer.Edit{{Start: at, End: at, Text: "let a = 1\nlet b = 2\n"}}
	ns, _ = s.Apply(edits)
	h.Edit(s, ns, edits)
	h.Highlight(ns, 203, time.Hour) // the inserted lines and the one after
	for _, l := range []int{201, 202, 300, 401} {
		wantSummary(t, lx, h, ns, l)
	}
	// Another function: none until tokenized again.
	h.SetLineSummary("other", lenSummary)
	if _, ok := h.LineSummary("len", ns, 300, nil); ok {
		t.Fatal("a summary of the previous function")
	}
	// The same key again keeps them.
	h.SetLineSummary("other", lenSummary)

	// The background goroutine summarizes too.
	h = New(lx)
	h.SetLineSummary("len", lenSummary)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan buffer.Snapshot, 1)
	ch <- ns
	done := make(chan struct{})
	h.Start(ctx, ch, func() {
		if h.Done(ns) {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("background highlighting not done")
	}
	for _, l := range []int{0, 250, 401} {
		wantSummary(t, lx, h, ns, l)
	}
}

func TestLineSummaryInvalidatedPastTokenCut(t *testing.T) {
	s := buffer.NewSnapshot(strings.Repeat(" ", cutSafe+10))
	h := New(tsLexer(t))
	h.SetLineSummary("len", lenSummary)
	highlightAll(t, h, s)
	if _, ok := h.LineSummary("len", s, 0, nil); !ok {
		t.Fatal("missing initial summary")
	}
	edit := buffer.Edit{Start: s.Len(), End: s.Len(), Text: "x"}
	ns, err := s.Apply([]buffer.Edit{edit})
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, []buffer.Edit{edit})
	if got, ok := h.LineSummary("len", ns, 0, nil); ok {
		t.Fatalf("edit beyond token cut retained stale summary %v", got)
	}
	if !h.FinalBefore(ns, 1) {
		t.Fatal("edit beyond token cut unnecessarily invalidated tokens")
	}
}
