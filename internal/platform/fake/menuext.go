package fake

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Context menus and tray icons (platform.PopupMenu, platform.Trays).
//
// State lives beside Backend (keyed by it) so this file needs no fields in
// the shared Backend struct.

var (
	_ platform.PopupMenu = (*Backend)(nil)
	_ platform.Trays     = (*Backend)(nil)
)

type menuExt struct {
	mu     sync.Mutex
	popups []*Popup
	trays  []*Tray
}

var menuExts sync.Map // *Backend → *menuExt

func (b *Backend) menuExt() *menuExt {
	v, _ := menuExts.LoadOrStore(b, &menuExt{})
	return v.(*menuExt)
}

// Popup is a context menu shown with PopupMenu. It stays open until the
// test calls Choose or Dismiss.
type Popup struct {
	b       *Backend
	Surface platform.SurfaceID
	X, Y    int
	Items   []platform.MenuItem

	onClick func(uint32)
	onClose func()
	mu      sync.Mutex
	closed  bool
}

func (b *Backend) PopupMenu(surface platform.SurfaceID, x, y int, items []platform.MenuItem, onClick func(uint32), onClose func()) {
	b.mustMain("PopupMenu")
	p := &Popup{b: b, Surface: surface, X: x, Y: y, Items: items, onClick: onClick, onClose: onClose}
	e := b.menuExt()
	e.mu.Lock()
	e.popups = append(e.popups, p)
	e.mu.Unlock()
}

// Popups returns every popup shown, in order (closed ones included).
func (b *Backend) Popups() []*Popup {
	e := b.menuExt()
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.popups)
}

// Choose simulates the user picking item id: onClick(id) then onClose, as
// one native event. It reports false if the popup was already closed.
func (p *Popup) Choose(id uint32) bool { return p.finish(func() { p.onClick(id) }) }

// Dismiss simulates the user closing the popup without choosing.
func (p *Popup) Dismiss() bool { return p.finish(nil) }

func (p *Popup) finish(click func()) bool {
	ok := false
	p.b.Do(func() {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		p.closed = true
		p.mu.Unlock()
		if click != nil && p.onClick != nil {
			click()
		}
		if p.onClose != nil {
			p.onClose()
		}
		ok = true
	})
	return ok
}

// Closed reports whether the popup was chosen from or dismissed.
func (p *Popup) Closed() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.closed }

// Tray is a fake tray icon. Accessors are safe from any goroutine;
// the simulators (Click, ChooseMenu, ...) run as native events.
type Tray struct {
	b *Backend
	h platform.TrayHandlers

	mu      sync.Mutex
	opts    platform.TrayOptions
	items   []platform.MenuItem
	removed bool
	sets    int // SetMenu calls
}

// TrayBounds is what every fake tray's Bounds reports.
var TrayBounds = platform.Rect{X: 1000, Y: 0, Width: 22, Height: 22}

func (b *Backend) NewTray(o platform.TrayOptions, h platform.TrayHandlers) (platform.Tray, error) {
	b.mustMain("NewTray")
	t := &Tray{b: b, h: h, opts: o}
	e := b.menuExt()
	e.mu.Lock()
	e.trays = append(e.trays, t)
	e.mu.Unlock()
	return t, nil
}

// Trays returns every tray created, in order (removed ones included).
func (b *Backend) Trays() []*Tray {
	e := b.menuExt()
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.trays)
}

func (t *Tray) SetOptions(o platform.TrayOptions) {
	t.b.mustMain("Tray.SetOptions")
	t.mu.Lock()
	t.opts = o
	t.mu.Unlock()
}

func (t *Tray) SetMenu(items []platform.MenuItem) {
	t.b.mustMain("Tray.SetMenu")
	t.mu.Lock()
	t.items = items
	t.sets++
	t.mu.Unlock()
}

func (t *Tray) Bounds() (platform.Rect, bool) {
	t.b.mustMain("Tray.Bounds")
	return TrayBounds, true
}

func (t *Tray) Remove() {
	t.b.mustMain("Tray.Remove")
	t.mu.Lock()
	t.removed = true
	t.mu.Unlock()
}

func (t *Tray) Options() platform.TrayOptions { t.mu.Lock(); defer t.mu.Unlock(); return t.opts }
func (t *Tray) Menu() []platform.MenuItem     { t.mu.Lock(); defer t.mu.Unlock(); return t.items }
func (t *Tray) MenuSets() int                 { t.mu.Lock(); defer t.mu.Unlock(); return t.sets }
func (t *Tray) Removed() bool                 { t.mu.Lock(); defer t.mu.Unlock(); return t.removed }

func (t *Tray) fire(fn func()) bool {
	if fn == nil || t.Removed() {
		return false
	}
	return t.b.Do(fn)
}

// Click, RightClick and DoubleClick simulate the user; they report false if
// the handler is unset, the tray was removed or the loop stopped.
func (t *Tray) Click() bool       { return t.fire(t.h.OnClick) }
func (t *Tray) RightClick() bool  { return t.fire(t.h.OnRightClick) }
func (t *Tray) DoubleClick() bool { return t.fire(t.h.OnDoubleClick) }

// ChooseMenu simulates the user picking item id from the tray menu.
func (t *Tray) ChooseMenu(id uint32) bool {
	if t.h.OnMenu == nil {
		return false
	}
	return t.fire(func() { t.h.OnMenu(id) })
}
