//go:build ios

package ios

/*
#include <stdlib.h>
#include "webview_ext.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Advanced WebView features (internal/platform/webview_ext.go), native half
// in webview_ext.m. WebHeaders is not implemented: WKWebView can add
// headers to the requests the app loads itself but not to subresources or
// page navigations.

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

var errSurfaceClosed = errors.New("tango/ios: surface is closed")

// Pending asynchronous surface calls by token. Main thread only.
var (
	webErrCalls    = map[uint64]func(error){}
	webBytesCalls  = map[uint64]func([]byte, error){}
	webCookieCalls = map[uint64]func([]*http.Cookie, error){}
	webFindCalls   = map[uint64]func(platform.FindResult, error){}
	nextWebTok     uint64
)

func webTok() C.uint64_t {
	nextWebTok++
	return C.uint64_t(nextWebTok)
}

// webErr maps a TGW_* result code to an error.
func webErr(code C.int, msg *C.char) error {
	switch code {
	case C.TGW_OK:
		return nil
	case C.TGW_UNSUPPORTED:
		return platform.ErrUnsupported
	}
	if m := C.GoString(msg); m != "" {
		return errors.New(m)
	}
	return errors.New("tango/ios: WebView operation failed")
}

// webHost returns the core's WebHost, or nil (fake hosts).
func webHost(id C.uint64_t) (*backend, platform.WebHost) {
	b := current.Load()
	if b == nil || !b.ready.Load() {
		return nil, nil
	}
	w, _ := b.h.(platform.WebHost)
	return b, w
}

// ---- WebCreator ----

// NewSurfaceWeb sets the User-Agent before the first request. Extra
// headers are ignored (no WebHeaders on iOS).
func (b *backend) NewSurfaceWeb(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	return b.newSurfaceUA(id, opts, web.UserAgent)
}

// ---- WebUserAgent ----

func (s *surface) SetUserAgent(ua string) {
	if s.closed {
		return
	}
	u, free := cstring(ua)
	defer free()
	C.tg_web_set_user_agent(s.sid(), u)
}

// ---- WebFinder ----

func (s *surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	if s.closed {
		done(platform.FindResult{}, errSurfaceClosed)
		return
	}
	tok := webTok()
	webFindCalls[uint64(tok)] = done
	t, free := cstring(text)
	defer free()
	C.tg_web_find(s.sid(), tok, t, cbool(opts.MatchCase), cbool(opts.Backward), cbool(opts.NoWrap))
}

func (s *surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	if s.closed {
		done(platform.FindResult{}, nil)
		return
	}
	tok := webTok()
	webFindCalls[uint64(tok)] = done
	C.tg_web_find_next(s.sid(), tok, cbool(backward))
}

func (s *surface) StopFind() {
	if !s.closed {
		C.tg_web_find_stop(s.sid())
	}
}

//export tangoIOSWebFound
func tangoIOSWebFound(tok C.uint64_t, code C.int, msg *C.char, found, matches C.int) {
	fn := webFindCalls[uint64(tok)]
	delete(webFindCalls, uint64(tok))
	if fn == nil {
		return
	}
	if err := webErr(code, msg); err != nil {
		fn(platform.FindResult{}, err)
		return
	}
	fn(platform.FindResult{Found: found != 0, Matches: int(matches)}, nil)
}

// ---- WebCookies ----

// Cookies always returns every cookie of the store; the core filters by
// url.
func (s *surface) Cookies(url string, done func([]*http.Cookie, error)) {
	if s.closed {
		done(nil, errSurfaceClosed)
		return
	}
	tok := webTok()
	webCookieCalls[uint64(tok)] = done
	C.tg_web_cookies(s.sid(), tok)
}

func (s *surface) SetCookie(c *http.Cookie, done func(error)) {
	if s.closed {
		done(errSurfaceClosed)
		return
	}
	if c == nil || c.Domain == "" {
		done(errors.New("tango/ios: cookie needs a Domain"))
		return
	}
	if c.MaxAge < 0 {
		s.DeleteCookie(c, done)
		return
	}
	var exp float64
	switch {
	case c.MaxAge > 0:
		exp = float64(time.Now().Unix() + int64(c.MaxAge))
	case !c.Expires.IsZero():
		exp = float64(c.Expires.UnixNano()) / 1e9
	}
	ss := C.int(0)
	switch c.SameSite {
	case http.SameSiteLaxMode:
		ss = 1
	case http.SameSiteStrictMode:
		ss = 2
	}
	tok := webTok()
	webErrCalls[uint64(tok)] = done
	n, f1 := cstring(c.Name)
	defer f1()
	v, f2 := cstring(c.Value)
	defer f2()
	d, f3 := cstring(c.Domain)
	defer f3()
	p, f4 := cstring(c.Path)
	defer f4()
	C.tg_web_set_cookie(s.sid(), tok, n, v, d, p, C.double(exp), cbool(c.Secure), cbool(c.HttpOnly), ss)
}

func (s *surface) DeleteCookie(c *http.Cookie, done func(error)) {
	if s.closed {
		done(errSurfaceClosed)
		return
	}
	tok := webTok()
	webErrCalls[uint64(tok)] = done
	n, f1 := cstring(c.Name)
	defer f1()
	d, f2 := cstring(c.Domain)
	defer f2()
	p, f3 := cstring(c.Path)
	defer f3()
	C.tg_web_delete_cookie(s.sid(), tok, n, d, p)
}

func (s *surface) ClearCookies(domain string, done func(error)) {
	if s.closed {
		done(errSurfaceClosed)
		return
	}
	tok := webTok()
	webErrCalls[uint64(tok)] = done
	d, free := cstring(domain)
	defer free()
	C.tg_web_clear_cookies(s.sid(), tok, d)
}

// nativeCookie is the JSON form webview_ext.m reports cookies in.
type nativeCookie struct {
	Name     string  `json:"n"`
	Value    string  `json:"v"`
	Domain   string  `json:"d"`
	Path     string  `json:"p"`
	Expires  float64 `json:"e"` // Unix seconds, 0 = session
	Secure   bool    `json:"s"`
	HttpOnly bool    `json:"h"`
	SameSite int     `json:"ss"` // 0 unset, 1 lax, 2 strict
}

//export tangoIOSWebCookies
func tangoIOSWebCookies(tok C.uint64_t, code C.int, msg *C.char, js *C.char, n C.int) {
	fn := webCookieCalls[uint64(tok)]
	delete(webCookieCalls, uint64(tok))
	if fn == nil {
		return
	}
	if err := webErr(code, msg); err != nil {
		fn(nil, err)
		return
	}
	var list []nativeCookie
	if err := json.Unmarshal(C.GoBytes(unsafe.Pointer(js), n), &list); err != nil {
		fn(nil, err)
		return
	}
	out := make([]*http.Cookie, 0, len(list))
	for _, nc := range list {
		c := &http.Cookie{
			Name: nc.Name, Value: nc.Value, Domain: nc.Domain, Path: nc.Path,
			Secure: nc.Secure, HttpOnly: nc.HttpOnly,
		}
		if nc.Expires != 0 {
			sec, frac := math.Modf(nc.Expires)
			c.Expires = time.Unix(int64(sec), int64(frac*1e9))
		}
		switch nc.SameSite {
		case 1:
			c.SameSite = http.SameSiteLaxMode
		case 2:
			c.SameSite = http.SameSiteStrictMode
		}
		out = append(out, c)
	}
	fn(out, nil)
}

// ---- WebData ----

func (s *surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	if s.closed {
		done(errSurfaceClosed)
		return
	}
	tok := webTok()
	webErrCalls[uint64(tok)] = done
	C.tg_web_clear_data(s.sid(), tok, C.uint32_t(kinds&platform.DataAll))
}

//export tangoIOSWebDone
func tangoIOSWebDone(tok C.uint64_t, code C.int, msg *C.char) {
	fn := webErrCalls[uint64(tok)]
	delete(webErrCalls, uint64(tok))
	if fn != nil {
		fn(webErr(code, msg))
	}
}

// ---- WebPDF / WebSnapshot ----

// PrintToPDF renders the whole page as one PDF page (WebKit's
// createPDF); Landscape and page sizes are ignored, NoBackground maps to
// a transparent background on iOS 17+.
func (s *surface) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	if s.closed {
		done(nil, errSurfaceClosed)
		return
	}
	tok := webTok()
	webBytesCalls[uint64(tok)] = done
	C.tg_web_pdf(s.sid(), tok, cbool(opts.NoBackground))
}

func (s *surface) Screenshot(done func([]byte, error)) {
	if s.closed {
		done(nil, errSurfaceClosed)
		return
	}
	tok := webTok()
	webBytesCalls[uint64(tok)] = done
	C.tg_web_snapshot(s.sid(), tok)
}

//export tangoIOSWebBytes
func tangoIOSWebBytes(tok C.uint64_t, code C.int, msg *C.char, p unsafe.Pointer, n C.int) {
	fn := webBytesCalls[uint64(tok)]
	delete(webBytesCalls, uint64(tok))
	if fn == nil {
		return
	}
	if err := webErr(code, msg); err != nil {
		fn(nil, err)
		return
	}
	fn(C.GoBytes(p, n), nil)
}

// ---- WebDownloads ----

// CancelDownload is per app, not per surface: download ids are unique.
func (s *surface) CancelDownload(id uint64) {
	C.tg_web_download_cancel(C.uint64_t(id))
}

// ---- Callbacks from the WebKit delegates ----

//export tangoIOSPermission
func tangoIOSPermission(sid, tok C.uint64_t, origin *C.char, kinds C.int) {
	answered := false
	done := func(d platform.PermissionDecision) {
		if answered {
			return
		}
		answered = true
		C.tg_web_permission_done(tok, C.int(d))
	}
	_, w := webHost(sid)
	if w == nil {
		done(platform.PermissionPrompt)
		return
	}
	req := platform.PermissionRequest{Origin: C.GoString(origin)}
	if kinds&C.TGW_PERM_CAMERA != 0 {
		req.Kinds = append(req.Kinds, platform.WebPermCamera)
	}
	if kinds&C.TGW_PERM_MICROPHONE != 0 {
		req.Kinds = append(req.Kinds, platform.WebPermMicrophone)
	}
	if kinds&C.TGW_PERM_MOTION != 0 {
		req.Kinds = append(req.Kinds, platform.WebPermMotion)
	}
	if len(req.Kinds) == 0 {
		req.Kinds = []platform.WebPermission{platform.WebPermOther}
	}
	w.OnPermissionRequest(platform.SurfaceID(sid), req, done)
}

//export tangoIOSShouldNavigate
func tangoIOSShouldNavigate(sid C.uint64_t, url *C.char, user C.int) C.int {
	_, w := webHost(sid)
	if w == nil {
		return 1
	}
	if w.ShouldNavigate(platform.SurfaceID(sid), platform.NavigationRequest{URL: C.GoString(url), UserInitiated: user != 0}) {
		return 1
	}
	return 0
}

//export tangoIOSNewWindow
func tangoIOSNewWindow(sid C.uint64_t, url *C.char, user, w, h C.int) {
	if _, wh := webHost(sid); wh != nil {
		wh.OnNewWindow(platform.SurfaceID(sid), platform.NewWindowRequest{
			URL: C.GoString(url), UserInitiated: user != 0, Width: int(w), Height: int(h),
		})
	}
}

//export tangoIOSAuth
func tangoIOSAuth(sid, tok C.uint64_t, url, host *C.char, port C.int, realm, scheme *C.char, proxy, retry C.int) {
	answered := false
	done := func(r platform.AuthResponse) {
		if answered {
			return
		}
		answered = true
		u, f1 := cstring(r.User)
		defer f1()
		p, f2 := cstring(r.Password)
		defer f2()
		C.tg_web_auth_done(tok, C.int(r.Action), u, p)
	}
	_, w := webHost(sid)
	if w == nil {
		done(platform.AuthResponse{Action: platform.AuthDefault})
		return
	}
	w.OnAuthChallenge(platform.SurfaceID(sid), platform.AuthChallenge{
		URL: C.GoString(url), Host: C.GoString(host), Port: int(port), Realm: C.GoString(realm),
		Scheme: C.GoString(scheme), Proxy: proxy != 0, Retry: int(retry),
	}, done)
}

//export tangoIOSDownloadStart
func tangoIOSDownloadStart(sid, did C.uint64_t, url, filename, mime *C.char, size C.int64_t) {
	answered := false
	done := func(path string) {
		if answered {
			return
		}
		answered = true
		p, free := cstring(path)
		defer free()
		C.tg_web_download_dest(did, p)
	}
	_, w := webHost(sid)
	if w == nil {
		done("")
		return
	}
	w.OnDownloadStarted(platform.SurfaceID(sid), platform.DownloadRequest{
		ID: uint64(did), URL: C.GoString(url), Filename: C.GoString(filename), MIME: C.GoString(mime), Size: int64(size),
	}, done)
}

// tangoIOSDownloadEvent is delivered even after the surface closed: every
// accepted download ends with one event.
//
//export tangoIOSDownloadEvent
func tangoIOSDownloadEvent(sid, did C.uint64_t, state C.int, received, total C.int64_t, path, errMsg *C.char) {
	_, w := webHost(sid)
	if w == nil {
		return
	}
	w.OnDownloadEvent(platform.SurfaceID(sid), platform.DownloadEvent{
		ID: uint64(did), State: platform.DownloadState(state), Received: int64(received), Total: int64(total),
		Path: C.GoString(path), Error: C.GoString(errMsg),
	})
}
