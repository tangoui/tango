package tango

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform/fake"
)

func init() {
	// The test binary's own arguments are not launch arguments.
	osArgs = func() []string { return nil }
}

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		t.Fatal("timed out")
		panic("unreachable")
	}
}

func TestParseArgs(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	for _, f := range []string{f1, f2} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := parseArgs([]string{
		"-flag", "--x=1", "MyApp://open?id=1", "other://nope", "a.txt", "missing.txt",
		"file://" + filepath.ToSlash(f2), "--", "-dash", "myapp:x",
	}, dir, []string{"myapp"})
	want := []AppEvent{
		{Kind: EventOpenURL, URL: "MyApp://open?id=1"},
		{Kind: EventOpenURL, URL: "myapp:x"},
		{Kind: EventOpenFiles, Paths: []string{f1, f2}},
	}
	if runtime.GOOS == "windows" {
		want[2].Paths = []string{f1} // file:// + C:/... is not a valid file URL form here
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].URL != want[i].URL || !slices.Equal(got[i].Paths, want[i].Paths) {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if evs := parseArgs([]string{"c:/x", "myapp://y"}, dir, nil); len(evs) != 0 {
		t.Errorf("no schemes: got %+v", evs)
	}
	if s, ok := urlScheme(`C:\x`); ok {
		t.Errorf("drive letter taken as scheme %q", s)
	}
}

func TestLaunchArgsDelivered(t *testing.T) {
	old := osArgs
	osArgs = func() []string { return []string{"demo://launch"} }
	defer func() { osArgs = old }()
	fb := fake.New()
	a := newWithBackend(Options{Name: "T", URLSchemes: []string{"demo"}}, fb)
	runApp(t, a)
	// Registered after launch: the pending event is replayed.
	got := make(chan string, 1)
	a.OnOpenURL(func(u string) { got <- u })
	if u := recv(t, got); u != "demo://launch" {
		t.Fatalf("got %q", u)
	}
}

func TestPendingEventsByKind(t *testing.T) {
	a, fb := newTestApp(t)
	fb.OpenFiles("/a")
	fb.DeepLink("x://1")
	// A URL listener must not swallow the pending files event.
	urls := make(chan string, 2)
	a.OnOpenURL(func(u string) { urls <- u })
	if u := recv(t, urls); u != "x://1" {
		t.Fatalf("url %q", u)
	}
	files := make(chan []string, 1)
	a.OnOpenFiles(func(p []string) { files <- p })
	if p := recv(t, files); !slices.Equal(p, []string{"/a"}) {
		t.Fatalf("files %v", p)
	}
	// Live events reach every matching listener.
	all := make(chan AppEvent, 4)
	a.OnEvent(func(ev AppEvent) { all <- ev })
	fb.Reopen()
	if ev := recv(t, all); ev.Kind != EventReopen {
		t.Fatalf("got %+v", ev)
	}
	fb.DeepLink("x://2")
	if u := recv(t, urls); u != "x://2" {
		t.Fatalf("url %q", u)
	}
	if ev := recv(t, all); ev.URL != "x://2" {
		t.Fatalf("got %+v", ev)
	}
}

func TestHandleArgs(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{Name: "T", URLSchemes: []string{"demo"}}, fb)
	runApp(t, a)
	got := make(chan string, 1)
	a.OnOpenURL(func(u string) {
		if !a.isMain() {
			t.Error("listener off the main thread")
		}
		got <- u
	})
	go a.HandleArgs([]string{"--verbose", "demo://second"}, "")
	if u := recv(t, got); u != "demo://second" {
		t.Fatalf("got %q", u)
	}
}

func TestQuitRequest(t *testing.T) {
	a, fb := newTestApp(t)
	asked := 0
	a.OnQuitRequest(func() bool { asked++; return asked > 1 })
	if fb.RequestQuit() {
		t.Fatal("first quit request should be refused")
	}
	if fb.Quits() != 0 {
		t.Fatal("backend quit after refusal")
	}
	if !fb.RequestQuit() {
		t.Fatal("second quit request should be allowed")
	}
	select {
	case <-fb.Done():
	case <-time.After(timeout):
		t.Fatal("app did not quit")
	}
	if asked != 2 {
		t.Fatalf("asked %d times", asked)
	}
}

func TestQuitFromCodeNotIntercepted(t *testing.T) {
	a, fb := newTestApp(t)
	a.OnQuitRequest(func() bool { t.Error("App.Quit asked OnQuitRequest"); return false })
	a.Quit()
	select {
	case <-fb.Done():
	case <-time.After(timeout):
		t.Fatal("app did not quit")
	}
}

func TestQuitOnLastClose(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		opt  *bool
		quit bool
	}{
		{"on", &yes, true},
		{"off", &no, false},
		{"default", nil, runtime.GOOS != "darwin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fb := fake.New()
			a := newWithBackend(Options{Name: "T", QuitOnLastClose: tc.opt}, fb)
			runApp(t, a)
			s1, err := a.NewSurface(SurfaceOptions{})
			if err != nil {
				t.Fatal(err)
			}
			s2, _ := a.NewSurface(SurfaceOptions{})
			fb.RequestClose(s1.ID())
			if fb.Quits() != 0 {
				t.Fatal("quit with a surface left")
			}
			fb.RequestClose(s2.ID())
			if got := fb.Quits() > 0; got != tc.quit {
				t.Fatalf("quit = %v, want %v", got, tc.quit)
			}
		})
	}
}

// An Auxiliary surface (an update window, a palette) does not keep the
// app running: closing the last ordinary surface quits it. Nor does
// closing one ever quit the app, even when no ordinary surface is open
// (a background app showing only an update prompt).
func TestQuitOnLastCloseIgnoresAuxiliary(t *testing.T) {
	yes := true
	fb := fake.New()
	a := newWithBackend(Options{Name: "T", QuitOnLastClose: &yes}, fb)
	runApp(t, a)
	newSurface := func(o SurfaceOptions) *Surface {
		t.Helper()
		s, err := a.NewSurface(o)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	lone := newSurface(SurfaceOptions{Auxiliary: true})
	fb.RequestClose(lone.ID())
	if fb.Quits() != 0 {
		t.Fatal("closing the only, auxiliary, surface quit the app")
	}
	main := newSurface(SurfaceOptions{})
	aux1 := newSurface(SurfaceOptions{Auxiliary: true})
	newSurface(SurfaceOptions{Auxiliary: true})
	fb.RequestClose(aux1.ID())
	if fb.Quits() != 0 {
		t.Fatal("closing an auxiliary surface quit with the main one open")
	}
	fb.RequestClose(main.ID())
	if fb.Quits() == 0 {
		t.Fatal("an auxiliary surface kept the app running")
	}
}

// A framework listener (jsapi) registering before the app's own OnOpenURL
// must not swallow the launch deep link: every listener registered during
// launch sees it once, later listeners don't get it again.
func TestLaunchEventReachesAllLaunchListeners(t *testing.T) {
	old := osArgs
	osArgs = func() []string { return []string{"demo://launch"} }
	defer func() { osArgs = old }()
	fb := fake.New()
	a := newWithBackend(Options{Name: "T", URLSchemes: []string{"demo"}}, fb)
	first := make(chan AppEvent, 2)
	second := make(chan string, 2)
	a.OnEvent(func(ev AppEvent) { first <- ev }) // registered before Run
	a.OnReady(func() {
		a.OnOpenURL(func(u string) { second <- u }) // app code in OnReady
	})
	runApp(t, a)
	if ev := recv(t, first); ev.URL != "demo://launch" {
		t.Fatalf("first listener got %+v", ev)
	}
	if u := recv(t, second); u != "demo://launch" {
		t.Fatalf("second listener got %q", u)
	}

	// After launch has settled, a new listener does not get it replayed.
	late := make(chan string, 1)
	Await(a, func() struct{} {
		a.OnOpenURL(func(u string) { late <- u })
		return struct{}{}
	})
	Await(a, func() struct{} { return struct{}{} }) // flush posted replays
	select {
	case u := <-late:
		t.Fatalf("stale launch URL replayed to late listener: %q", u)
	default:
	}
	select {
	case ev := <-first:
		t.Fatalf("first listener got the launch event twice: %+v", ev)
	default:
	}
}
