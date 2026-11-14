import { test } from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { createClient, lanes, GAP_TIMEOUT_MS, FRAME_OVERHEAD } from "../dist/index.js";

// registerLane and the packet codec are internal (not on the package
// entry): bundle them from src for the test. The lane registry lives on a
// global registry symbol, so modules registered here are seen by the
// client in dist/index.js.
const internal = await (async () => {
  const src = fileURLToPath(new URL("../src/", import.meta.url));
  const out = await build({
    stdin: { contents: 'export * from "./lanes.ts"; export * from "./packet.ts";', resolveDir: src, loader: "ts" },
    bundle: true,
    format: "esm",
    target: "es2020",
    write: false,
    logLevel: "warning",
  });
  return import("data:text/javascript;base64," + Buffer.from(out.outputFiles[0].text).toString("base64"));
})();
const { registerLane, encodePacket, decodePacket, INTERNAL_PREFIX } = internal;

function fakeBridge() {
  let sub = null;
  const b = {
    out: [],
    post(s) {
      let v = s;
      try {
        v = JSON.parse(s);
      } catch {}
      b.out.push(v);
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
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Fake lanes. Each test sets behaviour through `fake` before the hello.
// upDispatch: the fake up lane reports every frame it takes as dispatched
// from inside send (like up:post when the frame completes a batch);
// false: the test calls ctx.dispatched itself.
const fake = { down: [], up: [], upMin: 1000, upOk: true, upDispatch: true, startFail: new Set() };
function resetFake() {
  fake.down = [];
  fake.up = [];
  fake.upMin = 1000;
  fake.upOk = true;
  fake.upDispatch = true;
  fake.startFail = new Set();
}
registerLane("test:down", () => {
  const m = {
    ctx: null,
    stopped: false,
    start(ctx) {
      m.ctx = ctx;
      return fake.startFail.has("test:down") ? Promise.reject(new Error("no")) : Promise.resolve();
    },
    stop() {
      m.stopped = true;
    },
  };
  fake.down.push(m);
  return m;
});
registerLane("test:up", () => {
  const m = {
    ctx: null,
    stopped: false,
    sent: [],
    get minSize() {
      return fake.upMin;
    },
    start(ctx) {
      m.ctx = ctx;
      return fake.startFail.has("test:up") ? Promise.reject(new Error("no")) : Promise.resolve();
    },
    send(f) {
      m.sent.push(f);
      if (fake.upOk && fake.upDispatch) m.ctx.dispatched(f.stream, f.seq);
      return fake.upOk;
    },
    stop() {
      m.stopped = true;
    },
  };
  fake.up.push(m);
  return m;
});

const ctl = (op, lane, gen) => INTERNAL_PREFIX + "lane:" + JSON.stringify({ op, lane, gen });

function hello(b, list, gen = 3) {
  b.recv({ tango: "hello", proto: 2, win: 1048576, lanes: { gen, tok: "t", list } });
}

// A client whose hello offered list, with the lanes started.
async function laneClient(list, opts) {
  const b = fakeBridge();
  const c = createClient(b, opts);
  hello(b, list);
  await tick();
  b.take();
  return { b, c };
}

test("INTERNAL_PREFIX mirrors internal/corehook", () => {
  assert.equal(INTERNAL_PREFIX, "\x00tango-int:");
  assert.equal(GAP_TIMEOUT_MS, 5000);
});

test("packet round trip and rejects", () => {
  assert.deepEqual([...encodePacket([])], [0, 0]);
  const big = new Uint8Array(64 << 10).map((_, i) => (i * 7) & 255);
  const cases = [
    [],
    [{ stream: 1, seq: 0, payload: new Uint8Array(0) }],
    [
      { stream: 1, seq: 5, payload: new TextEncoder().encode("abc") },
      { stream: Number.MAX_SAFE_INTEGER, seq: 2 ** 40 + 9, payload: big },
      { stream: 2, seq: 0, payload: new Uint8Array(0) },
    ],
  ];
  for (const frames of cases) {
    const got = decodePacket(encodePacket(frames));
    assert.equal(got.length, frames.length);
    got.forEach((g, i) => {
      assert.equal(g.stream, frames[i].stream);
      assert.equal(g.seq, frames[i].seq);
      assert.deepEqual([...g.payload], [...frames[i].payload]);
    });
  }
  // Exact big-endian layout.
  assert.deepEqual(
    [...encodePacket([{ stream: 0x0102, seq: 3, payload: new Uint8Array([9]) }])],
    [0, 1, 0, 0, 0, 0, 0, 0, 1, 2, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 1, 9],
  );

  const header = (stream, seq, n, hi = 0) => {
    const u = new Uint8Array(20);
    const dv = new DataView(u.buffer);
    dv.setUint32(0, hi);
    dv.setUint32(4, stream);
    dv.setUint32(12, seq);
    dv.setUint32(16, n);
    return [...u];
  };
  const valid = [...encodePacket([{ stream: 1, seq: 0, payload: new TextEncoder().encode("abc") }])];
  const rejects = {
    empty: [],
    "one byte": [0],
    "short header": [0, 1, ...header(1, 0, 0).slice(0, 19)],
    "len past end": [0, 1, ...header(1, 0, 4), 97, 98, 99],
    "len over max": [0, 1, ...header(1, 0, 16777217)],
    "trailing bytes": [...valid, 0],
    "count too high": [0, 2, ...valid.slice(2)],
    "empty plus extra": [0, 0, 0],
    "stream over 2^53": [0, 1, ...header(0, 0, 0, 0x200000)],
  };
  for (const [name, p] of Object.entries(rejects)) {
    assert.throws(() => decodePacket(new Uint8Array(p)), undefined, name);
  }
  assert.throws(() => encodePacket([{ stream: 2 ** 64, seq: 0, payload: new Uint8Array(0) }]));
});

test("hello lanes starts modules and posts ready", async () => {
  resetFake();
  const b = fakeBridge();
  createClient(b);
  b.take();
  hello(b, ["test:down", "test:up", "nope"]);
  assert.equal(fake.down.length, 1);
  assert.equal(fake.up.length, 1);
  assert.equal(fake.down[0].ctx.gen, 3);
  assert.equal(fake.down[0].ctx.tok, "t");
  assert.equal(typeof fake.down[0].ctx.fetch, "function");
  await tick();
  const out = b.take();
  assert.ok(out.includes(ctl("ready", "test:down", 3)), JSON.stringify(out));
  assert.ok(out.includes(ctl("ready", "test:up", 3)), JSON.stringify(out));
  assert.equal(out.length, 2);
});

test("start failure posts fail", async () => {
  resetFake();
  fake.startFail.add("test:down");
  const { c } = await (async () => {
    const b = fakeBridge();
    const c = createClient(b);
    b.take();
    hello(b, ["test:down"]);
    await tick();
    assert.deepEqual(b.take(), [ctl("fail", "test:down", 3)]);
    return { b, c };
  })();
  assert.equal(fake.down[0].stopped, true);
  assert.deepEqual(c.lanes(), { down: "fallback", up: "fallback" });
});

test("a new hello or a failed client stops every lane", async () => {
  resetFake();
  const { b } = await laneClient(["test:down", "test:up"]);
  b.recv({ tango: "hello", proto: 2, win: 1048576 });
  assert.equal(fake.down[0].stopped, true);
  assert.equal(fake.up[0].stopped, true);

  resetFake();
  const x = await laneClient(["test:down", "test:up"]);
  const saved = globalThis.setTimeout;
  const reported = [];
  globalThis.setTimeout = (fn) => {
    try {
      fn();
    } catch (e) {
      reported.push(e);
    }
  };
  try {
    x.b.recv({ tango: "hello", error: { message: "bad", code: "bad_args" } });
    // A later hello does not restart lanes on a failed client.
    hello(x.b, ["test:down"]);
  } finally {
    globalThis.setTimeout = saved;
  }
  assert.equal(reported.length, 1);
  assert.equal(fake.down[0].stopped, true);
  assert.equal(fake.up[0].stopped, true);
  assert.equal(fake.down.length, 1);
});

test("down packets feed streams in seq order", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"]);
  const s = c.stream("Bytes");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 66, bin: "AAE=" }); // [0,1]
  const payload = new Uint8Array(100).fill(7);
  fake.down[0].ctx.onPacket(encodePacket([{ stream: 1, seq: 1, payload }, { stream: 99, seq: 0, payload }]));
  b.recv({ tango: "stream", id: 1, op: "data", seq: 2, c: 65, bin: "Ag==" }); // [2]
  const it = s[Symbol.asyncIterator]();
  const r0 = await it.next();
  const r1 = await it.next();
  const r2 = await it.next();
  assert.deepEqual([...r0.value], [0, 1]);
  assert.deepEqual([...r1.value], [...payload]);
  assert.deepEqual([...r2.value], [2]);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "ack", b: 66 + 100 + FRAME_OVERHEAD + 65 }]);
});

test("bad down packets are dropped with one warning", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"]);
  const warn = console.warn;
  let warned = 0;
  console.warn = () => warned++;
  try {
    const s = c.stream("Bytes");
    b.take();
    fake.down[0].ctx.onPacket(new Uint8Array([0, 1, 2]));
    fake.down[0].ctx.onPacket(new Uint8Array([9]));
    fake.down[0].ctx.onPacket(encodePacket([{ stream: 1, seq: 0, payload: new Uint8Array([5]) }]));
    const r = await s[Symbol.asyncIterator]().next();
    assert.deepEqual([...r.value], [5]);
  } finally {
    console.warn = warn;
  }
  assert.equal(warned, 1);
});

test("up routing by minSize", async () => {
  resetFake();
  const b = fakeBridge();
  const c = createClient(b);
  b.recv({ tango: "hello", proto: 2, win: 3000, lanes: { gen: 3, tok: "t", list: ["test:up"] } });
  await tick();
  b.take();
  const s = c.stream("Up");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  await s.sendBytes(new Uint8Array(10));
  const [m0] = b.take();
  assert.equal(m0.op, "data");
  assert.equal(m0.seq, 0);
  assert.equal(m0.c, 10 + FRAME_OVERHEAD);
  assert.equal(typeof m0.bin, "string");
  const big = new Uint8Array(2000).fill(3);
  await s.sendBytes(big);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "boundary", seq: 2 }]);
  const up = fake.up[0];
  assert.equal(up.sent.length, 1);
  assert.equal(up.sent[0].stream, 1);
  assert.equal(up.sent[0].seq, 1);
  assert.deepEqual([...up.sent[0].payload], [...big]);
  // Credit as usual: 74 + 2064 in flight in a 3000-byte window, so the
  // next 2000-byte frame waits for an ack.
  let done = false;
  const p = s.sendBytes(big).then(() => (done = true));
  await tick();
  assert.equal(done, false);
  assert.equal(up.sent.length, 1);
  b.recv({ tango: "stream", id: 1, op: "ack", b: 2064 });
  await p;
  assert.equal(up.sent.length, 2);
  assert.equal(up.sent[1].seq, 2);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "boundary", seq: 3 }]);
  // JSON frames never take the lane; end counts lane frames.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 74 + 2064 });
  await s.send("x".repeat(2000));
  s.end();
  const out = b.take();
  assert.equal(out[0].op, "data");
  assert.equal(out[0].seq, 3);
  assert.deepEqual(out[1], { tango: "stream", id: 1, op: "end", seq: 4 });
  assert.equal(up.sent.length, 2);
});

for (const { secondaryFailure = false, reply } of [
  {},
  { secondaryFailure: true },
  { reply: "close" },
  { reply: "error" },
  { reply: "ack" },
]) {
  test(`a throwing boundary post settles ack-flushed sends${secondaryFailure ? " even if error post fails" : ""}${reply ? ` with synchronous peer ${reply} during error post` : ""}`, { timeout: 2000 }, async () => {
    resetFake();
    const b = fakeBridge();
    const c = createClient(b);
    b.recv({ tango: "hello", proto: 2, win: 2064, lanes: { gen: 3, tok: "t", list: ["test:up"] } });
    await tick();
    const s = c.stream("Up");
    const big = new Uint8Array(2000).fill(7);
    // Fill the credit window on the message path before accept; only the
    // ack-flushed frame below is accepted by the lane.
    await s.sendBytes(big);
    b.recv({ tango: "stream", id: 1, op: "accept", in: true });
    b.take();
    const sends = [s.sendBytes(big), s.sendBytes(big), s.sendBytes(big)];
    // Leave every send unobserved across a turn: their internal quiet
    // catches must handle both the dequeued and queued rejections.
    assert.equal(s.bufferedAmount, 6192);
    assert.equal(fake.up.at(-1).sent.length, 0);
    const original = new Error("boundary bridge failed");
    const secondary = new Error("error bridge failed");
    const attempts = [];
    const notificationStates = [];
    const post = b.post;
    b.post = (raw) => {
      const m = JSON.parse(raw);
      attempts.push(m);
      if (m.op === "boundary") throw original;
      if (m.op === "error") notificationStates.push([c.streams.has(1), s.bufferedAmount, s.sendDone]);
      if (secondaryFailure && m.op === "error") throw secondary;
      if (m.op === "error" && reply) {
        // Reenter during notification, not after local cleanup returned.
        b.recv({
          tango: "stream", id: 1, op: reply, seq: 0,
          ...(reply === "error" ? { error: { message: "peer failed", code: "internal" } } : {}),
          ...(reply === "ack" ? { b: 2064 } : {}),
        });
      }
      post(raw);
    };
    const timers = [];
    const setTimeout = globalThis.setTimeout;
    globalThis.setTimeout = (fn) => { timers.push(fn); return 0; };
    try {
      assert.doesNotThrow(() => b.recv({ tango: "stream", id: 1, op: "ack", b: 2064 }));
    } finally {
      globalThis.setTimeout = setTimeout;
    }
    await tick();
    const settled = [];
    sends[0].then(() => settled.push("resolved"), (e) => settled.push(e));
    await tick();
    assert.deepEqual(settled, [original], "the dequeued send rejects promptly with the original error");
    await assert.rejects(sends[0], (e) => e === original);
    for (const p of sends.slice(1)) {
      await assert.rejects(p, (e) => e.code === "transport" && e.message === original.message);
    }
    assert.equal(s.bufferedAmount, 0);
    assert.equal(c.streams.has(1), false);
    const failure = await s._finished;
    assert.equal(failure?.code, "transport", "local transport failure wins over a synchronous peer terminal");
    assert.equal(failure.message, original.message);
    assert.equal(s.sent, 2, "accepted sequence must not be rolled back");
    assert.equal(s.inflight, 2064, "accepted lane bytes remain charged");
    assert.deepEqual(fake.up.at(-1).sent, [{ stream: 1, seq: 1, payload: big }]);
    assert.deepEqual(attempts.map((m) => [m.op, m.seq]), [["boundary", 2], ["error", 2]]);
    assert.equal(attempts[1].error.code, "transport");
    assert.deepEqual(notificationStates, [[false, 0, true]], "finish locally before notifying the peer");
    assert.equal(attempts.some((m) => m.op === "data"), false, "never replay lane-accepted bytes via messages");
    assert.equal(timers.length, secondaryFailure ? 1 : 0);
    if (secondaryFailure) assert.throws(timers[0], (e) => e === secondary);
    b.recv({ tango: "stream", id: 1, op: "error", seq: 0, error: { message: "late", code: "internal" } });
    b.recv({ tango: "stream", id: 1, op: "ack", b: 2064 });
    await assert.rejects(sends[0], (e) => e === original);
    await assert.rejects(s.sendBytes(big), (e) => e.code === "transport");
    await tick();
    assert.deepEqual(settled, [original]);
    assert.equal(fake.up.at(-1).sent.length, 1, "late messages and sends cannot reuse the sequence");
    assert.equal(attempts.length, 2);
    assert.equal(s.sent, 2);
    assert.equal(s.bufferedAmount, 0);
    assert.equal(c.streams.has(1), false);
  });
}

test("a stream JS opens uses the up lane only after accept", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:up"]);
  const s = c.stream("Up");
  b.take();
  const big = new Uint8Array(2000).fill(1);
  // Before accept a lane packet could overtake the open: message channel.
  await s.sendBytes(big);
  assert.equal(fake.up[0].sent.length, 0);
  const [m] = b.take();
  assert.equal(m.op, "data");
  assert.equal(m.seq, 0);
  assert.equal(typeof m.bin, "string");
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  await s.sendBytes(big);
  assert.equal(fake.up[0].sent.length, 1);
  assert.equal(fake.up[0].sent[0].seq, 1);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "boundary", seq: 2 }]);
});

test("a stream Go opens uses the up lane at once", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:up"]);
  let got;
  c.onStream("Feed", (s) => (got = s));
  b.recv({ tango: "stream", id: 2, op: "open", method: "Feed", args: [] });
  assert.deepEqual(b.take(), [{ tango: "stream", id: 2, op: "accept", in: true }]);
  await got.sendBytes(new Uint8Array(2000));
  assert.equal(fake.up[0].sent.length, 1);
  assert.equal(fake.up[0].sent[0].stream, 2);
  assert.equal(fake.up[0].sent[0].seq, 0);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 2, op: "boundary", seq: 1 }]);
});

test("up lane rejection falls back", async () => {
  resetFake();
  fake.upOk = false;
  const { b, c } = await laneClient(["test:up"]);
  const s = c.stream("Up");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  await s.sendBytes(new Uint8Array(2000));
  assert.equal(fake.up[0].sent.length, 1);
  const [m] = b.take();
  assert.equal(m.op, "data");
  assert.equal(m.seq, 0);
  assert.equal(m.c, 2000 + FRAME_OVERHEAD);
  assert.equal(typeof m.bin, "string");
  assert.equal(c.lanes().up, "test:up");
});

test("lane fail posts fail and falls back", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down", "test:up"]);
  assert.deepEqual(c.lanes(), { down: "test:down", up: "test:up" });
  fake.up[0].ctx.fail(new Error("broken"));
  assert.deepEqual(b.take(), [ctl("fail", "test:up", 3)]);
  assert.equal(fake.up[0].stopped, true);
  assert.deepEqual(c.lanes(), { down: "test:down", up: "fallback" });
  const s = c.stream("Up");
  b.take();
  await s.sendBytes(new Uint8Array(2000));
  assert.equal(fake.up[0].sent.length, 0);
  assert.equal(b.take()[0].op, "data");
  // A stopped lane's late callbacks are ignored.
  fake.up[0].ctx.fail(new Error("again"));
  assert.deepEqual(b.take(), []);

  fake.down[0].ctx.fail("down broke");
  assert.deepEqual(b.take(), [ctl("fail", "test:down", 3)]);
  assert.deepEqual(c.lanes(), { down: "fallback", up: "fallback" });
});

test("gap timeout fails stream with transport", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 50 });
  const s = c.stream("Gap");
  const t = c.stream("Other");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 65, bin: "AQ==" });
  b.recv({ tango: "stream", id: 3, op: "data", seq: 0, c: 65, bin: "AQ==" });
  await sleep(20);
  assert.deepEqual(b.take(), []);
  await sleep(60);
  const out = b.take();
  assert.equal(out.length, 1);
  assert.equal(out[0].id, 1);
  assert.equal(out[0].op, "error");
  assert.equal(out[0].seq, 0);
  assert.equal(out[0].error.code, "transport");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "transport");
  const r = await t[Symbol.asyncIterator]().next();
  assert.deepEqual([...r.value], [1]);
});

test("gap timer restarts on progress and stops when the gap closes", async () => {
  // A 400 ms timer and two 220 ms steps: each step tolerates 180 ms of
  // scheduler delay; the second step always ends past the first deadline.
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 400 });
  const s = c.stream("Gap");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "data", seq: 2, c: 65, bin: "Ag==" });
  await sleep(220);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 0, c: 65, bin: "AA==" }); // progress, seq 1 still missing
  await sleep(220); // 440ms since the gap opened, 220 since progress
  assert.deepEqual(b.take(), []);
  b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 65, bin: "AQ==" }); // gap closed
  await sleep(450);
  assert.deepEqual(b.take(), []);
  const it = s[Symbol.asyncIterator]();
  for (const want of [0, 1, 2]) assert.deepEqual([...(await it.next()).value], [want]);
});

test("gap before end counts as a gap", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 40 });
  const s = c.stream("Gap");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "end", seq: 1 }); // frame 0 not here yet
  await sleep(80);
  const out = b.take();
  assert.equal(out.length, 1);
  assert.equal(out[0].op, "error");
  assert.equal(out[0].error.code, "transport");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "transport");

  // The frame arriving in time closes the gap.
  const s2 = c.stream("Gap2");
  b.take();
  b.recv({ tango: "stream", id: 3, op: "end", seq: 1 });
  fake.down[0].ctx.onPacket(encodePacket([{ stream: 3, seq: 0, payload: new Uint8Array([4]) }]));
  await sleep(80);
  assert.deepEqual(b.take(), []);
  const it = s2[Symbol.asyncIterator]();
  assert.deepEqual([...(await it.next()).value], [4]);
  assert.equal((await it.next()).done, true);
});

test("lanes() reports active lanes", async () => {
  resetFake();
  const b = fakeBridge();
  const c = createClient(b);
  assert.deepEqual(c.lanes(), { down: "fallback", up: "fallback" });
  hello(b, ["nope", "test:up", "test:down"]);
  assert.deepEqual(c.lanes(), { down: "fallback", up: "fallback" }); // not ready yet
  await tick();
  assert.deepEqual(c.lanes(), { down: "test:down", up: "test:up" });
  // The module-level lanes() needs window.__tango.
  assert.throws(() => lanes(), (e) => e.code === "not_found");
  globalThis.__tango = b;
  try {
    assert.deepEqual(lanes(), { down: "fallback", up: "fallback" });
  } finally {
    delete globalThis.__tango;
  }
});


test("reliable boundary fails only the lost open duplex tail without later data or end", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 30 });
  const lost = c.stream("Lost"), good = c.stream("Good");
  b.take();
  b.recv({ tango: "stream", id: 1, op: "boundary", seq: 1 });
  // A fully delivered parallel stream owns no unresolved boundary.
  b.recv({ tango: "stream", id: 3, op: "boundary", seq: 1 });
  fake.down[0].ctx.onPacket(encodePacket([{ stream: 3, seq: 0, payload: new Uint8Array([7]) }]));
  fake.down[0].ctx.fail(new Error("definitive lane loss"));
  await sleep(70);
  const errors = b.take().filter(m => m.op === "error");
  assert.deepEqual(errors.map(m => [m.id, m.error.code]), [[1, "transport"]]);
  await assert.rejects(lost[Symbol.asyncIterator]().next(), e => e.code === "transport");
  assert.deepEqual([...(await good[Symbol.asyncIterator]().next()).value], [7]);
  good.cancel();
});

// ---- I2: boundaries follow dispatch, not acceptance ----

// An accepted stream on a client whose up lane dispatches only when told.
async function manualUp(win = 1048576) {
  resetFake();
  fake.upDispatch = false;
  const b = fakeBridge();
  const c = createClient(b);
  b.recv({ tango: "hello", proto: 2, win, lanes: { gen: 3, tok: "t", list: ["test:up"] } });
  await tick();
  const s = c.stream("Up");
  b.recv({ tango: "stream", id: 1, op: "accept", in: true });
  b.take();
  return { b, c, s, up: fake.up[0] };
}

test("an up-lane boundary is posted only once its frame is dispatched", async () => {
  const { b, s, up } = await manualUp();
  const big = new Uint8Array(2000).fill(5);
  // The sends resolve at acceptance (credit is charged); nothing tells Go
  // about the frames until the lane puts them on the wire.
  await s.sendBytes(big);
  await s.sendBytes(big);
  s.end();
  assert.equal(up.sent.length, 2);
  assert.deepEqual(b.take(), [], "no boundary and no end before dispatch");
  up.ctx.dispatched(1, 0);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "boundary", seq: 1 }]);
  up.ctx.dispatched(1, 1);
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "boundary", seq: 2 },
    { tango: "stream", id: 1, op: "end", seq: 2 },
  ]);
  // A repeated or stale dispatch posts nothing.
  up.ctx.dispatched(1, 1);
  up.ctx.dispatched(1, 0);
  assert.deepEqual(b.take(), []);
});

test("a queued close waits for dispatch too", async () => {
  const { b, s, up } = await manualUp();
  await s.sendBytes(new Uint8Array(2000));
  s._queueClose();
  assert.deepEqual(b.take(), []);
  up.ctx.dispatched(1, 0);
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "boundary", seq: 1 },
    { tango: "stream", id: 1, op: "close", seq: 1 },
  ]);
});

test("a throwing boundary post at dispatch time fails the stream with transport", async () => {
  // Window 2064: the second frame waits for credit, so it is still queued
  // when the dispatch fails.
  const { b, c, s, up } = await manualUp(2064);
  const big = new Uint8Array(2000).fill(7);
  await s.sendBytes(big);
  const queued = s.sendBytes(big);
  const original = new Error("boundary bridge failed");
  const attempts = [];
  const post = b.post;
  b.post = (raw) => {
    const m = JSON.parse(raw);
    attempts.push(m);
    if (m.op === "boundary") throw original;
    post(raw);
  };
  assert.doesNotThrow(() => up.ctx.dispatched(1, 0));
  await assert.rejects(queued, (e) => e.code === "transport" && e.message === original.message);
  const failure = await s._finished;
  assert.equal(failure?.code, "transport");
  assert.equal(failure.message, original.message);
  assert.equal(s.sent, 1, "accepted sequence must not be rolled back");
  assert.equal(s.inflight, 2064, "accepted lane bytes remain charged");
  assert.equal(s.bufferedAmount, 0);
  assert.equal(c.streams.has(1), false);
  assert.deepEqual(attempts.map((m) => [m.op, m.seq]), [["boundary", 1], ["error", 1]]);
  assert.equal(attempts[1].error.code, "transport");
  assert.equal(attempts.some((m) => m.op === "data"), false, "never replay lane-accepted bytes via messages");
  // Late messages, dispatches and sends change nothing.
  b.recv({ tango: "stream", id: 1, op: "ack", b: 2064 });
  up.ctx.dispatched(1, 0);
  await assert.rejects(s.sendBytes(big), (e) => e.code === "transport");
  assert.equal(attempts.length, 2);
  assert.equal(up.sent.length, 1);
});

test("a stream that finished before its frame was dispatched ignores the dispatch", async () => {
  const { b, s, up } = await manualUp();
  await s.sendBytes(new Uint8Array(2000));
  s.cancel();
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "cancel" }]);
  up.ctx.dispatched(1, 0);
  assert.deepEqual(b.take(), []);
});

test("an up lane that dies releases undispatched frames as lost", async () => {
  const { b, s, up } = await manualUp();
  await s.sendBytes(new Uint8Array(2000));
  s.end();
  assert.deepEqual(b.take(), []);
  up.ctx.fail(new Error("broken"));
  // The frame is lost: the boundary lets Go detect it (transport after its
  // gap timeout), and the end is no longer held.
  assert.deepEqual(b.take(), [
    { tango: "stream", id: 1, op: "boundary", seq: 1 },
    { tango: "stream", id: 1, op: "end", seq: 1 },
    ctl("fail", "test:up", 3),
  ]);
  // A dead lane's late dispatch is ignored.
  up.ctx.dispatched(1, 0);
  assert.deepEqual(b.take(), []);
});

// ---- I2(b): a gap timer that fires late (stalled main thread) re-arms once ----

test("a late gap timer re-arms once, then fails on the second expiry", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 5000 });
  const s = c.stream("Gap");
  b.take();
  const timers = [];
  const savedSet = globalThis.setTimeout;
  const savedNow = Date.now;
  let now = 1_000_000;
  globalThis.setTimeout = (fn, ms) => {
    timers.push({ fn, ms });
    return timers.length;
  };
  Date.now = () => now;
  try {
    b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 65, bin: "AQ==" }); // seq 0 missing
    assert.equal(timers.length, 1);
    assert.equal(timers[0].ms, 5000);
    // The page stalled: the timer fires 1.5 s after its deadline.
    now += 5000 + 1500;
    timers[0].fn();
    assert.deepEqual(b.take(), [], "a late expiry does not fail the stream");
    assert.equal(timers.length, 2, "re-armed");
    assert.equal(timers[1].ms, 5000, "with the full timeout");
    // The second expiry fails as before, even when late again.
    now += 5000 + 1500;
    timers[1].fn();
  } finally {
    globalThis.setTimeout = savedSet;
    Date.now = savedNow;
  }
  const out = b.take();
  assert.equal(out.length, 1);
  assert.equal(out[0].op, "error");
  assert.equal(out[0].error.code, "transport");
  await assert.rejects(s[Symbol.asyncIterator]().next(), (e) => e.code === "transport");
});

test("an on-time gap timer (late by at most 1 s) fails at once", async () => {
  resetFake();
  const { b, c } = await laneClient(["test:down"], { gapTimeoutMs: 5000 });
  c.stream("Gap");
  b.take();
  const timers = [];
  const savedSet = globalThis.setTimeout;
  const savedNow = Date.now;
  let now = 2_000_000;
  globalThis.setTimeout = (fn, ms) => {
    timers.push({ fn, ms });
    return timers.length;
  };
  Date.now = () => now;
  try {
    b.recv({ tango: "stream", id: 1, op: "data", seq: 1, c: 65, bin: "AQ==" });
    now += 5000 + 1000;
    timers[0].fn();
  } finally {
    globalThis.setTimeout = savedSet;
    Date.now = savedNow;
  }
  assert.equal(timers.length, 1, "not re-armed");
  const out = b.take();
  assert.equal(out.length, 1);
  assert.equal(out[0].error.code, "transport");
});
