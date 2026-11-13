package linux

// nativeShim returns the script defining window.__tangoNative before any
// page script runs, for the document that will hold token (msggate.go).
//
// It posts through the "tango" WebKit script message handler, prefixing
// every message with token. The token lives only in this closure: it is
// not on window, so neither page script enumerating globals nor a
// same-origin sub-frame reading parent.window can read it, and
// Function.prototype.toString of post shows the code, not the value. The
// handler, its postMessage, Reflect.apply and String are captured before
// page script runs, so later monkey-patching cannot observe the token
// either. String(s) makes the message a JS string so the Go side always
// gets text. __tangoNative is non-writable and non-configurable.
func nativeShim(token string) string {
	return `(function () {
  "use strict";
  if (window !== window.top || window.__tangoNative) return;
  var w = window.webkit, h = w && w.messageHandlers && w.messageHandlers.` + messageHandlerName + `;
  if (!h || typeof h.postMessage !== "function") return;
  var pm = h.postMessage, ap = Reflect.apply, S = String, t = "` + token + `";
  Object.defineProperty(window, "__tangoNative", {
    value: Object.freeze({ post: function (s) { ap(pm, h, [t + S(s)]); } })
  });
})();`
}

// messageHandlerName is the WebKitUserContentManager handler name.
const messageHandlerName = "tango"

// documentStartScript is the single user script injected at document start
// in the top frame: the backend shim (carrying token) first, then the
// core's BridgeScript. One script keeps the order guaranteed.
func documentStartScript(token, bridge string) string {
	if bridge == "" {
		return nativeShim(token)
	}
	return nativeShim(token) + "\n;" + bridge
}
