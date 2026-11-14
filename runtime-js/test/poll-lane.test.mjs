import { test } from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { createClient } from "../dist/index.js";

// down:poll: the core's downShim polls /__tango/lane/poll and hands byte
// packets to window.__tango._onPacket subscribers; the lane module only
// subscribes and passes each packet to the client. The module and the
// packet codec are internal: bundle them from src for the test.
const internal = await (async () => {
  const src = fileURLToPath(new URL("../src/", import.meta.url));
  const out = await build({
    stdin: {
      contents: 'export * from "./lanes/poll.ts"; export * from "./packet.ts"; export * from "./lanes.ts";',
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
})();
const { newPollDown, encodePacket, INTERNAL_PREFIX } = internal;

const tick = () => new Promise((r) => setImmediate(r));
const bytes = (n, seed = 1) => new Uint8Array(n).map((_, i) => (i * seed + 3) & 255);
const ctl = (op, gen) => INTERNAL_PREFIX + "lane:" + JSON.stringify({ op, lane: "down:poll", gen });

// A bridge like the core's: post/onMessage, plus _onPacket when hook is
// true (a core with downShim). push(p) delivers a packet to the current
// packet subscribers.
function fakeCore({ hook = true } = {}) {
  let sub = null;
  const b = {
    out: [],
    pktSubs: [],
    unsubscribed: 0,
    post(s) {
      b.out.push(s);
    },
    onMessage(fn) {
      sub = fn;
      return () => (sub = null);
    },
    recv(m) {
      sub(typeof m === "string" ? m : JSON.stringify(m));
    },
    push(p) {
      for (const fn of [...b.pktSubs]) fn(p);
    },
  };
  if (hook) {
    b._onPacket = (fn) => {
      b.pktSubs.push(fn);
      return () => {
        const i = b.pktSubs.indexOf(fn);
        if (i >= 0) {
          b.pktSubs.splice(i, 1);
          b.unsubscribed++;
        }
      };
    };
  }
  return b;
}

// Installs b as window.__tango for the test (the lane reads the global).
function asCore(t, b) {
  const prev = Object.getOwnPropertyDescriptor(globalThis, "__tango");
  globalThis.__tango = b;
  t.after(() => {
    if (prev) Object.defineProperty(globalThis, "__tango", prev);
    else delete globalThis.__tango;
  });
}

const hello = (gen, list = ["down:poll"]) => ({ tango: "hello", proto: 2, win: 1048576, lanes: { gen, tok: "t", list } });

test("poll lane: packets reach client", async (t) => {
  const b = fakeCore();
  asCore(t, b);
  const c = createClient(b);
  b.recv(hello(3));
  await tick();
  assert.deepEqual(b.out.filter((s) => s.startsWith(INTERNAL_PREFIX)), [ctl("ready", 3)]);
  assert.deepEqual(c.lanes().down, "down:poll");
  assert.equal(b.pktSubs.length, 1);

  let s;
  c.onStream("Feed", (x) => (s = x));
  b.recv({ tango: "stream", id: 2, op: "open", method: "Feed", args: [] });
  const one = bytes(5000, 3);
  const two = bytes(7000, 5);
  b.push(encodePacket([{ stream: 2, seq: 0, payload: one }]));
  b.push(encodePacket([{ stream: 2, seq: 1, payload: two }]));
  const it = s[Symbol.asyncIterator]();
  assert.deepEqual(new Uint8Array((await it.next()).value), one);
  assert.deepEqual(new Uint8Array((await it.next()).value), two);
});

test("poll lane: packets reach LaneContext.onPacket", async () => {
  const b = fakeCore();
  const prev = globalThis.__tango;
  globalThis.__tango = b;
  try {
    const got = [];
    const lane = newPollDown();
    await lane.start({ gen: 1, tok: "t", onPacket: (p) => got.push(p), fail() {}, dispatched() {}, fetch() {} });
    const p1 = new Uint8Array([0, 0]);
    const p2 = encodePacket([{ stream: 4, seq: 0, payload: bytes(10) }]);
    b.push(p1);
    b.push(p2);
    assert.deepEqual(got, [p1, p2]);
    lane.stop();
    assert.equal(b.pktSubs.length, 0);
  } finally {
    globalThis.__tango = prev;
  }
});

test("poll lane: missing hook fails", async (t) => {
  const b = fakeCore({ hook: false });
  asCore(t, b);
  const c = createClient(b);
  b.recv(hello(5));
  await tick();
  assert.deepEqual(b.out.filter((s) => s.startsWith(INTERNAL_PREFIX)), [ctl("fail", 5)]);
  assert.equal(c.lanes().down, "fallback");
});

test("poll lane: stop unsubscribes", async (t) => {
  const b = fakeCore();
  asCore(t, b);
  const c = createClient(b);
  b.recv(hello(1));
  await tick();
  assert.equal(b.pktSubs.length, 1);
  const first = b.pktSubs[0];
  b.recv(hello(2));
  await tick();
  assert.equal(b.unsubscribed, 1, "the old generation's subscription was cancelled");
  assert.equal(b.pktSubs.length, 1);
  assert.notEqual(b.pktSubs[0], first);
  assert.deepEqual(b.out.filter((s) => s.startsWith(INTERNAL_PREFIX)), [ctl("ready", 1), ctl("ready", 2)]);
  assert.equal(c.lanes().down, "down:poll");
  // A hello without lanes stops it too.
  b.recv({ tango: "hello", proto: 2, win: 1048576 });
  await tick();
  assert.equal(b.pktSubs.length, 0);
  assert.equal(c.lanes().down, "fallback");
});
