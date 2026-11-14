package ipc

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/lane"
)

type sessionConn struct {
	*fakeBinConn
	state   *lane.State
	entered chan struct{}
	resume  chan struct{}
}

func (c *sessionConn) LaneHello() json.RawMessage {
	return c.state.Hello([]string{lane.DownScheme, lane.UpPost}, true)
}
func (c *sessionConn) BinarySession() (uint64, BinaryConn) {
	gen := c.state.Generation()
	return gen, &sessionTransport{sessionConn: c, gen: gen, entered: c.entered, resume: c.resume}
}

type sessionTransport struct {
	*sessionConn
	gen     uint64
	entered chan struct{}
	resume  chan struct{}
}

func (c *sessionTransport) BinaryMinSize() int { return 0 }
func (c *sessionTransport) SendBinary(p []byte) bool {
	if c.entered != nil {
		close(c.entered)
		<-c.resume
	}
	return c.state.SendGeneration(c.gen, p)
}
func readySession(c *sessionConn) {
	c.state.Control(`{"op":"ready","lane":"down:scheme","gen":` + fmtUint(c.state.Generation()) + `}`)
}
func fmtUint(n uint64) string { b, _ := json.Marshal(n); return string(b) }

func TestOldWriterCannotEnterSuccessorQueue(t *testing.T) {
	r := NewRouter()
	c := &sessionConn{fakeBinConn: newFakeBin(""), state: lane.New(nil), entered: make(chan struct{}), resume: make(chan struct{})}
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	c.next(t)
	readySession(c)
	old, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	c.next(t)
	r.HandleMessage(c, `{"tango":"stream","id":2,"op":"accept","in":true}`)
	done := make(chan error, 1)
	go func() { done <- old.WriteBytes(make([]byte, 4096)) }()
	within(t, c.entered, time.Second, "old writer at admission")
	// Go ids are never reused across hello; Drop (the page closed, the
	// surface reused) forgets them, which gives the successor the same id.
	r.Drop(c)
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	c.next(t)
	readySession(c)
	c.entered = nil
	fresh, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	c.next(t)
	if fresh.id != old.id {
		t.Fatal("test requires reused ID")
	}
	close(c.resume)
	within(t, done, time.Second, "old writer")
	if n, b := c.state.Queued(); n != 0 || b != 0 {
		t.Fatalf("stale send entered successor: %d/%d", n, b)
	}
	r.Drop(c)
}

func TestDelayedAuthorizedIngressCannotReachSuccessor(t *testing.T) {
	r := NewRouter()
	c := &sessionConn{fakeBinConn: newFakeBin(""), state: lane.New(nil)}
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	c.next(t)
	oldGen := c.state.Generation()
	old, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	c.next(t)
	authorized, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	packet := AppendPacket(nil, PacketFrame{Stream: old.id, Seq: 0, Payload: []byte("stale")})
	go func() { close(authorized); <-resume; r.HandleBinarySession(c, oldGen, packet); close(done) }()
	within(t, authorized, time.Second, "authorized ingress")
	// Drop occurs before LaneHello. Neither that interval nor a new same-ID
	// stream may make the captured authorization refer to the successor.
	r.Drop(c)
	mid, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	c.next(t)
	r.HandleBinarySession(c, oldGen, packet)
	mid.mu.Lock()
	injected := mid.recvNext != 0
	mid.mu.Unlock()
	if injected {
		t.Fatal("old authorization reached state published between Drop and LaneHello")
	}
	// Go ids are never reused across hello; Drop forgets them, which gives
	// the successor the old stream's id.
	r.Drop(c)
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	c.next(t)
	fresh, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	c.next(t)
	if fresh.id != old.id {
		t.Fatal("test requires reused ID")
	}
	close(resume)
	within(t, done, time.Second, "delayed callback")
	r.HandleBinarySession(c, c.state.Generation(), AppendPacket(nil, PacketFrame{Stream: fresh.id, Seq: 0, Payload: []byte("fresh")}))
	p, err := fresh.ReceiveBytes()
	if err != nil || string(p) != "fresh" {
		t.Fatalf("got %q, %v", p, err)
	}
	r.Drop(c)
}

func (c *sessionConn) DropBinarySession() { c.state.Reset() }

// HandleBinary cannot know which document a session connection's packet
// was authorized for, so it must not deliver into any: it drops the packet
// and says once per Router (with slog.Warn) to use HandleBinarySession.
func TestHandleBinaryOnSessionConnWarnsOnce(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	r := NewRouter()
	c := &sessionConn{fakeBinConn: newFakeBin(""), state: lane.New(nil)}
	got := make(chan []byte, 4)
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		for {
			p, err := s.ReceiveBytes()
			if err != nil {
				return err
			}
			got <- p
		}
	})
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	c.next(t)
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"Sink","args":[]}`)
	c.next(t) // accept
	for seq := uint64(0); seq < 3; seq++ {
		r.HandleBinary(c, AppendPacket(nil, PacketFrame{Stream: 1, Seq: seq, Payload: []byte("x")}))
	}
	select {
	case p := <-got:
		t.Fatalf("HandleBinary delivered %q on a session connection", p)
	case <-time.After(50 * time.Millisecond):
	}
	if n := strings.Count(logs.String(), "HandleBinarySession"); n != 1 {
		t.Fatalf("want exactly one warning naming HandleBinarySession, got %d:\n%s", n, logs.String())
	}
	// Another Router warns on its own.
	r2 := NewRouter()
	r2.HandleBinary(c, AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte("x")}))
	if n := strings.Count(logs.String(), "HandleBinarySession"); n != 2 {
		t.Fatalf("want one warning per Router, got %d:\n%s", n, logs.String())
	}
	// The session entry point still delivers.
	r.HandleBinarySession(c, c.state.Generation(), AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte("ok")}))
	if p := within(t, got, time.Second, "ReceiveBytes"); string(p) != "ok" {
		t.Fatalf("received %q", p)
	}
	r.Drop(c)
}

// helloRaceConn opens a Go stream from inside LaneHello: the stream is
// published between hello's Drop and its drop(c,false), and its open goes
// to the new document.
type helloRaceConn struct {
	*sessionConn
	r      *Router
	opened chan *Stream
}

func (c *helloRaceConn) LaneHello() json.RawMessage {
	raw := c.sessionConn.LaneHello()
	if c.opened != nil {
		if s, err := c.r.OpenStream(c, "Late"); err == nil {
			c.opened <- s
		}
		c.opened = nil
	}
	return raw
}

// A Go stream reaped by the hello transition is canceled toward the page:
// its open may already have reached the new document, whose handler must
// not wait forever.
func TestHelloRaceOrphanIsCanceled(t *testing.T) {
	r := NewRouter()
	c := &helloRaceConn{sessionConn: &sessionConn{fakeBinConn: newFakeBin(""), state: lane.New(nil)}, r: r, opened: make(chan *Stream, 1)}
	opened := c.opened
	r.HandleMessage(c, `{"tango":"hello","proto":2}`)
	s := within(t, opened, time.Second, "racing OpenStream")
	if o := c.next(t); o.Op != "open" || o.ID != s.id || o.Method != "Late" {
		t.Fatalf("want open, got %+v", o)
	}
	if o := c.next(t); o.Tango != "stream" || o.Op != "cancel" || o.ID != s.id {
		t.Fatalf("want cancel for the reaped stream, got %+v", o)
	}
	if o := c.next(t); o.Tango != "hello" {
		t.Fatalf("want hello reply, got %+v", o)
	}
	if s.Context().Err() == nil {
		t.Fatal("reaped stream still live")
	}
	c.none(t, 50*time.Millisecond)
	// The successor never reuses the reaped id, so that cancel can only
	// ever name the reaped stream.
	fresh, err := r.OpenStream(c, "Next")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.id == s.id {
		t.Fatalf("successor reused reaped Go stream id %d", s.id)
	}
	if o := c.next(t); o.Op != "open" || o.ID != fresh.id {
		t.Fatalf("want open %d, got %+v", fresh.id, o)
	}
	c.none(t, 50*time.Millisecond)
	if fresh.Context().Err() != nil {
		t.Fatal("successor stream canceled")
	}
	r.Drop(c)
}

// Go stream ids are never reused on one Conn across hello: a late message
// about an old document's stream cannot name a new document's stream.
func TestGoStreamIDsMonotonicAcrossHello(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	send(t, r, c, `{"tango":"hello","proto":2}`)
	c.next(t)
	var ids []uint64
	for i := 0; i < 3; i++ {
		s, err := r.OpenStream(c, "Feed")
		if err != nil {
			t.Fatal(err)
		}
		c.next(t) // open
		ids = append(ids, s.id)
		send(t, r, c, `{"tango":"hello","proto":2}`)
		c.next(t)
	}
	if !(ids[0] < ids[1] && ids[1] < ids[2]) || ids[0]%2 != 0 || ids[1]%2 != 0 {
		t.Fatalf("Go ids not monotonic even across hello: %v", ids)
	}
	r.Drop(c)
}
