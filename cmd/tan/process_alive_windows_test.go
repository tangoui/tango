package main

import "golang.org/x/sys/windows"

// processAlive reports whether a process with pid is still running. An
// exited process can still be opened while a handle to it is open (tan
// mcp keeps one), so wait on it instead: Windows refuses Signal(0).
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	event, _ := windows.WaitForSingleObject(h, 0)
	return event == uint32(windows.WAIT_TIMEOUT)
}
