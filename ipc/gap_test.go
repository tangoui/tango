package ipc

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

// recvErrDuplex receives until an error and reports it (io.EOF included).
func recvErrDuplex(errs chan<- error) DuplexFunc {
	return func(s *Stream, _ json.RawMessage) error {
		for {
			if _, err := s.Receive(); err != nil {
				errs <- err
				return err
			}
		}
	}
}

// laneFrame hands one byte frame of stream id to r as an upstream lane
// packet.
func laneFrame(r *Router, c *fakeBinConn, id, seq uint64, p string) {
	r.HandleBinary(c, AppendPacket(nil, PacketFrame{Stream: id, Seq: seq, Payload: []byte(p)}))
}

func wantTransportError(t *testing.T, c *fakeBinConn, id uint64) {
	t.Helper()
	acked := 0
	o := nextSkipAcks(t, c.fakeConn, id, &acked)
	if o.Op != "error" || o.ID != id || o.Error == nil || o.Error.Code != CodeTransport {
		t.Fatalf("want error transport on %d, got %+v", id, o)
	}
}

func TestGapTimeoutTransport(t *testing.T) {
	r := NewRouter()
	setGapTimeout(r, 100*time.Millisecond)
	c := newFakeBin("")
	errs := make(chan error, 1)
	r.HandleDuplex("Sink", recvErrDuplex(errs))
	r.HandleDuplex("Echo", echoDuplex)
	openBin(t, r, c, 1, "Sink")
	openBin(t, r, c, 3, "Echo")

	start := time.Now()
	laneFrame(r, c, 1, 1, "late") // seq 0 never arrives
	wantTransportError(t, c, 1)
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("error after %v, before the gap timeout", d)
	}
	wantCode(t, within(t, errs, time.Second, "Receive"), CodeTransport, "frames lost on a fast lane")

	// Another stream on the same connection is unaffected.
	send(t, r, c, `{"tango":"stream","id":3,"op":"data","seq":0,"c":%d,"bin":"aGk="}`, 2+FrameOverhead)
	acked := 0
	if o := expectDataSkipAcks(t, c.fakeConn, 3, 0, 2+FrameOverhead, &acked); o.Bin == nil || *o.Bin != "aGk=" {
		t.Fatalf("stream 3 echo %+v", o)
	}
	c.none(t, 150*time.Millisecond) // nothing more for stream 1
}

// A close counting a frame that never arrives is a gap too: Receive must not
// wait forever for it.
func TestGapBeforeCloseTransport(t *testing.T) {
	r := NewRouter()
	setGapTimeout(r, 100*time.Millisecond)
	c := newFakeBin("")
	errs := make(chan error, 1)
	r.HandleDuplex("Sink", recvErrDuplex(errs))
	openBin(t, r, c, 1, "Sink")

	send(t, r, c, `{"tango":"stream","id":1,"op":"end","seq":1}`) // seq 0 lost
	wantTransportError(t, c, 1)
	wantCode(t, within(t, errs, time.Second, "Receive"), CodeTransport, "")
	c.none(t, 150*time.Millisecond)
}

func TestGapFilledInTime(t *testing.T) {
	r := NewRouter()
	setGapTimeout(r, 100*time.Millisecond)
	c := newFakeBin("")
	r.HandleDuplex("Echo", echoDuplex)
	openBin(t, r, c, 1, "Echo")

	laneFrame(r, c, 1, 1, "b")
	time.Sleep(20 * time.Millisecond)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":%d,"bin":"YQ=="}`, 1+FrameOverhead)
	acked := 0
	if o := expectDataSkipAcks(t, c.fakeConn, 1, 0, 1+FrameOverhead, &acked); *o.Bin != "YQ==" {
		t.Fatalf("seq 0 echo %+v", o)
	}
	if o := expectDataSkipAcks(t, c.fakeConn, 1, 1, 1+FrameOverhead, &acked); *o.Bin != "Yg==" {
		t.Fatalf("seq 1 echo %+v", o)
	}
	noneSkipAcks(t, c.fakeConn, 250*time.Millisecond)
}

// When recvNext advances but a gap remains, the timer restarts from that
// moment instead of keeping the first deadline. Timings: a 1 s timer and
// two 550 ms steps, so each step tolerates 450 ms of scheduler delay
// (sleeps never return early, so the second step always ends past the
// first deadline).
func TestGapTimerRestartsOnProgress(t *testing.T) {
	r := NewRouter()
	setGapTimeout(r, time.Second)
	c := newFakeBin("")
	r.HandleDuplex("Echo", echoDuplex)
	openBin(t, r, c, 1, "Echo")

	r.HandleBinary(c, AppendPacket(nil,
		PacketFrame{Stream: 1, Seq: 1, Payload: []byte("b")},
		PacketFrame{Stream: 1, Seq: 3, Payload: []byte("d")},
	))
	time.Sleep(550 * time.Millisecond)
	laneFrame(r, c, 1, 0, "a")         // recvNext 0 → 2; seq 2 still missing
	time.Sleep(550 * time.Millisecond) // past the first deadline (1 s)
	laneFrame(r, c, 1, 2, "c")
	acked := 0
	for seq, want := range []string{"YQ==", "Yg==", "Yw==", "ZA=="} {
		if o := expectDataSkipAcks(t, c.fakeConn, 1, uint64(seq), 1+FrameOverhead, &acked); *o.Bin != want {
			t.Fatalf("seq %d echo %+v", seq, o)
		}
	}
	noneSkipAcks(t, c.fakeConn, 400*time.Millisecond)
}

func TestGapTimerStopsOnFinish(t *testing.T) {
	base := runtime.NumGoroutine()
	r := NewRouter()
	setGapTimeout(r, 100*time.Millisecond)
	c := newFakeBin("")
	errs := make(chan error, 1)
	r.HandleDuplex("Sink", recvErrDuplex(errs))
	openBin(t, r, c, 1, "Sink")

	laneFrame(r, c, 1, 1, "late")
	s := r.state(c).stream(1)
	if s == nil || !gapTimerActive(s) {
		t.Fatal("gap timer not running")
	}
	send(t, r, c, `{"tango":"stream","id":1,"op":"cancel"}`)
	wantCode(t, within(t, errs, time.Second, "Receive"), CodeCanceled, "")
	if gapTimerActive(s) {
		t.Fatal("gap timer still set after finish")
	}
	c.none(t, 200*time.Millisecond) // 2× the timeout: no error goes out
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: %d, baseline %d", runtime.NumGoroutine(), base)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A local Close drops pending frames, so the gap timer stops with it.
func TestGapTimerStopsOnLocalClose(t *testing.T) {
	r := NewRouter()
	setGapTimeout(r, 100*time.Millisecond)
	c := newFakeBin("")
	ready := make(chan *Stream, 1)
	r.HandleDuplex("Hold", func(s *Stream, _ json.RawMessage) error {
		ready <- s
		<-s.Context().Done()
		return nil
	})
	openBin(t, r, c, 1, "Hold")
	s := within(t, ready, time.Second, "handler")
	laneFrame(r, c, 1, 1, "late")
	if !gapTimerActive(s) {
		t.Fatal("gap timer not running")
	}
	s.Close()
	if o := c.next(t); o.Op != "close" || o.ID != 1 {
		t.Fatalf("want close, got %+v", o)
	}
	if gapTimerActive(s) {
		t.Fatal("gap timer still set after Close")
	}
	c.none(t, 200*time.Millisecond)
}

// noneSkipAcks fails if anything other than an ack arrives within d.
func noneSkipAcks(t *testing.T, c *fakeConn, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m := <-c.ch:
			var o out
			if err := json.Unmarshal([]byte(m), &o); err != nil || o.Op != "ack" {
				t.Fatalf("unexpected message %s", m)
			}
		case <-deadline:
			return
		}
	}
}
