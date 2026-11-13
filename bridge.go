package tango

import (
	"encoding/json"
	"strings"
)

// coreShim is the start of every surface's BridgeScript. The backend's own
// shim has already defined window.__tangoNative = { post(str) }.
//
//	window.__tango.post(str)      send a raw message to Go (App.OnMessage)
//	window.__tango.onMessage(fn)  subscribe to Go's Surface.Send; returns an
//	                             unsubscribe function
//	window.__tango._recv(str)     called by the core with Eval
//
// Messages that arrive while nobody is subscribed are kept and delivered,
// in order and asynchronously (a microtask), once a subscriber registers.
// A throwing subscriber does not stop the others; its error is rethrown
// asynchronously so it still reaches the console.
const coreShim = `(function () {
  "use strict";
  if (window.__tango) return;
  var native = window.__tangoNative;
  var nativePost = native && typeof native.post === "function" ? native.post.bind(native) : null;
  var subs = [];
  var queue = [];
  var flushing = false;
  function deliver(msg) {
    var list = subs.slice();
    for (var i = 0; i < list.length; i++) {
      try { list[i](msg); } catch (e) { setTimeout(function () { throw e; }, 0); }
    }
  }
  function flush() {
    flushing = false;
    while (queue.length && subs.length) deliver(queue.shift());
  }
  function schedule() {
    if (flushing) return;
    flushing = true;
    Promise.resolve().then(flush);
  }
  window.__tango = Object.freeze({
    post: function (msg) {
      if (typeof msg !== "string") throw new TypeError("__tango.post: message must be a string");
      var p = nativePost || (window.__tangoNative && window.__tangoNative.post.bind(window.__tangoNative));
      if (!p) throw new Error("__tango.post: native bridge missing");
      p(msg);
    },
    onMessage: function (fn) {
      if (typeof fn !== "function") throw new TypeError("__tango.onMessage: handler must be a function");
      subs.push(fn);
      if (queue.length) schedule();
      return function () {
        var i = subs.indexOf(fn);
        if (i >= 0) subs.splice(i, 1);
      };
    },
    _recv: function (msg) {
      if (!subs.length || queue.length) {
        queue.push(msg);
        if (subs.length) schedule();
        return;
      }
      deliver(msg);
    }
  });
})();
`

var lineSepEscaper = strings.NewReplacer("\u2028", `\u2028`, "\u2029", `\u2029`)

// recvJS returns the script delivering msg to the page's __tango._recv.
// encoding/json escapes U+2028 and U+2029 (line terminators in pre-ES2019
// JavaScript string literals) and <, >, & (so "</script>" cannot break out
// of an inline script); the explicit replacements below keep that true even
// if the encoder ever changes.
func recvJS(msg string) string {
	b, _ := json.Marshal(msg) // a string never fails to marshal
	lit := lineSepEscaper.Replace(string(b))
	return "window.__tango._recv(" + lit + ")"
}
