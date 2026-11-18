package uithread

import "syscall"

// osThreadID returns the calling OS thread's id.
func osThreadID() (uint64, bool) { return uint64(syscall.Gettid()), true }
