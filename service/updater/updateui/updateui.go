package updateui

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/service/updater"
)

// session is updateui's state for one *updater.Updater: its window model,
// its flow and its window. Start and CheckNow share it, so the Updater's
// OnEvent is wrapped once.
type session struct {
	app  *tango.App
	u    *updater.Updater
	opts Options
	m    *model
	f    *flow

	origOnEvent func(updater.Event)

	// ctx ends when the session stops: every check (background, CheckNow,
	// Retry) runs under it, so stop can end them.
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	started  bool
	stopFn   func()
	i18nStop func()

	win window // main thread only

	// stopping and done: stop has begun (set under sessionsMu) and has
	// finished (closed). The session stays in sessions until done, so a
	// new one never wraps u.OnEvent before the old stop restores it.
	stopping bool
	done     chan struct{}
}

var (
	sessionsMu sync.Mutex
	sessions   = map[*updater.Updater]*session{}
)

// sessionFor returns u's session, creating it (and wrapping u.OnEvent)
// the first time. While an earlier session of u is stopping it waits for
// that stop to finish.
func sessionFor(app *tango.App, u *updater.Updater, opts Options) *session {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	for {
		s := sessions[u]
		if s == nil {
			break
		}
		if !s.stopping {
			return s
		}
		sessionsMu.Unlock()
		<-s.done
		sessionsMu.Lock()
	}
	s := &session{app: app, u: u, opts: opts, m: &model{current: u.Version}, done: make(chan struct{})}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.f = newFlow(u, s.m)
	s.f.show = s.show
	s.f.quit = app.Quit
	s.origOnEvent = u.OnEvent
	orig, f := u.OnEvent, s.f
	u.OnEvent = func(e updater.Event) {
		if orig != nil {
			orig(e)
		}
		f.onEvent(e)
	}
	sessions[u] = s
	return s
}

// strings returns the window's texts for the current locale.
func (s *session) strings() *Strings { return stringsFor(s.opts) }

// Start shows the update window when an update is found: it confirms a
// newly installed version (Updater.Confirm, so it is kept), offers an
// update downloaded at an earlier launch, then checks in the background
// every Options.Interval. Call it once the app is running (App.OnReady or
// later). It returns an error, and changes nothing, when u lacks
// ManifestURL, Version or a public key. stop ends the background checks,
// closes the window and gives u back its own OnEvent; calling Start again
// for the same Updater returns the same stop. A Start while an earlier
// stop of the same Updater is still running waits for it to finish.
func Start(app *tango.App, u *updater.Updater, opts Options) (stop func(), err error) {
	switch {
	case app == nil || u == nil:
		return nil, errors.New("updateui: nil app or updater")
	case u.ManifestURL == "":
		return nil, errors.New("updateui: Updater.ManifestURL is empty")
	case u.Version == "":
		return nil, errors.New("updateui: Updater.Version is empty")
	case u.PublicKey == "" && len(u.PublicKeys) == 0:
		return nil, errors.New("updateui: Updater has no PublicKey")
	}
	s := sessionFor(app, u, opts)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return s.stopFn, nil
	}
	s.started = true
	s.opts = opts
	s.stopFn = sync.OnceFunc(s.stop)

	if out, err := u.Confirm(); err != nil {
		log.Printf("updateui: confirm: %v", err)
	} else if out.Status == updater.StatusRolledBack {
		s.m.set(func(m *model) { m.phase = phaseRolledBack })
		s.show(false)
	}

	f := s.f
	f.mu.Lock()
	hadPending := f.st.Pending != nil
	pp := f.st.usablePending(u.Version)
	if hadPending && pp == nil {
		f.saveLocked() // the kept update is gone or stale: forget it
	}
	if pp != nil {
		f.payload = pp.payload()
	}
	if !f.busy { // a CheckNow before Start may be downloading
		keep := ""
		if f.payload != nil {
			keep = f.payload.Path
		}
		pruneDownloads(u, keep)
	}
	f.mu.Unlock()
	if pp != nil {
		rel := f.payload.Release
		s.m.set(func(m *model) { m.phase, m.rel = phaseDownloaded, rel })
		s.show(false)
	}

	f.schedule(s.ctx, opts.interval(), opts.firstDelay())
	s.i18nStop = i18n.OnChange(func(string) { s.m.set(func(*model) {}) })
	return s.stopFn, nil
}

// CheckNow checks for an update right away and shows the result in the
// window, including "up to date" and errors (for a "Check for Updates…"
// menu item). While a check or download runs it just shows the window.
// It returns at once; the check runs on its own goroutine.
func CheckNow(app *tango.App, u *updater.Updater, opts Options) {
	if app == nil || u == nil {
		return
	}
	go func() {
		s := sessionFor(app, u, opts) // may wait for a stop in progress
		s.f.check(s.ctx, true)
	}()
}

// stop ends the session (run once, through stopFn). It cancels a running
// check or download and waits for it before giving u back its OnEvent:
// the Updater reads OnEvent while it works and is not safe for
// concurrent use.
func (s *session) stop() {
	sessionsMu.Lock()
	s.stopping = true // a Start or CheckNow from now on waits for done
	sessionsMu.Unlock()
	s.cancel()
	s.mu.Lock()
	if s.i18nStop != nil {
		s.i18nStop()
	}
	s.mu.Unlock()
	s.f.halt()
	s.u.OnEvent = s.origOnEvent
	s.app.Post(s.closeWindow)
	sessionsMu.Lock()
	if sessions[s.u] == s {
		delete(sessions, s.u) // a later Start or CheckNow starts afresh
	}
	sessionsMu.Unlock()
	close(s.done)
}
