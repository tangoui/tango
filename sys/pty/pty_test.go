//go:build (darwin && !ios) || (linux && !android)

package pty

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// PTY satisfies ui/terminal.Conn structurally.
var _ interface {
	io.ReadWriter
	Resize(cols, rows, widthPx, heightPx int) error
	Close() error
} = (*PTY)(nil)

// start starts sh -c script on a new PTY and closes it at the end of
// the test.
func start(t *testing.T, script string) *PTY {
	t.Helper()
	p, err := Start(Options{Path: "/bin/sh", Args: []string{"sh", "-c", script}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// readAll reads p until Read fails and returns the output and the
// error. It fails the test if that takes longer than 10 s.
func readAll(t *testing.T, p *PTY) (string, error) {
	t.Helper()
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var out bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := p.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				ch <- result{out.String(), err}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		return r.out, r.err
	case <-time.After(10 * time.Second):
		p.Close()
		t.Fatal("Read did not reach EOF within 10s")
		return "", nil
	}
}

// wait calls p.Wait, failing the test if it takes longer than 10 s.
func wait(t *testing.T, p *PTY) (int, error) {
	t.Helper()
	type result struct {
		code int
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		code, err := p.Wait()
		ch <- result{code, err}
	}()
	select {
	case r := <-ch:
		return r.code, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return within 10s")
		return 0, nil
	}
}

func TestPTYEcho(t *testing.T) {
	p := start(t, "printf hi; exit 3")
	out, err := readAll(t, p)
	if err != io.EOF {
		t.Errorf("Read error = %v, want io.EOF", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("output = %q, want it to contain %q", out, "hi")
	}
	code, err := wait(t, p)
	if code != 3 || err != nil {
		t.Errorf("Wait() = %d, %v, want 3, nil", code, err)
	}
	if p.Pid() <= 0 {
		t.Errorf("Pid() = %d", p.Pid())
	}
}

func TestPTYExitStatus(t *testing.T) {
	p := start(t, "exit 0")
	if _, err := readAll(t, p); err != io.EOF {
		t.Errorf("Read error = %v, want io.EOF", err)
	}
	code, err := wait(t, p)
	if code != 0 || err != nil {
		t.Errorf("Wait() = %d, %v, want 0, nil", code, err)
	}
	// Wait again returns the same result.
	if code, err := wait(t, p); code != 0 || err != nil {
		t.Errorf("second Wait() = %d, %v, want 0, nil", code, err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestPTYResize(t *testing.T) {
	p := start(t, "sleep 0.2; stty size")
	if err := p.Resize(100, 30, 0, 0); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	out, _ := readAll(t, p)
	if !strings.Contains(out, "30 100") {
		t.Errorf("output = %q, want it to contain %q", out, "30 100")
	}
}

func TestPTYDefaultSize(t *testing.T) {
	p := start(t, "stty size")
	out, _ := readAll(t, p)
	if !strings.Contains(out, "24 80") {
		t.Errorf("output = %q, want it to contain %q", out, "24 80")
	}
}

func TestPTYIsTTY(t *testing.T) {
	p := start(t, "test -t 0 && echo tty")
	out, _ := readAll(t, p)
	if !strings.Contains(out, "tty") {
		t.Errorf("output = %q, want it to contain %q", out, "tty")
	}
}

func TestPTYEnv(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("COLORTERM", "")
	t.Setenv("TANGO_PTY_TEST", "inherited")
	p := start(t, "echo $TERM $COLORTERM $TANGO_PTY_TEST")
	out, _ := readAll(t, p)
	if !strings.Contains(out, "xterm-256color truecolor inherited") {
		t.Errorf("output = %q, want it to contain %q", out, "xterm-256color truecolor inherited")
	}
}

func TestPTYDirAndEnv(t *testing.T) {
	dir := t.TempDir()
	p, err := Start(Options{
		Path: "sh", // looked up in $PATH
		Args: []string{"sh", "-c", "pwd; echo $FOO $TERM"},
		Dir:  dir,
		Env:  []string{"FOO=bar", "TERM=vt100", "PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()
	out, _ := readAll(t, p)
	// On macOS t.TempDir is under /var, a symlink to /private/var.
	if !strings.Contains(out, dir) && !strings.Contains(out, "/private"+dir) {
		t.Errorf("output = %q, want it to contain %q", out, dir)
	}
	if !strings.Contains(out, "bar xterm-256color") {
		t.Errorf("output = %q, want it to contain %q", out, "bar xterm-256color")
	}
}

func TestPTYWrite(t *testing.T) {
	p := start(t, "read line; echo got-$line")
	if _, err := p.Write([]byte("abc\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, _ := readAll(t, p)
	if !strings.Contains(out, "got-abc") {
		t.Errorf("output = %q, want it to contain %q", out, "got-abc")
	}
}

func TestPTYStartError(t *testing.T) {
	if _, err := Start(Options{Path: "/nonexistent/program"}); err == nil {
		t.Error("Start of a missing program succeeded")
	}
}

// TestPTYCloseUnblocksRead checks the Conn contract: Close returns, and
// makes a pending Read return, while the program is still running.
func TestPTYCloseUnblocksRead(t *testing.T) {
	p := start(t, "sleep 30")
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		for {
			if _, err := p.Read(buf); err != nil {
				done <- err
				return
			}
		}
	}()
	time.Sleep(100 * time.Millisecond) // let Read block
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("Read returned nil error after Close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not unblock Read")
	}
	// Closing the master hangs up the session: the shell ends.
	if code, err := wait(t, p); code == 0 && err == nil {
		t.Errorf("Wait() = %d, %v after Close, want a hangup", code, err)
	}
	if _, err := p.Read(make([]byte, 1)); err == nil {
		t.Error("Read after Close succeeded")
	}
	if _, err := p.Write([]byte("x")); err == nil {
		t.Error("Write after Close succeeded")
	}
	if err := p.Resize(80, 24, 0, 0); err == nil {
		t.Error("Resize after Close succeeded")
	}
}

// TestPTYCloseUnblocksWrite fills the terminal's input queue of a
// program that never reads it, then checks Close unblocks the Write.
func TestPTYCloseUnblocksWrite(t *testing.T) {
	p := start(t, "stty -icanon -echo; sleep 30")
	time.Sleep(200 * time.Millisecond) // let stty run
	done := make(chan error, 1)
	go func() {
		buf := bytes.Repeat([]byte("x"), 4096)
		for {
			if _, err := p.Write(buf); err != nil {
				done <- err
				return
			}
		}
	}()
	select {
	case err := <-done:
		t.Fatalf("Write failed before Close: %v", err)
	case <-time.After(500 * time.Millisecond): // Write is blocked
	}
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked")
	}
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrClosed) {
			t.Logf("Write error after Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not unblock Write")
	}
}

func TestDefaultShell(t *testing.T) {
	path, args := DefaultShell()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("DefaultShell() path %q: %v", path, err)
	}
	if len(args) == 0 {
		t.Errorf("DefaultShell() args empty")
	}
}

func TestDefaultShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if path, _ := DefaultShell(); path != "/bin/sh" {
		t.Errorf("DefaultShell() with no $SHELL = %q, want /bin/sh", path)
	}
}

func TestEnviron(t *testing.T) {
	got := environ([]string{"A=1", "TERM=dumb", "COLORTERM=no", "B=2"})
	want := []string{"A=1", "B=2", "TERM=xterm-256color", "COLORTERM=truecolor"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("environ = %q, want %q", got, want)
	}
}
