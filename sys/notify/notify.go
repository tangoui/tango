// Package notify shows system notifications (Notification Center on macOS,
// the notification area on Windows, the freedesktop notification service on
// Linux) and reports clicks on them.
//
//	if ok, err := notify.RequestPermission(a); ok && err == nil {
//		id, err := notify.Send(a, notify.Message{Title: "Build done", Body: "3 warnings"})
//		...
//	}
//	notify.OnClick(a, func(id string) { win.Show(); win.Focus() })
//
// Functions are safe from any goroutine once the app runs. They return
// ErrUnsupported when the backend has no notifications and
// tango.ErrNotRunning after the app quit.
//
// Platform notes: on macOS notifications need the app to run from a .app
// bundle with a bundle identifier (a bare `go run` binary gets a clear
// error, not a crash), and the user must allow them. On Windows they are
// WinRT toast notifications: the app's ID (else Name) becomes the
// process's AppUserModelID and is registered under HKCU so an app without
// a Start-menu shortcut can show toasts; a click on a toast still in the
// Action Center calls OnClick while the app runs, but cannot launch a
// closed app (that needs a shortcut or package with the same AUMID and a
// COM activator). Where toasts are unavailable or turned off — or with
// TANGO_WINDOWS_TOAST=0 — they fall back to notification-area balloons.
// On Linux they go to
// org.freedesktop.Notifications over D-Bus. On iOS they are local
// UNUserNotificationCenter notifications, shown as banners also while the
// app is in the foreground; Send asks for permission the first time. On
// Android they go to a "Notifications" channel (a "Silent notifications"
// one for Silent); on Android 13+ the app must declare "notifications" in
// tan.json (POST_NOTIFICATIONS) and Send asks the first time. On both,
// a tap opens the app and calls OnClick; a tap on a notification of an
// earlier run (including the one that launched the app) arrives through
// OnClick too.
package notify

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// Message is the content of a notification.
type Message struct {
	Title    string
	Subtitle string // shown under the title on macOS; merged into the body elsewhere
	Body     string
	Silent   bool // no sound
}

// appState holds the click handlers of one App.
type appState struct {
	a        *tango.App
	mu       sync.Mutex
	handlers []func(id string)
	hooked   bool // listening for EventNotificationOpened
}

var (
	states sync.Map // *tango.App -> *appState
	seq    atomic.Uint64
	// prefix keeps ids unique across runs of the app, since macOS keeps
	// delivered notifications (and their ids) after the app quits.
	prefix = fmt.Sprintf("tango-%x", time.Now().UnixNano())
)

func state(a *tango.App) *appState {
	v, _ := states.LoadOrStore(a, &appState{a: a})
	return v.(*appState)
}

func notifier(a *tango.App) (platform.Notifier, error) {
	n, ok := a.Backend().(platform.Notifier)
	if !ok {
		return nil, ErrUnsupported
	}
	return n, nil
}

type permResult struct {
	granted bool
	err     error
}

// RequestPermission asks the user, if the platform requires it and they
// were not asked before, to allow notifications. It reports whether
// notifications may be shown. It blocks until the user answers; called on
// the main thread it keeps the event loop running meanwhile.
func RequestPermission(a *tango.App) (bool, error) {
	n, err := notifier(a)
	if err != nil {
		return false, err
	}
	r, err := tango.TryAwaitAsync(a, func(done func(permResult)) {
		n.RequestNotifyPermission(func(g bool, err error) { done(permResult{g, err}) })
	})
	if err != nil {
		return false, err
	}
	return r.granted, r.err
}

type sendResult struct {
	err error
	ran bool
}

// Send shows a notification and returns its id, which OnClick handlers
// receive when the user clicks it.
func Send(a *tango.App, m Message) (string, error) {
	n, err := notifier(a)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("%s-%d", prefix, seq.Add(1))
	st := state(a)
	pn := platform.Notification{ID: id, Title: m.Title, Subtitle: m.Subtitle, Body: m.Body, Silent: m.Silent}
	r := tango.Await(a, func() sendResult {
		return sendResult{n.Notify(pn, func() { st.clicked(id) }), true}
	})
	if !r.ran {
		return "", tango.ErrNotRunning
	}
	if r.err != nil {
		return "", r.err
	}
	return id, nil
}

// OnClick registers fn to run on the main thread when the user clicks a
// notification this app sent. That includes, where the platform reports
// them (macOS), notifications sent by an earlier run of the app — the
// click that launched it among them: register before Run or in OnReady
// so that one is not missed. Ids stay unique across runs.
func OnClick(a *tango.App, fn func(id string)) {
	st := state(a)
	st.mu.Lock()
	st.handlers = append(st.handlers, fn)
	hook := !st.hooked
	st.hooked = true
	st.mu.Unlock()
	if hook {
		a.OnEventKind(tango.EventNotificationOpened, func(ev tango.AppEvent) {
			st.clicked(ev.Notification)
		})
	}
}

func (st *appState) clicked(id string) {
	st.mu.Lock()
	fns := slices.Clone(st.handlers)
	st.mu.Unlock()
	// Called by the backend's click callback directly (Send) or from an
	// app event: guard each handler on its own so a panic in one does not
	// skip the rest.
	for _, fn := range fns {
		corehook.Guard(st.a, "event handler", func() { fn(id) })
	}
}
