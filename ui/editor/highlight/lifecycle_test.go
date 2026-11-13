package highlight

import (
	"context"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// runExits starts the background loop as Start does (Start is just
// "go h.run(...)"), in a goroutine the test can wait for.
func runExits(t *testing.T, h *Highlighter, ctx context.Context, snaps <-chan buffer.Snapshot) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.run(ctx, snaps, func() {})
	}()
	return done
}

func waitExit(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second * raceFactor):
		t.Fatalf("background goroutine did not exit after %s", what)
	}
}

// The background goroutine exits when its context is cancelled or its
// snapshot channel is closed, whether it is idle or still has work (the
// editor's model pool creates and drops many Highlighters).
func TestBackgroundGoroutineExits(t *testing.T) {
	lx := tsLexer(t)
	big := buffer.NewSnapshot(tsDoc(t, 200_000))
	for _, c := range []struct {
		name string
		busy bool // a large document still being tokenized
		stop func(cancel context.CancelFunc, snaps chan buffer.Snapshot)
	}{
		{"cancel idle", false, func(cancel context.CancelFunc, _ chan buffer.Snapshot) { cancel() }},
		{"cancel busy", true, func(cancel context.CancelFunc, _ chan buffer.Snapshot) { cancel() }},
		{"close idle", false, func(_ context.CancelFunc, s chan buffer.Snapshot) { close(s) }},
		{"close busy", true, func(_ context.CancelFunc, s chan buffer.Snapshot) { close(s) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := New(lx)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			snaps := make(chan buffer.Snapshot, 1)
			done := runExits(t, h, ctx, snaps)
			if c.busy {
				snaps <- big
				// Wait until it has started on the document.
				deadline := time.Now().Add(10 * time.Second * raceFactor)
				for h.tokenized.Load() == 0 {
					if time.Now().After(deadline) {
						t.Fatal("background never started")
					}
					time.Sleep(time.Millisecond)
				}
			} else {
				snaps <- buffer.NewSnapshot("let a = 1;\nlet b = 2;\n")
				deadline := time.Now().Add(10 * time.Second * raceFactor)
				for h.tokenized.Load() < 2 {
					if time.Now().After(deadline) {
						t.Fatal("background never finished the small document")
					}
					time.Sleep(time.Millisecond)
				}
				time.Sleep(10 * time.Millisecond) // back in its idle select
			}
			c.stop(cancel, snaps)
			waitExit(t, done, c.name)
		})
	}
}

// WindowRange reports the token window, which follows the requested line.
func TestWindowRange(t *testing.T) {
	h := New(tsLexer(t))
	h.TokenWindow = 100
	s := buffer.NewSnapshot(tsDoc(t, 5000))
	h.Highlight(s, 10, time.Hour)
	if lo, hi := h.WindowRange(); lo > 0 || hi < 10 {
		t.Fatalf("window %d..%d after Highlight up to 10", lo, hi)
	}
	h.Tokens(s, 3000, nil)
	if lo, hi := h.WindowRange(); lo > 3000 || hi <= 3000 || hi-lo > 201 {
		t.Fatalf("window %d..%d after Tokens(3000)", lo, hi)
	}
}
