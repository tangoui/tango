package pty

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// These tests need Windows 10 1809 or later; CI only compiles them.

func TestConPTYEcho(t *testing.T) {
	p, err := Start(Options{Path: "cmd.exe", Args: []string{"cmd", "/c", "echo hi"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Close()
	if err := p.Resize(100, 30, 0, 0); err != nil {
		t.Errorf("Resize: %v", err)
	}
	ch := make(chan string, 1)
	go func() {
		var out bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := p.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				ch <- out.String()
				return
			}
		}
	}()
	select {
	case out := <-ch:
		if !strings.Contains(out, "hi") {
			t.Errorf("output = %q, want it to contain %q", out, "hi")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Read did not reach EOF within 10s")
	}
	if code, err := p.Wait(); code != 0 || err != nil {
		t.Errorf("Wait() = %d, %v, want 0, nil", code, err)
	}
}

func TestConPTYCloseUnblocksRead(t *testing.T) {
	p, err := Start(Options{Path: "cmd.exe", Args: []string{"cmd"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := p.Read(buf); err != nil {
				done <- err
				return
			}
		}
	}()
	time.Sleep(500 * time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close blocked")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not unblock Read")
	}
}

func TestDefaultShell(t *testing.T) {
	if path, args := DefaultShell(); path == "" || len(args) == 0 {
		t.Errorf("DefaultShell() = %q, %q", path, args)
	}
}
