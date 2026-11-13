package jsapi

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
)

func TestWebForbiddenByDefault(t *testing.T) {
	e := setup(t, Options{})
	p := e.open()
	for _, m := range []string{"web.find", "web.screenshot", "web.cookies", "web.cancelDownload", "web.setUserAgent"} {
		p.fails(CodeForbidden, m, "x")
	}
	if Defaults&WebAll != 0 || All&WebAll != WebAll {
		t.Fatal("Web caps must be off by default and part of All")
	}
}

func TestWebFindAndCapture(t *testing.T) {
	e := setup(t, Options{Allow: WebFind | WebCapture})
	p := e.open()
	p.fs.SetPageText("one two One")

	var r findResultJS
	json.Unmarshal(p.ok("web.find", "one", map[string]any{}), &r)
	if r.Matches != 2 || r.Active != 1 || !r.Found {
		t.Fatalf("find %+v", r)
	}
	json.Unmarshal(p.ok("web.findNext"), &r)
	if r.Active != 2 {
		t.Fatalf("findNext %+v", r)
	}
	json.Unmarshal(p.ok("web.find", "one", map[string]any{"matchCase": true}), &r)
	if r.Matches != 1 {
		t.Fatalf("matchCase %+v", r)
	}
	p.ok("web.stopFind")

	var b64 string
	json.Unmarshal(p.ok("web.printToPDF", map[string]any{"landscape": true}), &b64)
	if b, _ := base64.StdEncoding.DecodeString(b64); string(b) != string(fake.FakePDF) {
		t.Fatalf("pdf %q", b64)
	}
	json.Unmarshal(p.ok("web.screenshot"), &b64)
	if b, _ := base64.StdEncoding.DecodeString(b64); string(b) != string(fake.FakePNG) {
		t.Fatalf("png %q", b64)
	}
	p.fails(CodeForbidden, "web.cookies")
}

func TestWebCookies(t *testing.T) {
	e := setup(t, Options{Allow: WebCookies})
	p := e.open()
	p.ok("web.setCookie", map[string]any{"name": "a", "value": "1", "domain": "example.com", "expires": 4102444800000, "sameSite": "lax", "httpOnly": true})
	p.fails("bad_args", "web.setCookie", map[string]any{"name": "a"})
	p.fails("bad_args", "web.setCookie", map[string]any{"name": "a", "domain": "x", "sameSite": "weird"})

	var cs []cookieJS
	json.Unmarshal(p.ok("web.cookies", "https://example.com/"), &cs)
	if len(cs) != 1 || cs[0].Value != "1" || cs[0].SameSite != "lax" || !cs[0].HTTPOnly || cs[0].Expires == nil || *cs[0].Expires != 4102444800000 {
		t.Fatalf("cookies %+v", cs)
	}
	json.Unmarshal(p.ok("web.cookies", "https://other.test/"), &cs)
	if len(cs) != 0 {
		t.Fatalf("other site %+v", cs)
	}
	p.fails("bad_args", "web.cookies", "::nonsense")
	p.ok("web.deleteCookie", map[string]any{"name": "a", "domain": "example.com"})
	json.Unmarshal(p.ok("web.cookies"), &cs)
	if len(cs) != 0 {
		t.Fatalf("after delete %+v", cs)
	}
	p.ok("web.clearCookies", "example.com")
	p.ok("web.clearData", []string{"cache", "storage"})
	p.fails("bad_args", "web.clearData", []string{"nope"})
	if got := p.fs.ClearedData(); len(got) != 1 || got[0] != tango.DataCache|tango.DataStorage {
		t.Fatalf("cleared %v", got)
	}
}

func TestWebDownloadEventsAndUA(t *testing.T) {
	e := setup(t, Options{Allow: WebDownloads | WebControl})
	p := e.open()
	other := e.open()
	e.fb.DownloadEvent(p.s.ID(), tango.DownloadEvent{ID: 3, State: tango.DownloadDone, Received: 4, Total: 4, Path: "/tmp/x"})
	var d downloadJS
	json.Unmarshal(p.event("web.download", 1), &d)
	if d.ID != 3 || d.State != "done" || d.Path != "/tmp/x" {
		t.Fatalf("event %+v", d)
	}
	if other.count("web.download") != 0 {
		t.Fatal("download event sent to another page")
	}
	p.ok("web.cancelDownload", 3)
	if got := p.fs.CanceledDownloads(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("canceled %v", got)
	}
	p.ok("web.setUserAgent", "Page/1")
	if p.fs.UserAgent() != "Page/1" {
		t.Fatal("user agent not set")
	}
}
