package linux

import (
	"strings"
	"testing"
)

func TestMsgGateTokens(t *testing.T) {
	a, b := newToken(), newToken()
	if len(a) != tokenLen || a == b || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("tokens %q %q", a, b)
	}
	if !strings.Contains(nativeShim(a), `"`+a+`"`) {
		t.Fatal("shim does not carry the token")
	}
	if strings.Contains(nativeShim(a), "window.__tangoToken") || strings.Contains(nativeShim(a), "window."+a) {
		t.Fatal("token exposed on window")
	}
}

// loadDoc runs one main-frame navigation that commits uri and returns the
// token of the new document.
func loadDoc(g *msgGate, uri string) string {
	tok := g.started()
	g.committed(uri)
	g.finished()
	return tok
}

func TestMsgGateRotation(t *testing.T) {
	var g msgGate
	if _, _, ok := g.check(newToken() + "x"); ok {
		t.Fatal("zero gate accepted a message")
	}
	first := g.init()
	if _, _, ok := g.check(first + "x"); ok {
		t.Fatal("accepted before any commit")
	}
	t1 := loadDoc(&g, "tango://app/index.html")
	if t1 == first {
		t.Fatal("load-started did not rotate the token")
	}
	origin, msg, ok := g.check(t1 + `{"tango":"hello"}`)
	if !ok || origin != "tango://app" || msg != `{"tango":"hello"}` {
		t.Fatalf("check = %q %q %v", origin, msg, ok)
	}
	if _, msg, ok := g.check(t1); !ok || msg != "" {
		t.Fatal("empty message with token rejected")
	}
	// No token, a wrong one, a short one, a token not at the start.
	for _, raw := range []string{`{"tango":"hello"}`, newToken() + "x", t1[:10], "x" + t1, ""} {
		if _, _, ok := g.check(raw); ok {
			t.Errorf("accepted %q", raw)
		}
	}

	t2 := loadDoc(&g, "tango://app/next.html")
	if t2 == t1 {
		t.Fatal("same token for two documents")
	}
	if _, _, ok := g.check(t1 + "x"); ok {
		t.Fatal("previous document's token still accepted")
	}
	if _, _, ok := g.check(t2 + "x"); !ok {
		t.Fatal("current token rejected")
	}
}

func TestMsgGateNavigationWindow(t *testing.T) {
	var g msgGate
	g.init()
	ext := loadDoc(&g, "https://evil.example/")
	if o, _, ok := g.check(ext + "x"); !ok || o != "https://evil.example" {
		t.Fatal("external page's own message should carry its own origin")
	}
	// The app navigates home: from load-started to the commit the old
	// document still runs while the view's URI may already be the new one.
	next := g.started()
	for _, tok := range []string{ext, next} {
		if _, _, ok := g.check(tok + "x"); ok {
			t.Fatal("message accepted between load-started and commit")
		}
	}
	g.committed("tango://app/")
	if _, _, ok := g.check(ext + "x"); ok {
		t.Fatal("old document's token accepted after the commit")
	}
	if o, _, ok := g.check(next + "x"); !ok || o != "tango://app" {
		t.Fatal("new document rejected")
	}
}

func TestMsgGateLoadWithoutCommit(t *testing.T) {
	var g msgGate
	g.init()
	cur := loadDoc(&g, "tango://app/")
	// A download or a failed provisional load: started, then finished
	// without a commit. The old document is still there.
	g.started()
	if _, _, ok := g.check(cur + "x"); ok {
		t.Fatal("accepted while pending")
	}
	g.started() // a second navigation before any commit
	g.finished()
	if o, _, ok := g.check(cur + "x"); !ok || o != "tango://app" {
		t.Fatal("document left in place lost its token")
	}
	next := loadDoc(&g, "tango://app/b")
	if _, _, ok := g.check(cur + "x"); ok {
		t.Fatal("old token accepted")
	}
	if _, _, ok := g.check(next + "x"); !ok {
		t.Fatal("new token rejected")
	}
	// The origin restored after an uncommitted load is the old document's,
	// not the URI the view showed while loading.
	var e msgGate
	e.init()
	ext := loadDoc(&e, "https://evil.example/")
	e.started()
	e.finished()
	if o, _, ok := e.check(ext + "x"); !ok || o != "https://evil.example" {
		t.Fatalf("restored origin %q", o)
	}
}

func TestGateDocumentOrigin(t *testing.T) {
	var g msgGate
	g.init()
	if _, ok := g.documentOrigin(); ok {
		t.Fatal("origin before the first commit")
	}
	g.started()
	g.committed("tango://app/index.html")
	if o, ok := g.documentOrigin(); !ok || o != "tango://app" {
		t.Fatalf("committed: %q %v", o, ok)
	}
	g.started() // app-initiated navigation away: nothing to credit until commit
	if o, ok := g.documentOrigin(); ok {
		t.Fatalf("pending navigation credited %q", o)
	}
	g.committed("https://evil.example/")
	if o, _ := g.documentOrigin(); o != "https://evil.example" {
		t.Fatalf("after commit: %q", o)
	}
	g.started()
	g.finished() // the load never committed: the old document is back
	if o, ok := g.documentOrigin(); !ok || o != "https://evil.example" {
		t.Fatalf("restored: %q %v", o, ok)
	}
}

func TestAddRequestOrigin(t *testing.T) {
	get := addRequestOrigin(nil, "GET", "https://evil.example")
	if len(get) != 0 {
		t.Fatalf("GET got %v", get)
	}
	post := addRequestOrigin([][2]string{{"Content-Type", "text/plain"}}, "post", "https://evil.example")
	if len(post) != 2 || post[1] != [2]string{"Origin", "https://evil.example"} {
		t.Fatalf("POST got %v", post)
	}
	kept := addRequestOrigin([][2]string{{"origin", "tango://app"}}, "PUT", "https://evil.example")
	if len(kept) != 1 || kept[0][1] != "tango://app" {
		t.Fatalf("existing Origin replaced: %v", kept)
	}
	if u := addRequestOrigin(nil, "DELETE", ""); len(u) != 1 || u[0][1] != "null" {
		t.Fatalf("unknown sender: %v", u)
	}

	var g msgGate
	g.init()
	g.started()
	g.committed("https://evil.example/form.html")
	g.started() // the page posts a form to the asset origin: it is still the sender
	if o := g.requestOrigin(); o != "https://evil.example" {
		t.Fatalf("pending navigation sender: %q", o)
	}
	g.committed("tango://app/api")
	if o := g.requestOrigin(); o != "tango://app" {
		t.Fatalf("committed: %q", o)
	}
}
