//go:build ios

package ios

/*
// PhotosUI, Photos and AVFoundation are not linked: mobile.m loads them on
// first use (see "Frameworks loaded on first use" there).
#cgo LDFLAGS: -framework LocalAuthentication -framework Security -framework CoreLocation -framework UserNotifications
#include <stdlib.h>
#include "bridge.h"
#include "mobile.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Mobile capabilities (internal/platform/mobile_ext.go), implemented in
// mobile.m.

var (
	_ platform.Sharer       = (*backend)(nil)
	_ platform.Haptics      = (*backend)(nil)
	_ platform.SafeArea     = (*backend)(nil)
	_ platform.Orientations = (*backend)(nil)
	_ platform.Biometrics   = (*backend)(nil)
	_ platform.SecureStore  = (*backend)(nil)
	_ platform.MediaPicker  = (*backend)(nil)
	_ platform.Camera       = (*backend)(nil)
	_ platform.Locator      = (*backend)(nil)
	_ platform.Push         = (*backend)(nil)
	_ platform.Permissions  = (*backend)(nil)
)

// Pending asynchronous calls by token. Main thread only.
var (
	mobileCalls = map[int]func(code int, s string){}
	locCalls    = map[int]func(code int, msg string, l platform.Location){}
	nextMobile  int
)

func addCall(fn func(code int, s string)) C.int {
	nextMobile++
	mobileCalls[nextMobile] = fn
	return C.int(nextMobile)
}

func addLocCall(fn func(code int, msg string, l platform.Location)) C.int {
	nextMobile++
	locCalls[nextMobile] = fn
	return C.int(nextMobile)
}

//export tangoIOSMobileDone
func tangoIOSMobileDone(tok, code C.int, s *C.char) {
	fn := mobileCalls[int(tok)]
	delete(mobileCalls, int(tok))
	if fn != nil {
		fn(int(code), C.GoString(s))
	}
}

//export tangoIOSLocation
func tangoIOSLocation(tok, code C.int, msg *C.char, l *C.tg_location) {
	fn := locCalls[int(tok)]
	if fn == nil {
		return
	}
	if int(tok) != updatesTok {
		delete(locCalls, int(tok)) // a single fix answers once
	}
	fn(int(code), C.GoString(msg), goLocation(l))
}

// mobileErr maps a TGM_* code to an error.
func mobileErr(code int, msg string) error {
	var base error
	switch code {
	case C.TGM_OK:
		return nil
	case C.TGM_CANCELED:
		return platform.ErrCanceled
	case C.TGM_AUTH_FAILED:
		base = platform.ErrAuthFailed
	case C.TGM_NOT_ENROLLED:
		base = platform.ErrNotEnrolled
	case C.TGM_LOCKED_OUT:
		base = platform.ErrLockedOut
	case C.TGM_NOT_AVAILABLE:
		base = platform.ErrNotAvailable
	case C.TGM_NOT_CONFIGURED:
		base = platform.ErrNotConfigured
	case C.TGM_PERMISSION_DENIED:
		base = platform.ErrPermissionDenied
	case C.TGM_UNSUPPORTED:
		return platform.ErrUnsupported
	case C.TGM_NOT_FOUND:
		return platform.ErrNotFound
	default:
		if msg == "" {
			msg = "failed"
		}
		return errors.New("tango/ios: " + msg)
	}
	if msg == "" {
		return base
	}
	return fmt.Errorf("%w (%s)", base, msg)
}

// ---- app events ----

// pendingEvents holds app events that arrived before LifecycleStarted
// (a link or notification tap that launched the app). Main thread only.
var pendingEvents []platform.AppEvent

func (b *backend) emit(ev platform.AppEvent) {
	if !b.started || b.terminated {
		pendingEvents = append(pendingEvents, ev)
		return
	}
	// State events carry the state at delivery time.
	switch ev.Kind {
	case platform.AppSafeAreaChanged:
		ev.Insets = b.SafeArea()
	case platform.AppOrientationChanged:
		ev.Orientation = b.Orientation()
	}
	if hx, ok := b.h.(platform.HostEx); ok {
		hx.OnAppEvent(ev)
	}
}

// flushAppEvents runs right after LifecycleStarted was delivered.
func (b *backend) flushAppEvents() {
	evs := pendingEvents
	pendingEvents = nil
	for _, ev := range evs {
		b.emit(ev)
	}
}

func emitEvent(ev platform.AppEvent) {
	if b := current.Load(); b != nil && b.ready.Load() {
		b.emit(ev)
		return
	}
	// Before App.Run: keep links and pushes; state events are re-read.
	switch ev.Kind {
	case platform.AppOpenURL, platform.AppPushOpened, platform.AppPushReceived, platform.AppPushToken,
		platform.AppBackgroundTask, platform.AppPurchaseUpdated, platform.AppNotificationOpened:
		pendingEvents = append(pendingEvents, ev)
	}
}

//export tangoIOSAppEvent
func tangoIOSAppEvent(kind C.int, s *C.char) {
	str := C.GoString(s)
	var ev platform.AppEvent
	switch kind {
	case C.TGE_OPEN_URL:
		ev = platform.AppEvent{Kind: platform.AppOpenURL, URL: str}
	case C.TGE_SAFE_AREA:
		ev.Kind = platform.AppSafeAreaChanged // payload filled in by emit
	case C.TGE_ORIENTATION:
		ev.Kind = platform.AppOrientationChanged
	case C.TGE_PUSH_TOKEN:
		ev = platform.AppEvent{Kind: platform.AppPushToken, Push: platform.PushEvent{Token: str}}
	case C.TGE_PUSH_RECEIVED:
		ev = platform.AppEvent{Kind: platform.AppPushReceived, Push: platform.PushEvent{Payload: platform.ParsePushPayload(str)}}
	case C.TGE_PUSH_OPENED:
		ev = platform.AppEvent{Kind: platform.AppPushOpened, Push: platform.PushEvent{Payload: platform.ParsePushPayload(str)}}
	case C.TGE_NOTIFICATION_OPENED:
		notificationOpened(str) // sysx.go
		return
	default:
		return
	}
	emitEvent(ev)
}

//export tangoIOSWebViewTerminated
func tangoIOSWebViewTerminated(id C.uint64_t, url *C.char) {
	u := C.GoString(url)
	b, s := live(id)
	if s == nil {
		return
	}
	// The page is gone: hold Go→JS messages until the reload has loaded.
	b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: u})
	if u == "" {
		u = assetOrigin + "/"
	}
	s.Navigate(u)
	b.emit(platform.AppEvent{Kind: platform.AppWebViewRestored, URL: u, Surface: s.id})
}

// ---- Sharer ----

func (b *backend) Share(opts platform.ShareOptions, done func(error)) {
	text, f1 := cstring(opts.Text)
	defer f1()
	url, f2 := cstring(opts.URL)
	defer f2()
	paths, f3 := cstring(strings.Join(opts.Paths, "\n"))
	defer f3()
	title, f4 := cstring(opts.Title)
	defer f4()
	tok := addCall(func(code int, s string) { done(mobileErr(code, s)) })
	C.tgm_share(tok, text, url, paths, title)
}

// ---- Haptics ----

func (b *backend) Haptic(k platform.HapticKind) error {
	if k < platform.HapticLight || k > platform.HapticError {
		return fmt.Errorf("tango/ios: unknown haptic %d", k)
	}
	C.tgm_haptic(C.int(k))
	return nil
}

// ---- SafeArea / Orientations ----

func (b *backend) SafeArea() platform.Insets {
	var t, bo, l, r C.double
	C.tgm_safe_area(&t, &bo, &l, &r)
	return platform.Insets{Top: float64(t), Bottom: float64(bo), Left: float64(l), Right: float64(r)}
}

func (b *backend) Orientation() platform.Orientation {
	return platform.Orientation(C.tgm_orientation())
}

func (b *backend) LockOrientation(m platform.OrientationMask) error {
	C.tgm_lock_orientation(C.int(m & platform.MaskAll))
	return nil
}

// ---- Biometrics ----

func (b *backend) BiometryKind() (platform.BiometryKind, error) {
	var k C.int
	code := C.tgm_biometry(&k)
	return platform.BiometryKind(k), mobileErr(int(code), "")
}

func (b *backend) Authenticate(opts platform.BiometricOptions, done func(error)) {
	reason, f1 := cstring(opts.Reason)
	defer f1()
	cancel, f2 := cstring(opts.CancelTitle)
	defer f2()
	pass := C.int(0)
	if opts.AllowPasscode {
		pass = 1
	}
	tok := addCall(func(code int, s string) { done(mobileErr(code, s)) })
	C.tgm_authenticate(tok, reason, cancel, pass)
}

// ---- SecureStore (any goroutine) ----

func (b *backend) SecureSet(key string, value []byte) error {
	k, free := cstring(key)
	defer free()
	var p unsafe.Pointer
	if len(value) > 0 {
		p = C.CBytes(value)
		defer C.free(p)
	}
	return keychainErr(C.tgm_secure_set(k, p, C.int(len(value))))
}

func (b *backend) SecureGet(key string) ([]byte, error) {
	k, free := cstring(key)
	defer free()
	var out unsafe.Pointer
	var n C.int
	if err := keychainErr(C.tgm_secure_get(k, &out, &n)); err != nil {
		return nil, err
	}
	defer C.free(out)
	return C.GoBytes(out, n), nil
}

func (b *backend) SecureDelete(key string) error {
	k, free := cstring(key)
	defer free()
	return keychainErr(C.tgm_secure_delete(k))
}

func keychainErr(code C.int) error {
	switch code {
	case C.TGM_OK:
		return nil
	case C.TGM_NOT_FOUND:
		return platform.ErrNotFound
	case C.TGM_NOT_CONFIGURED:
		return fmt.Errorf("%w (Keychain: missing entitlement; sign the app)", platform.ErrNotConfigured)
	}
	return errors.New("tango/ios: Keychain operation failed")
}

// ---- MediaPicker / Camera ----

func (b *backend) PickMedia(opts platform.PickMediaOptions, done func([]string, error)) {
	tok := addCall(func(code int, s string) {
		if err := mobileErr(code, s); err != nil {
			done(nil, err)
			return
		}
		done(strings.Split(s, "\n"), nil)
	})
	C.tgm_pick_media(tok, C.int(opts.Types), C.int(max(opts.Limit, 1)))
}

func (b *backend) CapturePhoto(done func(string, error)) {
	tok := addCall(func(code int, s string) {
		if err := mobileErr(code, s); err != nil {
			done("", err)
			return
		}
		done(s, nil)
	})
	C.tgm_capture_photo(tok)
}

// ---- Locator ----

// updatesTok is the token of the running update session, 0 for none.
var updatesTok int

func goLocation(l *C.tg_location) platform.Location {
	if l == nil {
		return platform.Location{}
	}
	sec, frac := math.Modf(float64(l.time))
	loc := platform.Location{
		Latitude:         float64(l.lat),
		Longitude:        float64(l.lon),
		Altitude:         float64(l.alt),
		Accuracy:         float64(l.acc),
		AltitudeAccuracy: float64(l.alt_acc),
		Speed:            float64(l.speed),
		Heading:          float64(l.course),
	}
	if l.time != 0 {
		loc.Time = time.Unix(int64(sec), int64(frac*1e9))
	}
	return loc
}

func (b *backend) CurrentLocation(opts platform.LocationOptions, done func(platform.Location, error)) {
	tok := addLocCall(func(code int, msg string, l platform.Location) {
		if err := mobileErr(code, msg); err != nil {
			done(platform.Location{}, err)
			return
		}
		done(l, nil)
	})
	C.tgm_location_once(tok, C.int(opts.Accuracy))
}

func (b *backend) StartLocationUpdates(opts platform.LocationOptions, fn func(platform.Location, error)) {
	if updatesTok != 0 {
		delete(locCalls, updatesTok)
	}
	var tok C.int
	tok = addLocCall(func(code int, msg string, l platform.Location) {
		if err := mobileErr(code, msg); err != nil {
			if updatesTok == int(tok) {
				updatesTok = 0
				delete(locCalls, int(tok))
			}
			fn(platform.Location{}, err)
			return
		}
		fn(l, nil)
	})
	updatesTok = int(tok)
	C.tgm_location_start(tok, C.int(opts.Accuracy), C.double(opts.DistanceFilter))
}

func (b *backend) StopLocationUpdates() {
	if updatesTok != 0 {
		delete(locCalls, updatesTok)
		updatesTok = 0
	}
	C.tgm_location_stop()
}

// ---- Push ----

func (b *backend) RegisterPush(done func(string, error)) {
	tok := addCall(func(code int, s string) {
		if err := mobileErr(code, s); err != nil {
			done("", err)
			return
		}
		done(s, nil)
	})
	C.tgm_push_register(tok)
}

// ---- Permissions ----

func (b *backend) CheckPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	b.permission(p, false, done)
}

func (b *backend) RequestPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	b.permission(p, true, done)
}

func (b *backend) permission(p platform.Permission, request bool, done func(platform.PermissionStatus, error)) {
	name, free := cstring(string(p))
	defer free()
	req := C.int(0)
	if request {
		req = 1
	}
	tok := addCall(func(code int, s string) {
		if err := mobileErr(code, s); err != nil {
			done(platform.StatusDenied, err)
			return
		}
		done(platform.PermissionStatus(s), nil)
	})
	C.tgm_permission(tok, name, req)
}
