//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"
)

// C-callable trampolines.
//
// purego can only create a bounded number of callbacks and never releases
// them, so the package makes this fixed set once and multiplexes it: every
// registration passes a dispatch id as the C user_data, and the trampoline
// forwards to the Go handler stored under that id.
var (
	dispatch = newDispatchTable()

	// cbData1 is a GSourceFunc: gboolean f(gpointer user_data).
	cbData1 ptr
	// cbArg2 takes one argument before user_data: signals without
	// parameters ("destroy", "activate") and WebKitURISchemeRequestCallback.
	cbArg2 ptr
	// cbArg3 takes two arguments before user_data: one-parameter signals
	// ("delete-event", "response", "load-changed", ...) and
	// SoupMessageHeadersForeachFunc.
	cbArg3 ptr
	// cbFree drops the dispatch entry named by its first argument. It serves
	// as GDestroyNotify (data) and as GClosureNotify (data, closure), so
	// GLib cleans up when a source or signal connection goes away.
	cbFree ptr
	// cbWake is the idle callback Backend.Wake schedules; it runs wakeTarget.
	cbWake ptr

	wakeTarget func()
)

// GLib constants used with the trampolines.
const (
	gPriorityDefault = 0 // G_PRIORITY_DEFAULT
	gSourceRemove    = 0 // G_SOURCE_REMOVE: a GSourceFunc result that ends the source
)

// initCallbacks creates the trampolines; calls after the first do nothing.
var initCallbacks = sync.OnceFunc(func() {
	viaDispatch := func(data, a, b ptr) ptr { return dispatch.call(data, a, b) }

	cbData1 = purego.NewCallback(func(data ptr) ptr { return viaDispatch(data, 0, 0) })
	cbArg2 = purego.NewCallback(func(a, data ptr) ptr { return viaDispatch(data, a, 0) })
	cbArg3 = purego.NewCallback(func(a, b, data ptr) ptr { return viaDispatch(data, a, b) })
	cbFree = purego.NewCallback(func(data ptr) { dispatch.remove(data) })
	cbWake = purego.NewCallback(func(ptr) ptr {
		if fn := wakeTarget; fn != nil {
			fn()
		}
		return gSourceRemove
	})
})

// connect connects h to signal on instance and returns the handler id.
// args counts the signal's parameters between the instance and user_data
// (0 or 1); h receives (instance, parameter). cbFree, installed as the
// closure notify, forgets h when the connection is broken or the instance
// dies.
func connect(instance ptr, signal string, args int, h handler) uint {
	trampoline := cbArg2
	if args == 1 {
		trampoline = cbArg3
	}
	return gSignalConnectData(instance, signal, trampoline, dispatch.add(h), cbFree, 0)
}

// later schedules fn to run once on the main loop: after ms milliseconds,
// or at the next idle moment when ms is 0. Both use the default priority.
func later(ms uint32, fn func()) {
	id := dispatch.add(func(_, _ ptr) ptr {
		fn()
		return gSourceRemove // GLib then calls cbFree for id
	})
	if ms > 0 {
		gTimeoutAddFull(gPriorityDefault, ms, cbData1, id, cbFree)
		return
	}
	gIdleAddFull(gPriorityDefault, cbData1, id, cbFree)
}

// gbool converts to a gboolean return value.
func gbool(v bool) ptr {
	if !v {
		return 0
	}
	return 1
}
