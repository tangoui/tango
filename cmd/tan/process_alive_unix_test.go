//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether a process with pid exists.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks without signalling.
	err = p.Signal(syscall.Signal(0))
	return err == nil || !errors.Is(err, os.ErrProcessDone)
}
