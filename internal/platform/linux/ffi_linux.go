//go:build linux && !android

// Run-time bindings to GLib, GTK 3 and WebKitGTK, without cgo.
//
// The shared libraries are dlopen'ed by purego when the backend starts.
// Each C entry point is a Go function variable; load walks the symbol
// tables further down and points every variable at its C symbol. The
// variables are typed by call shape (the "c…" types) rather than one
// literal func type per symbol, so a table row is just "C name -> variable".

package linux

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

type ptr = uintptr

// Library handles; 0 until load opens them. libJSC and libSoup may stay 0:
// their symbols are then looked up through the WebKit handle.
var libGLib, libGObject, libGIO, libGTK, libWebKit, libJSC, libSoup ptr

// Call shapes shared by many GLib/GTK/WebKit entry points (o is the
// receiver object, i.e. the first C argument).
type (
	cVoid    func()                // void f(void)
	cMake    func() ptr            // T *f(void)
	cOn      func(o ptr)           // void f(T *o)
	cGet     func(o ptr) ptr       // U *f(T *o)
	cPair    func(o, arg ptr)      // void f(T *o, U *arg)
	cSetStr  func(o ptr, s string) // void f(T *o, const gchar *s)
	cSetBool func(o ptr, b bool)   // void f(T *o, gboolean b)
	cSetWH   func(o ptr, a, b int32)
	cMakeStr func(s string) ptr // T *f(const gchar *s)
)

// ---- GLib, GObject, GIO ----

var (
	gFree, gSlistFree, gBytesUnref, gErrorFree, gObjectUnref cOn

	gSetPrgname, gSetApplicationName func(string)

	gMemoryInputStreamNewFromBytes cGet

	// g_free's address itself, for APIs that want a GDestroyNotify.
	gFreeSym ptr

	// guint g_idle_add_full(gint prio, GSourceFunc, gpointer, GDestroyNotify)
	gIdleAddFull func(int32, ptr, ptr, ptr) uint32
	// guint g_timeout_add_full(gint prio, guint ms, GSourceFunc, gpointer, GDestroyNotify)
	gTimeoutAddFull func(int32, uint32, ptr, ptr, ptr) uint32
	// gboolean g_main_context_iteration(GMainContext*, gboolean may_block)
	gMainContextIteration func(ptr, bool) bool
	// GBytes *g_bytes_new(gconstpointer, gsize)
	gBytesNew func(unsafe.Pointer, uintptr) ptr
	// gconstpointer g_bytes_get_data(GBytes*, gsize *size)
	gBytesGetData func(ptr, *uintptr) ptr
	// GError *g_error_new_literal(GQuark, gint, const gchar*)
	gErrorNewLiteral func(uint32, int32, string) ptr
	// GQuark g_quark_from_string(const gchar*)
	gQuarkFromString func(string) uint32
	// gulong g_signal_connect_data(gpointer, const gchar*, GCallback, gpointer, GClosureNotify, GConnectFlags)
	gSignalConnectData func(ptr, string, ptr, ptr, ptr, int32) uint
	// void g_signal_handler_disconnect(gpointer, gulong)
	gSignalHandlerDisconnect func(ptr, uint)
	// gboolean g_input_stream_read_all(GInputStream*, void*, gsize, gsize *read, GCancellable*, GError**)
	gInputStreamReadAll func(ptr, unsafe.Pointer, uintptr, *uintptr, ptr, *ptr) bool
)

// ---- GTK 3 ----

var (
	gtkMain, gtkMainQuit cVoid

	gtkAccelGroupNew, gtkMenuBarNew, gtkMenuNew, gtkSeparatorMenuItemNew,
	gtkAboutDialogNew, gtkFileFilterNew cMake

	gtkMenuItemNewWithMnemonic, gtkCheckMenuItemNewWithMnemonic cMakeStr

	gtkWindowPresent, gtkWindowIconify, gtkWindowClose,
	gtkWidgetShow, gtkWidgetShowAll, gtkWidgetHide, gtkWidgetDestroy,
	gtkNativeDialogShow cOn

	gtkBinGetChild, gtkClipboardGet, gtkClipboardWaitForText,
	gtkFileChooserGetFilenames cGet

	gtkWindowSetTransientFor, gtkWindowAddAccelGroup, gtkWindowRemoveAccelGroup,
	gtkContainerAdd, gtkMenuItemSetSubmenu, gtkMenuShellAppend,
	gtkFileChooserAddFilter cPair

	gtkWindowSetTitle, gtkAboutDialogSetProgramName, gtkFileChooserSetCurrentName,
	gtkFileFilterSetName, gtkFileFilterAddPattern cSetStr

	gtkWindowSetResizable, gtkWidgetSetSensitive, gtkCheckMenuItemSetActive,
	gtkNativeDialogSetModal, gtkFileChooserSetSelectMultiple,
	gtkFileChooserSetDoOverwriteConf cSetBool

	gtkWindowSetDefaultSize, gtkWindowResize, gtkWidgetSetSizeRequest cSetWH

	// gboolean gtk_init_check(int *argc, char ***argv)
	gtkInitCheck func(ptr, ptr) bool
	// guint gtk_main_level(void)
	gtkMainLevel func() uint32
	// GtkWidget *gtk_window_new(GtkWindowType)
	gtkWindowNew func(int32) ptr
	// void gtk_window_get_size(GtkWindow*, gint*, gint*)
	gtkWindowGetSize func(ptr, *int32, *int32)
	// void gtk_widget_add_accelerator(GtkWidget*, const gchar *signal, GtkAccelGroup*, guint key, GdkModifierType, GtkAccelFlags)
	gtkWidgetAddAccelerator func(ptr, string, ptr, uint32, uint32, uint32)
	// GtkWidget *gtk_box_new(GtkOrientation, gint spacing)
	gtkBoxNew func(int32, int32) ptr
	// void gtk_box_pack_start(GtkBox*, GtkWidget*, gboolean expand, gboolean fill, guint padding)
	gtkBoxPackStart func(ptr, ptr, bool, bool, uint32)
	// void gtk_box_reorder_child(GtkBox*, GtkWidget*, gint position)
	gtkBoxReorderChild func(ptr, ptr, int32)
	// void gtk_accelerator_parse(const gchar*, guint *key, GdkModifierType *mods)
	gtkAcceleratorParse func(string, *uint32, *uint32)
	// void gtk_accel_label_set_accel(GtkAccelLabel*, guint key, GdkModifierType)
	gtkAccelLabelSetAccel func(ptr, uint32, uint32)
	// void gtk_clipboard_set_text(GtkClipboard*, const gchar*, gint len)
	gtkClipboardSetText func(ptr, string, int32)
	// GtkWidget *gtk_message_dialog_new(GtkWindow*, GtkDialogFlags, GtkMessageType, GtkButtonsType, const gchar *fmt, ...)
	// (always called with fmt "%s" and one string argument)
	gtkMessageDialogNew func(ptr, int32, int32, int32, string, string) ptr
	// GtkWidget *gtk_dialog_add_button(GtkDialog*, const gchar*, gint response)
	gtkDialogAddButton func(ptr, string, int32) ptr
	// GtkFileChooserNative *gtk_file_chooser_native_new(const gchar *title, GtkWindow*, GtkFileChooserAction, const gchar *accept, const gchar *cancel)
	gtkFileChooserNativeNew func(*byte, ptr, int32, *byte, *byte) ptr
	// gboolean gtk_file_chooser_set_current_folder(GtkFileChooser*, const gchar*)
	gtkFileChooserSetCurrentFolder func(ptr, string) bool
)

// ---- WebKitGTK, JavaScriptCore, libsoup ----
//
// Variables noted "may be nil" are optional (newer WebKitGTK only); callers
// test them and fall back.

var (
	webkitWebContextGetDefault, webkitUserContentManagerNew cMake

	webkitUserScriptUnref, webkitWebInspectorShow cOn

	webkitUserContentManagerRemoveAllScripts cOn

	webkitWebContextGetSecurityManager, webkitWebViewNewWithUserContentManager,
	webkitWebViewGetSettings, webkitWebViewGetURI, webkitWebViewGetInspector,
	webkitJavascriptResultGetJSValue,
	webkitURISchemeRequestGetURI, webkitURISchemeRequestGetWebView,
	jscValueToString cGet

	webkitURISchemeRequestGetHTTPMethod  cGet // may be nil (< 2.36)
	webkitURISchemeRequestGetHTTPHeaders cGet // may be nil (< 2.36)
	webkitURISchemeRequestGetHTTPBody    cGet // may be nil (< 2.40)
	jscValueToStringAsBytes              cGet // may be nil

	webkitUserContentManagerAddScript, webkitURISchemeRequestFinishError,
	webkitURISchemeResponseSetHTTPHeaders cPair

	webkitURISchemeRequestFinishWithResponse cPair // may be nil (< 2.36)
	webkitUserContentManagerRemoveScript     cPair // may be nil (< 2.32)

	webkitSecurityManagerRegisterSecure, webkitSecurityManagerRegisterCORS,
	webkitUserContentManagerUnregisterHandler, webkitWebViewLoadURI,
	webkitWebViewExecuteEditingCommand, webkitURISchemeResponseSetContentType cSetStr

	webkitSettingsSetEnableDeveloperExtras cSetBool
	webkitSettingsSetEnablePageCache       cSetBool // may be nil

	// void webkit_web_context_register_uri_scheme(WebKitWebContext*, const gchar*, WebKitURISchemeRequestCallback, gpointer, GDestroyNotify)
	webkitWebContextRegisterURIScheme func(ptr, string, ptr, ptr, ptr)
	// gboolean webkit_user_content_manager_register_script_message_handler(WebKitUserContentManager*, const gchar*)
	webkitUserContentManagerRegisterHandler func(ptr, string) bool
	// WebKitUserScript *webkit_user_script_new(const gchar*, WebKitUserContentInjectedFrames, WebKitUserScriptInjectionTime, const gchar* const *allow, const gchar* const *block)
	webkitUserScriptNew func(string, int32, int32, ptr, ptr) ptr
	// 2.40+: webkit_web_view_evaluate_javascript(view, script, length, world, source_uri, cancellable, callback, data); may be nil
	webkitWebViewEvaluateJavascript func(ptr, string, int, *byte, *byte, ptr, ptr, ptr)
	// deprecated predecessor: webkit_web_view_run_javascript(view, script, cancellable, callback, data)
	webkitWebViewRunJavascript func(ptr, string, ptr, ptr, ptr)
	// void webkit_uri_scheme_request_finish(request, GInputStream*, gint64 length, const gchar *content_type)
	webkitURISchemeRequestFinish func(ptr, ptr, int64, *byte)
	// WebKitURISchemeResponse *webkit_uri_scheme_response_new(GInputStream*, gint64 length)
	webkitURISchemeResponseNew func(ptr, int64) ptr
	// void webkit_uri_scheme_response_set_status(response, guint code, const gchar *reason)
	webkitURISchemeResponseSetStatus func(ptr, uint32, *byte)

	// SoupMessageHeaders *soup_message_headers_new(SoupMessageHeadersType)
	soupMessageHeadersNew func(int32) ptr
	// void soup_message_headers_append(SoupMessageHeaders*, const char *name, const char *value)
	soupMessageHeadersAppend func(ptr, string, string)
	// void soup_message_headers_foreach(SoupMessageHeaders*, SoupMessageHeadersForeachFunc, gpointer)
	soupMessageHeadersForeach func(ptr, ptr, ptr)
)

// ---- symbol tables ----

// symbol pairs a C name with the Go variable (a pointer to it) that
// should call it.
type symbol struct {
	name string
	fn   any
}

// symbolSet is a list of symbols searched for in libs, in order.
type symbolSet struct {
	libs []ptr
	syms []symbol
}

// requiredSymbols lists every symbol load cannot do without. It is built
// at call time because it captures the library handles.
func requiredSymbols() []symbolSet {
	return []symbolSet{
		{[]ptr{libGLib}, []symbol{
			{"g_free", &gFree},
			{"g_idle_add_full", &gIdleAddFull},
			{"g_timeout_add_full", &gTimeoutAddFull},
			{"g_main_context_iteration", &gMainContextIteration},
			{"g_set_prgname", &gSetPrgname},
			{"g_set_application_name", &gSetApplicationName},
			{"g_slist_free", &gSlistFree},
			{"g_bytes_new", &gBytesNew},
			{"g_bytes_get_data", &gBytesGetData},
			{"g_bytes_unref", &gBytesUnref},
			{"g_error_new_literal", &gErrorNewLiteral},
			{"g_error_free", &gErrorFree},
			{"g_quark_from_string", &gQuarkFromString},
		}},
		{[]ptr{libGObject}, []symbol{
			{"g_signal_connect_data", &gSignalConnectData},
			{"g_signal_handler_disconnect", &gSignalHandlerDisconnect},
			{"g_object_unref", &gObjectUnref},
		}},
		{[]ptr{libGIO}, []symbol{
			{"g_memory_input_stream_new_from_bytes", &gMemoryInputStreamNewFromBytes},
			{"g_input_stream_read_all", &gInputStreamReadAll},
		}},
		{[]ptr{libGTK}, []symbol{
			{"gtk_init_check", &gtkInitCheck},
			{"gtk_main", &gtkMain},
			{"gtk_main_quit", &gtkMainQuit},
			{"gtk_main_level", &gtkMainLevel},

			{"gtk_window_new", &gtkWindowNew},
			{"gtk_window_set_title", &gtkWindowSetTitle},
			{"gtk_window_set_default_size", &gtkWindowSetDefaultSize},
			{"gtk_window_resize", &gtkWindowResize},
			{"gtk_window_get_size", &gtkWindowGetSize},
			{"gtk_window_set_resizable", &gtkWindowSetResizable},
			{"gtk_window_present", &gtkWindowPresent},
			{"gtk_window_iconify", &gtkWindowIconify},
			{"gtk_window_close", &gtkWindowClose},
			{"gtk_window_set_transient_for", &gtkWindowSetTransientFor},
			{"gtk_window_add_accel_group", &gtkWindowAddAccelGroup},
			{"gtk_window_remove_accel_group", &gtkWindowRemoveAccelGroup},

			{"gtk_widget_show", &gtkWidgetShow},
			{"gtk_widget_show_all", &gtkWidgetShowAll},
			{"gtk_widget_hide", &gtkWidgetHide},
			{"gtk_widget_destroy", &gtkWidgetDestroy},
			{"gtk_widget_set_sensitive", &gtkWidgetSetSensitive},
			{"gtk_widget_set_size_request", &gtkWidgetSetSizeRequest},
			{"gtk_widget_add_accelerator", &gtkWidgetAddAccelerator},
			{"gtk_container_add", &gtkContainerAdd},
			{"gtk_box_new", &gtkBoxNew},
			{"gtk_box_pack_start", &gtkBoxPackStart},
			{"gtk_box_reorder_child", &gtkBoxReorderChild},
			{"gtk_bin_get_child", &gtkBinGetChild},

			{"gtk_accel_group_new", &gtkAccelGroupNew},
			{"gtk_accelerator_parse", &gtkAcceleratorParse},
			{"gtk_accel_label_set_accel", &gtkAccelLabelSetAccel},
			{"gtk_menu_bar_new", &gtkMenuBarNew},
			{"gtk_menu_new", &gtkMenuNew},
			{"gtk_menu_item_new_with_mnemonic", &gtkMenuItemNewWithMnemonic},
			{"gtk_menu_item_set_submenu", &gtkMenuItemSetSubmenu},
			{"gtk_check_menu_item_new_with_mnemonic", &gtkCheckMenuItemNewWithMnemonic},
			{"gtk_check_menu_item_set_active", &gtkCheckMenuItemSetActive},
			{"gtk_separator_menu_item_new", &gtkSeparatorMenuItemNew},
			{"gtk_menu_shell_append", &gtkMenuShellAppend},

			{"gtk_clipboard_get", &gtkClipboardGet},
			{"gtk_clipboard_set_text", &gtkClipboardSetText},
			{"gtk_clipboard_wait_for_text", &gtkClipboardWaitForText},

			{"gtk_message_dialog_new", &gtkMessageDialogNew},
			{"gtk_dialog_add_button", &gtkDialogAddButton},
			{"gtk_about_dialog_new", &gtkAboutDialogNew},
			{"gtk_about_dialog_set_program_name", &gtkAboutDialogSetProgramName},
			{"gtk_file_chooser_native_new", &gtkFileChooserNativeNew},
			{"gtk_native_dialog_set_modal", &gtkNativeDialogSetModal},
			{"gtk_native_dialog_show", &gtkNativeDialogShow},
			{"gtk_file_chooser_set_select_multiple", &gtkFileChooserSetSelectMultiple},
			{"gtk_file_chooser_set_current_folder", &gtkFileChooserSetCurrentFolder},
			{"gtk_file_chooser_set_current_name", &gtkFileChooserSetCurrentName},
			{"gtk_file_chooser_set_do_overwrite_confirmation", &gtkFileChooserSetDoOverwriteConf},
			{"gtk_file_chooser_get_filenames", &gtkFileChooserGetFilenames},
			{"gtk_file_chooser_add_filter", &gtkFileChooserAddFilter},
			{"gtk_file_filter_new", &gtkFileFilterNew},
			{"gtk_file_filter_set_name", &gtkFileFilterSetName},
			{"gtk_file_filter_add_pattern", &gtkFileFilterAddPattern},
		}},
		{[]ptr{libWebKit}, []symbol{
			{"webkit_web_context_get_default", &webkitWebContextGetDefault},
			{"webkit_web_context_register_uri_scheme", &webkitWebContextRegisterURIScheme},
			{"webkit_web_context_get_security_manager", &webkitWebContextGetSecurityManager},
			{"webkit_security_manager_register_uri_scheme_as_secure", &webkitSecurityManagerRegisterSecure},
			{"webkit_security_manager_register_uri_scheme_as_cors_enabled", &webkitSecurityManagerRegisterCORS},

			{"webkit_user_content_manager_new", &webkitUserContentManagerNew},
			{"webkit_user_content_manager_register_script_message_handler", &webkitUserContentManagerRegisterHandler},
			{"webkit_user_content_manager_unregister_script_message_handler", &webkitUserContentManagerUnregisterHandler},
			{"webkit_user_content_manager_add_script", &webkitUserContentManagerAddScript},
			{"webkit_user_content_manager_remove_all_scripts", &webkitUserContentManagerRemoveAllScripts},
			{"webkit_user_script_new", &webkitUserScriptNew},
			{"webkit_user_script_unref", &webkitUserScriptUnref},

			{"webkit_web_view_new_with_user_content_manager", &webkitWebViewNewWithUserContentManager},
			{"webkit_web_view_get_settings", &webkitWebViewGetSettings},
			{"webkit_settings_set_enable_developer_extras", &webkitSettingsSetEnableDeveloperExtras},
			{"webkit_web_view_load_uri", &webkitWebViewLoadURI},
			{"webkit_web_view_get_uri", &webkitWebViewGetURI},
			{"webkit_web_view_get_inspector", &webkitWebViewGetInspector},
			{"webkit_web_inspector_show", &webkitWebInspectorShow},
			{"webkit_web_view_execute_editing_command", &webkitWebViewExecuteEditingCommand},
			{"webkit_javascript_result_get_js_value", &webkitJavascriptResultGetJSValue},

			{"webkit_uri_scheme_request_get_uri", &webkitURISchemeRequestGetURI},
			{"webkit_uri_scheme_request_get_web_view", &webkitURISchemeRequestGetWebView},
			{"webkit_uri_scheme_request_finish", &webkitURISchemeRequestFinish},
			{"webkit_uri_scheme_request_finish_error", &webkitURISchemeRequestFinishError},
		}},
		// JSC and soup: their own library if it opened, else whatever the
		// WebKit handle resolves through its dependencies.
		{[]ptr{libJSC, libWebKit}, []symbol{
			{"jsc_value_to_string", &jscValueToString},
		}},
		{[]ptr{libSoup, libWebKit}, []symbol{
			{"soup_message_headers_new", &soupMessageHeadersNew},
			{"soup_message_headers_append", &soupMessageHeadersAppend},
			{"soup_message_headers_foreach", &soupMessageHeadersForeach},
		}},
	}
}

// bindVersioned binds the WebKitGTK entry points whose presence depends on
// the installed release.
func bindVersioned(bd *binder) {
	wk := libWebKit

	// Running page script: evaluate_javascript (2.40) supersedes
	// run_javascript. Exactly one of them is needed.
	if !bd.opt(&webkitWebViewEvaluateJavascript, "webkit_web_view_evaluate_javascript", wk) {
		bd.req(&webkitWebViewRunJavascript, "webkit_web_view_run_javascript", wk)
	}

	// Request details for custom schemes; nil means "not available".
	for _, s := range []symbol{
		{"webkit_uri_scheme_request_get_http_method", &webkitURISchemeRequestGetHTTPMethod},
		{"webkit_uri_scheme_request_get_http_headers", &webkitURISchemeRequestGetHTTPHeaders},
		{"webkit_uri_scheme_request_get_http_body", &webkitURISchemeRequestGetHTTPBody},
	} {
		bd.opt(s.fn, s.name, wk)
	}

	// WebKitURISchemeResponse (status + headers) came with
	// finish_with_response; when that exists the rest of the response API
	// must too.
	if bd.opt(&webkitURISchemeRequestFinishWithResponse, "webkit_uri_scheme_request_finish_with_response", wk) {
		for _, s := range []symbol{
			{"webkit_uri_scheme_response_new", &webkitURISchemeResponseNew},
			{"webkit_uri_scheme_response_set_status", &webkitURISchemeResponseSetStatus},
			{"webkit_uri_scheme_response_set_content_type", &webkitURISchemeResponseSetContentType},
			{"webkit_uri_scheme_response_set_http_headers", &webkitURISchemeResponseSetHTTPHeaders},
		} {
			bd.req(s.fn, s.name, wk)
		}
	}

	bd.opt(&jscValueToStringAsBytes, "jsc_value_to_string_as_bytes", libJSC, wk)

	// Rotating the document-start script (msggate.go): remove_script
	// (2.32) removes just ours, remove_all_scripts is the fallback.
	bd.opt(&webkitUserContentManagerRemoveScript, "webkit_user_content_manager_remove_script", wk)
	bd.opt(&webkitSettingsSetEnablePageCache, "webkit_settings_set_enable_page_cache", wk)
}

// ---- loading ----

const dlFlags = purego.RTLD_NOW | purego.RTLD_GLOBAL

// open dlopens the first of names that loads and reports which one it was.
// The error lists why every candidate failed.
func open(names ...string) (ptr, string, error) {
	reasons := make([]string, 0, len(names))
	for _, soname := range names {
		h, err := purego.Dlopen(soname, dlFlags)
		if err != nil {
			reasons = append(reasons, err.Error())
			continue
		}
		return h, soname, nil
	}
	return 0, "", fmt.Errorf("tango/linux: cannot load %s: %s", names[0], strings.Join(reasons, "; "))
}

// lookup returns the address of name in the first non-zero handle that
// exports it, or 0.
func lookup(name string, libs []ptr) ptr {
	for _, h := range libs {
		if h == 0 {
			continue
		}
		if addr, err := purego.Dlsym(h, name); err == nil && addr != 0 {
			return addr
		}
	}
	return 0
}

// binder binds symbols and remembers the required ones it could not find,
// so a failed load names all of them at once.
type binder struct{ missing []string }

// opt binds fn (a pointer to a func variable) to name if any of libs has
// it and reports whether it did. A miss is not recorded.
func (bd *binder) opt(fn any, name string, libs ...ptr) bool {
	addr := lookup(name, libs)
	if addr == 0 {
		return false
	}
	purego.RegisterFunc(fn, addr)
	return true
}

// req is opt for a symbol that must exist; a miss is recorded.
func (bd *binder) req(fn any, name string, libs ...ptr) {
	if !bd.opt(fn, name, libs...) {
		bd.missing = append(bd.missing, name)
	}
}

// webkitBuild is one WebKitGTK API flavour and the JSC / libsoup it was
// built against. SoupMessageHeaders must come from the same libsoup major
// version WebKit uses, so the pair is chosen by which WebKit opened.
type webkitBuild struct{ webkit, jsc, soup string }

var webkitBuilds = []webkitBuild{
	{"libwebkit2gtk-4.1.so.0", "libjavascriptcoregtk-4.1.so.0", "libsoup-3.0.so.0"}, // preferred
	{"libwebkit2gtk-4.0.so.37", "libjavascriptcoregtk-4.0.so.18", "libsoup-2.4.so.1"},
}

// openLibraries opens the core libraries in dependency order, then the
// WebKit flavour and its companions (which may fail: see requiredSymbols).
func openLibraries() error {
	core := []struct {
		handle *ptr
		soname string
		hint   string
	}{
		{&libGLib, "libglib-2.0.so.0", ""},
		{&libGObject, "libgobject-2.0.so.0", ""},
		{&libGIO, "libgio-2.0.so.0", ""},
		{&libGTK, "libgtk-3.so.0", "GTK 3 is required: install libgtk-3-0 (Debian, Ubuntu) or gtk3 (Fedora)"},
	}
	for _, l := range core {
		h, _, err := open(l.soname)
		if err != nil {
			if l.hint != "" {
				return fmt.Errorf("%w (%s)", err, l.hint)
			}
			return err
		}
		*l.handle = h
	}

	sonames := make([]string, len(webkitBuilds))
	for i, b := range webkitBuilds {
		sonames[i] = b.webkit
	}
	h, got, err := open(sonames...)
	if err != nil {
		return fmt.Errorf("%w (WebKitGTK is required: install libwebkit2gtk-4.1-0 (Debian, Ubuntu) or webkit2gtk4.1 (Fedora))", err)
	}
	libWebKit = h
	for _, b := range webkitBuilds {
		if b.webkit == got {
			libJSC, _, _ = open(b.jsc)
			libSoup, _, _ = open(b.soup)
			break
		}
	}
	return nil
}

// load opens the libraries and binds every symbol of this file (plus the
// optional window extensions). It is called once from Init.
func load() error {
	if err := openLibraries(); err != nil {
		return err
	}
	var bd binder
	for _, set := range requiredSymbols() {
		for _, s := range set.syms {
			bd.req(s.fn, s.name, set.libs...)
		}
	}
	gFreeSym = lookup("g_free", []ptr{libGLib})
	bindVersioned(&bd)

	bindWindowExt() // best effort: a miss only turns the window extensions off

	if len(bd.missing) != 0 {
		return errors.New("tango/linux: missing native symbols: " + strings.Join(bd.missing, ", "))
	}
	return nil
}

// ---- C memory helpers ----

// toPtr turns an address handed out by C into an unsafe.Pointer. Going
// through memory keeps vet's unsafeptr check quiet; the target is never Go
// heap, so the GC does not care.
func toPtr(p ptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// cStrLen is strlen over C memory.
func cStrLen(at unsafe.Pointer) int {
	n := 0
	for *(*byte)(unsafe.Add(at, n)) != 0 {
		n++
	}
	return n
}

// goStr copies the NUL-terminated string at p (owned by C; not freed).
// NULL reads as "".
func goStr(p ptr) string {
	if p == 0 {
		return ""
	}
	at := toPtr(p)
	return string(unsafe.Slice((*byte)(at), cStrLen(at)))
}

// takeStr is goStr for a string the caller owns: it is g_free'd after the
// copy.
func takeStr(p ptr) string {
	if p == 0 {
		return ""
	}
	defer gFree(p)
	return goStr(p)
}

// cstr makes a NUL-terminated copy of s for a nullable const gchar*
// parameter: "" becomes NULL.
func cstr(s string) *byte {
	if len(s) == 0 {
		return nil
	}
	return &append([]byte(s), 0)[0]
}

// gErrorLayout mirrors GError { GQuark domain; gint code; gchar *message; }.
type gErrorLayout struct {
	domain  uint32
	code    int32
	message ptr
}

// gErrorMessage returns e's message and frees e. NULL gives "".
func gErrorMessage(e ptr) string {
	if e == 0 {
		return ""
	}
	defer gErrorFree(e)
	return goStr((*gErrorLayout)(toPtr(e)).message)
}

// gSListNode mirrors GSList { gpointer data; GSList *next; }.
type gSListNode struct{ data, next ptr }

// slistStrings collects a GSList whose elements are g_malloc'ed strings,
// freeing the strings and the list.
func slistStrings(head ptr) []string {
	if head == 0 {
		return nil
	}
	var out []string
	for at := head; at != 0; {
		node := (*gSListNode)(toPtr(at))
		out = append(out, takeStr(node.data))
		at = node.next
	}
	gSlistFree(head)
	return out
}
