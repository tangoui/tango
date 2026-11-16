//go:build !windows

package kv

import "os"

// replaceFile renames from over to: rename replaces a file other processes
// have open, so there is nothing to wait for.
func replaceFile(from, to string) error { return os.Rename(from, to) }
