package tango

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/ipc"
)

// panicApp runs an app whose OnPanic sends every panic to the returned
// channel and answers keep.
func panicApp(t *testing.T, keep bool) (*App, *fake.Backend, <-chan Panic) {
	t.Helper()
	ch := make(chan Panic, 16)
	fb := fake.New()
	a := newWithBackend(Options{Name: "Test", OnPanic: func(p Panic) bool { ch <- p; return keep }}, fb)
	runApp(t, a)
	return a, fb, ch
}

// noExit replaces exitProcess for the test, recording exit codes.
func noExit(t *testing.T) *atomic.Int32 {
	t.Helper()
	var code atomic.Int32
	code.Store(-1)
	old := exitProcess
	exitProcess = func(c int) { code.Store(int32(c)) }
	t.Cleanup(func() { exitProcess = old })
	return &code
}

func nextPanic(t *testing.T, ch <-chan Panic) Panic {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(timeout):
		t.Fatal("no panic reported")
		return Panic{}
	}
}

func noMorePanics(t *testing.T, ch <-chan Panic) {
	t.Helper()
	select {
	case p := <-ch:
		t.Fatalf("unexpected extra panic: %+v", p)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPanicMainQueueContinues(t *testing.T) {
	code := noExit(t)
	a, _, ch := panicApp(t, true)
	ran := make(chan struct{})
	a.Post(func() { panic("boom") })
	a.Post(func() { close(ran) })
	select {
	case <-ran:
	case <-time.After(timeout):
		t.Fatal("queue stopped after a panic")
	}
	p := nextPanic(t, ch)
	if p.Where != whereMain || p.Value != "boom" || !strings.Contains(string(p.Stack), "panic_test.go") {
		t.Fatalf("panic = %q %v, stack:\n%s", p.Where, p.Value, p.Stack)
	}
	// Await from another goroutine: zero value, and the app keeps going.
	if v := Await(a, func() int { panic("await") }); v != 0 {
		t.Fatalf("Await = %d", v)
	}
	if p := nextPanic(t, ch); p.Where != whereMain || p.Value != "await" {
		t.Fatalf("Await panic = %+v", p)
	}
	_, err := TryAwaitAsync(a, func(done func(int)) { panic("async") })
	if !errors.Is(err, ErrPanicked) {
		t.Fatalf("TryAwaitAsync err = %v", err)
	}
	if p := nextPanic(t, ch); p.Value != "async" {
		t.Fatalf("AwaitAsync panic = %+v", p)
	}
	if Await(a, func() int { return 7 }) != 7 {
		t.Fatal("Await after panics")
	}
	noMorePanics(t, ch)
	if c := code.Load(); c != -1 {
		t.Fatalf("exited with %d", c)
	}
}

func TestPanicEventHandlers(t *testing.T) {
	noExit(t)
	a, fb, ch := panicApp(t, true)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second := make(chan struct{}, 1)
	s.OnLoaded(func() { panic("loaded") })
	s.OnLoaded(func() { second <- struct{}{} })
	fb.Load(s.ID())
	if p := nextPanic(t, ch); p.Where != whereEvent || p.Value != "loaded" {
		t.Fatalf("panic = %+v", p)
	}
	select {
	case <-second:
	case <-time.After(timeout):
		t.Fatal("handler after the panicking one did not run")
	}

	a.OnEvent(func(AppEvent) { panic("event") })
	fb.AppEvent(platform.AppEvent{Kind: platform.AppThemeChanged})
	if p := nextPanic(t, ch); p.Where != whereEvent || p.Value != "event" {
		t.Fatalf("app event panic = %+v", p)
	}

	// A panicking OnClose handler does not keep the window open.
	s.OnClose(func() bool { panic("close") })
	fb.RequestClose(s.ID())
	if p := nextPanic(t, ch); p.Value != "close" {
		t.Fatalf("OnClose panic = %+v", p)
	}
	waitUntil(t, "surface closed", func() bool { return fb.Surface(s.ID()).Closed() })

	a.OnReady(func() { panic("ready") })
	if p := nextPanic(t, ch); p.Where != whereReady {
		t.Fatalf("OnReady panic = %+v", p)
	}
}

func TestPanicOnPanicDecides(t *testing.T) {
	t.Run("keep", func(t *testing.T) {
		code := noExit(t)
		a, _, ch := panicApp(t, true)
		a.Post(func() { panic("x") })
		nextPanic(t, ch)
		if Await(a, func() int { return 1 }) != 1 || code.Load() != -1 {
			t.Fatalf("app stopped, exit code %d", code.Load())
		}
	})
	t.Run("exit", func(t *testing.T) {
		code := noExit(t)
		a, _, ch := panicApp(t, false)
		a.Post(func() { panic("x") })
		nextPanic(t, ch)
		waitUntil(t, "exit", func() bool { return code.Load() == panicExitCode })
	})
}

func TestPanicGo(t *testing.T) {
	code := noExit(t)
	a, _, ch := panicApp(t, false)
	a.Go(func() { panic("bg") })
	if p := nextPanic(t, ch); p.Where != whereGo || p.Value != "bg" {
		t.Fatalf("panic = %+v", p)
	}
	waitUntil(t, "exit", func() bool { return code.Load() == panicExitCode })
}

func TestPanicReleaseDefault(t *testing.T) {
	code := noExit(t)
	old := releaseBuild
	releaseBuild = true
	t.Cleanup(func() { releaseBuild = old })
	shown := make(chan platform.MessageOptions, 1)
	fb := fake.New()
	fb.OnMessageDialog = func(_ platform.SurfaceID, o platform.MessageOptions) (int, error) {
		shown <- o
		return 0, nil
	}
	a := newWithBackend(Options{Name: "Demo"}, fb)
	runApp(t, a)
	a.Post(func() { panic("bad state") })
	select {
	case o := <-shown:
		if o.Kind != platform.MessageError || !strings.Contains(o.Message, "Demo") || !strings.Contains(o.Message, "bad state") {
			t.Fatalf("dialog = %+v", o)
		}
	case <-time.After(timeout):
		t.Fatal("no error dialog")
	}
	waitUntil(t, "exit", func() bool { return code.Load() == panicExitCode })
}

func TestPanicBindingReported(t *testing.T) {
	code := noExit(t)
	a, fb, ch := panicApp(t, false) // keepRunning false is ignored for bindings
	r := ipc.NewRouter()
	r.Handle("Svc.Boom", func(context.Context, json.RawMessage) (any, error) { panic("binding boom") })
	a.OnMessage(func(s *Surface, m string) { r.HandleMessage(s, m) })
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fb.Load(s.ID())
	fb.Message(s.ID(), `{"tango":"call","id":1,"method":"Svc.Boom","args":[]}`)
	p := nextPanic(t, ch)
	if p.Where != "binding Svc.Boom" || p.Value != "binding boom" || len(p.Stack) == 0 {
		t.Fatalf("panic = %q %v", p.Where, p.Value)
	}
	waitUntil(t, "error reply", func() bool {
		for _, e := range fb.Surface(s.ID()).Evals() {
			if strings.Contains(e, "panic: binding boom") && strings.Contains(e, "internal") {
				return true
			}
		}
		return false
	})
	noMorePanics(t, ch)
	if c := code.Load(); c != -1 {
		t.Fatalf("binding panic exited with %d", c)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}
