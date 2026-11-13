//go:build linux && !android

package linux

// AT-SPI2 bridge of the canvas (docs/specs/a11y-bridge.md): the canvas
// implements platform.CanvasAccessible and exposes the semantics tree of
// the ui package (ui/a11y) over D-Bus, following the at-spi2-core
// interface definitions (org.a11y.atspi.Accessible, Component, Action,
// Value, Text, EditableText, Event.Object).
//
// Bus. The accessibility bus address comes from org.a11y.Bus.GetAddress
// on the session bus; we open our own connection to it (GTK's atk-bridge
// has its own), so our objects live under our unique name. Whether
// assistive technologies are on comes from org.a11y.Status (IsEnabled,
// ScreenReaderEnabled), followed through PropertiesChanged: while off,
// AccessibilityActive is false and the runner builds no tree.
//
// Objects. One flat subtree at /org/a11y/atspi/accessible
// (g_dbus_connection_register_subtree, dispatched on the main loop):
//
//	root        the application object libatspi looks up by bus name
//	c<key>      a canvas's root (whatever tree.Root is; it exists before
//	            the first tree, so it can be embedded early)
//	c<key>_<id> node id of that canvas
//
// Hooking into GTK's tree. The canvas widget is a TangoCanvasArea, a
// GtkDrawingArea subclass whose get_accessible returns an AtkSocket. Once
// our connection is up the socket is embedded with the plug id
// "<our name>:<canvas root path>"; atk-bridge then answers references to
// the socket with our canvas root and tells us the socket's path
// (Socket.Embedded), which becomes the canvas root's Parent. Without
// atk-bridge (NO_AT_BRIDGE=1, GTK_A11Y=none, or TANGO_ATSPI_EMBED=registry)
// our application root is embedded in the registry instead
// (org.a11y.atspi.Socket.Embed on org.a11y.atspi.Registry), so the canvas
// shows as a separate application.
//
// Events. Tree updates become object:children-changed, property-change
// (accessible-name / -description / -value), text-changed and
// state-changed; focus is state-changed:focused; Context.Announce is the
// Announcement signal. Live regions are exposed the native AT-SPI way:
// live / container-live attributes plus the text and children events of
// the region, which screen readers turn into speech themselves.

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

var _ platform.CanvasAccessible = (*canvas)(nil)

// canvasA11y is the bridge state of one canvas widget. Main thread.
type canvasA11y struct {
	key      uint64
	widget   ptr
	c        *canvas // nil until the canvas first uses the bridge
	socket   ptr     // AtkSocket returned by get_accessible (our reference)
	parentW  ptr     // widget whose accessible is the socket's parent
	embedded bool    // atk_socket_embed done

	// Parent of the canvas root, from Socket.Embedded (atk-bridge).
	parentBus, parentPath string

	tree  *a11y.Tree
	focus a11y.NodeID
}

var atspi struct {
	startOnce sync.Once
	status    atomic.Bool // org.a11y.Status says assistive technologies are on
	active    atomic.Bool // status and our objects are exported

	conn      ptr // accessibility bus connection (ours)
	name      string
	subtreeID uint32
	noBridge  bool
	desktop   string // registry fallback: the desktop's reference (GVariant text)
	appID     int32

	vtable  [11]ptr // GDBusInterfaceVTable
	subtree [11]ptr // GDBusSubtreeVTable
	ifaces  map[string]ptr

	seq      uint64
	byKey    map[uint64]*canvasA11y
	byWidget map[ptr]*canvasA11y
}

// atspiFFI holds the symbols the bridge needs beyond dbus_linux.go and
// canvas_ffi_linux.go. ATK symbols are optional (no socket: registry
// fallback).
var atspiFFI struct {
	once sync.Once
	err  error

	connNewForAddress  func(addr string, flags int32, observer, cancellable ptr, gerr *ptr) ptr
	registerSubtree    func(conn ptr, path string, vtable ptr, flags int32, data, free ptr, gerr *ptr) uint32
	interfaceInfoRef   func(info ptr) ptr
	malloc0            func(n uintptr) ptr
	variantGetString   func(v ptr, length ptr) ptr
	variantGetUint32   func(v ptr) uint32
	typeRegisterSimple func(parent uintptr, name string, classSize uint32, classInit ptr, instanceSize uint32, instanceInit ptr, flags int32) uintptr
	typeQuery          func(t uintptr, q *gTypeQueryMirror)
	drawingAreaType    func() uintptr
	objectNewWithProps func(t uintptr, n uint32, names, values ptr) ptr
	objectNewv         func(t uintptr, n uint32, params ptr) ptr

	widgetGetToplevel   func(w ptr) ptr
	widgetTranslate     func(src, dst ptr, x, y int32, dx, dy *int32) bool
	widgetGetParent     func(w ptr) ptr
	widgetGetAccessible func(w ptr) ptr
	windowGetOrigin     func(win ptr, x, y *int32) int32
	appName             func() ptr // g_get_application_name, optional

	atk          ptr
	socketNew    func() ptr
	socketEmbed  func(s ptr, plugID string)
	objSetParent func(o, parent ptr)
}

func loadAtspiFFI() error {
	atspiFFI.once.Do(func() {
		if err := loadDBus(); err != nil {
			atspiFFI.err = err
			return
		}
		if err := loadCanvasFFI(); err != nil {
			atspiFFI.err = err
			return
		}
		f := &atspiFFI
		var bd binder
		io, g, o, t, d := libGIO, libGLib, libGObject, libGTK, canvasFFI.gdk
		bd.req(&f.connNewForAddress, "g_dbus_connection_new_for_address_sync", io)
		bd.req(&f.registerSubtree, "g_dbus_connection_register_subtree", io)
		bd.req(&f.interfaceInfoRef, "g_dbus_interface_info_ref", io)
		bd.req(&f.malloc0, "g_malloc0", g)
		bd.req(&f.variantGetString, "g_variant_get_string", g)
		bd.req(&f.variantGetUint32, "g_variant_get_uint32", g)
		bd.req(&f.typeRegisterSimple, "g_type_register_static_simple", o)
		bd.req(&f.typeQuery, "g_type_query", o)
		bd.req(&f.drawingAreaType, "gtk_drawing_area_get_type", t)
		if !bd.opt(&f.objectNewWithProps, "g_object_new_with_properties", o) { // GLib 2.54+
			bd.req(&f.objectNewv, "g_object_newv", o)
		}
		bd.req(&f.widgetGetToplevel, "gtk_widget_get_toplevel", t)
		bd.req(&f.widgetTranslate, "gtk_widget_translate_coordinates", t)
		bd.req(&f.widgetGetParent, "gtk_widget_get_parent", t)
		bd.req(&f.widgetGetAccessible, "gtk_widget_get_accessible", t)
		bd.req(&f.windowGetOrigin, "gdk_window_get_origin", d, t)
		bd.opt(&f.appName, "g_get_application_name", g)
		if len(bd.missing) > 0 {
			atspiFFI.err = errors.New("tango/linux: a11y: missing native symbols: " + strings.Join(bd.missing, ", "))
			return
		}
		if h, _, err := open("libatk-1.0.so.0"); err == nil {
			f.atk = h
			if !(bd.opt(&f.socketNew, "atk_socket_new", h) &&
				bd.opt(&f.socketEmbed, "atk_socket_embed", h) &&
				bd.opt(&f.objSetParent, "atk_object_set_parent", h)) {
				f.socketNew = nil
			}
		}
	})
	return atspiFFI.err
}

// ---- TangoCanvasArea ----

var canvasArea struct {
	once         sync.Once
	typ          uintptr
	parentGetAcc ptr // GtkWidgetClass.get_accessible of GtkDrawingArea
	classInit    ptr
	getAcc       ptr
}

// newCanvasArea creates the canvas widget: a TangoCanvasArea when the
// subclass can be registered, else a plain GtkDrawingArea. Like
// gtk_drawing_area_new it returns a floating reference. Main thread.
func newCanvasArea() ptr {
	canvasArea.once.Do(registerCanvasArea)
	if canvasArea.typ != 0 {
		f := &atspiFFI
		var w ptr
		if f.objectNewWithProps != nil {
			w = f.objectNewWithProps(canvasArea.typ, 0, 0, 0)
		} else {
			w = f.objectNewv(canvasArea.typ, 0, 0)
		}
		if w != 0 {
			return w
		}
	}
	return canvasFFI.drawingAreaNew()
}

func registerCanvasArea() {
	if unsafe.Sizeof(ptr(0)) != 8 || loadAtspiFFI() != nil {
		return
	}
	f := &atspiFFI
	parent := f.drawingAreaType()
	var q gTypeQueryMirror
	f.typeQuery(parent, &q)
	if q.classSize < gtkWidgetClassGetAccessible+8 || q.instanceSize == 0 {
		return
	}
	canvasArea.getAcc = purego.NewCallback(func(widget ptr) ptr { return canvasGetAccessible(widget) })
	canvasArea.classInit = purego.NewCallback(func(klass, _ ptr) {
		slot := (*ptr)(unsafe.Add(toPtr(klass), gtkWidgetClassGetAccessible))
		canvasArea.parentGetAcc = *slot // GtkDrawingArea's, copied into our class
		if atspiFFI.socketNew != nil {
			*slot = canvasArea.getAcc
		}
	})
	canvasArea.typ = f.typeRegisterSimple(parent, "TangoCanvasArea", q.classSize, canvasArea.classInit, q.instanceSize, 0, 0)
}

// canvasGetAccessible is TangoCanvasArea's get_accessible: an AtkSocket
// that atk-bridge resolves to our canvas root once embedded.
func canvasGetAccessible(widget ptr) ptr {
	e := atspiEntry(widget)
	f := &atspiFFI
	if e.socket == 0 {
		e.socket = f.socketNew()
		if e.socket == 0 {
			if canvasArea.parentGetAcc != 0 {
				r, _, _ := purego.SyscallN(canvasArea.parentGetAcc, widget)
				return r
			}
			return 0
		}
	}
	if p := f.widgetGetParent(widget); p != e.parentW {
		e.parentW = p
		var pa ptr
		if p != 0 {
			pa = f.widgetGetAccessible(p)
		}
		f.objSetParent(e.socket, pa)
	}
	atspiStart() // GTK asks for accessibles when an AT is around
	e.embed()
	return e.socket
}

// embed hands the socket our canvas root as plug once the connection is
// up.
func (e *canvasA11y) embed() {
	if e.embedded || e.socket == 0 || atspi.conn == 0 || atspi.noBridge {
		return
	}
	e.embedded = true
	atspiFFI.socketEmbed(e.socket, atspiPlugID(atspi.name, atspiPath(atspiObj{key: e.key, alias: true})))
}

// ---- entries ----

func atspiEntry(widget ptr) *canvasA11y {
	if atspi.byWidget == nil {
		atspi.byWidget = map[ptr]*canvasA11y{}
		atspi.byKey = map[uint64]*canvasA11y{}
	}
	if e := atspi.byWidget[widget]; e != nil {
		return e
	}
	atspi.seq++
	e := &canvasA11y{key: atspi.seq, widget: widget}
	atspi.byWidget[widget] = e
	atspi.byKey[e.key] = e
	return e
}

// a11y binds the canvas to its bridge entry. Main thread.
func (c *canvas) a11y() *canvasA11y {
	if c.ax != nil {
		return c.ax
	}
	e := atspiEntry(c.widget)
	e.c = c
	c.ax = e
	connect(c.widget, "notify::has-focus", 1, func(_, _ ptr) ptr {
		e.focusMoved()
		return 0
	})
	return e
}

// a11yDestroy forgets the canvas (from destroy). Main thread.
func (c *canvas) a11yDestroy() {
	e := c.ax
	if e == nil && atspi.byWidget != nil {
		e = atspi.byWidget[c.widget]
	}
	if e == nil {
		return
	}
	if atspi.active.Load() && e.tree != nil {
		e.emit(atspiObj{key: e.key, alias: true}, "StateChanged", "defunct", 1, 0, "")
	}
	delete(atspi.byWidget, e.widget)
	delete(atspi.byKey, e.key)
	if e.socket != 0 {
		gObjectUnref(e.socket)
		e.socket = 0
	}
	e.tree, e.c = nil, nil
	c.ax = nil
}

// ---- activation ----

const (
	a11yBusName  = "org.a11y.Bus"
	a11yBusPath  = "/org/a11y/bus"
	a11yStatusIf = "org.a11y.Status"
)

// atspiStart reads org.a11y.Status once and follows it. Main thread.
func atspiStart() {
	atspi.startOnce.Do(func() {
		if loadAtspiFFI() != nil {
			return
		}
		sess, err := bus(gBusTypeSession)
		if err != nil {
			return
		}
		atspi.noBridge = os.Getenv("NO_AT_BRIDGE") == "1" || os.Getenv("GTK_A11Y") == "none" ||
			os.Getenv("TANGO_ATSPI_EMBED") == "registry" || atspiFFI.socketNew == nil
		subscribe(sess, a11yBusName, "org.freedesktop.DBus.Properties", "PropertiesChanged", a11yBusPath, a11yStatusIf,
			func(_, _, _, _ string, _ ptr) { atspiStatusChanged(sess) })
		atspiStatusChanged(sess)
	})
}

func atspiReadStatus(sess ptr) bool {
	on := false
	for _, p := range []string{"IsEnabled", "ScreenReaderEnabled"} {
		v, err := dbusGet(sess, a11yBusName, a11yBusPath, a11yStatusIf, p)
		if err != nil {
			continue
		}
		if gvType(v) == "b" && gd.vBoolean(v) {
			on = true
		}
		gVariantUnref(v)
	}
	return on
}

func atspiStatusChanged(sess ptr) {
	on := atspiReadStatus(sess)
	atspi.status.Store(on)
	if on && atspi.conn == 0 {
		if err := atspiConnect(sess); err != nil {
			on = false
		}
	}
	if atspi.active.Swap(on) == on {
		return
	}
	for _, e := range atspi.byKey {
		if e.c != nil && !e.c.closed {
			e.c.Invalidate() // the next frame builds (or drops) the tree
		}
	}
}

// atspiConnect opens the accessibility bus, exports the subtree and
// embeds the canvases. Main thread.
func atspiConnect(sess ptr) error {
	reply, err := dbusCall(sess, a11yBusName, a11yBusPath, a11yBusName, "GetAddress", "", 3000)
	if err != nil {
		return err
	}
	av := gvChild(reply, 0)
	addr := gvStr(av)
	gVariantUnref(av)
	gVariantUnref(reply)
	if addr == "" {
		return errors.New("tango/linux: a11y: no accessibility bus address")
	}
	f := &atspiFFI
	var gerr ptr
	conn := f.connNewForAddress(addr, 1|8, 0, 0, &gerr) // AUTHENTICATION_CLIENT | MESSAGE_BUS_CONNECTION
	if conn == 0 {
		return errors.New("tango/linux: a11y bus: " + gErrorMessage(gerr))
	}
	if err := atspiExport(conn); err != nil {
		gObjectUnref(conn)
		return err
	}
	atspi.conn = conn
	atspi.name = goStr(gd.uniqueName(conn))
	if atspi.noBridge {
		root := gvRef(atspi.name, atspiRootPath)
		if r, err := dbusCall(conn, atspiRegistryName, atspiRootPath, atspiIfaceSocket, "Embed", "("+root+",)", 3000); err == nil {
			atspi.desktop = gvRefText(gvChild(r, 0))
			gVariantUnref(r)
		}
	}
	for _, e := range atspi.byKey {
		e.embed()
	}
	return nil
}

// gvRefText turns a (so) value into gvRef text and unrefs it.
func gvRefText(v ptr) string {
	if v == 0 {
		return ""
	}
	defer gVariantUnref(v)
	b, p := gvChild(v, 0), gvChild(v, 1)
	defer gVariantUnref(b)
	defer gVariantUnref(p)
	return gvRef(gvStr(b), gvStr(p))
}

func atspiExport(conn ptr) error {
	var gerr ptr
	node := gd.nodeInfoNew(atspiIntrospection, &gerr)
	if node == 0 {
		return errors.New("tango/linux: a11y XML: " + gErrorMessage(gerr))
	}
	atspi.ifaces = map[string]ptr{}
	for _, name := range []string{atspiIfaceAccessible, atspiIfaceApplication, atspiIfaceComponent, atspiIfaceAction,
		atspiIfaceValue, atspiIfaceText, atspiIfaceEditableText, atspiIfaceSocket} {
		atspi.ifaces[name] = gd.nodeLookupIface(node, name) // node is kept forever
	}
	atspi.vtable[0] = purego.NewCallback(func(_, sender, path, iface, method, params, inv, _ ptr) {
		atspiCall(goStr(sender), goStr(path), goStr(iface), goStr(method), params, inv)
	})
	atspi.vtable[1] = purego.NewCallback(func(_, _, path, iface, prop, gerr, _ ptr) ptr {
		v := atspiProperty(goStr(path), goStr(iface), goStr(prop))
		if v == 0 && gerr != 0 {
			*(*ptr)(toPtr(gerr)) = gErrorNewLiteral(gd.errorQuark(), 16, "unknown property "+goStr(prop)) // G_DBUS_ERROR_INVALID_ARGS
		}
		return v
	})
	atspi.vtable[2] = purego.NewCallback(func(_, _, path, iface, prop, value, _, _ ptr) ptr {
		return gbool(atspiSetProperty(goStr(path), goStr(iface), goStr(prop), value))
	})
	atspi.subtree[0] = purego.NewCallback(func(_, _, _, _ ptr) ptr {
		return atspiFFI.malloc0(8) // no enumerated children (dispatch reaches unenumerated nodes)
	})
	atspi.subtree[1] = purego.NewCallback(func(_, _, _, node, _ ptr) ptr {
		names := atspiNodeInterfaces(goStr(node))
		if names == nil {
			return 0
		}
		arr := atspiFFI.malloc0(uintptr(len(names)+1) * 8)
		for i, n := range names {
			*(*ptr)(unsafe.Add(toPtr(arr), i*8)) = atspiFFI.interfaceInfoRef(atspi.ifaces[n])
		}
		return arr
	})
	atspi.subtree[2] = purego.NewCallback(func(_, _, _, iface, node, out, _ ptr) ptr {
		if _, ok := atspiParse(goStr(node)); !ok || atspi.ifaces[goStr(iface)] == 0 {
			return 0
		}
		*(*ptr)(toPtr(out)) = 0
		return ptr(unsafe.Pointer(&atspi.vtable[0]))
	})
	id := atspiFFI.registerSubtree(conn, atspiBase, ptr(unsafe.Pointer(&atspi.subtree[0])), 1, 0, 0, &gerr) // DISPATCH_TO_UNENUMERATED_NODES
	if id == 0 {
		return errors.New("tango/linux: a11y subtree: " + gErrorMessage(gerr))
	}
	atspi.subtreeID = id
	return nil
}

// ---- resolving objects ----

// atspiTarget is a resolved object: the app root, or a node of a canvas.
type atspiTarget struct {
	obj atspiObj
	e   *canvasA11y
	n   *a11y.Node // nil: the app root, or a node that no longer exists
}

func atspiResolve(path string) (atspiTarget, bool) {
	o, ok := atspiParse(path)
	if !ok {
		return atspiTarget{}, false
	}
	if o.app {
		return atspiTarget{obj: o}, true
	}
	e := atspi.byKey[o.key]
	if e == nil {
		return atspiTarget{}, false
	}
	t := atspiTarget{obj: o, e: e}
	switch {
	case o.alias && e.tree != nil:
		t.n = e.tree.Node(e.tree.Root)
	case o.alias:
		w, h := e.size()
		t.n = &a11y.Node{Role: a11y.RoleWindow, Bounds: a11y.Rect{W: w, H: h}}
	default:
		t.n = e.tree.Node(o.id)
		if t.n != nil && t.n.ID == e.tree.Root {
			t.obj = atspiObj{key: e.key, alias: true}
		}
	}
	return t, true
}

func atspiNodeInterfaces(node string) []string {
	t, ok := atspiResolve(node)
	if !ok {
		return nil
	}
	if t.obj.app {
		return []string{atspiIfaceAccessible, atspiIfaceApplication}
	}
	if t.n == nil {
		return []string{atspiIfaceAccessible}
	}
	out := atspiInterfaces(t.n)
	if t.obj.alias {
		out = append(out, atspiIfaceSocket)
	}
	return out
}

func (e *canvasA11y) size() (float32, float32) {
	f := &canvasFFI
	return float32(max(f.widgetGetAllocatedWidth(e.widget), 0)), float32(max(f.widgetGetAllocatedHeight(e.widget), 0))
}

// objOf is the object of node id (the alias for the tree's root).
func (e *canvasA11y) objOf(id a11y.NodeID) atspiObj {
	if e.tree == nil || id == e.tree.Root || id == 0 {
		return atspiObj{key: e.key, alias: true}
	}
	return atspiObj{key: e.key, id: id}
}

func (e *canvasA11y) ref(id a11y.NodeID) string { return gvRef(atspi.name, atspiPath(e.objOf(id))) }

func appRef() string { return gvRef(atspi.name, atspiRootPath) }

func (t atspiTarget) parentRef() string {
	switch {
	case t.obj.app:
		if atspi.desktop != "" {
			return atspi.desktop
		}
		return gvRef("", "")
	case t.obj.alias:
		if t.e.parentBus != "" {
			return gvRef(t.e.parentBus, t.e.parentPath)
		}
		return appRef()
	case t.n == nil:
		return gvRef("", "")
	}
	return t.e.ref(t.n.Parent)
}

func (t atspiTarget) children() []string {
	if t.obj.app {
		keys := make([]uint64, 0, len(atspi.byKey))
		for k := range atspi.byKey {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = gvRef(atspi.name, atspiPath(atspiObj{key: k, alias: true}))
		}
		return out
	}
	if t.n == nil || t.e.tree == nil {
		return nil
	}
	out := make([]string, 0, len(t.n.Children))
	for _, k := range t.n.Children {
		out = append(out, t.e.ref(k))
	}
	return out
}

func (t atspiTarget) name() string {
	switch {
	case t.obj.app:
		if atspiFFI.appName != nil {
			return goStr(atspiFFI.appName())
		}
		return ""
	case t.n == nil:
		return ""
	case t.n.Name == "" && t.n.LabelledBy != 0 && t.e.tree != nil:
		return t.e.tree.Text(t.n.LabelledBy)
	}
	return t.n.Name
}

func (t atspiTarget) description() string {
	if t.n == nil {
		return ""
	}
	if t.n.Description == "" && t.n.DescribedBy != 0 && t.e.tree != nil {
		return t.e.tree.Text(t.n.DescribedBy)
	}
	return t.n.Description
}

func (t atspiTarget) role() uint32 {
	if t.obj.app {
		id, _ := a11ybridge.ATSPIRoleID("APPLICATION")
		return id
	}
	if t.n == nil {
		id, _ := a11ybridge.ATSPIRoleID("INVALID")
		return id
	}
	return a11ybridge.ATSPIRole(t.n)
}

func (t atspiTarget) focused() bool {
	return t.e != nil && canvasFFI.widgetHasFocus(t.e.widget)
}

func (t atspiTarget) states() [2]uint32 {
	switch {
	case t.obj.app:
		return [2]uint32{}
	case t.n == nil:
		return [2]uint32{1 << a11ybridge.StateDefunct}
	}
	// FOCUSED follows the effective focus (ActiveDescendant resolved)
	// while the widget itself has keyboard focus.
	set := a11ybridge.ATSPIStates(t.n, false)
	if t.e.tree != nil && t.n.ID != 0 && t.n.ID == a11ybridge.EffectiveFocus(t.e.tree) && t.focused() {
		set[0] |= 1 << a11ybridge.StateFocused
	}
	return set
}

// ---- method calls ----

func returnErr(inv ptr, msg string) {
	gd.invReturnError(inv, "org.freedesktop.DBus.Error.Failed", msg)
}

func pInt(params ptr, i int) int32 {
	v := gvChild(params, i)
	defer gVariantUnref(v)
	return gd.vInt32(v)
}

func pUint(params ptr, i int) uint32 {
	v := gvChild(params, i)
	defer gVariantUnref(v)
	return atspiFFI.variantGetUint32(v)
}

func pStr(params ptr, i int) string {
	v := gvChild(params, i)
	defer gVariantUnref(v)
	return gvStr(v)
}

func tuple(s string) string { return "(" + s + ",)" }

func i32(v int) string { return "int32 " + strconv.Itoa(v) }

func gvBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func atspiCall(sender, path, iface, method string, params, inv ptr) {
	t, ok := atspiResolve(path)
	if !ok {
		gd.invReturnError(inv, "org.freedesktop.DBus.Error.UnknownObject", "no such object")
		return
	}
	switch iface {
	case atspiIfaceAccessible:
		t.accessibleCall(method, params, inv)
	case atspiIfaceApplication:
		returnValue(inv, tuple(gvString("")))
	case atspiIfaceSocket:
		if method == "Embedded" && t.obj.alias {
			t.e.parentBus, t.e.parentPath = sender, pStr(params, 0)
		}
		returnValue(inv, "")
	default:
		if t.n == nil || t.e == nil {
			gd.invReturnError(inv, "org.freedesktop.DBus.Error.UnknownObject", "defunct object")
			return
		}
		switch iface {
		case atspiIfaceComponent:
			t.componentCall(method, params, inv)
		case atspiIfaceAction:
			t.actionCall(method, params, inv)
		case atspiIfaceText:
			t.textCall(method, params, inv)
		case atspiIfaceEditableText:
			returnValue(inv, tuple(gvBool(t.perform(a11y.SetValue, pStr(params, 0)) == nil)))
		default:
			returnErr(inv, "unsupported interface")
		}
	}
}

func (t atspiTarget) accessibleCall(method string, params, inv ptr) {
	switch method {
	case "GetChildAtIndex":
		kids, i := t.children(), int(pInt(params, 0))
		if i < 0 || i >= len(kids) {
			returnValue(inv, tuple(gvRef("", "")))
			return
		}
		returnValue(inv, tuple(kids[i]))
	case "GetChildren":
		returnValue(inv, tuple(gvRefs(t.children())))
	case "GetIndexInParent":
		idx := 0
		switch {
		case t.obj.app:
			idx = -1
		case t.obj.alias:
			idx = 0
		case t.n != nil:
			idx = a11ybridge.IndexInParent(t.e.tree, t.n.ID)
		default:
			idx = -1
		}
		returnValue(inv, tuple(i32(idx)))
	case "GetRelationSet":
		var parts []string
		if t.n != nil && t.e.tree != nil {
			for _, r := range atspiRelations(t.e.tree, t.n) {
				parts = append(parts, "(uint32 "+strconv.Itoa(int(r.Type))+", ["+t.e.ref(r.Target)+"])")
			}
		}
		if len(parts) == 0 {
			returnValue(inv, tuple("@a(ua(so)) []"))
			return
		}
		returnValue(inv, tuple("["+strings.Join(parts, ", ")+"]"))
	case "GetRole":
		returnValue(inv, tuple("uint32 "+strconv.FormatUint(uint64(t.role()), 10)))
	case "GetRoleName":
		returnValue(inv, tuple(gvString(a11ybridge.ATSPIRoleName(t.role()))))
	case "GetLocalizedRoleName":
		name := a11ybridge.ATSPIRoleName(t.role())
		if t.n != nil && t.n.RoleDescription != "" {
			name = t.n.RoleDescription
		}
		returnValue(inv, tuple(gvString(name)))
	case "GetState":
		returnValue(inv, tuple(gvStates(t.states())))
	case "GetAttributes":
		m := map[string]string{"toolkit": "tango"}
		if t.n != nil && t.e != nil {
			var live *a11y.Node
			if t.e.tree != nil {
				live = a11ybridge.LiveRoot(t.e.tree, t.n.ID)
			}
			m = atspiAttributes(t.n, live)
		}
		returnValue(inv, tuple(gvDict(m)))
	case "GetApplication":
		returnValue(inv, tuple(appRef()))
	case "GetInterfaces":
		returnValue(inv, tuple(gvStrings(atspiNodeInterfaces(atspiNodeName(t.obj)))))
	default:
		returnErr(inv, "unknown method "+method)
	}
}

// offset is where canvas coordinate (0, 0) is in coordType space.
func (t atspiTarget) offset(coord uint32) (float64, float64) {
	f := &atspiFFI
	w := t.e.widget
	if coord == atspiCoordParent {
		if t.obj.alias || t.n.Parent == 0 || t.e.tree == nil {
			return 0, 0
		}
		if p := t.e.tree.Node(t.n.Parent); p != nil {
			return -float64(p.Bounds.X), -float64(p.Bounds.Y)
		}
		return 0, 0
	}
	top := f.widgetGetToplevel(w)
	var x, y int32
	if top != 0 && top != w {
		f.widgetTranslate(w, top, 0, 0, &x, &y)
	}
	if coord == atspiCoordScreen && top != 0 {
		if win := canvasFFI.widgetGetWindow(top); win != 0 {
			var ox, oy int32
			f.windowGetOrigin(win, &ox, &oy) // Wayland: 0,0 (window coordinates)
			x, y = x+ox, y+oy
		}
	}
	return float64(x), float64(y)
}

func (t atspiTarget) extents(coord uint32) (x, y, w, h int32) {
	dx, dy := t.offset(coord)
	return a11ybridge.Offset(t.n.Bounds, dx, dy)
}

// toCanvas converts a point in coordType space to canvas coordinates.
func (t atspiTarget) toCanvas(x, y int32, coord uint32) (float32, float32) {
	if coord == atspiCoordParent { // relative to t itself for hit testing
		return t.n.Bounds.X + float32(x), t.n.Bounds.Y + float32(y)
	}
	dx, dy := t.offset(coord)
	return float32(float64(x) - dx), float32(float64(y) - dy)
}

func (t atspiTarget) componentCall(method string, params, inv ptr) {
	switch method {
	case "GetExtents":
		x, y, w, h := t.extents(pUint(params, 0))
		returnValue(inv, tuple("("+i32(int(x))+", "+strconv.Itoa(int(y))+", "+strconv.Itoa(int(w))+", "+strconv.Itoa(int(h))+")"))
	case "GetPosition":
		x, y, _, _ := t.extents(pUint(params, 0))
		returnValue(inv, "("+i32(int(x))+", "+i32(int(y))+")")
	case "GetSize":
		_, _, w, h := t.extents(atspiCoordWindow)
		returnValue(inv, "("+i32(int(w))+", "+i32(int(h))+")")
	case "Contains":
		px, py := t.toCanvas(pInt(params, 0), pInt(params, 1), pUint(params, 2))
		b := t.n.Bounds
		returnValue(inv, tuple(gvBool(px >= b.X && py >= b.Y && px < b.X+b.W && py < b.Y+b.H)))
	case "GetAccessibleAtPoint":
		px, py := t.toCanvas(pInt(params, 0), pInt(params, 1), pUint(params, 2))
		returnValue(inv, tuple(t.childAt(px, py)))
	case "GetLayer":
		returnValue(inv, tuple("uint32 3")) // ATSPI_LAYER_WIDGET
	case "GetMDIZOrder":
		returnValue(inv, tuple("int16 0"))
	case "GetAlpha":
		returnValue(inv, tuple("1.0"))
	case "GrabFocus":
		returnValue(inv, tuple(gvBool(t.perform(a11y.Focus, "") == nil)))
	case "ScrollTo", "ScrollToPoint":
		returnValue(inv, tuple(gvBool(t.perform(a11y.ScrollIntoView, "") == nil)))
	default:
		returnErr(inv, "unknown method "+method)
	}
}

// childAt returns the child of t on the path to the deepest node at
// (x, y), or a null reference.
func (t atspiTarget) childAt(x, y float32) string {
	tr := t.e.tree
	if tr == nil {
		return gvRef("", "")
	}
	hit := a11ybridge.HitTest(tr, x, y)
	for n := tr.Node(hit); n != nil && n.ID != t.n.ID; n = tr.Node(n.Parent) {
		if n.Parent == t.n.ID {
			return t.e.ref(n.ID)
		}
	}
	return gvRef("", "")
}

func (t atspiTarget) actionCall(method string, params, inv ptr) {
	acts := a11ybridge.ATSPIActions(t.n)
	at := func() (a11ybridge.ATSPIAction, bool) {
		i := int(pInt(params, 0))
		if i < 0 || i >= len(acts) {
			return a11ybridge.ATSPIAction{}, false
		}
		return acts[i], true
	}
	switch method {
	case "GetName", "GetLocalizedName":
		a, _ := at()
		returnValue(inv, tuple(gvString(a.Name)))
	case "GetDescription":
		returnValue(inv, tuple(gvString("")))
	case "GetKeyBinding":
		kb := ""
		if i := int(pInt(params, 0)); i == 0 && len(acts) > 0 {
			kb = t.n.Shortcut
		}
		returnValue(inv, tuple(gvString(kb)))
	case "GetActions":
		if len(acts) == 0 {
			returnValue(inv, tuple("@a(sss) []"))
			return
		}
		parts := make([]string, len(acts))
		for i, a := range acts {
			kb := ""
			if i == 0 {
				kb = t.n.Shortcut
			}
			parts[i] = "(" + gvString(a.Name) + ", '', " + gvString(kb) + ")"
		}
		returnValue(inv, tuple("["+strings.Join(parts, ", ")+"]"))
	case "DoAction":
		a, ok := at()
		returnValue(inv, tuple(gvBool(ok && t.perform(a.Act, "") == nil)))
	default:
		returnErr(inv, "unknown method "+method)
	}
}

func (t atspiTarget) textCall(method string, params, inv ptr) {
	s := atspiText(t.n)
	n := len([]rune(s))
	switch method {
	case "GetText":
		returnValue(inv, tuple(gvString(runeSlice(s, int(pInt(params, 0)), int(pInt(params, 1))))))
	case "GetCharacterAtOffset":
		r := []rune(s)
		c, i := 0, int(pInt(params, 0))
		if i >= 0 && i < len(r) {
			c = int(r[i])
		}
		returnValue(inv, tuple(i32(c)))
	case "GetStringAtOffset", "GetTextAtOffset":
		g := pUint(params, 1)
		if method == "GetTextAtOffset" { // AtspiTextBoundaryType: CHAR, WORD_START, WORD_END, …
			switch g {
			case 0:
			case 1, 2:
				g = atspiGranularityWord
			default:
				g = 4
			}
		}
		str, a, b := atspiStringAt(s, int(pInt(params, 0)), g)
		returnValue(inv, "("+gvString(str)+", "+i32(a)+", "+i32(b)+")")
	case "GetNSelections":
		returnValue(inv, tuple(i32(0)))
	case "GetCharacterExtents":
		x, y, w, h := t.extents(pUint(params, 1))
		if n > 0 {
			w /= int32(n) // no glyph positions: an even share of the node
			x += w * min(max(pInt(params, 0), 0), int32(n-1))
		}
		returnValue(inv, "("+i32(int(x))+", "+i32(int(y))+", "+i32(int(w))+", "+i32(int(h))+")")
	case "GetOffsetAtPoint":
		returnValue(inv, tuple(i32(-1)))
	case "GetAttributeValue":
		returnValue(inv, tuple(gvString(atspiTextAttributes(t.n)[pStr(params, 1)])))
	case "GetAttributes", "GetAttributeRun":
		// One run covering the whole text (see atspiTextAttributes); the
		// attributes are also the defaults, so includeDefaults changes
		// nothing.
		returnValue(inv, "("+gvDict(atspiTextAttributes(t.n))+", "+i32(0)+", "+i32(n)+")")
	case "GetDefaultAttributes", "GetDefaultAttributeSet":
		returnValue(inv, tuple(gvDict(atspiTextAttributes(t.n))))
	default:
		returnErr(inv, "unknown method "+method)
	}
}

// perform runs act through the sink. Main thread, outside CanvasFrame
// (D-Bus calls are dispatched by the main loop between frames).
func (t atspiTarget) perform(act a11y.Action, arg string) error {
	c := t.e.c
	if c == nil || c.closed || t.n == nil {
		return errors.New("tango/linux: a11y: canvas gone")
	}
	sink, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return errors.New("tango/linux: a11y: sink takes no actions")
	}
	if act == a11y.Focus {
		canvasFFI.widgetGrabFocus(c.widget)
	}
	id := t.n.ID
	if c.inFrame { // never from inside CanvasFrame
		later(0, func() { _ = sink.AccessibilityAction(id, act, arg) })
		return nil
	}
	return sink.AccessibilityAction(id, act, arg)
}

// ---- properties ----

func atspiProperty(path, iface, prop string) ptr {
	t, ok := atspiResolve(path)
	if !ok {
		return 0
	}
	switch iface {
	case atspiIfaceAccessible:
		switch prop {
		case "Name":
			return propValue(gvString(t.name()))
		case "Description", "HelpText":
			return propValue(gvString(t.description()))
		case "Parent":
			return propValue(t.parentRef())
		case "ChildCount":
			return propValue(i32(len(t.children())))
		case "Locale":
			return propValue(gvString(""))
		case "AccessibleId":
			id := ""
			if t.n != nil && t.n.ID != 0 {
				id = strconv.FormatUint(uint64(t.n.ID), 16)
			}
			return propValue(gvString(id))
		}
	case atspiIfaceApplication:
		switch prop {
		case "ToolkitName":
			return propValue(gvString("Tango"))
		case "Version":
			return propValue(gvString("1"))
		case "AtspiVersion":
			return propValue(gvString("2.1"))
		case "Id":
			return propValue(i32(int(atspi.appID)))
		}
	}
	if t.n == nil {
		return 0
	}
	switch iface {
	case atspiIfaceAction:
		if prop == "NActions" {
			return propValue(i32(len(a11ybridge.ATSPIActions(t.n))))
		}
	case atspiIfaceValue:
		switch prop {
		case "MinimumValue":
			return propValue(gvDouble(t.n.ValueMin))
		case "MaximumValue":
			return propValue(gvDouble(t.n.ValueMax))
		case "MinimumIncrement":
			return propValue(gvDouble(t.n.Step))
		case "CurrentValue":
			return propValue(gvDouble(t.n.ValueNow))
		case "Text":
			return propValue(gvString(t.n.Value))
		}
	case atspiIfaceText:
		switch prop {
		case "CharacterCount":
			return propValue(i32(len([]rune(atspiText(t.n)))))
		case "CaretOffset":
			return propValue(i32(len([]rune(atspiText(t.n))))) // caret position is not exposed: the end
		}
	}
	return 0
}

func atspiSetProperty(path, iface, prop string, value ptr) bool {
	t, ok := atspiResolve(path)
	if !ok || value == 0 {
		return false
	}
	switch {
	case iface == atspiIfaceApplication && prop == "Id" && gvType(value) == "i":
		atspi.appID = gd.vInt32(value)
		return true
	case iface == atspiIfaceValue && prop == "CurrentValue" && t.n != nil && gvType(value) == "d":
		v := gd.vDouble(value)
		act := a11ybridge.RangeAction(t.n, v)
		if act == 0 {
			return v == t.n.ValueNow
		}
		arg := ""
		if act == a11y.SetValue {
			arg = strconv.FormatFloat(v, 'g', -1, 64)
		}
		return t.perform(act, arg) == nil
	}
	return false
}

// ---- platform.CanvasAccessible ----

// AccessibilityActive implements platform.CanvasAccessible: an AT-SPI
// client is enabled and our objects are exported. Called every frame from
// the UI (main) thread; the first call starts following org.a11y.Status.
func (c *canvas) AccessibilityActive() bool {
	if c.closed {
		return false
	}
	c.a11y()
	atspiStart()
	return atspi.active.Load()
}

// SetAccessibilityTree implements platform.CanvasAccessible.
func (c *canvas) SetAccessibilityTree(t *a11y.Tree) {
	if c.closed {
		return
	}
	e := c.a11y()
	old := e.tree
	alias := atspiObj{key: e.key, alias: true}
	if old != nil {
		if r := old.Node(old.Root); r != nil {
			for i := len(r.Children) - 1; i >= 0; i-- {
				e.emit(alias, "ChildrenChanged", "remove", int32(i), 0, gvRef(atspi.name, atspiPath(atspiObj{key: e.key, id: r.Children[i]})))
			}
		}
	}
	e.tree = t
	if t == nil {
		e.focus = 0
		return
	}
	if r := t.Node(t.Root); r != nil {
		for i, k := range r.Children {
			e.emit(alias, "ChildrenChanged", "add", int32(i), 0, e.ref(k))
		}
		e.emit(alias, "PropertyChange", "accessible-name", 0, 0, gvString(r.Name))
	}
}

// UpdateAccessibilityNodes implements platform.CanvasAccessible.
func (c *canvas) UpdateAccessibilityNodes(t *a11y.Tree, changed []a11y.NodeID) {
	if c.closed {
		return
	}
	e := c.a11y()
	prev := e.tree
	if prev == nil {
		c.SetAccessibilityTree(t)
		return
	}
	e.tree = t
	focused := canvasFFI.widgetHasFocus(c.widget)
	for _, id := range changed {
		pn, nn := prev.Node(id), t.Node(id)
		switch d := a11ybridge.Diff(pn, nn); {
		case d.Has(a11ybridge.ChangeAdded):
			if p := t.Node(nn.Parent); p != nil && prev.Node(p.ID) != nil {
				e.emit(e.objOf(p.ID), "ChildrenChanged", "add", int32(a11ybridge.IndexInParent(t, id)), 0, e.ref(id))
			}
		case d.Has(a11ybridge.ChangeRemoved):
			obj := atspiObj{key: e.key, id: id}
			if p := t.Node(pn.Parent); p != nil {
				e.emit(e.objOf(p.ID), "ChildrenChanged", "remove", int32(a11ybridge.IndexInParent(prev, id)), 0, gvRef(atspi.name, atspiPath(obj)))
			}
			e.emit(obj, "StateChanged", "defunct", 1, 0, "")
		default:
			e.nodeChanged(pn, nn, focused)
		}
	}
}

// nodeChanged emits the events for a modified node.
func (e *canvasA11y) nodeChanged(pn, nn *a11y.Node, focused bool) {
	obj := e.objOf(nn.ID)
	if pn.Name != nn.Name {
		e.emit(obj, "PropertyChange", "accessible-name", 0, 0, gvString(nn.Name))
	}
	if pn.Description != nn.Description {
		e.emit(obj, "PropertyChange", "accessible-description", 0, 0, gvString(nn.Description))
	}
	if nn.HasRange && (pn.ValueNow != nn.ValueNow || !pn.HasRange) {
		e.emit(obj, "PropertyChange", "accessible-value", 0, 0, gvDouble(nn.ValueNow))
	}
	if atspiHasText(nn) {
		if a, b := atspiText(pn), atspiText(nn); a != b {
			if na := len([]rune(a)); na > 0 {
				e.emit(obj, "TextChanged", "delete", 0, int32(na), gvString(a))
			}
			if nb := len([]rune(b)); nb > 0 {
				e.emit(obj, "TextChanged", "insert", 0, int32(nb), gvString(b))
			}
		}
	}
	if pn.States != nn.States {
		for _, s := range a11ybridge.ATSPIStateDiff(a11ybridge.ATSPIStates(pn, focused), a11ybridge.ATSPIStates(nn, focused)) {
			if s.State == a11ybridge.StateFocused {
				continue // AccessibilityFocusChanged
			}
			on := int32(0)
			if s.On {
				on = 1
			}
			e.emit(obj, "StateChanged", a11ybridge.ATSPIStateName(s.State), on, 0, "")
		}
	}
	// Children added or removed were reported for the children themselves;
	// a pure reorder sends nothing (clients re-read GetChildren on demand).
}

// AccessibilityFocusChanged implements platform.CanvasAccessible.
func (c *canvas) AccessibilityFocusChanged(id a11y.NodeID) {
	if c.closed {
		return
	}
	e := c.a11y()
	old := e.focus
	e.focus = id
	if old != 0 && old != id {
		e.emit(e.objOf(old), "StateChanged", "focused", 0, 0, "")
	}
	if id != 0 && canvasFFI.widgetHasFocus(c.widget) {
		e.emit(e.objOf(id), "StateChanged", "focused", 1, 0, "")
	}
}

// focusMoved follows the widget's own keyboard focus: the focused node
// gains or loses FOCUSED with it.
func (e *canvasA11y) focusMoved() {
	if e.c == nil || e.c.closed || e.focus == 0 {
		return
	}
	on := int32(0)
	if canvasFFI.widgetHasFocus(e.widget) {
		on = 1
	}
	e.emit(e.objOf(e.focus), "StateChanged", "focused", on, 0, "")
}

// AccessibilityAnnounce implements platform.CanvasAccessible. Messages
// without a node go out as the Announcement signal (at-spi2-core 2.46+,
// Orca 45+). Live regions (source != 0) are announced by screen readers
// from the region's live attributes and its text and children events,
// which UpdateAccessibilityNodes sent, so nothing more is emitted for them.
func (c *canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	if c.closed || source != 0 || text == "" {
		return
	}
	e := c.a11y()
	e.emit(atspiObj{key: e.key, alias: true}, "Announcement", "", atspiPoliteness(live), 0, gvString(text))
}

// emit sends an org.a11y.atspi.Event.Object signal from obj.
func (e *canvasA11y) emit(obj atspiObj, signal, detail string, d1, d2 int32, anyData string) {
	if atspi.conn == 0 || !atspi.active.Load() {
		return
	}
	emit(atspi.conn, atspiPath(obj), atspiEventObject, signal, atspiEventArgs(detail, d1, d2, anyData))
}
