package tango

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

const timeout = 5 * time.Second

// runApp runs a on its own goroutine (the test's "main thread") and waits
// until it is ready. Cleanup quits it and waits for Run to return.
func runApp(t *testing.T, a *App) <-chan error {
	t.Helper()
	ready := make(chan struct{})
	a.OnReady(func() { close(ready) })
	errc := make(chan error, 1)
	go func() { errc <- a.Run() }()
	select {
	case <-ready:
	case err := <-errc:
		t.Fatalf("Run returned early: %v", err)
	case <-time.After(timeout):
		t.Fatal("app did not start")
	}
	t.Cleanup(func() {
		a.Quit()
		select {
		case <-a.closedCh:
		case <-time.After(timeout):
			t.Error("app did not quit")
		}
	})
	return errc
}

func newTestApp(t *testing.T) (*App, *fake.Backend) {
	t.Helper()
	fb := fake.New()
	a := newWithBackend(Options{Name: "Test"}, fb)
	runApp(t, a)
	return a, fb
}

func waitErr(t *testing.T, errc <-chan error) error {
	t.Helper()
	select {
	case err := <-errc:
		return err
	case <-time.After(timeout):
		t.Fatal("Run did not return")
		return nil
	}
}

func TestPostOrder(t *testing.T) {
	a, _ := newTestApp(t)
	var got []int
	for i := range 100 {
		a.Post(func() { got = append(got, i) })
	}
	n := Await(a, func() int { return len(got) })
	if n != 100 {
		t.Fatalf("ran %d", n)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("order: got[%d] = %d", i, v)
		}
	}
}

func TestPostBeforeRunRunsInOrder(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	var got []string
	a.Post(func() { got = append(got, "a") })
	a.Post(func() { got = append(got, "b") })
	runApp(t, a)
	if s := Await(a, func() string { return strings.Join(got, "") }); s != "ab" {
		t.Fatalf("got %q", s)
	}
}

func TestConcurrentPost(t *testing.T) {
	a, _ := newTestApp(t)
	const G, N = 16, 200
	perG := make([][]int, G) // touched on the main thread only
	var wg sync.WaitGroup
	for g := range G {
		wg.Go(func() {
			for i := range N {
				a.Post(func() { perG[g] = append(perG[g], i) })
			}
		})
	}
	wg.Wait()
	Await(a, func() struct{} { return struct{}{} })
	Await(a, func() struct{} {
		for g, s := range perG {
			if len(s) != N {
				t.Errorf("goroutine %d: %d ran", g, len(s))
			}
			for i, v := range s {
				if v != i {
					t.Errorf("goroutine %d: out of order at %d", g, i)
					break
				}
			}
		}
		return struct{}{}
	})
}

func TestAwaitOffAndOnMain(t *testing.T) {
	a, fb := newTestApp(t)
	onMain := Await(a, func() bool { return a.isMain() && fb.IsMainThread() })
	if !onMain {
		t.Fatal("Await fn not on main thread")
	}
	if a.isMain() {
		t.Fatal("test goroutine reported as main")
	}
	// Nested Await on the main thread runs directly (would deadlock otherwise).
	v := Await(a, func() int {
		return Await(a, func() int { return 41 }) + 1
	})
	if v != 42 {
		t.Fatalf("got %d", v)
	}
}

func TestAwaitConcurrent(t *testing.T) {
	a, _ := newTestApp(t)
	var counter int // main thread only
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 20 {
				Await(a, func() int { counter++; return counter })
			}
		})
	}
	wg.Wait()
	if n := Await(a, func() int { return counter }); n != 1000 {
		t.Fatalf("counter %d", n)
	}
}

func TestAwaitAsyncOffMain(t *testing.T) {
	a, _ := newTestApp(t)
	v := AwaitAsync(a, func(done func(string)) {
		if !a.isMain() {
			t.Error("start not on main thread")
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			done("x")
			done("ignored")
		}()
	})
	if v != "x" {
		t.Fatalf("got %q", v)
	}
}

func TestAwaitAsyncOnMainPumps(t *testing.T) {
	a, fb := newTestApp(t)
	steps := fb.Steps()
	v := Await(a, func() int {
		// done is called from a goroutine that needs the main thread first:
		// only works if AwaitAsync keeps draining while it waits.
		return AwaitAsync(a, func(done func(int)) {
			go func() {
				n := Await(a, func() int { return 7 })
				done(n * 6)
			}()
		})
	})
	if v != 42 {
		t.Fatalf("got %d", v)
	}
	if fb.Steps() == steps {
		t.Fatal("AwaitAsync on main did not Step")
	}
}

func TestAwaitAsyncOnMainNativeEvent(t *testing.T) {
	a, fb := newTestApp(t)
	// The answer arrives as a native event (like a dialog callback).
	fb.OnMessageDialog = nil
	v := Await(a, func() int {
		return AwaitAsync(a, func(done func(int)) {
			fb.MessageDialog(0, platform.MessageOptions{}, func(n int, _ error) { done(n + 5) })
		})
	})
	if v != 5 {
		t.Fatalf("got %d", v)
	}
}

func TestAwaitAsyncSyncDone(t *testing.T) {
	a, fb := newTestApp(t)
	steps := fb.Steps()
	v := Await(a, func() int {
		return AwaitAsync(a, func(done func(int)) { done(3) })
	})
	if v != 3 || fb.Steps() != steps {
		t.Fatalf("got %d, steps %d -> %d", v, steps, fb.Steps())
	}
}

func TestAwaitAsyncOnMainQuit(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	errc := runApp(t, a)
	got := make(chan int, 1)
	a.Post(func() {
		got <- AwaitAsync(a, func(done func(int)) {
			a.Quit() // never calls done
		})
	})
	select {
	case v := <-got:
		if v != 0 {
			t.Fatalf("got %d", v)
		}
	case <-time.After(timeout):
		t.Fatal("AwaitAsync on main did not return after Quit")
	}
	if err := waitErr(t, errc); err != nil {
		t.Fatal(err)
	}
}

func TestQuit(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	var events []Lifecycle
	var mu sync.Mutex
	a.OnLifecycle(func(ev Lifecycle) { mu.Lock(); events = append(events, ev); mu.Unlock() })
	errc := runApp(t, a)
	a.Quit()
	if err := waitErr(t, errc); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if fmt.Sprint(events) != fmt.Sprint([]Lifecycle{Started, Terminating}) {
		t.Fatalf("events %v", events)
	}
	mu.Unlock()

	// Nothing blocks after quit.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ran := false
		a.Post(func() { ran = true })
		if v := Await(a, func() int { ran = true; return 1 }); v != 0 {
			t.Errorf("Await after quit = %d", v)
		}
		if v := AwaitAsync(a, func(d func(int)) { d(1) }); v != 0 {
			t.Errorf("AwaitAsync after quit = %d", v)
		}
		if s, err := a.NewSurface(SurfaceOptions{}); s != nil || !errors.Is(err, ErrNotRunning) {
			t.Errorf("NewSurface after quit = %v, %v", s, err)
		}
		if ran {
			t.Error("fn ran after quit")
		}
		a.Quit()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("calls after quit blocked")
	}
}

func TestQuitReleasesWaiters(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	errc := runApp(t, a)
	block := make(chan struct{})
	a.Post(func() {
		a.Quit()
		<-block
	})
	res := make(chan int, 1)
	go func() { res <- Await(a, func() int { return 1 }) }()
	time.Sleep(20 * time.Millisecond)
	close(block)
	select {
	case <-res: // ran (still inside the last drain) or dropped: either way released
	case <-time.After(timeout):
		t.Fatal("waiter not released")
	}
	waitErr(t, errc)
}

func TestQuitBeforeRun(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	a.Quit()
	errc := make(chan error, 1)
	go func() { errc <- a.Run() }()
	if err := waitErr(t, errc); err != nil {
		t.Fatal(err)
	}
	if fb.Quits() != 1 {
		t.Fatalf("quits %d", fb.Quits())
	}
}

func TestRunErrors(t *testing.T) {
	if err := newWithBackend(Options{}, nil).Run(); !errors.Is(err, ErrNoBackend) {
		t.Fatalf("nil backend: %v", err)
	}
	fb := fake.New()
	fb.InitErr = errors.New("init failed")
	a := newWithBackend(Options{}, fb)
	if err := a.Run(); err != fb.InitErr {
		t.Fatalf("init err: %v", err)
	}
	if v := Await(a, func() int { return 1 }); v != 0 {
		t.Fatal("Await after failed Run ran")
	}
	if err := a.Run(); err == nil {
		t.Fatal("second Run succeeded")
	}
}

func TestOnReadyAfterStart(t *testing.T) {
	a, _ := newTestApp(t)
	ch := make(chan bool, 1)
	a.OnReady(func() { ch <- a.isMain() })
	select {
	case onMain := <-ch:
		if !onMain {
			t.Fatal("OnReady not on main")
		}
	case <-time.After(timeout):
		t.Fatal("late OnReady not called")
	}
}

func TestLifecycleEvents(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb)
	ch := make(chan Lifecycle, 8)
	a.OnLifecycle(func(ev Lifecycle) { ch <- ev })
	runApp(t, a)
	fb.Lifecycle(Background)
	fb.Lifecycle(Foreground)
	for _, want := range []Lifecycle{Started, Background, Foreground} {
		if got := <-ch; got != want {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestMainGoroutineBeforeRun(t *testing.T) {
	// Simulate being the process' main goroutine.
	old := mainGoroutine
	mainGoroutine = goroutineID()
	defer func() { mainGoroutine = old }()
	a := newWithBackend(Options{}, fake.New())
	if _, err := a.NewSurface(SurfaceOptions{}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("NewSurface: %v", err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Await did not panic")
			}
		}()
		Await(a, func() int { return 1 })
	}()
}

func TestSurfaceLifecycle(t *testing.T) {
	a, fb := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{Title: "Hi", Width: 300, Height: 200, URL: "/index.html", Resizable: true, Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID() == 0 {
		t.Fatal("zero id")
	}
	ns := fb.Surface(s.ID())
	o := ns.Options()
	if o.URL != "tango://app/index.html" || !o.Visible || o.Title != "Hi" || o.Width != 300 || !o.Resizable || !o.Debug {
		t.Fatalf("options %+v", o)
	}
	if !strings.HasPrefix(o.BridgeScript, coreShim) {
		t.Fatal("bridge script lacks core shim")
	}
	s.SetTitle("New")
	s.SetSize(640, 480)
	if w, h := s.Size(); w != 640 || h != 480 || ns.Title() != "New" {
		t.Fatalf("size %dx%d title %q", w, h, ns.Title())
	}
	s.Hide()
	if ns.Visible() {
		t.Fatal("visible after Hide")
	}
	s.Show()
	s.Focus()
	if !ns.Visible() || ns.Focuses() != 1 {
		t.Fatal("Show/Focus")
	}
	if s.Native() != ns.Native() {
		t.Fatal("Native")
	}
	if ss := a.Surfaces(); len(ss) != 1 || ss[0] != s {
		t.Fatalf("Surfaces %v", ss)
	}

	closed := make(chan bool, 1)
	s.OnClosed(func() { closed <- a.isMain() })
	s.Close()
	if !<-closed {
		t.Fatal("OnClosed off main")
	}
	if !ns.Closed() || len(a.Surfaces()) != 0 {
		t.Fatal("not closed")
	}
	// Methods on a closed surface do nothing and do not block.
	s.SetTitle("x")
	s.Send("x")
	s.Navigate("/")
	if w, h := s.Size(); w != 0 || h != 0 || s.Native() != 0 {
		t.Fatal("closed surface answered")
	}
}

func TestSurfaceHiddenAndIDs(t *testing.T) {
	a, fb := newTestApp(t)
	s1, _ := a.NewSurface(SurfaceOptions{Hidden: true})
	s2, _ := a.NewSurface(SurfaceOptions{URL: "http://localhost:5173/x"})
	if fb.Surface(s1.ID()).Options().Visible {
		t.Fatal("hidden surface visible")
	}
	if s1.ID() == s2.ID() {
		t.Fatal("ids repeat")
	}
	if u := fb.Surface(s1.ID()).Options().URL; u != "tango://app/" {
		t.Fatalf("empty URL resolved to %q", u)
	}
	if u := fb.Surface(s2.ID()).Options().URL; u != "http://localhost:5173/x" {
		t.Fatalf("absolute URL changed to %q", u)
	}
}

func TestNewSurfaceError(t *testing.T) {
	a, fb := newTestApp(t)
	fb.NewSurfaceErr = errors.New("no webview")
	s, err := a.NewSurface(SurfaceOptions{})
	if s != nil || err != fb.NewSurfaceErr {
		t.Fatalf("%v %v", s, err)
	}
	if len(a.Surfaces()) != 0 {
		t.Fatal("failed surface registered")
	}
}

func TestResolveURL(t *testing.T) {
	a := newWithBackend(Options{}, fake.New())
	a.origin = "https://tango.localhost"
	for in, want := range map[string]string{
		"":                    "https://tango.localhost/",
		"/a/b.html?x=1":       "https://tango.localhost/a/b.html?x=1",
		"page.html":           "https://tango.localhost/page.html",
		"https://example.com": "https://example.com",
		"about:blank":         "about:blank",
	} {
		if got := a.resolveURL(in); got != want {
			t.Errorf("resolveURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSendBuffersUntilLoaded(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{URL: "/"})
	ns := fb.Surface(s.ID())
	s.Send("one")
	s.Send("two\u2028\u2029</script>")
	Await(a, func() int { return 0 })
	if len(ns.Evals()) != 0 {
		t.Fatalf("sent before Loaded: %v", ns.Evals())
	}
	var order []string
	s.OnLoaded(func() { order = append(order, fmt.Sprint(len(ns.Evals()))) })
	fb.Load(s.ID())
	ev := ns.Evals()
	want := []string{
		`window.__tango._recv("one")`,
		`window.__tango._recv("two\u2028\u2029\u003c/script\u003e")`,
	}
	if fmt.Sprint(ev) != fmt.Sprint(want) {
		t.Fatalf("evals\n%q\nwant\n%q", ev, want)
	}
	if Await(a, func() string { return strings.Join(order, ",") }) != "2" {
		t.Fatal("OnLoaded ran before flush")
	}
	s.Send("three")
	Await(a, func() int { return 0 })
	if ev := ns.Evals(); len(ev) != 3 || ev[2] != `window.__tango._recv("three")` {
		t.Fatalf("after load: %q", ev)
	}

	// Navigate: buffer again until the new page loads.
	s.Navigate("/other.html")
	s.Send("four")
	Await(a, func() int { return 0 })
	if len(ns.Evals()) != 3 {
		t.Fatal("sent during navigation")
	}
	if ns.URL() != "tango://app/other.html" {
		t.Fatalf("navigated to %q", ns.URL())
	}
	fb.Load(s.ID())
	if ev := ns.Evals(); len(ev) != 4 || ev[3] != `window.__tango._recv("four")` {
		t.Fatalf("after reload: %q", ev)
	}
}

func TestSendConcurrentOrderAndEval(t *testing.T) {
	a, fb := newTestApp(t)
	fb.AutoLoad = false
	s, _ := a.NewSurface(SurfaceOptions{})
	ns := fb.Surface(s.ID())
	fb.Load(s.ID())
	for i := range 50 {
		if i%10 == 0 {
			s.Eval(fmt.Sprintf("e%d", i))
		}
		s.Send(fmt.Sprint(i))
	}
	Await(a, func() int { return 0 })
	ev := ns.Evals()
	if len(ev) != 55 {
		t.Fatalf("%d evals", len(ev))
	}
	n := 0
	for _, e := range ev {
		if strings.HasPrefix(e, "e") {
			continue
		}
		if e != fmt.Sprintf(`window.__tango._recv("%d")`, n) {
			t.Fatalf("out of order: %q at %d", e, n)
		}
		n++
	}
}

func TestSendToUntrustedPageWaits(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ns := fb.Surface(s.ID())
	fb.LoadURL(s.ID(), "https://evil.example/")
	s.Send("secret")
	Await(a, func() int { return 0 })
	if len(ns.Evals()) != 0 {
		t.Fatal("message sent to an untrusted page")
	}
	fb.LoadURL(s.ID(), "tango://app/")
	if len(ns.Evals()) != 1 {
		t.Fatal("message not delivered after returning")
	}
}

func TestAutoLoad(t *testing.T) {
	fb := fake.New()
	fb.AutoLoad = true
	a := newWithBackend(Options{}, fb)
	runApp(t, a)
	s, _ := a.NewSurface(SurfaceOptions{})
	s.Send("hi")
	deadline := time.Now().Add(timeout)
	for len(fb.Surface(s.ID()).Evals()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no Loaded")
		}
		time.Sleep(time.Millisecond)
	}
	if ev := fb.Surface(s.ID()).Evals(); len(ev) != 1 {
		t.Fatalf("evals %q", ev)
	}
}

func TestOnMessage(t *testing.T) {
	a, fb := newTestApp(t)
	type got struct {
		s    *Surface
		msg  string
		main bool
	}
	ch := make(chan got, 8)
	a.OnMessage(func(s *Surface, msg string) { ch <- got{s, msg, a.isMain()} })
	s, _ := a.NewSurface(SurfaceOptions{})
	dev, _ := a.NewSurface(SurfaceOptions{URL: "http://localhost:5173/"})

	fb.Message(s.ID(), "hello")
	fb.MessageFrom(s.ID(), "https://evil.example", "dropped")
	fb.MessageFrom(dev.ID(), "http://localhost:5173", "dev")
	fb.MessageFrom(dev.ID(), "", "dropped")
	fb.Message(9999, "unknown surface")
	for _, want := range []got{{s, "hello", true}, {dev, "dev", true}} {
		select {
		case g := <-ch:
			if g != want {
				t.Fatalf("got %+v want %+v", g, want)
			}
		case <-time.After(timeout):
			t.Fatal("missing message")
		}
	}
	select {
	case g := <-ch:
		t.Fatalf("unexpected %+v", g)
	default:
	}
}

func TestOnCloseVeto(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	var allow atomic.Bool
	var asked atomic.Int32
	s.OnClose(func() bool { asked.Add(1); return true })
	s.OnClose(func() bool { asked.Add(1); return allow.Load() })
	var third atomic.Int32
	s.OnClose(func() bool { third.Add(1); return true })
	closed := make(chan struct{})
	s.OnClosed(func() { close(closed) })

	if fb.RequestClose(s.ID()) {
		t.Fatal("closed despite veto")
	}
	if asked.Load() != 2 || third.Load() != 0 || len(a.Surfaces()) != 1 {
		t.Fatal("veto bookkeeping: later handler asked or surface gone")
	}
	allow.Store(true)
	if !fb.RequestClose(s.ID()) || third.Load() != 1 {
		t.Fatal("not closed when allowed")
	}
	<-closed
}

func TestCloseIgnoresOnClose(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	s.OnClose(func() bool { return false })
	s.Close()
	if !fb.Surface(s.ID()).Closed() {
		t.Fatal("Close asked OnClose")
	}
}

func TestBridgeScripts(t *testing.T) {
	a, fb := newTestApp(t)
	a.AddBridgeScript("window.one=1")
	a.AddBridgeScript("window.two=2")
	s, _ := a.NewSurface(SurfaceOptions{})
	bs := fb.Surface(s.ID()).Options().BridgeScript
	i0, i1, i2 := strings.Index(bs, "window.__tango = "), strings.Index(bs, "window.one=1"), strings.Index(bs, "window.two=2")
	if !(i0 >= 0 && i0 < i1 && i1 < i2) {
		t.Fatalf("order %d %d %d", i0, i1, i2)
	}
}

func TestServeAsset(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "asset "+r.URL.Path)
	})}, fb)
	h := host{a}
	rec := httptest.NewRecorder()
	h.ServeAsset(0, rec, httptest.NewRequest("GET", "tango://app/x.js", nil))
	if rec.Body.String() != "asset /x.js" {
		t.Fatalf("%q", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	host{newWithBackend(Options{}, fb)}.ServeAsset(0, rec, httptest.NewRequest("GET", "tango://app/x.js", nil))
	if rec.Code != 404 {
		t.Fatalf("nil Assets: %d", rec.Code)
	}
}

func TestRecvJSEscapes(t *testing.T) {
	js := recvJS("a\u2028b\u2029c\"\\\n")
	if strings.ContainsAny(js, "\u2028\u2029\n") {
		t.Fatalf("unescaped: %q", js)
	}
	if js != `window.__tango._recv("a\u2028b\u2029c\"\\\n")` {
		t.Fatalf("%q", js)
	}
}
