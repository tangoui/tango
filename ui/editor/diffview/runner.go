package diffview

import (
	"context"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// stopper is a timer that can be stopped (*time.Timer).
type stopper interface{ Stop() bool }

// clock schedules the debounce timer; tests inject a fake one.
type clock interface {
	AfterFunc(d time.Duration, f func()) stopper
}

type realClock struct{}

func (realClock) AfterFunc(d time.Duration, f func()) stopper { return time.AfterFunc(d, f) }

// computeFunc diffs two snapshots; it runs on a background goroutine and
// should return early (a coarser result) when ctx is cancelled.
type computeFunc func(ctx context.Context, orig, mod buffer.Snapshot) diff.Result

// computeConfig is what the diff computation depends on in the view's
// options.
type computeConfig struct {
	ignoreTrimWhitespace bool
	computeMoves         bool // forced off for large files by the caller
	timeout              time.Duration
}

func (c computeConfig) compute() computeFunc {
	opts := diff.Options{
		IgnoreTrimWhitespace: c.ignoreTrimWhitespace,
		ComputeMoves:         c.computeMoves,
		Timeout:              c.timeout,
	}
	return func(ctx context.Context, o, m buffer.Snapshot) diff.Result {
		return diff.Snapshots(ctx, o, m, opts)
	}
}

// outcome is a finished computation: the result for the texts orig and mod.
type outcome struct {
	result    diff.Result
	orig, mod buffer.Snapshot
	gen       uint64
}

// runner schedules the background diff of a diff view. The UI goroutine
// calls schedule on every change of either text: that cancels the running
// computation and restarts the debounce timer. When the timer fires, the
// latest snapshots (immutable, safe to read anywhere) are diffed on a new
// goroutine; when it finishes and no newer schedule happened meanwhile, the
// result is kept and wake is called (ui.Invalidate in the view) so that the
// UI goroutine collects it with take. Every schedule bumps a generation;
// results of older generations are dropped, at the end of the computation
// and again in take.
//
// All methods are safe for concurrent use; wake and compute run on other
// goroutines.
type runner struct {
	clk      clock
	debounce time.Duration
	compute  computeFunc
	wake     func()

	mu        sync.Mutex
	gen       uint64 // generation of the latest schedule
	delivered uint64 // generation of the last result taken
	timer     stopper
	cancel    context.CancelFunc // of the running computation
	orig, mod buffer.Snapshot    // texts of the latest schedule
	ready     *outcome
	closed    bool
	wg        sync.WaitGroup
}

// newRunner returns a runner; a nil clk uses the real clock.
func newRunner(clk clock, debounce time.Duration, compute computeFunc, wake func()) *runner {
	if clk == nil {
		clk = realClock{}
	}
	return &runner{clk: clk, debounce: debounce, compute: compute, wake: wake}
}

// schedule asks for the diff of orig and mod after the debounce delay.
func (r *runner) schedule(orig, mod buffer.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	gen := r.restart(orig, mod)
	r.timer = r.clk.AfterFunc(r.debounce, func() { r.fire(gen) })
}

// now asks for the diff of orig and mod without waiting (first display,
// after a revert).
func (r *runner) now(orig, mod buffer.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.start(r.restart(orig, mod))
}

// restart starts a new generation for orig and mod: it stops the timer,
// cancels the running computation and forgets an untaken result.
func (r *runner) restart(orig, mod buffer.Snapshot) uint64 {
	r.gen++
	r.orig, r.mod = orig, mod
	r.ready = nil
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return r.gen
}

func (r *runner) fire(gen uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || gen != r.gen {
		return
	}
	r.timer = nil
	r.start(gen)
}

// start launches the computation of generation gen (r.mu held).
func (r *runner) start(gen uint64) {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	orig, mod := r.orig, r.mod
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		res := r.compute(ctx, orig, mod)
		r.mu.Lock()
		keep := !r.closed && gen == r.gen && ctx.Err() == nil
		if keep {
			r.ready = &outcome{result: res, orig: orig, mod: mod, gen: gen}
			r.cancel = nil
		}
		r.mu.Unlock()
		if keep && r.wake != nil {
			r.wake()
		}
	}()
}

// take returns the finished result of the latest generation, once.
func (r *runner) take() (outcome, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.ready
	r.ready = nil
	if out == nil || out.gen != r.gen {
		return outcome{}, false
	}
	r.delivered = out.gen
	return *out, true
}

// upToDate reports whether the last result taken is for the latest texts
// (Monaco's isDiffUpToDate).
func (r *runner) upToDate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gen != 0 && r.delivered == r.gen
}

// close stops the timer and cancels the running computation; later calls
// do nothing. It does not wait for the computation to return.
func (r *runner) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	r.restart(buffer.Snapshot{}, buffer.Snapshot{})
}

// wait blocks until no computation goroutine is running.
func (r *runner) wait() { r.wg.Wait() }

// configure changes the debounce delay and the computation of the next
// schedules.
func (r *runner) configure(debounce time.Duration, compute computeFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debounce = debounce
	if compute != nil {
		r.compute = compute
	}
}
