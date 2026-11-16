package main

import (
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

const highlightStallBudget = 5 * time.Second

// The demo is tested with -race alongside other packages on CI. The existing
// smoke scale bounds the entire wait; it is not a tokenization speed threshold.
func highlightWatchdogBudget() time.Duration {
	f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 1 {
		return highlightStallBudget
	}
	scaled := float64(highlightStallBudget) * f
	// float64(MaxInt64) rounds up to 2^63, so the upper comparison must be
	// exclusive and precede conversion; a finite factor can still overflow.
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled >= float64(math.MaxInt64) {
		return highlightStallBudget
	}
	return time.Duration(scaled)
}

// A separate clock-driven observer makes progress/notification semantics
// testable without sleeps or slowing a real lexer down by a guessed amount.
type highlightWait struct {
	started, deadline, lastProgress time.Time
	finalLines, readyCount          int
}

func newHighlightWait(now time.Time, budget time.Duration) highlightWait {
	return highlightWait{started: now, deadline: now.Add(budget), lastProgress: now}
}

func (w *highlightWait) observe(now time.Time, finalLines int, ready, done bool) (bool, string) {
	if ready {
		w.readyCount++
	}
	if !now.Before(w.deadline) {
		return false, "outer watchdog expired"
	}
	// Only advancing final lines is progress. Ready storms cannot hide a stuck
	// catch-up loop; a retry normally finishes within the unchanged 5s budget.
	if finalLines > w.finalLines {
		w.finalLines = finalLines
		w.lastProgress = now
	}
	if now.Sub(w.lastProgress) >= highlightStallBudget {
		return false, "background made no progress"
	}
	return ready && done, ""
}

// Query callbacks may wait for the background lock, so sampling the deadline
// clock belongs to the same observation as their returned state.
func sampleHighlight(query func() (int, bool), clock func() time.Time) (int, bool, time.Time) {
	final, done := query()
	return final, done, clock()
}

func waitHighlight(t *testing.T, m *editor.Model, life *modelHighlight, ready <-chan struct{}) {
	t.Helper()
	budget := highlightWatchdogBudget()
	w := newHighlightWait(time.Now(), budget)
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		notified := false
		select {
		case <-ready:
			notified = true
		case <-tick.C:
		case <-deadline.C:
		}
		h, snap := m.Highlighter(), m.Snapshot()
		if h == nil {
			t.Fatal("highlighter disappeared while waiting")
		}
		final, done, now := sampleHighlight(func() (int, bool) {
			return h.FinalLines(snap), h.Done(snap)
		}, time.Now)
		finished, reason := w.observe(now, final, notified, done)
		if reason != "" {
			life.mu.Lock()
			started, closed, dirty := life.started, life.closed, life.dirty
			life.mu.Unlock()
			t.Fatalf("%s: final=%d/%d done=%t ready=%d elapsed=%s lastProgress=%s started=%t closed=%t dirty=%t cancelled=%t seed=%d",
				reason, final, snap.LineCount(), done, w.readyCount,
				now.Sub(w.started), now.Sub(w.lastProgress), started, closed, dirty, life.ctx.Err() != nil, len(life.snapshots))
		}
		if finished {
			return
		}
	}
}

func TestHighlightWaitQueryCannotOutliveWatchdog(t *testing.T) {
	start := time.Unix(0, 0)
	w := newHighlightWait(start, 15*time.Second)
	now := start.Add(4 * time.Second)
	final, done, sampled := sampleHighlight(func() (int, bool) {
		// A lock-taking query returns only after the outer deadline.
		now = start.Add(16 * time.Second)
		return 100, true
	}, func() time.Time { return now })
	if finished, reason := w.observe(sampled, final, true, done); finished || reason != "outer watchdog expired" {
		t.Fatalf("query crossed deadline: finished=%t reason=%q sampled=%s", finished, reason, sampled.Sub(start))
	}
}

func TestHighlightWaitTracksProgressPastOriginalDeadline(t *testing.T) {
	start := time.Unix(0, 0)
	w := newHighlightWait(start, 15*time.Second)
	for i := 1; i <= 3; i++ {
		finished, reason := w.observe(start.Add(time.Duration(2*i)*time.Second), i*100, false, false)
		if finished || reason != "" {
			t.Fatalf("continuing background progress at %ds: finished=%t reason=%q", 2*i, finished, reason)
		}
	}
	if finished, reason := w.observe(start.Add(7*time.Second), 400, true, true); !finished || reason != "" {
		t.Fatalf("final background notification: finished=%t reason=%q", finished, reason)
	}
}

func TestHighlightWaitRequiresFinalNotificationAndBoundsStalls(t *testing.T) {
	start := time.Unix(0, 0)
	t.Run("ready storm is not progress", func(t *testing.T) {
		w := newHighlightWait(start, 15*time.Second)
		for i := 1; i <= 5; i++ {
			finished, reason := w.observe(start.Add(time.Duration(i)*time.Second), 0, true, false)
			if finished || (reason != "") != (i == 5) {
				t.Fatalf("ready %d: finished=%t reason=%q", i, finished, reason)
			}
		}
		if w.readyCount != 5 {
			t.Fatalf("ready count = %d", w.readyCount)
		}
	})
	t.Run("done without ready is not completion", func(t *testing.T) {
		w := newHighlightWait(start, 15*time.Second)
		if finished, reason := w.observe(start.Add(time.Second), 100, false, true); finished || reason != "" {
			t.Fatalf("poll bypassed ready: finished=%t reason=%q", finished, reason)
		}
		if finished, reason := w.observe(start.Add(6*time.Second), 100, false, true); finished || reason == "" {
			t.Fatalf("missing ready never failed: finished=%t reason=%q", finished, reason)
		}
	})
	t.Run("earlier progress ready cannot replace final ready", func(t *testing.T) {
		w := newHighlightWait(start, 15*time.Second)
		if finished, reason := w.observe(start.Add(time.Second), 50, true, false); finished || reason != "" {
			t.Fatalf("progress ready completed: finished=%t reason=%q", finished, reason)
		}
		if finished, reason := w.observe(start.Add(2*time.Second), 100, false, true); finished || reason != "" {
			t.Fatalf("earlier ready licensed a later Done poll: finished=%t reason=%q", finished, reason)
		}
		if finished, reason := w.observe(start.Add(3*time.Second), 100, true, true); !finished || reason != "" {
			t.Fatalf("final ready did not complete: finished=%t reason=%q", finished, reason)
		}
	})
	t.Run("outer watchdog bounds continuous progress", func(t *testing.T) {
		w := newHighlightWait(start, 15*time.Second)
		for i := 1; i <= 15; i++ {
			finished, reason := w.observe(start.Add(time.Duration(i)*time.Second), i, false, false)
			if finished || (reason != "") != (i == 15) {
				t.Fatalf("progress %d: finished=%t reason=%q", i, finished, reason)
			}
		}
	})
}

func TestHighlightWaitWatchdogUsesProjectScale(t *testing.T) {
	belowLimit := math.Nextafter(float64(math.MaxInt64), 0) / float64(highlightStallBudget)
	atLimit := float64(math.MaxInt64) / float64(highlightStallBudget)
	for _, scale := range []string{"", "bad", "-1", "1", "3", "+Inf", "NaN", "1e20", "1e308",
		strconv.FormatFloat(belowLimit, 'g', -1, 64), strconv.FormatFloat(atLimit, 'g', -1, 64)} {
		t.Run(scale, func(t *testing.T) {
			t.Setenv("TANGO_SMOKE_TIMEOUT_SCALE", scale)
			want := highlightStallBudget
			if scale == "3" {
				want *= 3
			} else if scale == strconv.FormatFloat(belowLimit, 'g', -1, 64) {
				want = time.Duration(float64(highlightStallBudget) * belowLimit)
			}
			if got := highlightWatchdogBudget(); got != want {
				t.Fatalf("budget = %s, want %s", got, want)
			}
		})
	}
}

func TestDemoPlaintextSetupStartsSharedBackground(t *testing.T) {
	setupDocument("unknown.extension", genTS(1000), theme.Base("vs-dark"), true, false)
	defer st.Close()
	defer second.Close()
	defer background.Close()
	if st.Highlighter() != nil || second.Model != st.Model {
		t.Fatal("not an initially plaintext shared model")
	}
	ready := make(chan struct{}, 128)
	background.SetNotify(func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	// This is the same Model.SetLanguage path used by the language selector.
	second.Model.SetLanguage("typescript")
	waitHighlight(t, st.Model, background, ready)
	if st.Highlighter() != second.Highlighter() {
		t.Fatal("split panes do not share the one background highlighter")
	}
}

func TestDemoHighlightPlaintextLanguageRoundtrip(t *testing.T) {
	m := editor.NewModel("unknown.extension", genTS(2000), editor.DefaultRegistry())
	life := newModelHighlight(m)
	defer life.Close()
	if m.Highlighter() != nil || life.started {
		t.Fatal("plaintext unexpectedly started a lexer")
	}
	// Change language before OnReady supplies an invalidator. No View is
	// built or Tokens/Highlight called: only the real background can finish.
	m.SetLanguage("typescript")
	h := m.Highlighter()
	if h == nil {
		t.Fatal("missing TypeScript highlighter")
	}
	ready := make(chan struct{}, 128)
	life.SetNotify(func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	waitHighlight(t, m, life, ready)
	if !life.started {
		t.Fatal("background never started")
	}
	ctx, snapshots := life.ctx, life.snapshots
	for _, lang := range []string{editor.PlainText, "go", editor.PlainText, "typescript"} {
		m.SetLanguage(lang)
		if lang == editor.PlainText {
			continue
		}
		waitHighlight(t, m, life, ready)
		if m.Highlighter() != h || life.ctx != ctx || life.snapshots != snapshots {
			t.Fatal("language roundtrip replaced the shared background lifetime")
		}
	}
}

func TestDemoHighlightCloseCancelsAndUnsubscribes(t *testing.T) {
	m := editor.NewModel("unknown.extension", genTS(1000), editor.DefaultRegistry())
	life := newModelHighlight(m)
	var calls atomic.Int32
	ready := make(chan struct{}, 128)
	life.SetNotify(func() {
		calls.Add(1)
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	m.SetLanguage("typescript")
	waitHighlight(t, m, life, ready)
	life.Close()
	life.Close() // idempotent, including channel disposal
	select {
	case <-life.ctx.Done():
	default:
		t.Fatal("close did not cancel background context")
	}
	if life.unsubscribe != nil {
		t.Fatal("model subscription retained")
	}
	if _, ok := <-life.snapshots; ok {
		t.Fatal("snapshot channel not closed")
	}
	n := calls.Load()
	life.SetNotify(func() { calls.Add(1) })
	life.ready() // an already-dispatched background ready must be ignored
	// Reset the same internal highlighter after cancellation. A leaked run
	// would consume this reset and produce final tokens without any View.
	m.SetLanguage(editor.PlainText)
	m.SetLanguage("typescript")
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	if calls.Load() != n {
		t.Fatal("ready notified after close")
	}
	if m.Highlighter().Done(m.Snapshot()) {
		t.Fatal("background still processed a language reset after close")
	}
}

func TestDemoHighlightReadyBeforeWindowAttachment(t *testing.T) {
	m := editor.NewModel("sample.ts", genTS(1000), editor.DefaultRegistry())
	life := newModelHighlight(m)
	defer life.Close()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		life.mu.Lock()
		pending := life.dirty
		life.mu.Unlock()
		if pending && m.Highlighter().Done(m.Snapshot()) {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("real background did not finish before window attachment")
		}
	}
	var calls atomic.Int32
	life.SetNotify(func() { calls.Add(1) })
	if calls.Load() != 1 {
		t.Fatal("completed pre-window work was not replayed exactly once on attachment")
	}
}

func TestDemoHighlightNotifyCloseRace(t *testing.T) {
	m := editor.NewModel("sample.ts", genTS(2000), editor.DefaultRegistry())
	life := newModelHighlight(m)
	var calls atomic.Int32
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			life.SetNotify(func() { calls.Add(1) })
		}
	})
	wg.Go(func() {
		for range 1000 {
			life.ready()
		}
	})
	life.Close() // UI thread owns Model subscription disposal
	wg.Wait()
	n := calls.Load()
	life.ready()
	if !life.isClosed() || calls.Load() != n {
		t.Fatal("notification escaped closed lifetime")
	}
}

func TestDemoHighlightCloseBeforeFirstLexer(t *testing.T) {
	m := editor.NewModel("unknown.extension", "const n = 1;", editor.DefaultRegistry())
	life := newModelHighlight(m)
	life.Close()
	m.SetLanguage("typescript")
	if life.started {
		t.Fatal("language subscription started background after close")
	}
	select {
	case <-life.ctx.Done():
	default:
		t.Fatal("unstarted lifetime not cancelled")
	}
}
