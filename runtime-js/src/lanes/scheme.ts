// The scheme lanes, served by Go's internal/lane (http.go) on the asset
// origin, so they work on every platform whose pages come from it:
//
//   down:scheme  GET  /__tango/lane/down?gen=&tok=  an endless response
//                of packets back to back (ServeDown), starting with the
//                empty packet [0,0] so that its headers go out at once.
//   up:post      POST /__tango/lane/up?gen=&tok=    one packet per
//                request, answered 204 (ServeUp). start posts the empty
//                packet [0,0] first and is ready only on its 204.
//
// Both use relative URLs: Go offers them only to pages from the asset
// origin, where a relative path is tango://app/... or
// https://tango.localhost/... . They register at module load, so the ESM
// entry and the injected IIFE have them before the client's hello reply.

import { registerLane, type DownLane, type LaneContext, type UpLane } from "../lanes.js";
import { encodePacket, MAX_FRAME, type PacketFrame } from "../packet.js";

/** How long down:scheme waits for its first packet before start rejects
 * (a start that never settles would block every later downstream lane). */
export const DOWN_START_TIMEOUT_MS = 5000;
/** up:post takes byte frames of at least this many bytes; smaller ones go
 * on the message channel. */
export const POST_MIN_SIZE = 16 << 10;
/** An up:post batch this big (encoded) is sent without waiting (Go:
 * lane.PostBatchBytes). */
export const POST_BATCH_BYTES = 16 << 10;
/** An up:post batch is sent at most this long after its first frame (Go:
 * lane.PostBatchDelay). */
export const POST_BATCH_DELAY_MS = 4;
/** The largest packet in one up:post request (Go: lane.MaxPostPacket). */
export const MAX_POST_PACKET = 1 << 20;
export const MAX_POST_RETAINED = 8 << 20;
/** Deadline of one up:post request. Below GAP_TIMEOUT_MS: a hung POST
 * fails the lane (later frames fall back to the message channel) before
 * the streams it carries fail with transport. */
export const POST_TIMEOUT_MS = 4000;
/** A POST timer firing later than this after its deadline means the main
 * thread was stalled: it re-arms once (client.ts GAP_LATE_MS likewise). */
const POST_LATE_MS = 1000;

const PACKET_HEADER = 2;
const FRAME_HEADER = 20;
const MAX_FRAMES = 0xffff;

const laneURL = (path: string, ctx: LaneContext) =>
  `/__tango/lane/${path}?gen=${ctx.gen}&tok=${encodeURIComponent(ctx.tok)}`;

/** Cuts a byte stream into packets. Current single-frame downlink packets
 * avoid repeated prefix copies. Multi-frame split packets can recopy prefixes;
 * use an amortized reader before enabling a batching downlink producer. */
class PacketReader {
  private chunks: Uint8Array[] = [];
  private have = 0; // bytes in chunks
  private pos = 0; // bytes of the current packet parsed so far
  private left = -1; // frames of the current packet still to parse; -1: header not read

  push(c: Uint8Array): void {
    if (c.length === 0) return;
    this.chunks.push(c);
    this.have += c.length;
  }

  /** The next complete packet (a view that is valid until the next push),
   * or null. Throws on a frame length beyond MAX_FRAME. */
  next(): Uint8Array | null {
    for (;;) {
      const need = this.left < 0 ? PACKET_HEADER : this.left > 0 ? this.pos + FRAME_HEADER : this.pos;
      if (this.have < need) return null;
      const buf = this.flat();
      if (this.left < 0) {
        this.left = (buf[0] << 8) | buf[1];
        this.pos = PACKET_HEADER;
      } else if (this.left > 0) {
        const at = this.pos + 16;
        const n = buf[at] * 0x1000000 + ((buf[at + 1] << 16) | (buf[at + 2] << 8) | buf[at + 3]);
        if (n > MAX_FRAME) throw new Error(`down:scheme: frame of ${n} bytes exceeds MAX_FRAME_BYTES`);
        this.pos += FRAME_HEADER + n;
        this.left--;
      } else {
        const p = buf.subarray(0, this.pos);
        const rest = buf.subarray(this.pos);
        this.chunks = rest.length ? [rest] : [];
        this.have = rest.length;
        this.pos = 0;
        this.left = -1;
        return p;
      }
    }
  }

  private flat(): Uint8Array {
    if (this.chunks.length > 1) {
      const out = new Uint8Array(this.have);
      let at = 0;
      for (const c of this.chunks) {
        out.set(c, at);
        at += c.length;
      }
      this.chunks = [out];
    }
    return this.chunks[0];
  }
}

export interface SchemeDownOptions {
  /** Default DOWN_START_TIMEOUT_MS. */
  startTimeoutMs?: number;
}

class SchemeDown implements DownLane {
  private ac: AbortController | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;
  private end: (() => void) | null = null;

  constructor(private readonly timeoutMs: number) {}

  start(ctx: LaneContext): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      let state: "starting" | "started" | "done" = "starting";
      // (A function: TypeScript would keep state narrowed across awaits.)
      const done = () => state === "done";
      const started = () => {
        if (state !== "starting") return;
        state = "started";
        this.clearTimer();
        resolve();
      };
      // The lane broke: before the first packet that rejects start, after
      // it the lane fails (unless stop did it).
      const broken = (err: unknown) => {
        if (state === "done") return;
        const wasStarting = state === "starting";
        state = "done";
        this.release();
        if (wasStarting) reject(err);
        else if (!this.stopped) ctx.fail(err);
      };
      this.end = () => broken(new Error("down:scheme: stopped"));
      if (this.stopped) return this.end();

      this.timer = setTimeout(
        () => broken(new Error(`down:scheme: no response within ${this.timeoutMs} ms`)),
        this.timeoutMs,
      );
      const read = async () => {
        const ac = new AbortController();
        this.ac = ac;
        const res = await ctx.fetch(laneURL("down", ctx), { cache: "no-store", signal: ac.signal });
        if (done()) {
          res.body?.cancel().catch(() => {});
          return;
        }
        if (res.status !== 200) throw new Error(`down:scheme: HTTP ${res.status}`);
        if (!res.body) throw new Error("down:scheme: the response has no body stream");
        const reader = res.body.getReader();
        const packets = new PacketReader();
        for (;;) {
          const { done: end, value } = await reader.read();
          if (done()) return;
          if (end) throw new Error("down:scheme: the response ended");
          packets.push(value);
          for (let p = packets.next(); p; p = packets.next()) {
            started();
            if (done()) return;
            if (p.length > PACKET_HEADER) ctx.onPacket(p); // empty packets carry nothing
          }
        }
      };
      read().catch(broken);
    });
  }

  stop(): void {
    this.stopped = true;
    if (this.end) this.end();
    else this.release();
  }

  private clearTimer(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }

  private release(): void {
    this.clearTimer();
    const ac = this.ac;
    this.ac = null;
    if (ac) {
      try {
        ac.abort();
      } catch {}
    }
  }
}

/** A down:scheme module (registered as "down:scheme"; tests build their
 * own with options). */
export function newSchemeDown(opts?: SchemeDownOptions): DownLane {
  const t = opts?.startTimeoutMs;
  return new SchemeDown(typeof t === "number" && t >= 0 ? t : DOWN_START_TIMEOUT_MS);
}

export interface PostUpOptions {
  /** Default POST_BATCH_DELAY_MS. */
  batchDelayMs?: number;
  /** Deadline for each POST, default POST_TIMEOUT_MS. */
  postTimeoutMs?: number;
}

interface Batch {
  frames: PacketFrame[];
  bytes: number; // encoded packet size
}

class PostUp implements UpLane {
  readonly minSize = POST_MIN_SIZE;
  private ctx: LaneContext | null = null;
  /** Frames taken and not posted yet, packet by packet; only the last one
   * still grows. */
  private batches: Batch[] = [];
  private timer: ReturnType<typeof setTimeout> | null = null;
  private busy = false; // a POST is in flight
  private dead = false; // failed or stopped
  private ac: AbortController | null = null;

  private retained = 0; // encoded bytes, including the in-flight packet
  private postTimer: ReturnType<typeof setTimeout> | null = null;
  /** Rejects a start probe still in flight (stop). */
  private end: (() => void) | null = null;

  constructor(private readonly delayMs: number, private readonly timeoutMs: number) {}

  /** Posts the empty packet [0,0] (no frames) and resolves on its 204.
   * Anything else rejects start, so the lane fails before any frame rides
   * it: a page whose CSP connect-src excludes the asset origin (fetch
   * rejects), a refusal (403/503), or no answer within the POST timeout.
   * Until it resolves send takes no frames. */
  start(ctx: LaneContext): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      if (this.dead) return reject(new Error("up:post: stopped"));
      let settled = false;
      const ac = new AbortController();
      this.ac = ac;
      const finish = (err?: unknown) => {
        if (settled) return;
        settled = true;
        this.end = null;
        this.clearPostTimer();
        if (this.ac === ac) this.ac = null;
        if (err !== undefined) {
          try {
            ac.abort(); // a timed-out probe must not linger
          } catch {}
          this.dead = true;
          this.release();
          reject(err);
        } else if (this.dead) {
          reject(new Error("up:post: stopped"));
        } else {
          this.ctx = ctx;
          resolve();
        }
      };
      this.end = () => finish(new Error("up:post: stopped"));
      this.armPostTimer(false, () => finish(new Error(`up:post: start probe timeout after ${this.timeoutMs} ms`)));
      new Promise<Response>((res) => {
        const body = new Uint8Array(PACKET_HEADER).buffer; // count 0
        res(ctx.fetch(laneURL("up", ctx), { method: "POST", body, cache: "no-store", signal: ac.signal }));
      })
        .then((res) => {
          if (res.status !== 204) throw new Error(`up:post: start probe HTTP ${res.status}`);
          finish();
        })
        .catch((err) => finish(err ?? new Error("up:post: start probe failed")));
    });
  }

  send(f: PacketFrame): boolean {
    if (this.dead || !this.ctx) return false;
    const n = FRAME_HEADER + f.payload.length;
    if (PACKET_HEADER + n > MAX_POST_PACKET) return false; // the message channel takes it
    let b = this.batches[this.batches.length - 1];
    const fresh = !b || b.bytes + n > MAX_POST_PACKET || b.frames.length >= MAX_FRAMES;
    const added = n + (fresh ? PACKET_HEADER : 0);
    if (this.retained + added > MAX_POST_RETAINED) return false;
    this.retained += added;
    if (fresh) {
      b = { frames: [], bytes: PACKET_HEADER };
      this.batches.push(b);
    }
    b.frames.push(f);
    b.bytes += n;
    // While a POST is in flight frames gather; its completion posts them.
    if (!this.busy) {
      if (this.batches.length > 1 || b.bytes >= POST_BATCH_BYTES) this.flush();
      else if (this.timer === null) {
        this.timer = setTimeout(() => {
          this.timer = null;
          this.flush();
        }, this.delayMs);
      }
    }
    return true;
  }

  stop(): void {
    this.dead = true;
    this.release();
    this.end?.();
  }

  /** Posts the oldest batch unless a POST is in flight. */
  private flush(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    const ctx = this.ctx;
    if (this.busy || this.dead || !ctx) return;
    const b = this.batches.shift();
    if (!b) return;
    this.busy = true;
    const ac = new AbortController();
    this.ac = ac;
    this.armPostTimer(false);
    const sent = b.frames.map((f) => [f.stream, f.seq] as const);
    // Settles asynchronously even if encoding or fetch throws, so that
    // ctx.fail never runs inside send.
    new Promise<Response>((resolve) => {
      const p = encodePacket(b.frames);
      b.frames = []; // do not retain payloads through a hung/aborted fetch
      // An ArrayBuffer of exactly the packet (not a Blob, not a view):
      // encodePacket returns a view of a buffer of its own.
      const whole = p.byteOffset === 0 && p.byteLength === p.buffer.byteLength;
      const body = (whole ? p.buffer : p.buffer.slice(p.byteOffset, p.byteOffset + p.byteLength)) as ArrayBuffer;
      resolve(ctx.fetch(laneURL("up", ctx), { method: "POST", body, cache: "no-store", signal: ac.signal }));
    })
      .then((res) => {
        if (this.dead) return;
        // 403 (generation over), 413, 503 (lane unusable): the frames of
        // this batch are lost; the receiver's gap timer fails their streams.
        if (res.status !== 204) throw new Error(`up:post: HTTP ${res.status}`);
        this.clearPostTimer();
        this.retained -= b.bytes;
        this.busy = false;
        this.ac = null;
        if (this.batches.length) this.flush();
      })
      .catch((err) => this.broken(err));
    // The batch is on its way (or, if encoding or fetch threw, lost: the
    // lane fails right after). Only now may Go learn the frames' seq: a
    // boundary for a frame still queued here would start Go's gap timer
    // while the page (a stalled main thread) cannot send it.
    for (const [stream, seq] of sent) ctx.dispatched(stream, seq);
  }

  private broken(err: unknown): void {
    if (this.dead) return;
    this.dead = true;
    this.release();
    this.ctx?.fail(err);
  }

  /** The deadline of the POST in flight. A timer that fires more than
   * POST_LATE_MS after its deadline found the main thread stalled (the
   * response may be waiting behind it): it re-arms once with the full
   * timeout instead of failing the lane; the second expiry fails it. */
  private armPostTimer(rearmed: boolean, expired?: () => void): void {
    const due = Date.now() + this.timeoutMs;
    this.postTimer = setTimeout(() => {
      this.postTimer = null;
      if (!rearmed && Date.now() - due > POST_LATE_MS) return this.armPostTimer(true, expired);
      if (expired) expired();
      else this.broken(new Error(`up:post: timeout after ${this.timeoutMs} ms`));
    }, this.timeoutMs);
  }

  private clearPostTimer(): void {
    if (this.postTimer !== null) clearTimeout(this.postTimer);
    this.postTimer = null;
  }

  private release(): void {
    this.clearPostTimer();
    this.retained = 0;
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.batches = [];
    this.busy = false;
    const ac = this.ac;
    this.ac = null;
    if (ac) {
      try {
        ac.abort();
      } catch {}
    }
  }
}

/** An up:post module (registered as "up:post"; tests build their own with
 * options). */
export function newPostUp(opts?: PostUpOptions): UpLane {
  const d = opts?.batchDelayMs;
  const t = opts?.postTimeoutMs;
  return new PostUp(typeof d === "number" && d >= 0 ? d : POST_BATCH_DELAY_MS,
    typeof t === "number" && t >= 0 ? t : POST_TIMEOUT_MS);
}

registerLane("down:scheme", () => newSchemeDown());
registerLane("up:post", () => newPostUp());
