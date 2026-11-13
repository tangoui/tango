//go:build android

package android

/*
#include <stdlib.h>
#include "jni_android.h"
#include "webview_ext_android.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Advanced WebView features (internal/platform/webview_ext.go) over one JNI
// method, org.tango.host.WebExt.call(page, cb, op, json), in the style of
// Mobile.call: synchronous ops return their result ("!code message" on
// failure), asynchronous ones (cb > 0) answer on the main thread through
// WebExt.nativeDone. Page requests (permissions, navigation, new windows,
// authentication, downloads) arrive through the other WebExt native
// methods; Go answers them by request id with "permissionDone",
// "authDone" and "downloadStart".
//
// Not implemented: WebPDF (Android has no public API that writes a PDF
// without the system print UI) and WebHeaders (WebView can only add
// headers to loadUrl, not to every request).

var (
	_ platform.WebCreator   = (*Backend)(nil)
	_ platform.WebFinder    = (*surface)(nil)
	_ platform.WebCookies   = (*surface)(nil)
	_ platform.WebData      = (*surface)(nil)
	_ platform.WebUserAgent = (*surface)(nil)
	_ platform.WebSnapshot  = (*surface)(nil)
	_ platform.WebDownloads = (*surface)(nil)
)

const webExtClassName = "org/tango/host/WebExt"

var (
	webExtClass C.jclass
	mWebCall    = &jmethod{cls: &webExtClass, static: true, name: "call",
		sig: "(Lorg/tango/host/Page;ILjava/lang/String;Ljava/lang/String;)Ljava/lang/String;"}
)

// lookupWebExt resolves WebExt.call. Main thread, from nativeInit. Without
// the class (an older copy of mobile/android) the features fail with
// errJava and the page requests never arrive.
func lookupWebExt(e env) error {
	n := C.CString(webExtClassName)
	webExtClass = C.tg_find_class(e.p, n)
	C.free(unsafe.Pointer(n))
	if webExtClass == 0 {
		return errJavaClass(webExtClassName)
	}
	name, sig := C.CString(mWebCall.name), C.CString(mWebCall.sig)
	mWebCall.id = C.tg_method(e.p, webExtClass, name, sig, 1)
	C.free(unsafe.Pointer(name))
	C.free(unsafe.Pointer(sig))
	if mWebCall.id == nil {
		webExtClass = 0
		return errJavaClass(webExtClassName + ".call")
	}
	return nil
}

// webCall runs op on the calling thread. page may be 0 for ops that do
// not need a page.
func webCall(page C.jobject, cb int, op string, arg any) (string, error) {
	if webExtClass == 0 {
		return "", errJava
	}
	a := "{}"
	if arg != nil {
		a = args(arg)
	}
	var res string
	var err error
	if !withEnv(func(e env) {
		var o C.jobject
		o, err = e.callStaticObject(mWebCall, jvObj(page), jvInt(cb), jvObj(e.str(op)), jvObj(e.str(a)))
		if err == nil {
			res = e.gostr(C.jstring(o))
		}
	}) && err == nil {
		err = errJava
	}
	if err != nil {
		return "", err
	}
	return splitResult(res)
}

// Callbacks of asynchronous ops.
var webCBs struct {
	sync.Mutex
	next int
	m    map[int]func(res string, data []byte, err error)
}

func addWebCB(fn func(string, []byte, error)) int {
	webCBs.Lock()
	defer webCBs.Unlock()
	if webCBs.m == nil {
		webCBs.m = map[int]func(string, []byte, error){}
	}
	webCBs.next++
	if webCBs.next <= 0 {
		webCBs.next = 1
	}
	webCBs.m[webCBs.next] = fn
	return webCBs.next
}

func takeWebCB(cb int) func(string, []byte, error) {
	webCBs.Lock()
	defer webCBs.Unlock()
	fn := webCBs.m[cb]
	delete(webCBs.m, cb)
	return fn
}

// webAsync starts an asynchronous op on page; fn runs once, on the main
// thread.
func webAsync(page C.jobject, op string, arg any, fn func(string, []byte, error)) {
	cb := addWebCB(fn)
	if _, err := webCall(page, cb, op, arg); err != nil {
		if f := takeWebCB(cb); f != nil {
			f("", nil, err)
		}
	}
}

//export Java_org_tango_host_WebExt_nativeDone
func Java_org_tango_host_WebExt_nativeDone(penv *C.JNIEnv, cls C.jclass, cb C.jint, res C.jstring, data C.jbyteArray, errMsg C.jstring) {
	e := env{penv}
	fn := takeWebCB(int(cb))
	if fn == nil {
		return
	}
	var b []byte
	if data != 0 {
		n := C.tg_array_length(penv, C.jarray(data))
		b = make([]byte, int(n))
		if n > 0 {
			C.tg_get_bytes(penv, data, n, unsafe.Pointer(&b[0]))
		}
	}
	fn(e.gostr(res), b, javaErr(e.gostr(errMsg)))
}

// webHost is the core's WebHost, or nil (no app yet, or a fake host).
func webHost() platform.WebHost {
	wh, _ := backend.host().(platform.WebHost)
	return wh
}

func jbool(b bool) C.jboolean {
	if b {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

// ---- page requests (main thread) ----

//export Java_org_tango_host_WebExt_nativePermission
func Java_org_tango_host_WebExt_nativePermission(penv *C.JNIEnv, cls C.jclass, page C.jlong, id C.jint, origin C.jstring, kinds C.jobjectArray) {
	e := env{penv}
	req := platform.PermissionRequest{Origin: e.gostr(origin)}
	for _, k := range e.gostrs(kinds) {
		req.Kinds = append(req.Kinds, platform.WebPermission(k))
	}
	answered := false
	done := func(d platform.PermissionDecision) {
		if answered {
			return
		}
		answered = true
		webCall(0, 0, "permissionDone", map[string]int{"id": int(id), "decision": int(d)})
	}
	wh := webHost()
	if wh == nil {
		done(platform.PermissionDefault)
		return
	}
	wh.OnPermissionRequest(platform.SurfaceID(page), req, done)
}

//export Java_org_tango_host_WebExt_nativeShouldNavigate
func Java_org_tango_host_WebExt_nativeShouldNavigate(penv *C.JNIEnv, cls C.jclass, page C.jlong, u C.jstring, gesture C.jboolean) C.jboolean {
	wh := webHost()
	if wh == nil {
		return C.JNI_TRUE
	}
	e := env{penv}
	return jbool(wh.ShouldNavigate(platform.SurfaceID(page),
		platform.NavigationRequest{URL: e.gostr(u), UserInitiated: gesture != 0}))
}

//export Java_org_tango_host_WebExt_nativeNewWindow
func Java_org_tango_host_WebExt_nativeNewWindow(penv *C.JNIEnv, cls C.jclass, page C.jlong, u C.jstring, gesture C.jboolean) C.jboolean {
	wh := webHost()
	if wh == nil {
		return C.JNI_FALSE
	}
	e := env{penv}
	wh.OnNewWindow(platform.SurfaceID(page), platform.NewWindowRequest{URL: e.gostr(u), UserInitiated: gesture != 0})
	return C.JNI_TRUE
}

//export Java_org_tango_host_WebExt_nativeAuth
func Java_org_tango_host_WebExt_nativeAuth(penv *C.JNIEnv, cls C.jclass, page C.jlong, id C.jint, host, realm C.jstring, retry C.jint) C.jboolean {
	wh := webHost()
	if wh == nil {
		return C.JNI_FALSE
	}
	e := env{penv}
	ch := platform.AuthChallenge{Host: e.gostr(host), Realm: e.gostr(realm), Retry: int(retry)}
	// HttpAuthHandler names neither the URL, port nor scheme. host may
	// carry a port.
	if h, p, ok := strings.Cut(ch.Host, ":"); ok && !strings.Contains(p, ":") {
		if n, err := parsePort(p); err == nil {
			ch.Host, ch.Port = h, n
		}
	}
	answered := false
	wh.OnAuthChallenge(platform.SurfaceID(page), ch, func(r platform.AuthResponse) {
		if answered {
			return
		}
		answered = true
		webCall(0, 0, "authDone", map[string]any{
			"id": int(id), "action": int(r.Action), "user": r.User, "password": r.Password,
		})
	})
	return C.JNI_TRUE
}

func parsePort(s string) (int, error) {
	n := 0
	if s == "" || len(s) > 5 {
		return 0, errors.New("bad port")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("bad port")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

//export Java_org_tango_host_WebExt_nativeDownloadStarted
func Java_org_tango_host_WebExt_nativeDownloadStarted(penv *C.JNIEnv, cls C.jclass, page C.jlong, id C.jlong, u, filename, mime C.jstring, size C.jlong) C.jboolean {
	wh := webHost()
	if wh == nil {
		return C.JNI_FALSE
	}
	e := env{penv}
	d := platform.DownloadRequest{
		ID: uint64(id), URL: e.gostr(u), Filename: e.gostr(filename), MIME: e.gostr(mime), Size: int64(size),
	}
	answered := false
	wh.OnDownloadStarted(platform.SurfaceID(page), d, func(path string) {
		if answered {
			return
		}
		answered = true
		webCall(0, 0, "downloadStart", map[string]any{"id": d.ID, "path": path})
	})
	return C.JNI_TRUE
}

//export Java_org_tango_host_WebExt_nativeDownloadEvent
func Java_org_tango_host_WebExt_nativeDownloadEvent(penv *C.JNIEnv, cls C.jclass, page C.jlong, id C.jlong, state C.jint, received, total C.jlong, path, errMsg C.jstring) {
	wh := webHost()
	if wh == nil {
		return
	}
	e := env{penv}
	wh.OnDownloadEvent(platform.SurfaceID(page), platform.DownloadEvent{
		ID: uint64(id), State: platform.DownloadState(state), Received: int64(received), Total: int64(total),
		Path: e.gostr(path), Error: e.gostr(errMsg),
	})
}

// ---- creation ----

// NewSurfaceWeb creates the page with its user agent in place before the
// first request. Extra headers are not supported (see WebHeaders above).
func (b *Backend) NewSurfaceWeb(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	if web.UserAgent != "" {
		// The next Page created takes it (WebExt.state); cleared again in
		// case creation failed before a Page existed.
		webCall(0, 0, "presetUserAgent", map[string]string{"ua": web.UserAgent})
		defer webCall(0, 0, "presetUserAgent", map[string]string{"ua": ""})
	}
	if cx, ok := any(b).(platform.CreatorEx); ok {
		return cx.NewSurfaceEx(id, opts, ex)
	}
	return b.NewSurface(id, opts)
}

// ---- surface controls ----

var errPageClosed = errors.New("tango/android: surface closed")

// pageAsync runs an asynchronous op on s's page.
func (s *surface) pageAsync(op string, arg any, fn func(string, []byte, error)) {
	if s.closed || s.page == 0 {
		fn("", nil, errPageClosed)
		return
	}
	webAsync(s.page, op, arg, fn)
}

type javaFind struct {
	Matches int  `json:"matches"`
	Active  int  `json:"active"`
	Found   bool `json:"found"`
}

func findResult(done func(platform.FindResult, error)) func(string, []byte, error) {
	return func(res string, _ []byte, err error) {
		if err != nil {
			done(platform.FindResult{}, err)
			return
		}
		var j javaFind
		if err := json.Unmarshal([]byte(res), &j); err != nil {
			done(platform.FindResult{}, errors.New("tango/android: bad find result "+res))
			return
		}
		done(platform.FindResult{Matches: j.Matches, Active: j.Active, Found: j.Found}, nil)
	}
}

// Find uses WebView.findAllAsync, which always ignores case
// (opts.MatchCase is not honoured).
func (s *surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	s.pageAsync("find", map[string]any{"text": text, "backward": opts.Backward, "noWrap": opts.NoWrap}, findResult(done))
}

func (s *surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	s.pageAsync("findNext", map[string]any{"backward": backward}, findResult(done))
}

func (s *surface) StopFind() {
	if !s.closed && s.page != 0 {
		webCall(s.page, 0, "stopFind", nil)
	}
}

// Cookies reads CookieManager.getCookie(url), which gives only names and
// values: Domain is url's host and Path "/". All cookies ("") cannot be
// listed.
func (s *surface) Cookies(rawURL string, done func([]*http.Cookie, error)) {
	if rawURL == "" {
		done(nil, platform.ErrUnsupported)
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		done(nil, errors.New("tango/android: bad cookie URL "+rawURL))
		return
	}
	res, err := webCall(0, 0, "getCookie", map[string]string{"url": rawURL})
	if err != nil {
		done(nil, err)
		return
	}
	var out []*http.Cookie
	for _, part := range strings.Split(res, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		out = append(out, &http.Cookie{Name: name, Value: value, Domain: u.Hostname(), Path: "/"})
	}
	done(out, nil)
}

// cookieLine is c as a Set-Cookie value and the URL to set it for. A
// Domain without a leading dot makes a host-only cookie (no Domain
// attribute).
func cookieLine(c *http.Cookie, secure bool) (string, string, error) {
	if c == nil || c.Domain == "" || c.Name == "" {
		return "", "", errors.New("tango/android: cookie needs Name and Domain")
	}
	host := strings.TrimPrefix(c.Domain, ".")
	cc := *c
	if !strings.HasPrefix(c.Domain, ".") {
		cc.Domain = ""
	} else {
		cc.Domain = host
	}
	if cc.Path == "" {
		cc.Path = "/"
	}
	cc.Secure = cc.Secure || secure
	line := cc.String()
	if line == "" {
		return "", "", errors.New("tango/android: invalid cookie " + c.Name)
	}
	scheme := "http"
	if cc.Secure {
		scheme = "https"
	}
	return line, scheme + "://" + host + cc.Path, nil
}

func (s *surface) SetCookie(c *http.Cookie, done func(error)) {
	line, u, err := cookieLine(c, false)
	if err != nil {
		done(err)
		return
	}
	webAsync(0, "setCookie", map[string]string{"url": u, "value": line}, func(_ string, _ []byte, err error) { done(err) })
}

// DeleteCookie sets the cookie expired (CookieManager cannot delete one).
func (s *surface) DeleteCookie(c *http.Cookie, done func(error)) {
	if c == nil {
		done(errors.New("tango/android: nil cookie"))
		return
	}
	dc := &http.Cookie{Name: c.Name, Domain: c.Domain, Path: c.Path, Expires: time.Unix(1, 0), MaxAge: -1}
	line, u, err := cookieLine(dc, true)
	if err != nil {
		done(err)
		return
	}
	webAsync(0, "setCookie", map[string]string{"url": u, "value": line}, func(_ string, _ []byte, err error) { done(err) })
}

// ClearCookies with a domain reaches only the cookies CookieManager sends
// to http(s)://domain/ (it cannot enumerate cookies): those of subdomains
// or with other paths stay.
func (s *surface) ClearCookies(domain string, done func(error)) {
	webAsync(0, "clearCookies", map[string]string{"domain": domain}, func(_ string, _ []byte, err error) { done(err) })
}

func (s *surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	s.pageAsync("clearData", map[string]int{"kinds": int(kinds)}, func(_ string, _ []byte, err error) { done(err) })
}

func (s *surface) SetUserAgent(ua string) {
	if !s.closed && s.page != 0 {
		webCall(s.page, 0, "userAgent", map[string]string{"ua": ua})
	}
}

func (s *surface) Screenshot(done func([]byte, error)) {
	s.pageAsync("screenshot", nil, func(_ string, png []byte, err error) {
		if err == nil && len(png) == 0 {
			err = errJava
		}
		done(png, err)
	})
}

func (s *surface) CancelDownload(id uint64) {
	webCall(0, 0, "cancelDownload", map[string]uint64{"id": id})
}
