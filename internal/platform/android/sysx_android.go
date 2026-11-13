//go:build android

package android

import (
	"github.com/tangoui/tango/internal/platform"
)

// Desktop capabilities that also make sense on a phone, over Mobile.call,
// implemented in org.tango.host.Sysx: Shell (Intent.ACTION_VIEW for URLs,
// and for files through TangoFileProvider), Notifier (NotificationManager
// with a notification channel; POST_NOTIFICATIONS is asked for on Android
// 13+), Network (ConnectivityManager's default-network callback) and Power
// (the battery broadcast and PowerManager's power-saver mode). The
// language list (Locales, LocaleWatcher) is in system_android.go and is
// also registered for i18n here.

var (
	_ platform.Shell    = (*Backend)(nil)
	_ platform.Notifier = (*Backend)(nil)
	_ platform.Network  = (*Backend)(nil)
	_ platform.Power    = (*Backend)(nil)
)

// The language list needs no Backend value, only the host's Mobile class,
// which nativeInit resolves before main.main runs. Registering it here
// makes i18n follow the system language without i18n.Init, as on desktop.
func init() {
	var b *Backend
	platform.SetLocaleSource(b.PreferredLocales)
	platform.SetLocaleWatch(b.WatchLocales)
}

// ---- Shell ----

// OpenURL starts the app that handles u (ACTION_VIEW, browsable).
func (b *Backend) OpenURL(u string) error {
	return call0("openURL", map[string]string{"url": u})
}

// OpenPath opens a file with the app the system picks for its type, as a
// content:// URI with a read grant. Folders return ErrUnsupported.
func (b *Backend) OpenPath(path string) error {
	return call0("openPath", map[string]string{"path": path})
}

// RevealPath is not possible: Android has no standard file manager that
// can select a file in an app's private storage.
func (b *Backend) RevealPath(string) error { return platform.ErrUnsupported }

// ---- Notifier ----

// notifyClicks maps the ids of notifications sent in this run to their
// onClick. Main thread only.
var notifyClicks = map[string]func(){}

func (b *Backend) RequestNotifyPermission(done func(granted bool, err error)) {
	mobileAsync("notifyPermission", nil, func(s string, err error) { done(err == nil && s == "1", err) })
}

// Notify shows n now. On Android 13+ the user is asked first when they
// never were; a refusal then only drops the notification, while one
// known beforehand is returned as ErrPermissionDenied.
func (b *Backend) Notify(n platform.Notification, onClick func()) error {
	if err := call0("notify", map[string]any{"id": n.ID, "title": n.Title, "subtitle": n.Subtitle,
		"body": n.Body, "silent": n.Silent}); err != nil {
		return err
	}
	if onClick != nil {
		notifyClicks[n.ID] = onClick
	}
	return nil
}

// notificationOpened routes a tap on a local notification to its onClick
// when this run sent it, and reports false otherwise (the caller then
// delivers AppNotificationOpened). Main thread.
func notificationOpened(id string) bool {
	fn := notifyClicks[id]
	if fn == nil {
		return false
	}
	delete(notifyClicks, id)
	fn()
	return true
}

// ---- Network ----

func (b *Backend) NetworkStatus() (platform.NetworkStatus, error) {
	s, err := mobileCall(0, "networkStatus", nil)
	if err != nil {
		return platform.NetworkStatus{}, err
	}
	return decodeNetwork(s)
}

var networkCB int // the watch's callback; main thread

func (b *Backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	if networkCB != 0 {
		dropMobileCB(networkCB)
		networkCB = 0
		mobileCall(0, "networkUnwatch", nil)
	}
	if fn == nil {
		return nil
	}
	cb := addMobileCB(true, func(s string, err error) {
		if err != nil {
			return
		}
		if st, err := decodeNetwork(s); err == nil {
			fn(st)
		}
	})
	if _, err := mobileCall(cb, "networkWatch", nil); err != nil {
		dropMobileCB(cb)
		return err
	}
	networkCB = cb
	return nil
}

// ---- Power ----

func (b *Backend) PowerStatus() (platform.PowerStatus, error) {
	s, err := mobileCall(0, "powerStatus", nil)
	if err != nil {
		return platform.PowerStatus{Battery: -1}, err
	}
	return decodePower(s)
}

var powerCB int // the watch's callback; main thread

// SetPowerHandler reports PowerStatusChanged for battery, charger and
// power-saver changes. Apps are not told about system sleep (Doze), so
// PowerSuspending and PowerResumed never come.
func (b *Backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	if powerCB != 0 {
		dropMobileCB(powerCB)
		powerCB = 0
		mobileCall(0, "powerUnwatch", nil)
	}
	if fn == nil {
		return nil
	}
	cb := addMobileCB(true, func(_ string, err error) {
		if err == nil {
			fn(platform.PowerStatusChanged)
		}
	})
	if _, err := mobileCall(cb, "powerWatch", nil); err != nil {
		dropMobileCB(cb)
		return err
	}
	powerCB = cb
	return nil
}

// Sleep blocks by token; the host keeps the screen on (FLAG_KEEP_SCREEN_ON)
// while a display block is held, and holds a partial wake lock for system
// blocks when the app declares WAKE_LOCK (else keeps the screen on too).
// Main thread only.
var (
	sleepBlocks = map[uint64]platform.SleepBlock{}
	nextBlock   uint64
)

func syncSleepBlocks() error {
	var display, system int
	for _, k := range sleepBlocks {
		if k == platform.BlockDisplaySleep {
			display++
		} else {
			system++
		}
	}
	return call0("sleepBlocks", map[string]int{"display": display, "system": system})
}

func (b *Backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	if kind != platform.BlockSystemSleep && kind != platform.BlockDisplaySleep {
		return 0, platform.ErrUnsupported
	}
	nextBlock++
	sleepBlocks[nextBlock] = kind
	if err := syncSleepBlocks(); err != nil {
		delete(sleepBlocks, nextBlock)
		return 0, err
	}
	return nextBlock, nil
}

func (b *Backend) UnblockSleep(token uint64) {
	if _, ok := sleepBlocks[token]; !ok {
		return
	}
	delete(sleepBlocks, token)
	syncSleepBlocks()
}
