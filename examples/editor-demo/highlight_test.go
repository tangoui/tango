package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

func waitHighlight(t *testing.T, m *editor.Model, ready <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ready:
			if h := m.Highlighter(); h != nil && h.Done(m.Snapshot()) {
				return
			}
		case <-deadline.C:
			t.Fatal("background did not report final tokens")
		}
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
	waitHighlight(t, st.Model, ready)
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
	waitHighlight(t, m, ready)
	if !life.started {
		t.Fatal("background never started")
	}
	ctx, snapshots := life.ctx, life.snapshots
	for _, lang := range []string{editor.PlainText, "go", editor.PlainText, "typescript"} {
		m.SetLanguage(lang)
		if lang == editor.PlainText {
			continue
		}
		waitHighlight(t, m, ready)
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
	waitHighlight(t, m, ready)
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
