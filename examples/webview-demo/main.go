// Command webview-demo exercises the advanced WebView features (package
// tango, webview.go): permission requests, downloads, cookies, HTTP
// authentication, find in page, user agent and extra headers, new-window
// requests, the navigation policy, PDF and PNG capture.
//
//	go run ./examples/webview-demo          # opens the demo page
//	go run ./examples/webview-demo -smoke   # runs the checks, exits 0 on success
//
// The smoke run serves a page from the app's assets and a second one from
// a local HTTP server (for Basic authentication and a secure-context
// getUserMedia). Generated files go to a temporary directory that is
// printed and removed at the end (kept with -keep).
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
)

const page = `<!doctype html><meta charset="utf-8"><title>WebView demo</title>
<style>body{font:15px system-ui;margin:24px}</style>
<h1>WebView demo</h1>
<p>apple banana Apple cherry apple pie</p>
<p><a id="dl" href="/files/report.txt">download report.txt</a> ·
<a id="ext" href="https://example.com/" target="_blank">example.com (new window)</a></p>
<p><button onclick="navigator.mediaDevices&&navigator.mediaDevices.getUserMedia({video:true}).catch(e=>console.log(e))">camera</button></p>
`

const report = "Tango WebView demo report\n"

var binData = bytes.Repeat([]byte("0123456789abcdef"), 64<<10) // 1 MiB

var (
	failed int
	mu     sync.Mutex
)

func check(name string, err error) {
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		if g := checkGroup(name); g != "" && smokeSkip(g) {
			log.Printf("SKIP %s: %v (TANGO_SMOKE_SKIP=%s)", name, err, g)
			return
		}
		failed++
		log.Printf("FAIL %s: %v", name, err)
		return
	}
	log.Printf("PASS %s", name)
}

// checkGroup names the environment-dependent group a check belongs to, for
// unattended runs: headless CI (Xvfb, containers) has no camera, and its
// WebKitGTK build reports find-in-page matches and downloads differently.
func checkGroup(name string) string {
	switch {
	case strings.HasPrefix(name, "Find"):
		return "find"
	case strings.Contains(name, "download to chosen path"), strings.HasPrefix(name, "http download"):
		return "download"
	case strings.HasPrefix(name, "permission "):
		return "camera"
	}
	return ""
}

// smokeSkip reports whether TANGO_SMOKE_SKIP (comma-separated groups) names
// group: a failure in it is logged as SKIP and does not fail the run.
func smokeSkip(group string) bool {
	for _, g := range strings.Split(os.Getenv("TANGO_SMOKE_SKIP"), ",") {
		if strings.TrimSpace(g) == group {
			return true
		}
	}
	return false
}

func main() {
	smoke := flag.Bool("smoke", false, "run the checks automatically and exit")
	keep := flag.Bool("keep", false, "keep the generated files")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	var assetHeader sync.Map // last X-Demo header the asset handler saw
	assets := http.NewServeMux()
	assets.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		assetHeader.Store("x", r.Header.Get("X-Demo"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, page)
	})
	assets.HandleFunc("/files/report.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
		io.WriteString(w, report)
	})

	app := tango.New(tango.Options{Name: "WebView Demo", ID: "dev.tango.webview-demo", Assets: assets})
	msgs := newMailbox()
	app.OnMessage(func(s *tango.Surface, m string) { msgs.put(m) })

	if !*smoke {
		app.OnReady(func() {
			_, err := app.NewSurface(tango.SurfaceOptions{Title: "WebView demo", Width: 900, Height: 640, Resizable: true, Debug: true})
			if err != nil {
				log.Fatal(err)
			}
		})
		if err := app.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	dir, err := os.MkdirTemp("", "tango-webview-demo-")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("output directory %s", dir)
	app.OnReady(func() {
		go func() {
			run(app, msgs, dir, &assetHeader)
			app.Quit()
		}()
	})
	time.AfterFunc(scaled(40*time.Second), func() {
		log.Print("FAIL timeout")
		os.Exit(2)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	if !*keep {
		os.RemoveAll(dir)
	}
	if failed > 0 {
		log.Printf("%d check(s) failed", failed)
		os.Exit(1)
	}
	log.Print("all checks passed")
}

func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 0 {
		return time.Duration(float64(d) * f)
	}
	return d
}

// mailbox collects raw page messages.
type mailbox struct {
	mu sync.Mutex
	ch chan string
}

func newMailbox() *mailbox { return &mailbox{ch: make(chan string, 64)} }

func (m *mailbox) put(s string) {
	select {
	case m.ch <- s:
	default:
	}
}

// wait returns the next message starting with prefix (others are dropped).
func (m *mailbox) wait(prefix string, d time.Duration) (string, error) {
	t := time.After(scaled(d))
	for {
		select {
		case s := <-m.ch:
			if strings.HasPrefix(s, prefix) {
				return strings.TrimPrefix(s, prefix), nil
			}
		case <-t:
			return "", fmt.Errorf("no %q message", prefix)
		}
	}
}

func waitFor(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(scaled(d))
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func run(app *tango.App, msgs *mailbox, dir string, assetHeader *sync.Map) {
	// A local HTTP server: Basic authentication and a localhost page (a
	// secure context, for getUserMedia).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		check("local server", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><title>local</title><p>local page</p><a id="bin" href="/data.bin">data.bin</a>`)
	})
	mux.HandleFunc("/data.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(binData)))
		w.Write(binData)
	})
	mux.HandleFunc("/protected", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "demo" || p != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="tango-demo"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, "<!doctype html><title>ok</title><p>welcome</p>")
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())
	local := "http://" + ln.Addr().String()

	// ---- main surface: assets, user agent, extra asset headers ----
	var navMu sync.Mutex
	var navAsked []string
	app.SetNavigationPolicy(func(s *tango.Surface, nav tango.NavigationRequest) bool {
		navMu.Lock()
		navAsked = append(navAsked, nav.URL)
		navMu.Unlock()
		// Stay on the app's own pages and the local server.
		return strings.HasPrefix(nav.URL, "tango://") || strings.HasPrefix(nav.URL, "https://tango.localhost") ||
			strings.HasPrefix(nav.URL, local) || nav.URL == "about:blank"
	})
	loaded := make(chan struct{}, 8)
	s, err := app.NewSurface(tango.SurfaceOptions{
		Title: "WebView demo", Width: 900, Height: 640, Resizable: true, Debug: true,
		WebView: tango.WebViewOptions{UserAgent: "TangoDemo/1.0", Headers: http.Header{"X-Demo": {"yes"}}},
	})
	if err != nil {
		check("NewSurface", err)
		return
	}
	s.OnLoaded(func() {
		select {
		case loaded <- struct{}{}:
		default:
		}
	})
	select {
	case <-loaded:
	case <-time.After(scaled(10 * time.Second)):
		check("page load", errors.New("timeout"))
		return
	}

	s.Eval(`window.__tango.post("ua:"+navigator.userAgent)`)
	ua, err := msgs.wait("ua:", 5*time.Second)
	if err == nil && ua != "TangoDemo/1.0" {
		err = fmt.Errorf("navigator.userAgent = %q", ua)
	}
	check("user agent at creation", err)
	if v, _ := assetHeader.Load("x"); v != "yes" {
		check("extra asset headers", fmt.Errorf("X-Demo = %v", v))
	} else {
		check("extra asset headers", nil)
	}
	if err := s.SetUserAgent("TangoDemo/2.0"); err != nil {
		check("SetUserAgent", err)
	} else {
		s.Eval(`window.__tango.post("ua:"+navigator.userAgent)`)
		ua, err = msgs.wait("ua:", 5*time.Second)
		if err == nil && ua != "TangoDemo/2.0" {
			err = fmt.Errorf("navigator.userAgent = %q", ua)
		}
		check("SetUserAgent", err)
	}

	// ---- find in page ----
	r, err := s.Find("apple", tango.FindOptions{})
	if err == nil && (!r.Found || r.Matches != 3 || r.Active != 1) {
		err = fmt.Errorf("Find = %+v, want 3 matches, active 1", r)
	}
	check("Find count", err)
	r, err = s.FindNext()
	if err == nil && (!r.Found || r.Active != 2) {
		err = fmt.Errorf("FindNext = %+v", r)
	}
	check("FindNext", err)
	r, err = s.FindPrevious()
	if err == nil && (!r.Found || r.Active != 1) {
		err = fmt.Errorf("FindPrevious = %+v", r)
	}
	check("FindPrevious", err)
	r, err = s.Find("apple", tango.FindOptions{MatchCase: true})
	if err == nil && r.Matches != 2 {
		err = fmt.Errorf("MatchCase Find = %+v", r)
	}
	check("Find match case", err)
	r, err = s.Find("durian", tango.FindOptions{})
	if err == nil && (r.Found || r.Matches != 0) {
		err = fmt.Errorf("Find(missing) = %+v", r)
	}
	check("Find no match", err)
	check("StopFind", s.StopFind())

	// ---- new window requests ----
	newWin := make(chan tango.NewWindowRequest, 4)
	s.SetNewWindowHandler(func(req tango.NewWindowRequest) tango.NewWindowAction {
		newWin <- req
		return tango.NewWindowDeny
	})
	s.Eval(`window.__tango.post("open:"+(window.open("https://example.com/popup","_blank","width=400,height=300")===null))`)
	select {
	case req := <-newWin:
		var err error
		if req.URL != "https://example.com/popup" {
			err = fmt.Errorf("request %+v", req)
		}
		check("window.open intercepted", err)
		log.Printf("    window features %dx%d user=%v", req.Width, req.Height, req.UserInitiated)
	case <-time.After(scaled(5 * time.Second)):
		check("window.open intercepted", errors.New("no OnNewWindow"))
	}
	if v, err := msgs.wait("open:", 3*time.Second); err != nil || v != "true" {
		check("window.open returns null", fmt.Errorf("%q %v", v, err))
	} else {
		check("window.open returns null", nil)
	}
	s.Eval(`document.getElementById("ext").click()`)
	select {
	case req := <-newWin:
		var err error
		if req.URL != "https://example.com/" {
			err = fmt.Errorf("request %+v", req)
		}
		check("target=_blank intercepted", err)
	case <-time.After(scaled(5 * time.Second)):
		check("target=_blank intercepted", errors.New("no OnNewWindow"))
	}

	// ---- navigation policy ----
	navMu.Lock()
	navAsked = nil
	navMu.Unlock()
	navigating := make(chan string, 4)
	s.OnNavigating(func(u string) { navigating <- u })
	s.Eval(`location.href="https://example.com/blocked"`)
	ok := waitFor(5*time.Second, func() bool {
		navMu.Lock()
		defer navMu.Unlock()
		for _, u := range navAsked {
			if u == "https://example.com/blocked" {
				return true
			}
		}
		return false
	})
	time.Sleep(300 * time.Millisecond)
	select {
	case u := <-navigating:
		check("navigation policy cancels", fmt.Errorf("navigation to %s started", u))
	default:
		if !ok {
			check("navigation policy cancels", errors.New("policy not asked"))
		} else {
			check("navigation policy cancels", nil)
		}
	}

	// ---- downloads ----
	dlDone := make(chan tango.DownloadEvent, 4)
	var dlReq tango.DownloadRequest
	s.SetDownloadHandler(func(req tango.DownloadRequest) string {
		dlReq = req
		return filepath.Join(dir, "downloads", req.Filename)
	})
	s.OnDownloadEvent(func(ev tango.DownloadEvent) {
		if ev.State != tango.DownloadProgress {
			dlDone <- ev
		}
	})
	s.Eval(`document.getElementById("dl").click()`)
	select {
	case ev := <-dlDone:
		var err error
		if ev.State != tango.DownloadDone {
			err = fmt.Errorf("download ended %+v", ev)
		} else if b, rerr := os.ReadFile(ev.Path); rerr != nil || string(b) != report {
			err = fmt.Errorf("file %s: %q %v", ev.Path, b, rerr)
		} else if dlReq.Filename != "report.txt" {
			err = fmt.Errorf("request %+v", dlReq)
		}
		check("download to chosen path", err)
		log.Printf("    request %+v -> %s (%d bytes)", dlReq, ev.Path, ev.Received)
	case <-time.After(scaled(10 * time.Second)):
		check("download to chosen path", errors.New("no download event"))
	}

	// ---- cookies ----
	err = s.SetCookie(&http.Cookie{Name: "demo", Value: "42", Domain: "127.0.0.1", Path: "/", Expires: time.Now().Add(time.Hour)})
	check("SetCookie", err)
	cs, err := s.Cookies(local + "/")
	if err == nil && !hasCookie(cs, "demo", "42") {
		err = fmt.Errorf("cookies %v", cs)
	}
	check("Cookies(url)", err)
	all, err := s.Cookies("")
	if err == nil && !hasCookie(all, "demo", "42") {
		err = fmt.Errorf("all cookies %v", all)
	}
	check("Cookies(all)", err)
	check("DeleteCookie", s.DeleteCookie(&http.Cookie{Name: "demo", Domain: "127.0.0.1", Path: "/"}))
	if cs, err := s.Cookies(local + "/"); err != nil || hasCookie(cs, "demo", "") {
		check("cookie deleted", fmt.Errorf("%v %v", cs, err))
	} else {
		check("cookie deleted", nil)
	}

	// ---- local page: page cookies, permissions, Basic auth ----
	drain(loaded)
	s.Navigate(local + "/")
	select {
	case <-loaded:
	case <-time.After(scaled(10 * time.Second)):
		check("local page load", errors.New("timeout"))
	}
	s.Eval(`document.cookie="page=1; path=/"; window.__tango.post("ck:"+document.cookie)`)
	if _, err := msgs.wait("ck:", 5*time.Second); err != nil {
		check("page cookie", err)
	} else {
		cs, err := s.Cookies(local + "/")
		if err == nil && !hasCookie(cs, "page", "1") {
			err = fmt.Errorf("cookies %v", cs)
		}
		check("page cookie visible to Go", err)
	}
	s.Eval(`document.getElementById("bin").click()`)
	select {
	case ev := <-dlDone:
		var err error
		if ev.State != tango.DownloadDone {
			err = fmt.Errorf("download ended %+v", ev)
		} else if b, rerr := os.ReadFile(ev.Path); rerr != nil || !bytes.Equal(b, binData) {
			err = fmt.Errorf("file %s: %d bytes %v", ev.Path, len(b), rerr)
		}
		check("http download (WKDownload / native)", err)
		log.Printf("    request %+v -> %s (%d bytes)", dlReq, ev.Path, ev.Received)
	case <-time.After(scaled(10 * time.Second)):
		check("http download (WKDownload / native)", errors.New("no download event"))
	}
	s.Eval(`window.__tango.post("alive:1")`)
	if _, err := msgs.wait("alive:", 3*time.Second); err != nil {
		check("page still talks after download", err)
	} else {
		check("page still talks after download", nil)
	}

	check("ClearCookies(domain)", s.ClearCookies("127.0.0.1"))
	if cs, _ := s.Cookies(local + "/"); len(cs) != 0 {
		check("cookies cleared", fmt.Errorf("%v", cs))
	} else {
		check("cookies cleared", nil)
	}
	check("ClearBrowsingData", s.ClearBrowsingData(tango.DataCache|tango.DataStorage))

	enableMockCapture(s)
	perm := make(chan tango.PermissionRequest, 2)
	s.SetPermissionHandler(func(req tango.PermissionRequest) tango.PermissionDecision {
		perm <- req
		return tango.PermissionDeny
	})
	s.Eval(`try{navigator.mediaDevices.getUserMedia({video:true}).then(()=>window.__tango.post("gum:ok"),e=>window.__tango.post("gum:"+e.name))}catch(e){window.__tango.post("gum:sync "+e+" "+typeof navigator.mediaDevices+" "+isSecureContext)}`)
	select {
	case req := <-perm:
		var err error
		if len(req.Kinds) == 0 || req.Kinds[0] != tango.WebPermCamera {
			err = fmt.Errorf("request %+v", req)
		}
		check("permission request callback", err)
		log.Printf("    permission request %+v", req)
		if v, err := msgs.wait("gum:", 5*time.Second); err != nil || v != "NotAllowedError" {
			check("permission denied reaches the page", fmt.Errorf("%q %v", v, err))
		} else {
			check("permission denied reaches the page", nil)
		}
	case <-time.After(scaled(5 * time.Second)):
		v, _ := msgs.wait("gum:", time.Second)
		check("permission request callback", fmt.Errorf("no callback (page: %q)", v))
	}

	auth := make(chan tango.AuthChallenge, 2)
	s.SetAuthHandler(func(ch tango.AuthChallenge) tango.AuthResponse {
		auth <- ch
		return tango.AuthResponse{Action: tango.AuthCredentials, User: "demo", Password: "secret"}
	})
	drain(loaded)
	s.Navigate(local + "/protected")
	select {
	case ch := <-auth:
		var err error
		if ch.Scheme != "basic" || ch.Realm != "tango-demo" {
			err = fmt.Errorf("challenge %+v", ch)
		}
		check("HTTP auth challenge", err)
	case <-time.After(scaled(8 * time.Second)):
		check("HTTP auth challenge", errors.New("no callback"))
	}
	select {
	case <-loaded:
		s.Eval(`window.__tango.post("title:"+document.title)`)
		if t, err := msgs.wait("title:", 5*time.Second); err != nil || t != "ok" {
			check("authenticated page", fmt.Errorf("%q %v", t, err))
		} else {
			check("authenticated page", nil)
		}
	case <-time.After(scaled(8 * time.Second)):
		check("authenticated page", errors.New("not loaded"))
	}

	// ---- PDF and screenshot ----
	pdf, err := s.PrintToPDF(tango.PDFOptions{})
	if err == nil && !bytes.HasPrefix(pdf, []byte("%PDF")) {
		err = fmt.Errorf("not a PDF (%d bytes)", len(pdf))
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "page.pdf"), pdf, 0o644)
	}
	check("PrintToPDF", err)
	png, err := s.Screenshot()
	if err == nil && !bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")) {
		err = fmt.Errorf("not a PNG (%d bytes)", len(png))
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "page.png"), png, 0o644)
	}
	check("Screenshot", err)
	log.Printf("    pdf %d bytes, png %d bytes", len(pdf), len(png))
}

func drain(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func hasCookie(cs []*http.Cookie, name, value string) bool {
	for _, c := range cs {
		if c.Name == name && (value == "" || c.Value == value) {
			return true
		}
	}
	return false
}
