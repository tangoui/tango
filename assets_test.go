package tango

import (
	"context"
	"errors"
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

// flushErrWriter is a ResponseWriter whose FlushError fails, like the
// macOS/iOS asset writer after WebKit stopped the task.
type flushErrWriter struct {
	*httptest.ResponseRecorder
	err     error
	flushed int
}

func (w *flushErrWriter) FlushError() error { w.flushed++; return w.err }

// A handler behind the redirect follower sees the wrapped writer's flush
// error through http.ResponseController; the old Flush keeps working, and
// a swallowed redirect flushes nothing.
func TestRedirectCatcherFlushError(t *testing.T) {
	under := &flushErrWriter{ResponseRecorder: httptest.NewRecorder(), err: context.Canceled}
	var got error
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("flush of a swallowed redirect: %v", err)
			}
			return
		}
		io.WriteString(w, "chunk")
		got = http.NewResponseController(w).Flush()
		w.(http.Flusher).Flush() // the plain Flusher still works
	})
	serveFollowingRedirects(h, under, httptest.NewRequest("GET", "tango://app/old", nil))
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("ResponseController.Flush = %v, want context.Canceled", got)
	}
	if under.flushed != 1 {
		t.Fatalf("underlying FlushError called %d times, want 1 (not for the swallowed redirect)", under.flushed)
	}
	if !under.ResponseRecorder.Flushed {
		t.Fatal("plain Flush did not reach the underlying writer")
	}
}

// plainWriter can neither flush nor unwrap, like the darwin download sink
// (copyAsset's fileSink).
type plainWriter struct {
	h    http.Header
	body strings.Builder
}

func (w *plainWriter) Header() http.Header         { return w.h }
func (w *plainWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *plainWriter) WriteHeader(int)             {}

// Behind the redirect follower, flushing a writer that cannot flush stays
// a no-op: ResponseController.Flush returns nil, not http.ErrNotSupported,
// so a handler that stops on a flush error does not cut a download short.
func TestRedirectCatcherFlushUnsupported(t *testing.T) {
	under := &plainWriter{h: http.Header{}}
	var got error
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "part1")
		got = http.NewResponseController(w).Flush()
		if got != nil {
			return
		}
		io.WriteString(w, "part2")
	})
	serveFollowingRedirects(h, under, httptest.NewRequest("GET", "tango://app/file", nil))
	if got != nil {
		t.Fatalf("ResponseController.Flush = %v, want nil", got)
	}
	if under.body.String() != "part1part2" {
		t.Fatalf("body %q, want the whole file", under.body.String())
	}
}
