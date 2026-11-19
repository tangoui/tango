package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// jobScript makes the jobs run an sh script with body instead of tan, and
// returns a manager that is stopped when the test ends.
func jobScript(t *testing.T, body string) *jobManager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("job tests run sh scripts")
	}
	path := filepath.Join(t.TempDir(), "tan.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := selfExe
	selfExe = func() (string, error) { return path, nil }
	t.Cleanup(func() { selfExe = old })
	m := newJobManager()
	t.Cleanup(m.stopAll)
	return m
}

// waitJob polls the job until it is no longer running.
func waitJob(t *testing.T, m *jobManager, id string) jobInfo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		info, err := m.status(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if info.State != "running" {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s still running: %+v", id, info)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestJobOutputAndExit(t *testing.T) {
	m := jobScript(t, "echo hi; echo oops >&2; exit 3")
	dir := t.TempDir()
	info, err := m.start("build", dir, []string{"build", "-x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "job-1" || info.Kind != "build" || info.Dir != dir || info.State != "running" ||
		strings.Join(info.Args, " ") != "build -x" || info.Started.IsZero() {
		t.Fatalf("start = %+v", info)
	}
	info = waitJob(t, m, info.ID)
	if info.State != "exited" || info.ExitCode == nil || *info.ExitCode != 3 {
		t.Fatalf("status = %+v", info)
	}
	if !strings.Contains(info.Output, "hi") || !strings.Contains(info.Output, "oops") || info.Truncated {
		t.Fatalf("output = %q truncated=%v", info.Output, info.Truncated)
	}
	if l := m.list(); len(l) != 1 || l[0].ID != "job-1" || l[0].Output != "" {
		t.Fatalf("list = %+v", l)
	}
	if _, err := m.status("job-9", 0); err == nil {
		t.Fatal("status of an unknown job: no error")
	}
}

func TestJobOutputTruncated(t *testing.T) {
	// 300 lines of 1 KiB: the last 200 lines are more than 64 KiB.
	m := jobScript(t, `line=$(printf '%01023d' 0); i=0; while [ $i -lt 300 ]; do echo "$line"; i=$((i+1)); done`)
	info, err := m.start("build", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, m, info.ID)
	info, err = m.status(info.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(info.Output); n > 64<<10 || n < 60<<10 || !info.Truncated {
		t.Fatalf("output %d bytes, truncated=%v", n, info.Truncated)
	}
	if !strings.HasPrefix(info.Output, "0000") || !strings.HasSuffix(info.Output, "0\n") {
		t.Fatal("output not cut at a line boundary")
	}
}

func TestJobStopAll(t *testing.T) {
	// "ready" comes after the shell forked sleep: a SIGTERM sent while
	// dash forks (it blocks signals around the fork) misses the new child,
	// which then holds the output until the kill after jobStopGrace.
	m := jobScript(t, "sleep 60 & echo ready; wait")
	a, err := m.start("dev", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.start("dev", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatalf("same id %s", a.ID)
	}
	for _, id := range []string{a.ID, b.ID} {
		for deadline := time.Now().Add(10 * time.Second); ; {
			info, err := m.status(id, 0)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(info.Output, "ready") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never ready: %+v", id, info)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	start := time.Now()
	m.stopAll()
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("stopAll took %v", d)
	}
	for _, id := range []string{a.ID, b.ID} {
		info, err := m.status(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if info.State != "stopped" {
			t.Errorf("%s: %+v", id, info)
		}
	}
	if _, err := m.start("dev", t.TempDir(), nil, nil); err == nil {
		t.Error("start after stopAll: no error")
	}
}

func TestJobEnvHasLaunchBackground(t *testing.T) {
	m := jobScript(t, `printf %s "$TANGO_LAUNCH_BACKGROUND"`)
	info, err := m.start("dev", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info = waitJob(t, m, info.ID); info.Output != "1" {
		t.Fatalf("output = %q", info.Output)
	}
}

func TestJobEnvCannotUnsetLaunchBackground(t *testing.T) {
	m := jobScript(t, `printf %s "$TANGO_LAUNCH_BACKGROUND"`)
	info, err := m.start("dev", t.TempDir(), nil, []string{"TANGO_LAUNCH_BACKGROUND=0"})
	if err != nil {
		t.Fatal(err)
	}
	if info = waitJob(t, m, info.ID); info.Output != "1" {
		t.Fatalf("output = %q", info.Output)
	}
}

func TestTailBufferLinesAndTail(t *testing.T) {
	b := newTailBuffer(16)
	b.Write([]byte("a\nb\nc\n"))
	if s, tr := b.Lines(2, 100); s != "b\nc\n" || !tr {
		t.Errorf("Lines(2) = %q, %v", s, tr)
	}
	if s, tr := b.Lines(3, 100); s != "a\nb\nc\n" || tr {
		t.Errorf("Lines(3) = %q, %v", s, tr)
	}
	if s, tr := b.Lines(2, 3); s != "c\n" || !tr {
		t.Errorf("Lines(2, 3) = %q, %v", s, tr)
	}
	if s, tr := b.Tail(100); s != "a\nb\nc\n" || tr {
		t.Errorf("Tail = %q, %v", s, tr)
	}
	b.Write([]byte("0123456789xyz")) // 19 bytes written: keeps the last 16
	if s, tr := b.Tail(100); s != "\nc\n0123456789xyz" || !tr {
		t.Errorf("Tail after drop = %q, %v", s, tr)
	}
	if s, tr := b.Lines(5, 100); s != "c\n0123456789xyz" || !tr {
		t.Errorf("Lines after drop = %q, %v", s, tr)
	}
	c := newTailBuffer(16)
	c.Write([]byte("0123456789abcdef0123456789ABCDEFGH\nlast\n")) // past twice max: compacts
	if s, tr := c.Tail(100); s != "89ABCDEFGH\nlast\n" || !tr {
		t.Errorf("Tail after compaction = %q, %v", s, tr)
	}
	c.Write([]byte("x\n"))
	if s, tr := c.Lines(10, 0); s != "last\nx\n" || !tr {
		t.Errorf("Lines after compaction = %q, %v", s, tr)
	}
	u := newTailBuffer(7)
	u.Write([]byte("ééééé")) // 10 bytes: the first kept byte is inside a rune
	if s, tr := u.Tail(100); s != "ééé" || !tr {
		t.Errorf("Tail at rune cut = %q, %v", s, tr)
	}
}
