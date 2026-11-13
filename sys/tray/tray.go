// Package tray shows an icon in the system tray (macOS menu bar extras,
// Windows notification area, Linux AppIndicator / StatusNotifierItem).
//
//	t, err := tray.New(a, tray.Options{Icon: png, Template: true, Tooltip: "My App"})
//
// The menu bar on macOS wants a monochrome template image while Windows and
// Linux show a coloured icon; give both and each platform picks its own:
//
//	tray.Options{Icon: colourPNG, MacIcon: templatePNG, Template: true}
//	m := menu.New()
//	m.Add("Open", show)
//	m.Separator()
//	m.Role(menu.Quit)
//	t.SetMenu(m)
//	t.OnClick(show)
//
// With a menu set, by default a primary click opens it on macOS and Linux
// and a right click opens it on Windows; OnClick then fires only where the
// platform separates the two (Windows left click). Options.MenuOn changes
// that: MenuOnSecondary opens the menu on a right click and reports left
// clicks with OnClick (macOS, Windows); MenuOnPrimary opens it on a left
// click everywhere. Without a menu, every click reaches the handlers.
// On Linux the icon is a StatusNotifierItem (KDE, GNOME with the
// AppIndicator extension, most other panels): clicks and MenuOn work as
// above, and Bounds is the point of the last primary click (1x1). Where
// no StatusNotifierWatcher runs (or TANGO_LINUX_TRAY=appindicator) it
// falls back to libayatana-appindicator, which reports neither clicks nor
// bounds and opens the menu on any click; set a menu there.
//
// All methods are safe from any goroutine; handlers run on the main
// thread. Changing the tray menu's items takes effect with Menu.Update.
package tray

import (
	"runtime"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/menu"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// Rect is a screen rectangle in logical px, origin top-left of the primary
// display.
type Rect = platform.Rect

// MenuTrigger chooses which click opens the tray menu.
type MenuTrigger = platform.TrayMenuTrigger

const (
	MenuOnDefault   = platform.TrayMenuDefault   // left click on macOS and Linux, right click on Windows
	MenuOnPrimary   = platform.TrayMenuPrimary   // left click; OnClick does not fire while a menu is set
	MenuOnSecondary = platform.TrayMenuSecondary // right click; left clicks reach OnClick
)

// Options configures a tray icon.
type Options struct {
	Icon []byte // PNG; 32x32 or 44x44 px (for 2x displays) works well
	// MacIcon, if set, is shown on macOS instead of Icon: typically a black
	// on transparent PNG (36x36 or 44x44 px) used with Template.
	MacIcon  []byte
	Template bool // macOS: the image is a monochrome template that follows the menu bar's appearance
	Tooltip  string
	Title    string      // macOS: text shown next to the icon
	MenuOn   MenuTrigger // which click opens the menu; see the package comment
}

// Tray is one tray icon. Create it with New.
type Tray struct {
	app *tango.App

	mu      sync.Mutex
	opts    Options
	onClick []func()
	onRight []func()
	onDbl   []func()
	removed bool

	// Main thread only.
	native  platform.Tray
	onMenu  func(id uint32)
	hasMenu bool
}

// New adds a tray icon. It returns ErrUnsupported where the platform has no
// tray (or, on Linux, neither a StatusNotifierWatcher nor an
// AppIndicator library is available).
func New(a *tango.App, o Options) (*Tray, error) {
	tb, ok := a.Backend().(platform.Trays)
	if !ok {
		return nil, ErrUnsupported
	}
	t := &Tray{app: a, opts: o}
	h := platform.TrayHandlers{
		OnClick:       func() { t.fire(&t.onClick) },
		OnRightClick:  func() { t.fire(&t.onRight) },
		OnDoubleClick: func() { t.fire(&t.onDbl) },
		OnMenu: func(id uint32) {
			if t.onMenu != nil {
				t.onMenu(id)
			}
		},
	}
	type result struct {
		n   platform.Tray
		err error
	}
	r, ran := awaitResult(a, func() result {
		n, err := tb.NewTray(o.platform(), h)
		return result{n, err}
	})
	if !ran {
		return nil, tango.ErrNotRunning
	}
	if r.err != nil {
		return nil, r.err
	}
	t.native = r.n
	return t, nil
}

func awaitResult[T any](a *tango.App, fn func() T) (T, bool) {
	type wrapped struct {
		v  T
		ok bool
	}
	w := tango.Await(a, func() wrapped { return wrapped{fn(), true} })
	return w.v, w.ok
}

func (o Options) platform() platform.TrayOptions {
	icon := o.Icon
	if goos == "darwin" && len(o.MacIcon) > 0 {
		icon = o.MacIcon
	}
	return platform.TrayOptions{Icon: icon, Template: o.Template, Tooltip: o.Tooltip, Title: o.Title, MenuOn: o.MenuOn}
}

// goos is runtime.GOOS; tests replace it.
var goos = runtime.GOOS

// fire runs the handlers in list; the backend calls it directly, so each
// handler is guarded on its own (Where "tray click"): a panic in one does
// not skip the rest.
func (t *Tray) fire(list *[]func()) {
	t.mu.Lock()
	fns := append([]func(){}, *list...)
	t.mu.Unlock()
	for _, fn := range fns {
		corehook.Guard(t.app, "tray click", fn)
	}
}

// update changes the options and sends them to the native icon.
func (t *Tray) update(change func(o *Options)) {
	t.mu.Lock()
	if t.removed {
		t.mu.Unlock()
		return
	}
	change(&t.opts)
	t.mu.Unlock()
	t.app.Post(func() {
		t.mu.Lock()
		o, removed := t.opts, t.removed
		t.mu.Unlock()
		if !removed {
			t.native.SetOptions(o.platform())
		}
	})
}

// SetIcon replaces the icon (PNG); template as in Options.Template.
func (t *Tray) SetIcon(png []byte, template bool) {
	t.update(func(o *Options) { o.Icon, o.Template = png, template })
}

// SetMacIcon replaces the macOS image (Options.MacIcon); nil falls back to
// Icon.
func (t *Tray) SetMacIcon(png []byte) { t.update(func(o *Options) { o.MacIcon = png }) }

// SetTooltip sets the hover text.
func (t *Tray) SetTooltip(text string) { t.update(func(o *Options) { o.Tooltip = text }) }

// SetTitle sets the text next to the icon (macOS).
func (t *Tray) SetTitle(text string) { t.update(func(o *Options) { o.Title = text }) }

// SetMenuOn chooses which click opens the menu (Options.MenuOn).
func (t *Tray) SetMenuOn(m MenuTrigger) { t.update(func(o *Options) { o.MenuOn = m }) }

// SetMenu attaches m to the icon (nil removes the menu). Later changes to
// m are shown after m.Update().
func (t *Tray) SetMenu(m *menu.Menu) error {
	t.mu.Lock()
	removed := t.removed
	t.mu.Unlock()
	if removed {
		return nil
	}
	var mm any
	if m != nil {
		mm = m
	}
	return corehook.AttachMenu(t.app, t, mm, func(items []platform.MenuItem, onClick func(uint32)) {
		t.mu.Lock()
		removed := t.removed
		t.mu.Unlock()
		if removed {
			return
		}
		t.onMenu = onClick
		if items == nil {
			if t.hasMenu {
				t.native.SetMenu(nil)
			}
			t.hasMenu = false
			return
		}
		t.hasMenu = true
		t.native.SetMenu(items)
	})
}

// OnClick adds fn for a primary click (see the package comment for when
// platforms report it).
func (t *Tray) OnClick(fn func()) { t.mu.Lock(); t.onClick = append(t.onClick, fn); t.mu.Unlock() }

// OnRightClick adds fn for a secondary click.
func (t *Tray) OnRightClick(fn func()) { t.mu.Lock(); t.onRight = append(t.onRight, fn); t.mu.Unlock() }

// OnDoubleClick adds fn for a double click (Windows; macOS reports it
// after the two single clicks).
func (t *Tray) OnDoubleClick(fn func()) { t.mu.Lock(); t.onDbl = append(t.onDbl, fn); t.mu.Unlock() }

// Bounds returns the icon's rectangle on screen, if the platform knows it
// (useful to place a popover window under it; see sys/flyout). On Linux
// it is the 1x1 point of the last click on the icon.
func (t *Tray) Bounds() (Rect, bool) {
	type result struct {
		r  Rect
		ok bool
	}
	r := tango.Await(t.app, func() result {
		t.mu.Lock()
		removed := t.removed
		t.mu.Unlock()
		if removed {
			return result{}
		}
		rc, ok := t.native.Bounds()
		return result{rc, ok}
	})
	return r.r, r.ok
}

// Remove takes the icon away. The Tray is unusable afterwards.
func (t *Tray) Remove() {
	t.mu.Lock()
	if t.removed {
		t.mu.Unlock()
		return
	}
	t.removed = true
	t.mu.Unlock()
	corehook.DetachMenu(t)
	t.app.Post(func() {
		t.onMenu = nil
		t.native.Remove()
	})
}
