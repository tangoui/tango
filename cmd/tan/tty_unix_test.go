//go:build darwin || (linux && !android)

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/sys/pty"
	"golang.org/x/sys/unix"
)

// A program tan dev starts can move the terminal's foreground to a process
// group of its own (an interactive shell with job control) and exit without
// giving it back. Ctrl-C then reaches no process; tan dev takes the terminal
// back once that group is gone.
func TestTTYGuardReclaimsForegroundFromExitedGroup(t *testing.T) {
	if os.Getenv("TAN_TTY_GUARD_HELPER") == "1" {
		ttyGuardHelper()
		return
	}
	p, err := pty.Start(pty.Options{
		Path: os.Args[0],
		Args: []string{os.Args[0], "-test.run=^TestTTYGuardReclaimsForegroundFromExitedGroup$"},
		Env:  append(os.Environ(), "TAN_TTY_GUARD_HELPER=1"),
		Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer p.Close()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() { io.Copy(&out, p); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("helper did not finish:\n%s", out.String())
	}
	s := out.String()
	if !strings.Contains(s, "STOLEN") {
		t.Fatalf("the helper could not hand the terminal to another group:\n%s", s)
	}
	if !strings.Contains(s, "RECLAIMED") {
		t.Fatalf("the terminal was not taken back:\n%s", s)
	}
}

func ttyGuardHelper() {
	g := newTTYGuard()
	// A child in its own group takes the foreground and exits.
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: 0}
	if err := cmd.Run(); err != nil {
		os.Stdout.WriteString("RUN " + err.Error() + "\n")
		return
	}
	if fg, err := unix.IoctlGetInt(0, unix.TIOCGPGRP); err == nil && fg != unix.Getpgrp() {
		os.Stdout.WriteString("STOLEN\n")
	}
	g.check()
	if fg, err := unix.IoctlGetInt(0, unix.TIOCGPGRP); err == nil && fg == unix.Getpgrp() {
		os.Stdout.WriteString("RECLAIMED\n")
	}
}
