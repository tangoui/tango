package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ipc"
)

type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	r.log = append(r.log, s)
	r.mu.Unlock()
}

func (r *recorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.log)
}

type svc struct {
	name     string
	rec      *recorder
	startErr error
	ctx      context.Context
}

func (s *svc) Startup(ctx context.Context) error {
	s.ctx = ctx
	s.rec.add("start " + s.name)
	return s.startErr
}

func (s *svc) Shutdown() error {
	if s.ctx != nil && s.ctx.Err() == nil {
		s.rec.add("ctx live at shutdown " + s.name)
	}
	s.rec.add("stop " + s.name)
	return nil
}

func (s *svc) Hello(name string) string { return "hi " + name }

type startOnly struct{ rec *recorder }

func (s startOnly) Startup(context.Context) error { s.rec.add("start only"); return nil }

func quitAndWait(t *testing.T, a *tango.App, done <-chan struct{}) {
	t.Helper()
	a.Quit()
	select {
	case <-done:
	case <-time.After(faketest.Timeout):
		t.Fatal("app did not quit")
	}
}

func TestOrder(t *testing.T) {
	rec := &recorder{}
	a, fb := faketest.NewApp(tango.Options{})
	Register(a, &svc{name: "a", rec: rec}, nil, startOnly{rec}, struct{}{})
	Register(a, &svc{name: "b", rec: rec})
	faketest.Start(t, a, fb)

	// Registered after the app is ready: starts soon.
	late := make(chan struct{})
	Register(a, &svc{name: "c", rec: rec})
	a.OnReady(func() { close(late) })
	<-late

	quitAndWait(t, a, fb.Done())
	want := []string{"start a", "start only", "start b", "start c", "stop c", "stop b", "stop a"}
	if got := rec.get(); !slices.Equal(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if err := Err(a); err != nil {
		t.Fatal(err)
	}
	if err := Shutdown(a); err != nil { // idempotent
		t.Fatal(err)
	}
	if got := rec.get(); len(got) != len(want) {
		t.Fatalf("shut down twice: %q", got)
	}
}

func TestStartupFailure(t *testing.T) {
	rec := &recorder{}
	boom := errors.New("boom")
	a, fb := faketest.NewApp(tango.Options{})
	Register(a, &svc{name: "a", rec: rec}, &svc{name: "b", rec: rec, startErr: boom}, &svc{name: "c", rec: rec})
	errc := make(chan error, 1)
	go func() { errc <- a.Run() }()
	select {
	case <-fb.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("app did not quit after a failed Startup")
	}
	<-errc
	want := []string{"start a", "start b", "stop a"}
	if got := rec.get(); !slices.Equal(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if err := Err(a); !errors.Is(err, boom) {
		t.Fatalf("Err = %v", err)
	}
}

type conn struct{ ch chan string }

func (c *conn) Send(m string) { c.ch <- m }

func TestBind(t *testing.T) {
	rec := &recorder{}
	a, fb := faketest.NewApp(tango.Options{})
	r := ipc.NewRouter()
	if err := Bind(a, r, "Svc", &svc{name: "x", rec: rec}); err != nil {
		t.Fatal(err)
	}
	faketest.Start(t, a, fb)

	c := &conn{ch: make(chan string, 4)}
	call := func(method string) (json.RawMessage, *ipc.Error) {
		r.HandleMessage(c, `{"tango":"call","id":1,"method":"`+method+`","args":["bob"]}`)
		var o struct {
			Result json.RawMessage `json:"result"`
			Error  *ipc.Error      `json:"error"`
		}
		select {
		case m := <-c.ch:
			if err := json.Unmarshal([]byte(m), &o); err != nil {
				t.Fatal(err)
			}
		case <-time.After(faketest.Timeout):
			t.Fatal("no reply")
		}
		return o.Result, o.Error
	}
	if res, e := call("Svc.Hello"); e != nil || string(res) != `"hi bob"` {
		t.Fatalf("Hello: %s %v", res, e)
	}
	for _, m := range []string{"Svc.Startup", "Svc.Shutdown"} {
		if _, e := call(m); e == nil || e.Code != ipc.CodeNotFound {
			t.Fatalf("%s: %v", m, e)
		}
	}
	if got := rec.get(); !slices.Equal(got, []string{"start x"}) {
		t.Fatalf("got %q", got)
	}
	if err := Bind(a, r, "", &svc{}); err == nil || !strings.Contains(err.Error(), "bind") {
		t.Fatalf("Bind with empty name: %v", err)
	}
}
