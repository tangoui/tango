package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Stream is one end of a stream: the writer handed to a StreamFunc or
// DuplexFunc, the value Router.OpenStream returns for Go-opened streams, or
// the client end Router.Open returns for in-process streams.
//
// A Stream talks to the other end only through its peer (frames and control
// messages out; onCtl in), its parent context and its release function, so
// the same state machine serves runtime-js connections and in-process
// clients.
//
// Locking: writeMu serializes frame hand-off and the local end/close/error,
// so those control messages follow every frame they count. A local close
// never waits for credit while holding it: it first marks the send
// direction terminal under mu, which makes a writer blocked on credit
// return at once. readMu serializes Receive* calls. mu guards the state
// below it and is never held while calling the peer. onCtl and onFrame take
// only mu and never block: a peer may deliver synchronously while this side
// holds writeMu or readMu.
//
// Lifecycle: each direction ends on its own. The send direction is
// terminal after a local CloseSend/Close/CloseWithError or a peer
// close/cancel/error; the receive direction after Receive* returned io.EOF
// (or the peer's error), a local Close, or a peer cancel. When both are
// terminal the stream has fully finished: release is called (once) and the
// context is canceled. Aborts (peer cancel, peer error without seq, local
// error, Drop) finish at once; a peer error with seq is ordered like
// end/close (see peerError).
type Stream struct {
	p       peer
	id      uint64
	ctx     context.Context
	cancel  context.CancelCauseFunc
	release func(*Stream)
	sendWin int // peer's receive window in bytes
	recvWin int // this end's receive window in bytes

	writeMu sync.Mutex
	readMu  sync.Mutex

	mu       sync.Mutex
	seq      uint64 // data frames sent so far (next frame's seq)
	inflight int    // charged bytes sent but not yet acked
	sendErr  error  // non-nil: Write* returns it
	// awaitAccept: this end opened the stream (Router.OpenStream) and the
	// peer's accept has not arrived; byte frames stay off fast lanes until
	// then (frame.noLane), since a lane packet could overtake the open.
	awaitAccept bool
	sendDone    bool // the send direction is terminal (see Lifecycle)
	sentEnd     bool // end, close or error was sent (or will not be: aborted)
	sentLast    bool // close or error was sent (or will not be: aborted)
	closing     int  // local end/close/error calls not yet on the wire
	finished    bool // release already called
	wake        chan struct{}

	peerClosed  bool // the peer sent close: it no longer receives
	localClosed bool // Close/CloseWithError called: inbound frames are dropped
	recvDone    bool // the receive direction is terminal: Receive* returns io.EOF

	// Receive direction.
	in         bool             // false: inbound frames are dropped (and acked), not queued
	recvNext   uint64           // seq of the next frame to deliver (in=false: frames seen)
	queue      []frame          // in-order frames not yet read
	pending    map[uint64]frame // out-of-order frames (seq > recvNext)
	held       int              // sum of len(payload)+FrameOverhead over queue and pending
	unacked    int              // charged bytes read (or dropped) but not yet acked
	recvEnd    bool             // the peer ended its direction (end/close) ...
	recvEndSeq uint64           // ... after sending this many frames
	recvErr    error            // the peer's error (with seq): Receive* returns it after the frames before it
	rwake      chan struct{}

	// Gap detection: frames of one stream may travel on two transports
	// (the message channel and a fast lane), so a frame can be missing
	// while later ones (or the end counting it) are already here. When
	// that lasts gapWait, the frame is taken as lost (see updateGapLocked).
	boundary uint64 // reliable exclusive upper bound of accepted lane sends
	gapWait  time.Duration
	gapTimer *time.Timer // non-nil: armed for the current gap
	gapGen   uint64      // bumped on every arm/stop; a stale callback sees a different value

	// io view (ReadWriter): rwReadMu guards rwRest, the unread rest of a
	// partially read byte frame; rwWriteMu keeps the frames of one Write
	// together. Lock order: rwReadMu before readMu, rwWriteMu before
	// writeMu.
	rwReadMu  sync.Mutex
	rwRest    []byte
	rwWriteMu sync.Mutex
}

// Message is one frame received from the peer.
type Message struct {
	Binary bool            // true: Bytes holds the payload; false: JSON does
	Bytes  []byte          // byte frame payload
	JSON   json.RawMessage // JSON frame payload
}

// newStream returns a Stream whose context derives from parent. sendWin is
// the peer's receive window (bytes this end may have in flight), recvWin
// this end's receive window. release is called exactly once, when the
// stream has fully finished. The stream receives data; an end that only
// sends (a StreamFunc) sets in to false before the stream is published.
func newStream(parent context.Context, p peer, id uint64, sendWin, recvWin int, release func(*Stream)) *Stream {
	ctx, cancel := context.WithCancelCause(parent)
	return &Stream{
		p: p, id: id, ctx: ctx, cancel: cancel, release: release,
		sendWin: sendWin, recvWin: recvWin,
		wake: make(chan struct{}, 1),
		in:   true, rwake: make(chan struct{}, 1),
		gapWait: GapTimeout,
	}
}

// ID returns the stream id on the wire.
func (s *Stream) ID() uint64 { return s.id }

// Context is canceled when the stream has fully finished (both directions
// terminal) or was aborted: canceled or failed by either side, or the
// connection dropped. A half-closed stream (CloseSend, or the peer's end)
// keeps it live; so does a peer error while frames sent before it are still
// unread (Receive* delivers them, then the error, then it is canceled).
func (s *Stream) Context() context.Context { return s.ctx }

// Write JSON-encodes v and sends it as one JSON frame. It blocks while the
// frame does not fit in the peer's byte window (FrameOverhead plus the
// encoded size is charged per frame; a frame is always sent when nothing is
// in flight). An encoding larger than MaxFrameBytes fails with CodeBadArgs
// and leaves the stream usable.
func (s *Stream) Write(v any) error {
	raw, err := marshalValue(v)
	if err != nil {
		return err
	}
	if _, ok := v.(json.RawMessage); ok {
		raw = wireJSON(raw)
	}
	if len(raw) > MaxFrameBytes {
		return NewError(CodeBadArgs, "stream frame exceeds MaxFrameBytes")
	}
	return s.sendFrame(frame{cost: len(raw) + FrameOverhead, payload: raw})
}

// WriteBytes sends p as one byte frame, with the same flow control as Write
// (charged len(p)+FrameOverhead). p may be reused once WriteBytes returns.
// A p larger than MaxFrameBytes fails with CodeBadArgs and leaves the stream
// usable.
func (s *Stream) WriteBytes(p []byte) error {
	if len(p) > MaxFrameBytes {
		return NewError(CodeBadArgs, "stream frame exceeds MaxFrameBytes")
	}
	// Copy before taking the send lock (see bytesPreparer).
	return s.sendFrame(frame{cost: len(p) + FrameOverhead, bin: true, payload: prepareBytes(s.p, p)})
}

// wireJSON returns a private copy of a caller-supplied json.RawMessage in
// exactly the form encoding/json puts on the wire (compact, HTML-escaped),
// so the charged size matches the bytes sent and the peer may keep it.
func wireJSON(raw []byte) []byte {
	var compact, out bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		// marshalValue validated raw already.
		panic("ipc: compact: " + err.Error())
	}
	json.HTMLEscape(&out, compact.Bytes())
	return out.Bytes()
}

// sendFrame assigns f its seq and hands it to the peer once it fits in the
// send window, blocking until then.
func (s *Stream) sendFrame(f frame) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for {
		s.mu.Lock()
		if err := s.sendErr; err != nil {
			s.mu.Unlock()
			return err
		}
		if err := context.Cause(s.ctx); err != nil {
			s.mu.Unlock()
			return err
		}
		if s.inflight == 0 || s.inflight+f.cost <= s.sendWin {
			f.seq = s.seq
			f.noLane = s.awaitAccept
			s.seq++
			s.inflight += f.cost
			s.mu.Unlock()
			// Outside mu: a local peer may ack synchronously into onCtl.
			s.p.sendFrame(s.id, f)
			return nil
		}
		s.mu.Unlock()
		select {
		case <-s.ctx.Done():
		case <-s.wake:
		}
	}
}

// signal wakes a writer blocked in sendFrame. Only one writer can wait at a
// time (writeMu), so one buffered slot is enough; spurious wakes recheck.
func (s *Stream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// onCtl handles a control message from the peer. It takes only mu (never
// writeMu) and must not block.
func (s *Stream) onCtl(c ctl) {
	switch c.op {
	case "boundary":
		s.mu.Lock()
		if c.seq > s.boundary {
			s.boundary = c.seq
		}
		s.updateGapLocked(false)
		s.mu.Unlock()
	case "ack":
		if c.b <= 0 {
			return
		}
		s.mu.Lock()
		s.inflight -= c.b
		if s.inflight < 0 { // over-ack from a buggy peer: never go negative
			s.inflight = 0
		}
		s.mu.Unlock()
		s.signal()
	case "accept":
		s.mu.Lock()
		s.awaitAccept = false
		s.mu.Unlock()
		if c.in {
			return
		}
		// The peer does not receive: fail local sends. Frames already sent
		// are dropped (and acked) by the peer.
		s.mu.Lock()
		if s.sendErr == nil {
			s.sendErr = NewError(CodeBadArgs, "stream does not accept data")
		}
		s.mu.Unlock()
		s.signal()
	case "cancel":
		s.finish(NewError(CodeCanceled, "stream canceled"))
	case "error":
		// The peer failed or refused the stream (e.g. no JS handler for a
		// Go-opened stream).
		e := c.err
		if e == nil {
			e = NewError(CodeInternal, "stream failed")
		}
		if !c.hasSeq {
			s.finish(e) // protocol-level failure: abort at once
			return
		}
		s.peerError(e, c.seq)
	case "end", "close":
		// The peer's direction ends after c.seq frames; close also says the
		// peer no longer receives, which ends this side's send direction.
		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		closed := c.op == "close" && !s.peerClosed
		if closed {
			s.peerClosed, s.sendDone = true, true
			if s.sendErr == nil {
				s.sendErr = NewError(CodeCanceled, "stream closed by peer")
			}
		}
		bad, ended := false, false
		if !s.recvEnd && !s.localClosed { // only the first end/close counts
			// recvNext counts frames delivered (in=false: seen); pending
			// holds frames beyond it.
			bad = s.recvNext > c.seq
			for seq := range s.pending {
				bad = bad || seq >= c.seq
			}
			s.recvEnd, s.recvEndSeq, ended = true, c.seq, true
			s.updateGapLocked(false) // frames before c.seq may be missing
		}
		s.mu.Unlock()
		if bad {
			s.fail(NewError(CodeBadArgs, "frame after end"))
			return
		}
		if closed {
			s.signal() // a writer blocked on credit fails now
		}
		if ended {
			s.rsignal()
		}
		s.maybeFinish()
	}
}

// peerError handles the peer's error carrying seq n, which is ordered like
// end/close: the send direction stops at once (Write* returns e) and nothing
// more is sent to the peer, but Receive* still delivers the frames before n
// and only then returns e. The stream finishes (with cause e) once nothing
// is left to deliver: at once if this end does not receive or already
// closed, otherwise when the reader reaches the error.
func (s *Stream) peerError(e *Error, n uint64) {
	s.mu.Lock()
	if s.ctx.Err() != nil || s.recvErr != nil {
		s.mu.Unlock()
		return
	}
	// The peer stopped listening: nothing more goes out (no close or error
	// back), and writers fail with the peer's error.
	s.peerClosed, s.sendDone, s.sentEnd, s.sentLast = true, true, true, true
	s.sendErr = e
	s.recvErr = e
	bad := false
	if !s.recvEnd && !s.recvDone && !s.localClosed {
		bad = s.recvNext > n
		for seq := range s.pending {
			bad = bad || seq >= n
		}
		s.recvEnd, s.recvEndSeq = true, n
		s.updateGapLocked(false) // frames before n may be missing
	}
	now := s.recvDone || s.localClosed || (len(s.queue) == 0 && s.recvTerminalLocked())
	s.mu.Unlock()
	s.signal() // a writer blocked on credit fails now
	if bad {
		s.finish(NewError(CodeBadArgs, "frame after end"))
		return
	}
	if now {
		s.finish(e)
		return
	}
	s.rsignal()
}

// setSendOnly marks a stream that does not receive (a StreamFunc, accepted
// with in:false): inbound frames are dropped and acked, and the receive
// direction counts as terminal from the start. Call it before the stream is
// published.
func (s *Stream) setSendOnly() {
	s.in, s.recvDone = false, true
}

// recvLimit is the most payload (plus FrameOverhead per frame) the peer may
// make this end hold unread: one window plus one maximal frame (the peer
// may send a frame larger than the window when nothing is in flight).
func (s *Stream) recvLimit() int { return s.recvWin + MaxFrameBytes + FrameOverhead }

// onFrame handles a data frame from the peer. Byte payloads are raw (the
// transport decoded them). It takes only mu and never blocks; protocol
// violations fail the stream.
func (s *Stream) onFrame(f frame) {
	s.mu.Lock()
	if s.ctx.Err() != nil { // finished: late frames are dropped
		s.mu.Unlock()
		return
	}
	if s.localClosed {
		// This side closed: frames still in flight are dropped and their
		// credit is not returned (the peer's sends fail on our close).
		s.mu.Unlock()
		return
	}
	if !s.in {
		// Send-only end: drop the frame but return its credit at once
		// (nothing will ever be read), so an opener that wrote before
		// seeing accept{in:false} is not stalled. Its seq is still checked
		// against end.
		if s.recvEnd && f.seq >= s.recvEndSeq {
			s.mu.Unlock()
			s.fail(NewError(CodeBadArgs, "frame after end"))
			return
		}
		if f.seq >= s.recvNext {
			s.recvNext = f.seq + 1
		}
		ack := s.creditLocked(f.cost, true)
		s.mu.Unlock()
		if ack > 0 {
			s.p.sendCtl(s.id, ctl{op: "ack", b: ack})
		}
		return
	}
	var bad *Error
	_, dup := s.pending[f.seq]
	switch {
	case f.seq < s.recvNext || dup:
		bad = NewError(CodeBadArgs, "duplicate frame")
	case s.recvEnd && f.seq >= s.recvEndSeq:
		bad = NewError(CodeBadArgs, "frame after end")
	default:
		s.held += len(f.payload) + FrameOverhead
		if s.held > s.recvLimit() {
			bad = NewError(CodeBadArgs, "flow control violated")
		}
	}
	if bad != nil {
		s.mu.Unlock()
		s.fail(bad)
		return
	}
	ready := f.seq == s.recvNext
	if ready {
		s.queue = append(s.queue, f)
		s.recvNext++
		for {
			g, ok := s.pending[s.recvNext]
			if !ok {
				break
			}
			delete(s.pending, s.recvNext)
			s.queue = append(s.queue, g)
			s.recvNext++
		}
	} else {
		if s.pending == nil {
			s.pending = make(map[uint64]frame)
		}
		s.pending[f.seq] = f
	}
	s.updateGapLocked(ready)
	s.mu.Unlock()
	if ready {
		s.rsignal()
	}
}

// gapLocked reports whether this end is waiting for a frame that may have
// been lost: it receives, and frames beyond recvNext are pending or the
// peer's end/close/error counts frames not yet here. Callers hold mu.
func (s *Stream) gapLocked() bool {
	if !s.in || s.localClosed || s.finished || s.ctx.Err() != nil {
		return false
	}
	return s.recvNext < s.boundary || len(s.pending) > 0 || (s.recvEnd && s.recvNext < s.recvEndSeq)
}

// updateGapLocked re-evaluates the gap timer after the receive state
// changed; progress reports that recvNext advanced. No gap: the timer
// stops. A gap with no timer, or one that remains after progress: the
// timer (re)starts from now. A gap that only grew keeps its deadline.
// Callers hold mu.
func (s *Stream) updateGapLocked(progress bool) {
	if s.recvNext >= s.boundary {
		s.boundary = 0
	}
	if !s.gapLocked() {
		s.stopGapLocked()
		return
	}
	if s.gapTimer != nil && !progress {
		return
	}
	s.stopGapLocked()
	s.gapGen++
	gen := s.gapGen
	s.gapTimer = time.AfterFunc(s.gapWait, func() { s.gapExpired(gen) })
}

// stopGapLocked disarms the gap timer. A callback already running sees a
// new gapGen and does nothing. Callers hold mu.
func (s *Stream) stopGapLocked() {
	if s.gapTimer != nil {
		s.gapTimer.Stop()
		s.gapTimer = nil
		s.gapGen++
	}
}

// gapExpired runs on the timer goroutine when a gap lasted gapWait: the
// missing frames are taken as lost and the stream fails with
// CodeTransport. Like onFrame it takes only mu and calls the peer (via
// fail) outside it.
func (s *Stream) gapExpired(gen uint64) {
	s.mu.Lock()
	if gen != s.gapGen {
		s.mu.Unlock()
		return
	}
	s.gapTimer = nil
	lost := s.gapLocked()
	s.mu.Unlock()
	if lost {
		s.fail(NewError(CodeTransport, "frames lost on a fast lane"))
	}
}

// creditLocked records cost charged bytes as consumed and returns the
// amount to ack now: everything accumulated once it reaches half the
// receive window, or as soon as the reader has caught up (caughtUp: nothing
// left unread). The second rule matters: credit left below half a window
// with nothing to read would otherwise never return, and a sender whose
// next frame does not fit beside it would block forever. Callers hold mu
// and send the ack after unlocking.
func (s *Stream) creditLocked(cost int, caughtUp bool) int {
	s.unacked += cost
	if s.unacked <= 0 || (s.unacked < s.recvWin/2 && !caughtUp) {
		return 0
	}
	b := s.unacked
	s.unacked = 0
	return b
}

// rsignal wakes a reader blocked in recv. Only one reader can wait at a
// time (readMu), so one buffered slot is enough; spurious wakes recheck.
func (s *Stream) rsignal() {
	select {
	case s.rwake <- struct{}{}:
	default:
	}
}

// Receive returns the next frame from the peer, blocking until one arrives.
// It returns io.EOF once the peer ended its direction (end or close) and
// every frame before the end was read, or once this side called Close (no
// longer receiving; unread frames are dropped). When the peer failed the
// stream (its CloseWithError, or a page handler's error), the frames it sent
// before failing are delivered first and then the peer's *Error is returned.
// It returns context.Cause of the stream context once the stream is
// canceled or aborted, or the connection dropped.
func (s *Stream) Receive() (Message, error) {
	f, err := s.recv(recvAny)
	if err != nil {
		return Message{}, err
	}
	if f.bin {
		return Message{Binary: true, Bytes: f.payload}, nil
	}
	return Message{JSON: f.payload}, nil
}

// ReceiveBytes is Receive for a byte frame. If the next frame is a JSON
// frame it returns an *Error with CodeBadArgs and leaves the frame queued.
func (s *Stream) ReceiveBytes() ([]byte, error) {
	f, err := s.recv(recvBytes)
	if err != nil {
		return nil, err
	}
	return f.payload, nil
}

// ReceiveJSON receives a JSON frame and decodes it into v. If the next
// frame is a byte frame it returns an *Error with CodeBadArgs and leaves the
// frame queued; if decoding fails it returns an *Error with CodeBadArgs and
// the frame is consumed.
func (s *Stream) ReceiveJSON(v any) error {
	f, err := s.recv(recvJSON)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(f.payload, v); err != nil {
		return NewError(CodeBadArgs, "decode stream frame: "+err.Error())
	}
	return nil
}

// Frame kinds accepted by recv.
const (
	recvAny = iota
	recvBytes
	recvJSON
)

// recv takes the next frame off the queue if it is of the wanted kind,
// blocking until one arrives, and returns its credit to the peer once
// enough has accumulated.
func (s *Stream) recv(want int) (frame, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for {
		s.mu.Lock()
		if s.recvDone {
			s.mu.Unlock()
			return frame{}, io.EOF
		}
		if err := context.Cause(s.ctx); err != nil {
			s.mu.Unlock()
			return frame{}, err
		}
		if len(s.queue) > 0 {
			f := s.queue[0]
			if want == recvBytes && !f.bin {
				s.mu.Unlock()
				return frame{}, NewError(CodeBadArgs, "stream frame is JSON, not bytes")
			}
			if want == recvJSON && f.bin {
				s.mu.Unlock()
				return frame{}, NewError(CodeBadArgs, "stream frame is bytes, not JSON")
			}
			s.queue[0] = frame{} // let the payload be collected
			s.queue = s.queue[1:]
			s.held -= len(f.payload) + FrameOverhead
			ack := s.creditLocked(f.cost, len(s.queue) == 0)
			s.mu.Unlock()
			if ack > 0 {
				s.p.sendCtl(s.id, ctl{op: "ack", b: ack})
			}
			return f, nil
		}
		if s.recvTerminalLocked() {
			if e := s.recvErr; e != nil {
				// The peer's error comes after its frames. The stream
				// finishes with it, so later calls return it too.
				s.mu.Unlock()
				s.finish(e)
				return frame{}, e
			}
			s.recvDone = true
			s.mu.Unlock()
			s.maybeFinish()
			return frame{}, io.EOF
		}
		s.mu.Unlock()
		select {
		case <-s.ctx.Done():
		case <-s.rwake:
		}
	}
}

// recvTerminalLocked reports whether the receive direction ended normally:
// the peer sent end/close and every frame before it was delivered. Callers
// hold mu.
func (s *Stream) recvTerminalLocked() bool {
	return s.recvEnd && s.recvNext >= s.recvEndSeq
}

// CloseSend ends this side's send direction: it sends end with the number
// of frames sent, and Write* returns an *Error with CodeCanceled ("stream
// closed") from then on. The stream keeps receiving until the peer ends its
// direction. A Write blocked on credit in another goroutine fails at once
// and sends nothing. It is a no-op once the send direction has ended.
func (s *Stream) CloseSend() error {
	s.closeLocal("end", nil)
	return nil
}

// Close ends the stream normally: it sends close with the number of frames
// sent, which also tells the peer this side no longer receives. Unread and
// later frames are dropped without returning their credit, Receive* returns
// io.EOF and Write* an *Error with CodeCanceled. A Write blocked on credit in
// another goroutine fails at once and sends nothing. Nothing is sent if the
// stream already finished or the peer closed it. Further calls are no-ops.
func (s *Stream) Close() error {
	s.closeLocal("close", nil)
	return nil
}

// CloseWithError fails the stream in both directions: it sends error with
// the number of frames sent, and the peer sees err (an *Error keeps its
// code, others become "internal") after the frames sent before it. This
// side does not see err: its own Receive* and Write* return an *Error with
// CodeCanceled ("stream closed") from then on. Like Close, it does not wait
// for credit, and it is a no-op once Close or CloseWithError was called.
func (s *Stream) CloseWithError(err error) error {
	if err == nil {
		return s.Close()
	}
	s.closeLocal("error", err)
	return nil
}

// closeLocal implements CloseSend (op "end"), Close ("close") and
// CloseWithError ("error").
func (s *Stream) closeLocal(op string, err error) {
	// Phase 1, under mu only: end the send direction, so a writer blocked
	// on credit returns (sending nothing) instead of holding writeMu.
	s.mu.Lock()
	if s.finished || (op == "end" && s.sendDone) || (op != "end" && s.localClosed) {
		s.mu.Unlock()
		return
	}
	s.sendDone = true
	if s.sendErr == nil {
		s.sendErr = NewError(CodeCanceled, "stream closed")
	}
	if op != "end" {
		// This side stops receiving: drop what is unread.
		s.localClosed = true
		s.queue, s.pending, s.held = nil, nil, 0
		s.stopGapLocked()
		if op == "close" {
			s.recvDone = true // an error aborts instead (see below)
		}
	}
	s.closing++ // the stream must not finish before the message is out
	s.mu.Unlock()
	s.signal()
	s.rsignal()

	// Phase 2: writeMu comes free as soon as a frame being handed off is
	// out (peer calls never block), so the seq read below is final and the
	// message follows every frame it counts.
	s.writeMu.Lock()
	s.mu.Lock()
	emit := s.ctx.Err() == nil && !s.peerClosed && !s.sentLast && (op != "end" || !s.sentEnd)
	if emit {
		s.sentEnd = true
		s.sentLast = op != "end"
	}
	n := s.seq
	s.mu.Unlock()
	if emit {
		c := ctl{op: op, seq: n, hasSeq: true}
		if op == "error" {
			c.err = toWireError(err)
		}
		s.p.sendCtl(s.id, c)
	}
	s.writeMu.Unlock()

	s.mu.Lock()
	s.closing--
	s.mu.Unlock()
	if op == "error" {
		s.finish(NewError(CodeCanceled, "stream closed"))
		return
	}
	s.maybeFinish()
}

// maybeFinish finishes the stream normally once both directions are
// terminal and no local end/close is still on its way out.
func (s *Stream) maybeFinish() {
	s.mu.Lock()
	done := !s.finished && s.sendDone && s.recvDone && s.closing == 0
	var cause error = NewError(CodeCanceled, "stream closed")
	if s.recvErr != nil { // closed before reading up to the peer's error
		cause = s.recvErr
	}
	s.mu.Unlock()
	if done {
		s.finish(cause)
	}
}

// fail ends the stream with a local protocol error: it sends error (with
// the number of frames sent so far) unless close/error was already sent or
// the peer closed, then finishes with err, which Receive* and Write* return
// from then on. It takes only mu (never writeMu), so onFrame/onCtl may call
// it. A Write that already took its seq may hand that frame to the peer
// after the error; the error's seq counts it.
func (s *Stream) fail(err *Error) {
	s.mu.Lock()
	send := !s.sentLast && !s.peerClosed && s.ctx.Err() == nil
	s.sendDone, s.sentEnd, s.sentLast = true, true, true
	if s.sendErr == nil {
		s.sendErr = err
	}
	n := s.seq
	s.mu.Unlock()
	if send {
		s.p.sendCtl(s.id, ctl{op: "error", seq: n, hasSeq: true, err: err})
	}
	s.finish(err)
}

// finish ends the stream without sending anything: it cancels the context
// with cause (unless already canceled), which releases blocked writers and
// readers, and calls release exactly once. Used for normal completion
// (maybeFinish) and for aborts (peer cancel/error, local error, Drop).
// It reports whether this call finished the stream (false: already
// finished).
func (s *Stream) finish(cause error) bool {
	s.mu.Lock()
	s.sendDone, s.sentEnd, s.sentLast = true, true, true
	s.stopGapLocked()
	if s.finished {
		s.mu.Unlock()
		return false
	}
	s.finished = true
	s.mu.Unlock()
	s.cancel(cause)
	if s.release != nil {
		s.release(s)
	}
	return true
}
