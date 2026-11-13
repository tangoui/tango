package android

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

func TestInjectHTML(t *testing.T) {
	const js = "X"
	tag := "<script>X</script>"
	cases := []struct{ in, want string }{
		{"<!doctype html><html><head><title>t</title></head></html>", "<!doctype html><html><head>" + tag + "<title>t</title></head></html>"},
		{"<!DOCTYPE html>\n<HTML lang=\"en\">\n<HEAD data-x='a>b'>\n<script>1</script>", "<!DOCTYPE html>\n<HTML lang=\"en\">\n<HEAD data-x='a>b'>" + tag + "\n<script>1</script>"},
		{"<!-- <head> --><html><body>x", "<!-- <head> --><html>" + tag + "<body>x"},
		{"<header>x</header>", tag + "<header>x</header>"},
		{"\xEF\xBB\xBF<!doctype html><p>x", "\xEF\xBB\xBF<!doctype html>" + tag + "<p>x"},
		{"plain text", tag + "plain text"},
		{"", tag},
		{"<html", tag + "<html"},
	}
	for _, c := range cases {
		if got := string(injectHTML([]byte(c.in), js)); got != c.want {
			t.Errorf("injectHTML(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestInlineScript(t *testing.T) {
	got := inlineScript(`a="</SCRIPT><!--é😀"`)
	want := `a="<\/SCRIPT><\!--` + "\\u00e9\\ud83d\\ude00" + `"`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestPageScript(t *testing.T) {
	s := pageScript("tok", "BRIDGE")
	if !strings.Contains(s, `t = "tok"`) || !strings.HasSuffix(s, "BRIDGE") || !strings.Contains(s, "delete window."+jsInterface) {
		t.Fatal(s)
	}
	if a, b := newToken(), newToken(); a == b || len(a) != 32 {
		t.Fatal(a, b)
	}
}

func TestNewAssetRequest(t *testing.T) {
	r, err := newAssetRequest(context.Background(), "", "https://tango.localhost/a/b?q=1",
		[]string{"accept", "text/html", "If-None-Match", `"x"`, "Range", "bytes=0-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "GET" || r.URL.Path != "/a/b" || r.RequestURI != "/a/b?q=1" || r.Host != "tango.localhost" {
		t.Fatalf("%+v", r)
	}
	if r.Header.Get("Accept") != "text/html" || r.Header.Get("If-None-Match") != "" || r.Header.Get("Range") != "bytes=0-1" {
		t.Fatal(r.Header)
	}
	if b, _ := io.ReadAll(r.Body); len(b) != 0 {
		t.Fatal(b)
	}
}

func serveReq(t *testing.T, h http.HandlerFunc, url, script string) assetResult {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r, err := newAssetRequest(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return serveAsset(serveFunc(h), r, cancel, script)
}

func TestServeAssetWhole(t *testing.T) {
	res := serveReq(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-A", "1")
		w.Header().Add("X-A", "2")
		io.WriteString(w, "<html><head></head></html>")
	}, "https://tango.localhost/", "S")
	if res.Status != 200 || res.Reason != "OK" || res.MIME != "text/html" || res.Encoding != "utf-8" || !res.Injected || res.Stream != nil {
		t.Fatalf("%+v", res)
	}
	if string(res.Body) != "<html><head><script>S</script></head></html>" {
		t.Fatalf("%q", res.Body)
	}
	if !slices.Equal(res.Headers, []string{"X-A", "1, 2"}) {
		t.Fatal(res.Headers)
	}

	res = serveReq(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		io.WriteString(w, "{}")
	}, "https://tango.localhost/x", "S")
	// The handler may still be running when serveAsset looks: drain a stream.
	body := res.Body
	if res.Stream != nil {
		body = readAll(res.Stream)
	}
	if res.Status != 201 || res.MIME != "application/json" || res.Injected || string(body) != "{}" {
		t.Fatalf("%+v %q", res, body)
	}

	res = serveReq(t, func(w http.ResponseWriter, r *http.Request) {}, "https://tango.localhost/empty", "")
	if res.Status != 200 || res.MIME != "application/octet-stream" || len(res.Body) != 0 {
		t.Fatalf("%+v", res)
	}

	res = serveReq(t, func(w http.ResponseWriter, r *http.Request) { panic("boom") }, "https://tango.localhost/p", "")
	if res.Status != 500 {
		t.Fatalf("%+v", res)
	}
}

func readAll(w *assetWriter) []byte {
	var out []byte
	buf := make([]byte, 3)
	for {
		n, eof := w.Read(buf)
		out = append(out, buf[:n]...)
		if eof {
			return out
		}
	}
}

func TestServeAssetStream(t *testing.T) {
	release := make(chan struct{})
	res := serveReq(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: 1\n\n")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "data: 2\n\n")
	}, "https://tango.localhost/events", "S")
	if res.Stream == nil || res.MIME != "text/event-stream" || res.Injected {
		t.Fatalf("%+v", res)
	}
	close(release)
	if got := string(readAll(res.Stream)); got != "data: 1\n\ndata: 2\n\n" {
		t.Fatalf("%q", got)
	}
}

func TestServeAssetStreamClose(t *testing.T) {
	werr := make(chan error, 1)
	var ctxDone <-chan struct{}
	res := serveReq(t, func(w http.ResponseWriter, r *http.Request) {
		ctxDone = r.Context().Done()
		w.WriteHeader(200)
		chunk := make([]byte, 64<<10)
		for {
			if _, err := w.Write(chunk); err != nil {
				werr <- err
				return
			}
		}
	}, "https://tango.localhost/big", "")
	if res.Stream == nil {
		t.Fatalf("%+v", res)
	}
	res.closeStream()
	select {
	case err := <-werr:
		if err != errClosed {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer not released")
	}
	<-ctxDone
}

func TestServeAssetRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/new/", http.StatusFound) })
	mux.HandleFunc("/new/", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "new "+r.URL.Path) })
	mux.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	res := serveReq(t, mux.ServeHTTP, "https://tango.localhost/old", "")
	body := res.Body
	if res.Stream != nil {
		body = readAll(res.Stream)
	}
	if res.Status != 200 || string(body) != "new /new/" {
		t.Fatalf("%+v %q", res, body)
	}
	for _, p := range []string{"/away", "/loop"} {
		if res := serveReq(t, mux.ServeHTTP, "https://tango.localhost"+p, ""); res.Status != http.StatusBadGateway {
			t.Fatalf("%s: %+v", p, res)
		}
	}
}

func TestSplitContentType(t *testing.T) {
	for _, c := range [][3]string{
		{"text/html; charset=UTF-8", "text/html", "UTF-8"},
		{"Image/PNG", "image/png", ""},
		{"", "application/octet-stream", ""},
		{"text/plain;;bad", "text/plain", ""},
	} {
		if m, e := splitContentType(c[0]); m != c[1] || e != c[2] {
			t.Errorf("%q: %q %q", c[0], m, e)
		}
	}
}

func TestOriginOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://Tango.localhost/x?y": "https://tango.localhost",
		"http://10.0.2.2:5173/":       "http://10.0.2.2:5173",
		"about:blank":                 "",
	} {
		if got := originOf(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestLifecycle(t *testing.T) {
	const (
		S = platform.LifecycleStarted
		F = platform.LifecycleForeground
		B = platform.LifecycleBackground
		T = platform.LifecycleTerminating
	)
	var l lifecycle
	for _, ev := range []platform.LifecycleEvent{S, F} {
		if got := l.report(ev); got != nil {
			t.Fatal(got)
		}
	}
	if got := l.start(); !slices.Equal(got, []platform.LifecycleEvent{S, F}) {
		t.Fatal(got)
	}
	if got := l.start(); got != nil {
		t.Fatal(got)
	}
	if got := l.report(F); got != nil {
		t.Fatal("duplicate foreground", got)
	}
	if got := l.report(B); !slices.Equal(got, []platform.LifecycleEvent{B}) {
		t.Fatal(got)
	}
	if got := l.report(T); !slices.Equal(got, []platform.LifecycleEvent{T}) {
		t.Fatal(got)
	}
	if got := l.report(F); got != nil {
		t.Fatal("after terminating", got)
	}

	var early lifecycle
	early.report(S)
	early.report(F)
	early.report(T)
	if got := early.start(); !slices.Equal(got, []platform.LifecycleEvent{S, T}) {
		t.Fatal(got)
	}
}
