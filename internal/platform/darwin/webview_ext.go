//go:build darwin && !ios

package darwin

// Advanced WebView features (contract round 4, platform/webview_ext.go) on
// WKWebView, following Apple's WebKit documentation:
//
//   - WKUIDelegate: requestMediaCapturePermissionForOrigin (macOS 12+) for
//     camera / microphone permission requests; createWebViewWithConfiguration
//     for window.open and target=_blank (always answered with nil, the core
//     decides what to do with the URL).
//   - WKNavigationDelegate: decidePolicyForNavigationAction (navigation
//     policy; downloads requested with the download attribute),
//     decidePolicyForNavigationResponse (responses that cannot be shown or
//     are attachments become downloads), didBecomeDownload, and
//     didReceiveAuthenticationChallenge for HTTP authentication.
//   - WKDownloadDelegate (macOS 11.3+): destination, completion, failure;
//     progress is polled from WKDownload.progress while downloads run.
//   - WKHTTPCookieStore and WKWebsiteDataStore for cookies and website data.
//   - findString:withConfiguration:completionHandler: (macOS 11+). WebKit
//     does not count matches through public API, so Matches is counted in
//     the page's rendered text (innerText) and Active follows the search
//     order.
//   - createPDFWithConfiguration (macOS 11+, the whole page as one PDF page)
//     and takeSnapshotWithConfiguration (PNG of the visible area).
//
// Extra headers on every request are not possible with WKWebView, so
// surfaces do not implement platform.WebHeaders.

import (
	"context"
	"errors"
	"math"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

const (
	wkNavigationActionPolicyCancel   = 0
	wkNavigationActionPolicyAllow    = 1
	wkNavigationActionPolicyDownload = 2

	wkNavigationTypeLinkActivated = 0
	wkNavigationTypeFormSubmitted = 1

	wkPermissionDecisionPrompt = 0
	wkPermissionDecisionGrant  = 1
	wkPermissionDecisionDeny   = 2

	wkMediaCaptureTypeCamera              = 0
	wkMediaCaptureTypeMicrophone          = 1
	wkMediaCaptureTypeCameraAndMicrophone = 2

	nsURLSessionAuthChallengeUseCredential          = 0
	nsURLSessionAuthChallengePerformDefaultHandling = 1
	nsURLSessionAuthChallengeCancel                 = 2
	nsURLCredentialPersistenceForSession            = 1

	webKitErrorFrameLoadInterrupted = 102
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

var errNoWebView = errors.New("tango/darwin: not a WebView surface")

var (
	blockCopyFn, blockReleaseFn uintptr
	msgTimer                    func(cls id, s objc.SEL, interval float64, repeats bool, blk objc.Block) id
	msgDateSince1970            func(cls id, s objc.SEL, t float64) id
	webExtOnce                  sync.Once
)

func loadWebExt() {
	webExtOnce.Do(func() {
		loadExt()
		blockCopyFn = mustDlsym(purego.RTLD_DEFAULT, "_Block_copy")
		blockReleaseFn = mustDlsym(purego.RTLD_DEFAULT, "_Block_release")
		purego.RegisterFunc(&msgTimer, msgSendAddr)
		purego.RegisterFunc(&msgDateSince1970, msgSendAddr)
	})
}

// heldBlock is a block the system handed us that we answer later.
type heldBlock uintptr

func holdBlock(blk id) heldBlock {
	r, _, _ := purego.SyscallN(blockCopyFn, uintptr(blk))
	return heldBlock(r)
}

// call invokes the block once and releases it.
func (h *heldBlock) call(args ...uintptr) {
	if *h == 0 {
		return
	}
	b := uintptr(*h)
	*h = 0
	invokeBlock(b, args...)
	purego.SyscallN(blockReleaseFn, b)
}

// webExt is a WebView surface's round-4 state. Main thread.
type webExt struct {
	find        string
	findOpts    platform.FindOptions
	findCount   int
	findActive  int
	downloading bool // a response was just turned into a download
}

var webExts = map[*surface]*webExt{}

func (s *surface) wx() *webExt {
	w := webExts[s]
	if w == nil {
		w = &webExt{}
		webExts[s] = w
	}
	return w
}

// pendingWeb carries NewSurfaceWeb's options into newSurface. Main thread.
var pendingWeb *platform.WebOptions

// NewSurfaceWeb implements platform.WebCreator: the user agent is set
// before the first request. Extra headers are not supported.
func (b *backend) NewSurfaceWeb(sid platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	pendingWeb = &web
	defer func() { pendingWeb = nil }()
	return b.newSurface(sid, opts, &ex)
}

// initWebExt wires the WebView's UI delegate and applies creation options.
// Called from newSurface inside withPool, before the first navigation.
func (s *surface) initWebExt() {
	loadWebExt()
	send(s.web, "setUIDelegate:", uintptr(s.delegate))
	if w := pendingWeb; w != nil && w.UserAgent != "" {
		send(s.web, "setCustomUserAgent:", uintptr(nsString(w.UserAgent)))
	}
}

// destroyWebExt runs from destroy.
func (s *surface) destroyWebExt() {
	if s.web != 0 {
		send(s.web, "setUIDelegate:", 0)
	}
	delete(webExts, s)
	for _, d := range activeDownloads() {
		if d.s == s {
			if d.obj != 0 {
				send(d.obj, "cancel:", 0)
			}
			d.end(platform.DownloadEvent{State: platform.DownloadCanceled})
		}
	}
}

func (s *surface) webHost() platform.WebHost {
	h, _ := s.b.h.(platform.WebHost)
	return h
}

// ignoreLoadError reports whether a navigation failure only means that the
// response became a download (WebKit reports "Frame load interrupted").
func (s *surface) ignoreLoadError(err id) bool {
	w := webExts[s]
	if w == nil || !w.downloading {
		return false
	}
	w.downloading = false
	return sendInt(err, "code") == webKitErrorFrameLoadInterrupted
}

// ---------------------------------------------------------------------------
// delegate methods

func webDelegateMethods(of func(self id) *surface) []objc.MethodDef {
	return []objc.MethodDef{
		// WKNavigationDelegate
		method("webView:decidePolicyForNavigationAction:decisionHandler:", func(self id, _ objc.SEL, web, action, handler id) {
			policy := uintptr(wkNavigationActionPolicyAllow)
			if s := of(self); s != nil {
				policy = s.navigationPolicy(action)
			}
			invokeBlock(uintptr(handler), policy)
		}),
		method("webView:decidePolicyForNavigationResponse:decisionHandler:", func(self id, _ objc.SEL, web, resp, handler id) {
			policy := uintptr(wkNavigationActionPolicyAllow)
			if s := of(self); s != nil {
				policy = s.responsePolicy(resp)
			}
			invokeBlock(uintptr(handler), policy)
		}),
		method("webView:navigationAction:didBecomeDownload:", func(self id, _ objc.SEL, web, action, dl id) {
			if s := of(self); s != nil {
				s.adoptDownload(dl)
			}
		}),
		method("webView:navigationResponse:didBecomeDownload:", func(self id, _ objc.SEL, web, resp, dl id) {
			if s := of(self); s != nil {
				s.adoptDownload(dl)
			}
		}),
		method("webView:didReceiveAuthenticationChallenge:completionHandler:", func(self id, _ objc.SEL, web, ch, handler id) {
			s := of(self)
			if s == nil {
				invokeBlock(uintptr(handler), nsURLSessionAuthChallengePerformDefaultHandling, 0)
				return
			}
			s.authChallenge(ch, handler)
		}),

		// WKUIDelegate
		method("webView:createWebViewWithConfiguration:forNavigationAction:windowFeatures:", func(self id, _ objc.SEL, web, cfg, action, features id) id {
			if s := of(self); s != nil {
				s.newWindow(action, features)
			}
			return 0
		}),
		method("webView:requestMediaCapturePermissionForOrigin:initiatedByFrame:type:decisionHandler:", func(self id, _ objc.SEL, web, origin, frame id, typ int, handler id) {
			s := of(self)
			if s == nil {
				invokeBlock(uintptr(handler), wkPermissionDecisionDeny)
				return
			}
			s.mediaPermission(origin, typ, handler)
		}),

		// WKDownloadDelegate
		method("download:decideDestinationUsingResponse:suggestedFilename:completionHandler:", func(self id, _ objc.SEL, dl, resp, name, handler id) {
			decideDestination(dl, resp, name, handler)
		}),
		method("downloadDidFinish:", func(self id, _ objc.SEL, dl id) {
			if d := downloads[dl]; d != nil {
				d.end(platform.DownloadEvent{State: platform.DownloadDone})
			}
		}),
		method("download:didFailWithError:resumeData:", func(self id, _ objc.SEL, dl, err, resume id) {
			d := downloads[dl]
			if d == nil {
				return
			}
			if err != 0 && sendInt(err, "code") == nsURLErrorCancelled {
				d.end(platform.DownloadEvent{State: platform.DownloadCanceled})
				return
			}
			var msg string
			withPool(func() { msg = goString(send(err, "localizedDescription")) })
			d.end(platform.DownloadEvent{State: platform.DownloadFailed, Error: msg})
		}),
	}
}

// ---- navigation policy and new windows ----

func (s *surface) navigationPolicy(action id) uintptr {
	if respondsTo(action, "shouldPerformDownload") && sendBool(action, "shouldPerformDownload") {
		var u string
		withPool(func() { u = goString(send(send(send(action, "request"), "URL"), "absoluteString")) })
		if strings.HasPrefix(strings.ToLower(u), assetScheme+":") {
			s.startAssetDownload(platform.DownloadRequest{URL: u, Size: -1})
			return wkNavigationActionPolicyCancel
		}
		return wkNavigationActionPolicyDownload
	}
	frame := send(action, "targetFrame")
	if frame == 0 || !sendBool(frame, "isMainFrame") {
		// nil target: a new window (createWebView follows); sub-frames are
		// not subject to the policy.
		return wkNavigationActionPolicyAllow
	}
	h := s.webHost()
	if h == nil {
		return wkNavigationActionPolicyAllow
	}
	var u string
	withPool(func() { u = goString(send(send(send(action, "request"), "URL"), "absoluteString")) })
	t := sendInt(action, "navigationType")
	if h.ShouldNavigate(s.id, platform.NavigationRequest{URL: u, UserInitiated: t == wkNavigationTypeLinkActivated || t == wkNavigationTypeFormSubmitted}) {
		return wkNavigationActionPolicyAllow
	}
	return wkNavigationActionPolicyCancel
}

func (s *surface) newWindow(action, features id) {
	h := s.webHost()
	if h == nil {
		return
	}
	req := platform.NewWindowRequest{}
	withPool(func() {
		req.URL = goString(send(send(send(action, "request"), "URL"), "absoluteString"))
		t := sendInt(action, "navigationType")
		req.UserInitiated = t == wkNavigationTypeLinkActivated || t == wkNavigationTypeFormSubmitted
		if features != 0 {
			if w := send(features, "width"); w != 0 {
				req.Width = sendInt(w, "integerValue")
			}
			if ht := send(features, "height"); ht != 0 {
				req.Height = sendInt(ht, "integerValue")
			}
		}
	})
	if req.URL == "about:blank" {
		req.URL = ""
	}
	h.OnNewWindow(s.id, req)
}

// ---- permissions ----

func (s *surface) mediaPermission(origin id, typ int, handler id) {
	h := s.webHost()
	if h == nil {
		invokeBlock(uintptr(handler), wkPermissionDecisionPrompt)
		return
	}
	req := platform.PermissionRequest{}
	switch typ {
	case wkMediaCaptureTypeCamera:
		req.Kinds = []platform.WebPermission{platform.WebPermCamera}
	case wkMediaCaptureTypeMicrophone:
		req.Kinds = []platform.WebPermission{platform.WebPermMicrophone}
	case wkMediaCaptureTypeCameraAndMicrophone:
		req.Kinds = []platform.WebPermission{platform.WebPermCamera, platform.WebPermMicrophone}
	default:
		req.Kinds = []platform.WebPermission{platform.WebPermOther}
	}
	if origin != 0 {
		withPool(func() {
			req.Origin = formatOrigin(goString(send(origin, "protocol")), goString(send(origin, "host")), sendInt(origin, "port"))
		})
	}
	hb := holdBlock(handler)
	h.OnPermissionRequest(s.id, req, func(d platform.PermissionDecision) {
		v := uintptr(wkPermissionDecisionPrompt)
		switch d {
		case platform.PermissionAllow:
			v = wkPermissionDecisionGrant
		case platform.PermissionDeny:
			v = wkPermissionDecisionDeny
		}
		hb.call(v)
	})
}

// ---- HTTP authentication ----

var authSchemes = map[string]string{
	"NSURLAuthenticationMethodHTTPBasic":  "basic",
	"NSURLAuthenticationMethodHTTPDigest": "digest",
	"NSURLAuthenticationMethodNTLM":       "ntlm",
	"NSURLAuthenticationMethodNegotiate":  "negotiate",
	"NSURLAuthenticationMethodDefault":    "",
	"NSURLAuthenticationMethodHTMLForm":   "",
}

func (s *surface) authChallenge(ch, handler id) {
	var c platform.AuthChallenge
	known := false
	withPool(func() {
		space := send(ch, "protectionSpace")
		method := goString(send(space, "authenticationMethod"))
		c.Scheme, known = authSchemes[method]
		c.Host = goString(send(space, "host"))
		c.Port = sendInt(space, "port")
		c.Realm = goString(send(space, "realm"))
		c.Proxy = sendBool(space, "isProxy")
		c.Retry = sendInt(ch, "previousFailureCount")
		if r := send(ch, "failureResponse"); r != 0 {
			c.URL = goString(send(send(r, "URL"), "absoluteString"))
		}
	})
	h := s.webHost()
	if !known || h == nil {
		// Server trust, client certificates: the platform default.
		invokeBlock(uintptr(handler), nsURLSessionAuthChallengePerformDefaultHandling, 0)
		return
	}
	hb := holdBlock(handler)
	h.OnAuthChallenge(s.id, c, func(r platform.AuthResponse) {
		switch r.Action {
		case platform.AuthCredentials:
			withPool(func() {
				cred := send(class("NSURLCredential"), "credentialWithUser:password:persistence:",
					uintptr(nsString(r.User)), uintptr(nsString(r.Password)), nsURLCredentialPersistenceForSession)
				hb.call(nsURLSessionAuthChallengeUseCredential, uintptr(cred))
			})
		case platform.AuthCancel:
			hb.call(nsURLSessionAuthChallengeCancel, 0)
		default:
			hb.call(nsURLSessionAuthChallengePerformDefaultHandling, 0)
		}
	})
}

// ---- downloads ----
//
// Downloads of http(s) pages are WKDownloads. WebKit cannot turn a
// response of a custom-scheme handler into a WKDownload, so downloads from
// the asset origin cancel the navigation and fetch the asset again
// straight from Host.ServeAsset into the file, on a goroutine.

type download struct {
	obj      id // WKDownload, retained; 0 for an asset download
	s        *surface
	id       uint64
	path     string
	accepted bool
	ended    bool
	received int64
	total    int64

	// asset downloads
	cancel   context.CancelFunc
	got      atomic.Int64
	finished atomic.Bool
	errMsg   atomic.Pointer[string]
}

var (
	downloads      = map[id]*download{}     // by WKDownload
	assetDownloads = map[uint64]*download{} // by id
	nextDownload   uint64
	downloadTimer  id
)

// assetAttachments remembers the Content-Disposition of asset responses
// sent as attachments: WebKit hands custom-scheme responses to the
// navigation delegate as plain NSURLResponse, without headers.
var (
	attachMu         sync.Mutex
	assetAttachments = map[string]string{}
)

// noteAttachment is called by the scheme handler for every asset response.
func noteAttachment(u id, header http.Header) {
	cd := strings.TrimSpace(header.Get("Content-Disposition"))
	if !strings.HasPrefix(strings.ToLower(cd), "attachment") {
		return
	}
	var key string
	withPool(func() { key = goString(send(u, "absoluteString")) })
	attachMu.Lock()
	if len(assetAttachments) > 256 {
		clear(assetAttachments)
	}
	assetAttachments[key] = cd
	attachMu.Unlock()
}

// dispositionName returns the filename parameter of a Content-Disposition.
func dispositionName(cd string) string {
	if _, params, err := mime.ParseMediaType(cd); err == nil {
		return params["filename"]
	}
	return ""
}

// responsePolicy decides whether a navigation response is shown or
// downloaded.
func (s *surface) responsePolicy(resp id) uintptr {
	r := send(resp, "response")
	if r == 0 {
		return wkNavigationActionPolicyAllow
	}
	var key, cd string
	withPool(func() { key = goString(send(send(r, "URL"), "absoluteString")) })
	attachMu.Lock()
	cd, hit := assetAttachments[key]
	delete(assetAttachments, key)
	attachMu.Unlock()
	isAsset := strings.HasPrefix(strings.ToLower(key), assetScheme+":")
	download := hit || !sendBool(resp, "canShowMIMEType")
	if !download && !isAsset && sendBool(r, "isKindOfClass:", uintptr(class("NSHTTPURLResponse"))) && respondsTo(r, "valueForHTTPHeaderField:") {
		withPool(func() { cd = goString(send(r, "valueForHTTPHeaderField:", uintptr(nsString("Content-Disposition")))) })
		download = strings.HasPrefix(strings.ToLower(strings.TrimSpace(cd)), "attachment")
	}
	if !download {
		return wkNavigationActionPolicyAllow
	}
	if sendBool(resp, "isForMainFrame") {
		s.wx().downloading = true
	}
	if isAsset {
		req := platform.DownloadRequest{URL: key, Size: -1}
		withPool(func() {
			req.MIME = goString(send(r, "MIMEType"))
			req.Size = int64(send(r, "expectedContentLength"))
			req.Filename = dispositionName(cd)
			if req.Filename == "" {
				req.Filename = goString(send(r, "suggestedFilename"))
			}
		})
		s.startAssetDownload(req)
		return wkNavigationActionPolicyCancel
	}
	if !hasClass("WKDownload") { // before macOS 11.3
		return wkNavigationActionPolicyCancel
	}
	return wkNavigationActionPolicyDownload
}

func (s *surface) adoptDownload(dl id) {
	if dl == 0 || downloads[dl] != nil {
		return
	}
	nextDownload++
	downloads[retain(dl)] = &download{obj: dl, s: s, id: nextDownload, total: -1}
	send(dl, "setDelegate:", uintptr(s.delegate))
}

func decideDestination(dl, resp, name, handler id) {
	d := downloads[dl]
	var h platform.WebHost
	if d != nil && !d.s.closed {
		h = d.s.webHost()
	}
	if h == nil {
		invokeBlock(uintptr(handler), 0)
		if d != nil {
			d.forget()
		}
		return
	}
	req := platform.DownloadRequest{ID: d.id, Size: -1}
	withPool(func() {
		req.Filename = goString(name)
		if resp != 0 {
			req.MIME = goString(send(resp, "MIMEType"))
			req.Size = int64(send(resp, "expectedContentLength"))
			req.URL = goString(send(send(resp, "URL"), "absoluteString"))
		}
		if req.URL == "" {
			req.URL = goString(send(send(send(dl, "originalRequest"), "URL"), "absoluteString"))
		}
	})
	if req.Size < 0 {
		req.Size = -1
	}
	d.total = req.Size
	hb := holdBlock(handler)
	h.OnDownloadStarted(d.s.id, req, func(path string) {
		if path == "" || d.ended {
			hb.call(0)
			d.forget()
			return
		}
		// WKDownload refuses to overwrite; the contract replaces.
		_ = os.Remove(path)
		d.path, d.accepted = path, true
		withPool(func() { hb.call(uintptr(fileURL(path))) })
		startDownloadTimer()
	})
}

// startAssetDownload asks for a destination and copies the asset there.
func (s *surface) startAssetDownload(req platform.DownloadRequest) {
	h := s.webHost()
	if h == nil {
		return
	}
	if req.Size < 0 {
		req.Size = -1
	}
	nextDownload++
	d := &download{s: s, id: nextDownload, total: req.Size}
	req.ID = d.id
	h.OnDownloadStarted(s.id, req, func(path string) {
		if path == "" || s.closed {
			return
		}
		_ = os.Remove(path)
		f, err := os.Create(path)
		if err != nil {
			d.path, d.accepted = path, true
			assetDownloads[d.id] = d
			d.end(platform.DownloadEvent{State: platform.DownloadFailed, Error: err.Error()})
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		d.path, d.accepted, d.cancel = path, true, cancel
		assetDownloads[d.id] = d
		host, sid := s.b.h, s.id
		go func() {
			msg := copyAsset(ctx, host, sid, req.URL, f, &d.got)
			if cerr := f.Close(); msg == "" && cerr != nil {
				msg = cerr.Error()
			}
			if msg != "" {
				_ = os.Remove(path)
				d.errMsg.Store(&msg)
			}
			d.finished.Store(true)
		}()
		startDownloadTimer()
	})
}

// fileSink is the http.ResponseWriter of an asset download.
type fileSink struct {
	f      *os.File
	h      http.Header
	status int
	got    *atomic.Int64
	err    error
}

func (w *fileSink) Header() http.Header { return w.h }

func (w *fileSink) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *fileSink) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.status/100 != 2 || w.err != nil {
		return len(p), nil
	}
	n, err := w.f.Write(p)
	w.got.Add(int64(n))
	if err != nil {
		w.err = err
	}
	return len(p), nil
}

// copyAsset serves url from the asset handler into f; it returns an error
// message, "" on success.
func copyAsset(ctx context.Context, host platform.Host, sid platform.SurfaceID, url string, f *os.File, got *atomic.Int64) string {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err.Error()
	}
	w := &fileSink{f: f, h: http.Header{}, got: got}
	host.ServeAsset(sid, w, r)
	switch {
	case ctx.Err() != nil:
		return "canceled"
	case w.err != nil:
		return w.err.Error()
	case w.status != 0 && w.status/100 != 2:
		return http.StatusText(w.status)
	}
	return ""
}

// forget drops a download that was never accepted (no events).
func (d *download) forget() {
	d.ended = true
	if d.obj != 0 {
		delete(downloads, d.obj)
		autorelease(d.obj)
	}
}

// end reports the terminal event of a download (once).
func (d *download) end(ev platform.DownloadEvent) {
	if d.ended {
		return
	}
	d.ended = true
	if d.obj != 0 {
		delete(downloads, d.obj)
	} else {
		delete(assetDownloads, d.id)
		if d.cancel != nil {
			d.cancel()
		}
	}
	if d.accepted {
		d.poll()
		ev.ID, ev.Path, ev.Received, ev.Total = d.id, d.path, d.received, d.total
		if ev.State == platform.DownloadDone {
			if fi, err := os.Stat(d.path); err == nil {
				ev.Received = fi.Size()
				if ev.Total < 0 {
					ev.Total = ev.Received
				}
			}
		}
		if h := d.s.webHost(); h != nil {
			h.OnDownloadEvent(d.s.id, ev)
		}
	}
	if d.obj != 0 {
		autorelease(d.obj)
	}
}

// poll reads the download's progress; it reports whether it changed.
func (d *download) poll() bool {
	var got, total int64
	if d.obj == 0 {
		got, total = d.got.Load(), d.total
	} else {
		p := send(d.obj, "progress")
		if p == 0 {
			return false
		}
		got, total = int64(send(p, "completedUnitCount")), int64(send(p, "totalUnitCount"))
		if total <= 0 {
			total = d.total
		}
	}
	changed := got != d.received || total != d.total
	d.received, d.total = got, total
	return changed
}

func (d *download) tick() {
	if d.obj == 0 && d.finished.Load() {
		if msg := d.errMsg.Load(); msg != nil {
			st := platform.DownloadFailed
			if *msg == "canceled" {
				st = platform.DownloadCanceled
			}
			d.end(platform.DownloadEvent{State: st, Error: *msg})
		} else {
			d.end(platform.DownloadEvent{State: platform.DownloadDone})
		}
		return
	}
	if d.poll() {
		if h := d.s.webHost(); h != nil {
			h.OnDownloadEvent(d.s.id, platform.DownloadEvent{ID: d.id, State: platform.DownloadProgress,
				Received: d.received, Total: d.total, Path: d.path})
		}
	}
}

func activeDownloads() []*download {
	var out []*download
	for _, d := range downloads {
		if d.accepted && !d.ended {
			out = append(out, d)
		}
	}
	for _, d := range assetDownloads {
		if d.accepted && !d.ended {
			out = append(out, d)
		}
	}
	return out
}

func startDownloadTimer() {
	if downloadTimer != 0 {
		return
	}
	blk := objc.NewBlock(func(_ objc.Block, t id) {
		ds := activeDownloads()
		for _, d := range ds {
			d.tick()
		}
		if len(ds) == 0 && downloadTimer != 0 {
			send(downloadTimer, "invalidate")
			release(downloadTimer)
			downloadTimer = 0
		}
	})
	downloadTimer = retain(msgTimer(class("NSTimer"), sel("scheduledTimerWithTimeInterval:repeats:block:"), 0.25, true, blk))
	blk.Release()
}

func (s *surface) CancelDownload(dlID uint64) {
	for _, d := range activeDownloads() {
		if d.s == s && d.id == dlID {
			if d.obj != 0 {
				send(d.obj, "cancel:", 0)
			}
			d.end(platform.DownloadEvent{State: platform.DownloadCanceled})
			return
		}
	}
}

// ---- find in page ----

const countJS = `(function(t,cs){var s=document.body?document.body.innerText||"":"";if(!t)return 0;if(!cs){s=s.toLowerCase();t=t.toLowerCase();}var n=0,i=0;while((i=s.indexOf(t,i))!==-1){n++;i+=t.length;}return n;})`

// evalResult evaluates js and calls done with the result object (an
// NSNumber, NSString, ... or 0) on the main thread.
func (s *surface) evalResult(js string, done func(res id, err error)) {
	blk := objc.NewBlock(func(_ objc.Block, res, nsErr id) {
		if nsErr != 0 {
			var msg string
			withPool(func() { msg = goString(send(nsErr, "localizedDescription")) })
			done(0, errors.New(msg))
			return
		}
		done(res, nil)
	})
	withPool(func() {
		send(s.web, "evaluateJavaScript:completionHandler:", uintptr(nsString(js)), uintptr(blk))
	})
	blk.Release()
}

func (s *surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	if s.closed || s.web == 0 {
		done(platform.FindResult{}, errNoWebView)
		return
	}
	if !hasClass("WKFindConfiguration") || !respondsTo(s.web, "findString:withConfiguration:completionHandler:") {
		done(platform.FindResult{}, platform.ErrUnsupported)
		return
	}
	w := s.wx()
	w.find, w.findOpts, w.findCount, w.findActive = text, opts, -1, 0
	s.evalResult(countJS+"("+jsQuote(text)+","+boolJS(opts.MatchCase)+")", func(res id, err error) {
		if err == nil && res != 0 && sendBool(res, "isKindOfClass:", uintptr(class("NSNumber"))) {
			w.findCount = sendInt(res, "integerValue")
		}
		s.findStep(opts.Backward, true, done)
	})
}

func (s *surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	w := webExts[s]
	if s.closed || w == nil || w.find == "" {
		done(platform.FindResult{}, nil)
		return
	}
	s.findStep(backward, false, done)
}

func (s *surface) findStep(backward, first bool, done func(platform.FindResult, error)) {
	w := s.wx()
	if s.closed {
		done(platform.FindResult{}, nil)
		return
	}
	text, opts := w.find, w.findOpts
	var cfg id
	withPool(func() {
		cfg = alloc("WKFindConfiguration")
		send(cfg, "setBackwards:", boolArg(backward))
		send(cfg, "setCaseSensitive:", boolArg(opts.MatchCase))
		send(cfg, "setWraps:", boolArg(!opts.NoWrap))
	})
	blk := objc.NewBlock(func(_ objc.Block, res id) {
		found := res != 0 && sendBool(res, "matchFound")
		r := platform.FindResult{Matches: w.findCount, Found: found}
		if found {
			n := w.findCount
			switch {
			case n <= 0:
				w.findActive = 0
			case first && !backward:
				w.findActive = 1
			case first:
				w.findActive = n
			case backward:
				w.findActive--
				if w.findActive < 1 {
					w.findActive = n
				}
			default:
				w.findActive++
				if w.findActive > n {
					w.findActive = 1
				}
			}
			r.Active = w.findActive
			if r.Matches == 0 {
				r.Matches = -1 // the text count missed what WebKit found
			}
		} else if w.findCount < 0 {
			r.Matches = 0
		}
		done(r, nil)
	})
	withPool(func() {
		send(s.web, "findString:withConfiguration:completionHandler:", uintptr(nsString(text)), uintptr(cfg), uintptr(blk))
	})
	release(cfg)
	blk.Release()
}

func (s *surface) StopFind() {
	if w := webExts[s]; w != nil {
		w.find, w.findCount, w.findActive = "", 0, 0
	}
	if !s.closed && s.web != 0 {
		s.Eval("window.getSelection&&window.getSelection().removeAllRanges()")
	}
}

// ---- cookies and website data ----

func (s *surface) dataStore() id {
	return send(send(s.web, "configuration"), "websiteDataStore")
}

func (s *surface) cookieStore() id { return send(s.dataStore(), "httpCookieStore") }

// allCookies fetches the store's NSHTTPCookie objects (retained in the
// array passed to fn, valid during fn).
func (s *surface) allCookies(fn func(cookies []id)) {
	blk := objc.NewBlock(func(_ objc.Block, arr id) {
		withPool(func() { fn(arrayItems(arr)) })
	})
	withPool(func() { send(s.cookieStore(), "getAllCookies:", uintptr(blk)) })
	blk.Release()
}

func (s *surface) Cookies(_ string, done func([]*http.Cookie, error)) {
	if s.closed || s.web == 0 {
		done(nil, errNoWebView)
		return
	}
	s.allCookies(func(cs []id) {
		out := make([]*http.Cookie, 0, len(cs))
		for _, c := range cs {
			out = append(out, goCookie(c))
		}
		done(out, nil) // the core filters by URL
	})
}

func goCookie(c id) *http.Cookie {
	hc := &http.Cookie{
		Name:     goString(send(c, "name")),
		Value:    goString(send(c, "value")),
		Domain:   goString(send(c, "domain")),
		Path:     goString(send(c, "path")),
		Secure:   sendBool(c, "isSecure"),
		HttpOnly: sendBool(c, "isHTTPOnly"),
	}
	if !sendBool(c, "isSessionOnly") {
		if d := send(c, "expiresDate"); d != 0 {
			t := msgGetFloat(d, sel("timeIntervalSince1970"))
			sec, frac := math.Modf(t)
			hc.Expires = time.Unix(int64(sec), int64(frac*1e9)).UTC()
		}
	}
	if respondsTo(c, "sameSitePolicy") {
		switch strings.ToLower(goString(send(c, "sameSitePolicy"))) {
		case "lax":
			hc.SameSite = http.SameSiteLaxMode
		case "strict":
			hc.SameSite = http.SameSiteStrictMode
		}
	}
	return hc
}

// nsCookie builds an autoreleased NSHTTPCookie. Inside withPool.
func nsCookie(c *http.Cookie) id {
	key := func(name, fallback string) uintptr {
		if p := constPtr(libFoundation, name); p != 0 {
			return p
		}
		return uintptr(nsString(fallback))
	}
	props := send(class("NSMutableDictionary"), "dictionary")
	set := func(k uintptr, v id) { send(props, "setObject:forKey:", uintptr(v), k) }
	set(key("NSHTTPCookieName", "Name"), nsString(c.Name))
	set(key("NSHTTPCookieValue", "Value"), nsString(c.Value))
	set(key("NSHTTPCookieDomain", "Domain"), nsString(c.Domain))
	path := c.Path
	if path == "" {
		path = "/"
	}
	set(key("NSHTTPCookiePath", "Path"), nsString(path))
	if !c.Expires.IsZero() {
		t := float64(c.Expires.UnixNano()) / 1e9
		set(key("NSHTTPCookieExpires", "Expires"), msgDateSince1970(class("NSDate"), sel("dateWithTimeIntervalSince1970:"), t))
	} else if c.MaxAge > 0 {
		set(key("NSHTTPCookieMaximumAge", "Max-Age"), nsString(strconv.Itoa(c.MaxAge)))
	}
	if c.Secure {
		set(key("NSHTTPCookieSecure", "Secure"), nsString("TRUE"))
	}
	if c.HttpOnly {
		set(uintptr(nsString("HttpOnly")), nsString("TRUE")) // no public constant
	}
	switch c.SameSite {
	case http.SameSiteLaxMode:
		set(key("NSHTTPCookieSameSitePolicy", "SameSite"), nsString("lax"))
	case http.SameSiteStrictMode:
		set(key("NSHTTPCookieSameSitePolicy", "SameSite"), nsString("strict"))
	}
	return send(class("NSHTTPCookie"), "cookieWithProperties:", uintptr(props))
}

func (s *surface) SetCookie(c *http.Cookie, done func(error)) {
	if s.closed || s.web == 0 {
		done(errNoWebView)
		return
	}
	var nc id
	withPool(func() { nc = retain(nsCookie(c)) })
	if nc == 0 {
		done(errors.New("tango/darwin: invalid cookie"))
		return
	}
	blk := objc.NewBlock(func(_ objc.Block) { done(nil) })
	withPool(func() { send(s.cookieStore(), "setCookie:completionHandler:", uintptr(nc), uintptr(blk)) })
	blk.Release()
	release(nc)
}

func (s *surface) DeleteCookie(c *http.Cookie, done func(error)) {
	s.deleteCookiesWhere(func(x *http.Cookie) bool {
		return x.Name == c.Name && strings.EqualFold(x.Domain, c.Domain) && (x.Path == c.Path || c.Path == "")
	}, done)
}

func (s *surface) ClearCookies(domain string, done func(error)) {
	s.deleteCookiesWhere(func(x *http.Cookie) bool {
		d := strings.ToLower(strings.TrimPrefix(x.Domain, "."))
		return domain == "" || d == domain || strings.HasSuffix(d, "."+domain)
	}, done)
}

func (s *surface) deleteCookiesWhere(match func(*http.Cookie) bool, done func(error)) {
	if s.closed || s.web == 0 {
		done(errNoWebView)
		return
	}
	s.allCookies(func(cs []id) {
		var del []id
		for _, c := range cs {
			if match(goCookie(c)) {
				del = append(del, retain(c))
			}
		}
		if len(del) == 0 {
			done(nil)
			return
		}
		left := len(del)
		blk := objc.NewBlock(func(_ objc.Block) {
			left--
			if left == 0 {
				done(nil)
			}
		})
		store := s.cookieStore()
		for _, c := range del {
			send(store, "deleteCookie:completionHandler:", uintptr(c), uintptr(blk))
			release(c)
		}
		blk.Release()
	})
}

func (s *surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	if s.closed || s.web == 0 {
		done(errNoWebView)
		return
	}
	var names []string
	if kinds&platform.DataCookies != 0 {
		names = append(names, "WKWebsiteDataTypeCookies")
	}
	if kinds&platform.DataCache != 0 {
		names = append(names, "WKWebsiteDataTypeDiskCache", "WKWebsiteDataTypeMemoryCache",
			"WKWebsiteDataTypeOfflineWebApplicationCache", "WKWebsiteDataTypeFetchCache")
	}
	if kinds&platform.DataStorage != 0 {
		names = append(names, "WKWebsiteDataTypeLocalStorage", "WKWebsiteDataTypeSessionStorage",
			"WKWebsiteDataTypeIndexedDBDatabases", "WKWebsiteDataTypeWebSQLDatabases",
			"WKWebsiteDataTypeServiceWorkerRegistrations", "WKWebsiteDataTypeFileSystem")
	}
	blk := objc.NewBlock(func(_ objc.Block) { done(nil) })
	withPool(func() {
		set := send(class("NSMutableSet"), "set")
		for _, n := range names {
			if p := constPtr(libWebKit, n); p != 0 {
				send(set, "addObject:", p)
			}
		}
		since := send(class("NSDate"), "distantPast")
		send(s.dataStore(), "removeDataOfTypes:modifiedSince:completionHandler:", uintptr(set), uintptr(since), uintptr(blk))
	})
	blk.Release()
}

// ---- user agent ----

func (s *surface) SetUserAgent(ua string) {
	if s.closed || s.web == 0 {
		return
	}
	withPool(func() {
		if ua == "" {
			send(s.web, "setCustomUserAgent:", 0)
			return
		}
		send(s.web, "setCustomUserAgent:", uintptr(nsString(ua)))
	})
}

// ---- PDF and screenshot ----

func (s *surface) PrintToPDF(_ platform.PDFOptions, done func([]byte, error)) {
	if s.closed || s.web == 0 {
		done(nil, errNoWebView)
		return
	}
	if !hasClass("WKPDFConfiguration") || !respondsTo(s.web, "createPDFWithConfiguration:completionHandler:") {
		done(nil, platform.ErrUnsupported)
		return
	}
	blk := objc.NewBlock(func(_ objc.Block, data, nsErr id) {
		if nsErr != 0 || data == 0 {
			done(nil, nsError(nsErr, "PDF creation failed"))
			return
		}
		done(goBytes(data), nil)
	})
	withPool(func() {
		send(s.web, "createPDFWithConfiguration:completionHandler:", 0, uintptr(blk))
	})
	blk.Release()
}

func (s *surface) Screenshot(done func([]byte, error)) {
	if s.closed || s.web == 0 {
		done(nil, errNoWebView)
		return
	}
	if !respondsTo(s.web, "takeSnapshotWithConfiguration:completionHandler:") {
		done(nil, platform.ErrUnsupported)
		return
	}
	blk := objc.NewBlock(func(_ objc.Block, img, nsErr id) {
		if nsErr != 0 || img == 0 {
			done(nil, nsError(nsErr, "snapshot failed"))
			return
		}
		var png []byte
		withPool(func() {
			tiff := send(img, "TIFFRepresentation")
			rep := send(class("NSBitmapImageRep"), "imageRepWithData:", uintptr(tiff))
			if rep == 0 {
				return
			}
			png = goBytes(send(rep, "representationUsingType:properties:", nsBitmapImageFileTypePNG,
				uintptr(send(class("NSDictionary"), "dictionary"))))
		})
		if len(png) == 0 {
			done(nil, errors.New("tango/darwin: PNG encoding failed"))
			return
		}
		done(png, nil)
	})
	withPool(func() {
		send(s.web, "takeSnapshotWithConfiguration:completionHandler:", 0, uintptr(blk))
	})
	blk.Release()
}

// ---- helpers ----

func nsError(nsErr id, fallback string) error {
	if nsErr == 0 {
		return errors.New("tango/darwin: " + fallback)
	}
	var msg string
	withPool(func() { msg = goString(send(nsErr, "localizedDescription")) })
	return errors.New(msg)
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// jsQuote renders s as a JavaScript string literal.
func jsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x2028 || r == 0x2029:
			const hex = "0123456789abcdef"
			b.WriteString(`\u`)
			for sh := 12; sh >= 0; sh -= 4 {
				b.WriteByte(hex[(r>>uint(sh))&0xf])
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
