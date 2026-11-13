//go:build ios

package ios

/*
#include "bridge.h"
#include "startup.h"
#include "system.h"
*/
import "C"

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe" // also for go:linkname
)

// userMain is the program's main.main. A c-archive never runs it by itself
// (the Go runtime only initialises packages), so TangoStart does: the app's
// main package stays the same as on desktop.
//
//go:linkname userMain main.main
func userMain()

var (
	hostStarted  atomic.Bool
	mainG        atomic.Uint64
	mainReturned = make(chan struct{})
)

// MainGoroutine returns the id of the goroutine running main.main, or 0
// before TangoStart. The core uses it to catch waits for the main thread
// made from main before App.Run, which would deadlock.
func MainGoroutine() uint64 { return mainG.Load() }

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(string(s), 10, 64)
	return id
}

// TangoStart is called once by the native host on the main thread from
// application:didFinishLaunchingWithOptions:. It installs the UIKit side of
// the backend (wake source, lifecycle observers, the window when the app
// has no scene manifest) and runs main.main on a new goroutine. When main
// returns the process exits, as it does on desktop.
//
//export TangoStart
func TangoStart() {
	if !hostStarted.CompareAndSwap(false, true) {
		return
	}
	C.tg_trace(C.CString("TangoStart"))
	C.tg_install()
	C.tgs_install() // system.m: appearance, motion, contrast
	go func() {
		mainG.Store(goroutineID())
		C.tg_trace(C.CString("Go main starts"))
		defer func() {
			close(mainReturned)
			C.tg_exit_on_main(0)
		}()
		userMain()
	}()
	// While main starts on its goroutine, let WebKit launch its processes.
	C.tg_prewarm_webkit()
}

// waitMainReturn gives main up to two seconds to return. Main thread, from
// the termination notification.
func waitMainReturn() {
	select {
	case <-mainReturned:
	case <-time.After(2 * time.Second):
		os.Stderr.WriteString("tango/ios: main did not return before termination\n")
	}
}

func unsafePointer(p *C.char) unsafe.Pointer { return unsafe.Pointer(p) }
