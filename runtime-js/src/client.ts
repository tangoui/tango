// Tango IPC client. Implements the wire protocol described in
// docs/architecture.md ("IPC 线上协议") on top of window.__tango.

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

/** A stream of values produced by Go. Iterate with for await; breaking out
 * of the loop or calling cancel() cancels it on the Go side. */
export interface TangoStream<T = unknown> extends AsyncIterable<T> {
  readonly id: number;
  cancel(): void;
}

export type StreamHandler = (stream: TangoStream<any>, ...args: any[]) => void;

export interface Client {
  call<T = unknown>(method: string, ...args: unknown[]): Promise<T>;
  on(name: string, fn: (data: any) => void): () => void;
  emit(name: string, data?: unknown): void;
  stream<T = unknown>(method: string, ...args: unknown[]): TangoStream<T>;
  /** Handles streams Go opens with Router.OpenStream(conn, name, ...). */
  onStream(name: string, fn: StreamHandler): () => void;
  /** Subscribes to non-ipc raw messages. Messages that arrive before the
   * first subscriber are buffered and delivered to it. */
  onRaw(fn: (msg: string) => void): () => void;
  postRaw(msg: string): void;
}

/** Messages Go may have in flight before it waits for an ack. */
export const WINDOW = 64;
/** The consumer acks after this many values. */
export const ACK_EVERY = 16;

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
  n?: number;
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

class StreamImpl<T> implements TangoStream<T> {
  private buf: T[] = [];
  private waiters: { resolve(r: Result<T>): void; reject(e: unknown): void }[] = [];
  private remoteDone = false; // end/error received
  private localDone = false; // canceled or fully consumed
  private err: TangoError | null = null;
  private unacked = 0;

  constructor(
    readonly id: number,
    private readonly c: ClientImpl,
  ) {}

  _data(v: T): void {
    if (this.remoteDone || this.localDone) return;
    const w = this.waiters.shift();
    if (w) {
      this.consumed();
      w.resolve({ value: v, done: false });
    } else {
      this.buf.push(v);
    }
  }

  _finish(err: TangoError | null): void {
    if (this.remoteDone) return;
    this.remoteDone = true;
    this.err = err;
    this.c._forget(this.id);
    // Waiters only exist while the buffer is empty.
    for (const w of this.waiters.splice(0)) {
      if (this.err) {
        w.reject(this.err);
        this.err = null;
        this.localDone = true;
      } else {
        w.resolve({ value: undefined, done: true });
      }
    }
  }

  private consumed(): void {
    if (++this.unacked >= ACK_EVERY && !this.remoteDone) {
      this.c._post({ tango: "stream", id: this.id, op: "ack", n: this.unacked });
      this.unacked = 0;
    }
  }

  next(): Promise<Result<T>> {
    if (this.buf.length) {
      const value = this.buf.shift() as T;
      this.consumed();
      return Promise.resolve({ value, done: false });
    }
    if (this.localDone) return Promise.resolve({ value: undefined, done: true });
    if (this.remoteDone) {
      this.localDone = true;
      const e = this.err;
      this.err = null;
      return e ? Promise.reject(e) : Promise.resolve({ value: undefined, done: true });
    }
    return new Promise((resolve, reject) => this.waiters.push({ resolve, reject }));
  }

  cancel(): void {
    if (this.localDone) return;
    this.localDone = true;
    this.buf = [];
    if (!this.remoteDone) {
      this.remoteDone = true;
      this.c._forget(this.id);
      this.c._post({ tango: "stream", id: this.id, op: "cancel" });
    }
    for (const w of this.waiters.splice(0)) w.resolve({ value: undefined, done: true });
  }

  [Symbol.asyncIterator](): AsyncIterator<T, undefined> {
    return {
      next: () => this.next(),
      return: () => {
        this.cancel();
        return Promise.resolve({ value: undefined, done: true });
      },
    };
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

  constructor(private readonly bridge: Bridge) {
    bridge.onMessage((msg) => this.receive(msg));
    // Tell Go a new document started so state of the previous page is dropped.
    this._post({ tango: "hello" });
  }

  _post(m: Wire): void {
    this.bridge.post(JSON.stringify(m));
  }

  _forget(id: number): void {
    this.streams.delete(id);
  }

  private id(): number {
    this.nextId += 2;
    return this.nextId;
  }

  call<T = unknown>(method: string, ...args: unknown[]): Promise<T> {
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
    this.streams.set(s.id, s as StreamImpl<unknown>);
    this._post({ tango: "stream", id: s.id, op: "open", method, args });
    return s;
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
      const fn = this.streamHandlers.get(m.method as string);
      if (!fn) {
        this._post({
          tango: "stream",
          id,
          op: "error",
          error: { message: "no stream handler: " + m.method, code: "not_found" },
        });
        return;
      }
      const s = new StreamImpl<unknown>(id, this);
      this.streams.set(id, s);
      try {
        fn(s, ...(Array.isArray(m.args) ? m.args : []));
      } catch (e) {
        s.cancel();
        report(e);
      }
      return;
    }
    const s = this.streams.get(id);
    if (!s) return;
    switch (m.op) {
      case "data":
        s._data(m.data);
        return;
      case "end":
        s._finish(null);
        return;
      case "error":
        s._finish(toError(m.error));
        return;
    }
  }
}

/** Creates a client on an explicit bridge. Most code uses the module-level
 * functions, which share one client per window.__tango. */
export function createClient(bridge: Bridge): Client {
  return new ClientImpl(bridge);
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
