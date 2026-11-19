package ui

import (
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui/paint"
)

// offThreadMsg is the panic of a development-build thread check.
const offThreadMsg = "ui: called off the window's UI thread; use Runner.Post"

// UIPoster is implemented by hosts whose UI thread is the main thread.
// PostUI runs fn on that thread later, never inline; any goroutine may call it.
// Runner.Post uses it when no thread is bound. Experimental.
type UIPoster interface{ PostUI(fn func()) }

// uiPosterRef boxes the attached window's UIPoster for an atomic.Pointer.
type uiPosterRef struct{ UIPoster }

// BindThread makes t the runner's UI thread: Post queues work there and, in
// development builds, the UI-thread entry points panic when called from
// elsewhere. Call it once, before Attach; the binding cannot change.
// Experimental: used by the platform host (uihost).
func (r *Runner) BindThread(t *uithread.Thread) {
	if t == nil {
		panic("ui: Runner.BindThread: nil thread")
	}
	if r.win != nil {
		panic("ui: Runner.BindThread: called after Attach")
	}
	if !r.thread.CompareAndSwap(nil, t) {
		panic("ui: Runner.BindThread: runner already bound to a thread")
	}
}

func init() {
	corehook.FinishRunnerStartup = func(runner, owner, renderer any) {
		r := runner.(*Runner)
		if r.win == nil || r.thread.Load() != nil {
			panic("ui: invalid startup handoff")
		}
		if renderer != nil {
			r.renderer = renderer.(paint.Renderer)
			r.cpu, _ = r.renderer.(*paint.CPU)
			r.c.noBlur = !rendererBlurs(r.renderer)
			r.c.imageCache, _ = r.renderer.(imageInvalidator)
		}
		r.thread.Store(owner.(*uithread.Thread))
	}
	corehook.ReturnRunnerToMain = func(runner any) {
		r := runner.(*Runner)
		r.thread.Store(nil)
		r.cpu = paint.NewCPU()
		r.renderer = r.cpu
		r.c.noBlur = !rendererBlurs(r.renderer)
		r.c.imageCache, _ = r.renderer.(imageInvalidator)
		r.dmg.started = false // the next frame is presented whole
	}
}
