package main

import (
	"context"
	"sync"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
)

// modelHighlight owns background work for one Model, not one split pane.
// Construction, language notifications and Close run on the UI thread (as
// Model requires). ready and SetNotify synchronize separately: a lexer can
// finish before OnReady has a window to invalidate.
type modelHighlight struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	snapshots   chan buffer.Snapshot
	unsubscribe func()
	started     bool
	closed      bool
	dirty       bool
	notify      func()
}

func newModelHighlight(m *editor.Model) *modelHighlight {
	ctx, cancel := context.WithCancel(context.Background())
	d := &modelHighlight{ctx: ctx, cancel: cancel, snapshots: make(chan buffer.Snapshot, 1)}
	d.unsubscribe = m.Subscribe(func(ch editor.ModelChange) {
		if ch.Language {
			d.start(m)
		}
	})
	d.start(m)
	return d
}

func (d *modelHighlight) start(m *editor.Model) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.started {
		return
	}
	if h := m.Highlighter(); h != nil {
		// Model keeps this internal Highlighter when switching to plaintext
		// and back. Calling Start again would launch a second catch-up loop.
		d.started = true
		d.snapshots <- m.Snapshot()
		h.Start(d.ctx, d.snapshots, d.ready)
	}
}

// SetNotify attaches a nonblocking enqueue/invalidator; it must not call
// back into this lifetime. Calls are serialized with Close, so none can
// start after Close returns. A ready received before attachment is replayed.
func (d *modelHighlight) SetNotify(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.notify = fn
	if d.dirty && fn != nil {
		d.dirty = false
		fn()
	}
}

func (d *modelHighlight) ready() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.dirty = true
	if d.notify != nil {
		d.dirty = false
		d.notify()
	}
}

func (d *modelHighlight) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func (d *modelHighlight) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.notify = nil
	d.cancel()
	// Only start sends one seed, under this same lock. No sender can race
	// channel disposal. Edits/reset wake the existing Highlighter directly.
	close(d.snapshots)
	unsubscribe := d.unsubscribe
	d.unsubscribe = nil
	d.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

// attachHighlightNotify follows each document's lifetime, including files
// opened after startup. Closed lifetimes cannot update the current view.
func attachHighlightNotify(life *modelHighlight, m *editor.Model) {
	if app == nil || runner == nil || life == nil {
		return
	}
	r := runner
	life.SetNotify(func() {
		r.Post(func() { // hlDone and the model belong to the window's UI thread
			if life.isClosed() {
				return
			}
			if h := m.Highlighter(); h != nil && h.Done(m.Snapshot()) {
				hlDone = true
			}
		})
	})
}
