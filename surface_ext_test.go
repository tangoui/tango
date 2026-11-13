package tango

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

func recvCount(evals []string) int {
	n := 0
	for _, e := range evals {
		if strings.HasPrefix(e, "window.__tango._recv(") {
			n++
		}
	}
	return n
}

func sync0(a *App) { Await(a, func() int { return 0 }) }

func TestSurfaceExtrasPassedToCreatorEx(t *testing.T) {
	a, fb := newTestApp(t)
	parent, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.NewSurface(SurfaceOptions{
		Width: 300, Height: 200, X: 10, Y: 20, MaxWidth: 800, MaxHeight: 600,
		Kind: KindPanel, Parent: parent, State: StateMaximized, AcceptFiles: true,
		Appearance: Appearance{TitleBar: TitleBarNone, Transparent: true, NoClose: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ex, ok := fb.Surface(s.ID()).Extras()
	if !ok {
		t.Fatal("NewSurfaceEx not used")
	}
	want := platform.SurfaceExtras{
		Kind: KindPanel, Parent: parent.ID(), Bounds: platform.Rect{X: 10, Y: 20, Width: 300, Height: 200},
		MaxWidth: 800, MaxHeight: 600, State: StateMaximized, AcceptFiles: true,
		Appearance: platform.Appearance{TitleBar: TitleBarNone, Transparent: true, NoClose: true},
	}
	if ex != want {
		t.Fatalf("extras\n%+v\nwant\n%+v", ex, want)
	}
	if !strings.Contains(fb.Surface(s.ID()).Options().BridgeScript, "__tangoFrame") {
		t.Fatal("frame shim missing from the bridge script")
	}
	c, _ := a.NewSurface(SurfaceOptions{})
	if ex, _ := fb.Surface(c.ID()).Extras(); !ex.Centered {
		t.Fatal("X=Y=0 should center")
	}
	if got := s.Appearance(); !got.Transparent || got.TitleBar != TitleBarNone {
		t.Fatalf("Appearance() = %+v", got)
	}
}

func TestModalNeedsParent(t *testing.T) {
	a, _ := newTestApp(t)
	for _, k := range []SurfaceKind{KindModal, KindSheet} {
		if _, err := a.NewSurface(SurfaceOptions{Kind: k}); !errors.Is(err, errNeedParent) {
			t.Fatalf("kind %d: err %v", k, err)
		}
	}
}

func TestSurfaceFallbackWithoutCreatorEx(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb.Minimal())
	runApp(t, a)
	s, err := a.NewSurface(SurfaceOptions{
		Width: 300, Height: 200, X: 5, Y: 6, MaxWidth: 900, State: StateMaximized,
		Appearance: Appearance{AlwaysOnTop: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ns := fb.Surface(s.ID())
	if _, ok := ns.Extras(); ok {
		t.Fatal("Minimal backend got NewSurfaceEx")
	}
	if b := s.Bounds(); b != (Rect{X: 5, Y: 6, Width: 300, Height: 200}) {
		t.Fatalf("bounds %+v", b)
	}
	if _, _, mw, _ := ns.SizeLimits(); mw != 900 {
		t.Fatalf("max width %d", mw)
	}
	if ap, n := ns.CurrentAppearance(); n != 1 || !ap.AlwaysOnTop || ap.NoClose {
		t.Fatalf("appearance %+v (%d)", ap, n)
	}
	if s.State() != StateMaximized {
		t.Fatal("state not applied")
	}
}

func TestSurfaceWindowMethods(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ns := fb.Surface(s.ID())

	if err := s.SetBounds(Rect{X: 1, Y: 2, Width: 640, Height: 480}); err != nil {
		t.Fatal(err)
	}
	if b := s.Bounds(); b != (Rect{X: 1, Y: 2, Width: 640, Height: 480}) {
		t.Fatalf("bounds %+v", b)
	}
	if w, h := s.Size(); w != 640 || h != 480 {
		t.Fatalf("size %dx%d", w, h)
	}
	s.Center()
	s.SetSizeLimits(100, 100, 2000, 1500)
	if a, b, c, d := ns.SizeLimits(); a != 100 || b != 100 || c != 2000 || d != 1500 {
		t.Fatal("limits")
	}

	var states []WindowState
	s.OnStateChanged(func(st WindowState) { states = append(states, st) })
	s.Maximize()
	if s.State() != StateMaximized {
		t.Fatal("not maximized")
	}
	s.Minimize()
	s.Fullscreen()
	s.Restore()
	waitFor(t, a, func() bool { return len(states) == 4 })
	if fmt.Sprint(states) != fmt.Sprint([]WindowState{StateMaximized, StateMinimized, StateFullscreen, StateNormal}) {
		t.Fatalf("states %v", states)
	}

	if err := s.SetZoom(1.5); err != nil || s.Zoom() != 1.5 {
		t.Fatalf("zoom %v %v", err, s.Zoom())
	}
	if s.SetZoom(0) == nil {
		t.Fatal("zoom 0 accepted")
	}
	s.Print()
	s.SetAppearance(Appearance{Material: MaterialMica, NoMinimize: true})
	if ap, _ := ns.CurrentAppearance(); ap.Material != MaterialMica || !ap.NoMinimize || ap.NoMaximize {
		t.Fatalf("appearance %+v", ap)
	}
	if s.Appearance().Material != MaterialMica {
		t.Fatal("Appearance() not updated")
	}
	reloads, prints, centers, _ := ns.WindowCounts()
	if reloads != 0 || prints != 1 || centers != 1 {
		t.Fatal(reloads, prints, centers)
	}

	s.Close()
	if s.Maximize() != nil || s.Bounds() != (Rect{}) || s.Zoom() != 1 {
		t.Fatal("closed surface")
	}
}

// waitFor polls cond on the main thread.
func waitFor(t *testing.T, a *App, cond func() bool) {
	t.Helper()
	for range 1000 {
		if Await(a, cond) {
			return
		}
		sync0(a)
	}
	t.Fatal("condition not met")
}

// bareBackend hides every optional surface capability.
type bareBackend struct{ platform.Backend }
type bareSurface struct{ platform.Surface }

func (b bareBackend) NewSurface(id platform.SurfaceID, o platform.SurfaceOptions) (platform.Surface, error) {
	ns, err := b.Backend.NewSurface(id, o)
	if err != nil {
		return nil, err
	}
	return bareSurface{ns}, nil
}

func TestSurfaceWindowUnsupported(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, bareBackend{fb})
	runApp(t, a)
	s, err := a.NewSurface(SurfaceOptions{Appearance: Appearance{TitleBar: TitleBarNone}, Resizable: true})
	if err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"SetBounds":     s.SetBounds(Rect{Width: 1, Height: 1}),
		"Center":        s.Center(),
		"SetSizeLimits": s.SetSizeLimits(0, 0, 0, 0),
		"Maximize":      s.Maximize(),
		"SetAppearance": s.SetAppearance(Appearance{}),
		"Reload":        s.Reload(),
		"SetZoom":       s.SetZoom(2),
		"Print":         s.Print(),
	} {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if s.Zoom() != 1 || s.State() != StateNormal {
		t.Fatal("getters")
	}
	// Without a Dragger the page's edges are never turned on.
	for _, e := range fb.Surface(s.ID()).Evals() {
		if strings.Contains(e, "__tangoFrame(true)") {
			t.Fatal("edges enabled without Dragger")
		}
	}
}

func TestSurfaceExtEvents(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{AcceptFiles: true})
	id := s.ID()
	var log []string
	add := func(f string, v ...any) { log = append(log, fmt.Sprintf(f, v...)) }
	s.OnResized(func(w, h int) { add("resized %d %d", w, h) })
	s.OnMoved(func(x, y int) { add("moved %d %d", x, y) })
	s.OnStateChanged(func(st WindowState) { add("state %d", st) })
	s.OnFilesDropped(func(p []string, x, y int) { add("files %v %d %d", p, x, y) })
	s.OnNavigating(func(u string) { add("nav %s", u) })
	s.OnLoadFailed(func(u, e string) { add("fail %s %s", u, e) })

	fb.Resize(id, 10, 20)
	fb.Move(id, 30, 40)
	fb.ChangeState(id, StateMinimized)
	fb.DropFiles(id, []string{"/a", "/b"}, 5, 6)
	fb.Navigating(id, "tango://app/x")
	fb.LoadFailed(id, "tango://app/x", "boom")
	want := []string{"resized 10 20", "moved 30 40", "state 1", "files [/a /b] 5 6", "nav tango://app/x", "fail tango://app/x boom"}
	if got := Await(a, func() []string { return slices.Clone(log) }); !slices.Equal(got, want) {
		t.Fatalf("events\n%q\nwant\n%q", got, want)
	}
	s.Close()
	fb.Move(id, 1, 1) // after close: ignored, no panic
}

func TestNavigatingRebuffersSend(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ns := fb.Surface(s.ID())
	fb.Load(s.ID())
	s.Send("one")
	sync0(a)
	if recvCount(ns.Evals()) != 1 {
		t.Fatal("not delivered after load")
	}
	// A link click: the page starts unloading; messages must wait.
	fb.Navigating(s.ID(), "tango://app/next")
	s.Send("two")
	sync0(a)
	if recvCount(ns.Evals()) != 1 {
		t.Fatal("sent into a page that is navigating away")
	}
	fb.LoadURL(s.ID(), "tango://app/next")
	if ev := ns.Evals(); recvCount(ev) != 2 || ev[len(ev)-1] != `window.__tango._recv("two")` {
		t.Fatalf("evals %q", ev)
	}

	// Reload re-buffers immediately, before the backend reports anything.
	s.Reload()
	s.Send("three")
	sync0(a)
	if recvCount(ns.Evals()) != 2 {
		t.Fatal("sent during reload")
	}
	fb.Load(s.ID())
	if recvCount(ns.Evals()) != 3 {
		t.Fatal("not delivered after reload")
	}
}

func TestLoadFailedProbe(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ns := fb.Surface(s.ID())
	fb.Load(s.ID())
	fb.Navigating(s.ID(), "https://down.example/")
	s.Send("kept")
	fb.LoadFailed(s.ID(), "https://down.example/", "offline")
	ev := ns.Evals()
	probe := ev[len(ev)-1]
	if !strings.Contains(probe, `tango-int:frame:alive:2`) || recvCount(ev) != 0 {
		t.Fatalf("evals %q", ev)
	}
	// A stale answer (from before the latest navigation) is ignored.
	fb.Message(s.ID(), "\x00tango-int:frame:alive:1")
	if recvCount(ns.Evals()) != 0 {
		t.Fatal("stale probe answer flushed")
	}
	// The old page is still there and answers: the message goes to it.
	var user []string
	a.OnMessage(func(_ *Surface, m string) { user = append(user, m) })
	fb.Message(s.ID(), "\x00tango-int:frame:alive:2")
	if ev := ns.Evals(); ev[len(ev)-1] != `window.__tango._recv("kept")` {
		t.Fatalf("evals %q", ev)
	}
	if n := Await(a, func() int { return len(user) }); n != 0 {
		t.Fatal("internal message reached OnMessage")
	}

	// No answer (an error page replaced it): kept until the next load.
	fb.Navigating(s.ID(), "https://down.example/")
	s.Send("later")
	fb.LoadFailed(s.ID(), "https://down.example/", "offline")
	sync0(a)
	if recvCount(ns.Evals()) != 1 {
		t.Fatal("delivered without a page")
	}
	fb.Load(s.ID())
	if recvCount(ns.Evals()) != 2 {
		t.Fatal("not delivered on next load")
	}
}

func TestDragMessages(t *testing.T) {
	a, fb := newTestApp(t)
	var user []string
	a.OnMessage(func(_ *Surface, m string) { user = append(user, m) })
	const P = "\x00tango-int:frame:"

	// A normal window: moves and double clicks work, edges do not.
	s, _ := a.NewSurface(SurfaceOptions{Resizable: true})
	ns := fb.Surface(s.ID())
	fb.Message(s.ID(), P+"move")
	fb.Message(s.ID(), P+"resize:n")
	if _, _, _, moves := ns.WindowCounts(); moves != 1 || len(ns.Resizes()) != 0 {
		t.Fatalf("moves %d resizes %v", moves, ns.Resizes())
	}
	fb.Message(s.ID(), P+"toggle-max")
	if ns.CurrentState() != StateMaximized {
		t.Fatal("toggle-max did not maximize")
	}
	fb.Message(s.ID(), P+"toggle-max")
	if ns.CurrentState() != StateNormal {
		t.Fatal("toggle-max did not restore")
	}

	// Frameless and resizable: the page's edges resize.
	f, _ := a.NewSurface(SurfaceOptions{Resizable: true, Appearance: Appearance{TitleBar: TitleBarNone, NoMaximize: true}})
	nf := fb.Surface(f.ID())
	if ev := nf.Evals(); len(ev) != 1 || ev[0] != "window.__tangoFrame&&window.__tangoFrame(true)" {
		t.Fatalf("frame evals %q", ev)
	}
	fb.Load(f.ID())
	if ev := nf.Evals(); len(ev) != 2 || ev[1] != ev[0] {
		t.Fatalf("frame not re-sent on load: %q", ev)
	}
	fb.Message(f.ID(), P+"resize:se")
	fb.Message(f.ID(), P+"resize:bogus")
	fb.Message(f.ID(), P+"toggle-max") // NoMaximize
	if r := nf.Resizes(); !slices.Equal(r, []string{"se"}) {
		t.Fatalf("resizes %v", r)
	}
	if nf.CurrentState() != StateNormal {
		t.Fatal("NoMaximize ignored")
	}
	f.SetAppearance(Appearance{})
	if ev := nf.Evals(); ev[len(ev)-1] != "window.__tangoFrame&&window.__tangoFrame(false)" {
		t.Fatalf("edges not turned off: %q", ev)
	}
	fb.Message(f.ID(), P+"resize:se")
	if len(nf.Resizes()) != 1 {
		t.Fatal("resize after edges were turned off")
	}

	// Untrusted pages cannot drag; nothing reaches OnMessage.
	fb.MessageFrom(s.ID(), "https://evil.example", P+"move")
	if _, _, _, moves := ns.WindowCounts(); moves != 1 {
		t.Fatal("untrusted move")
	}
	if n := Await(a, func() int { return len(user) }); n != 0 {
		t.Fatalf("OnMessage got %q", user)
	}
}
