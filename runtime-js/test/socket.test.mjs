import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { createClient, TangoSocket, socket, FRAME_OVERHEAD } from "../dist/index.js";

// A mock window.__tango: records what JS posts, lets the test play Go.
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
async function until(cond) {
  for (let i = 0; i < 100; i++) {
    if (cond()) return;
    await tick();
  }
  assert.fail("condition not reached");
}

// Opens socket id 1 on a client that got Go's hello; optionally accepted.
async function openSocket({ accept = true } = {}) {
  const b = fakeBridge();
  const c = createClient(b);
  b.recv({ tango: "hello", proto: 2, win: 1048576 });
  const ws = c.socket("Chat.Join", "room");
  b.take();
  const log = [];
  for (const t of ["open", "message", "error", "close"]) {
    ws.addEventListener(t, (e) => log.push(t === "close" ? `close:${e.code}:${e.reason}:${e.wasClean}` : t));
  }
  if (accept) {
    b.recv({ tango: "stream", id: 1, op: "accept", in: true });
    await tick();
  }
  return { b, c, ws, log };
}

const go = (op, extra = {}) => ({ tango: "stream", id: 1, op, ...extra });
const frame = (seq, payload) => go("data", { seq, c: 70, ...payload });

test("readyState transitions and open event", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  b.take();
  const ws = c.socket("Chat.Join", "room", 2);
  assert.ok(ws instanceof TangoSocket);
  assert.ok(ws instanceof EventTarget);
  assert.deepEqual(
    [TangoSocket.CONNECTING, TangoSocket.OPEN, TangoSocket.CLOSING, TangoSocket.CLOSED],
    [0, 1, 2, 3],
  );
  assert.deepEqual([ws.CONNECTING, ws.OPEN, ws.CLOSING, ws.CLOSED], [0, 1, 2, 3]);
  assert.equal(ws.url, "tango:Chat.Join");
  assert.equal(ws.readyState, TangoSocket.CONNECTING);
  assert.equal(ws.binaryType, "arraybuffer");
  assert.equal(ws.bufferedAmount, 0);
  assert.deepEqual(b.take(), [{ tango: "stream", id: 1, op: "open", method: "Chat.Join", args: ["room", 2] }]);

  const events = [];
  ws.onopen = (e) => events.push("onopen:" + e.type);
  ws.addEventListener("open", () => events.push("open"));
  ws.onclose = (e) => events.push("onclose:" + e.code);
  await tick();
  assert.deepEqual(events, []);
  b.recv(go("accept", { in: true }));
  await tick();
  assert.equal(ws.readyState, TangoSocket.OPEN);
  assert.deepEqual(events, ["onopen:open", "open"]);

  ws.close();
  assert.equal(ws.readyState, TangoSocket.CLOSING);
  assert.deepEqual(b.take(), [go("close", { seq: 0 })]);
  ws.close(4000, "again"); // idempotent
  assert.deepEqual(b.take(), []);
  await tick(); // closed without waiting for Go
  assert.equal(ws.readyState, TangoSocket.CLOSED);
  assert.deepEqual(events, ["onopen:open", "open", "onclose:1000"]);
});

test("module socket and IIFE expose socket", async () => {
  assert.equal(typeof socket, "function");
  const b = fakeBridge();
  const saved = globalThis.__tango;
  globalThis.__tango = b;
  try {
    const ws = socket("A.B", 1);
    assert.ok(ws instanceof TangoSocket);
    assert.deepEqual(
      b.take().filter((m) => m.tango === "stream"),
      [{ tango: "stream", id: 1, op: "open", method: "A.B", args: [1] }],
    );
  } finally {
    globalThis.__tango = saved;
  }

  // The IIFE still loads where EventTarget is missing; socket() then fails.
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const ctx = vm.createContext({ __tango: fakeBridge(), setTimeout });
  vm.runInContext(src, ctx);
  assert.equal(typeof ctx.tango.socket, "function");
  assert.equal(typeof ctx.tango.TangoSocket, "function");
  assert.throws(() => ctx.tango.socket("X"), (e) => e.code === "unsupported");
});

test("message data mapping", async () => {
  const { b, ws, log } = await openSocket();
  const got = [];
  ws.onmessage = (e) => got.push(e);
  b.recv(frame(0, { data: "hi" }));
  b.recv(frame(1, { data: { a: 1 } }));
  b.recv(frame(2, { data: 5 }));
  b.recv(frame(3, { data: null }));
  b.recv(frame(4, { bin: "AQ==" }));
  await until(() => got.length === 5);
  assert.ok(got.every((e) => e instanceof MessageEvent && e.type === "message"));
  assert.deepEqual(got.slice(0, 4).map((e) => e.data), ["hi", '{"a":1}', "5", "null"]);
  assert.ok(got[4].data instanceof ArrayBuffer);
  assert.deepEqual([...new Uint8Array(got[4].data)], [1]);

  ws.binaryType = "nonsense"; // ignored, as by WebSocket
  assert.equal(ws.binaryType, "arraybuffer");
  ws.binaryType = "blob";
  assert.equal(ws.binaryType, "blob");
  b.recv(frame(5, { bin: "AQ==" }));
  b.recv(frame(6, { bin: "" }));
  await until(() => got.length === 7);
  assert.ok(got[5].data instanceof Blob);
  assert.equal(got[5].data.size, 1);
  assert.deepEqual([...new Uint8Array(await got[5].data.arrayBuffer())], [1]);
  assert.equal(got[6].data.size, 0);
  assert.deepEqual(log, ["open", ...Array(7).fill("message")]);
});

test("send mapping keeps order across Blob", async () => {
  const { b, ws } = await openSocket();
  const u = new Uint8Array([2]);
  ws.send("x");
  ws.send(new Blob([new Uint8Array([1])]));
  ws.send(u);
  u[0] = 9; // copied at send time even while waiting behind the Blob
  ws.send(new Uint8Array([3, 4]).buffer);
  ws.send(new DataView(new Uint8Array([5]).buffer));
  ws.send({ toString: () => "obj" }); // other values are sent as their string
  // Everything after the Blob waits for it: x went out, the rest is buffered.
  assert.equal(ws.bufferedAmount, 1 + 1 + 2 + 1 + 3 + 5 * FRAME_OVERHEAD);
  ws.close(); // close goes behind the queued sends
  await until(() => b.out.length >= 7);
  const msgs = b.take();
  assert.deepEqual(
    msgs.map((m) => [m.op, m.seq, m.data ?? m.bin]),
    [
      ["data", 0, "x"],
      ["data", 1, "AQ=="],
      ["data", 2, "Ag=="],
      ["data", 3, "AwQ="],
      ["data", 4, "BQ=="],
      ["data", 5, "obj"],
      ["close", 6, undefined],
    ],
  );
  assert.equal(ws.bufferedAmount, 0);
});

test("send while CONNECTING queues", async () => {
  const { b, ws } = await openSocket({ accept: false });
  assert.equal(ws.readyState, TangoSocket.CONNECTING);
  ws.send("early"); // no InvalidStateError, unlike WebSocket
  ws.send(new Uint8Array([7]));
  await until(() => b.out.length >= 2);
  assert.deepEqual(
    b.take().map((m) => [m.op, m.seq, m.data ?? m.bin]),
    [
      ["data", 0, "early"],
      ["data", 1, "Bw=="],
    ],
  );
  b.recv(go("accept", { in: true }));
  await tick();
  assert.equal(ws.readyState, TangoSocket.OPEN);
});

test("close → close marker, code 1000 without waiting for Go", async () => {
  const { b, ws, log } = await openSocket();
  const closes = [];
  ws.onclose = (e) => closes.push(e);
  ws.close(1000, "bye");
  assert.equal(ws.readyState, TangoSocket.CLOSING);
  assert.deepEqual(b.take(), [go("close", { seq: 0 })]);
  await until(() => closes.length === 1);
  assert.equal(ws.readyState, TangoSocket.CLOSED);
  assert.deepEqual(log, ["open", "close:1000::true"]);
  assert.ok(closes[0] instanceof CloseEvent);
  b.recv(frame(0, { data: "late" })); // the stream is finished: ignored
  b.recv(go("close", { seq: 1 }));
  await tick();
  assert.deepEqual(b.take(), []);
  assert.deepEqual(log, ["open", "close:1000::true"]);
  ws.send("after close"); // ignored
  ws.close();
  await tick();
  assert.deepEqual(b.take(), []);
});

test("close waits behind sends blocked on credit", async () => {
  const { b, ws, log } = await openSocket();
  const big = new Uint8Array(1048576); // fills Go's window: the second waits
  ws.send(big);
  ws.send(new Uint8Array([1]));
  ws.close();
  await tick();
  assert.deepEqual(b.take().map((m) => [m.op, m.seq]), [["data", 0]]);
  assert.equal(ws.readyState, TangoSocket.CLOSING);
  b.recv(go("ack", { b: 1048576 + FRAME_OVERHEAD }));
  await until(() => log.length === 2);
  assert.deepEqual(b.take().map((m) => [m.op, m.seq]), [["data", 1], ["close", 2]]);
  assert.deepEqual(log, ["open", "close:1000::true"]);
});

test("close on a stream Go does not send to (in:false)", async () => {
  const { b, ws, log } = await openSocket({ accept: false });
  b.recv(go("accept", { in: false }));
  b.recv(frame(0, { data: "tick" }));
  await until(() => log.length === 2);
  ws.close();
  assert.deepEqual(b.take().filter((m) => m.op !== "ack"), [go("close", { seq: 0 })]);
  await until(() => log.length === 3);
  assert.deepEqual(log, ["open", "message", "close:1000::true"]);
});

test("Go's error while close waits for credit → 1011, nothing more posted", async () => {
  const { b, ws, log } = await openSocket();
  ws.send(new Uint8Array(1048576));
  ws.send(new Uint8Array([1])); // waits for credit; close waits behind it
  ws.close();
  await tick();
  assert.deepEqual(b.take().map((m) => [m.op, m.seq]), [["data", 0]]);
  b.recv(go("error", { seq: 0, error: { message: "boom", code: "internal" } }));
  await until(() => log.length === 3);
  assert.deepEqual(log, ["open", "error", "close:1011:boom:false"]);
  await tick();
  assert.deepEqual(b.take(), []);
});

test("a message that cannot be converted cancels the socket", async () => {
  const { b, ws, log } = await openSocket();
  const saved = globalThis.Blob;
  const savedTimeout = globalThis.setTimeout;
  const reported = [];
  ws.binaryType = "blob";
  globalThis.Blob = class {
    constructor() {
      throw new Error("no Blob here");
    }
  };
  globalThis.setTimeout = (fn) => {
    try {
      fn();
    } catch (e) {
      reported.push(e.message);
    }
  };
  try {
    b.recv(frame(0, { bin: "AQ==" }));
    b.recv(frame(1, { bin: "Ag==" }));
    await until(() => log.length === 2);
  } finally {
    globalThis.Blob = saved;
    globalThis.setTimeout = savedTimeout;
  }
  assert.deepEqual(log, ["open", "close:1006::false"]);
  assert.deepEqual(reported, ["no Blob here"]);
  // The stream was canceled, not left with unread frames.
  assert.deepEqual(b.take().filter((m) => m.op !== "ack"), [go("cancel")]);
});

test("Go ending its side closes the socket with 1000", async () => {
  for (const op of ["end", "close"]) {
    const { b, ws, log } = await openSocket();
    ws.send(new Blob([new Uint8Array([1])])); // still delivered before the answering end
    b.recv(frame(0, { data: "last" }));
    b.recv(go(op, { seq: 1 }));
    await until(() => log.length === 3);
    assert.equal(ws.readyState, TangoSocket.CLOSED);
    assert.deepEqual(log, ["open", "message", "close:1000::true"]);
    const msgs = b.take();
    if (op === "end") {
      assert.deepEqual(
        msgs.filter((m) => m.op !== "ack").map((m) => [m.op, m.seq]),
        [
          ["data", 0],
          ["end", 1],
        ],
      );
    } else {
      // Go stopped receiving: nothing more is sent.
      assert.deepEqual(msgs.filter((m) => m.op !== "ack"), []);
    }
  }
});

test("remote error → error then close 1011", async () => {
  for (const code of ["internal", "canceled"]) {
    const { b, ws, log } = await openSocket();
    const order = [];
    ws.onerror = (e) => order.push("onerror:" + e.type);
    ws.onclose = (e) => order.push(`onclose:${e.code}:${e.reason}:${e.wasClean}`);
    b.recv(frame(0, { data: "before" }));
    b.recv(go("error", { seq: 1, error: { message: "disk full", code } }));
    await until(() => log.length === 4);
    assert.equal(ws.readyState, TangoSocket.CLOSED);
    assert.deepEqual(log, ["open", "message", "error", "close:1011:disk full:false"]);
    assert.deepEqual(order, ["onerror:error", "onclose:1011:disk full:false"]);
  }
});

test("Go refusing the open → error then close 1011, no open", async () => {
  const { b, ws, log } = await openSocket({ accept: false });
  b.recv(go("error", { seq: 0, error: { message: "no such stream", code: "not_found" } }));
  await until(() => log.length === 2);
  assert.deepEqual(log, ["error", "close:1011:no such stream:false"]);
  assert.equal(ws.readyState, TangoSocket.CLOSED);
});

test("remote cancel → close 1006", async () => {
  const { b, ws, log } = await openSocket();
  b.recv(go("cancel"));
  await until(() => log.length === 2);
  assert.deepEqual(log, ["open", "close:1006::false"]);
  assert.equal(ws.readyState, TangoSocket.CLOSED);
});

test("protocol mismatch → close 1006", async () => {
  const b = fakeBridge();
  const c = createClient(b);
  const ws = c.socket("Chat.Join");
  const log = [];
  ws.addEventListener("error", () => log.push("error"));
  ws.addEventListener("close", (e) => log.push(`close:${e.code}:${e.wasClean}`));
  const saved = globalThis.setTimeout;
  globalThis.setTimeout = () => {}; // swallow the client's report of the mismatch
  try {
    b.recv({ tango: "hello", error: { code: "bad_args", message: "runtime-js protocol mismatch" } });
  } finally {
    globalThis.setTimeout = saved;
  }
  await until(() => log.length === 1);
  assert.deepEqual(log, ["close:1006:false"]);
  assert.equal(ws.readyState, TangoSocket.CLOSED);
  // A socket opened on the failed client closes straight away.
  const ws2 = c.socket("Chat.Join");
  const codes = [];
  ws2.onclose = (e) => codes.push(e.code);
  await until(() => codes.length === 1);
  assert.deepEqual(codes, [1006]);
});

test("protocol violation by Go → close 1006", async () => {
  const { b, log } = await openSocket();
  b.recv(go("data", { seq: 0, c: 70 })); // neither data nor bin
  await until(() => log.length === 2);
  assert.deepEqual(log, ["open", "close:1006::false"]);
  assert.equal(b.take().find((m) => m.op === "error")?.error.code, "bad_args");
});

test("CloseEvent fallback when the environment lacks it", async () => {
  const saved = globalThis.CloseEvent;
  delete globalThis.CloseEvent;
  try {
    const { b, ws } = await openSocket();
    const got = [];
    ws.onclose = (e) => got.push(e);
    b.recv(go("error", { seq: 0, error: { message: "boom", code: "internal" } }));
    await until(() => got.length === 1);
    const e = got[0];
    assert.ok(e instanceof Event);
    assert.equal(e.constructor.name, "CloseEvent");
    assert.equal(e.type, "close");
    assert.deepEqual([e.code, e.reason, e.wasClean], [1011, "boom", false]);
  } finally {
    globalThis.CloseEvent = saved;
  }
});

test("Blob read failure cancels the socket", async () => {
  const { b, ws, log } = await openSocket();
  const bad = new Blob(["x"]);
  bad.arrayBuffer = () => Promise.reject(new Error("unreadable"));
  ws.send(bad);
  ws.send("after");
  await until(() => log.length === 2);
  assert.deepEqual(log, ["open", "close:1006::false"]);
  assert.deepEqual(b.take(), [go("cancel")]);
});

test("a send rejected with bad_args fails the socket", async () => {
  const { b, ws, log } = await openSocket();
  ws.send(new Uint8Array(16777217)); // over MAX_FRAME_BYTES
  await until(() => log.length === 2);
  assert.deepEqual(log, ["open", "close:1006::false"]);
  const [m] = b.take();
  assert.equal(m.op, "error");
  assert.equal(m.error.code, "bad_args");
  assert.match(m.error.message, /MAX_FRAME_BYTES/);
});
