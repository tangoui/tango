package uithread

import "syscall"

var getCurrentThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")

// osThreadID returns the calling OS thread's id.
func osThreadID() (uint64, bool) {
	id, _, _ := getCurrentThreadID.Call()
	return uint64(id), true
}
