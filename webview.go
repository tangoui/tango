package tango

// Advanced WebView features (contract round 4, platform/webview_ext.go):
// page permission requests, downloads, cookies and website data, HTTP
// authentication, find in page, user agent and extra request headers,
// new-window requests, PDF and PNG capture, and a navigation policy.
//
// Decisions (permission, download destination, authentication, new
// window, navigation) are made by handlers: the surface's own handler
// first, then the app's, then the built-in default. Handlers run on the
// main thread while the page waits; keep them short (on desktop they may
// show a dialog — the dialog functions keep the UI running while they
// wait).
//
// Controls that return data (Find, Cookies, PrintToPDF, ...) block the
// caller until the platform answers; they report ErrUnsupported where the
// platform cannot do it and ErrSurfaceClosed after the surface closed.
// Call them from a goroutine other than the main thread on Android, where
// the main thread cannot wait (they return ErrNestedWait there).

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// ErrSurfaceClosed is returned by WebView controls called after the
// surface closed.
var ErrSurfaceClosed = errors.New("tango: surface is closed")

// WebPermission names a feature a page asks for.
type WebPermission = platform.WebPermission

const (
	WebPermCamera        = platform.WebPermCamera
	WebPermMicrophone    = platform.WebPermMicrophone
	WebPermGeolocation   = platform.WebPermGeolocation
	WebPermNotifications = platform.WebPermNotifications
	WebPermClipboardRead = platform.WebPermClipboardRead
	WebPermMIDI          = platform.WebPermMIDI
	WebPermScreen        = platform.WebPermScreen
	WebPermMotion        = platform.WebPermMotion
	WebPermStorage       = platform.WebPermStorage
	WebPermMedia         = platform.WebPermMedia
	WebPermOther         = platform.WebPermOther
)

// PermissionRequest is a page asking for one or more features at once
// (camera and microphone for one getUserMedia call).
type PermissionRequest = platform.PermissionRequest

// PermissionDecision answers a PermissionRequest. PermissionDefault
// means "no opinion": the next handler, then the policy decides.
type PermissionDecision = platform.PermissionDecision

const (
	PermissionDefault = platform.PermissionDefault
	PermissionPrompt  = platform.PermissionPrompt // ask the user (the platform's prompt where it has one)
	PermissionAllow   = platform.PermissionAllow
	PermissionDeny    = platform.PermissionDeny
)

// NavigationRequest is a main-frame navigation about to start.
type NavigationRequest = platform.NavigationRequest

// NewWindowRequest is the page asking for a new window (window.open,
// target=_blank).
type NewWindowRequest = platform.NewWindowRequest

// NewWindowAction is what to do with a NewWindowRequest.
type NewWindowAction int

const (
	// NewWindowDefault: the next handler decides; finally the default —
	// pages of the app's own origin (the asset origin or the origin the
	// surface was navigated to) open in a new surface, http(s) and mailto
	// URLs elsewhere open in the system browser, anything else is denied.
	NewWindowDefault NewWindowAction = iota
	NewWindowBrowser                 // open the URL with the system's default handler
	NewWindowSurface                 // open it in a new surface with the opener's WebView options
	NewWindowCurrent                 // load it in the requesting surface
	NewWindowDeny                    // do nothing (the handler may have handled it itself)
)

// AuthChallenge is an HTTP authentication challenge (Basic, Digest, ...).
type AuthChallenge = platform.AuthChallenge

// AuthResponse answers an AuthChallenge. The zero value (AuthDefault)
// leaves it to the next handler, then to the platform.
type AuthResponse = platform.AuthResponse

type AuthAction = platform.AuthAction

const (
	AuthDefault     = platform.AuthDefault
	AuthCredentials = platform.AuthCredentials
	AuthCancel      = platform.AuthCancel
)

// DownloadRequest describes a download the page started.
type DownloadRequest = platform.DownloadRequest

// DownloadEvent reports progress or the end of an accepted download.
type DownloadEvent = platform.DownloadEvent

type DownloadState = platform.DownloadState

const (
	DownloadProgress = platform.DownloadProgress
	DownloadDone     = platform.DownloadDone
	DownloadFailed   = platform.DownloadFailed
	DownloadCanceled = platform.DownloadCanceled
)

// FindOptions configures Find.
type FindOptions = platform.FindOptions

// FindResult reports a find-in-page search: Matches is -1 where the
// platform does not count, Active is the 1-based highlighted match (0
// unknown).
type FindResult = platform.FindResult

// BrowsingData selects kinds of website data for ClearBrowsingData.
type BrowsingData = platform.BrowsingData

const (
	DataCookies = platform.DataCookies
	DataCache   = platform.DataCache
	DataStorage = platform.DataStorage
	DataAll     = platform.DataAll
)

// PDFOptions configures PrintToPDF.
type PDFOptions = platform.PDFOptions

// HeaderScope selects the requests extra headers are added to.
type HeaderScope int

const (
	// HeadersAssets adds the headers to requests for the app's assets
	// (the handler in Options.Assets sees them). Every platform.
	HeadersAssets HeaderScope = iota
	// HeadersAll adds them to every request of the WebView, every origin.
	// Only where the platform can (Windows); ErrUnsupported elsewhere.
	HeadersAll
)

// WebViewOptions are WebView settings of a new surface
// (SurfaceOptions.WebView).
type WebViewOptions struct {
	// UserAgent replaces the platform's User-Agent; "" keeps it.
	UserAgent string
	// Headers are extra request headers for the requests HeaderScope
	// selects.
	Headers     http.Header
	HeaderScope HeaderScope
}

// ---------------------------------------------------------------------------
// state

// appWeb is the app's WebView state, guarded by App.mu.
type appWeb struct {
	onPermission func(*Surface, PermissionRequest) PermissionDecision
	policy       map[WebPermission]PermissionDecision
	onDownload   func(*Surface, DownloadRequest) string
	downloadDir  string
	onDownloadEv []func(*Surface, DownloadEvent)
	onAuth       func(*Surface, AuthChallenge) AuthResponse
	onNewWindow  func(*Surface, NewWindowRequest) NewWindowAction
	onNavigate   func(*Surface, NavigationRequest) bool
}

// surfaceWeb is a surface's WebView state. Handlers are guarded by
// Surface.mu, headers by hmu (ServeAsset runs on any thread), the rest is
// main-thread only.
type surfaceWeb struct {
	onPermission func(PermissionRequest) PermissionDecision
	onDownload   func(DownloadRequest) string
	onDownloadEv []func(DownloadEvent)
	onAuth       func(AuthChallenge) AuthResponse
	onNewWindow  func(NewWindowRequest) NewWindowAction
	onNavigate   func(NavigationRequest) bool

	hmu         sync.Mutex
	headers     http.Header
	headerScope HeaderScope

	opts      SurfaceOptions // creation options, for NewWindowSurface
	userAgent string
	coreNav   string // URL the core itself is navigating to; allowed once
	// pageOrigin is the origin of the page last loaded (SurfaceLoaded),
	// the source of its downloads and of permission requests that name no
	// origin. Until the first load, the creation URL's.
	pageOrigin string
}

func (s *Surface) clearWebHandlers() { // s.mu held
	s.web.onPermission, s.web.onDownload, s.web.onDownloadEv = nil, nil, nil
	s.web.onAuth, s.web.onNewWindow, s.web.onNavigate = nil, nil, nil
}

// createNative creates the native surface with the creation extras and
// WebView options, through the richest interface the backend has. Main
// thread.
func (a *App) createNative(s *Surface, opts SurfaceOptions, po platform.SurfaceOptions, ex platform.SurfaceExtras) (platform.Surface, error) {
	wv := opts.WebView
	s.web.opts = opts
	s.web.userAgent = wv.UserAgent
	s.web.coreNav = po.URL
	s.web.pageOrigin = originOf(po.URL)
	s.setHeaders(wv.Headers, wv.HeaderScope)
	web := platform.WebOptions{UserAgent: wv.UserAgent}
	if wv.HeaderScope == HeadersAll && len(wv.Headers) > 0 {
		web.Headers = wv.Headers.Clone()
	}
	hasWeb := web.UserAgent != "" || len(web.Headers) > 0
	if c, ok := a.b.(platform.WebCreator); ok && hasWeb {
		return c.NewSurfaceWeb(s.id, po, ex, web)
	}
	var ns platform.Surface
	var err error
	if c, ok := a.b.(platform.CreatorEx); ok {
		ns, err = c.NewSurfaceEx(s.id, po, ex)
	} else {
		ns, err = a.b.NewSurface(s.id, po)
	}
	if err != nil || !hasWeb {
		return ns, err
	}
	// Best effort: may miss the first request.
	if u, ok := ns.(platform.WebUserAgent); ok && web.UserAgent != "" {
		u.SetUserAgent(web.UserAgent)
	}
	if h, ok := ns.(platform.WebHeaders); ok && len(web.Headers) > 0 {
		h.SetExtraHeaders(web.Headers)
	}
	return ns, nil
}

func (s *Surface) setHeaders(h http.Header, scope HeaderScope) {
	s.web.hmu.Lock()
	s.web.headers, s.web.headerScope = h.Clone(), scope
	s.web.hmu.Unlock()
}

// assetRequest adds the surface's extra headers to an asset request. Any
// goroutine.
func (a *App) assetRequest(id platform.SurfaceID, r *http.Request) *http.Request {
	if id == 0 {
		return r
	}
	s := a.surface(id)
	if s == nil {
		return r
	}
	s.web.hmu.Lock()
	h := s.web.headers
	s.web.hmu.Unlock()
	if len(h) == 0 {
		return r
	}
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header[http.CanonicalHeaderKey(k)] = slices.Clone(v)
	}
	return r
}

// ---------------------------------------------------------------------------
// App-level handlers

// SetPermissionHandler sets the app's handler for page permission
// requests of surfaces without their own handler (or whose handler
// returned PermissionDefault). nil removes it.
func (a *App) SetPermissionHandler(fn func(s *Surface, req PermissionRequest) PermissionDecision) {
	a.mu.Lock()
	a.web.onPermission = fn
	a.mu.Unlock()
}

// SetPermissionPolicy sets the decision for kind when no handler decides;
// kind "" sets it for every kind without its own policy. Without a policy
// the platform asks the user (PermissionPrompt). A request naming several
// kinds is denied if one is denied, allowed if all are allowed, and asks
// the user otherwise.
//
// The policy applies to the app's own pages only (the asset origin and
// the origin the app navigated the surface to). Requests of any other
// origin that no handler decides are denied; a permission handler that
// checks PermissionRequest.Origin can allow them.
func (a *App) SetPermissionPolicy(kind WebPermission, d PermissionDecision) {
	a.mu.Lock()
	if a.web.policy == nil {
		a.web.policy = map[WebPermission]PermissionDecision{}
	}
	if d == PermissionDefault {
		delete(a.web.policy, kind)
	} else {
		a.web.policy[kind] = d
	}
	a.mu.Unlock()
}

// SetDownloadHandler sets the app's handler choosing where downloads of
// surfaces without their own handler are saved. It returns an absolute
// path (a relative one is taken relative to the download directory), or
// "" to cancel. Missing directories are created; an existing file is
// replaced. nil removes it.
//
// Without a handler, downloads started by the app's own pages (the page
// shown is of the asset origin or the origin the app navigated the
// surface to) go to DownloadPath(req); downloads started by any other
// page are canceled. A handler decides for every page; to save them all
// as before, return a.DownloadPath(req).
func (a *App) SetDownloadHandler(fn func(s *Surface, req DownloadRequest) string) {
	a.mu.Lock()
	a.web.onDownload = fn
	a.mu.Unlock()
}

// SetDownloadDir sets the directory of DownloadPath; "" restores the
// default, the user's Downloads folder (the temporary directory where
// there is none).
func (a *App) SetDownloadDir(dir string) {
	a.mu.Lock()
	a.web.downloadDir = dir
	a.mu.Unlock()
}

// DownloadDir returns the directory DownloadPath uses.
func (a *App) DownloadDir() string {
	a.mu.Lock()
	dir := a.web.downloadDir
	a.mu.Unlock()
	if dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		d := filepath.Join(home, "Downloads")
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return os.TempDir()
}

// DownloadPath is the default destination of a download: the suggested
// file name (or the last URL path segment, or "download") in DownloadDir,
// with " (1)", " (2)", ... added before the extension if a file of that
// name exists.
func (a *App) DownloadPath(req DownloadRequest) string {
	return uniquePath(filepath.Join(a.DownloadDir(), downloadName(req)))
}

// OnDownloadEvent registers fn for progress and completion of every
// download of every surface.
func (a *App) OnDownloadEvent(fn func(s *Surface, ev DownloadEvent)) {
	a.mu.Lock()
	a.web.onDownloadEv = append(a.web.onDownloadEv, fn)
	a.mu.Unlock()
}

// SetAuthHandler sets the app's handler for HTTP authentication
// challenges of surfaces without their own handler (or whose handler
// returned AuthDefault). nil removes it.
func (a *App) SetAuthHandler(fn func(s *Surface, ch AuthChallenge) AuthResponse) {
	a.mu.Lock()
	a.web.onAuth = fn
	a.mu.Unlock()
}

// SetNewWindowHandler sets the app's handler for new-window requests of
// surfaces without their own handler (or whose handler returned
// NewWindowDefault). nil removes it.
func (a *App) SetNewWindowHandler(fn func(s *Surface, req NewWindowRequest) NewWindowAction) {
	a.mu.Lock()
	a.web.onNewWindow = fn
	a.mu.Unlock()
}

// SetNavigationPolicy sets the app's navigation policy: fn is asked before
// main-frame navigations of every surface (after the surface's own
// policy) and returning false cancels the navigation. Navigations the app
// starts itself (SurfaceOptions.URL, Navigate) are not asked about. nil
// removes it.
//
// Without any navigation policy (neither the app's nor the surface's) the
// default applies: a navigation to an http(s) URL outside the app's own
// origins (Surface trust: the asset origin and the origin the app
// navigated the surface to) is canceled and the URL is opened in the
// system browser; everything else proceeds. A policy, once set, decides
// alone: returning true loads the URL in the surface, as before this
// default existed.
func (a *App) SetNavigationPolicy(fn func(s *Surface, nav NavigationRequest) bool) {
	a.mu.Lock()
	a.web.onNavigate = fn
	a.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Surface-level handlers

// SetPermissionHandler sets the surface's handler for page permission
// requests; PermissionDefault defers to the app's handler and policy. nil
// removes it.
func (s *Surface) SetPermissionHandler(fn func(req PermissionRequest) PermissionDecision) {
	s.mu.Lock()
	s.web.onPermission = fn
	s.mu.Unlock()
}

// SetDownloadHandler sets the surface's handler choosing where its
// downloads are saved (see App.SetDownloadHandler); it replaces the app's
// handler for this surface. nil removes it.
func (s *Surface) SetDownloadHandler(fn func(req DownloadRequest) string) {
	s.mu.Lock()
	s.web.onDownload = fn
	s.mu.Unlock()
}

// OnDownloadEvent registers fn for progress and completion of the
// surface's downloads.
func (s *Surface) OnDownloadEvent(fn func(ev DownloadEvent)) {
	s.mu.Lock()
	s.web.onDownloadEv = append(s.web.onDownloadEv, fn)
	s.mu.Unlock()
}

// SetAuthHandler sets the surface's handler for HTTP authentication
// challenges; AuthDefault defers to the app's handler. nil removes it.
func (s *Surface) SetAuthHandler(fn func(ch AuthChallenge) AuthResponse) {
	s.mu.Lock()
	s.web.onAuth = fn
	s.mu.Unlock()
}

// SetNewWindowHandler sets the surface's handler for new-window requests;
// NewWindowDefault defers to the app's handler and the default. nil
// removes it.
func (s *Surface) SetNewWindowHandler(fn func(req NewWindowRequest) NewWindowAction) {
	s.mu.Lock()
	s.web.onNewWindow = fn
	s.mu.Unlock()
}

// SetNavigationPolicy sets the surface's navigation policy (see
// App.SetNavigationPolicy); both must allow a navigation. nil removes it.
func (s *Surface) SetNavigationPolicy(fn func(nav NavigationRequest) bool) {
	s.mu.Lock()
	s.web.onNavigate = fn
	s.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Controls

// webAsync runs an asynchronous capability call of the native surface and
// waits for its result.
func webAsync[C, T any](s *Surface, call func(c C, done func(T, error))) (T, error) {
	type res struct {
		v   T
		err error
	}
	var zero T
	if s.closed.Load() {
		return zero, ErrSurfaceClosed
	}
	r, err := TryAwaitAsync(s.app, func(done func(res)) {
		if s.closed.Load() || s.ns == nil {
			done(res{err: ErrSurfaceClosed})
			return
		}
		c, ok := s.ns.(C)
		if !ok {
			done(res{err: ErrUnsupported})
			return
		}
		call(c, func(v T, err error) { done(res{v, err}) })
	})
	if err != nil {
		return zero, err
	}
	return r.v, r.err
}

// webAsyncErr is webAsync for calls without a result.
func webAsyncErr[C any](s *Surface, call func(c C, done func(error))) error {
	_, err := webAsync(s, func(c C, done func(struct{}, error)) {
		call(c, func(err error) { done(struct{}{}, err) })
	})
	return err
}

// Find searches the page for text and highlights the first match (the
// last with Backward).
func (s *Surface) Find(text string, opts FindOptions) (FindResult, error) {
	if text == "" {
		return FindResult{}, s.StopFind()
	}
	return webAsync(s, func(f platform.WebFinder, done func(FindResult, error)) { f.Find(text, opts, done) })
}

// FindNext highlights the next match of the current search.
func (s *Surface) FindNext() (FindResult, error) {
	return webAsync(s, func(f platform.WebFinder, done func(FindResult, error)) { f.FindNext(false, done) })
}

// FindPrevious highlights the previous match of the current search.
func (s *Surface) FindPrevious() (FindResult, error) {
	return webAsync(s, func(f platform.WebFinder, done func(FindResult, error)) { f.FindNext(true, done) })
}

// StopFind ends the search and clears its highlight.
func (s *Surface) StopFind() error {
	return capDo(s, platform.WebFinder.StopFind)
}

// Cookies returns the cookies that would be sent to rawURL, or every
// cookie when rawURL is "" (ErrUnsupported on Android, which cannot list
// them). The cookie store is shared by all surfaces of the app.
func (s *Surface) Cookies(rawURL string) ([]*http.Cookie, error) {
	var u *url.URL
	if rawURL != "" {
		p, err := url.Parse(rawURL)
		if err != nil || p.Host == "" {
			return nil, fmt.Errorf("tango: bad cookie URL %q", rawURL)
		}
		u = p
	}
	cs, err := webAsync(s, func(c platform.WebCookies, done func([]*http.Cookie, error)) { c.Cookies(rawURL, done) })
	if err != nil || u == nil {
		return cs, err
	}
	out := cs[:0:0]
	for _, c := range cs {
		if cookieMatches(c, u) {
			out = append(out, c)
		}
	}
	return out, nil
}

// SetCookie adds or replaces a cookie. c.Domain must be set ("example.com"
// for a host-only cookie, ".example.com" to include subdomains); Path
// defaults to "/".
func (s *Surface) SetCookie(c *http.Cookie) error {
	if c == nil || c.Name == "" || c.Domain == "" {
		return errors.New("tango: SetCookie needs a Name and a Domain")
	}
	cc := *c
	if cc.Path == "" {
		cc.Path = "/"
	}
	return webAsyncErr(s, func(w platform.WebCookies, done func(error)) { w.SetCookie(&cc, done) })
}

// DeleteCookie deletes the cookie with c's Name, Domain and Path (Path
// defaults to "/").
func (s *Surface) DeleteCookie(c *http.Cookie) error {
	if c == nil || c.Name == "" || c.Domain == "" {
		return errors.New("tango: DeleteCookie needs a Name and a Domain")
	}
	cc := *c
	if cc.Path == "" {
		cc.Path = "/"
	}
	return webAsyncErr(s, func(w platform.WebCookies, done func(error)) { w.DeleteCookie(&cc, done) })
}

// ClearCookies deletes the cookies of domain and its subdomains, or every
// cookie when domain is "".
func (s *Surface) ClearCookies(domain string) error {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	return webAsyncErr(s, func(w platform.WebCookies, done func(error)) { w.ClearCookies(domain, done) })
}

// ClearBrowsingData clears website data of every site: cookies, caches,
// storage (localStorage, IndexedDB, service workers, ...).
func (s *Surface) ClearBrowsingData(kinds BrowsingData) error {
	if kinds == 0 {
		return nil
	}
	return webAsyncErr(s, func(w platform.WebData, done func(error)) { w.ClearBrowsingData(kinds, done) })
}

// SetUserAgent changes the User-Agent of later requests; "" restores the
// platform's.
func (s *Surface) SetUserAgent(ua string) error {
	return capDo(s, func(w platform.WebUserAgent) {
		s.web.userAgent = ua
		w.SetUserAgent(ua)
	})
}

// UserAgent returns the User-Agent last set (SurfaceOptions.WebView or
// SetUserAgent); "" means the platform's.
func (s *Surface) UserAgent() (ua string) {
	s.do(func(platform.Surface) { ua = s.web.userAgent })
	return
}

// SetExtraHeaders sets headers added to requests of the given scope; nil
// removes them. HeadersAll is ErrUnsupported where the platform cannot
// change every request; the headers then still go to asset requests.
func (s *Surface) SetExtraHeaders(h http.Header, scope HeaderScope) error {
	if s.closed.Load() {
		return nil
	}
	s.setHeaders(h, scope)
	var err error
	s.do(func(ns platform.Surface) {
		w, ok := ns.(platform.WebHeaders)
		switch {
		case ok && scope == HeadersAll:
			w.SetExtraHeaders(h.Clone())
		case ok:
			w.SetExtraHeaders(nil)
		case scope == HeadersAll:
			err = ErrUnsupported
		}
	})
	return err
}

// PrintToPDF renders the page as a PDF document.
func (s *Surface) PrintToPDF(opts PDFOptions) ([]byte, error) {
	return webAsync(s, func(w platform.WebPDF, done func([]byte, error)) { w.PrintToPDF(opts, done) })
}

// Screenshot captures the visible part of the page as a PNG image.
func (s *Surface) Screenshot() ([]byte, error) {
	return webAsync(s, func(w platform.WebSnapshot, done func([]byte, error)) { w.Screenshot(done) })
}

// CancelDownload cancels a download of this surface (DownloadEvent.ID);
// a DownloadCanceled event follows. Unknown ids are ignored.
func (s *Surface) CancelDownload(id uint64) error {
	return capDo(s, func(w platform.WebDownloads) { w.CancelDownload(id) })
}

// cookieMatches reports whether c would be sent to u (RFC 6265 domain and
// path matching, Secure).
func cookieMatches(c *http.Cookie, u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	d := strings.ToLower(c.Domain)
	if d != "" {
		d = strings.TrimPrefix(d, ".")
		if host != d && !strings.HasSuffix(host, "."+d) {
			return false
		}
	}
	if c.Secure && !strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "wss") && host != "localhost" {
		return false
	}
	p := c.Path
	if p == "" || p == "/" {
		return true
	}
	up := u.EscapedPath()
	if up == "" {
		up = "/"
	}
	return up == p || strings.HasPrefix(up, strings.TrimSuffix(p, "/")+"/")
}
