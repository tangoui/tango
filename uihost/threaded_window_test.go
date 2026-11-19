package uihost_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/uihost"
)

// Threaded windows: the ui.Window calls that need the main thread are
// posted to it, and the app-wide notifications that arrive there reach the
// runner on its UI thread (spec §4.2, §5.3, §5.7–5.9).

// panicLog collects the app's reported panics (an off-thread runner call
// panics in tango_dev builds).
type panicLog struct {
	mu sync.Mutex
	ps []tango.Panic
}

func (l *panicLog) opts() tango.Options {
	return tango.Options{OnPanic: func(p tango.Panic) bool {
		l.mu.Lock()
		l.ps = append(l.ps, p)
		l.mu.Unlock()
		return true
	}}
}

func (l *panicLog) check(t *testing.T) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.ps {
		t.Errorf("panic in %s: %v", p.Where, p.Value)
	}
}

// syncAppPosts is a barrier in the App.Post queue, not the fake's queue.
func syncAppPosts(a *tango.App) { tango.Await(a, func() struct{} { return struct{}{} }) }

// blockMain parks a's main thread until the returned release is called.
func blockMain(t *testing.T, a *tango.App) (release func()) {
	t.Helper()
	parked, gate := make(chan struct{}), make(chan struct{})
	a.Post(func() { close(parked); <-gate })
	<-parked
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return release
}

// blockUI parks tc's UI thread until the returned release is called. The
// UI thread's earlier work happens before blockUI returns.
func blockUI(t *testing.T, tc *threadedCanvas) (release func()) {
	t.Helper()
	parked, gate := make(chan struct{}), make(chan struct{})
	tc.th.Post(func() { close(parked); <-gate })
	<-parked
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return release
}

// onUIWithin runs fn on tc's UI thread and reports whether it returned
// within d.
func onUIWithin(tc *threadedCanvas, d time.Duration, fn func()) bool {
	done := make(chan struct{})
	go func() { tc.th.Do(fn); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func gidsAll(t *testing.T, what string, gids []uint64, want uint64) {
	t.Helper()
	if len(gids) == 0 {
		t.Fatalf("%s never called", what)
	}
	for _, g := range gids {
		if g != want {
			t.Fatalf("%s called on goroutines %v, main is %d", what, gids, want)
		}
	}
}

func TestCursorAndTextInputPostedToMain(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {})
	tc.flush(t)
	caret := geom.Rect{X: 10, Y: 20, W: 2, H: 16}
	tc.th.Do(func() {
		w := r.Context().Window()
		w.SetCursor(input.CursorPointer)
		w.SetTextInput(true, caret)
	})
	syncAppPosts(a)
	main := mainGID(a)
	tc.mu.Lock()
	cursor, text := slices.Clone(tc.cursorGIDs), slices.Clone(tc.textGIDs)
	tc.mu.Unlock()
	gidsAll(t, "SetCursor", cursor, main)
	gidsAll(t, "SetTextInput", text, main)
	if cv.Cursor() != input.CursorPointer {
		t.Fatalf("cursor %v", cv.Cursor())
	}
	if on, got := cv.TextInput(); !on || got != caret {
		t.Fatalf("text input %v %v", on, got)
	}
}

// The final-state calls do not wait for the main thread: they return while
// it is busy and take effect once it runs.
func TestWindowStatePostedToMainWithoutWaiting(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {})
	tc.flush(t)
	release := blockMain(t, a)
	if !onUIWithin(tc, faketest.Timeout/2, func() {
		w := r.Context().Window()
		w.SetCursor(input.CursorText)
		w.SetTextInput(true, geom.Rect{W: 1, H: 10})
		w.(ui.ShortcutCaptureWindow).SetShortcutCapture(true)
		w.WriteClipboard("posted")
		w.(ui.WindowFrame).SetWindowState(ui.WindowMaximized)
		w.(ui.WindowFrame).StartWindowMove()
	}) {
		release()
		t.Fatal("a final-state call waited for the main thread")
	}
	release()
	syncAppPosts(a)
	if st := s.State(); st != tango.StateMaximized {
		t.Fatalf("state %v", st)
	}
	if got := fb.ClipboardText(); got != "posted" {
		t.Fatalf("clipboard %q", got)
	}
	if cv.Cursor() != input.CursorText {
		t.Fatalf("cursor %v", cv.Cursor())
	}

	release = blockMain(t, a)
	if !onUIWithin(tc, faketest.Timeout/2, func() { r.Context().Window().(ui.WindowFrame).CloseWindow() }) {
		release()
		t.Fatal("CloseWindow waited for the main thread")
	}
	release()
	select {
	case <-tc.th.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("window did not close")
	}
}

func TestReadClipboardAwaitsMainAndSetsFlag(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {})
	tc.flush(t)
	fb.Do(func() { _ = fb.ClipboardWriteText("héllo") })
	release := blockMain(t, a)
	got := make(chan string, 1)
	go tc.th.Do(func() { got <- r.Context().Window().ReadClipboard() })
	deadline := time.Now().Add(faketest.Timeout)
	for !tc.th.AwaitingMain() {
		if time.Now().After(deadline) {
			release()
			t.Fatal("AwaitingMain false while ReadClipboard waits for the main thread")
		}
		time.Sleep(time.Millisecond)
	}
	release()
	select {
	case s := <-got:
		if s != "héllo" {
			t.Fatalf("ReadClipboard = %q", s)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("ReadClipboard did not return")
	}
	tc.sync(t)
	if tc.th.AwaitingMain() {
		t.Fatal("AwaitingMain still true after ReadClipboard returned")
	}
}

func TestThemeChangePostedToUIThread(t *testing.T) {
	var pl panicLog
	a, fb := faketest.Run(t, pl.opts())
	fb.SetTheme(platform.Theme{})
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) { ui.Text(c, "theme") })
	for tc.flush(t) {
	}
	release := blockUI(t, tc)
	fb.SetTheme(platform.Theme{Dark: true})
	syncAppPosts(a)
	// The UI thread is parked: the notification may only have queued work.
	if r.Context().Theme.Dark {
		t.Fatal("theme switched on the main thread")
	}
	release()
	for tc.flush(t) {
	}
	var dark bool
	tc.th.Do(func() { dark = r.Context().Theme.Dark })
	if !dark {
		t.Fatal("dark theme not applied on the UI thread")
	}
	if !tango.Await(a, func() bool { return uihost.FollowingSystemTheme(r) }) {
		t.Fatal("not following after the switch")
	}
	pl.check(t)
}

// Contrast, reduce-motion and memory notifications reach a threaded runner
// on its UI thread too.
func TestSystemPreferencesPostedToUIThread(t *testing.T) {
	var pl panicLog
	a, fb := faketest.Run(t, pl.opts())
	fb.SetTheme(platform.Theme{})
	fb.SetHighContrast(false)
	fb.SetReduceMotion(false)
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) { ui.Text(c, "prefs") })
	for tc.flush(t) {
	}
	release := blockUI(t, tc)
	fb.SetHighContrast(true)
	fb.SetReduceMotion(true)
	fb.AppEvent(platform.AppEvent{Kind: platform.AppMemoryWarning})
	syncAppPosts(a)
	if c := r.Context(); c.Theme.HighContrast || c.ReduceMotion() {
		t.Fatal("preference applied on the main thread")
	}
	release()
	for tc.flush(t) {
	}
	var hc, rm bool
	tc.th.Do(func() { hc, rm = r.Context().Theme.HighContrast, r.Context().ReduceMotion() })
	if !hc || !rm {
		t.Fatalf("high contrast %v, reduce motion %v after the UI thread ran", hc, rm)
	}
	pl.check(t)
}

func TestAccessibilityActionRunsOnUIThread(t *testing.T) {
	var pl panicLog
	a, fb := faketest.Run(t, pl.opts())
	var mu sync.Mutex
	clicks := 0
	var clickGID uint64
	var tree *a11y.Tree
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		if ui.Button(c, "Go").Clicked() {
			mu.Lock()
			clicks++
			clickGID = uithread.GoroutineID()
			mu.Unlock()
		}
	})
	if err := tango.Await(a, func() error { return tc.sink.(platform.CanvasAccessibleSink).AccessibilityAction(1, a11y.Press, "") }); err == nil {
		t.Error("accepted an action before a tree was published")
	}
	tc.th.Do(func() {
		uihost.A11yForce(r)
		uihost.A11yTap(r, func(tr *a11y.Tree, _ []a11y.NodeID) {
			mu.Lock()
			tree = tr
			mu.Unlock()
		})
	})
	for tc.flush(t) {
	}
	mu.Lock()
	var btn *a11y.Node
	if tree != nil {
		btn = tree.Find(a11y.RoleButton, "Go")
	}
	mu.Unlock()
	if btn == nil {
		t.Fatal("no button in the tree")
	}
	sink := tc.sink.(platform.CanvasAccessibleSink)
	main := mainGID(a)
	release := blockUI(t, tc)
	tc.mu.Lock()
	before := len(tc.reqGIDs)
	tc.mu.Unlock()
	var err error
	fb.Do(func() {
		if sink.AccessibilityAction(0, a11y.Press, "") == nil {
			t.Error("accepted an unknown node")
		}
		if sink.AccessibilityAction(btn.ID, a11y.SetValue, "") == nil {
			t.Error("accepted an unsupported action")
		}
		err = sink.AccessibilityAction(btn.ID, a11y.Press, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	tc.mu.Lock()
	onMain := slices.Contains(tc.reqGIDs[before:], main)
	tc.mu.Unlock()
	if onMain {
		t.Fatal("the action reached the runner on the main thread")
	}
	release()
	for tc.flush(t) {
	}
	mu.Lock()
	defer mu.Unlock()
	if clicks != 1 || clickGID != tc.gid {
		t.Fatalf("clicks %d on goroutine %d, UI thread %d", clicks, clickGID, tc.gid)
	}
	pl.check(t)
}

// A threaded window's shortcuts are matched on the main thread and their
// functions run there; a consumed key never reaches the runner.
func TestThreadedShortcutRunsOnMain(t *testing.T) {
	var pl panicLog
	a, fb := faketest.Run(t, pl.opts())
	var mu sync.Mutex
	var keys []input.KeyEvent
	_, s, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		ks := c.KeysFor(0)
		mu.Lock()
		keys = append(keys, ks...)
		mu.Unlock()
	})
	for tc.flush(t) {
	}
	var fnGID uint64
	fired := 0
	shortcut(t, s, "F5", uihost.ShortcutOptions{}, func() { fired++; fnGID = uithread.GoroutineID() })
	press(cv, input.KeyF5, 0)
	press(cv, input.KeyF6, 0)
	for tc.flush(t) {
	}
	main := mainGID(a)
	var n int
	var g uint64
	fb.Do(func() { n, g = fired, fnGID })
	if n != 1 || g != main {
		t.Fatalf("shortcut fired %d times, on goroutine %d, main %d", n, g, main)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, k := range keys {
		if k.Key == input.KeyF5 {
			t.Fatalf("consumed key reached the runner: %v", keys)
		}
	}
	if !slices.ContainsFunc(keys, func(k input.KeyEvent) bool { return k.Key == input.KeyF6 }) {
		t.Fatalf("unbound key did not reach the runner: %v", keys)
	}
	pl.check(t)
}

func TestThreadedLocaleSurvivesCloseAndConsecutiveChanges(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	i18n.SetLocale("en")
	_, s, tc, _ := openThreaded(t, a, fb, func(*ui.Context) {})
	release := blockUI(t, tc)
	defer release()
	i18n.SetLocale("ja")
	i18n.SetLocale("zh-TW")
	syncAppPosts(a)
	fb.Do(func() {
		if got := text.DefaultLang(); got != ui.ZhTW.Code {
			t.Errorf("consecutive changes left language %q", got)
		}
		s.Close()
	})
	release()
	if got := text.DefaultLang(); got != ui.ZhTW.Code {
		t.Errorf("closing the window left language %q", got)
	}
}

func TestThreadedFrameInfoDoesNotWaitForMain(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, tc, _ := openThreaded(t, a, fb, func(*ui.Context) {})
	check := func(want ui.WindowState, noClose bool) {
		t.Helper()
		release := blockMain(t, a)
		var got ui.WindowFrameInfo
		ok := onUIWithin(tc, faketest.Timeout/2, func() { got = r.Context().Window().(ui.WindowFrame).FrameInfo() })
		release()
		if !ok {
			t.Fatal("FrameInfo waited for the main thread")
		}
		if got.State != want || got.NoClose != noClose {
			t.Fatalf("FrameInfo = %+v, want state %v NoClose %v", got, want, noClose)
		}
	}
	check(ui.WindowNormal, false)
	fb.Do(func() { _ = s.Maximize() })
	check(ui.WindowMaximized, false)
	fb.Do(func() { _ = s.SetAppearance(tango.Appearance{NoClose: true}) })
	check(ui.WindowMaximized, true)
}

// Recording reaches the main thread's decisions with the frame snapshot
// (snapshot.go) and native menu accelerators with a main-thread task;
// neither takes effect while main is still running the press.
func TestThreadedCaptureCommitsOnMain(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
	fired := 0
	shortcut(t, s, "F5", uihost.ShortcutOptions{}, func() { fired++ })
	fb.Do(func() {
		tc.th.Do(func() { r.Context().Window().(ui.ShortcutCaptureWindow).SetShortcutCapture(true) })
		tc.sink.CanvasInput(input.KeyEvent{Key: input.KeyF5, Down: true})
		tc.sink.CanvasInput(input.KeyEvent{Key: input.KeyF5, Down: false})
		if fired != 1 || cv.ShortcutCapture() {
			t.Errorf("capture committed before main task: fired=%d native=%v", fired, cv.ShortcutCapture())
		}
	})
	syncAppPosts(a)
	tc.tick(t) // publishes the recording state
	press(cv, input.KeyF5, 0)
	fb.Do(func() {
		if fired != 1 || !cv.ShortcutCapture() {
			t.Errorf("capture not committed: fired=%d native=%v", fired, cv.ShortcutCapture())
		}
	})
}

func TestThreadedLocaleLateCallbackUsesLatest(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	i18n.SetLocale("en")
	parked, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cancel := i18n.OnChange(func(code string) {
		if code == "ja" {
			close(parked)
			<-release
		}
	})
	defer cancel()
	openThreaded(t, a, fb, func(*ui.Context) {})
	go func() { defer close(done); i18n.SetLocale("ja") }()
	select {
	case <-parked:
	case <-time.After(faketest.Timeout):
		close(release)
		t.Fatal("older listener did not start")
	}
	i18n.SetLocale("zh-TW")
	syncAppPosts(a)
	close(release)
	<-done
	cancel()
	syncAppPosts(a)
	if got := text.DefaultLang(); got != ui.ZhTW.Code {
		t.Errorf("late callback restored language %q", got)
	}
	ui.SetDefaultLocale(ui.EnUS)
	i18n.SetLocale("ja")
	syncAppPosts(a)
	if got := text.DefaultLang(); got != ui.EnUS.Code {
		t.Errorf("follow overwrote explicit language with %q", got)
	}
}

func TestThreadedA11yAcceptsDuringPublication(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) { ui.Button(c, "Go") })
	tc.th.Do(func() {
		cancel := uihost.A11yTap(r, func(tree *a11y.Tree, _ []a11y.NodeID) {
			if tree == nil {
				return
			}
			btn := tree.Find(a11y.RoleButton, "Go")
			if btn == nil {
				t.Error("published without the button")
				return
			}
			if err := cv.A11yAction(btn.ID, a11y.Press, ""); err != nil {
				t.Errorf("published action rejected inside frame: %v", err)
			}
		})
		t.Cleanup(func() { tc.th.Do(cancel) })
		uihost.A11yForce(r)
	})
	tc.tick(t)
}

func TestThreadedA11yDropsQueuedActionOnClose(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) { ui.Button(c, "Go") })
	tc.th.Do(func() { uihost.A11yForce(r) })
	tc.tick(t)
	var id a11y.NodeID
	tc.th.Do(func() { id = r.AccessibilityTree().Find(a11y.RoleButton, "Go").ID })
	release := blockUI(t, tc)
	tc.mu.Lock()
	before := tc.requests
	tc.mu.Unlock()
	if err := cv.A11yAction(id, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if !fb.RequestClose(s.ID()) {
		t.Fatal("window did not close")
	}
	syncAppPosts(a)
	release()
	select {
	case <-tc.th.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("UI thread did not stop")
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.requests != before {
		t.Error("queued action requested a frame after close")
	}
}

func TestThreadedA11yDropsQueuedActionOnRunnerClose(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) { ui.Button(c, "Go") })
	tc.th.Do(func() { uihost.A11yForce(r) })
	tc.tick(t)
	var id a11y.NodeID
	tc.th.Do(func() { id = r.AccessibilityTree().Find(a11y.RoleButton, "Go").ID })
	release := blockUI(t, tc)
	count := func() int { tc.mu.Lock(); defer tc.mu.Unlock(); return tc.requests }
	before := count()
	if err := cv.A11yAction(id, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	r.Close() // Close only the runner while the valid action is still queued.
	release()
	tc.sync(t)
	if cv.Surface().Closed() {
		t.Fatal("closing the runner closed the window")
	}
	if got := count() - before; got != 0 {
		t.Errorf("queued action requested %d frames after runner close; want 0", got)
	}
}

func TestThreadedA11yActionFrameRequests(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	shown := true // UI thread only after Open
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		if shown {
			ui.Button(c, "Go")
		}
	})
	tc.th.Do(func() { uihost.A11yForce(r) })
	tc.tick(t)
	var id a11y.NodeID
	tc.th.Do(func() { id = r.AccessibilityTree().Find(a11y.RoleButton, "Go").ID })
	count := func() int { tc.mu.Lock(); defer tc.mu.Unlock(); return tc.requests }
	before := count()
	if err := cv.A11yAction(id, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	tc.sync(t)
	if got := count() - before; got != 1 {
		t.Errorf("valid action requested %d frames; want 1", got)
	}

	release := blockUI(t, tc)
	tc.th.Post(func() {
		shown = false
		tc.tickFn()
		before = count()
	})
	// The main thread still sees the button, but the queued frame removes it
	// before the action is revalidated by the runner.
	if err := cv.A11yAction(id, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	release()
	tc.sync(t)
	if got := count() - before; got != 0 {
		t.Errorf("stale action requested %d frames; want 0", got)
	}
}

func TestThreadedA11yRequestPanicGoesToOnPanic(t *testing.T) {
	panics := make(chan tango.Panic, 1)
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool { panics <- p; return true }})
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) { ui.Button(c, "Go") })
	tc.th.Do(func() { uihost.A11yForce(r) })
	tc.tick(t)
	var id a11y.NodeID
	tc.th.Do(func() { id = r.AccessibilityTree().Find(a11y.RoleButton, "Go").ID })
	const boom = "controlled accessibility frame request panic"
	tc.mu.Lock()
	tc.requestPanic = boom
	tc.mu.Unlock()
	if err := cv.A11yAction(id, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	tc.sync(t)
	select {
	case p := <-panics:
		if p.Value != boom || p.Where != "event handler" {
			t.Fatalf("unexpected panic report: %+v", p)
		}
	default:
		t.Fatal("frame request panic did not reach OnPanic")
	}
}
