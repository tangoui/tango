// Package macoslite is a minimal macOS-native window host for small utility
// windows (settings, updaters, prompts). It talks to AppKit, CoreGraphics and
// CoreText directly through purego and does not load the Tango App, WebView,
// ui.Runner, layout, accessibility or GPU renderer, so an idle window costs
// far less memory than a full Tango surface.
//
// Experimental: the API may change incompatibly between minor releases.
//
// It is independent of uihost.Open and does not replace it. On platforms
// other than macOS, New returns ErrUnsupported. See
// docs/specs/macos-native-lite-design.md.
package macoslite

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsupported is returned by New on platforms other than macOS.
	ErrUnsupported = errors.New("macoslite: unsupported platform")
	// ErrInvalidOptions wraps option validation failures.
	ErrInvalidOptions = errors.New("macoslite: invalid options")
	// ErrNilView is returned when New is given a nil View.
	ErrNilView = errors.New("macoslite: nil view")
)

// Options configures the single window of an App.
type Options struct {
	Name, Title   string
	Width, Height float64 // content size in points; both must be > 0
}

// Frame describes the frame being drawn, in points; Scale is the backing
// scale factor (2 on Retina).
type Frame struct {
	Width, Height, Scale float64
}

// View receives drawing and input callbacks on the main thread.
type View interface {
	Draw(*Frame, *Canvas)
	Mouse(MouseEvent)
	Key(KeyEvent)
	Close()
}

// Point is a position in points, origin top-left.
type Point struct{ X, Y float64 }

// Rect is an axis-aligned rectangle in points, origin top-left.
type Rect struct{ X, Y, W, H float64 }

// Contains reports whether p lies in r; the right and bottom edges are
// excluded.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.W && p.Y < r.Y+r.H
}

// Color is a non-premultiplied 8-bit RGBA color.
type Color struct{ R, G, B, A uint8 }

// MouseKind classifies a MouseEvent.
type MouseKind int

const (
	MouseDown MouseKind = iota
	MouseUp
	MouseMove
)

// MouseEvent is a pointer event in view coordinates.
type MouseEvent struct {
	Kind   MouseKind
	Pos    Point
	Button int
}

// KeyEvent is a key press or release. Key is "Escape", "Enter", "Tab",
// "Backspace" or the typed characters; Code is the hardware key code.
type KeyEvent struct {
	Key  string
	Code uint16
	Down bool
}

func validate(o Options, v View) error {
	switch {
	case v == nil:
		return ErrNilView
	case o.Title == "":
		return fmt.Errorf("%w: empty Title", ErrInvalidOptions)
	case !(o.Width > 0):
		return fmt.Errorf("%w: Width must be > 0", ErrInvalidOptions)
	case !(o.Height > 0):
		return fmt.Errorf("%w: Height must be > 0", ErrInvalidOptions)
	}
	return nil
}
