//go:build linux && !android

package linux

// Desktop integration (contract round 2): displays (GdkMonitor), system
// theme (XDG desktop portal, else GtkSettings), shell (GIO default handlers,
// org.freedesktop.FileManager1), app control (window urgency hint and the
// com.canonical.Unity.LauncherEntry badge/progress API that Ubuntu Dock,
// Dash to Dock and KDE Plasma implement). D-Bus goes through the GIO
// bindings of notifyhotkey_linux.go; everything else is bound here, in the
// ix struct, so names cannot collide with other files.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.Displays    = (*backend)(nil)
	_ platform.SystemTheme = (*backend)(nil)
	_ platform.Shell       = (*backend)(nil)
	_ platform.AppControl  = (*backend)(nil)
	_ platform.WebViewInfo = (*backend)(nil)
)

type gdkRectangle struct{ X, Y, Width, Height int32 }

// gValue is GValue: a GType and two 8-byte data words.
type gValue struct {
	typ  uintptr
	data [2]uint64
}

const (
	gTypeBoolean = 5 << 2  // G_TYPE_BOOLEAN
	gTypeString  = 16 << 2 // G_TYPE_STRING

	portalBus   = "org.freedesktop.portal.Desktop"
	portalPath  = "/org/freedesktop/portal/desktop"
	portalIface = "org.freedesktop.portal.Settings"
	appearance  = "org.freedesktop.appearance"
)

var ix struct {
	once sync.Once
	ok   bool

	displayGetDefault        func() ptr
	displayGetNMonitors      func(d ptr) int32
	displayGetMonitor        func(d ptr, i int32) ptr
	displayGetPrimaryMonitor func(d ptr) ptr
	monitorGetGeometry       func(m ptr, r *gdkRectangle)
	monitorGetWorkarea       func(m ptr, r *gdkRectangle)
	monitorGetScaleFactor    func(m ptr) int32
	monitorGetModel          func(m ptr) ptr
	monitorGetManufacturer   func(m ptr) ptr
	screenGetDefault         func() ptr

	settingsGetDefault func() ptr
	objectGetProperty  func(o ptr, name string, v *gValue)
	valueInit          func(v *gValue, typ uintptr) ptr
	valueUnset         func(v *gValue)
	valueGetBoolean    func(v *gValue) bool
	valueGetString     func(v *gValue) ptr

	variantGetVariant    func(v ptr) ptr
	variantGetTypeString func(v ptr) ptr
	variantGetDouble     func(v ptr) float64
	connEmitSignal       func(conn ptr, dest *byte, path, iface, signal string, params ptr, gerr *ptr) bool

	showURIOnWindow     func(parent ptr, uri string, timestamp uint32, gerr *ptr) bool // optional, GTK 3.22
	windowSetUrgency    func(w ptr, on bool)
	widgetGetVisible    func(w ptr) bool
	webkitMajor         func() uint32
	webkitMinor         func() uint32
	webkitMicro         func() uint32
	portalSignalCB      ptr
	portalSubscribed    bool
	portalMissing       bool // the portal answered with an error once: stop asking
	conn                ptr
	lastTheme           platform.Theme
	lastDisplays        []platform.Display
	launcher            launcherState
	hidden              []platform.SurfaceID
	urgencyHandlers     map[ptr]uint
	launcherUnavailable bool
}

func bindIntegration() bool {
	ix.once.Do(func() {
		if err := loadSystemLibs(); err != nil { // libgdk, GIO D-Bus
			return
		}
		var bd binder
		d, t, g, o, w := libGDK, libGTK, libGLib, libGObject, libWebKit
		bd.req(&ix.displayGetDefault, "gdk_display_get_default", d, t)
		bd.req(&ix.displayGetNMonitors, "gdk_display_get_n_monitors", d, t)
		bd.req(&ix.displayGetMonitor, "gdk_display_get_monitor", d, t)
		bd.req(&ix.displayGetPrimaryMonitor, "gdk_display_get_primary_monitor", d, t)
		bd.req(&ix.monitorGetGeometry, "gdk_monitor_get_geometry", d, t)
		bd.req(&ix.monitorGetWorkarea, "gdk_monitor_get_workarea", d, t)
		bd.req(&ix.monitorGetScaleFactor, "gdk_monitor_get_scale_factor", d, t)
		bd.req(&ix.monitorGetModel, "gdk_monitor_get_model", d, t)
		bd.req(&ix.monitorGetManufacturer, "gdk_monitor_get_manufacturer", d, t)
		bd.req(&ix.screenGetDefault, "gdk_screen_get_default", d, t)
		bd.req(&ix.settingsGetDefault, "gtk_settings_get_default", t)
		bd.req(&ix.objectGetProperty, "g_object_get_property", o)
		bd.req(&ix.valueInit, "g_value_init", o)
		bd.req(&ix.valueUnset, "g_value_unset", o)
		bd.req(&ix.valueGetBoolean, "g_value_get_boolean", o)
		bd.req(&ix.valueGetString, "g_value_get_string", o)
		bd.req(&ix.variantGetVariant, "g_variant_get_variant", g)
		bd.req(&ix.variantGetTypeString, "g_variant_get_type_string", g)
		bd.req(&ix.variantGetDouble, "g_variant_get_double", g)
		bd.req(&ix.connEmitSignal, "g_dbus_connection_emit_signal", libGIO)
		bd.opt(&ix.showURIOnWindow, "gtk_show_uri_on_window", t)
		bd.req(&ix.windowSetUrgency, "gtk_window_set_urgency_hint", t)
		bd.req(&ix.widgetGetVisible, "gtk_widget_get_visible", t)
		if !(bd.opt(&ix.webkitMajor, "webkit_get_major_version", w) &&
			bd.opt(&ix.webkitMinor, "webkit_get_minor_version", w) &&
			bd.opt(&ix.webkitMicro, "webkit_get_micro_version", w)) {
			ix.webkitMajor = nil
		}
		ix.urgencyHandlers = map[ptr]uint{}
		ix.ok = len(bd.missing) == 0
	})
	return ix.ok
}

// sessionBus returns the shared session bus connection, or 0.
func sessionBus() ptr {
	if ix.conn == 0 {
		var gerr ptr
		ix.conn = gBusGetSync(gBusTypeSession, 0, &gerr)
		if ix.conn == 0 {
			gErrorMessage(gerr)
		}
	}
	return ix.conn
}

// installSystemWatch connects the signals behind AppThemeChanged and
// AppDisplaysChanged. Called from Init after gtk_init.
func (b *backend) installSystemWatch() {
	if !bindIntegration() {
		return
	}
	ix.lastTheme = b.SystemTheme()
	ix.lastDisplays = b.Displays()
	if scr := ix.screenGetDefault(); scr != 0 {
		// "monitors-changed" covers added/removed monitors and geometry or
		// scale changes; "size-changed" the X screen size.
		for _, sig := range []string{"monitors-changed", "size-changed"} {
			connect(scr, sig, 0, func(_, _ ptr) ptr { b.displaysMaybeChanged(); return 0 })
		}
	}
	if st := ix.settingsGetDefault(); st != 0 {
		for _, sig := range []string{"notify::gtk-application-prefer-dark-theme", "notify::gtk-theme-name"} {
			connect(st, sig, 1, func(_, _ ptr) ptr {
				b.themeMaybeChanged()
				b.contrastMaybeChanged() // contrast_linux.go: "HighContrast" themes
				return 0
			})
		}
	}
	if conn := sessionBus(); conn != 0 {
		ix.portalSignalCB = purego.NewCallback(func(conn, sender, path, iface, signal, params, data ptr) {
			b.themeMaybeChanged()
			b.contrastMaybeChanged() // contrast_linux.go: appearance contrast
		})
		// The spec names the signal SettingChanged(s namespace, s key, v value).
		gDBusConnectionSignalSubscr(conn, portalBus, portalIface, "SettingChanged", portalPath, cstr(appearance), 0, ix.portalSignalCB, 0, 0)
		ix.portalSubscribed = true
	}
}

func (b *backend) themeMaybeChanged() {
	t := b.SystemTheme()
	if t == ix.lastTheme {
		return
	}
	ix.lastTheme = t
	if hx, ok := b.host.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppThemeChanged, Theme: t})
	}
}

func (b *backend) displaysMaybeChanged() {
	ds := b.Displays()
	if slices.Equal(ds, ix.lastDisplays) {
		return
	}
	ix.lastDisplays = ds
	if hx, ok := b.host.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppDisplaysChanged})
	}
}

// ---- platform.Displays ----

// Displays lists GdkMonitors. GDK reports application (logical) pixels
// with the origin at the top-left of the whole screen layout; they are
// translated so the primary monitor's top-left is 0,0, as the contract
// wants (the same translation window positions get, screenOrigin). Wayland
// may report no primary monitor (the first one is taken then).
func (b *backend) Displays() []platform.Display {
	if !bindIntegration() {
		return nil
	}
	disp := ix.displayGetDefault()
	if disp == 0 {
		return nil
	}
	primary := ix.displayGetPrimaryMonitor(disp)
	n := int(ix.displayGetNMonitors(disp))
	out := make([]platform.Display, 0, n)
	geos := make([]platform.Rect, 0, n)
	primaryAt := -1
	for i := range n {
		m := ix.displayGetMonitor(disp, int32(i))
		if m == 0 {
			continue
		}
		var geo, work gdkRectangle
		ix.monitorGetGeometry(m, &geo)
		ix.monitorGetWorkarea(m, &work)
		model := goStr(ix.monitorGetModel(m))
		name := strings.TrimSpace(goStr(ix.monitorGetManufacturer(m)) + " " + model)
		if m == primary {
			primaryAt = len(out)
		}
		bounds := platform.Rect{X: int(geo.X), Y: int(geo.Y), Width: int(geo.Width), Height: int(geo.Height)}
		geos = append(geos, bounds)
		out = append(out, platform.Display{
			ID:       fmt.Sprintf("%d:%s", i, model),
			Name:     name,
			Bounds:   bounds,
			WorkArea: platform.Rect{X: int(work.X), Y: int(work.Y), Width: int(work.Width), Height: int(work.Height)},
			Scale:    float64(ix.monitorGetScaleFactor(m)),
		})
	}
	if primaryAt < 0 && len(out) > 0 {
		primaryAt = 0
	}
	off := primaryOffset(geos, primaryAt)
	for i := range out {
		out[i].Primary = i == primaryAt
		out[i].Bounds = off.rect(out[i].Bounds)
		out[i].WorkArea = off.rect(out[i].WorkArea)
	}
	return out
}

// ---- platform.SystemTheme ----

// portalRead reads one org.freedesktop.appearance setting and returns the
// innermost value (Read wraps it in one or two variants); 0 if unavailable.
// The caller unrefs the result.
func portalRead(key string) ptr {
	if ix.portalMissing {
		return 0
	}
	conn := sessionBus()
	if conn == 0 {
		return 0
	}
	var gerr ptr
	params := gVariantParse(0, portalReadArgs(appearance, key), 0, 0, &gerr)
	if params == 0 {
		gErrorMessage(gerr)
		return 0
	}
	reply := gDBusConnectionCallSync(conn, portalBus, portalPath, portalIface, "Read", params, 0, 0, 500, 0, &gerr)
	if reply == 0 {
		msg := gErrorMessage(gerr)
		// No portal at all (as opposed to an unknown key): don't ask again.
		if strings.Contains(msg, "ServiceUnknown") || strings.Contains(msg, "UnknownMethod") || strings.Contains(msg, "NoReply") {
			ix.portalMissing = true
		}
		return 0
	}
	v := gVariantGetChildValue(reply, 0)
	gVariantUnref(reply)
	for v != 0 && goStr(ix.variantGetTypeString(v)) == "v" {
		inner := ix.variantGetVariant(v)
		gVariantUnref(v)
		v = inner
	}
	return v
}

func gtkSetting(st ptr, name string, typ uintptr) (bool, string) {
	var v gValue
	ix.valueInit(&v, typ)
	ix.objectGetProperty(st, name, &v)
	defer ix.valueUnset(&v)
	if typ == gTypeBoolean {
		return ix.valueGetBoolean(&v), ""
	}
	return false, goStr(ix.valueGetString(&v))
}

func (b *backend) SystemTheme() platform.Theme {
	var t platform.Theme
	if !bindIntegration() {
		return t
	}
	var scheme uint32
	have := false
	if v := portalRead("color-scheme"); v != 0 {
		if goStr(ix.variantGetTypeString(v)) == "u" {
			scheme, have = gVariantGetUint32(v), true
		}
		gVariantUnref(v)
	}
	preferDark, themeName := false, ""
	if st := ix.settingsGetDefault(); st != 0 {
		preferDark, _ = gtkSetting(st, "gtk-application-prefer-dark-theme", gTypeBoolean)
		_, themeName = gtkSetting(st, "gtk-theme-name", gTypeString)
	}
	t.Dark = themeIsDark(scheme, have, preferDark, themeName)
	if v := portalRead("accent-color"); v != 0 { // portal 1.19+ / GNOME 47
		if goStr(ix.variantGetTypeString(v)) == "(ddd)" {
			var c [3]float64
			for i := range c {
				e := gVariantGetChildValue(v, uintptr(i))
				c[i] = ix.variantGetDouble(e)
				gVariantUnref(e)
			}
			t.Accent = portalAccent(c[0], c[1], c[2])
		}
		gVariantUnref(v)
	}
	return t
}

// ---- platform.Shell ----

// showURI opens uri with the default handler: gtk_show_uri_on_window goes
// through GIO (and the OpenURI portal inside Flatpak); without it
// xdg-open is started.
func showURI(uri string) error {
	if bindIntegration() && ix.showURIOnWindow != nil {
		var gerr ptr
		if ix.showURIOnWindow(0, uri, 0, &gerr) {
			return nil
		}
		return errors.New("tango/linux: " + gErrorMessage(gerr))
	}
	cmd := exec.Command("xdg-open", uri)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap; xdg-open's exit status is unreliable across desktops
	return nil
}

func (b *backend) OpenURL(u string) error       { return showURI(u) }
func (b *backend) OpenPath(path string) error   { return showURI(fileURI(path)) }
func (b *backend) RevealPath(path string) error { return revealPath(path) }

// revealPath asks the file manager to select path through
// org.freedesktop.FileManager1 (Nautilus, Dolphin, Nemo, Caja, Thunar
// implement it); otherwise the containing folder is opened.
func revealPath(path string) error {
	if bindIntegration() {
		if conn := sessionBus(); conn != 0 {
			var gerr ptr
			params := gVariantParse(0, showItemsArgs(path), 0, 0, &gerr)
			if params != 0 {
				reply := gDBusConnectionCallSync(conn, "org.freedesktop.FileManager1", "/org/freedesktop/FileManager1",
					"org.freedesktop.FileManager1", "ShowItems", params, 0, 0, 2000, 0, &gerr)
				if reply != 0 {
					gVariantUnref(reply)
					return nil
				}
			}
			gErrorMessage(gerr)
		}
	}
	return showURI(fileURI(filepath.Dir(path)))
}

// ---- platform.AppControl ----

func (b *backend) liveSurfaces() []*surface {
	out := make([]*surface, 0, len(b.surfaces))
	for _, s := range b.surfaces {
		if !s.closed {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(x, y *surface) int { return int(x.id) - int(y.id) })
	return out
}

func (b *backend) HideApp() {
	if !bindIntegration() {
		return
	}
	ix.hidden = ix.hidden[:0]
	for _, s := range b.liveSurfaces() {
		if ix.widgetGetVisible(s.win) {
			ix.hidden = append(ix.hidden, s.id)
			s.Hide()
		}
	}
}

func (b *backend) ShowApp() {
	shown := false
	for _, id := range ix.hidden {
		if s := b.surfaces[id]; s != nil && !s.closed {
			s.Show() // gtk_window_present: maps and raises
			shown = true
		}
	}
	ix.hidden = ix.hidden[:0]
	if !shown {
		if ss := b.liveSurfaces(); len(ss) > 0 {
			ss[0].Show()
		}
	}
}

// AppControlOps reports what Linux desktops can do: no dock icon to hide
// (SetDockVisible is a no-op), and launcher badges are numbers only.
func (b *backend) AppControlOps() platform.AppControlOp {
	return platform.CtlHide | platform.CtlBadge | platform.CtlProgress | platform.CtlAttention
}

func (b *backend) SetDockVisible(bool) {}

// RequestAttention sets the urgency hint of every visible window until it
// gets focus (window managers highlight it in the taskbar / dock).
// critical is not distinguished.
func (b *backend) RequestAttention(critical bool) {
	if !bindIntegration() {
		return
	}
	for _, s := range b.liveSurfaces() {
		if !ix.widgetGetVisible(s.win) {
			continue
		}
		win := s.win
		ix.windowSetUrgency(win, true)
		if _, ok := ix.urgencyHandlers[win]; ok {
			continue
		}
		ix.urgencyHandlers[win] = connect(win, "focus-in-event", 1, func(_, _ ptr) ptr {
			ix.windowSetUrgency(win, false)
			if id, ok := ix.urgencyHandlers[win]; ok {
				delete(ix.urgencyHandlers, win)
				gSignalHandlerDisconnect(win, id)
			}
			return 0 // let GTK handle the event too
		})
	}
}

func (b *backend) SetBadge(text string) {
	n, ok := launcherCount(text)
	ix.launcher.count, ix.launcher.countOn = n, ok && text != ""
	b.updateLauncher()
}

func (b *backend) SetProgress(p float64) {
	ix.launcher.progress, ix.launcher.progOn = p, p >= 0
	b.updateLauncher()
}

// appURI names the app's desktop entry, which the dock matches against
// the running app: application://<ID>.desktop, the ID being what Init
// gave GTK as program name.
func (b *backend) appURI() string {
	id := b.opts.ID
	if id == "" {
		if exe, err := os.Executable(); err == nil {
			id = filepath.Base(exe)
		}
	}
	return "application://" + id + ".desktop"
}

// updateLauncher sends the Unity LauncherEntry Update signal. Desktops
// without a listener simply ignore it.
func (b *backend) updateLauncher() {
	if ix.launcherUnavailable || !bindIntegration() {
		return
	}
	conn := sessionBus()
	if conn == 0 {
		ix.launcherUnavailable = true
		return
	}
	uri := b.appURI()
	var gerr ptr
	params := gVariantParse(0, launcherArgs(uri, ix.launcher), 0, 0, &gerr)
	if params == 0 {
		gErrorMessage(gerr)
		return
	}
	if !ix.connEmitSignal(conn, nil, launcherPath(uri), "com.canonical.Unity.LauncherEntry", "Update", params, &gerr) {
		gErrorMessage(gerr)
	}
}

// WebViewVersion implements platform.WebViewInfo with the WebKitGTK
// version ("" when the version functions are missing).
func (b *backend) WebViewVersion() (engine, version string) {
	if bindIntegration() && ix.webkitMajor != nil {
		version = fmt.Sprintf("%d.%d.%d", ix.webkitMajor(), ix.webkitMinor(), ix.webkitMicro())
	}
	return "WebKitGTK", version
}
