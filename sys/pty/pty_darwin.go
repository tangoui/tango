//go:build !ios

package pty

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// utf8Ctype is the LC_CTYPE a program gets when the environment sets no
// locale. C.UTF-8 rather than Terminal.app's bare "UTF-8": ssh forwards
// LC_* (macOS ssh_config SendEnv), and Linux hosts accept C.UTF-8 but
// warn that "UTF-8" is no locale. macOS releases without C.UTF-8 get
// "UTF-8".
var utf8Ctype = func() string {
	if _, err := os.Stat("/usr/share/locale/C.UTF-8"); err == nil {
		return "C.UTF-8"
	}
	return "UTF-8"
}()

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
