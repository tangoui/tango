package fake

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Mobile capabilities of the fake backend (internal/platform/mobile_ext.go):
// Sharer, Haptics, SafeArea, Orientations, Biometrics, SecureStore,
// MediaPicker, Camera, Locator, Push and Permissions. State lives beside
// Backend, like sysState, so fake.go needs no change. Asynchronous answers
// are delivered as later native events, as on a real platform.

type mobileState struct {
	mu sync.Mutex

	shares   []platform.ShareOptions
	shareErr error

	haptics []platform.HapticKind

	insets      platform.Insets
	orientation platform.Orientation
	lock        platform.OrientationMask
	locks       []platform.OrientationMask

	bioKind   platform.BiometryKind
	bioErr    error // BiometryKind's error
	authErr   error // Authenticate's answer
	authCalls []platform.BiometricOptions

	secure    map[string][]byte
	secureErr error

	pickPaths []string
	pickErr   error
	picks     []platform.PickMediaOptions
	photoPath string
	photoErr  error

	location    platform.Location
	locationErr error
	watchOpts   *platform.LocationOptions
	watchFn     func(platform.Location, error)

	pushToken string
	pushErr   error

	perms    map[platform.Permission]platform.PermissionStatus
	onAsk    map[platform.Permission]platform.PermissionStatus
	requests []platform.Permission
}

var mobileStates sync.Map // *Backend -> *mobileState

func (b *Backend) mobile() *mobileState {
	if v, ok := mobileStates.Load(b); ok {
		return v.(*mobileState)
	}
	v, _ := mobileStates.LoadOrStore(b, &mobileState{
		orientation: platform.OrientationPortrait,
		bioKind:     platform.BiometryFaceID,
		secure:      map[string][]byte{},
		perms:       map[platform.Permission]platform.PermissionStatus{},
		onAsk:       map[platform.Permission]platform.PermissionStatus{},
		pushToken:   "fake-push-token",
		photoPath:   "/fake/cache/photo.jpg",
		pickPaths:   []string{"/fake/cache/picked-1.jpg"},
	})
	return v.(*mobileState)
}

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

// with runs fn under the state lock.
func (b *Backend) with(fn func(m *mobileState)) {
	m := b.mobile()
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m)
}

// ---- Sharer ----

// SetShareErr sets what Share answers (nil: shared).
func (b *Backend) SetShareErr(err error) { b.with(func(m *mobileState) { m.shareErr = err }) }

// Shares returns every Share request, in order.
func (b *Backend) Shares() (out []platform.ShareOptions) {
	b.with(func(m *mobileState) { out = slices.Clone(m.shares) })
	return
}

func (b *Backend) Share(opts platform.ShareOptions, done func(error)) {
	b.mustMain("Share")
	var err error
	b.with(func(m *mobileState) { m.shares = append(m.shares, opts); err = m.shareErr })
	b.enqueue(func() { done(err) })
}

// ---- Haptics ----

func (b *Backend) Haptic(k platform.HapticKind) error {
	b.mustMain("Haptic")
	b.with(func(m *mobileState) { m.haptics = append(m.haptics, k) })
	return nil
}

// Haptics returns every haptic played, in order.
func (b *Backend) Haptics() (out []platform.HapticKind) {
	b.with(func(m *mobileState) { out = slices.Clone(m.haptics) })
	return
}

// ---- SafeArea / Orientations ----

func (b *Backend) SafeArea() (in platform.Insets) {
	b.mustMain("SafeArea")
	b.with(func(m *mobileState) { in = m.insets })
	return
}

// SetSafeArea changes the insets and emits AppSafeAreaChanged, like a
// rotation would. It reports false if the loop stopped first.
func (b *Backend) SetSafeArea(in platform.Insets) bool {
	b.with(func(m *mobileState) { m.insets = in })
	return b.AppEvent(platform.AppEvent{Kind: platform.AppSafeAreaChanged, Insets: in})
}

func (b *Backend) Orientation() (o platform.Orientation) {
	b.mustMain("Orientation")
	b.with(func(m *mobileState) { o = m.orientation })
	return
}

// Rotate changes the orientation and emits AppOrientationChanged.
func (b *Backend) Rotate(o platform.Orientation) bool {
	b.with(func(m *mobileState) { m.orientation = o })
	return b.AppEvent(platform.AppEvent{Kind: platform.AppOrientationChanged, Orientation: o})
}

func (b *Backend) LockOrientation(mask platform.OrientationMask) error {
	b.mustMain("LockOrientation")
	b.with(func(m *mobileState) { m.lock = mask; m.locks = append(m.locks, mask) })
	return nil
}

// OrientationLocks returns every LockOrientation mask, in order.
func (b *Backend) OrientationLocks() (out []platform.OrientationMask) {
	b.with(func(m *mobileState) { out = slices.Clone(m.locks) })
	return
}

// ---- Biometrics ----

// SetBiometry sets what BiometryKind reports (FaceID, nil by default).
func (b *Backend) SetBiometry(k platform.BiometryKind, err error) {
	b.with(func(m *mobileState) { m.bioKind, m.bioErr = k, err })
}

// SetAuthResult sets what Authenticate answers (nil: success).
func (b *Backend) SetAuthResult(err error) { b.with(func(m *mobileState) { m.authErr = err }) }

// AuthRequests returns every Authenticate request, in order.
func (b *Backend) AuthRequests() (out []platform.BiometricOptions) {
	b.with(func(m *mobileState) { out = slices.Clone(m.authCalls) })
	return
}

func (b *Backend) BiometryKind() (k platform.BiometryKind, err error) {
	b.mustMain("BiometryKind")
	b.with(func(m *mobileState) { k, err = m.bioKind, m.bioErr })
	return
}

func (b *Backend) Authenticate(opts platform.BiometricOptions, done func(error)) {
	b.mustMain("Authenticate")
	var err error
	b.with(func(m *mobileState) { m.authCalls = append(m.authCalls, opts); err = m.authErr })
	b.enqueue(func() { done(err) })
}

// ---- SecureStore (any goroutine) ----

// SetSecureErr makes every SecureStore call fail with err (nil to recover).
func (b *Backend) SetSecureErr(err error) { b.with(func(m *mobileState) { m.secureErr = err }) }

func (b *Backend) SecureSet(key string, value []byte) (err error) {
	b.with(func(m *mobileState) {
		if err = m.secureErr; err == nil {
			m.secure[key] = slices.Clone(value)
		}
	})
	return
}

func (b *Backend) SecureGet(key string) (v []byte, err error) {
	b.with(func(m *mobileState) {
		if err = m.secureErr; err != nil {
			return
		}
		got, ok := m.secure[key]
		if !ok {
			err = platform.ErrNotFound
			return
		}
		v = slices.Clone(got)
	})
	return
}

func (b *Backend) SecureDelete(key string) (err error) {
	b.with(func(m *mobileState) {
		if err = m.secureErr; err == nil {
			delete(m.secure, key)
		}
	})
	return
}

// ---- MediaPicker / Camera ----

// SetPickResult sets what PickMedia answers.
func (b *Backend) SetPickResult(paths []string, err error) {
	b.with(func(m *mobileState) { m.pickPaths, m.pickErr = slices.Clone(paths), err })
}

// PickRequests returns every PickMedia request, in order.
func (b *Backend) PickRequests() (out []platform.PickMediaOptions) {
	b.with(func(m *mobileState) { out = slices.Clone(m.picks) })
	return
}

// SetPhotoResult sets what CapturePhoto answers.
func (b *Backend) SetPhotoResult(path string, err error) {
	b.with(func(m *mobileState) { m.photoPath, m.photoErr = path, err })
}

func (b *Backend) PickMedia(opts platform.PickMediaOptions, done func([]string, error)) {
	b.mustMain("PickMedia")
	var paths []string
	var err error
	b.with(func(m *mobileState) {
		m.picks = append(m.picks, opts)
		paths, err = slices.Clone(m.pickPaths), m.pickErr
	})
	if err != nil {
		paths = nil
	}
	b.enqueue(func() { done(paths, err) })
}

func (b *Backend) CapturePhoto(done func(string, error)) {
	b.mustMain("CapturePhoto")
	var p string
	var err error
	b.with(func(m *mobileState) { p, err = m.photoPath, m.photoErr })
	if err != nil {
		p = ""
	}
	b.enqueue(func() { done(p, err) })
}

// ---- Locator ----

// SetLocation sets what CurrentLocation answers.
func (b *Backend) SetLocation(l platform.Location, err error) {
	b.with(func(m *mobileState) { m.location, m.locationErr = l, err })
}

func (b *Backend) CurrentLocation(opts platform.LocationOptions, done func(platform.Location, error)) {
	b.mustMain("CurrentLocation")
	var l platform.Location
	var err error
	b.with(func(m *mobileState) { l, err = m.location, m.locationErr })
	b.enqueue(func() { done(l, err) })
}

func (b *Backend) StartLocationUpdates(opts platform.LocationOptions, fn func(platform.Location, error)) {
	b.mustMain("StartLocationUpdates")
	b.with(func(m *mobileState) { m.watchOpts, m.watchFn = &opts, fn })
}

func (b *Backend) StopLocationUpdates() {
	b.mustMain("StopLocationUpdates")
	b.with(func(m *mobileState) { m.watchOpts, m.watchFn = nil, nil })
}

// LocationUpdates reports whether an update session runs, and its options.
func (b *Backend) LocationUpdates() (opts platform.LocationOptions, on bool) {
	b.with(func(m *mobileState) {
		if m.watchOpts != nil {
			opts, on = *m.watchOpts, true
		}
	})
	return
}

// MoveTo delivers a location update (or err, which ends the session) to
// the running session. It reports false when none runs.
func (b *Backend) MoveTo(l platform.Location, err error) bool {
	var fn func(platform.Location, error)
	b.with(func(m *mobileState) { fn = m.watchFn })
	if fn == nil {
		return false
	}
	return b.Do(func() {
		if err != nil {
			b.with(func(m *mobileState) { m.watchOpts, m.watchFn = nil, nil })
		}
		fn(l, err)
	})
}

// ---- Push ----

// PushToken simulates the platform issuing a new push token
// (AppPushToken).
func (b *Backend) PushToken(token string) bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppPushToken, Push: platform.PushEvent{Token: token}})
}

// SetPushToken sets what RegisterPush answers.
func (b *Backend) SetPushToken(token string, err error) {
	b.with(func(m *mobileState) { m.pushToken, m.pushErr = token, err })
}

func (b *Backend) RegisterPush(done func(string, error)) {
	b.mustMain("RegisterPush")
	var t string
	var err error
	b.with(func(m *mobileState) { t, err = m.pushToken, m.pushErr })
	b.enqueue(func() { done(t, err) })
}

// PushNotification simulates a remote notification: received while the app
// runs, or opened (tapped) when opened is true.
func (b *Backend) PushNotification(p platform.PushPayload, opened bool) bool {
	k := platform.AppPushReceived
	if opened {
		k = platform.AppPushOpened
	}
	return b.AppEvent(platform.AppEvent{Kind: k, Push: platform.PushEvent{Payload: p}})
}

// WebViewRestored simulates the system killing surface id's web content
// process and the backend reloading url.
func (b *Backend) WebViewRestored(id platform.SurfaceID, url string) bool {
	return b.AppEvent(platform.AppEvent{Kind: platform.AppWebViewRestored, URL: url, Surface: id})
}

// ---- Permissions ----

// SetPermission sets the current status of p (notDetermined by default) and
// the status the user picks when asked (granted by default).
func (b *Backend) SetPermission(p platform.Permission, now, onAsk platform.PermissionStatus) {
	b.with(func(m *mobileState) { m.perms[p], m.onAsk[p] = now, onAsk })
}

// PermissionRequests returns every RequestPermission call, in order.
func (b *Backend) PermissionRequests() (out []platform.Permission) {
	b.with(func(m *mobileState) { out = slices.Clone(m.requests) })
	return
}

func knownPermission(p platform.Permission) bool {
	switch p {
	case platform.PermCamera, platform.PermPhotos, platform.PermLocation, platform.PermLocationAlways,
		platform.PermNotifications, platform.PermMicrophone:
		return true
	}
	return false
}

func (m *mobileState) status(p platform.Permission) platform.PermissionStatus {
	if s, ok := m.perms[p]; ok {
		return s
	}
	return platform.StatusNotDetermined
}

func (b *Backend) CheckPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	b.mustMain("CheckPermission")
	if !knownPermission(p) {
		b.enqueue(func() { done(platform.StatusDenied, platform.ErrUnsupported) })
		return
	}
	var s platform.PermissionStatus
	b.with(func(m *mobileState) { s = m.status(p) })
	b.enqueue(func() { done(s, nil) })
}

func (b *Backend) RequestPermission(p platform.Permission, done func(platform.PermissionStatus, error)) {
	b.mustMain("RequestPermission")
	if !knownPermission(p) {
		b.enqueue(func() { done(platform.StatusDenied, platform.ErrUnsupported) })
		return
	}
	var s platform.PermissionStatus
	b.with(func(m *mobileState) {
		m.requests = append(m.requests, p)
		s = m.status(p)
		if s == platform.StatusNotDetermined {
			s = platform.StatusGranted
			if a, ok := m.onAsk[p]; ok {
				s = a
			}
			m.perms[p] = s
		}
	})
	b.enqueue(func() { done(s, nil) })
}
