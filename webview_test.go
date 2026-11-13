package tango

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

func TestWebPermissionChain(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	cam := PermissionRequest{Origin: "tango://app", Kinds: []WebPermission{WebPermCamera}}
	both := PermissionRequest{Kinds: []WebPermission{WebPermCamera, WebPermMicrophone}}

	if d := fb.PagePermission(s.ID(), cam); d != PermissionPrompt {
		t.Fatalf("no policy: %v, want prompt", d)
	}
	a.SetPermissionPolicy(WebPermCamera, PermissionAllow)
	if d := fb.PagePermission(s.ID(), cam); d != PermissionAllow {
		t.Fatalf("policy allow: %v", d)
	}
	if d := fb.PagePermission(s.ID(), both); d != PermissionPrompt {
		t.Fatalf("camera allowed, mic unset: %v, want prompt", d)
	}
	a.SetPermissionPolicy("", PermissionDeny)
	if d := fb.PagePermission(s.ID(), both); d != PermissionDeny {
		t.Fatalf("default deny: %v", d)
	}
	var got PermissionRequest
	a.SetPermissionHandler(func(ss *Surface, req PermissionRequest) PermissionDecision {
		if ss != s || !a.isMain() {
			t.Error("app handler: wrong surface or thread")
		}
		got = req
		return PermissionAllow
	})
	if d := fb.PagePermission(s.ID(), both); d != PermissionAllow || len(got.Kinds) != 2 {
		t.Fatalf("app handler: %v %+v", d, got)
	}
	s.SetPermissionHandler(func(req PermissionRequest) PermissionDecision { return PermissionDeny })
	if d := fb.PagePermission(s.ID(), cam); d != PermissionDeny {
		t.Fatalf("surface handler: %v", d)
	}
	s.SetPermissionHandler(func(req PermissionRequest) PermissionDecision { return PermissionDefault })
	if d := fb.PagePermission(s.ID(), cam); d != PermissionAllow {
		t.Fatalf("surface default defers to app: %v", d)
	}
}

func TestWebNavigationPolicy(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{URL: "https://example.com/start"})
	external := func(nav NavigationRequest) bool { return originOf(nav.URL) != "https://example.com" }
	a.SetNavigationPolicy(func(_ *Surface, nav NavigationRequest) bool { return !external(nav) })

	// The core's own first navigation is not asked about.
	if !fb.ShouldNavigate(s.ID(), NavigationRequest{URL: "https://example.com/start"}) {
		t.Fatal("initial URL blocked")
	}
	if fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/"}) {
		t.Fatal("external navigation allowed")
	}
	if !fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://example.com/next"}) {
		t.Fatal("same-site navigation blocked")
	}
	// Navigate by the app passes once, even to an external site.
	s.Navigate("https://evil.test/ok")
	if !fb.ShouldNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/ok"}) {
		t.Fatal("app Navigate blocked")
	}
	if fb.ShouldNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/ok"}) {
		t.Fatal("app Navigate allowance not one-shot")
	}
	s.SetNavigationPolicy(func(nav NavigationRequest) bool { return false })
	if fb.ShouldNavigate(s.ID(), NavigationRequest{URL: "https://example.com/x"}) {
		t.Fatal("surface policy ignored")
	}
}

func TestWebNewWindowDefaults(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{Width: 640, Height: 480, Debug: true, WebView: WebViewOptions{UserAgent: "UA/1"}})
	n := len(fb.Surfaces())

	fb.NewWindow(s.ID(), NewWindowRequest{URL: "https://example.org/page"})
	sync0(a)
	calls := fb.ShellCalls()
	if len(calls) != 1 || calls[0].Op != "url" || calls[0].Arg != "https://example.org/page" {
		t.Fatalf("external link: shell calls %+v", calls)
	}

	fb.NewWindow(s.ID(), NewWindowRequest{URL: "javascript:alert(1)"})
	fb.NewWindow(s.ID(), NewWindowRequest{URL: "file:///etc/passwd"})
	sync0(a)
	if len(fb.ShellCalls()) != 1 || len(fb.Surfaces()) != n {
		t.Fatal("unsafe schemes must be denied")
	}

	fb.NewWindow(s.ID(), NewWindowRequest{URL: fake.DefaultOrigin + "/other.html", Width: 300, Height: 200})
	sync0(a)
	ss := fb.Surfaces()
	if len(ss) != n+1 {
		t.Fatalf("app page: %d surfaces, want %d", len(ss), n+1)
	}
	ns := ss[len(ss)-1]
	if ns.URL() != fake.DefaultOrigin+"/other.html" || !ns.Options().Debug {
		t.Fatalf("new surface %q debug=%v", ns.URL(), ns.Options().Debug)
	}
	if w, h := ns.Options().Width, ns.Options().Height; w != 300 || h != 200 {
		t.Fatalf("new surface size %dx%d", w, h)
	}
	if web, ok := ns.WebOptions(); !ok || web.UserAgent != "UA/1" {
		t.Fatalf("web options not inherited: %+v %v", web, ok)
	}

	var got NewWindowRequest
	s.SetNewWindowHandler(func(req NewWindowRequest) NewWindowAction { got = req; return NewWindowCurrent })
	fb.NewWindow(s.ID(), NewWindowRequest{URL: "https://example.org/here", UserInitiated: true})
	sync0(a)
	sync0(a)
	if !got.UserInitiated || fb.Surface(s.ID()).URL() != "https://example.org/here" {
		t.Fatalf("NewWindowCurrent: req %+v url %q", got, fb.Surface(s.ID()).URL())
	}
	s.SetNewWindowHandler(func(NewWindowRequest) NewWindowAction { return NewWindowDeny })
	fb.NewWindow(s.ID(), NewWindowRequest{URL: "https://example.org/no"})
	sync0(a)
	if len(fb.ShellCalls()) != 1 {
		t.Fatal("deny opened the browser")
	}
}

func TestWebAuthChain(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	ch := AuthChallenge{Host: "intranet", Port: 443, Realm: "r", Scheme: "basic"}
	if r := fb.HTTPAuth(s.ID(), ch); r.Action != AuthDefault {
		t.Fatalf("no handler: %+v", r)
	}
	a.SetAuthHandler(func(_ *Surface, c AuthChallenge) AuthResponse {
		return AuthResponse{Action: AuthCredentials, User: "u", Password: "p:" + c.Realm}
	})
	if r := fb.HTTPAuth(s.ID(), ch); r.Action != AuthCredentials || r.Password != "p:r" {
		t.Fatalf("app handler: %+v", r)
	}
	s.SetAuthHandler(func(AuthChallenge) AuthResponse { return AuthResponse{Action: AuthCancel} })
	if r := fb.HTTPAuth(s.ID(), ch); r.Action != AuthCancel {
		t.Fatalf("surface handler: %+v", r)
	}
}

func TestWebDownloads(t *testing.T) {
	a, fb := newTestApp(t)
	dir := t.TempDir()
	a.SetDownloadDir(dir)
	s, _ := a.NewSurface(SurfaceOptions{})

	if err := os.WriteFile(filepath.Join(dir, "report.pdf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, p := fb.StartDownload(s.ID(), DownloadRequest{URL: "https://x/report.pdf", Filename: "report.pdf"})
	if p != filepath.Join(dir, "report (1).pdf") {
		t.Fatalf("default path %q", p)
	}
	_, p = fb.StartDownload(s.ID(), DownloadRequest{URL: "https://x/a%20b.txt?x=1"})
	if p != filepath.Join(dir, "a b.txt") {
		t.Fatalf("name from URL: %q", p)
	}
	_, p = fb.StartDownload(s.ID(), DownloadRequest{Filename: "../../etc/passwd"})
	if filepath.Dir(p) != dir {
		t.Fatalf("suggested name escaped the directory: %q", p)
	}

	a.SetDownloadHandler(func(_ *Surface, d DownloadRequest) string { return "sub/" + d.Filename })
	_, p = fb.StartDownload(s.ID(), DownloadRequest{Filename: "f.bin"})
	if p != filepath.Join(dir, "sub", "f.bin") {
		t.Fatalf("relative handler path %q", p)
	}
	if fi, err := os.Stat(filepath.Join(dir, "sub")); err != nil || !fi.IsDir() {
		t.Fatal("parent directory not created")
	}
	s.SetDownloadHandler(func(DownloadRequest) string { return "" })
	if _, p = fb.StartDownload(s.ID(), DownloadRequest{Filename: "f.bin"}); p != "" {
		t.Fatalf("surface handler cancel: %q", p)
	}

	var sEv, aEv []DownloadEvent
	s.OnDownloadEvent(func(ev DownloadEvent) { sEv = append(sEv, ev) })
	a.OnDownloadEvent(func(ss *Surface, ev DownloadEvent) {
		if ss == s {
			aEv = append(aEv, ev)
		}
	})
	fb.DownloadEvent(s.ID(), DownloadEvent{ID: 7, State: DownloadProgress, Received: 5, Total: 10})
	fb.DownloadEvent(s.ID(), DownloadEvent{ID: 7, State: DownloadDone, Received: 10, Total: 10, Path: "/x"})
	if len(sEv) != 2 || len(aEv) != 2 || sEv[1].State != DownloadDone {
		t.Fatalf("events surface %+v app %+v", sEv, aEv)
	}
	if err := s.CancelDownload(9); err != nil {
		t.Fatal(err)
	}
	if got := fb.Surface(s.ID()).CanceledDownloads(); len(got) != 1 || got[0] != 9 || sEv[2].State != DownloadCanceled {
		t.Fatalf("cancel: %v %+v", got, sEv)
	}
}

func TestWebControls(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	fs := fb.Surface(s.ID())
	fs.SetPageText("Tango tango TANGO mango")

	r, err := s.Find("tango", FindOptions{})
	if err != nil || r.Matches != 3 || r.Active != 1 || !r.Found {
		t.Fatalf("Find: %+v %v", r, err)
	}
	r, _ = s.FindNext()
	if r.Active != 2 {
		t.Fatalf("FindNext: %+v", r)
	}
	r, _ = s.FindPrevious()
	r, _ = s.FindPrevious()
	if r.Active != 3 {
		t.Fatalf("FindPrevious wrap: %+v", r)
	}
	r, _ = s.Find("tango", FindOptions{MatchCase: true})
	if r.Matches != 1 {
		t.Fatalf("MatchCase: %+v", r)
	}
	if err := s.StopFind(); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.FindNext(); r.Found {
		t.Fatal("FindNext after StopFind found something")
	}

	if err := s.SetCookie(&http.Cookie{Name: "a", Value: "1", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}
	s.SetCookie(&http.Cookie{Name: "b", Value: "2", Domain: ".other.test", Path: "/x", Secure: true})
	s.SetCookie(&http.Cookie{Name: "a", Value: "3", Domain: "example.com"}) // replaces
	if err := s.SetCookie(&http.Cookie{Name: "nodomain"}); err == nil {
		t.Fatal("cookie without domain accepted")
	}
	all, err := s.Cookies("")
	if err != nil || len(all) != 2 {
		t.Fatalf("all cookies: %v %v", all, err)
	}
	got, _ := s.Cookies("https://example.com/")
	if len(got) != 1 || got[0].Value != "3" {
		t.Fatalf("example.com cookies %v", got)
	}
	if got, _ := s.Cookies("http://www.other.test/x/y"); len(got) != 0 {
		t.Fatalf("secure cookie over http: %v", got)
	}
	if got, _ := s.Cookies("https://www.other.test/x/y"); len(got) != 1 {
		t.Fatalf("subdomain + path match: %v", got)
	}
	if got, _ := s.Cookies("https://www.other.test/xy"); len(got) != 0 {
		t.Fatalf("path prefix must stop at a slash: %v", got)
	}
	s.DeleteCookie(&http.Cookie{Name: "a", Domain: "example.com"})
	s.ClearCookies("OTHER.test")
	if all, _ := s.Cookies(""); len(all) != 0 {
		t.Fatalf("after delete/clear: %v", all)
	}
	if err := s.ClearBrowsingData(DataCache | DataStorage); err != nil {
		t.Fatal(err)
	}
	if c := fs.ClearedData(); len(c) != 1 || c[0] != DataCache|DataStorage {
		t.Fatalf("cleared %v", c)
	}

	if err := s.SetUserAgent("X/2"); err != nil || fs.UserAgent() != "X/2" || s.UserAgent() != "X/2" {
		t.Fatalf("user agent %q %v", fs.UserAgent(), err)
	}
	pdf, err := s.PrintToPDF(PDFOptions{})
	if err != nil || string(pdf) != string(fake.FakePDF) {
		t.Fatalf("pdf %q %v", pdf, err)
	}
	png, err := s.Screenshot()
	if err != nil || string(png) != string(fake.FakePNG) {
		t.Fatalf("png %q %v", png, err)
	}
	// From the main thread too (nested wait).
	png = Await(a, func() []byte { b, _ := s.Screenshot(); return b })
	if string(png) != string(fake.FakePNG) {
		t.Fatal("Screenshot on the main thread failed")
	}

	s.Close()
	sync0(a)
	if _, err := s.Screenshot(); !errors.Is(err, ErrSurfaceClosed) {
		t.Fatalf("closed surface: %v", err)
	}
}

func TestWebUnsupported(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, fb.Minimal())
	runApp(t, a)
	s, err := a.NewSurface(SurfaceOptions{WebView: WebViewOptions{UserAgent: "U"}})
	if err != nil {
		t.Fatal(err)
	}
	// The minimal view hides the backend's capabilities but the native
	// surface still has them; check one the core reaches through the
	// backend only (WebCreator), and the unsupported paths with a
	// surface without capabilities.
	if _, ok := fb.Surface(s.ID()).WebOptions(); ok {
		t.Fatal("WebCreator used through Minimal")
	}
	if fb.Surface(s.ID()).UserAgent() != "U" {
		t.Fatal("user agent not applied after creation")
	}
	if _, err := s.Cookies("not a url"); err == nil {
		t.Fatal("bad URL accepted")
	}
}

func TestWebHeaders(t *testing.T) {
	var seen http.Header
	fb := fake.New()
	a := newWithBackend(Options{Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	})}, fb)
	runApp(t, a)
	s, _ := a.NewSurface(SurfaceOptions{WebView: WebViewOptions{Headers: http.Header{"X-App": {"1"}}}})
	if web, ok := fb.Surface(s.ID()).WebOptions(); ok && len(web.Headers) != 0 {
		t.Fatal("asset-scope headers passed to the backend")
	}
	serve := func(id platform.SurfaceID) {
		host{a}.ServeAsset(id, httptest.NewRecorder(), httptest.NewRequest("GET", "tango://app/x", nil))
	}
	serve(s.ID())
	if seen.Get("X-App") != "1" {
		t.Fatalf("asset request headers %v", seen)
	}
	serve(0)
	if seen.Get("X-App") != "" {
		t.Fatal("headers added to a request without surface")
	}
	if err := s.SetExtraHeaders(http.Header{"X-All": {"2"}}, HeadersAll); err != nil {
		t.Fatal(err)
	}
	if fb.Surface(s.ID()).ExtraHeaders().Get("X-All") != "2" {
		t.Fatal("HeadersAll not passed to WebHeaders")
	}
	serve(s.ID())
	if seen.Get("X-All") != "2" || seen.Get("X-App") != "" {
		t.Fatalf("after SetExtraHeaders %v", seen)
	}
	s2, _ := a.NewSurface(SurfaceOptions{WebView: WebViewOptions{UserAgent: "UA", Headers: http.Header{"X-B": {"3"}}, HeaderScope: HeadersAll}})
	if web, ok := fb.Surface(s2.ID()).WebOptions(); !ok || web.UserAgent != "UA" || web.Headers.Get("X-B") != "3" {
		t.Fatalf("NewSurfaceWeb options %+v %v", web, ok)
	}
}

func TestUniquePathAndName(t *testing.T) {
	cases := map[string]DownloadRequest{
		"download":  {},
		"x.zip":     {URL: "https://h/d/x.zip"},
		"a_b.txt":   {Filename: "a/b.txt"},
		"download2": {Filename: "..."},
	}
	for want, req := range cases {
		got := downloadName(req)
		if want == "download2" {
			want = "download"
		}
		if got != want {
			t.Errorf("downloadName(%+v) = %q, want %q", req, got, want)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	os.WriteFile(p, nil, 0o644)
	os.WriteFile(p+" (1)", nil, 0o644)
	if got := uniquePath(p); got != p+" (2)" {
		t.Fatalf("uniquePath %q", got)
	}
}

func TestWebDefaultNavigationOpensExternalInBrowser(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	sync0(a)

	// The app's own pages and non-web URLs load in the surface.
	for _, u := range []string{fake.DefaultOrigin + "/next.html", "about:blank", "data:text/html,x"} {
		if !fb.PageNavigate(s.ID(), NavigationRequest{URL: u}) {
			t.Fatalf("%s blocked", u)
		}
	}
	sync0(a)
	if calls := fb.ShellCalls(); len(calls) != 0 {
		t.Fatalf("own pages went to the browser: %+v", calls)
	}
	// Another site: canceled here, opened in the system browser.
	if fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/page", UserInitiated: true}) {
		t.Fatal("external navigation loaded in the surface")
	}
	sync0(a)
	calls := fb.ShellCalls()
	if len(calls) != 1 || calls[0].Op != "url" || calls[0].Arg != "https://evil.test/page" {
		t.Fatalf("external link: shell calls %+v", calls)
	}
	// A site the app navigated the surface to is its own.
	s.Navigate("https://example.com/start")
	if !fb.ShouldNavigate(s.ID(), NavigationRequest{URL: "https://example.com/start"}) {
		t.Fatal("app Navigate blocked")
	}
	if !fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://example.com/next"}) {
		t.Fatal("same-origin navigation of a site the app opened blocked")
	}
	if fb.PageNavigate(s.ID(), NavigationRequest{URL: "http://other.test/"}) {
		t.Fatal("external http navigation loaded")
	}
	sync0(a)
	if n := len(fb.ShellCalls()); n != 2 {
		t.Fatalf("%d shell calls, want 2", n)
	}

	// Explicit override: a policy decides alone (true loads it here).
	a.SetNavigationPolicy(func(*Surface, NavigationRequest) bool { return true })
	if !fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/again"}) {
		t.Fatal("app policy allowing external navigation ignored")
	}
	a.SetNavigationPolicy(nil)
	s.SetNavigationPolicy(func(nav NavigationRequest) bool { return nav.URL == "https://allowed.test/" })
	if !fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://allowed.test/"}) {
		t.Fatal("surface policy allowing external navigation ignored")
	}
	if fb.PageNavigate(s.ID(), NavigationRequest{URL: "https://evil.test/x"}) {
		t.Fatal("surface policy refusal ignored")
	}
	sync0(a)
	if n := len(fb.ShellCalls()); n != 2 {
		t.Fatalf("a policy's decisions must not open the browser: %d shell calls", n)
	}
}

func TestWebDefaultDownloadNeedsTrustedPage(t *testing.T) {
	a, fb := newTestApp(t)
	dir := t.TempDir()
	a.SetDownloadDir(dir)
	s, _ := a.NewSurface(SurfaceOptions{})
	sync0(a)

	// The app's page: saved to the download directory, as before.
	if _, p := fb.StartDownload(s.ID(), DownloadRequest{URL: fake.DefaultOrigin + "/r.txt", Filename: "r.txt"}); p != filepath.Join(dir, "r.txt") {
		t.Fatalf("trusted download: %q", p)
	}
	// Another site's page (let in by a policy): canceled, whatever the URL.
	fb.LoadURL(s.ID(), "https://evil.test/")
	for _, u := range []string{"https://evil.test/x.exe", fake.DefaultOrigin + "/r.txt", "blob:https://evil.test/1"} {
		if _, p := fb.StartDownload(s.ID(), DownloadRequest{URL: u, Filename: "x.exe"}); p != "" {
			t.Fatalf("download %s from an external page saved to %q", u, p)
		}
	}
	// Explicit override: a handler decides for every page.
	a.SetDownloadHandler(func(_ *Surface, d DownloadRequest) string { return a.DownloadPath(d) })
	if _, p := fb.StartDownload(s.ID(), DownloadRequest{URL: "https://evil.test/x.exe", Filename: "x.exe"}); p != filepath.Join(dir, "x.exe") {
		t.Fatalf("app handler ignored: %q", p)
	}
	a.SetDownloadHandler(nil)
	s.SetDownloadHandler(func(d DownloadRequest) string { return filepath.Join(dir, "s-"+d.Filename) })
	if _, p := fb.StartDownload(s.ID(), DownloadRequest{Filename: "y.bin"}); p != filepath.Join(dir, "s-y.bin") {
		t.Fatalf("surface handler ignored: %q", p)
	}
	s.SetDownloadHandler(nil)
	// Back on the app's page.
	fb.LoadURL(s.ID(), fake.DefaultOrigin+"/")
	if _, p := fb.StartDownload(s.ID(), DownloadRequest{Filename: "z.txt"}); p != filepath.Join(dir, "z.txt") {
		t.Fatalf("trusted again: %q", p)
	}
	// A site the app navigated to is trusted.
	s.Navigate("https://example.com/")
	fb.LoadURL(s.ID(), "https://example.com/")
	if _, p := fb.StartDownload(s.ID(), DownloadRequest{Filename: "w.txt"}); p != filepath.Join(dir, "w.txt") {
		t.Fatalf("download of the app's own site: %q", p)
	}
}

func TestWebPermissionPolicyOnlyForTrustedOrigins(t *testing.T) {
	a, fb := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	sync0(a)
	cam := []WebPermission{WebPermCamera}
	own := PermissionRequest{Origin: fake.DefaultOrigin, Kinds: cam}
	ext := PermissionRequest{Origin: "https://evil.test", Kinds: cam}

	// No policy: the own page is asked about, another site denied.
	if d := fb.PagePermission(s.ID(), own); d != PermissionPrompt {
		t.Fatalf("own page, no policy: %v", d)
	}
	if d := fb.PagePermission(s.ID(), ext); d != PermissionDeny {
		t.Fatalf("external, no policy: %v", d)
	}
	// The app's grant applies to its own pages only.
	a.SetPermissionPolicy(WebPermCamera, PermissionAllow)
	if d := fb.PagePermission(s.ID(), own); d != PermissionAllow {
		t.Fatalf("own page, policy allow: %v", d)
	}
	if d := fb.PagePermission(s.ID(), ext); d != PermissionDeny {
		t.Fatalf("external origin got the app's grant: %v", d)
	}
	// No origin reported: the page shown decides.
	noOrigin := PermissionRequest{Kinds: cam}
	if d := fb.PagePermission(s.ID(), noOrigin); d != PermissionAllow {
		t.Fatalf("no origin on the own page: %v", d)
	}
	fb.LoadURL(s.ID(), "https://evil.test/")
	if d := fb.PagePermission(s.ID(), noOrigin); d != PermissionDeny {
		t.Fatalf("no origin on an external page: %v", d)
	}
	// Explicit, by origin: a handler can allow another site.
	a.SetPermissionHandler(func(_ *Surface, req PermissionRequest) PermissionDecision {
		if req.Origin == "https://meet.test" {
			return PermissionAllow
		}
		return PermissionDefault
	})
	if d := fb.PagePermission(s.ID(), PermissionRequest{Origin: "https://meet.test", Kinds: cam}); d != PermissionAllow {
		t.Fatalf("handler allowing by origin: %v", d)
	}
	if d := fb.PagePermission(s.ID(), ext); d != PermissionDeny {
		t.Fatalf("handler default for another origin: %v", d)
	}
	s.SetPermissionHandler(func(req PermissionRequest) PermissionDecision { return PermissionPrompt })
	if d := fb.PagePermission(s.ID(), ext); d != PermissionPrompt {
		t.Fatalf("surface handler asking: %v", d)
	}
}

func TestHostTrustsOrigin(t *testing.T) {
	a, _ := newTestApp(t)
	s, _ := a.NewSurface(SurfaceOptions{})
	sync0(a)
	h := host{a}
	if !h.TrustsOrigin(s.ID(), fake.DefaultOrigin) {
		t.Fatal("asset origin not trusted")
	}
	for _, o := range []string{"https://evil.test", "", "null", "file://"} {
		if h.TrustsOrigin(s.ID(), o) {
			t.Fatalf("%q trusted", o)
		}
	}
	if h.TrustsOrigin(s.ID()+100, fake.DefaultOrigin) {
		t.Fatal("unknown surface trusted")
	}
	s.Navigate("https://example.com/")
	if !h.TrustsOrigin(s.ID(), "https://example.com") {
		t.Fatal("origin the app navigated to not trusted")
	}
	s.Close()
	sync0(a)
	if h.TrustsOrigin(s.ID(), fake.DefaultOrigin) {
		t.Fatal("closed surface trusted")
	}
}
