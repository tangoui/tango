package asset

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// Independent copy of the pre-optimization path handling from 902c52d.
func referenceCleanPath(p string) (string, bool) {
	if strings.ContainsAny(p, "\\\x00") {
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", false
		}
	}
	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name == "" {
		return "", true
	}
	return name, fs.ValidPath(name)
}

func TestAssetPathsPreserveLegacyBehavior(t *testing.T) {
	parts := []string{"", ".", "..", "...", "a", "file.js", "图标", "%2e%2e", "\\", "\x00", "\xff", "a\nb", "a b"}
	var seed uint32 = 5829
	for n := 0; n < 6000; n++ {
		segments := make([]string, n%7)
		for i := range segments {
			seed = seed*1664525 + 1013904223
			segments[i] = parts[int(seed>>8)%len(parts)]
		}
		p := strings.Join(segments, "/")
		if n%2 == 0 {
			p = "/" + p
		}
		got, ok := cleanPath(p)
		want, wantOK := referenceCleanPath(p)
		if got != want || ok != wantOK {
			t.Fatalf("path %q: got %q/%v, want %q/%v", p, got, ok, want, wantOK)
		}
	}
}

func TestAssetNormalizedPathsPreserveHTTPBehavior(t *testing.T) {
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fsys := fstest.MapFS{
		"app.js":     {Data: []byte("0123456789"), ModTime: modified},
		"index.html": {Data: []byte("root"), ModTime: modified},
	}
	h := FS(fsys, Options{SPA: true, Headers: http.Header{"X-App": {"test"}}})
	for _, tc := range []struct {
		name, method, target string
		headers              http.Header
		status               int
		body, contentRange   string
	}{
		{"get", "GET", "/app.js", nil, 200, "0123456789", ""},
		{"head", "HEAD", "/app.js", nil, 200, "", ""},
		{"range", "GET", "/app.js", http.Header{"Range": {"bytes=2-5"}}, 206, "2345", "bytes 2-5/10"},
		{"head-range", "HEAD", "/app.js", http.Header{"Range": {"bytes=2-5"}}, 206, "", "bytes 2-5/10"},
		{"unchanged", "GET", "/app.js", http.Header{"If-Modified-Since": {modified.Format(http.TimeFormat)}}, 304, "", ""},
		{"older", "GET", "/app.js", http.Header{"If-Modified-Since": {modified.Add(-time.Hour).Format(http.TimeFormat)}}, 200, "0123456789", ""},
		{"if-range-match", "GET", "/app.js", http.Header{"Range": {"bytes=2-5"}, "If-Range": {modified.Format(http.TimeFormat)}}, 206, "2345", "bytes 2-5/10"},
		{"if-range-changed", "GET", "/app.js", http.Header{"Range": {"bytes=2-5"}, "If-Range": {modified.Add(-time.Hour).Format(http.TimeFormat)}}, 200, "0123456789", ""},
		{"cleaned", "GET", "/assets/./../app.js", nil, 400, "bad path\n", ""},
		{"repeated", "GET", "//./app.js", nil, 200, "0123456789", ""},
		{"spa", "GET", "/settings//./profile", nil, 200, "root", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://tango.localhost/", nil)
			r.URL.Path = tc.target
			if tc.headers != nil {
				r.Header = tc.headers
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || w.Body.String() != tc.body || w.Header().Get("Content-Range") != tc.contentRange {
				t.Fatalf("got %d %q range %q; want %d %q range %q", w.Code, w.Body.String(), w.Header().Get("Content-Range"), tc.status, tc.body, tc.contentRange)
			}
			if w.Code != 400 && w.Header().Get("X-App") != "test" {
				t.Fatal("lost configured header")
			}
		})
	}
	fsys["app.js"] = &fstest.MapFile{Data: []byte("changed"), ModTime: modified.Add(time.Hour)}
	w := get(t, h, "GET", "/app.js")
	if w.Code != 200 || w.Body.String() != "changed" {
		t.Fatalf("stale dynamic file: %d %q", w.Code, w.Body.String())
	}
}
