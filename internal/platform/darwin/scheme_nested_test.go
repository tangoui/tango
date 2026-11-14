//go:build darwin && !ios

package darwin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"testing"
)

// Re-entrancy tests of assetWriter.drain (asset.go), mirroring
// internal/platform/ios's TestIOSWriterNestedRunLoop and
// TestIOSWriterNestedStop so the two writers keep the same contract.

// hookSink is a recSink whose next data call runs onData (on the "main
// thread", inside the call: e.g. a nested run loop or WebKit stopping the
// task), and which flags calls made after stop or inside another call.
type hookSink struct {
	recSink
	onData  func()
	inCall  int
	stopped bool
}

func (s *hookSink) mark(op string) string {
	if s.stopped {
		op = "AFTER STOP: " + op
	}
	if s.inCall > 0 {
		op = "INSIDE ANOTHER CALL: " + op
	}
	return op
}

func (s *hookSink) respond(status int, h http.Header) {
	s.mu.Lock()
	s.head = h
	s.ops = append(s.ops, s.mark("respond "+strconv.Itoa(status)))
	s.mu.Unlock()
}

func (s *hookSink) data(p []byte) {
	s.mu.Lock()
	s.body.Write(p)
	s.ops = append(s.ops, s.mark("data "+strconv.Itoa(len(p))))
	hook := s.onData
	s.onData = nil
	s.inCall++
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	s.inCall--
	s.mu.Unlock()
}

func (s *hookSink) finish() {
	s.mu.Lock()
	s.ops = append(s.ops, s.mark("finish"))
	s.mu.Unlock()
}

// TestAsyncWriterNestedRunLoop: while the main thread runs one of the
// task's actions, a nested run loop runs another drain of the same writer
// after the handler queued more. The nested drain must return at once; the
// task still gets respond, all data in order, then finish: nothing
// delivered twice, dropped or reordered.
func TestAsyncWriterNestedRunLoop(t *testing.T) {
	ex := &manualExec{}
	s := &hookSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/n.bin")
	io.WriteString(w, "one")
	w.Flush()
	io.WriteString(w, "two")
	w.Flush()
	s.onData = func() { // inside "data 3" (one): the handler went on, then a nested run loop
		io.WriteString(w, "three")
		w.Flush()
		w.finish()
		ex.post(w.drain) // a drain of this writer in the nested pass
		ex.advance()
	}
	ex.advance()
	ex.advance()
	want := []string{"respond 200", "data 3", "data 3", "data 5", "finish"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	if got := s.body.String(); got != "onetwothree" {
		t.Fatalf("body %q", got)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != 0 || w.draining || w.scheduled {
		t.Fatalf("after the drains: pending %d draining %v scheduled %v", w.pending, w.draining, w.scheduled)
	}
}

// TestAsyncWriterStopInsideSink: WebKit stops the task from inside a data
// call, then a nested run loop runs another drain. Nothing more reaches
// the sink, and pending stays zero instead of going negative.
func TestAsyncWriterStopInsideSink(t *testing.T) {
	ex := &manualExec{}
	s := &hookSink{}
	w := newAssetWriter(context.Background(), s, ex.post, "/n.bin")
	io.WriteString(w, "one")
	w.Flush()
	io.WriteString(w, "two")
	w.Flush()
	s.onData = func() {
		io.WriteString(w, "three")
		w.Flush()
		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		w.stop()
		ex.post(w.drain)
		ex.advance()
	}
	ex.advance()
	ex.advance()
	want := []string{"respond 200", "data 3"}
	if got := s.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ops %q, want %q", got, want)
	}
	w.mu.Lock()
	pending, draining := w.pending, w.draining
	w.mu.Unlock()
	if pending != 0 || draining {
		t.Fatalf("after stop: pending %d (want 0), draining %v", pending, draining)
	}
	if _, err := w.Write([]byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write after stop: %v, want context.Canceled", err)
	}
}
