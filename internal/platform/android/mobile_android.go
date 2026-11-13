//go:build android

package android

/*
#include <stdlib.h>
#include "jni_android.h"
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Mobile capabilities (internal/platform/mobile_ext.go) over one JNI
// method, org.tango.host.Mobile.call(cb, op, json): op names the operation
// and json its arguments. Synchronous operations return their result
// ("!code message" on failure). Asynchronous ones (cb > 0) answer later on
// the main thread through Mobile.nativeDone(cb, result, error); location
// updates answer repeatedly with the same cb until stopped. App events
// (deep links, push, safe area, orientation, WebView restored) arrive
// through Mobile.nativeEvent.

var (
	_ platform.Sharer       = (*Backend)(nil)
	_ platform.Haptics      = (*Backend)(nil)
	_ platform.SafeArea     = (*Backend)(nil)
	_ platform.Orientations = (*Backend)(nil)
	_ platform.Biometrics   = (*Backend)(nil)
	_ platform.SecureStore  = (*Backend)(nil)
	_ platform.MediaPicker  = (*Backend)(nil)
	_ platform.Camera       = (*Backend)(nil)
	_ platform.Locator      = (*Backend)(nil)
	_ platform.Push         = (*Backend)(nil)
	_ platform.Permissions  = (*Backend)(nil)
)

const mobileClassName = "org/tango/host/Mobile"

var (
	mobileClass C.jclass
	mMobileCall = &jmethod{cls: &mobileClass, static: true, name: "call", sig: "(ILjava/lang/String;Ljava/lang/String;)Ljava/lang/String;"}
)

// lookupMobile resolves Mobile.call. Main thread, from nativeInit. A host
// without the class (an older copy of mobile/android) keeps working; the
// mobile capabilities then fail with errJava.
func lookupMobile(e env) error {
	n := C.CString(mobileClassName)
	mobileClass = C.tg_find_class(e.p, n)
	C.free(unsafe.Pointer(n))
	if mobileClass == 0 {
		return errJavaClass(mobileClassName)
	}
	name, sig := C.CString(mMobileCall.name), C.CString(mMobileCall.sig)
	mMobileCall.id = C.tg_method(e.p, mobileClass, name, sig, 1)
	C.free(unsafe.Pointer(name))
	C.free(unsafe.Pointer(sig))
	if mMobileCall.id == nil {
		mobileClass = 0
		return errJavaClass(mobileClassName + ".call")
	}
	return nil
}

type javaMissing string

func (j javaMissing) Error() string { return "tango/android: Java " + string(j) + " not found" }

func errJavaClass(s string) error { return javaMissing(s) }

// mobileCall runs op synchronously on the calling thread.
func mobileCall(cb int, op string, arg any) (string, error) {
	if mobileClass == 0 {
		return "", errJava
	}
	a := "{}"
	if arg != nil {
		a = args(arg)
	}
	var res string
	var err error
	if !withEnv(func(e env) {
		var o C.jobject
		o, err = e.callStaticObject(mMobileCall, jvInt(cb), jvObj(e.str(op)), jvObj(e.str(a)))
		if err == nil {
			res = e.gostr(C.jstring(o))
		}
	}) && err == nil {
		err = errJava
	}
	if err != nil {
		return "", err
	}
	return splitResult(res)
}

// Callbacks of asynchronous operations.
var mobileCBs struct {
	sync.Mutex
	next int
	m    map[int]mobileCB
}

type mobileCB struct {
	fn   func(res string, err error)
	keep bool // answered repeatedly (location updates)
}

func addMobileCB(keep bool, fn func(string, error)) int {
	mobileCBs.Lock()
	defer mobileCBs.Unlock()
	if mobileCBs.m == nil {
		mobileCBs.m = map[int]mobileCB{}
	}
	mobileCBs.next++
	if mobileCBs.next <= 0 {
		mobileCBs.next = 1
	}
	mobileCBs.m[mobileCBs.next] = mobileCB{fn, keep}
	return mobileCBs.next
}

func dropMobileCB(cb int) {
	mobileCBs.Lock()
	delete(mobileCBs.m, cb)
	mobileCBs.Unlock()
}

// mobileAsync starts an asynchronous op; fn runs once on the main thread.
func mobileAsync(op string, arg any, fn func(string, error)) {
	cb := addMobileCB(false, fn)
	if _, err := mobileCall(cb, op, arg); err != nil {
		dropMobileCB(cb)
		fn("", err)
	}
}

//export Java_org_tango_host_Mobile_nativeDone
func Java_org_tango_host_Mobile_nativeDone(penv *C.JNIEnv, cls C.jclass, cb C.jint, res, errMsg C.jstring) {
	e := env{penv}
	mobileCBs.Lock()
	c, ok := mobileCBs.m[int(cb)]
	err := javaErr(e.gostr(errMsg))
	if ok && (!c.keep || err != nil) {
		delete(mobileCBs.m, int(cb))
	}
	mobileCBs.Unlock()
	if ok {
		c.fn(e.gostr(res), err)
	}
}

// ---- app events ----

var mobileEvents eventQueue // main thread

//export Java_org_tango_host_Mobile_nativeEvent
func Java_org_tango_host_Mobile_nativeEvent(penv *C.JNIEnv, cls C.jclass, kind C.jint, url, path C.jstring) {
	e := env{penv}
	ev, arg := platform.AppEvent{Kind: platform.AppEventKind(kind)}, e.gostr(url)
	b := backend
	switch ev.Kind {
	case platform.AppPushToken:
		ev.Push.Token = arg
	case platform.AppPushReceived, platform.AppPushOpened:
		ev.Push.Payload = platform.ParsePushPayload(arg)
	case platform.AppWebViewRestored:
		ev.URL = arg
		// path is the SurfaceID in decimal. The page has a new WebView:
		// forget the old one's reference.
		if id, err := strconv.ParseUint(e.gostr(path), 10, 64); err == nil {
			ev.Surface = platform.SurfaceID(id)
			if s := b.surface(ev.Surface); s != nil && s.view != 0 {
				v := s.view
				s.view = 0
				e.deleteGlobal(v)
			}
		}
	case platform.AppSafeAreaChanged, platform.AppOrientationChanged, platform.AppBackgroundTask:
		// Payload read at delivery (deliverMobileEvent) or taken by the listener.
	case platform.AppKeyboardChanged:
		ev.Insets.Bottom = parseFloat(arg) // dp
	case platform.AppThemeChanged, platform.AppReduceMotionChanged, platform.AppContrastChanged:
		// Theme read at delivery (deliverMobileEvent); the others are signals.
	case platform.AppMemoryWarning:
		// A signal (TangoActivity.onTrimMemory / onLowMemory).
		logf(logInfo, "memory warning: delivering AppMemoryWarning")
	default:
		ev.URL = arg
		if p := e.gostr(path); p != "" {
			ev.Paths = []string{p}
		}
	}
	h := b.host()
	if h == nil || !b.life.running {
		mobileEvents.add(ev)
		return
	}
	if hx, ok := h.(platform.HostEx); ok {
		b.deliverMobileEvent(hx, ev)
	}
}

// deliverMobileEvent fills in the state events' payload (the state at
// delivery time) and delivers ev. Main thread.
func (b *Backend) deliverMobileEvent(hx platform.HostEx, ev platform.AppEvent) {
	switch ev.Kind {
	case platform.AppSafeAreaChanged:
		ev.Insets = b.SafeArea()
	case platform.AppOrientationChanged:
		ev.Orientation = b.Orientation()
	case platform.AppThemeChanged:
		ev.Theme = b.SystemTheme() // system_android.go
	}
	hx.OnAppEvent(ev)
}

// flushMobileEvents delivers events reported before Started. Main thread,
// right after the lifecycle replay.
func (b *Backend) flushMobileEvents(h platform.Host) {
	hx, ok := h.(platform.HostEx)
	for _, ev := range mobileEvents.take() {
		if ok {
			b.deliverMobileEvent(hx, ev)
		}
	}
}

// ---- Sharer ----

func (b *Backend) Share(opts platform.ShareOptions, done func(error)) {
	mobileAsync("share", map[string]any{"text": opts.Text, "url": opts.URL, "paths": opts.Paths, "title": opts.Title},
		func(_ string, err error) { done(err) })
}

// ---- Haptics ----

func (b *Backend) Haptic(k platform.HapticKind) error {
	_, err := mobileCall(0, "haptic", map[string]int{"kind": int(k)})
	return err
}

// ---- SafeArea / Orientations ----

func (b *Backend) SafeArea() platform.Insets {
	var in platform.Insets
	s, err := mobileCall(0, "safeArea", nil)
	if err != nil {
		return in
	}
	var v struct{ Top, Bottom, Left, Right float64 }
	if json.Unmarshal([]byte(s), &v) == nil {
		in = platform.Insets{Top: v.Top, Bottom: v.Bottom, Left: v.Left, Right: v.Right}
	}
	return in
}

func (b *Backend) Orientation() platform.Orientation {
	s, err := mobileCall(0, "orientation", nil)
	if err != nil {
		return platform.OrientationUnknown
	}
	n, _ := strconv.Atoi(s)
	return platform.Orientation(n)
}

func (b *Backend) LockOrientation(m platform.OrientationMask) error {
	_, err := mobileCall(0, "lockOrientation", map[string]int{"mask": int(m)})
	return err
}

// ---- Biometrics ----

func (b *Backend) BiometryKind() (platform.BiometryKind, error) {
	s, err := mobileCall(0, "biometryKind", nil)
	if err != nil {
		return platform.BiometryNone, err
	}
	var v struct {
		Kind  int
		Error string
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return platform.BiometryNone, err
	}
	return platform.BiometryKind(v.Kind), javaErr(v.Error)
}

func (b *Backend) Authenticate(opts platform.BiometricOptions, done func(error)) {
	mobileAsync("authenticate", map[string]any{"reason": opts.Reason, "title": opts.Title,
		"cancel": opts.CancelTitle, "passcode": opts.AllowPasscode}, func(_ string, err error) { done(err) })
}

// ---- SecureStore (any goroutine) ----

func (b *Backend) SecureSet(key string, value []byte) error {
	_, err := mobileCall(0, "secureSet", map[string]string{"key": key, "value": base64.StdEncoding.EncodeToString(value)})
	return err
}

func (b *Backend) SecureGet(key string) ([]byte, error) {
	s, err := mobileCall(0, "secureGet", map[string]string{"key": key})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(s)
}

func (b *Backend) SecureDelete(key string) error {
	_, err := mobileCall(0, "secureDelete", map[string]string{"key": key})
	return err
}

// ---- MediaPicker / Camera ----

func (b *Backend) PickMedia(opts platform.PickMediaOptions, done func([]string, error)) {
	mobileAsync("pickMedia", map[string]int{"types": int(opts.Types), "limit": opts.Limit}, func(s string, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		done(decodePaths(s))
	})
}

func (b *Backend) CapturePhoto(done func(string, error)) {
	mobileAsync("capturePhoto", nil, done)
}

// ---- Locator ----

var locationCB int // the update session's callback; main thread

func locArgs(o platform.LocationOptions) map[string]any {
	return map[string]any{"accuracy": int(o.Accuracy), "distance": o.DistanceFilter}
}

func (b *Backend) CurrentLocation(opts platform.LocationOptions, done func(platform.Location, error)) {
	mobileAsync("currentLocation", locArgs(opts), func(s string, err error) {
		if err != nil {
			done(platform.Location{}, err)
			return
		}
		done(decodeLocation(s))
	})
}

func (b *Backend) StartLocationUpdates(opts platform.LocationOptions, fn func(platform.Location, error)) {
	b.StopLocationUpdates()
	var cb int
	cb = addMobileCB(true, func(s string, err error) {
		if err != nil {
			if locationCB == cb {
				locationCB = 0
			}
			fn(platform.Location{}, err)
			return
		}
		if l, err := decodeLocation(s); err == nil {
			fn(l, nil)
		}
	})
	locationCB = cb
	a := locArgs(opts)
	if _, err := mobileCall(cb, "startLocation", a); err != nil {
		dropMobileCB(cb)
		locationCB = 0
		fn(platform.Location{}, err)
	}
}

func (b *Backend) StopLocationUpdates() {
	if locationCB == 0 {
		return
	}
	cb := locationCB
	locationCB = 0
	dropMobileCB(cb)
	mobileCall(cb, "stopLocation", nil)
}

// ---- Push ----

func (b *Backend) RegisterPush(done func(string, error)) {
	mobileAsync("registerPush", nil, done)
}

// ---- Permissions ----

func (b *Backend) CheckPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	mobileAsync("checkPermission", map[string]string{"name": string(p)}, permDone(done))
}

func (b *Backend) RequestPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	mobileAsync("requestPermission", map[string]string{"name": string(p)}, permDone(done))
}

func permDone(done func(platform.PermissionStatus, error)) func(string, error) {
	return func(s string, err error) {
		if err != nil {
			done(platform.StatusDenied, err)
			return
		}
		done(platform.PermissionStatus(s), nil)
	}
}
