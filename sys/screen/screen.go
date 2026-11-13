// Package screen lists the displays attached to the computer.
//
// Coordinates are logical (DIP) pixels with the origin at the top-left
// corner of the primary display; displays left of or above it have
// negative coordinates. Scale is the ratio of physical to logical pixels.
//
// Functions are safe from any goroutine once the app runs. They return
// ErrUnsupported when the backend cannot list displays (mobile, minimal
// backends) and tango.ErrNotRunning after the app quit. Display changes
// (a monitor plugged in, the arrangement or scale changed) arrive as
// tango.EventDisplaysChanged; OnChange is a shortcut for listening to them.
package screen

import (
	"errors"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

// ErrNoDisplay is returned when the system reports no display at all
// (a headless session).
var ErrNoDisplay = errors.New("tango: no display")

// Rect is a rectangle in logical pixels.
type Rect = platform.Rect

// Display is one monitor. WorkArea excludes the menu bar, dock and
// taskbars.
type Display = platform.Display

type result struct {
	ds  []Display
	ran bool
}

// All returns every display; the primary one is first.
func All(a *tango.App) ([]Display, error) {
	d, ok := a.Backend().(platform.Displays)
	if !ok {
		return nil, ErrUnsupported
	}
	r := tango.Await(a, func() result { return result{d.Displays(), true} })
	if !r.ran {
		return nil, tango.ErrNotRunning
	}
	return primaryFirst(r.ds), nil
}

// Primary returns the primary display (the one with the menu bar on
// macOS, the main taskbar on Windows).
func Primary(a *tango.App) (Display, error) {
	ds, err := All(a)
	if err != nil {
		return Display{}, err
	}
	if len(ds) == 0 {
		return Display{}, ErrNoDisplay
	}
	return ds[0], nil
}

// At returns the display containing the point (x, y), or the display
// nearest to it when the point is on none.
func At(a *tango.App, x, y int) (Display, error) {
	ds, err := All(a)
	if err != nil {
		return Display{}, err
	}
	d, ok := nearest(ds, x, y)
	if !ok {
		return Display{}, ErrNoDisplay
	}
	return d, nil
}

// OnChange calls fn on the main thread whenever displays are added,
// removed, rearranged or rescaled.
func OnChange(a *tango.App, fn func()) {
	a.OnEvent(func(ev tango.AppEvent) {
		if ev.Kind == tango.EventDisplaysChanged {
			fn()
		}
	})
}

// primaryFirst moves the primary display to the front, keeping the order
// of the others. If none is marked primary the first one becomes it.
func primaryFirst(ds []Display) []Display {
	if len(ds) == 0 {
		return ds
	}
	p := 0
	for i, d := range ds {
		if d.Primary {
			p = i
			break
		}
	}
	out := make([]Display, 0, len(ds))
	out = append(out, ds[p])
	out[0].Primary = true
	for i, d := range ds {
		if i != p {
			d.Primary = false
			out = append(out, d)
		}
	}
	return out
}

func contains(r Rect, x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

// distance2 is the squared distance from (x, y) to the nearest point of r.
func distance2(r Rect, x, y int) int64 {
	dx := int64(0)
	if x < r.X {
		dx = int64(r.X - x)
	} else if x >= r.X+r.Width {
		dx = int64(x - (r.X + r.Width - 1))
	}
	dy := int64(0)
	if y < r.Y {
		dy = int64(r.Y - y)
	} else if y >= r.Y+r.Height {
		dy = int64(y - (r.Y + r.Height - 1))
	}
	return dx*dx + dy*dy
}

func nearest(ds []Display, x, y int) (Display, bool) {
	best, bestD := -1, int64(0)
	for i, d := range ds {
		if contains(d.Bounds, x, y) {
			return d, true
		}
		if dd := distance2(d.Bounds, x, y); best < 0 || dd < bestD {
			best, bestD = i, dd
		}
	}
	if best < 0 {
		return Display{}, false
	}
	return ds[best], true
}
