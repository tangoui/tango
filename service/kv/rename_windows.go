package kv

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// Windows refuses to replace a file another process has open without
// FILE_SHARE_DELETE: an antivirus scanner or the search indexer reading the
// store a moment after the previous write. Such a hold is brief, so the
// rename is retried for a while instead of failing the Set.
const (
	errorAccessDenied     syscall.Errno = 5
	errorSharingViolation syscall.Errno = 32
	errorLockViolation    syscall.Errno = 33

	replaceTimeout = 500 * time.Millisecond
)

// replaceFile renames from over to, retrying while to is held open.
func replaceFile(from, to string) error {
	deadline := time.Now().Add(replaceTimeout)
	delay := time.Millisecond
	for {
		err := os.Rename(from, to)
		if err == nil || !replaceHeld(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(2*delay, 50*time.Millisecond)
	}
}

// replaceHeld reports whether err is the error of a rename over a file
// another process has open.
func replaceHeld(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errorAccessDenied || errno == errorSharingViolation || errno == errorLockViolation
}
