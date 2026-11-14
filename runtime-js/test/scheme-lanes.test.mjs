import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import vm from "node:vm";
import { execFileSync } from "node:child_process";
import { build } from "esbuild";
import { createClient, GAP_TIMEOUT_MS } from "../dist/index.js";

// The lane modules and the packet codec are internal (not on the package
// entry): bundle them from src for the test.
const internal = await (async () => {
  const src = fileURLToPath(new URL("../src/", import.meta.url));
  const out = await build({
    stdin: {
      contents: 'export * from "./lanes/scheme.ts"; export * from "./packet.ts"; export * from "./lanes.ts";',
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
const {
  newSchemeDown,
  newPostUp,
  DOWN_START_TIMEOUT_MS,
  POST_MIN_SIZE,
  POST_BATCH_BYTES,
  POST_BATCH_DELAY_MS,
  MAX_POST_PACKET,
  POST_TIMEOUT_MS,
  encodePacket,
  decodePacket,
  INTERNAL_PREFIX,
} = internal;

const tick = () => new Promise((r) => setImmediate(r));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const bytes = (n, seed = 1) => new Uint8Array(n).map((_, i) => (i * seed + 3) & 255);

// A controllable fetch for down:scheme: each call records its url/init and
// returns a response whose body the test feeds (push/end/error). An abort
// of init.signal errors the body (or rejects the fetch), as browsers do.
function downFetch({ status = 200, noBody = false, hang = false } = {}) {
  const f = {
    calls: [],
    body: null, // ReadableStreamDefaultController of the last response
    fn(url, init) {
      const call = { url, init };
      f.calls.push(call);
      return new Promise((resolve, reject) => {
        const abortErr = () => new DOMException("aborted", "AbortError");
        if (hang) {
          init?.signal?.addEventListener("abort", () => reject(abortErr()));
          return;
        }
        let ctrl;
        const body = new ReadableStream({
          start(c) {
            ctrl = c;
          },
        });
        f.body = {
          push: (arr) => ctrl.enqueue(arr instanceof Uint8Array ? arr : new Uint8Array(arr)),
          end: () => ctrl.close(),
          error: (e) => ctrl.error(e),
        };
        init?.signal?.addEventListener("abort", () => {
          call.aborted = true;
          try {
            ctrl.error(abortErr());
          } catch {}
        });
        resolve({ status, ok: status >= 200 && status < 300, body: noBody ? null : body });
      });
    },
  };
  return f;
}

function fakeCtx(fetchFn) {
  const ctx = {
    gen: 7,
    tok: "a+b/c d",
    packets: [],
    fails: [],
    onWire: [],
    onPacket(p) {
      ctx.packets.push(Uint8Array.from(p)); // the module may reuse p
    },
    fail(e) {
      ctx.fails.push(e);
    },
    dispatched(stream, seq) {
      ctx.onWire.push([stream, seq]);
    },
    fetch: fetchFn,
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

// ---- down:scheme ----

test("constants", () => {
  assert.equal(DOWN_START_TIMEOUT_MS, 5000);
  assert.equal(POST_MIN_SIZE, 16 * 1024);
  assert.equal(POST_BATCH_BYTES, 16 * 1024);
  assert.equal(POST_BATCH_DELAY_MS, 4);
  assert.equal(MAX_POST_PACKET, 1 << 20);
  // A hung POST must fail the lane (and fall back) before a stream's gap
  // timeout fails its streams.
  assert.equal(POST_TIMEOUT_MS, 4000);
  assert.ok(POST_TIMEOUT_MS < GAP_TIMEOUT_MS);
});

test("down:scheme requests the relative down url without caching", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const st = settled(lane.start(ctx));
  await tick();
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].url, "/__tango/lane/down?gen=7&tok=" + encodeURIComponent("a+b/c d"));
  assert.equal(f.calls[0].init.cache, "no-store");
  assert.ok(f.calls[0].init.signal);
  f.body.push([0, 0]);
  await tick();
  assert.equal(st.ok, true);
  lane.stop();
});

test("down:scheme parses packets split across chunks", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const st = settled(lane.start(ctx));
  await tick();
  const p1 = encodePacket([{ stream: 2, seq: 0, payload: bytes(9000) }]);
  const p2 = encodePacket([
    { stream: 2, seq: 1, payload: bytes(5, 3) },
    { stream: 4, seq: 0, payload: new Uint8Array(0) },
  ]);
  f.body.push([0, 0]);
  await tick();
  assert.equal(st.ok, true, "start resolves on the first (empty) packet");
  assert.equal(ctx.packets.length, 0, "the empty packet is not handed on");
  f.body.push(p1.subarray(0, 4000));
  await tick();
  assert.equal(ctx.packets.length, 0);
  const rest = new Uint8Array(p1.length - 4000 + p2.length);
  rest.set(p1.subarray(4000));
  rest.set(p2, p1.length - 4000);
  f.body.push(rest);
  await tick();
  assert.equal(ctx.packets.length, 2);
  assert.deepEqual(ctx.packets[0], p1);
  assert.deepEqual(ctx.packets[1], p2);
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

test("down:scheme parses a stream fed one byte at a time", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const st = settled(lane.start(ctx));
  await tick();
  const pkts = [
    encodePacket([]),
    encodePacket([{ stream: 1, seq: 0, payload: bytes(30) }]),
    encodePacket([]), // an empty packet later in the stream is skipped too
    encodePacket([
      { stream: 1, seq: 1, payload: bytes(3, 5) },
      { stream: 3, seq: 9, payload: bytes(25, 7) },
      { stream: 5, seq: 2, payload: new Uint8Array(0) },
    ]),
  ];
  for (const p of pkts) for (const b of p) f.body.push([b]);
  // One read per tick at most; give the reader time.
  for (let i = 0; i < 400 && ctx.packets.length < 2; i++) await tick();
  assert.equal(st.ok, true);
  assert.equal(ctx.packets.length, 2);
  assert.deepEqual(ctx.packets[0], pkts[1]);
  assert.deepEqual(ctx.packets[1], pkts[3]);
  assert.equal(decodePacket(ctx.packets[1]).length, 3);
  lane.stop();
});

test("down:scheme hands on packets that arrive with the first one", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const st = settled(lane.start(ctx));
  await tick();
  const p1 = encodePacket([{ stream: 2, seq: 0, payload: bytes(10) }]);
  const all = new Uint8Array(2 + p1.length);
  all.set(p1, 2);
  f.body.push(all);
  await tick();
  assert.equal(st.ok, true);
  assert.deepEqual(ctx.packets, [p1]);
  lane.stop();
});

test("down:scheme start fails on non-200", async () => {
  for (const status of [403, 500, 204]) {
    const f = downFetch({ status });
    const ctx = fakeCtx(f.fn);
    const lane = newSchemeDown();
    await assert.rejects(lane.start(ctx));
    assert.deepEqual(ctx.fails, []);
    lane.stop();
  }
});

test("down:scheme start fails without a body or when fetch rejects", async () => {
  const f = downFetch({ noBody: true });
  await assert.rejects(newSchemeDown().start(fakeCtx(f.fn)));
  const ctx = fakeCtx(() => Promise.reject(new TypeError("network")));
  await assert.rejects(newSchemeDown().start(ctx), /network/);
  const ctx2 = fakeCtx(() => {
    throw new TypeError("sync throw");
  });
  await assert.rejects(newSchemeDown().start(ctx2), /sync throw/);
});

test("down:scheme stream end triggers fail", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  f.body.push([0, 0]);
  await p;
  f.body.end();
  await tick();
  await tick();
  assert.equal(ctx.fails.length, 1);
  lane.stop();
});

test("down:scheme read error triggers fail", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  f.body.push([0, 0]);
  await p;
  f.body.error(new TypeError("connection reset"));
  await tick();
  await tick();
  assert.equal(ctx.fails.length, 1);
  lane.stop();
});

test("down:scheme stream end before the first packet fails start", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  f.body.push([0]); // half a packet
  f.body.end();
  await assert.rejects(p);
  assert.deepEqual(ctx.fails, [], "a start failure is the rejection, not fail");
  lane.stop();
});

test("down:scheme frame length beyond MAX_FRAME_BYTES fails the lane", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  f.body.push([0, 0]);
  await p;
  const bad = new Uint8Array(22);
  const dv = new DataView(bad.buffer);
  dv.setUint16(0, 1);
  dv.setUint32(18, (16 << 20) + 1);
  f.body.push(bad);
  await tick();
  await tick();
  assert.equal(ctx.fails.length, 1);
  assert.equal(f.calls[0].aborted, true, "the fetch is aborted");
  lane.stop();
});

test("down:scheme stop aborts the fetch and never calls fail", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  f.body.push([0, 0]);
  await p;
  lane.stop();
  await tick();
  await tick();
  assert.equal(f.calls[0].aborted, true);
  assert.deepEqual(ctx.fails, []);
  lane.stop(); // twice is harmless
});

test("down:scheme stop before the first packet settles start", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown();
  const p = lane.start(ctx);
  await tick();
  lane.stop();
  await assert.rejects(p);
  assert.deepEqual(ctx.fails, []);
});

test("down:scheme start times out without a first packet", async () => {
  // Headers arrive but no bytes.
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown({ startTimeoutMs: 30 });
  const t0 = Date.now();
  await assert.rejects(lane.start(ctx), /5 s|timed out|within/);
  assert.ok(Date.now() - t0 >= 25);
  assert.equal(f.calls[0].aborted, true, "the fetch is aborted");
  assert.deepEqual(ctx.fails, []);
  lane.stop();

  // No response at all.
  const g = downFetch({ hang: true });
  const lane2 = newSchemeDown({ startTimeoutMs: 30 });
  await assert.rejects(lane2.start(fakeCtx(g.fn)));
  assert.equal(g.calls[0].init.signal.aborted, true);
  lane2.stop();
});

test("down:scheme start timer stops once started", async () => {
  const f = downFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newSchemeDown({ startTimeoutMs: 30 });
  const p = lane.start(ctx);
  await tick();
  f.body.push([0, 0]);
  await p;
  await sleep(60);
  assert.notEqual(f.calls[0].aborted, true);
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

// ---- up:post ----

// A fetch for up:post: records every POST; each returns a promise the
// test settles with respond(status) or reject(err).
function upFetch() {
  const f = {
    calls: [],
    fn(url, init) {
      return new Promise((resolve, reject) => {
        const call = {
          url,
          init,
          respond: (status = 204) => resolve({ status, ok: status >= 200 && status < 300 }),
          reject,
        };
        init?.signal?.addEventListener("abort", () => {
          call.aborted = true;
          reject(new DOMException("aborted", "AbortError"));
        });
        f.calls.push(call);
      });
    },
    frames(i) {
      return decodePacket(new Uint8Array(f.calls[i].init.body));
    },
  };
  return f;
}

const frame = (stream, seq, n, seed = 1) => ({ stream, seq, payload: bytes(n, seed) });

// Starts an up:post lane through its start probe (answered 204) and drops
// the probe from f.calls, so that tests index data POSTs only.
async function startUp(lane, ctx, f) {
  const started = lane.start(ctx);
  await tick();
  assert.equal(f.calls.length, 1, "start posts the probe");
  f.calls.shift().respond(204);
  await started;
}

test("up:post start posts one empty packet, ready only on 204; minSize is 16 KiB", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  assert.equal(lane.minSize, 16 * 1024);
  assert.equal(typeof lane.send, "function");
  let ready = false;
  const started = lane.start(ctx).then(() => (ready = true));
  await tick();
  assert.equal(f.calls.length, 1, "exactly one probe");
  const c = f.calls[0];
  assert.equal(c.url, "/__tango/lane/up?gen=7&tok=" + encodeURIComponent("a+b/c d"));
  assert.equal(c.init.method, "POST");
  assert.equal(c.init.cache, "no-store");
  assert.ok(c.init.body instanceof ArrayBuffer, "body is an ArrayBuffer");
  assert.deepEqual([...new Uint8Array(c.init.body)], [0, 0], "count 0, no frames");
  assert.deepEqual(decodePacket(new Uint8Array(c.init.body)), []);
  // Before the probe's 204 the lane takes no frames.
  assert.equal(ready, false);
  assert.equal(lane.send(frame(1, 0, 17000)), false, "send before start resolves");
  assert.equal(f.calls.length, 1);
  c.respond(204);
  await started;
  assert.equal(ready, true);
  assert.equal(lane.send(frame(1, 0, 17000)), true);
  assert.equal(f.calls.length, 2, "data goes after the probe");
  assert.deepEqual(ctx.fails, []);
  assert.deepEqual(ctx.onWire, [[1, 0]], "the probe announces no frames");
  lane.stop();
});

test("up:post start probe failure rejects start and the lane takes no frames", async () => {
  const cases = [
    ["fetch rejects (CSP)", (c) => c.reject(new TypeError("Refused to connect: Content Security Policy"))],
    ["403", (c) => c.respond(403)],
    ["503", (c) => c.respond(503)],
    ["200", (c) => c.respond(200)],
  ];
  for (const [name, settle] of cases) {
    const f = upFetch();
    const ctx = fakeCtx(f.fn);
    const lane = newPostUp();
    const started = lane.start(ctx);
    await tick();
    assert.equal(f.calls.length, 1, name);
    settle(f.calls[0]);
    await assert.rejects(started, name);
    assert.equal(lane.send(frame(1, 0, 17000)), false, `${name}: no frames after a failed start`);
    await sleep(10);
    assert.equal(f.calls.length, 1, `${name}: nothing else posted`);
    assert.deepEqual(ctx.fails, [], `${name}: start rejects instead of fail`);
    assert.deepEqual(ctx.onWire, []);
    lane.stop();
  }

  // A fetch that throws synchronously rejects start too.
  const ctx2 = fakeCtx(() => {
    throw new TypeError("sync");
  });
  const lane2 = newPostUp();
  await assert.rejects(lane2.start(ctx2));
  assert.equal(lane2.send(frame(1, 0, 17000)), false);
  assert.deepEqual(ctx2.fails, []);
});

test("up:post start probe timeout rejects start and aborts the probe", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp({ postTimeoutMs: 15 });
  const started = lane.start(ctx);
  await assert.rejects(started, /timeout/);
  assert.equal(f.calls[0].aborted, true);
  assert.equal(lane.send(frame(1, 0, 17000)), false);
  f.calls[0].respond(204); // a late answer changes nothing
  await tick();
  assert.equal(lane.send(frame(1, 0, 17000)), false);
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

test("up:post stop during the start probe rejects start", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  const started = lane.start(ctx);
  await tick();
  lane.stop();
  await assert.rejects(started);
  assert.equal(f.calls[0].aborted, true);
  assert.equal(lane.send(frame(1, 0, 17000)), false);
  assert.deepEqual(ctx.fails, []);
});

test("up:post batches to 16KiB or 4ms", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  assert.equal(lane.send(frame(1, 0, 6144, 1)), true);
  assert.equal(lane.send(frame(1, 1, 6144, 3)), true);
  assert.equal(f.calls.length, 0);
  assert.equal(lane.send(frame(3, 0, 6144, 5)), true);
  assert.equal(f.calls.length, 1, "the batch reached 16 KiB");
  const c = f.calls[0];
  assert.equal(c.url, "/__tango/lane/up?gen=7&tok=" + encodeURIComponent("a+b/c d"));
  assert.equal(c.init.method, "POST");
  const got = f.frames(0);
  assert.deepEqual(
    got.map((g) => [g.stream, g.seq, g.payload.length]),
    [
      [1, 0, 6144],
      [1, 1, 6144],
      [3, 0, 6144],
    ],
  );
  assert.deepEqual(got[2].payload, bytes(6144, 5));
  c.respond(204);
  await tick();

  assert.equal(lane.send(frame(1, 2, 20)), true);
  assert.equal(f.calls.length, 1);
  await sleep(30);
  assert.equal(f.calls.length, 2, "sent after the batch delay");
  assert.deepEqual(
    f.frames(1).map((g) => [g.stream, g.seq, g.payload.length]),
    [[1, 2, 20]],
  );
  f.calls[1].respond(204);
  await tick();
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

test("up:post batch delay is counted from the first frame", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp({ batchDelayMs: 40 });
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 10));
  await sleep(25);
  lane.send(frame(1, 1, 10));
  await sleep(25);
  assert.equal(f.calls.length, 1, "not restarted by the second frame");
  assert.equal(f.frames(0).length, 2);
  lane.stop();
});

test("up:post one request in flight", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 20000));
  assert.equal(f.calls.length, 1);
  lane.send(frame(1, 1, 20000));
  lane.send(frame(1, 2, 100));
  await sleep(20);
  assert.equal(f.calls.length, 1, "nothing else while a POST is in flight");
  f.calls[0].respond(204);
  await tick();
  assert.equal(f.calls.length, 2, "the next batch goes at once");
  assert.deepEqual(
    f.frames(1).map((g) => [g.seq, g.payload.length]),
    [
      [1, 20000],
      [2, 100],
    ],
  );
  f.calls[1].respond(204);
  await tick();
  await sleep(20);
  assert.equal(f.calls.length, 2);
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

test("up:post uses ArrayBuffer body and fails on non-204", async () => {
  for (const status of [500, 403, 413, 503, 200]) {
    const f = upFetch();
    const ctx = fakeCtx(f.fn);
    const lane = newPostUp();
    await startUp(lane, ctx, f);
    lane.send(frame(1, 0, 17000));
    const body = f.calls[0].init.body;
    assert.ok(body instanceof ArrayBuffer, "body is an ArrayBuffer");
    assert.equal(body.byteLength, 2 + 20 + 17000, "exactly the packet bytes");
    lane.send(frame(1, 1, 17000)); // queued behind the POST
    f.calls[0].respond(status);
    await tick();
    assert.equal(ctx.fails.length, 1, `status ${status} fails the lane`);
    assert.equal(f.calls.length, 1, "no further batch");
    assert.equal(lane.send(frame(1, 2, 17000)), false, "a failed lane takes no frames");
    await sleep(20);
    assert.equal(f.calls.length, 1);
    lane.stop();
  }
});

test("up:post network error fails the lane", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 17000));
  f.calls[0].reject(new TypeError("network"));
  await tick();
  assert.equal(ctx.fails.length, 1);
  assert.equal(lane.send(frame(1, 1, 17000)), false);
  lane.stop();

  // A data fetch that throws synchronously, too.
  let calls = 0;
  const ctx2 = fakeCtx(() => {
    if (calls++ === 0) return Promise.resolve({ status: 204, ok: true }); // the start probe
    throw new TypeError("sync");
  });
  const lane2 = newPostUp();
  await lane2.start(ctx2);
  lane2.send(frame(1, 0, 17000));
  await tick();
  assert.equal(ctx2.fails.length, 1);
  lane2.stop();
});

test("up:post rejects frames that cannot fit 1MiB", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  assert.equal(lane.send(frame(1, 0, 2 << 20)), false);
  assert.equal(lane.send(frame(1, 0, (1 << 20) - 21)), false, "22 bytes of headers must fit too");
  assert.equal(f.calls.length, 0);
  assert.equal(lane.send(frame(1, 0, (1 << 20) - 22)), true);
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].init.body.byteLength, 1 << 20);
  lane.stop();
});

test("up:post splits batches at 1MiB", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 600 << 10)); // goes at once (>= 16 KiB)
  lane.send(frame(1, 1, 600 << 10));
  lane.send(frame(1, 2, 600 << 10)); // does not fit next to seq 1
  lane.send(frame(1, 3, 100));
  assert.equal(f.calls.length, 1);
  f.calls[0].respond(204);
  await tick();
  assert.equal(f.calls.length, 2);
  assert.deepEqual(
    f.frames(1).map((g) => g.seq),
    [1],
  );
  f.calls[1].respond(204);
  await tick();
  assert.equal(f.calls.length, 3);
  assert.deepEqual(
    f.frames(2).map((g) => g.seq),
    [2, 3],
  );
  for (let i = 0; i < 3; i++) assert.ok(f.calls[i].init.body.byteLength <= 1 << 20);
  f.calls[2].respond(204);
  await tick();
  assert.deepEqual(ctx.fails, []);
  lane.stop();
});

test("up:post stop drops queued frames, aborts the POST and never calls fail", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 17000));
  lane.send(frame(1, 1, 10));
  lane.stop();
  assert.equal(lane.send(frame(1, 2, 17000)), false);
  await sleep(20);
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].aborted, true);
  assert.deepEqual(ctx.fails, []);

  // A pending batch delay does not fire after stop.
  const g = upFetch();
  const lane2 = newPostUp();
  await startUp(lane2, fakeCtx(g.fn), g);
  lane2.send(frame(1, 0, 10));
  lane2.stop();
  await sleep(20);
  assert.equal(g.calls.length, 0);
});

// ---- registration ----

const REG = Symbol.for("tango.ipc.lanes");

test("the clean ESM entry registers both lanes before synchronous eager hello", () => {
  const entry = new URL("../dist/index.js", import.meta.url).href;
  const script = `
    import assert from "node:assert/strict";
    let recv;
    globalThis.fetch = async (_url, init) => init?.method === "POST" ? {status:204} : ({status:200,body:new ReadableStream({start(c){c.enqueue(new Uint8Array([0,0]));}})});
    globalThis.__tango = {
      onMessage(fn){recv=fn;return ()=>{};},
      post(raw){if(raw[0]!=="{")return;const m=JSON.parse(raw);if(m.tango==="hello")recv(JSON.stringify({tango:"hello",proto:2,win:1048576,lanes:{gen:1,tok:"t",list:["down:scheme","up:post"]}}));}
    };
    const api=await import(${JSON.stringify(entry)});
    await new Promise(r=>setImmediate(r));
    assert.deepEqual(api.lanes(),{down:"down:scheme",up:"up:post"});
  `;
  execFileSync(process.execPath, ["--input-type=module", "-e", script], { timeout: 10000 });
});

function iifeBridge() {
  let sub = null;
  const b = {
    out: [],
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
  };
  return b;
}

// The IIFE (what Go injects) must have registered both lanes by the time
// its eager client sees Go's hello reply.
test("IIFE registers the scheme lanes before the client starts", async () => {
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const b = iifeBridge();
  const f = downFetch();
  const ups = [];
  const fetchFn = (url, init) => {
    if (url.startsWith("/__tango/lane/up")) {
      ups.push({ url, init });
      return Promise.resolve({ status: 204, ok: true });
    }
    return f.fn(url, init);
  };
  const ctx = vm.createContext({
    __tango: b,
    setTimeout,
    clearTimeout,
    fetch: fetchFn,
    AbortController,
    console,
  });
  vm.runInContext(src, ctx);
  assert.equal(ctx[REG]?.has("down:scheme"), true);
  assert.equal(ctx[REG]?.has("up:post"), true);
  assert.deepEqual(b.out.splice(0), ['{"tango":"hello","proto":2}']);
  b.recv({ tango: "hello", proto: 2, win: 1048576, lanes: { gen: 4, tok: "k", list: ["down:scheme", "up:post"] } });
  await tick();
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].url, "/__tango/lane/down?gen=4&tok=k");
  const ctl = (op, lane) => INTERNAL_PREFIX + "lane:" + JSON.stringify({ op, lane, gen: 4 });
  assert.deepEqual(b.out.splice(0), [ctl("ready", "up:post")]);
  f.body.push([0, 0]);
  await tick();
  assert.deepEqual(b.out.splice(0), [ctl("ready", "down:scheme")]);
  assert.equal(JSON.stringify(ctx.tango.lanes()), JSON.stringify({ down: "down:scheme", up: "up:post" })); // other realm
});

// Ordering in the IIFE: even a hello reply delivered synchronously from
// the eager client's own hello post (the earliest possible moment) finds
// both lanes registered.
test("IIFE registers the scheme lanes before its eager client posts hello", async () => {
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const f = downFetch();
  let sub = null;
  const out = [];
  const b = {
    post(s) {
      out.push(s);
      if (s.includes('"hello"')) {
        sub(JSON.stringify({ tango: "hello", proto: 2, lanes: { gen: 1, tok: "k", list: ["up:post", "down:scheme"] } }));
      }
    },
    onMessage(fn) {
      sub = fn;
      return () => {};
    },
  };
  // up:post's start probe gets its 204; down:scheme gets f.
  const fetchFn = (url, init) => (url.startsWith("/__tango/lane/up") ? Promise.resolve({ status: 204, ok: true }) : f.fn(url, init));
  const ctx = vm.createContext({ __tango: b, setTimeout, clearTimeout, fetch: fetchFn, AbortController, console });
  vm.runInContext(src, ctx);
  await tick();
  assert.equal(f.calls.length, 1, "down:scheme was started");
  assert.ok(out.some((s) => s.includes('"op":"ready","lane":"up:post"')), "up:post was started");
});

// End to end through the client: a Go-opened stream gets its bytes from
// the down:scheme response and sends big frames with up:post.
test("client streams through down:scheme and up:post", async (t) => {
  const f = downFetch();
  const posts = [];
  let probes = 0;
  const prev = globalThis.fetch;
  globalThis.fetch = (url, init) => {
    if (url.startsWith("/__tango/lane/up")) {
      const frames = decodePacket(new Uint8Array(init.body));
      if (frames.length) posts.push(frames);
      else probes++; // up:post's start probe
      return Promise.resolve({ status: 204, ok: true });
    }
    return f.fn(url, init);
  };
  t.after(() => (globalThis.fetch = prev));
  let sub;
  const out = [];
  const b = { post: (s) => out.push(s), onMessage: (fn) => ((sub = fn), () => {}) };
  const c = createClient(b);
  sub(JSON.stringify({ tango: "hello", proto: 2, win: 1048576, lanes: { gen: 1, tok: "t", list: ["down:scheme", "up:post"] } }));
  await tick();
  f.body.push([0, 0]);
  await tick();
  assert.deepEqual(c.lanes(), { down: "down:scheme", up: "up:post" });
  assert.equal(probes, 1, "up:post was ready after one start probe");
  let s;
  c.onStream("Feed", (x) => (s = x));
  sub(JSON.stringify({ tango: "stream", id: 2, op: "open", method: "Feed", args: [] }));
  const payload = bytes(8000, 9);
  const pkt = encodePacket([{ stream: 2, seq: 0, payload }]);
  f.body.push(pkt.subarray(0, 100));
  f.body.push(pkt.subarray(100));
  const it = s[Symbol.asyncIterator]();
  const { value } = await it.next();
  assert.deepEqual(new Uint8Array(value), payload);

  await s.sendBytes(bytes(20000, 11));
  await tick();
  assert.equal(posts.length, 1);
  assert.deepEqual(
    posts[0].map((g) => [g.stream, g.seq, g.payload.length]),
    [[2, 0, 20000]],
  );
  c.lanes(); // still both
  assert.deepEqual(c.lanes(), { down: "down:scheme", up: "up:post" });
});


test("up:post timeout aborts once and ignores late success", async () => {
  let settle, signal;
  let probed = false;
  const ctx = fakeCtx((_url, init) => {
    if (!probed) { probed = true; return Promise.resolve({ status: 204 }); } // the start probe
    signal = init.signal; return new Promise(r => { settle = r; });
  });
  const up = newPostUp({ postTimeoutMs: 15 });
  await up.start(ctx);
  assert.equal(up.send({ stream: 1, seq: 0, payload: bytes(POST_MIN_SIZE) }), true);
  await sleep(40);
  assert.equal(signal.aborted, true);
  assert.equal(ctx.fails.length, 1);
  settle({ status: 204 });
  await tick();
  assert.equal(ctx.fails.length, 1);
  assert.equal(up.send({ stream: 3, seq: 0, payload: bytes(POST_MIN_SIZE) }), false);
});

test("up:post aggregate bound includes flight and recovers after settlement", async () => {
  const pending = [];
  const ctx = fakeCtx(() => new Promise(r => pending.push(r)));
  const up = newPostUp();
  const started = up.start(ctx);
  pending.shift()({ status: 204 }); // the start probe
  await started;
  const payload = bytes(POST_MIN_SIZE);
  let accepted = 0;
  while (up.send({ stream: accepted * 2 + 1, seq: 0, payload })) {
    if (++accepted > 1000) break;
  }
  assert.ok(accepted < 512, `accepted ${accepted}`);
  assert.equal(pending.length, 1);
  pending.shift()({ status: 204 });
  await tick();
  assert.equal(up.send({ stream: 2001, seq: 0, payload }), true);
  up.stop();
  pending.shift()?.({ status: 204 });
  await tick();
  assert.equal(ctx.fails.length, 0);
});

test("default adapter ignores external HTML base, pins normal and opaque locations, refuses redirects", async () => {
  const calls = [];
  globalThis.document = { baseURI: "https://external.example/" };
  try {
    for (const href of ["https://tango.localhost/page", "tango://app/page"]) {
      const adapter = internal.locationFetch(href, async (url, init) => {
        calls.push({ url, init }); return { status: 204, redirected: false };
      });
      await adapter("/__tango/lane/up?gen=1&tok=secret", { method: "POST", body: new ArrayBuffer(1) });
      assert.equal(calls.at(-1).url, new URL("/__tango/lane/up?gen=1&tok=secret", href).href);
      assert.equal(calls.at(-1).init.redirect, "error");
      await assert.rejects(adapter("https://external.example/__tango/lane/up?gen=1"));
    }
    const redirect = internal.locationFetch("https://tango.localhost/", async () => ({ redirected: true }));
    await assert.rejects(redirect("/__tango/lane/down?gen=1&tok=secret"), /redirect/);
  } finally { delete globalThis.document; }
});


test("POST success and stop clear deadline; late rejection after stop is ignored", async () => {
  for (const action of ["success", "stop"]) {
    let resolve, reject, probed = false;
    const ctx = fakeCtx(() => {
      if (!probed) { probed = true; return Promise.resolve({ status: 204 }); } // the start probe
      return new Promise((yes, no) => { resolve = yes; reject = no; });
    });
    const up = newPostUp({ postTimeoutMs: 15 });
    await up.start(ctx);
    up.send({ stream: 1, seq: 0, payload: bytes(POST_MIN_SIZE) });
    if (action === "success") resolve({ status: 204 });
    else { up.stop(); reject(new Error("late abort")); }
    await sleep(40);
    assert.equal(ctx.fails.length, 0);
    up.stop();
  }
});

test("POST saturation falls back with original sequence and keeps retained packets ordered", async () => {
  const pending = [], posted = [];
  const up = newPostUp();
  internal.registerLane("test:boundedpost", () => up);
  const b = iifeBridge();
  const c = createClient(b);
  // The production adapter intentionally retains injectable global fetch in Node.
  const saved = globalThis.fetch;
  globalThis.fetch = (_url, init) => {
    const frames = decodePacket(new Uint8Array(init.body));
    if (!frames.length) return Promise.resolve({ status: 204 }); // the start probe
    posted.push(...frames.map(f => [f.stream, f.seq]));
    return new Promise(resolve => pending.push(resolve));
  };
  b.recv({ tango: "hello", proto: 2, win: 1048576, lanes: { gen: 1, tok: "t", list: ["test:boundedpost"] } });
  const streams = [];
  try {
    await tick();
    let fallback;
    for (let i = 0; i < 600; i++) {
      const stream = c.stream("Fill"); streams.push(stream);
      const id = i * 2 + 1;
      b.recv({ tango: "stream", id, op: "accept", in: true });
      await stream.sendBytes(bytes(POST_MIN_SIZE));
      fallback = b.out.map(raw => {try{return JSON.parse(raw);}catch{return {};}}).find(m => m.id === id && m.op === "data");
      if (fallback) break;
    }
    assert.ok(fallback, "aggregate cap must force fallback");
    assert.equal(fallback.seq, 0);
    assert.equal(fallback.c, POST_MIN_SIZE + 64);
    const accepted = streams.length - 1;
    while (pending.length) { pending.shift()({ status: 204 }); await tick(); }
    assert.deepEqual(posted, Array.from({ length: accepted }, (_,i) => [i*2+1, 0]));
    // Capacity recovered: next frame of the stream whose first went by
    // message takes the lane with seq=1, preserving mixed-path ordering.
    await streams.at(-1).sendBytes(bytes(POST_MIN_SIZE));
    assert.deepEqual(posted.at(-1), [fallback.id, 1]);
  } finally {
    up.stop(); for (const s of streams) s.cancel(); globalThis.fetch = saved;
    while (pending.length) pending.shift()({ status: 204 });
  }
});

test("lane generation captures actual location rather than external base or later location", async () => {
  const saved = globalThis.fetch;
  const calls = [];
  let ctx;
  internal.registerLane("test:pinnedfetch", () => ({ minSize: 0, start(c) { ctx = c; return Promise.resolve(); }, send() { return false; }, stop() {} }));
  globalThis.location = { href: "https://tango.localhost/original" };
  globalThis.document = { baseURI: "https://external.example/" };
  globalThis.fetch = async (url, init) => { calls.push({ url, init }); return { status: 204 }; };
  const lanes = new internal.Lanes({ post() {}, onPacket() {}, onDispatched() {}, onUpStopped() {} });
  try {
    lanes.offer({ gen: 1, tok: "token", list: ["test:pinnedfetch"] });
    globalThis.location.href = "https://successor.example/";
    await ctx.fetch("/__tango/lane/up?gen=1&tok=token");
    assert.equal(calls[0].url, "https://tango.localhost/__tango/lane/up?gen=1&tok=token");
    assert.equal(calls[0].init.redirect, "error");
  } finally {
    lanes.stopAll(); globalThis.fetch = saved; delete globalThis.location; delete globalThis.document;
  }
});

// ---- I2: up:post reports a frame dispatched when its batch goes to fetch ----

test("up:post reports frames dispatched when their batch is handed to fetch", async () => {
  const f = upFetch();
  const ctx = fakeCtx(f.fn);
  const lane = newPostUp();
  await startUp(lane, ctx, f);
  lane.send(frame(1, 0, 20000)); // a full batch: POSTed at once
  assert.equal(f.calls.length, 1);
  assert.deepEqual(ctx.onWire, [[1, 0]]);
  lane.send(frame(1, 1, 20000)); // queued behind the POST
  lane.send(frame(3, 0, 100));
  await sleep(20);
  assert.deepEqual(ctx.onWire, [[1, 0]], "queued frames are not dispatched");
  f.calls[0].respond(204);
  await tick();
  assert.equal(f.calls.length, 2);
  assert.deepEqual(ctx.onWire, [[1, 0], [1, 1], [3, 0]]);
  // A batch waiting for its delay is not dispatched before it is posted.
  f.calls[1].respond(204);
  await tick();
  lane.send(frame(1, 2, 10));
  assert.equal(ctx.onWire.length, 3);
  await sleep(20);
  assert.equal(f.calls.length, 3);
  assert.deepEqual(ctx.onWire.at(-1), [1, 2]);
  // Frames dropped by stop are never reported: the client releases them.
  lane.send(frame(1, 3, 20000));
  lane.send(frame(1, 4, 20000));
  lane.stop();
  assert.deepEqual(ctx.onWire.at(-1), [1, 2]);
  assert.deepEqual(ctx.fails, []);
});

test("behind a never-settling POST, frames get their boundary only when dispatched", async () => {
  const up = newPostUp({ postTimeoutMs: 60000 });
  internal.registerLane("test:hungpost", () => up);
  const b = iifeBridge();
  const c = createClient(b);
  const saved = globalThis.fetch;
  const posts = [];
  globalThis.fetch = (_url, init) => {
    const frames = decodePacket(new Uint8Array(init.body)).map((g) => [g.stream, g.seq]);
    if (!frames.length) return Promise.resolve({ status: 204 }); // the start probe
    return new Promise((resolve) => posts.push({ frames, resolve }));
  };
  b.recv({ tango: "hello", proto: 2, win: 1048576, lanes: { gen: 1, tok: "t", list: ["test:hungpost"] } });
  const msgs = () =>
    b.out
      .splice(0)
      .map((raw) => {
        try {
          return JSON.parse(raw);
        } catch {
          return {};
        }
      })
      .filter((m) => m.tango === "stream");
  try {
    await tick();
    const s = c.stream("Up");
    b.recv({ tango: "stream", id: 1, op: "accept", in: true });
    msgs();
    await s.sendBytes(bytes(POST_MIN_SIZE + 100));
    assert.deepEqual(posts.map((p) => p.frames), [[[1, 0]]]);
    assert.deepEqual(msgs(), [{ tango: "stream", id: 1, op: "boundary", seq: 1 }]);
    // The first POST never settles: the frames behind it wait in up:post.
    await s.sendBytes(bytes(POST_MIN_SIZE + 100));
    await s.sendBytes(bytes(POST_MIN_SIZE + 100));
    s.end();
    await sleep(30);
    assert.equal(posts.length, 1);
    assert.deepEqual(msgs(), [], "no boundary (and no end) before the batch is dispatched");
    // Once the batch goes to fetch: one boundary per frame, then the end.
    posts[0].resolve({ status: 204 });
    await tick();
    assert.deepEqual(posts.map((p) => p.frames), [[[1, 0]], [[1, 1], [1, 2]]]);
    assert.deepEqual(msgs(), [
      { tango: "stream", id: 1, op: "boundary", seq: 2 },
      { tango: "stream", id: 1, op: "boundary", seq: 3 },
      { tango: "stream", id: 1, op: "end", seq: 3 },
    ]);
  } finally {
    up.stop();
    globalThis.fetch = saved;
    for (const p of posts) p.resolve({ status: 204 });
  }
});

// ---- round 2 N1: message-channel frames wait behind undispatched lane frames ----

test("message-channel data of a stream waits until its queued lane frames are dispatched", async () => {
  const up = newPostUp({ postTimeoutMs: 60000 });
  internal.registerLane("test:hungpost2", () => up);
  const b = iifeBridge();
  const c = createClient(b);
  const saved = globalThis.fetch;
  const posts = [];
  globalThis.fetch = (_url, init) => {
    const frames = decodePacket(new Uint8Array(init.body)).map((g) => [g.stream, g.seq]);
    if (!frames.length) return Promise.resolve({ status: 204 }); // the start probe
    return new Promise((resolve) => posts.push({ frames, resolve }));
  };
  b.recv({ tango: "hello", proto: 2, win: 1048576, lanes: { gen: 1, tok: "t", list: ["test:hungpost2"] } });
  const msgs = () =>
    b.out
      .splice(0)
      .map((raw) => {
        try {
          return JSON.parse(raw);
        } catch {
          return {};
        }
      })
      .filter((m) => m.tango === "stream");
  try {
    await tick();
    const s = c.stream("Up");
    b.recv({ tango: "stream", id: 1, op: "accept", in: true });
    msgs();
    await s.sendBytes(bytes(POST_MIN_SIZE + 100)); // seq 0: POSTed at once
    await s.sendBytes(bytes(POST_MIN_SIZE + 100)); // seq 1: queued behind the hung POST
    assert.deepEqual(msgs(), [{ tango: "stream", id: 1, op: "boundary", seq: 1 }]);
    // Later frames of the same stream that take the message channel.
    let done = 0;
    const p1 = s.send({ hi: 1 }).then(() => done++);
    const p2 = s.sendBytes(new Uint8Array([1, 2, 3])).then(() => done++);
    await sleep(30);
    assert.deepEqual(msgs(), [], "no message-channel data may reveal seq 1 before it is dispatched");
    assert.equal(done, 0);
    assert.equal(s.sent, 2);
    posts[0].resolve({ status: 204 });
    await tick();
    await Promise.all([p1, p2]);
    assert.deepEqual(posts.map((p) => p.frames), [[[1, 0]], [[1, 1]]]);
    const out = msgs();
    assert.deepEqual(
      out.map((m) => [m.op, m.seq]),
      [["boundary", 2], ["data", 2], ["data", 3]],
    );
    assert.deepEqual(out[1].data, { hi: 1 });
    assert.equal(out[2].bin, "AQID");
    assert.equal(out[1].c, JSON.stringify({ hi: 1 }).length + 64);
    assert.equal(out[2].c, 3 + 64);
    assert.equal(s.bufferedAmount, 0);
  } finally {
    up.stop();
    globalThis.fetch = saved;
    for (const p of posts) p.resolve({ status: 204 });
  }
});

// ---- round 2 N2: a late POST timer (stalled main thread) re-arms once ----

async function stubbedPostTimer(lateBy, run) {
  const timers = [];
  const savedSet = globalThis.setTimeout;
  const savedClear = globalThis.clearTimeout;
  const savedNow = Date.now;
  let now = 1_000_000;
  let probed = false;
  const ctx = fakeCtx(() => {
    if (!probed) { probed = true; return Promise.resolve({ status: 204 }); } // the start probe
    return new Promise(() => {}); // the POST never settles
  });
  const up = newPostUp();
  await up.start(ctx);
  globalThis.setTimeout = (fn, ms) => {
    timers.push({ fn, ms });
    return timers.length;
  };
  globalThis.clearTimeout = () => {};
  Date.now = () => now;
  try {
    up.send(frame(1, 0, 20000)); // a full batch: POSTed at once
    run({ timers, ctx, late: () => (now += POST_TIMEOUT_MS + lateBy) });
  } finally {
    globalThis.setTimeout = savedSet;
    globalThis.clearTimeout = savedClear;
    Date.now = savedNow;
    up.stop();
  }
}

test("a late up:post timer re-arms once, then fails the lane on the second expiry", async () => {
  await stubbedPostTimer(1500, ({ timers, ctx, late }) => {
    assert.equal(timers.length, 1);
    assert.equal(timers[0].ms, POST_TIMEOUT_MS);
    late();
    timers[0].fn();
    assert.equal(ctx.fails.length, 0, "a late expiry does not fail the lane");
    assert.equal(timers.length, 2, "re-armed");
    assert.equal(timers[1].ms, POST_TIMEOUT_MS, "with the full timeout");
    late();
    timers[1].fn();
    assert.equal(ctx.fails.length, 1, "the second expiry fails the lane");
    assert.match(String(ctx.fails[0]?.message), /timeout/);
  });
});

test("an on-time up:post timer (late by at most 1 s) fails the lane at once", async () => {
  await stubbedPostTimer(1000, ({ timers, ctx, late }) => {
    late();
    timers[0].fn();
    assert.equal(ctx.fails.length, 1);
    assert.equal(timers.length, 1, "not re-armed");
  });
});