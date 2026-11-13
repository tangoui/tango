package fake

import (
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// WebView features (platform/webview_ext.go). The fake keeps one cookie
// jar per Backend, records the controls called on each surface and lets
// tests drive the WebHost callbacks with the helpers below. Find counts
// case-(in)sensitive occurrences of the text in Surface.SetPageText.

var (
	_ platform.WebCreator   = (*Backend)(nil)
	_ platform.WebFinder    = (*Surface)(nil)
	_ platform.WebCookies   = (*Surface)(nil)
	_ platform.WebData      = (*Surface)(nil)
	_ platform.WebUserAgent = (*Surface)(nil)
	_ platform.WebHeaders   = (*Surface)(nil)
	_ platform.WebPDF       = (*Surface)(nil)
	_ platform.WebSnapshot  = (*Surface)(nil)
	_ platform.WebDownloads = (*Surface)(nil)
)

// FakePDF and FakePNG are what PrintToPDF and Screenshot return.
var (
	FakePDF = []byte("%PDF-1.4\n% tango fake\n%%EOF\n")
	FakePNG = []byte("\x89PNG\r\n\x1a\nfake")
)

type webState struct {
	mu        sync.Mutex
	web       platform.WebOptions
	created   bool // created through NewSurfaceWeb
	userAgent string
	headers   http.Header
	text      string
	find      string
	findOpts  platform.FindOptions
	active    int
	cleared   []platform.BrowsingData
	canceled  []uint64
}

var webStates sync.Map // *Surface -> *webState

func (s *Surface) webst() *webState {
	if w, ok := webStates.Load(s); ok {
		return w.(*webState)
	}
	w, _ := webStates.LoadOrStore(s, &webState{})
	return w.(*webState)
}

type jar struct {
	mu      sync.Mutex
	cookies []*http.Cookie
	nextDL  uint64
}

var jars sync.Map // *Backend -> *jar

func (b *Backend) jar() *jar {
	if j, ok := jars.Load(b); ok {
		return j.(*jar)
	}
	j, _ := jars.LoadOrStore(b, &jar{})
	return j.(*jar)
}

// NewSurfaceWeb implements platform.WebCreator.
func (b *Backend) NewSurfaceWeb(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, web platform.WebOptions) (platform.Surface, error) {
	ns, err := b.NewSurfaceEx(id, opts, ex)
	if err != nil {
		return nil, err
	}
	w := ns.(*Surface).webst()
	w.mu.Lock()
	w.web, w.created = web, true
	w.userAgent, w.headers = web.UserAgent, web.Headers
	w.mu.Unlock()
	return ns, nil
}

// ---- controls ----

func (s *Surface) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	s.b.mustMain("Find")
	w := s.webst()
	w.mu.Lock()
	w.find, w.findOpts, w.active = text, opts, 0
	r := w.step(opts.Backward)
	w.mu.Unlock()
	done(r, nil)
}

func (s *Surface) FindNext(backward bool, done func(platform.FindResult, error)) {
	s.b.mustMain("FindNext")
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.find == "" {
		done(platform.FindResult{}, nil)
		return
	}
	done(w.step(backward), nil)
}

func (w *webState) step(backward bool) platform.FindResult { // w.mu held
	hay, needle := w.text, w.find
	if !w.findOpts.MatchCase {
		hay, needle = strings.ToLower(hay), strings.ToLower(needle)
	}
	n := strings.Count(hay, needle)
	if n == 0 {
		w.active = 0
		return platform.FindResult{Matches: 0}
	}
	switch {
	case backward && w.active <= 1:
		if w.active == 1 && w.findOpts.NoWrap {
			break
		}
		w.active = n
	case backward:
		w.active--
	case w.active >= n:
		if w.findOpts.NoWrap {
			break
		}
		w.active = 1
	default:
		w.active++
	}
	return platform.FindResult{Matches: n, Active: w.active, Found: true}
}

func (s *Surface) StopFind() {
	s.b.mustMain("StopFind")
	w := s.webst()
	w.mu.Lock()
	w.find, w.active = "", 0
	w.mu.Unlock()
}

func (s *Surface) Cookies(url string, done func([]*http.Cookie, error)) {
	s.b.mustMain("Cookies")
	j := s.b.jar()
	j.mu.Lock()
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		cc := *c
		out = append(out, &cc)
	}
	j.mu.Unlock()
	done(out, nil) // the core filters by url
}

func sameCookie(a, b *http.Cookie) bool {
	return a.Name == b.Name && strings.EqualFold(strings.TrimPrefix(a.Domain, "."), strings.TrimPrefix(b.Domain, ".")) && a.Path == b.Path
}

func (s *Surface) SetCookie(c *http.Cookie, done func(error)) {
	s.b.mustMain("SetCookie")
	j := s.b.jar()
	cc := *c
	j.mu.Lock()
	j.cookies = slices.DeleteFunc(j.cookies, func(x *http.Cookie) bool { return sameCookie(x, &cc) })
	j.cookies = append(j.cookies, &cc)
	j.mu.Unlock()
	done(nil)
}

func (s *Surface) DeleteCookie(c *http.Cookie, done func(error)) {
	s.b.mustMain("DeleteCookie")
	j := s.b.jar()
	j.mu.Lock()
	j.cookies = slices.DeleteFunc(j.cookies, func(x *http.Cookie) bool { return sameCookie(x, c) })
	j.mu.Unlock()
	done(nil)
}

func (s *Surface) ClearCookies(domain string, done func(error)) {
	s.b.mustMain("ClearCookies")
	j := s.b.jar()
	j.mu.Lock()
	j.cookies = slices.DeleteFunc(j.cookies, func(x *http.Cookie) bool {
		d := strings.ToLower(strings.TrimPrefix(x.Domain, "."))
		return domain == "" || d == domain || strings.HasSuffix(d, "."+domain)
	})
	j.mu.Unlock()
	done(nil)
}

func (s *Surface) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	s.b.mustMain("ClearBrowsingData")
	if kinds&platform.DataCookies != 0 {
		j := s.b.jar()
		j.mu.Lock()
		j.cookies = nil
		j.mu.Unlock()
	}
	w := s.webst()
	w.mu.Lock()
	w.cleared = append(w.cleared, kinds)
	w.mu.Unlock()
	done(nil)
}

func (s *Surface) SetUserAgent(ua string) {
	s.b.mustMain("SetUserAgent")
	w := s.webst()
	w.mu.Lock()
	w.userAgent = ua
	w.mu.Unlock()
}

func (s *Surface) SetExtraHeaders(h http.Header) {
	s.b.mustMain("SetExtraHeaders")
	w := s.webst()
	w.mu.Lock()
	w.headers = h.Clone()
	w.mu.Unlock()
}

func (s *Surface) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	s.b.mustMain("PrintToPDF")
	// Asynchronous, like the real backends.
	s.b.enqueue(func() { done(slices.Clone(FakePDF), nil) })
}

func (s *Surface) Screenshot(done func([]byte, error)) {
	s.b.mustMain("Screenshot")
	s.b.enqueue(func() { done(slices.Clone(FakePNG), nil) })
}

func (s *Surface) CancelDownload(id uint64) {
	s.b.mustMain("CancelDownload")
	w := s.webst()
	w.mu.Lock()
	w.canceled = append(w.canceled, id)
	w.mu.Unlock()
	if h, ok := s.b.h.(platform.WebHost); ok {
		h.OnDownloadEvent(s.id, platform.DownloadEvent{ID: id, State: platform.DownloadCanceled, Total: -1})
	}
}

// ---- accessors (any goroutine) ----

// SetPageText sets the text Find searches.
func (s *Surface) SetPageText(text string) {
	w := s.webst()
	w.mu.Lock()
	w.text = text
	w.mu.Unlock()
}

// WebOptions returns the options given to NewSurfaceWeb and whether the
// surface was created with it.
func (s *Surface) WebOptions() (platform.WebOptions, bool) {
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.web, w.created
}

// UserAgent returns the user agent last set.
func (s *Surface) UserAgent() string {
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.userAgent
}

// ExtraHeaders returns the headers last set with SetExtraHeaders.
func (s *Surface) ExtraHeaders() http.Header {
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.headers.Clone()
}

// ClearedData returns the ClearBrowsingData calls.
func (s *Surface) ClearedData() []platform.BrowsingData {
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.cleared)
}

// CanceledDownloads returns the CancelDownload calls.
func (s *Surface) CanceledDownloads() []uint64 {
	w := s.webst()
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.canceled)
}

// ---- WebHost drivers (any goroutine; they run as native events) ----

func (b *Backend) webHost() platform.WebHost {
	h, _ := b.h.(platform.WebHost)
	return h
}

// PagePermission simulates the page of surface id asking for req and
// returns the decision.
func (b *Backend) PagePermission(id platform.SurfaceID, req platform.PermissionRequest) platform.PermissionDecision {
	ch := make(chan platform.PermissionDecision, 1)
	if !b.Do(func() {
		if h := b.webHost(); h != nil {
			h.OnPermissionRequest(id, req, func(d platform.PermissionDecision) { ch <- d })
		} else {
			ch <- platform.PermissionDefault
		}
	}) {
		return platform.PermissionDefault
	}
	return <-ch
}

// PageNavigate simulates a page-initiated main-frame navigation and reports
// whether the policy allowed it. If allowed, the surface's URL changes
// and SurfaceNavigating is emitted.
func (b *Backend) PageNavigate(id platform.SurfaceID, nav platform.NavigationRequest) bool {
	ok := true
	b.Do(func() {
		if h := b.webHost(); h != nil {
			ok = h.ShouldNavigate(id, nav)
		}
		if ok {
			if s := b.Surface(id); s != nil && !s.Closed() {
				s.mu.Lock()
				s.url = nav.URL
				s.navs = append(s.navs, nav.URL)
				s.mu.Unlock()
				b.h.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: nav.URL})
			}
		}
	})
	return ok
}

// ShouldNavigate asks the WebHost about nav without navigating.
func (b *Backend) ShouldNavigate(id platform.SurfaceID, nav platform.NavigationRequest) bool {
	ok := true
	b.Do(func() {
		if h := b.webHost(); h != nil {
			ok = h.ShouldNavigate(id, nav)
		}
	})
	return ok
}

// NewWindow simulates window.open / target=_blank.
func (b *Backend) NewWindow(id platform.SurfaceID, req platform.NewWindowRequest) bool {
	return b.Do(func() {
		if h := b.webHost(); h != nil {
			h.OnNewWindow(id, req)
		}
	})
}

// HTTPAuth simulates an HTTP authentication challenge and returns the
// answer.
func (b *Backend) HTTPAuth(id platform.SurfaceID, ch platform.AuthChallenge) platform.AuthResponse {
	out := make(chan platform.AuthResponse, 1)
	if !b.Do(func() {
		if h := b.webHost(); h != nil {
			h.OnAuthChallenge(id, ch, func(r platform.AuthResponse) { out <- r })
		} else {
			out <- platform.AuthResponse{}
		}
	}) {
		return platform.AuthResponse{}
	}
	return <-out
}

// StartDownload simulates the page starting a download. It allocates the
// id (when d.ID is 0) and returns it with the destination chosen (""
// canceled). Tests report progress with DownloadEvent.
func (b *Backend) StartDownload(id platform.SurfaceID, d platform.DownloadRequest) (uint64, string) {
	if d.ID == 0 {
		j := b.jar()
		j.mu.Lock()
		j.nextDL++
		d.ID = j.nextDL
		j.mu.Unlock()
	}
	out := make(chan string, 1)
	if !b.Do(func() {
		if h := b.webHost(); h != nil {
			h.OnDownloadStarted(id, d, func(p string) { out <- p })
		} else {
			out <- ""
		}
	}) {
		return d.ID, ""
	}
	return d.ID, <-out
}

// DownloadEvent reports a download event of surface id.
func (b *Backend) DownloadEvent(id platform.SurfaceID, ev platform.DownloadEvent) bool {
	return b.Do(func() {
		if h := b.webHost(); h != nil {
			h.OnDownloadEvent(id, ev)
		}
	})
}
