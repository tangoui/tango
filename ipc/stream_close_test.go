package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// wantCode checks err is an *Error with code (and message, when non-empty).
func wantCode(t *testing.T, err error, code, msg string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code || (msg != "" && e.Message != msg) {
		t.Fatalf("want *Error{%s, %q}, got %v", code, msg, err)
	}
}

// within fails unless ch delivers within d.
func within[T any](t *testing.T, ch <-chan T, d time.Duration, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("%s: not within %v", what, d)
	}
	var zero T
	return zero
}

func TestCloseSendHalfClose(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct {
		frames  int
		err     error
		ctxDone bool
	}
	got := make(chan res, 1)
	r.HandleDuplex("H", func(s *Stream, _ json.RawMessage) error {
		s.Write(1)
		s.Write(2)
		if err := s.CloseSend(); err != nil {
			return err
		}
		var x res
		wantCode(t, s.Write(3), CodeCanceled, "stream closed")
		if s.Context().Err() != nil {
			x.ctxDone = true // half-closed only: must still be live
		}
		for {
			if _, err := s.Receive(); err != nil {
				x.err = err
				break
			}
			x.frames++
		}
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "H")
	expectData(t, c, 1, 0, 65)
	expectData(t, c, 1, 1, 65)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"end","seq":2}` {
		t.Fatalf("end %s", raw)
	}
	sendBin(t, r, c, 1, 0, []byte{1})
	sendBin(t, r, c, 1, 1, []byte{2})
	send(t, r, c, `{"tango":"stream","id":1,"op":"end","seq":2}`)
	x := within(t, got, time.Second, "handler")
	if x.err != io.EOF || x.frames != 2 || x.ctxDone {
		t.Fatalf("got %+v", x)
	}
	acked := 0
	for acked < 130 {
		if o := c.next(t); o.Op != "ack" {
			t.Fatalf("want ack, got %+v", o)
		} else {
			acked += o.B
		}
	}
	c.none(t, 50*time.Millisecond) // fully finished: no close on return
}

func TestPeerCloseStopsWrites(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct {
		frames   int
		recvErr  error
		writeErr error
		ctxDone  bool
	}
	got := make(chan res, 1)
	r.HandleDuplex("P", func(s *Stream, _ json.RawMessage) error {
		var x res
		for {
			if _, err := s.Receive(); err != nil {
				x.recvErr = err
				break
			}
			x.frames++
		}
		x.writeErr = s.Write(1)
		select {
		case <-s.Context().Done():
			x.ctxDone = true
		case <-time.After(time.Second):
		}
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "P")
	sendBin(t, r, c, 1, 0, []byte{1})
	send(t, r, c, `{"tango":"stream","id":1,"op":"close","seq":1}`)
	x := within(t, got, 2*time.Second, "handler")
	if x.frames != 1 || x.recvErr != io.EOF || !x.ctxDone {
		t.Fatalf("got %+v", x)
	}
	wantCode(t, x.writeErr, CodeCanceled, "stream closed by peer")
	// The read may return its credit; nothing else follows (the peer
	// closed, so no close is sent back).
	for {
		select {
		case m := <-c.ch:
			if m != `{"tango":"stream","id":1,"op":"ack","b":65}` {
				t.Fatalf("unexpected %s", m)
			}
			continue
		case <-time.After(50 * time.Millisecond):
		}
		break
	}
}

func TestPeerErrorPropagates(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct{ recvErr, writeErr error }
	got := make(chan res, 1)
	r.HandleDuplex("E", func(s *Stream, _ json.RawMessage) error {
		var x res
		_, x.recvErr = s.Receive()
		x.writeErr = s.Write(1)
		got <- x
		return x.recvErr
	})
	openDuplex(t, r, c, 1, "E")
	send(t, r, c, `{"tango":"stream","id":1,"op":"error","seq":0,"error":{"code":"internal","message":"x"}}`)
	x := within(t, got, time.Second, "handler")
	wantCode(t, x.recvErr, CodeInternal, "x")
	wantCode(t, x.writeErr, CodeInternal, "x")
	c.none(t, 50*time.Millisecond)
}

func TestPeerErrorAfterFramesDeliversThem(t *testing.T) {
	// An error with seq is ordered like end/close: the frames before it
	// are delivered first, then the peer's error; writes fail with it at
	// once; the context is canceled (cause: the peer's error) only after
	// the reader reached it.
	r := NewRouter()
	c := newFake()
	gotErr := make(chan struct{})
	readNow := make(chan struct{})
	type res struct {
		lines            []string
		writeErr, ctxErr error
		ctxLiveBefore    bool
	}
	got := make(chan res, 1)
	r.HandleDuplex("E", func(s *Stream, _ json.RawMessage) error {
		<-gotErr
		var x res
		x.writeErr = s.Write("late") // send direction stopped already
		x.ctxLiveBefore = s.Context().Err() == nil
		<-readNow
		for {
			m, err := s.Receive()
			x.lines = append(x.lines, describe(m, err))
			if err != nil {
				break
			}
		}
		_, again := s.Receive()
		x.lines = append(x.lines, "again "+describeErr(again))
		<-s.Context().Done()
		x.ctxErr = context.Cause(s.Context())
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "E")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":65,"data":1}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":65,"data":2}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"error","seq":2,"error":{"code":"x","message":"boom"}}`)
	close(gotErr)
	close(readNow)
	x := within(t, got, time.Second, "handler")
	want := []string{"json 1", "json 2", `error x "boom"`, `again error x "boom"`}
	if !reflect.DeepEqual(x.lines, want) {
		t.Fatalf("received %q, want %q", x.lines, want)
	}
	wantCode(t, x.writeErr, "x", "boom")
	if !x.ctxLiveBefore {
		t.Fatal("context canceled before the frames were read")
	}
	wantCode(t, x.ctxErr, "x", "boom")
	// Nothing goes back: no close/error after the peer's error (acks for
	// the frames read are fine).
	for {
		select {
		case m := <-c.ch:
			if !strings.Contains(m, `"op":"ack"`) {
				t.Fatalf("unexpected message after peer error: %s", m)
			}
			continue
		case <-time.After(50 * time.Millisecond):
		}
		break
	}
	if r.state(c).stream(1) != nil {
		t.Fatal("stream not released")
	}
}

func TestPeerErrorWithoutSeqAborts(t *testing.T) {
	// An error without seq (a protocol-level failure) aborts at once:
	// queued frames are dropped.
	r := NewRouter()
	c := newFake()
	gotErr := make(chan struct{})
	got := make(chan error, 1)
	r.HandleDuplex("E", func(s *Stream, _ json.RawMessage) error {
		<-gotErr
		_, err := s.Receive()
		got <- err
		return nil
	})
	openDuplex(t, r, c, 1, "E")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":65,"data":1}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"error","error":{"code":"x","message":"boom"}}`)
	close(gotErr)
	wantCode(t, within(t, got, time.Second, "Receive"), "x", "boom")
	c.none(t, 50*time.Millisecond)
}

func TestPeerErrorFrameBeyondSeq(t *testing.T) {
	// A frame at or beyond the error's seq is a protocol violation: the
	// stream fails with bad_args (nothing is sent back: the peer stopped).
	r := NewRouter()
	c := newFake()
	got := make(chan error, 1)
	r.HandleDuplex("E", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done()
		got <- context.Cause(s.Context())
		return nil
	})
	openDuplex(t, r, c, 1, "E")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":65,"data":1}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"error","seq":1,"error":{"code":"x","message":"boom"}}`)
	wantBadArgs(t, within(t, got, time.Second, "context"))
	c.none(t, 50*time.Millisecond)
}

// blockedDuplex is a handler whose second goroutine fills the 4096-byte
// send window with 1000-byte frames (three fit), while the handler blocks
// in Receive. Both results are reported; the handler returns only after
// the writer has.
func blockedDuplex(recvErr, writeErr chan<- error) DuplexFunc {
	return func(s *Stream, _ json.RawMessage) error {
		wdone := make(chan struct{})
		go func() {
			defer close(wdone)
			for {
				if err := s.WriteBytes(make([]byte, 1000)); err != nil {
					writeErr <- err
					return
				}
			}
		}()
		_, err := s.Receive()
		recvErr <- err
		<-wdone
		return err
	}
}

func TestPeerCancelUnblocks(t *testing.T) {
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	recvErr, writeErr := make(chan error, 1), make(chan error, 1)
	r.HandleDuplex("B", blockedDuplex(recvErr, writeErr))
	openDuplex(t, r, c, 1, "B")
	for i := uint64(0); i < 3; i++ {
		expectData(t, c, 1, i, 1064)
	}
	c.none(t, 50*time.Millisecond) // the writer is blocked on credit
	send(t, r, c, `{"tango":"stream","id":1,"op":"cancel"}`)
	wantCode(t, within(t, recvErr, time.Second, "Receive"), CodeCanceled, "")
	wantCode(t, within(t, writeErr, time.Second, "WriteBytes"), CodeCanceled, "")
	c.none(t, 50*time.Millisecond)
}

func TestDropUnblocksDuplex(t *testing.T) {
	base := runtime.NumGoroutine()
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	recvErr, writeErr := make(chan error, 1), make(chan error, 1)
	r.HandleDuplex("B", blockedDuplex(recvErr, writeErr))
	openDuplex(t, r, c, 1, "B")
	for i := uint64(0); i < 3; i++ {
		expectData(t, c, 1, i, 1064)
	}
	c.none(t, 50*time.Millisecond)
	send(t, r, c, `{"tango":"hello","proto":2}`) // page reload
	if o := c.next(t); o.Tango != "hello" {
		t.Fatalf("want hello reply, got %+v", o)
	}
	wantCode(t, within(t, recvErr, time.Second, "Receive"), CodeCanceled, "")
	wantCode(t, within(t, writeErr, time.Second, "WriteBytes"), CodeCanceled, "")
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > base+2 {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: %d, baseline %d", runtime.NumGoroutine(), base)
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.none(t, 50*time.Millisecond)
}

func TestGoOpenedDuplex(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	send(t, r, c, `{"tango":"stream","id":2,"op":"accept","in":true}`)
	send(t, r, c, `{"tango":"stream","id":2,"op":"data","seq":0,"c":65,"bin":"AQ=="}`)
	b, err := s.ReceiveBytes()
	if err != nil || len(b) != 1 || b[0] != 1 {
		t.Fatalf("ReceiveBytes %v %v", b, err)
	}
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"ack","b":65}` {
		t.Fatalf("ack %s", raw)
	}
	s.Close()
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"close","seq":0}` {
		t.Fatalf("close %s", raw)
	}
	within(t, s.Context().Done(), time.Second, "Context after Close")
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("Receive after Close: %v", err)
	}
	c.none(t, 20*time.Millisecond)
}

func TestGoOpenedWriteBeforeAccept(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	if err := s.Write(1); err != nil {
		t.Fatal(err)
	}
	expectData(t, c, 2, 0, 65)
	send(t, r, c, `{"tango":"stream","id":2,"op":"accept","in":false}`)
	wantCode(t, s.Write(2), CodeBadArgs, "stream does not accept data")
	s.Close()
	if o := c.next(t); o.Op != "close" || o.Seq != 1 {
		t.Fatalf("got %+v", o)
	}
}

func TestDuplexPanic(t *testing.T) {
	r := NewRouter()
	wheres := make(chan string, 2)
	r.OnPanic(func(_ Conn, where string, _ any, _ []byte) { wheres <- where })
	r.HandleDuplex("Panicky", func(s *Stream, _ json.RawMessage) error { panic("p") })
	c := newFake()
	openDuplex(t, r, c, 1, "Panicky")
	if o := c.next(t); o.Op != "error" || o.Error == nil || o.Error.Code != CodeInternal {
		t.Fatalf("got %+v", o)
	}
	if w := within(t, wheres, time.Second, "OnPanic"); w != "stream Panicky" {
		t.Fatalf("where = %q", w)
	}
	c.none(t, 20*time.Millisecond)
}

func TestCloseWhileWriteBlocked(t *testing.T) {
	// R1: Close from another goroutine does not wait for credit; the
	// blocked Write fails without sending and close counts what was sent.
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	for i := uint64(0); i < 3; i++ {
		if err := s.WriteBytes(make([]byte, 1000)); err != nil {
			t.Fatal(err)
		}
		expectData(t, c, 2, i, 1064)
	}
	blocked := make(chan error, 1)
	go func() { blocked <- s.WriteBytes(make([]byte, 1000)) }()
	c.none(t, 50*time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	if err := within(t, closed, time.Second, "Close"); err != nil {
		t.Fatal(err)
	}
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"close","seq":3}` {
		t.Fatalf("close %s", raw)
	}
	wantCode(t, within(t, blocked, time.Second, "blocked WriteBytes"), CodeCanceled, "stream closed")
	c.none(t, 50*time.Millisecond)
}

func TestCloseSendWhileWriteBlocked(t *testing.T) {
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	for i := uint64(0); i < 3; i++ {
		s.WriteBytes(make([]byte, 1000))
		expectData(t, c, 2, i, 1064)
	}
	blocked := make(chan error, 1)
	go func() { blocked <- s.WriteBytes(make([]byte, 1000)) }()
	c.none(t, 50*time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- s.CloseSend() }()
	within(t, done, time.Second, "CloseSend")
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"end","seq":3}` {
		t.Fatalf("end %s", raw)
	}
	wantCode(t, within(t, blocked, time.Second, "blocked WriteBytes"), CodeCanceled, "stream closed")
	if s.Context().Err() != nil {
		t.Fatal("half-closed stream's context canceled")
	}
	// Close after CloseSend still tells the peer this side stops receiving.
	s.Close()
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"close","seq":3}` {
		t.Fatalf("close %s", raw)
	}
	within(t, s.Context().Done(), time.Second, "Context after Close")
}

func TestLocalCloseDropsInbound(t *testing.T) {
	// R3: after a local Close, Receive returns io.EOF; queued and later
	// frames are dropped without returning credit.
	r := NewRouter()
	c := newFake()
	closed := make(chan struct{})
	got := make(chan error, 1)
	r.HandleDuplex("C", func(s *Stream, _ json.RawMessage) error {
		<-closed
		s.Close()
		_, err := s.Receive()
		got <- err
		return nil
	})
	openDuplex(t, r, c, 1, "C")
	sendBin(t, r, c, 1, 0, []byte{1}) // queued, never read
	close(closed)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"close","seq":0}` {
		t.Fatalf("close %s", raw)
	}
	if err := within(t, got, time.Second, "Receive"); err != io.EOF {
		t.Fatalf("Receive after Close: %v", err)
	}
	sendBin(t, r, c, 1, 1, []byte{2})
	c.none(t, 50*time.Millisecond) // no ack, no error
}

func TestCloseWithErrorFailsBoth(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	recvErr := make(chan error, 1)
	go func() { _, err := s.Receive(); recvErr <- err }()
	time.Sleep(20 * time.Millisecond)
	s.CloseWithError(NewError(CodeBadArgs, "nope"))
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":2,"op":"error","seq":0,"error":{"message":"nope","code":"bad_args"}}` {
		t.Fatalf("error %s", raw)
	}
	if err := within(t, recvErr, time.Second, "Receive"); err == nil || err == io.EOF {
		t.Fatalf("Receive after CloseWithError: %v", err)
	}
	if s.Write(1) == nil {
		t.Fatal("Write after CloseWithError succeeded")
	}
	within(t, s.Context().Done(), time.Second, "Context")
	s.Close() // no-op
	c.none(t, 20*time.Millisecond)
}

func TestStreamFuncEndValidated(t *testing.T) {
	// R4: an in:false stream validates end like a receiving one.
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	r.HandleStream("F", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		<-ctx.Done()
		return nil
	})
	// Frame after end.
	openJS(t, r, c, 1, "F")
	send(t, r, c, `{"tango":"stream","id":1,"op":"end","seq":0}`)
	sendBin(t, r, c, 1, 0, []byte{1})
	if o := c.next(t); o.ID != 1 || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
		t.Fatalf("frame after end: %+v", o)
	}
	// End below frames already received.
	openJS(t, r, c, 3, "F")
	sendBin(t, r, c, 3, 0, []byte{1})
	sendBin(t, r, c, 3, 1, []byte{1})
	for i := 0; i < 2; i++ {
		if o := c.next(t); o.Op != "ack" {
			t.Fatalf("want ack, got %+v", o)
		}
	}
	send(t, r, c, `{"tango":"stream","id":3,"op":"end","seq":1}`)
	if o := c.next(t); o.ID != 3 || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
		t.Fatalf("end below frames: %+v", o)
	}
	// A consistent end is fine.
	openJS(t, r, c, 5, "F")
	sendBin(t, r, c, 5, 0, []byte{1})
	c.next(t) // ack
	send(t, r, c, `{"tango":"stream","id":5,"op":"end","seq":1}`)
	c.none(t, 50*time.Millisecond)
	r.Drop(c)
}

// recPeer records control ops sent by a Stream.
type recPeer struct {
	mu  sync.Mutex
	ops []string
}

func (p *recPeer) sendFrame(uint64, frame) {}
func (p *recPeer) sendCtl(_ uint64, c ctl) {
	p.mu.Lock()
	p.ops = append(p.ops, c.op)
	p.mu.Unlock()
}

func TestStreamReleaseOnce(t *testing.T) {
	var released atomic.Int32
	p := &recPeer{}
	s := newStream(context.Background(), p, 2, 4096, 4096, func(*Stream) { released.Add(1) })
	s.CloseSend()
	s.onCtl(ctl{op: "end", seq: 0})
	if s.Context().Err() != nil || released.Load() != 0 {
		t.Fatal("finished before the receive direction ended")
	}
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("Receive: %v", err)
	}
	if s.Context().Err() == nil || released.Load() != 1 {
		t.Fatalf("not finished: ctx %v, released %d", s.Context().Err(), released.Load())
	}
	s.Close()
	s.CloseWithError(errors.New("x"))
	s.onCtl(ctl{op: "cancel"})
	s.onCtl(ctl{op: "error"})
	s.finish(errors.New("y"))
	if released.Load() != 1 {
		t.Fatalf("released %d times", released.Load())
	}
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("Receive after finish: %v", err)
	}
	if fmt.Sprint(p.ops) != "[end]" {
		t.Fatalf("ops %v", p.ops)
	}
}

func TestDropReleasesGoOpenedStreams(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	cs := r.state(c)
	r.Drop(c)
	within(t, s.Context().Done(), time.Second, "Context after Drop")
	cs.mu.Lock()
	n := len(cs.streams)
	cs.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d streams still registered after Drop", n)
	}
	wantCode(t, s.Write(1), CodeCanceled, "connection closed")
}
