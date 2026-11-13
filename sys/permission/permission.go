// Package permission queries and requests runtime permissions with one
// API on iOS and Android.
//
//	st, err := permission.Check(a, permission.Camera)
//	if st == permission.NotDetermined {
//		st, err = permission.Request(a, permission.Camera)
//	}
//
// Statuses: Granted, Denied (refused, restricted by policy, or "don't ask
// again" on Android), Limited (selected photos only, approximate location,
// provisional notifications) and NotDetermined (never asked: Request will
// prompt). Request does not prompt again once the user decided; send the
// user to the system settings instead.
//
// The app must declare what it asks for — an Info.plist usage description
// on iOS (Request returns ErrNotConfigured instead of crashing without
// it), a manifest permission on Android. See docs/mobile-permissions.md;
// tan generates them from "permissions" in tan.json.
//
// Functions return ErrUnsupported where the backend has no runtime
// permissions (desktop) or does not know the permission, and
// tango.ErrNotRunning after the app quit. On Android call Request from a
// goroutine, not from a main-thread callback.
package permission

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported   = platform.ErrUnsupported
	ErrNotConfigured = platform.ErrNotConfigured
)

// Kind names a permission (the same names as tan.json "permissions").
type Kind = platform.Permission

const (
	Camera         = platform.PermCamera
	Photos         = platform.PermPhotos
	Location       = platform.PermLocation       // while the app is in use
	LocationAlways = platform.PermLocationAlways // also in the background
	Notifications  = platform.PermNotifications
	Microphone     = platform.PermMicrophone
)

// Status of a permission: "granted", "denied", "limited", "notDetermined".
type Status = platform.PermissionStatus

const (
	Granted       = platform.StatusGranted
	Denied        = platform.StatusDenied
	Limited       = platform.StatusLimited
	NotDetermined = platform.StatusNotDetermined
)

// All lists every Kind.
var All = []Kind{Camera, Photos, Location, LocationAlways, Notifications, Microphone}

// Check returns the current status without prompting.
func Check(a *tango.App, k Kind) (Status, error) {
	p, err := capcall.Backend[platform.Permissions](a)
	if err != nil {
		return NotDetermined, err
	}
	return capcall.Async(a, func(done func(Status, error)) { p.CheckPermission(k, done) })
}

// Request prompts the user if they were never asked and returns the
// resulting status.
func Request(a *tango.App, k Kind) (Status, error) {
	p, err := capcall.Backend[platform.Permissions](a)
	if err != nil {
		return NotDetermined, err
	}
	return capcall.Async(a, func(done func(Status, error)) { p.RequestPermission(k, done) })
}
