// Package push registers the app for remote (push) notifications and
// reports notifications that arrive or that the user taps.
//
//	push.OnOpen(a, func(m push.Message) { route(m.Data) })   // before Run or in OnReady
//	token, err := push.Register(a)                          // send token to your server
//
// iOS: APNs. Register asks for alert/sound/badge permission if the user
// was never asked, then returns the device token as lowercase hex. The app
// needs the aps-environment entitlement (a provisioning profile with Push
// Notifications); without it Register fails with the system's error. The
// simulator issues tokens too (iOS 16+ on Apple silicon) and accepts
// simulated pushes: xcrun simctl push booted <bundle-id> payload.apns.
//
// Android: Firebase Cloud Messaging needs Google services, which Tango
// does not bundle. Register returns ErrUnsupported until the app wires FCM
// in (add the Firebase dependency and a FirebaseMessagingService that calls
// org.tango.host.TangoPush.token / .received / .opened — see
// docs/mobile-permissions.md); after that it returns the FCM token and the
// events below work.
//
// Notifications arriving while the app runs in the foreground are shown as
// banners on iOS and reported with OnReceive. Taps — also the one that
// launched the app — are reported with OnOpen; register handlers before
// Run or in OnReady so the launch tap is not missed. Handlers run on the
// main thread.
package push

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported      = platform.ErrUnsupported
	ErrPermissionDenied = platform.ErrPermissionDenied
)

// Message is a remote notification.
type Message struct {
	ID     string // the notification identifier, when the platform gives one
	Title  string
	Body   string
	Action string         // OnOpen: action identifier of a notification action button, "" for a plain tap
	Data   map[string]any // the whole payload: APNs userInfo (with "aps") on iOS, FCM data on Android
}

// Register registers with the push service and returns the token.
func Register(a *tango.App) (string, error) {
	p, err := capcall.Backend[platform.Push](a)
	if err != nil {
		return "", err
	}
	return capcall.Async(a, func(done func(string, error)) { p.RegisterPush(done) })
}

// OnToken calls fn with the token whenever the platform issues a new one
// (after Register, and when FCM rotates it).
func OnToken(a *tango.App, fn func(token string)) {
	a.OnEventKind(tango.EventPushToken, func(ev tango.AppEvent) { fn(ev.Push.Token) })
}

// OnReceive calls fn for notifications that arrive while the app runs.
func OnReceive(a *tango.App, fn func(Message)) { on(a, tango.EventPushReceived, fn) }

// OnOpen calls fn when the user taps a notification.
func OnOpen(a *tango.App, fn func(Message)) { on(a, tango.EventPushOpened, fn) }

func on(a *tango.App, kind platform.AppEventKind, fn func(Message)) {
	a.OnEventKind(kind, func(ev tango.AppEvent) {
		p := ev.Push.Payload
		fn(Message{ID: p.ID, Title: p.Title, Body: p.Body, Action: p.Action, Data: p.Data})
	})
}
