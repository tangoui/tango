package linux

import (
	"io"
	"net/http"
	"reflect"
	"testing"
)

func TestNewAssetRequest(t *testing.T) {
	r, err := newAssetRequest("", "tango://app/index.html?x=1", [][2]string{
		{"accept", "text/html"}, {"X-Multi", "a"}, {"x-multi", "b"}, {"Host", "evil"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != http.MethodGet || r.URL.Scheme != "tango" || r.Host != "app" || r.URL.Path != "/index.html" || r.URL.RawQuery != "x=1" {
		t.Errorf("request = %s %s host=%s", r.Method, r.URL, r.Host)
	}
	if r.RequestURI != "/index.html?x=1" {
		t.Errorf("RequestURI = %q", r.RequestURI)
	}
	if r.Header.Get("Accept") != "text/html" || !reflect.DeepEqual(r.Header["X-Multi"], []string{"a", "b"}) || r.Header.Get("Host") != "" {
		t.Errorf("header = %v", r.Header)
	}

	r, err = newAssetRequest("POST", "tango://app/api", nil, []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	if r.Method != "POST" || string(b) != "hi" || r.ContentLength != 2 {
		t.Errorf("post = %s %q %d", r.Method, b, r.ContentLength)
	}

	if _, err := newAssetRequest("GET", "::bad", nil, nil); err == nil {
		t.Error("want error for bad URI")
	}
}

func TestAssetWriterDefaults(t *testing.T) {
	w := newAssetWriter()
	io.WriteString(w, "<!doctype html><p>hi")
	w.WriteHeader(http.StatusTeapot) // too late: ignored
	resp := w.finish("GET")
	if resp.Status != 200 || resp.Reason != "OK" {
		t.Errorf("status = %d %q", resp.Status, resp.Reason)
	}
	if resp.ContentType != "text/html; charset=utf-8" {
		t.Errorf("content type = %q", resp.ContentType)
	}
	if !reflect.DeepEqual(resp.Headers, [][2]string{{"Content-Length", "20"}}) {
		t.Errorf("headers = %v", resp.Headers)
	}
}

func TestAssetWriterExplicit(t *testing.T) {
	w := newAssetWriter()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Add("Set-Cookie", "a=1")
	w.Header().Add("Set-Cookie", "b=2")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", "999") // corrected from the body
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{}`))
	resp := w.finish("GET")
	if resp.Status != 404 || resp.Reason != "Not Found" || resp.ContentType != "application/json" || string(resp.Body) != "{}" {
		t.Errorf("resp = %+v", resp)
	}
	want := [][2]string{{"Cache-Control", "no-cache"}, {"Content-Length", "2"}, {"Set-Cookie", "a=1"}, {"Set-Cookie", "b=2"}}
	if !reflect.DeepEqual(resp.Headers, want) {
		t.Errorf("headers = %v\nwant %v", resp.Headers, want)
	}
}

func TestAssetWriterEmpty(t *testing.T) {
	w := newAssetWriter()
	w.WriteHeader(http.StatusNoContent)
	resp := w.finish("GET")
	if resp.Status != 204 || resp.ContentType != "" || !reflect.DeepEqual(resp.Headers, [][2]string{{"Content-Length", "0"}}) {
		t.Errorf("resp = %+v", resp)
	}
	// HEAD keeps the handler's Content-Length.
	w = newAssetWriter()
	w.Header().Set("Content-Length", "42")
	resp = w.finish("HEAD")
	if !reflect.DeepEqual(resp.Headers, [][2]string{{"Content-Length", "42"}}) {
		t.Errorf("head headers = %v", resp.Headers)
	}
	// Explicitly empty Content-Type is not sniffed.
	w = newAssetWriter()
	w.Header()["Content-Type"] = nil
	w.Write([]byte("<html>"))
	if ct := w.finish("GET").ContentType; ct != "" {
		t.Errorf("content type = %q", ct)
	}
}

func TestAssetWriterIsHandlerCompatible(t *testing.T) {
	var _ http.ResponseWriter = newAssetWriter()
	var _ http.Flusher = newAssetWriter()
	h := http.NotFoundHandler()
	w := newAssetWriter()
	r, _ := newAssetRequest("GET", "tango://app/missing", nil, nil)
	h.ServeHTTP(w, r)
	if resp := w.finish("GET"); resp.Status != 404 {
		t.Errorf("status = %d", resp.Status)
	}
}

func TestOriginOf(t *testing.T) {
	cases := map[string]string{
		"tango://app/index.html":       "tango://app",
		"TANGO://App/x":                "tango://app",
		"https://example.com:8443/a?b": "https://example.com:8443",
		"about:blank":                  "",
		"":                             "",
		"data:text/html,hi":            "",
	}
	for in, want := range cases {
		if got := originOf(in); got != want {
			t.Errorf("originOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocumentStartScript(t *testing.T) {
	tok := newToken()
	if documentStartScript(tok, "") != nativeShim(tok) {
		t.Error("empty bridge should be the shim alone")
	}
	got := documentStartScript(tok, "window.x=1")
	if got != nativeShim(tok)+"\n;window.x=1" {
		t.Errorf("script = %q", got)
	}
}
