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

// A seed is only an initializer, not an instruction to restore an old
// snapshot. Reset during a synchronous invocation of the real background batch
// must discard its old generation. Then the real worker must ignore an old seed
// and finish the current snapshot without any foreground Highlight.
func TestBackgroundResetDuringBatchIgnoresStaleSeed(t *testing.T) {
	lx := grammarLexer(t, "reset-seed", `{"tokenizer":{"root":[[".","text"]]}}`)
	h := New(lx)
	old := buffer.NewSnapshot("old\nseed")
	current, err := old.Apply([]buffer.Edit{{Start: 0, End: old.Len(), Text: "current\nsnapshot\nonly"}})
	if err != nil {
		t.Fatal(err)
	}
	h.Sync(old)
	res := func() batchResult {
		// No worker exists in this scope. Even a panic/Goexit unwinds the
		// hook before a worker could read it; no timeout cleanup races a
		// live worker or leaves a closure pointing at this test behind.
		testHookBatchTokenized = func() {
			h.Reset(nil)
			h.Sync(current)
			h.Reset(lx) // coalesces wake signals before the worker starts
		}
		defer func() { testHookBatchTokenized = nil }()
		return h.batch()
	}()
	if res.applied != 0 || h.Done(current) {
		t.Fatal("batch applied results from the old reset generation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	snaps := make(chan buffer.Snapshot, 1)
	snaps <- old
	ready, exited := make(chan struct{}, 8), make(chan struct{})
	go func() {
		defer close(exited)
		h.run(ctx, snaps, func() { ready <- struct{}{} })
	}()
	defer func() {
		cancel()
		waitExit(t, exited, "controlled reset")
	}()
	select {
	case <-ready:
		if !h.Done(current) || h.Done(old) {
			t.Fatal("ready did not describe the current reset generation")
		}
	case <-time.After(10 * time.Second * raceFactor):
		t.Fatal("reset wake was lost")
	}
}

// Inject a transient degraded result, then explicitly make its retry due.
// The first ready is a progress notification, not final readiness. The second
// must follow a clean real-background retry; no wall-clock sleep is involved.
func TestBackgroundTransientRetryNotifiesFinalReadiness(t *testing.T) {
	lx := grammarLexer(t, "retry-ready", `{"tokenizer":{"root":[[".","text"]]}}`)
	h := New(lx)
	s := buffer.NewSnapshot("retry")
	ctx, cancel := context.WithCancel(context.Background())
	snaps := make(chan buffer.Snapshot, 1)
	snaps <- s
	ready, release, exited := make(chan bool, 8), make(chan struct{}), make(chan struct{})
	first := true
	go func() {
		defer close(exited)
		h.run(ctx, snaps, func() {
			if first {
				first = false
				h.mu.Lock()
				h.st.noteResult(0, true, time.Now(), h.MaxRetries, time.Hour)
				h.mu.Unlock()
				ready <- h.Done(s)
				<-release
				return
			}
			ready <- h.Done(s)
		})
	}()
	defer func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		waitExit(t, exited, "controlled transient retry")
	}()
	select {
	case done := <-ready:
		if done {
			t.Fatal("progress notification accepted a pending degraded result")
		}
	case <-time.After(10 * time.Second * raceFactor):
		t.Fatal("no initial background progress")
	}
	h.mu.Lock()
	h.st.deg[0].due = time.Now().Add(-time.Hour)
	h.mu.Unlock()
	h.signal() // signal before the worker enters its idle select
	close(release)
	select {
	case done := <-ready:
		if !done {
			t.Fatal("clean background retry did not report final readiness")
		}
	case <-time.After(10 * time.Second * raceFactor):
		t.Fatal("retry wake was lost")
	}
	h.mu.Lock()
	pending := len(h.st.deg)
	h.mu.Unlock()
	if pending != 0 {
		t.Fatal("clean retry accepted degraded tokens instead of clearing them")
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
