package tango

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// App-level events (deep links, opened files, theme and display changes)
// and quit interception.

// AppEvent is something that happened to the app as a whole.
type AppEvent = platform.AppEvent

// AppEventKind values.
const (
	EventOpenURL         = platform.AppOpenURL
	EventOpenFiles       = platform.AppOpenFiles
	EventThemeChanged    = platform.AppThemeChanged
	EventDisplaysChanged = platform.AppDisplaysChanged
	EventReopen          = platform.AppReopen
	// EventNotificationOpened: a notification sent by an earlier run of
	// the app was clicked (Notification holds its id); see notify.OnClick.
	EventNotificationOpened = platform.AppNotificationOpened
)

type appExt struct {
	mu      sync.Mutex
	onEvent []eventListener
	onQuit  []func() bool
	pending []AppEvent // events that arrived before anyone listened (e.g. launch-time deep link)

	// During launch (until the OnReady callbacks have run) pending events are
	// replayed to every listener that registers, not just the first one, so
	// a framework listener (jsapi) cannot swallow the launch deep link the
	// app's own OnOpenURL registers for a moment later.
	settleHooked bool
	settled      bool

	quitOnClose bool // last-window policy already quit the app; see lifecycle.go
}

// eventListener receives events of one kind, or all kinds when kind is 0.
type eventListener struct {
	kind platform.AppEventKind
	fn   func(AppEvent)
}

func (l eventListener) wants(k platform.AppEventKind) bool { return l.kind == 0 || l.kind == k }

// OnEvent registers fn for app-level events. Events that arrived before the
// first listener registered (a deep link that launched the app) are replayed
// to it on the main thread.
func (a *App) OnEvent(fn func(AppEvent)) { a.listen(0, fn) }

// OnEventKind registers fn for app-level events of one kind. Unlike an
// OnEvent listener, it leaves pending events of other kinds (a launch deep
// link, say) for the listeners that want them.
func (a *App) OnEventKind(kind AppEventKind, fn func(AppEvent)) {
	if kind == 0 {
		return
	}
	a.listen(kind, fn)
}

// AppEventKind identifies an AppEvent.
type AppEventKind = platform.AppEventKind

// listen registers fn for events of kind (0: all). Pending events fn wants
// are replayed to it; the others stay pending for a later listener.
func (a *App) listen(kind platform.AppEventKind, fn func(AppEvent)) {
	a.hookLaunchSettle()
	l := eventListener{kind, fn}
	a.ext.mu.Lock()
	a.ext.onEvent = append(a.ext.onEvent, l)
	var replay, keep []AppEvent
	for _, ev := range a.ext.pending {
		if l.wants(ev.Kind) {
			replay = append(replay, ev)
		} else {
			keep = append(keep, ev)
		}
	}
	if a.ext.settled {
		a.ext.pending = keep // after launch: first interested listener consumes
	}
	a.ext.mu.Unlock()
	if len(replay) > 0 {
		a.Post(func() {
			for _, ev := range replay {
				a.guard(whereEvent, func() { fn(ev) })
			}
		})
	}
}

// OnQuitRequest registers fn to be asked when the user quits the app
// (Cmd+Q, a Quit menu item). Returning false cancels the quit. App.Quit
// called from code is never intercepted.
func (a *App) OnQuitRequest(fn func() bool) {
	a.ext.mu.Lock()
	a.ext.onQuit = append(a.ext.onQuit, fn)
	a.ext.mu.Unlock()
}

// emitAppEvent is also used by core features (single instance forwarding).
func (a *App) emitAppEvent(ev AppEvent) {
	a.ext.mu.Lock()
	var fns []func(AppEvent)
	for _, l := range a.ext.onEvent {
		if l.wants(ev.Kind) {
			fns = append(fns, l.fn)
		}
	}
	// Only events that can arrive before user code registers (launch-time
	// deep links / files / reopen) are worth replaying; stale theme or display
	// changes would just accumulate.
	if len(fns) == 0 && replayable(ev.Kind) {
		a.ext.pending = append(a.ext.pending, ev)
	}
	a.ext.mu.Unlock()
	for _, fn := range fns {
		a.guard(whereEvent, func() { fn(ev) })
	}
}

func (h host) OnAppEvent(ev platform.AppEvent) {
	defer h.a.enterMain()()
	h.a.emitAppEvent(ev)
}

func (h host) ShouldQuit() bool {
	defer h.a.enterMain()()
	h.a.ext.mu.Lock()
	fns := slices.Clone(h.a.ext.onQuit)
	h.a.ext.mu.Unlock()
	for _, fn := range fns {
		// A panicking handler does not cancel the quit.
		if !guardCall(h.a, whereEvent, true, fn) {
			return false
		}
	}
	return true
}

var _ platform.HostEx = host{}

func replayable(k platform.AppEventKind) bool {
	switch k {
	case platform.AppOpenURL, platform.AppOpenFiles, platform.AppReopen,
		platform.AppPushOpened, platform.AppNotificationOpened:
		return true
	}
	return false
}

// hookLaunchSettle arranges for the launch window to close once the app has
// started and its OnReady callbacks have run; from then on pending events
// are consumed by the first interested listener.
func (a *App) hookLaunchSettle() {
	a.ext.mu.Lock()
	if a.ext.settleHooked {
		a.ext.mu.Unlock()
		return
	}
	a.ext.settleHooked = true
	a.ext.mu.Unlock()

	a.mu.Lock()
	started := a.started
	a.mu.Unlock()
	if started {
		a.settleLaunch()
		return
	}
	a.OnLifecycle(func(ev Lifecycle) {
		if ev == platform.LifecycleStarted {
			// OnReady callbacks run right after lifecycle listeners in the
			// same main-thread turn; Post lands after them.
			a.Post(a.settleLaunch)
		}
	})
}

func (a *App) settleLaunch() {
	a.ext.mu.Lock()
	defer a.ext.mu.Unlock()
	if a.ext.settled {
		return
	}
	a.ext.settled = true
	// Launch events every launch-time listener already saw are done; keep
	// only kinds nobody listens to yet, for the first later listener.
	var keep []AppEvent
	for _, ev := range a.ext.pending {
		heard := false
		for _, l := range a.ext.onEvent {
			if l.wants(ev.Kind) {
				heard = true
				break
			}
		}
		if !heard {
			keep = append(keep, ev)
		}
	}
	a.ext.pending = keep
}
