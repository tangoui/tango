package asset

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var testFS = fstest.MapFS{
	"index.html":         {Data: []byte("<!doctype html><title>root</title>")},
	"app.js":             {Data: []byte("console.log(1)")},
	"style.css":          {Data: []byte("body{}")},
	"mod.wasm":           {Data: []byte("\x00asm")},
	"img/logo.svg":       {Data: []byte("<svg/>")},
	"docs/index.html":    {Data: []byte("<p>docs</p>")},
	"empty/readme.txt":   {Data: []byte("hi")},
	"noext":              {Data: []byte("plain text here")},
	"data.unknownext123": {Data: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}},
}

func get(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://tango.localhost/", nil)
	req.URL.Path = target // bypass client-side cleaning
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestFSContentTypes(t *testing.T) {
	h := FS(testFS, Options{})
	cases := map[string]string{
		"/":                   "text/html; charset=utf-8",
		"/index.html":         "text/html; charset=utf-8",
		"/app.js":             "text/javascript; charset=utf-8",
		"/style.css":          "text/css; charset=utf-8",
		"/mod.wasm":           "application/wasm",
		"/img/logo.svg":       "image/svg+xml",
		"/noext":              "text/plain; charset=utf-8", // sniffed
		"/data.unknownext123": "image/png",                 // sniffed
	}
	for p, want := range cases {
		w := get(t, h, "GET", p)
		if w.Code != 200 || w.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want %q", p, w.Code, w.Header().Get("Content-Type"), want)
		}
	}
	if w := get(t, h, "GET", "/app.js"); w.Body.String() != "console.log(1)" {
		t.Errorf("body %q", w.Body.String())
	}
	if w := get(t, h, "HEAD", "/app.js"); w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD: %d %d", w.Code, w.Body.Len())
	}
	if w := get(t, h, "POST", "/app.js"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
}

func TestFSDirectories(t *testing.T) {
	h := FS(testFS, Options{})
	if w := get(t, h, "GET", "/docs/"); w.Code != 200 || w.Body.String() != "<p>docs</p>" {
		t.Errorf("/docs/: %d %q", w.Code, w.Body.String())
	}
	if w := get(t, h, "GET", "/docs"); w.Code != 200 || w.Body.String() != "<p>docs</p>" {
		t.Errorf("/docs: %d %q", w.Code, w.Body.String())
	}
	// No listing for a directory without index.
	if w := get(t, h, "GET", "/empty/"); w.Code != 404 {
		t.Errorf("/empty/: %d", w.Code)
	}
	if w := get(t, h, "GET", "/missing.js"); w.Code != 404 {
		t.Errorf("missing: %d", w.Code)
	}
}

func TestFSTraversal(t *testing.T) {
	h := FS(testFS, Options{SPA: true})
	for _, p := range []string{"/../index.html", "/img/../../app.js", "/..", "/a\\..\\b", "/x\x00y"} {
		w := get(t, h, "GET", p)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: %d", p, w.Code)
		}
	}
	// Percent-encoded traversal decodes into URL.Path and is rejected too.
	req := httptest.NewRequest("GET", "http://tango.localhost/%2e%2e/app.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("encoded traversal: %d", w.Code)
	}
}

func TestFSSPA(t *testing.T) {
	h := FS(testFS, Options{SPA: true, Headers: http.Header{"Content-Security-Policy": {"default-src 'self'"}}})
	w := get(t, h, "GET", "/settings/profile")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "root") {
		t.Errorf("spa: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("custom header missing")
	}
	if w := get(t, h, "GET", "/missing.js"); w.Code != 404 {
		t.Errorf("asset with extension must 404: %d", w.Code)
	}
	if w := get(t, FS(testFS, Options{}), "GET", "/settings"); w.Code != 404 {
		t.Errorf("non-SPA: %d", w.Code)
	}
}

func TestDevProxyHTTPAndWebSocket(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http") {
				http.Error(w, "bad origin "+o, 403)
				return
			}
			conn, brw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			brw.Flush()
			// Echo one line.
			line, _ := brw.ReadString('\n')
			brw.WriteString("echo:" + line)
			brw.Flush()
			return
		}
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, "host="+r.Host+" path="+r.URL.Path)
	}))
	defer backend.Close()

	proxy := httptest.NewServer(DevProxy(backend.URL))
	defer proxy.Close()

	res, err := http.Get(proxy.URL + "/src/main.ts")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	wantHost := strings.TrimPrefix(backend.URL, "http://")
	if string(body) != "host="+wantHost+" path=/src/main.ts" {
		t.Errorf("proxied body %q", body)
	}

	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(proxy.URL, "http://"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	io.WriteString(conn, "GET /hmr HTTP/1.1\r\nHost: tango.localhost\r\nOrigin: tango://app\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: x\r\nSec-WebSocket-Version: 13\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 101 {
		t.Fatalf("upgrade status %d", resp.StatusCode)
	}
	io.WriteString(conn, "ping\n")
	line, err := br.ReadString('\n')
	if err != nil || line != "echo:ping\n" {
		t.Fatalf("ws echo %q %v", line, err)
	}
}

func TestDevProxyErrors(t *testing.T) {
	w := get(t, DevProxy("not a url"), "GET", "/")
	if w.Code != http.StatusBadGateway {
		t.Errorf("bad url: %d", w.Code)
	}
	// Unreachable target.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if w := get(t, DevProxy("http://"+addr), "GET", "/"); w.Code != http.StatusBadGateway {
		t.Errorf("unreachable: %d", w.Code)
	}
}

func TestDevProxyLoopbackOnly(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "dev")
	}))
	defer backend.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(backend.URL, "http://"))
	for _, target := range []string{"http://127.0.0.1:" + port, "http://localhost:" + port} {
		if w := get(t, DevProxy(target), "GET", "/"); w.Code != 200 || w.Body.String() != "dev" {
			t.Errorf("%s: %d %q", target, w.Code, w.Body.String())
		}
	}
	for _, target := range []string{"http://evil.example", "https://10.0.0.5:5173", "http://192.168.1.2:5173", "http://[2001:db8::1]:80", "http://localhost.evil.example"} {
		w := get(t, DevProxy(target), "GET", "/")
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "loopback") {
			t.Errorf("%s: %d %q, want 502 (not loopback)", target, w.Code, w.Body.String())
		}
	}
	// AllowRemote lifts the restriction (the target is then just unreachable).
	if w := get(t, DevProxy("http://192.0.2.1:9", DevProxyOptions{AllowRemote: true}), "GET", "/"); strings.Contains(w.Body.String(), "loopback") {
		t.Errorf("AllowRemote: %q", w.Body.String())
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "localhost.": true, "app.localhost": true,
		"127.0.0.1": true, "127.1.2.3": true, "::1": true, "[::1]": true, "::ffff:127.0.0.1": true,
		"": false, "0.0.0.0": false, "10.0.0.1": false, "example.com": false,
		"localhost.example.com": false, "::": false, "128.0.0.1": false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}
