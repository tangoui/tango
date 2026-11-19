//go:build !windows

package main

import (
	"syscall"
	"testing"
	"time"
)

// SIGTERM to tan build (tango_job_stop signals the job's group) cancels
// the build context, and runShell then ends the front-end command, which
// runs in a process group of its own.
func TestSignalContextStopsShell(t *testing.T) {
	ctx, stop := signalContext()
	defer stop()
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- runShell(ctx, t.TempDir(), nil, "sleep 60") }()
	time.Sleep(300 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("runShell = nil after SIGTERM, want the killed command's error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the shell command still running 5 s after SIGTERM")
	}
	if ctx.Err() == nil {
		t.Error("context not cancelled by SIGTERM")
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("took %v", d)
	}
}
