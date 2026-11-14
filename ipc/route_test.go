package ipc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// pattern returns n bytes that differ by position, seeded so frames differ.
func pattern(n int, seed byte) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i*7) + seed
	}
	return p
}

// lanePackets returns a copy of the packets SendBinary accepted so far.
func (c *fakeBinConn) lanePackets() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte{}, c.sent...)
}

func (c *fakeBinConn) setAccept(v bool) {
	c.mu.Lock()
	c.accept = v
	c.mu.Unlock()
}

// waitPackets waits until exactly n packets were sent on the lane.
func waitPackets(t *testing.T, c *fakeBinConn, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		ps := c.lanePackets()
		if len(ps) == n {
			return ps
		}
		if len(ps) > n || time.Now().After(deadline) {
			t.Fatalf("want %d lane packets, have %d", n, len(ps))
		}
		time.Sleep(time.Millisecond)
	}
}

// expectLaneFrame checks that packet p holds exactly one frame {id, seq, want}.
func expectLaneFrame(t *testing.T, p []byte, id, seq uint64, want []byte) {
	t.Helper()
	fs, err := ParsePacket(p)
	if err != nil {
		t.Fatalf("lane packet: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("lane packet: want 1 frame, got %d", len(fs))
	}
	if fs[0].Stream != id || fs[0].Seq != seq || !bytes.Equal(fs[0].Payload, want) {
		t.Fatalf("lane packet: want id=%d seq=%d len=%d, got id=%d seq=%d len=%d",
			id, seq, len(want), fs[0].Stream, fs[0].Seq, len(fs[0].Payload))
	}
}

// expectBin reads one message-channel byte frame and checks its payload.
func expectBin(t *testing.T, c *fakeBinConn, id, seq uint64, want []byte) {
	t.Helper()
	o := expectData(t, c.fakeConn, id, seq, len(want)+FrameOverhead)
	if o.Bin == nil || o.Data != nil || *o.Bin != base64.StdEncoding.EncodeToString(want) {
		t.Fatalf("seq %d: want bin of %d bytes, got %+v", seq, len(want), o)
	}
}

// writer registers a stream handler that writes whatever the test sends on
// the returned channel: []byte via WriteBytes, anything else via Write.
// Closing the channel ends the stream. Each write's error is reported on
// errs.
func writer(r *Router, method string) (chan<- any, <-chan error) {
	cmds := make(chan any)
	errs := make(chan error, 64)
	r.HandleStream(method, func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		s := w.(*Stream)
		for v := range cmds {
			var err error
			if p, ok := v.([]byte); ok {
				err = s.WriteBytes(p)
			} else {
				err = s.Write(v)
			}
			errs <- err
			if err != nil {
				return err
			}
		}
		return nil
	})
	return cmds, errs
}

func TestRoutingByMinSize(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	c.min, c.accept = 4096, true
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	small, big := pattern(100, 1), pattern(64<<10, 2)
	cmds <- small
	within(t, errs, 2*time.Second, "write small")
	expectBin(t, c, 1, 0, small)

	cmds <- big
	within(t, errs, 2*time.Second, "write big")
	ps := waitPackets(t, c, 1)
	expectLaneFrame(t, ps[0], 1, 1, big)
	expectBoundary(t, c, 1, 2)
	c.none(t, 30*time.Millisecond) // no data for the big frame on the message channel

	// Exactly min goes to the lane too; min-1 does not.
	edge := pattern(4096, 3)
	cmds <- edge
	within(t, errs, 2*time.Second, "write edge")
	ps = waitPackets(t, c, 2)
	expectLaneFrame(t, ps[1], 1, 2, edge)
	expectBoundary(t, c, 1, 3)
	below := pattern(4095, 4)
	cmds <- below
	within(t, errs, 2*time.Second, "write below")
	expectBin(t, c, 1, 3, below)

	close(cmds)
	if o := c.next(t); o.Op != "close" || o.Seq != 4 {
		t.Fatalf("want close seq 4, got %+v", o)
	}
	waitPackets(t, c, 2)
}

func TestRoutingFallbackWhenRejected(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	c.min, c.accept = 0, false
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	big := pattern(64<<10, 5)
	for i := uint64(0); i < 3; i++ {
		cmds <- big
		within(t, errs, 2*time.Second, "write")
		expectBin(t, c, 1, i, big)
	}
	if ps := c.lanePackets(); len(ps) != 0 {
		t.Fatalf("rejected lane kept %d packets", len(ps))
	}

	// The decision is per frame: once the lane accepts again, the next frame
	// takes it, and rejecting again falls back again, seq still contiguous.
	c.setAccept(true)
	cmds <- big
	within(t, errs, 2*time.Second, "write")
	expectLaneFrame(t, waitPackets(t, c, 1)[0], 1, 3, big)
	expectBoundary(t, c, 1, 4)
	c.setAccept(false)
	cmds <- big
	within(t, errs, 2*time.Second, "write")
	expectBin(t, c, 1, 4, big)

	close(cmds)
	if o := c.next(t); o.Op != "close" || o.Seq != 5 {
		t.Fatalf("want close seq 5, got %+v", o)
	}
}

func TestRoutingDisabledWhenMinNegative(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	c.min, c.accept = -1, true
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	for i, n := range []int{0, 1, 100, 64 << 10} {
		p := pattern(n, byte(i))
		cmds <- p
		within(t, errs, 2*time.Second, "write")
		expectBin(t, c, 1, uint64(i), p)
	}
	close(cmds)
	if o := c.next(t); o.Op != "close" || o.Seq != 4 {
		t.Fatalf("want close seq 4, got %+v", o)
	}
	if ps := c.lanePackets(); len(ps) != 0 {
		t.Fatalf("disabled lane got %d packets", len(ps))
	}
}

func TestRoutingKeepsOrderAcrossLanes(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	c.min, c.accept = 4096, true
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	a, big, b := pattern(100, 1), pattern(64<<10, 2), pattern(100, 3)
	bigJSON := strings.Repeat("x", 64<<10) // JSON frames never use the lane
	for _, v := range []any{a, big, b, bigJSON} {
		cmds <- v
		within(t, errs, 2*time.Second, "write")
	}
	close(cmds)

	expectBin(t, c, 1, 0, a)
	expectBoundary(t, c, 1, 2)
	expectBin(t, c, 1, 2, b)
	o := expectData(t, c.fakeConn, 1, 3, len(bigJSON)+2+FrameOverhead)
	if o.Bin != nil || string(o.Data) != `"`+bigJSON+`"` {
		t.Fatalf("json frame not on the message channel: bin=%v data len %d", o.Bin != nil, len(o.Data))
	}
	if o := c.next(t); o.Op != "close" || o.Seq != 4 {
		t.Fatalf("want close seq 4, got %+v", o)
	}
	ps := waitPackets(t, c, 1)
	expectLaneFrame(t, ps[0], 1, 1, big)
}

func TestRoutingCreditUnchanged(t *testing.T) {
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFakeBin("")
	c.min, c.accept = 0, true
	r.HandleStream("Bulk", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		for i := 0; i < 5; i++ {
			if err := w.(*Stream).WriteBytes(pattern(1000, byte(i))); err != nil {
				return err
			}
		}
		return nil
	})
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"Bulk","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	ps := waitPackets(t, c, 3)
	time.Sleep(50 * time.Millisecond) // 4*1064 > 4096: the fourth is blocked
	waitPackets(t, c, 3)
	for i := uint64(1); i <= 3; i++ {
		expectBoundary(t, c, 1, i)
	}
	c.none(t, 10*time.Millisecond)

	send(t, r, c, `{"tango":"stream","id":1,"op":"ack","b":1064}`)
	ps = waitPackets(t, c, 4)
	time.Sleep(50 * time.Millisecond)
	waitPackets(t, c, 4) // one ack of 1064 frees exactly one more frame
	send(t, r, c, `{"tango":"stream","id":1,"op":"ack","b":1064}`)
	ps = waitPackets(t, c, 5)
	expectBoundary(t, c, 1, 4)
	expectBoundary(t, c, 1, 5)
	for i, p := range ps {
		expectLaneFrame(t, p, 1, uint64(i), pattern(1000, byte(i)))
	}
	if o := c.next(t); o.Op != "close" || o.Seq != 5 {
		t.Fatalf("want close seq 5, got %+v", o)
	}
}

// A stream Go opens keeps its byte frames off the downstream lane until
// JS accepted it: a lane packet could overtake the open message and reach
// a page that does not know the stream yet. A stream JS opened (Go
// accepts) may use the lane at once (TestRoutingByMinSize).
func TestRoutingWaitsForAcceptOnGoOpened(t *testing.T) {
	r := NewRouter()
	c := newFakeBin("")
	c.min, c.accept = 4096, true
	s, err := r.OpenStream(c, "Sink")
	if err != nil {
		t.Fatal(err)
	}
	o := c.next(t)
	if o.Op != "open" {
		t.Fatalf("want open, got %+v", o)
	}
	id := o.ID
	before, after := pattern(64<<10, 5), pattern(64<<10, 6)
	if err := s.WriteBytes(before); err != nil {
		t.Fatal(err)
	}
	expectBin(t, c, id, 0, before)
	if n := len(c.lanePackets()); n != 0 {
		t.Fatalf("%d lane packets before accept, want 0", n)
	}
	send(t, r, c, `{"tango":"stream","id":%d,"op":"accept","in":true}`, id)
	if err := s.WriteBytes(after); err != nil {
		t.Fatal(err)
	}
	ps := waitPackets(t, c, 1)
	expectLaneFrame(t, ps[0], id, 1, after)
	expectBoundary(t, c, id, 2)
	c.none(t, 30*time.Millisecond)
	s.Close()
}

func expectBoundary(t *testing.T, c *fakeBinConn, id, seq uint64) {
	t.Helper()
	o := c.next(t)
	if o.Op != "boundary" || o.ID != id || o.Seq != seq {
		t.Fatalf("want boundary %d/%d, got %+v", id, seq, o)
	}
}
