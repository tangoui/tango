package main

import "golang.org/x/sys/windows"

// isTerminal reports whether fd is a console: GetConsoleMode fails on files,
// pipes and NUL.
func isTerminal(fd uintptr) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}
