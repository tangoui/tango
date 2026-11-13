//go:build linux && !android

package linux

// WebView extensions (contract round 4, internal/platform/webview_ext.go)
// for WebKitGTK 4.1 / 4.0: permission requests, navigation and new-window
// policy, HTTP authentication, downloads, cookies, website data, user
// agent, find in page, PDF and screenshot capture.
//
// Every symbol is bound by bindWebExt with a private binder; a missing
// symbol only disables the feature it belongs to (the methods then report
// platform.ErrUnsupported or the signal keeps WebKit's default).
//
// Not implemented: platform.WebHeaders. WebKitGTK's UI-process API cannot
// add headers to every request (only a web process extension can, through
// WebKitWebPage::send-request), so the core's fallback applies.

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.WebCreator   = (*backend)(nil)
	_ platform.WebFinder    = (*surface)(nil)
	_ platform.WebCookies   = (*surface)(nil)
	_ platform.WebData      = (*surface)(nil)
	_ platform.WebUserAgent = (*surface)(nil)
	_ platform.WebPDF       = (*surface)(nil)
	_ platform.WebSnapshot  = (*surface)(nil)
	_ platform.WebDownloads = (*surface)(nil)
)

// WebKit / GTK enum values used here.
const (
	wkPolicyNavigation = 0 // WebKitPolicyDecisionType
	wkPolicyNewWindow  = 1
	wkPolicyResponse   = 2

	wkPolicyErrorInterrupted = 102 // WEBKIT_POLICY_ERROR_FRAME_LOAD_INTERRUPTED_BY_POLICY_CHANGE
	wkCredentialForSession   = 1   // WebKitCredentialPersistence
	wkSnapshotVisible        = 0   // WebKitSnapshotRegion
	wkSnapshotOptionsNone    = 0

	gtkPageLandscape = 1 // GtkPageOrientation
	gtkUnitInch      = 2 // GtkUnit

	findTimeout = 3000 // ms without found-text / failed-to-find-text
)

// wv holds the native symbols of this file. The ...OK flags tell which
// features have every symbol they need.
var wv struct {
	once sync.Once
	ok   bool // core symbols (ref, type name, default context)

	objectRef        func(o ptr) ptr
	typeName         func(t uintptr) ptr
	listFree         func(l ptr)
	filenameToURI    func(name string, host, gerr ptr) ptr
	dgettext         func(domain, msgid string) ptr
	policyErrQuark   func() uint32
	dlErrQuark       func() uint32
	settingsSetUA    func(s ptr, ua *byte)
	webViewGetCtx    func(v ptr) ptr
	webCtxDefault    func() ptr
	uriRequestGetURI func(r ptr) ptr

	// permission-request
	permOK      bool
	permAllow   func(r ptr)
	permDeny    func(r ptr)
	umIsAudio   func(r ptr) bool
	umIsVideo   func(r ptr) bool
	umIsDisplay func(r ptr) bool // optional, 2.34

	// decide-policy / create
	policyOK          bool
	navDecisionAction func(d ptr) ptr
	navActionRequest  func(a ptr) ptr
	navActionGesture  func(a ptr) bool
	policyIgnore      func(d ptr)
	policyDownload    func(d ptr)
	respMIMEOK        func(d ptr) bool
	respGetResponse   func(d ptr) ptr
	respIsMainFrame   func(d ptr) bool // optional, 2.40
	uriRespHeaders    func(r ptr) ptr
	uriRespMIME       func(r ptr) ptr
	uriRespLength     func(r ptr) uint64
	soupHeadersGetOne func(h ptr, name string) ptr

	// authenticate
	authOK         bool
	authHost       func(r ptr) ptr
	authPort       func(r ptr) uint32
	authRealm      func(r ptr) ptr
	authScheme     func(r ptr) int32
	authProxy      func(r ptr) bool
	authRetry      func(r ptr) bool
	authAuth       func(r, cred ptr)
	authCancel     func(r ptr)
	credNew        func(user, password string, persistence int32) ptr
	credFree       func(c ptr)
	authOrigin     func(r ptr) ptr // optional, 2.30 (with the two below)
	originToString func(o ptr) ptr
	originUnref    func(o ptr)

	// downloads
	dlOK           bool
	dlWebView      func(d ptr) ptr
	dlRequest      func(d ptr) ptr
	dlResponse     func(d ptr) ptr
	dlSetDest      func(d ptr, uri string)
	dlSetOverwrite func(d ptr, on bool) // optional, 2.6
	dlCancel       func(d ptr)
	dlReceived     func(d ptr) uint64

	// cookies (libsoup 3) and website data
	cookiesOK     bool
	ctxCookieMgr  func(ctx ptr) ptr
	cmGet         func(cm ptr, uri string, cancellable, cb, data ptr)
	cmGetFinish   func(cm, res ptr, gerr *ptr) ptr
	cmGetAll      func(cm ptr, cancellable, cb, data ptr) // optional, 2.42
	cmGetAllFin   func(cm, res ptr, gerr *ptr) ptr
	cmAdd         func(cm, cookie, cancellable, cb, data ptr)
	cmAddFinish   func(cm, res ptr, gerr *ptr) bool
	cmDelete      func(cm, cookie, cancellable, cb, data ptr)
	cmDelFinish   func(cm, res ptr, gerr *ptr) bool
	soupNew       func(name, value, domain, path string, maxAge int32) ptr
	soupFree      func(c ptr)
	soupName      func(c ptr) ptr
	soupValue     func(c ptr) ptr
	soupDomain    func(c ptr) ptr
	soupPath      func(c ptr) ptr
	soupExpires   func(c ptr) ptr // GDateTime*, NULL for session cookies
	soupSecure    func(c ptr) bool
	soupHTTPOnly  func(c ptr) bool
	soupSameSite  func(c ptr) int32
	soupSetExp    func(c, dt ptr)
	soupSetSecure func(c ptr, on bool)
	soupSetHTTPO  func(c ptr, on bool)
	soupSetSame   func(c ptr, policy int32)
	dtNewUnix     func(t int64) ptr
	dtToUnix      func(dt ptr) int64
	dtUnref       func(dt ptr)

	dataOK      bool
	ctxDataMgr  func(ctx ptr) ptr
	dmClear     func(m ptr, types uint32, timespan int64, cancellable, cb, data ptr)
	dmClearDone func(m, res ptr, gerr *ptr) bool

	// find in page
	findOK       bool
	findCtl      func(v ptr) ptr
	findSearch   func(fc ptr, text string, opts, max uint32)
	findNext     func(fc ptr)
	findPrevious func(fc ptr)
	findFinish   func(fc ptr)

	// PDF
	pdfOK           bool
	printOpNew      func(v ptr) ptr
	printOpSettings func(op, settings ptr)
	printOpSetup    func(op, setup ptr)
	printOpPrint    func(op ptr)
	psNew           func() ptr
	psSet           func(s ptr, key, value string)
	psSetOrient     func(s ptr, o int32)
	psSetPaper      func(s, paper ptr)
	setupNew        func() ptr
	setupSetOrient  func(s ptr, o int32)
	setupSetPaper   func(s, paper ptr)
	paperNewCustom  func(name, display string, w, h float64, unit int32) ptr
	paperFree       func(p ptr)
	getPrintBG      func(s ptr) bool // optional
	setPrintBG      func(s ptr, on bool)

	// screenshot
	snapOK        bool
	snapshot      func(v ptr, region, options int32, cancellable, cb, data ptr)
	snapshotDone  func(v, res ptr, gerr *ptr) ptr
	cairoWritePNG func(surface ptr, filename string) int32
	cairoDestroy  func(surface ptr)
}

func bindWebExt() {
	var bd binder
	g, o, t, w := libGLib, libGObject, libGTK, libWebKit
	soup := []ptr{libSoup, w}

	wv.ok = bd.opt(&wv.objectRef, "g_object_ref", o) &&
		bd.opt(&wv.typeName, "g_type_name", o) &&
		bd.opt(&wv.listFree, "g_list_free", g) &&
		bd.opt(&wv.webCtxDefault, "webkit_web_context_get_default", w) &&
		bd.opt(&wv.webViewGetCtx, "webkit_web_view_get_context", w) &&
		bd.opt(&wv.uriRequestGetURI, "webkit_uri_request_get_uri", w)
	if !wv.ok {
		return
	}
	bd.opt(&wv.filenameToURI, "g_filename_to_uri", g)
	bd.opt(&wv.dgettext, "g_dgettext", g)
	bd.opt(&wv.policyErrQuark, "webkit_policy_error_quark", w)
	bd.opt(&wv.dlErrQuark, "webkit_download_error_quark", w)
	bd.opt(&wv.settingsSetUA, "webkit_settings_set_user_agent", w)

	wv.permOK = bd.opt(&wv.permAllow, "webkit_permission_request_allow", w) &&
		bd.opt(&wv.permDeny, "webkit_permission_request_deny", w) &&
		bd.opt(&wv.umIsAudio, "webkit_user_media_permission_is_for_audio_device", w) &&
		bd.opt(&wv.umIsVideo, "webkit_user_media_permission_is_for_video_device", w)
	bd.opt(&wv.umIsDisplay, "webkit_user_media_permission_is_for_display_device", w)

	wv.policyOK = bd.opt(&wv.navDecisionAction, "webkit_navigation_policy_decision_get_navigation_action", w) &&
		bd.opt(&wv.navActionRequest, "webkit_navigation_action_get_request", w) &&
		bd.opt(&wv.navActionGesture, "webkit_navigation_action_is_user_gesture", w) &&
		bd.opt(&wv.policyIgnore, "webkit_policy_decision_ignore", w) &&
		bd.opt(&wv.policyDownload, "webkit_policy_decision_download", w) &&
		bd.opt(&wv.respMIMEOK, "webkit_response_policy_decision_is_mime_type_supported", w) &&
		bd.opt(&wv.respGetResponse, "webkit_response_policy_decision_get_response", w) &&
		bd.opt(&wv.uriRespHeaders, "webkit_uri_response_get_http_headers", w) &&
		bd.opt(&wv.uriRespMIME, "webkit_uri_response_get_mime_type", w) &&
		bd.opt(&wv.uriRespLength, "webkit_uri_response_get_content_length", w) &&
		bd.opt(&wv.soupHeadersGetOne, "soup_message_headers_get_one", soup...)
	bd.opt(&wv.respIsMainFrame, "webkit_response_policy_decision_is_main_frame_main_resource", w)

	wv.authOK = bd.opt(&wv.authHost, "webkit_authentication_request_get_host", w) &&
		bd.opt(&wv.authPort, "webkit_authentication_request_get_port", w) &&
		bd.opt(&wv.authRealm, "webkit_authentication_request_get_realm", w) &&
		bd.opt(&wv.authScheme, "webkit_authentication_request_get_scheme", w) &&
		bd.opt(&wv.authProxy, "webkit_authentication_request_is_for_proxy", w) &&
		bd.opt(&wv.authRetry, "webkit_authentication_request_is_retry", w) &&
		bd.opt(&wv.authAuth, "webkit_authentication_request_authenticate", w) &&
		bd.opt(&wv.authCancel, "webkit_authentication_request_cancel", w) &&
		bd.opt(&wv.credNew, "webkit_credential_new", w) &&
		bd.opt(&wv.credFree, "webkit_credential_free", w)
	if !(bd.opt(&wv.authOrigin, "webkit_authentication_request_get_security_origin", w) &&
		bd.opt(&wv.originToString, "webkit_security_origin_to_string", w) &&
		bd.opt(&wv.originUnref, "webkit_security_origin_unref", w)) {
		wv.authOrigin = nil
	}

	wv.dlOK = bd.opt(&wv.dlWebView, "webkit_download_get_web_view", w) &&
		bd.opt(&wv.dlRequest, "webkit_download_get_request", w) &&
		bd.opt(&wv.dlResponse, "webkit_download_get_response", w) &&
		bd.opt(&wv.dlSetDest, "webkit_download_set_destination", w) &&
		bd.opt(&wv.dlCancel, "webkit_download_cancel", w) &&
		bd.opt(&wv.dlReceived, "webkit_download_get_received_data_length", w) &&
		wv.uriRespMIME != nil && wv.uriRespLength != nil
	bd.opt(&wv.dlSetOverwrite, "webkit_download_set_allow_overwrite", w)

	// SoupCookie accessors differ between libsoup 2.4 (WebKitGTK 4.0:
	// SoupDate, public struct fields) and 3; only libsoup 3 is used.
	var soupMajor func() uint32
	soup3 := bd.opt(&soupMajor, "soup_get_major_version", soup...) && soupMajor() == 3
	wv.cookiesOK = soup3 &&
		bd.opt(&wv.ctxCookieMgr, "webkit_web_context_get_cookie_manager", w) &&
		bd.opt(&wv.cmGet, "webkit_cookie_manager_get_cookies", w) &&
		bd.opt(&wv.cmGetFinish, "webkit_cookie_manager_get_cookies_finish", w) &&
		bd.opt(&wv.cmAdd, "webkit_cookie_manager_add_cookie", w) &&
		bd.opt(&wv.cmAddFinish, "webkit_cookie_manager_add_cookie_finish", w) &&
		bd.opt(&wv.cmDelete, "webkit_cookie_manager_delete_cookie", w) &&
		bd.opt(&wv.cmDelFinish, "webkit_cookie_manager_delete_cookie_finish", w) &&
		bd.opt(&wv.soupNew, "soup_cookie_new", soup...) &&
		bd.opt(&wv.soupFree, "soup_cookie_free", soup...) &&
		bd.opt(&wv.soupName, "soup_cookie_get_name", soup...) &&
		bd.opt(&wv.soupValue, "soup_cookie_get_value", soup...) &&
		bd.opt(&wv.soupDomain, "soup_cookie_get_domain", soup...) &&
		bd.opt(&wv.soupPath, "soup_cookie_get_path", soup...) &&
		bd.opt(&wv.soupExpires, "soup_cookie_get_expires", soup...) &&
		bd.opt(&wv.soupSecure, "soup_cookie_get_secure", soup...) &&
		bd.opt(&wv.soupHTTPOnly, "soup_cookie_get_http_only", soup...) &&
		bd.opt(&wv.soupSameSite, "soup_cookie_get_same_site_policy", soup...) &&
		bd.opt(&wv.soupSetExp, "soup_cookie_set_expires", soup...) &&
		bd.opt(&wv.soupSetSecure, "soup_cookie_set_secure", soup...) &&
		bd.opt(&wv.soupSetHTTPO, "soup_cookie_set_http_only", soup...) &&
		bd.opt(&wv.soupSetSame, "soup_cookie_set_same_site_policy", soup...) &&
		bd.opt(&wv.dtNewUnix, "g_date_time_new_from_unix_utc", g) &&
		bd.opt(&wv.dtToUnix, "g_date_time_to_unix", g) &&
		bd.opt(&wv.dtUnref, "g_date_time_unref", g)
	if wv.cookiesOK && !(bd.opt(&wv.cmGetAll, "webkit_cookie_manager_get_all_cookies", w) &&
		bd.opt(&wv.cmGetAllFin, "webkit_cookie_manager_get_all_cookies_finish", w)) {
		wv.cmGetAll = nil
	}

	wv.dataOK = bd.opt(&wv.ctxDataMgr, "webkit_web_context_get_website_data_manager", w) &&
		bd.opt(&wv.dmClear, "webkit_website_data_manager_clear", w) &&
		bd.opt(&wv.dmClearDone, "webkit_website_data_manager_clear_finish", w)

	wv.findOK = bd.opt(&wv.findCtl, "webkit_web_view_get_find_controller", w) &&
		bd.opt(&wv.findSearch, "webkit_find_controller_search", w) &&
		bd.opt(&wv.findNext, "webkit_find_controller_search_next", w) &&
		bd.opt(&wv.findPrevious, "webkit_find_controller_search_previous", w) &&
		bd.opt(&wv.findFinish, "webkit_find_controller_search_finish", w)

	wv.pdfOK = bd.opt(&wv.printOpNew, "webkit_print_operation_new", w) &&
		bd.opt(&wv.printOpSettings, "webkit_print_operation_set_print_settings", w) &&
		bd.opt(&wv.printOpSetup, "webkit_print_operation_set_page_setup", w) &&
		bd.opt(&wv.printOpPrint, "webkit_print_operation_print", w) &&
		bd.opt(&wv.psNew, "gtk_print_settings_new", t) &&
		bd.opt(&wv.psSet, "gtk_print_settings_set", t) &&
		bd.opt(&wv.psSetOrient, "gtk_print_settings_set_orientation", t) &&
		bd.opt(&wv.psSetPaper, "gtk_print_settings_set_paper_size", t) &&
		bd.opt(&wv.setupNew, "gtk_page_setup_new", t) &&
		bd.opt(&wv.setupSetOrient, "gtk_page_setup_set_orientation", t) &&
		bd.opt(&wv.setupSetPaper, "gtk_page_setup_set_paper_size", t) &&
		bd.opt(&wv.paperNewCustom, "gtk_paper_size_new_custom", t) &&
		bd.opt(&wv.paperFree, "gtk_paper_size_free", t)
	if !(bd.opt(&wv.getPrintBG, "webkit_settings_get_print_backgrounds", w) &&
		bd.opt(&wv.setPrintBG, "webkit_settings_set_print_backgrounds", w)) {
		wv.getPrintBG = nil
	}

	cairo, _, _ := open("libcairo.so.2")
	wv.snapOK = bd.opt(&wv.snapshot, "webkit_web_view_get_snapshot", w) &&
		bd.opt(&wv.snapshotDone, "webkit_web_view_get_snapshot_finish", w) &&
		bd.opt(&wv.cairoWritePNG, "cairo_surface_write_to_png", cairo, w) &&
		bd.opt(&wv.cairoDestroy, "cairo_surface_destroy", cairo, w)
}

// Trampoline for signals with two arguments after the instance
// ("decide-policy"). Same scheme as connectExt (ffi_ext_linux.go).
var (
	webCBOnce sync.Once
	cbExt3    ptr // (instance, a, b, data)
)

func connectExt2(instance ptr, signal string, h extHandler) uint {
	initExtCallbacks()
	webCBOnce.Do(func() {
		cbExt3 = purego.NewCallback(func(i, a, b, data ptr) ptr {
			return extDispatch.call(data, [7]uintptr{i, a, b})
		})
	})
	return gSignalConnectData(instance, signal, cbExt3, extDispatch.add(h), cbExtFree, 0)
}

// asyncData registers a one-shot GAsyncReadyCallback handler and returns
// the user_data to pass along with cbArg3 (source, res, data).
func asyncData(fn func(res ptr)) ptr {
	var id uintptr
	id = dispatch.add(func(_, res ptr) ptr {
		dispatch.remove(id)
		fn(res)
		return 0
	})
	return id
}

// gErrorText reads the message of a GError owned by someone else.
func gErrorText(e ptr) string {
	if e == 0 {
		return ""
	}
	return goStr(*(*ptr)(unsafe.Add(toPtr(e), 8)))
}

// gErrorCode reads domain and code of a GError.
func gErrorCode(e ptr) (uint32, int32) {
	return *(*uint32)(toPtr(e)), *(*int32)(unsafe.Add(toPtr(e), 4))
}

// typeNameOf is g_type_name(G_TYPE_FROM_INSTANCE(inst)): GTypeInstance
// starts with its GTypeClass*, whose first field is the GType.
func typeNameOf(inst ptr) string {
	cls := *(*ptr)(toPtr(inst))
	if cls == 0 {
		return ""
	}
	return goStr(wv.typeName(*(*uintptr)(toPtr(cls))))
}

func toFileURI(path string) string {
	if wv.filenameToURI != nil {
		if p := wv.filenameToURI(path, 0, 0); p != 0 {
			return takeStr(p)
		}
	}
	return fileURI(path)
}

// webExt is the per-surface state of the WebView extensions (surface.wext).
type webExt struct {
	fc         ptr // WebKitFindController, owned by the web view
	findActive bool
	findCount  int
	findSeq    int
	findDone   func(platform.FindResult, error)
	findNew    bool // the pending request is a new search (not FindNext)
}

// pendingWeb carries the WebOptions of NewSurfaceWeb into initWebExt
// (main thread only).
var pendingWeb *platform.WebOptions

func (b *backend) NewSurfaceWeb(id platform.SurfaceID, o platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	pendingWeb = &web
	defer func() { pendingWeb = nil }()
	return b.NewSurfaceEx(id, o, ex)
}

func (s *surface) webHost() platform.WebHost {
	wh, _ := s.b.host.(platform.WebHost)
	return wh
}

// initWebExt runs from newSurface right after the web view is created and
// before its first load. Its "load-failed" handler is connected before
// initExt's, so it can hide policy interruptions (downloads) from it.
func (s *surface) initWebExt() {
	wv.once.Do(bindWebExt)
	s.wext = &webExt{}
	if !wv.ok || s.web == 0 {
		return
	}
	if pendingWeb != nil && pendingWeb.UserAgent != "" {
		s.SetUserAgent(pendingWeb.UserAgent)
	}
	if wv.permOK {
		connect(s.web, "permission-request", 1, s.onPermissionRequest)
	}
	if wv.policyOK {
		connectExt2(s.web, "decide-policy", s.onDecidePolicy)
		connect(s.web, "create", 1, s.onCreate)
	}
	if wv.authOK {
		connect(s.web, "authenticate", 1, s.onAuthenticate)
	}
	if wv.policyErrQuark != nil {
		connectExt(s.web, "load-failed", 3, s.onLoadInterrupted)
	}
	if wv.findOK {
		if fc := wv.findCtl(s.web); fc != 0 {
			s.wext.fc = fc
			connect(fc, "found-text", 1, s.onFoundText)
			connect(fc, "failed-to-find-text", 0, s.onFailedToFind)
		}
	}
	if wv.dlOK {
		hookDownloads(s.b, wv.webViewGetCtx(s.web))
	}
}

// onLoadInterrupted swallows the load failure WebKit reports when a
// response became a download (or was ignored): TRUE stops initExt's
// handler (no SurfaceLoadFailed) and WebKit's error page.
func (s *surface) onLoadInterrupted(a [7]uintptr) uintptr {
	if gerr := a[3]; gerr != 0 {
		if d, c := gErrorCode(gerr); d == wv.policyErrQuark() && c == wkPolicyErrorInterrupted {
			return 1
		}
	}
	return 0
}

// Permissions -----------------------------------------------------------------

// onPermissionRequest: permission-request(web_view, request). Without a
// WebHost WebKit's default (deny) applies. The request is referenced
// until the decision, which may come later.
func (s *surface) onPermissionRequest(_, req ptr) ptr {
	wh := s.webHost()
	if s.closed || wh == nil || req == 0 {
		return 0
	}
	// The requesting document is the committed one: webkit_web_view_get_uri
	// already names the destination once an app-initiated load starts, so
	// it would credit the old (possibly untrusted) page with the new
	// origin. While a navigation is pending no document can be attributed:
	// deny (msggate.go).
	origin, ok := s.gate.documentOrigin()
	if !ok {
		wv.permDeny(req)
		return 1
	}
	name := typeNameOf(req)
	var audio, video, display bool
	if name == "WebKitUserMediaPermissionRequest" {
		audio, video = wv.umIsAudio(req), wv.umIsVideo(req)
		if wv.umIsDisplay != nil {
			display = wv.umIsDisplay(req)
		}
	}
	pr := platform.PermissionRequest{
		Origin: origin,
		Kinds:  permissionKinds(name, audio, video, display),
	}
	wv.objectRef(req)
	answered := false
	finish := func(allow bool) {
		if allow {
			wv.permAllow(req)
		} else {
			wv.permDeny(req)
		}
		gObjectUnref(req)
	}
	wh.OnPermissionRequest(s.id, pr, func(d platform.PermissionDecision) {
		if answered {
			return
		}
		answered = true
		switch d {
		case platform.PermissionAllow:
			finish(true)
		case platform.PermissionDeny:
			finish(false)
		default: // Prompt / Default: WebKitGTK has no prompt of its own
			s.promptPermission(pr, finish)
		}
	})
	return 1
}

// promptPermission asks the user with a GtkMessageDialog. Closing the
// dialog (or its window) denies.
func (s *surface) promptPermission(pr platform.PermissionRequest, finish func(bool)) {
	var parent ptr
	if !s.closed {
		parent = s.win
	}
	d := gtkMessageDialogNew(parent, gtkDialogModal|gtkDialogDestroyWithParent, gtkMessageQuestion,
		gtkButtonsNone, "%s", permissionPrompt(pr.Origin, pr.Kinds))
	if d == 0 {
		finish(false)
		return
	}
	gtkWindowSetTitle(d, "Permission request")
	gtkDialogAddButton(d, "_Deny", 0)
	gtkDialogAddButton(d, "_Allow", 1)
	fired := false
	connect(d, "response", 1, func(_, resp ptr) ptr {
		if !fired {
			fired = true
			gtkWidgetDestroy(d)
			finish(int32(resp) == 1)
		}
		return 0
	})
	connect(d, "destroy", 0, func(_, _ ptr) ptr {
		if !fired {
			fired = true
			finish(false)
		}
		return 0
	})
	gtkWindowPresent(d)
}

// Navigation policy -------------------------------------------------------------

// onDecidePolicy: decide-policy(web_view, decision, type). FALSE leaves
// the decision to WebKit's default handler (use / download unsupported
// MIME types).
func (s *surface) onDecidePolicy(a [7]uintptr) uintptr {
	dec, typ := a[1], int32(a[2])
	if s.closed || dec == 0 {
		return 0
	}
	switch typ {
	case wkPolicyNavigation:
		wh := s.webHost()
		if wh == nil {
			return 0
		}
		act := wv.navDecisionAction(dec)
		if act == 0 {
			return 0
		}
		nav := platform.NavigationRequest{
			URL:           goStr(wv.uriRequestGetURI(wv.navActionRequest(act))),
			UserInitiated: wv.navActionGesture(act),
		}
		if !wh.ShouldNavigate(s.id, nav) {
			wv.policyIgnore(dec)
			return 1
		}
	case wkPolicyNewWindow:
		// Always suppress the popup; the core decides about the URL.
		if act := wv.navDecisionAction(dec); act != 0 {
			s.reportNewWindow(act)
		}
		wv.policyIgnore(dec)
		return 1
	case wkPolicyResponse:
		resp := wv.respGetResponse(dec)
		disp := ""
		if resp != 0 {
			if h := wv.uriRespHeaders(resp); h != 0 {
				disp = goStr(wv.soupHeadersGetOne(h, "Content-Disposition"))
			}
		}
		main := wv.respIsMainFrame == nil || wv.respIsMainFrame(dec)
		if shouldDownload(wv.respMIMEOK(dec), disp, main) {
			wv.policyDownload(dec)
			return 1
		}
	}
	return 0
}

func (s *surface) reportNewWindow(act ptr) {
	wh := s.webHost()
	if wh == nil {
		return
	}
	u := ""
	if req := wv.navActionRequest(act); req != 0 {
		u = goStr(wv.uriRequestGetURI(req))
	}
	wh.OnNewWindow(s.id, platform.NewWindowRequest{URL: u, UserInitiated: wv.navActionGesture(act)})
}

// onCreate: create(web_view, navigation_action) for window.open (target
// links come through decide-policy and are ignored there). Returning NULL
// makes window.open return null.
func (s *surface) onCreate(_, act ptr) ptr {
	if !s.closed && act != 0 {
		s.reportNewWindow(act)
	}
	return 0
}

// HTTP authentication -------------------------------------------------------------

// onAuthenticate: authenticate(web_view, request). FALSE runs WebKit's
// own credential dialog (the platform default); TLS client certificate
// and server trust requests always do.
func (s *surface) onAuthenticate(_, req ptr) ptr {
	wh := s.webHost()
	if s.closed || wh == nil || req == 0 {
		return 0
	}
	scheme, ok := authSchemeName(wv.authScheme(req))
	if !ok {
		return 0
	}
	ch := platform.AuthChallenge{
		Host:   goStr(wv.authHost(req)),
		Port:   int(wv.authPort(req)),
		Realm:  goStr(wv.authRealm(req)),
		Scheme: scheme,
		Proxy:  wv.authProxy(req),
	}
	if wv.authRetry(req) {
		ch.Retry = 1
	}
	if wv.authOrigin != nil {
		if so := wv.authOrigin(req); so != 0 {
			ch.URL = takeStr(wv.originToString(so))
			wv.originUnref(so)
		}
	}
	wv.objectRef(req)
	cancelled := false
	connect(req, "cancelled", 0, func(_, _ ptr) ptr { cancelled = true; return 0 })
	inCall, answered := true, false
	var syncResp *platform.AuthResponse
	wh.OnAuthChallenge(s.id, ch, func(r platform.AuthResponse) {
		if answered {
			return
		}
		answered = true
		if inCall {
			syncResp = &r
			return
		}
		if !cancelled {
			applyAuth(req, r)
		}
		gObjectUnref(req)
	})
	inCall = false
	if syncResp == nil {
		return 1 // answered later
	}
	defer gObjectUnref(req)
	if syncResp.Action == platform.AuthDefault {
		return 0
	}
	applyAuth(req, *syncResp)
	return 1
}

// applyAuth answers a request we claimed. AuthDefault after the signal
// returned continues without credentials (the server's 401 page).
func applyAuth(req ptr, r platform.AuthResponse) {
	switch r.Action {
	case platform.AuthCredentials:
		cred := wv.credNew(r.User, r.Password, wkCredentialForSession)
		wv.authAuth(req, cred)
		wv.credFree(cred)
	case platform.AuthCancel:
		wv.authCancel(req)
	default:
		wv.authAuth(req, 0)
	}
}

// Downloads ------------------------------------------------------------------------

type download struct {
	obj      ptr // WebKitDownload, referenced until "finished"
	id       uint64
	sid      platform.SurfaceID
	host     platform.WebHost
	accepted bool
	ended    bool
	path     string
	last     time.Time
}

var dls struct {
	hooked map[ptr]bool // web contexts with a download-started handler
	next   uint64
	byID   map[uint64]*download
}

func init() {
	dls.hooked = map[ptr]bool{}
	dls.byID = map[uint64]*download{}
}

func (dl *download) reportEnd(st platform.DownloadState, msg string) {
	if dl.ended {
		return
	}
	dl.ended = true
	if !dl.accepted {
		return
	}
	ev := platform.DownloadEvent{ID: dl.id, State: st, Path: dl.path, Error: msg, Received: int64(wv.dlReceived(dl.obj)), Total: dl.total()}
	dl.host.OnDownloadEvent(dl.sid, ev)
}

func (dl *download) total() int64 {
	if r := wv.dlResponse(dl.obj); r != 0 {
		return downloadSize(wv.uriRespLength(r))
	}
	return -1
}

// hookDownloads connects download-started on a web context once.
func hookDownloads(b *backend, ctx ptr) {
	if ctx == 0 || dls.hooked[ctx] {
		return
	}
	dls.hooked[ctx] = true
	connect(ctx, "download-started", 1, func(_, obj ptr) ptr {
		b.onDownloadStarted(obj)
		return 0
	})
}

// onDownloadStarted: WebKitWebContext::download-started(context,
// download). Downloads of surfaces without a WebHost (and without a web
// view) keep WebKit's default: saved to the XDG download directory.
func (b *backend) onDownloadStarted(obj ptr) {
	if obj == 0 {
		return
	}
	s := b.byWeb[wv.dlWebView(obj)]
	if s == nil || s.closed {
		return
	}
	wh := s.webHost()
	if wh == nil {
		return
	}
	dls.next++
	dl := &download{obj: obj, id: dls.next, sid: s.id, host: wh}
	wv.objectRef(obj)
	dls.byID[dl.id] = dl
	connect(obj, "decide-destination", 1, dl.onDecideDestination)
	connect(obj, "received-data", 1, dl.onReceivedData)
	connect(obj, "failed", 1, dl.onFailed)
	connect(obj, "finished", 0, dl.onFinished)
}

// onDecideDestination: decide-destination(download, suggested_filename).
// TRUE: we set (or will set) the destination. A later answer needs a
// WebKitGTK that accepts webkit_download_set_destination after the signal.
func (dl *download) onDecideDestination(_, suggested ptr) ptr {
	if dl.ended {
		return 0
	}
	req := platform.DownloadRequest{ID: dl.id, Filename: goStr(suggested), Size: -1}
	if r := wv.dlRequest(dl.obj); r != 0 {
		req.URL = goStr(wv.uriRequestGetURI(r))
	}
	if r := wv.dlResponse(dl.obj); r != 0 {
		req.MIME = goStr(wv.uriRespMIME(r))
		req.Size = downloadSize(wv.uriRespLength(r))
	}
	answered := false
	dl.host.OnDownloadStarted(dl.sid, req, func(path string) {
		if answered || dl.ended {
			answered = true
			return
		}
		answered = true
		if path == "" {
			wv.dlCancel(dl.obj) // not accepted: no events
			return
		}
		dl.accepted, dl.path = true, path
		if wv.dlSetOverwrite != nil {
			wv.dlSetOverwrite(dl.obj, true)
		}
		wv.dlSetDest(dl.obj, toFileURI(path))
	})
	return 1
}

func (dl *download) onReceivedData(_, _ ptr) ptr {
	if !dl.accepted || dl.ended {
		return 0
	}
	now := time.Now()
	if now.Sub(dl.last) < progressInterval {
		return 0
	}
	dl.last = now
	dl.host.OnDownloadEvent(dl.sid, platform.DownloadEvent{
		ID: dl.id, State: platform.DownloadProgress, Path: dl.path,
		Received: int64(wv.dlReceived(dl.obj)), Total: dl.total(),
	})
	return 0
}

// onFailed: failed(download, GError*); "finished" follows.
func (dl *download) onFailed(_, gerr ptr) ptr {
	st, msg := platform.DownloadFailed, "download failed"
	if gerr != 0 {
		d, c := gErrorCode(gerr)
		st = downloadFailState(c, wv.dlErrQuark != nil && d == wv.dlErrQuark())
		msg = gErrorText(gerr)
	}
	if st == platform.DownloadCanceled {
		msg = ""
	}
	dl.reportEnd(st, msg)
	return 0
}

func (dl *download) onFinished(_, _ ptr) ptr {
	dl.reportEnd(platform.DownloadDone, "")
	delete(dls.byID, dl.id)
	obj := dl.obj
	later(0, func() { gObjectUnref(obj) }) // not from inside the emission
	return 0
}

// CancelDownload cancels an accepted download; "failed" then reports
// DownloadCanceled.
func (s *surface) CancelDownload(id uint64) {
	if dl := dls.byID[id]; dl != nil && dl.accepted && !dl.ended {
		wv.dlCancel(dl.obj)
	}
}

// Cookies --------------------------------------------------------------------------

func cookieManager() ptr {
	if !wv.ok || !wv.cookiesOK {
		return 0
	}
	return wv.ctxCookieMgr(wv.webCtxDefault())
}

// readSoupCookie copies a SoupCookie.
func readSoupCookie(c ptr) cookieFields {
	f := cookieFields{
		Name: goStr(wv.soupName(c)), Value: goStr(wv.soupValue(c)),
		Domain: goStr(wv.soupDomain(c)), Path: goStr(wv.soupPath(c)),
		Secure: wv.soupSecure(c), HTTPOnly: wv.soupHTTPOnly(c),
		SameSite: int(wv.soupSameSite(c)), Session: true,
	}
	if dt := wv.soupExpires(c); dt != 0 { // owned by the cookie
		f.Session, f.Expires = false, wv.dtToUnix(dt)
	}
	return f
}

func newSoupCookie(f cookieFields) ptr {
	c := wv.soupNew(f.Name, f.Value, f.Domain, f.Path, -1)
	if c == 0 {
		return 0
	}
	if !f.Session {
		dt := wv.dtNewUnix(f.Expires)
		wv.soupSetExp(c, dt)
		wv.dtUnref(dt)
	}
	wv.soupSetSecure(c, f.Secure)
	wv.soupSetHTTPO(c, f.HTTPOnly)
	if f.SameSite >= 0 {
		wv.soupSetSame(c, int32(f.SameSite))
	}
	return c
}

// cookieList is a GList of SoupCookie* returned by a finish function.
type cookieList struct {
	head ptr
	ptrs []ptr
}

func takeCookieList(l ptr) *cookieList {
	cl := &cookieList{head: l}
	// struct GList { gpointer data; GList *next; GList *prev; }
	for n := l; n != 0; n = *(*ptr)(unsafe.Add(toPtr(n), unsafe.Sizeof(ptr(0)))) {
		if c := *(*ptr)(toPtr(n)); c != 0 {
			cl.ptrs = append(cl.ptrs, c)
		}
	}
	return cl
}

func (cl *cookieList) free() {
	for _, c := range cl.ptrs {
		wv.soupFree(c)
	}
	if cl.head != 0 {
		wv.listFree(cl.head)
	}
	cl.ptrs, cl.head = nil, 0
}

// fetchCookies lists cookies for uri, or all cookies for "" (2.42+).
func fetchCookies(uri string, done func(*cookieList, error)) {
	cm := cookieManager()
	if cm == 0 || (uri == "" && wv.cmGetAll == nil) {
		done(nil, platform.ErrUnsupported)
		return
	}
	finish := wv.cmGetFinish
	if uri == "" {
		finish = wv.cmGetAllFin
	}
	data := asyncData(func(res ptr) {
		var gerr ptr
		l := finish(cm, res, &gerr)
		if gerr != 0 {
			done(nil, errors.New("tango/linux: "+gErrorMessage(gerr)))
			return
		}
		done(takeCookieList(l), nil)
	})
	if uri == "" {
		wv.cmGetAll(cm, 0, cbArg3, data)
	} else {
		wv.cmGet(cm, uri, 0, cbArg3, data)
	}
}

func (s *surface) Cookies(uri string, done func([]*http.Cookie, error)) {
	fetchCookies(uri, func(cl *cookieList, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		out := make([]*http.Cookie, 0, len(cl.ptrs))
		for _, c := range cl.ptrs {
			out = append(out, readSoupCookie(c).httpCookie())
		}
		cl.free()
		done(out, nil)
	})
}

func (s *surface) SetCookie(c *http.Cookie, done func(error)) {
	cm := cookieManager()
	if cm == 0 {
		done(platform.ErrUnsupported)
		return
	}
	if c == nil || c.Domain == "" || c.Name == "" {
		done(errors.New("tango/linux: SetCookie needs Name and Domain"))
		return
	}
	sc := newSoupCookie(cookieToFields(c, time.Now()))
	if sc == 0 {
		done(errors.New("tango/linux: soup_cookie_new failed"))
		return
	}
	wv.cmAdd(cm, sc, 0, cbArg3, asyncData(func(res ptr) {
		var gerr ptr
		ok := wv.cmAddFinish(cm, res, &gerr)
		wv.soupFree(sc)
		done(finishErr(ok, gerr, "add_cookie"))
	}))
}

func finishErr(ok bool, gerr ptr, what string) error {
	if gerr != 0 {
		return errors.New("tango/linux: " + gErrorMessage(gerr))
	}
	if !ok {
		return errors.New("tango/linux: " + what + " failed")
	}
	return nil
}

// deleteCookies deletes the cookies of cl selected by match, then calls
// done once with the first error. libsoup matches a deleted cookie by name,
// value and path, so the cookies read back from WebKit are passed as is.
func deleteCookies(cl *cookieList, match func(cookieFields) bool, done func(error)) {
	cm := cookieManager()
	var sel []ptr
	for _, c := range cl.ptrs {
		if match(readSoupCookie(c)) {
			sel = append(sel, c)
		}
	}
	if len(sel) == 0 {
		cl.free()
		done(nil)
		return
	}
	left := len(sel)
	var first error
	for _, c := range sel {
		wv.cmDelete(cm, c, 0, cbArg3, asyncData(func(res ptr) {
			var gerr ptr
			ok := wv.cmDelFinish(cm, res, &gerr)
			if err := finishErr(ok, gerr, "delete_cookie"); err != nil && first == nil {
				first = err
			}
			if left--; left == 0 {
				cl.free()
				done(first)
			}
		}))
	}
}

func (s *surface) DeleteCookie(c *http.Cookie, done func(error)) {
	if c == nil || c.Domain == "" {
		done(errors.New("tango/linux: DeleteCookie needs Domain"))
		return
	}
	uri := ""
	if wv.cmGetAll == nil {
		uri = cookieLookupURL(c)
	}
	fetchCookies(uri, func(cl *cookieList, err error) {
		if err != nil {
			done(err)
			return
		}
		deleteCookies(cl, func(f cookieFields) bool { return sameCookie(f, c) }, done)
	})
}

// ClearCookies: "" clears the cookies of the website data manager; a
// domain needs get_all_cookies (WebKitGTK 2.42).
func (s *surface) ClearCookies(domain string, done func(error)) {
	if domain == "" {
		s.ClearBrowsingData(platform.DataCookies, done)
		return
	}
	if cookieManager() == 0 || wv.cmGetAll == nil {
		done(platform.ErrUnsupported)
		return
	}
	fetchCookies("", func(cl *cookieList, err error) {
		if err != nil {
			done(err)
			return
		}
		deleteCookies(cl, func(f cookieFields) bool { return cookieDomainMatch(f.Domain, domain) }, done)
	})
}

// Website data ----------------------------------------------------------------------

func (s *surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	wv.once.Do(bindWebExt)
	if !wv.ok || !wv.dataOK {
		done(platform.ErrUnsupported)
		return
	}
	types := websiteDataTypes(kinds)
	if types == 0 {
		done(nil)
		return
	}
	m := wv.ctxDataMgr(wv.webCtxDefault())
	if m == 0 {
		done(platform.ErrUnsupported)
		return
	}
	wv.dmClear(m, types, 0, 0, cbArg3, asyncData(func(res ptr) {
		var gerr ptr
		ok := wv.dmClearDone(m, res, &gerr)
		done(finishErr(ok, gerr, "website_data_manager_clear"))
	}))
}

// User agent ------------------------------------------------------------------------

func (s *surface) SetUserAgent(ua string) {
	if s.closed || s.web == 0 || !wv.ok || wv.settingsSetUA == nil {
		return
	}
	wv.settingsSetUA(webkitWebViewGetSettings(s.web), cstr(ua)) // NULL: default
}

// Find in page ------------------------------------------------------------------------

func (s *surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	if s.closed || s.wext == nil || s.wext.fc == 0 {
		done(platform.FindResult{}, platform.ErrUnsupported)
		return
	}
	x := s.wext
	s.resolveFind(platform.FindResult{}) // a pending request is superseded
	if text == "" {
		s.StopFind()
		done(platform.FindResult{}, nil)
		return
	}
	x.findActive, x.findCount = true, 0
	s.startFind(done, true)
	wv.findSearch(x.fc, text, findOptions(opts), findMaxMatches)
}

func (s *surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	if s.closed || s.wext == nil || s.wext.fc == 0 {
		done(platform.FindResult{}, platform.ErrUnsupported)
		return
	}
	x := s.wext
	s.resolveFind(platform.FindResult{})
	if !x.findActive {
		done(platform.FindResult{}, nil)
		return
	}
	s.startFind(done, false)
	if backward {
		wv.findPrevious(x.fc)
	} else {
		wv.findNext(x.fc)
	}
}

func (s *surface) StopFind() {
	if s.wext == nil || s.wext.fc == 0 {
		return
	}
	s.resolveFind(platform.FindResult{})
	s.wext.findActive, s.wext.findCount = false, 0
	if !s.closed {
		wv.findFinish(s.wext.fc)
	}
}

// startFind records done as the pending request; a timeout answers it if
// WebKit never reports (done must run exactly once).
func (s *surface) startFind(done func(platform.FindResult, error), isNew bool) {
	x := s.wext
	x.findSeq++
	seq := x.findSeq
	x.findDone, x.findNew = done, isNew
	later(findTimeout, func() {
		if x.findSeq == seq && x.findDone != nil {
			s.resolveFind(platform.FindResult{Matches: -1})
		}
	})
}

func (s *surface) resolveFind(r platform.FindResult) {
	x := s.wext
	if x == nil || x.findDone == nil {
		return
	}
	done := x.findDone
	x.findDone = nil
	done(r, nil)
}

// onFoundText: found-text(find_controller, match_count). WebKitGTK does
// not report which match is highlighted, so Active stays 0.
func (s *surface) onFoundText(_, n ptr) ptr {
	if x := s.wext; x != nil && x.findDone != nil {
		x.findCount = findMatches(uint32(n))
		s.resolveFind(platform.FindResult{Matches: x.findCount, Found: true})
	}
	return 0
}

// onFailedToFind: failed-to-find-text, no (further) match. For FindNext
// without wrapping the page still has the matches counted before.
func (s *surface) onFailedToFind(_, _ ptr) ptr {
	if x := s.wext; x != nil && x.findDone != nil {
		if x.findNew {
			x.findCount = 0
		}
		s.resolveFind(platform.FindResult{Matches: x.findCount})
	}
	return 0
}

// PDF -------------------------------------------------------------------------------

// PrintToPDF prints through GTK's "Print to File" printer into a
// temporary file and reads it back.
func (s *surface) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	if s.closed || s.web == 0 || !wv.ok || !wv.pdfOK {
		done(nil, platform.ErrUnsupported)
		return
	}
	f, err := os.CreateTemp("", "tango-*.pdf")
	if err != nil {
		done(nil, err)
		return
	}
	path := f.Name()
	f.Close()

	settings := wv.psNew()
	printer := "Print to File" // GTK's file printer; its name is translated
	if wv.dgettext != nil {
		if p := goStr(wv.dgettext("gtk30", printer)); p != "" {
			printer = p
		}
	}
	wv.psSet(settings, "printer", printer)
	wv.psSet(settings, "output-file-format", "pdf")
	wv.psSet(settings, "output-uri", toFileURI(path))
	setup := wv.setupNew()
	if opts.Landscape {
		wv.psSetOrient(settings, gtkPageLandscape)
		wv.setupSetOrient(setup, gtkPageLandscape)
	}
	if pw, ph, ok := pdfPaper(opts); ok {
		paper := wv.paperNewCustom("tango-custom", "Custom", pw, ph, gtkUnitInch)
		wv.psSetPaper(settings, paper)
		wv.setupSetPaper(setup, paper)
		wv.paperFree(paper)
	}

	op := wv.printOpNew(s.web)
	if op == 0 {
		gObjectUnref(settings)
		gObjectUnref(setup)
		os.Remove(path)
		done(nil, errors.New("tango/linux: webkit_print_operation_new failed"))
		return
	}
	wv.printOpSettings(op, settings)
	wv.printOpSetup(op, setup)
	gObjectUnref(settings) // the operation keeps its own references
	gObjectUnref(setup)

	ws := webkitWebViewGetSettings(s.web)
	restoreBG := func() {}
	if opts.NoBackground && wv.getPrintBG != nil {
		prev := wv.getPrintBG(ws)
		wv.setPrintBG(ws, false)
		restoreBG = func() { wv.setPrintBG(ws, prev) }
	}

	failMsg := ""
	fired := false
	connect(op, "failed", 1, func(_, gerr ptr) ptr {
		failMsg = gErrorText(gerr)
		if failMsg == "" {
			failMsg = "printing failed"
		}
		return 0
	})
	connect(op, "finished", 0, func(_, _ ptr) ptr {
		later(0, func() { // not from inside the emission
			if fired {
				return
			}
			fired = true
			restoreBG()
			gObjectUnref(op)
			data, rerr := os.ReadFile(path)
			os.Remove(path)
			switch {
			case failMsg != "":
				done(nil, errors.New("tango/linux: "+failMsg))
			case rerr != nil:
				done(nil, rerr)
			case len(data) == 0:
				done(nil, errors.New("tango/linux: print produced no PDF"))
			default:
				done(data, nil)
			}
		})
		return 0
	})
	wv.printOpPrint(op)
}

// Screenshot ------------------------------------------------------------------------

// Screenshot snapshots the visible region to a cairo surface and writes it
// as PNG through a temporary file.
func (s *surface) Screenshot(done func([]byte, error)) {
	if s.closed || s.web == 0 || !wv.ok || !wv.snapOK {
		done(nil, platform.ErrUnsupported)
		return
	}
	web := s.web
	wv.snapshot(web, wkSnapshotVisible, wkSnapshotOptionsNone, 0, cbArg3, asyncData(func(res ptr) {
		var gerr ptr
		cs := wv.snapshotDone(web, res, &gerr)
		if gerr != 0 || cs == 0 {
			msg := "snapshot failed"
			if gerr != 0 {
				msg = gErrorMessage(gerr)
			}
			if cs != 0 {
				wv.cairoDestroy(cs)
			}
			done(nil, errors.New("tango/linux: "+msg))
			return
		}
		f, err := os.CreateTemp("", "tango-*.png")
		if err != nil {
			wv.cairoDestroy(cs)
			done(nil, err)
			return
		}
		path := f.Name()
		f.Close()
		st := wv.cairoWritePNG(cs, path)
		wv.cairoDestroy(cs)
		data, rerr := os.ReadFile(path)
		os.Remove(path)
		switch {
		case st != 0:
			done(nil, errors.New("tango/linux: cairo_surface_write_to_png failed"))
		case rerr != nil:
			done(nil, rerr)
		default:
			done(data, nil)
		}
	}))
}
