package ipc

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBinConn is a fakeConn that also implements BinaryConn.
type fakeBinConn struct {
	*fakeConn
	hello json.RawMessage // LaneHello result
	min   int             // BinaryMinSize result

	mu     sync.Mutex
	sent   [][]byte // packets accepted by SendBinary (copies)
	accept bool     // SendBinary result
}

func newFakeBin(hello string) *fakeBinConn {
	c := &fakeBinConn{fakeConn: newFake(), min: -1}
	if hello != "" {
		c.hello = json.RawMessage(hello)
	}
	return c
}

func (c *fakeBinConn) LaneHello() json.RawMessage { return c.hello }
func (c *fakeBinConn) BinaryMinSize() int         { return c.min }

func (c *fakeBinConn) SendBinary(p []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.accept {
		return false
	}
	c.sent = append(c.sent, append([]byte{}, p...))
	return true
}

var _ BinaryConn = (*fakeBinConn)(nil)

// openBin opens duplex stream id from "JS" on c (the BinaryConn itself is
// the connection key) and consumes Go's accept.
func openBin(t *testing.T, r *Router, c *fakeBinConn, id uint64, method string) {
	t.Helper()
	send(t, r, c, `{"tango":"stream","id":%d,"op":"open","method":%q,"args":[]}`, id, method)
	if o := c.next(t); o.Op != "accept" || o.ID != id || o.In == nil || !*o.In {
		t.Fatalf("open %s: want accept in:true, got %+v", method, o)
	}
}

func TestHelloIncludesLanes(t *testing.T) {
	r := NewRouter()
	c := newFakeBin(`{"gen":1,"tok":"ab","list":["down:scheme"]}`)
	send(t, r, c, `{"tango":"hello","proto":2}`)
	want := `{"tango":"hello","proto":2,"win":1048576,"lanes":{"gen":1,"tok":"ab","list":["down:scheme"]}}`
	if raw := c.nextRaw(t); raw != want {
		t.Fatalf("reply %s\nwant  %s", raw, want)
	}
}

func TestHelloOmitsLanesWhenNil(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	send(t, r, c, `{"tango":"hello","proto":2}`)
	raw := c.nextRaw(t)
	if strings.Contains(raw, "lanes") || raw != `{"tango":"hello","proto":2,"win":1048576}` {
		t.Fatalf("reply %s", raw)
	}
	// A protocol mismatch never carries lanes either.
	c.hello = json.RawMessage(`{"gen":1}`)
	send(t, r, c, `{"tango":"hello","proto":1}`)
	if raw := c.nextRaw(t); strings.Contains(raw, "lanes") {
		t.Fatalf("mismatch reply %s", raw)
	}
}

func TestHandleBinaryDelivers(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	got := make(chan []byte, 1)
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		p, err := s.ReceiveBytes()
		if err != nil {
			return err
		}
		got <- p
		<-s.Context().Done()
		return nil
	})
	openBin(t, r, c, 1, "Sink")
	packet := AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte{1, 2, 3}})
	r.HandleBinary(c, packet)
	for i := range packet {
		packet[i] = 0xEE // HandleBinary copied the payload
	}
	if p := within(t, got, 2*time.Second, "ReceiveBytes"); !bytes.Equal(p, []byte{1, 2, 3}) {
		t.Fatalf("received %v", p)
	}
	if o := c.next(t); o.Op != "ack" || o.ID != 1 || o.B != 3+FrameOverhead {
		t.Fatalf("want ack b=67, got %+v", o)
	}
}

func TestHandleBinaryMalformed(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	r := NewRouter()
	c := newFakeBin("")
	r.HandleDuplex("Echo", echoDuplex)
	openBin(t, r, c, 1, "Echo")
	openBin(t, r, c, 3, "Echo")

	// Truncated: a valid frame for stream 1, then half a frame header. The
	// whole packet is dropped, so stream 1's seq 0 is not consumed.
	truncated := AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte("bad")})
	truncated[1] = 2
	truncated = append(truncated, header(nil, 1, 1, 0)[:10]...)
	r.HandleBinary(c, truncated)
	// len > MaxFrameBytes.
	r.HandleBinary(c, header([]byte{0, 1}, 1, 0, MaxFrameBytes+1))
	r.HandleBinary(c, nil)
	c.none(t, 50*time.Millisecond)
	if n := strings.Count(logs.String(), "ipc: malformed lane packet"); n != 3 {
		t.Fatalf("want 3 malformed warnings, got %d:\n%s", n, logs.String())
	}

	// Unknown stream ids are ignored; the other frames of the packet are
	// delivered.
	r.HandleBinary(c, AppendPacket(nil,
		PacketFrame{Stream: 99, Seq: 0, Payload: []byte("zz")},
		PacketFrame{Stream: 3, Seq: 0, Payload: []byte("hi")},
	))
	acked := 0
	if o := expectDataSkipAcks(t, c.fakeConn, 3, 0, 2+FrameOverhead, &acked); o.Bin == nil || *o.Bin != "aGk=" {
		t.Fatalf("stream 3 echo %+v", o)
	}

	// Stream 1 is untouched: seq 0 is still expected.
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":%d,"bin":"YWJj"}`, 3+FrameOverhead)
	if o := expectDataSkipAcks(t, c.fakeConn, 1, 0, 3+FrameOverhead, &acked); o.Bin == nil || *o.Bin != "YWJj" {
		t.Fatalf("stream 1 echo %+v", o)
	}
	// And stream 3 keeps going over the lane.
	r.HandleBinary(c, AppendPacket(nil, PacketFrame{Stream: 3, Seq: 1, Payload: []byte("yo")}))
	if o := expectDataSkipAcks(t, c.fakeConn, 3, 1, 2+FrameOverhead, &acked); o.Bin == nil || *o.Bin != "eW8=" {
		t.Fatalf("stream 3 second echo %+v", o)
	}
}

// HandleBinary on a connection that has no state yet (no hello, no
// streams) does nothing.
func TestHandleBinaryUnknownConn(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	r.HandleBinary(c, AppendPacket(nil, PacketFrame{Stream: 1, Seq: 0, Payload: []byte("x")}))
	r.HandleBinary(c, AppendPacket(nil))
	c.none(t, 20*time.Millisecond)
}
