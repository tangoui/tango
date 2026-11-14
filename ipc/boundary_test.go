package ipc

import (
	"testing"
	"time"
)

// A reliable boundary exposes a lost tail even when the duplex stays open.
func TestLaneBoundaryLostOpenTail(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096) // a 4032-byte lost frame fills the receive window
	setGapTimeout(r, 30*time.Millisecond)
	c := newFakeBin("")
	errs := make(chan error, 1)
	r.HandleDuplex("Sink", recvErrDuplex(errs))
	r.HandleDuplex("Echo", echoDuplex)
	openBin(t, r, c, 1, "Sink")
	openBin(t, r, c, 3, "Echo")
	send(t, r, c, `{"tango":"stream","id":1,"op":"boundary","seq":1}`)
	wantTransportError(t, c, 1)
	wantCode(t, within(t, errs, time.Second, "Receive"), CodeTransport, "")
	send(t, r, c, `{"tango":"stream","id":3,"op":"boundary","seq":1}`)
	send(t, r, c, `{"tango":"stream","id":3,"op":"data","seq":0,"c":66,"bin":"aGk="}`)
	acked := 0
	expectDataSkipAcks(t, c.fakeConn, 3, 0, 66, &acked)
}
