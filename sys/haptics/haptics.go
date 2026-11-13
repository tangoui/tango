// Package haptics plays haptic feedback: impacts, selection ticks and
// notification patterns (UIFeedbackGenerator on iOS,
// View.performHapticFeedback on Android, which needs no permission).
//
//	haptics.Play(a, haptics.Success)
//
// Devices without a haptic engine (iPad, the iOS simulator, most desktops)
// silently do nothing; the call still succeeds where the API exists.
// Play returns ErrUnsupported where the backend has no haptics and
// tango.ErrNotRunning after the app quit. It does not wait for the
// pattern to finish.
package haptics

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var ErrUnsupported = platform.ErrUnsupported

// Kind is a feedback pattern.
type Kind = platform.HapticKind

const (
	Light     = platform.HapticLight  // light impact
	Medium    = platform.HapticMedium // medium impact
	Heavy     = platform.HapticHeavy  // heavy impact
	Selection = platform.HapticSelection
	Success   = platform.HapticSuccess
	Warning   = platform.HapticWarning
	Error     = platform.HapticError
)

// Play plays k.
func Play(a *tango.App, k Kind) error {
	h, err := capcall.Backend[platform.Haptics](a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, h.Haptic(k) })
	return err
}
