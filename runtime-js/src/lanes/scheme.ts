// The up:post lane, served by Go's internal/lane (http.go) on the asset
// origin, so it works on every platform whose pages come from it:
//
//   up:post      POST /__tango/lane/up?gen=&tok=    one packet per
//                request, answered 204 (ServeUp). start posts the empty
//                packet [0,0] first and is ready only on its 204.
//
// (Its downstream sibling down:scheme is gone: down:poll, lanes/poll.ts,
// replaced it.) The URL is relative: Go offers the lane only to pages from
// the asset origin, where a relative path is tango://app/... or
// https://tango.localhost/... . It registers at module load, so the ESM
// entry and the injected IIFE have it before the client's hello reply.

import { registerLane, type LaneContext, type UpLane } from "../lanes.js";
import { encodePacket, type PacketFrame } from "../packet.js";

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

registerLane("up:post", () => newPostUp());
