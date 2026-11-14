// TangoSocket: a WebSocket-like EventTarget over a TangoStream, for code
// written against the WebSocket API.
//
//   const ws = tango.socket("Chat.Join", "lobby");   // Router.HandleDuplex in Go
//   ws.onmessage = (e) => console.log(e.data);
//   ws.onopen = () => ws.send("hi");

import { FRAME_OVERHEAD, TangoError, copyBytes, isBytes, type TangoStream } from "./client.js";

/** What TangoSocket uses beyond the public TangoStream (StreamImpl). */
interface Internal extends TangoStream<unknown> {
  readonly _peerError: TangoError | null;
  readonly _finished: Promise<TangoError | null>;
  _close(err?: TangoError): void;
  _queueClose(): void;
}

const g = globalThis as any;

// The module must load where EventTarget is missing (bare JS contexts);
// creating a socket there fails instead.
const Base: typeof EventTarget =
  g.EventTarget ??
  (class {
    constructor() {
      throw new TangoError("tango.socket needs EventTarget", "unsupported");
    }
  } as any);

const CONNECTING = 0;
const OPEN = 1;
const CLOSING = 2;
const CLOSED = 3;

/** Reports err without interrupting the caller, like an error thrown by an
 * event listener. */
function reportLater(err: unknown): void {
  g.setTimeout(() => {
    throw err;
  }, 0);
}

let FallbackMessageEvent: any = null;
let FallbackCloseEvent: any = null;

function messageEvent(data: unknown): MessageEvent {
  if (typeof g.MessageEvent === "function") return new g.MessageEvent("message", { data });
  FallbackMessageEvent ??= class MessageEvent extends Event {
    readonly data: unknown;
    constructor(type: string, init: { data?: unknown } = {}) {
      super(type);
      this.data = init.data;
    }
  };
  return new FallbackMessageEvent("message", { data });
}

function closeEvent(code: number, reason: string, wasClean: boolean): CloseEvent {
  if (typeof g.CloseEvent === "function") return new g.CloseEvent("close", { code, reason, wasClean });
  FallbackCloseEvent ??= class CloseEvent extends Event {
    readonly code: number;
    readonly reason: string;
    readonly wasClean: boolean;
    constructor(type: string, init: { code?: number; reason?: string; wasClean?: boolean } = {}) {
      super(type);
      this.code = init.code ?? 0;
      this.reason = init.reason ?? "";
      this.wasClean = init.wasClean ?? false;
    }
  };
  return new FallbackCloseEvent("close", { code, reason, wasClean });
}

/** A stream served by Router.HandleDuplex (or HandleStream), shaped like a
 * WebSocket; create it with socket(method, ...args). url is "tango:" +
 * method.
 *
 * - readyState: CONNECTING until Go accepts the stream (open event), OPEN,
 *   CLOSING after close() or once Go ended its side, CLOSED after the close
 *   event.
 * - message: data is the string for a JSON string value, JSON.stringify of
 *   any other JSON value, and an ArrayBuffer (or a Blob when binaryType is
 *   "blob") for a byte frame. Messages arriving after close() are dropped.
 * - send(): strings go as JSON string frames, ArrayBuffer/ArrayBufferView
 *   and Blob as byte frames, anything else as String(data), all in call
 *   order. Unlike WebSocket, send() while CONNECTING does not throw: the
 *   frame is queued. Sends after close() are ignored. A send rejected with
 *   bad_args (payload over MAX_FRAME_BYTES, or Go does not accept data)
 *   fails the socket. bufferedAmount counts charged bytes (payload +
 *   FRAME_OVERHEAD) not yet written.
 * - close(): closes the stream after queued sends: once they are written
 *   it sends close to Go (Go's Write* then fails with canceled "stream
 *   closed by peer"; its Receive* returns io.EOF after the frames sent) and
 *   the close event (1000, wasClean) follows at once, without waiting for
 *   Go. code and reason are not sent to Go. It works the same on a stream
 *   that does not accept data (HandleStream). When Go ends its side first,
 *   the socket answers with its own end and closes with 1000 too.
 * - Go's error: error event, then close with 1011 and Go's error message
 *   as reason (wasClean false). Go's cancel or a client failure (e.g. a
 *   protocol mismatch): close with 1006 (wasClean false), no error event. */
export class TangoSocket extends Base {
  static readonly CONNECTING = CONNECTING;
  static readonly OPEN = OPEN;
  static readonly CLOSING = CLOSING;
  static readonly CLOSED = CLOSED;
  readonly CONNECTING = CONNECTING;
  readonly OPEN = OPEN;
  readonly CLOSING = CLOSING;
  readonly CLOSED = CLOSED;

  readonly url: string;
  onopen: ((e: Event) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  onclose: ((e: CloseEvent) => void) | null = null;

  private state = CONNECTING;
  private type: "arraybuffer" | "blob" = "arraybuffer";
  private readonly s: Internal;
  /** Sends that wait for an earlier Blob to be read run on this chain. */
  private tail: Promise<void> = Promise.resolve();
  private waiting = 0; // chained steps not yet run
  private waitingBytes = 0; // their charged bytes

  /** Use socket(method, ...args); open opens the underlying stream. */
  constructor(method: string, open: () => TangoStream<unknown>) {
    super();
    this.url = "tango:" + method;
    for (const t of ["open", "message", "error", "close"]) {
      this.addEventListener(t, (e) => {
        const h = (this as any)["on" + t];
        if (typeof h === "function") h.call(this, e);
      });
    }
    this.s = open() as Internal;
    this.s.accepted.then(
      () => this.opened(),
      () => {},
    );
    void this.run();
  }

  get readyState(): number {
    return this.state;
  }

  get binaryType(): "arraybuffer" | "blob" {
    return this.type;
  }

  set binaryType(v: "arraybuffer" | "blob") {
    if (v === "arraybuffer" || v === "blob") this.type = v; // others are ignored, as by WebSocket
  }

  get bufferedAmount(): number {
    return this.s.bufferedAmount + this.waitingBytes;
  }

  send(data: string | ArrayBuffer | ArrayBufferView | Blob): void {
    if (this.state >= CLOSING) return;
    if (typeof g.Blob === "function" && data instanceof g.Blob) {
      const blob = data as Blob;
      this.later(blob.size + FRAME_OVERHEAD, () => blob.arrayBuffer().then((b) => this.watch(this.s.sendBytes(b))));
      return;
    }
    if (isBytes(data)) {
      if (!this.waiting) return this.watch(this.s.sendBytes(data));
      const u = copyBytes(data)!; // captured now, sent after the Blob
      this.later(u.length + FRAME_OVERHEAD, () => this.watch(this.s.sendBytes(u)));
      return;
    }
    const str = String(data);
    if (!this.waiting) return this.watch(this.s.send(str));
    this.later(str.length + FRAME_OVERHEAD, () => this.watch(this.s.send(str)));
  }

  /** Closes the stream after queued sends (see the class comment); code
   * and reason are not sent to Go. */
  close(code?: number, reason?: string): void {
    void code, reason; // not sent to Go
    if (this.state >= CLOSING) return;
    this.state = CLOSING;
    if (this.waiting) this.later(0, () => this.s._queueClose());
    else this.s._queueClose();
  }

  private endSending(): void {
    if (this.waiting) this.later(0, () => this.s.end());
    else this.s.end();
  }

  /** Runs step after the earlier chained steps; a failing step (a Blob
   * that cannot be read) cancels the stream. */
  private later(n: number, step: () => unknown): void {
    this.waiting++;
    this.waitingBytes += n;
    const done = () => {
      this.waiting--;
      this.waitingBytes -= n;
    };
    this.tail = this.tail.then(step).then(done, () => {
      done();
      this.s.cancel();
    });
  }

  /** A send rejected with bad_args fails the socket (nothing else reports
   * it); other rejections mean the stream is closing and are expected. */
  private watch(p: Promise<void>): void {
    p.catch((e) => {
      if (e instanceof TangoError && e.code === "bad_args" && e !== this.s._peerError) this.s._close(e);
    });
  }

  private opened(): void {
    if (this.state !== CONNECTING) return;
    this.state = OPEN;
    this.dispatchEvent(new Event("open"));
  }

  private data(v: unknown): unknown {
    if (v instanceof Uint8Array) {
      if (this.type === "blob") return new g.Blob([v]);
      return v.byteLength === v.buffer.byteLength ? v.buffer : v.slice().buffer;
    }
    return typeof v === "string" ? v : JSON.stringify(v);
  }

  /** Delivers messages, then closes once the stream is finished. */
  private async run(): Promise<void> {
    const it = this.s[Symbol.asyncIterator]();
    for (;;) {
      let r: IteratorResult<unknown>;
      try {
        r = await it.next();
      } catch {
        break; // Go's error, a cancel or a failure: told apart below by the stream's state.
      }
      if (r.done) break;
      this.opened(); // frames only follow Go's accept
      if (this.state !== OPEN) continue; // after close(): dropped
      let ev: MessageEvent;
      try {
        ev = messageEvent(this.data(r.value));
      } catch (e) {
        // The frame cannot be delivered (e.g. no Blob for binaryType
        // "blob"): fail the socket rather than leave the stream unread.
        this.s.cancel();
        reportLater(e);
        continue; // the iterator now ends
      }
      this.dispatchEvent(ev);
    }
    if (this.state < CLOSING) {
      // Go ended its side: answer with ours, like a WebSocket close handshake.
      this.state = CLOSING;
      this.endSending();
    }
    const aborted = await this.s._finished;
    this.state = CLOSED;
    const goErr = this.s._peerError;
    if (goErr) {
      this.dispatchEvent(new Event("error"));
      this.dispatchEvent(closeEvent(1011, goErr.message, false));
    } else if (aborted) {
      this.dispatchEvent(closeEvent(1006, "", false));
    } else {
      this.dispatchEvent(closeEvent(1000, "", true));
    }
  }
}
