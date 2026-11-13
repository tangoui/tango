package linux

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// Script message authentication.
//
// webkit.messageHandlers.tango is visible to every frame of the page, and
// in the WebKitGTK 4.0/4.1 API script-message-received does not say which
// frame (or which document) posted. Instead of trusting the main frame's
// current URI, every main-frame document gets its own random token: the
// document-start user script (injected in the top frame only) keeps it in
// a closure that __tangoNative.post prepends to every message. Sub-frames
// cannot read the top frame's closure, cross-origin frames cannot reach the
// top frame's __tangoNative at all, and a document that is being navigated
// away from loses its token when the navigation starts.
//
// The lifecycle, driven by load-changed (main frame only):
//
//	load-started    a new token is installed for the next document; the
//	                current one is revoked and every message is dropped
//	load-committed  the installed token becomes current; the origin is the
//	                committed URI's (not the provisional one)
//	load-finished   without a commit since load-started (download, policy
//	                interruption, failed provisional load): the old
//	                document is still there, so its token and origin return
//
// Same-document navigations (fragments, pushState) emit no load-changed and
// keep the token; they cannot change the origin.

// tokenLen is the length of a message token (hex of 16 random bytes).
const tokenLen = 32

// newToken returns a fresh random token.
func newToken() string {
	var b [tokenLen / 2]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("tango/linux: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// msgGate decides which script messages come from the committed main-frame
// document. The zero value accepts nothing; main thread only.
type msgGate struct {
	installed string // token in the user script, handed to the next document
	current   string // token of the committed document; "" while navigating
	origin    string // origin of the committed document

	pending               bool   // load-started seen, no commit yet
	prevToken, prevOrigin string // the document before load-started
}

// init sets the token of the first document (before the first load).
func (g *msgGate) init() string {
	g.installed = newToken()
	return g.installed
}

// started handles WEBKIT_LOAD_STARTED and returns the token to install
// for the document the navigation will commit.
func (g *msgGate) started() string {
	if !g.pending {
		g.prevToken, g.prevOrigin = g.current, g.origin
		g.pending = true
	}
	g.current, g.origin = "", ""
	g.installed = newToken()
	return g.installed
}

// committed handles WEBKIT_LOAD_COMMITTED; uri is the committed URI.
func (g *msgGate) committed(uri string) {
	g.current, g.origin = g.installed, originOf(uri)
	g.pending, g.prevToken, g.prevOrigin = false, "", ""
}

// finished handles WEBKIT_LOAD_FINISHED. A load that finished without
// committing left the previous document in place.
func (g *msgGate) finished() {
	if g.pending {
		g.current, g.origin = g.prevToken, g.prevOrigin
		g.pending, g.prevToken, g.prevOrigin = false, "", ""
	}
}

// documentOrigin returns the origin recorded when the current document
// committed. ok is false before the first commit and while a navigation is
// pending, when no document can be credited with a request.
func (g *msgGate) documentOrigin() (origin string, ok bool) {
	if g.pending || g.current == "" {
		return "", false
	}
	return g.origin, true
}

// requestOrigin names the document that issued a request now: the
// committed one, or while a navigation is pending the previous committed
// document (a form POST navigation is sent by the page being left, before
// the new one commits). "" when no document is known.
func (g *msgGate) requestOrigin() string {
	if g.pending {
		return g.prevOrigin
	}
	return g.origin
}

// check authenticates raw (token + message) and returns the sender's
// origin and the message. ok is false for messages without the current
// token, and for every message while a navigation is pending.
func (g *msgGate) check(raw string) (origin, msg string, ok bool) {
	if g.pending || len(g.current) != tokenLen || len(raw) < tokenLen {
		return "", "", false
	}
	if subtle.ConstantTimeCompare([]byte(raw[:tokenLen]), []byte(g.current)) != 1 {
		return "", "", false
	}
	return g.origin, raw[tokenLen:], true
}

// addRequestOrigin returns h with an Origin header naming origin when the
// request changes state (not GET/HEAD) and carries none. An unknown sender
// becomes "null", which the core's cross-origin check refuses.
func addRequestOrigin(h [][2]string, method, origin string) [][2]string {
	switch strings.ToUpper(method) {
	case "", "GET", "HEAD":
		return h
	}
	for _, kv := range h {
		if strings.EqualFold(kv[0], "Origin") {
			return h
		}
	}
	if origin == "" {
		origin = "null"
	}
	return append(h, [2]string{"Origin", origin})
}
