package ipc

import (
	"context"
	"sync"
)

// Stream is the producer side of one stream: the writer handed to a
// StreamFunc, or the value Router.OpenStream returns for Go-opened streams.
type Stream struct {
	cs     *connState
	id     uint64
	ctx    context.Context
	cancel context.CancelCauseFunc

	writeMu sync.Mutex // serializes Write/Close so sends stay ordered

	mu     sync.Mutex
	sent   int
	acked  int
	closed bool
	wake   chan struct{} // signaled on ack
}

func newStream(cs *connState, id uint64) *Stream {
	ctx, cancel := context.WithCancelCause(cs.ctx)
	return &Stream{cs: cs, id: id, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
}

// ID returns the stream id on the wire.
func (s *Stream) ID() uint64 { return s.id }

// Context is canceled when the consumer cancels, the connection is dropped
// or the stream is closed.
func (s *Stream) Context() context.Context { return s.ctx }

// Write JSON-encodes v and sends it as one data message, blocking while
// DefaultWindow messages are unacknowledged.
func (s *Stream) Write(v any) error {
	raw, err := marshalValue(v)
	if err != nil {
		return err
	}
	msg := encode(streamData{Tango: "stream", ID: s.id, Op: "data", Data: raw})

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for {
		if err := context.Cause(s.ctx); err != nil {
			return err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return NewError(CodeCanceled, "stream closed")
		}
		if s.sent-s.acked < DefaultWindow {
			s.sent++
			s.mu.Unlock()
			s.cs.send(msg)
			return nil
		}
		s.mu.Unlock()
		select {
		case <-s.ctx.Done():
		case <-s.wake:
		}
	}
}

// Close ends the stream normally. It is a no-op once the stream is closed
// or canceled.
func (s *Stream) Close() error { return s.close(nil) }

// CloseWithError fails the stream; the consumer sees err (an *Error keeps
// its code, others become "internal").
func (s *Stream) CloseWithError(err error) error {
	if err == nil {
		return s.Close()
	}
	return s.close(err)
}

func (s *Stream) close(err error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !s.markClosed() {
		return nil
	}
	if s.ctx.Err() == nil {
		if err == nil {
			s.cs.send(encode(streamCtl{Tango: "stream", ID: s.id, Op: "end"}))
		} else {
			s.cs.send(encode(streamCtl{Tango: "stream", ID: s.id, Op: "error", Error: toWireError(err)}))
		}
	}
	s.finish()
	return nil
}

func (s *Stream) markClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	return true
}

// finish releases the stream without sending anything.
func (s *Stream) finish() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cs.removeStream(s)
	s.cancel(NewError(CodeCanceled, "stream closed"))
}

func (s *Stream) ack(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	s.acked += n
	if s.acked > s.sent {
		s.acked = s.sent
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// abort is a cancellation by the consumer.
func (s *Stream) abort(cause error) {
	s.cancel(cause)
	s.cs.removeStream(s)
}
