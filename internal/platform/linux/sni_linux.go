//go:build linux && !android

package linux

// System tray as a StatusNotifierItem exported by Tango itself over
// GDBus (KDE/freedesktop StatusNotifierItem specification), with its menu
// exported as com.canonical.dbusmenu. Unlike AppIndicator this receives
// the host's Activate (primary click), SecondaryActivate (middle click),
// ContextMenu (secondary click when the host shows no menu itself) and
// Scroll, so TrayHandlers and TrayOptions.MenuOn work. It is used when a
// StatusNotifierWatcher is on the session bus (KDE Plasma, GNOME with the
// AppIndicator extension, XFCE, Cinnamon, MATE, waybar, ...); otherwise —
// or with TANGO_LINUX_TRAY=appindicator — tray_linux.go's AppIndicator
// path is used.

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/tangoui/tango/internal/platform"
)

const (
	sniWatcherBus   = "org.kde.StatusNotifierWatcher"
	sniWatcherPath  = "/StatusNotifierWatcher"
	sniIface        = "org.kde.StatusNotifierItem"
	sniPath         = "/StatusNotifierItem"
	dbusMenuIface   = "com.canonical.dbusmenu"
	dbusMenuPathFmt = "/StatusNotifierItem/Menu"
)

const sniXML = `<node><interface name="org.kde.StatusNotifierItem">
<property name="Category" type="s" access="read"/>
<property name="Id" type="s" access="read"/>
<property name="Title" type="s" access="read"/>
<property name="Status" type="s" access="read"/>
<property name="WindowId" type="i" access="read"/>
<property name="IconName" type="s" access="read"/>
<property name="IconPixmap" type="a(iiay)" access="read"/>
<property name="OverlayIconName" type="s" access="read"/>
<property name="OverlayIconPixmap" type="a(iiay)" access="read"/>
<property name="AttentionIconName" type="s" access="read"/>
<property name="AttentionIconPixmap" type="a(iiay)" access="read"/>
<property name="AttentionMovieName" type="s" access="read"/>
<property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
<property name="ItemIsMenu" type="b" access="read"/>
<property name="Menu" type="o" access="read"/>
<method name="ContextMenu"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
<method name="Activate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
<method name="SecondaryActivate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
<method name="Scroll"><arg name="delta" type="i" direction="in"/><arg name="orientation" type="s" direction="in"/></method>
<signal name="NewTitle"/><signal name="NewIcon"/><signal name="NewAttentionIcon"/><signal name="NewOverlayIcon"/>
<signal name="NewToolTip"/><signal name="NewStatus"><arg name="status" type="s"/></signal>
</interface></node>`

const dbusMenuXML = `<node><interface name="com.canonical.dbusmenu">
<property name="Version" type="u" access="read"/>
<property name="TextDirection" type="s" access="read"/>
<property name="Status" type="s" access="read"/>
<property name="IconThemePath" type="as" access="read"/>
<method name="GetLayout"><arg type="i" name="parentId" direction="in"/><arg type="i" name="recursionDepth" direction="in"/>
<arg type="as" name="propertyNames" direction="in"/><arg type="u" name="revision" direction="out"/><arg type="(ia{sv}av)" name="layout" direction="out"/></method>
<method name="GetGroupProperties"><arg type="ai" name="ids" direction="in"/><arg type="as" name="propertyNames" direction="in"/>
<arg type="a(ia{sv})" name="properties" direction="out"/></method>
<method name="GetProperty"><arg type="i" name="id" direction="in"/><arg type="s" name="name" direction="in"/><arg type="v" name="value" direction="out"/></method>
<method name="Event"><arg type="i" name="id" direction="in"/><arg type="s" name="eventId" direction="in"/><arg type="v" name="data" direction="in"/><arg type="u" name="timestamp" direction="in"/></method>
<method name="EventGroup"><arg type="a(isvu)" name="events" direction="in"/><arg type="ai" name="idErrors" direction="out"/></method>
<method name="AboutToShow"><arg type="i" name="id" direction="in"/><arg type="b" name="needUpdate" direction="out"/></method>
<method name="AboutToShowGroup"><arg type="ai" name="ids" direction="in"/><arg type="ai" name="updatesNeeded" direction="out"/><arg type="ai" name="idErrors" direction="out"/></method>
<signal name="ItemsPropertiesUpdated"><arg type="a(ia{sv})" name="updatedProps"/><arg type="a(ias)" name="removedProps"/></signal>
<signal name="LayoutUpdated"><arg type="u" name="revision"/><arg type="i" name="parent"/></signal>
<signal name="ItemActivationRequested"><arg type="i" name="id"/><arg type="u" name="timestamp"/></signal>
</interface></node>`

type sniTray struct {
	b    *backend
	n    int
	h    platform.TrayHandlers
	conn ptr
	name string // well-known bus name

	opts    platform.TrayOptions
	pixmap  string // IconPixmap text for opts.Icon
	menu    *dbusMenu
	hasMenu bool
	rev     uint32

	clickX, clickY int
	clicked        bool

	unexport []func()
	unwatch  func()
	removed  bool
}

// newSNITray returns (tray, true, err) when the StatusNotifierItem path is
// usable, (nil, false, nil) to fall back to AppIndicator.
func (b *backend) newSNITray(o platform.TrayOptions, h platform.TrayHandlers) (platform.Tray, bool, error) {
	if os.Getenv("TANGO_LINUX_TRAY") == "appindicator" {
		return nil, false, nil
	}
	conn, err := bus(gBusTypeSession)
	if err != nil || !nameHasOwner(conn, sniWatcherBus) {
		return nil, false, nil
	}
	traySeq++
	t := &sniTray{b: b, n: traySeq, h: h, conn: conn, name: sniBusName(os.Getpid(), traySeq)}
	t.opts = o
	t.pixmap = sniPixmap(o.Icon)
	t.menu = buildDBusMenu(nil)
	path := t.itemPath()
	un1, err := exportObject(conn, path, sniIface, sniXML, (*sniItemObj)(t))
	if err != nil {
		return nil, true, err
	}
	un2, err := exportObject(conn, t.menuPath(), dbusMenuIface, dbusMenuXML, (*sniMenuObj)(t))
	if err != nil {
		un1()
		return nil, true, err
	}
	t.unexport = []func(){un1, un2}
	if r, err := dbusCall(conn, "org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "RequestName",
		"("+gvString(t.name)+", uint32 4)", 3000); err == nil { // DBUS_NAME_FLAG_DO_NOT_QUEUE
		gVariantUnref(r)
	} else {
		t.Remove()
		return nil, true, err
	}
	if err := t.register(); err != nil {
		t.Remove()
		return nil, false, nil // a broken watcher: try AppIndicator
	}
	// Re-register when the watcher restarts (plasmashell / panel restart).
	t.unwatch = subscribe(conn, "org.freedesktop.DBus", "org.freedesktop.DBus", "NameOwnerChanged", "/org/freedesktop/DBus", sniWatcherBus,
		func(_, _, _, _ string, params ptr) {
			nv := gvChild(params, 2)
			owner := gvStr(nv)
			gVariantUnref(nv)
			if owner != "" && !t.removed {
				_ = t.register()
			}
		})
	return t, true, nil
}

// Each tray gets its own object paths so several trays can share the
// connection.
func (t *sniTray) itemPath() string {
	if t.n == 1 {
		return sniPath
	}
	return sniPath + strconv.Itoa(t.n)
}

func (t *sniTray) menuPath() string { return dbusMenuPathFmt + strconv.Itoa(t.n) }

func (t *sniTray) register() error {
	r, err := dbusCall(t.conn, sniWatcherBus, sniWatcherPath, sniWatcherBus, "RegisterStatusNotifierItem", "("+gvString(t.name)+",)", 3000)
	if err != nil {
		return err
	}
	gVariantUnref(r)
	return nil
}

func (t *sniTray) title() string {
	if t.opts.Title != "" {
		return t.opts.Title
	}
	if t.b.opts.Name != "" {
		return t.b.opts.Name
	}
	return t.opts.Tooltip
}

func (t *sniTray) emit(signal string) {
	if !t.removed {
		emit(t.conn, t.itemPath(), sniIface, signal, "")
	}
}

func (t *sniTray) SetOptions(o platform.TrayOptions) {
	if t.removed {
		return
	}
	old := t.opts
	t.opts = o
	if !bytes.Equal(old.Icon, o.Icon) {
		t.pixmap = sniPixmap(o.Icon)
		t.emit("NewIcon")
	}
	if old.Tooltip != o.Tooltip {
		t.emit("NewToolTip")
	}
	if old.Title != o.Title || old.MenuOn != o.MenuOn {
		t.emit("NewTitle") // hosts re-read the item's properties
	}
}

func (t *sniTray) SetMenu(items []platform.MenuItem) {
	if t.removed {
		return
	}
	t.hasMenu = items != nil
	t.menu = buildDBusMenu(buildMenuEntries(items, t.b.opts.Name))
	t.rev++
	emit(t.conn, t.menuPath(), dbusMenuIface, "LayoutUpdated", fmt.Sprintf("(uint32 %d, 0)", t.rev))
	t.emit("NewTitle")
}

// Bounds reports the point of the last Activate as a 1x1 rectangle. The
// specification gives no icon geometry, but hosts pass the click position
// in screen coordinates with Activate, which is enough to place a flyout
// next to the icon. Before the first click, or where the host sends (0, 0)
// (many Wayland hosts), the bounds are unknown.
func (t *sniTray) Bounds() (platform.Rect, bool) {
	if t.removed || !t.clicked {
		return platform.Rect{}, false
	}
	x, y := screenOrigin().toContract(t.clickX, t.clickY)
	return platform.Rect{X: x, Y: y, Width: 1, Height: 1}, true
}

func (t *sniTray) Remove() {
	if t.removed {
		return
	}
	t.removed = true
	if t.unwatch != nil {
		t.unwatch()
	}
	for _, u := range t.unexport {
		u()
	}
	if r, err := dbusCall(t.conn, "org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "ReleaseName",
		"("+gvString(t.name)+",)", 3000); err == nil {
		gVariantUnref(r)
	}
}

func (t *sniTray) noteClick(params ptr) {
	xv, yv := gvChild(params, 0), gvChild(params, 1)
	x, y := int(gd.vInt32(xv)), int(gd.vInt32(yv))
	gVariantUnref(xv)
	gVariantUnref(yv)
	if x != 0 || y != 0 {
		t.clickX, t.clickY, t.clicked = x, y, true
	}
}

// fire runs a handler after the D-Bus reply went out, so a handler that
// blocks (a dialog) does not stall the host.
func (t *sniTray) fire(fn func()) {
	if fn == nil {
		return
	}
	later(0, func() {
		if !t.removed {
			fn()
		}
	})
}

// ---- org.kde.StatusNotifierItem ----

type sniItemObj sniTray

func (o *sniItemObj) call(method string, params, inv ptr) {
	t := (*sniTray)(o)
	returnValue(inv, "")
	switch method {
	case "Activate":
		t.noteClick(params)
		if !sniItemIsMenu(t.hasMenu, t.opts.MenuOn) {
			t.fire(t.h.OnClick)
		}
	case "SecondaryActivate", "ContextMenu":
		t.noteClick(params)
		t.fire(t.h.OnRightClick)
	case "Scroll":
		// No contract event for scrolling.
	}
}

func (o *sniItemObj) property(name string) ptr {
	t := (*sniTray)(o)
	switch name {
	case "Category":
		return propValue("'ApplicationStatus'")
	case "Id":
		return propValue(gvString(secretApp(t.b.opts.ID, t.b.opts.Name) + "-tray-" + strconv.Itoa(t.n)))
	case "Title":
		return propValue(gvString(t.title()))
	case "Status":
		return propValue("'Active'")
	case "WindowId":
		return propValue("0")
	case "IconName":
		if len(t.opts.Icon) == 0 {
			return propValue(gvString(trayFallbackIcon))
		}
		return propValue("''")
	case "IconPixmap":
		return propValue(t.pixmap)
	case "OverlayIconName", "AttentionIconName", "AttentionMovieName":
		return propValue("''")
	case "OverlayIconPixmap", "AttentionIconPixmap":
		return propValue("@a(iiay) []")
	case "ToolTip":
		return propValue(sniToolTip(t.opts.Tooltip))
	case "ItemIsMenu":
		return propValue(strconv.FormatBool(sniItemIsMenu(t.hasMenu, t.opts.MenuOn)))
	case "Menu":
		if !t.hasMenu {
			// "/" tells hosts there is no menu, so a secondary click
			// arrives as ContextMenu (OnRightClick).
			return propValue("objectpath '/'")
		}
		return propValue("objectpath " + gvString(t.menuPath()))
	}
	return 0
}

// ---- com.canonical.dbusmenu ----

type sniMenuObj sniTray

func (o *sniMenuObj) call(method string, params, inv ptr) {
	t := (*sniTray)(o)
	switch method {
	case "GetLayout":
		pv, dv := gvChild(params, 0), gvChild(params, 1)
		parent, depth := gd.vInt32(pv), int(gd.vInt32(dv))
		gVariantUnref(pv)
		gVariantUnref(dv)
		returnValue(inv, fmt.Sprintf("(uint32 %d, %s)", t.rev, t.menu.subLayout(parent, depth)))
	case "GetGroupProperties":
		returnValue(inv, "("+t.menu.groupProperties(int32s(params, 0))+",)")
	case "GetProperty":
		iv, nv := gvChild(params, 0), gvChild(params, 1)
		id, name := gd.vInt32(iv), gvStr(nv)
		gVariantUnref(iv)
		gVariantUnref(nv)
		if v, ok := t.menu.prop(id, name); ok {
			returnValue(inv, "(<"+v+">,)")
			return
		}
		gd.invReturnError(inv, "org.freedesktop.DBus.Error.InvalidArgs", "unknown item or property")
	case "Event":
		iv, ev := gvChild(params, 0), gvChild(params, 1)
		id, event := gd.vInt32(iv), gvStr(ev)
		gVariantUnref(iv)
		gVariantUnref(ev)
		returnValue(inv, "")
		t.menuEvent(id, event)
	case "EventGroup":
		events := gvChild(params, 0)
		n := int(gd.vNChildren(events))
		for i := range n {
			e := gvChild(events, i)
			iv, ev := gvChild(e, 0), gvChild(e, 1)
			t.menuEvent(gd.vInt32(iv), gvStr(ev))
			gVariantUnref(iv)
			gVariantUnref(ev)
			gVariantUnref(e)
		}
		gVariantUnref(events)
		returnValue(inv, "(@ai [],)")
	case "AboutToShow":
		returnValue(inv, "(false,)")
	case "AboutToShowGroup":
		returnValue(inv, "(@ai [], @ai [])")
	default:
		gd.invReturnError(inv, "org.freedesktop.DBus.Error.UnknownMethod", method)
	}
}

func (o *sniMenuObj) property(name string) ptr {
	switch name {
	case "Version":
		return propValue("uint32 3")
	case "TextDirection":
		return propValue("'ltr'")
	case "Status":
		return propValue("'normal'")
	case "IconThemePath":
		return propValue("@as []")
	}
	return 0
}

func (t *sniTray) menuEvent(id int32, event string) {
	if event != "clicked" {
		return
	}
	e, ok := t.menu.byID[id]
	if !ok || e.Disabled {
		return
	}
	later(0, func() {
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
	})
}

// int32s reads an "ai" child of params.
func int32s(params ptr, i int) []int32 {
	a := gvChild(params, i)
	defer gVariantUnref(a)
	n := int(gd.vNChildren(a))
	out := make([]int32, 0, n)
	for j := range n {
		v := gvChild(a, j)
		out = append(out, gd.vInt32(v))
		gVariantUnref(v)
	}
	return out
}
