package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeConn records messages sent to the "page".
type fakeConn struct {
	ch chan string
}

func newFake() *fakeConn          { return &fakeConn{ch: make(chan string, 1024)} }
func (f *fakeConn) Send(m string) { f.ch <- m }

type out struct {
	Tango  string          `json:"tango"`
	ID     uint64          `json:"id"`
	Op     string          `json:"op"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
	Name   string          `json:"name"`
	Data   json.RawMessage `json:"data"`
}

func (f *fakeConn) next(t *testing.T) out {
	t.Helper()
	select {
	case m := <-f.ch:
		var o out
		if err := json.Unmarshal([]byte(m), &o); err != nil {
			t.Fatalf("bad message %q: %v", m, err)
		}
		return o
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
	return out{}
}

func (f *fakeConn) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case m := <-f.ch:
		t.Fatalf("unexpected message %s", m)
	case <-time.After(d):
	}
}

func send(t *testing.T, r *Router, c Conn, format string, a ...any) {
	t.Helper()
	if !r.HandleMessage(c, fmt.Sprintf(format, a...)) {
		t.Fatalf("message not claimed by ipc: %s", fmt.Sprintf(format, a...))
	}
}

func TestRawMessagesNotClaimed(t *testing.T) {
	r := NewRouter()
	c := newFake()
	for _, m := range []string{"hello", "", "[1,2]", `{"type":"x"}`, `{"tango"`, `"tango"`} {
		if r.HandleMessage(c, m) {
			t.Errorf("claimed raw message %q", m)
		}
	}
	for _, m := range []string{`{"tango":"call","id":"bad"}`, `{"tango":"nope"}`, ` {"tango":1}`} {
		if !r.HandleMessage(c, m) {
			t.Errorf("did not claim ipc message %q", m)
		}
	}
	c.none(t, 20*time.Millisecond)
}

func TestCallResultAndErrors(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.Handle("Greeter.Hello", func(ctx context.Context, args json.RawMessage) (any, error) {
		var a []string
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, NewError(CodeBadArgs, err.Error())
		}
		if cc, ok := ConnFrom(ctx); !ok || cc != Conn(c) {
			return nil, errors.New("ConnFrom mismatch")
		}
		return "Hello " + a[0], nil
	})
	r.Handle("Nil", func(context.Context, json.RawMessage) (any, error) { return nil, nil })
	r.Handle("Boom", func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("boom") })
	r.Handle("Coded", func(context.Context, json.RawMessage) (any, error) {
		return nil, fmt.Errorf("wrapped: %w", NewError(CodeBadArgs, "nope"))
	})
	r.Handle("Panic", func(context.Context, json.RawMessage) (any, error) { panic("ouch") })
	r.Handle("Args", func(_ context.Context, a json.RawMessage) (any, error) { return a, nil })

	send(t, r, c, `{"tango":"call","id":1,"method":"Greeter.Hello","args":["world"]}`)
	o := c.next(t)
	if o.Tango != "ret" || o.ID != 1 || string(o.Result) != `"Hello world"` || o.Error != nil {
		t.Fatalf("got %+v", o)
	}

	cases := []struct {
		method, code string
	}{{"Boom", CodeInternal}, {"Coded", CodeBadArgs}, {"Panic", CodeInternal}, {"Missing", CodeNotFound}}
	for i, tc := range cases {
		id := i + 10
		send(t, r, c, `{"tango":"call","id":%d,"method":%q}`, id, tc.method)
		o := c.next(t)
		if o.ID != uint64(id) || o.Error == nil || o.Error.Code != tc.code {
			t.Fatalf("%s: got %+v err=%+v", tc.method, o, o.Error)
		}
	}

	send(t, r, c, `{"tango":"call","id":20,"method":"Nil"}`)
	if o := c.next(t); string(o.Result) != "null" {
		t.Fatalf("nil result: %+v", o)
	}
	// Missing args normalize to [].
	send(t, r, c, `{"tango":"call","id":21,"method":"Args"}`)
	if o := c.next(t); string(o.Result) != "[]" {
		t.Fatalf("args: %s", o.Result)
	}
}

func TestCallsRunConcurrently(t *testing.T) {
	r := NewRouter()
	c := newFake()
	release := make(chan struct{})
	r.Handle("Block", func(ctx context.Context, _ json.RawMessage) (any, error) {
		<-release
		return "blocked", nil
	})
	r.Handle("Fast", func(context.Context, json.RawMessage) (any, error) { return "fast", nil })
	send(t, r, c, `{"tango":"call","id":1,"method":"Block"}`)
	send(t, r, c, `{"tango":"call","id":3,"method":"Fast"}`)
	if o := c.next(t); o.ID != 3 {
		t.Fatalf("fast call blocked behind slow one: %+v", o)
	}
	close(release)
	if o := c.next(t); o.ID != 1 {
		t.Fatalf("got %+v", o)
	}
}

func TestDropCancelsCallsAndDiscardsResults(t *testing.T) {
	r := NewRouter()
	c := newFake()
	started := make(chan struct{})
	done := make(chan error, 1)
	r.Handle("Wait", func(ctx context.Context, _ json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		done <- context.Cause(ctx)
		return "late", nil
	})
	send(t, r, c, `{"tango":"call","id":1,"method":"Wait"}`)
	<-started
	r.Drop(c)
	select {
	case err := <-done:
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeCanceled {
			t.Fatalf("cause = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not canceled")
	}
	c.none(t, 50*time.Millisecond)
}

func TestHelloResetsConnection(t *testing.T) {
	r := NewRouter()
	c := newFake()
	started := make(chan struct{})
	r.Handle("Wait", func(ctx context.Context, _ json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, nil
	})
	r.Handle("Ping", func(context.Context, json.RawMessage) (any, error) { return "pong", nil })
	send(t, r, c, `{"tango":"call","id":1,"method":"Wait"}`)
	<-started
	send(t, r, c, `{"tango":"hello"}`)
	send(t, r, c, `{"tango":"call","id":1,"method":"Ping"}`)
	if o := c.next(t); string(o.Result) != `"pong"` {
		t.Fatalf("got %+v", o)
	}
	c.none(t, 50*time.Millisecond)
}

func TestEvents(t *testing.T) {
	r := NewRouter()
	c := newFake()
	var mu sync.Mutex
	var got []string
	off := r.On("progress", func(cc Conn, data json.RawMessage) {
		mu.Lock()
		got = append(got, string(data))
		mu.Unlock()
	})
	send(t, r, c, `{"tango":"event","name":"progress","data":{"pct":42}}`)
	send(t, r, c, `{"tango":"event","name":"progress"}`)
	send(t, r, c, `{"tango":"event","name":"other","data":1}`)
	off()
	send(t, r, c, `{"tango":"event","name":"progress","data":2}`)
	if len(got) != 2 || got[0] != `{"pct":42}` || got[1] != "null" {
		t.Fatalf("got %v", got)
	}

	if err := r.Emit(c, "tick", map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	o := c.next(t)
	if o.Tango != "event" || o.Name != "tick" || string(o.Data) != `{"n":1}` {
		t.Fatalf("got %+v", o)
	}
	if err := r.Emit(c, "bad", make(chan int)); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestStreamBackpressure(t *testing.T) {
	r := NewRouter()
	c := newFake()
	const total = 200
	progress := make(chan int, total)
	r.HandleStream("Count", func(ctx context.Context, args json.RawMessage, w StreamWriter) error {
		for i := 0; i < total; i++ {
			if err := w.Write(i); err != nil {
				return err
			}
			progress <- i
		}
		return nil
	})
	send(t, r, c, `{"tango":"stream","id":3,"op":"open","method":"Count","args":[]}`)

	recv := 0
	readData := func(n int) {
		for i := 0; i < n; i++ {
			o := c.next(t)
			if o.Op != "data" || o.ID != 3 || string(o.Data) != fmt.Sprint(recv) {
				t.Fatalf("at %d got %+v", recv, o)
			}
			recv++
		}
	}
	readData(DefaultWindow)
	c.none(t, 50*time.Millisecond) // window full: producer blocked
	if len(progress) != DefaultWindow {
		t.Fatalf("producer wrote %d, want %d", len(progress), DefaultWindow)
	}
	send(t, r, c, `{"tango":"stream","id":3,"op":"ack","n":16}`)
	readData(16)
	c.none(t, 50*time.Millisecond)
	for recv < total {
		send(t, r, c, `{"tango":"stream","id":3,"op":"ack","n":16}`)
		n := min(16, total-recv)
		readData(n)
	}
	if o := c.next(t); o.Op != "end" || o.ID != 3 {
		t.Fatalf("got %+v", o)
	}
	// Late acks for a finished stream are ignored.
	send(t, r, c, `{"tango":"stream","id":3,"op":"ack","n":16}`)
	c.none(t, 20*time.Millisecond)
}

func TestStreamCancel(t *testing.T) {
	r := NewRouter()
	c := newFake()
	result := make(chan error, 1)
	r.HandleStream("Forever", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		for {
			if err := w.Write("x"); err != nil {
				result <- err
				return err
			}
		}
	})
	send(t, r, c, `{"tango":"stream","id":5,"op":"open","method":"Forever"}`)
	for i := 0; i < DefaultWindow; i++ {
		c.next(t)
	}
	send(t, r, c, `{"tango":"stream","id":5,"op":"cancel"}`)
	select {
	case err := <-result:
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeCanceled {
			t.Fatalf("Write err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked Write not released by cancel")
	}
	c.none(t, 50*time.Millisecond) // no end/error after cancel
}

func TestStreamDropReleasesWriter(t *testing.T) {
	r := NewRouter()
	c := newFake()
	result := make(chan error, 1)
	r.HandleStream("Forever", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		for {
			if err := w.Write(1); err != nil {
				result <- err
				return err
			}
		}
	})
	send(t, r, c, `{"tango":"stream","id":7,"op":"open","method":"Forever"}`)
	for i := 0; i < DefaultWindow; i++ {
		c.next(t)
	}
	r.Drop(c)
	select {
	case <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked Write not released by Drop")
	}
	c.none(t, 50*time.Millisecond)
}

func TestStreamErrorsAndNotFound(t *testing.T) {
	r := NewRouter()
	c := newFake()
	r.HandleStream("Fail", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		w.Write("a")
		return NewError(CodeBadArgs, "bad")
	})
	send(t, r, c, `{"tango":"stream","id":9,"op":"open","method":"Fail","args":[]}`)
	if o := c.next(t); o.Op != "data" {
		t.Fatalf("got %+v", o)
	}
	if o := c.next(t); o.Op != "error" || o.Error == nil || o.Error.Code != CodeBadArgs {
		t.Fatalf("got %+v", o)
	}
	send(t, r, c, `{"tango":"stream","id":11,"op":"open","method":"Nope"}`)
	if o := c.next(t); o.ID != 11 || o.Op != "error" || o.Error.Code != CodeNotFound {
		t.Fatalf("got %+v", o)
	}
}

func TestGoOpenedStream(t *testing.T) {
	r := NewRouter()
	c := newFake()
	s, err := r.OpenStream(c, "news", "sports", 3)
	if err != nil {
		t.Fatal(err)
	}
	o := c.next(t)
	if o.Op != "open" || o.ID != 2 || o.Method != "news" || string(o.Args) != `["sports",3]` {
		t.Fatalf("got %+v", o)
	}
	s2, _ := r.OpenStream(c, "other")
	if o := c.next(t); o.ID != 4 || string(o.Args) != "[]" {
		t.Fatalf("second id/args: %+v", o)
	}

	for i := 0; i < DefaultWindow; i++ {
		if err := s.Write(i); err != nil {
			t.Fatal(err)
		}
		c.next(t)
	}
	blocked := make(chan error, 1)
	go func() { blocked <- s.Write("over") }()
	select {
	case <-blocked:
		t.Fatal("Write did not block at window")
	case <-time.After(50 * time.Millisecond):
	}
	send(t, r, c, `{"tango":"stream","id":2,"op":"ack","n":1}`)
	if err := <-blocked; err != nil {
		t.Fatal(err)
	}
	if o := c.next(t); string(o.Data) != `"over"` {
		t.Fatalf("got %+v", o)
	}
	s.Close()
	if o := c.next(t); o.Op != "end" || o.ID != 2 {
		t.Fatalf("got %+v", o)
	}
	if err := s.Write(1); err == nil {
		t.Fatal("Write after Close succeeded")
	}

	// Consumer refuses s2 (no JS handler).
	send(t, r, c, `{"tango":"stream","id":4,"op":"error","error":{"message":"no handler","code":"not_found"}}`)
	<-s2.Context().Done()
	var e *Error
	if err := s2.Write(1); !errors.As(err, &e) || e.Code != CodeNotFound {
		t.Fatalf("Write err = %v", err)
	}
	c.none(t, 20*time.Millisecond)
}

func TestConcurrentConnsIsolated(t *testing.T) {
	r := NewRouter()
	a, b := newFake(), newFake()
	r.Handle("Echo", func(_ context.Context, args json.RawMessage) (any, error) { return args, nil })
	send(t, r, a, `{"tango":"call","id":1,"method":"Echo","args":["a"]}`)
	send(t, r, b, `{"tango":"call","id":1,"method":"Echo","args":["b"]}`)
	if o := a.next(t); string(o.Result) != `["a"]` {
		t.Fatal(o)
	}
	if o := b.next(t); string(o.Result) != `["b"]` {
		t.Fatal(o)
	}
	r.Drop(a)
	send(t, r, b, `{"tango":"call","id":3,"method":"Echo","args":[1]}`)
	if o := b.next(t); string(o.Result) != `[1]` {
		t.Fatal(o)
	}
}
