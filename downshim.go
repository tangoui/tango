package tango

// downShim is the page side of the down:poll lane
// (docs/superpowers/specs/2026-11-14-down-poll-design.md §5.3–5.5, 5.8,
// 5.9). It is not a script of its own: coreShim includes it inside its
// function, after post (postMsg) and the subscriber queue (recv), and before
// window.__tango is frozen, which then gets these members:
//
//	_open({gen, tok, path})  the core (Eval) opens the poll lane of a
//	                         generation: GET path?gen=&tok=&ack= in a loop
//	_recvSeq(gen, items)     the core's fallback (Eval): items are
//	                         [seq, kind, data]; kind 0 data is the message,
//	                         1 the packet in base64, 2 the barrier id (the
//	                         core runs that barrier's script itself)
//	_release(gen, id)        the core ran the script of barrier id
//	_onPacket(fn)            runtime-js subscribes to byte packets
//	                         (Uint8Array); returns an unsubscribe function
//
// Items carry sequence numbers from 1 within a generation and are
// delivered in that order whichever way they came; ones ahead of the next
// number wait, ones already delivered or waiting are dropped (the core may
// send an item both ways). Messages go to the onMessage subscribers (kept
// until one registers, like _recv), packets to the _onPacket subscribers
// (kept the same way). A barrier that comes in a poll response stops
// delivery and polling: the page tells the core, which runs the Eval script
// and calls _release; a barrier that comes by _recvSeq was run by the core
// already, and releases the barrier waited on if it is the same one.
//
// A generation newer than the current one, from _open or _recvSeq, starts
// it: numbering from 1, waiting items and a waited-on barrier dropped, the
// old poll loop stopped. Older generations are ignored.
//
// Internal messages to the core go by __tango.post as
// "\x00tango-int:down:" + JSON: {"op":"poll","gen"} before the first poll,
// {"op":"barrier","gen","id"} when delivery reaches a polled barrier, and
// {"op":"fail","gen","acked"} (the last sequence number delivered; a
// barrier reached counts) once when a poll fails — a network error, a
// status other than 200 and 410, or a malformed batch — after which the
// page stops polling and relies on _recvSeq. 410 (the generation ended)
// stops polling quietly.
//
// Batches are big-endian: firstSeq:u64 | count:u32 | (kind:u8 | len:u32 |
// payload)*count. Packets are Uint8Array views into the response body.
const downShim = `
  var downPrefix = "\u0000tango-int:down:";
  var downFetch = typeof window.fetch === "function" ? window.fetch.bind(window) : null;
  var downUTF8 = typeof TextDecoder === "function" ? new TextDecoder() : null;
  var downGen = 0;        // current generation; 0: none yet
  var downNext = 1;       // the sequence number to deliver next
  var downHeld = {};      // seq -> [kind, data, polled], ahead of downNext
  var downWait = null;    // {id} of the polled barrier delivery waits on
  var downOpened = false; // _open seen for downGen (the loop runs at most once)
  var downLoop = 0;       // generation of the running poll loop; 0: none
  var downResume = null;  // continues the poll loop after a release
  var pktSubs = [];
  var pktQueue = [];
  var pktFlushing = false;

  function downPost(o) {
    try { postMsg(downPrefix + JSON.stringify(o)); } catch (e) {}
  }
  function deliverPkt(p) {
    var list = pktSubs.slice();
    for (var i = 0; i < list.length; i++) {
      try { list[i](p); } catch (e) { rethrow(e); }
    }
  }
  function flushPkts() {
    pktFlushing = false;
    while (pktQueue.length && pktSubs.length) deliverPkt(pktQueue.shift());
  }
  function recvPkt(p) {
    if (!pktSubs.length || pktQueue.length) {
      pktQueue.push(p);
      if (pktSubs.length && !pktFlushing) { pktFlushing = true; Promise.resolve().then(flushPkts); }
      return;
    }
    deliverPkt(p);
  }
  function onPacket(fn) {
    if (typeof fn !== "function") throw new TypeError("__tango._onPacket: handler must be a function");
    pktSubs.push(fn);
    if (pktQueue.length && !pktFlushing) { pktFlushing = true; Promise.resolve().then(flushPkts); }
    return function () {
      var i = pktSubs.indexOf(fn);
      if (i >= 0) pktSubs.splice(i, 1);
    };
  }

  // downAt makes gen current if it is newer; false if it is older or bad.
  function downAt(gen) {
    if (typeof gen !== "number" || !(gen >= 1) || gen < downGen) return false;
    if (gen > downGen) {
      downGen = gen;
      downNext = 1;
      downHeld = {};
      downWait = null;
      downOpened = false;
      downLoop = 0;
      downResume = null;
    }
    return true;
  }
  // downPut takes item seq; polled tells a poll response from _recvSeq.
  function downPut(seq, kind, data, polled) {
    if (seq < downNext) {
      // The fallback's copy of the barrier waited on: the core ran it.
      if (!polled && kind === 2 && downWait && downWait.id === data) downRelease(downGen, data);
      return;
    }
    var h = downHeld[seq];
    if (h && !(kind === 2 && !polled)) return; // a fallback barrier replaces a polled one
    downHeld[seq] = [kind, data, polled];
  }
  // downDrain delivers in order until a gap or a polled barrier.
  function downDrain() {
    while (!downWait) {
      var h = downHeld[downNext];
      if (!h) return;
      delete downHeld[downNext];
      var seq = downNext++;
      if (h[0] === 0) recv(h[1]);
      else if (h[0] === 1) recvPkt(h[1]);
      else if (h[2]) {
        downWait = { id: h[1] };
        downPost({ op: "barrier", gen: downGen, id: h[1] });
      }
    }
  }
  function downRelease(gen, id) {
    if (gen !== downGen || !downWait || downWait.id !== id) return;
    downWait = null;
    downDrain();
    if (!downWait && downResume) {
      var r = downResume;
      downResume = null;
      Promise.resolve().then(r); // after the caller's items: the ack is current
    }
  }
  function downDecode(buf) {
    var v = new DataView(buf), n = buf.byteLength;
    if (n < 12) throw new Error("batch header cut short");
    var first = v.getUint32(0) * 4294967296 + v.getUint32(4);
    var count = v.getUint32(8), off = 12, out = [];
    for (var i = 0; i < count; i++) {
      if (n - off < 5) throw new Error("item header cut short");
      var kind = v.getUint8(off), len = v.getUint32(off + 1), data;
      off += 5;
      if (len > n - off) throw new Error("item payload cut short");
      if (kind === 0) data = downUTF8.decode(new Uint8Array(buf, off, len));
      else if (kind === 1) data = new Uint8Array(buf, off, len);
      else if (kind === 2 && len === 8) data = v.getUint32(off) * 4294967296 + v.getUint32(off + 4);
      else throw new Error("bad item kind " + kind);
      out.push([first + i, kind, data]);
      off += len;
    }
    if (off !== n) throw new Error("trailing bytes after the batch");
    return out;
  }
  function downPoll(gen, tok, path) {
    var live = function () { return downGen === gen && downLoop === gen; };
    var fail = function () {
      if (!live()) return;
      downLoop = 0;
      downResume = null;
      downPost({ op: "fail", gen: gen, acked: downNext - 1 });
    };
    var next = function () {
      if (!live()) return;
      if (downWait) { downResume = next; return; }
      var url = path + (path.indexOf("?") < 0 ? "?" : "&") + "gen=" + gen +
        "&tok=" + encodeURIComponent(tok) + "&ack=" + (downNext - 1);
      var f = downFetch || (typeof window.fetch === "function" ? window.fetch.bind(window) : null);
      var p;
      try { p = f(url, { cache: "no-store" }); } catch (e) { fail(); return; }
      Promise.resolve(p).then(function (r) {
        if (!live()) return;
        if (r.status === 410) { downLoop = 0; return; }
        if (r.status !== 200) { fail(); return; }
        return r.arrayBuffer().then(function (buf) {
          if (!live()) return;
          var items = downDecode(buf);
          for (var i = 0; i < items.length; i++) downPut(items[i][0], items[i][1], items[i][2], true);
          downDrain();
          next();
        });
      }).catch(fail);
    };
    downPost({ op: "poll", gen: gen });
    next();
  }
  function downOpen(o) {
    if (!o || typeof o.tok !== "string" || !downAt(o.gen) || downOpened) return;
    downOpened = true;
    downLoop = o.gen;
    downPoll(o.gen, o.tok, typeof o.path === "string" && o.path ? o.path : "/__tango/lane/poll");
  }
  function downRecvSeq(gen, items) {
    if (!downAt(gen) || !items) return;
    for (var i = 0; i < items.length; i++) {
      var it = items[i], kind = it[1], data = it[2];
      if (kind === 1) {
        var s = atob(data), b = new Uint8Array(s.length);
        for (var j = 0; j < s.length; j++) b[j] = s.charCodeAt(j);
        data = b;
      }
      downPut(it[0], kind, data, false);
    }
    downDrain();
  }
`
