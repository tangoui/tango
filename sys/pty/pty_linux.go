//go:build !android

package pty

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// unlock unlocks the subsidiary device of the /dev/ptmx descriptor fd
// and returns its path.
func unlock(fd int) (string, error) {
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return "", os.NewSyscallError("ioctl TIOCSPTLCK", err)
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		return "", os.NewSyscallError("ioctl TIOCGPTN", err)
	}
	return "/dev/pts/" + strconv.FormatUint(uint64(n), 10), nil
}
