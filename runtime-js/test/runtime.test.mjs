import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { createClient, TangoError, WINDOW, ACK_EVERY } from "../dist/index.js";

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
  assert.deepEqual(b.take(), [{ tango: "hello" }]);
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

// Plays a Go producer honoring the ack window.
function produce(b, id, total) {
  let sent = 0;
  let acked = 0;
  let ended = false;
  const pump = () => {
    for (const m of b.take()) {
      if (m.op === "ack" && m.id === id) acked += m.n;
      if (m.op === "cancel" && m.id === id) ended = true;
    }
    while (!ended && sent < total && sent - acked < WINDOW) b.recv({ tango: "stream", id, op: "data", data: sent++ });
    if (!ended && sent === total) {
      ended = true;
      b.recv({ tango: "stream", id, op: "end" });
    }
    return { sent, acked };
  };
  return pump;
}

test("stream with acks and backpressure", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const s = c.stream("Count", 200);
  const [open] = b.take();
  assert.deepEqual(open, { tango: "stream", id: 1, op: "open", method: "Count", args: [200] });
  const pump = produce(b, 1, 200);
  assert.equal(pump().sent, WINDOW); // producer stops at the window
  const got = [];
  const acks = [];
  const origPost = b.post;
  b.post = (s) => {
    const m = JSON.parse(s);
    if (m.op === "ack") acks.push(m.n);
    origPost(s);
    pump();
  };
  for await (const v of s) got.push(v);
  assert.deepEqual(got, Array.from({ length: 200 }, (_, i) => i));
  // Go needed (200-64)/16 acks to finish; none are sent once it ended.
  assert.ok(acks.length >= Math.ceil((200 - WINDOW) / ACK_EVERY), `acks=${acks.length}`);
  assert.ok(acks.every((n) => n === ACK_EVERY));
});

test("stream error after buffered data", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Fail");
  b.recv({ tango: "stream", id: 1, op: "data", data: "a" });
  b.recv({ tango: "stream", id: 1, op: "data", data: "b" });
  b.recv({ tango: "stream", id: 1, op: "error", error: { message: "bad", code: "bad_args" } });
  const got = [];
  await assert.rejects(
    (async () => {
      for await (const v of s) got.push(v);
    })(),
    (e) => e instanceof TangoError && e.code === "bad_args",
  );
  assert.deepEqual(got, ["a", "b"]);
});

test("stream waiting consumer gets end and error", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Wait");
  const it = s[Symbol.asyncIterator]();
  const p = it.next();
  b.recv({ tango: "stream", id: 1, op: "data", data: 1 });
  assert.deepEqual(await p, { value: 1, done: false });
  const p2 = it.next();
  b.recv({ tango: "stream", id: 1, op: "error", error: { message: "x", code: "not_found" } });
  await assert.rejects(p2, (e) => e.code === "not_found");
  assert.deepEqual(await it.next(), { value: undefined, done: true });
});

test("break cancels the stream", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const s = c.stream("Forever");
  for (let i = 0; i < 5; i++) b.recv({ tango: "stream", id: 1, op: "data", data: i });
  const got = [];
  for await (const v of s) {
    got.push(v);
    if (v === 2) break;
  }
  assert.deepEqual(got, [0, 1, 2]);
  const msgs = b.take();
  assert.deepEqual(msgs.at(-1), { tango: "stream", id: 1, op: "cancel" });
  b.recv({ tango: "stream", id: 1, op: "data", data: 9 }); // late data ignored
  s.cancel(); // idempotent
  assert.equal(b.take().length, 0);
});

test("explicit cancel resolves a pending next", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const s = c.stream("Forever");
  const it = s[Symbol.asyncIterator]();
  const p = it.next();
  s.cancel();
  assert.deepEqual(await p, { value: undefined, done: true });
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
  b.recv({ tango: "stream", id: 2, op: "data", data: "goal" });
  b.recv({ tango: "stream", id: 2, op: "end" });
  await tick();
  assert.deepEqual(seen, [["sports", 3, "goal"], "done"]);

  b.recv({ tango: "stream", id: 4, op: "open", method: "missing", args: [] });
  const [err] = b.take();
  assert.equal(err.op, "error");
  assert.equal(err.id, 4);
  assert.equal(err.error.code, "not_found");
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
  assert.deepEqual(b.take(), [{ tango: "hello" }]);
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
  assert.deepEqual(posted, ['{"tango":"hello"}']);
});
