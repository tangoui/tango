import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

// down:android / up:android (src/lanes/android.ts) against a fake
// __tangoNative.lane. The module is internal: bundle it from src. copy
// bundles another copy (a second runtime on the same page, as the ESM
// build next to the injected IIFE).
async function bundle(copy) {
  const src = fileURLToPath(new URL("../src/", import.meta.url));
  const out = await build({
    stdin: {
      contents: `export * from "./lanes/android.ts"; export * from "./packet.ts"; export const COPY = ${copy};`,
      resolveDir: src,
      loader: "ts",
    },
    bundle: true,
    format: "esm",
    target: "es2020",
    write: false,
    logLevel: "warning",
  });
  return import("data:text/javascript;base64," + Buffer.from(out.outputFiles[0].text).toString("base64"));
}
const A = await bundle(1);
const { newAndroidDown, newAndroidUp, ANDROID_PROBE_TIMEOUT_MS, decodePacket } = A;

const CONN = Symbol.for("tango.androidLane");
const tick = () => new Promise((r) => setImmediate(r));

// A fresh page: a fake lane recording posted buffers (copies, as the
// structured clone makes) and the listen callback.
function page({ throwOnPost = false } = {}) {
  delete globalThis[CONN];
  const lane = {
    posts: [],
    fn: null,
    listens: 0,
    post(u8) {
      if (throwOnPost) throw new Error("post failed");
      lane.posts.push(Uint8Array.from(u8));
    },
    listen(fn) {
      lane.listens++;
      lane.fn = fn;
    },
    reply(...bytes) {
      lane.fn(Uint8Array.from(bytes).buffer);
    },
  };
  globalThis.__tangoNative = { post() {}, lane };
  return lane;
}

function ctxFor(gen) {
  const ctx = {
    gen,
    tok: "unused",
    packets: [],
    fails: [],
    onWire: [],
    onPacket(p) {
      ctx.packets.push(Uint8Array.from(p));
    },
    fail(e) {
      ctx.fails.push(e);
    },
    dispatched(stream, seq) {
      ctx.onWire.push([stream, seq]);
    },
    fetch() {
      throw new Error("no fetch on android lanes");
    },
  };
  return ctx;
}

const settled = (p) => {
  const s = { done: false, ok: undefined, err: undefined };
  p.then(
    () => Object.assign(s, { done: true, ok: true }),
    (e) => Object.assign(s, { done: true, ok: false, err: e }),
  );
  return s;
};

const genOf = (u8) => {
  const dv = new DataView(u8.buffer, u8.byteOffset);
  return dv.getUint32(16, true) + dv.getUint32(20, true) * 0x100000000;
};

test("start rejects at once without __tangoNative.lane", async () => {
  delete globalThis[CONN];
  globalThis.__tangoNative = { post() {} };
  const d = settled(newAndroidDown().start(ctxFor(1)));
  const u = settled(newAndroidUp().start(ctxFor(1)));
  await tick();
  assert.equal(d.ok, false);
  assert.equal(u.ok, false);
  delete globalThis.__tangoNative;
  const d2 = settled(newAndroidDown().start(ctxFor(1)));
  await tick();
  assert.equal(d2.ok, false);
});

test("probe: 26 bytes, gen little-endian, ends 00 00; resolves on [0]", async () => {
  const lane = page();
  const gen = 0x1_0000_0002; // above 2^32
  const down = newAndroidDown();
  const up = newAndroidUp();
  const d = settled(down.start(ctxFor(gen)));
  const u = settled(up.start(ctxFor(gen)));
  assert.equal(lane.posts.length, 1, "one probe for both modules");
  const p = lane.posts[0];
  assert.equal(p.length, 26);
  assert.deepEqual([...p.subarray(0, 16)], new Array(16).fill(0), "16 bytes headroom");
  assert.equal(genOf(p), gen);
  assert.deepEqual([...p.subarray(16, 24)], [2, 0, 0, 0, 1, 0, 0, 0]);
  assert.deepEqual([...p.subarray(24)], [0, 0]);
  await tick();
  assert.equal(d.done, false);
  lane.reply(0);
  await tick();
  assert.equal(d.ok, true);
  assert.equal(u.ok, true);
  assert.equal(lane.listens, 1);
  down.stop();
  up.stop();
});

test("probe: rejects on [1] and after 2000 ms without a reply", async () => {
  let lane = page();
  const a = settled(newAndroidDown().start(ctxFor(3)));
  lane.reply(1);
  await tick();
  assert.equal(a.ok, false);

  mock.timers.enable({ apis: ["setTimeout"] });
  try {
    lane = page();
    const b = settled(newAndroidUp().start(ctxFor(4)));
    mock.timers.tick(ANDROID_PROBE_TIMEOUT_MS - 1);
    await tick();
    assert.equal(b.done, false);
    mock.timers.tick(1);
    await tick();
    assert.equal(b.ok, false);
    assert.match(String(b.err), /no probe reply/);
  } finally {
    mock.timers.reset();
  }
});

test("stale reply ignored: a late [1] for the previous generation's probe does not fail the current start", async () => {
  mock.timers.enable({ apis: ["setTimeout"] });
  let lane;
  try {
    lane = page();
    const d1 = newAndroidDown();
    const first = settled(d1.start(ctxFor(5)));
    mock.timers.tick(ANDROID_PROBE_TIMEOUT_MS);
    await tick();
    assert.equal(first.ok, false);
    d1.stop();
  } finally {
    mock.timers.reset();
  }
  const d2 = newAndroidDown();
  const second = settled(d2.start(ctxFor(6)));
  assert.equal(lane.posts.length, 2);
  lane.reply(1); // the reply to generation 5's probe
  await tick();
  assert.equal(second.done, false);
  lane.reply(0); // generation 6's
  await tick();
  assert.equal(second.ok, true);
  d2.stop();
});

test("discards down packets before this probe's ack, delivers those after", async () => {
  const lane = page();
  const ctx = ctxFor(7);
  const d = newAndroidDown();
  const s = settled(d.start(ctx));
  lane.reply(0, 0, 9); // an earlier generation's packet, ahead of the ack
  lane.reply(0); // ack
  lane.reply(0, 1, 4, 4);
  await tick();
  assert.equal(s.ok, true);
  assert.deepEqual(ctx.packets.map((p) => [...p]), [[0, 1, 4, 4]]);
  d.stop();
});

test("up send: one packet per frame with 16-byte headroom and gen; dispatched in seq order", async () => {
  const lane = page();
  const ctx = ctxFor(0x2_0000_0001);
  const up = newAndroidUp();
  assert.equal(up.minSize, 0);
  const s = settled(up.start(ctx));
  lane.reply(0);
  await tick();
  assert.equal(s.ok, true);
  const pay = (n, k) => new Uint8Array(n).map((_, i) => (i + k) & 255);
  assert.equal(up.send({ stream: 3, seq: 0, payload: pay(5, 1) }), true);
  assert.equal(up.send({ stream: 3, seq: 1, payload: pay(0, 0) }), true);
  assert.equal(up.send({ stream: 2 ** 40 + 9, seq: 2 ** 33, payload: pay(70000, 2) }), true);
  assert.deepEqual(ctx.onWire, [[3, 0], [3, 1], [2 ** 40 + 9, 2 ** 33]]);
  const sent = lane.posts.slice(1);
  assert.equal(sent.length, 3);
  for (const p of sent) {
    assert.deepEqual([...p.subarray(0, 16)], new Array(16).fill(0));
    assert.equal(genOf(p), ctx.gen);
  }
  const frames = sent.map((p) => decodePacket(p.subarray(24)));
  assert.deepEqual(frames.map((f) => f.length), [1, 1, 1]);
  assert.equal(sent[0].length, 24 + 2 + 20 + 5);
  assert.deepEqual([...frames[0][0].payload], [...pay(5, 1)]);
  assert.equal(frames[2][0].stream, 2 ** 40 + 9);
  assert.equal(frames[2][0].seq, 2 ** 33);
  assert.deepEqual(frames[2][0].payload, pay(70000, 2));
  up.stop();
});

test("failsOnRefusal: [1] after start fails up:android via ctx.fail (not inside send)", async () => {
  const lane = page();
  const ctx = ctxFor(8);
  const up = newAndroidUp();
  const s = settled(up.start(ctx));
  lane.reply(0);
  await tick();
  assert.equal(s.ok, true);
  assert.equal(up.send({ stream: 1, seq: 0, payload: new Uint8Array(3) }), true);
  assert.equal(ctx.fails.length, 0);
  lane.reply(1);
  assert.equal(ctx.fails.length, 1);
  assert.match(String(ctx.fails[0]), /refused by Go/);
  assert.equal(up.send({ stream: 1, seq: 1, payload: new Uint8Array(3) }), false, "dead after the refusal");
  lane.reply(1);
  assert.equal(ctx.fails.length, 1);
});

test("post throwing: send returns false, ctx.fail runs in a microtask", async () => {
  const lane = page();
  const ctx = ctxFor(9);
  const up = newAndroidUp();
  const s = settled(up.start(ctx));
  lane.reply(0);
  await tick();
  assert.equal(s.ok, true);
  lane.post = () => {
    throw new Error("detached");
  };
  assert.equal(up.send({ stream: 1, seq: 0, payload: new Uint8Array(1) }), false);
  assert.equal(ctx.fails.length, 0, "not inside send");
  assert.deepEqual(ctx.onWire, []);
  await Promise.resolve();
  assert.equal(ctx.fails.length, 1);
  assert.match(String(ctx.fails[0]), /detached/);
});

test("two runtime copies share one connection; listen is called once", async () => {
  const B = await bundle(2);
  assert.notEqual(B.newAndroidDown, A.newAndroidDown);
  const lane = page();
  const ctx = ctxFor(10);
  const down = A.newAndroidDown();
  const up = B.newAndroidUp();
  const d = settled(down.start(ctx));
  const u = settled(up.start(ctxFor(10)));
  assert.equal(lane.listens, 1);
  assert.equal(lane.posts.length, 1, "one probe per generation across copies");
  lane.reply(0);
  await tick();
  assert.equal(d.ok, true);
  assert.equal(u.ok, true);
  lane.reply(0, 0);
  assert.equal(ctx.packets.length, 1);
  down.stop();
  up.stop();
});

test("after stop, messages are dropped; a new start probes again", async () => {
  const lane = page();
  const ctx = ctxFor(11);
  const down = newAndroidDown();
  const up = newAndroidUp();
  settled(down.start(ctx));
  settled(up.start(ctxFor(11)));
  lane.reply(0);
  await tick();
  down.stop();
  up.stop();
  lane.reply(0, 0, 1);
  lane.reply(1);
  assert.equal(ctx.packets.length, 0);
  const ctx2 = ctxFor(12);
  const down2 = newAndroidDown();
  const s = settled(down2.start(ctx2));
  assert.equal(lane.posts.length, 2);
  assert.equal(genOf(lane.posts[1]), 12);
  lane.reply(0);
  await tick();
  assert.equal(s.ok, true);
  lane.reply(0, 0, 2);
  assert.deepEqual(ctx2.packets.map((p) => [...p]), [[0, 0, 2]]);
  down2.stop();
});
