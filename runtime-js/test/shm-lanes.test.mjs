import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";

// The lane modules are internal. Bundle them with their normal registry,
// after installing the fake WebView2 API so it can receive early buffers.
const compiled = await build({
  stdin: {
    contents: 'export * from "./lanes/shm.ts"; export * from "./lanes/ring.ts"; export * from "./lanes.ts"; export * from "./packet.ts";',
    resolveDir: fileURLToPath(new URL("../src/", import.meta.url)),
    loader: "ts",
  },
  bundle: true,
  format: "esm",
  target: "es2020",
  write: false,
});
const source = Buffer.from(compiled.outputFiles[0].text).toString("base64");
let pageID = 0;
const capacity = 65536;
const header = 64;
const prefix = "\x00tango-int:lane:";

async function page(t, withWebView = true) {
  const listeners = new Map();
  const posts = [];
  const releases = [];
  const webview = {
    addEventListener(name, fn) {
      assert.equal(listeners.has(name), false, "one listener per event");
      listeners.set(name, fn);
    },
    postMessage(msg) {
      posts.push(msg);
    },
    releaseBuffer(buffer) {
      releases.push(buffer);
      structuredClone(buffer, { transfer: [buffer] });
    },
  };
  globalThis.window = withWebView ? { chrome: { webview } } : {};
  t.after(() => {
    delete globalThis.window;
    delete globalThis[Symbol.for("tango.shmLanes")];
    delete globalThis[Symbol.for("tango.ipc.lanes")];
  });
  const runtime = await import("data:text/javascript;base64," + source + "#" + ++pageID);
  return {
    ...runtime,
    posts,
    releases,
    receive(ring, gen, buffer) {
      listeners.get("sharedbufferreceived")({
        getBuffer: () => buffer,
        additionalData: { tango: "lane", ring, gen },
      });
    },
    wake(ring, gen) {
      // Go uses PostWebMessageAsString.
      listeners.get("message")({ data: JSON.stringify({ tango: "lane", op: "wake", ring, gen }) });
    },
  };
}

function mapping(gen) {
  const buffer = new ArrayBuffer(header + capacity);
  const view = new DataView(buffer);
  view.setBigUint64(16, BigInt(gen), true);
  view.setUint32(24, 0x31524754, true);
  view.setUint32(28, capacity, true);
  return buffer;
}

function ctxFor(gen) {
  const packets = [];
  const fails = [];
  const dispatched = [];
  return {
    gen,
    tok: "unused",
    packets,
    fails,
    onWire: dispatched,
    onPacket: (packet) => packets.push([...packet]),
    fail: (err) => fails.push(err),
    dispatched: (stream, seq) => dispatched.push([stream, seq]),
    fetch: () => assert.fail("shm lanes must not fetch"),
  };
}

test("ring golden", async (t) => {
  const { Ring } = await page(t);
  const gen = 0x0102030405060708n;
  const golden = readFileSync(new URL("../../internal/lane/shmring/testdata/golden.bin", import.meta.url));
  const last = [Uint8Array.of(0x10, 0x20, 0x30, 0x40, 0x50), Uint8Array.of(0xab)];
  const captured = new Ring(Uint8Array.from(golden).buffer, gen);
  const packets = [];
  captured.read((p) => packets.push(p.slice()));
  assert.deepEqual(packets, last);

  const mem = mapping(gen);
  const ring = new Ring(mem, gen);
  const block = (byte) => new Uint8Array(32760).fill(byte);
  for (const packet of [block(0xa1), block(0xb2), Uint8Array.of(0, 0x7f, 0xff), block(0xc3), block(0xd4)]) {
    assert.equal(ring.tryWrite(packet).ok, true);
    const drained = [];
    ring.read((p) => drained.push(p.slice()));
    assert.deepEqual(drained, [packet]);
  }
  for (const packet of last) assert.equal(ring.tryWrite(packet).ok, true);
  assert.deepEqual(Buffer.from(mem), golden);
});

test("buffers before hello", async (t) => {
  const web = await page(t);
  const gen = 0x1_0000_0002;
  const down = mapping(gen);
  const up = mapping(gen);
  web.receive("down", gen, down);
  web.receive("up", gen, up);
  const lanes = new web.Lanes({
    post: (msg) => web.posts.push(msg),
    onPacket: () => {},
    onDispatched: () => {},
    onUpStopped: () => {},
  });
  lanes.offer({ gen, tok: "unused", list: ["down:shm", "up:shm"] });
  await new Promise(setImmediate);
  assert.deepEqual(lanes.names(), { down: "down:shm", up: "up:shm" });
  assert.deepEqual(web.posts, ["down:shm", "up:shm"].map(
    (lane) => prefix + JSON.stringify({ op: "ready", lane, gen }),
  ));
  assert.deepEqual(web.releases, []);
  lanes.stopAll();
  assert.deepEqual(web.releases, [down, up]);
});

test("buffers of another gen are released", async (t) => {
  const web = await page(t);
  const stale = mapping(1);
  web.receive("up", 1, stale);
  const down = web.newShmDown();
  const start = down.start(ctxFor(2));
  assert.deepEqual(web.releases, [stale]);
  const wrong = mapping(3);
  web.receive("down", 3, wrong);
  assert.deepEqual(web.releases, [stale, wrong]);
  const current = mapping(2);
  web.receive("down", 2, current);
  await start;
  const next = web.newShmDown();
  const nextStart = next.start(ctxFor(3));
  assert.deepEqual(web.releases, [stale, wrong, current], "replacement releases the active mapping");
  const fresh = mapping(3);
  web.receive("down", 3, fresh);
  await nextStart;
  next.stop();
  const late = mapping(3);
  web.receive("down", 3, late);
  assert.deepEqual(web.releases, [stale, wrong, current, fresh, late]);
});

test("no buffers within 5 s fails the lane", async (t) => {
  const web = await page(t);
  mock.timers.enable({ apis: ["setTimeout"] });
  t.after(() => mock.timers.reset());
  const ctx = ctxFor(4);
  const lane = web.newShmUp();
  const rejected = assert.rejects(lane.start(ctx), /timeout/);
  mock.timers.tick(4999);
  assert.deepEqual(ctx.fails, []);
  mock.timers.tick(1);
  assert.equal(ctx.fails.length, 1);
  await rejected;
  const late = mapping(4);
  web.receive("up", 4, late);
  assert.deepEqual(web.releases, [late]);
});

test("up writes queue while full and dispatch on wake", async (t) => {
  const web = await page(t);
  const ctx = ctxFor(5);
  const up = web.newShmUp();
  const mem = mapping(ctx.gen);
  const view = new DataView(mem);
  view.setUint32(0, capacity, true); // full ring
  const start = up.start(ctx);
  web.receive("up", ctx.gen, mem);
  await start;
  assert.equal(up.minSize, 0);
  assert.equal(up.send({ stream: 7, seq: 0, payload: new Uint8Array(capacity / 2 - 21) }), false);
  assert.equal(up.send({ stream: 7, seq: 0, payload: Uint8Array.of(1) }), true);
  assert.equal(up.send({ stream: 7, seq: 1, payload: Uint8Array.of(2) }), true);
  assert.equal(view.getUint32(12, true), 1);
  assert.deepEqual(ctx.onWire, []);
  assert.deepEqual(web.posts, []);
  view.setUint32(4, capacity, true); // Go drained and slept
  view.setUint32(8, 1, true);
  web.wake("up", ctx.gen - 1);
  assert.deepEqual(ctx.onWire, []);
  web.wake("up", ctx.gen);
  assert.deepEqual(ctx.onWire, [[7, 0], [7, 1]]);
  assert.equal(view.getUint32(12, true), 0);
  assert.ok(web.posts.length > 0);
  assert.ok(web.posts.every((msg) => msg === prefix + '{"op":"wake","ring":"up","gen":5}'));
  const received = [];
  new web.Ring(mem, BigInt(ctx.gen)).read((p) => received.push(...web.decodePacket(p)));
  assert.deepEqual(received, [0, 1].map((seq) => ({ stream: 7, seq, payload: Uint8Array.of(seq + 1) })));
  up.stop();
});

test("up wakes Go for a lone wrap marker before dispatch", async (t) => {
  const web = await page(t);
  const ctx = ctxFor(6);
  const up = web.newShmUp();
  const mem = mapping(ctx.gen);
  const view = new DataView(mem);
  view.setUint32(0, capacity / 2, true);
  view.setUint32(4, capacity / 2, true);
  view.setUint32(8, 1, true);
  new Uint8Array(mem).fill(0xa5, header + capacity / 2);
  web.receive("up", ctx.gen, mem);
  await up.start(ctx);
  assert.equal(up.send({ stream: 9, seq: 0, payload: new Uint8Array(capacity / 2 - 22) }), true);
  assert.equal(view.getUint32(0, true), capacity);
  assert.equal(view.getUint32(header + capacity / 2, true), 0xffffffff);
  assert.ok(new Uint8Array(mem, header + capacity / 2 + 4).every((b) => b === 0xa5));
  assert.deepEqual(ctx.onWire, []);
  assert.deepEqual(web.posts, [prefix + '{"op":"wake","ring":"up","gen":6}']);
  view.setUint32(4, capacity, true);
  web.wake("up", ctx.gen);
  assert.deepEqual(ctx.onWire, [[9, 0]]);
  up.stop();
});

test("down wake drains and sleeps", async (t) => {
  const web = await page(t);
  const ctx = ctxFor(7);
  const down = web.newShmDown();
  const mem = mapping(ctx.gen);
  const view = new DataView(mem);
  web.receive("down", ctx.gen, mem);
  await down.start(ctx);
  assert.equal(view.getUint32(8, true), 1, "armed before ready");
  // Two published records, as written by Go.
  new Uint8Array(mem, header, 16).set([1, 0, 0, 0, 11, 0, 0, 0, 2, 0, 0, 0, 22, 23, 0, 0]);
  view.setUint32(0, 16, true);
  view.setUint32(12, 1, true);
  web.wake("down", ctx.gen - 1);
  assert.deepEqual(ctx.packets, []);
  web.wake("down", ctx.gen);
  assert.deepEqual(ctx.packets, [[11], [22, 23]]);
  assert.equal(view.getUint32(4, true), 16);
  assert.equal(view.getUint32(8, true), 1);
  assert.deepEqual(web.posts, [prefix + '{"op":"wake","ring":"down","gen":7}']);
  down.stop();
});

test("down fails on corrupt ring headers, cursors, lengths and wrap markers", async (t) => {
  const cases = [
    [24, 0, 0, /header/],
    [32, 1, 0, /header/],
    [8, 2, 0, /header/],
    [12, 2, 0, /header/],
    [0, 5, 0, /cursors/],
    [0, capacity + 4, 0, /cursors/],
    [header, capacity / 2 + 1, 4, /length/],
    [header, 3, 4, /length/],
    [header, 0xffffffff, 4, /wrap/],
  ];
  for (const [offset, value, write, err] of cases) {
    await t.test("offset " + offset + " value " + value, async (t) => {
      const web = await page(t);
      const ctx = ctxFor(8);
      const down = web.newShmDown();
      const mem = mapping(ctx.gen);
      const view = new DataView(mem);
      web.receive("down", ctx.gen, mem);
      await down.start(ctx);
      view.setUint32(0, write, true);
      view.setUint32(offset, value, true);
      web.wake("down", ctx.gen);
      assert.deepEqual(ctx.packets, []);
      assert.equal(ctx.fails.length, 1);
      assert.match(String(ctx.fails[0]), err);
      assert.deepEqual(web.releases, [mem]);
    });
  }
});

test("start fails immediately without chrome.webview", async (t) => {
  const web = await page(t, false);
  for (const make of [web.newShmDown, web.newShmUp]) {
    const ctx = ctxFor(9);
    const start = make().start(ctx);
    assert.equal(ctx.fails.length, 1);
    await assert.rejects(start, /webview/);
  }
});
