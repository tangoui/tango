package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

// parityStream is the client end a parity scenario drives: Router.Open's
// *Stream or the wireClient's "JS" stream.
type parityStream interface {
	Write(v any) error
	WriteBytes(p []byte) error
	Receive() (Message, error)
	CloseSend()
	Close()
	Cancel()
}

// localParity adapts the in-process client.
type localParity struct {
	s      *Stream
	cancel context.CancelFunc
}

func (l localParity) Write(v any) error         { return l.s.Write(v) }
func (l localParity) WriteBytes(p []byte) error { return l.s.WriteBytes(p) }
func (l localParity) Receive() (Message, error) { return l.s.Receive() }
func (l localParity) CloseSend()                { l.s.CloseSend() }
func (l localParity) Close()                    { l.s.Close() }
func (l localParity) Cancel() {
	// Canceling ctx cancels the stream asynchronously (context.AfterFunc);
	// wait for it so the scenario sees the same order as the wire client.
	l.cancel()
	<-l.s.Context().Done()
}

// wireParity adapts the wire ("JS") client.
type wireParity struct{ ws *wireStream }

func (w wireParity) Write(v any) error         { return w.ws.writeJSON(v) }
func (w wireParity) WriteBytes(p []byte) error { return w.ws.writeBytes(p) }
func (w wireParity) Receive() (Message, error) { return w.ws.receive() }
func (w wireParity) CloseSend()                { w.ws.closeSend() }
func (w wireParity) Close()                    { w.ws.close() }
func (w wireParity) Cancel()                   { w.ws.cancel() }

type opener func(t *testing.T, r *Router, method string, args ...any) (parityStream, error)

func openLocal(t *testing.T, r *Router, method string, args ...any) (parityStream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := r.Open(ctx, method, args...)
	if err != nil {
		return nil, err
	}
	return localParity{s, cancel}, nil
}

func openWire(t *testing.T, r *Router, method string, args ...any) (parityStream, error) {
	ws := newWireClient(t, r).open(method, args...)
	if ws.openErr != nil {
		return nil, ws.openErr
	}
	return wireParity{ws}, nil
}

// plog is an observation log, safe from any goroutine.
type plog struct {
	mu    sync.Mutex
	lines []string
}

func (l *plog) add(format string, a ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(format, a...))
	l.mu.Unlock()
}

func (l *plog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// describe renders a Receive result.
func describe(m Message, err error) string {
	if err != nil {
		return describeErr(err)
	}
	if m.Binary {
		return fmt.Sprintf("bytes %v (nil=%v)", m.Bytes, m.Bytes == nil)
	}
	return "json " + string(m.JSON)
}

func describeErr(err error) string {
	var e *Error
	switch {
	case err == nil:
		return "ok"
	case err == io.EOF:
		return "EOF"
	case errors.As(err, &e):
		return fmt.Sprintf("error %s %q", e.Code, e.Message)
	default:
		return fmt.Sprintf("non-ipc error %T %v", err, err)
	}
}

// receiveAll logs every Receive result up to and including the first error.
func receiveAll(l *plog, s parityStream) {
	for {
		m, err := s.Receive()
		l.add("client recv %s", describe(m, err))
		if err != nil {
			return
		}
	}
}

type parityScenario struct {
	name string
	// run registers handlers on a fresh router, drives the client through
	// open and logs what both sides observe.
	run func(t *testing.T, r *Router, open opener, cl, sv *plog)
}

func TestParity(t *testing.T) {
	scenarios := []parityScenario{
		{"echo mixed", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			r.HandleDuplex("Echo", func(s *Stream, args json.RawMessage) error {
				sv.add("server args %s", args)
				err := echoDuplex(s, args)
				sv.add("server done %s", describeErr(err))
				return err
			})
			s, err := open(t, r, "Echo", "a", 1)
			if err != nil {
				t.Fatal(err)
			}
			cl.add("write %s", describeErr(s.Write(map[string]int{"a": 1})))
			cl.add("write %s", describeErr(s.WriteBytes([]byte{1, 2, 3})))
			cl.add("write %s", describeErr(s.WriteBytes([]byte{})))
			cl.add("write %s", describeErr(s.Write("s")))
			s.CloseSend()
			receiveAll(cl, s)
		}},
		{"credit", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			r.SetStreamWindow(4096)
			readTwo, readRest := make(chan struct{}), make(chan struct{})
			r.HandleDuplex("Slow", func(s *Stream, _ json.RawMessage) error {
				<-readTwo
				for i := 0; i < 2; i++ {
					m, err := s.Receive()
					sv.add("server recv len %d %s", len(m.Bytes), describeErr(err))
				}
				<-readRest
				for {
					m, err := s.Receive()
					sv.add("server recv len %d %s", len(m.Bytes), describeErr(err))
					if err != nil {
						return nil
					}
				}
			})
			s, err := open(t, r, "Slow")
			if err != nil {
				t.Fatal(err)
			}
			p := make([]byte, 1000)
			for i := 0; i < 3; i++ {
				cl.add("write %d %s", i, describeErr(s.WriteBytes(p)))
			}
			fourth := make(chan error, 1)
			go func() { fourth <- s.WriteBytes(p) }()
			select {
			case err := <-fourth:
				cl.add("write 3 did not block: %s", describeErr(err))
			case <-time.After(100 * time.Millisecond):
				cl.add("write 3 blocked")
			}
			close(readTwo)
			cl.add("write 3 %s", describeErr(within(t, fourth, time.Second, "fourth write after server read 2")))
			close(readRest)
			s.CloseSend()
			receiveAll(cl, s)
		}},
		{"server close", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			r.HandleDuplex("One", func(s *Stream, _ json.RawMessage) error {
				sv.add("server write %s", describeErr(s.Write("one")))
				return nil
			})
			s, err := open(t, r, "One")
			if err != nil {
				t.Fatal(err)
			}
			receiveAll(cl, s)
			cl.add("write %s", describeErr(s.Write("late")))
		}},
		{"server error", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			r.HandleDuplex("Fail", func(*Stream, json.RawMessage) error { return NewError("x", "boom") })
			s, err := open(t, r, "Fail")
			if err != nil {
				t.Fatal(err)
			}
			receiveAll(cl, s)
		}},
		{"client cancel", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			started, got := make(chan struct{}), make(chan error, 1)
			r.HandleDuplex("Block", func(s *Stream, _ json.RawMessage) error {
				close(started)
				_, err := s.Receive()
				got <- err
				return err
			})
			s, err := open(t, r, "Block")
			if err != nil {
				t.Fatal(err)
			}
			<-started
			time.Sleep(20 * time.Millisecond)
			s.Cancel()
			sv.add("server recv %s", describeErr(within(t, got, time.Second, "server Receive after cancel")))
			receiveAll(cl, s)
			cl.add("write %s", describeErr(s.Write(1)))
		}},
		{"server writes N frames then returns error", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			done := make(chan struct{})
			r.HandleDuplex("WriteFail", func(s *Stream, _ json.RawMessage) error {
				defer close(done)
				for i := 0; i < 3; i++ {
					sv.add("server write %d %s", i, describeErr(s.Write(i)))
				}
				return NewError("x", "boom")
			})
			s, err := open(t, r, "WriteFail")
			if err != nil {
				t.Fatal(err)
			}
			// Let the error arrive before reading: the frames before it must
			// still be delivered.
			within(t, done, time.Second, "server done")
			time.Sleep(20 * time.Millisecond)
			receiveAll(cl, s)
			cl.add("write %s", describeErr(s.Write("late")))
			want := []string{"client recv json 0", "client recv json 1", "client recv json 2",
				`client recv error x "boom"`, `write error x "boom"`}
			if got := cl.get(); !reflect.DeepEqual(got, want) {
				t.Errorf("client saw %q, want %q", got, want)
			}
		}},
		{"stream func", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			r.HandleStream("Count", func(_ context.Context, _ json.RawMessage, w StreamWriter) error {
				for i := 0; i < 3; i++ {
					if err := w.Write(i); err != nil {
						return err
					}
				}
				return nil
			})
			s, err := open(t, r, "Count")
			if err != nil {
				t.Fatal(err)
			}
			cl.add("write %s", describeErr(s.Write(1)))
			receiveAll(cl, s)
		}},
		{"client close", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			got := make(chan []string, 1)
			r.HandleDuplex("Sink", func(s *Stream, _ json.RawMessage) error {
				var seen []string
				for {
					m, err := s.Receive()
					seen = append(seen, describe(m, err))
					if err != nil {
						break
					}
				}
				seen = append(seen, "write "+describeErr(s.Write("late")))
				got <- seen
				return nil
			})
			s, err := open(t, r, "Sink")
			if err != nil {
				t.Fatal(err)
			}
			s.Write(1)
			s.Close()
			for _, line := range within(t, got, time.Second, "server done") {
				sv.add("server %s", line)
			}
			receiveAll(cl, s)
			cl.add("write %s", describeErr(s.Write(2)))
		}},
		{"not found", func(t *testing.T, r *Router, open opener, cl, sv *plog) {
			_, err := open(t, r, "Nope")
			cl.add("open %s", describeErr(err))
		}},
	}
	clients := []struct {
		name string
		open opener
	}{{"wire", openWire}, {"local", openLocal}}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			var cls, svs [2][]string
			for i, c := range clients {
				var cl, sv plog
				t.Run(c.name, func(t *testing.T) { sc.run(t, NewRouter(), c.open, &cl, &sv) })
				cls[i], svs[i] = cl.get(), sv.get()
				t.Logf("%s client: %q", c.name, cls[i])
				t.Logf("%s server: %q", c.name, svs[i])
			}
			if len(cls[0]) == 0 {
				t.Fatal("scenario logged nothing")
			}
			if !reflect.DeepEqual(cls[0], cls[1]) {
				t.Errorf("client observations differ:\n wire:  %q\n local: %q", cls[0], cls[1])
			}
			if !reflect.DeepEqual(svs[0], svs[1]) {
				t.Errorf("server observations differ:\n wire:  %q\n local: %q", svs[0], svs[1])
			}
		})
	}
}
