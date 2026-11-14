package updateui

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/service/updater"
)

// phase is what the update window shows.
type phase int

const (
	phaseIdle        phase = iota
	phaseChecking          // a check is running (shown only when asked)
	phaseAvailable         // a newer release: skip, later, install
	phaseDownloading       // progress, cancel
	phaseReady             // downloaded and verified: restart now or next launch
	phaseDownloaded        // at launch: an update downloaded earlier
	phaseError             // a step failed: retry, close
	phaseUpToDate          // no newer release (shown only when asked)
	phaseRolledBack        // the last update did not start and was undone
)

// model is the window's state. The flow and the updater's OnEvent write
// it from their goroutines; the view reads snapshots on the UI thread.
type model struct {
	mu          sync.Mutex
	phase       phase
	current     string           // the running version
	rel         *updater.Release // the release on offer
	patch       bool             // the download is a patch
	done, total int64            // download progress (total -1: unknown)
	err         error
	unsupported bool // the installation cannot update itself
	manual      bool // the window was asked for (Check for Updates…)

	// onChange is called after every change, outside the lock (the window
	// sets it to Runner.Invalidate).
	onChange func()
}

// view is a copy of the model for one frame.
type view struct {
	phase       phase
	current     string
	rel         *updater.Release
	patch       bool
	done, total int64
	errText     string
	unsupported bool
	manual      bool
}

func (m *model) snapshot() view {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := view{phase: m.phase, current: m.current, rel: m.rel, patch: m.patch,
		done: m.done, total: m.total, unsupported: m.unsupported, manual: m.manual}
	if m.err != nil {
		v.errText = m.err.Error()
	}
	return v
}

// set changes the model under its lock, then reports the change.
func (m *model) set(fn func(*model)) {
	m.mu.Lock()
	fn(m)
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// applyEvent follows an updater event (Updater.OnEvent).
func (m *model) applyEvent(e updater.Event) {
	m.set(func(m *model) {
		switch e.Kind {
		case updater.EventChecking:
			m.phase = phaseChecking
		case updater.EventAvailable:
			m.phase, m.rel = phaseAvailable, e.Release
		case updater.EventUpToDate:
			m.phase = phaseUpToDate
		case updater.EventDownloading:
			m.phase, m.patch, m.done, m.total = phaseDownloading, e.Patch, 0, -1
		case updater.EventProgress:
			if m.phase == phaseDownloading {
				m.done, m.total = e.Done, e.Total
			}
		case updater.EventPatchFailed:
			// The full file follows; not an error for the user.
			m.patch = false
		case updater.EventVerified:
			m.phase = phaseReady
		case updater.EventError:
			m.phase, m.err = phaseError, errors.New(e.Error)
		case updater.EventReady:
			// Apply staged it; the caller quits the app.
		}
	})
}

// progress is the download's fraction in [0, 1], or -1 when the size is
// unknown.
func (v view) progress() float64 {
	if v.total <= 0 {
		return -1
	}
	p := float64(v.done) / float64(v.total)
	return min(max(p, 0), 1)
}
