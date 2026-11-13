package tango

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tangoui/tango/internal/platform/fake"
)

// ---------------------------------------------------------------------------
// Cross-site request guard

// Requests that may change state reach Options.Assets only from the asset
// origin itself (CSRF): a remote page cannot post a form to /api.
func TestServeAssetCrossOrigin(t *testing.T) {
	fb := fake.New()
	hits := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ })
	a := newWithBackend(Options{Assets: h}, fb)
	a.origin = fb.AssetOrigin() // Run sets it
	origin := a.assetOrigin()
	if origin == "" {
		t.Fatal("no asset origin")
	}
	for _, tc := range []struct {
		method, origin string
		ok             bool
	}{
		{"GET", "", true},
		{"GET", "https://evil.example", true}, // must be side-effect free
		{"HEAD", "https://evil.example", true},
		{"POST", "", true}, // no page behind it
		{"POST", origin, true},
		{"PUT", strings.ToUpper(origin), true},
		{"POST", origin + "/", true},
		{"POST", "https://evil.example", false},
		{"DELETE", "https://evil.example", false},
		{"OPTIONS", "https://evil.example", false},
		{"POST", "null", false},
		{"POST", origin + ".evil.example", false},
	} {
		r := httptest.NewRequest(tc.method, origin+"/api", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		rec := httptest.NewRecorder()
		before := hits
		host{a}.ServeAsset(0, rec, r)
		if got := hits > before; got != tc.ok || !tc.ok && rec.Code != http.StatusForbidden {
			t.Errorf("%s from %q: reached handler %v (status %d), want %v", tc.method, tc.origin, got, rec.Code, tc.ok)
		}
	}
	// The check can be turned off.
	a = newWithBackend(Options{Assets: h, AllowCrossOriginAssets: true}, fb)
	a.origin = fb.AssetOrigin()
	r := httptest.NewRequest("POST", origin+"/api", nil)
	r.Header.Set("Origin", "https://evil.example")
	before := hits
	host{a}.ServeAsset(0, httptest.NewRecorder(), r)
	if hits == before {
		t.Error("AllowCrossOriginAssets: request refused")
	}
}

// ---------------------------------------------------------------------------
// Same-origin redirects

func TestServeFollowsSameOriginRedirects(t *testing.T) {
	site := fstest.MapFS{"index.html": {Data: []byte("<h1>home</h1>")}}
	fileServer := http.FileServer(http.FS(site))

	req := httptest.NewRequest("GET", "tango://app/index.html", nil)
	rec := httptest.NewRecorder()
	serveFollowingRedirects(fileServer, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (redirect should be followed)", rec.Code)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != "<h1>home</h1>" {
		t.Fatalf("body = %q", body)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("Location leaked into final response: %q", loc)
	}
}

func TestServeKeepsCrossOriginRedirect(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/", http.StatusFound)
	})
	rec := httptest.NewRecorder()
	serveFollowingRedirects(h, rec, httptest.NewRequest("GET", "tango://app/out", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://example.com/" {
		t.Fatalf("got %d %q, want the cross-origin redirect untouched", rec.Code, rec.Header().Get("Location"))
	}
}

func TestServeStopsRedirectLoops(t *testing.T) {
	calls := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	rec := httptest.NewRecorder()
	serveFollowingRedirects(h, rec, httptest.NewRequest("GET", "tango://app/loop", nil))
	if calls != maxAssetRedirects+1 {
		t.Fatalf("handler called %d times, want %d", calls, maxAssetRedirects+1)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("final status = %d, want the last redirect passed through", rec.Code)
	}
}

func TestServePostRedirectBecomesGet(t *testing.T) {
	var methods []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.URL.Path == "/submit" {
			http.Redirect(w, r, "/done", http.StatusSeeOther)
			return
		}
		io.WriteString(w, "ok")
	})
	rec := httptest.NewRecorder()
	serveFollowingRedirects(h, rec, httptest.NewRequest("POST", "tango://app/submit", nil))
	if rec.Body.String() != "ok" || len(methods) != 2 || methods[1] != "GET" {
		t.Fatalf("methods=%v body=%q", methods, rec.Body.String())
	}
}
