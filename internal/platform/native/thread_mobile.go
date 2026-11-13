//go:build ios || android

package native

// RunOwnsMainThread reports whether App.Run executes on the platform's main
// (UI) thread. On mobile the native host owns the main thread and Run runs
// on the goroutine that called it (main.main, started by the host; see
// docs/mobile.md): the core then learns the main thread only from Host
// callbacks, and the process' main goroutine is not locked to any thread.
const RunOwnsMainThread = false

// mobileMainGoroutine returns the id of the goroutine the native host
// started main.main on, or 0 if not known yet. Set by the mobile backend
// files (backend_ios.go, backend_android.go); nil means unknown.
var mobileMainGoroutine func() uint64

// MainGoroutine returns the id of the goroutine the native host started
// main.main on, or 0 if not known yet. On mobile that is not the goroutine
// that ran package initialization: the host starts main.main on a
// goroutine of its own.
func MainGoroutine() uint64 {
	if mobileMainGoroutine == nil {
		return 0
	}
	return mobileMainGoroutine()
}
