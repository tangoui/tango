package ipc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
)

// wireClient plays runtime-js over a fakeConn: it speaks protocol v2 to a
// Router exactly as the JS client does (hello, odd stream ids, seq, byte
// credit with the same ack rule, base64 byte frames). The parity tests run
// the same scenarios on it and on Router.Open.
type wireClient struct {
	t       *testing.T
	r       *Router
	c       *fakeConn
	sendWin int // Go's receive window, from the hello reply
	recvWin int // JS's receive window (the window Go sends into)
	done    chan struct{}

	mu      sync.Mutex
	nextID  uint64
	streams map[uint64]*wireStream
}

// newWireClient connects a fresh fake page to r (hello handshake) and starts
// routing Go's messages to the client's streams.
func newWireClient(t *testing.T, r *Router) *wireClient {
	t.Helper()
	c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	h := c.next(t)
	if h.Tango != "hello" || h.Proto != ProtoVersion || h.Win == 0 {
		t.Fatalf("hello reply = %+v", h)
	}
	w := &wireClient{
		t: t, r: r, c: c,
		sendWin: h.Win, recvWin: r.jsSendWindow(),
		done:    make(chan struct{}),
		streams: make(map[uint64]*wireStream),
	}
	go w.pump()
	t.Cleanup(func() {
		close(w.done)
		r.Drop(c)
	})
	return w
}

// pump dispatches every message Go sends to the stream it belongs to.
func (w *wireClient) pump() {
	for {
		select {
		case <-w.done:
			return
		case m := <-w.c.ch:
			var o out
			if err := json.Unmarshal([]byte(m), &o); err != nil || o.Tango != "stream" {
				continue
			}
			w.mu.Lock()
			ws := w.streams[o.ID]
			w.mu.Unlock()
			if ws != nil {
				ws.onMsg(o)
			}
		}
	}
}

// post sends one raw message to the Router as the page.
func (w *wireClient) post(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if !w.r.HandleMessage(w.c, msg) {
		panic("message not claimed by ipc: " + msg)
	}
}

// open opens a stream from "JS" and waits for Go's accept or error. A
// refused open (e.g. not_found) leaves the error in ws.openErr.
func (w *wireClient) open(method string, args ...any) *wireStream {
	if args == nil {
		args = []any{}
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		w.t.Fatalf("marshal args: %v", err)
	}
	w.mu.Lock()
	w.nextID++
	id := 2*w.nextID - 1 // JS opens odd ids
	ws := &wireStream{w: w, id: id, changed: make(chan struct{})}
	w.streams[id] = ws
	w.mu.Unlock()
	w.post(`{"tango":"stream","id":%d,"op":"open","method":%q,"args":%s}`, id, method, rawArgs)
	ws.wait(func() bool { return ws.accepted || ws.err != nil })
	ws.mu.Lock()
	if !ws.accepted {
		ws.openErr = ws.err
	}
	ws.mu.Unlock()
	return ws
}

// wireFrame is a received frame waiting to be read.
type wireFrame struct {
	m    Message
	cost int
}

// wireStream is the "JS" end of one stream. It follows the same contract as
// a Go Stream: reorders by seq, returns credit when half the window has
// accumulated or the reader has caught up, stops sending after close or
// error, and after its own close (or cancel) ignores everything the other
// side still sends (a late ack included).
type wireStream struct {
	w       *wireClient
	id      uint64
	openErr error

	writeMu sync.Mutex

	mu       sync.Mutex
	changed  chan struct{} // closed and replaced on every state change
	accepted bool

	seq        uint64
	inflight   int
	sendErr    error // non-nil: writes return it
	sendDone   bool  // end/close sent
	peerClosed bool
	peerErr    *Error // Go's error (with seq): receive returns it after the frames before it

	queue    []wireFrame
	pending  map[uint64]wireFrame
	recvNext uint64
	ended    bool
	endSeq   uint64
	unacked  int

	closed bool  // this side sent close: Receive returns io.EOF, inbound ignored
	err    error // terminal: peer error/cancel or local cancel
}

// broadcastLocked wakes every waiter. Callers hold mu.
func (ws *wireStream) broadcastLocked() {
	close(ws.changed)
	ws.changed = make(chan struct{})
}

// wait blocks until cond (evaluated under mu) holds.
func (ws *wireStream) wait(cond func() bool) {
	for {
		ws.mu.Lock()
		if cond() {
			ws.mu.Unlock()
			return
		}
		ch := ws.changed
		ws.mu.Unlock()
		<-ch
	}
}

// onMsg applies one message from Go.
func (ws *wireStream) onMsg(o out) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.closed || ws.err != nil {
		return // finished on this side: late messages (acks too) are ignored
	}
	switch o.Op {
	case "accept":
		ws.accepted = true
		if o.In != nil && !*o.In && ws.sendErr == nil {
			ws.sendErr = NewError(CodeBadArgs, "stream does not accept data")
		}
	case "ack":
		ws.inflight -= o.B
		if ws.inflight < 0 {
			ws.inflight = 0
		}
	case "data":
		f := wireFrame{cost: o.C}
		if o.Bin != nil {
			p, err := base64.StdEncoding.DecodeString(*o.Bin)
			if err != nil {
				ws.err = NewError(CodeBadArgs, "bad base64 from Go")
				break
			}
			f.m = Message{Binary: true, Bytes: p}
		} else {
			f.m = Message{JSON: o.Data}
		}
		if o.Seq == ws.recvNext {
			ws.queue = append(ws.queue, f)
			ws.recvNext++
			for {
				g, ok := ws.pending[ws.recvNext]
				if !ok {
					break
				}
				delete(ws.pending, ws.recvNext)
				ws.queue = append(ws.queue, g)
				ws.recvNext++
			}
		} else {
			if ws.pending == nil {
				ws.pending = make(map[uint64]wireFrame)
			}
			ws.pending[o.Seq] = f
		}
	case "end", "close":
		if !ws.ended {
			ws.ended, ws.endSeq = true, o.Seq
		}
		if o.Op == "close" {
			ws.peerClosed = true
			if ws.sendErr == nil {
				ws.sendErr = NewError(CodeCanceled, "stream closed by peer")
			}
		}
	case "error":
		e := NewError(CodeInternal, "stream failed")
		if o.Error != nil {
			c := *o.Error
			e = &c
		}
		if !ws.accepted {
			ws.err = e // refused open (no seq): immediate
			break
		}
		// Like runtime-js: the error is ordered after the frames Go sent
		// before it; sends fail with it at once.
		ws.peerErr, ws.peerClosed = e, true
		if ws.sendErr == nil {
			ws.sendErr = e
		}
		if !ws.ended {
			ws.ended, ws.endSeq = true, o.Seq
		}
	case "cancel":
		ws.err = NewError(CodeCanceled, "stream canceled")
	}
	ws.broadcastLocked()
}

func (ws *wireStream) writeJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ws.writeFrame(false, raw)
}

func (ws *wireStream) writeBytes(p []byte) error {
	return ws.writeFrame(true, p)
}

// writeFrame sends one frame once it fits in Go's window (a frame always
// goes when nothing is in flight).
func (ws *wireStream) writeFrame(bin bool, p []byte) error {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	cost := len(p) + FrameOverhead
	var seq uint64
	var err error
	ws.wait(func() bool {
		switch {
		case ws.sendErr != nil:
			err = ws.sendErr
		case ws.err != nil:
			err = ws.err
		case ws.inflight == 0 || ws.inflight+cost <= ws.w.sendWin:
			seq = ws.seq
			ws.seq++
			ws.inflight += cost
		default:
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	if bin {
		ws.w.post(`{"tango":"stream","id":%d,"op":"data","seq":%d,"c":%d,"bin":%q}`,
			ws.id, seq, cost, base64.StdEncoding.EncodeToString(p))
	} else {
		ws.w.post(`{"tango":"stream","id":%d,"op":"data","seq":%d,"c":%d,"data":%s}`, ws.id, seq, cost, p)
	}
	return nil
}

// receive returns the next frame, io.EOF after Go's end/close (every frame
// before it read) or after this side's close, or the terminal error.
func (ws *wireStream) receive() (Message, error) {
	var (
		f   wireFrame
		got bool
		err error
		ack int
	)
	ws.wait(func() bool {
		switch {
		case ws.closed:
			err = io.EOF
		case ws.err != nil:
			err = ws.err
		case len(ws.queue) > 0:
			f, got = ws.queue[0], true
			ws.queue = ws.queue[1:]
			ws.unacked += f.cost
			if ws.unacked >= ws.w.recvWin/2 || len(ws.queue) == 0 {
				ack, ws.unacked = ws.unacked, 0
			}
		case ws.ended && ws.recvNext >= ws.endSeq:
			err = io.EOF
			if ws.peerErr != nil {
				err = ws.peerErr
			}
		default:
			return false
		}
		return true
	})
	if ack > 0 {
		ws.w.post(`{"tango":"stream","id":%d,"op":"ack","b":%d}`, ws.id, ack)
	}
	if !got {
		return Message{}, err
	}
	return f.m, nil
}

// closeSend ends this side's direction (end with the frames sent).
func (ws *wireStream) closeSend() {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	ws.mu.Lock()
	emit := !ws.sendDone && !ws.peerClosed && !ws.closed && ws.err == nil
	ws.sendDone = true
	if ws.sendErr == nil {
		ws.sendErr = NewError(CodeCanceled, "stream closed")
	}
	n := ws.seq
	ws.broadcastLocked()
	ws.mu.Unlock()
	if emit {
		ws.w.post(`{"tango":"stream","id":%d,"op":"end","seq":%d}`, ws.id, n)
	}
}

// close ends the stream from this side: close with the frames sent; from
// then on the stream is finished here.
func (ws *wireStream) close() {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	ws.mu.Lock()
	emit := !ws.closed && !ws.peerClosed && ws.err == nil
	ws.closed, ws.sendDone = true, true
	if ws.sendErr == nil {
		ws.sendErr = NewError(CodeCanceled, "stream closed")
	}
	ws.queue, ws.pending = nil, nil
	n := ws.seq
	ws.broadcastLocked()
	ws.mu.Unlock()
	if emit {
		ws.w.post(`{"tango":"stream","id":%d,"op":"close","seq":%d}`, ws.id, n)
	}
}

// cancel abandons the stream at once.
func (ws *wireStream) cancel() {
	ws.mu.Lock()
	emit := !ws.closed && ws.err == nil
	if emit {
		ws.err = NewError(CodeCanceled, "stream canceled")
		ws.queue, ws.pending = nil, nil
		ws.broadcastLocked()
	}
	ws.mu.Unlock()
	if emit {
		ws.w.post(`{"tango":"stream","id":%d,"op":"cancel"}`, ws.id)
	}
}
