package ui

import "github.com/tangoui/tango/internal/uithread"

// offThreadMsg is the panic of a development-build thread check.
const offThreadMsg = "ui: called off the window's UI thread; use Runner.Post"

// BindThread makes t the runner's UI thread: Post queues work there and, in
// development builds, the UI-thread entry points panic when called from
// elsewhere. Call it once, before Attach; the binding cannot change.
// Experimental: used by the platform host (uihost).
func (r *Runner) BindThread(t *uithread.Thread) {
	if t == nil {
		panic("ui: Runner.BindThread: nil thread")
	}
	if !r.thread.CompareAndSwap(nil, t) {
		panic("ui: Runner.BindThread: runner already bound to a thread")
	}
}
