package updateui

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/service/updater"
)

// en makes the window title deterministic whatever the test's locale.
var en = Options{Text: stringsEN, Interval: -1}

func runApp(t *testing.T) (*tango.App, *fake.Backend) {
	t.Helper()
	a, fb := faketest.NewApp(tango.Options{})
	faketest.Start(t, a, fb)
	return a, fb
}

// updateWindows lists the open update windows.
func updateWindows(fb *fake.Backend) []*fake.Surface {
	var out []*fake.Surface
	for _, s := range fb.Surfaces() {
		if s.Title() == stringsEN.Title && !s.Closed() {
			out = append(out, s)
		}
	}
	return out
}

func sessionModel(u *updater.Updater) *model {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s := sessions[u]; s != nil {
		return s.m
	}
	return nil
}

func TestStartValidatesConfig(t *testing.T) {
	a, _ := runApp(t)
	orig := func(updater.Event) {}
	for name, u := range map[string]*updater.Updater{
		"no url":     {PublicKey: "k", Version: "1.0.0"},
		"no version": {ManifestURL: "https://x", PublicKey: "k"},
		"no key":     {ManifestURL: "https://x", Version: "1.0.0"},
	} {
		u.OnEvent = orig
		if _, err := Start(a, u, en); err == nil {
			t.Errorf("%s: Start accepted it", name)
		}
		if sessionModel(u) != nil {
			t.Errorf("%s: a session was left behind", name)
		}
	}
}

func TestCheckNowShowsUpToDate(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	CheckNow(a, u, en)
	waitFor(t, "the update window", func() bool { return len(updateWindows(fb)) == 1 })
	waitFor(t, "up to date", func() bool { m := sessionModel(u); return m != nil && m.snapshot().phase == phaseUpToDate })
}

func TestOneWindowPerApp(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	CheckNow(a, u, en)
	waitFor(t, "the update window", func() bool { return len(updateWindows(fb)) == 1 })
	waitFor(t, "up to date", func() bool { return sessionModel(u).snapshot().phase == phaseUpToDate })
	CheckNow(a, u, en)
	waitFor(t, "the second check", func() bool { return s.hits.Load() == 2 })
	time.Sleep(50 * time.Millisecond)
	if n := len(updateWindows(fb)); n != 1 {
		t.Fatalf("%d update windows, want 1", n)
	}
}

func TestOnEventWrappedOnce(t *testing.T) {
	a, _ := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	var calls atomic.Int32
	u.OnEvent = func(updater.Event) { calls.Add(1) }
	stop1, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	stop2, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	CheckNow(a, u, en)
	waitFor(t, "the check", func() bool { return sessionModel(u).snapshot().phase == phaseAvailable })
	if n := calls.Load(); n != 2 { // checking, available
		t.Fatalf("app OnEvent called %d times, want 2", n)
	}
	stop2()
	stop1()
	calls.Store(0)
	u.OnEvent(updater.Event{Kind: updater.EventChecking})
	if n := calls.Load(); n != 1 || sessionModel(u) != nil {
		t.Fatalf("after stop: app OnEvent calls %d, session %v", n, sessionModel(u))
	}
}

func TestStartOffersDownloadedUpdate(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	file := filepath.Join(t.TempDir(), "new.AppImage")
	os.WriteFile(file, []byte("x"), 0o600)
	pay := &updater.Payload{Release: &updater.Release{Version: "1.3.0"}, Path: file}
	if err := saveState(u, persisted{Pending: newPending(pay)}); err != nil {
		t.Fatal(err)
	}
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the downloaded-update window", func() bool { return len(updateWindows(fb)) == 1 })
	if v := sessionModel(u).snapshot(); v.phase != phaseDownloaded || v.rel == nil || v.rel.Version != "1.3.0" {
		t.Fatalf("model %+v", v)
	}
	stop()
	waitFor(t, "the window to close", func() bool { return len(updateWindows(fb)) == 0 })

	os.Remove(file)
	stop, err = Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	time.Sleep(50 * time.Millisecond)
	if n := len(updateWindows(fb)); n != 0 {
		t.Fatalf("missing download offered: %d windows", n)
	}
	if loadState(u).Pending != nil {
		t.Fatal("stale kept update not forgotten")
	}
}

// Final review I4: Start removes leftover downloads but keeps the one it
// offers.
func TestStartPrunesDownloads(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	stale := filepath.Join(downloadsOf(u), "1.1.0")
	file := filepath.Join(downloadsOf(u), "1.3.0", "new.AppImage")
	os.MkdirAll(stale, 0o700)
	os.MkdirAll(filepath.Dir(file), 0o700)
	os.WriteFile(file, []byte("x"), 0o600)
	pay := &updater.Payload{Release: &updater.Release{Version: "1.3.0"}, Path: file}
	if err := saveState(u, persisted{Pending: newPending(pay)}); err != nil {
		t.Fatal(err)
	}
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	waitFor(t, "the downloaded-update window", func() bool { return len(updateWindows(fb)) == 1 })
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("leftover download kept: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("offered download removed: %v", err)
	}
}

func TestStartReportsRolledBack(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	os.WriteFile(filepath.Join(u.StateDir, "pending.json"), []byte(`{"from":"1.2.0","to":"1.3.0"}`), 0o600)
	os.WriteFile(filepath.Join(u.StateDir, "rolledback"), []byte("did not confirm"), 0o600)
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	waitFor(t, "the rolled-back window", func() bool { return len(updateWindows(fb)) == 1 })
	if v := sessionModel(u).snapshot(); v.phase != phaseRolledBack {
		t.Fatalf("model %+v", v)
	}
}

func TestStopClosesWindowAndRestores(t *testing.T) {
	defer func(f func(time.Duration) <-chan time.Time) { after = f }(after)
	fire := make(chan time.Time)
	after = func(time.Duration) <-chan time.Time { return fire }
	a, fb := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	stop, err := Start(a, u, Options{Text: stringsEN, Interval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	CheckNow(a, u, en)
	waitFor(t, "the update window", func() bool { return len(updateWindows(fb)) == 1 })
	waitFor(t, "the manual check", func() bool { return s.hits.Load() == 1 })
	stop()
	stop() // twice is fine
	waitFor(t, "the window to close", func() bool { return len(updateWindows(fb)) == 0 })
	select {
	case fire <- time.Now(): // a stopped schedule may still be waiting on it
	case <-time.After(100 * time.Millisecond):
	}
	time.Sleep(50 * time.Millisecond)
	if n := s.hits.Load(); n != 1 {
		t.Fatalf("background check after stop: %d manifest requests", n)
	}
}

// stop during a running check must not change u.OnEvent under the
// Updater: it cancels the check and waits for it before restoring.
func TestStopWaitsForRunningCheck(t *testing.T) {
	a, _ := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.3.0")
	hold := make(chan struct{}) // released only at cleanup: cancelling must end the check
	t.Cleanup(func() { close(hold) })
	s.mu.Lock()
	s.hold = hold
	s.mu.Unlock()
	u := s.updater(t, "1.2.0")
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	CheckNow(a, u, en)
	waitFor(t, "the check to reach the server", func() bool { return s.hits.Load() == 1 })
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not cancel the running check")
	}
	// stop returned, so the client side of the check is over; the server
	// learns of the cancelled request a moment later.
	waitFor(t, "the server to see the check cancelled", func() bool { return s.aborted.Load() == 1 })
	if u.OnEvent != nil {
		t.Fatal("OnEvent not restored to the app's (nil) after stop")
	}
}

// Final review I1: a show queued by a check that ends as the session stops
// must not open the window again after stop.
func TestNoWindowAfterStop(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	sessionsMu.Lock()
	sess := sessions[u]
	sessionsMu.Unlock()
	stop()
	sess.show(true)
	done := make(chan struct{})
	a.Post(func() { close(done) }) // runs after show's callback
	<-done
	if n := len(updateWindows(fb)); n != 0 {
		t.Fatalf("%d update windows after stop", n)
	}
}

// Final review I2: a CheckNow while an earlier stop still waits for a
// running operation gets a fresh session once that stop is done, and the
// stop's restore of u.OnEvent does not undo the new session's wrapper.
func TestCheckNowDuringStop(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	sessionsMu.Lock()
	old := sessions[u]
	sessionsMu.Unlock()
	if !old.f.begin() { // an operation stop must wait for
		t.Fatal("flow busy")
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	waitFor(t, "stop to halt the flow", old.f.isStopped)
	CheckNow(a, u, en)
	time.Sleep(50 * time.Millisecond)
	if n := s.hits.Load(); n != 0 {
		t.Fatalf("checked beside the stopping session: %d requests", n)
	}
	old.f.end()
	<-stopped
	waitFor(t, "the new session's check", func() bool { return s.hits.Load() == 1 })
	waitFor(t, "its window", func() bool { return len(updateWindows(fb)) == 1 })
	if m := sessionModel(u); m == nil || m == old.m {
		t.Fatal("no fresh session")
	}
	if u.OnEvent == nil {
		t.Fatal("the old stop undid the new session's OnEvent wrapper")
	}
}

// The update window is auxiliary (tango.SurfaceOptions.Auxiliary): with
// QuitOnLastClose, closing the app's main window quits the app even while
// the update window is open.
func TestUpdateWindowDoesNotKeepAppRunning(t *testing.T) {
	yes := true
	a, fb := faketest.NewApp(tango.Options{QuitOnLastClose: &yes})
	faketest.Start(t, a, fb)
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	main, err := a.NewSurface(tango.SurfaceOptions{Title: "Main"})
	if err != nil {
		t.Fatal(err)
	}
	CheckNow(a, u, en)
	waitFor(t, "the update window", func() bool { return len(updateWindows(fb)) == 1 })
	tango.Await(a, func() bool { return fb.RequestClose(main.ID()) })
	waitFor(t, "the app to quit", func() bool { return fb.Quits() > 0 })
}

// Review: a release downloaded through a CheckNow before Start stays on
// offer, and its files stay, even when Start finds one kept from an
// earlier launch.
func TestStartKeepsCheckNowDownload(t *testing.T) {
	a, fb := runApp(t)
	s := newReleaseServer(t)
	s.publish("1.4.0")
	u := s.updater(t, "1.2.0")
	kept := filepath.Join(downloadsOf(u), "1.3.0", "new.AppImage")
	os.MkdirAll(filepath.Dir(kept), 0o700)
	os.WriteFile(kept, []byte("x"), 0o600)
	pay := &updater.Payload{Release: &updater.Release{Version: "1.3.0"}, Path: kept}
	if err := saveState(u, persisted{Pending: newPending(pay)}); err != nil {
		t.Fatal(err)
	}
	CheckNow(a, u, en)
	waitFor(t, "the offer", func() bool { m := sessionModel(u); return m != nil && m.snapshot().phase == phaseAvailable })
	sessionsMu.Lock()
	sess := sessions[u]
	sessionsMu.Unlock()
	sess.f.download()
	waitFor(t, "the download", func() bool { return sess.m.snapshot().phase == phaseReady })
	sess.f.mu.Lock()
	got := sess.f.payload.Path
	sess.f.mu.Unlock()
	stop, err := Start(a, u, en)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if v := sess.m.snapshot(); v.phase != phaseReady || v.rel.Version != "1.4.0" {
		t.Fatalf("Start replaced the 1.4.0 download: %+v", v)
	}
	for _, f := range []string{got, kept} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("Start removed %s: %v", f, err)
		}
	}
	_ = fb
}
