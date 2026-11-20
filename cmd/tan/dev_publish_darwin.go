package main

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
)

func publishDevOutput(ctx context.Context, src, dst string) error {
	_, err := os.Lstat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	if os.IsNotExist(err) {
		return unix.RenameatxNp(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_EXCL)
	}
	// Exchange two complete bundles. The caller removes the old one at src.
	return unix.RenameatxNp(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_SWAP)
}
