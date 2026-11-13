package platform

import (
	"encoding/json"
	"errors"
	"time"
)

// Contract extensions for mobile system capabilities (share, haptics, safe
// area, orientation, biometrics, secure storage, media, location, push,
// permissions, WebView recovery). Like ext.go, everything here is
// OPTIONAL: a backend implements an interface only where the platform has
// the feature, and the core / capability packages discover it with a type
// assertion. Desktop backends may implement the ones that make sense there
// (share, biometrics, secure storage on macOS).
//
// Threading is the same as backend.go: methods are called on the main
// thread and callbacks run on the main thread, unless stated otherwise.
// Asynchronous methods call done exactly once; a backend that cannot show
// UI (no page on screen, app quitting) calls done with an error instead of
// never calling it.

// ---------------------------------------------------------------------------
// Errors shared by the capabilities below. ErrUnsupported and ErrCanceled
// (caps.go) keep their meaning: no such capability here / the user
// dismissed the UI.

var (
	// ErrNotAvailable: the capability exists on the platform but cannot be
	// used now (no biometric hardware, no camera, location services off).
	ErrNotAvailable = errors.New("tango: not available on this device")
	// ErrNotEnrolled: biometric hardware without enrolled faces/fingers.
	ErrNotEnrolled = errors.New("tango: no biometrics enrolled")
	// ErrLockedOut: too many failed biometric attempts.
	ErrLockedOut = errors.New("tango: biometrics locked out")
	// ErrAuthFailed: the user was not recognised.
	ErrAuthFailed = errors.New("tango: authentication failed")
	// ErrPermissionDenied: the user (or policy) denied the permission the
	// operation needs.
	ErrPermissionDenied = errors.New("tango: permission denied")
	// ErrNotFound: no such item (secure storage key).
	ErrNotFound = errors.New("tango: not found")
	// ErrNotConfigured: the app bundle lacks a declaration the OS requires
	// (an Info.plist usage description, a manifest permission, an
	// entitlement). The error text names it; see docs/mobile-permissions.md.
	ErrNotConfigured = errors.New("tango: app is not configured for this")
)

// ---------------------------------------------------------------------------
// App events (HostEx.OnAppEvent). Their payloads are the typed AppEvent
// fields added in round 3 (before, they borrowed URL and Paths).

const (
	// AppSafeAreaChanged: the safe area insets changed (rotation, status
	// bar, split view). Insets = the new insets.
	AppSafeAreaChanged AppEventKind = 100 + iota
	// AppOrientationChanged: the interface orientation changed.
	// Orientation = the new orientation.
	AppOrientationChanged
	// AppPushToken: the push token was issued or changed. Push.Token = the
	// token.
	AppPushToken
	// AppPushReceived: a remote notification arrived while the app runs.
	// Push.Payload = the notification.
	AppPushReceived
	// AppPushOpened: the user tapped a remote notification (also the one
	// that launched the app). Push.Payload as for AppPushReceived, with
	// Action set.
	AppPushOpened
	// AppWebViewRestored: the system killed a page's web content process
	// and the backend reloaded the page. Surface = the surface, URL = the
	// URL being reloaded. The backend reports SurfaceNavigating before the
	// reload, so messages are held until the page has loaded again.
	AppWebViewRestored
)

// PushEvent is the payload of the push app events.
type PushEvent struct {
	Token   string      // AppPushToken
	Payload PushPayload // AppPushReceived, AppPushOpened
}

// PushPayload is a remote notification. Data holds the platform payload:
// the APNs userInfo dictionary on iOS (including "aps"), the FCM data map
// on Android. It is also the JSON object the native hosts hand to the
// backends.
type PushPayload struct {
	ID     string         `json:"id,omitempty"` // notification identifier, when the platform gives one
	Title  string         `json:"title,omitempty"`
	Body   string         `json:"body,omitempty"`
	Action string         `json:"action,omitempty"` // opened: the action identifier, "" for the default tap
	Data   map[string]any `json:"data,omitempty"`
}

// ParsePushPayload decodes the JSON a native host reports for a push
// notification. Input that is not a PushPayload object ends up raw in
// Data["raw"], so nothing the host sent is lost.
func ParsePushPayload(s string) PushPayload {
	var p PushPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return PushPayload{Data: map[string]any{"raw": s}}
	}
	return p
}

// ---------------------------------------------------------------------------
// Share sheet

type ShareOptions struct {
	Text  string
	URL   string
	Paths []string // files; must exist
	Title string   // chooser title (Android) / subject (mail targets)
}

// Sharer shows the system share sheet. done(nil) means shared (iOS, macOS)
// or the chooser was shown (Android cannot tell); ErrCanceled means the
// user dismissed it.
type Sharer interface {
	Share(opts ShareOptions, done func(err error))
}

// ---------------------------------------------------------------------------
// Haptics

type HapticKind int

const (
	HapticLight HapticKind = iota + 1 // impact
	HapticMedium
	HapticHeavy
	HapticSelection // selection changed (picker tick)
	HapticSuccess   // notification feedback
	HapticWarning
	HapticError
)

type Haptics interface {
	Haptic(k HapticKind) error
}

// ---------------------------------------------------------------------------
// Safe area and orientation

// Insets are in CSS pixels (points / dp), measured from the edges of the
// page's web view: the parts of the page covered by system UI (status bar,
// notch, home indicator, navigation bar). Where the host already keeps the
// web view clear of system UI they are zero.
type Insets struct {
	Top, Bottom, Left, Right float64
}

type SafeArea interface {
	SafeArea() Insets
}

// Orientation is the interface orientation, named by where the device's
// top edge points: OrientationLandscapeLeft is the device turned 90°
// counter-clockwise from portrait (UIInterfaceOrientationLandscapeRight,
// Android ROTATION_90 on phones).
type Orientation int

const (
	OrientationUnknown Orientation = iota
	OrientationPortrait
	OrientationPortraitUpsideDown
	OrientationLandscapeLeft
	OrientationLandscapeRight
)

// OrientationMask is a set of orientations; 0 means "no lock" (whatever the
// app declares as supported).
type OrientationMask int

const (
	MaskPortrait           OrientationMask = 1 << iota
	MaskPortraitUpsideDown                 // phones without a home button do not support it
	MaskLandscapeLeft
	MaskLandscapeRight
	MaskLandscape = MaskLandscapeLeft | MaskLandscapeRight
	MaskAll       = MaskPortrait | MaskPortraitUpsideDown | MaskLandscape
)

type Orientations interface {
	Orientation() Orientation
	// LockOrientation restricts the interface to m (0 unlocks) and rotates
	// to an allowed orientation if needed.
	LockOrientation(m OrientationMask) error
}

// ---------------------------------------------------------------------------
// Biometrics

type BiometryKind int

const (
	BiometryNone    BiometryKind = iota
	BiometryTouchID              // also Android fingerprint
	BiometryFaceID               // also Android face
	BiometryOpticID              // Vision Pro
	BiometryOther                // available, kind not reported (Android)
)

type BiometricOptions struct {
	Reason        string // shown in the prompt (iOS: localizedReason; Android: subtitle)
	Title         string // Android prompt title; macOS/iOS ignore it
	CancelTitle   string // label of the cancel button; "" = system default
	AllowPasscode bool   // fall back to the device passcode / PIN
}

// Biometrics authenticates the device owner. done(nil) on success;
// otherwise ErrCanceled, ErrAuthFailed, ErrNotEnrolled, ErrLockedOut,
// ErrNotAvailable or ErrNotConfigured (iOS without NSFaceIDUsageDescription).
type Biometrics interface {
	// BiometryKind reports what the device has; err explains BiometryNone
	// (ErrNotAvailable, ErrNotEnrolled, ErrLockedOut).
	BiometryKind() (BiometryKind, error)
	Authenticate(opts BiometricOptions, done func(err error))
}

// ---------------------------------------------------------------------------
// Secure storage

// SecureStore keeps small secrets (tokens, keys) in the platform's protected
// store: the Keychain (iOS, macOS), Android Keystore-encrypted
// SharedPreferences. Items belong to the app and are not synced.
//
// EXCEPTION to the main-thread rule: SecureStore methods may be called from
// any goroutine (the stores are thread-safe and can be slow).
type SecureStore interface {
	SecureSet(key string, value []byte) error
	SecureGet(key string) ([]byte, error) // ErrNotFound when absent
	SecureDelete(key string) error        // nil when absent
}

// ---------------------------------------------------------------------------
// Photos and camera

type MediaType int

const (
	MediaImages MediaType = 1 << iota
	MediaVideos
)

type PickMediaOptions struct {
	Types MediaType // 0 = images
	Limit int       // max items; 0 or 1 = single selection
}

// MediaPicker lets the user choose from the photo library (PHPicker,
// Android Photo Picker) without a library permission. done receives paths
// of copies in the app's cache directory, ErrCanceled when dismissed.
type MediaPicker interface {
	PickMedia(opts PickMediaOptions, done func(paths []string, err error))
}

// Camera takes a photo with the system camera UI. done receives the path
// of a JPEG in the app's cache directory.
type Camera interface {
	CapturePhoto(done func(path string, err error))
}

// ---------------------------------------------------------------------------
// Location

type LocationAccuracy int

const (
	AccuracyBalanced LocationAccuracy = iota // ~100 m, network-assisted
	AccuracyHigh                             // best available (GPS)
	AccuracyLow                              // ~1-3 km, battery friendly
)

type LocationOptions struct {
	Accuracy LocationAccuracy
	// DistanceFilter (metres) for updates: only report moves at least this
	// far; 0 reports every update.
	DistanceFilter float64
}

type Location struct {
	Latitude, Longitude float64
	Altitude            float64 // metres; valid when AltitudeAccuracy > 0
	Accuracy            float64 // horizontal, metres
	AltitudeAccuracy    float64
	Speed               float64 // m/s; negative when unknown
	Heading             float64 // course in degrees from north; negative when unknown
	Time                time.Time
}

// Locator reads the device location. Both ask for when-in-use permission
// first if the user has not been asked; ErrPermissionDenied if refused.
type Locator interface {
	// CurrentLocation delivers one fix.
	CurrentLocation(opts LocationOptions, done func(Location, error))
	// StartLocationUpdates delivers fixes to fn until StopLocationUpdates.
	// One session at a time: starting again replaces opts and fn. An error
	// passed to fn ends the session.
	StartLocationUpdates(opts LocationOptions, fn func(Location, error))
	StopLocationUpdates()
}

// ---------------------------------------------------------------------------
// Push notifications

// Push registers the app with the platform's push service. done receives
// the token to send to your server: the APNs device token as lowercase hex
// on iOS; the FCM token on Android when the app wired Firebase in (see
// docs/mobile-permissions.md), else ErrUnsupported. Incoming and opened
// notifications arrive as AppPushReceived / AppPushOpened events.
type Push interface {
	RegisterPush(done func(token string, err error))
}

// ---------------------------------------------------------------------------
// Permissions

// Permission names. They match the permission names of tan.json.
type Permission string

const (
	PermCamera         Permission = "camera"
	PermPhotos         Permission = "photos"
	PermLocation       Permission = "location"        // when in use
	PermLocationAlways Permission = "location-always" // background
	PermNotifications  Permission = "notifications"
	PermMicrophone     Permission = "microphone"
)

type PermissionStatus string

const (
	StatusNotDetermined PermissionStatus = "notDetermined" // never asked; Request will prompt
	StatusGranted       PermissionStatus = "granted"
	StatusDenied        PermissionStatus = "denied"  // refused, restricted by policy, or "don't ask again"
	StatusLimited       PermissionStatus = "limited" // partial (selected photos, approximate location, provisional notifications)
)

// Permissions queries and requests runtime permissions. Unknown names give
// ErrUnsupported. Request does not prompt again once the user decided; it
// then reports the current status.
type Permissions interface {
	CheckPermission(p Permission, done func(PermissionStatus, error))
	RequestPermission(p Permission, done func(PermissionStatus, error))
}

// ===========================================================================
// Round 4: background tasks, in-app purchase, device controls (store
// review, settings page, screen, status bar, keyboard) and home-screen
// widgets. Same rules as above: every interface is optional.

const (
	// AppKeyboardChanged: the software keyboard appeared, disappeared or
	// changed height. Insets.Bottom = the height of the part of the screen
	// the keyboard covers, in CSS pixels; 0 means hidden.
	AppKeyboardChanged AppEventKind = AppWebViewRestored + 1 + iota // 106
	// AppBackgroundTask: the system started (or expired) background tasks.
	// A signal without payload: the listener takes the updates with
	// BackgroundTasks.TakeBackgroundTasks (on the main thread).
	AppBackgroundTask // 107
	// AppPurchaseUpdated: in-app purchase transactions changed state. A
	// signal: take them with Purchases.TakePurchaseUpdates.
	AppPurchaseUpdated // 108
)

// ---------------------------------------------------------------------------
// Background tasks

// BackgroundKind is the kind of a background task. Each kind has at most
// one pending request: scheduling again replaces it.
type BackgroundKind int

const (
	// BackgroundRefresh: short work (about 30 s on iOS) to refresh content,
	// run when the system sees fit (BGAppRefreshTask; Android JobScheduler).
	BackgroundRefresh BackgroundKind = iota + 1
	// BackgroundProcessing: longer maintenance work (minutes), typically
	// while the device is idle (BGProcessingTask; Android JobScheduler).
	BackgroundProcessing
)

type BackgroundRequest struct {
	Kind BackgroundKind
	// Earliest delays the task: not before now+Earliest. 0 = whenever the
	// system decides. It is a lower bound; the system picks the time.
	Earliest         time.Duration
	RequiresNetwork  bool
	RequiresCharging bool // BackgroundProcessing on iOS; any kind on Android
}

// BackgroundTask is one update in the queue TakeBackgroundTasks drains:
// a task the system started (Expired false), or the system asking a
// running task to stop now (Expired true, same ID).
type BackgroundTask struct {
	ID       int64
	Kind     BackgroundKind
	Deadline time.Time // latest end the backend knows of; zero when unknown
	Expired  bool
}

// BackgroundTasks schedules work the system runs while the app is in the
// background, possibly after launching it for that (main.main and App.Run
// then run without a visible page). Every started task must be finished
// with FinishBackgroundTask, at the latest when it expired.
type BackgroundTasks interface {
	ScheduleBackground(r BackgroundRequest) error
	CancelBackground(k BackgroundKind) error
	TakeBackgroundTasks() []BackgroundTask
	FinishBackgroundTask(id int64, success bool)
}

// ---------------------------------------------------------------------------
// In-app purchase

type ProductKind int

const (
	ProductUnknown ProductKind = iota
	ProductConsumable
	ProductNonConsumable
	ProductSubscription
)

type Product struct {
	ID          string
	Title       string
	Description string
	Price       string  // localized, with currency ("¥6.00")
	PriceValue  float64 // in Currency units
	Currency    string  // ISO 4217
	Kind        ProductKind
}

type TransactionState int

const (
	TxPurchasing TransactionState = iota + 1
	TxPurchased
	TxFailed
	TxRestored
	TxDeferred // waiting for approval (Ask to Buy) or pending payment
)

type Transaction struct {
	ID         string // store transaction id ("" while purchasing)
	OriginalID string // for restored transactions: the original purchase
	ProductID  string
	Quantity   int
	State      TransactionState
	Time       time.Time
	Error      string // TxFailed
	Canceled   bool   // TxFailed because the user canceled
}

// Purchases is the platform store. Transactions in TxPurchased or
// TxRestored state stay queued in the store until FinishTransaction (after
// the app delivered the content); the backend finishes failed ones itself.
// Updates for every transaction (also from purchases made outside the app
// or interrupted earlier) arrive as AppPurchaseUpdated.
type Purchases interface {
	CanMakePayments() bool
	// QueryProducts looks products up; invalid lists unknown ids.
	QueryProducts(ids []string, done func(products []Product, invalid []string, err error))
	// Purchase buys a product queried before; done receives the final
	// state (TxPurchased, TxFailed, TxDeferred). ErrCanceled when the user
	// canceled.
	Purchase(productID string, quantity int, done func(Transaction, error))
	// RestorePurchases re-delivers completed non-consumable and
	// subscription transactions as TxRestored updates; done when finished.
	RestorePurchases(done func(error))
	TakePurchaseUpdates() []Transaction
	FinishTransaction(id string) error
	// PurchaseReceipt returns the app's store receipt for server-side
	// validation (iOS: the App Store receipt), ErrNotFound without one.
	PurchaseReceipt() ([]byte, error)
}

// ---------------------------------------------------------------------------
// Device controls

// StoreReview asks for an App Store / Play rating. iOS may show nothing
// (the system limits prompts); Android opens the store page.
type StoreReview interface {
	RequestReview() error
}

// SettingsOpener opens the app's page in the system settings.
type SettingsOpener interface {
	OpenAppSettings() error
}

// ScreenControl keeps the screen on and reads/sets its brightness (0-1).
// SetBrightness(-1) gives brightness back to the system (Android; iOS
// keeps the last value, the system restores its own when the device
// locks).
type ScreenControl interface {
	SetKeepScreenOn(on bool) error
	Brightness() (float64, error)
	SetBrightness(v float64) error
}

type StatusBarStyle int

const (
	StatusBarDefault StatusBarStyle = iota // follows light/dark mode
	StatusBarLight                         // light (white) content, for dark backgrounds
	StatusBarDark                          // dark content, for light backgrounds
)

type StatusBar struct {
	Style  StatusBarStyle
	Hidden bool
	// Color is the background behind the status bar as "#rrggbb" or
	// "#aarrggbb"; "" = unchanged. Android only (iOS pages draw under the
	// status bar themselves); iOS returns ErrUnsupported for a color.
	Color string
}

type StatusBarControl interface {
	SetStatusBar(s StatusBar) error
}

// Keyboard reports the software keyboard. Changes arrive as
// AppKeyboardChanged.
type Keyboard interface {
	// KeyboardHeight is the covered height in CSS pixels, 0 when hidden.
	KeyboardHeight() float64
	HideKeyboard() error
}

// ---------------------------------------------------------------------------
// Home-screen widget

type WidgetContent struct {
	Title string
	Text  string
}

// Widgets updates the app's home-screen widget (Android AppWidget declared
// with the tan.json "widget" capability). The content persists across app
// launches; tapping the widget opens the app.
type Widgets interface {
	UpdateWidget(c WidgetContent) error
}
