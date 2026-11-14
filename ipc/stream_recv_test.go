package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// openDuplex opens stream id from the "JS" side and checks Go's accept
// advertises in:true.
func openDuplex(t *testing.T, r *Router, c *fakeConn, id uint64, method string) {
	t.Helper()
	send(t, r, c, `{"tango":"stream","id":%d,"op":"open","method":%q,"args":[]}`, id, method)
	o := c.next(t)
	if o.Tango != "stream" || o.ID != id || o.Op != "accept" || o.In == nil || !*o.In {
		t.Fatalf("open %s: want accept in:true, got %+v", method, o)
	}
}

// sendBin sends one byte frame from "JS" with the correct charged size.
func sendBin(t *testing.T, r *Router, c *fakeConn, id, seq uint64, p []byte) {
	t.Helper()
	send(t, r, c, `{"tango":"stream","id":%d,"op":"data","seq":%d,"c":%d,"bin":%q}`,
		id, seq, len(p)+FrameOverhead, base64.StdEncoding.EncodeToString(p))
}

// echoDuplex receives until EOF and writes every frame back unchanged.
func echoDuplex(s *Stream, _ json.RawMessage) error {
	for {
		m, err := s.Receive()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if m.Binary {
			err = s.WriteBytes(m.Bytes)
		} else {
			err = s.Write(m.JSON)
		}
		if err != nil {
			return err
		}
	}
}

func wantBadArgs(t *testing.T, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeBadArgs {
		t.Fatalf("want *Error bad_args, got %v", err)
	}
}

// nextSkipAcks returns the next message that is not an ack for stream id,
// adding the credit of the acks it skips to *acked. Go acks as soon as its
// reader catches up, so how a handler's reads split into acks depends on
// timing; tests assert the total.
func nextSkipAcks(t *testing.T, c *fakeConn, id uint64, acked *int) out {
	t.Helper()
	for {
		o := c.next(t)
		if o.ID == id && o.Op == "ack" {
			*acked += o.B
			continue
		}
		return o
	}
}

// expectDataSkipAcks is expectData, skipping (and totaling) acks.
func expectDataSkipAcks(t *testing.T, c *fakeConn, id, seq uint64, cost int, acked *int) out {
	t.Helper()
	o := nextSkipAcks(t, c, id, acked)
	if o.Op != "data" || o.ID != id || o.Seq != seq || o.C != cost {
		t.Fatalf("want data id=%d seq=%d c=%d, got %+v", id, seq, cost, o)
	}
	return o
}

func TestDuplexEcho(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleDuplex("Echo", echoDuplex)
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"Echo","args":[]}`)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"accept","in":true}` {
		t.Fatalf("accept %s", raw)
	}
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":71,"data":{"x":1}}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":67,"bin":"AQID"}`)
	acked := 0
	if o := expectDataSkipAcks(t, c, 1, 0, 71, &acked); string(o.Data) != `{"x":1}` || o.Bin != nil {
		t.Fatalf("json echo %+v", o)
	}
	if o := expectDataSkipAcks(t, c, 1, 1, 67, &acked); o.Bin == nil || *o.Bin != "AQID" || o.Data != nil {
		t.Fatalf("bytes echo %+v", o)
	}
	send(t, r, c, `{"tango":"stream","id":1,"op":"end","seq":2}`)
	if o := nextSkipAcks(t, c, 1, &acked); o.Op != "close" || o.Seq != 2 {
		t.Fatalf("want close seq 2, got %+v", o)
	}
	if acked != 71+67 { // the reader caught up: all credit returned
		t.Fatalf("acked %d, want %d", acked, 71+67)
	}
	c.none(t, 20*time.Millisecond)
}

func TestDuplexLeftoverCreditFlushed(t *testing.T) {
	// Credit below half the window is returned once the reader has caught
	// up; otherwise a sender with that much in flight could block forever
	// on a frame that does not fit beside it.
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	read := make(chan struct{})
	readDone := make(chan error)
	r.HandleDuplex("L", func(s *Stream, _ json.RawMessage) error {
		for range read {
			_, err := s.Receive()
			readDone <- err
		}
		return nil
	})
	step := func() {
		t.Helper()
		read <- struct{}{}
		if err := <-readDone; err != nil {
			t.Fatal(err)
		}
	}
	openDuplex(t, r, c, 1, "L")

	// One frame, read: the queue is empty, so its 1064 is acked at once.
	sendBin(t, r, c, 1, 0, make([]byte, 1000))
	step()
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"ack","b":1064}` {
		t.Fatalf("ack %s", raw)
	}

	// Two frames queued: the first read leaves one queued (1064 < 2048,
	// no ack); the second empties the queue and acks both: 2128.
	sendBin(t, r, c, 1, 1, make([]byte, 1000))
	sendBin(t, r, c, 1, 2, make([]byte, 1000))
	time.Sleep(20 * time.Millisecond) // both delivered before reading
	step()
	c.none(t, 50*time.Millisecond)
	step()
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"ack","b":2128}` {
		t.Fatalf("ack %s", raw)
	}
	c.none(t, 20*time.Millisecond)
	close(read)
	r.Drop(c)
}

func TestDuplexReorder(t *testing.T) {
	r := NewRouter()
	c := newFake()
	got := make(chan []byte, 2)
	r.HandleDuplex("R", func(s *Stream, _ json.RawMessage) error {
		for i := 0; i < 2; i++ {
			b, err := s.ReceiveBytes()
			if err != nil {
				return err
			}
			got <- b
		}
		return nil
	})
	openDuplex(t, r, c, 1, "R")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":65,"bin":"Ag=="}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":65,"bin":"AQ=="}`)
	for _, want := range []byte{1, 2} {
		select {
		case b := <-got:
			if len(b) != 1 || b[0] != want {
				t.Fatalf("got %v, want [%d]", b, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestDuplexAckAfterConsume(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	start := make(chan struct{})
	done := make(chan error, 1)
	r.HandleDuplex("A", func(s *Stream, _ json.RawMessage) error {
		<-start
		for i := 0; i < 2; i++ {
			if _, err := s.Receive(); err != nil {
				done <- err
				return err
			}
		}
		done <- nil
		<-s.Context().Done()
		return nil
	})
	openDuplex(t, r, c, 1, "A")
	sendBin(t, r, c, 1, 0, make([]byte, 1000)) // c=1064
	sendBin(t, r, c, 1, 1, make([]byte, 1000))
	c.none(t, 50*time.Millisecond) // nothing read yet: no credit returned
	close(start)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"ack","b":2128}` {
		t.Fatalf("ack %s", raw)
	}
	c.none(t, 20*time.Millisecond)
	r.Drop(c)
}

func TestDuplexReceiveTypeMismatch(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct {
		bytesErr error
		m        Message
		err      error
	}
	got := make(chan res, 1)
	r.HandleDuplex("T", func(s *Stream, _ json.RawMessage) error {
		var x res
		_, x.bytesErr = s.ReceiveBytes()
		x.m, x.err = s.Receive()
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "T")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":68,"data":"hi"}`)
	x := <-got
	wantBadArgs(t, x.bytesErr)
	if x.err != nil || x.m.Binary || string(x.m.JSON) != `"hi"` {
		t.Fatalf("Receive after mismatch: %+v, %v", x.m, x.err)
	}
}

func TestDuplexReceiveJSON(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct {
		mismatch, decode, ok error
		v                    struct{ X int }
		b                    []byte
		null                 Message
	}
	got := make(chan res, 1)
	r.HandleDuplex("J", func(s *Stream, _ json.RawMessage) error {
		var x res
		var v struct{ X int }
		x.mismatch = s.ReceiveJSON(&v) // head is bytes: stays queued
		x.b, _ = s.ReceiveBytes()
		x.decode = s.ReceiveJSON(&v) // "str" into a struct: consumed
		x.ok = s.ReceiveJSON(&x.v)
		x.null, _ = s.Receive()
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "J")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":65,"bin":"Bw=="}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":69,"data":"str"}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":2,"c":71,"data":{"X":7}}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":3,"c":68,"data":null}`)
	x := <-got
	wantBadArgs(t, x.mismatch)
	if len(x.b) != 1 || x.b[0] != 7 {
		t.Fatalf("bytes after mismatch %v", x.b)
	}
	wantBadArgs(t, x.decode)
	if x.ok != nil || x.v.X != 7 {
		t.Fatalf("ReceiveJSON: %v %+v", x.ok, x.v)
	}
	if x.null.Binary || string(x.null.JSON) != "null" {
		t.Fatalf("null frame %+v", x.null)
	}
}

func TestDuplexBadFrames(t *testing.T) {
	r := NewRouter()
	c := newFake()
	errs := make(chan error, 8)
	r.HandleDuplex("B", func(s *Stream, _ json.RawMessage) error {
		_, err := s.Receive()
		errs <- err
		return err
	})
	bad := []string{
		`"c":70,"data":1,"bin":"AQ=="`, // both payloads
		`"c":70`,                       // neither payload
		`"data":1`,                     // missing c
		`"c":10,"data":1`,              // c below FrameOverhead
		`"c":66,"bin":"!!"`,            // invalid base64
	}
	for i, fields := range bad {
		id := uint64(2*i + 1)
		openDuplex(t, r, c, id, "B")
		send(t, r, c, `{"tango":"stream","id":%d,"op":"data","seq":0,%s}`, id, fields)
		o := c.next(t)
		if o.ID != id || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
			t.Fatalf("%s: want error bad_args, got %+v", fields, o)
		}
		select {
		case err := <-errs:
			wantBadArgs(t, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: Receive did not return", fields)
		}
		c.none(t, 20*time.Millisecond) // handler return sends nothing more
	}
}

func TestDecodeDataRejectsOversizedBytes(t *testing.T) {
	ok := base64.StdEncoding.EncodeToString(make([]byte, MaxFrameBytes))
	if _, err := decodeData(&wireIn{C: MaxFrameBytes + FrameOverhead, Bin: &ok}); err != nil {
		t.Fatalf("MaxFrameBytes payload: %v", err)
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, MaxFrameBytes+1))
	_, err := decodeData(&wireIn{C: MaxFrameBytes + 1 + FrameOverhead, Bin: &big})
	if err == nil || err.Code != CodeBadArgs {
		t.Fatalf("oversized payload: want bad_args, got %v", err)
	}
}

func TestDuplexFlowViolation(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	recvErr := make(chan error, 1)
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done() // never reads until the stream fails
		_, err := s.Receive()
		recvErr <- err
		return err
	})
	r.HandleDuplex("Echo", echoDuplex)
	openDuplex(t, r, c, 1, "Sink")
	// Held: MaxFrameBytes+64, then 4 x 1064 = 4256 > 4096 more.
	sendBin(t, r, c, 1, 0, make([]byte, MaxFrameBytes))
	for seq := uint64(1); seq <= 4; seq++ {
		sendBin(t, r, c, 1, seq, make([]byte, 1000))
	}
	o := c.next(t)
	if o.ID != 1 || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs || o.Error.Message != "flow control violated" {
		t.Fatalf("want error bad_args flow control violated, got %+v", o)
	}
	wantBadArgs(t, <-recvErr)

	openDuplex(t, r, c, 3, "Echo")
	send(t, r, c, `{"tango":"stream","id":3,"op":"data","seq":0,"c":65,"data":1}`)
	acked := 0
	if o := expectDataSkipAcks(t, c, 3, 0, 65, &acked); string(o.Data) != "1" || acked != 65 {
		t.Fatalf("echo %+v (acked %d)", o, acked)
	}
	r.Drop(c)
}

func TestDuplexFlowLimitBoundary(t *testing.T) {
	// Exactly recvWin+MaxFrameBytes+FrameOverhead held is still allowed.
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done()
		return nil
	})
	openDuplex(t, r, c, 1, "Sink")
	sendBin(t, r, c, 1, 0, make([]byte, MaxFrameBytes))
	sendBin(t, r, c, 1, 1, make([]byte, 4096-FrameOverhead))
	c.none(t, 50*time.Millisecond)
	sendBin(t, r, c, 1, 2, nil) // one more frame: 64 bytes over
	if o := c.next(t); o.Op != "error" || o.Error == nil || o.Error.Message != "flow control violated" {
		t.Fatalf("got %+v", o)
	}
}

func TestDuplexDuplicateFrame(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleDuplex("D", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done()
		return nil
	})
	for _, tc := range []struct {
		id   uint64
		seqs []uint64
	}{
		{1, []uint64{0, 0}}, // already delivered
		{3, []uint64{2, 2}}, // already buffered out of order
	} {
		openDuplex(t, r, c, tc.id, "D")
		for _, seq := range tc.seqs {
			sendBin(t, r, c, tc.id, seq, []byte{1})
		}
		o := c.next(t)
		if o.ID != tc.id || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs || o.Error.Message != "duplicate frame" {
			t.Fatalf("seqs %v: got %+v", tc.seqs, o)
		}
	}
}

func TestStreamFuncDropsInbound(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096)
	c := newFake()
	r.HandleStream("Feed", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		<-ctx.Done()
		return nil
	})
	openJS(t, r, c, 1, "Feed")
	sendBin(t, r, c, 1, 0, make([]byte, 1000))
	sendBin(t, r, c, 1, 1, make([]byte, 1000))
	// Dropped frames are never read: each one's credit returns at once.
	for i := 0; i < 2; i++ {
		if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"ack","b":1064}` {
			t.Fatalf("ack %d: %s", i, raw)
		}
	}
	c.none(t, 50*time.Millisecond)
	r.Drop(c)
}

func TestDuplexEndEOF(t *testing.T) {
	r := NewRouter()
	c := newFake()
	type res struct {
		frames [][]byte
		err    error
	}
	got := make(chan res, 1)
	r.HandleDuplex("E", func(s *Stream, _ json.RawMessage) error {
		var x res
		for {
			b, err := s.ReceiveBytes()
			if err != nil {
				x.err = err
				break
			}
			x.frames = append(x.frames, b)
		}
		got <- x
		return nil
	})
	openDuplex(t, r, c, 1, "E")
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":0,"c":65,"bin":"AQ=="}`)
	send(t, r, c, `{"tango":"stream","id":1,"op":"end","seq":2}`)
	time.Sleep(20 * time.Millisecond) // end is known before seq 1 arrives
	send(t, r, c, `{"tango":"stream","id":1,"op":"data","seq":1,"c":65,"bin":"Ag=="}`)
	x := <-got
	if x.err != io.EOF || len(x.frames) != 2 || x.frames[0][0] != 1 || x.frames[1][0] != 2 {
		t.Fatalf("frames %v err %v", x.frames, x.err)
	}
	acked := 0
	if o := nextSkipAcks(t, c, 1, &acked); o.Op != "close" || o.Seq != 0 || acked != 130 {
		t.Fatalf("got %+v (acked %d, want 130)", o, acked)
	}
}

func TestDuplexFrameAfterEnd(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleDuplex("F", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done() // does not read, so the stream stays open
		return nil
	})
	for _, tc := range []struct {
		id    uint64
		first string // frames before end, then end, then the bad frame
		end   uint64
		bad   uint64
	}{
		{1, "", 0, 0},
		{3, "AQ==", 1, 1},
	} {
		openDuplex(t, r, c, tc.id, "F")
		if tc.first != "" {
			send(t, r, c, `{"tango":"stream","id":%d,"op":"data","seq":0,"c":65,"bin":%q}`, tc.id, tc.first)
		}
		send(t, r, c, `{"tango":"stream","id":%d,"op":"end","seq":%d}`, tc.id, tc.end)
		sendBin(t, r, c, tc.id, tc.bad, []byte{9})
		o := c.next(t)
		if o.ID != tc.id || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
			t.Fatalf("id %d: want error bad_args, got %+v", tc.id, o)
		}
	}
}

func TestDuplexEndBelowReceivedFrames(t *testing.T) {
	// end seq N when a frame with seq >= N already arrived (delivered or
	// still buffered out of order) is a protocol error.
	r := NewRouter()
	c := newFake()
	r.HandleDuplex("F", func(s *Stream, _ json.RawMessage) error {
		<-s.Context().Done()
		return nil
	})
	for _, tc := range []struct {
		id   uint64
		seqs []uint64
	}{
		{1, []uint64{0, 1}}, // seq 1 delivered
		{3, []uint64{2}},    // seq 2 buffered
	} {
		openDuplex(t, r, c, tc.id, "F")
		for _, seq := range tc.seqs {
			sendBin(t, r, c, tc.id, seq, []byte{1})
		}
		send(t, r, c, `{"tango":"stream","id":%d,"op":"end","seq":1}`, tc.id)
		o := c.next(t)
		if o.ID != tc.id || o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
			t.Fatalf("id %d: want error bad_args, got %+v", tc.id, o)
		}
	}
}

func TestGoOpenedStreamReceives(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	send(t, r, c, `{"tango":"stream","id":2,"op":"accept","in":true}`)
	send(t, r, c, `{"tango":"stream","id":2,"op":"data","seq":0,"c":67,"data":"a"}`)
	m, err := s.Receive()
	if err != nil || m.Binary || string(m.JSON) != `"a"` {
		t.Fatalf("Receive %+v %v", m, err)
	}
	r.Drop(c)
	if _, err := s.Receive(); err == nil || err == io.EOF {
		t.Fatalf("Receive after Drop: %v", err)
	}
}

func TestHandleDuplexPanics(t *testing.T) {
	r := NewRouter()
	for _, f := range []func(){
		func() { r.HandleDuplex("", echoDuplex) },
		func() { r.HandleDuplex("x", nil) },
	} {
		func() {
			defer func() {
				if p := recover(); p == nil || !strings.Contains(p.(string), "HandleDuplex") {
					t.Fatalf("want HandleDuplex panic, got %v", p)
				}
			}()
			f()
		}()
	}
	r.HandleDuplex("x", echoDuplex)
	if !r.Has("x") {
		t.Fatal("Has(x) = false")
	}
}
