package ipc

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandlerPanicReported(t *testing.T) {
	r := NewRouter()
	type rep struct {
		c     Conn
		where string
		value any
		stack []byte
	}
	reps := make(chan rep, 4)
	r.OnPanic(func(c Conn, where string, value any, stack []byte) { reps <- rep{c, where, value, stack} })
	r.Handle("Svc.Boom", func(context.Context, json.RawMessage) (any, error) { panic("boom") })
	r.HandleStream("Svc.Flow", func(context.Context, json.RawMessage, StreamWriter) error { panic("flow") })
	c := newFake()

	send(t, r, c, `{"tango":"call","id":1,"method":"Svc.Boom","args":[]}`)
	o := c.next(t)
	if o.Tango != "ret" || o.Error == nil || o.Error.Code != CodeInternal || o.Error.Message != "panic: boom" {
		t.Fatalf("reply = %+v", o)
	}
	p := <-reps
	if p.c != c || p.where != "binding Svc.Boom" || p.value != "boom" || len(p.stack) == 0 {
		t.Fatalf("report = %+v", p)
	}

	send(t, r, c, `{"tango":"stream","id":3,"op":"open","method":"Svc.Flow","args":[]}`)
	if o := c.next(t); o.Op != "error" || o.Error == nil || o.Error.Message != "panic: flow" {
		t.Fatalf("stream reply = %+v", o)
	}
	if p := <-reps; p.where != "stream Svc.Flow" || p.value != "flow" {
		t.Fatalf("stream report = %+v", p)
	}
	select {
	case p := <-reps:
		t.Fatalf("reported twice: %+v", p)
	case <-time.After(50 * time.Millisecond):
	}
}
