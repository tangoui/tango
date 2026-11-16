package ipc

import (
	"sync"
	"testing"
	"time"
)

// fakeOrderedConn is a fakeBinConn that also implements OrderedBinaryConn:
// SendBinaryOrdered reports ordered for the packets it takes.
type fakeOrderedConn struct {
	*fakeBinConn
	omu     sync.Mutex
	ordered bool
}

func (c *fakeOrderedConn) SendBinaryOrdered(p []byte) (sent, ordered bool) {
	c.omu.Lock()
	o := c.ordered
	c.omu.Unlock()
	if !c.SendBinary(p) {
		return false, false
	}
	return true, o
}

func (c *fakeOrderedConn) setOrdered(v bool) {
	c.omu.Lock()
	c.ordered = v
	c.omu.Unlock()
}

var _ OrderedBinaryConn = (*fakeOrderedConn)(nil)

// A lane that delivers in order with the message channel (down:poll) needs
// no boundary: its packets cannot be lost while later messages arrive. When
// the same connection's lane is not ordered, boundary is sent as before.
func TestOrderedLaneSkipsBoundary(t *testing.T) {
	r := NewRouter()
	c := &fakeOrderedConn{fakeBinConn: newFakeBin(""), ordered: true}
	c.min, c.accept = 0, true
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	a, b, d := pattern(100, 1), pattern(200, 2), pattern(300, 3)
	cmds <- a
	within(t, errs, 2*time.Second, "write a")
	cmds <- b
	within(t, errs, 2*time.Second, "write b")
	ps := waitPackets(t, c.fakeBinConn, 2)
	expectLaneFrame(t, ps[0], 1, 0, a)
	expectLaneFrame(t, ps[1], 1, 1, b)
	c.none(t, 30*time.Millisecond) // no boundary on the message channel

	// The lane stops being ordered (say, another lane took over): boundary
	// again, so a lost packet is still detected.
	c.setOrdered(false)
	cmds <- d
	within(t, errs, 2*time.Second, "write d")
	ps = waitPackets(t, c.fakeBinConn, 3)
	expectLaneFrame(t, ps[2], 1, 2, d)
	expectBoundary(t, c.fakeBinConn, 1, 3)

	close(cmds)
	if o := c.next(t); o.Op != "close" || o.Seq != 3 {
		t.Fatalf("want close seq 3, got %+v", o)
	}
}

// A refused ordered send falls back to the message channel, without a
// boundary.
func TestOrderedLaneRefusedFallsBack(t *testing.T) {
	r := NewRouter()
	c := &fakeOrderedConn{fakeBinConn: newFakeBin(""), ordered: true}
	c.min, c.accept = 0, false
	cmds, errs := writer(r, "W")
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	if o := c.next(t); o.Op != "accept" {
		t.Fatalf("want accept, got %+v", o)
	}
	a := pattern(100, 1)
	cmds <- a
	within(t, errs, 2*time.Second, "write a")
	expectBin(t, c.fakeBinConn, 1, 0, a)
	c.none(t, 30*time.Millisecond)
	close(cmds)
	if o := c.next(t); o.Op != "close" || o.Seq != 1 {
		t.Fatalf("want close seq 1, got %+v", o)
	}
}
