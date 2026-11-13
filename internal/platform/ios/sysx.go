//go:build ios

package ios

/*
#cgo LDFLAGS: -framework Network
#include <stdlib.h>
#include "mobile.h"
#include "mobile2.h"
#include "sysx.h"
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Desktop capabilities that also make sense on a phone, implemented in
// sysx.m: Shell (open a URL with its app, preview a file), Notifier (local
// notifications through UNUserNotificationCenter), Network
// (Network.framework's path monitor) and Power (UIDevice battery state and
// Low Power Mode). The language list (Locales, LocaleWatcher) is in
// system.go and is also registered for i18n here.

var (
	_ platform.Shell    = (*backend)(nil)
	_ platform.Notifier = (*backend)(nil)
	_ platform.Network  = (*backend)(nil)
	_ platform.Power    = (*backend)(nil)
)

// The language list does not need a running backend (NSLocale answers on
// any thread), so the package registers it for i18n when imported, like
// the desktop backends: i18n.Locale then follows the system language
// without i18n.Init.
func init() {
	var b *backend
	platform.SetLocaleSource(b.PreferredLocales)
	platform.SetLocaleWatch(b.WatchLocales)
}

// ---- Shell ----

// OpenURL hands u to the system (Safari for http/https, Mail for mailto,
// another app for its scheme). The system decides asynchronously; a URL
// no app handles is only logged.
func (b *backend) OpenURL(u string) error {
	cs := C.CString(u)
	defer C.free(unsafe.Pointer(cs))
	if C.tgx_open_url(cs) != C.TGM_OK {
		return errors.New("tango/ios: bad URL " + u)
	}
	return nil
}

// OpenPath shows a QuickLook preview of a file, or the "Open in…" menu
// for types QuickLook cannot show. Folders return ErrUnsupported.
func (b *backend) OpenPath(path string) error {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	return mobileErr(int(C.tgx_open_path(cs)), "")
}

// RevealPath is not possible: there is no file manager view of the app's
// sandbox to select a file in.
func (b *backend) RevealPath(string) error { return platform.ErrUnsupported }

// ---- Notifier ----

// notifyClicks maps the ids of notifications sent in this run to their
// onClick. Main thread only.
var notifyClicks = map[string]func(){}

func (b *backend) RequestNotifyPermission(done func(granted bool, err error)) {
	C.tgx_notify_permission(addCall(func(code int, s string) {
		done(code == C.TGM_OK && s == "1", mobileErr(code, ""))
	}))
}

// Notify shows n now. Permission is asked for on first use; a refusal
// then only drops the notification, while a refusal known beforehand is
// returned as ErrPermissionDenied.
func (b *backend) Notify(n platform.Notification, onClick func()) error {
	cs := func(s string) *C.char { return C.CString(s) }
	id, t, st, body := cs(n.ID), cs(n.Title), cs(n.Subtitle), cs(n.Body)
	defer func() {
		for _, p := range []*C.char{id, t, st, body} {
			C.free(unsafe.Pointer(p))
		}
	}()
	silent := C.int(0)
	if n.Silent {
		silent = 1
	}
	if code := int(C.tgx_notify(id, t, st, body, silent)); code != C.TGM_OK {
		return mobileErr(code, "notifications are turned off for this app (Settings > Notifications)")
	}
	if onClick != nil {
		notifyClicks[n.ID] = onClick
	}
	return nil
}

// notificationOpened routes a tap on a local notification: to its onClick
// when this run sent it, otherwise as AppNotificationOpened (a
// notification of an earlier run, also the tap that launched the app).
// Main thread.
func notificationOpened(id string) {
	if fn := notifyClicks[id]; fn != nil {
		delete(notifyClicks, id)
		fn()
		return
	}
	emitEvent(platform.AppEvent{Kind: platform.AppNotificationOpened, Notification: id})
}

// ---- Network ----

func (b *backend) NetworkStatus() (platform.NetworkStatus, error) {
	var online, kind, expensive, constrained C.int
	C.tgx_network_status(&online, &kind, &expensive, &constrained)
	return platform.NetworkStatus{
		Online:      online != 0,
		Kind:        platform.NetworkKind(kind),
		Metered:     expensive != 0,
		Constrained: constrained != 0,
	}, nil
}

var networkHandler func(platform.NetworkStatus) // main thread

func (b *backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	networkHandler = fn
	on := C.int(0)
	if fn != nil {
		on = 1
	}
	C.tgx_network_watch(on)
	return nil
}

//export tangoIOSNetworkChanged
func tangoIOSNetworkChanged() {
	if fn := networkHandler; fn != nil {
		if b := current.Load(); b != nil {
			st, _ := b.NetworkStatus()
			fn(st)
		}
	}
}

// ---- Power ----

// PowerStatus reports the battery as UIDevice sees it. The simulator has
// no battery: state unknown, HasBattery false, Battery -1.
func (b *backend) PowerStatus() (platform.PowerStatus, error) {
	var state, low C.int
	var level C.double
	C.tgx_power_status(&state, &level, &low)
	return powerStatus(int(state), float64(level), low != 0), nil
}

var powerHandler func(platform.PowerEvent) // main thread

// SetPowerHandler reports PowerStatusChanged for battery state, level and
// Low Power Mode changes. iOS apps are suspended rather than told about
// system sleep, so PowerSuspending and PowerResumed never come (use the
// app lifecycle).
func (b *backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	powerHandler = fn
	on := C.int(0)
	if fn != nil {
		on = 1
	}
	C.tgx_power_watch(on)
	return nil
}

//export tangoIOSPowerChanged
func tangoIOSPowerChanged() {
	if fn := powerHandler; fn != nil {
		fn(platform.PowerStatusChanged)
	}
}

// Sleep blocks: an app cannot keep the device awake with the screen off,
// so both kinds disable the idle timer (screen auto-lock) while any block
// is held or device.KeepScreenOn asked for it. Main thread only.
var (
	keepScreenOn bool
	sleepBlocks  = map[uint64]platform.SleepBlock{}
	nextBlock    uint64
)

func applyIdleTimer() {
	on := C.int(0)
	if keepScreenOn || len(sleepBlocks) > 0 {
		on = 1
	}
	C.tgm2_keep_screen_on(on)
}

func (b *backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	if kind != platform.BlockSystemSleep && kind != platform.BlockDisplaySleep {
		return 0, platform.ErrUnsupported
	}
	nextBlock++
	sleepBlocks[nextBlock] = kind
	applyIdleTimer()
	return nextBlock, nil
}

func (b *backend) UnblockSleep(token uint64) {
	if _, ok := sleepBlocks[token]; !ok {
		return
	}
	delete(sleepBlocks, token)
	applyIdleTimer()
}
