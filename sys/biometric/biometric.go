// Package biometric verifies the device owner with Face ID / Touch ID
// (iOS, macOS) or BiometricPrompt (Android 9+).
//
//	if err := biometric.Authenticate(a, biometric.Options{Reason: "Unlock your notes"}); err != nil {
//		// ErrCanceled, ErrFailed, ErrNotEnrolled, ErrLockedOut, ErrNotAvailable ...
//	}
//
// iOS needs NSFaceIDUsageDescription in Info.plist (permission
// "biometric" in tan.json); without it Authenticate returns
// ErrNotConfigured instead of crashing. Android needs
// android.permission.USE_BIOMETRIC.
//
// Functions return ErrUnsupported where the backend has no biometrics and
// tango.ErrNotRunning after the app quit. On Android call Authenticate
// from a goroutine, not from a main-thread callback.
package biometric

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported   = platform.ErrUnsupported
	ErrCanceled      = platform.ErrCanceled
	ErrFailed        = platform.ErrAuthFailed
	ErrNotAvailable  = platform.ErrNotAvailable
	ErrNotEnrolled   = platform.ErrNotEnrolled
	ErrLockedOut     = platform.ErrLockedOut
	ErrNotConfigured = platform.ErrNotConfigured
)

// Kind is the biometry the device offers.
type Kind = platform.BiometryKind

const (
	None    = platform.BiometryNone
	TouchID = platform.BiometryTouchID // also Android fingerprint
	FaceID  = platform.BiometryFaceID  // also Android face
	OpticID = platform.BiometryOpticID
	Other   = platform.BiometryOther // available, kind not reported (Android)
)

// String names k: "none", "touchID", "faceID", "opticID", "biometric".
func String(k Kind) string {
	switch k {
	case TouchID:
		return "touchID"
	case FaceID:
		return "faceID"
	case OpticID:
		return "opticID"
	case Other:
		return "biometric"
	}
	return "none"
}

// Options configures the prompt.
type Options struct {
	_ [0]func()

	Reason        string // why the app asks; required on iOS/macOS
	Title         string // Android prompt title (default: the app name)
	CancelTitle   string // cancel button label; "" = system default
	AllowPasscode bool   // let the user fall back to the device passcode / PIN
}

// Available reports the device's biometry. When it is None, err says why
// (ErrNotAvailable, ErrNotEnrolled, ErrLockedOut).
func Available(a *tango.App) (Kind, error) {
	b, err := capcall.Backend[platform.Biometrics](a)
	if err != nil {
		return None, err
	}
	return capcall.Sync(a, b.BiometryKind)
}

// Authenticate shows the system prompt and waits; nil means the owner was
// verified.
func Authenticate(a *tango.App, o Options) error {
	b, err := capcall.Backend[platform.Biometrics](a)
	if err != nil {
		return err
	}
	if o.Reason == "" {
		o.Reason = "Confirm it's you"
	}
	opts := platform.BiometricOptions{Reason: o.Reason, Title: o.Title, CancelTitle: o.CancelTitle, AllowPasscode: o.AllowPasscode}
	_, err = capcall.Async(a, func(done func(struct{}, error)) {
		b.Authenticate(opts, func(err error) { done(struct{}{}, err) })
	})
	return err
}
