package ipc

import (
	"context"
	"encoding/json"
	"sync"
)

// In-process clients: Go code in the same process (a Go-drawn UI, or a Go
// panel next to a page) uses the services registered with Handle,
// HandleStream and HandleDuplex without going through a page connection.
// Streams run on the same Stream state machine as page streams; only the
// peer differs (localPeer hands frames and control messages straight to
// the other end: no JSON envelope, no base64, one copy per byte frame).

// localPeer delivers to the other end of an in-process stream
// synchronously. That is safe: Stream calls its peer outside its state
// mutex, and the other end's onFrame/onCtl take only that end's state mutex
// and never block, so neither end can wait on the other's locks.
//
// localPeer does not implement bytesPreparer, so WriteBytes hands it a
// private copy of the raw bytes, which the receiving end may keep.
type localPeer struct{ to *Stream }

func (p *localPeer) sendFrame(_ uint64, f frame) { p.to.onFrame(f) }
func (p *localPeer) sendCtl(_ uint64, c ctl)     { p.to.onCtl(c) }

// Open opens a stream to the handler registered with HandleStream or
// HandleDuplex for method, in process, and returns the client end. args
// are JSON-encoded as the argument array, as a page's would be. The client
// end has the same methods and semantics as the serving end (Write,
// WriteBytes, Receive*, CloseSend, Close, CloseWithError, Context); for a
// HandleStream handler, Write* returns an *Error with CodeBadArgs ("stream
// does not accept data"). Both directions use SetStreamWindow's window.
//
// Open returns once the stream is accepted, which in process is at once.
// An unknown method returns an *Error with CodeNotFound. Canceling ctx
// cancels the stream (both ends see CodeCanceled), whether before or after
// Open returns; a ctx already done when Open is called returns its cause
// as an *Error and starts nothing.
//
// The handler cannot tell an in-process client from a page, except that
// ConnFrom reports ok=false: in-process streams belong to no connection,
// so Drop and page reloads do not affect them. A panic in the handler is
// reported through OnPanic with a nil Conn (or logged) and fails the
// stream with CodeInternal.
//
// Open, Receive* and Write* may block; do not call them on the UI thread.
//
// The caller must end the stream: Close it (or read to the end and let
// the handler close it) or cancel ctx. An abandoned stream is never
// collected: its handler goroutine keeps running, blocked on the stream.
func (r *Router) Open(ctx context.Context, method string, args ...any) (*Stream, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, toWireError(err)
	}
	r.mu.Lock()
	h, ok := r.streams[method]
	var id uint64
	if ok {
		// The client opens, so it takes odd ids as runtime-js does. The
		// counter is the Router's own: no page connection shares it.
		r.localID += 2
		id = r.localID - 1
	}
	r.mu.Unlock()
	if !ok {
		return nil, NewError(CodeNotFound, "stream not found: "+method)
	}
	rawArgs, err := marshalArgs(args)
	if err != nil {
		return nil, err
	}
	win := r.streamWindow()
	cp, sp := &localPeer{}, &localPeer{}
	lc := &localCancel{}
	client := newStream(context.Background(), cp, id, win, win, lc.release)
	// The serving end's parent is Background: no connection in its
	// context (ConnFrom ok=false), nothing to drop.
	server := newStream(context.Background(), sp, id, win, win, nil)
	cp.to, sp.to = server, client
	if h.duplex == nil {
		server.setSendOnly() // a StreamFunc only sends; set before publishing
	}
	server.p.sendCtl(id, ctl{op: "accept", in: server.in})
	lc.set(context.AfterFunc(ctx, func() { client.cancelStream() }))
	go r.runStream(nil, method, h, server, rawArgs)
	return client, nil
}

// cancelStream abandons the stream from this end, as runtime-js's cancel
// does: it finishes this end with CodeCanceled and tells the peer, unless
// the stream already finished.
func (s *Stream) cancelStream() {
	if s.finish(NewError(CodeCanceled, "stream canceled")) {
		s.p.sendCtl(s.id, ctl{op: "cancel"})
	}
}

// localCancel unregisters an in-process stream's ctx watch (the
// context.AfterFunc) once the stream finishes, whichever of the two
// happens first.
type localCancel struct {
	mu   sync.Mutex
	stop func() bool
	done bool
}

func (l *localCancel) set(stop func() bool) {
	l.mu.Lock()
	done := l.done
	if !done {
		l.stop = stop
	}
	l.mu.Unlock()
	if done {
		stop()
	}
}

// release is the client end's release function.
func (l *localCancel) release(*Stream) {
	l.mu.Lock()
	l.done = true
	stop := l.stop
	l.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// Call calls the handler registered with Handle for method, in process,
// and returns the JSON encoding of its result. args are JSON-encoded as
// the argument array, as a page's would be. Every error is an *Error with
// the code a page would see: CodeNotFound for an unknown method, the
// handler's *Error code, CodeInternal for other errors and panics,
// CodeCanceled when ctx is canceled.
//
// The handler's context is canceled when ctx is; it carries no connection
// (ConnFrom reports ok=false) and none of ctx's values. If ctx is canceled
// Call returns at once, even if the handler is still running. Call blocks
// until then; do not call it on the UI thread.
func (r *Router) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	r.mu.Lock()
	h := r.handlers[method]
	r.mu.Unlock()
	if h == nil {
		return nil, NewError(CodeNotFound, "method not found: "+method)
	}
	rawArgs, err := marshalArgs(args)
	if err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, toWireError(err)
	}
	hctx, cancel := context.WithCancelCause(context.Background())
	stop := context.AfterFunc(ctx, func() { cancel(context.Cause(ctx)) })
	defer stop()
	type result struct {
		raw json.RawMessage
		err *Error
	}
	done := make(chan result, 1)
	go func() {
		defer cancel(nil)
		raw, err := r.invoke(hctx, nil, method, h, rawArgs)
		done <- result{raw, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res.raw, nil
	case <-ctx.Done():
		return nil, toWireError(context.Cause(ctx))
	}
}

// marshalArgs encodes in-process call/stream arguments as the JSON array a
// page would send.
func marshalArgs(args []any) (json.RawMessage, error) {
	if args == nil {
		args = []any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, toWireError(err)
	}
	return raw, nil
}
