package updateui

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// Seams replaced in tests.
var (
	apply = (*updater.Updater).Apply
	after = time.After
)

// flow drives one *updater.Updater for the window: checks (background or
// asked for), the download, and installing. The Updater is not safe for
// concurrent use, so at most one check or download runs at a time (busy).
type flow struct {
	u *updater.Updater
	m *model

	busy        atomic.Bool
	downloading atomic.Bool // OnEvent download events go to the model

	mu           sync.Mutex
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
	if !f.busy.CompareAndSwap(false, true) {
		if manual {
			f.show(true)
		}
		return
	}
	defer f.busy.Store(false)
	if manual {
		f.m.set(func(m *model) { m.phase, m.manual, m.err, m.unsupported = phaseChecking, true, nil, false })
		f.show(true)
	}
	rel, err := f.u.Check(ctx)

	f.mu.Lock()
	f.st.LastCheck = time.Now()
	st := f.st
	skipped := rel != nil && (rel.Version == f.st.Skipped || rel.Version == f.laterThisRun)
	f.mu.Unlock()
	if err := saveState(f.u, st); err != nil {
		log.Printf("updateui: %v", err)
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
	if rel == nil || !f.busy.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.mu.Lock()
	f.cancel = cancel
	f.mu.Unlock()
	f.downloading.Store(true)
	f.m.set(func(m *model) { m.phase, m.done, m.total, m.err = phaseDownloading, 0, -1, nil })
	go func() {
		defer f.busy.Store(false)
		p, err := f.u.Download(ctx, rel, "")
		f.downloading.Store(false)
		f.mu.Lock()
		f.cancel = nil
		if err == nil {
			f.payload = p
		}
		f.mu.Unlock()
		cancel()
		switch {
		case err == nil:
			f.m.set(func(m *model) { m.phase, m.patch = phaseReady, p.Patched })
		case ctx.Err() != nil:
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
	st := f.st
	f.mu.Unlock()
	if err := saveState(f.u, st); err != nil {
		log.Printf("updateui: %v", err)
	}
}

// later puts the release on offer off for this run ("Remind Me Later").
func (f *flow) later() {
	if rel := f.m.snapshot().rel; rel != nil {
		f.mu.Lock()
		f.laterThisRun = rel.Version
		f.mu.Unlock()
	}
}

// restartNow installs the downloaded release and quits the app.
func (f *flow) restartNow() error {
	f.mu.Lock()
	p := f.payload
	f.mu.Unlock()
	if p == nil {
		return errors.New("updateui: nothing downloaded")
	}
	if err := apply(f.u, p); err != nil {
		f.m.set(func(m *model) {
			m.phase, m.err, m.unsupported = phaseError, err, errors.Is(err, updater.ErrUnsupported)
		})
		return err
	}
	f.mu.Lock()
	f.st.Pending = nil
	st := f.st
	f.mu.Unlock()
	if err := saveState(f.u, st); err != nil {
		log.Printf("updateui: %v", err)
	}
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
	st := f.st
	f.mu.Unlock()
	if err := saveState(f.u, st); err != nil {
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
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-after(wait):
			}
			f.check(ctx, false)
			wait = interval
		}
	}()
}
