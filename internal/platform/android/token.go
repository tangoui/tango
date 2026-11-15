package android

import (
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
	"sync"
)

// Bridge tokens, one per document.
//
// Every main-frame document of a trusted origin gets the page script with
// a fresh random token baked into its closure (pageScript). Go accepts a
// message or a request body only with the token of the surface's current
// document, and reports the origin that token was issued for, not
// whatever URL the WebView shows by the time the message is handled. When
// the next document starts (Page.Client.onPageStarted → nativeBeginPage)
// the previous token stops working, whether or not the new document gets
// one.
//
// A token put in a main-frame response Go served (asset origin) is
// accepted from the moment the response leaves: the document's first
// scripts can post before onPageStarted reaches the main thread. It
// becomes the current token when that document starts and is forgotten if
// another one starts instead.
//
// Trusted origins are those of the core (Surface.trusts in the root
// package): the asset origin and the origin of the URL the core last
// navigated the surface to (NewSurface, Navigate).

// docToken is the token of one document and the origin it was issued for.
type docToken struct {
	token, origin, url string
}

// docTokens holds one surface's tokens. Its methods may be called from any
// thread: the request thread (served), the JavaBridge thread (origin) and
// the main thread.
type docTokens struct {
	mu        sync.Mutex
	assetOrig string
	navOrigin string
	cur       docToken // the current document's; zero if it has none
	pending   docToken // in a main-frame response not yet started
	gen       func() string
}

func newDocTokens(assetOrigin, navURL string) *docTokens {
	return &docTokens{assetOrig: assetOrigin, navOrigin: originOf(navURL), gen: newToken}
}

// navigated records a navigation by the core, which trusts the new URL's
// origin from now on (and no longer the previous one).
func (d *docTokens) navigated(u string) {
	d.mu.Lock()
	d.navOrigin = originOf(u)
	d.mu.Unlock()
}

// trusts reports whether a document at origin gets the bridge. d.mu held.
func (d *docTokens) trusts(origin string) bool {
	return origin != "" && (origin == d.assetOrig || origin == d.navOrigin)
}

// serve returns a new token for a main-frame response to rawURL that Go
// is about to put the page script in, or "" when that URL's origin is not
// trusted. The caller confirms with served once the script is in.
func (d *docTokens) serve(rawURL string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.trusts(originOf(rawURL)) {
		return ""
	}
	return d.gen()
}

// served records that the response for rawURL carries token: it is valid
// from now on, until another document starts.
func (d *docTokens) served(token, rawURL string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = docToken{token: token, origin: originOf(rawURL), url: rawURL}
}

// begin is called when a main-frame document starts at rawURL. The
// previous document's token stops working. If Go served this document
// with the script inside, its token becomes current and begin returns "".
// Otherwise a trusted document gets a new token, which begin returns for
// the host to inject; an untrusted one gets nothing.
func (d *docTokens) begin(rawURL string) string {
	tok, _ := d.beginDoc(rawURL)
	return tok
}

// beginDoc is begin, also returning the pending token it made current
// ("" if it promoted none), for the lanes (beginPage, lanes.go).
func (d *docTokens) beginDoc(rawURL string) (inject, promoted string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.pending
	d.pending = docToken{}
	if p.token != "" && p.url == rawURL {
		d.cur = p
		return "", p.token
	}
	d.cur = docToken{}
	origin := originOf(rawURL)
	if !d.trusts(origin) {
		return "", ""
	}
	d.cur = docToken{token: d.gen(), origin: origin, url: rawURL}
	return d.cur.token, ""
}

// origin returns the origin of the document token was issued to, if it is
// still valid.
func (d *docTokens) origin(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range []docToken{d.cur, d.pending} {
		if t.token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(t.token)) == 1 {
			return t.origin, true
		}
	}
	return "", false
}

// rawToken is the 16-byte form of a token (its 32 hex characters decoded),
// as lane packets carry it (lanes.go); nil if tok is not a token.
func rawToken(tok string) []byte {
	if len(tok) != tokenLen {
		return nil
	}
	b, err := hex.DecodeString(tok)
	if err != nil {
		return nil
	}
	return b
}

// originRaw is origin for a token in its 16-byte form (rawToken).
func (d *docTokens) originRaw(raw []byte) (string, bool) {
	if len(raw) != tokenLen/2 {
		return "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range []docToken{d.cur, d.pending} {
		if r := rawToken(t.token); r != nil && subtle.ConstantTimeCompare(raw, r) == 1 {
			return t.origin, true
		}
	}
	return "", false
}

// sameOrigin compares a token's origin with the source origin the WebView
// reported for a message (WebMessageListener), which omits default ports.
func sameOrigin(a, b string) bool {
	return a != "" && normOrigin(a) == normOrigin(b)
}

func normOrigin(o string) string {
	u, err := url.Parse(strings.TrimSuffix(o, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme, host, port := strings.ToLower(u.Scheme), strings.ToLower(u.Hostname()), u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	switch {
	case port != "":
		host = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"): // IPv6
		host = "[" + host + "]"
	}
	return scheme + "://" + host
}

// Messages from a WebMessageListener (Page.java) are strings: a kind
// byte, the 32-character token, then the payload.
const (
	wireMessage = 'm' // payload: the message for Host.OnMessage
	wireBody    = 'b' // payload: id "\n" content type "\n" base64 body
	tokenLen    = 32
)

// parseWire splits a WebMessageListener message.
func parseWire(data string) (kind byte, token, payload string, ok bool) {
	if len(data) < 1+tokenLen {
		return 0, "", "", false
	}
	kind = data[0]
	if kind != wireMessage && kind != wireBody {
		return 0, "", "", false
	}
	return kind, data[1 : 1+tokenLen], data[1+tokenLen:], true
}

// parseWireBody splits a wireBody payload.
func parseWireBody(p string) (id, ctype, b64 string, ok bool) {
	id, rest, ok1 := strings.Cut(p, "\n")
	ctype, b64, ok2 := strings.Cut(rest, "\n")
	return id, ctype, b64, ok1 && ok2 && id != ""
}

// wireMsg is a WebMessageListener message checked against the tokens.
type wireMsg struct {
	kind    byte
	origin  string // the token's document's, when valid
	payload string
	valid   bool // the token is current and was issued to sourceOrigin
}

// check parses data from a main-frame WebMessageListener message that the
// WebView says came from sourceOrigin. ok is false for malformed data.
func (d *docTokens) check(sourceOrigin, data string) (m wireMsg, ok bool) {
	kind, token, payload, ok := parseWire(data)
	if !ok {
		return m, false
	}
	origin, valid := d.origin(token)
	valid = valid && sameOrigin(origin, sourceOrigin)
	return wireMsg{kind: kind, origin: origin, payload: payload, valid: valid}, true
}
