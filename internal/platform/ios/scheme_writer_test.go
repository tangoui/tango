package ios

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

	"github.com/tangoui/tango/internal/platform"
)

// Tests of the asynchronous responseWriter (scheme_writer.go). The handler
// writes on its own goroutine; the writer queues actions and kicks a drain
// on the main thread (tg_task_kick in production: a block on the main run
// loop). fakeMain plays scheme.go's and bridge.m's part: a kick queues a
// block; the test runs the queued blocks when it advances "the main
// thread", detaching the list like CFRunLoop does for one pass. A block
// drains the writer unless the task is gone (stopped, or finished: the Go
// side no longer finds it in b.tasks). The sink records the task calls and
// flags any call on a task that is gone. Same contract as
// internal/platform/darwin's scheme_async_test.go.

type fakeMain struct {
	mu      sync.Mutex
	blocks  []func() // kicked drains, not yet run
	w       *responseWriter
	removed bool // stopURLSchemeTask: or didFinish took the task out of tasks

	ops  []string
	body bytes.Buffer
	head http.Header

	// onData, when set, runs (on the "main thread") inside the next data
	// call, and is cleared first: e.g. a nested run loop.
	onData func()
	inCall int // a task call is on the stack
}

func newFake(ctx context.Context, reqPath string) (*fakeMain, *responseWriter) {
	f := &fakeMain{}
	f.w = newResponseWriter(ctx, f, f.kick, reqPath)
	return f, f.w
}

// kick is the writer's kick: it must not block.
func (f *fakeMain) kick() {
	f.mu.Lock()
	f.blocks = append(f.blocks, func() {
		f.mu.Lock()
		gone := f.removed
		f.mu.Unlock()
		if !gone {
			f.w.drain()
		}
	})
	f.mu.Unlock()
}

// advance runs the blocks queued so far on the calling goroutine (the
// "main thread"), and what they queue in turn; it returns how many ran.
// Like CFRunLoop it detaches the list for a pass: a nested advance inside
// a block only sees blocks queued after the outer pass started.
func (f *fakeMain) advance() int {
	n := 0
	for {
		f.mu.Lock()
		q := f.blocks
		f.blocks = nil
		f.mu.Unlock()
		if len(q) == 0 {
			return n
		}
		for _, b := range q {
			b()
			n++
		}
	}
}

func (f *fakeMain) record(op string) {
	if f.removed {
		op = "AFTER REMOVAL: " + op
	}
	if f.inCall > 0 {
		op = "INSIDE ANOTHER CALL: " + op
	}
	f.ops = append(f.ops, op)
}

func (f *fakeMain) respond(status int, h http.Header) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.head = h
	f.record("respond " + strconv.Itoa(status))
}

func (f *fakeMain) data(p []byte) {
	f.mu.Lock()
	f.body.Write(p)
	f.record("data " + strconv.Itoa(len(p)))
	hook := f.onData
	f.onData = nil
	f.inCall++
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	f.mu.Lock()
	f.inCall--
	f.mu.Unlock()
}

func (f *fakeMain) finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("finish")
	f.removed = true
}

// stopTask is webView:stopURLSchemeTask: (main thread).
func (f *fakeMain) stopTask() {
	f.mu.Lock()
	f.removed = true
	f.mu.Unlock()
	f.w.stop()
}

func (f *fakeMain) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ops...)
}

func (f *fakeMain) bodyBytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.body.Bytes()...)
}

// queued is how many kicked drains wait for the main thread.
func (f *fakeMain) queued() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.blocks)
}

func TestIOSWriterOrdersActions(t *testing.T) {
	f, w := newFake(context.Background(), "/a.txt")
	io.WriteString(w, "a")
	io.WriteString(w, "bb")
	if err := w.FlushError(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "ccc")
	w.finish()
	if ops := f.snapshot(); len(ops) != 0 {
		t.Fatalf("task called before the main thread ran: %q", ops)
	}
	f.advance()
	want := []string{"respond 200", "data 3", "data 3", "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if string(f.bodyBytes()) != "abbccc" {
		t.Fatalf("body %q", f.bodyBytes())
	}
	if f.head.Get("Content-Length") != "" {
		t.Fatalf("streamed response got Content-Length %q", f.head.Get("Content-Length"))
	}
	w.finish()
	if f.advance() != 0 || len(f.snapshot()) != 4 {
		t.Fatal("actions after finish")
	}
	// A late write fails as on macOS (internal/platform/darwin).
	if _, err := w.Write([]byte("x")); !errors.Is(err, platform.ErrResponseFinished) {
		t.Fatalf("write after finish: %v", err)
	}
	if err := w.FlushError(); !errors.Is(err, platform.ErrResponseFinished) {
		t.Fatalf("flush after finish: %v", err)
	}
}

// TestIOSWriterConcurrentOrder runs the handler and the main thread on
// different goroutines (run with -race): bytes reach the task in write
// order, respond exactly once first, finish exactly once last.
func TestIOSWriterConcurrentOrder(t *testing.T) {
	f, w := newFake(context.Background(), "/s.bin")
	stopMain := make(chan struct{})
	mainDone := make(chan struct{})
	go func() { // the "main thread"
		defer close(mainDone)
		for {
			select {
			case <-stopMain:
				f.advance()
				return
			default:
				if f.advance() == 0 {
					time.Sleep(100 * time.Microsecond)
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
		ops := f.snapshot()
		if len(ops) > 0 && ops[len(ops)-1] == "finish" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no finish: %d ops", len(ops))
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(stopMain)
	<-mainDone
	ops := strings.Join(f.snapshot(), ",")
	if !strings.HasPrefix(ops, "respond 200,") || strings.Count(ops, "respond") != 1 || strings.Count(ops, "finish") != 1 {
		t.Fatalf("ops %q", ops)
	}
	if !bytes.Equal(f.bodyBytes(), want.Bytes()) {
		t.Fatalf("body differs: %d bytes, want %d", len(f.bodyBytes()), want.Len())
	}
}

func TestIOSWriterBackpressure(t *testing.T) {
	f, w := newFake(context.Background(), "/big.bin")
	const writes = 24 // 1.5 MiB in 64 KiB writes
	var done atomic.Int32
	finished := make(chan error, 1)
	go func() {
		chunk := make([]byte, flushAt)
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
	// The main thread runs nothing: the writer must stop once more than
	// pendingLimit bytes wait for it.
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-finished:
		t.Fatalf("writer did not block (err %v, %d writes)", err, done.Load())
	default:
	}
	if n := int(done.Load()) * flushAt; n > pendingLimit+flushAt {
		t.Fatalf("%d bytes accepted without the main thread, limit %d", n, pendingLimit)
	}
	deadline := time.After(10 * time.Second)
	for {
		f.advance()
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
			f.advance()
			if n := len(f.bodyBytes()); n != writes*flushAt {
				t.Fatalf("delivered %d bytes, want %d", n, writes*flushAt)
			}
			if ops := f.snapshot(); ops[len(ops)-1] != "finish" {
				t.Fatalf("last op %q", ops[len(ops)-1])
			}
			return
		case <-deadline:
			t.Fatalf("writer still blocked after advancing (%d writes)", done.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestIOSWriterStopUnblocks(t *testing.T) {
	f, w := newFake(context.Background(), "/x")
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

	f.stopTask()
	for name, ch := range map[string]chan error{"Write": writeErr, "FlushError": flushErr} {
		select {
		case err := <-ch:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s after stop: %v, want context.Canceled", name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s still blocked 1 s after stop", name)
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
	before := f.queued()
	w.finish()
	if f.queued() != before {
		t.Fatal("finish after stop posted an action")
	}
	f.advance()
	if ops := f.snapshot(); len(ops) != 0 {
		t.Fatalf("stopped task got %q", ops)
	}
}

// Stop after some actions ran: the rest never reaches the task, and the
// writer posts nothing more.
func TestIOSWriterStopDropsQueued(t *testing.T) {
	f, w := newFake(context.Background(), "/x")
	io.WriteString(w, "one")
	w.Flush()
	f.advance()
	io.WriteString(w, "two")
	w.Flush()
	f.stopTask()
	before := f.queued()
	io.WriteString(w, "three")
	w.Flush()
	w.finish()
	if f.queued() != before {
		t.Fatal("writer posted after stop")
	}
	f.advance()
	w.drain() // a late drain must not reach a stopped task
	want := []string{"respond 200", "data 3"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

func TestIOSWriterContextUnblocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, w := newFake(ctx, "/x")
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

func TestIOSWriterPanicBeforeHeaders(t *testing.T) {
	f, w := newFake(context.Background(), "/p.html")
	runAssetHandler(w, func() {
		w.Header().Set("X-Custom", "1")
		io.WriteString(w, "<p>partial")
		panic("boom")
	})
	f.advance()
	msg := "tango: asset handler panic: boom"
	want := []string{"respond 500", "data " + strconv.Itoa(len(msg)), "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if got := string(f.bodyBytes()); got != msg {
		t.Fatalf("body %q", got)
	}
	if f.head.Get("X-Custom") != "" {
		t.Fatalf("handler headers kept on the 500: %v", f.head)
	}
	if got := f.head.Get("Content-Length"); got != strconv.Itoa(len(msg)) {
		t.Fatalf("Content-Length %q", got)
	}
}

func TestIOSWriterPanicAfterHeaders(t *testing.T) {
	f, w := newFake(context.Background(), "/p.txt")
	runAssetHandler(w, func() {
		io.WriteString(w, "first")
		w.Flush()
		panic("late")
	})
	f.advance()
	want := []string{"respond 200", "data 5", "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

func TestIOSWriterContentLength(t *testing.T) {
	f, w := newFake(context.Background(), "/small.js")
	body := strings.Repeat("x", 5000)
	runAssetHandler(w, func() { io.WriteString(w, body) })
	f.advance()
	if got := f.head.Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length %q, want %d", got, len(body))
	}
	// mime reads the host's tables: Windows registries may say
	// application/javascript (iOS itself uses Go's built-in text/javascript).
	if got := f.head.Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") && !strings.HasPrefix(got, "application/javascript") {
		t.Fatalf("Content-Type %q", got)
	}
	want := []string{"respond 200", "data 5000", "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}

// An empty response (the handler wrote nothing) keeps the old iOS answer:
// 200 with Content-Length 0 and no data call.
func TestIOSWriterEmptyResponse(t *testing.T) {
	f, w := newFake(context.Background(), "/empty")
	runAssetHandler(w, func() {})
	f.advance()
	want := []string{"respond 200", "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if got := f.head.Get("Content-Length"); got != "0" {
		t.Fatalf("Content-Length %q", got)
	}
}

// The writer satisfies what internal/lane's ServeDown needs.
func TestIOSWriterFlushInterfaces(t *testing.T) {
	_, w := newFake(context.Background(), "/")
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

func TestIOSBuildRequest(t *testing.T) {
	r, err := buildRequest(context.Background(), "POST", "tango://app/x/y?q=1",
		"Content-Type: text/plain\nOrigin: tango://app\n", []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.Host != "app" || r.RequestURI != "/x/y?q=1" || r.ContentLength != 2 {
		t.Fatalf("request %s %s %s %d", r.Method, r.Host, r.RequestURI, r.ContentLength)
	}
	if r.Header.Get("Origin") != "tango://app" || r.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("header %v", r.Header)
	}
	b, _ := io.ReadAll(r.Body)
	if string(b) != "hi" {
		t.Fatalf("body %q", b)
	}
}

// TestIOSWriterNestedRunLoop: while the main thread runs one of the task's
// actions, something on the main thread starts a nested run loop (a Go
// callback waiting in tg_step), after the handler posted more. The nested
// loop runs whatever it finds; the task must still get respond, all data in
// order, then finish: nothing dropped, nothing reordered.
func TestIOSWriterNestedRunLoop(t *testing.T) {
	f, w := newFake(context.Background(), "/n.bin")
	io.WriteString(w, "one")
	w.Flush()
	io.WriteString(w, "two")
	w.Flush()
	f.onData = func() { // inside "data 3" (one): the handler went on, then a nested run loop
		io.WriteString(w, "three")
		w.Flush()
		w.finish()
		f.kick() // a drain of this task in the nested pass (e.g. kicked earlier)
		f.advance()
	}
	f.advance()
	f.advance()
	want := []string{"respond 200", "data 3", "data 3", "data 5", "finish"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if got := string(f.bodyBytes()); got != "onetwothree" {
		t.Fatalf("body %q", got)
	}
}

// A stop inside the nested run loop: nothing more reaches the task.
func TestIOSWriterNestedStop(t *testing.T) {
	f, w := newFake(context.Background(), "/n.bin")
	io.WriteString(w, "one")
	w.Flush()
	io.WriteString(w, "two")
	w.Flush()
	f.onData = func() {
		io.WriteString(w, "three")
		w.Flush()
		f.stopTask()
		f.kick()
		f.advance()
	}
	f.advance()
	f.advance()
	want := []string{"respond 200", "data 3"}
	if got := f.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
}
