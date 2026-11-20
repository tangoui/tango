//go:build !windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryDevPublicationLock(f *os.File) (func(), error) {
	fd := int(f.Fd())
	err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN) }, nil
}
