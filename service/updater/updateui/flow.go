package updateui

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// Seams replaced in tests.
var (
	apply = (*updater.Updater).Apply
	after = time.After
	save  = saveState
)

// flow drives one *updater.Updater for the window: checks (background or
// asked for), the download, and installing. The Updater is not safe for
// concurrent use, so at most one of those runs at a time: each claims the
// flow with begin and releases it with end.
type flow struct {
	u *updater.Updater
	m *model

	downloading atomic.Bool    // OnEvent download events go to the model
	running     sync.WaitGroup // the claimed operation (halt waits for it)

	mu           sync.Mutex
	busy         bool // an operation holds the Updater
	stopped      bool // halt ran: nothing starts any more
	st           persisted
	laterThisRun string           // "Remind Me Later" for this run
	payload      *updater.Payload // downloaded and verified
	cancel       context.CancelFunc

	// show asks for the window (opened or brought forward); quit quits
	// the app after a successful Apply. Set before use.
	show func(manual bool)
	quit func()
}

func newFlow(u *updater.Updater, m *model) *flow {
	return &flow{u: u, m: m, st: loadState(u)}
}

// begin claims the Updater for one operation; false while another runs or
// after halt. running is added under the same lock halt sets stopped
// under, so halt's Wait never races an Add.
func (f *flow) begin() bool { return f.beginWith(nil) }

// beginWith is begin running claimed (when non-nil) under the same lock
// once the claim is taken: a download sets its cancel there, so a halt
// that sees the claim also sees the cancel.
func (f *flow) beginWith(claimed func()) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy || f.stopped {
		return false
	}
	f.busy = true
	f.running.Add(1)
	if claimed != nil {
		claimed()
	}
	return true
}

// end releases the claim taken by begin.
func (f *flow) end() {
	f.mu.Lock()
	f.busy = false
	f.mu.Unlock()
	f.running.Done()
}

// isBusy reports a claimed operation.
func (f *flow) isBusy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy
}

// isStopped reports whether halt ran.
func (f *flow) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

// halt stops the flow for good: nothing new starts, a running download is
// cancelled, and it returns once the claimed operation has ended. (A check
// ends through its context, which the session cancels first.)
func (f *flow) halt() {
	f.mu.Lock()
	f.stopped = true
	c := f.cancel
	f.mu.Unlock()
	if c != nil {
		c()
	}
	f.running.Wait()
}

// onEvent takes the updater's download events while a download runs; the
// flow sets every other phase itself, so a background check never
// replaces what the window shows.
func (f *flow) onEvent(e updater.Event) {
	if !f.downloading.Load() {
		return
	}
	switch e.Kind {
	case updater.EventDownloading, updater.EventProgress, updater.EventPatchFailed:
		f.m.applyEvent(e)
	}
}

// check looks for a newer release. In the background (manual false) it
// shows the window only for a release that is neither skipped nor put off
// for this run, and stays silent when up to date or on errors; asked for,
// it shows every outcome. A check or download already running is not
// repeated: a manual check then just shows the window.
func (f *flow) check(ctx context.Context, manual bool) {
	if !manual {
		f.mu.Lock()
		ready := f.payload != nil
		f.mu.Unlock()
		if ready {
			return // an update is downloaded: keep offering it
		}
	}
	if !f.begin() {
		if manual && !f.isStopped() {
			f.show(true)
		}
		return
	}
	defer f.end()
	if ctx.Err() != nil {
		return // the session stopped
	}
	if manual {
		f.m.set(func(m *model) { m.phase, m.manual, m.err, m.unsupported = phaseChecking, true, nil, false })
		f.show(true)
	}
	rel, err := f.u.Check(ctx)
	if ctx.Err() != nil {
		return // the session stopped: not a check, nothing to show
	}

	f.mu.Lock()
	f.st.LastCheck = time.Now()
	f.saveLocked()
	skipped := rel != nil && (rel.Version == f.st.Skipped || rel.Version == f.laterThisRun)
	kept := f.payload
	f.mu.Unlock()
	if kept != nil && kept.Release != nil && (err != nil || rel == nil || !newer(rel.Version, kept.Release.Version)) {
		// Nothing newer than the release already downloaded, or the check
		// failed (only a manual check gets here): keep offering the
		// restart.
		f.m.set(func(m *model) { m.phase, m.rel, m.err, m.unsupported = phaseReady, kept.Release, nil, false })
		return
	}

	switch {
	case err != nil:
		if !manual {
			log.Printf("updateui: background check: %v", err)
			return
		}
		f.m.set(func(m *model) {
			m.phase, m.err, m.unsupported = phaseError, err, errors.Is(err, updater.ErrUnsupported)
		})
	case rel == nil:
		if !manual {
			return
		}
		f.m.set(func(m *model) { m.phase = phaseUpToDate })
	default:
		if skipped && !manual {
			return
		}
		f.m.set(func(m *model) {
			m.phase, m.rel, m.manual, m.err, m.unsupported = phaseAvailable, rel, manual, nil, false
		})
		if !manual {
			f.show(false) // a manual check opened the window when it started
		}
	}
}

// download fetches the release on offer on a new goroutine: phaseReady
// when it is verified, phaseAvailable again when cancelled, phaseError
// otherwise.
func (f *flow) download() {
	rel := f.m.snapshot().rel
	if rel == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	if !f.beginWith(func() {
		f.cancel = cancel
		f.payload = nil // its files may be the ones downloadDir empties
	}) {
		cancel()
		return
	}
	f.downloading.Store(true)
	f.m.set(func(m *model) { m.phase, m.done, m.total, m.err = phaseDownloading, 0, -1, nil })
	go func() {
		defer f.end()
		dir, err := downloadDir(f.u, rel.Version)
		if err != nil {
			log.Printf("updateui: %v; downloading to a temporary directory", err)
			dir = "" // Download makes one
		}
		p, err := f.u.Download(ctx, rel, dir)
		if err != nil && dir != "" {
			os.RemoveAll(dir) // cancelled or failed: nothing to keep
		}
		f.downloading.Store(false)
		f.mu.Lock()
		f.cancel = nil
		if err == nil {
			f.payload = p
		}
		f.mu.Unlock()
		cancelled := ctx.Err() != nil // read before cancel() below sets it
		cancel()
		switch {
		case err == nil:
			f.m.set(func(m *model) { m.phase, m.patch = phaseReady, p.Patched })
		case cancelled:
			f.m.set(func(m *model) { m.phase = phaseAvailable })
		default:
			f.m.set(func(m *model) { m.phase, m.err = phaseError, err })
		}
	}()
}

// cancelDownload stops a running download.
func (f *flow) cancelDownload() {
	f.mu.Lock()
	c := f.cancel
	f.mu.Unlock()
	if c != nil {
		c()
	}
}

// skip remembers the release on offer as skipped ("Skip This Version").
func (f *flow) skip() {
	rel := f.m.snapshot().rel
	if rel == nil {
		return
	}
	f.mu.Lock()
	f.st.Skipped = rel.Version
	f.saveLocked()
	f.mu.Unlock()
}

// later puts the release on offer off for this run ("Remind Me Later").
func (f *flow) later() {
	if rel := f.m.snapshot().rel; rel != nil {
		f.mu.Lock()
		f.laterThisRun = rel.Version
		f.mu.Unlock()
	}
}

// errBusy is restartNow's answer while another operation holds the
// Updater (a second click, a check) or after halt.
var errBusy = errors.New("updateui: busy")

// restartNow installs the downloaded release and quits the app. It claims
// the Updater like a check or a download, and shows phaseInstalling (no
// buttons) while Apply stages the release.
func (f *flow) restartNow() error {
	f.mu.Lock()
	p := f.payload
	f.mu.Unlock()
	if p == nil {
		return errors.New("updateui: nothing downloaded")
	}
	if !f.begin() {
		return errBusy
	}
	defer f.end()
	f.m.set(func(m *model) { m.phase = phaseInstalling })
	if err := apply(f.u, p); err != nil {
		f.m.set(func(m *model) {
			m.phase, m.err, m.unsupported = phaseError, err, errors.Is(err, updater.ErrUnsupported)
		})
		return err
	}
	f.mu.Lock()
	f.st.Pending = nil
	f.saveLocked()
	f.mu.Unlock()
	f.quit()
	return nil
}

// installNextLaunch keeps the download; the next Start offers it.
func (f *flow) installNextLaunch() {
	f.mu.Lock()
	if f.payload == nil {
		f.mu.Unlock()
		return
	}
	f.st.Pending = newPending(f.payload)
	f.saveLocked()
	f.mu.Unlock()
}

// saveLocked writes the state file; f.mu is held, so saves are written in
// the order the state changed and none overwrites a newer one.
func (f *flow) saveLocked() {
	if err := save(f.u, f.st); err != nil {
		log.Printf("updateui: %v", err)
	}
}

// schedule checks in the background every interval, the first time
// firstDelay after now or interval after the last check, whichever is
// later; interval < 0 schedules nothing. It stops when ctx ends.
func (f *flow) schedule(ctx context.Context, interval, firstDelay time.Duration) {
	if interval < 0 {
		return
	}
	f.mu.Lock()
	last := f.st.LastCheck
	f.mu.Unlock()
	wait := firstDelay
	if !last.IsZero() {
		if d := time.Until(last.Add(interval)); d > wait {
			wait = d
		}
	}
	timer := after // read here, not on the goroutine (tests replace it)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer(wait):
			}
			if ctx.Err() != nil {
				return // both were ready and select picked the timer
			}
			f.check(ctx, false)
			wait = interval
		}
	}()
}
