package android

import (
	"fmt"
	"testing"
)

func testTokens(navURL string) *docTokens {
	d := newDocTokens("https://tango.localhost", navURL)
	n := 0
	d.gen = func() string { n++; return fmt.Sprintf("%032d", n) }
	return d
}

func TestTokenPerDocument(t *testing.T) {
	d := testTokens("https://tango.localhost/")

	// Go serves the first page with the script inside: its token works as
	// soon as the response leaves, and stays when the document starts.
	t1 := d.serve("https://tango.localhost/")
	if t1 == "" {
		t.Fatal("no token for the asset origin")
	}
	d.served(t1, "https://tango.localhost/")
	if o, ok := d.origin(t1); !ok || o != "https://tango.localhost" {
		t.Fatal("served token refused before the page started", o, ok)
	}
	if js := d.begin("https://tango.localhost/"); js != "" {
		t.Fatal("served page injected again")
	}
	if _, ok := d.origin(t1); !ok {
		t.Fatal("current token refused")
	}

	// The next page gets a new token; the old one is refused from then on.
	t2 := d.serve("https://tango.localhost/b.html")
	d.served(t2, "https://tango.localhost/b.html")
	if t2 == t1 {
		t.Fatal("token reused")
	}
	if _, ok := d.origin(t1); !ok {
		t.Fatal("old page cut off before the new one started")
	}
	d.begin("https://tango.localhost/b.html")
	if _, ok := d.origin(t1); ok {
		t.Fatal("old token accepted after the next page started")
	}
	if _, ok := d.origin(t2); !ok {
		t.Fatal("new token refused")
	}

	// A served response that never becomes the document (a download, or
	// another page started instead) does not get a valid token.
	t3 := d.serve("https://tango.localhost/file.html")
	d.served(t3, "https://tango.localhost/file.html")
	t4 := d.begin("https://tango.localhost/c.html") // not served with the script: inject
	if t4 == "" || t4 == t2 || t4 == t3 {
		t.Fatal("no fresh token for a trusted page Go did not inject", t4)
	}
	for _, old := range []string{t2, t3} {
		if _, ok := d.origin(old); ok {
			t.Fatal("stale token accepted", old)
		}
	}
	if _, ok := d.origin(""); ok {
		t.Fatal("empty token accepted")
	}
}

func TestTokenUntrustedOrigin(t *testing.T) {
	d := testTokens("https://tango.localhost/")
	t1 := d.begin("https://tango.localhost/")

	// The user follows a link to another origin: no script, no token, and
	// the app page's token is dead.
	if js := d.begin("http://127.0.0.1:8000/other.html"); js != "" {
		t.Fatal("untrusted page got a token")
	}
	if _, ok := d.origin(t1); ok {
		t.Fatal("app page token still valid on an untrusted page")
	}
	if tok := d.serve("http://127.0.0.1:8000/x"); tok != "" {
		t.Fatal("token served for an untrusted origin")
	}
	for _, u := range []string{"about:blank", "data:text/html,x", "file:///sdcard/a.html", ""} {
		if d.begin(u) != "" {
			t.Fatal("token for", u)
		}
	}

	// Back on the app page, the bridge works again with a new token.
	t2 := d.begin("https://tango.localhost/")
	if o, ok := d.origin(t2); t2 == "" || !ok || o != "https://tango.localhost" {
		t.Fatal("app page after the untrusted one", t2, o, ok)
	}

	// An origin the core navigated to is trusted, until it navigates
	// elsewhere.
	d.navigated("http://10.0.2.2:5173/")
	t3 := d.begin("http://10.0.2.2:5173/")
	if o, ok := d.origin(t3); t3 == "" || !ok || o != "http://10.0.2.2:5173" {
		t.Fatal("navigated origin", t3, o, ok)
	}
	d.navigated("https://tango.localhost/")
	if d.begin("http://10.0.2.2:5173/") != "" {
		t.Fatal("previous navigation's origin still trusted")
	}
	if d.begin("https://tango.localhost/") == "" {
		t.Fatal("asset origin not trusted")
	}
}

func TestSameOrigin(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"https://tango.localhost", "https://tango.localhost", true},
		{"https://tango.localhost", "https://Tango.localhost/", true},
		{"https://x.test:443", "https://x.test", true},
		{"http://10.0.2.2:5173", "http://10.0.2.2:5173", true},
		{"http://10.0.2.2:5173", "http://10.0.2.2", false},
		{"https://tango.localhost", "http://tango.localhost", false},
		{"https://tango.localhost", "https://evil.test", false},
		{"", "", false},
		{"https://tango.localhost", "null", false},
	} {
		if got := sameOrigin(c.a, c.b); got != c.want {
			t.Errorf("sameOrigin(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestParseWire(t *testing.T) {
	tok := fmt.Sprintf("%032d", 7)
	k, gotTok, p, ok := parseWire("m" + tok + `{"x":1}`)
	if !ok || k != wireMessage || gotTok != tok || p != `{"x":1}` {
		t.Fatal(k, gotTok, p, ok)
	}
	k, _, p, ok = parseWire("b" + tok + "id1\ntext/plain\nQUJD")
	if !ok || k != wireBody {
		t.Fatal(k, ok)
	}
	if id, ct, b64, ok := parseWireBody(p); !ok || id != "id1" || ct != "text/plain" || b64 != "QUJD" {
		t.Fatal(id, ct, b64, ok)
	}
	for _, bad := range []string{"", "m", "m" + tok[:10], "x" + tok + "hi"} {
		if _, _, _, ok := parseWire(bad); ok {
			t.Errorf("parseWire(%q) accepted", bad)
		}
	}
	if _, _, _, ok := parseWireBody("noid"); ok {
		t.Error("body without separators accepted")
	}
}

func TestCheckWire(t *testing.T) {
	d := testTokens("https://tango.localhost/")
	tok := d.begin("https://tango.localhost/")
	if m, ok := d.check("https://tango.localhost", "m"+tok+"hi"); !ok || !m.valid || m.origin != "https://tango.localhost" || m.payload != "hi" {
		t.Fatal("app page message refused", m, ok)
	}
	// The right token from another origin (leaked to a page elsewhere).
	if m, ok := d.check("http://127.0.0.1:8000", "m"+tok+"hi"); !ok || m.valid {
		t.Fatal("message from an untrusted origin accepted", m)
	}
	// A wrong token from the right origin.
	if m, _ := d.check("https://tango.localhost", "m"+fmt.Sprintf("%032d", 99)+"hi"); m.valid {
		t.Fatal("wrong token accepted")
	}
	// The previous document's token after the next document started.
	d.begin("https://tango.localhost/next.html")
	if m, _ := d.check("https://tango.localhost", "m"+tok+"hi"); m.valid {
		t.Fatal("old token accepted")
	}
}
