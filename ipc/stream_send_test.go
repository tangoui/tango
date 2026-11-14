package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// nextRaw returns the next raw message sent to the page.
func (f *fakeConn) nextRaw(t *testing.T) string {
	t.Helper()
	select {
	case m := <-f.ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
	return ""
}

// openJS opens stream id from the "JS" side and consumes Go's accept.
func openJS(t *testing.T, r *Router, c *fakeConn, id uint64, method string) {
	t.Helper()
	send(t, r, c, `{"tango":"stream","id":%d,"op":"open","method":%q,"args":[]}`, id, method)
	if o := c.next(t); o.Tango != "stream" || o.ID != id || o.Op != "accept" {
		t.Fatalf("open %s: want accept, got %+v", method, o)
	}
}

// expectData reads one data message and checks its id, seq and charged size.
func expectData(t *testing.T, c *fakeConn, id, seq uint64, cost int) out {
	t.Helper()
	o := c.next(t)
	if o.Op != "data" || o.ID != id || o.Seq != seq || o.C != cost {
		t.Fatalf("want data id=%d seq=%d c=%d, got %+v", id, seq, cost, o)
	}
	return o
}

func TestStreamDataEncoding(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleStream("Enc", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		s := w.(*Stream)
		if err := s.Write(map[string]int{"a": 1}); err != nil {
			return err
		}
		if err := s.WriteBytes([]byte("hello")); err != nil {
			return err
		}
		return s.WriteBytes(nil)
	})
	openJS(t, r, c, 1, "Enc")

	o := expectData(t, c, 1, 0, len(`{"a":1}`)+FrameOverhead) // 71
	if string(o.Data) != `{"a":1}` || o.Bin != nil {
		t.Fatalf("json frame %+v", o)
	}
	o = expectData(t, c, 1, 1, 69)
	if o.Bin == nil || *o.Bin != "aGVsbG8=" || o.Data != nil {
		t.Fatalf("bytes frame %+v", o)
	}
	o = expectData(t, c, 1, 2, 64)
	if o.Bin == nil || *o.Bin != "" || o.Data != nil {
		t.Fatalf("empty bytes frame %+v", o)
	}
	if o := c.next(t); o.Op != "close" || o.Seq != 3 || o.Error != nil {
		t.Fatalf("want close seq 3, got %+v", o)
	}
	c.none(t, 20*time.Millisecond)
}

func TestStreamCreditBytes(t *testing.T) {
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	r.HandleStream("Bulk", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		for i := 0; i < 10; i++ {
			if err := w.(*Stream).WriteBytes(make([]byte, 1000)); err != nil {
				return err
			}
		}
		return nil
	})
	openJS(t, r, c, 1, "Bulk")
	for i := uint64(0); i < 3; i++ {
		expectData(t, c, 1, i, 1064)
	}
	c.none(t, 50*time.Millisecond) // 4*1064 > 4096: blocked
	send(t, r, c, `{"tango":"stream","id":1,"op":"ack","b":1064}`)
	expectData(t, c, 1, 3, 1064)
	c.none(t, 50*time.Millisecond)
	r.Drop(c)
}

func TestStreamOversizeFrameWhenIdle(t *testing.T) {
	r := NewRouter()
	setJSSendWindow(r, 4096)
	c := newFake()
	r.HandleStream("Big", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		s := w.(*Stream)
		if err := s.WriteBytes(make([]byte, 10000)); err != nil {
			return err
		}
		return s.WriteBytes([]byte{1})
	})
	openJS(t, r, c, 1, "Big")
	expectData(t, c, 1, 0, 10064) // nothing in flight: may exceed the window
	c.none(t, 50*time.Millisecond)
	send(t, r, c, `{"tango":"stream","id":1,"op":"ack","b":10064}`)
	if o := expectData(t, c, 1, 1, 65); o.Bin == nil || *o.Bin != "AQ==" {
		t.Fatalf("got %+v", o)
	}
	if o := c.next(t); o.Op != "close" || o.Seq != 2 {
		t.Fatalf("got %+v", o)
	}
}

func TestStreamMaxFrame(t *testing.T) {
	r := NewRouter()
	c := newFake()
	bigErr := make(chan error, 1)
	r.HandleStream("Max", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		s := w.(*Stream)
		bigErr <- s.WriteBytes(make([]byte, MaxFrameBytes+1))
		return s.WriteBytes([]byte("x"))
	})
	openJS(t, r, c, 1, "Max")
	var e *Error
	if err := <-bigErr; !errors.As(err, &e) || e.Code != CodeBadArgs {
		t.Fatalf("oversize WriteBytes err = %v", err)
	}
	if o := expectData(t, c, 1, 0, 65); o.Bin == nil || *o.Bin != "eA==" {
		t.Fatalf("got %+v", o)
	}
	if o := c.next(t); o.Op != "close" || o.Seq != 1 {
		t.Fatalf("got %+v", o)
	}
}

func TestStreamAcceptInFalse(t *testing.T) {
	r := NewRouter()
	c := newFake()
	release := make(chan struct{})
	r.HandleStream("Feed", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		<-release
		return nil
	})
	send(t, r, c, `{"tango":"stream","id":1,"op":"open","method":"Feed","args":[]}`)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"accept","in":false}` {
		t.Fatalf("first reply %s", raw)
	}
	close(release)
	if o := c.next(t); o.Op != "close" || o.Seq != 0 {
		t.Fatalf("got %+v", o)
	}
}

func TestHandlerReturnSendsCloseOrError(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleStream("Ok", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		w.Write(1)
		w.Write(2)
		return nil
	})
	r.HandleStream("Boom", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		w.Write(1)
		w.Write(2)
		return errors.New("boom")
	})

	openJS(t, r, c, 1, "Ok")
	expectData(t, c, 1, 0, 65)
	expectData(t, c, 1, 1, 65)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":1,"op":"close","seq":2}` {
		t.Fatalf("close %s", raw)
	}

	openJS(t, r, c, 3, "Boom")
	expectData(t, c, 3, 0, 65)
	expectData(t, c, 3, 1, 65)
	if raw := c.nextRaw(t); raw != `{"tango":"stream","id":3,"op":"error","seq":2,"error":{"message":"boom","code":"internal"}}` {
		t.Fatalf("error %s", raw)
	}
	c.none(t, 20*time.Millisecond)
}

func TestStreamRawMessageCostMatchesWire(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	raw := json.RawMessage(`{ "a" : "<b>" }`)
	if err := s.Write(raw); err != nil {
		t.Fatal(err)
	}
	raw[3] = 'Z' // the caller may reuse its buffer
	m := c.nextRaw(t)
	bs := "\\" // encoding/json HTML-escapes < and > as backslash-u003c / -u003e
	want := `{"a":"` + bs + `u003cb` + bs + `u003e"}`
	if m != `{"tango":"stream","id":2,"op":"data","seq":0,"c":`+fmt.Sprint(len(want)+FrameOverhead)+`,"data":`+want+`}` {
		t.Fatalf("got %s", m)
	}
}

func TestGoOpenedAcceptInFalse(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, _ := r.OpenStream(c, "x")
	c.next(t) // open
	send(t, r, c, `{"tango":"stream","id":2,"op":"accept","in":false}`)
	var e *Error
	if err := s.WriteBytes([]byte{1}); !errors.As(err, &e) || e.Code != CodeBadArgs || e.Message != "stream does not accept data" {
		t.Fatalf("WriteBytes err = %v", err)
	}
	c.none(t, 20*time.Millisecond)
	s.Close()
	if o := c.next(t); o.Op != "close" || o.Seq != 0 {
		t.Fatalf("got %+v", o)
	}
}
