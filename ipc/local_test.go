package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestLocalCall(t *testing.T) {
	r := NewRouter()
	connOK := make(chan bool, 1)
	r.Handle("Greeter.Hello", func(ctx context.Context, args json.RawMessage) (any, error) {
		_, ok := ConnFrom(ctx)
		connOK <- ok
		var a []string
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		return "Hello " + a[0], nil
	})
	r.Handle("E.Coded", func(context.Context, json.RawMessage) (any, error) { return nil, NewError("x", "y") })
	r.Handle("E.Plain", func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("z") })
	ctx := context.Background()

	res, err := r.Call(ctx, "Greeter.Hello", "world")
	if err != nil || string(res) != `"Hello world"` {
		t.Fatalf("Call = %s, %v", res, err)
	}
	if ok := <-connOK; ok {
		t.Fatal("ConnFrom ok == true in an in-process call")
	}
	_, err = r.Call(ctx, "Nope")
	wantCode(t, err, CodeNotFound, "method not found: Nope")
	_, err = r.Call(ctx, "E.Coded")
	wantCode(t, err, "x", "y")
	_, err = r.Call(ctx, "E.Plain")
	wantCode(t, err, CodeInternal, "z")
}

func TestLocalCallCtxAndPanic(t *testing.T) {
	r := NewRouter()
	type rep struct {
		c     Conn
		where string
	}
	reps := make(chan rep, 2)
	r.OnPanic(func(c Conn, where string, _ any, _ []byte) { reps <- rep{c, where} })
	r.Handle("P.Boom", func(context.Context, json.RawMessage) (any, error) { panic("boom") })
	r.Handle("P.Block", func(ctx context.Context, _ json.RawMessage) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	r.Handle("P.Bad", func(context.Context, json.RawMessage) (any, error) { return func() {}, nil })

	_, err := r.Call(context.Background(), "P.Boom")
	wantCode(t, err, CodeInternal, "panic: boom")
	if p := <-reps; p.c != nil || p.where != "binding P.Boom" {
		t.Fatalf("panic report = %+v", p)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.Call(ctx, "P.Block")
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	wantCode(t, within(t, done, time.Second, "Call after cancel"), CodeCanceled, "")

	_, err = r.Call(context.Background(), "P.Bad")
	wantCode(t, err, CodeInternal, "")
}

func TestLocalOpenNotFound(t *testing.T) {
	r := NewRouter()
	s, err := r.Open(context.Background(), "Nope")
	if s != nil {
		t.Fatal("Open returned a stream for an unknown method")
	}
	wantCode(t, err, CodeNotFound, "stream not found: Nope")
}

func TestLocalOpenStreamFunc(t *testing.T) {
	r := NewRouter()
	connOK := make(chan bool, 1)
	r.HandleStream("Count", func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		_, ok := ConnFrom(ctx)
		connOK <- ok
		for i := 0; i < 3; i++ {
			if err := w.Write(i); err != nil {
				return err
			}
		}
		return nil
	})
	s, err := r.Open(context.Background(), "Count")
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, s.Write("x"), CodeBadArgs, "stream does not accept data")
	wantCode(t, s.WriteBytes([]byte{1}), CodeBadArgs, "stream does not accept data")
	for i := 0; i < 3; i++ {
		var v int
		if err := s.ReceiveJSON(&v); err != nil || v != i {
			t.Fatalf("frame %d: %d, %v", i, v, err)
		}
	}
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("after 3 frames: %v, want io.EOF", err)
	}
	if ok := <-connOK; ok {
		t.Fatal("ConnFrom ok == true in an in-process stream")
	}
	within(t, s.Context().Done(), time.Second, "client stream finished")
}

func TestLocalCtxCancel(t *testing.T) {
	r := NewRouter()
	got := make(chan error, 1)
	started := make(chan struct{})
	r.HandleDuplex("Block", func(s *Stream, _ json.RawMessage) error {
		close(started)
		_, err := s.Receive()
		got <- err
		return err
	})
	ctx, cancel := context.WithCancel(context.Background())
	s, err := r.Open(ctx, "Block")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	time.Sleep(20 * time.Millisecond) // let the handler block in Receive
	cancel()
	wantCode(t, within(t, got, time.Second, "server Receive after cancel"), CodeCanceled, "stream canceled")
	within(t, s.Context().Done(), time.Second, "client stream canceled")
	_, err = s.Receive()
	wantCode(t, err, CodeCanceled, "stream canceled")
}

func TestLocalCtxCanceledBeforeOpen(t *testing.T) {
	r := NewRouter()
	ran := make(chan struct{}, 1)
	r.HandleDuplex("E", func(*Stream, json.RawMessage) error { ran <- struct{}{}; return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := r.Open(ctx, "E")
	if s != nil {
		t.Fatal("Open returned a stream for a canceled ctx")
	}
	wantCode(t, err, CodeCanceled, "")
	select {
	case <-ran:
		t.Fatal("handler ran for a canceled ctx")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestLocalCancelAfterFinishIsNoop(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("Echo", echoDuplex)
	ctx, cancel := context.WithCancel(context.Background())
	s, err := r.Open(ctx, "Echo")
	if err != nil {
		t.Fatal(err)
	}
	s.Write(1)
	s.CloseSend()
	if m, err := s.Receive(); err != nil || string(m.JSON) != "1" {
		t.Fatalf("echo = %+v, %v", m, err)
	}
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("want io.EOF, got %v", err)
	}
	within(t, s.Context().Done(), time.Second, "stream finished")
	cancel()
	if _, err := s.Receive(); err != io.EOF {
		t.Fatalf("after late cancel: want io.EOF, got %v", err)
	}
}

func TestLocalUnaffectedByDrop(t *testing.T) {
	r := NewRouter()
	r.HandleDuplex("Echo", echoDuplex)
	c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	c.next(t)
	openDuplex(t, r, c, 1, "Echo") // a page stream too, so Drop has work
	s, err := r.Open(context.Background(), "Echo")
	if err != nil {
		t.Fatal(err)
	}
	r.Drop(c)
	if err := s.Write("after drop"); err != nil {
		t.Fatal(err)
	}
	if m, err := s.Receive(); err != nil || string(m.JSON) != `"after drop"` {
		t.Fatalf("echo after Drop = %+v, %v", m, err)
	}
	if s.Context().Err() != nil {
		t.Fatal("Drop canceled an in-process stream")
	}
	s.Close()
}

func TestLocalBytesCopied(t *testing.T) {
	r := NewRouter()
	gate := make(chan struct{})
	got := make(chan []byte, 2)
	r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
		<-gate
		p, err := s.ReceiveBytes()
		if err != nil {
			return err
		}
		got <- append([]byte(nil), p...)
		if err := s.WriteBytes(p); err != nil {
			return err
		}
		p[0] = 77 // the server reuses its buffer after WriteBytes
		return nil
	})
	s, err := r.Open(context.Background(), "Sink")
	if err != nil {
		t.Fatal(err)
	}
	buf := []byte{1, 2, 3}
	if err := s.WriteBytes(buf); err != nil {
		t.Fatal(err)
	}
	buf[0] = 99
	close(gate)
	if p := within(t, got, time.Second, "server frame"); !bytes.Equal(p, []byte{1, 2, 3}) {
		t.Fatalf("server got %v, want [1 2 3]", p)
	}
	b, err := s.ReceiveBytes()
	if err != nil || !bytes.Equal(b, []byte{1, 2, 3}) {
		t.Fatalf("client got %v, %v", b, err)
	}
}

func TestLocalPanic(t *testing.T) {
	r := NewRouter()
	reps := make(chan string, 2)
	r.OnPanic(func(c Conn, where string, _ any, _ []byte) {
		if c != nil {
			where += " (non-nil conn)"
		}
		reps <- where
	})
	r.HandleDuplex("Boom", func(*Stream, json.RawMessage) error { panic("boom") })
	s, err := r.Open(context.Background(), "Boom")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Receive()
	wantCode(t, err, CodeInternal, "panic: boom")
	if w := within(t, reps, time.Second, "panic report"); w != "stream Boom" {
		t.Fatalf("report where = %q", w)
	}
}

func TestLocalIDsFromRouterCounter(t *testing.T) {
	r := NewRouter()
	ids := make(chan uint64, 4)
	r.HandleDuplex("Id", func(s *Stream, _ json.RawMessage) error { ids <- s.ID(); return nil })
	for i := 0; i < 2; i++ {
		s, err := r.Open(context.Background(), "Id")
		if err != nil {
			t.Fatal(err)
		}
		if sid := within(t, ids, time.Second, "server id"); sid != s.ID() {
			t.Fatalf("server id %d != client id %d", sid, s.ID())
		}
	}
	// A page connection's ids are its own.
	c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	c.next(t)
	if s, _ := r.OpenStream(c, "x"); s.ID() != 2 {
		t.Fatalf("first Go-opened page stream id = %d, want 2", s.ID())
	}
}

// TestLocalFullDuplexNoDeadlock has both ends write far past a small window
// at the same time while both read: synchronous delivery in both directions
// must not form a lock cycle.
func TestLocalFullDuplexNoDeadlock(t *testing.T) {
	r := NewRouter()
	r.SetStreamWindow(4096)
	const n, size = 300, 1000
	pump := func(s *Stream, tag byte) error {
		var wg sync.WaitGroup
		var werr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := make([]byte, size)
			for i := 0; i < n; i++ {
				p[0], p[1] = tag, byte(i)
				if werr = s.WriteBytes(p); werr != nil {
					return
				}
				if i%7 == 0 {
					if werr = s.Write(i); werr != nil {
						return
					}
				}
			}
			werr = s.CloseSend()
		}()
		got := 0
		for {
			m, err := s.Receive()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if m.Binary {
				if len(m.Bytes) != size || m.Bytes[1] != byte(got) {
					return fmt.Errorf("frame %d: len %d idx %d", got, len(m.Bytes), m.Bytes[1])
				}
				got++
			}
		}
		wg.Wait()
		if werr != nil {
			return werr
		}
		if got != n {
			return fmt.Errorf("got %d byte frames, want %d", got, n)
		}
		return nil
	}
	srv := make(chan error, 1)
	r.HandleDuplex("Both", func(s *Stream, _ json.RawMessage) error {
		err := pump(s, 'S')
		srv <- err
		return err
	})
	s, err := r.Open(context.Background(), "Both")
	if err != nil {
		t.Fatal(err)
	}
	cli := make(chan error, 1)
	go func() { cli <- pump(s, 'C') }()
	if err := within(t, cli, 10*time.Second, "client side"); err != nil {
		t.Fatal("client:", err)
	}
	if err := within(t, srv, 10*time.Second, "server side"); err != nil {
		t.Fatal("server:", err)
	}
	within(t, s.Context().Done(), time.Second, "client stream finished")
}
