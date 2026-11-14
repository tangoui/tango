import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import {
  createClient,
  TangoError,
  PROTO_VERSION,
  DEFAULT_WINDOW_BYTES,
  FRAME_OVERHEAD,
  MAX_FRAME_BYTES,
} from "../dist/index.js";

// A mock window.__tango: records what JS posts, lets the test play Go.
function fakeBridge() {
  let sub = null;
  const b = {
    out: [], // parsed ipc messages and raw strings posted by JS
    waiters: [],
    post(s) {
      let v = s;
      try {
        v = JSON.parse(s);
      } catch {}
      b.out.push(v);
      for (const w of b.waiters.splice(0)) w();
    },
    onMessage(fn) {
      sub = fn;
      return () => (sub = null);
    },
    recv(m) {
      sub(typeof m === "string" ? m : JSON.stringify(m));
    },
    take() {
      return b.out.splice(0);
    },
  };
  return b;
}

const tick = () => new Promise((r) => setImmediate(r));

test("hello is posted on start", () => {
  const b = fakeBridge();
  createClient(b);
  assert.deepEqual(b.take(), [{ tango: "hello", proto: 2 }]);
});

test("protocol constants match Go", () => {
  assert.equal(PROTO_VERSION, 2);
  assert.equal(DEFAULT_WINDOW_BYTES, 1048576);
  assert.equal(FRAME_OVERHEAD, 64);
  assert.equal(MAX_FRAME_BYTES, 16777216);
});

test("call round trip, odd ids, errors", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const p1 = c.call("Greeter.Hello", "world");
  const p2 = c.call("Boom");
  const p3 = c.call("NoArgs");
  const [m1, m2, m3] = b.take();
  assert.deepEqual(m1, { tango: "call", id: 1, method: "Greeter.Hello", args: ["world"] });
  assert.equal(m2.id, 3);
  assert.equal(m3.id, 5);
  assert.deepEqual(m3.args, []);
  b.recv({ tango: "ret", id: 3, error: { message: "boom", code: "internal" } });
  b.recv({ tango: "ret", id: 1, result: "Hello world" });
  b.recv({ tango: "ret", id: 5 });
  b.recv({ tango: "ret", id: 99, result: "unknown id ignored" });
  assert.equal(await p1, "Hello world");
  await assert.rejects(p2, (e) => e instanceof TangoError && e.code === "internal" && e.message === "boom");
  assert.equal(await p3, undefined);
});

test("events both ways", () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const got = [];
  const off = c.on("progress", (d) => got.push(d));
  c.on("progress", (d) => got.push(["second", d]));
  b.recv({ tango: "event", name: "progress", data: { pct: 42 } });
  b.recv({ tango: "event", name: "other", data: 1 });
  off();
  b.recv({ tango: "event", name: "progress", data: 2 });
  assert.deepEqual(got, [{ pct: 42 }, ["second", { pct: 42 }], ["second", 2]]);
  c.emit("clicked", { x: 1 });
  c.emit("ping");
  assert.deepEqual(b.take(), [
    { tango: "event", name: "clicked", data: { x: 1 } },
    { tango: "event", name: "ping", data: null },
  ]);
});

// Plays a Go producer honoring the byte credit window: it blocks while
// inflight > 0 && inflight + c > win (a frame always goes out when nothing is
// in flight), then ends its direction with seq = frames sent.
function produce(b, id, total, c, win = DEFAULT_WINDOW_BYTES) {
  const st = { sent: 0, inflight: 0, maxInflight: 0, acks: [], ended: false };
  const pump = () => {
    for (const m of b.take()) {
      if (m.id !== id) continue;
      if (m.op === "ack") {
        st.acks.push(m.b);
        st.inflight -= m.b;
      }
      if (m.op === "cancel") st.ended = true;
    }
    while (!st.ended && st.sent < total && (st.inflight === 0 || st.inflight + c <= win)) {
      const seq = st.sent++;
      st.inflight += c;
      st.maxInflight = Math.max(st.maxInflight, st.inflight);
      b.recv({ tango: "stream", id, op: "data", seq, c, data: seq });
    }
    if (!st.ended && st.sent === total) {
      st.ended = true;
      b.recv({ tango: "stream", id, op: "close", seq: total });
    }
    return st;
  };
  return pump;
}

test("stream with byte acks and backpressure", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const s = c.stream("Count", 200);
  const [open] = b.take();
  assert.deepEqual(open, { tango: "stream", id: 1, op: "open", method: "Count", args: [200] });
  b.recv({ tango: "stream", id: 1, op: "accept", in: false });
  const C = 100000;
  const pump = produce(b, 1, 200, C);
  assert.equal(pump().sent, 10); // the producer stops at the byte window
  const origPost = b.post;
  b.post = (msg) => {
    origPost(msg);
    pump();
  };
  const got = [];
  for await (const v of s) got.push(v);
  const st = pump();
  assert.deepEqual(got, Array.from({ length: 200 }, (_, i) => i));
  assert.ok(st.maxInflight <= DEFAULT_WINDOW_BYTES, `maxInflight=${st.maxInflight}`);
  assert.ok(st.acks.length > 0);
  assert.ok(st.acks.every((n) => n > 0 && n % C === 0), `acks=${st.acks}`);
  assert.ok(st.acks.some((n) => n >= DEFAULT_WINDOW_BYTES / 2), `acks=${st.acks}`);
});

test("acks by bytes after consumption", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Big");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 300000, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 300000, data: 2 });
  assert.deepEqual(b.take(), []); // nothing is acked before it is read
  const it = s[Symbol.asyncIterator]();
  assert.deepEqual(await it.next(), { value: 1, done: false });
  assert.deepEqual(b.take(), []); // 300000 < half a window, one frame still queued
  assert.deepEqual(await it.next(), { value: 2, done: false });
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "ack", b: 600000 }]);
});

test("acks a small remainder once the reader caught up", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Small");
  b.take();
  const it = s[Symbol.asyncIterator]();
  const p = it.next(); // a waiting reader reads the frame on arrival
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 70, data: "x" });
  assert.deepEqual(await p, { value: "x", done: false });
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "ack", b: 70 }]);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 80, data: "y" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 2, c: 90, data: "z" });
  assert.deepEqual(await it.next(), { value: "y", done: false });
  assert.deepEqual(b.take(), []); // "z" still queued
  assert.deepEqual(await it.next(), { value: "z", done: false });
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "ack", b: 170 }]);
});

test("no acks once Go ended its direction", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Done");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 70, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 1 });
  const got = [];
  for await (const v of s) got.push(v);
  assert.deepEqual(got, [1]);
  assert.deepEqual(b.take(), []);
});

test("binary frames decode, including empty", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Bytes");
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 69, bin: "aGVsbG8=" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 64, bin: "" });
  b.recv({ tango: "stream", id: 1, op: "close", seq: 2 });
  const got = [];
  for await (const v of s) got.push(v);
  assert.equal(got.length, 2);
  assert.ok(got[0] instanceof Uint8Array && got[1] instanceof Uint8Array);
  assert.deepEqual([...got[0]], [104, 101, 108, 108, 111]);
  assert.equal(got[1].length, 0);
});

test("binary frames decode without Uint8Array.fromBase64", async () => {
  const saved = Object.getOwnPropertyDescriptor(Uint8Array, "fromBase64");
  if (saved) delete Uint8Array.fromBase64;
  try {
    const b = fakeBridge();
    const c = createClient(b);
    const s = c.stream("Bytes");
    b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, bin: "AP+A" });
    b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 64, bin: "" });
    b.recv({ tango: "stream", id: 1, op: "end", seq: 2 });
    const got = [];
    for await (const v of s) got.push(v);
    assert.deepEqual(
      got.map((u) => [u instanceof Uint8Array, ...u]),
      [[true, 0, 255, 128], [true]],
    );
  } finally {
    if (saved) Object.defineProperty(Uint8Array, "fromBase64", saved);
  }
});

test("reorders by seq and end waits for gaps", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Reorder");
  const got = [];
  let finished = false;
  const done = (async () => {
    for await (const v of s) got.push(v);
    finished = true;
  })();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 67, data: "b" });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 2 });
  await tick();
  assert.deepEqual(got, []); // seq 0 still missing
  assert.equal(finished, false);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, data: "a" });
  await done;
  assert.deepEqual(got, ["a", "b"]);
});

test("stream error after buffered data", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Fail");
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, data: "a" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 67, data: "b" });
  b.recv({ tango: "stream", id: 1, op: "error", seq: 2, error: { message: "bad", code: "bad_args" } });
  const got = [];
  await assert.rejects(
    (async () => {
      for await (const v of s) got.push(v);
    })(),
    (e) => e instanceof TangoError && e.code === "bad_args",
  );
  assert.deepEqual(got, ["a", "b"]);
});

test("error waits for frames before its seq; late messages are ignored", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Fail");
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, data: "a" });
  b.recv({ tango: "stream", id: 1, op: "error", seq: 2, error: { message: "bad", code: "internal" } });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 67, data: "b" }); // counted by the error
  const got = [];
  await assert.rejects(
    (async () => {
      for await (const v of s) got.push(v);
    })(),
    (e) => e.code === "internal" && e.message === "bad",
  );
  assert.deepEqual(got, ["a", "b"]);
  b.take();
  // The stream finished: late data, acks and control messages are ignored.
  b.recv({ tango: "stream", id: 1, op: "data", seq: 7, c: 67, data: "late" });
  b.recv({ tango: "stream", id: 1, op: "ack", b: 100 });
  b.recv({ tango: "stream", id: 1, op: "close", seq: 9 });
  assert.deepEqual(b.take(), []);
});

test("close finishes the stream; late messages are ignored", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Closer");
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, data: "a" });
  b.recv({ tango: "stream", id: 1, op: "close", seq: 1 });
  const got = [];
  for await (const v of s) got.push(v);
  assert.deepEqual(got, ["a"]);
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 67, data: "late" });
  b.recv({ tango: "stream", id: 1, op: "ack", b: 64 });
  s.cancel(); // finished: nothing to cancel on the wire
  assert.deepEqual(b.take(), []);
});

test("unknown ops are ignored", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("X");
  b.recv({ tango: "stream", id: 1, op: "future-op", x: 1 });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 1 });
  const got = [];
  for await (const v of s) got.push(v);
  assert.deepEqual(got, [1]);
});

test("protocol violations fail the stream with bad_args", async () => {
  const cases = [
    [{ seq: 0, c: 67, data: 1, bin: "" }], // both payloads
    [{ seq: 0, c: 67 }], // neither payload
    [{ seq: 0, data: 1 }], // missing c
    [{ seq: 0, c: 10, data: 1 }], // c below the frame overhead
    [{ c: 67, data: 1 }], // missing seq
    [{ seq: 0, c: 67, bin: "!!!" }], // invalid base64
    [{ seq: 0, c: 67, data: 1 }, { seq: 0, c: 67, data: 1 }], // duplicate seq
  ];
  for (const frames of cases) {
    const b = fakeBridge();
    const c = createClient(b);
    const s = c.stream("Bad");
    b.take();
    const it = s[Symbol.asyncIterator]();
    for (const f of frames) b.recv({ tango: "stream", id: 1, op: "data", ...f });
    const msgs = b.take();
    const err = msgs.find((m) => m.op === "error");
    assert.ok(err, JSON.stringify(frames));
    assert.equal(err.id, 1);
    assert.equal(err.seq, 0);
    assert.equal(err.error.code, "bad_args");
    // A violation aborts the stream: unread frames are dropped.
    await assert.rejects(it.next(), (e) => e instanceof TangoError && e.code === "bad_args");
    assert.deepEqual(await it.next(), { value: undefined, done: true });
    // Failed: further messages for the stream are ignored.
    b.recv({ tango: "stream", id: 1, op: "data", seq: 9, c: 67, data: 1 });
    assert.deepEqual(b.take(), []);
  }
});

test("frame after end fails the stream", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Bad");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 67, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 1 }); // seq 1 is beyond the end
  const [err] = b.take();
  assert.equal(err.op, "error");
  assert.equal(err.error.code, "bad_args");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "bad_args");
});

test("a violation after Go's end still tells Go", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Bad");
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 0 }); // half-close: Go still receives
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 67, data: 1 });
  const [err] = b.take();
  assert.equal(err.op, "error");
  assert.equal(err.error.code, "bad_args");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "bad_args");
});

test("stream waiting consumer gets end and error", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Wait");
  const it = s[Symbol.asyncIterator]();
  const p = it.next();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 1 });
  assert.deepEqual(await p, { value: 1, done: false });
  const p2 = it.next();
  b.recv({ tango: "stream", id: 1, op: "error", seq: 1, error: { message: "x", code: "not_found" } });
  await assert.rejects(p2, (e) => e.code === "not_found");
  assert.deepEqual(await it.next(), { value: undefined, done: true });
});

test("peer cancel rejects the reader and drops queued data", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("X");
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "cancel" });
  const it = s[Symbol.asyncIterator]();
  await assert.rejects(it.next(), (e) => e instanceof TangoError && e.code === "canceled");
  assert.deepEqual(await it.next(), { value: undefined, done: true });
  b.take();
  s.cancel();
  assert.deepEqual(b.take(), []);
});

test("break cancels the stream", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const s = c.stream("Forever");
  for (let i = 0; i < 5; i++) b.recv({ tango: "stream", id: 1, op: "data", seq: i, c: 65, data: i });
  const got = [];
  for await (const v of s) {
    got.push(v);
    if (v === 2) break;
  }
  assert.deepEqual(got, [0, 1, 2]);
  const msgs = b.take();
  assert.deepEqual(msgs.at(-1), { tango: "stream", id: 1, op: "cancel" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 5, c: 65, data: 9 }); // late data ignored
  s.cancel(); // idempotent
  assert.equal(b.take().length, 0);
});

test("explicit cancel resolves a pending next and rejects accepted", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Forever");
  const it = s[Symbol.asyncIterator]();
  const p = it.next();
  s.cancel();
  assert.deepEqual(await p, { value: undefined, done: true });
  await assert.rejects(s.accepted, (e) => e instanceof TangoError && e.code === "canceled");
});

test("accepted resolves with in", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("X");
  b.recv({ tango: "stream", id: 1, op: "accept", in: false });
  assert.deepEqual(await s.accepted, { in: false });
  const s2 = c.stream("Y");
  b.recv({ tango: "stream", id: 3, op: "accept", in: true });
  assert.deepEqual(await s2.accepted, { in: true });
});

test("error before accept rejects accepted", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("X");
  b.recv({ tango: "stream", id: 1, op: "error", error: { message: "stream not found: X", code: "not_found" } });
  await assert.rejects(s.accepted, (e) => e instanceof TangoError && e.code === "not_found");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "not_found");
});

test("unobserved accepted rejections are not unhandled", async () => {
  let unhandled = 0;
  const onUnhandled = () => unhandled++;
  process.on("unhandledRejection", onUnhandled);
  try {
    const b = fakeBridge();
    const c = createClient(b);
    c.stream("X");
    b.recv({ tango: "stream", id: 1, op: "error", error: { message: "nope", code: "not_found" } });
    c.stream("Y").cancel();
    await tick();
    await tick();
  } finally {
    process.off("unhandledRejection", onUnhandled);
  }
  assert.equal(unhandled, 0);
});

test("onStream sends accept in:true first", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  let posted = null;
  let stream = null;
  c.onStream("news", (s, topic) => {
    posted = b.take();
    stream = s;
    assert.equal(topic, "sports");
  });
  b.recv({ tango: "stream", id: 2, op: "open", method: "news", args: ["sports"] });
  assert.deepEqual(posted, [{ tango: "stream", id: 2, op: "accept", in: true }]);
  assert.equal(stream.id, 2);
  assert.deepEqual(await stream.accepted, { in: true });
});

test("Go-opened streams", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const seen = [];
  c.onStream("news", async (s, topic, n) => {
    for await (const v of s) seen.push([topic, n, v]);
    seen.push("done");
  });
  b.recv({ tango: "stream", id: 2, op: "open", method: "news", args: ["sports", 3] });
  b.recv({ tango: "stream", id: 2, op: "data", seq: 0, c: 70, data: "goal" });
  b.recv({ tango: "stream", id: 2, op: "close", seq: 1 });
  await tick();
  assert.deepEqual(seen, [["sports", 3, "goal"], "done"]);
  b.take();

  b.recv({ tango: "stream", id: 4, op: "open", method: "missing", args: [] });
  const [err] = b.take();
  assert.equal(err.op, "error");
  assert.equal(err.id, 4);
  assert.equal(err.seq, 0);
  assert.equal(err.error.code, "not_found");
});

test("Go hello with win keeps the client working", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  b.recv({ tango: "hello", proto: 2, win: 4096 });
  const p = c.call("A");
  assert.equal(b.take()[0].method, "A");
  b.recv({ tango: "ret", id: 1, result: 1 });
  assert.equal(await p, 1);
});

test("protocol mismatch fails the client", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const early = c.call("Early");
  const s = c.stream("Before");
  b.take();
  const reported = [];
  const saved = globalThis.setTimeout;
  globalThis.setTimeout = (fn) => {
    try {
      fn();
    } catch (e) {
      reported.push(e);
    }
  };
  try {
    b.recv({ tango: "hello", error: { code: "bad_args", message: "runtime-js protocol mismatch" } });
    b.recv({ tango: "hello", error: { code: "bad_args", message: "runtime-js protocol mismatch" } });
  } finally {
    globalThis.setTimeout = saved;
  }
  assert.equal(reported.length, 1);
  assert.ok(reported[0] instanceof TangoError && reported[0].code === "bad_args");
  const isMismatch = (e) =>
    e instanceof TangoError && e.code === "bad_args" && e.message === "runtime-js protocol mismatch";
  await assert.rejects(c.call("A"), isMismatch);
  await assert.rejects(early, isMismatch);
  await assert.rejects(s[Symbol.asyncIterator]().next(), isMismatch);
  await assert.rejects(s.accepted, isMismatch);
  const s2 = c.stream("After");
  await assert.rejects(s2[Symbol.asyncIterator]().next(), isMismatch);
  await assert.rejects(s2.accepted, isMismatch);
  b.recv({ tango: "stream", id: 2, op: "open", method: "news", args: [] });
  assert.deepEqual(b.take(), []); // nothing more goes to Go
});

// ---- Send side ----

// Opens JS stream id 1 (after Go's hello with win, if given) and drains the outbox.
function openSender(win) {
  const b = fakeBridge();
  const c = createClient(b);
  if (win !== undefined) b.recv({ tango: "hello", proto: 2, win });
  const s = c.stream("Sink");
  b.take();
  return { b, c, s };
}
const dataOf = (msgs) => msgs.filter((m) => m.op === "data");
const isErr = (code, message) => (e) =>
  e instanceof TangoError && e.code === code && (message === undefined || e.message === message);

test("send JSON frame shape", async () => {
  const { b, s } = openSender();
  const obj = { a: 1 };
  const p = s.send(obj);
  obj.a = 2; // the value is encoded at call time
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "data", seq: 0, c: 71, data: { a: 1 } }]);
  await p;
  s.send("é"); // c counts UTF-8 bytes: "\"é\"" is 4 bytes
  s.send(undefined); // sent as null
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "data", seq: 1, c: 68, data: "é" },
    { tango: "stream", id: 1, op: "data", seq: 2, c: 68, data: null },
  ]);
  await assert.rejects(s.send(10n), isErr("bad_args"));
  const cyc = {};
  cyc.self = cyc;
  await assert.rejects(s.send(cyc), isErr("bad_args"));
  await assert.rejects(s.send(() => {}), isErr("bad_args"));
  assert.deepEqual(b.take(), []);
  s.send(1);
  assert.equal(b.take()[0].seq, 3);
});

test("sendBytes frame shape and copy", async () => {
  const { b, s } = openSender();
  const u = new Uint8Array([104, 105]);
  const p = s.sendBytes(u);
  u[0] = 0;
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "data", seq: 0, c: 66, bin: "aGk=" }]);
  await p;
  s.sendBytes(new Uint8Array(0));
  s.sendBytes(new Uint8Array([1, 104, 105, 2]).buffer);
  s.sendBytes(new DataView(new Uint8Array([9, 104, 105, 9]).buffer, 1, 2));
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "data", seq: 1, c: 64, bin: "" },
    { tango: "stream", id: 1, op: "data", seq: 2, c: 68, bin: "AWhpAg==" },
    { tango: "stream", id: 1, op: "data", seq: 3, c: 66, bin: "aGk=" },
  ]);
  await assert.rejects(s.sendBytes("hi"), isErr("bad_args"));
});

test("sendBytes encodes without Uint8Array.prototype.toBase64", async () => {
  const proto = Uint8Array.prototype;
  const had = Object.getOwnPropertyDescriptor(proto, "toBase64");
  if (had) delete proto.toBase64;
  try {
    const { b, s } = openSender();
    const big = new Uint8Array(0x8000 * 2 + 5);
    for (let i = 0; i < big.length; i++) big[i] = (i * 7) & 255;
    s.sendBytes(new Uint8Array([104, 105]));
    s.sendBytes(big);
    const [m1, m2] = b.take();
    assert.equal(m1.bin, "aGk=");
    assert.equal(m2.bin, Buffer.from(big).toString("base64"));
  } finally {
    if (had) Object.defineProperty(proto, "toBase64", had);
  }
});

test("credit blocks until ack", async () => {
  const { b, s } = openSender(4096);
  const ps = [];
  for (let i = 0; i < 5; i++) ps.push(s.sendBytes(new Uint8Array(1000)));
  assert.equal(dataOf(b.take()).length, 3);
  assert.equal(s.bufferedAmount, 2 * 1064);
  await Promise.all(ps.slice(0, 3)); // written frames resolve
  let fourth = false;
  ps[3].then(() => (fourth = true));
  await tick();
  assert.equal(fourth, false); // a queued frame waits
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  const more = dataOf(b.take());
  assert.equal(more.length, 1);
  assert.equal(more[0].seq, 3);
  assert.equal(s.bufferedAmount, 1064);
  await ps[3];
  b.recv({ tango: "stream", id: 1, op: "ack", b: 0 }); // ignored
  b.recv({ tango: "stream", id: 1, op: "ack", b: -5 }); // ignored
  assert.deepEqual(b.take(), []);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  assert.equal(dataOf(b.take()).length, 1);
  assert.equal(s.bufferedAmount, 0);
  await Promise.all(ps);
});

test("window is DEFAULT_WINDOW_BYTES before Go's hello", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Sink");
  b.take();
  const fit = Math.floor(DEFAULT_WINDOW_BYTES / 1064);
  const n = fit + 3;
  for (let i = 0; i < n; i++) s.sendBytes(new Uint8Array(1000));
  assert.equal(dataOf(b.take()).length, fit);
  // A larger window from Go's hello lets queued frames go at once.
  b.recv({ tango: "hello", proto: 2, win: 2 * DEFAULT_WINDOW_BYTES });
  assert.equal(dataOf(b.take()).length, 3);
  assert.equal(s.bufferedAmount, 0);
});

test("idle frame may exceed window", async () => {
  const { b, s } = openSender(4096);
  const p = s.sendBytes(new Uint8Array(10000));
  const [m] = b.take();
  assert.equal(m.op, "data");
  assert.equal(m.c, 10064);
  await p;
  s.send(1); // inflight 10064 > window: blocked
  assert.deepEqual(b.take(), []);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 10064 });
  assert.equal(b.take()[0].seq, 1);
});

test("oversize frame rejected, stream usable", async () => {
  const { b, s } = openSender();
  await assert.rejects(s.sendBytes(new Uint8Array(MAX_FRAME_BYTES + 1)), isErr("bad_args"));
  await assert.rejects(s.send("x".repeat(MAX_FRAME_BYTES)), isErr("bad_args")); // 16 MiB + 2 JSON bytes
  assert.equal(s.bufferedAmount, 0);
  s.send(1);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 1 }]);
  // Exactly MAX_FRAME_BYTES is allowed (once nothing is in flight).
  b.recv({ tango: "stream", id: 1, op: "ack", b: 65 });
  await s.sendBytes(new Uint8Array(MAX_FRAME_BYTES));
  assert.equal(b.take()[0].c, MAX_FRAME_BYTES + FRAME_OVERHEAD);
});

test("accept in:false rejects sends", async () => {
  const { b, s } = openSender(4096);
  await s.send(1);
  for (let i = 0; i < 4; i++) s.sendBytes(new Uint8Array(1000)); // the last one queues
  b.take();
  const queued = s.sendBytes(new Uint8Array(1000));
  b.recv({ tango: "stream", id: 1, op: "accept", in: false });
  await assert.rejects(queued, isErr("bad_args", "stream does not accept data"));
  await assert.rejects(s.send(2), isErr("bad_args", "stream does not accept data"));
  assert.equal(s.bufferedAmount, 0);
  s.end(); // nothing to end: Go does not receive
  assert.deepEqual(b.take(), []);
});

test("end is queued after frames", async () => {
  const { b, s } = openSender(4096);
  for (let i = 0; i < 5; i++) s.sendBytes(new Uint8Array(1000));
  s.end();
  s.end(); // idempotent
  assert.equal(dataOf(b.take()).length, 3);
  await assert.rejects(s.send(1), isErr("canceled", "stream closed"));
  await assert.rejects(s.sendBytes(new Uint8Array(1)), isErr("canceled", "stream closed"));
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  assert.deepEqual(
    b.take().map((m) => m.op),
    ["data"],
  );
  b.recv({ tango: "stream", id: 1, op: "ack", b: 2128 });
  const last = b.take();
  assert.deepEqual(
    last.map((m) => [m.op, m.seq]),
    [
      ["data", 4],
      ["end", 5],
    ],
  );
  assert.deepEqual(last[1], { tango: "stream", id: 1, op: "end", seq: 5 });
});

test("end before any frame and on a Go-opened stream", async () => {
  const { b, s } = openSender();
  s.end();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "end", seq: 0 }]);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 7 });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 1 });
  const got = [];
  for await (const v of s) got.push(v);
  assert.deepEqual(got, [7]);

  const b2 = fakeBridge();
  const c2 = createClient(b2);
  c2.onStream("echo", (gs) => {
    gs.send("hi");
    gs.end();
  });
  b2.recv({ tango: "stream", id: 2, op: "open", method: "echo", args: [] });
  assert.deepEqual(b2.take().slice(1), [
    { tango: "stream", id: 2, op: "accept", in: true },
    { tango: "stream", id: 2, op: "data", seq: 0, c: 68, data: "hi" },
    { tango: "stream", id: 2, op: "end", seq: 1 },
  ]);
});

test("send side stays open until end is posted", async () => {
  const { b, s } = openSender(4096);
  for (let i = 0; i < 5; i++) s.sendBytes(new Uint8Array(1000));
  s.end();
  b.recv({ tango: "stream", id: 1, op: "end", seq: 0 }); // Go ended its direction
  for await (const _ of s);
  b.take();
  // Not forgotten yet: Go's acks still release the queued frames and the end.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 3192 });
  assert.deepEqual(
    b.take().map((m) => [m.op, m.seq]),
    [
      ["data", 3],
      ["data", 4],
      ["end", 5],
    ],
  );
  // Both directions are over now: later messages are ignored.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 2128 });
  s.cancel();
  assert.deepEqual(b.take(), []);
});

test("un-awaited sends do not cause unhandled rejections", async () => {
  let unhandled = 0;
  const onUnhandled = () => unhandled++;
  process.on("unhandledRejection", onUnhandled);
  try {
    const { s } = openSender(4096);
    s.sendBytes(new Uint8Array(5000));
    s.send(1);
    s.send(2);
    s.send(3);
    s.cancel();
    s.send(4); // after cancel: rejects at once
    s.sendBytes(new Uint8Array(MAX_FRAME_BYTES + 1));
    await tick();
    await tick();
  } finally {
    process.off("unhandledRejection", onUnhandled);
  }
  assert.equal(unhandled, 0);
});

test("local cancel rejects queued and later sends", async () => {
  const { b, s } = openSender(4096);
  s.sendBytes(new Uint8Array(5000));
  const q = s.send(1);
  b.take();
  s.cancel();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "cancel" }]);
  await assert.rejects(q, isErr("canceled"));
  await assert.rejects(s.send(2), isErr("canceled"));
  assert.equal(s.bufferedAmount, 0);
  s.end();
  assert.deepEqual(b.take(), []);
});

test("remote close rejects pending sends; iterator drains", async () => {
  const { b, s } = openSender(4096);
  s.sendBytes(new Uint8Array(5000));
  const q1 = s.send(1);
  const q2 = s.send(2);
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: "a" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 65, data: "b" });
  b.recv({ tango: "stream", id: 1, op: "close", seq: 2 });
  await assert.rejects(q1, isErr("canceled", "stream closed by peer"));
  await assert.rejects(q2, isErr("canceled", "stream closed by peer"));
  await assert.rejects(s.send(3), isErr("canceled", "stream closed by peer"));
  assert.equal(s.bufferedAmount, 0);
  const got = [];
  for await (const v of s) got.push(v);
  assert.deepEqual(got, ["a", "b"]);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 5064 }); // late ack ignored
  assert.deepEqual(b.take(), []);
});

test("peer error and peer cancel reject sends", async () => {
  const e = openSender(4096);
  e.s.sendBytes(new Uint8Array(5000));
  const q = e.s.send(1);
  e.b.recv({ tango: "stream", id: 1, op: "error", seq: 0, error: { message: "disk full", code: "internal" } });
  await assert.rejects(q, isErr("internal", "disk full"));
  await assert.rejects(e.s.send(2), isErr("internal", "disk full"));

  const k = openSender(4096);
  k.s.sendBytes(new Uint8Array(5000));
  const q2 = k.s.send(1);
  k.b.recv({ tango: "stream", id: 1, op: "cancel" });
  await assert.rejects(q2, isErr("canceled"));
  await assert.rejects(k.s.send(2), isErr("canceled"));
});

test("internal _close finishes at once; _close(err) posts error", async () => {
  const { b, s } = openSender(4096);
  s.sendBytes(new Uint8Array(5000));
  const q = s.send(1);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: "unread" });
  b.take();
  s._close();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "close", seq: 1 }]);
  await assert.rejects(q, isErr("canceled", "stream closed"));
  await assert.rejects(s.send(2), isErr("canceled", "stream closed"));
  assert.deepEqual(await s[Symbol.asyncIterator]().next(), { value: undefined, done: true });
  await assert.rejects(s.accepted, isErr("canceled"));
  // Finished without waiting for Go: late messages are ignored.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 5064 });
  b.recv({ tango: "stream", id: 1, op: "close", seq: 1 });
  s.cancel();
  s._close();
  assert.deepEqual(b.take(), []);

  const e = openSender();
  e.s.send(1);
  e.b.take();
  const err = new TangoError("writer aborted", "canceled");
  e.s._close(err);
  assert.deepEqual(e.b.take(), [
    { tango: "stream", id: 1, op: "error", seq: 1, error: { message: "writer aborted", code: "canceled" } },
  ]);
  await assert.rejects(e.s.send(2), (x) => x === err);
  await assert.rejects(e.s[Symbol.asyncIterator]().next(), (x) => x === err);
});

// --- Web Streams views ---

function openReader(name = "Src") {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream(name);
  b.take();
  return { b, c, s };
}
const settle = async (n = 5) => {
  for (let i = 0; i < n; i++) await tick();
};

test("readable yields values and bytes then closes", async () => {
  const { b, s } = openReader();
  const r = s.readable;
  assert.ok(r instanceof ReadableStream);
  assert.equal(s.readable, r); // created once, on first access
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 66, bin: "aGk=" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 2, c: 64, bin: "" });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 3, c: 71, data: { a: 1 } });
  b.recv({ tango: "stream", id: 1, op: "end", seq: 4 });
  const got = [];
  for await (const v of r) got.push(v);
  assert.equal(got.length, 4);
  assert.equal(got[0], 1);
  assert.ok(got[1] instanceof Uint8Array);
  assert.deepEqual([...got[1]], [104, 105]);
  assert.ok(got[2] instanceof Uint8Array);
  assert.equal(got[2].length, 0);
  assert.deepEqual(got[3], { a: 1 });
});

test("readable pulls lazily: ack only after reads", async () => {
  const { b, s } = openReader("Big");
  const reader = s.readable.getReader();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 300000, data: 1 });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 300000, data: 2 });
  await settle();
  assert.deepEqual(b.take(), []); // nothing read yet: nothing acked
  assert.deepEqual(await reader.read(), { value: 1, done: false });
  await settle();
  assert.deepEqual(b.take(), []); // one frame still queued, below half a window
  assert.deepEqual(await reader.read(), { value: 2, done: false });
  await settle();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "ack", b: 600000 }]);
});

test("readable and iterator are exclusive", async () => {
  const { s } = openReader();
  s.readable.getReader();
  assert.throws(() => s[Symbol.asyncIterator]().next(), isErr("bad_args", "stream already consumed"));
  const k = openReader();
  k.s[Symbol.asyncIterator]();
  k.s[Symbol.asyncIterator](); // the iterator may be taken again
  assert.throws(() => k.s.readable, isErr("bad_args", "stream already consumed"));
});

test("readable errors with Go's error; cancel cancels the stream", async () => {
  const { b, s } = openReader();
  const reader = s.readable.getReader();
  const p = reader.read();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, data: 7 });
  assert.deepEqual(await p, { value: 7, done: false });
  b.recv({ tango: "stream", id: 1, op: "error", seq: 1, error: { message: "disk full", code: "internal" } });
  await assert.rejects(reader.read(), isErr("internal", "disk full"));

  const k = openReader();
  const kr = k.s.readable.getReader();
  const pending = kr.read();
  await kr.cancel("bye");
  assert.deepEqual(await pending, { value: undefined, done: true });
  assert.deepEqual(k.b.take(), [{ tango: "stream", id: 1, op: "cancel" }]);
  await assert.rejects(k.s.send(1), isErr("canceled"));
});

test("writable maps chunks, close → end", async () => {
  const { b, s } = openSender();
  const w = s.writable;
  assert.ok(w instanceof WritableStream);
  assert.equal(s.writable, w); // created once, on first access
  const writer = w.getWriter();
  await writer.write(new Uint8Array([1]));
  await writer.write({ a: 1 });
  await writer.close();
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "data", seq: 0, c: 65, bin: "AQ==" },
    { tango: "stream", id: 1, op: "data", seq: 1, c: 71, data: { a: 1 } },
    { tango: "stream", id: 1, op: "end", seq: 2 },
  ]);

  const k = openSender();
  const kw = k.s.writable.getWriter();
  await kw.write(new Uint8Array([1, 2, 3]).buffer);
  await kw.write(new DataView(new Uint8Array([9, 104, 105, 9]).buffer, 1, 2));
  await kw.write("s");
  await kw.write(undefined);
  assert.deepEqual(k.b.take(), [
    { tango: "stream", id: 1, op: "data", seq: 0, c: 67, bin: "AQID" },
    { tango: "stream", id: 1, op: "data", seq: 1, c: 66, bin: "aGk=" },
    { tango: "stream", id: 1, op: "data", seq: 2, c: 67, data: "s" },
    { tango: "stream", id: 1, op: "data", seq: 3, c: 68, data: null },
  ]);
});

test("writable close delivers credit-blocked frames before end", async () => {
  const { b, s } = openSender(4096);
  const writer = s.writable.getWriter();
  for (let i = 0; i < 5; i++) writer.write(new Uint8Array(1000));
  let closed = false;
  const cp = writer.close().then(() => (closed = true));
  await settle();
  assert.equal(dataOf(b.take()).length, 3);
  assert.equal(closed, false);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  await settle();
  assert.deepEqual(b.take().map((m) => m.op), ["data"]);
  assert.equal(closed, false); // end waits behind the last frame
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  await settle();
  const zeros = Buffer.from(new Uint8Array(1000)).toString("base64");
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "data", seq: 4, c: 1064, bin: zeros },
    { tango: "stream", id: 1, op: "end", seq: 5 },
  ]);
  await cp;
  assert.equal(closed, true);
});

test("writable close after end() resolves; rejects when sending stopped", async () => {
  const { b, s } = openSender();
  s.end();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "end", seq: 0 }]);
  await s.writable.getWriter().close();
  assert.deepEqual(b.take(), []);

  const k = openSender();
  k.b.recv({ tango: "stream", id: 1, op: "accept", in: false });
  await assert.rejects(k.s.writable.getWriter().close(), isErr("bad_args"));
  assert.deepEqual(k.b.take(), []);

  // The send direction stops while end waits for credit: close rejects.
  const g = openSender(4096);
  const gw = g.s.writable.getWriter();
  gw.write(new Uint8Array(4000));
  gw.write(new Uint8Array(1000)).catch(() => {});
  const cp = gw.close();
  await settle();
  g.b.recv({ tango: "stream", id: 1, op: "close", seq: 0 });
  await assert.rejects(cp, isErr("canceled", "stream closed by peer"));
});

test("writable abort sends error", async () => {
  const { b, s } = openSender();
  const writer = s.writable.getWriter();
  await writer.write(1);
  b.take();
  await writer.abort(new Error("stop"));
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "error", seq: 1, error: { message: "stop", code: "canceled" } },
  ]);
  await assert.rejects(s.send(2), isErr("canceled", "stop"));
  await assert.rejects(s[Symbol.asyncIterator]().next(), isErr("canceled", "stop"));
  await assert.rejects(s.accepted, isErr("canceled"));
  b.recv({ tango: "stream", id: 1, op: "ack", b: 65 }); // finished: ignored
  assert.deepEqual(b.take(), []);

  const k = openSender();
  await k.s.writable.abort("plain");
  assert.deepEqual(k.b.take(), [
    { tango: "stream", id: 1, op: "error", seq: 0, error: { message: "plain", code: "canceled" } },
  ]);
});

test("writable abort while a write waits for credit", async () => {
  const { b, s } = openSender(4096);
  const writer = s.writable.getWriter();
  await writer.write(new Uint8Array(4000));
  const blocked = writer.write(new Uint8Array(1000)); // waits for credit
  blocked.catch(() => {});
  await settle();
  b.take();
  const ap = writer.abort(new Error("stop"));
  await settle();
  // Posted at once, not after the blocked write (which never gets credit).
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "error", seq: 1, error: { message: "stop", code: "canceled" } },
  ]);
  await assert.rejects(blocked);
  await ap;
  assert.deepEqual(b.take(), []);
});

test("pipeTo respects credit", async () => {
  const { b, s } = openSender(4096);
  let made = 0;
  const src = new ReadableStream({
    pull(ctrl) {
      if (made < 10) {
        made++;
        ctrl.enqueue(new Uint8Array(1000));
      } else ctrl.close();
    },
  });
  let done = false;
  const pp = src.pipeTo(s.writable).then(() => (done = true));
  await settle(20);
  assert.equal(dataOf(b.take()).length, 3);
  await settle(20);
  assert.deepEqual(b.take(), []); // stopped: no credit
  assert.equal(done, false);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 });
  await settle(20);
  const all = b.take();
  assert.deepEqual(all.map((m) => [m.op, m.seq]), [["data", 3]]);
  for (let i = 0; i < 50 && !done; i++) {
    b.recv({ tango: "stream", id: 1, op: "ack", b: 1064 * 4 });
    await settle(5);
    all.push(...b.take());
  }
  await pp;
  assert.deepEqual(dataOf(all).map((m) => m.seq), [3, 4, 5, 6, 7, 8, 9]);
  assert.deepEqual(all.at(-1), { tango: "stream", id: 1, op: "end", seq: 10 });
});

test("writable write rejected with bad_args fails the stream and tells Go", async () => {
  const { b, c, s } = openSender();
  const writer = s.writable.getWriter();
  await writer.write(1);
  b.take();
  await assert.rejects(writer.write(10n), isErr("bad_args"));
  const msgs = b.take();
  assert.equal(msgs.length, 1);
  assert.equal(msgs[0].op, "error");
  assert.equal(msgs[0].seq, 1);
  assert.equal(msgs[0].error.code, "bad_args");
  assert.equal(c.streams.has(1), false); // finished: forgotten by the client
  await assert.rejects(s.send(2), isErr("bad_args"));
  b.recv({ tango: "stream", id: 1, op: "ack", b: 65 }); // late: ignored
  assert.deepEqual(b.take(), []);

  // The peer's own error is not echoed back.
  const k = openSender();
  const kw = k.s.writable.getWriter();
  k.b.recv({ tango: "stream", id: 1, op: "error", seq: 0, error: { message: "nope", code: "bad_args" } });
  await assert.rejects(kw.write(1), isErr("bad_args", "nope"));
  assert.deepEqual(k.b.take(), []);
});

test("credit is charged before posting (synchronous bridge acks)", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.recv({ tango: "hello", proto: 2, win: 4096 });
  const s = c.stream("Sink");
  // Go acks every data frame synchronously, inside post.
  const post = b.post;
  b.post = (msg) => {
    post(msg);
    const m = JSON.parse(msg);
    if (m.op === "data") b.recv({ tango: "stream", id: 1, op: "ack", b: m.c });
  };
  b.take();
  for (let i = 0; i < 10; i++) s.sendBytes(new Uint8Array(1000));
  await settle();
  assert.equal(dataOf(b.take()).length, 10); // no credit leaked: nothing blocks
  assert.equal(s.bufferedAmount, 0);
});

test("a throwing end post rejects close, does not escape into the ack path", async () => {
  const { b, c, s } = openSender(4096);
  const writer = s.writable.getWriter();
  writer.write(new Uint8Array(4000));
  writer.write(new Uint8Array(1000)); // waits for credit; end waits behind it
  const cp = writer.close();
  cp.catch(() => {});
  await settle();
  b.take();
  const post = b.post;
  const boom = new Error("bridge down");
  b.post = (msg) => {
    if (JSON.parse(msg).op === "end") throw boom;
    post(msg);
  };
  // The ack flushes the frame, then the end, whose post throws.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 4064 });
  await assert.rejects(cp, (e) => e === boom);
  assert.deepEqual(b.take().map((m) => m.op), ["data"]);
  // The stream still finishes once Go ends its side.
  b.recv({ tango: "stream", id: 1, op: "end", seq: 0 });
  await s[Symbol.asyncIterator]().next();
  assert.equal(c.streams.has(1), false);
});

test("raw messages are buffered until the first onRaw subscriber", () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  b.recv("plain text");
  b.recv('{"type":"user json"}');
  b.recv("[1,2]");
  b.recv({ tango: "event", name: "x", data: 1 }); // ipc: never raw
  const got = [];
  const off = c.onRaw((m) => got.push(m));
  b.recv("after");
  off();
  b.recv("dropped");
  assert.deepEqual(got, ["plain text", '{"type":"user json"}', "[1,2]", "after"]);
  c.postRaw("hi go");
  assert.deepEqual(b.take(), ["hi go"]);
});

test("IIFE exposes window.tango and shares one client per bridge", async () => {
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const b = fakeBridge();
  const ctx = vm.createContext({ __tango: b, setTimeout });
  ctx.window = ctx;
  vm.runInContext(src, ctx);
  vm.runInContext(src, ctx); // a second bundle (e.g. the ESM build) must reuse the client
  assert.deepEqual(b.take(), [{ tango: "hello", proto: 2 }]);
  assert.equal(typeof ctx.tango.call, "function");
  const p = ctx.tango.call("A.B", 1);
  const [m] = b.take();
  assert.deepEqual(m, { tango: "call", id: 1, method: "A.B", args: [1] });
  b.recv({ tango: "ret", id: 1, result: 2 });
  assert.equal(await p, 2);
});

test("module functions reject when not inside Tango", async () => {
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const ctx = vm.createContext({ setTimeout });
  vm.runInContext(src, ctx);
  await assert.rejects(ctx.tango.call("X"), (e) => e.code === "not_found");
});

// Regression: the core shim freezes window.__tango (bridge.go), so the IIFE
// must not try to cache its client on it.
test("IIFE exposes window.tango when __tango is frozen", () => {
  const posted = [];
  const ctx = {
    __tango: Object.freeze({ post: (s) => posted.push(s), onMessage: () => () => {} }),
  };
  ctx.globalThis = ctx;
  vm.createContext(ctx);
  vm.runInContext(readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8"), ctx);
  assert.equal(typeof ctx.tango, "object");
  assert.equal(typeof ctx.tango.call, "function");
  assert.deepEqual(posted, ['{"tango":"hello","proto":2}']);
});
