//go:build darwin && !ios

package darwin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestBuildRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := buildRequest(ctx, nativeRequest{
		Method: "POST",
		URL:    "tango://app/api/echo?x=1",
		Header: [][2]string{{"Content-Type", "text/plain"}, {"X-A", "b"}},
		Body:   []byte("ping"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.URL.Scheme != "tango" || r.Host != "app" || r.URL.Path != "/api/echo" || r.RequestURI != "/api/echo?x=1" {
		t.Fatalf("bad request: %s %s host=%s uri=%s", r.Method, r.URL, r.Host, r.RequestURI)
	}
	if r.URL.Query().Get("x") != "1" || r.Header.Get("X-A") != "b" || r.ContentLength != 4 {
		t.Fatalf("query/header/len: %v %v %d", r.URL.Query(), r.Header, r.ContentLength)
	}
	b, _ := io.ReadAll(r.Body)
	if string(b) != "ping" {
		t.Fatalf("body %q", b)
	}
	cancel()
	if r.Context().Err() == nil {
		t.Fatal("context not propagated")
	}
}

func TestBuildRequestDefaults(t *testing.T) {
	r, err := buildRequest(context.Background(), nativeRequest{URL: "tango://app"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != http.MethodGet || r.URL.Path != "/" || r.Body != http.NoBody {
		t.Fatalf("defaults: %s %q %v", r.Method, r.URL.Path, r.Body)
	}
	if _, err := buildRequest(context.Background(), nativeRequest{URL: "tango://app/%zz"}); err == nil {
		t.Fatal("want error for bad URL")
	}
}

type fakeSink struct{ ops []string }

func (f *fakeSink) respond(status int, h http.Header) {
	f.ops = append(f.ops, "respond "+http.StatusText(status)+" "+h.Get("Content-Type")+" len="+h.Get("Content-Length"))
}
func (f *fakeSink) data(p []byte) { f.ops = append(f.ops, "data "+string(p)) }
func (f *fakeSink) finish()       { f.ops = append(f.ops, "finish") }

func TestAssetWriterBuffered(t *testing.T) {
	s := &fakeSink{}
	w := newAssetWriter(s, "/index.html")
	io.WriteString(w, "<p>hi</p>")
	w.finish()
	want := []string{"respond OK text/html; charset=utf-8 len=9", "data <p>hi</p>", "finish"}
	if !reflect.DeepEqual(s.ops, want) {
		t.Fatalf("ops %q, want %q", s.ops, want)
	}
	if _, err := w.Write([]byte("x")); !errors.Is(err, errResponseDone) {
		t.Fatalf("write after finish: %v", err)
	}
}

func TestAssetWriterStreaming(t *testing.T) {
	s := &fakeSink{}
	w := newAssetWriter(s, "/s")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusAccepted)
	w.WriteHeader(http.StatusTeapot) // ignored: status already set
	io.WriteString(w, "a")
	w.Flush()
	io.WriteString(w, "b")
	w.finish()
	w.finish()
	want := []string{"respond Accepted text/plain len=", "data a", "data b", "finish"}
	if !reflect.DeepEqual(s.ops, want) {
		t.Fatalf("ops %q, want %q", s.ops, want)
	}
}

func TestAssetWriterLargeWritesStream(t *testing.T) {
	s := &fakeSink{}
	w := newAssetWriter(s, "/big.bin")
	w.Write(make([]byte, streamChunk+1))
	if len(s.ops) != 2 {
		t.Fatalf("expected respond+data before finish, got %d ops", len(s.ops))
	}
}

func TestAssetWriterNotFoundAndSniff(t *testing.T) {
	s := &fakeSink{}
	w := newAssetWriter(s, "/missing")
	http.NotFound(w, &http.Request{})
	w.finish()
	if !strings.HasPrefix(s.ops[0], "respond Not Found text/plain") {
		t.Fatalf("ops %q", s.ops)
	}

	s = &fakeSink{}
	w = newAssetWriter(s, "/noext")
	io.WriteString(w, "<!doctype html><html></html>")
	w.finish()
	if !strings.HasPrefix(s.ops[0], "respond OK text/html") {
		t.Fatalf("sniff: %q", s.ops)
	}

	s = &fakeSink{}
	w = newAssetWriter(s, "/empty")
	w.WriteHeader(http.StatusNoContent)
	w.finish()
	if s.ops[0] != "respond No Content  len=" {
		t.Fatalf("204: %q", s.ops)
	}
}

func TestAssetWriterStopped(t *testing.T) {
	s := &fakeSink{}
	w := newAssetWriter(s, "/x")
	io.WriteString(w, "a")
	w.stop()
	if _, err := w.Write([]byte("b")); !errors.Is(err, context.Canceled) {
		t.Fatalf("write after stop: %v", err)
	}
	w.Flush()
	w.finish()
	if len(s.ops) != 0 {
		t.Fatalf("stopped task got %q", s.ops)
	}
}

func TestParseAccelerator(t *testing.T) {
	cases := []struct {
		in   string
		key  string
		mods uint
	}{
		{"CmdOrCtrl+Shift+N", "n", modCommand | modShift},
		{"Cmd+Q", "q", modCommand},
		{"Ctrl+Alt+Delete", "", modControl | modOption},
		{"Option+F5", "", modOption},
		{"CmdOrCtrl++", "+", modCommand},
		{"CmdOrCtrl+Plus", "+", modCommand},
		{"Escape", "\x1b", 0},
		{"Super+Up", "", modCommand},
		{"cmd+,", ",", modCommand},
		{"", "", 0},
	}
	for _, c := range cases {
		key, mods, err := parseAccelerator(c.in)
		if err != nil || key != c.key || mods != c.mods {
			t.Errorf("%q: got (%q, %#x, %v), want (%q, %#x)", c.in, key, mods, err, c.key, c.mods)
		}
	}
	for _, bad := range []string{"Cmd+Hyper+K", "Cmd+Foo", "Cmd+Shift"} {
		if _, _, err := parseAccelerator(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestMenuRoles(t *testing.T) {
	// Every role named in the platform contract must be handled.
	for _, r := range []string{"quit", "about", "hide", "copy", "paste", "cut", "selectAll", "undo", "redo", "minimize", "close"} {
		ri, ok := menuRoles[r]
		if !ok || !strings.HasSuffix(ri.selector, ":") || ri.label == "" {
			t.Errorf("role %q: %+v", r, ri)
		}
		if ri.accel != "" {
			if _, _, err := parseAccelerator(ri.accel); err != nil {
				t.Errorf("role %q accel: %v", r, err)
			}
		}
	}
	for _, r := range []string{"appMenu", "editMenu", "windowMenu"} {
		rm, ok := roleMenus[r]
		if !ok || rm.title == "" {
			t.Fatalf("menu role %q missing", r)
		}
		for _, item := range rm.items {
			if _, ok := menuRoles[item]; item != "" && !ok {
				t.Errorf("menu role %q lists unknown role %q", r, item)
			}
		}
	}
	if menuRoles["copy"].selector != "copy:" || menuRoles["quit"].selector != "terminate:" {
		t.Error("standard selectors changed")
	}
	if got := roleLabel(menuRoles["quit"].label, "My App"); got != "Quit My App" {
		t.Errorf("label %q", got)
	}
}

func TestFilterExtensions(t *testing.T) {
	exts, all := filterExtensions([]platform.FileFilter{
		{Name: "Images", Patterns: []string{"*.png", "*.JPG", ".gif", "png"}},
		{Name: "Bad", Patterns: []string{"", "*.t?t"}},
	})
	if all || !reflect.DeepEqual(exts, []string{"png", "jpg", "gif"}) {
		t.Fatalf("got %v %v", exts, all)
	}
	if _, all := filterExtensions([]platform.FileFilter{{Patterns: []string{"*.txt"}}, {Patterns: []string{"*"}}}); !all {
		t.Fatal("* should allow all")
	}
	if _, all := filterExtensions(nil); !all {
		t.Fatal("no filters should allow all")
	}
}

func TestAlertMapping(t *testing.T) {
	if i, err := alertResult(1001, 2); i != 1 || err != nil {
		t.Fatalf("got %d %v", i, err)
	}
	for _, resp := range []int{0, 999, 1002, -1000} {
		if _, err := alertResult(resp, 2); !errors.Is(err, platform.ErrCanceled) {
			t.Errorf("%d: %v", resp, err)
		}
	}
	if alertStyle(platform.MessageError) != nsAlertStyleCritical || alertStyle(platform.MessageWarning) != nsAlertStyleWarning ||
		alertStyle(platform.MessageQuestion) != nsAlertStyleInformational {
		t.Error("alert styles")
	}
}

func TestFormatOrigin(t *testing.T) {
	if o := formatOrigin("tango", "app", 0); o != "tango://app" {
		t.Error(o)
	}
	if o := formatOrigin("http", "localhost", 8080); o != "http://localhost:8080" {
		t.Error(o)
	}
}

func TestNewIsBackend(t *testing.T) {
	b := New()
	if b.AssetOrigin() != "tango://app" {
		t.Fatal(b.AssetOrigin())
	}
	if _, ok := b.(platform.Clipboard); !ok {
		t.Fatal("no Clipboard")
	}
	if _, ok := b.(platform.Dialogs); !ok {
		t.Fatal("no Dialogs")
	}
	if _, ok := b.(platform.AppMenu); !ok {
		t.Fatal("no AppMenu")
	}
	b.Wake() // before Init: must not crash
}
