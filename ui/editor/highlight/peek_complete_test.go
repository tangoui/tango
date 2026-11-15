package highlight

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// These cases catch returning hint/stale/degraded tokens as complete results.
func TestPeekCompleteTokensValidity(t *testing.T) {
	lx := grammarLexer(t, "complete-peek", `{"tokenPostfix":"","tokenizer":{"root":[["x","identifier"],["y","string"],[".","text"]]}}`)
	for _, name := range []string{
		"complete", "unknown", "other-buffer-same-version", "old-snapshot",
		"dirty", "dirty-above", "pending-degraded", "accepted-degraded",
		"wrong-start", "invalid-slot", "evicted", "reset", "plain", "negative", "past-end",
	} {
		t.Run(name, func(t *testing.T) {
			snap := buffer.NewSnapshot("xy\nxy")
			h := New(lx)
			line := 0
			if name != "unknown" {
				if !h.Highlight(snap, snap.LineCount(), time.Second) {
					t.Fatal("setup not final")
				}
			}
			switch name {
			case "other-buffer-same-version":
				snap = buffer.NewSnapshot("yy\nyy")
			case "old-snapshot":
				edits := []buffer.Edit{{Start: 0, End: 1, Text: "y"}}
				ns, err := snap.Apply(edits)
				if err != nil {
					t.Fatal(err)
				}
				h.Edit(snap, ns, edits)
			case "dirty":
				h.st.markDirty(0)
			case "dirty-above":
				h.st.markDirty(0)
				line = 1
			case "pending-degraded", "accepted-degraded":
				if name == "accepted-degraded" {
					h.MaxRetries = 0
				}
				start := h.startState(0)
				h.apply(0, start, start, true, []monarch.Token{{Type: "preview"}}, true, time.Now())
				if name == "accepted-degraded" && (!h.st.final(0) || !h.st.acceptedDegraded(0)) {
					t.Fatal("setup must be final but degraded")
				}
				if _, ok := h.PeekTokens(snap, 0, nil); !ok {
					t.Fatal("setup must retain preview tokens ordinary PeekTokens would expose")
				}
			case "wrong-start":
				h.st.slot(0).start = lx.Initial() + 1
			case "invalid-slot":
				h.st.slot(0).ok = false
			case "evicted":
				h.DropView(DefaultView)
			case "reset":
				h.Reset(grammarLexer(t, "reset-complete-peek", `{"tokenizer":{"root":[[".+","comment"]]}}`))
			case "plain":
				h.Reset(nil)
			case "negative":
				line = -1
			case "past-end":
				line = snap.LineCount()
			}
			got, ok := h.PeekCompleteTokens(snap, line, []monarch.Token{{Type: "old-dst"}})
			if name != "complete" {
				if ok || len(got) != 0 {
					t.Fatalf("incomplete cache returned %v, %v", got, ok)
				}
				return
			}
			want := []monarch.Token{{Type: "identifier"}, {Start: 1, ByteStart: 1, Type: "string"}}
			if !ok || !slices.Equal(got, want) {
				t.Fatalf("complete tokens = %v, %v; want %v", got, ok, want)
			}
			got[0].Type = "caller-mutated"
			again, ok := h.PeekCompleteTokens(snap, 0, nil)
			if !ok || !slices.Equal(again, want) {
				t.Fatal("peek returned an alias of cached tokens")
			}
		})
	}
}

func TestPeekCompleteTokensKeepsTwoViews(t *testing.T) {
	lx := grammarLexer(t, "peek-views", `{"tokenPostfix":"","tokenizer":{"root":[["x","identifier"]]}}`)
	h := New(lx)
	h.TokenWindow = 2
	text := "x"
	for range 99 {
		text += "\nx"
	}
	snap := buffer.NewSnapshot(text)
	if !h.Highlight(snap, snap.LineCount(), time.Second) {
		t.Fatal("setup not final")
	}
	a, b := NewViewKey(), NewViewKey()
	h.TokensFor(a, snap, 10, nil)
	h.TokensFor(b, snap, 80, nil)
	before := make([]window, len(h.st.wins))
	copy(before, h.st.wins)
	clock, calls := h.st.clock, h.tokenized.Load()
	for range 10 {
		for _, line := range []int{10, 80, 50} {
			got, ok := h.PeekCompleteTokens(snap, line, nil)
			if line == 50 {
				if ok || len(got) != 0 {
					t.Fatal("uncached final line must miss")
				}
			} else if !ok || !slices.Equal(got, []monarch.Token{{Type: "identifier"}}) {
				t.Fatalf("line %d tokens %v, %v", line, got, ok)
			}
		}
	}
	if h.st.clock != clock || h.tokenized.Load() != calls || !reflect.DeepEqual(before, h.st.wins) {
		t.Fatal("complete peek moved/created a window, touched LRU or tokenized")
	}
	for k := range before {
		if len(before[k].slots) > 0 && &before[k].slots[0] != &h.st.wins[k].slots[0] {
			t.Fatal("complete peek reallocated a view window")
		}
	}
}

func TestPeekCompleteTokensEmptyCachedLine(t *testing.T) {
	lx := grammarLexer(t, "peek-empty", `{"tokenizer":{"root":[["x","identifier"]]}}`)
	h := New(lx)
	snap := buffer.NewSnapshot("")
	if !h.Highlight(snap, 1, time.Second) {
		t.Fatal("setup not final")
	}
	got, ok := h.PeekCompleteTokens(snap, 0, nil)
	if !ok || len(got) != 0 {
		t.Fatalf("empty cached complete line must hit: %v, %v", got, ok)
	}
}
