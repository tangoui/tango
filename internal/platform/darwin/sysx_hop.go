//go:build darwin && !ios

package darwin

// sysxHop runs functions on the main thread from any thread through the
// main dispatch queue (dispatch_async_f), which the main run loop services
// in the common modes. Used by the sysx capabilities, whose system
// callbacks (Network.framework's queue, NSProcessInfo notifications) arrive
// on other threads.

import (
	"sync"

	"github.com/ebitengine/purego"
)

var sysxHop struct {
	once      sync.Once
	asyncF    func(queue, ctx, fn uintptr)
	mainQueue uintptr
	cb        uintptr
}

func hopMain(fn func()) { runOnMain(fn) }

// dispatchMainDrain asks the main dispatch queue to run drainMainQueue. Only
// used before the backend's run loop source exists.
func dispatchMainDrain() {
	h := &sysxHop
	h.once.Do(func() {
		purego.RegisterLibFunc(&h.asyncF, purego.RTLD_DEFAULT, "dispatch_async_f")
		// dispatch_get_main_queue() is a macro for &_dispatch_main_q.
		q, err := purego.Dlsym(purego.RTLD_DEFAULT, "_dispatch_main_q")
		if err != nil {
			panic("tango/darwin: _dispatch_main_q not found")
		}
		h.mainQueue = q
		h.cb = purego.NewCallback(func(_ uintptr) uintptr {
			drainMainQueue()
			return 0
		})
	})
	h.asyncF(h.mainQueue, 0, h.cb)
}
