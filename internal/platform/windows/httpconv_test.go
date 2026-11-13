package windows

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewAssetRequest(t *testing.T) {
	h := http.Header{"Accept": {"text/html"}, "Range": {"bytes=0-"}}
	r, err := newAssetRequest("POST", "https://tango.localhost/api/x?y=1#frag", h, []byte("body"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.Host != "tango.localhost" || r.URL.Path != "/api/x" || r.URL.Query().Get("y") != "1" {
		t.Errorf("request = %s %s %s", r.Method, r.Host, r.URL)
	}
	if r.RequestURI != "/api/x?y=1" {
		t.Errorf("RequestURI = %q", r.RequestURI)
	}
	if r.Header.Get("Range") != "bytes=0-" || r.ContentLength != 4 {
		t.Errorf("header/length = %v %d", r.Header, r.ContentLength)
	}
	if b, _ := io.ReadAll(r.Body); string(b) != "body" {
		t.Errorf("body = %q", b)
	}
	r, err = newAssetRequest("", "https://tango.localhost/", nil, nil)
	if err != nil || r.Method != "GET" || r.Body != http.NoBody || r.Header == nil {
		t.Errorf("defaults = %v %v", r, err)
	}
	if _, err := newAssetRequest("GET", "://bad", nil, nil); err == nil {
		t.Error("bad URL accepted")
	}
}

func TestHeaderBlock(t *testing.T) {
	h := http.Header{"Content-Type": {"text/plain"}, "X-Multi": {"a", "b"}, "X-Evil": {"v\r\nSet-Cookie: x=1"}}
	got := headerBlock(h)
	want := "Content-Type: text/plain\r\nX-Evil: v  Set-Cookie: x=1\r\nX-Multi: a\r\nX-Multi: b"
	if got != want {
		t.Errorf("headerBlock = %q\nwant %q", got, want)
	}
	back := parseHeaderBlock(got)
	if back.Get("Content-Type") != "text/plain" || len(back["X-Multi"]) != 2 || len(back) != 3 {
		t.Errorf("parse back = %v", back)
	}
	if p := parseHeaderBlock("A: 1\nbroken\n: novalue\r\nB:2\r\n"); p.Get("A") != "1" || p.Get("B") != "2" || len(p) != 2 {
		t.Errorf("lenient parse = %v", p)
	}
	if headerBlock(nil) != "" {
		t.Error("empty header block")
	}
}

func TestAssetResponse(t *testing.T) {
	// Handler that writes HTML without Content-Type: sniffed, 200.
	rec := newAssetResponse()
	io.WriteString(rec, "<!doctype html><p>hi")
	status, reason, headers, body := rec.finish()
	if status != 200 || reason != "OK" || string(body) != "<!doctype html><p>hi" {
		t.Errorf("finish = %d %q %q", status, reason, body)
	}
	if !strings.Contains(headers, "Content-Type: text/html; charset=utf-8") || !strings.Contains(headers, "Content-Length: 20") {
		t.Errorf("headers = %q", headers)
	}

	// http.NotFound through the recorder.
	rec = newAssetResponse()
	r, _ := newAssetRequest("GET", "https://tango.localhost/missing", nil, nil)
	http.NotFound(rec, r)
	status, reason, headers, _ = rec.finish()
	if status != 404 || reason != "Not Found" || !strings.Contains(headers, "text/plain") {
		t.Errorf("404 = %d %q %q", status, reason, headers)
	}

	// Explicit status, first WriteHeader wins, explicit nil Content-Type
	// disables sniffing.
	rec = newAssetResponse()
	rec.Header()["Content-Type"] = nil
	rec.WriteHeader(201)
	rec.WriteHeader(500)
	rec.Write([]byte("x"))
	status, _, headers, _ = rec.finish()
	if status != 201 || strings.Contains(headers, "Content-Type") {
		t.Errorf("201 = %d %q", status, headers)
	}

	// Empty response: 200, no sniffed type, 204 has no Content-Length.
	rec = newAssetResponse()
	status, _, headers, body = rec.finish()
	if status != 200 || len(body) != 0 || strings.Contains(headers, "Content-Type") {
		t.Errorf("empty = %d %q", status, headers)
	}
	rec = newAssetResponse()
	rec.WriteHeader(204)
	if _, _, headers, _ = rec.finish(); headers != "" {
		t.Errorf("204 headers = %q", headers)
	}
	rec = newAssetResponse()
	rec.WriteHeader(799)
	if _, reason, _, _ = rec.finish(); reason != "Status 799" {
		t.Errorf("unknown reason = %q", reason)
	}
	rec = newAssetResponse()
	rec.WriteHeader(42)
	if status, _, _, _ = rec.finish(); status != 500 {
		t.Errorf("invalid status mapped to %d", status)
	}
	var _ http.Flusher = rec
}

func TestOriginOf(t *testing.T) {
	cases := map[string]string{
		"https://tango.localhost/index.html?x#y": "https://tango.localhost",
		"https://TANGO.localhost:8443/":          "https://tango.localhost:8443",
		"http://127.0.0.1:5173/app":              "http://127.0.0.1:5173",
		"about:blank":                            "null",
		"data:text/html,hi":                      "null",
		"":                                       "null",
	}
	for in, want := range cases {
		if got := originOf(in); got != want {
			t.Errorf("originOf(%q) = %q, want %q", in, got, want)
		}
	}
	if originOf(assetOrigin+"/") != assetOrigin {
		t.Error("asset origin must round-trip through originOf")
	}
}
