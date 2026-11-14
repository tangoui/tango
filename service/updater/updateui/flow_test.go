package updateui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// ---- a release server ----

const testPlatform = "linux-amd64" // fixed, so the tests do not depend on the host

type releaseServer struct {
	t        *testing.T
	srv      *httptest.Server
	pub      string
	priv     string
	mu       sync.Mutex
	envelope []byte
	status   int           // non-zero: the manifest answers with this status
	file     []byte        // the release file
	hold     chan struct{} // non-nil: requests wait on it (manifest and file)
	hits     atomic.Int32  // manifest requests
	aborted  atomic.Int32  // held requests the client gave up on
}

func newReleaseServer(t *testing.T) *releaseServer {
	t.Helper()
	pub, priv, err := updater.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s := &releaseServer{t: t, pub: pub, priv: priv, file: []byte("new release bytes")}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.wait(r)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.status != 0 {
			w.WriteHeader(s.status)
			return
		}
		w.Write(s.envelope)
	})
	mux.HandleFunc("/app.AppImage", func(w http.ResponseWriter, r *http.Request) {
		s.wait(r)
		s.mu.Lock()
		b := s.file
		s.mu.Unlock()
		w.Write(b)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *releaseServer) wait(r *http.Request) {
	s.mu.Lock()
	h := s.hold
	s.mu.Unlock()
	if h != nil {
		select {
		case <-h:
		case <-r.Context().Done():
			s.aborted.Add(1)
		}
	}
}

// publish signs a manifest for version v with one AppImage for testPlatform.
func (s *releaseServer) publish(v string) {
	s.t.Helper()
	sum := sha256.Sum256(s.file)
	m := &updater.Manifest{
		App: "com.example.test", Version: v, PubDate: time.Now(), Notes: "- fixes",
		Platforms: map[string]updater.Asset{testPlatform: {
			Format: updater.FormatAppImage,
			File:   updater.File{URL: s.srv.URL + "/app.AppImage", Size: int64(len(s.file)), SHA256: hex.EncodeToString(sum[:])},
		}},
	}
	env, err := updater.SignManifest(s.priv, m)
	if err != nil {
		s.t.Fatal(err)
	}
	s.mu.Lock()
	s.envelope, s.status = env, 0
	s.mu.Unlock()
}

func (s *releaseServer) updater(t *testing.T, version string) *updater.Updater {
	u := &updater.Updater{
		ManifestURL: s.srv.URL + "/latest.json", PublicKey: s.pub, Version: version,
		AppID: "com.example.test", Platform: testPlatform, StateDir: t.TempDir(),
	}
	// Registered after TempDir, so it runs first: a session a test left
	// running (CheckNow has no stop) must not write to the state
	// directory while it is removed.
	t.Cleanup(func() {
		sessionsMu.Lock()
		sess := sessions[u]
		sessionsMu.Unlock()
		if sess != nil {
			sess.stopFn()
		}
	})
	return u
}

// testFlow is a flow over s whose show and quit calls are recorded.
type testFlow struct {
	*flow
	mu    sync.Mutex
	shows []bool
	quits int
}

func newTestFlow(_ *testing.T, u *updater.Updater) *testFlow {
	tf := &testFlow{}
	tf.flow = newFlow(u, &model{current: u.Version})
	tf.flow.show = func(manual bool) { tf.mu.Lock(); tf.shows = append(tf.shows, manual); tf.mu.Unlock() }
	tf.flow.quit = func() { tf.mu.Lock(); tf.quits++; tf.mu.Unlock() }
	u.OnEvent = tf.flow.onEvent
	return tf
}

func (tf *testFlow) showCalls() []bool {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return append([]bool(nil), tf.shows...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ---- tests ----

func TestCheckAvailableShowsWindow(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	if got := tf.showCalls(); len(got) != 1 || got[0] {
		t.Fatalf("show calls %v, want one background show", got)
	}
	if v := tf.m.snapshot(); v.phase != phaseAvailable || v.rel == nil || v.rel.Version != "1.3.0" {
		t.Fatalf("model %+v", v)
	}
}

func TestBackgroundSkipsSkippedVersion(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	tf.check(context.Background(), false)
	tf.skip()
	if loadState(u).Skipped != "1.3.0" {
		t.Fatal("skip not saved")
	}
	tf2 := newTestFlow(t, u) // next launch
	tf2.check(context.Background(), false)
	if got := tf2.showCalls(); len(got) != 0 {
		t.Fatalf("skipped version shown again: %v", got)
	}
	s.publish("1.4.0")
	tf2.check(context.Background(), false)
	if got := tf2.showCalls(); len(got) != 1 {
		t.Fatalf("newer version not shown: %v", got)
	}
	tf2.check(context.Background(), true) // asked for: shown even when skipped
	if got := tf2.showCalls(); len(got) != 2 || !got[1] {
		t.Fatalf("manual check: %v", got)
	}
}

func TestLaterSuppressesForThisRunOnly(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	tf.check(context.Background(), false)
	tf.later()
	tf.check(context.Background(), false)
	if got := tf.showCalls(); len(got) != 1 {
		t.Fatalf("later: shown again this run: %v", got)
	}
	tf2 := newTestFlow(t, u)
	tf2.check(context.Background(), false)
	if got := tf2.showCalls(); len(got) != 1 {
		t.Fatalf("later: not shown next run: %v", got)
	}
}

func TestBackgroundSilentWhenUpToDateOrError(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.2.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	s.mu.Lock()
	s.status = http.StatusInternalServerError
	s.mu.Unlock()
	tf.check(context.Background(), false)
	if got := tf.showCalls(); len(got) != 0 {
		t.Fatalf("background up-to-date/error shown: %v", got)
	}
	tf.check(context.Background(), true)
	if v := tf.m.snapshot(); v.phase != phaseError || v.errText == "" {
		t.Fatalf("manual error: %+v", v)
	}
	s.publish("1.2.0")
	tf.check(context.Background(), true)
	if v := tf.m.snapshot(); v.phase != phaseUpToDate {
		t.Fatalf("manual up to date: %+v", v)
	}
	if got := tf.showCalls(); len(got) != 2 || !got[0] || !got[1] {
		t.Fatalf("manual shows %v", got)
	}
}

func TestNoConcurrentCheck(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	hold := make(chan struct{})
	s.mu.Lock()
	s.hold = hold
	s.mu.Unlock()
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	done := make(chan struct{})
	go func() { tf.check(context.Background(), false); close(done) }()
	waitFor(t, "the first check to reach the server", func() bool { return s.hits.Load() == 1 })
	tf.check(context.Background(), true) // must not start a second Check
	if got := tf.showCalls(); len(got) != 1 || !got[0] {
		t.Fatalf("manual check during a check: shows %v", got)
	}
	close(hold)
	<-done
	if n := s.hits.Load(); n != 1 {
		t.Fatalf("manifest requests %d, want 1", n)
	}
}

func TestDownloadReady(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	tf.download()
	waitFor(t, "the download", func() bool { return tf.m.snapshot().phase == phaseReady })
	if tf.isBusy() {
		t.Fatal("still busy after the download")
	}
	tf.flow.mu.Lock()
	p := tf.payload
	tf.flow.mu.Unlock()
	if p == nil || p.Release.Version != "1.3.0" {
		t.Fatalf("payload %+v", p)
	}
}

func TestCancelDownload(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	s.mu.Lock()
	s.hold = make(chan struct{})
	s.mu.Unlock()
	tf.download()
	waitFor(t, "downloading", func() bool { return tf.m.snapshot().phase == phaseDownloading })
	tf.cancelDownload()
	waitFor(t, "the cancel", func() bool { return !tf.isBusy() })
	if v := tf.m.snapshot(); v.phase != phaseAvailable {
		t.Fatalf("after cancel: %+v", v)
	}
}

func TestBackgroundCheckKeepsReadyScreen(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	tf.download()
	waitFor(t, "the download", func() bool { return tf.m.snapshot().phase == phaseReady })
	tf.check(context.Background(), false)
	if v := tf.m.snapshot(); v.phase != phaseReady {
		t.Fatalf("background check replaced the ready screen: %+v", v)
	}
}

func TestLastCheckPersisted(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.2.0")
	u := s.updater(t, "1.2.0")
	newTestFlow(t, u).check(context.Background(), false)
	if loadState(u).LastCheck.IsZero() {
		t.Fatal("last check not saved")
	}
}

func TestRestartNow(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	tf.check(context.Background(), false)
	tf.download()
	waitFor(t, "the download", func() bool { return tf.m.snapshot().phase == phaseReady })
	tf.installNextLaunch()
	if loadState(u).Pending == nil {
		t.Fatal("install on next launch not saved")
	}

	defer func(f func(*updater.Updater, *updater.Payload) error) { apply = f }(apply)
	apply = func(*updater.Updater, *updater.Payload) error { return updater.ErrUnsupported }
	if err := tf.restartNow(); err == nil {
		t.Fatal("unsupported install reported success")
	}
	if v := tf.m.snapshot(); v.phase != phaseError || !v.unsupported || tf.quits != 0 {
		t.Fatalf("unsupported: %+v quits %d", v, tf.quits)
	}
	if loadState(u).Pending == nil {
		t.Fatal("failed install dropped the kept update")
	}

	apply = func(*updater.Updater, *updater.Payload) error { return nil }
	if err := tf.restartNow(); err != nil {
		t.Fatal(err)
	}
	if tf.quits != 1 || loadState(u).Pending != nil {
		t.Fatalf("installed: quits %d, pending %+v", tf.quits, loadState(u).Pending)
	}
}

func TestScheduleRespectsLastCheck(t *testing.T) {
	defer func(f func(time.Duration) <-chan time.Time) { after = f }(after)
	waits := make(chan time.Duration, 4)
	after = func(d time.Duration) <-chan time.Time { waits <- d; return make(chan time.Time) }
	first := func(last time.Time, interval time.Duration) (time.Duration, bool) {
		u := &updater.Updater{StateDir: t.TempDir(), Version: "1.0.0"}
		saveState(u, persisted{LastCheck: last})
		f := newFlow(u, &model{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.schedule(ctx, interval, 30*time.Second)
		select {
		case d := <-waits:
			return d, true
		case <-time.After(200 * time.Millisecond):
			return 0, false
		}
	}
	if d, ok := first(time.Now().Add(-time.Hour), 24*time.Hour); !ok || d < 22*time.Hour || d > 24*time.Hour {
		t.Fatalf("checked an hour ago: first wait %v %v, want ~23h", d, ok)
	}
	if d, ok := first(time.Time{}, 24*time.Hour); !ok || d != 30*time.Second {
		t.Fatalf("never checked: first wait %v %v, want 30s", d, ok)
	}
	if d, ok := first(time.Now().Add(-48*time.Hour), 24*time.Hour); !ok || d != 30*time.Second {
		t.Fatalf("overdue: first wait %v %v, want 30s", d, ok)
	}
	if _, ok := first(time.Time{}, -1); ok {
		t.Fatal("negative interval scheduled a check")
	}
}

func (tf *testFlow) quitCount() int {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	return tf.quits
}

// readyFlow is a flow with a downloaded release (version 1.3.0).
func readyFlow(t *testing.T) (*releaseServer, *updater.Updater, *testFlow) {
	t.Helper()
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	tf.check(context.Background(), false)
	tf.download()
	waitFor(t, "the download", func() bool { return tf.m.snapshot().phase == phaseReady })
	return s, u, tf
}

// Final review C1: installing claims the Updater like a check or a
// download: a second Restart Now, a check, or stop must not run beside it.
func TestRestartNowSerialized(t *testing.T) {
	s, _, tf := readyFlow(t)
	defer func(f func(*updater.Updater, *updater.Payload) error) { apply = f }(apply)
	release := make(chan struct{})
	var applies atomic.Int32
	apply = func(*updater.Updater, *updater.Payload) error {
		applies.Add(1)
		<-release
		return nil
	}
	first := make(chan error, 1)
	go func() { first <- tf.restartNow() }()
	waitFor(t, "the install to start", func() bool { return applies.Load() == 1 })
	if v := tf.m.snapshot(); v.phase != phaseInstalling {
		t.Fatalf("while installing: %+v", v)
	}
	if err := tf.restartNow(); err == nil {
		t.Fatal("a second Restart Now ran beside the first")
	}
	hits := s.hits.Load()
	tf.check(context.Background(), true)
	if s.hits.Load() != hits {
		t.Fatal("a check ran during the install")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if applies.Load() != 1 || tf.quitCount() != 1 {
		t.Fatalf("applies %d quits %d", applies.Load(), tf.quitCount())
	}
}

// Final review I1: once halted, nothing starts, and a check cancelled by
// the stop does not report "context canceled" or show the window again.
func TestHaltStopsChecks(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	s.mu.Lock()
	s.hold = hold
	s.mu.Unlock()
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { tf.check(ctx, true); close(done) }()
	waitFor(t, "the check to reach the server", func() bool { return s.hits.Load() == 1 })
	cancel()
	tf.halt()
	<-done
	if v := tf.m.snapshot(); v.phase == phaseError {
		t.Fatalf("a cancelled check reported an error: %+v", v)
	}
	if !loadState(u).LastCheck.IsZero() {
		t.Fatal("a cancelled check counted as a check")
	}
	tf.check(context.Background(), true)
	tf.check(context.Background(), false)
	tf.download()
	if n := s.hits.Load(); n != 1 {
		t.Fatalf("checks after halt: %d manifest requests", n)
	}
	if n := len(tf.showCalls()); n != 1 {
		t.Fatalf("window asked for %d times (want once, when the first check began)", n)
	}
}

// downloadsOf is u's download directory (Updater.StateDir is set in tests).
func downloadsOf(u *updater.Updater) string { return filepath.Join(u.StateDir, downloadsDir) }

// Final review I4: the release is downloaded into the state directory
// (kept across reboots, unlike $TMPDIR, for "Install on Next Launch"),
// and a cancelled or failed download leaves nothing behind.
func TestDownloadFilesKeptOrRemoved(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	u := s.updater(t, "1.2.0")
	tf := newTestFlow(t, u)
	tf.check(context.Background(), false)

	s.mu.Lock()
	s.hold = make(chan struct{})
	s.mu.Unlock()
	tf.download()
	waitFor(t, "the download directory", func() bool {
		_, err := os.Stat(filepath.Join(downloadsOf(u), "1.3.0"))
		return err == nil
	})
	tf.cancelDownload()
	waitFor(t, "the cancel", func() bool { return !tf.isBusy() })
	if _, err := os.Stat(filepath.Join(downloadsOf(u), "1.3.0")); !os.IsNotExist(err) {
		t.Fatalf("cancelled download left files: %v", err)
	}

	s.mu.Lock()
	s.hold = nil
	s.file = []byte("not what the manifest signed")
	s.mu.Unlock()
	tf.download()
	waitFor(t, "the failure", func() bool { return tf.m.snapshot().phase == phaseError })
	if _, err := os.Stat(filepath.Join(downloadsOf(u), "1.3.0")); !os.IsNotExist(err) {
		t.Fatalf("failed download left files: %v", err)
	}

	tf.m.set(func(m *model) { m.phase = phaseAvailable })
	s.mu.Lock()
	s.file = []byte("new release bytes")
	s.mu.Unlock()
	tf.download()
	waitFor(t, "the download", func() bool { return tf.m.snapshot().phase == phaseReady })
	tf.flow.mu.Lock()
	p := tf.payload
	tf.flow.mu.Unlock()
	if !strings.HasPrefix(p.Path, downloadsOf(u)+string(filepath.Separator)) {
		t.Fatalf("downloaded to %s, not under %s", p.Path, downloadsOf(u))
	}
}

// Final review I4: a new session removes downloads nothing refers to any
// more, and keeps the one kept for "Install on Next Launch".
func TestPruneDownloads(t *testing.T) {
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	stale := filepath.Join(downloadsOf(u), "1.1.0", "old.AppImage")
	kept := filepath.Join(downloadsOf(u), "1.3.0", "new.AppImage")
	for _, f := range []string{stale, kept} {
		os.MkdirAll(filepath.Dir(f), 0o700)
		os.WriteFile(f, []byte("x"), 0o600)
	}
	pruneDownloads(u, kept)
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Fatalf("stale download kept: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("kept download removed: %v", err)
	}
	pruneDownloads(u, "")
	if _, err := os.Stat(filepath.Dir(kept)); !os.IsNotExist(err) {
		t.Fatalf("unreferenced download kept: %v", err)
	}
}

// A download that fails (here: the file does not match the signed
// digest) shows the error; only a cancelled one goes back to the offer.
func TestDownloadFailureShowsError(t *testing.T) {
	s := newReleaseServer(t)
	s.publish("1.3.0")
	tf := newTestFlow(t, s.updater(t, "1.2.0"))
	tf.check(context.Background(), false)
	s.mu.Lock()
	s.file = []byte("not what the manifest signed")
	s.mu.Unlock()
	tf.download()
	waitFor(t, "the failure", func() bool { return !tf.isBusy() && tf.m.snapshot().phase != phaseDownloading })
	if v := tf.m.snapshot(); v.phase != phaseError || v.errText == "" {
		t.Fatalf("failed download: %+v", v)
	}
}

// Final review I5: "Check for Updates…" with a release downloaded keeps
// offering the restart instead of offering the same release for download
// again; a newer release than the downloaded one is offered.
func TestManualCheckKeepsDownloaded(t *testing.T) {
	s, _, tf := readyFlow(t)
	tf.check(context.Background(), true)
	if v := tf.m.snapshot(); v.phase != phaseReady || v.rel == nil || v.rel.Version != "1.3.0" {
		t.Fatalf("manual check with 1.3.0 downloaded: %+v", v)
	}
	s.publish("1.4.0")
	tf.check(context.Background(), true)
	if v := tf.m.snapshot(); v.phase != phaseAvailable || v.rel == nil || v.rel.Version != "1.4.0" {
		t.Fatalf("manual check with 1.4.0 out: %+v", v)
	}
}

// Final review M3: a check saving its LastCheck must not overwrite the
// state "Install on Next Launch" saved meanwhile (each save writes the
// state as it is when written, under the flow's lock).
func TestSavesDoNotLoseInstallNextLaunch(t *testing.T) {
	_, u, tf := readyFlow(t)
	defer func(f func(*updater.Updater, persisted) error) { save = f }(save)
	checkSaving := make(chan struct{})
	var once sync.Once
	save = func(u *updater.Updater, p persisted) error {
		if p.Pending == nil { // the check's save
			once.Do(func() { close(checkSaving) })
			time.Sleep(50 * time.Millisecond) // installNextLaunch runs now
		}
		return saveState(u, p)
	}
	done := make(chan struct{})
	go func() { tf.check(context.Background(), true); close(done) }()
	<-checkSaving
	tf.installNextLaunch()
	<-done
	if loadState(u).Pending == nil {
		t.Fatal("the check's save lost Install on Next Launch")
	}
}

// Review: a manual check that fails with a release downloaded keeps
// offering the restart (offline is no reason to lose it).
func TestManualCheckErrorKeepsDownloaded(t *testing.T) {
	s, _, tf := readyFlow(t)
	s.mu.Lock()
	s.status = http.StatusInternalServerError
	s.mu.Unlock()
	tf.check(context.Background(), true)
	if v := tf.m.snapshot(); v.phase != phaseReady || v.rel == nil || v.rel.Version != "1.3.0" {
		t.Fatalf("failed manual check with 1.3.0 downloaded: %+v", v)
	}
}

// Review: a version that would name the downloads directory itself, or
// its parent, is refused before anything is removed.
func TestDownloadDirRefusesDotVersions(t *testing.T) {
	s := newReleaseServer(t)
	u := s.updater(t, "1.2.0")
	for _, v := range []string{"", ".", ".."} {
		if dir, err := downloadDir(u, v); err == nil {
			t.Errorf("version %q: got %s", v, dir)
		}
	}
	if _, err := os.Stat(u.StateDir); err != nil {
		t.Fatalf("state directory: %v", err)
	}
}
