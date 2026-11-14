//go:build darwin && !ios

package darwin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of the asynchronous assetWriter: the handler writes on its own
// goroutine and every taskSink call goes through an injected executor (the
// main-thread queue in production). manualExec only runs what was posted
// when the test advances it, so the tests see exactly what reached the
// "main thread" and when.

type manualExec struct {
	mu  sync.Mutex
	fns []func()
}

func (e *manualExec) post(fn func()) {
	e.mu.Lock()
	e.fns = append(e.fns, fn)
	e.mu.Unlock()
}

// advance runs everything posted so far, and what that posts in turn.
func (e *manualExec) advance() int {
	n := 0
	for {
		e.mu.Lock()
		fns := e.fns
		e.fns = nil
		e.mu.Unlock()
		if len(fns) == 0 {
			return n
		}
		for _, fn := range fns {
			fn()
			n++
		}
	}
}

func inlineExec(fn func()) { fn() }

// recSink records taskSink calls; safe for concurrent use.
type recSink struct {
	mu   sync.Mutex
	ops  []string
	body bytes.Buffer
	head http.Header
}

func (s *recSink) respond(status int, h http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head = h
	s.ops = append(s.ops, "respond "+strconv.Itoa(status))
}

func (s *recSink) data(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body.Write(p)
	s.ops = append(s.ops, "data "+strconv.Itoa(len(p)))
}

func (s *recSink) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "finish")
}

func (s *recSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ops...)
}

func TestAsyncWriterOrdersActions(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/a.txt")
	io.WriteString(w, "a")
	io.WriteString(w, "bb")
	if err := w.FlushError(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "ccc")
	w.finish()
	if ops := s.snapshot(); len(ops) != 0 {
		t.Fatalf("sink called before the executor ran: %q", ops)
	}
	ex.advance()
	want := []string{"respond 200", "data 3", "data 3", "finish"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if s.body.String() != "abbccc" {
		t.Fatalf("body %q", s.body.String())
	}
	if s.head.Get("Content-Length") != "" {
		t.Fatalf("streamed response got Content-Length %q", s.head.Get("Content-Length"))
	}
	if ex.advance() != 0 || len(s.snapshot()) != 4 {
		t.Fatal("actions after finish")
	}
}

// TestAsyncWriterConcurrentOrder runs the handler and the executor on
// different goroutines (run with -race): the bytes reach the sink in
// write order, respond exactly once first, finish exactly once last.
func TestAsyncWriterConcurrentOrder(t *testing.T) {
	var queue = make(chan func(), 1<<16)
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, func(fn func()) { queue <- fn }, "/s.bin")
	stopExec := make(chan struct{})
	execDone := make(chan struct{})
	go func() { // the "main thread"
		defer close(execDone)
		for {
			select {
			case fn := <-queue:
				fn()
			case <-stopExec:
				for {
					select {
					case fn := <-queue:
						fn()
					default:
						return
					}
				}
			}
		}
	}()
	var want bytes.Buffer
	for i := range 400 {
		p := bytes.Repeat([]byte{byte(i)}, 1+(i*977)%9000)
		want.Write(p)
		if _, err := w.Write(p); err != nil {
			t.Fatal(err)
		}
		if i%7 == 0 {
			w.Flush()
		}
	}
	w.finish()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ops := s.snapshot()
		if len(ops) > 0 && ops[len(ops)-1] == "finish" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no finish: %d ops", len(ops))
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(stopExec)
	<-execDone
	ops := s.snapshot()
	if ops[0] != "respond 200" || strings.Count(strings.Join(ops, ","), "respond") != 1 || strings.Count(strings.Join(ops, ","), "finish") != 1 {
		t.Fatalf("ops %q", ops)
	}
	if !bytes.Equal(s.body.Bytes(), want.Bytes()) {
		t.Fatalf("body differs: %d bytes, want %d", s.body.Len(), want.Len())
	}
}

func TestAsyncWriterBackpressure(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/big.bin")
	const writes = 24 // 1.5 MiB in 64 KiB writes
	var done atomic.Int32
	finished := make(chan error, 1)
	go func() {
		chunk := make([]byte, streamChunk)
		for range writes {
			if _, err := w.Write(chunk); err != nil {
				finished <- err
				return
			}
			done.Add(1)
		}
		w.finish()
		finished <- nil
	}()
	// Nobody runs the executor: the writer must stop once more than
	// pendingLimit bytes wait for the main thread.
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-finished:
		t.Fatalf("writer did not block (err %v, %d writes)", err, done.Load())
	default:
	}
	if n := int(done.Load()) * streamChunk; n > pendingLimit+streamChunk {
		t.Fatalf("%d bytes accepted without the main thread, limit %d", n, pendingLimit)
	}
	// Advancing lets it go on until the end.
	deadline := time.After(10 * time.Second)
	for {
		ex.advance()
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
			ex.advance()
			if s.body.Len() != writes*streamChunk {
				t.Fatalf("delivered %d bytes, want %d", s.body.Len(), writes*streamChunk)
			}
			if ops := s.snapshot(); ops[len(ops)-1] != "finish" {
				t.Fatalf("last op %q", ops[len(ops)-1])
			}
			return
		case <-deadline:
			t.Fatalf("writer still blocked after advancing (%d writes)", done.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestAsyncWriterStopUnblocks(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/x")
	writeErr := make(chan error, 1)
	flushErr := make(chan error, 1)
	go func() {
		_, err := w.Write(make([]byte, 2*pendingLimit)) // over the limit at once
		writeErr <- err
	}()
	time.Sleep(50 * time.Millisecond)
	go func() { flushErr <- w.FlushError() }()
	time.Sleep(150 * time.Millisecond)
	select {
	case err := <-writeErr:
		t.Fatalf("Write did not block: %v", err)
	case err := <-flushErr:
		t.Fatalf("FlushError did not block: %v", err)
	default:
	}

	w.stop() // webView:stopURLSchemeTask:
	for name, ch := range map[string]chan error{"Write": writeErr, "FlushError": flushErr} {
		select {
		case err := <-ch:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s after stop: %v, want context.Canceled", name, err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("%s still blocked 100 ms after stop", name)
		}
	}
	start := time.Now()
	if err := w.FlushError(); !errors.Is(err, context.Canceled) {
		t.Fatalf("FlushError after stop: %v", err)
	}
	if _, err := w.Write([]byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write after stop: %v", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("calls after stop took %v", d)
	}
	w.finish()
	ex.advance()
	if ops := s.snapshot(); len(ops) != 0 {
		t.Fatalf("stopped task got %q", ops)
	}
}

// Stop after some actions ran: the rest is dropped, nothing follows.
func TestAsyncWriterStopDropsQueued(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/x")
	io.WriteString(w, "one")
	w.Flush()
	ex.advance()
	io.WriteString(w, "two")
	w.Flush()
	w.finish()
	w.stop()
	ex.advance()
	want := []string{"respond 200", "data 3"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

func TestAsyncWriterContextUnblocks(t *testing.T) {
	ex := &manualExec{}
	ctx, cancel := context.WithCancel(context.Background())
	w := newAssetWriter(ctx, &recSink{}, ex.post, "/x")
	errc := make(chan error, 1)
	go func() {
		_, err := w.Write(make([]byte, 2*pendingLimit))
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Write after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write still blocked after the context was canceled")
	}
}

func TestAsyncWriterPanicBeforeHeaders(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/p.html")
	runAssetHandler(w, func() {
		w.Header().Set("X-Custom", "1")
		io.WriteString(w, "<p>partial")
		panic("boom")
	})
	ex.advance()
	want := []string{"respond 500", "data " + strconv.Itoa(len("tango: asset handler panic: boom")), "finish"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if got := s.body.String(); got != "tango: asset handler panic: boom" {
		t.Fatalf("body %q", got)
	}
	if s.head.Get("X-Custom") != "" {
		t.Fatalf("handler headers kept on the 500: %v", s.head)
	}
}

func TestAsyncWriterPanicAfterHeaders(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/p.txt")
	runAssetHandler(w, func() {
		io.WriteString(w, "first")
		w.Flush()
		panic("late")
	})
	ex.advance()
	want := []string{"respond 200", "data 5", "finish"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

func TestAsyncWriterContentLength(t *testing.T) {
	ex := &manualExec{}
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/small.js")
	body := strings.Repeat("x", 5000)
	runAssetHandler(w, func() { io.WriteString(w, body) })
	ex.advance()
	if got := s.head.Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length %q, want %d", got, len(body))
	}
	if got := s.head.Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("Content-Type %q", got)
	}
	want := []string{"respond 200", "data 5000", "finish"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

// The writer satisfies what internal/lane's ServeDown needs.
func TestAsyncWriterFlushInterfaces(t *testing.T) {
	ex := &manualExec{}
	w := newAssetWriter(context.Background(), &recSink{}, ex.post, "/")
	var rw http.ResponseWriter = w
	if _, ok := rw.(http.Flusher); !ok {
		t.Fatal("not an http.Flusher")
	}
	if _, ok := rw.(interface{ FlushError() error }); !ok {
		t.Fatal("no FlushError")
	}
	if err := http.NewResponseController(rw).Flush(); err != nil {
		t.Fatal(err)
	}
}

// An executor that runs each action inline, inside Write, must not
// deadlock the writer. No production path posts inline any more (the old
// synchronous path is gone and the error path uses postMain); this guards
// the writer's locking against re-entrant delivery.
func TestAsyncWriterInlineExec(t *testing.T) {
	s := &recSink{}
	w := newAssetWriter(context.Background(), s, inlineExec, "/i.bin")
	runAssetHandler(w, func() {
		w.Write(make([]byte, 3*pendingLimit))
		w.Flush()
		w.Write([]byte("tail"))
	})
	if s.body.Len() != 3*pendingLimit+4 {
		t.Fatalf("delivered %d", s.body.Len())
	}
	if ops := s.snapshot(); ops[len(ops)-1] != "finish" {
		t.Fatalf("ops %q", ops)
	}
}
