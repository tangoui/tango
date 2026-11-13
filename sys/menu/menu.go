// Package menu builds native menus in Go and routes their clicks to Go
// callbacks.
//
//	m := menu.New()
//	app := m.Submenu("App")
//	app.Role(menu.About)
//	app.Separator()
//	app.Role(menu.Quit)
//	file := m.Submenu("File")
//	file.Add("New", func() { ... }).Accelerator = "CmdOrCtrl+N"
//	wrap := file.Check("Word Wrap", true, nil)
//	view := m.Submenu("View")
//	view.Radio("size", "Small", false, nil)
//	view.Radio("size", "Large", true, nil)
//	m.Submenu("Edit").Role(menu.EditMenu)
//	err := menu.SetAppMenu(a, m)
//
// The same Menu can be shown in several places: the menu bar
// (SetAppMenu / SetSurfaceMenu), a tray icon (package tray), a context
// menu (Popup, RegisterContext).
//
// # Changing a menu
//
// A Menu is a description. After changing items (SetLabel, SetChecked,
// SetDisabled, SetIcon, or adding items), call Update on the menu, or on
// any submenu of it, to send the new state to every place it is shown.
// Checkable and radio items are the exception: clicking them toggles
// Checked (radio: checks the item and unchecks the others of its group in
// the same submenu) and refreshes the native menus by itself, before
// OnClick runs.
//
// Item fields may be written directly before the menu is first set; after
// that use the setters (any goroutine), or write fields on the main thread
// (inside a click callback, or with App.Post).
//
// Click callbacks run on the main thread. Item ids are allocated
// internally, unique across every menu set.
package menu

import (
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// Role is a platform standard item whose label and action the backend
// supplies.
type Role string

const (
	Quit       Role = "quit"
	About      Role = "about"
	Hide       Role = "hide"
	Copy       Role = "copy"
	Paste      Role = "paste"
	Cut        Role = "cut"
	SelectAll  Role = "selectAll"
	Undo       Role = "undo"
	Redo       Role = "redo"
	Minimize   Role = "minimize"
	Close      Role = "close"
	AppMenu    Role = "appMenu"
	EditMenu   Role = "editMenu"
	WindowMenu Role = "windowMenu"
)

// Menu is a list of items: the menu bar, or a submenu.
type Menu struct {
	Items []*Item
}

// Item is one menu entry.
type Item struct {
	Label       string
	Accelerator string // "CmdOrCtrl+Shift+N"
	Role        Role   // non-empty for a standard item; OnClick is not called
	Separator   bool
	Disabled    bool
	Checked     bool
	// Checkable shows a check mark slot; clicking toggles Checked.
	Checkable bool
	// RadioGroup makes a radio item. Items of the same submenu sharing a
	// group are exclusive: clicking one checks it and unchecks the others.
	RadioGroup string
	// Icon is a PNG shown next to the label where supported (macOS,
	// Windows); 16x16 logical px is typical. Ignored elsewhere.
	Icon    []byte
	Submenu *Menu
	OnClick func() // main thread

	parent *Menu // set by the builders and by every build; guarded by mu
}

// mu guards Item and Menu fields once a menu has been set, and the
// attachment registry (bind.go).
var mu sync.Mutex

// New returns an empty menu.
func New() *Menu { return &Menu{} }

func (m *Menu) add(it *Item) *Item {
	it.parent = m
	m.Items = append(m.Items, it)
	return it
}

// Add appends a clickable item.
func (m *Menu) Add(label string, onClick func()) *Item {
	return m.add(&Item{Label: label, OnClick: onClick})
}

// Check appends a checkable item. Clicking it toggles Checked before
// onClick runs; read the new state with IsChecked.
func (m *Menu) Check(label string, checked bool, onClick func()) *Item {
	return m.add(&Item{Label: label, Checkable: true, Checked: checked, OnClick: onClick})
}

// Radio appends a radio item of group. Clicking it checks it and unchecks
// the other items of group in m before onClick runs.
func (m *Menu) Radio(group, label string, checked bool, onClick func()) *Item {
	return m.add(&Item{Label: label, RadioGroup: group, Checked: checked, OnClick: onClick})
}

// Separator appends a separator.
func (m *Menu) Separator() *Item { return m.add(&Item{Separator: true}) }

// Role appends a standard item.
func (m *Menu) Role(r Role) *Item { return m.add(&Item{Role: r}) }

// Submenu appends an item opening a new submenu and returns the submenu.
func (m *Menu) Submenu(label string) *Menu {
	sub := New()
	m.add(&Item{Label: label, Submenu: sub})
	return sub
}

// SetLabel changes the label. Call Update to show it.
func (it *Item) SetLabel(label string) { mu.Lock(); it.Label = label; mu.Unlock() }

// SetDisabled enables or disables the item. Call Update to show it.
func (it *Item) SetDisabled(disabled bool) { mu.Lock(); it.Disabled = disabled; mu.Unlock() }

// SetIcon sets the PNG icon (nil removes it). Call Update to show it.
func (it *Item) SetIcon(png []byte) { mu.Lock(); it.Icon = png; mu.Unlock() }

// SetChecked checks or unchecks the item. Checking a radio item unchecks
// the others of its group. Call Update to show it.
func (it *Item) SetChecked(checked bool) {
	mu.Lock()
	it.setChecked(checked)
	mu.Unlock()
}

// IsChecked reads Checked; safe from any goroutine.
func (it *Item) IsChecked() bool { mu.Lock(); defer mu.Unlock(); return it.Checked }

// setChecked applies the radio rule. mu held.
func (it *Item) setChecked(checked bool) {
	it.Checked = checked
	if !checked || it.RadioGroup == "" || it.parent == nil {
		return
	}
	for _, o := range it.parent.Items {
		if o != nil && o != it && o.RadioGroup == it.RadioGroup {
			o.Checked = false
		}
	}
}

var nextID atomic.Uint32

// build converts m to the platform description, allocating ids for
// clickable items and recording them in handlers. mu held.
func build(m *Menu, handlers map[uint32]*Item, seen map[*Menu]bool) []platform.MenuItem {
	if m == nil || seen[m] {
		return nil // nil or cyclic submenu
	}
	seen[m] = true
	defer delete(seen, m)
	out := make([]platform.MenuItem, 0, len(m.Items))
	for _, it := range m.Items {
		if it == nil {
			continue
		}
		it.parent = m
		pi := platform.MenuItem{
			Label:       it.Label,
			Accelerator: it.Accelerator,
			Role:        string(it.Role),
			Separator:   it.Separator,
			Disabled:    it.Disabled,
			Checked:     it.Checked,
			Checkable:   it.Checkable,
			RadioGroup:  it.RadioGroup,
			Icon:        it.Icon,
		}
		switch {
		case it.Separator, it.Role != "":
		case it.Submenu != nil:
			pi.Submenu = build(it.Submenu, handlers, seen)
		default:
			id := nextID.Add(1)
			if id == 0 { // wrapped
				id = nextID.Add(1)
			}
			pi.ID = id
			handlers[id] = it
		}
		out = append(out, pi)
	}
	return out
}

// convert builds m under mu and returns the items and a click router that
// applies check/radio state, refreshes attached menus if that changed, then
// runs OnClick. The router runs on the main thread, called by the backend
// directly: a panic in OnClick goes to a's panic handling (Where "menu
// item").
func convert(a *tango.App, m *Menu) ([]platform.MenuItem, func(id uint32)) {
	handlers := map[uint32]*Item{}
	mu.Lock()
	items := build(m, handlers, map[*Menu]bool{})
	mu.Unlock()
	// handlers is read-only from here on.
	return items, func(id uint32) {
		if it := handlers[id]; it != nil {
			clicked(a, it)
		}
	}
}

func clicked(a *tango.App, it *Item) {
	mu.Lock()
	changed := false
	switch {
	case it.RadioGroup != "":
		it.setChecked(true)
		changed = true
	case it.Checkable:
		it.Checked = !it.Checked
		changed = true
	}
	parent, fn := it.parent, it.OnClick
	mu.Unlock()
	if changed && parent != nil {
		_ = parent.Update()
	}
	if fn != nil {
		corehook.Guard(a, "menu item", fn)
	}
}

func set(a *tango.App, parent platform.SurfaceID, m *Menu) error {
	am, ok := appMenuOf(a, parent)
	if !ok {
		return ErrUnsupported
	}
	key := appMenuKey{a, parent}
	if m == nil {
		detach(key)
		// Clearing: send an empty menu.
		if !tango.Await(a, func() bool { am.SetAppMenu(parent, nil, func(uint32) {}); return true }) {
			return tango.ErrNotRunning
		}
		return nil
	}
	return attach(a, key, m, func(items []platform.MenuItem, onClick func(uint32)) {
		am.SetAppMenu(parent, items, onClick)
	})
}

type appMenuKey struct {
	a      *tango.App
	parent platform.SurfaceID
}

// SetAppMenu sets the application menu: the menu bar on macOS, the menu of
// every current and future window on Windows and Linux. A nil menu clears it.
func SetAppMenu(a *tango.App, m *Menu) error { return set(a, 0, m) }

// SetSurfaceMenu sets the window menu of s (Windows/Linux; on macOS the
// backend decides, usually the shared menu bar).
func SetSurfaceMenu(a *tango.App, s *tango.Surface, m *Menu) error {
	return set(a, s.ID(), m)
}
