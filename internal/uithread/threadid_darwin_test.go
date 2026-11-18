package uithread

import "syscall"

// osThreadID returns the calling OS thread's id.
func osThreadID() (uint64, bool) {
	id, _, errno := syscall.RawSyscall(syscall.SYS_THREAD_SELFID, 0, 0, 0)
	return uint64(id), errno == 0
}
