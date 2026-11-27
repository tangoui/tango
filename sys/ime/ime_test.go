package ime

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

type fakeInputMethod struct {
	sink                  platform.InputMethodSink
	connection            string
	marked, committed     string
	markedID, committedID uint64
	cursor                int
	err                   error
}

func (f *fakeInputMethod) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	f.connection, f.sink = connection, sink
	return f.err
}

func (f *fakeInputMethod) IMESetComposition(id uint64, text string, cursor int) {
	f.markedID, f.marked, f.cursor = id, text, cursor
}

func (f *fakeInputMethod) IMECommit(id uint64, text string) {
	f.committedID, f.committed = id, text
}

func (*fakeInputMethod) IMECaret(uint64) platform.Rect {
	return platform.Rect{X: 8, Y: 30, Width: 1, Height: 16}
}

func (*fakeInputMethod) IMEClientApp(uint64) string { return "com.example.client" }

func runInputMethod(t *testing.T, opts tango.Options) (*tango.App, *fakeInputMethod) {
	t.Helper()
	f := &fakeInputMethod{}
	original := backendOf
	t.Cleanup(func() {
		backendOf = original
		serving.Store(false)
	})
	backendOf = func(*tango.App) (platform.InputMethods, bool) { return f, true }
	a, _ := faketest.Run(t, opts)
	return a, f
}

func passKey(*Session, Key) bool { return false }

func TestServeUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if err := Serve(a, Handler{Key: passKey}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Serve = %v, want ErrUnsupported", err)
	}
}

func TestServeTwice(t *testing.T) {
	a, _ := runInputMethod(t, tango.Options{})
	other, _ := faketest.Run(t, tango.Options{})
	if err := Serve(a, Handler{Key: passKey}); err != nil {
		t.Fatal(err)
	}
	if err := Serve(other, Handler{Key: passKey}); !errors.Is(err, ErrServing) {
		t.Fatalf("Serve = %v, want ErrServing", err)
	}
}

func TestServeNeedsKey(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	if err := Serve(a, Handler{}); err == nil || f.sink != nil {
		t.Fatalf("Serve without Key = %v, backend started = %t", err, f.sink != nil)
	}
}

func TestDefaultConnection(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{ID: "com.example.inputmethod.toy"})
	if err := Serve(a, Handler{Key: passKey}); err != nil {
		t.Fatal(err)
	}
	if want := "com.example.inputmethod.toy_Connection"; f.connection != want {
		t.Errorf("connection = %q, want %q", f.connection, want)
	}
}

func TestServeRetriesFailedStart(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	f.err = errors.New("server unavailable")
	if err := Serve(a, Handler{Key: passKey}); !errors.Is(err, f.err) {
		t.Fatalf("Serve = %v, want %v", err, f.err)
	}
	f.err = nil
	if err := Serve(a, Handler{Connection: "ToyConnection", Key: passKey}); err != nil {
		t.Fatal(err)
	}
	if f.connection != "ToyConnection" {
		t.Errorf("connection = %q", f.connection)
	}
}

func TestSessionRouting(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	var sessions []*Session
	var keys []Key
	err := Serve(a, Handler{Key: func(s *Session, key Key) bool {
		sessions = append(sessions, s)
		keys = append(keys, key)
		s.SetComposition("ni", 2)
		s.Commit("你")
		return f.marked == "ni" && f.committed == "你"
	}})
	if err != nil {
		t.Fatal(err)
	}
	handled := tango.Await(a, func() bool {
		first := f.sink.IMEKey(7, platform.IMEKey{Code: "KeyN", Text: "N", Shift: true, Ctrl: true})
		return first && f.sink.IMEKey(7, platform.IMEKey{})
	})
	if !handled || len(sessions) != 2 || sessions[0].ID() != 7 || sessions[1] != sessions[0] {
		t.Fatalf("handled = %t, sessions = %v", handled, sessions)
	}
	if want := (Key{Code: "KeyN", Text: "N", Shift: true, Ctrl: true}); keys[0] != want {
		t.Errorf("key = %+v, want %+v", keys[0], want)
	}
	if f.markedID != 7 || f.marked != "ni" || f.cursor != 2 || f.committedID != 7 || f.committed != "你" {
		t.Fatalf("backend = %+v", f)
	}
	s := sessions[0]
	s.SetComposition("wo", 2)
	s.Commit("我")
	if got := s.Caret(); got != (tango.Rect{X: 8, Y: 30, Width: 1, Height: 16}) {
		t.Errorf("Caret = %v", got)
	}
	if got := s.ClientApp(); got != "com.example.client" {
		t.Errorf("ClientApp = %q", got)
	}
	if f.marked != "wo" || f.committed != "我" {
		t.Errorf("posted composition and commit = %q and %q", f.marked, f.committed)
	}
}

func TestEndForgetsSession(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	var sessions []*Session
	record := func(s *Session) { sessions = append(sessions, s) }
	if err := Serve(a, Handler{
		Key:      func(s *Session, _ Key) bool { record(s); return true },
		Activate: record, Deactivate: record, CommitComposition: record,
	}); err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool {
		f.sink.IMEActivate(7)
		f.sink.IMECommitComposition(7)
		f.sink.IMEDeactivate(7)
		f.sink.IMEEnd(7)
		f.sink.IMEKey(7, platform.IMEKey{})
		sessions[0].Commit("stale")
		return true
	})
	if len(sessions) != 4 || sessions[0] != sessions[1] || sessions[0] != sessions[2] || sessions[0] == sessions[3] {
		t.Fatalf("session lifecycle = %v", sessions)
	}
	if f.committed != "" {
		t.Fatalf("ended session committed %q", f.committed)
	}
}
