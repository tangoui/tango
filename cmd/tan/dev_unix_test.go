//go:build !windows

package main

import (
	"strconv"
	"syscall"
	"testing"
	"time"
)

// fakeDevServer is a dev server command line: a child that would outlive a
// plain kill of the shell (its pid in child.pid), then the URL line, then
// wait.
const fakeDevServer = `sleep 60 & echo $! > child.pid; echo "  Local: http://localhost:4321/"; wait`

// assertGone fails unless the process pid has exited.
func assertGone(t *testing.T, pid string) {
	t.Helper()
	n, err := strconv.Atoi(pid)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(n, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("child %d of the dev server survived stop", n)
}
