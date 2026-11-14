package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ipc"
)

func TestCoalescerPostsOnce(t *testing.T) {
	var wakes atomic.Int32
	c := newCoalescer(func() { wakes.Add(1) })

	var wg sync.WaitGroup
	for g := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				c.push(strconv.Itoa(g*100 + i))
			}
		}()
	}
	wg.Wait()
	if n := wakes.Load(); n != 1 {
		t.Fatalf("wake called %d times before drain, want 1", n)
	}
	if got := c.drain(); len(got) != 1000 {
		t.Fatalf("drain returned %d items, want 1000", len(got))
	}
	c.push("again")
	if n := wakes.Load(); n != 2 {
		t.Fatalf("wake called %d times after drain + push, want 2", n)
	}
	if got := c.drain(); len(got) != 1 || got[0] != "again" {
		t.Fatalf("second drain %q", got)
	}
}

func TestCoalescerNoLostItems(t *testing.T) {
	const total = 10000
	var (
		mu      sync.Mutex
		drained []string
		drains  sync.WaitGroup
		c       *coalescer
	)
	// wake drains at once, on another goroutine, while pushes go on.
	c = newCoalescer(func() {
		drains.Add(1)
		go func() {
			defer drains.Done()
			items := c.drain()
			mu.Lock()
			drained = append(drained, items...)
			mu.Unlock()
		}()
	})

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := g; i < total; i += 4 {
				c.push(strconv.Itoa(i))
			}
		}()
	}
	wg.Wait()
	drains.Wait()
	// The last wake may have drained before the last push; take the rest.
	rest := c.drain()
	drained = append(drained, rest...)

	if len(drained) != total {
		t.Fatalf("drained %d items, want %d", len(drained), total)
	}
	seen := make(map[string]bool, total)
	for _, s := range drained {
		if seen[s] {
			t.Fatalf("item %s drained twice", s)
		}
		seen[s] = true
	}
}

func TestTermEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Term runs cmd.exe on Windows; this test drives /bin/sh")
	}
	r := ipc.NewRouter()
	registerServices(r)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := r.Open(ctx, "Term")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rw := s.ReadWriter()

	// One reader goroutine collects everything the shell prints.
	var (
		mu  sync.Mutex
		out bytes.Buffer
	)
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := rw.Read(buf)
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				readErr <- err
				return
			}
		}
	}()
	output := func() string { mu.Lock(); defer mu.Unlock(); return out.String() }

	if _, err := rw.Write([]byte("echo tango-$((40+2))\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !bytes.Contains([]byte(output()), []byte("tango-42")) {
		if time.Now().After(deadline) {
			t.Fatalf("no tango-42 within 5s; output %q", output())
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := rw.Write([]byte("exit\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readErr:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("read after exit: %v, want io.EOF (output %q)", err, output())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no io.EOF within 5s after exit; output %q", output())
	}
}
