package highlight

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// TestTokenizeSpan: a span of a line is tokenized from the stored start
// state of the line (here inside a block comment), with the current lexer.
func TestTokenizeSpan(t *testing.T) {
	lx := tsLexer(t)
	b := buffer.New("/* open\n" + strings.Repeat("x ", 50) + "\n*/ let y = 1\n")
	snap := b.Snapshot()
	h := New(lx)
	if !h.Highlight(snap, snap.LineCount(), 1e9) {
		t.Fatal("not final")
	}
	span := snap.Line(1)[:20]
	toks := h.TokenizeSpan(snap, 1, span, nil)
	if len(toks) == 0 {
		t.Fatal("no tokens")
	}
	for _, tk := range toks {
		if !strings.HasPrefix(tk.Type, "comment") {
			t.Errorf("token %+v not a comment inside /* */", tk)
		}
	}
	// Line 0 starts in the initial state: "x" is an identifier.
	toks = h.TokenizeSpan(snap, 0, "x = 1", toks)
	if len(toks) == 0 || strings.HasPrefix(toks[0].Type, "comment") {
		t.Errorf("line 0 span tokens %+v", toks)
	}
	// A snapshot the Highlighter has not seen (a later version) tokenizes from the initial
	// state (a hint), never panics.
	other, err := b.Edit([]buffer.Edit{{Start: 0, End: 2, Text: "//"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.TokenizeSpan(other, 1, "x y", nil); len(got) == 0 || strings.HasPrefix(got[0].Type, "comment") {
		t.Errorf("unknown snapshot: %+v", got)
	}
	if got := h.TokenizeSpan(snap, 99, "x", nil); len(got) == 0 {
		t.Errorf("line out of range: %+v", got)
	}
	// Reset swaps the lexer: TokenizeSpan follows it.
	h.Reset(nil)
	if got := h.TokenizeSpan(snap, 1, span, []monarch.Token{{Type: "junk"}}); len(got) != 0 {
		t.Errorf("plain text after Reset(nil): %+v", got)
	}
}
