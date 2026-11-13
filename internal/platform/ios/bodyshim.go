//go:build ios

package ios

// bodyShim runs at document start in every page, after nativeShim. WebKit
// hands a WKURLSchemeHandler the bytes of string and ArrayBuffer request
// bodies, but not of Blob-backed ones: a fetch or XMLHttpRequest to
// tango://app with a Blob, File or FormData body (a file upload) reached Go
// empty. The shim turns such bodies into an ArrayBuffer first, with the
// Content-Type the browser would have sent (a multipart boundary matching
// the encoded form). Requests to other origins are left alone; synchronous
// XMLHttpRequest cannot wait for the conversion and is left alone too.
const bodyShim = `(function () {
  "use strict";
  if (window.__tangoBodyShim) return;
  window.__tangoBodyShim = true;
  function ours(u) {
    try { var x = new URL(String(u), location.href); return x.protocol === "tango:" && x.host === "app"; }
    catch (e) { return false; }
  }
  function blobby(b) {
    return (typeof Blob !== "undefined" && b instanceof Blob) || (typeof FormData !== "undefined" && b instanceof FormData);
  }
  function hasBody(m) { m = String(m || "GET").toUpperCase(); return m !== "GET" && m !== "HEAD"; }
  var F = window.fetch;
  if (F) window.fetch = function (input, init) {
    try {
      var fromRequest = typeof Request !== "undefined" && input instanceof Request && !(init && "body" in init);
      if ((fromRequest || (init && blobby(init.body))) ) {
        var req = new Request(input, init);
        if (ours(req.url) && hasBody(req.method) && req.body !== null) {
          return req.arrayBuffer().then(function (buf) {
            return F.call(window, req.url, {method: req.method, headers: req.headers, body: buf,
              credentials: req.credentials, redirect: req.redirect, signal: req.signal, cache: req.cache});
          });
        }
      }
    } catch (e) {}
    return F.apply(this, arguments);
  };
  var X = window.XMLHttpRequest && XMLHttpRequest.prototype;
  if (!X) return;
  var open = X.open, send = X.send, setHeader = X.setRequestHeader;
  X.open = function (method, url, async) {
    this.__tangoReq = {method: method, url: url, async: arguments.length < 3 || !!async, type: false};
    return open.apply(this, arguments);
  };
  X.setRequestHeader = function (name, value) {
    if (this.__tangoReq && String(name).toLowerCase() === "content-type") this.__tangoReq.type = true;
    return setHeader.apply(this, arguments);
  };
  X.send = function (body) {
    var t = this.__tangoReq, x = this;
    if (t && t.async && blobby(body) && hasBody(t.method) && ours(t.url)) {
      var r = new Response(body), type = r.headers.get("Content-Type");
      r.arrayBuffer().then(function (buf) {
        if (!t.type && type) setHeader.call(x, "Content-Type", type);
        send.call(x, buf);
      }, function () { send.call(x, body); });
      return;
    }
    return send.apply(this, arguments);
  };
})();
`
