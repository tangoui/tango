//go:build !ios

package pty

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// unlock grants and unlocks the subsidiary device of the /dev/ptmx
// descriptor fd and returns its path.
func unlock(fd int) (string, error) {
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		return "", os.NewSyscallError("ioctl TIOCPTYGRANT", err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		return "", os.NewSyscallError("ioctl TIOCPTYUNLK", err)
	}
	// TIOCPTYGNAME fills a 128-byte buffer with a NUL-terminated path.
	var buf [128]byte
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		return "", os.NewSyscallError("ioctl TIOCPTYGNAME", errno)
	}
	return unix.ByteSliceToString(buf[:]), nil
}
