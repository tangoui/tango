package tango

// The asset origin: requests the core serves from Options.Assets.

import (
	"net/http"
	"net/url"
	"strings"
)

// ---------------------------------------------------------------------------
// Cross-site request guard

// assetRequestAllowed guards the asset origin against cross-site request
// forgery. Options.Assets cannot tell which page a request comes from, so
// any page a surface shows (a remote site the user followed a link to, an
// iframe) could otherwise send a form POST to <asset origin>/api and have
// it run with the app's authority.
//
// GET and HEAD always pass: they must not change state, and pages load
// their resources with them. Other methods pass when the Origin header is
// the asset origin itself, or absent (requests the WebView or the core make
// without a page behind them; browsers always send Origin on these methods
// from pages). "null" (sandboxed frames, data: URLs, no-referrer pages)
// and every other origin are refused.
func (a *App) assetRequestAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, "":
		return true
	}
	o, ok := r.Header["Origin"]
	if !ok || len(o) == 0 {
		return true
	}
	if len(o) != 1 {
		return false
	}
	origin := strings.ToLower(strings.TrimSuffix(o[0], "/"))
	return origin != "" && origin == strings.ToLower(a.assetOrigin())
}

// ---------------------------------------------------------------------------
// Same-origin redirects

// maxAssetRedirects bounds same-origin redirects followed inside ServeAsset.
const maxAssetRedirects = 5

// serveFollowingRedirects serves r with h, but resolves same-origin 3xx
// responses itself instead of handing them to the WebView.
//
// Custom-scheme handlers (WKURLSchemeHandler on Apple platforms, WebKitGTK
// URI schemes, Android request interception) do not follow redirects: the
// page simply stays blank. Handlers such as http.FileServer redirect
// "/index.html" to "/", so the core follows those hops and answers with the
// final resource. Redirects to another origin are passed through unchanged.
func serveFollowingRedirects(h http.Handler, w http.ResponseWriter, r *http.Request) {
	for hop := 0; ; hop++ {
		rw := &redirectCatcher{ResponseWriter: w, req: r, allow: hop < maxAssetRedirects}
		h.ServeHTTP(rw, r)
		if rw.next == nil {
			return
		}
		r = rw.next
	}
}

// redirectCatcher passes writes through unchanged unless the handler answers
// with a same-origin redirect, in which case it swallows that response and
// records the follow-up request.
type redirectCatcher struct {
	http.ResponseWriter
	req     *http.Request
	allow   bool
	decided bool
	next    *http.Request
}

func (c *redirectCatcher) WriteHeader(code int) {
	if c.decided {
		return
	}
	c.decided = true
	if c.allow && isRedirect(code) {
		if next := c.follow(); next != nil {
			c.next = next
			return
		}
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *redirectCatcher) Write(b []byte) (int, error) {
	if !c.decided {
		c.WriteHeader(http.StatusOK)
	}
	if c.next != nil {
		return len(b), nil // body of a swallowed redirect
	}
	return c.ResponseWriter.Write(b)
}

func (c *redirectCatcher) Flush() {
	if c.next != nil {
		return
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *redirectCatcher) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// follow builds the request for the Location header if it stays on the
// asset origin, clearing the headers the redirect response had set.
func (c *redirectCatcher) follow() *http.Request {
	loc := c.Header().Get("Location")
	if loc == "" {
		return nil
	}
	target, err := c.req.URL.Parse(loc)
	if err != nil || !sameOrigin(c.req.URL, target) {
		return nil
	}
	for k := range c.Header() {
		delete(c.Header(), k)
	}
	next := c.req.Clone(c.req.Context())
	next.URL = target
	next.RequestURI = target.RequestURI()
	next.Host = target.Host
	if c.req.Method != http.MethodGet && c.req.Method != http.MethodHead {
		next.Method = http.MethodGet // 301/302/303 semantics for a page load
		next.Body = http.NoBody
		next.ContentLength = 0
	}
	return next
}

func isRedirect(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

func sameOrigin(a, b *url.URL) bool {
	return b.Scheme == a.Scheme && b.Host == a.Host
}
