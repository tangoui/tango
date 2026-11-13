//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package main

import "golang.org/x/sys/unix"

// isTerminal reports whether fd is a terminal: the termios ioctl only
// succeeds on a tty, unlike a character-device check, which /dev/null passes.
func isTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), ioctlReadTermios)
	return err == nil
}
