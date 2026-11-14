// E2E harness: runs the embedded IIFE in a VM context whose window.__tango is
// wired to stdio. Each line on stdin/stdout is one raw message encoded as a
// JSON string literal. Usage:
//
//   node harness.mjs <tango.iife.js>                    the ipc scenarios
//   node harness.mjs <tango.iife.js> lanes <base URL>   the fast lane scenarios
//   node harness.mjs <tango.iife.js> nolanes <base URL> the same, lanes off
//
// In the lane modes the page's relative fetch("/__tango/lane/...") goes to
// <base URL> (Go's httptest server), as it would go to the asset origin in
// a WebView, and the client is created with a 300 ms gap timeout.
//
// Scenarios run one after another. The original ones fill the map sent with
// the final "DONE <json>" line; the stream scenarios report through
// tango.emit("result:<name>", …), and e2e_test.go asserts both. Sizes and
// byte patterns must match e2e_test.go.
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { createInterface } from "node:readline";
import vm from "node:vm";

const [scriptPath, mode = "ipc", laneBase = ""] = process.argv.slice(2);
const laneMode = mode === "lanes" || mode === "nolanes";
const LANE_GAP_MS = 300; // e2e_test.go: laneGapMs

const out = (msg) => process.stdout.write(JSON.stringify(msg) + "\n");

// Minimal core shim: buffer Go→JS messages until the first subscriber.
const subs = new Set();
let pending = [];
const bridge = {
  post: (s) => out(String(s)),
  onMessage(fn) {
    subs.add(fn);
    if (pending) {
      const p = pending;
      pending = null;
      for (const m of p) fn(m);
    }
    return () => subs.delete(fn);
  },
};
createInterface({ input: process.stdin }).on("line", (line) => {
  const msg = JSON.parse(line);
  if (pending) pending.push(msg);
  else for (const fn of [...subs]) fn(msg);
});

// A VM context has only the JS builtins: hand it the web APIs the runtime
// uses (streams, events, Blob, base64, text encoding, timers).
const globals = {
  __tango: bridge,
  setTimeout,
  clearTimeout,
  setInterval,
  clearInterval,
  setImmediate,
  clearImmediate,
  queueMicrotask,
  console,
  atob,
  btoa,
  TextEncoder,
  TextDecoder,
  Blob,
  ReadableStream,
  WritableStream,
  TransformStream,
  EventTarget,
  Event,
  MessageEvent,
};
if (typeof globalThis.CloseEvent === "function") globals.CloseEvent = globalThis.CloseEvent;
if (laneMode) {
  // A page on the asset origin resolves the lanes' relative URLs against
  // that origin; node's fetch has no base URL, so supply Go's server.
  globals.fetch = (input, init) => fetch(new URL(String(input), laneBase), init);
  globals.AbortController = AbortController;
  // No eager client: the lane modes create it with a short gap timeout.
  delete globals.__tango;
}
const ctx = vm.createContext(globals);
ctx.window = ctx;
vm.runInContext(readFileSync(scriptPath, "utf8"), ctx);
const tango = ctx.tango;
if (laneMode) {
  const client = tango.createClient(bridge, { gapTimeoutMs: LANE_GAP_MS });
  // Make it window.__tango's shared client (getClient's cache, keyed by a
  // registry symbol shared across realms), so tango.* uses it like a page.
  Object.defineProperty(ctx, Symbol.for("tango.ipc.client"), { value: { bridge, client }, configurable: true });
  ctx.__tango = bridge;
  if (tango.getClient() !== client) throw new Error("harness: tango.getClient() is not the lane client");
  // Go is done once it closes stdin; the down:scheme fetch would keep node alive.
  process.stdin.on("close", () => process.exit(0));
}

const once = (name) => new Promise((resolve) => {
  const off = tango.on(name, (d) => {
    off();
    resolve(d);
  });
});

// Values from the VM realm fail instanceof checks against this realm's
// constructors; compare by tag instead.
const isU8 = (v) => Object.prototype.toString.call(v) === "[object Uint8Array]";
const sha = (u8) => createHash("sha256").update(u8).digest("hex");

async function basics() {
  const r = {};
  const news = new Promise((resolve, reject) => {
    tango.onStream("news", async (s, topic) => {
      try {
        const items = [];
        for await (const v of s) items.push(v);
        // Go only ended its side: write one frame back, then end ours.
        await s.send({ got: items.length, last: items.at(-1) });
        s.end();
        resolve({ topic, count: items.length, last: items.at(-1) });
      } catch (e) {
        reject(e);
      }
    });
  });

  r.hello = await tango.call("Greeter.Hello", "world");
  r.div = await tango.call("Svc.Div", 9, 3);
  try {
    await tango.call("Svc.Div", 1, 0);
  } catch (e) {
    r.divErr = e.code;
  }
  try {
    await tango.call("Svc.Div", "x");
  } catch (e) {
    r.badArgs = e.code;
  }
  try {
    await tango.call("Nope");
  } catch (e) {
    r.missing = e.code;
  }

  let sum = 0;
  let n = 0;
  for await (const v of tango.stream("Count", 300)) {
    sum += v;
    n++;
  }
  r.count = { n, sum };

  const canceled = once("canceled");
  let taken = 0;
  for await (const _ of tango.stream("Forever")) if (++taken === 10) break;
  r.canceled = await canceled;

  const pong = once("pong");
  tango.emit("ping", { n: 41 });
  r.pong = await pong;

  tango.emit("open-news");
  r.news = await news;

  const echo = new Promise((resolve) => tango.onRaw(resolve));
  tango.postRaw("raw hello");
  r.raw = await echo;
  return r;
}

const ECHO_BIG = 1 << 20;
const echoBig = () => {
  const b = new Uint8Array(ECHO_BIG);
  for (let i = 0; i < b.length; i++) b[i] = i % 251;
  return b;
};

/** Same shape as Go's echoSummary. */
function summarize(frames) {
  const out = [];
  for (const v of frames) out.push(isU8(v) ? `bytes:${v.length}:${sha(v)}` : "json:" + JSON.stringify(v));
  let bigPattern = frames.length === 4 && isU8(frames[3]) && frames[3].length === ECHO_BIG;
  if (bigPattern) {
    const b = frames[3];
    for (let i = 0; i < b.length; i++) {
      if (b[i] !== i % 251) {
        bigPattern = false;
        break;
      }
    }
  }
  return { frames: out, bigPattern };
}

let echoResult = null;

async function duplexEcho() {
  const s = tango.stream("Echo");
  const received = (async () => {
    const frames = [];
    for await (const v of s) frames.push(v);
    return frames;
  })();
  await Promise.all([
    s.send({ k: "v" }),
    s.sendBytes(new Uint8Array([0, 255])),
    s.sendBytes(new Uint8Array(0)),
    s.sendBytes(echoBig()),
  ]);
  s.end();
  echoResult = summarize(await received);
  tango.emit("result:duplexEcho", echoResult);
}

async function backpressureBothWays() {
  const TOTAL = 4 << 20;
  const FRAME = 64 << 10;
  const s = tango.stream("Pump");
  const received = (async () => {
    let n = 0;
    let last = -1;
    let ok = true;
    let frames = 0;
    for await (const v of s) {
      if (!isU8(v)) throw new Error("Pump sent a JSON frame");
      for (let i = 0; i < v.length; i++) if (v[i] !== (n + i) % 251) ok = false;
      n += v.length;
      if (v.length) last = v[v.length - 1];
      frames++;
    }
    return { n, last, ok, frames };
  })();
  for (let off = 0; off < TOTAL; off += FRAME) {
    const b = new Uint8Array(FRAME);
    for (let i = 0; i < FRAME; i++) b[i] = (off + i) % 253;
    await s.sendBytes(b);
  }
  s.end();
  tango.emit("result:backpressureBothWays", await received);
}

async function pipeUpload() {
  const TOTAL = 3 << 20;
  const CHUNK = 50000; // not a divisor of TOTAL: the last chunk is short
  const local = createHash("sha256");
  let off = 0;
  const rs = new ReadableStream({
    pull(ctrl) {
      if (off >= TOTAL) {
        ctrl.close();
        return;
      }
      const b = new Uint8Array(Math.min(CHUNK, TOTAL - off));
      for (let i = 0; i < b.length; i++) b[i] = ((off + i) * 31 + 7) & 255;
      local.update(b);
      off += b.length;
      ctrl.enqueue(b);
    },
  });
  const s = tango.stream("Upload");
  const received = (async () => {
    const frames = [];
    for await (const v of s) frames.push(v);
    return frames;
  })();
  await rs.pipeTo(s.writable);
  const [n, goSha, ...rest] = await received;
  if (rest.length) throw new Error("Upload sent extra frames: " + JSON.stringify(rest));
  tango.emit("result:pipeUpload", { n, sha256: goSha, local: local.digest("hex") });
}

async function socketEcho() {
  const ws = tango.socket("Echo");
  const messages = [];
  const closed = new Promise((resolve) => {
    ws.onopen = () => {
      ws.send("hello");
      ws.send(new Uint8Array([1, 2, 3]));
    };
    ws.onmessage = (e) => {
      messages.push(typeof e.data === "string" ? e.data : Array.from(new Uint8Array(e.data)));
      if (messages.length === 2) ws.close();
    };
    ws.onerror = () => messages.push("error event");
    ws.onclose = (e) => resolve({ code: e.code, wasClean: e.wasClean });
  });
  const c = await closed;
  tango.emit("result:socketEcho", { messages, ...c, readyState: ws.readyState });
}

async function inProcessParity() {
  // Go runs duplexEcho's inputs through the same Echo with Router.Open.
  const go = await tango.call("Parity.Run");
  tango.emit("result:inProcessParity", { same: JSON.stringify(go) === JSON.stringify(echoResult), go });
}

async function main() {
  tango.emit("e2e:start", "basics");
  const r = await basics();
  for (const fn of [duplexEcho, backpressureBothWays, pipeUpload, socketEcho, inProcessParity]) {
    tango.emit("e2e:start", fn.name);
    await fn();
  }
  return r;
}

// ---- Fast lane scenarios (modes lanes and nolanes; see lanes_e2e_test.go) ----

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/** Reads s in the background: frames, waitFor(n), finished, error. */
function collect(s) {
  const frames = [];
  let waiters = [];
  let done = false;
  let error = null;
  const wake = () => {
    waiters = waiters.filter((w) => {
      if (frames.length < w.n && !done) return true;
      w.resolve();
      return false;
    });
  };
  const finished = (async () => {
    try {
      for await (const v of s) {
        frames.push(v);
        wake();
      }
    } catch (e) {
      error = e;
    } finally {
      done = true;
      wake();
    }
  })();
  return {
    frames,
    finished,
    get error() {
      return error;
    },
    waitFor: (n) =>
      new Promise((resolve) => {
        waiters.push({ n, resolve });
        wake();
      }),
  };
}

/** Frame k of a tagged sequence: bytes 0..3 tag, 4..7 k, then (k*7+i)&255. */
function tagged(tag, k, len) {
  const b = new Uint8Array(len);
  for (let i = 8; i < len; i++) b[i] = (k * 7 + i) & 255;
  const dv = new DataView(b.buffer);
  dv.setUint32(0, tag);
  dv.setUint32(4, k);
  return b;
}

const sameBytes = (a, b) => {
  if (!isU8(a) || a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
};

/** Polls tango.lanes() until ok(lanes) or ms pass; returns the last value. */
async function lanesWhen(ok, ms) {
  const end = Date.now() + ms;
  let l = tango.lanes();
  while (!ok(l) && Date.now() < end) {
    await sleep(5);
    l = tango.lanes();
  }
  return { down: l.down, up: l.up };
}

async function lanesUp() {
  const want = mode === "lanes" ? { down: "down:scheme", up: "up:post" } : { down: "fallback", up: "fallback" };
  // up:post is ready at once, down:scheme once its response has started.
  tango.emit("result:lanesUp", await lanesWhen((l) => l.down === want.down && l.up === want.up, 5000));
}

const BIG_TOTAL = 8 << 20; // bigBothWays: byte i = i % 251
const BIG_FRAME = 64 << 10;

async function bigBothWays() {
  const s = tango.stream("Echo");
  const c = collect(s);
  for (let off = 0; off < BIG_TOTAL; off += BIG_FRAME) {
    const b = new Uint8Array(BIG_FRAME);
    for (let i = 0; i < BIG_FRAME; i++) b[i] = (off + i) % 251;
    await s.sendBytes(b);
  }
  s.end();
  await c.finished;
  if (c.error) throw c.error;
  let n = 0;
  let ok = true;
  for (const v of c.frames) {
    if (!isU8(v)) {
      ok = false;
      continue;
    }
    for (let i = 0; i < v.length; i++) if (v[i] !== (n + i) % 251) ok = false;
    n += v.length;
  }
  tango.emit("result:bigBothWays", { n, frames: c.frames.length, ok });
}

async function mixedSizesOrder() {
  const s = tango.stream("Echo");
  const c = collect(s);
  const sent = [];
  for (let k = 0; k < 40; k++) sent.push(tagged(0x4d495844, k, k % 2 ? 64 << 10 : 100));
  // Frame 0 (small) first: once it is back, Go has accepted the stream, so
  // the opener rule no longer keeps JS's big frames off up:post.
  await s.sendBytes(sent[0]);
  await c.waitFor(1);
  // No await between the rest: small (message) and big (lane) frames race.
  await Promise.all(sent.slice(1).map((b) => s.sendBytes(b)));
  s.end();
  await c.finished;
  if (c.error) throw c.error;
  let inOrder = c.frames.length === sent.length;
  for (let k = 0; inOrder && k < sent.length; k++) inOrder = sameBytes(c.frames[k], sent[k]);
  tango.emit("result:mixedSizesOrder", { frames: c.frames.length, inOrder });
}

async function laneFailure() {
  const A = 0x41414141;
  const B = 0x42424242;
  const BIG = 64 << 10;
  const a = tango.stream("Echo");
  const b = tango.stream("Echo");
  const ca = collect(a);
  const cb = collect(b);
  const bSent = [];
  const sendB = async (k, len) => {
    const f = tagged(B, k, len);
    bSent.push(f);
    await b.sendBytes(f);
  };

  // Both streams carry big frames on the lanes first.
  for (let k = 0; k < 4; k++) await sendB(k, BIG);
  for (let k = 0; k < 2; k++) await a.sendBytes(tagged(A, k, BIG));
  await cb.waitFor(4);
  await ca.waitFor(2);

  // Go cuts the down:scheme response at the next packet it writes: A's
  // echo of frame 2 (B is idle). That packet, and any queued behind it,
  // is lost.
  const armed = once("lanefail:armed");
  tango.emit("lanefail:arm");
  await armed;
  const tripped = once("lanefail:tripped");
  await a.sendBytes(tagged(A, 2, BIG));
  await tripped; // down:scheme has failed in Go
  const t0 = Date.now();

  // Later frames: A's echoes go as messages (A still misses frame 2); B
  // goes on in parallel.
  const aLater = (async () => {
    try {
      for (let k = 3; k < 8; k++) await a.sendBytes(tagged(A, k, k === 7 ? 100 : BIG));
      a.end();
    } catch {
      // A may already have failed (gap timeout): the expected end.
    }
  })();
  const bLater = (async () => {
    for (let k = 4; k < 12; k++) await sendB(k, k % 2 ? 100 : BIG);
  })();
  await Promise.all([aLater, bLater]);
  await ca.finished;
  const aElapsedMs = Date.now() - t0;

  await cb.waitFor(bSent.length);
  b.end();
  await cb.finished;
  let bOk = !cb.error && cb.frames.length === bSent.length;
  for (let k = 0; bOk && k < bSent.length; k++) bOk = sameBytes(cb.frames[k], bSent[k]);

  // The down:scheme response ended, so the page fails the lane as well.
  const lanes = await lanesWhen((l) => l.down === "fallback", 5000);
  tango.emit("result:laneFailure", {
    aCode: ca.error ? ca.error.code : null,
    aReceived: ca.frames.length,
    aElapsedMs,
    bOk,
    bFrames: cb.frames.length,
    lanes,
  });
}

async function laneMain() {
  const fns = mode === "lanes" ? [lanesUp, bigBothWays, mixedSizesOrder, laneFailure] : [lanesUp, bigBothWays];
  for (const fn of fns) {
    tango.emit("e2e:start", fn.name);
    await fn();
  }
  return {};
}

(laneMode ? laneMain() : main()).then(
  (r) => {
    out("DONE " + JSON.stringify(r));
  },
  (e) => {
    out("FAIL " + (e && e.stack ? e.stack : String(e)));
  },
);
