package highlight

import (
	"context"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// TestHighlightViewForLeavesFarCatchUpToBackground: while a background
// goroutine runs, a view's frame far below the first line not final
// tokenizes nothing (the goroutine catches up); close to it, or with no
// goroutine, it spends its budget as HighlightFor.
func TestHighlightViewForLeavesFarCatchUpToBackground(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 20000))
	n := s.LineCount()
	view := n - 10 // the view's last line, far below line 0

	// No goroutine: the frame tokenizes.
	h := New(lx)
	n0 := h.tokenized.Load()
	h.HighlightViewFor(DefaultView, s, view, time.Millisecond)
	if h.tokenized.Load() == n0 {
		t.Fatal("without a background goroutine the frame tokenized nothing")
	}

	// A goroutine running (counted as Start counts it): nothing.
	h = New(lx)
	h.bg.Add(1)
	n0 = h.tokenized.Load()
	if h.HighlightViewFor(DefaultView, s, view, time.Hour) {
		t.Fatal("reported the view ready")
	}
	if d := h.tokenized.Load() - n0; d != 0 {
		t.Fatalf("tokenized %d lines far above the view while the goroutine catches up", d)
	}
	// The view's lines still draw (from their stored start states).
	if toks, final := h.TokensFor(DefaultView, s, view-1, nil); len(toks) == 0 || final {
		t.Fatalf("view line: %d tokens, final %v; want guessed tokens", len(toks), final)
	}

	// The lines above final up to close to the view: the frame finishes it.
	if !h.HighlightFor(DefaultView, s, view-viewCatchUpLines/2, time.Hour) {
		t.Fatal("setup: not highlighted")
	}
	if !h.HighlightViewFor(DefaultView, s, view, time.Hour) {
		t.Fatal("close to the view: the frame did not finish the lines up to it")
	}
}

// TestStartCountsBackground: Start counts its goroutine while it runs.
func TestStartCountsBackground(t *testing.T) {
	h := New(tsLexer(t))
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan buffer.Snapshot)
	h.Start(ctx, ch, func() {})
	if h.bg.Load() != 1 {
		t.Fatalf("running goroutines %d, want 1", h.bg.Load())
	}
	cancel()
	close(ch)
	for deadline := time.Now().Add(5 * time.Second); h.bg.Load() != 0; {
		if time.Now().After(deadline) {
			t.Fatal("the goroutine is still counted after it stopped")
		}
		time.Sleep(time.Millisecond)
	}
}
