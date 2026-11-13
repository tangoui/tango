//go:build windows && (amd64 || arm64)

package windows

// Advanced WebView capabilities (platform/webview_ext.go) on WebView2:
// permission, navigation policy, new-window, Basic authentication and
// download events reported to platform.WebHost, and the optional surface
// controls WebFinder, WebCookies, WebData, WebUserAgent, WebHeaders,
// WebPDF, WebSnapshot and WebDownloads, plus WebCreator. Written against
// Microsoft's WebView2 documentation; interface IDs and vtable indices are
// from the WebView2 SDK's WebView2.h (1.0.4078.44). Interfaces newer than
// the installed runtime are detected with QueryInterface: the event is
// then not registered, or the control reports platform.ErrUnsupported.
// Pure logic is in webview_ext_plan.go.

import (
	"errors"
	"math"
	"net/http"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.WebCreator   = (*Backend)(nil)
	_ platform.WebFinder    = (*surface)(nil)
	_ platform.WebCookies   = (*surface)(nil)
	_ platform.WebData      = (*surface)(nil)
	_ platform.WebUserAgent = (*surface)(nil)
	_ platform.WebHeaders   = (*surface)(nil)
	_ platform.WebPDF       = (*surface)(nil)
	_ platform.WebSnapshot  = (*surface)(nil)
	_ platform.WebDownloads = (*surface)(nil)
)

var (
	iidICoreWebView2_2            = guid("9E8F0CF8-E670-4B5E-B2BC-73E061E3184C")
	iidICoreWebView2_4            = guid("20d02d59-6df2-42dc-bd06-f98a694b1302")
	iidICoreWebView2_7            = guid("79c24d83-09a3-45ae-9418-487f32a58740")
	iidICoreWebView2_10           = guid("b1690564-6f5a-4983-8e48-31d1143fecdb")
	iidICoreWebView2_13           = guid("f75f09a8-667e-4983-88d6-c8773f315e84")
	iidICoreWebView2_28           = guid("62e50381-5bf5-51a8-aae0-f20a3a9c8a90")
	iidICoreWebView2Settings2     = guid("ee9a0f68-f46c-4e32-ac23-ef8cac224d2a")
	iidICoreWebView2Profile2      = guid("fa740d4b-5eae-4344-a8ad-74be31925397")
	iidICoreWebView2Environment6  = guid("e59ee362-acbd-4857-9a8e-d3644d9459a9")
	iidICoreWebView2Environment15 = guid("2ac5ebfb-e654-5961-a667-7971885c7b27")
)

// Vtable indices (WebView2.h; each count includes IUnknown's three).
const (
	// ICoreWebView2
	wvAddPermissionRequested            = 23
	wvCapturePreview                    = 30
	wvAddNewWindowRequested             = 44
	wvRemoveWebResourceRequestedFilter  = 58
	wv2GetCookieManager                 = 66  // ICoreWebView2_2
	wv4AddDownloadStarting              = 75  // ICoreWebView2_4
	wv7PrintToPdf                       = 80  // ICoreWebView2_7
	wv10AddBasicAuthenticationRequested = 97  // ICoreWebView2_10
	wv13GetProfile                      = 105 // ICoreWebView2_13
	wv28GetFind                         = 135 // ICoreWebView2_28

	// ICoreWebView2Environment6 / 15
	env6CreatePrintSettings = 14
	env15CreateFindOptions  = 29

	// ICoreWebView2Settings2
	set2GetUserAgent = 21
	set2PutUserAgent = 22

	// ICoreWebView2Deferral
	deferralComplete = 3

	// ICoreWebView2PermissionRequestedEventArgs
	permArgsGetURI      = 3
	permArgsGetKind     = 4
	permArgsPutState    = 7
	permArgsGetDeferral = 8

	// ICoreWebView2NavigationStartingEventArgs / CompletedEventArgs
	navStartingGetIsUserInitiated = 4
	navStartingGetNavigationID    = 9
	navCompletedGetNavigationID   = 5

	// ICoreWebView2NewWindowRequestedEventArgs, ICoreWebView2WindowFeatures
	newWinGetURI             = 3
	newWinPutHandled         = 6
	newWinGetIsUserInitiated = 8
	newWinGetWindowFeatures  = 10
	featGetHasSize           = 4
	featGetHeight            = 7
	featGetWidth             = 8

	// ICoreWebView2BasicAuthenticationRequestedEventArgs / Response
	authArgsGetURI       = 3
	authArgsGetChallenge = 4
	authArgsGetResponse  = 5
	authArgsPutCancel    = 7
	authArgsGetDeferral  = 8
	authRespPutUserName  = 4
	authRespPutPassword  = 6

	// ICoreWebView2DownloadStartingEventArgs
	dlArgsGetOperation  = 3
	dlArgsPutCancel     = 5
	dlArgsGetResultPath = 6
	dlArgsPutResultPath = 7
	dlArgsPutHandled    = 9
	dlArgsGetDeferral   = 10

	// ICoreWebView2DownloadOperation
	dlAddBytesReceivedChanged    = 3
	dlRemoveBytesReceivedChanged = 4
	dlAddStateChanged            = 7
	dlRemoveStateChanged         = 8
	dlGetURI                     = 9
	dlGetContentDisposition      = 10
	dlGetMimeType                = 11
	dlGetTotalBytes              = 12
	dlGetBytesReceived           = 13
	dlGetState                   = 16
	dlGetInterruptReason         = 17
	dlCancel                     = 18

	// ICoreWebView2CookieManager
	cmCreateCookie                   = 3
	cmGetCookies                     = 5
	cmAddOrUpdateCookie              = 6
	cmDeleteCookie                   = 7
	cmDeleteCookiesWithDomainAndPath = 9
	cmDeleteAllCookies               = 10

	// ICoreWebView2CookieList
	clGetCount        = 3
	clGetValueAtIndex = 4

	// ICoreWebView2Cookie
	ckGetName       = 3
	ckGetValue      = 4
	ckGetDomain     = 6
	ckGetPath       = 7
	ckGetExpires    = 8
	ckPutExpires    = 9
	ckGetIsHttpOnly = 10
	ckPutIsHttpOnly = 11
	ckGetSameSite   = 12
	ckPutSameSite   = 13
	ckGetIsSecure   = 14
	ckPutIsSecure   = 15
	ckGetIsSession  = 16

	// ICoreWebView2Profile2
	prof2ClearBrowsingData = 10

	// ICoreWebView2PrintSettings
	psPutOrientation            = 4
	psPutPageWidth              = 8
	psPutPageHeight             = 10
	psPutShouldPrintBackgrounds = 20

	// ICoreWebView2Find, ICoreWebView2FindOptions
	findGetActiveMatchIndex        = 3
	findGetMatchCount              = 4
	findStart                      = 9
	findFindNext                   = 10
	findFindPrevious               = 11
	findStop                       = 12
	foPutFindTerm                  = 4
	foPutIsCaseSensitive           = 6
	foPutShouldHighlightAllMatches = 8
	foPutSuppressDefaultFindDialog = 12

	// ICoreWebView2HttpRequestHeaders
	headersSetHeader = 6

	// IStream
	streamSeek = 5

	capturePreviewPNG = 0 // COREWEBVIEW2_CAPTURE_PREVIEW_IMAGE_FORMAT_PNG
	printLandscape    = 1 // COREWEBVIEW2_PRINT_ORIENTATION_LANDSCAPE
)

var errWebClosed = errors.New("tango: surface closed")

// downloadSeq allocates download IDs, unique for the process.
var downloadSeq atomic.Uint64

// webExt is the advanced-WebView state of a surface.
type webExt struct {
	ua        string
	uaSet     bool   // SetUserAgent / WebOptions was given
	defaultUA string // the runtime's, read before the first change
	haveUA    bool

	headers      [][2]string // extra request headers (non-asset requests)
	headerFilter bool        // the "*" WebResourceRequested filter is added

	canceledNavs map[uint64]bool // navigations canceled by ShouldNavigate

	cookies uintptr // ICoreWebView2CookieManager, once asked for

	find       uintptr // ICoreWebView2Find, once asked for
	findOn     bool
	findNoWrap bool
	findActive int

	downloads map[uint64]*webDownload

	// waiting holds the error callbacks of operations in flight, run when
	// the surface goes away first (done is called exactly once).
	waiting map[uint64]func(error)
	waitSeq uint64
}

type webDownload struct {
	id           uint64
	op           uintptr // ICoreWebView2DownloadOperation (referenced)
	path         string
	total        int64
	lastProgress time.Time
	bytesTok     int64
	stateTok     int64
}

// webHost is the Host's WebHost, or nil (fake hosts).
func (s *surface) webHost() platform.WebHost {
	wh, _ := s.b.h.(platform.WebHost)
	return wh
}

// addEvent registers an event handler on obj; false if it failed.
func addEvent(obj uintptr, index int, fn func(sender, args uintptr)) (int64, bool) {
	var tok int64
	hr := withHandler(fn, func(h uintptr) uintptr { return comCall(obj, index, h, uintptr(unsafe.Pointer(&tok))) })
	return tok, !failed(hr)
}

func comBool(obj uintptr, index int) bool {
	var v int32
	if failed(comCall(obj, index, uintptr(unsafe.Pointer(&v)))) {
		return false
	}
	return v != 0
}

func comObject(obj uintptr, index int) uintptr {
	var out uintptr
	if failed(comCall(obj, index, uintptr(unsafe.Pointer(&out)))) {
		return 0
	}
	return out
}

// getDeferral returns the referenced deferral of event args, or 0.
func getDeferral(args uintptr, index int) uintptr { return comObject(args, index) }

func completeDeferral(d uintptr) {
	if d != 0 {
		comCall(d, deferralComplete)
		release(d)
	}
}

// initWebExt runs from setUp once the ICoreWebView2 exists, before the
// first navigation: it registers the WebHost events and applies the user
// agent and extra headers.
func (s *surface) initWebExt() {
	if wh := s.webHost(); wh != nil {
		addEvent(s.webview, wvAddPermissionRequested, s.permissionRequested)
		addEvent(s.webview, wvAddNewWindowRequested, s.newWindowRequested)
		if wv := queryInterface(s.webview, &iidICoreWebView2_10); wv != 0 {
			addEvent(wv, wv10AddBasicAuthenticationRequested, s.basicAuthenticationRequested)
			release(wv)
		}
		if wv := queryInterface(s.webview, &iidICoreWebView2_4); wv != 0 {
			addEvent(wv, wv4AddDownloadStarting, s.downloadStarting)
			release(wv)
		}
	}
	s.applyUserAgent()
	s.syncHeaderFilter()
}

// closeWebExt runs when the window is destroyed, before SurfaceClosed:
// accepted downloads end with DownloadCanceled, operations in flight get
// an error, COM references are dropped.
func (s *surface) closeWebExt() {
	for _, d := range s.web.downloads {
		comCall(d.op, dlCancel)
		s.endDownload(d, platform.DownloadCanceled, "")
	}
	s.dropWebWaiters(errWebClosed)
	release(s.web.cookies)
	release(s.web.find)
	s.web.cookies, s.web.find = 0, 0
}

// dropWebWaiters fails every operation in flight (surface closed, or the
// WebView could not be created).
func (s *surface) dropWebWaiters(err error) {
	w := s.web.waiting
	s.web.waiting = nil
	for _, fail := range w {
		fail(err)
	}
}

// track registers fail for the case the surface goes away first; claim
// reports, once, whether the caller still owns the operation's callback.
func (s *surface) track(fail func(error)) (claim func() bool) {
	if s.web.waiting == nil {
		s.web.waiting = map[uint64]func(error){}
	}
	s.web.waitSeq++
	k := s.web.waitSeq
	s.web.waiting[k] = fail
	return func() bool {
		if _, ok := s.web.waiting[k]; !ok {
			return false
		}
		delete(s.web.waiting, k)
		return true
	}
}

// webCall runs fn once the WebView is ready; fail receives the error when
// it never will be. fn calls claim before invoking its callback.
func (s *surface) webCall(fail func(error), fn func(claim func() bool)) {
	switch {
	case s.canvas != nil:
		fail(platform.ErrUnsupported)
		return
	case s.closed:
		fail(errWebClosed)
		return
	case s.failed != nil:
		fail(s.failed)
		return
	}
	claim := s.track(fail)
	s.whenReady(func() { fn(claim) })
}

// ---------------------------------------------------------------------------
// WebHost events

func (s *surface) permissionRequested(_, args uintptr) {
	wh := s.webHost()
	if wh == nil || s.closed {
		return
	}
	var kind int32
	comCall(args, permArgsGetKind, uintptr(unsafe.Pointer(&kind)))
	req := platform.PermissionRequest{
		Origin: requestOrigin(comString(args, permArgsGetURI)),
		Kinds:  []platform.WebPermission{webPermission(kind)},
	}
	addRef(args)
	deferral := getDeferral(args, permArgsGetDeferral)
	answered := false
	wh.OnPermissionRequest(s.id, req, func(d platform.PermissionDecision) {
		if answered {
			return
		}
		answered = true
		comCall(args, permArgsPutState, uintptr(permissionState(d)))
		completeDeferral(deferral)
		release(args)
	})
}

// allowNavigation asks ShouldNavigate about a main-frame navigation and
// cancels it when refused (navigationStarting, surface_ext_windows.go).
func (s *surface) allowNavigation(args uintptr, uri string) bool {
	wh := s.webHost()
	if wh == nil {
		return true
	}
	nav := platform.NavigationRequest{URL: uri, UserInitiated: comBool(args, navStartingGetIsUserInitiated)}
	if wh.ShouldNavigate(s.id, nav) {
		return true
	}
	var id uint64
	if !failed(comCall(args, navStartingGetNavigationID, uintptr(unsafe.Pointer(&id)))) {
		if s.web.canceledNavs == nil || len(s.web.canceledNavs) > 64 {
			s.web.canceledNavs = map[uint64]bool{}
		}
		s.web.canceledNavs[id] = true
	}
	comCall(args, navStartingPutCancel, 1)
	return false
}

// canceledNavigation reports (once) whether a completed navigation is one
// ShouldNavigate refused: it is then reported neither as loaded nor as
// failed.
func (s *surface) canceledNavigation(args uintptr) bool {
	if len(s.web.canceledNavs) == 0 {
		return false
	}
	var id uint64
	if failed(comCall(args, navCompletedGetNavigationID, uintptr(unsafe.Pointer(&id)))) || !s.web.canceledNavs[id] {
		return false
	}
	delete(s.web.canceledNavs, id)
	return true
}

func (s *surface) newWindowRequested(_, args uintptr) {
	// Always handled: no WebView2 popup window; window.open returns null.
	comCall(args, newWinPutHandled, 1)
	wh := s.webHost()
	if wh == nil || s.closed {
		return
	}
	req := platform.NewWindowRequest{
		URL:           newWindowURL(comString(args, newWinGetURI)),
		UserInitiated: comBool(args, newWinGetIsUserInitiated),
	}
	if f := comObject(args, newWinGetWindowFeatures); f != 0 {
		if comBool(f, featGetHasSize) {
			var w, h uint32
			comCall(f, featGetWidth, uintptr(unsafe.Pointer(&w)))
			comCall(f, featGetHeight, uintptr(unsafe.Pointer(&h)))
			req.Width, req.Height = int(w), int(h)
		}
		release(f)
	}
	wh.OnNewWindow(s.id, req)
}

func (s *surface) basicAuthenticationRequested(_, args uintptr) {
	wh := s.webHost()
	if wh == nil || s.closed {
		return
	}
	ch := authChallenge(comString(args, authArgsGetURI), comString(args, authArgsGetChallenge))
	addRef(args)
	deferral := getDeferral(args, authArgsGetDeferral)
	answered := false
	wh.OnAuthChallenge(s.id, ch, func(r platform.AuthResponse) {
		if answered {
			return
		}
		answered = true
		switch r.Action {
		case platform.AuthCredentials:
			if resp := comObject(args, authArgsGetResponse); resp != 0 {
				comCall(resp, authRespPutUserName, uintptr(unsafe.Pointer(u16(r.User))))
				comCall(resp, authRespPutPassword, uintptr(unsafe.Pointer(u16(r.Password))))
				release(resp)
			}
		case platform.AuthCancel:
			comCall(args, authArgsPutCancel, 1)
		}
		completeDeferral(deferral)
		release(args)
	})
}

func (s *surface) downloadStarting(_, args uintptr) {
	wh := s.webHost()
	if wh == nil || s.closed {
		return
	}
	op := comObject(args, dlArgsGetOperation)
	if op == 0 {
		return
	}
	var total int64
	if failed(comCall(op, dlGetTotalBytes, uintptr(unsafe.Pointer(&total)))) {
		total = -1
	}
	uri := comString(op, dlGetURI)
	req := platform.DownloadRequest{
		ID:       downloadSeq.Add(1),
		URL:      uri,
		Filename: downloadFilename(comString(op, dlGetContentDisposition), comString(args, dlArgsGetResultPath), uri),
		MIME:     comString(op, dlGetMimeType),
		Size:     total,
	}
	comCall(args, dlArgsPutHandled, 1) // no default download UI
	addRef(args)
	deferral := getDeferral(args, dlArgsGetDeferral)
	answered := false
	wh.OnDownloadStarted(s.id, req, func(path string) {
		if answered {
			return
		}
		answered = true
		if path == "" || s.closed {
			comCall(args, dlArgsPutCancel, 1)
			release(op)
		} else {
			if fi, err := os.Lstat(path); err == nil && fi.Mode().IsRegular() {
				_ = os.Remove(path) // replaced, not renamed "name (1)"
			}
			comCall(args, dlArgsPutResultPath, uintptr(unsafe.Pointer(u16(path))))
			s.trackDownload(&webDownload{id: req.ID, op: op, path: path, total: total})
		}
		completeDeferral(deferral)
		release(args)
	})
}

// trackDownload reports progress and the end of an accepted download.
func (s *surface) trackDownload(d *webDownload) {
	if s.web.downloads == nil {
		s.web.downloads = map[uint64]*webDownload{}
	}
	s.web.downloads[d.id] = d
	d.bytesTok, _ = addEvent(d.op, dlAddBytesReceivedChanged, func(_, _ uintptr) { s.downloadChanged(d, false) })
	d.stateTok, _ = addEvent(d.op, dlAddStateChanged, func(_, _ uintptr) { s.downloadChanged(d, true) })
}

func (s *surface) downloadReceived(d *webDownload) int64 {
	var n int64
	comCall(d.op, dlGetBytesReceived, uintptr(unsafe.Pointer(&n)))
	return n
}

func (s *surface) downloadChanged(d *webDownload, stateChange bool) {
	if s.web.downloads[d.id] != d {
		return // already ended
	}
	var state, reason int32
	comCall(d.op, dlGetState, uintptr(unsafe.Pointer(&state)))
	if state == wvDownloadInterrupted {
		comCall(d.op, dlGetInterruptReason, uintptr(unsafe.Pointer(&reason)))
	}
	st, end := downloadState(state, reason)
	if end {
		msg := ""
		if st == platform.DownloadFailed {
			msg = downloadError(reason)
		}
		s.endDownload(d, st, msg)
		return
	}
	now := time.Now()
	if !stateChange && now.Sub(d.lastProgress) < progressInterval {
		return
	}
	d.lastProgress = now
	if wh := s.webHost(); wh != nil {
		wh.OnDownloadEvent(s.id, platform.DownloadEvent{ID: d.id, State: platform.DownloadProgress,
			Received: s.downloadReceived(d), Total: d.total, Path: d.path})
	}
}

// endDownload reports the one final event of d and forgets it.
func (s *surface) endDownload(d *webDownload, st platform.DownloadState, msg string) {
	if s.web.downloads[d.id] != d {
		return
	}
	delete(s.web.downloads, d.id)
	ev := platform.DownloadEvent{ID: d.id, State: st, Received: s.downloadReceived(d), Total: d.total, Path: d.path, Error: msg}
	if st == platform.DownloadDone && ev.Total < 0 {
		ev.Total = ev.Received
	}
	comCall(d.op, dlRemoveBytesReceivedChanged, uintptr(d.bytesTok))
	comCall(d.op, dlRemoveStateChanged, uintptr(d.stateTok))
	release(d.op)
	d.op = 0
	if wh := s.webHost(); wh != nil {
		wh.OnDownloadEvent(s.id, ev)
	}
}

// platform.WebDownloads

func (s *surface) CancelDownload(id uint64) {
	if d := s.web.downloads[id]; d != nil {
		if failed(comCall(d.op, dlCancel)) {
			s.endDownload(d, platform.DownloadCanceled, "")
		}
		// Otherwise StateChanged reports the cancellation.
	}
}

// ---------------------------------------------------------------------------
// WebCreator, WebUserAgent, WebHeaders

func (b *Backend) NewSurfaceWeb(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	ns, err := b.newSurface(id, opts, &ex)
	if err != nil {
		return ns, err
	}
	// The controller is created asynchronously, so setUp (initWebExt)
	// still sees these before the first navigation.
	s := ns.(*surface)
	if web.UserAgent != "" {
		s.SetUserAgent(web.UserAgent)
	}
	if len(web.Headers) > 0 {
		s.SetExtraHeaders(web.Headers)
	}
	return s, nil
}

func (s *surface) SetUserAgent(ua string) {
	s.web.ua, s.web.uaSet = ua, true
	if s.ready && !s.closed {
		s.applyUserAgent()
	}
}

// applyUserAgent sets ICoreWebView2Settings2::UserAgent; "" restores the
// runtime's, read before the first change.
func (s *surface) applyUserAgent() {
	if !s.web.uaSet || s.webview == 0 {
		return
	}
	settings := comObject(s.webview, wvGetSettings)
	if settings == 0 {
		return
	}
	defer release(settings)
	s2 := queryInterface(settings, &iidICoreWebView2Settings2)
	if s2 == 0 {
		return
	}
	defer release(s2)
	if !s.web.haveUA {
		s.web.defaultUA, s.web.haveUA = comString(s2, set2GetUserAgent), true
	}
	ua := s.web.ua
	if ua == "" {
		ua = s.web.defaultUA
	}
	comCall(s2, set2PutUserAgent, uintptr(unsafe.Pointer(u16(ua))))
}

// SetExtraHeaders adds h to every request other than asset requests
// (which get the core's own headers): WebResourceRequested for all URLs
// sets them on the request while any are set.
func (s *surface) SetExtraHeaders(h http.Header) {
	s.web.headers = headerValues(h)
	if s.ready && !s.closed {
		s.syncHeaderFilter()
	}
}

func (s *surface) syncHeaderFilter() {
	want := len(s.web.headers) > 0
	if want == s.web.headerFilter || s.webview == 0 {
		return
	}
	index := wvAddWebResourceRequestedFilter
	if !want {
		index = wvRemoveWebResourceRequestedFilter
	}
	if !failed(comCall(s.webview, index, uintptr(unsafe.Pointer(u16("*"))), webResourceContextAll)) {
		s.web.headerFilter = want
	}
}

// extraRequestHeaders is called by webResourceRequested for every
// intercepted request; it sets the extra headers on non-asset requests and
// reports whether uri is an asset request, which the caller then serves.
func (s *surface) extraRequestHeaders(req uintptr, uri string) bool {
	if isAssetURI(uri) {
		return true
	}
	if len(s.web.headers) == 0 {
		return false
	}
	headers := comObject(req, reqGetHeaders)
	if headers == 0 {
		return false
	}
	for _, kv := range s.web.headers {
		comCall(headers, headersSetHeader, uintptr(unsafe.Pointer(u16(kv[0]))), uintptr(unsafe.Pointer(u16(kv[1]))))
	}
	release(headers)
	return false
}

// ---------------------------------------------------------------------------
// WebCookies

func (s *surface) cookieManager() uintptr {
	if s.web.cookies == 0 && s.webview != 0 {
		if wv := queryInterface(s.webview, &iidICoreWebView2_2); wv != 0 {
			s.web.cookies = comObject(wv, wv2GetCookieManager)
			release(wv)
		}
	}
	return s.web.cookies
}

// getCookies lists the cookies for uri ("" = all) as COM objects; fn owns
// none of them (they are released after fn returns).
func (s *surface) getCookies(uri string, claim func() bool, fail func(error), fn func(list uintptr)) {
	cm := s.cookieManager()
	if cm == 0 {
		if claim() {
			fail(platform.ErrUnsupported)
		}
		return
	}
	hr := withHandler(func(hr, list uintptr) {
		if !claim() {
			return
		}
		if failed(hr) || list == 0 {
			fail(hresultError("ICoreWebView2CookieManager::GetCookies", hr))
			return
		}
		fn(list)
	}, func(h uintptr) uintptr {
		return comCall(cm, cmGetCookies, uintptr(unsafe.Pointer(u16(uri))), h)
	})
	if failed(hr) && claim() {
		fail(hresultError("ICoreWebView2CookieManager::GetCookies", hr))
	}
}

// eachCookie calls fn with every cookie of a list.
func eachCookie(list uintptr, fn func(cookie uintptr)) {
	var n uint32
	comCall(list, clGetCount, uintptr(unsafe.Pointer(&n)))
	for i := uint32(0); i < n; i++ {
		var c uintptr
		if failed(comCall(list, clGetValueAtIndex, uintptr(i), uintptr(unsafe.Pointer(&c)))) || c == 0 {
			continue
		}
		fn(c)
		release(c)
	}
}

func httpCookie(c uintptr) *http.Cookie {
	hc := &http.Cookie{
		Name:     comString(c, ckGetName),
		Value:    comString(c, ckGetValue),
		Domain:   comString(c, ckGetDomain),
		Path:     comString(c, ckGetPath),
		HttpOnly: comBool(c, ckGetIsHttpOnly),
		Secure:   comBool(c, ckGetIsSecure),
	}
	var ss int32
	if !failed(comCall(c, ckGetSameSite, uintptr(unsafe.Pointer(&ss)))) {
		hc.SameSite = sameSiteFromWV(ss)
	}
	if !comBool(c, ckGetIsSession) {
		var exp float64
		if !failed(comCall(c, ckGetExpires, uintptr(unsafe.Pointer(&exp)))) && exp > 0 {
			hc.Expires = fromUnixSeconds(exp)
		}
	}
	return hc
}

func (s *surface) Cookies(uri string, done func([]*http.Cookie, error)) {
	fail := func(err error) { done(nil, err) }
	s.webCall(fail, func(claim func() bool) {
		s.getCookies(uri, claim, fail, func(list uintptr) {
			var out []*http.Cookie
			eachCookie(list, func(c uintptr) { out = append(out, httpCookie(c)) })
			done(out, nil)
		})
	})
}

func (s *surface) SetCookie(c *http.Cookie, done func(error)) {
	if c == nil || c.Domain == "" || c.Name == "" {
		done(errors.New("tango: SetCookie needs a cookie with Name and Domain"))
		return
	}
	cc := *c
	s.webCall(done, func(claim func() bool) {
		expires, del := cookieExpiry(&cc, time.Now())
		if del {
			err := s.deleteCookie(&cc)
			if claim() {
				done(err)
			}
			return
		}
		if !expires.IsZero() && !doubleArgs {
			s.setCookieCDP(&cc, expires, claim, done)
			return
		}
		err := s.setCookie(&cc, expires)
		if claim() {
			done(err)
		}
	})
}

func (s *surface) setCookie(c *http.Cookie, expires time.Time) error {
	cm := s.cookieManager()
	if cm == 0 {
		return platform.ErrUnsupported
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	var ck uintptr
	hr := comCall(cm, cmCreateCookie, uintptr(unsafe.Pointer(u16(c.Name))), uintptr(unsafe.Pointer(u16(c.Value))),
		uintptr(unsafe.Pointer(u16(c.Domain))), uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&ck)))
	if failed(hr) || ck == 0 {
		return hresultError("ICoreWebView2CookieManager::CreateCookie", hr)
	}
	defer release(ck)
	comCall(ck, ckPutIsHttpOnly, boolArg(c.HttpOnly))
	comCall(ck, ckPutIsSecure, boolArg(c.Secure))
	if v, ok := sameSiteToWV(c.SameSite); ok {
		comCall(ck, ckPutSameSite, uintptr(v))
	}
	if !expires.IsZero() {
		comCall(ck, ckPutExpires, uintptr(math.Float64bits(unixSeconds(expires))))
	}
	if hr := comCall(cm, cmAddOrUpdateCookie, ck); failed(hr) {
		return hresultError("ICoreWebView2CookieManager::AddOrUpdateCookie", hr)
	}
	return nil
}

// setCookieCDP sets a cookie with an expiry through the DevTools protocol
// (ARM64, see doubleArgs).
func (s *surface) setCookieCDP(c *http.Cookie, expires time.Time, claim func() bool, done func(error)) {
	hr := withHandler(func(hr, _ uintptr) {
		if !claim() {
			return
		}
		if failed(hr) {
			done(hresultError("Network.setCookie", hr))
			return
		}
		done(nil)
	}, func(h uintptr) uintptr {
		return comCall(s.webview, wvCallDevToolsProtocolMethod, uintptr(unsafe.Pointer(u16("Network.setCookie"))),
			uintptr(unsafe.Pointer(u16(cdpSetCookieParams(c, expires)))), h)
	})
	if failed(hr) && claim() {
		done(hresultError("CallDevToolsProtocolMethod", hr))
	}
}

func (s *surface) deleteCookie(c *http.Cookie) error {
	cm := s.cookieManager()
	if cm == 0 {
		return platform.ErrUnsupported
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	hr := comCall(cm, cmDeleteCookiesWithDomainAndPath, uintptr(unsafe.Pointer(u16(c.Name))),
		uintptr(unsafe.Pointer(u16(c.Domain))), uintptr(unsafe.Pointer(u16(path))))
	if failed(hr) {
		return hresultError("ICoreWebView2CookieManager::DeleteCookiesWithDomainAndPath", hr)
	}
	return nil
}

func (s *surface) DeleteCookie(c *http.Cookie, done func(error)) {
	if c == nil {
		done(nil)
		return
	}
	cc := *c
	s.webCall(done, func(claim func() bool) {
		err := s.deleteCookie(&cc)
		if claim() {
			done(err)
		}
	})
}

func (s *surface) ClearCookies(domain string, done func(error)) {
	s.webCall(done, func(claim func() bool) {
		cm := s.cookieManager()
		if cm == 0 {
			if claim() {
				done(platform.ErrUnsupported)
			}
			return
		}
		if domain == "" {
			hr := comCall(cm, cmDeleteAllCookies)
			if claim() {
				if failed(hr) {
					done(hresultError("ICoreWebView2CookieManager::DeleteAllCookies", hr))
				} else {
					done(nil)
				}
			}
			return
		}
		s.getCookies("", claim, done, func(list uintptr) {
			eachCookie(list, func(c uintptr) {
				if cookieInDomain(comString(c, ckGetDomain), domain) {
					comCall(cm, cmDeleteCookie, c)
				}
			})
			done(nil)
		})
	})
}

// ---------------------------------------------------------------------------
// WebData

func (s *surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	s.webCall(done, func(claim func() bool) {
		mask := browsingDataKinds(kinds)
		if mask == 0 {
			if claim() {
				done(nil)
			}
			return
		}
		var profile2 uintptr
		if wv := queryInterface(s.webview, &iidICoreWebView2_13); wv != 0 {
			if p := comObject(wv, wv13GetProfile); p != 0 {
				profile2 = queryInterface(p, &iidICoreWebView2Profile2)
				release(p)
			}
			release(wv)
		}
		if profile2 == 0 {
			if claim() {
				done(platform.ErrUnsupported)
			}
			return
		}
		defer release(profile2)
		hr := withHandler(func(hr, _ uintptr) {
			if !claim() {
				return
			}
			if failed(hr) {
				done(hresultError("ICoreWebView2Profile2::ClearBrowsingData", hr))
				return
			}
			done(nil)
		}, func(h uintptr) uintptr { return comCall(profile2, prof2ClearBrowsingData, uintptr(mask), h) })
		if failed(hr) && claim() {
			done(hresultError("ICoreWebView2Profile2::ClearBrowsingData", hr))
		}
	})
}

// ---------------------------------------------------------------------------
// WebPDF, WebSnapshot

func (s *surface) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	fail := func(err error) { done(nil, err) }
	s.webCall(fail, func(claim func() bool) {
		wv7 := queryInterface(s.webview, &iidICoreWebView2_7)
		if wv7 == 0 {
			if claim() {
				fail(platform.ErrUnsupported)
			}
			return
		}
		defer release(wv7)
		f, err := os.CreateTemp("", "tango-*.pdf")
		if err != nil {
			if claim() {
				fail(err)
			}
			return
		}
		path := f.Name()
		f.Close()
		settings := s.printSettings(opts) // 0: the runtime's defaults
		defer release(settings)
		hr := withHandler(func(hr, ok uintptr) {
			data, rerr := os.ReadFile(path)
			_ = os.Remove(path)
			if !claim() {
				return
			}
			switch {
			case failed(hr):
				fail(hresultError("ICoreWebView2_7::PrintToPdf", hr))
			case uint32(ok) == 0:
				fail(errors.New("tango: PrintToPdf failed"))
			case rerr != nil:
				fail(rerr)
			default:
				done(data, nil)
			}
		}, func(h uintptr) uintptr {
			return comCall(wv7, wv7PrintToPdf, uintptr(unsafe.Pointer(u16(path))), settings, h)
		})
		if failed(hr) {
			_ = os.Remove(path)
			if claim() {
				fail(hresultError("ICoreWebView2_7::PrintToPdf", hr))
			}
		}
	})
}

// printSettings creates ICoreWebView2PrintSettings for opts, or 0.
func (s *surface) printSettings(opts platform.PDFOptions) uintptr {
	env6 := queryInterface(s.b.env, &iidICoreWebView2Environment6)
	if env6 == 0 {
		return 0
	}
	defer release(env6)
	ps := comObject(env6, env6CreatePrintSettings)
	if ps == 0 {
		return 0
	}
	if opts.Landscape {
		comCall(ps, psPutOrientation, printLandscape)
	}
	comCall(ps, psPutShouldPrintBackgrounds, boolArg(!opts.NoBackground))
	if doubleArgs {
		if opts.PageWidth > 0 {
			comCall(ps, psPutPageWidth, uintptr(math.Float64bits(opts.PageWidth)))
		}
		if opts.PageHeight > 0 {
			comCall(ps, psPutPageHeight, uintptr(math.Float64bits(opts.PageHeight)))
		}
	}
	return ps
}

func (s *surface) Screenshot(done func([]byte, error)) {
	fail := func(err error) { done(nil, err) }
	s.webCall(fail, func(claim func() bool) {
		stream := memStream(nil)
		if stream == 0 {
			if claim() {
				fail(errors.New("tango: SHCreateMemStream failed"))
			}
			return
		}
		hr := withHandler(func(hr, _ uintptr) {
			defer release(stream)
			if !claim() {
				return
			}
			if failed(hr) {
				fail(hresultError("ICoreWebView2::CapturePreview", hr))
				return
			}
			comCall(stream, streamSeek, 0, 0, 0) // STREAM_SEEK_SET to 0
			done(readStream(stream), nil)
		}, func(h uintptr) uintptr {
			return comCall(s.webview, wvCapturePreview, capturePreviewPNG, stream, h)
		})
		if failed(hr) {
			release(stream)
			if claim() {
				fail(hresultError("ICoreWebView2::CapturePreview", hr))
			}
		}
	})
}

// ---------------------------------------------------------------------------
// WebFinder (ICoreWebView2Find: WebView2 Runtime 1.0.3405+ / SDK
// ICoreWebView2_28; older runtimes report ErrUnsupported)

func (s *surface) finder() uintptr {
	if s.web.find == 0 && s.webview != 0 {
		if wv := queryInterface(s.webview, &iidICoreWebView2_28); wv != 0 {
			s.web.find = comObject(wv, wv28GetFind)
			release(wv)
		}
	}
	return s.web.find
}

func findCounts(f uintptr) (count, active int) {
	var c, a int32
	comCall(f, findGetMatchCount, uintptr(unsafe.Pointer(&c)))
	if failed(comCall(f, findGetActiveMatchIndex, uintptr(unsafe.Pointer(&a)))) {
		a = -1
	}
	return int(c), int(a)
}

func (s *surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	fail := func(err error) { done(platform.FindResult{}, err) }
	s.webCall(fail, func(claim func() bool) {
		f := s.finder()
		env15 := queryInterface(s.b.env, &iidICoreWebView2Environment15)
		if f == 0 || env15 == 0 {
			release(env15)
			if claim() {
				fail(platform.ErrUnsupported)
			}
			return
		}
		fo := comObject(env15, env15CreateFindOptions)
		release(env15)
		if fo == 0 {
			if claim() {
				fail(platform.ErrUnsupported)
			}
			return
		}
		defer release(fo)
		comCall(fo, foPutFindTerm, uintptr(unsafe.Pointer(u16(text))))
		comCall(fo, foPutIsCaseSensitive, boolArg(opts.MatchCase))
		comCall(fo, foPutShouldHighlightAllMatches, 1)
		comCall(fo, foPutSuppressDefaultFindDialog, 1)
		comCall(f, findStop) // a new session starts at the first match
		s.web.findOn, s.web.findNoWrap, s.web.findActive = false, opts.NoWrap, 0
		hr := withHandler(func(hr, _ uintptr) {
			if !claim() {
				return
			}
			if failed(hr) {
				fail(hresultError("ICoreWebView2Find::Start", hr))
				return
			}
			count, active := findCounts(f)
			r := findResult(count, active)
			if opts.Backward && count > 1 {
				comCall(f, findFindPrevious) // wraps to the last match
				r.Active = count
			}
			s.web.findOn, s.web.findActive = true, r.Active
			done(r, nil)
		}, func(h uintptr) uintptr { return comCall(f, findStart, fo, h) })
		if failed(hr) && claim() {
			fail(hresultError("ICoreWebView2Find::Start", hr))
		}
	})
}

func (s *surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	if s.canvas != nil {
		done(platform.FindResult{}, platform.ErrUnsupported)
		return
	}
	if !s.web.findOn || s.web.find == 0 || s.closed {
		done(platform.FindResult{}, nil)
		return
	}
	f := s.web.find
	count, active := findCounts(f)
	if active < 1 {
		active = s.web.findActive
	}
	next, move := nextFindIndex(count, active, backward, s.web.findNoWrap)
	if count <= 0 {
		done(platform.FindResult{}, nil)
		return
	}
	if !move {
		done(platform.FindResult{Matches: count, Active: active, Found: false}, nil)
		return
	}
	index := findFindNext
	if backward {
		index = findFindPrevious
	}
	if hr := comCall(f, index); failed(hr) {
		done(platform.FindResult{}, hresultError("ICoreWebView2Find::FindNext", hr))
		return
	}
	s.web.findActive = next
	done(platform.FindResult{Matches: count, Active: next, Found: true}, nil)
}

func (s *surface) StopFind() {
	if s.web.find != 0 && !s.closed {
		comCall(s.web.find, findStop)
	}
	s.web.findOn, s.web.findActive = false, 0
}
