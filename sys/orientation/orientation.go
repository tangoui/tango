// Package orientation reads and locks the interface orientation.
//
//	o, _ := orientation.Get(a)            // orientation.Portrait, ...
//	orientation.Lock(a, orientation.Landscape)
//	orientation.Unlock(a)
//	orientation.OnChange(a, func(o orientation.Orientation) { ... })
//
// Locking only chooses among the orientations the app declares
// (UISupportedInterfaceOrientations on iOS; Android allows all). On iPad,
// apps that support multitasking (split view) cannot lock; iOS ignores
// the request there.
//
// Functions return ErrUnsupported on desktop and tango.ErrNotRunning after
// the app quit.
package orientation

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var ErrUnsupported = platform.ErrUnsupported

// Orientation is named by where the device's top edge points:
// LandscapeLeft is the device turned 90° counter-clockwise from portrait.
type Orientation = platform.Orientation

const (
	Unknown            = platform.OrientationUnknown
	Portrait           = platform.OrientationPortrait
	PortraitUpsideDown = platform.OrientationPortraitUpsideDown
	LandscapeLeft      = platform.OrientationLandscapeLeft
	LandscapeRight     = platform.OrientationLandscapeRight
)

// Mask is a set of orientations for Lock.
type Mask = platform.OrientationMask

const (
	MaskPortrait           = platform.MaskPortrait
	MaskPortraitUpsideDown = platform.MaskPortraitUpsideDown
	MaskLandscapeLeft      = platform.MaskLandscapeLeft
	MaskLandscapeRight     = platform.MaskLandscapeRight
	Landscape              = platform.MaskLandscape // either landscape
	PortraitOnly           = platform.MaskPortrait
	All                    = platform.MaskAll
)

// String names o: "portrait", "portraitUpsideDown", "landscapeLeft",
// "landscapeRight" or "unknown".
func String(o Orientation) string {
	switch o {
	case Portrait:
		return "portrait"
	case PortraitUpsideDown:
		return "portraitUpsideDown"
	case LandscapeLeft:
		return "landscapeLeft"
	case LandscapeRight:
		return "landscapeRight"
	}
	return "unknown"
}

// IsLandscape reports whether o is a landscape orientation.
func IsLandscape(o Orientation) bool { return o == LandscapeLeft || o == LandscapeRight }

// Get returns the current interface orientation.
func Get(a *tango.App) (Orientation, error) {
	o, err := capcall.Backend[platform.Orientations](a)
	if err != nil {
		return Unknown, err
	}
	return capcall.Sync(a, func() (Orientation, error) { return o.Orientation(), nil })
}

// Lock restricts the interface to m, rotating if needed.
func Lock(a *tango.App, m Mask) error {
	o, err := capcall.Backend[platform.Orientations](a)
	if err != nil {
		return err
	}
	m &= All
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, o.LockOrientation(m) })
	return err
}

// Unlock lifts a Lock.
func Unlock(a *tango.App) error { return Lock(a, 0) }

// OnChange calls fn on the main thread with the new orientation whenever
// it changes. It does nothing on desktop.
func OnChange(a *tango.App, fn func(Orientation)) {
	if _, ok := a.Backend().(platform.Orientations); !ok {
		return
	}
	a.OnEventKind(tango.EventOrientationChanged, func(ev tango.AppEvent) { fn(ev.Orientation) })
}
