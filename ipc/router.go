package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/internal/corehook"
)

// Conn is one page connection. Send delivers a raw message to the page; it
// must be safe from any goroutine and should not block. Conn values are used
// as map keys, so they must be comparable (typically a pointer).
type Conn interface {
	Send(msg string)
}

// BinaryConn is a Conn with fast lanes: side channels that carry stream
// byte frames as binary packets (see the package comment) instead of JSON
// messages. *tango.Surface implements it. The Router finds it with a type
// assertion on the Conn. Feed a connection to one Router only: every hello a
// Router handles starts a new lane session, so two Routers on one surface
// would invalidate each other's lanes (unsupported).
type BinaryConn interface {
	Conn
	// LaneHello returns the JSON embedded in the "lanes" field of
	// the hello reply; nil means this page has no fast lanes and the field
	// is left out. The Router calls it once per hello from runtime-js,
	// after dropping the previous document's state.
	LaneHello() json.RawMessage
	// SendBinary hands one packet to the downstream lane. It must not
	// block; false means the lane is unavailable and the caller sends the
	// frames as messages instead. On true the lane owns packet (the caller
	// never touches it again); on false it must neither keep nor modify it.
	// The Router calls it on a stream's writing goroutine with no ipc lock
	// held other than that stream's send lock.
	SendBinary(packet []byte) bool
	// BinaryMinSize is the smallest byte payload, in bytes, worth sending
	// on the downstream lane (smaller frames go as messages, for latency).
	// A negative value means there is no downstream lane.
	BinaryMinSize() int
}

// SessionBinaryConn binds sends to an immutable document generation. Reusable
// surfaces must implement it; simple in-process BinaryConns may omit it.
// BinarySession is captured once when connState is published, never per send.
// Upstream packets for it go through Router.HandleBinarySession (HandleBinary
// drops them). One Router per surface: two Routers on one surface are
// unsupported.
type SessionBinaryConn interface {
	BinarySession() (generation uint64, transport BinaryConn)
	// DropBinarySession retires the transport identity, even before LaneHello.
	// Called under Router.mu: must not invoke callbacks or reenter Router.
	DropBinarySession()
}

// laneHello returns c's "lanes" hello field, or nil when c has no lanes.
func laneHello(c Conn) json.RawMessage {
	bc, ok := c.(BinaryConn)
	if !ok {
		return nil
	}
	raw := bc.LaneHello()
	if raw == nil {
		return nil
	}
	if !json.Valid(raw) {
		slog.Warn("ipc: LaneHello returned invalid JSON; fast lanes disabled for this page")
		return nil
	}
	return raw
}

// HandlerFunc serves a call. args is the raw JSON array of arguments ("[]"
// when none were sent). The returned value is JSON-encoded as the result.
// ctx is canceled when the connection is dropped.
type HandlerFunc func(ctx context.Context, args json.RawMessage) (any, error)

// StreamFunc serves a stream opened by JS. Values written to w are delivered
// in order. Returning nil ends the stream; returning an error fails it. ctx
// is canceled when JS cancels or closes the stream or the connection is
// dropped.
type StreamFunc func(ctx context.Context, args json.RawMessage, w StreamWriter) error

// StreamWriter is the producer side of a stream.
type StreamWriter interface {
	// Write JSON-encodes v and sends it. It blocks while the consumer's
	// byte window is full (see Stream.Write), and returns context.Cause of
	// the stream context once the stream is canceled or the connection
	// dropped.
	Write(v any) error
	// Context is canceled when the consumer cancels or the connection drops.
	Context() context.Context
}

// EventFunc receives an event sent by JS.
type EventFunc func(c Conn, data json.RawMessage)

// DuplexFunc serves a bidirectional stream opened by JS: it reads with
// s.Receive* and writes with s.Write*, concurrently if it likes. args is
// the raw JSON argument array. Returning nil closes the stream; returning
// an error fails it. s.Context() is canceled when the stream ends or the
// connection drops.
type DuplexFunc func(s *Stream, args json.RawMessage) error

// streamHandler is a registered stream handler: exactly one field is set.
type streamHandler struct {
	fn     StreamFunc
	duplex DuplexFunc
}

// Router dispatches ipc messages. The zero value is ready to use.
type Router struct {
	mu       sync.Mutex
	handlers map[string]HandlerFunc
	streams  map[string]streamHandler
	events   map[string][]*eventSub
	conns    map[Conn]*connState
	onPanic  func(c Conn, where string, value any, stack []byte)
	window   int // stream receive window in bytes; 0 means DefaultWindowBytes

	jsSendWin  int           // tests only (setJSSendWindow); 0 means DefaultWindowBytes
	gapTimeout time.Duration // tests only (setGapTimeout); 0 means GapTimeout

	localID uint64 // last in-process stream id (odd; see Open)
	// goIDs carries a Conn's last Go stream id across the hello transition
	// (see drop); an entry lives only until c's next state is created.
	goIDs map[Conn]uint64

	warnedHandleBinary atomic.Bool // HandleBinary on a SessionBinaryConn was logged
}

type eventSub struct{ fn EventFunc }

// NewRouter returns an empty Router.
func NewRouter() *Router { return &Router{} }

// SetStreamWindow sets Go's stream receive window in bytes, advertised to
// runtime-js in the hello reply. The default is DefaultWindowBytes. It
// panics if n < 4096.
//
// Call it before pages load (and before in-process streams open), typically
// right after NewRouter. A page uses the window from its hello reply for
// all its streams, so lowering the window after a page's hello lets that
// page send more than Go now accepts, which fails its streams with
// "flow control violated".
func (r *Router) SetStreamWindow(n int) {
	if n < 4096 {
		panic("ipc: SetStreamWindow: window must be at least 4096 bytes")
	}
	r.mu.Lock()
	r.window = n
	r.mu.Unlock()
}

// streamWindow returns Go's stream receive window in bytes.
func (r *Router) streamWindow() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.window == 0 {
		return DefaultWindowBytes
	}
	return r.window
}

// jsSendWindow returns the send window toward runtime-js, which does not
// advertise its receive window: always DefaultWindowBytes outside tests.
func (r *Router) jsSendWindow() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jsSendWin == 0 {
		return DefaultWindowBytes
	}
	return r.jsSendWin
}

// streamGapTimeout returns the gap timeout for new streams: always
// GapTimeout outside tests.
func (r *Router) streamGapTimeout() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gapTimeout == 0 {
		return GapTimeout
	}
	return r.gapTimeout
}

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
// previous stream handler (StreamFunc or DuplexFunc) for name. The stream
// only sends: Go accepts it with in:false, and data JS sends anyway is
// dropped (its credit is still returned).
func (r *Router) HandleStream(name string, fn StreamFunc) {
	if name == "" || fn == nil {
		panic("ipc: HandleStream: empty name or nil handler")
	}
	r.setStream(name, streamHandler{fn: fn})
}

// HandleDuplex registers fn for bidirectional streams JS opens with name,
// replacing any previous stream handler (StreamFunc or DuplexFunc) for
// name. Go accepts the stream with in:true.
func (r *Router) HandleDuplex(name string, fn DuplexFunc) {
	if name == "" || fn == nil {
		panic("ipc: HandleDuplex: empty name or nil handler")
	}
	r.setStream(name, streamHandler{duplex: fn})
}

func (r *Router) setStream(name string, h streamHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streams == nil {
		r.streams = make(map[string]streamHandler)
	}
	r.streams[name] = h
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
func (r *Router) Drop(c Conn) { r.drop(c, true, false) }

// drop retires c's connection state. keepIDs (the hello transition) carries
// the last Go stream id over to c's next state, so Go ids are never reused
// while the page lives: a late open or cancel about a reaped stream can then
// never name a stream of the new document. Drop (the page closed) forgets
// it, so a closed Conn leaves nothing behind in the Router.
func (r *Router) drop(c Conn, retireTransport, keepIDs bool) {
	r.mu.Lock()
	if retireTransport {
		if scoped, ok := c.(SessionBinaryConn); ok {
			scoped.DropBinarySession()
		}
	}
	cs := r.conns[c]
	delete(r.conns, c)
	if cs != nil {
		// Lock order Router.mu → connState.mu. reaped stops OpenStream
		// from allocating on cs after its last id was carried over.
		cs.mu.Lock()
		cs.reaped = true
		last := cs.nextID
		cs.mu.Unlock()
		if keepIDs && last > r.goIDs[c] {
			if r.goIDs == nil {
				r.goIDs = make(map[Conn]uint64)
			}
			r.goIDs[c] = last
		}
	}
	if !keepIDs {
		delete(r.goIDs, c)
	}
	r.mu.Unlock()
	if cs == nil {
		return
	}
	cs.cancel(NewError(CodeCanceled, "connection closed"))
	// Every stream is aborted: finish each one so it is released exactly
	// once now (a JS-opened stream's handler finishing it again later is a
	// no-op). release takes cs.mu, so snapshot first.
	cs.mu.Lock()
	ss := make([]*Stream, 0, len(cs.streams))
	for _, s := range cs.streams {
		ss = append(ss, s)
	}
	cs.mu.Unlock()
	for _, s := range ss {
		s.finish(context.Cause(s.ctx))
	}
	if !retireTransport {
		// The hello transition (see HandleMessage) reaps state a Go
		// opener published after the new document said hello: that
		// document may already have the open, so tell it the stream is
		// gone instead of leaving its handler waiting forever.
		for _, s := range ss {
			if s.id%2 == 0 {
				c.Send(encode(streamCtl{Tango: "stream", ID: s.id, Op: "cancel"}))
			}
		}
	}
}

// forgetIDs drops c's carried-over Go stream id (no state will follow).
func (r *Router) forgetIDs(c Conn) {
	r.mu.Lock()
	delete(r.goIDs, c)
	r.mu.Unlock()
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
		r.drop(c, true, true)
		if m.Proto != ProtoVersion {
			r.forgetIDs(c)
			slog.Warn("ipc: runtime-js protocol mismatch", "proto", m.Proto)
			c.Send(encode(helloOut{Tango: "hello", Error: NewError(CodeBadArgs, "runtime-js protocol mismatch")}))
			break
		}
		raw := laneHello(c)
		// Retire any state a concurrent Go opener published in the Drop →
		// LaneHello interval. Its immutable handle belongs to the old session.
		r.drop(c, false, true)
		r.state(c) // publish the new immutable session only after LaneHello
		c.Send(encode(helloOut{Tango: "hello", Proto: ProtoVersion, Win: r.streamWindow(), Lanes: raw}))
	case "call":
		r.handleCall(c, &m)
	case "event":
		r.handleEvent(c, &m)
	case "stream":
		r.handleStream(c, &m)
	}
	return true
}

// HandleBinary processes one upstream fast lane packet from c (see the
// package comment). Each frame goes to the stream with its id on c, charged
// len(payload)+FrameOverhead; frames for unknown or finished streams are
// ignored. A malformed packet is dropped whole and logged with slog.Warn.
// HandleBinary does not keep packet: payloads are copied. It never blocks.
//
// HandleBinary is only for simple connections that never host a second
// document (no SessionBinaryConn). A SessionBinaryConn such as
// *tango.Surface needs HandleBinarySession with the generation its packet
// was authorized for: HandleBinary drops such packets and logs one
// slog.Warn per Router naming HandleBinarySession.
func (r *Router) HandleBinary(c Conn, packet []byte) {
	if _, ok := c.(SessionBinaryConn); ok {
		if r.warnedHandleBinary.CompareAndSwap(false, true) {
			slog.Warn("ipc: Router.HandleBinary dropped a lane packet for a session connection; wire App.OnBinary to Router.HandleBinarySession(s, gen, p)")
		}
		return
	}
	r.handleBinary(c, 0, packet)
}

// HandleBinarySession delivers only into the authorized document session.
// Use with App.OnBinary; generation is captured at ingress authorization.
func (r *Router) HandleBinarySession(c Conn, generation uint64, packet []byte) {
	r.handleBinary(c, generation, packet)
}

func (r *Router) handleBinary(c Conn, generation uint64, packet []byte) {
	frames, err := ParsePacket(packet)
	if err != nil {
		slog.Warn("ipc: malformed lane packet", "err", err, "bytes", len(packet))
		return
	}
	if len(frames) == 0 {
		return
	}
	r.mu.Lock()
	cs := r.conns[c]
	r.mu.Unlock()
	if cs == nil || generation != cs.generation {
		return // no state yet, so no streams
	}
	for _, f := range frames {
		s := cs.stream(f.Stream)
		if s == nil {
			continue // finished or unknown stream
		}
		s.onFrame(frame{
			seq:     f.Seq,
			cost:    len(f.Payload) + FrameOverhead,
			bin:     true,
			payload: append([]byte{}, f.Payload...),
		})
	}
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
	sendWin, recvWin, gapWait := r.jsSendWindow(), r.streamWindow(), r.streamGapTimeout()
	var cs *connState
	for {
		cs = r.state(c)
		cs.mu.Lock()
		if !cs.reaped {
			break
		}
		// Dropped since state returned it: its ids were carried over to
		// (or forgotten for) the next state, which state now returns.
		cs.mu.Unlock()
	}
	cs.nextID += 2
	id := cs.nextID
	s := newStream(cs.ctx, wirePeer{cs}, id, sendWin, recvWin, cs.removeStream)
	s.gapWait = gapWait  // before publishing: onFrame reads it under mu only
	s.awaitAccept = true // no fast lane before JS's accept (it could overtake the open)
	cs.streams[id] = s
	cs.mu.Unlock()
	c.Send(encode(streamOpen{Tango: "stream", ID: id, Op: "open", Method: method, Args: rawArgs}))
	if cs.ctx.Err() != nil {
		// Dropped while the open was on its way: a document that got the
		// open must not wait for the stream forever. (A duplicate of the
		// cancel drop may have sent is ignored by runtime-js.)
		c.Send(encode(streamCtl{Tango: "stream", ID: id, Op: "cancel"}))
	}
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
	conn       Conn
	binary     BinaryConn
	generation uint64
	ctx        context.Context
	cancel     context.CancelCauseFunc

	mu      sync.Mutex
	streams map[uint64]*Stream
	nextID  uint64 // last Go-allocated stream id (even)
	reaped  bool   // dropped: allocates no more ids (see Router.drop)
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
	cs.nextID = r.goIDs[c] // continue after the previous document's Go ids
	delete(r.goIDs, c)
	if scoped, ok := c.(SessionBinaryConn); ok {
		cs.generation, cs.binary = scoped.BinarySession()
	} else {
		cs.binary, _ = c.(BinaryConn)
	}
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
		raw, err := r.invoke(ctx, c, m.Method, h, args)
		if err != nil {
			cs.send(encode(retErr{Tango: "ret", ID: id, Error: err}))
			return
		}
		cs.send(encode(retOK{Tango: "ret", ID: id, Result: raw}))
	}()
}

// invoke runs call handler h for method and returns its JSON-encoded
// result, or the error as the caller sees it on the wire. It serves calls
// from pages (c is the page) and in-process calls (c is nil).
func (r *Router) invoke(ctx context.Context, c Conn, method string, h HandlerFunc, args json.RawMessage) (json.RawMessage, *Error) {
	res, err := r.safeCall(c, "binding "+method, func() (any, error) { return h(ctx, args) })
	if err == nil {
		var raw json.RawMessage
		if raw, err = marshalValue(res); err == nil {
			return raw, nil
		}
		err = fmt.Errorf("encode result: %w", err)
	}
	return nil, toWireError(err)
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
		s.onCtl(ctl{op: "ack", b: m.B})
	case "accept":
		// A missing "in" counts as true: only an explicit false refuses
		// data.
		s.onCtl(ctl{op: "accept", in: m.In == nil || *m.In})
	case "cancel":
		s.onCtl(ctl{op: "cancel"})
	case "error":
		s.onCtl(ctl{op: "error", seq: m.seq(), hasSeq: m.Seq != nil, err: m.Error})
	case "boundary", "end", "close":
		s.onCtl(ctl{op: m.Op, seq: m.seq()})
	case "data":
		f, err := decodeData(m)
		if err != nil {
			s.fail(err)
			return
		}
		s.onFrame(f)
	}
	// Unknown ops are ignored.
}

// decodeData validates a data message from JS and returns its frame with a
// raw payload (bin decoded from base64).
func decodeData(m *wireIn) (frame, *Error) {
	if (m.Data != nil) == (m.Bin != nil) {
		return frame{}, NewError(CodeBadArgs, "stream data must have exactly one of data and bin")
	}
	if m.C < FrameOverhead {
		return frame{}, NewError(CodeBadArgs, "stream data has missing or invalid c")
	}
	f := frame{seq: m.seq(), cost: m.C}
	if m.Bin == nil {
		f.payload = m.Data // freshly decoded for this message: private
		return f, nil
	}
	p, err := base64.StdEncoding.DecodeString(*m.Bin)
	if err != nil {
		return frame{}, NewError(CodeBadArgs, "stream data has invalid base64 in bin")
	}
	if len(p) > MaxFrameBytes {
		return frame{}, NewError(CodeBadArgs, "stream frame exceeds MaxFrameBytes")
	}
	f.bin, f.payload = true, p
	return f, nil
}

func (r *Router) openFromJS(cs *connState, m *wireIn) {
	id := m.ID
	if id%2 == 0 {
		cs.send(encode(streamCtl{Tango: "stream", ID: id, Op: "error", Error: NewError(CodeBadArgs, "stream id from JS must be odd")}))
		return
	}
	r.mu.Lock()
	h, ok := r.streams[m.Method]
	r.mu.Unlock()
	if !ok {
		cs.send(encode(streamCtl{Tango: "stream", ID: id, Op: "error", Error: NewError(CodeNotFound, "stream not found: "+m.Method)}))
		return
	}
	sendWin, recvWin, gapWait := r.jsSendWindow(), r.streamWindow(), r.streamGapTimeout()
	cs.mu.Lock()
	if old := cs.streams[id]; old != nil {
		cs.mu.Unlock()
		return // duplicate open; ignore
	}
	s := newStream(cs.ctx, wirePeer{cs}, id, sendWin, recvWin, cs.removeStream)
	s.gapWait = gapWait // before publishing: onFrame reads it under mu only
	if h.duplex == nil {
		s.setSendOnly() // a StreamFunc only sends; set before publishing
	}
	cs.streams[id] = s
	cs.mu.Unlock()
	// Accept before the handler can write.
	s.p.sendCtl(id, ctl{op: "accept", in: s.in})
	go r.runStream(cs.conn, m.Method, h, s, normArgs(m.Args))
}

// runStream runs stream handler h on the serving end s and ends the stream
// when it returns: close on nil, error otherwise, nothing if the stream
// already finished or was aborted. It serves streams from pages (c is the
// page) and in-process streams (c is nil).
func (r *Router) runStream(c Conn, method string, h streamHandler, s *Stream, args json.RawMessage) {
	_, err := r.safeCall(c, "stream "+method, func() (any, error) {
		if h.duplex != nil {
			return nil, h.duplex(s, args)
		}
		return nil, h.fn(s.ctx, args, s)
	})
	if cause := context.Cause(s.ctx); cause != nil {
		// Fully finished, or aborted (canceled, failed, dropped): nothing
		// more to send.
		s.finish(cause)
		return
	}
	if err != nil {
		s.CloseWithError(err)
	} else {
		s.Close()
	}
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
