// Package flyout shows a borderless panel under a tray icon — a popover in
// the macOS menu bar, a flyout above the Windows notification area — that
// hides itself when it loses focus.
//
//	t, _ := tray.New(a, tray.Options{Icon: png, Template: true, MenuOn: tray.MenuOnSecondary})
//	f, err := flyout.New(a, t, flyout.Options{Width: 320, Height: 420, URL: "/panel.html"})
//	// a left click on the icon now toggles the panel; f.Surface() is the page
//
// The panel is an ordinary *tango.Surface (KindPanel, TitleBarNone,
// always on top, hidden until opened), so it can host a WebView page or,
// with Options.Surface, a surface the app created itself (a Go-drawn
// canvas surface, say). Open places it against the tray icon's bounds:
// below the icon when the icon is in the top half of its display (macOS
// menu bar, a top panel), above it when in the bottom half (Windows
// taskbar), beside it for vertical taskbars; always inside the display's
// work area. Where the platform does not report tray bounds (Linux
// AppIndicator) it goes to the top-right corner of the primary display's
// work area (bottom-right on Windows).
//
// With a tray menu, set tray.Options.MenuOn = tray.MenuOnSecondary so left
// clicks reach the flyout and right clicks open the menu.
//
// Methods are safe from any goroutine once the app runs.
package flyout

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/sys/screen"
	"github.com/tangoui/tango/sys/tray"
)

// Rect is a screen rectangle in logical pixels.
type Rect = tango.Rect

// Options configures a flyout.
type Options struct {
	Width, Height int    // panel size; default 320x400
	URL           string // page of the panel (relative to the asset origin); ignored with Surface
	Debug         bool   // allow the inspector
	Transparent   bool   // transparent window and page (round the corners in CSS)
	Material      tango.Material
	// Gap is the distance between the icon and the panel; default 6 px.
	Gap int
	// KeepOnBlur keeps the panel open when it loses focus.
	KeepOnBlur bool
	// NoToggle stops a primary click on the icon from toggling the panel;
	// call Open/Close/Toggle yourself.
	NoToggle bool
	// Surface, if set, is used as the panel instead of creating one. It
	// should be frameless and hidden; Close hides it, it is never closed.
	Surface *tango.Surface
}

// Flyout is a panel attached to a tray icon.
type Flyout struct {
	a    *tango.App
	t    *tray.Tray
	s    *tango.Surface
	opts Options

	mu       sync.Mutex
	open     bool
	closed   bool      // the surface was closed
	blurHide time.Time // when the panel last hid itself on blur
	onOpen   []func()
	onClose  []func()
}

// ErrClosed is returned by Open after the panel's surface was closed.
var ErrClosed = errors.New("tango/flyout: the panel surface was closed")

// now is replaced by tests.
var now = time.Now

// reopenGuard: a click on the tray icon first blurs the open panel (which
// hides it) and then reports the click; a click this soon after a blur
// hide is the same gesture and must not reopen it.
const reopenGuard = 400 * time.Millisecond

// New creates the panel (hidden) and attaches it to t; t may be nil, in
// which case Open uses the fallback corner.
func New(a *tango.App, t *tray.Tray, o Options) (*Flyout, error) {
	if o.Width <= 0 {
		o.Width = 320
	}
	if o.Height <= 0 {
		o.Height = 400
	}
	if o.Gap == 0 {
		o.Gap = 6
	}
	f := &Flyout{a: a, t: t, opts: o, s: o.Surface}
	if f.s == nil {
		s, err := a.NewSurface(tango.SurfaceOptions{
			Width: o.Width, Height: o.Height,
			URL: o.URL, Debug: o.Debug, Hidden: true,
			Kind: tango.KindPanel, X: -10000, Y: -10000,
			Appearance: tango.Appearance{
				TitleBar:    tango.TitleBarNone,
				AlwaysOnTop: true,
				Transparent: o.Transparent,
				Material:    o.Material,
				NoMinimize:  true,
				NoMaximize:  true,
			},
		})
		if err != nil {
			return nil, err
		}
		f.s = s
	}
	f.s.OnBlurred(f.blurred)
	f.s.OnClose(func() bool { f.Close(); return false }) // Cmd+W / Alt+F4 hide it
	f.s.OnClosed(func() {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
		f.setOpen(false, false)
	})
	if t != nil && !o.NoToggle {
		t.OnClick(f.Toggle)
	}
	return f, nil
}

// Surface is the panel's surface.
func (f *Flyout) Surface() *tango.Surface { return f.s }

// OnOpen adds fn to run (on the main thread) after the panel opened.
func (f *Flyout) OnOpen(fn func()) { f.mu.Lock(); f.onOpen = append(f.onOpen, fn); f.mu.Unlock() }

// OnClose adds fn to run after the panel hid (Close, blur, Cmd+W).
func (f *Flyout) OnClose(fn func()) { f.mu.Lock(); f.onClose = append(f.onClose, fn); f.mu.Unlock() }

// IsOpen reports whether the panel is showing.
func (f *Flyout) IsOpen() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.open }

// Open places the panel at the tray icon and shows and focuses it.
func (f *Flyout) Open() error {
	anchor, ok := Rect{}, false
	if f.t != nil {
		anchor, ok = f.t.Bounds()
	}
	return f.OpenAt(anchor, ok)
}

// OpenAt is Open against anchor instead of the tray icon's bounds (for
// example a point the app knows the user clicked: a 1x1 rect). With
// hasAnchor false it uses the fallback corner.
func (f *Flyout) OpenAt(anchor Rect, hasAnchor bool) error {
	work, haveWork := f.workArea(anchor, hasAnchor)
	x, y := Place(anchor, hasAnchor, work, haveWork, f.opts.Width, f.opts.Height, f.opts.Gap, runtime.GOOS == "windows")
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if err := f.s.SetBounds(Rect{X: x, Y: y, Width: f.opts.Width, Height: f.opts.Height}); err != nil && !errors.Is(err, tango.ErrUnsupported) {
		return err
	}
	f.s.Show()
	f.s.Focus()
	f.setOpen(true, false)
	return nil
}

func (f *Flyout) workArea(anchor Rect, hasAnchor bool) (Rect, bool) {
	var d screen.Display
	var err error
	if hasAnchor {
		d, err = screen.At(f.a, anchor.X+anchor.Width/2, anchor.Y+anchor.Height/2)
	} else {
		d, err = screen.Primary(f.a)
	}
	if err != nil || d.WorkArea.Width <= 0 || d.WorkArea.Height <= 0 {
		return Rect{}, false
	}
	return d.WorkArea, true
}

// Close hides the panel.
func (f *Flyout) Close() {
	f.s.Hide()
	f.setOpen(false, false)
}

// Toggle opens the panel if it is hidden and closes it otherwise. A
// toggle right after the panel hid itself on blur — the click on the tray
// icon that caused the blur — leaves it closed.
func (f *Flyout) Toggle() {
	f.mu.Lock()
	open := f.open
	recent := !f.blurHide.IsZero() && now().Sub(f.blurHide) < reopenGuard
	f.mu.Unlock()
	switch {
	case open:
		f.Close()
	case recent:
		f.mu.Lock()
		f.blurHide = time.Time{}
		f.mu.Unlock()
	default:
		_ = f.Open()
	}
}

func (f *Flyout) blurred() {
	if f.opts.KeepOnBlur || !f.IsOpen() {
		return
	}
	f.s.Hide()
	f.setOpen(false, true)
}

func (f *Flyout) setOpen(open, byBlur bool) {
	f.mu.Lock()
	changed := f.open != open
	f.open = open
	if byBlur {
		f.blurHide = now()
	}
	var fns []func()
	if changed {
		if open {
			fns = append(fns, f.onOpen...)
		} else {
			fns = append(fns, f.onClose...)
		}
	}
	f.mu.Unlock()
	// setOpen runs from tray clicks, surface callbacks and the app's own
	// Open/Close calls: guard each handler on its own so a panic in one
	// does not skip the rest.
	for _, fn := range fns {
		corehook.Guard(f.a, "event handler", fn)
	}
}

// Place computes the panel's top-left corner for a w x h panel against
// anchor (the tray icon) inside work (the work area of the anchor's
// display). Without an anchor the panel goes to the top-right corner of
// work, or the bottom-right when bottomFallback (Windows). Without a work
// area it is not clamped.
func Place(anchor Rect, hasAnchor bool, work Rect, hasWork bool, w, h, gap int, bottomFallback bool) (x, y int) {
	if !hasAnchor {
		if !hasWork {
			return 0, 0
		}
		x = work.X + work.Width - w - gap
		y = work.Y + gap
		if bottomFallback {
			y = work.Y + work.Height - h - gap
		}
		return clamp(x, y, w, h, work)
	}
	cx := anchor.X + anchor.Width/2
	cy := anchor.Y + anchor.Height/2
	if !hasWork {
		return cx - w/2, anchor.Y + anchor.Height + gap
	}
	// Which edge is the icon on? Taskbars and menu bars lie outside the
	// work area, so the side the icon's center is outside of decides;
	// otherwise (auto-hiding bars) the nearest edge of the work area.
	edge := nearestEdge(cx, cy, work)
	switch {
	case cy < work.Y:
		edge = edgeTop
	case cy >= work.Y+work.Height:
		edge = edgeBottom
	case cx < work.X:
		edge = edgeLeft
	case cx >= work.X+work.Width:
		edge = edgeRight
	}
	switch edge {
	case edgeTop:
		x, y = cx-w/2, max(anchor.Y+anchor.Height, work.Y)+gap
	case edgeBottom:
		x, y = cx-w/2, min(anchor.Y, work.Y+work.Height)-h-gap
	case edgeLeft:
		x, y = max(anchor.X+anchor.Width, work.X)+gap, cy-h/2
	default:
		x, y = min(anchor.X, work.X+work.Width)-w-gap, cy-h/2
	}
	return clamp(x, y, w, h, work)
}

type edge int

const (
	edgeTop edge = iota
	edgeBottom
	edgeLeft
	edgeRight
)

func nearestEdge(cx, cy int, r Rect) edge {
	e, d := edgeTop, cy-r.Y
	if v := r.Y + r.Height - cy; v < d {
		e, d = edgeBottom, v
	}
	if v := cx - r.X; v < d {
		e, d = edgeLeft, v
	}
	if v := r.X + r.Width - cx; v < d {
		e = edgeRight
	}
	return e
}

func clamp(x, y, w, h int, r Rect) (int, int) {
	if x+w > r.X+r.Width {
		x = r.X + r.Width - w
	}
	if x < r.X {
		x = r.X
	}
	if y+h > r.Y+r.Height {
		y = r.Y + r.Height - h
	}
	if y < r.Y {
		y = r.Y
	}
	return x, y
}
