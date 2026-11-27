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

func TestDefaultConnectionIsResolvedByBackend(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{ID: "com.example.inputmethod.toy"})
	if err := Serve(a, Handler{Key: passKey}); err != nil {
		t.Fatal(err)
	}
	if f.connection != "" {
		t.Errorf("connection = %q, want empty so the backend chooses it", f.connection)
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
	var sessions, ended []*Session
	record := func(s *Session) { sessions = append(sessions, s) }
	if err := Serve(a, Handler{
		Key:      func(s *Session, _ Key) bool { record(s); return true },
		Activate: record, Deactivate: record, CommitComposition: record,
		End: func(s *Session) {
			ended = append(ended, s)
			s.Commit("during End")
		},
	}); err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool {
		f.sink.IMEActivate(7)
		f.sink.IMECommitComposition(7)
		f.sink.IMEDeactivate(7)
		f.sink.IMEEnd(7)
		f.sink.IMEEnd(7)
		f.sink.IMEKey(7, platform.IMEKey{})
		sessions[0].Commit("stale")
		return true
	})
	if len(sessions) != 4 || sessions[0] != sessions[1] || sessions[0] != sessions[2] || sessions[0] == sessions[3] {
		t.Fatalf("session lifecycle = %v", sessions)
	}
	if len(ended) != 1 || ended[0] != sessions[0] {
		t.Fatalf("End received %v, want the original session once", ended)
	}
	if f.committed != "" {
		t.Fatalf("ended session committed %q", f.committed)
	}
}

// The backends need valid UTF-8: macOS cannot make an NSString of
// anything else.
func TestSessionReplacesInvalidUTF8(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	err := Serve(a, Handler{Key: func(s *Session, _ Key) bool {
		cut := "你好"[:4] // a rune cut in half
		s.SetComposition(cut, 2)
		s.Commit(cut)
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool { return f.sink.IMEKey(1, platform.IMEKey{}) })
	if want := "你\uFFFD"; f.marked != want || f.committed != want {
		t.Fatalf("marked %q, committed %q; want %q", f.marked, f.committed, want)
	}
}

func TestEndWithoutActivateHandler(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	var ended *Session
	if err := Serve(a, Handler{Key: passKey, End: func(s *Session) { ended = s }}); err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool {
		f.sink.IMEActivate(9)
		f.sink.IMEEnd(9)
		return true
	})
	if ended == nil || ended.ID() != 9 {
		t.Fatalf("End got %v, want session 9", ended)
	}
}

func TestModifierRouting(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	want := ModifierKey{Code: "ShiftRight", Down: true, CapsLock: true}
	var got ModifierKey
	var session uint64
	if err := Serve(a, Handler{Key: passKey, Modifier: func(s *Session, m ModifierKey) bool {
		got, session = m, s.ID()
		return true
	}}); err != nil {
		t.Fatal(err)
	}
	handled := tango.Await(a, func() bool {
		sink := f.sink.(platform.InputMethodModifiers)
		handled := sink.IMEModifier(7, platform.IMEModifierKey(want))
		f.sink.(*server).h.Modifier = nil
		return handled && !sink.IMEModifier(7, platform.IMEModifierKey(want))
	})
	if !handled || got != want || session != 7 {
		t.Fatalf("modifier = %+v, session = %d, routing passed = %t", got, session, handled)
	}
}

func TestKeyNewFields(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	var got Key
	if err := Serve(a, Handler{Key: func(_ *Session, k Key) bool {
		got = k
		return false
	}}); err != nil {
		t.Fatal(err)
	}
	tango.Await(a, func() bool {
		return f.sink.IMEKey(1, platform.IMEKey{CapsLock: true, Repeat: true})
	})
	if !got.CapsLock || !got.Repeat {
		t.Fatalf("key = %+v", got)
	}
}

type fakeInputModes struct {
	*fakeInputMethod
	mode string
	sets int
}

func (f *fakeInputModes) IMECurrentMode() string { return f.mode }

func (f *fakeInputModes) IMESetMode(id string) error {
	if id != "zh" && id != "en" {
		return errors.New("unknown mode")
	}
	f.mode = id
	f.sets++
	return nil
}

func runInputModes(t *testing.T) (*tango.App, *fakeInputModes) {
	t.Helper()
	a, f := runInputMethod(t, tango.Options{})
	modes := &fakeInputModes{fakeInputMethod: f, mode: "zh"}
	backendOf = func(*tango.App) (platform.InputMethods, bool) { return modes, true }
	return a, modes
}

func TestSetModeUnsupported(t *testing.T) {
	a, _ := runInputMethod(t, tango.Options{})
	if err := SetMode(a, "en"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SetMode = %v, want ErrUnsupported", err)
	}
}

func TestSetModeUnknown(t *testing.T) {
	a, _ := runInputModes(t)
	if err := SetMode(a, "unknown"); err == nil || err == ErrUnsupported {
		t.Fatalf("SetMode = %v, want an unknown mode error", err)
	}
}

func TestModeChangedNoLoop(t *testing.T) {
	a, f := runInputModes(t)
	var modes []string
	var setErr error
	if err := Serve(a, Handler{Key: passKey, ModeChanged: func(id string) {
		modes = append(modes, id)
		setErr = SetMode(a, id)
	}}); err != nil {
		t.Fatal(err)
	}
	got := tango.Await(a, func() string {
		f.mode = "en" // The native backend updates its state before notifying.
		f.sink.(platform.InputMethodModeSink).IMEModeChanged("en")
		return CurrentMode(a)
	})
	if got != "en" || len(modes) != 1 || modes[0] != "en" || setErr != nil || f.sets != 0 {
		t.Fatalf("mode = %q, callbacks = %v, SetMode = %v, backend sets = %d", got, modes, setErr, f.sets)
	}
}

func TestMenu(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	want := MenuItem{ID: "about", Title: "About", Checked: true, Disabled: true}
	var clicked string
	if err := Serve(a, Handler{Key: passKey,
		Menu:        func() []MenuItem { return []MenuItem{want} },
		MenuClicked: func(id string) { clicked = id },
	}); err != nil {
		t.Fatal(err)
	}
	got := tango.Await(a, func() []platform.IMEMenuItem {
		sink := f.sink.(platform.InputMethodMenus)
		sink.IMEMenuClicked("about")
		return sink.IMEMenu()
	})
	if len(got) != 1 || MenuItem(got[0]) != want || clicked != "about" {
		t.Fatalf("menu = %+v, clicked = %q", got, clicked)
	}
}

func TestMenuPanic(t *testing.T) {
	var panics []string
	var menuCalls, clickCalls int
	a, f := runInputMethod(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		panics = append(panics, p.Where)
		return false
	}})
	if err := Serve(a, Handler{Key: passKey,
		Menu: func() []MenuItem {
			menuCalls++
			if menuCalls == 1 {
				panic("menu")
			}
			return []MenuItem{{ID: "about", Title: "About"}}
		},
		MenuClicked: func(string) {
			clickCalls++
			if clickCalls == 1 {
				panic("click")
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	got := tango.Await(a, func() [2][]platform.IMEMenuItem {
		sink := f.sink.(platform.InputMethodMenus)
		sink.IMEMenuClicked("about")
		first, next := sink.IMEMenu(), sink.IMEMenu()
		sink.IMEMenuClicked("about")
		return [2][]platform.IMEMenuItem{first, next}
	})
	if len(got[0]) != 0 || len(got[1]) != 1 || got[1][0].ID != "about" || clickCalls != 2 ||
		len(panics) != 2 || panics[0] != "ime menu clicked" || panics[1] != "ime menu" {
		t.Fatalf("menus = %+v, clicks = %d, panic locations = %v", got, clickCalls, panics)
	}
}

func TestSetCandidatesNoBackend(t *testing.T) {
	a, f := runInputMethod(t, tango.Options{})
	if err := Serve(a, Handler{Key: func(s *Session, _ Key) bool {
		s.SetCandidates([]Candidate{{Text: "你", Comment: "ni"}}, 0)
		return true
	}}); err != nil {
		t.Fatal(err)
	}
	if !tango.Await(a, func() bool { return f.sink.IMEKey(1, platform.IMEKey{}) }) {
		t.Fatal("candidate call interrupted the key callback")
	}
}
