//go:build linux && !android

package linux

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// System tray. NewTray first tries Tango's own StatusNotifierItem
// (sni_linux.go), which reports clicks and honours TrayOptions.MenuOn; it
// needs a StatusNotifierWatcher on the session bus. Without one (or with
// TANGO_LINUX_TRAY=appindicator) it falls back to libayatana-appindicator3
// (or the older libappindicator3), implemented in this file: the indicator
// reads its icon from a file in an icon theme path and exports a GtkMenu
// over D-Bus.
//
// AppIndicator reports no clicks on the icon itself: TrayHandlers.OnClick,
// OnRightClick and OnDoubleClick are never called (activating the icon
// opens the menu); only OnMenu is. For the same reason TrayOptions.MenuOn
// is ignored on that path, as the contract allows: any click opens the
// menu, which is what TrayMenuDefault means on Linux. There is no
// GtkStatusIcon path.

var _ platform.Trays = (*backend)(nil)

// AppIndicator enums.
const (
	appIndicatorCategoryApplicationStatus = 0
	appIndicatorStatusPassive             = 0
	appIndicatorStatusActive              = 1

	trayFallbackIcon = "application-x-executable"
)

var ai struct {
	once sync.Once
	ok   bool
	lib  ptr

	newIndicator     func(id, iconName string, category int32) ptr
	setStatus        func(ind ptr, status int32)
	setMenu          func(ind, menu ptr)
	setIconThemePath func(ind ptr, path string)
	setIconFull      func(ind ptr, name, desc string)
	setTitle         func(ind ptr, title string)        // optional (0.5+)
	setLabel         func(ind ptr, label, guide string) // optional
}

func bindAppIndicator() bool {
	ai.once.Do(func() {
		var err error
		ai.lib, _, err = open("libayatana-appindicator3.so.1", "libappindicator3.so.1")
		if err != nil {
			return
		}
		var bd binder
		l := ai.lib
		bd.req(&ai.newIndicator, "app_indicator_new", l)
		bd.req(&ai.setStatus, "app_indicator_set_status", l)
		bd.req(&ai.setMenu, "app_indicator_set_menu", l)
		bd.req(&ai.setIconThemePath, "app_indicator_set_icon_theme_path", l)
		bd.req(&ai.setIconFull, "app_indicator_set_icon_full", l)
		bd.opt(&ai.setTitle, "app_indicator_set_title", l)
		bd.opt(&ai.setLabel, "app_indicator_set_label", l)
		ai.ok = len(bd.missing) == 0
	})
	return ai.ok
}

var traySeq int // trays created in this process (main thread only)

type tray struct {
	b   *backend
	n   int
	ind ptr
	h   platform.TrayHandlers

	menu    ptr // our reference to the current GtkMenu (never 0 while live)
	dir     string
	icon    []byte
	file    string // current icon file in dir ("" for the fallback)
	fileSeq int
	tooltip string
	title   string
	removed bool
}

func (b *backend) NewTray(o platform.TrayOptions, h platform.TrayHandlers) (platform.Tray, error) {
	if !b.loaded.Load() {
		return nil, fmt.Errorf("tango/linux: NewTray before Init")
	}
	if t, ok, err := b.newSNITray(o, h); ok {
		return t, err
	}
	if !bindAppIndicator() {
		return nil, fmt.Errorf("%w: system tray needs libayatana-appindicator3 (libayatana-appindicator3-1 on Debian/Ubuntu) or libappindicator3", platform.ErrUnsupported)
	}
	bindMenuExt()
	if mx.objectRefSink == nil {
		return nil, fmt.Errorf("%w: g_object_ref_sink missing", platform.ErrUnsupported)
	}
	dir, err := os.MkdirTemp("", "tango-tray-")
	if err != nil {
		return nil, fmt.Errorf("tango/linux: tray icon directory: %w", err)
	}
	traySeq++
	t := &tray{b: b, n: traySeq, h: h, dir: dir}

	appID := b.opts.ID
	if appID == "" {
		appID = "tango"
	}
	t.ind = ai.newIndicator(appID+"-tray-"+strconv.Itoa(t.n), trayFallbackIcon, appIndicatorCategoryApplicationStatus)
	if t.ind == 0 {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("tango/linux: app_indicator_new failed")
	}
	ai.setIconThemePath(t.ind, dir)
	if err := t.apply(o); err != nil {
		t.Remove()
		return nil, err
	}
	// An indicator without a menu is not shown at all: start with an empty
	// one.
	t.SetMenu(nil)
	ai.setStatus(t.ind, appIndicatorStatusActive)
	return t, nil
}

// apply updates whatever changed in o. Template has no meaning here.
func (t *tray) apply(o platform.TrayOptions) error {
	var err error
	iconChanged := !bytes.Equal(o.Icon, t.icon) || t.fileSeq == 0
	if iconChanged || o.Tooltip != t.tooltip {
		if iconChanged {
			err = t.setIcon(o.Icon, o.Tooltip)
		} else {
			name := trayFallbackIcon
			if t.file != "" {
				name = iconName(t.file)
			}
			ai.setIconFull(t.ind, name, o.Tooltip)
		}
		if ai.setTitle != nil {
			ai.setTitle(t.ind, o.Tooltip)
		}
		t.tooltip = o.Tooltip
	}
	if o.Title != t.title && ai.setLabel != nil {
		ai.setLabel(t.ind, o.Title, o.Title)
		t.title = o.Title
	}
	return err
}

// setIcon writes png under a new file name (hosts cache icons by name) and
// switches the indicator to it, then deletes the previous file.
func (t *tray) setIcon(png []byte, desc string) error {
	t.fileSeq++
	old := t.file
	t.icon = bytes.Clone(png)
	if len(png) == 0 {
		t.file = ""
		ai.setIconFull(t.ind, trayFallbackIcon, desc)
	} else {
		f := filepath.Join(t.dir, fmt.Sprintf("tango-tray-%d-%d.png", t.n, t.fileSeq))
		if err := os.WriteFile(f, png, 0o644); err != nil {
			return fmt.Errorf("tango/linux: tray icon: %w", err)
		}
		t.file = f
		ai.setIconFull(t.ind, iconName(f), desc)
	}
	if old != "" {
		os.Remove(old)
	}
	return nil
}

// iconName is the theme icon name of a file in the icon theme path.
func iconName(file string) string {
	b := filepath.Base(file)
	return b[:len(b)-len(filepath.Ext(b))]
}

func (t *tray) SetOptions(o platform.TrayOptions) {
	if t.removed {
		return
	}
	_ = t.apply(o) // a failed icon write keeps the previous icon
}

// SetMenu replaces the indicator's menu; nil installs an empty menu (the
// indicator would disappear without one).
func (t *tray) SetMenu(items []platform.MenuItem) {
	if t.removed {
		return
	}
	menu := gtkMenuNew()
	mx.objectRefSink(menu) // ours until replaced, whatever the indicator does
	entries := buildMenuEntries(items, t.b.opts.Name)
	mb := menuBuilder{activate: t.activate}
	mb.fill(menu, entries)
	ai.setMenu(t.ind, menu)
	t.dropMenu()
	t.menu = menu
}

func (t *tray) dropMenu() {
	if t.menu != 0 {
		gtkWidgetDestroy(t.menu) // disconnects the items' handlers
		gObjectUnref(t.menu)
		t.menu = 0
	}
}

func (t *tray) activate(e menuEntry) {
	if t.removed {
		return
	}
	switch e.Action {
	case actClick:
		if e.ID != 0 && t.h.OnMenu != nil {
			t.h.OnMenu(e.ID)
		}
	case actQuit:
		t.b.userQuit()
	case actAbout:
		t.b.showAbout(0)
	}
	// Edit and window roles need a window: nothing to do from the tray.
}

// Bounds is unknown: the StatusNotifierItem host draws the icon.
func (t *tray) Bounds() (platform.Rect, bool) { return platform.Rect{}, false }

func (t *tray) Remove() {
	if t.removed {
		return
	}
	t.removed = true
	ai.setStatus(t.ind, appIndicatorStatusPassive)
	gObjectUnref(t.ind)
	t.ind = 0
	t.dropMenu()
	os.RemoveAll(t.dir)
}
