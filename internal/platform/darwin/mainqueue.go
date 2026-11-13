//go:build darwin && !ios

package darwin

import "sync"

// Backend-level work for the main thread, posted from system callbacks that
// arrive on other threads (UserNotifications completion handlers, Carbon,
// Network.framework, NSProcessInfo notifications).
//
// It rides on the same version-0 CFRunLoopSource in the common modes that
// drives Host.Drain (see Init), so it runs during menu tracking, live
// resize and modal panels too, and before the core's own queue. Earlier
// code used -performSelectorOnMainThread: and dispatch_async_f to the main
// queue; the former never fired here and the latter stalls during modal
// loops, so both now delegate to runOnMain.
var mainQueue struct {
	mu  sync.Mutex
	fns []func()
}

// runOnMain runs fn on the main thread: immediately when already there,
// otherwise on the next turn of the main run loop.
func runOnMain(fn func()) {
	if isMainThread() {
		withPool(fn)
		return
	}
	mainQueue.mu.Lock()
	mainQueue.fns = append(mainQueue.fns, fn)
	mainQueue.mu.Unlock()
	if b := theBackend; b != nil && b.ready.Load() {
		b.Wake()
		return
	}
	// Before Init has created the run loop source (or without a backend,
	// as in some tests): fall back to the main dispatch queue.
	dispatchMainDrain()
}

// drainMainQueue runs queued backend work; called on the main thread from
// the run loop source and the dispatch fallback.
func drainMainQueue() {
	mainQueue.mu.Lock()
	fns := mainQueue.fns
	mainQueue.fns = nil
	mainQueue.mu.Unlock()
	for _, fn := range fns {
		withPool(fn)
	}
}
