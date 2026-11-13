//go:build linux && !android

package linux

// GDBus plumbing shared by the sysx features (power, network, secure
// store, StatusNotifierItem, GlobalShortcuts portal): extra GIO / GLib
// symbols, typed variant readers, signal subscriptions and exported
// objects. Everything goes through one purego trampoline per callback
// shape, looked up by user_data, so the callback count stays fixed.

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var gd struct {
	once sync.Once
	err  error

	callFDSync        func(conn ptr, busName, path, iface, method string, params, replyType ptr, flags, timeout int32, fdList ptr, outFDList *ptr, cancellable ptr, gerr *ptr) ptr
	signalSubscribe   func(conn ptr, sender, iface, member, path, arg0 *byte, flags int32, cb, data, destroy ptr) uint32
	signalUnsubscribe func(conn ptr, id uint32)
	errorQuark        func() uint32
	emitSignal        func(conn ptr, dest *byte, path, iface, signal string, params ptr, gerr *ptr) bool
	registerObject    func(conn ptr, path string, iface, vtable, data, free ptr, gerr *ptr) uint32
	unregisterObject  func(conn ptr, id uint32) bool
	nodeInfoNew       func(xml string, gerr *ptr) ptr
	nodeLookupIface   func(node ptr, name string) ptr
	invReturnValue    func(inv, params ptr)
	invReturnError    func(inv ptr, name, msg string)
	uniqueName        func(conn ptr) ptr
	fdListGet         func(list ptr, index int32, gerr *ptr) int32

	vNChildren  func(v ptr) uintptr
	vTypeString func(v ptr) ptr
	vBoolean    func(v ptr) bool
	vInt32      func(v ptr) int32
	vDouble     func(v ptr) float64
	vVariant    func(v ptr) ptr
	vHandle     func(v ptr) int32
	vLookup     func(dict ptr, key string, typ *byte) ptr
	vIsFloating func(v ptr) bool
	vRefSink    func(v ptr) ptr
}

func loadDBus() error {
	gd.once.Do(func() {
		if err := loadSystemLibs(); err != nil {
			gd.err = err
			return
		}
		var bd binder
		io, g := libGIO, libGLib
		bd.req(&gd.callFDSync, "g_dbus_connection_call_with_unix_fd_list_sync", io)
		bd.req(&gd.signalSubscribe, "g_dbus_connection_signal_subscribe", io)
		bd.req(&gd.signalUnsubscribe, "g_dbus_connection_signal_unsubscribe", io)
		bd.req(&gd.errorQuark, "g_dbus_error_quark", io)
		bd.req(&gd.emitSignal, "g_dbus_connection_emit_signal", io)
		bd.req(&gd.registerObject, "g_dbus_connection_register_object", io)
		bd.req(&gd.unregisterObject, "g_dbus_connection_unregister_object", io)
		bd.req(&gd.nodeInfoNew, "g_dbus_node_info_new_for_xml", io)
		bd.req(&gd.nodeLookupIface, "g_dbus_node_info_lookup_interface", io)
		bd.req(&gd.invReturnValue, "g_dbus_method_invocation_return_value", io)
		bd.req(&gd.invReturnError, "g_dbus_method_invocation_return_dbus_error", io)
		bd.req(&gd.uniqueName, "g_dbus_connection_get_unique_name", io)
		bd.req(&gd.fdListGet, "g_unix_fd_list_get", io)
		bd.req(&gd.vNChildren, "g_variant_n_children", g)
		bd.req(&gd.vTypeString, "g_variant_get_type_string", g)
		bd.req(&gd.vBoolean, "g_variant_get_boolean", g)
		bd.req(&gd.vInt32, "g_variant_get_int32", g)
		bd.req(&gd.vDouble, "g_variant_get_double", g)
		bd.req(&gd.vVariant, "g_variant_get_variant", g)
		bd.req(&gd.vHandle, "g_variant_get_handle", g)
		bd.req(&gd.vLookup, "g_variant_lookup_value", g)
		bd.req(&gd.vIsFloating, "g_variant_is_floating", g)
		bd.req(&gd.vRefSink, "g_variant_ref_sink", g)
		if len(bd.missing) > 0 {
			gd.err = errors.New("tango/linux: missing GIO symbols: " + strings.Join(bd.missing, ", "))
		}
	})
	return gd.err
}

const gBusTypeSystem = 1

var busConns struct {
	mu   sync.Mutex
	conn [3]ptr // by GBusType
}

// bus returns the shared connection to the system (gBusTypeSystem) or
// session (gBusTypeSession) bus. g_bus_get_sync hands out one singleton
// per bus; we keep our reference forever.
func bus(kind int32) (ptr, error) {
	if err := loadDBus(); err != nil {
		return 0, err
	}
	busConns.mu.Lock()
	defer busConns.mu.Unlock()
	if c := busConns.conn[kind]; c != 0 {
		return c, nil
	}
	var gerr ptr
	c := gBusGetSync(kind, 0, &gerr)
	if c == 0 {
		name := "session"
		if kind == gBusTypeSystem {
			name = "system"
		}
		return 0, fmt.Errorf("%w: no D-Bus %s bus: %s", platform.ErrUnsupported, name, gErrorMessage(gerr))
	}
	busConns.conn[kind] = c
	return c, nil
}

// dbusError is a failed D-Bus call. Missing reports a service or object
// that does not exist (as opposed to a call that failed).
type dbusError struct {
	op, msg string
}

func (e *dbusError) Error() string { return "tango/linux: " + e.op + ": " + e.msg }

func (e *dbusError) missing() bool {
	for _, s := range []string{"ServiceUnknown", "UnknownObject", "UnknownMethod", "UnknownInterface", "UnknownProperty", "NameHasNoOwner", "InvalidArgs", "No such interface"} {
		if strings.Contains(e.msg, s) {
			return true
		}
	}
	return false
}

func isMissing(err error) bool {
	var de *dbusError
	return errors.As(err, &de) && de.missing()
}

// dbusCall calls a method with args in GVariant text ("" for none) and
// returns the reply tuple (the caller unrefs it).
func dbusCall(conn ptr, dest, path, iface, method, args string, timeoutMS int32) (ptr, error) {
	params, err := gvParse(args)
	if err != nil {
		return 0, err
	}
	var gerr ptr
	reply := gDBusConnectionCallSync(conn, dest, path, iface, method, params, 0, 0, timeoutMS, 0, &gerr)
	if params != 0 {
		gVariantUnref(params)
	}
	if reply == 0 {
		return 0, &dbusError{iface + "." + method, gErrorMessage(gerr)}
	}
	return reply, nil
}

// dbusCallFD is dbusCall for methods returning a file descriptor (handle
// at index 0 of the reply); it returns the fd, owned by the caller.
func dbusCallFD(conn ptr, dest, path, iface, method, args string) (int, error) {
	params, err := gvParse(args)
	if err != nil {
		return -1, err
	}
	var gerr, fdList ptr
	reply := gd.callFDSync(conn, dest, path, iface, method, params, 0, 0, 5000, 0, &fdList, 0, &gerr)
	if params != 0 {
		gVariantUnref(params)
	}
	if reply == 0 {
		return -1, &dbusError{iface + "." + method, gErrorMessage(gerr)}
	}
	defer gVariantUnref(reply)
	if fdList == 0 {
		return -1, &dbusError{iface + "." + method, "no file descriptor in the reply"}
	}
	defer gObjectUnref(fdList)
	h := gvChild(reply, 0)
	idx := gd.vHandle(h)
	gVariantUnref(h)
	fd := gd.fdListGet(fdList, idx, &gerr) // dup'ed for us
	if fd < 0 {
		return -1, &dbusError{iface + "." + method, gErrorMessage(gerr)}
	}
	return int(fd), nil
}

// dbusGetAll reads all properties of iface (the a{sv} inside the reply;
// the caller unrefs it).
func dbusGetAll(conn ptr, dest, path, iface string) (ptr, error) {
	reply, err := dbusCall(conn, dest, path, "org.freedesktop.DBus.Properties", "GetAll", "("+gvString(iface)+",)", 3000)
	if err != nil {
		return 0, err
	}
	d := gvChild(reply, 0)
	gVariantUnref(reply)
	return d, nil
}

// dbusGet reads one property, unboxed (the caller unrefs it).
func dbusGet(conn ptr, dest, path, iface, prop string) (ptr, error) {
	reply, err := dbusCall(conn, dest, path, "org.freedesktop.DBus.Properties", "Get", "("+gvString(iface)+", "+gvString(prop)+")", 3000)
	if err != nil {
		return 0, err
	}
	v := gvChild(reply, 0)
	gVariantUnref(reply)
	return gvUnbox(v), nil
}

// nameHasOwner asks the bus whether name is owned.
func nameHasOwner(conn ptr, name string) bool {
	reply, err := dbusCall(conn, "org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "NameHasOwner", "("+gvString(name)+",)", 2000)
	if err != nil {
		return false
	}
	defer gVariantUnref(reply)
	v := gvChild(reply, 0)
	defer gVariantUnref(v)
	return gd.vBoolean(v)
}

// ---- variants ----

func gvParse(text string) (ptr, error) {
	if text == "" {
		return 0, nil
	}
	var gerr ptr
	v := gVariantParse(0, text, 0, 0, &gerr)
	if v == 0 {
		return 0, fmt.Errorf("tango/linux: bad GVariant %.80q: %s", text, gErrorMessage(gerr))
	}
	return v, nil
}

func gvChild(v ptr, i int) ptr { return gVariantGetChildValue(v, uintptr(i)) }

func gvType(v ptr) string { return goStr(gd.vTypeString(v)) }

// gvUnbox replaces a "v" (possibly nested) by its content, taking over v.
func gvUnbox(v ptr) ptr {
	for v != 0 && gvType(v) == "v" {
		in := gd.vVariant(v)
		gVariantUnref(v)
		v = in
	}
	return v
}

// gvLookup returns dict[key] (an a{sv}) or 0; the caller unrefs it.
func gvLookup(dict ptr, key string) ptr {
	if dict == 0 {
		return 0
	}
	return gvUnbox(gd.vLookup(dict, key, nil))
}

func gvStr(v ptr) string {
	if v == 0 {
		return ""
	}
	switch gvType(v) {
	case "s", "o", "g":
		return goStr(gVariantGetString(v, 0))
	}
	return ""
}

// The dict readers below read and unref one entry; ok is false when it is
// absent or of another type.

func dictBool(dict ptr, key string) (bool, bool) {
	v := gvLookup(dict, key)
	if v == 0 {
		return false, false
	}
	defer gVariantUnref(v)
	if gvType(v) != "b" {
		return false, false
	}
	return gd.vBoolean(v), true
}

func dictUint32(dict ptr, key string) (uint32, bool) {
	v := gvLookup(dict, key)
	if v == 0 {
		return 0, false
	}
	defer gVariantUnref(v)
	switch gvType(v) {
	case "u":
		return gVariantGetUint32(v), true
	case "i":
		return uint32(gd.vInt32(v)), true
	}
	return 0, false
}

func dictDouble(dict ptr, key string) (float64, bool) {
	v := gvLookup(dict, key)
	if v == 0 {
		return 0, false
	}
	defer gVariantUnref(v)
	if gvType(v) != "d" {
		return 0, false
	}
	return gd.vDouble(v), true
}

func dictString(dict ptr, key string) (string, bool) {
	v := gvLookup(dict, key)
	if v == 0 {
		return "", false
	}
	defer gVariantUnref(v)
	s := gvStr(v)
	return s, gvType(v) == "s" || gvType(v) == "o"
}

// ---- signals ----

// signalHandler receives a subscribed signal on the main loop; params is
// borrowed.
type signalHandler func(sender, path, iface, member string, params ptr)

var sigs struct {
	once sync.Once
	cb   ptr // GDBusSignalCallback(conn, sender, path, iface, signal, params, data)
	mu   sync.Mutex
	next uintptr
	m    map[uintptr]signalHandler
}

// subscribe listens for a signal. Empty strings match anything. Signals
// are delivered in the thread-default main context of the caller: call it
// on the main thread. The returned function unsubscribes.
func subscribe(conn ptr, sender, iface, member, path, arg0 string, h signalHandler) func() {
	sigs.once.Do(func() {
		sigs.m = map[uintptr]signalHandler{}
		sigs.cb = purego.NewCallback(func(_, sender, path, iface, member, params, data ptr) {
			sigs.mu.Lock()
			fn := sigs.m[data]
			sigs.mu.Unlock()
			if fn != nil {
				fn(goStr(sender), goStr(path), goStr(iface), goStr(member), params)
			}
		})
	})
	sigs.mu.Lock()
	sigs.next++
	key := sigs.next
	sigs.m[key] = h
	sigs.mu.Unlock()
	// cstr("") is NULL: match anything.
	id := gd.signalSubscribe(conn, cstr(sender), cstr(iface), cstr(member), cstr(path), cstr(arg0), 0, sigs.cb, key, 0)
	return func() {
		gd.signalUnsubscribe(conn, id)
		sigs.mu.Lock()
		delete(sigs.m, key)
		sigs.mu.Unlock()
	}
}

// ---- exported objects ----

// dbusObject is the Go side of an object exported with exportObject. Both
// methods run on the main loop.
type dbusObject interface {
	// call handles a method; it must return the invocation exactly once
	// (gd.invReturnValue / gd.invReturnError).
	call(method string, params, inv ptr)
	// property returns a property value (a new GVariant) or 0 if unknown.
	property(name string) ptr
}

var objs struct {
	once   sync.Once
	vtable [11]ptr // GDBusInterfaceVTable: method_call, get_property, set_property, padding[8]
	mu     sync.Mutex
	next   uintptr
	m      map[uintptr]dbusObject
	nodes  map[string]ptr // interface XML -> GDBusNodeInfo (kept forever)
}

func initObjects() {
	objs.once.Do(func() {
		objs.m = map[uintptr]dbusObject{}
		objs.nodes = map[string]ptr{}
		objs.vtable[0] = purego.NewCallback(func(_, _, _, _, method, params, inv, data ptr) {
			objs.mu.Lock()
			o := objs.m[data]
			objs.mu.Unlock()
			if o == nil {
				gd.invReturnError(inv, "org.freedesktop.DBus.Error.UnknownObject", "object removed")
				return
			}
			o.call(goStr(method), params, inv)
		})
		objs.vtable[1] = purego.NewCallback(func(_, _, _, _, name, gerr, data ptr) ptr {
			objs.mu.Lock()
			o := objs.m[data]
			objs.mu.Unlock()
			if o == nil {
				return 0
			}
			v := o.property(goStr(name))
			if v == 0 && gerr != 0 {
				*(*ptr)(toPtr(gerr)) = gErrorNewLiteral(gd.errorQuark(), 16, "unknown property "+goStr(name)) // G_DBUS_ERROR_INVALID_ARGS
			}
			return v // non-floating: GDBus drops our reference
		})
	})
}

// exportObject registers o at path with the interface described by xml
// (a <node> with one <interface name=iface>). It returns the unregister
// function.
func exportObject(conn ptr, path, iface, xml string, o dbusObject) (func(), error) {
	initObjects()
	node := objs.nodes[xml]
	if node == 0 {
		var gerr ptr
		node = gd.nodeInfoNew(xml, &gerr)
		if node == 0 {
			return nil, fmt.Errorf("tango/linux: interface XML: %s", gErrorMessage(gerr))
		}
		objs.nodes[xml] = node
	}
	info := gd.nodeLookupIface(node, iface)
	if info == 0 {
		return nil, fmt.Errorf("tango/linux: no interface %s in XML", iface)
	}
	objs.mu.Lock()
	objs.next++
	key := objs.next
	objs.m[key] = o
	objs.mu.Unlock()
	var gerr ptr
	id := gd.registerObject(conn, path, info, ptr(unsafe.Pointer(&objs.vtable[0])), key, 0, &gerr)
	if id == 0 {
		objs.mu.Lock()
		delete(objs.m, key)
		objs.mu.Unlock()
		return nil, fmt.Errorf("tango/linux: export %s: %s", path, gErrorMessage(gerr))
	}
	return func() {
		gd.unregisterObject(conn, id)
		objs.mu.Lock()
		delete(objs.m, key)
		objs.mu.Unlock()
	}, nil
}

// returnValue answers inv with the reply tuple in GVariant text ("" for
// none).
func returnValue(inv ptr, text string) {
	if text == "" {
		gd.invReturnValue(inv, 0)
		return
	}
	v, err := gvParse(text)
	if err != nil {
		gd.invReturnError(inv, "org.freedesktop.DBus.Error.Failed", err.Error())
		return
	}
	gd.invReturnValue(inv, v) // takes its own reference to a non-floating value
	gVariantUnref(v)
}

// propValue parses a property value for dbusObject.property.
func propValue(text string) ptr {
	v, err := gvParse(text)
	if err != nil {
		return 0
	}
	return v
}

// emit sends a signal from path with args in GVariant text.
func emit(conn ptr, path, iface, signal, args string) {
	v, err := gvParse(args)
	if err != nil {
		return
	}
	var gerr ptr
	if !gd.emitSignal(conn, nil, path, iface, signal, v, &gerr) {
		gErrorMessage(gerr)
	}
	if v != 0 {
		gVariantUnref(v)
	}
}
