package android

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// Request bodies for the asset origin.
//
// WebViewClient.shouldInterceptRequest gets method, URL and headers but
// never the body, so a fetch or XMLHttpRequest POST to the asset origin
// would reach the handler empty. The page script (pageScript) therefore
// wraps fetch and XMLHttpRequest: for a request with a body to the asset
// origin it first serializes the body (text, JSON, URLSearchParams,
// FormData with files, Blob, ArrayBuffer) and hands it, base64-encoded,
// with the document's token to the host's channel (a WebMessageListener →
// nativeWebMessage, or the JavascriptInterface Page.Channel.body →
// nativeStashBody), which stores it here under a random id; then, once Go
// has acknowledged it (the JavascriptInterface call is synchronous, a
// listener replies), it sends the request with that id in the bodyHeader
// header and an empty body, so the body is stored before the request
// leaves the page. The request thread takes it back by id (takeBody) and
// builds the http.Request with it.

// assetOriginJS is AssetOrigin (backend_android.go, android builds only).
const assetOriginJS = "https://tango.localhost"

// bodyHeader carries the stash id of the request's body.
const bodyHeader = "X-Tango-Body"

const (
	maxStashedBody  = 64 << 20  // per request
	maxStashedTotal = 128 << 20 // all pending bodies
	stashTTL        = time.Minute
)

type stashKey struct {
	sid platform.SurfaceID
	id  string
}

type stashedBody struct {
	data  []byte
	ctype string // Content-Type matching data; "" = none
	at    time.Time
}

// bodyStash holds bodies between the page handing them over (JavaBridge
// thread) and the request thread taking them.
type bodyStash struct {
	mu    sync.Mutex
	m     map[stashKey]stashedBody
	total int
	now   func() time.Time
}

var bodies = &bodyStash{}

func (s *bodyStash) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// put stores data under (sid, id). It refuses empty or oversized ids and
// bodies over the limits; bodies older than stashTTL (requests that never
// arrived) are dropped first.
func (s *bodyStash) put(sid platform.SurfaceID, id, ctype string, data []byte) bool {
	if id == "" || len(id) > 64 || len(data) > maxStashedBody {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[stashKey]stashedBody{}
	}
	now := s.clock()
	for k, b := range s.m {
		if now.Sub(b.at) > stashTTL {
			s.total -= len(b.data)
			delete(s.m, k)
		}
	}
	k := stashKey{sid, id}
	if old, ok := s.m[k]; ok {
		s.total -= len(old.data)
	}
	if s.total+len(data) > maxStashedTotal {
		return false
	}
	s.m[k] = stashedBody{data: data, ctype: ctype, at: now}
	s.total += len(data)
	return true
}

func (s *bodyStash) take(sid platform.SurfaceID, id string) (stashedBody, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := stashKey{sid, id}
	b, ok := s.m[k]
	if ok {
		delete(s.m, k)
		s.total -= len(b.data)
	}
	return b, ok
}

// dropSurface forgets the bodies of a closed surface.
func (s *bodyStash) dropSurface(sid platform.SurfaceID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, b := range s.m {
		if k.sid == sid {
			s.total -= len(b.data)
			delete(s.m, k)
		}
	}
}

// stashBase64 decodes and stores a body the page handed over.
func (s *bodyStash) stashBase64(sid platform.SurfaceID, id, ctype, b64 string) bool {
	if base64.StdEncoding.DecodedLen(len(b64)) > maxStashedBody+3 {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return false
	}
	return s.put(sid, id, ctype, data)
}

// takeBody removes the bodyHeader entry from a flat header list and
// returns the body it names. ok is false when the request has no stashed
// body (no header, or the id is unknown).
func (s *bodyStash) takeBody(sid platform.SurfaceID, headers []string) (rest []string, b stashedBody, ok bool) {
	rest = headers
	for i := 0; i+1 < len(headers); i += 2 {
		if !strings.EqualFold(strings.TrimSpace(headers[i]), bodyHeader) {
			continue
		}
		rest = append(append([]string(nil), headers[:i]...), headers[i+2:]...)
		b, ok = s.take(sid, strings.TrimSpace(headers[i+1]))
		return rest, b, ok
	}
	return rest, b, false
}

// setBody gives r the body and the Content-Type the page serialized it
// with ("" = none), which replaces the header the WebView sent for the
// empty body it actually transmitted.
func setBody(r *http.Request, b stashedBody) {
	data := b.data
	r.Body = io.NopCloser(bytes.NewReader(data))
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	r.ContentLength = int64(len(data))
	if b.ctype != "" {
		r.Header.Set("Content-Type", b.ctype)
	} else {
		r.Header.Del("Content-Type")
	}
	r.Header.Set("Content-Length", itoa(len(data)))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}

// bodyScript wraps fetch and XMLHttpRequest (see the comment at the top).
// It runs inside the page shim's closure (pageScript), which provides
// stashLater(id, type, b64), a Promise of whether Go stored the body, and,
// with a JavascriptInterface, stashNow, its synchronous form (null with a
// WebMessageListener, whose messages are asynchronous: there a synchronous
// XMLHttpRequest sends its body as the WebView would, that is, empty).
const bodyScript = `
  (function () {
    var ORIGIN = "` + assetOriginJS + `", HDR = "` + bodyHeader + `";
    function toAsset(u) {
      try { return new URL(u, location.href).origin === ORIGIN; } catch (e) { return false; }
    }
    function newId() {
      var a = new Uint8Array(12);
      crypto.getRandomValues(a);
      return Array.prototype.map.call(a, function (b) { return (b + 256).toString(16).slice(1); }).join("");
    }
    function b64(buf) {
      var u = new Uint8Array(buf), s = "", step = 0x8000;
      for (var i = 0; i < u.length; i += step) s += String.fromCharCode.apply(null, u.subarray(i, i + step));
      return btoa(s);
    }
    // stash hands the body to Go and resolves to the id for the header,
    // or null when Go refused it (too large). stashSync does it at once,
    // or returns undefined when the channel is asynchronous.
    function stash(buf, type) {
      var id = newId();
      return stashLater(id, type || "", b64(buf)).then(function (ok) { return ok ? id : null; });
    }
    function stashSync(buf, type) {
      if (!stashNow) return undefined;
      var id = newId();
      return stashNow(id, type || "", b64(buf)) ? id : null;
    }
    var bodyless = { GET: 1, HEAD: 1 };

    var origFetch = window.fetch;
    if (origFetch) {
      window.fetch = function (input, init) {
        var req;
        try { req = new Request(input, init); } catch (e) { return origFetch.apply(this, arguments); }
        if (bodyless[req.method.toUpperCase()] || !toAsset(req.url)) return origFetch.call(window, req);
        return req.clone().arrayBuffer().then(function (buf) {
          if (!buf.byteLength) return origFetch.call(window, req);
          return stash(buf, req.headers.get("content-type")).then(function (id) {
            if (!id) return origFetch.call(window, req);
            var h = new Headers(req.headers);
            h.set(HDR, id);
            // The WebView would drop the body anyway: send an empty one.
            return origFetch.call(window, new Request(req, { headers: h, body: "" }));
          });
        });
      };
    }

    var X = XMLHttpRequest.prototype, open = X.open, send = X.send, setHeader = X.setRequestHeader;
    var info = new WeakMap();
    X.open = function (method, url, async) {
      info.set(this, { m: String(method).toUpperCase(), u: String(url), ct: null, async: arguments.length < 3 || !!async });
      return open.apply(this, arguments);
    };
    X.setRequestHeader = function (name, value) {
      var s = info.get(this);
      if (s && String(name).toLowerCase() === "content-type") s.ct = String(value);
      return setHeader.apply(this, arguments);
    };
    function encodeNow(body) {
      if (typeof body === "string") return { buf: new TextEncoder().encode(body), type: "text/plain;charset=UTF-8" };
      if (body instanceof URLSearchParams)
        return { buf: new TextEncoder().encode(body.toString()), type: "application/x-www-form-urlencoded;charset=UTF-8" };
      if (body instanceof ArrayBuffer) return { buf: new Uint8Array(body), type: "" };
      if (ArrayBuffer.isView(body)) return { buf: new Uint8Array(body.buffer, body.byteOffset, body.byteLength), type: "" };
      return null;
    }
    X.send = function (body) {
      var s = info.get(this), x = this;
      if (!s || body == null || bodyless[s.m] || !toAsset(s.u)) return send.apply(this, arguments);
      var now = encodeNow(body);
      if (now) {
        var id = stashSync(now.buf, s.ct || now.type);
        if (id === null) return send.apply(this, arguments);
        if (id !== undefined) {
          setHeader.call(x, HDR, id);
          return send.call(x, "");
        }
      }
      // Blob and FormData need async reading, an asynchronous channel an
      // async request.
      if (!s.async || typeof Response !== "function") return send.apply(this, arguments);
      var r = now ? null : new Response(body);
      (now ? Promise.resolve(now.buf) : r.arrayBuffer()).then(function (buf) {
        return stash(buf, s.ct || (now ? now.type : r.headers.get("content-type")));
      }).then(function (id) {
        if (id) setHeader.call(x, HDR, id);
        send.call(x, id ? "" : body);
      }).catch(function () {
        try { send.call(x, body); } catch (e) {}
      });
    };
  })();
`
