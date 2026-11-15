package diffview

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// fakeClock fires AfterFunc callbacks when Advance moves time past them,
// on the goroutine calling Advance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers []*fakeTimer
}

type fakeTimer struct {
	c       *fakeClock
	at      time.Duration
	f       func()
	stopped bool
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) stopper {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, at: c.now + d, f: f}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := !t.stopped
	t.stopped = true
	return was
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now += d
	var due []func()
	keep := c.timers[:0]
	for _, t := range c.timers {
		switch {
		case t.stopped:
		case t.at <= c.now:
			t.stopped = true
			due = append(due, t.f)
		default:
			keep = append(keep, t)
		}
	}
	c.timers = keep
	c.mu.Unlock()
	for _, f := range due {
		f()
	}
}

// waitTimeout bounds every wait for a background goroutine. It is
// generous: the machine running the tests may be heavily loaded.
const waitTimeout = 20 * time.Second

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func snaps(o, m string) (buffer.Snapshot, buffer.Snapshot) {
	return buffer.NewSnapshot(o), buffer.NewSnapshot(m)
}

type harness struct {
	clk   *fakeClock
	r     *runner
	woken chan struct{}
	calls atomic.Int32
}

func newHarness(compute computeFunc) *harness {
	h := &harness{clk: &fakeClock{}, woken: make(chan struct{}, 16)}
	wrapped := func(ctx context.Context, o, m buffer.Snapshot) diff.Result {
		h.calls.Add(1)
		return compute(ctx, o, m)
	}
	h.r = newRunner(h.clk, 200*time.Millisecond, wrapped, func() { h.woken <- struct{}{} })
	return h
}

func plainCompute(ctx context.Context, o, m buffer.Snapshot) diff.Result {
	return diff.Snapshots(ctx, o, m, diff.Options{})
}

func TestRunnerDebounce(t *testing.T) {
	h := newHarness(plainCompute)
	defer h.r.close()
	for i := 0; i < 5; i++ {
		o, m := snaps("a\nb", "a\nb"+string(rune('0'+i)))
		h.r.schedule(o, m)
		h.clk.Advance(150 * time.Millisecond)
	}
	if n := h.calls.Load(); n != 0 {
		t.Fatalf("computed %d times while typing", n)
	}
	if h.r.upToDate() {
		t.Fatal("up to date while a computation is pending")
	}
	h.clk.Advance(50 * time.Millisecond)
	recv(t, h.woken, "result")
	out, ok := h.r.take()
	if !ok {
		t.Fatal("no result after wake")
	}
	if n := h.calls.Load(); n != 1 {
		t.Fatalf("computed %d times, want 1", n)
	}
	if got := out.mod.String(); got != "a\nb4" {
		t.Fatalf("computed for %q, want the last text", got)
	}
	if len(out.result.Changes) != 1 || !h.r.upToDate() {
		t.Fatalf("result %+v, upToDate %v", out.result, h.r.upToDate())
	}
	if _, ok := h.r.take(); ok {
		t.Fatal("result delivered twice")
	}
}

func TestRunnerCancelsRunningComputation(t *testing.T) {
	started := make(chan int, 4)
	cancelled := make(chan int, 4)
	var n atomic.Int32
	h := newHarness(func(ctx context.Context, o, m buffer.Snapshot) diff.Result {
		i := int(n.Add(1))
		started <- i
		if i == 1 {
			<-ctx.Done()
			cancelled <- i
			return diff.Result{TimedOut: true}
		}
		return plainCompute(ctx, o, m)
	})
	defer h.r.close()
	h.r.schedule(snaps("a", "b"))
	h.clk.Advance(200 * time.Millisecond)
	recv(t, started, "first computation")
	h.r.schedule(snaps("a", "c"))
	if got := recv(t, cancelled, "cancellation"); got != 1 {
		t.Fatalf("cancelled computation %d", got)
	}
	h.clk.Advance(200 * time.Millisecond)
	recv(t, started, "second computation")
	recv(t, h.woken, "result")
	out, ok := h.r.take()
	if !ok || out.mod.String() != "c" || out.result.TimedOut {
		t.Fatalf("take = %+v, %v", out, ok)
	}
	h.r.wait()
}

func TestRunnerDropsStaleResult(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	var n atomic.Int32
	h := newHarness(func(ctx context.Context, o, m buffer.Snapshot) diff.Result {
		started <- struct{}{}
		if n.Add(1) == 1 {
			<-release // ignores cancellation, like a computation between checks
		}
		return plainCompute(context.Background(), o, m)
	})
	defer h.r.close()
	h.r.schedule(snaps("a", "b"))
	h.clk.Advance(200 * time.Millisecond)
	recv(t, started, "first computation")
	h.r.schedule(snaps("a", "c")) // the text changed meanwhile
	close(release)
	h.r.wait()
	if out, ok := h.r.take(); ok {
		t.Fatalf("stale result delivered: %+v", out)
	}
	h.clk.Advance(200 * time.Millisecond)
	recv(t, h.woken, "result")
	if out, ok := h.r.take(); !ok || out.mod.String() != "c" {
		t.Fatalf("take = %+v, %v", out, ok)
	}
}

func TestRunnerNowAndClose(t *testing.T) {
	h := newHarness(plainCompute)
	h.r.now(snaps("x", "y"))
	recv(t, h.woken, "result")
	if out, ok := h.r.take(); !ok || out.mod.String() != "y" {
		t.Fatalf("take = %+v, %v", out, ok)
	}
	h.r.schedule(snaps("x", "z"))
	h.r.close()
	h.clk.Advance(time.Second)
	h.r.wait()
	if n := h.calls.Load(); n != 1 {
		t.Fatalf("computed %d times; a closed runner must not start work", n)
	}
	if _, ok := h.r.take(); ok {
		t.Fatal("result after close")
	}
	h.r.schedule(snaps("x", "w")) // no-op after close
	h.clk.Advance(time.Second)
	h.r.wait()
	if n := h.calls.Load(); n != 1 {
		t.Fatalf("computed %d times after close", n)
	}
}

// TestRunnerRealClock runs the runner on the real clock with a short
// debounce: a burst of schedules computes the last texts.
func TestRunnerRealClock(t *testing.T) {
	woken := make(chan struct{}, 16)
	r := newRunner(nil, 5*time.Millisecond, plainCompute, func() { woken <- struct{}{} })
	defer r.close()
	for i := 0; i < 20; i++ {
		r.schedule(snaps("a", "b"+string(rune('a'+i))))
	}
	for {
		recv(t, woken, "result")
		if out, ok := r.take(); ok {
			if out.mod.String() != "bt" {
				t.Fatalf("computed %q", out.mod.String())
			}
			return
		}
	}
}

func TestDiffCompute(t *testing.T) {
	cfg := computeConfig{ignoreTrimWhitespace: true, timeout: time.Second}
	o, m := snaps("a\n  b", "a\nb  ")
	if res := cfg.compute()(context.Background(), o, m); len(res.Changes) != 0 {
		t.Fatalf("whitespace change reported: %v", res.Changes)
	}
	cfg.ignoreTrimWhitespace = false
	if res := cfg.compute()(context.Background(), o, m); len(res.Changes) != 1 {
		t.Fatalf("changes %v", res.Changes)
	}
}
