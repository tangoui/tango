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
  var downHeldN = 0;      // entries in downHeld
  var downWait = null;    // {id} of the polled barrier delivery waits on
  var downOpened = false; // _open seen for downGen (the loop runs at most once)
  var downLoop = 0;       // generation of the running poll loop; 0: none
  var downResume = null;  // continues the poll loop after a release
  var downInFlight = 0;   // fetches issued and not fully returned
  var downInFlightMax = 0;
  var downModeTraceGen = 0;
  var pktSubs = [];
  var pktQueue = [];
  var pktFlushing = false;

  function downTrace(name, data) {
    var t = globalThis.__tangoIpcTrace;
    if (t && t.enabled && typeof t.event === "function") {
      try { t.event(name, data || {}); } catch (e) {}
    }
  }
  function downPollMode(gen) {
    var t = globalThis.__tangoIpcTrace;
    if (t && t.enabled && downModeTraceGen !== gen) {
      downModeTraceGen = gen;
      downTrace("down_mode", {gen: gen, mode: "B"});
    }
    return "B";
  }
  function downFetchDone() {
    if (downInFlight > 0) downInFlight--;
  }
  function downItemBytes(kind, data, size) {
    if (typeof size === "number") return size;
    if (kind === 1 && data) return data.byteLength;
    if (kind === 2) return 8;
    return typeof data === "string" ? data.length : 0;
  }
  function downDuplicate(seq, kind, data, size, source) {
    var bytes = downItemBytes(kind, data, size);
    downTrace("down_duplicate", {
      seq: seq, kind: kind, source: source || "poll",
      itemBytes: bytes, packetBytes: kind === 1 ? bytes : 0,
    });
  }
  function downPacketFrames(p) {
    return p && p.byteLength >= 2 ? new DataView(p.buffer, p.byteOffset, 2).getUint16(0) : 0;
  }

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
    downTrace("shim_dispatch", {kind: 1, bytes: p.byteLength, frames: downPacketFrames(p)});
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
      downHeldN = 0;
      downWait = null;
      downOpened = false;
      downLoop = 0;
      downResume = null;
    }
    return true;
  }
  // downPut takes item seq; polled tells a poll response from _recvSeq.
  function downPut(seq, kind, data, polled, size) {
    if (seq < downNext) {
      // The fallback's copy of the barrier waited on: the core ran it.
      if (!polled && kind === 2 && downWait && downWait.id === data) downRelease(downGen, data);
      else downDuplicate(seq, kind, data, size, polled ? "poll" : "fallback");
      return;
    }
    var h = downHeld[seq];
    if (h && !(kind === 2 && !polled)) {
      downDuplicate(seq, kind, data, size, polled ? "poll" : "fallback");
      return;
    }
    if (!h) downHeldN++;
    downHeld[seq] = [kind, data, polled, size];
  }
  // downDrain delivers in order until a gap or a polled barrier.
  function downDrain() {
    while (!downWait) {
      var h = downHeld[downNext];
      if (!h) return;
      delete downHeld[downNext];
      downHeldN--;
      downNext++;
      downDeliver(h[0], h[1], h[2]);
    }
  }
  // downDeliver delivers the item at downNext - 1.
  function downDeliver(kind, data, polled) {
    if (kind === 0) recv(data);
    else if (kind === 1) recvPkt(data);
    else if (polled) {
      downWait = { id: data };
      downPost({ op: "barrier", gen: downGen, id: data });
    }
  }
  // downBatch takes a decoded poll batch: in order with nothing held it
  // is delivered as it is, otherwise through downPut and downDrain.
  function downBatch(b) {
    var i = 0, n = b.kinds.length;
    downTrace("downBatch_start", {first: b.first, count: n, bytes: b.bytes || 0, ack: downNext - 1});
    if (downHeldN === 0) {
      while (i < n && !downWait && b.first + i === downNext) {
        downNext++;
        downDeliver(b.kinds[i], b.datas[i], true);
        i++;
      }
    }
    for (; i < n; i++) downPut(b.first + i, b.kinds[i], b.datas[i], true, b.sizes && b.sizes[i]);
    downDrain();
    downTrace("downBatch_end", {first: b.first, count: n, ack: downNext - 1, held: downHeldN, waiting: !!downWait});
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
    var count = v.getUint32(8), off = 12, kinds = [], datas = [], sizes = [];
    for (var i = 0; i < count; i++) {
      if (n - off < 5) throw new Error("item header cut short");
      var kind = v.getUint8(off), len = v.getUint32(off + 1), data;
      off += 5;
      if (len > n - off) throw new Error("item payload cut short");
      if (kind === 0) data = downUTF8.decode(new Uint8Array(buf, off, len));
      else if (kind === 1) data = new Uint8Array(buf, off, len);
      else if (kind === 2 && len === 8) data = v.getUint32(off) * 4294967296 + v.getUint32(off + 4);
      else throw new Error("bad item kind " + kind);
      kinds.push(kind);
      datas.push(data);
      sizes.push(len);
      off += len;
    }
    if (off !== n) throw new Error("trailing bytes after the batch");
    return { first: first, kinds: kinds, datas: datas, sizes: sizes, bytes: n };
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
      var ack = downNext - 1;
      var url = path + (path.indexOf("?") < 0 ? "?" : "&") + "gen=" + gen +
        "&tok=" + encodeURIComponent(tok) + "&ack=" + ack;
      var mode = downPollMode(gen);
      downTrace("poll_start", {gen: gen, ack: ack, mode: mode, inflight: downInFlight, inflightMax: downInFlightMax});
      downTrace("next_fetch_issue", {gen: gen, ack: ack, mode: mode, inflight: downInFlight + 1, inflightMax: Math.max(downInFlightMax, downInFlight + 1)});
      downInFlight++;
      if (downInFlight > downInFlightMax) downInFlightMax = downInFlight;
      var f = downFetch || (typeof window.fetch === "function" ? window.fetch.bind(window) : null);
      var finished = false;
      var finish = function () {
        if (!finished) { finished = true; downFetchDone(); }
      };
      var p;
      try { p = f(url, { cache: "no-store" }); } catch (e) { finish(); fail(); return; }
      Promise.resolve(p).then(function (r) {
        if (!live()) { finish(); return; }
        downTrace("fetch_response", {status: r.status, ack: ack});
        if (r.status === 410) { finish(); downLoop = 0; return; }
        if (r.status !== 200) { finish(); fail(); return; }
        return r.arrayBuffer().then(function (buf) {
          finish();
          if (!live()) return;
          downTrace("fetch_body", {bytes: buf.byteLength});
          var batch = downDecode(buf);
          var frames = 0;
          for (var i = 0; i < batch.kinds.length; i++) {
            if (batch.kinds[i] === 1) frames += downPacketFrames(batch.datas[i]);
          }
          downTrace("decode_done", {bytes: buf.byteLength, first: batch.first, count: batch.kinds.length, items: batch.kinds.length, frames: frames});
          downBatch(batch);
          next();
        }).catch(function () { finish(); fail(); });
      }).catch(function () { finish(); fail(); });
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
