package tango

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

// Contract round 3: accessors, focus events, the surface-created hook,
// typed event payloads and nested waits.

func TestAccessors(t *testing.T) {
	a, fb := newTestApp(t)
	if a.Name() != "Test" {
		t.Fatal(a.Name())
	}
	s, err := a.NewSurface(SurfaceOptions{Title: "One", Hidden: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.App() != a || s.Title() != "One" || s.IsVisible() || s.IsFocused() {
		t.Fatal("initial state")
	}
	s.SetTitle("Two")
	s.Show()
	if s.Title() != "Two" || !s.IsVisible() {
		t.Fatal("after SetTitle / Show")
	}
	s.OpenDevTools()
	if fb.Surface(s.ID()).DevToolsOpened() != 1 {
		t.Fatal("OpenDevTools not forwarded")
	}

	focus := make(chan bool, 4)
	s.OnFocused(func() { focus <- true })
	s.OnBlurred(func() { focus <- false })
	fb.FocusChange(s.ID(), true)
	if !<-focus || !s.IsFocused() {
		t.Fatal("focused")
	}
	fb.FocusChange(s.ID(), false)
	if <-focus || s.IsFocused() {
		t.Fatal("blurred")
	}
	s.Close()
	if s.Title() != "" || s.IsVisible() {
		t.Fatal("closed surface reports state")
	}
}

func TestOnSurfaceCreated(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	var got []*Surface
	a.OnSurfaceCreated(func(s *Surface) {
		if !a.isMain() {
			t.Error("not on the main thread")
		}
		got = append(got, s)
	})
	viaHook := make(chan any, 1)
	corehook.OnSurfaceCreated(a, func(s any) { viaHook <- s })
	runApp(t, a)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// NewSurface returns after the handlers ran (they run before it).
	if n := Await(a, func() int { return len(got) }); n != 1 || Await(a, func() *Surface { return got[0] }) != s {
		t.Fatalf("handlers saw %d surfaces", n)
	}
	select {
	case v := <-viaHook:
		if v.(*Surface) != s {
			t.Fatal("corehook got another surface")
		}
	case <-time.After(timeout):
		t.Fatal("corehook.OnSurfaceCreated not called")
	}
	fb.NewSurfaceErr = errors.New("boom")
	if _, err := a.NewSurface(SurfaceOptions{}); err == nil {
		t.Fatal("want error")
	}
	if n := Await(a, func() int { return len(got) }); n != 1 {
		t.Fatal("failed creation reported")
	}
}

func TestTryAwaitAsyncNoNestedLoop(t *testing.T) {
	fb := fake.New()
	fb.NoNestedLoop = true
	a := newWithBackend(Options{}, fb)
	runApp(t, a)
	started := false
	err := Await(a, func() error {
		_, err := TryAwaitAsync(a, func(done func(int)) { started = true; done(1) })
		return err
	})
	if !errors.Is(err, ErrNestedWait) || started {
		t.Fatalf("err=%v started=%v", err, started)
	}
	// Off the main thread it waits as usual.
	v, err := TryAwaitAsync(a, func(done func(int)) { done(7) })
	if v != 7 || err != nil {
		t.Fatal(v, err)
	}
}

func TestOnEventKindLeavesOtherKinds(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	runApp(t, a)
	fb.DeepLink("demo://x") // nobody listens yet: kept
	fb.NotificationOpened("n1")
	notes := make(chan string, 1)
	a.OnEventKind(EventNotificationOpened, func(ev AppEvent) { notes <- ev.Notification })
	if got := <-notes; got != "n1" {
		t.Fatal(got)
	}
	urls := make(chan string, 1)
	a.OnOpenURL(func(u string) { urls <- u })
	select {
	case u := <-urls:
		if u != "demo://x" {
			t.Fatal(u)
		}
	case <-time.After(timeout):
		t.Fatal("deep link swallowed by OnEventKind")
	}
}

func TestTypedMobilePayloads(t *testing.T) {
	a, fb := newTestApp(t)
	evs := make(chan AppEvent, 4)
	a.OnEvent(func(ev AppEvent) { evs <- ev })
	fb.SetSafeArea(platform.Insets{Top: 44})
	fb.Rotate(platform.OrientationLandscapeLeft)
	fb.PushToken("tok")
	if ev := <-evs; ev.Kind != EventSafeAreaChanged || ev.Insets.Top != 44 {
		t.Fatalf("%+v", ev)
	}
	if ev := <-evs; ev.Kind != EventOrientationChanged || ev.Orientation != platform.OrientationLandscapeLeft {
		t.Fatalf("%+v", ev)
	}
	if ev := <-evs; ev.Kind != EventPushToken || ev.Push.Token != "tok" {
		t.Fatalf("%+v", ev)
	}
}

func TestParsePushPayload(t *testing.T) {
	p := platform.ParsePushPayload(`{"id":"1","title":"T","data":{"k":"v"}}`)
	if p.ID != "1" || p.Title != "T" || p.Data["k"] != "v" {
		t.Fatalf("%+v", p)
	}
	if p := platform.ParsePushPayload("nope"); p.Data["raw"] != "nope" {
		t.Fatalf("%+v", p)
	}
}

// The zero Appearance is an ordinary window.
func TestZeroAppearance(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ex, _ := fb.Surface(s.ID()).Extras()
	if ap := ex.Appearance; ap.NoClose || ap.NoMinimize || ap.NoMaximize || ap.NoShadow {
		t.Fatalf("%+v", ap)
	}
}
