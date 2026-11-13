package android

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBodyStash(t *testing.T) {
	now := time.Unix(1000, 0)
	s := &bodyStash{now: func() time.Time { return now }}
	if !s.put(1, "a", "text/plain", []byte("hello")) {
		t.Fatal("put refused")
	}
	if s.put(1, "", "", nil) || s.put(1, strings.Repeat("x", 65), "", nil) {
		t.Fatal("bad id accepted")
	}
	hdrs := []string{"Accept", "*/*", "x-tango-body", " a ", "Content-Type", "text/plain;charset=UTF-8"}
	rest, b, ok := s.takeBody(1, hdrs)
	if !ok || string(b.data) != "hello" || b.ctype != "text/plain" || !slices.Equal(rest, []string{"Accept", "*/*", "Content-Type", "text/plain;charset=UTF-8"}) {
		t.Fatal(rest, b, ok)
	}
	if _, _, ok := s.takeBody(1, hdrs); ok {
		t.Fatal("taken twice")
	}
	// Another surface's id does not match.
	s.put(2, "b", "", []byte("x"))
	if _, ok := s.take(1, "b"); ok {
		t.Fatal("cross-surface take")
	}
	// Expiry and per-surface cleanup.
	now = now.Add(2 * stashTTL)
	s.put(3, "c", "", []byte("y"))
	if _, ok := s.take(2, "b"); ok {
		t.Fatal("expired body kept")
	}
	s.dropSurface(3)
	if s.total != 0 || len(s.m) != 0 {
		t.Fatal(s.total, s.m)
	}
	if !s.stashBase64(4, "d", "application/octet-stream", base64.StdEncoding.EncodeToString([]byte{0, 1, 2})) {
		t.Fatal("base64 refused")
	}
	if s.stashBase64(4, "e", "", "!!") {
		t.Fatal("bad base64 accepted")
	}
	if b, _ := s.take(4, "d"); len(b.data) != 3 {
		t.Fatal(b)
	}
}

func TestSetBody(t *testing.T) {
	r, err := newAssetRequest(context.Background(), "POST", "https://tango.localhost/api", []string{"Content-Type", "text/plain;charset=UTF-8"})
	if err != nil {
		t.Fatal(err)
	}
	setBody(r, stashedBody{data: []byte("a=1"), ctype: "application/x-www-form-urlencoded"})
	if err := r.ParseForm(); err != nil || r.PostForm.Get("a") != "1" || r.ContentLength != 3 {
		t.Fatal(err, r.PostForm, r.ContentLength)
	}
	r2, _ := newAssetRequest(context.Background(), "PUT", "https://tango.localhost/api", []string{"Content-Type", "text/plain"})
	setBody(r2, stashedBody{data: []byte{1, 2}})
	if r2.Header.Get("Content-Type") != "" {
		t.Fatal("content type kept for a body without one")
	}
}

// Same-origin 307 redirects resend the body.
func TestRedirectKeepsBody(t *testing.T) {
	r, _ := newAssetRequest(context.Background(), "POST", "https://tango.localhost/old", nil)
	setBody(r, stashedBody{data: []byte("payload"), ctype: "text/plain"})
	var got string
	serve := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusTemporaryRedirect)
			return
		}
		b, _ := io.ReadAll(r.Body)
		got = r.Method + " " + string(b) + " " + r.Header.Get("Content-Type")
	}
	_, cancel := context.WithCancel(context.Background())
	res := serveAsset(serve, r, cancel, "")
	if res.Status != 200 || got != "POST payload text/plain" {
		t.Fatal(res.Status, got)
	}
}

func TestFlatHeaderSetCookie(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "a=1; Path=/; Expires=Wed, 21 Oct 2026 07:28:00 GMT")
	h.Add("Set-Cookie", "b=2\r\n; HttpOnly")
	h.Add("Vary", "Accept")
	h.Add("Vary", "Origin")
	m := map[string]string{}
	fl := flatHeader(h)
	for i := 0; i < len(fl); i += 2 {
		m[fl[i]] = fl[i+1]
	}
	if m["Set-Cookie"] != "a=1; Path=/; Expires=Wed, 21 Oct 2026 07:28:00 GMT\nb=2  ; HttpOnly" {
		t.Fatalf("%q", m["Set-Cookie"])
	}
	if m["Vary"] != "Accept, Origin" {
		t.Fatalf("%q", m["Vary"])
	}
}

// TestBodyScript runs the page shim with the fetch/XMLHttpRequest wrapper
// in Node (when installed) against a fake host channel, synchronous
// (JavascriptInterface) and asynchronous (WebMessageListener), and checks
// what reaches the stash and what the wrapped request carries.
func TestBodyScript(t *testing.T) {
	for _, mode := range []string{"jsi", "wml"} {
		t.Run(mode, func(t *testing.T) { testBodyScript(t, mode) })
	}
}

func testBodyScript(t *testing.T, mode string) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	tok := strings.Repeat("ab", 16)
	harness := `
const MODE = "` + mode + `", TOK = "` + tok + `";
globalThis.window = globalThis;
window.top = window;
globalThis.document = { currentScript: null };
globalThis.location = { href: "https://tango.localhost/caps.html" };
// Node has no document base URL: resolve relative URLs like a page would.
const R = Request;
globalThis.Request = class extends R { constructor(i, o) { super(typeof i === "string" ? new URL(i, location.href).href : i, o); } };
const stashed = {}, posted = [];
if (MODE === "jsi") {
  window.__tangoAndroid = {
    post(tok, s) { posted.push(tok === TOK ? s : "BAD TOKEN"); },
    body(tok, id, type, b64) { if (tok !== TOK) return false; stashed[id] = { type, b64 }; return true; },
  };
} else {
  const ls = [];
  window.__tangoAndroid = {
    addEventListener(k, f) { if (k === "message") ls.push(f); },
    postMessage(d) {
      const tok = d.slice(1, 33), rest = d.slice(33);
      if (d[0] === "m") { posted.push(tok === TOK ? rest : "BAD TOKEN"); return; }
      const [id, type, b64] = rest.split("\n");
      if (tok === TOK) stashed[id] = { type, b64 };
      setTimeout(() => ls.forEach(f => f({ data: (tok === TOK ? "b1" : "b0") + id })), 1);
    },
  };
}
const sent = [];
window.fetch = async (req) => { sent.push({ url: req.url, method: req.method, id: req.headers.get("X-Tango-Body"),
  ct: req.headers.get("content-type"), len: (await req.arrayBuffer()).byteLength }); return new Response("{}"); };
class XHR { open(m, u) { this.m = m; this.u = u; this.h = {}; } setRequestHeader(k, v) { this.h[k] = v; }
  send(b) { sent.push({ xhr: true, url: this.u, method: this.m, id: this.h["X-Tango-Body"] || null, body: b === undefined ? null : b }); } }
globalThis.XMLHttpRequest = XHR;
` + pageScript(tok, "") + `
const hidden = typeof window.__tangoAndroid;
window.__tangoNative.post("hello");
(async () => {
  const f = new FormData(); f.append("name", "tango ✓"); f.append("file", new Blob([new Uint8Array([1, 2, 3])]), "a.bin");
  await fetch("/api/echo", { method: "POST", body: f });
  await fetch("/api/echo", { method: "POST", body: JSON.stringify({ a: 1 }), headers: { "Content-Type": "application/json" } });
  await fetch("https://example.com/x", { method: "POST", body: "remote" });
  await fetch("/api/get");
  await fetch("/api/echo", { method: "PUT", body: new Uint8Array([9, 8]).buffer });
  const x = new XMLHttpRequest(); x.open("POST", "/api/echo"); x.send("xhr ✓");
  await new Promise(r => setTimeout(r, 20));
  const y = new XMLHttpRequest(); y.open("POST", "/api/echo"); y.setRequestHeader("Content-Type", "text/csv"); y.send(new Blob(["a,b"]));
  await new Promise(r => setTimeout(r, 50));
  console.log(JSON.stringify({ stashed, sent, posted, hidden }));
})();
`
	dir := t.TempDir()
	f := filepath.Join(dir, "h.mjs")
	if err := os.WriteFile(f, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, f).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var res struct {
		Posted  []string
		Hidden  string
		Stashed map[string]struct{ Type, B64 string }
		Sent    []struct {
			XHR             bool
			URL, Method, CT string
			ID              *string
			Len             int
			Body            *string
		}
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(res.Sent) != 7 {
		t.Fatalf("%s", out)
	}
	if len(res.Posted) != 1 || res.Posted[0] != "hello" || res.Hidden != "undefined" {
		t.Fatalf("posted %q, channel global %q after the shim ran", res.Posted, res.Hidden)
	}
	body := func(i int) (string, []byte) {
		t.Helper()
		if res.Sent[i].ID == nil {
			t.Fatalf("request %d has no body id: %s", i, out)
		}
		st, ok := res.Stashed[*res.Sent[i].ID]
		if !ok {
			t.Fatalf("request %d: body not stashed", i)
		}
		b, _ := base64.StdEncoding.DecodeString(st.B64)
		return st.Type, b
	}
	// FormData: multipart with the boundary of its Content-Type, the
	// request itself carries an empty body.
	ct, b := body(0)
	mt, params, _ := mime.ParseMediaType(ct)
	if mt != "multipart/form-data" || res.Sent[0].Len != 0 {
		t.Fatal(ct, res.Sent[0])
	}
	form, err := multipart.NewReader(strings.NewReader(string(b)), params["boundary"]).ReadForm(1 << 20)
	if err != nil || form.Value["name"][0] != "tango ✓" || form.File["file"][0].Size != 3 {
		t.Fatal(err, form)
	}
	if ct, b := body(1); ct != "application/json" || string(b) != `{"a":1}` {
		t.Fatal(ct, string(b))
	}
	if res.Sent[2].ID != nil || res.Sent[3].ID != nil {
		t.Fatal("remote or GET request got a body id")
	}
	if ct, b := body(4); ct != "" || len(b) != 2 || res.Sent[4].Method != "PUT" {
		t.Fatal(ct, b)
	}
	if ct, b := body(5); ct != "text/plain;charset=UTF-8" || string(b) != "xhr ✓" || *res.Sent[5].Body != "" {
		t.Fatal(ct, string(b))
	}
	if ct, b := body(6); ct != "text/csv" || string(b) != "a,b" {
		t.Fatal(ct, string(b))
	}
}
