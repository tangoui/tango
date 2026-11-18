//go:build !darwin && !linux && !windows

package uithread

// osThreadID is not implemented here; tests that need it skip.
func osThreadID() (uint64, bool) { return 0, false }
