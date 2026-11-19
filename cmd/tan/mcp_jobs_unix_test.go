//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestJobStopKillsGrandchild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	m := jobScript(t, `sleep 60 & echo $! > "$PIDFILE"; wait`)
	info, err := m.start("dev", t.TempDir(), nil, []string{"PIDFILE=" + pidFile})
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	for deadline := time.Now().Add(10 * time.Second); pid == 0; {
		if b, err := os.ReadFile(pidFile); err == nil && strings.HasSuffix(string(b), "\n") {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if pid == 0 && time.Now().After(deadline) {
			t.Fatal("no grandchild pid")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	info, err = m.stop(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != "stopped" {
		t.Fatalf("stop = %+v", info)
	}
	// Once killed it may stay a zombie until init reaps it.
	for deadline := time.Now().Add(2 * time.Second); ; {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still there: kill(0) = %v", pid, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
