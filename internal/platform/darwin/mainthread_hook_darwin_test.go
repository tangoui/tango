//go:build darwin && !ios

package darwin

// MainThreadTestHook lets a test file run a child process's work on the
// main thread after package initialisation (TestMain in
// duplexbin_e2e_test.go calls it before m.Run). Work inside init() cannot
// receive Go callbacks on threads Go did not create: the runtime parks
// them until initialisation completes (runtime/cgocall.go, cgocallbackg).
// A file that sets the hook must also call runtime.LockOSThread in its
// init so the main goroutine is still on the main thread. The hook returns
// handled=false when the process is not one of its children.
var MainThreadTestHook func() (exitCode int, handled bool)
