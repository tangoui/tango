package ipc

import (
	"testing"
	"time"
)

func TestHelloV2Reply(t *testing.T) {
	r := NewRouter()
	c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	select {
	case raw := <-c.ch:
		if raw != `{"tango":"hello","proto":2,"win":1048576}` {
			t.Fatalf("reply %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for hello reply")
	}
}

func TestHelloSetStreamWindow(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(8192)
	c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	if o := c.next(t); o.Tango != "hello" || o.Proto != ProtoVersion || o.Win != 8192 {
		t.Fatalf("reply %+v", o)
	}
}

func TestHelloProtoMismatch(t *testing.T) {
	for _, m := range []string{`{"tango":"hello"}`, `{"tango":"hello","proto":1}`} {
		r := NewRouter()
		c := newFake()
		send(t, r, c, "%s", m)
		o := c.next(t)
		if o.Tango != "hello" || o.Proto != 0 || o.Win != 0 || o.Error == nil ||
			o.Error.Code != CodeBadArgs || o.Error.Message != "runtime-js protocol mismatch" {
			t.Fatalf("%s → %+v", m, o)
		}
	}
}

func TestSetStreamWindowTooSmall(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096) // the minimum is allowed
	defer func() {
		if recover() == nil {
			t.Fatal("SetStreamWindow(4095) did not panic")
		}
	}()
	r.SetStreamWindow(4095)
}
