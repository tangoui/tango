// Tango IPC client. Implements the wire protocol described in
// docs/architecture.md ("IPC 线上协议") on top of window.__tango.

import { TangoSocket } from "./socket.js";
import { decodePacket } from "./packet.js";
import { Lanes, parseOffer, type UpLane } from "./lanes.js";

/** The raw bridge installed by the Tango core shim as window.__tango. */
export interface Bridge {
  post(msg: string): void;
  /** Subscribes to Go→JS raw messages; returns an unsubscribe function. */
  onMessage(fn: (msg: string) => void): () => void;
}

/** Error codes used by Tango. Apps may use their own codes too.
 * - not_found: no such method (or the caller is not a Tango window)
 * - bad_args: wrong arguments
 * - internal: the Go side failed
 * - canceled: the user dismissed a dialog, or the call/stream was canceled
 * - forbidden: the capability is not enabled in jsapi.Options (runtime API)
 * - unsupported: the platform cannot do it
 * - not_running: the app is quitting
 * - busy: an update check or download is already running (updater methods) */
export type ErrorCode =
  | "not_found"
  | "bad_args"
  | "internal"
  | "canceled"
  | "forbidden"
  | "unsupported"
  | "not_running"
  | "busy"
  | (string & {});

/** Error raised for failed calls and streams; see ErrorCode. */
export class TangoError extends Error {
  readonly code: ErrorCode;
  constructor(message: string, code: ErrorCode = "internal") {
    super(message);
    this.name = "TangoError";
    this.code = code;
  }
}

/** A stream between the page and Go. Iterate with for await: JSON frames
 * yield their decoded value, byte frames a Uint8Array. Breaking out of the
 * loop or calling cancel() cancels it on the Go side.
 *
 * send/sendBytes queue a frame in call order and resolve once it was
 * written to the transport: posted on the message channel, or taken by a
 * fast lane (which may still hold it briefly before it goes on the wire;
 * its credit is charged either way, and end() waits until it went). A
 * frame waits while Go's receive window (in bytes) is full. They reject
 * with bad_args for a payload larger than
 * MAX_FRAME_BYTES (the stream stays usable), and reject queued and later
 * frames once the stream can no longer send: after end() ("stream closed",
 * canceled), when Go does not accept data (bad_args), when Go closed
 * ("stream closed by peer", canceled), failed (Go's error) or the stream
 * was canceled. Not awaiting them never causes unhandled rejections. */
export interface TangoStream<T = unknown> extends AsyncIterable<T | Uint8Array> {
  readonly id: number;
  /** Resolves when the receiving side accepted the stream; in tells whether
   * it receives data. Rejects if the stream fails (e.g. not_found) or is
   * canceled first. */
  readonly accepted: Promise<{ in: boolean }>;
  /** Sends v as a JSON frame (undefined is sent as null). The value is
   * encoded at call time. */
  send(v: unknown): Promise<void>;
  /** Sends a byte frame. The bytes are copied at call time. */
  sendBytes(b: Uint8Array | ArrayBuffer | ArrayBufferView): Promise<void>;
  /** Ends the send direction once every queued frame was written. Later
   * send calls reject; receiving goes on. Idempotent. */
  end(): void;
  /** Charged bytes (payload + FRAME_OVERHEAD per frame) of queued frames
   * not yet written to the transport. */
  readonly bufferedAmount: number;
  /** The receive direction as a ReadableStream (created on first access).
   * It pulls one frame per read, so credit goes back to Go only as frames
   * are read; cancel() cancels the stream. The readable and the async
   * iterator share one receive queue: once one is used, the other throws
   * TangoError("stream already consumed", "bad_args"). */
  readonly readable: ReadableStream<T | Uint8Array>;
  /** The send direction as a WritableStream (created on first access).
   * write() sends bytes (Uint8Array, ArrayBuffer, other ArrayBufferView)
   * with sendBytes and anything else with send, and resolves once Go's
   * window lets the frame go, so pipeTo follows Go's credit. A chunk that
   * cannot be sent (bad_args: not JSON, over MAX_FRAME_BYTES, or Go does
   * not accept data) errors the writable and fails the stream (Go sees the
   * error). close() ends
   * the send direction after every queued frame was written; abort(reason)
   * fails the stream with code canceled and tells Go. */
  readonly writable: WritableStream<unknown>;
  cancel(): void;
}

export type StreamHandler = (stream: TangoStream<any>, ...args: any[]) => void;

export interface Client {
  call<T = unknown>(method: string, ...args: unknown[]): Promise<T>;
  on(name: string, fn: (data: any) => void): () => void;
  emit(name: string, data?: unknown): void;
  stream<T = unknown>(method: string, ...args: unknown[]): TangoStream<T>;
  /** Opens a stream served by Router.HandleDuplex (or HandleStream) as a
   * WebSocket-like TangoSocket. */
  socket(method: string, ...args: unknown[]): TangoSocket;
  /** Handles streams Go opens with Router.OpenStream(conn, name, ...). */
  onStream(name: string, fn: StreamHandler): () => void;
  /** Subscribes to non-ipc raw messages. Messages that arrive before the
   * first subscriber are buffered and delivered to it. */
  onRaw(fn: (msg: string) => void): () => void;
  postRaw(msg: string): void;
  /** The fast lanes in use: each direction's ready lane name, or
   * "fallback" when its byte frames go on the message channel. */
  lanes(): { down: string; up: string };
}

/** Options of createClient. */
export interface ClientOptions {
  /** How long a stream waits for a missing frame before it fails with
   * code transport (default GAP_TIMEOUT_MS). */
  gapTimeoutMs?: number;
}

/** Wire protocol version carried in "hello" (Go: ipc.ProtoVersion). */
export const PROTO_VERSION = 2;
/** Default stream receive window in bytes (Go: ipc.DefaultWindowBytes). */
export const DEFAULT_WINDOW_BYTES = 1048576;
/** Bytes charged per stream frame on top of its payload (Go: ipc.FrameOverhead). */
export const FRAME_OVERHEAD = 64;
/** Largest raw payload of one stream frame (Go: ipc.MaxFrameBytes). */
export const MAX_FRAME_BYTES = 16777216;
/** How long, in milliseconds, a stream waits for a frame that is missing
 * while later frames (or Go's end counting it) already arrived, before it
 * fails with code transport (Go: ipc.GapTimeout). Only fast lanes can
 * lose frames. */
export const GAP_TIMEOUT_MS = 5000;
/** A gap timer firing later than this after its deadline means the main
 * thread was stalled: it re-arms once instead of failing (updateGap). */
const GAP_LATE_MS = 1000;

const RAW_BUFFER_MAX = 1000;

interface WireError {
  message?: string;
  code?: string;
}

interface Wire {
  tango: string;
  id?: number;
  method?: string;
  args?: unknown[];
  result?: unknown;
  error?: WireError;
  name?: string;
  data?: unknown;
  op?: string;
  proto?: number;
  win?: number; // hello: Go's stream receive window in bytes
  seq?: number; // data: frame number; end/close/error: frames sent
  c?: number; // data: charged size in bytes
  bin?: string; // data: byte payload, standard base64 ("" is an empty frame)
  b?: number; // ack: credit returned in bytes
  in?: boolean; // accept: whether the accepting side receives data
  lanes?: unknown; // hello: the fast lanes offered (internal/lane)
}

function toError(e: WireError | undefined): TangoError {
  return new TangoError(e?.message ?? "unknown error", e?.code || "internal");
}

function report(err: unknown): void {
  // Surface listener exceptions without breaking message dispatch.
  (globalThis as any).setTimeout(() => {
    throw err;
  }, 0);
}

/** Decodes standard base64; throws on invalid input. */
function decodeBase64(s: string): Uint8Array {
  const from = (Uint8Array as any).fromBase64;
  if (typeof from === "function") return from.call(Uint8Array, s);
  const bin: string = (globalThis as any).atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** Encodes standard base64 (with padding). */
function encodeBase64(u: Uint8Array): string {
  const to = (u as any).toBase64;
  if (typeof to === "function") return to.call(u);
  let s = "";
  for (let i = 0; i < u.length; i += 0x8000) {
    s += String.fromCharCode.apply(null, u.subarray(i, i + 0x8000) as unknown as number[]);
  }
  return (globalThis as any).btoa(s);
}

const tag = (v: unknown) => Object.prototype.toString.call(v);

/** Copies the bytes of a Uint8Array, ArrayBuffer or other ArrayBufferView;
 * null for anything else. */
export function copyBytes(b: unknown): Uint8Array | null {
  if (ArrayBuffer.isView(b)) return new Uint8Array(b.buffer, b.byteOffset, b.byteLength).slice();
  const t = tag(b);
  if (t === "[object ArrayBuffer]" || t === "[object SharedArrayBuffer]") {
    return new Uint8Array(b as ArrayBuffer).slice();
  }
  return null;
}

/** Whether a writable chunk (or socket message) is sent as a byte frame. */
export const isBytes = (v: unknown): v is ArrayBuffer | ArrayBufferView =>
  ArrayBuffer.isView(v) || tag(v) === "[object ArrayBuffer]" || tag(v) === "[object SharedArrayBuffer]";

let utf8: { encode(s: string): Uint8Array } | null = null;
const utf8Length = (s: string) => (utf8 ??= new (globalThis as any).TextEncoder()).encode(s).length;

/** Marks p handled so callers that never await it see no unhandled
 * rejection; awaiting callers still get the rejection. */
function quiet<T>(p: Promise<T>): Promise<T> {
  p.catch(() => {});
  return p;
}

const tooLarge = (n: number) =>
  new TangoError(`stream frame payload of ${n} bytes exceeds MAX_FRAME_BYTES (${MAX_FRAME_BYTES})`, "bad_args");

/** One queued outgoing message: a JSON data frame (body is its "data"
 * member, already encoded), a byte frame (bytes, base64-encoded only if it
 * goes on the message channel) or a marker (neither, c 0): end, or close
 * when close is set. */
interface Out {
  body: string | null;
  bytes?: Uint8Array;
  c: number;
  close?: boolean;
  resolve(): void;
  reject(e: unknown): void;
}

/** Parses msg if it is an ipc message (JSON object with a "tango" field). */
function parse(msg: string): Wire | null {
  let i = 0;
  while (i < msg.length && " \t\r\n".includes(msg[i])) i++;
  if (msg[i] !== "{") return null;
  try {
    const m = JSON.parse(msg);
    if (m && typeof m === "object" && !Array.isArray(m) && "tango" in m) return m as Wire;
  } catch {
    // not JSON: raw message
  }
  return null;
}

type Result<T> = IteratorResult<T, undefined>;

/** One received data frame: its value (Uint8Array for byte frames) and its
 * charged size, returned as credit once the frame is read. */
interface Frame {
  v: unknown;
  c: number;
}

const isSeq = (n: unknown): n is number => typeof n === "number" && Number.isInteger(n) && n >= 0;

/** One stream, either side's. Receive direction: frames are reordered by
 * seq into queue and read by the iterator; the direction is over once Go's
 * end/close/error arrived together with every frame it counts (or the
 * stream was aborted). The stream is finished (forgotten by the client, so
 * late messages are ignored) once the receive direction is over and the
 * send direction is done too. */
class StreamImpl<T> implements TangoStream<T> {
  readonly accepted: Promise<{ in: boolean }>;
  private acceptSettled = false;
  /** The stream was accepted: Go's accept arrived (JS opened it), or JS
   * accepted it (Go opened it). Until then byte frames stay off the up
   * lane: a lane packet could reach Go before the open message. */
  private acceptSeen = false;
  private resolveAccept!: (v: { in: boolean }) => void;
  private rejectAccept!: (e: TangoError) => void;

  private queue: Frame[] = []; // in-order frames not yet read
  private pending = new Map<number, Frame>(); // out-of-order frames (seq > recvNext)
  private recvNext = 0; // seq of the next in-order frame
  private boundary = 0; // reliable exclusive upper bound of lane sends
  private endSeq: number | null = null; // Go's end/close/error: frames it sent
  private over = false; // receive direction over: no more frames will be queued
  private overErr: TangoError | null = null; // delivered to the reader once queue is drained
  private readDone = false; // the reader saw done (or the final error)
  private unacked = 0; // charged bytes read but not yet acked
  private waiters: { resolve(r: Result<T | Uint8Array>): void; reject(e: unknown): void }[] = [];

  /** Go sent close: it ended its direction and no longer receives. */
  peerClosed = false;
  /** Go sent error: the stream failed on its side. */
  private peerFailed = false;
  /** Go's error once Go sent error (internal: lets TangoSocket tell it
   * apart from a peer cancel or a client failure, whatever its code). */
  _peerError: TangoError | null = null;
  /** Resolves once the stream is finished: with the abort error if it was
   * aborted (peer cancel, client failure, protocol violation, cancel),
   * null otherwise (internal, for TangoSocket). */
  readonly _finished: Promise<TangoError | null>;
  private resolveFinished!: (err: TangoError | null) => void;
  /** The send direction (JS → Go) is terminal: JS's end was posted, or
   * accept{in:false}, Go's close/error/cancel or an abort stopped it. */
  sendDone = false;
  /** Data frames JS sent on this stream (the seq of end/close/error). */
  sent = 0;
  private outq: Out[] = []; // frames (and the end marker) waiting for credit
  private inflight = 0; // charged bytes sent but not yet acked by Go
  private buffered = 0; // charged bytes of queued data frames
  private ended = false; // end() was called
  private flushing = false; // flush() is running
  private sendErr: TangoError | null = null; // why sends fail once sendDone
  private endPosted: Promise<void> | null = null; // settles when end is posted (or can no longer be)
  private closeQueued = false; // _queueClose() was called
  private finished = false;
  /** seq + 1 of the last frame the up lane took (0: none). */
  private laneTaken = 0;
  /** seq + 1 of the last frame the up lane dispatched (or lost): the
   * boundary Go has (or is about to get). end/close wait while it is
   * below laneTaken. */
  private laneDone = 0;
  /** A dispatch arrived while flush() ran: flush posts the boundary. */
  private boundaryDue = false;
  private gapTimer: unknown = null; // armed while a frame is missing (updateGap)
  /** Which view reads the receive queue; the other one then throws. */
  private consumer: "iterator" | "readable" | null = null;
  private rs: ReadableStream<T | Uint8Array> | null = null;
  private ws: WritableStream<unknown> | null = null;

  constructor(
    readonly id: number,
    private readonly c: ClientImpl,
  ) {
    this.accepted = new Promise((resolve, reject) => {
      this.resolveAccept = resolve;
      this.rejectAccept = reject;
    });
    // Callers that never look at accepted must not see unhandled rejections.
    this.accepted.catch(() => {});
    this._finished = new Promise((resolve) => (this.resolveFinished = resolve));
  }

  _accept(inn: boolean): void {
    if (this.finished) return;
    this.acceptSeen = true;
    this.settleAccept({ in: inn });
    if (!inn) this.stopSending(new TangoError("stream does not accept data", "bad_args"));
    this.maybeFinish();
  }

  private settleAccept(v: { in: boolean } | TangoError): void {
    if (this.acceptSettled) return;
    this.acceptSettled = true;
    if (v instanceof TangoError) this.rejectAccept(v);
    else this.resolveAccept(v);
  }

  _frame(m: Wire): void {
    if (this.finished) return;
    const hasData = "data" in m;
    const hasBin = "bin" in m;
    if (hasData === hasBin) return this.fail("stream data must have exactly one of data and bin");
    if (typeof m.c !== "number" || !(m.c >= FRAME_OVERHEAD)) return this.fail("stream data has missing or invalid c");
    const seq = m.seq;
    if (!isSeq(seq)) return this.fail("stream data has missing or invalid seq");
    if (seq < this.recvNext || this.pending.has(seq)) return this.fail("duplicate frame");
    if (this.endSeq !== null && seq >= this.endSeq) return this.fail("frame after end");
    let v: unknown = m.data;
    if (hasBin) {
      try {
        if (typeof m.bin !== "string") throw new TypeError("bin is not a string");
        v = decodeBase64(m.bin);
      } catch {
        return this.fail("stream data has invalid base64 in bin");
      }
    }
    this.accept(seq, { v, c: m.c });
  }

  /** A byte frame from a fast lane packet (charged len + FRAME_OVERHEAD). */
  _data(seq: number, payload: Uint8Array): void {
    if (this.finished) return;
    if (seq < this.recvNext || this.pending.has(seq)) return this.fail("duplicate frame");
    if (this.endSeq !== null && seq >= this.endSeq) return this.fail("frame after end");
    this.accept(seq, { v: payload, c: payload.length + FRAME_OVERHEAD });
  }

  /** Queues a checked frame in seq order. */
  private accept(seq: number, f: Frame): void {
    if (seq !== this.recvNext) {
      this.pending.set(seq, f);
      this.updateGap(false);
      return;
    }
    this.queue.push(f);
    this.recvNext++;
    for (let g = this.pending.get(this.recvNext); g; g = this.pending.get(this.recvNext)) {
      this.pending.delete(this.recvNext);
      this.queue.push(g);
      this.recvNext++;
    }
    this.checkOver();
    this.updateGap(true);
    this.pump();
  }

  /** Whether a frame is missing: frames beyond recvNext are pending, or
   * Go's end/close/error counts frames not here yet (Go: gapLocked). */
  _boundary(n: unknown): void {
    if (typeof n !== "number" || !Number.isSafeInteger(n) || n < 0) return;
    this.boundary = Math.max(this.boundary, n);
    this.updateGap(false);
  }

  private gap(): boolean {
    return !this.finished && !this.over && (this.recvNext < this.boundary || this.pending.size > 0 || (this.endSeq !== null && this.recvNext < this.endSeq));
  }

  /** Re-evaluates the gap timer after the receive state changed; progress:
   * recvNext advanced. No gap: the timer stops. A new gap, or one left
   * after progress: it (re)starts. A gap that only grew keeps its deadline
   * (Go: updateGapLocked). A timer that fires more than GAP_LATE_MS after
   * its deadline found the page's main thread stalled (the missing frame
   * may be sitting unread in a lane's buffer): it re-arms once with the
   * full timeout instead of failing. Go has no such rule: its timers do
   * not stall with the page. */
  private updateGap(progress: boolean): void {
    if (this.recvNext >= this.boundary) this.boundary = 0;
    if (!this.gap()) return this.stopGap();
    if (this.gapTimer !== null && !progress) return;
    this.stopGap();
    this.armGap(false);
  }

  private armGap(rearmed: boolean): void {
    const ms = this.c.gapTimeoutMs;
    const due = Date.now() + ms;
    this.gapTimer = (globalThis as any).setTimeout(() => {
      this.gapTimer = null;
      if (!this.gap()) return;
      if (!rearmed && Date.now() - due > GAP_LATE_MS) return this.armGap(true);
      this._close(new TangoError("frames lost on a fast lane", "transport"));
    }, ms);
  }

  private stopGap(): void {
    if (this.gapTimer === null) return;
    (globalThis as any).clearTimeout(this.gapTimer);
    this.gapTimer = null;
  }

  /** Go ended its direction: op is end, close or error; seq counts the
   * frames it sent (missing: finish with what arrived in order). */
  _end(op: string, seq: unknown, err: TangoError | null): void {
    if (this.finished) return;
    if (this.endSeq === null) {
      let n: number;
      if (isSeq(seq)) {
        n = seq;
        let bad = this.recvNext > n;
        for (const k of this.pending.keys()) bad = bad || k >= n;
        if (bad) return this.fail("frame after end");
      } else {
        n = this.recvNext;
        this.pending.clear();
      }
      this.endSeq = n;
      this.checkOver();
      this.updateGap(false); // frames before n may be missing
    }
    // A later close or error (after end) still counts for its own meaning.
    if (op === "close") {
      this.peerClosed = true;
      this.stopSending(new TangoError("stream closed by peer", "canceled"));
    } else if (op === "error") {
      this.peerFailed = true;
      this._peerError ??= err ?? new TangoError("stream failed", "internal");
      this.stopSending(this._peerError);
      if (!this.overErr && !this.readDone) this.overErr = err;
      if (err) this.settleAccept(err);
    }
    this.checkOver();
    this.pump();
    this.maybeFinish();
  }

  /** Go canceled the stream: unread data is dropped. */
  _peerCancel(): void {
    this.abort(new TangoError("stream canceled by peer", "canceled"));
  }

  /** The client failed (protocol mismatch). */
  _clientFailed(err: TangoError): void {
    this.abort(err);
  }

  private checkOver(): void {
    if (!this.over && this.endSeq !== null && this.recvNext >= this.endSeq) {
      this.over = true;
      this.pending.clear();
    }
  }

  /** Hands queued frames to waiting readers, then the end to the rest. */
  private pump(): void {
    while (this.waiters.length && this.queue.length) {
      const w = this.waiters.shift()!;
      const f = this.queue.shift()!;
      w.resolve({ value: f.v as T | Uint8Array, done: false });
      this.credit(f.c);
    }
    if (this.over && !this.queue.length) this.settleWaiters();
    this.maybeFinish();
  }

  /** Ends the reader: the first waiter (or next call) gets overErr, if any;
   * everyone else gets done. */
  private settleWaiters(): void {
    for (const w of this.waiters.splice(0)) {
      const e = this.readDone ? null : this.overErr;
      this.readDone = true;
      this.overErr = null;
      if (e) w.reject(e);
      else w.resolve({ value: undefined, done: true });
    }
  }

  /** Returns a read frame's credit: acks once half the window accumulated,
   * or as soon as the reader caught up. Once Go ended its direction the
   * credit is useless and is not sent. */
  private credit(c: number): void {
    this.unacked += c;
    if (this.finished || this.endSeq !== null) return;
    if (this.unacked < DEFAULT_WINDOW_BYTES / 2 && this.queue.length) return;
    const b = this.unacked;
    this.unacked = 0;
    this.c._post({ tango: "stream", id: this.id, op: "ack", b });
  }

  /** Fails the stream on a protocol violation by Go: tells Go, then aborts. */
  private fail(message: string): void {
    const err = new TangoError(message, "bad_args");
    // Go stopped listening after its close or error: nothing to tell it.
    if (!this.peerClosed && !this.peerFailed) {
      this.c._post({ tango: "stream", id: this.id, op: "error", seq: this.sent, error: { message, code: "bad_args" } });
    }
    this.abort(err);
  }

  /** Ends both directions at once and drops unread data. err (if any) goes to
   * the reader; null ends it quietly. Sends nothing. */
  private abort(err: TangoError | null): void {
    this.queue = [];
    this.pending.clear();
    this.over = true;
    this.stopSending(err ?? new TangoError("stream closed", "canceled"));
    if (!this.readDone) {
      this.overErr = err;
      if (!err) this.readDone = true;
    }
    this.settleWaiters();
    this.finish(err);
  }

  private maybeFinish(): void {
    if (this.over && this.sendDone) this.finish(null);
  }

  private finish(err: TangoError | null): void {
    if (this.finished) return;
    this.finished = true;
    this.stopGap();
    this.c._forget(this.id);
    this.settleAccept(err ?? new TangoError("stream closed", "canceled"));
    this.resolveFinished(err);
  }

  get bufferedAmount(): number {
    return this.buffered;
  }

  /** Why a send called now fails, or null if it can be queued. */
  private sendState(): TangoError | null {
    if (this.ended) return new TangoError("stream closed", "canceled");
    if (this.sendDone) return this.sendErr ?? new TangoError("stream closed", "canceled");
    return null;
  }

  send(v: unknown): Promise<void> {
    const bad = this.sendState();
    if (bad) return quiet(Promise.reject(bad));
    let json: string | undefined;
    try {
      json = JSON.stringify(v === undefined ? null : v);
    } catch (e) {
      return quiet(Promise.reject(new TangoError("stream data is not JSON: " + String((e as any)?.message ?? e), "bad_args")));
    }
    if (typeof json !== "string") {
      return quiet(Promise.reject(new TangoError("stream data is not JSON-serializable", "bad_args")));
    }
    // Every UTF-16 unit takes at least one UTF-8 byte: skip encoding huge strings.
    const n = json.length > MAX_FRAME_BYTES ? json.length : utf8Length(json);
    if (n > MAX_FRAME_BYTES) return quiet(Promise.reject(tooLarge(n)));
    return this.enqueue('"data":' + json, n + FRAME_OVERHEAD);
  }

  sendBytes(b: Uint8Array | ArrayBuffer | ArrayBufferView): Promise<void> {
    const bad = this.sendState();
    if (bad) return quiet(Promise.reject(bad));
    const n: unknown = (b as any)?.byteLength; // checked before copying a huge buffer
    if (typeof n === "number" && n > MAX_FRAME_BYTES) return quiet(Promise.reject(tooLarge(n)));
    const u = copyBytes(b);
    if (!u) {
      return quiet(Promise.reject(new TangoError("sendBytes needs a Uint8Array, ArrayBuffer or ArrayBufferView", "bad_args")));
    }
    return this.enqueue(null, u.length + FRAME_OVERHEAD, u);
  }

  end(): void {
    if (this.ended || this.sendDone) return;
    this.queueEnd();
  }

  /** Queues the end marker (once) and resolves when it was posted; rejects
   * when the send direction stops first, or already stopped for another
   * reason. */
  private queueEnd(): Promise<void> {
    if (this.endPosted) return this.endPosted;
    if (this.sendDone) return quiet(Promise.reject(this.sendErr ?? new TangoError("stream closed", "canceled")));
    this.ended = true;
    this.endPosted = quiet(
      new Promise<void>((resolve, reject) => {
        this.outq.push({ body: null, c: 0, resolve, reject });
      }),
    );
    this.flush();
    return this.endPosted;
  }

  private enqueue(body: string | null, c: number, bytes?: Uint8Array): Promise<void> {
    return quiet(
      new Promise<void>((resolve, reject) => {
        this.outq.push({ body, bytes, c, resolve, reject });
        this.buffered += c;
        this.flush();
      }),
    );
  }

  /** Go returned b bytes of credit. */
  _ack(b: unknown): void {
    if (typeof b !== "number" || !(b > 0)) return;
    this.inflight = Math.max(0, this.inflight - b);
    this.flush();
  }

  /** Writes queued frames while Go's window allows (one frame always goes
   * when nothing is in flight), then the end marker once it is first. */
  _flush(): void {
    this.flush();
  }

  private flush(): void {
    // A bridge that delivers Go's reply synchronously re-enters through
    // _ack; the running loop picks the new credit up.
    if (this.flushing) return;
    this.flushing = true;
    try {
      while (this.outq.length && !this.sendDone) {
        if (this.boundaryDue && !this.postBoundary(null)) return;
        const o = this.outq[0];
        if (o.body === null && !o.bytes) {
          // end/close count every frame sent; Go must not wait (gap timer)
          // for a lane frame still queued in the page. _laneDispatched
          // flushes again.
          if (this.laneDone < this.laneTaken) break;
          this.outq.shift();
          // Set first: the send direction is done once the marker is posted.
          this.sendDone = true;
          if (o.close) {
            o.resolve();
            this._close(); // posts close and finishes the stream
            return;
          }
          try {
            this.c._post({ tango: "stream", id: this.id, op: "end", seq: this.sent });
          } catch (e) {
            // Not posted: the caller (writable close) sees why; the stream
            // still finishes once the receive direction is over.
            o.reject(e);
            this.maybeFinish();
            return;
          }
          o.resolve();
          this.maybeFinish();
          return;
        }
        if (this.inflight > 0 && this.inflight + o.c > this.c.sendWindow) return;
        this.outq.shift();
        this.buffered -= o.c;
        // Charge before posting: a bridge that delivers Go's ack
        // synchronously must find the frame in flight.
        this.inflight += o.c;
        if (o.bytes && this.toLane(o.bytes)) {
          this.sent++;
          // The lane may have dispatched the frame from inside send.
          if (this.boundaryDue && !this.postBoundary(o)) return;
          o.resolve();
          continue;
        }
        if (this.laneDone < this.laneTaken) {
          // A message-channel frame with seq = sent would show Go a later
          // seq while an earlier lane frame is still queued in the page:
          // Go's gap timer would start, and a stalled page could fail the
          // stream. Put it back (nothing was taken or posted) until
          // _laneDispatched flushes again; lane frames may still pass,
          // the lane keeps their order.
          this.inflight -= o.c;
          this.buffered += o.c;
          this.outq.unshift(o);
          break;
        }
        try {
          const body = o.bytes ? '"bin":"' + encodeBase64(o.bytes) + '"' : o.body;
          this.c._postRaw(`{"tango":"stream","id":${this.id},"op":"data","seq":${this.sent},"c":${o.c},${body}}`);
        } catch (e) {
          this.inflight -= o.c; // not written: no credit used, seq stays unused
          o.reject(e);
          continue;
        }
        this.sent++;
        o.resolve();
      }
      if (this.boundaryDue && !this.finished) this.postBoundary(null);
    } finally {
      this.flushing = false;
    }
  }

  /** The up lane put this stream's frame seq on the wire (LaneContext.
   * dispatched). Posts the boundary now (or lets the running flush post
   * it), then flushes a held end/close. Ignored once the stream finished. */
  _laneDispatched(seq: number): void {
    if (this.finished || !(seq + 1 > this.laneDone)) return;
    this.laneDone = seq + 1;
    this.boundaryDue = true;
    if (this.flushing) return;
    if (this.postBoundary(null)) this.flush();
  }

  /** The up lane stopped: the frames it took and never dispatched are
   * lost. Their boundary lets Go detect that (transport after its gap
   * timeout), and a held end/close goes. */
  _laneLost(): void {
    if (this.laneDone < this.laneTaken) this._laneDispatched(this.laneTaken - 1);
  }

  /** Posts the boundary of the dispatched lane frames. If the bridge
   * throws, the frames keep their seq and credit (never replayed): o, a
   * send already taken off the queue, rejects with the bridge error, and
   * the stream fails with transport (rejecting the rest of the queue).
   * false: the stream is finished. */
  private postBoundary(o: Out | null): boolean {
    this.boundaryDue = false;
    try {
      this.c._post({ tango: "stream", id: this.id, op: "boundary", seq: this.laneDone });
    } catch (e) {
      o?.reject(e);
      this._close(new TangoError(String((e as any)?.message ?? e), "transport"));
      return false;
    }
    return true;
  }

  /** Hands the next byte frame (seq = sent) to the ready upstream lane if
   * it is large enough; false: it goes on the message channel. */
  private toLane(payload: Uint8Array): boolean {
    const lane: UpLane | null = this.c._upLane();
    if (!lane || !this.acceptSeen || !(payload.length >= lane.minSize)) return false;
    try {
      if (lane.send({ stream: this.id, seq: this.sent, payload }) !== true) return false;
    } catch (e) {
      report(e);
      return false;
    }
    this.laneTaken = this.sent + 1;
    return true;
  }

  /** Stops the send direction for err: queued frames reject with it, and so
   * do later sends. No-op once the direction is already done. */
  private stopSending(err: TangoError): void {
    if (this.sendDone) return;
    this.sendDone = true;
    this.sendErr = err;
    const q = this.outq;
    this.outq = [];
    this.buffered = 0;
    for (const o of q) o.reject(err);
  }

  /** Closes the stream from JS (for writable/socket close and abort paths):
   * finishes locally, then posts close (or error, if err is given) with
   * seq = frames written, without waiting for Go. Queued and later sends
   * reject (canceled "stream closed", or err); unread data is dropped and
   * the reader ends (done, or err). */
  _close(err?: TangoError): void {
    if (this.finished) return;
    // Capture the notification before finishing: Go stopped listening after
    // its close/error, but forgetting this stream must not suppress ours.
    const notify = !this.peerClosed && !this.peerFailed;
    const id = this.id, seq = this.sent;
    // A bridge may reply synchronously to the notification. Commit our
    // terminal state first so that reply cannot replace the local outcome.
    this.abort(err ?? null);
    if (notify) {
      try {
        if (err) {
          this.c._post({ tango: "stream", id, op: "error", seq, error: { message: err.message, code: err.code } });
        } else {
          this.c._post({ tango: "stream", id, op: "close", seq });
        }
      } catch (e) {
        report(e); // the bridge failed: the stream is already finished
      }
    }
  }

  /** Closes the stream after every queued frame was written (TangoSocket's
   * close()): queues the close marker, and once it is posted (seq = frames
   * sent) the stream is finished at once, as by _close(), without waiting
   * for Go. If the send direction is already done (e.g. Go accepted with
   * in:false, or end was posted), close is posted now. Later sends reject;
   * unread data is dropped. */
  _queueClose(): void {
    if (this.finished || this.closeQueued) return;
    this.closeQueued = true;
    if (this.sendDone) return this._close();
    // If the send direction stops before the marker is posted (Go's
    // accept{in:false}, close or error), close then: the socket no longer
    // receives either. Deferred, so an abort that stopped sending finishes
    // with its own error first (and _close is then a no-op).
    const closeLater = () => void Promise.resolve().then(() => this._close());
    if (this.ended) {
      // end is queued (always last): turn it into close.
      const o = this.outq[this.outq.length - 1];
      const reject = o.reject;
      o.close = true;
      o.reject = (e) => {
        reject(e);
        closeLater();
      };
      return;
    }
    this.ended = true;
    this.outq.push({ body: null, c: 0, close: true, resolve: () => {}, reject: closeLater });
    this.flush();
  }

  /** Reads the next frame; reading returns its credit. */
  private read(): Promise<Result<T | Uint8Array>> {
    const f = this.queue.shift();
    if (f) {
      this.credit(f.c);
      return Promise.resolve({ value: f.v as T | Uint8Array, done: false });
    }
    if (this.readDone) return Promise.resolve({ value: undefined, done: true });
    if (this.over) {
      this.readDone = true;
      const e = this.overErr;
      this.overErr = null;
      return e ? Promise.reject(e) : Promise.resolve({ value: undefined, done: true });
    }
    return new Promise((resolve, reject) => this.waiters.push({ resolve, reject }));
  }

  cancel(): void {
    if (!this.finished) this.c._post({ tango: "stream", id: this.id, op: "cancel" });
    this.readDone = true;
    this.abort(new TangoError("stream canceled", "canceled"));
  }

  /** Locks the receive queue to one view (the iterator may be taken again). */
  private consume(as: "iterator" | "readable"): void {
    if (this.consumer && this.consumer !== as) throw new TangoError("stream already consumed", "bad_args");
    this.consumer = as;
  }

  [Symbol.asyncIterator](): AsyncIterator<T | Uint8Array, undefined> {
    this.consume("iterator");
    return {
      next: () => this.read(),
      return: () => {
        this.cancel();
        return Promise.resolve({ value: undefined, done: true });
      },
    };
  }

  get readable(): ReadableStream<T | Uint8Array> {
    if (this.rs) return this.rs;
    this.consume("readable");
    let canceled = false;
    this.rs = new ReadableStream<T | Uint8Array>(
      {
        // highWaterMark 0: pull runs only for a pending read, one frame each.
        pull: async (ctrl) => {
          const r = await this.read(); // a rejection errors the readable
          if (canceled) return;
          if (r.done) ctrl.close();
          else ctrl.enqueue(r.value);
        },
        cancel: () => {
          canceled = true;
          this.cancel();
        },
      },
      { highWaterMark: 0 },
    );
    return this.rs;
  }

  get writable(): WritableStream<unknown> {
    if (this.ws) return this.ws;
    const abort = (reason: unknown) => {
      this._close(new TangoError(String((reason as any)?.message ?? reason), "canceled"));
    };
    this.ws = new WritableStream<unknown>(
      {
        start: (ctrl) => {
          // The sink's abort runs only after a pending write settles, and a
          // write waiting for credit may never settle: act on the signal.
          const sig = (ctrl as any).signal as AbortSignal | undefined;
          sig?.addEventListener("abort", () => abort(sig.reason));
        },
        write: (chunk) =>
          (isBytes(chunk) ? this.sendBytes(chunk) : this.send(chunk)).catch((e) => {
            // A chunk this side refused (not JSON, over MAX_FRAME_BYTES, Go
            // does not accept data) errors the writable; fail the whole
            // stream too, so Go is not left waiting for frames or an end.
            if (e instanceof TangoError && e.code === "bad_args" && e !== this._peerError) this._close(e);
            throw e;
          }),
        close: () => this.queueEnd(),
        abort, // no-op once the signal handled it: the stream is finished
      },
      { highWaterMark: 1 },
    );
    return this.ws;
  }
}

class ClientImpl implements Client {
  private nextId = -1; // JS allocates odd ids
  private pending = new Map<number, { resolve(v: any): void; reject(e: unknown): void }>();
  private streams = new Map<number, StreamImpl<unknown>>();
  private events = new Map<string, Set<(data: any) => void>>();
  private streamHandlers = new Map<string, StreamHandler>();
  private raw = new Set<(msg: string) => void>();
  private rawBuf: string[] | null = [];
  /** Set when Go rejected this runtime's hello: calls and streams fail. */
  private failed: TangoError | null = null;
  /** Go's stream receive window (bytes JS may have in flight per stream);
   * DEFAULT_WINDOW_BYTES until Go's hello says otherwise. */
  sendWindow = DEFAULT_WINDOW_BYTES;
  readonly gapTimeoutMs: number;
  private readonly laneSet: Lanes;
  private badPacketWarned = false;

  constructor(
    private readonly bridge: Bridge,
    opts?: ClientOptions,
  ) {
    const g = opts?.gapTimeoutMs;
    this.gapTimeoutMs = typeof g === "number" && g >= 0 ? g : GAP_TIMEOUT_MS;
    this.laneSet = new Lanes({
      post: (m) => bridge.post(m),
      onPacket: (p) => this.receivePacket(p),
      onDispatched: (stream, seq) => {
        if (!this.failed) this.streams.get(stream)?._laneDispatched(seq);
      },
      onUpStopped: () => {
        if (this.failed) return;
        for (const s of [...this.streams.values()]) s._laneLost();
      },
    });
    bridge.onMessage((msg) => this.receive(msg));
    // Tell Go a new document started so state of the previous page is dropped.
    this._post({ tango: "hello", proto: PROTO_VERSION });
  }

  _post(m: Wire): void {
    this.bridge.post(JSON.stringify(m));
  }

  /** Posts an already encoded ipc message. */
  _postRaw(msg: string): void {
    this.bridge.post(msg);
  }

  /** The ready upstream lane, or null. */
  _upLane(): UpLane | null {
    return this.laneSet.up();
  }

  lanes(): { down: string; up: string } {
    return this.laneSet.names();
  }

  /** A downstream lane packet: its frames go to their streams (frames of
   * streams this page does not know, e.g. of the page before a reload, are
   * dropped). A corrupt packet is dropped; the first one is logged. */
  private receivePacket(p: Uint8Array): void {
    let frames;
    try {
      frames = decodePacket(p);
    } catch (e) {
      if (!this.badPacketWarned) {
        this.badPacketWarned = true;
        console.warn("tango: dropped a corrupt fast lane packet:", e);
      }
      return;
    }
    for (const f of frames) {
      // Copied: a payload view would keep the whole packet alive.
      this.streams.get(f.stream)?._data(f.seq, f.payload.slice());
    }
  }

  _forget(id: number): void {
    this.streams.delete(id);
  }

  private id(): number {
    this.nextId += 2;
    return this.nextId;
  }

  call<T = unknown>(method: string, ...args: unknown[]): Promise<T> {
    if (this.failed) return Promise.reject(this.failed);
    const id = this.id();
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try {
        this._post({ tango: "call", id, method, args });
      } catch (e) {
        this.pending.delete(id);
        reject(e);
      }
    });
  }

  on(name: string, fn: (data: any) => void): () => void {
    let set = this.events.get(name);
    if (!set) this.events.set(name, (set = new Set()));
    set.add(fn);
    return () => {
      set.delete(fn);
      if (!set.size && this.events.get(name) === set) this.events.delete(name);
    };
  }

  emit(name: string, data?: unknown): void {
    this._post({ tango: "event", name, data: data === undefined ? null : data });
  }

  stream<T = unknown>(method: string, ...args: unknown[]): TangoStream<T> {
    const s = new StreamImpl<T>(this.id(), this);
    if (this.failed) {
      s._clientFailed(this.failed);
      return s;
    }
    this.streams.set(s.id, s as StreamImpl<unknown>);
    this._post({ tango: "stream", id: s.id, op: "open", method, args });
    return s;
  }

  socket(method: string, ...args: unknown[]): TangoSocket {
    return new TangoSocket(method, () => this.stream(method, ...args));
  }

  onStream(name: string, fn: StreamHandler): () => void {
    this.streamHandlers.set(name, fn);
    return () => {
      if (this.streamHandlers.get(name) === fn) this.streamHandlers.delete(name);
    };
  }

  onRaw(fn: (msg: string) => void): () => void {
    this.raw.add(fn);
    const buf = this.rawBuf;
    if (buf) {
      this.rawBuf = null;
      for (const m of buf) this.dispatchRaw(m);
    }
    return () => {
      this.raw.delete(fn);
    };
  }

  postRaw(msg: string): void {
    this.bridge.post(msg);
  }

  private dispatchRaw(msg: string): void {
    for (const fn of [...this.raw]) {
      try {
        fn(msg);
      } catch (e) {
        report(e);
      }
    }
  }

  /** Enters the failed state once: pending and later calls and streams
   * reject with err, which is also reported. */
  private fail(err: TangoError): void {
    if (this.failed) return;
    this.failed = err;
    this.laneSet.stopAll();
    report(err);
    const calls = [...this.pending.values()];
    this.pending.clear();
    for (const p of calls) p.reject(err);
    for (const s of [...this.streams.values()]) s._clientFailed(err);
  }

  private receive(msg: string): void {
    const m = parse(msg);
    if (!m) {
      if (this.rawBuf) {
        if (this.rawBuf.length < RAW_BUFFER_MAX) this.rawBuf.push(msg);
      } else {
        this.dispatchRaw(msg);
      }
      return;
    }
    switch (m.tango) {
      case "hello":
        if (m.error || m.proto !== PROTO_VERSION) {
          this.fail(new TangoError(m.error?.message || "runtime-js protocol mismatch", "bad_args"));
          return;
        }
        // Every hello reply starts a new lane generation.
        if (!this.failed) this.laneSet.offer(parseOffer(m.lanes));
        if (typeof m.win === "number" && m.win > 0) {
          this.sendWindow = m.win;
          // A larger window may let queued frames go.
          for (const s of [...this.streams.values()]) s._flush();
        }
        return;
      case "ret": {
        const p = this.pending.get(m.id as number);
        if (!p) return;
        this.pending.delete(m.id as number);
        if (m.error) p.reject(toError(m.error));
        else p.resolve(m.result);
        return;
      }
      case "event": {
        const set = this.events.get(m.name as string);
        if (!set) return;
        for (const fn of [...set]) {
          try {
            fn(m.data);
          } catch (e) {
            report(e);
          }
        }
        return;
      }
      case "stream":
        this.receiveStream(m);
        return;
    }
  }

  private receiveStream(m: Wire): void {
    const id = m.id as number;
    if (m.op === "open") {
      if (this.failed || this.streams.has(id)) return;
      const fn = this.streamHandlers.get(m.method as string);
      if (!fn) {
        this._post({
          tango: "stream",
          id,
          op: "error",
          seq: 0,
          error: { message: "no stream handler: " + m.method, code: "not_found" },
        });
        return;
      }
      const s = new StreamImpl<unknown>(id, this);
      this.streams.set(id, s);
      // Accept before the handler runs (and before it can send).
      s._accept(true);
      this._post({ tango: "stream", id, op: "accept", in: true });
      try {
        fn(s, ...(Array.isArray(m.args) ? m.args : []));
      } catch (e) {
        s.cancel();
        report(e);
      }
      return;
    }
    const s = this.streams.get(id);
    if (!s) return; // finished or unknown stream: late messages are normal
    switch (m.op) {
      case "boundary":
        s._boundary(m.seq);
        return;
      case "data":
        s._frame(m);
        return;
      case "accept":
        // Only an explicit false refuses data (as on the Go side).
        s._accept(m.in !== false);
        return;
      case "end":
      case "close":
        s._end(m.op, m.seq, null);
        return;
      case "error":
        s._end("error", m.seq, toError(m.error));
        return;
      case "cancel":
        s._peerCancel();
        return;
      case "ack":
        s._ack(m.b);
        return;
    }
    // Unknown ops are ignored.
  }
}

/** Creates a client on an explicit bridge. Most code uses the module-level
 * functions, which share one client per window.__tango. */
export function createClient(bridge: Bridge, opts?: ClientOptions): Client {
  return new ClientImpl(bridge, opts);
}

const KEY = Symbol.for("tango.ipc.client");

/** Returns the shared client for window.__tango, creating it on first use. */
export function getClient(): Client {
  const g = globalThis as any;
  const b = g.__tango as Bridge | undefined;
  if (!b) throw new TangoError("window.__tango is missing: not running in a Tango surface", "not_found");
  // The core freezes window.__tango, so the shared client is cached on the
  // global object instead (keyed by a registry symbol so the ESM build and
  // the injected IIFE share one client).
  const cached = g[KEY] as { bridge: Bridge; client: Client } | undefined;
  if (cached && cached.bridge === b) return cached.client;
  const client = createClient(b);
  Object.defineProperty(g, KEY, { value: { bridge: b, client }, configurable: true });
  return client;
}
