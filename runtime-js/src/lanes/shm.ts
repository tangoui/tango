// Windows WebView2 shared-buffer lanes (Go: internal/platform/windows/lanes.go).
// Buffer metadata identifies the ring and generation. Host wakes are JSON
// strings from PostWebMessageAsString. Page wakes use the internal lane prefix.
import { INTERNAL_PREFIX, registerLane, type DownLane, type LaneContext, type UpLane } from "../lanes.js";
import { encodePacket, type PacketFrame } from "../packet.js";
import { Ring } from "./ring.js";

const START_TIMEOUT_MS = 5000;
const MAX_QUEUED_BYTES = 8 << 20;
const CONNECTION = Symbol.for("tango.shmLanes");
type Direction = "down" | "up";

interface WebView {
  addEventListener(name: "sharedbufferreceived", fn: (event: SharedBufferEvent) => void): void;
  addEventListener(name: "message", fn: (event: MessageEvent) => void): void;
  postMessage(message: string): void;
  releaseBuffer(buffer: ArrayBuffer): void;
}

interface SharedBufferEvent {
  getBuffer(): ArrayBuffer;
  additionalData: unknown;
}

interface BufferOffer {
  gen: number;
  buffer: ArrayBuffer;
}

interface Slot {
  started: boolean;
  lane?: ShmLane;
  pending?: BufferOffer;
}

function isDirection(ring: unknown): ring is Direction {
  return ring === "down" || ring === "up";
}

function isGeneration(gen: unknown): gen is number {
  return typeof gen === "number" && Number.isSafeInteger(gen) && gen >= 0;
}

class Connection {
  private gen = -1;
  private readonly slots: Record<Direction, Slot> = {
    down: { started: false },
    up: { started: false },
  };

  constructor(private readonly webview: WebView) {
    webview.addEventListener("sharedbufferreceived", (event) => this.receive(event));
    webview.addEventListener("message", (event) => this.message(event.data));
  }

  bind(lane: ShmLane, gen: number): boolean {
    if (gen < this.gen) return false;
    if (gen > this.gen) {
      this.gen = gen;
      for (const slot of Object.values(this.slots)) {
        slot.lane?.stop();
        slot.started = false;
        if (slot.pending && slot.pending.gen !== gen) {
          this.release(slot.pending.buffer);
          slot.pending = undefined;
        }
      }
    }
    const slot = this.slots[lane.direction];
    if (slot.started) return false;
    slot.started = true;
    slot.lane = lane;
    const pending = slot.pending;
    slot.pending = undefined;
    if (pending) lane.attach(pending.buffer);
    return true;
  }

  unbind(lane: ShmLane): void {
    const slot = this.slots[lane.direction];
    if (slot.lane === lane) slot.lane = undefined;
  }

  release(buffer: ArrayBuffer): void {
    this.webview.releaseBuffer(buffer);
  }

  postWake(ring: Direction, gen: number): void {
    this.webview.postMessage(INTERNAL_PREFIX + "lane:" + JSON.stringify({ op: "wake", ring, gen }));
  }

  private receive(event: SharedBufferEvent): void {
    const meta = event.additionalData as { tango?: unknown; ring?: unknown; gen?: unknown } | null;
    if (meta?.tango !== "lane") return;
    const buffer = event.getBuffer();
    const { ring, gen } = meta;
    if (!isDirection(ring) || !isGeneration(gen)) {
      this.release(buffer);
      return;
    }
    const slot = this.slots[ring];
    if (slot.lane && gen === this.gen) {
      slot.lane.attach(buffer);
      return;
    }
    const active = this.slots.down.lane || this.slots.up.lane;
    // Keep one early mapping per ring. A stopped slot discards late offers
    // for that generation, including a buffer that arrives after timeout.
    const early = active ? gen === this.gen && !slot.started : gen > this.gen;
    if (!early || (slot.pending && gen <= slot.pending.gen)) {
      this.release(buffer);
      return;
    }
    if (slot.pending) this.release(slot.pending.buffer);
    slot.pending = { gen, buffer };
  }

  private message(data: unknown): void {
    if (typeof data !== "string" || data[0] !== "{") return;
    let msg: { tango?: unknown; op?: unknown; gen?: unknown; ring?: unknown };
    try {
      msg = JSON.parse(data);
    } catch {
      return;
    }
    if (msg?.tango === "lane" && msg.op === "wake" && msg.gen === this.gen && isDirection(msg.ring)) {
      this.slots[msg.ring].lane?.wake();
    }
  }
}

function connection(): Connection | undefined {
  const g = globalThis as typeof globalThis & { [CONNECTION]?: Connection };
  if (g[CONNECTION]) return g[CONNECTION];
  const page = g.window as (Window & { chrome?: { webview?: WebView } }) | undefined;
  const webview = page?.chrome?.webview;
  if (!webview || typeof webview.addEventListener !== "function" ||
      typeof webview.postMessage !== "function" || typeof webview.releaseBuffer !== "function") return;
  const conn = new Connection(webview);
  Object.defineProperty(g, CONNECTION, { value: conn, configurable: true });
  return conn;
}

abstract class ShmLane {
  protected ctx?: LaneContext;
  protected ring?: Ring;
  protected conn?: Connection;
  private buffer?: ArrayBuffer;
  private finishStart?: (err?: unknown) => void;

  constructor(readonly direction: Direction) {}

  start(ctx: LaneContext): Promise<void> {
    this.ctx = ctx;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => this.fail(new Error("shm lane: shared buffer timeout")), START_TIMEOUT_MS);
      this.finishStart = (err) => {
        clearTimeout(timer);
        this.finishStart = undefined;
        if (err === undefined) resolve();
        else reject(err);
      };
      try {
        const conn = connection();
        if (!conn) throw new Error("shm lane: chrome.webview unavailable");
        this.conn = conn;
        if (!conn.bind(this, ctx.gen)) throw new Error("shm lane: generation already started or retired");
      } catch (err) {
        this.fail(err);
      }
    });
  }

  attach(buffer: ArrayBuffer): void {
    const ctx = this.ctx;
    if (!ctx || this.buffer) {
      this.conn?.release(buffer);
      return;
    }
    this.buffer = buffer;
    try {
      this.ring = new Ring(buffer, BigInt(ctx.gen));
      // Arm down's reader before the core can announce readiness to Go.
      if (this.direction === "down") this.handleWake(this.ring, ctx);
      this.finishStart?.();
    } catch (err) {
      this.fail(err);
    }
  }

  wake(): void {
    const { ring, ctx } = this;
    if (!ring || !ctx) return;
    try {
      this.handleWake(ring, ctx);
    } catch (err) {
      this.fail(err);
    }
  }

  protected abstract handleWake(ring: Ring, ctx: LaneContext): void;

  protected fail(err: unknown): void {
    const ctx = this.ctx;
    if (!ctx) return;
    this.finishStart?.(err ?? new Error("shm lane: failed"));
    this.stop();
    ctx.fail(err);
  }

  stop(): void {
    const { conn, buffer } = this;
    this.ctx = undefined;
    this.ring = undefined;
    this.conn = undefined;
    this.buffer = undefined;
    this.finishStart?.(new Error("shm lane: stopped"));
    conn?.unbind(this);
    if (buffer) conn?.release(buffer);
  }
}

class ShmDown extends ShmLane implements DownLane {
  constructor() {
    super("down");
  }

  protected handleWake(ring: Ring, ctx: LaneContext): void {
    do {
      if (ring.read((packet) => ctx.onPacket(packet))) this.conn?.postWake("down", ctx.gen);
    } while (ring.sleep());
  }
}

interface Queued {
  packet: Uint8Array;
  stream: number;
  seq: number;
}

class ShmUp extends ShmLane implements UpLane {
  readonly minSize = 0;
  private readonly queue: Queued[] = [];
  private queuedBytes = 0;
  private waiting = false;
  private flushing = false;
  private broken = false;

  constructor() {
    super("up");
  }

  send(frame: PacketFrame): boolean {
    const { ring, ctx } = this;
    if (!ring || !ctx || this.broken || frame.payload.length > ring.capacity / 2) return false;
    let packet;
    try {
      packet = encodePacket([frame]);
    } catch {
      return false;
    }
    if (packet.length > ring.capacity / 2 || this.queuedBytes + packet.length > MAX_QUEUED_BYTES) return false;
    this.queue.push({ packet, stream: frame.stream, seq: frame.seq });
    this.queuedBytes += packet.length;
    this.flush(ring, ctx);
    return true;
  }

  protected handleWake(ring: Ring, ctx: LaneContext): void {
    this.waiting = false;
    ring.writerWoken();
    this.flush(ring, ctx);
  }

  private flush(ring: Ring, ctx: LaneContext): void {
    if (this.flushing || this.waiting || this.broken) return;
    this.flushing = true;
    let sent = 0;
    try {
      while (this.ring === ring && sent < this.queue.length) {
        const { packet, stream, seq } = this.queue[sent];
        const { ok, wakeReader } = ring.tryWrite(packet);
        this.waiting = !ok;
        if (wakeReader) this.conn?.postWake("up", ctx.gen);
        if (!ok) break;
        sent++;
        this.queuedBytes -= packet.length;
        ctx.dispatched(stream, seq);
      }
    } catch (err) {
      this.broken = true;
      // Let send return its acceptance before the client accounts for loss.
      queueMicrotask(() => this.fail(err));
    } finally {
      this.queue.splice(0, sent);
      this.flushing = false;
    }
  }

  stop(): void {
    this.queue.length = 0;
    this.queuedBytes = 0;
    super.stop();
  }
}

export const newShmDown = (): DownLane => new ShmDown();
export const newShmUp = (): UpLane => new ShmUp();

// Listen before the eager hello in index.ts. The IIFE and ESM share one
// connection so a SharedBuffer event has exactly one owner.
connection();
registerLane("down:shm", newShmDown);
registerLane("up:shm", newShmUp);
