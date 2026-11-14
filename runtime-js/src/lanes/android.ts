// The Android platform lanes (Go: internal/platform/android, lanes.go),
// over the page's WebMessageListener channel. The page script gives the
// runtime __tangoNative.lane = { post(u8), listen(fn) }:
//
//   post(u8)    sends u8.buffer to Go as an ArrayBuffer. u8 is a whole
//               buffer of at least 26 bytes whose first 16 are left for
//               the document token (post writes it, sends, clears it).
//   listen(fn)  once: fn gets every ArrayBuffer Java posts back.
//
// Up (page → Go): token[16] | gen:u64 LE | packet. The probe is exactly
// token | gen | 00 00 (26 bytes): Go keeps the probe's reply proxy for the
// downstream lane and answers [0], or answers [1] when the core refuses it.
// A data packet the core refuses is answered [1] too; an accepted one gets
// nothing. Down (Go → page): packets of at least 2 bytes through the
// proxy of the last accepted probe.
//
// Replies pair with probes first in, first out (each probe gets exactly
// one): only the reply to this generation's probe settles it, so a late
// reply to an earlier probe is consumed and ignored. Down packets that
// arrive before this generation's [0] belong to an earlier one (they were
// queued on the same proxy ahead of the ack) and are dropped.
//
// Both modules always register; start rejects at once when the page has
// no __tangoNative.lane (no lanes on this page, or the runtime loaded
// before the page script). The connection lives on the global object so
// the ESM build and the injected IIFE share it and listen is called once.

import { registerLane, type DownLane, type LaneContext, type UpLane } from "../lanes.js";
import type { PacketFrame } from "../packet.js";

/** How long a start waits for the reply to its generation's probe. */
export const ANDROID_PROBE_TIMEOUT_MS = 2000;

const HEADROOM = 16; // token, written by __tangoNative.lane.post
const UP_HEADER = HEADROOM + 8; // token | gen
const PROBE_SIZE = UP_HEADER + 2; // header | 00 00
const FRAME_HEADER = 20;
const TWO32 = 0x100000000;

interface NativeLane {
  post(u8: Uint8Array): void;
  listen(fn: (ab: ArrayBuffer) => void): void;
}

interface Probe {
  gen: number;
  state: "pending" | "ok" | "failed";
  done: Promise<void>;
  settle(err?: Error): void;
}

/** One user (started module) of the connection. */
interface Down {
  gen: number;
  onPacket(p: Uint8Array): void;
}
interface Up {
  gen: number;
  refused(): void;
}

interface Conn {
  native: NativeLane;
  /** Probes sent and not answered yet, oldest first. */
  unanswered: Probe[];
  /** This generation's probe (shared by both modules), or null. */
  probe: Probe | null;
  down: Down | null;
  up: Up | null;
}

const CONN = Symbol.for("tango.androidLane");

function nativeLane(): NativeLane | null {
  const n = (globalThis as any).__tangoNative;
  const l = n && n.lane;
  return l && typeof l.post === "function" && typeof l.listen === "function" ? (l as NativeLane) : null;
}

/** The page's connection, made (and listened on) the first time a lane
 * starts; null without __tangoNative.lane. */
function conn(): Conn | null {
  const g = globalThis as any;
  const have = g[CONN] as Conn | undefined;
  if (have) return have;
  const native = nativeLane();
  if (!native) return null;
  const c: Conn = { native, unanswered: [], probe: null, down: null, up: null };
  native.listen((ab) => receive(c, ab));
  Object.defineProperty(g, CONN, { value: c, configurable: true });
  return c;
}

function receive(c: Conn, ab: unknown): void {
  if (!(ab instanceof ArrayBuffer)) return;
  if (ab.byteLength >= 2) {
    const p = c.probe;
    const d = c.down;
    // Only after this generation's ack: earlier packets are an earlier
    // generation's.
    if (d && p && p.state === "ok" && p.gen === d.gen) d.onPacket(new Uint8Array(ab));
    return;
  }
  if (ab.byteLength !== 1) return;
  const ok = new Uint8Array(ab)[0] === 0;
  const head = c.unanswered.shift();
  if (head) {
    // The reply to head; it counts only if head is still this generation's.
    if (head === c.probe && head.state === "pending") head.settle(ok ? undefined : new Error("up:android: probe refused by Go"));
    return;
  }
  // No probe outstanding: Go refused a data packet.
  const p = c.probe;
  const u = c.up;
  if (!ok && u && p && p.state === "ok" && p.gen === u.gen) u.refused();
}

/** This generation's probe, sent now unless one is out already. */
function probe(c: Conn, gen: number): Promise<void> {
  const have = c.probe;
  if (have && have.gen === gen) return have.done;
  let settle!: (err?: Error) => void;
  const done = new Promise<void>((resolve, reject) => {
    settle = (err) => (err ? reject(err) : resolve());
  });
  done.catch(() => {}); // a probe nobody waits for any more
  let timer: ReturnType<typeof setTimeout> | null = null;
  const pr: Probe = {
    gen,
    state: "pending",
    done,
    settle(err) {
      if (pr.state !== "pending") return;
      pr.state = err ? "failed" : "ok";
      if (timer !== null) clearTimeout(timer);
      settle(err);
    },
  };
  c.probe = pr;
  const u8 = new Uint8Array(PROBE_SIZE);
  putGen(u8, gen);
  try {
    c.native.post(u8);
  } catch (err) {
    pr.settle(err instanceof Error ? err : new Error(String(err)));
    return done;
  }
  c.unanswered.push(pr);
  timer = setTimeout(
    () => pr.settle(new Error(`up:android: no probe reply within ${ANDROID_PROBE_TIMEOUT_MS} ms`)),
    ANDROID_PROBE_TIMEOUT_MS,
  );
  return done;
}

/** A module stopped: once both are, the connection forgets the probe (a
 * new start probes again) and drops what arrives. */
function release(c: Conn): void {
  if (c.down || c.up) return;
  const p = c.probe;
  c.probe = null;
  p?.settle(new Error("up:android: stopped"));
}

function putGen(u8: Uint8Array, gen: number): void {
  const dv = new DataView(u8.buffer);
  dv.setUint32(HEADROOM, gen % TWO32, true);
  dv.setUint32(HEADROOM + 4, Math.floor(gen / TWO32), true);
}

function noLane(name: string): Error {
  return new Error(`${name}: this page has no __tangoNative.lane`);
}

class AndroidDown implements DownLane {
  private c: Conn | null = null;
  private me: Down | null = null;
  private stopped = false;

  start(ctx: LaneContext): Promise<void> {
    const c = conn();
    if (!c) return Promise.reject(noLane("down:android"));
    if (this.stopped) return Promise.reject(new Error("down:android: stopped"));
    this.c = c;
    const me: Down = { gen: ctx.gen, onPacket: (p) => ctx.onPacket(p) };
    this.me = me;
    c.down = me;
    return probe(c, ctx.gen).then(() => {
      if (this.stopped || c.down !== me) throw new Error("down:android: stopped");
    });
  }

  stop(): void {
    this.stopped = true;
    const c = this.c;
    if (!c) return;
    if (c.down === this.me) c.down = null;
    this.me = null;
    release(c);
  }
}

class AndroidUp implements UpLane {
  readonly minSize = 0;
  private c: Conn | null = null;
  private me: Up | null = null;
  private ctx: LaneContext | null = null;
  private dead = false;

  start(ctx: LaneContext): Promise<void> {
    const c = conn();
    if (!c) return Promise.reject(noLane("up:android"));
    if (this.dead) return Promise.reject(new Error("up:android: stopped"));
    this.c = c;
    const me: Up = {
      gen: ctx.gen,
      refused: () => this.broken(new Error("up:android refused by Go")),
    };
    this.me = me;
    c.up = me;
    return probe(c, ctx.gen).then(() => {
      if (this.dead || c.up !== me) throw new Error("up:android: stopped");
      this.ctx = ctx;
    });
  }

  /** One packet per frame: headroom | gen | count 1 | frame. */
  send(f: PacketFrame): boolean {
    const ctx = this.ctx;
    const c = this.c;
    if (this.dead || !ctx || !c) return false;
    const n = f.payload.length;
    const u8 = new Uint8Array(UP_HEADER + 2 + FRAME_HEADER + n);
    putGen(u8, ctx.gen);
    const dv = new DataView(u8.buffer);
    let at = UP_HEADER;
    dv.setUint16(at, 1);
    at += 2;
    dv.setUint32(at, Math.floor(f.stream / TWO32));
    dv.setUint32(at + 4, f.stream % TWO32);
    dv.setUint32(at + 8, Math.floor(f.seq / TWO32));
    dv.setUint32(at + 12, f.seq % TWO32);
    dv.setUint32(at + 16, n);
    u8.set(f.payload, at + FRAME_HEADER);
    try {
      c.native.post(u8);
    } catch (err) {
      // ctx.fail must not run inside send.
      queueMicrotask(() => this.broken(err));
      return false;
    }
    ctx.dispatched(f.stream, f.seq);
    return true;
  }

  stop(): void {
    this.dead = true;
    this.release();
  }

  private broken(err: unknown): void {
    if (this.dead) return;
    const ctx = this.ctx;
    this.dead = true;
    this.release();
    ctx?.fail(err);
  }

  private release(): void {
    this.ctx = null;
    const c = this.c;
    if (!c) return;
    if (c.up === this.me) c.up = null;
    this.me = null;
    release(c);
  }
}

/** A down:android module (registered as "down:android"). */
export function newAndroidDown(): DownLane {
  return new AndroidDown();
}

/** An up:android module (registered as "up:android"). */
export function newAndroidUp(): UpLane {
  return new AndroidUp();
}

registerLane("down:android", () => newAndroidDown());
registerLane("up:android", () => newAndroidUp());
