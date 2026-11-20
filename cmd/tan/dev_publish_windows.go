package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryDevPublicationLock(f *os.File) (func(), error) {
	handle := windows.Handle(f.Fd())
	var offset windows.Overlapped
	err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &offset)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(handle, 0, 1, 0, &offset) }, nil
}
