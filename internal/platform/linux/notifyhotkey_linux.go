//go:build linux && !android

package linux

// Round-2 system integration: notifications (org.freedesktop.Notifications
// over GIO's D-Bus), global hotkeys (X11 XGrabKey; on Wayland the
// GlobalShortcuts portal, hotkey_portal_linux.go) and rich clipboard
// (GdkPixbuf images, URI lists). Libraries beyond the
// ones load() binds are opened lazily here, so apps that never use these
// features do not need them.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.Notifier      = (*backend)(nil)
	_ platform.Hotkeys       = (*backend)(nil)
	_ platform.ClipboardRich = (*backend)(nil)
)

var (
	nhOnce sync.Once
	nhErr  error

	// GIO D-Bus / GVariant.
	gBusGetSync                 func(busType int32, cancellable ptr, gerr *ptr) ptr
	gDBusConnectionCallSync     func(conn ptr, busName, path, iface, method string, params, replyType ptr, flags, timeout int32, cancellable ptr, gerr *ptr) ptr
	gDBusConnectionSignalSubscr func(conn ptr, sender, iface, member, path string, arg0 *byte, flags int32, cb, data, destroy ptr) uint32
	gVariantParse               func(typ ptr, text string, limit, endptr ptr, gerr *ptr) ptr
	gVariantGetChildValue       func(v ptr, i uintptr) ptr
	gVariantGetUint32           func(v ptr) uint32
	gVariantGetString           func(v ptr, length ptr) ptr
	gVariantUnref               func(v ptr)
	gStrfreev                   func(v ptr)
	gTypeCheckInstanceIsA       func(instance, typ ptr) bool
	gtkClipboardWaitForImage    func(cb ptr) ptr
	gtkClipboardSetImage        func(cb, pixbuf ptr)
	gtkClipboardWaitForURIs     func(cb ptr) ptr
	gtkClipboardSetWithData     func(cb ptr, targets unsafe.Pointer, n uint32, get, clear, data ptr) bool
	gtkSelectionDataSet         func(sd, typ ptr, format int32, data unsafe.Pointer, n int32)
	gtkSelectionDataSetText     func(sd ptr, text string, n int32) bool
	gtkSelectionDataTarget      func(sd ptr) ptr
	gdkPixbufSaveToBufferv      func(pb ptr, buf *ptr, size *uintptr, typ string, keys, values ptr, gerr *ptr) bool
	gdkPixbufLoaderNewWithType  func(typ string, gerr *ptr) ptr
	gdkPixbufLoaderWrite        func(l ptr, buf unsafe.Pointer, n uintptr, gerr *ptr) bool
	gdkPixbufLoaderClose        func(l ptr, gerr *ptr) bool
	gdkPixbufLoaderGetPixbuf    func(l ptr) ptr
	gdkDisplayGetDefault        func() ptr
	gdkGetDefaultRootWindow     func() ptr
	gdkWindowAddFilter          func(win, fn, data ptr)
	gdkX11DisplayGetType        func() ptr // optional (absent in Wayland-only GDK builds)
	gdkX11DisplayGetXDisplay    func(d ptr) ptr
	gdkX11GetDefaultRootXWindow func() uint64
	gdkX11DisplayErrorTrapPush  func(d ptr)
	gdkX11DisplayErrorTrapPop   func(d ptr) int32
	xKeysymToKeycode            func(dpy ptr, keysym uint64) uint8
	xGrabKey                    func(dpy ptr, keycode int32, mods uint32, win uint64, ownerEvents, pointerMode, keyboardMode int32) int32
	xUngrabKey                  func(dpy ptr, keycode int32, mods uint32, win uint64) int32
	libGDK, libPixbuf, libX11   ptr
	gdkX11Available, x11Libs    bool
)

func loadSystemLibs() error {
	nhOnce.Do(func() {
		var err error
		if libGDK, _, err = open("libgdk-3.so.0"); err != nil {
			nhErr = err
			return
		}
		if libPixbuf, _, err = open("libgdk_pixbuf-2.0.so.0"); err != nil {
			nhErr = err
			return
		}
		var bd binder
		io, g, o, t := libGIO, libGLib, libGObject, libGTK
		bd.req(&gBusGetSync, "g_bus_get_sync", io)
		bd.req(&gDBusConnectionCallSync, "g_dbus_connection_call_sync", io)
		bd.req(&gDBusConnectionSignalSubscr, "g_dbus_connection_signal_subscribe", io)
		bd.req(&gVariantParse, "g_variant_parse", g)
		bd.req(&gVariantGetChildValue, "g_variant_get_child_value", g)
		bd.req(&gVariantGetUint32, "g_variant_get_uint32", g)
		bd.req(&gVariantGetString, "g_variant_get_string", g)
		bd.req(&gVariantUnref, "g_variant_unref", g)
		bd.req(&gStrfreev, "g_strfreev", g)
		bd.req(&gTypeCheckInstanceIsA, "g_type_check_instance_is_a", o)
		bd.req(&gtkClipboardWaitForImage, "gtk_clipboard_wait_for_image", t)
		bd.req(&gtkClipboardSetImage, "gtk_clipboard_set_image", t)
		bd.req(&gtkClipboardWaitForURIs, "gtk_clipboard_wait_for_uris", t)
		bd.req(&gtkClipboardSetWithData, "gtk_clipboard_set_with_data", t)
		bd.req(&gtkSelectionDataSet, "gtk_selection_data_set", t)
		bd.req(&gtkSelectionDataSetText, "gtk_selection_data_set_text", t)
		bd.req(&gtkSelectionDataTarget, "gtk_selection_data_get_target", t)
		bd.req(&gdkPixbufSaveToBufferv, "gdk_pixbuf_save_to_bufferv", libPixbuf)
		bd.req(&gdkPixbufLoaderNewWithType, "gdk_pixbuf_loader_new_with_type", libPixbuf)
		bd.req(&gdkPixbufLoaderWrite, "gdk_pixbuf_loader_write", libPixbuf)
		bd.req(&gdkPixbufLoaderClose, "gdk_pixbuf_loader_close", libPixbuf)
		bd.req(&gdkPixbufLoaderGetPixbuf, "gdk_pixbuf_loader_get_pixbuf", libPixbuf)
		bd.req(&gdkDisplayGetDefault, "gdk_display_get_default", libGDK)
		bd.req(&gdkGetDefaultRootWindow, "gdk_get_default_root_window", libGDK)
		bd.req(&gdkWindowAddFilter, "gdk_window_add_filter", libGDK)
		gdkX11Available = bd.opt(&gdkX11DisplayGetType, "gdk_x11_display_get_type", libGDK) &&
			bd.opt(&gdkX11DisplayGetXDisplay, "gdk_x11_display_get_xdisplay", libGDK) &&
			bd.opt(&gdkX11GetDefaultRootXWindow, "gdk_x11_get_default_root_xwindow", libGDK) &&
			bd.opt(&gdkX11DisplayErrorTrapPush, "gdk_x11_display_error_trap_push", libGDK) &&
			bd.opt(&gdkX11DisplayErrorTrapPop, "gdk_x11_display_error_trap_pop", libGDK)
		if len(bd.missing) > 0 {
			nhErr = errors.New("tango/linux: missing native symbols: " + strings.Join(bd.missing, ", "))
		}
	})
	return nhErr
}

// ---------------------------------------------------------------------------
// Notifications

const (
	gBusTypeSession = 2
	notifyBus       = "org.freedesktop.Notifications"
	notifyPath      = "/org/freedesktop/Notifications"
)

var (
	notifyConn   ptr
	notifyClicks = map[uint32]func(){} // server id -> click, main thread
	notifySubOK  bool

	// GDBusSignalCallback(conn, sender, path, iface, signal, params, data).
	notifySignalCB     ptr
	notifySignalCBOnce sync.Once
)

func notifyConnection() (ptr, error) {
	if err := loadSystemLibs(); err != nil {
		return 0, err
	}
	if notifyConn != 0 {
		return notifyConn, nil
	}
	var gerr ptr
	c := gBusGetSync(gBusTypeSession, 0, &gerr)
	if c == 0 {
		return 0, fmt.Errorf("tango/linux: no D-Bus session bus: %s", gErrorMessage(gerr))
	}
	notifyConn = c
	notifySignalCBOnce.Do(func() {
		notifySignalCB = purego.NewCallback(func(conn, sender, path, iface, signal, params, data ptr) {
			onNotifySignal(goStr(signal), params)
		})
	})
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		gDBusConnectionSignalSubscr(c, notifyBus, notifyBus, member, notifyPath, nil, 0, notifySignalCB, 0, 0)
	}
	return c, nil
}

// onNotifySignal runs on the main loop (the subscription's context).
func onNotifySignal(signal string, params ptr) {
	idv := gVariantGetChildValue(params, 0)
	nid := gVariantGetUint32(idv)
	gVariantUnref(idv)
	fn, ok := notifyClicks[nid]
	if !ok {
		return
	}
	switch signal {
	case "ActionInvoked":
		delete(notifyClicks, nid)
		if fn != nil {
			fn()
		}
	case "NotificationClosed":
		delete(notifyClicks, nid)
	}
}

func (b *backend) RequestNotifyPermission(done func(granted bool, err error)) {
	// The freedesktop service has no permission model: notifications can
	// be shown whenever a server owns the name.
	_, err := notifyConnection()
	later(0, func() { done(err == nil, err) })
}

func (b *backend) Notify(n platform.Notification, onClick func()) error {
	conn, err := notifyConnection()
	if err != nil {
		return err
	}
	name := b.opts.Name
	if name == "" {
		if exe, err := os.Executable(); err == nil {
			name = filepath.Base(exe)
		}
	}
	text := notifyArgs(name, b.opts.ID, n.Title, n.Subtitle, n.Body, n.Silent)
	var gerr ptr
	params := gVariantParse(0, text, 0, 0, &gerr)
	if params == 0 {
		return fmt.Errorf("tango/linux: building notification: %s", gErrorMessage(gerr))
	}
	defer gVariantUnref(params)
	reply := gDBusConnectionCallSync(conn, notifyBus, notifyPath, notifyBus, "Notify", params, 0, 0, 5000, 0, &gerr)
	if reply == 0 {
		return fmt.Errorf("tango/linux: Notify: %s", gErrorMessage(gerr))
	}
	idv := gVariantGetChildValue(reply, 0)
	nid := gVariantGetUint32(idv)
	gVariantUnref(idv)
	gVariantUnref(reply)
	notifyClicks[nid] = onClick
	return nil
}

// ---------------------------------------------------------------------------
// Global hotkeys (X11)

var errWaylandHotkeys = fmt.Errorf("%w: global hotkeys need X11; Wayland compositors do not let apps grab keys "+
	"(use the desktop's own shortcut settings, or run with GDK_BACKEND=x11)", platform.ErrUnsupported)

type x11Grab struct {
	keycode int32
	mods    uint32
	fire    func()
}

var (
	x11Dpy        ptr
	x11Root       uint64
	x11FilterOn   bool
	x11Grabs      = map[uint32]x11Grab{} // core id -> grab, main thread
	x11LibOnce    sync.Once
	x11LibErr     error
	gdkDisplayX11 ptr
)

func x11Display() error {
	if err := loadSystemLibs(); err != nil {
		return err
	}
	d := gdkDisplayGetDefault()
	if !gdkX11Available || d == 0 || !gTypeCheckInstanceIsA(d, gdkX11DisplayGetType()) {
		return errWaylandHotkeys
	}
	x11LibOnce.Do(func() {
		lib, _, err := open("libX11.so.6")
		if err != nil {
			x11LibErr = err
			return
		}
		var bd binder
		bd.req(&xKeysymToKeycode, "XKeysymToKeycode", lib)
		bd.req(&xGrabKey, "XGrabKey", lib)
		bd.req(&xUngrabKey, "XUngrabKey", lib)
		if len(bd.missing) > 0 {
			x11LibErr = errors.New("tango/linux: missing X11 symbols: " + strings.Join(bd.missing, ", "))
		}
	})
	if x11LibErr != nil {
		return x11LibErr
	}
	gdkDisplayX11 = d
	x11Dpy = gdkX11DisplayGetXDisplay(d)
	x11Root = gdkX11GetDefaultRootXWindow()
	if !x11FilterOn {
		initCallbacks()
		// GdkFilterFunc(GdkXEvent *xevent, GdkEvent *event, gpointer data)
		id := dispatch.add(func(xev, _ ptr) ptr {
			onX11Event(xev)
			return 0 // GDK_FILTER_CONTINUE
		})
		gdkWindowAddFilter(gdkGetDefaultRootWindow(), cbArg3, id)
		x11FilterOn = true
	}
	return nil
}

// XKeyEvent (64-bit): type at 0, state at 80, keycode at 84.
func onX11Event(xev ptr) {
	if xev == 0 {
		return
	}
	p := toPtr(xev)
	if *(*int32)(p) != 2 { // KeyPress
		return
	}
	state := *(*uint32)(unsafe.Add(p, 80))
	keycode := int32(*(*uint32)(unsafe.Add(p, 84)))
	for _, g := range x11Grabs {
		if g.keycode == keycode && x11Matches(state, g.mods) && g.fire != nil {
			g.fire()
			return
		}
	}
}

func (b *backend) RegisterHotkey(id uint32, accelerator string, fire func()) error {
	acc, err := gtkAccelerator(accelerator)
	if err != nil {
		return err
	}
	if err := x11Display(); err != nil {
		if errors.Is(err, errWaylandHotkeys) {
			// Wayland: the GlobalShortcuts portal (hotkey_portal_linux.go).
			if perr := b.registerPortalHotkey(id, accelerator, fire); !errors.Is(perr, platform.ErrUnsupported) {
				return perr
			}
		}
		return err
	}
	var keyval, gmods uint32
	gtkAcceleratorParse(acc, &keyval, &gmods)
	if keyval == 0 {
		return fmt.Errorf("tango/linux: hotkey %q: unknown key", accelerator)
	}
	keycode := int32(xKeysymToKeycode(x11Dpy, uint64(keyval)))
	if keycode == 0 {
		return fmt.Errorf("tango/linux: hotkey %q: key not on the keyboard map", accelerator)
	}
	b.UnregisterHotkey(id)
	mods := x11GrabMods(gmods)
	gdkX11DisplayErrorTrapPush(gdkDisplayX11)
	for _, lock := range x11LockVariants {
		xGrabKey(x11Dpy, keycode, mods|lock, x11Root, 0, 1, 1) // GrabModeAsync
	}
	if code := gdkX11DisplayErrorTrapPop(gdkDisplayX11); code != 0 {
		gdkX11DisplayErrorTrapPush(gdkDisplayX11)
		for _, lock := range x11LockVariants {
			xUngrabKey(x11Dpy, keycode, mods|lock, x11Root)
		}
		gdkX11DisplayErrorTrapPop(gdkDisplayX11)
		return fmt.Errorf("tango/linux: cannot grab hotkey %q: taken by another client (X error %d)", accelerator, code)
	}
	x11Grabs[id] = x11Grab{keycode, mods, fire}
	return nil
}

func (b *backend) UnregisterHotkey(id uint32) {
	if b.unregisterPortalHotkey(id) {
		return
	}
	g, ok := x11Grabs[id]
	if !ok {
		return
	}
	delete(x11Grabs, id)
	// Another registration may share the combination; keep its grab.
	for _, o := range x11Grabs {
		if o.keycode == g.keycode && o.mods == g.mods {
			return
		}
	}
	gdkX11DisplayErrorTrapPush(gdkDisplayX11)
	for _, lock := range x11LockVariants {
		xUngrabKey(x11Dpy, g.keycode, g.mods|lock, x11Root)
	}
	gdkX11DisplayErrorTrapPop(gdkDisplayX11)
}

// ---------------------------------------------------------------------------
// Rich clipboard

// ClipboardReadImage and ClipboardReadFiles return (nil, nil) when the
// clipboard holds no image / no file list (the wait_for calls return NULL).
func (b *backend) ClipboardReadImage() ([]byte, error) {
	if err := loadSystemLibs(); err != nil {
		return nil, err
	}
	pb := gtkClipboardWaitForImage(gtkClipboardGet(gdkSelectionClipboard))
	if pb == 0 {
		return nil, nil
	}
	defer gObjectUnref(pb)
	var buf ptr
	var size uintptr
	var gerr ptr
	if !gdkPixbufSaveToBufferv(pb, &buf, &size, "png", 0, 0, &gerr) {
		return nil, fmt.Errorf("tango/linux: encoding clipboard image: %s", gErrorMessage(gerr))
	}
	out := append([]byte(nil), unsafe.Slice((*byte)(toPtr(buf)), size)...)
	gFree(buf)
	return out, nil
}

func (b *backend) ClipboardWriteImage(png []byte) error {
	if err := loadSystemLibs(); err != nil {
		return err
	}
	if len(png) == 0 {
		return errors.New("tango/linux: empty image")
	}
	var gerr ptr
	l := gdkPixbufLoaderNewWithType("png", &gerr)
	if l == 0 {
		return fmt.Errorf("tango/linux: no PNG loader: %s", gErrorMessage(gerr))
	}
	defer gObjectUnref(l)
	ok := gdkPixbufLoaderWrite(l, unsafe.Pointer(&png[0]), uintptr(len(png)), &gerr)
	if !ok {
		var e2 ptr
		gdkPixbufLoaderClose(l, &e2)
		if e2 != 0 {
			gErrorFree(e2)
		}
		return fmt.Errorf("tango/linux: decoding PNG: %s", gErrorMessage(gerr))
	}
	if !gdkPixbufLoaderClose(l, &gerr) {
		return fmt.Errorf("tango/linux: decoding PNG: %s", gErrorMessage(gerr))
	}
	pb := gdkPixbufLoaderGetPixbuf(l) // owned by the loader
	if pb == 0 {
		return errors.New("tango/linux: decoding PNG produced no image")
	}
	gtkClipboardSetImage(gtkClipboardGet(gdkSelectionClipboard), pb)
	return nil
}

func (b *backend) ClipboardReadFiles() ([]string, error) {
	if err := loadSystemLibs(); err != nil {
		return nil, err
	}
	v := gtkClipboardWaitForURIs(gtkClipboardGet(gdkSelectionClipboard))
	if v == 0 {
		return nil, nil
	}
	var uris []string
	for i := 0; ; i++ {
		s := *(*ptr)(unsafe.Add(toPtr(v), i*int(unsafe.Sizeof(ptr(0)))))
		if s == 0 {
			break
		}
		uris = append(uris, goStr(s))
	}
	gStrfreev(v)
	return uriPaths(uris), nil
}

// ClipboardWriteFiles owns the CLIPBOARD with a file list
// (fileListPayloads). The data is produced on request by clipGetCB; GTK
// calls clipClearCB when another owner (or our next write) takes over.
func (b *backend) ClipboardWriteFiles(paths []string) error {
	if err := loadSystemLibs(); err != nil {
		return err
	}
	d, err := fileListPayloads(paths)
	if err != nil {
		return err
	}
	clipCBOnce.Do(func() {
		clipGetCB = purego.NewCallback(func(cb, sd, info, data ptr) {
			clipServe(sd, uint32(info), data)
		})
		clipClearCB = purego.NewCallback(func(cb, data ptr) { delete(clipOffers, data) })
	})
	// struct GtkTargetEntry { gchar *target; guint flags; guint info; }
	type targetEntry struct {
		target *byte
		flags  uint32
		info   uint32
	}
	targets := []targetEntry{
		{cstr("x-special/gnome-copied-files"), 0, clipInfoGnome},
		{cstr("text/uri-list"), 0, clipInfoURIs},
		{cstr("UTF8_STRING"), 0, clipInfoText},
		{cstr("text/plain;charset=utf-8"), 0, clipInfoText},
		{cstr("text/plain"), 0, clipInfoText},
		{cstr("STRING"), 0, clipInfoText},
		{cstr("TEXT"), 0, clipInfoText},
	}
	clipSeq++
	id := clipSeq
	clipOffers[id] = d
	// GTK interns the target names; the table need not outlive the call.
	ok := gtkClipboardSetWithData(gtkClipboardGet(gdkSelectionClipboard), unsafe.Pointer(&targets[0]), uint32(len(targets)), clipGetCB, clipClearCB, id)
	runtime.KeepAlive(targets)
	if !ok {
		delete(clipOffers, id)
		return errors.New("tango/linux: could not take the clipboard")
	}
	return nil
}

const (
	clipInfoURIs = iota + 1
	clipInfoGnome
	clipInfoText
)

var (
	clipCBOnce  sync.Once
	clipGetCB   ptr // GtkClipboardGetFunc(clipboard, selection_data, info, data)
	clipClearCB ptr // GtkClipboardClearFunc(clipboard, data)
	clipSeq     ptr
	clipOffers  = map[ptr]fileListData{} // main thread
)

// clipServe answers one request for the offer with id data.
func clipServe(sd ptr, info uint32, data ptr) {
	d, ok := clipOffers[data]
	if !ok || sd == 0 {
		return
	}
	var v string
	switch info {
	case clipInfoText:
		gtkSelectionDataSetText(sd, d.text, int32(len(d.text)))
		return
	case clipInfoURIs:
		v = d.uriList
	case clipInfoGnome:
		v = d.gnome
	default:
		return
	}
	buf := []byte(v)
	gtkSelectionDataSet(sd, gtkSelectionDataTarget(sd), 8, unsafe.Pointer(&buf[0]), int32(len(buf)))
	runtime.KeepAlive(buf)
}

// ---------------------------------------------------------------------------
// File chooser flags (FileOptions.ShowHidden / CanCreateDirs)

var (
	chooserFlagsOnce        sync.Once
	gtkFileChooserSetHidden func(c ptr, show bool)
	gtkFileChooserSetCreate func(c ptr, create bool)
)

// setChooserFlags applies FileOptions.ShowHidden / CanCreateDirs. The
// xdg-desktop-portal dialog (Flatpak, Snap) may ignore them.
func setChooserFlags(fc ptr, showHidden, createDirs bool) {
	chooserFlagsOnce.Do(func() {
		var bd binder
		bd.opt(&gtkFileChooserSetHidden, "gtk_file_chooser_set_show_hidden", libGTK)
		bd.opt(&gtkFileChooserSetCreate, "gtk_file_chooser_set_create_folders", libGTK)
	})
	if gtkFileChooserSetHidden != nil {
		gtkFileChooserSetHidden(fc, showHidden)
	}
	if gtkFileChooserSetCreate != nil {
		gtkFileChooserSetCreate(fc, createDirs)
	}
}
