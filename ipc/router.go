package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/tangoui/tango/internal/corehook"
)

// Conn is one page connection. Send delivers a raw message to the page; it
// must be safe from any goroutine and should not block. Conn values are used
// as map keys, so they must be comparable (typically a pointer).
type Conn interface {
	Send(msg string)
}

// HandlerFunc serves a call. args is the raw JSON array of arguments ("[]"
// when none were sent). The returned value is JSON-encoded as the result.
// ctx is canceled when the connection is dropped.
type HandlerFunc func(ctx context.Context, args json.RawMessage) (any, error)

// StreamFunc serves a stream opened by JS. Values written to w are delivered
// in order. Returning nil ends the stream; returning an error fails it. ctx
// is canceled when JS cancels the stream or the connection is dropped.
type StreamFunc func(ctx context.Context, args json.RawMessage, w StreamWriter) error

// StreamWriter is the producer side of a stream.
type StreamWriter interface {
	// Write JSON-encodes v and sends it. It blocks while DefaultWindow
	// messages are unacknowledged, and returns context.Cause of the stream
	// context once the stream is canceled or the connection dropped.
	Write(v any) error
	// Context is canceled when the consumer cancels or the connection drops.
	Context() context.Context
}

// EventFunc receives an event sent by JS.
type EventFunc func(c Conn, data json.RawMessage)

// Router dispatches ipc messages. The zero value is ready to use.
type Router struct {
	mu       sync.Mutex
	handlers map[string]HandlerFunc
	streams  map[string]StreamFunc
	events   map[string][]*eventSub
	conns    map[Conn]*connState
	onPanic  func(c Conn, where string, value any, stack []byte)
}

type eventSub struct{ fn EventFunc }

// NewRouter returns an empty Router.
func NewRouter() *Router { return &Router{} }

// OnPanic sets fn to be told about a panic recovered in a call or stream
// handler, on the handler's goroutine. where is "binding Name" or "stream
// Name". The panic still becomes a CodeInternal error for the caller;
// reporting cannot change that, and nothing exits the process.
//
// Without fn, a panic on a connection that is a *tango.Surface goes to that
// app's panic handling (tango.Options.OnPanic or its default, report only:
// a keepRunning false is ignored for bindings); on other connections it is
// logged with log/slog.
func (r *Router) OnPanic(fn func(c Conn, where string, value any, stack []byte)) {
	r.mu.Lock()
	r.onPanic = fn
	r.mu.Unlock()
}

// reportPanic reports a recovered handler panic once; see OnPanic.
func (r *Router) reportPanic(c Conn, where string, value any, stack []byte) {
	defer func() { // a panicking OnPanic function must not crash the process
		if p := recover(); p != nil {
			slog.Error("ipc: panic in OnPanic", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		}
	}()
	r.mu.Lock()
	fn := r.onPanic
	r.mu.Unlock()
	switch {
	case fn != nil:
		fn(c, where, value, stack)
	case corehook.ReportPanic != nil && corehook.ReportPanic(c, where, value, stack):
	default:
		slog.Error("ipc: panic recovered", "where", where, "panic", fmt.Sprint(value), "stack", string(stack))
	}
}

// Handle registers fn for calls to name, replacing any previous handler.
func (r *Router) Handle(name string, fn HandlerFunc) {
	if name == "" || fn == nil {
		panic("ipc: Handle: empty name or nil handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[string]HandlerFunc)
	}
	r.handlers[name] = fn
}

// HandleStream registers fn for streams JS opens with name, replacing any
// previous handler.
func (r *Router) HandleStream(name string, fn StreamFunc) {
	if name == "" || fn == nil {
		panic("ipc: HandleStream: empty name or nil handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streams == nil {
		r.streams = make(map[string]StreamFunc)
	}
	r.streams[name] = fn
}

// Has reports whether a call or stream handler is registered for name.
func (r *Router) Has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, a := r.handlers[name]
	_, b := r.streams[name]
	return a || b
}

// On subscribes fn to events named name sent by JS. Handlers run
// synchronously inside HandleMessage (usually the main thread), so they must
// not block. The returned function unsubscribes.
func (r *Router) On(name string, fn EventFunc) (off func()) {
	sub := &eventSub{fn: fn}
	r.mu.Lock()
	if r.events == nil {
		r.events = make(map[string][]*eventSub)
	}
	r.events[name] = append(r.events[name], sub)
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			subs := r.events[name]
			for i, s := range subs {
				if s == sub {
					r.events[name] = append(subs[:i:i], subs[i+1:]...)
					break
				}
			}
			if len(r.events[name]) == 0 {
				delete(r.events, name)
			}
		})
	}
}

// Emit sends an event to the page behind c.
func (r *Router) Emit(c Conn, name string, data any) error {
	raw, err := marshalValue(data)
	if err != nil {
		return err
	}
	c.Send(encode(eventOut{Tango: "event", Name: name, Data: raw}))
	return nil
}

// Drop cancels every call and stream of c and forgets it. Call it when the
// page closes. Results of calls still running are discarded. A later message
// from c starts a fresh connection state.
func (r *Router) Drop(c Conn) {
	r.mu.Lock()
	cs := r.conns[c]
	delete(r.conns, c)
	r.mu.Unlock()
	if cs != nil {
		cs.cancel(NewError(CodeCanceled, "connection closed"))
	}
}

// HandleMessage processes one raw message from c. It reports whether the
// message belonged to ipc (a JSON object with a "tango" field); other
// messages are left to the caller. It never blocks: calls and streams run
// on their own goroutines.
func (r *Router) HandleMessage(c Conn, msg string) bool {
	if !isIPC(msg) {
		return false
	}
	var m wireIn
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return true // malformed ipc message: claim and drop
	}
	switch m.Tango {
	case "hello":
		// The JS runtime (re)started in this page: anything from the
		// previous document is dead.
		r.Drop(c)
	case "call":
		r.handleCall(c, &m)
	case "event":
		r.handleEvent(c, &m)
	case "stream":
		r.handleStream(c, &m)
	}
	return true
}

// OpenStream starts a Go→JS stream: JS receives it through
// tango.onStream(method, fn). args are JSON-encoded as the argument array.
// Write the values, then Close (or CloseWithError).
func (r *Router) OpenStream(c Conn, method string, args ...any) (*Stream, error) {
	if args == nil {
		args = []any{}
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	cs := r.state(c)
	cs.mu.Lock()
	cs.nextID += 2
	id := cs.nextID
	s := newStream(cs, id)
	cs.streams[id] = s
	cs.mu.Unlock()
	c.Send(encode(streamOpen{Tango: "stream", ID: id, Op: "open", Method: method, Args: rawArgs}))
	return s, nil
}

type connKey struct{}

// ConnFrom returns the connection a call or stream handler is serving.
func ConnFrom(ctx context.Context) (Conn, bool) {
	c, ok := ctx.Value(connKey{}).(Conn)
	return c, ok
}

// connState is the per-page state. A new one is created after Drop so that
// late results from the previous page are discarded.
type connState struct {
	conn   Conn
	ctx    context.Context
	cancel context.CancelCauseFunc

	mu      sync.Mutex
	streams map[uint64]*Stream
	nextID  uint64 // last Go-allocated stream id (even)
}

func (r *Router) state(c Conn) *connState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cs := r.conns[c]; cs != nil {
		return cs
	}
	if r.conns == nil {
		r.conns = make(map[Conn]*connState)
	}
	ctx, cancel := context.WithCancelCause(context.WithValue(context.Background(), connKey{}, c))
	cs := &connState{conn: c, ctx: ctx, cancel: cancel, streams: make(map[uint64]*Stream)}
	r.conns[c] = cs
	return cs
}

// send delivers msg unless the connection was dropped.
func (cs *connState) send(msg string) {
	if cs.ctx.Err() != nil {
		return
	}
	cs.conn.Send(msg)
}

func (cs *connState) stream(id uint64) *Stream {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.streams[id]
}

func (cs *connState) removeStream(s *Stream) {
	cs.mu.Lock()
	if cs.streams[s.id] == s {
		delete(cs.streams, s.id)
	}
	cs.mu.Unlock()
}

func (r *Router) handleCall(c Conn, m *wireIn) {
	cs := r.state(c)
	r.mu.Lock()
	h := r.handlers[m.Method]
	r.mu.Unlock()
	id := m.ID
	if h == nil {
		cs.send(encode(retErr{Tango: "ret", ID: id, Error: NewError(CodeNotFound, "method not found: "+m.Method)}))
		return
	}
	args := normArgs(m.Args)
	go func() {
		ctx, cancel := context.WithCancel(cs.ctx)
		defer cancel()
		res, err := r.safeCall(c, "binding "+m.Method, func() (any, error) { return h(ctx, args) })
		if err == nil {
			var raw json.RawMessage
			if raw, err = marshalValue(res); err == nil {
				cs.send(encode(retOK{Tango: "ret", ID: id, Result: raw}))
				return
			}
			err = fmt.Errorf("encode result: %w", err)
		}
		cs.send(encode(retErr{Tango: "ret", ID: id, Error: toWireError(err)}))
	}()
}

func (r *Router) handleEvent(c Conn, m *wireIn) {
	r.mu.Lock()
	subs := append([]*eventSub(nil), r.events[m.Name]...)
	r.mu.Unlock()
	data := m.Data
	if data == nil {
		data = json.RawMessage("null")
	}
	for _, s := range subs {
		s.fn(c, data)
	}
}

func (r *Router) handleStream(c Conn, m *wireIn) {
	cs := r.state(c)
	if m.Op == "open" {
		r.openFromJS(cs, m)
		return
	}
	s := cs.stream(m.ID)
	if s == nil {
		return // finished or unknown stream: late ack/cancel is normal
	}
	switch m.Op {
	case "ack":
		s.ack(m.N)
	case "cancel":
		s.abort(NewError(CodeCanceled, "stream canceled"))
	case "error":
		// The consumer refused or failed the stream (e.g. no JS handler for
		// a Go-opened stream).
		e := m.Error
		if e == nil {
			e = NewError(CodeInternal, "stream failed")
		}
		s.abort(e)
	}
	// "data"/"end" from JS: JS-produced streams are not supported; ignore.
}

func (r *Router) openFromJS(cs *connState, m *wireIn) {
	id := m.ID
	if id%2 == 0 {
		cs.send(encode(streamCtl{Tango: "stream", ID: id, Op: "error", Error: NewError(CodeBadArgs, "stream id from JS must be odd")}))
		return
	}
	r.mu.Lock()
	h := r.streams[m.Method]
	r.mu.Unlock()
	if h == nil {
		cs.send(encode(streamCtl{Tango: "stream", ID: id, Op: "error", Error: NewError(CodeNotFound, "stream not found: "+m.Method)}))
		return
	}
	cs.mu.Lock()
	if old := cs.streams[id]; old != nil {
		cs.mu.Unlock()
		return // duplicate open; ignore
	}
	s := newStream(cs, id)
	cs.streams[id] = s
	cs.mu.Unlock()
	args := normArgs(m.Args)
	go func() {
		_, err := r.safeCall(cs.conn, "stream "+m.Method, func() (any, error) { return nil, h(s.ctx, args, s) })
		if s.ctx.Err() != nil {
			s.finish() // canceled by peer or dropped: nothing to report
			return
		}
		if err != nil {
			s.CloseWithError(err)
		} else {
			s.Close()
		}
	}()
}

func normArgs(a json.RawMessage) json.RawMessage {
	if len(a) == 0 || string(a) == "null" {
		return json.RawMessage("[]")
	}
	return a
}

// safeCall runs a handler, turning a panic into a CodeInternal error for
// the caller and reporting it (reportPanic).
func (r *Router) safeCall(c Conn, where string, fn func() (any, error)) (res any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = NewError(CodeInternal, fmt.Sprintf("panic: %v", p))
			r.reportPanic(c, where, p, debug.Stack())
		}
	}()
	return fn()
}

func toWireError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		out := *e
		if out.Code == "" {
			out.Code = CodeInternal
		}
		if out.Message == "" {
			out.Message = err.Error()
		}
		return &out
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewError(CodeCanceled, err.Error())
	}
	return NewError(CodeInternal, err.Error())
}
