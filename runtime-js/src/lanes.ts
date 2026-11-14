// Fast lanes (see the Go package internal/lane and the ipc package comment,
// "Fast lanes"): side channels that carry stream byte frames as binary
// packets next to the message channel. Lane modules (one per lane name,
// e.g. down:scheme, up:post) register themselves with registerLane; the
// client starts the ones Go offers in its hello reply ("lanes" field) and
// reports each one ready or failed on the core's internal "lane" channel.

import type { PacketFrame } from "./packet.js";

/** Starts every internal page→Go message. Mirrors internal/corehook
 * InternalPrefix; keep the two in sync. */
export const INTERNAL_PREFIX = "\x00tango-int:";
/** The internal channel of lane control messages (Go: tango.laneChannel). */
const LANE_CHANNEL = "lane:";

/** What a lane module gets from the client when it is started. */
export interface LaneContext {
  /** The lane generation of Go's hello; part of every lane request. */
  gen: number;
  /** The generation's token (32 hex digits); part of every lane request. */
  tok: string;
  /** A downstream lane hands every packet it receives here. */
  onPacket(p: Uint8Array): void;
  /** The lane found itself broken: the client posts fail and stops it. */
  fail(err: unknown): void;
  /** An upstream lane calls this for a frame it took (send returned true)
   * once the frame is actually on its way to Go: handed to the transport
   * (up:post: the fetch of the batch holding it started), not merely
   * queued. Frames of one stream are reported in seq order; a lane may
   * call it from inside send. The client then posts the stream's boundary
   * (and a held end or close), so Go's gap timer never runs for a frame
   * still waiting in the page. Frames a lane drops when it stops or fails
   * are never reported: the client takes them as lost then. */
  dispatched(stream: number, seq: number): void;
  /** globalThis.fetch (tests replace the global). */
  fetch: typeof fetch;
}

/** A downstream (Go → JS) lane module. */
export interface DownLane {
  /** Resolves once the lane carries packets; rejects if it cannot. */
  start(ctx: LaneContext): Promise<void>;
  /** Releases the lane; called once, after start. Must not call ctx.fail. */
  stop(): void;
}

/** An upstream (JS → Go) lane module. */
export interface UpLane {
  /** Byte frames shorter than this go on the message channel. */
  readonly minSize: number;
  start(ctx: LaneContext): Promise<void>;
  /** Takes one frame (the lane owns its payload on true). false: not
   * taken now; the caller sends it on the message channel. Never throws
   * on purpose (a throw counts as false). A taken frame is reported with
   * ctx.dispatched once it goes on the wire. */
  send(frame: PacketFrame): boolean;
  stop(): void;
}

type Lane = DownLane | UpLane;
type Factory = () => Lane;

// The registry lives on the global object (keyed by a registry symbol), so
// the ESM build, the injected IIFE and platform scripts share it.
const REG = Symbol.for("tango.ipc.lanes");

function registry(): Map<string, Factory> {
  const g = globalThis as any;
  let m = g[REG] as Map<string, Factory> | undefined;
  if (!m) {
    m = new Map();
    Object.defineProperty(g, REG, { value: m, configurable: true });
  }
  return m;
}

/** Registers the module for lane name (a later registration replaces an
 * earlier one). make is called for every hello that offers name; it must
 * not do I/O (start does). Lanes registered after the hello reply arrived
 * are used from the next one. Internal: for the platform lane modules. */
export function registerLane(name: string, make: () => DownLane | UpLane): void {
  registry().set(name, make);
}

/** The "lanes" field of Go's hello reply. */
export interface LaneOffer {
  gen: number;
  tok: string;
  list: string[];
}

/** Validates the hello reply's "lanes" field; null if absent or malformed. */
export function parseOffer(v: unknown): LaneOffer | null {
  if (!v || typeof v !== "object") return null;
  const o = v as Record<string, unknown>;
  if (typeof o.gen !== "number" || !Number.isSafeInteger(o.gen) || o.gen < 0) return null;
  if (typeof o.tok !== "string" || !Array.isArray(o.list)) return null;
  return { gen: o.gen, tok: o.tok, list: o.list.filter((n): n is string => typeof n === "string") };
}

/** Pin requests to the captured document location, never the HTML base URL.
 * Exported internally for isolated adapter tests. Opaque custom-scheme origins
 * are compared by protocol and host rather than the string `null`. */
export function locationFetch(href: string | undefined, fetcher: typeof fetch): typeof fetch {
  const base = href ? new URL(href) : null;
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    if (!base) return fetcher(input, init); // non-browser injectable test adapter
    if (typeof input !== "string" || !/^\/__tango\/lane\/(up|down)\?/.test(input)) {
      throw new Error("tango: invalid lane destination");
    }
    const dest = new URL(input, base);
    if (dest.protocol !== base.protocol || dest.host !== base.host || dest.username || dest.password) {
      throw new Error("tango: cross-origin lane destination");
    }
    const res = await fetcher(dest.href, { ...init, redirect: "error" });
    if (res.redirected) throw new Error("tango: lane redirect refused");
    return res;
  }) as typeof fetch;
}

const isUp = (l: Lane): l is UpLane => typeof (l as UpLane).send === "function";

interface Entry {
  name: string;
  lane: Lane;
  started: boolean;
  ready: boolean;
  dead: boolean; // failed or stopped: its callbacks are ignored
}

/** The client's side of the lanes. */
export interface LaneHost {
  /** Posts a raw message on the bridge (control messages). */
  post(msg: string): void;
  /** A packet from the ready (or starting) downstream lane. */
  onPacket(p: Uint8Array): void;
  /** The upstream lane dispatched a frame (LaneContext.dispatched). */
  onDispatched(stream: number, seq: number): void;
  /** The ready upstream lane stopped or failed: frames it took and never
   * dispatched are lost. */
  onUpStopped(): void;
}

const report = (err: unknown) =>
  (globalThis as any).setTimeout(() => {
    throw err;
  }, 0);

/** The lanes of one client. Per direction, the offered modules are tried
 * one at a time in the order of Go's list: the first that starts is the
 * direction's lane; when it fails (to start, or later) the next one is
 * started. So each direction has at most one ready lane. */
export class Lanes {
  private request: typeof fetch = (...args) => (globalThis as any).fetch(...args);
  private gen = 0;
  private tok = "";
  private dirs: { down: Entry[]; up: Entry[] } = { down: [], up: [] };

  constructor(private readonly host: LaneHost) {}

  /** Applies a hello reply: every lane of the previous one stops, then the
   * offered (and registered) ones start. */
  offer(o: LaneOffer | null): void {
    this.stopAll();
    if (!o) return;
    this.request = locationFetch((globalThis as any).location?.href, (...args) => (globalThis as any).fetch(...args));
    this.gen = o.gen;
    this.tok = o.tok;
    const reg = registry();
    const seen = new Set<string>();
    for (const name of o.list) {
      const make = reg.get(name);
      if (!make || seen.has(name)) continue; // not available on this page
      seen.add(name);
      let lane: Lane;
      try {
        lane = make();
      } catch (e) {
        report(e);
        continue;
      }
      const e: Entry = { name, lane, started: false, ready: false, dead: false };
      (isUp(lane) ? this.dirs.up : this.dirs.down).push(e);
    }
    this.next(this.dirs.down);
    this.next(this.dirs.up);
  }

  /** Stops every lane (a new hello, or the client failed). Posts nothing:
   * Go dropped the generation already. */
  stopAll(): void {
    const all = [...this.dirs.down, ...this.dirs.up];
    this.dirs = { down: [], up: [] };
    for (const e of all) this.kill(e);
  }

  /** The ready upstream lane, or null. */
  up(): UpLane | null {
    const e = this.dirs.up[0];
    return e && e.ready ? (e.lane as UpLane) : null;
  }

  /** The ready lane names ("fallback": the message channel). */
  names(): { down: string; up: string } {
    const name = (d: Entry[]) => (d[0]?.ready ? d[0].name : "fallback");
    return { down: name(this.dirs.down), up: name(this.dirs.up) };
  }

  /** Starts the first entry of dir (the others wait their turn). */
  private next(dir: Entry[]): void {
    const e = dir[0];
    if (!e || e.started) return;
    e.started = true;
    const ctx: LaneContext = {
      gen: this.gen,
      tok: this.tok,
      onPacket: (p) => {
        if (!e.dead) this.host.onPacket(p);
      },
      fail: () => this.fail(e, dir),
      dispatched: (stream, seq) => {
        if (!e.dead) this.host.onDispatched(stream, seq);
      },
      fetch: this.request,
    };
    let started: Promise<void>;
    try {
      started = Promise.resolve(e.lane.start(ctx));
    } catch (err) {
      started = Promise.reject(err);
    }
    started.then(
      () => {
        if (e.dead) return;
        e.ready = true;
        this.control("ready", e.name);
      },
      () => this.fail(e, dir),
    );
  }

  /** e failed: post fail, stop it and start the next lane of dir. */
  private fail(e: Entry, dir: Entry[]): void {
    if (e.dead) return;
    this.kill(e);
    this.control("fail", e.name);
    if (dir[0] === e) {
      dir.shift();
      this.next(dir);
    }
  }

  private kill(e: Entry): void {
    if (e.dead) return;
    const wasReady = e.ready;
    e.dead = true;
    e.ready = false;
    if (!e.started) return;
    try {
      e.lane.stop();
    } catch (err) {
      report(err);
    }
    // Only a ready upstream lane takes frames; those it never dispatched
    // are lost now.
    if (wasReady && isUp(e.lane)) {
      try {
        this.host.onUpStopped();
      } catch (err) {
        report(err);
      }
    }
  }

  private control(op: "ready" | "fail", lane: string): void {
    try {
      this.host.post(INTERNAL_PREFIX + LANE_CHANNEL + JSON.stringify({ op, lane, gen: this.gen }));
    } catch (err) {
      report(err);
    }
  }
}
