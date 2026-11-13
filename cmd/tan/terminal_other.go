//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package main

// isTerminal is false where tan has no terminal check: init then needs a
// directory or -yes instead of asking questions.
func isTerminal(fd uintptr) bool { return false }
