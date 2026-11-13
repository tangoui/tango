package android

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// jsInterface is the name of the host's channel object in the page
// (Page.java): a WebMessageListener's object, which the WebView puts only
// in frames of the app's trusted origins, or, on WebViews without that
// feature, a JavascriptInterface, which every frame sees.
const jsInterface = "__tangoAndroid"

// newToken returns a random per-document secret for the bridge shim
// (token.go): 32 hex characters.
func newToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("tango: crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// pageScript is what runs at the start of a trusted main-frame document:
// the backend shim defining window.__tangoNative, then the core's
// BridgeScript. Each document gets its own token (token.go).
//
// The shim refuses to run in sub-frames. It takes the channel object and
// the token into its closure when it runs and deletes the global, so page
// scripts that run later cannot reach the channel or swap it, and the
// token is only in the closure (in injected HTML the script element
// removes itself, so the token does not linger in the DOM either). Every
// message carries the token; Go drops those without the current
// document's. bodyScript (body.go) wraps fetch and XMLHttpRequest so
// request bodies reach Go.
//
// With a JavascriptInterface (ch.post) calls are synchronous. With a
// WebMessageListener (ch.postMessage) messages are strings of a kind
// byte, the token and the payload (parseWire, token.go); a stored body is
// acknowledged with a reply "b1<id>" or "b0<id>".
func pageScript(token, bridge string) string {
	var b strings.Builder
	b.WriteString(`(function () {
  "use strict";
  var el = document.currentScript;
  if (el && el.parentNode) el.parentNode.removeChild(el);
  if (window !== window.top || Object.prototype.hasOwnProperty.call(window, "__tangoNative")) return;
  var ch = window.` + jsInterface + `, t = "` + token + `";
  if (!ch) return;
  try { delete window.` + jsInterface + `; } catch (e) {}
  var send, stashNow = null, stashLater;
  if (typeof ch.post === "function") {
    send = function (s) { ch.post(t, s); };
    stashNow = function (id, type, b64) { return ch.body(t, id, type, b64); };
    stashLater = function (id, type, b64) { return Promise.resolve(stashNow(id, type, b64)); };
  } else if (typeof ch.postMessage === "function") {
    var post = ch.postMessage, waiting = Object.create(null);
    ch.addEventListener("message", function (e) {
      var d = String(e.data), cb = waiting[d.slice(2)];
      if (d.charAt(0) !== "b" || !cb) return;
      delete waiting[d.slice(2)];
      cb(d.charAt(1) === "1");
    });
    send = function (s) { post.call(ch, "m" + t + s); };
    stashLater = function (id, type, b64) {
      return new Promise(function (resolve) {
        waiting[id] = resolve;
        post.call(ch, "b" + t + id + "\n" + String(type).replace(/[\r\n]/g, " ") + "\n" + b64);
      });
    };
  } else return;
` + bodyScript + `
  Object.defineProperty(window, "__tangoNative", {
    value: Object.freeze({ post: function (s) { send(String(s)); } })
  });
})();
`)
	b.WriteString(bridge)
	return b.String()
}

var (
	closeScript = regexp.MustCompile(`(?i)</(script)`)
	openComment = regexp.MustCompile(`<!--`)
)

// inlineScript makes js safe as the text of an inline <script> element in
// a document of any charset: no "</script" or "<!--" sequences, and only
// ASCII (non-ASCII becomes \uXXXX, which JavaScript reads the same in
// strings, identifiers, regular expressions and comments).
func inlineScript(js string) string {
	js = closeScript.ReplaceAllString(js, `<\/$1`)
	js = openComment.ReplaceAllString(js, `<\!--`)
	var b strings.Builder
	b.Grow(len(js))
	for _, r := range js {
		if r < 0x80 {
			b.WriteRune(r)
			continue
		}
		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			writeU(&b, hi)
			writeU(&b, lo)
			continue
		}
		writeU(&b, r)
	}
	return b.String()
}

func writeU(b *strings.Builder, r rune) {
	s := strconv.FormatInt(int64(r), 16)
	b.WriteString(`\u`)
	b.WriteString(strings.Repeat("0", 4-len(s)))
	b.WriteString(s)
}

// injectHTML inserts js as an inline script at the start of the document's
// <head> (after its start tag), or after <html> or the doctype when the
// document has no explicit head: either way before any of the page's own
// scripts.
func injectHTML(doc []byte, js string) []byte {
	tag := []byte("<script>" + inlineScript(js) + "</script>")
	at := insertionPoint(doc)
	out := make([]byte, 0, len(doc)+len(tag))
	out = append(out, doc[:at]...)
	out = append(out, tag...)
	out = append(out, doc[at:]...)
	return out
}

// insertionPoint scans the document prologue (BOM, whitespace, comments,
// doctype, processing instructions, <html>, <head>) and returns the offset
// right after <head ...>, else after <html ...>, else after the prologue.
func insertionPoint(doc []byte) int {
	i := 0
	if bytes.HasPrefix(doc, []byte("\xEF\xBB\xBF")) {
		i = 3
	}
	after := i // best insertion point so far
	for i < len(doc) {
		switch c := doc[i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
			continue
		case c != '<':
			return after
		}
		rest := doc[i:]
		switch {
		case bytes.HasPrefix(rest, []byte("<!--")):
			end := bytes.Index(rest[4:], []byte("-->"))
			if end < 0 {
				return after
			}
			i += 4 + end + 3
		case hasTagPrefix(rest, "<!doctype") || bytes.HasPrefix(rest, []byte("<?")):
			end := bytes.IndexByte(rest, '>')
			if end < 0 {
				return after
			}
			i += end + 1
		case hasTagPrefix(rest, "<html"):
			end := tagEnd(rest)
			if end < 0 {
				return after
			}
			i += end
		case hasTagPrefix(rest, "<head"):
			end := tagEnd(rest)
			if end < 0 {
				return after
			}
			return i + end
		default:
			return after
		}
		after = i
	}
	return after
}

// hasTagPrefix reports whether b starts with name (case-insensitive) followed
// by a character that ends a tag name.
func hasTagPrefix(b []byte, name string) bool {
	if len(b) < len(name) || !bytes.EqualFold(b[:len(name)], []byte(name)) {
		return false
	}
	if len(b) == len(name) {
		return false
	}
	switch b[len(name)] {
	case ' ', '\t', '\n', '\r', '\f', '>', '/':
		return true
	}
	return false
}

// tagEnd returns the offset just past the '>' closing the start tag at the
// beginning of b, honoring quoted attribute values, or -1.
func tagEnd(b []byte) int {
	var quote byte
	for i := 1; i < len(b); i++ {
		c := b[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i + 1
		}
	}
	return -1
}
