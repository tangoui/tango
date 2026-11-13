//go:build !ios && !android

package native

import "runtime"

func init() {
	// AppKit, Win32 and GTK must be driven from the thread that started the
	// process. Keep the main goroutine on it so App.Run, called from main(),
	// runs the native loop there. (On mobile the native host owns the main
	// thread.) LockOSThread in any package's init locks the goroutine that
	// later runs main.main.
	runtime.LockOSThread()
}

// RunOwnsMainThread reports whether App.Run executes on the platform's main
// (UI) thread: on desktop the native loop runs inside Run. See
// thread_mobile.go for mobile.
const RunOwnsMainThread = true

// MainGoroutine returns the id of the goroutine the native host started
// main.main on, or 0 if not known. Only meaningful on mobile: on desktop
// main.main runs on the goroutine that ran package initialization, which
// the core records itself.
func MainGoroutine() uint64 { return 0 }
