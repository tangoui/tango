// down:poll, the core's downstream transport (see the Go packages
// internal/downq and internal/lane, and the core's downShim): the page's
// core shim polls GET /__tango/lane/poll on the asset origin, puts the
// items of each batch back in sequence order and hands every byte packet
// (the lane packet format, count:u16 | frame*) to the subscribers of
// window.__tango._onPacket. Polling, ordering and falling back to Eval are
// the core's; this module only subscribes and passes the packets on, so it
// never fetches anything itself.
//
// It is ready as soon as it has subscribed: the shim delivers only packets
// it actually received (packets that arrived before the subscription are
// kept and delivered in order). A core without _onPacket (one older than
// down:poll) makes start reject, so the lane fails at once.
//
// It registers at module load, so the ESM entry and the injected IIFE have
// it before the client's hello reply.

import { registerLane, type DownLane, type LaneContext } from "../lanes.js";

class PollDown implements DownLane {
  private off: (() => void) | null = null;
  private stopped = false;

  start(ctx: LaneContext): Promise<void> {
    return new Promise<void>((resolve) => {
      if (this.stopped) throw new Error("down:poll: stopped");
      const core = (globalThis as any).__tango;
      const hook = core?._onPacket;
      if (typeof hook !== "function") {
        throw new Error("down:poll: window.__tango._onPacket is missing (core without down:poll)");
      }
      const off = hook.call(core, (p: Uint8Array) => {
        if (!this.stopped) ctx.onPacket(p);
      });
      this.off = typeof off === "function" ? off : null;
      resolve();
    });
  }

  stop(): void {
    this.stopped = true;
    const off = this.off;
    this.off = null;
    off?.();
  }
}

/** A down:poll module (registered as "down:poll"). */
export function newPollDown(): DownLane {
  return new PollDown();
}

registerLane("down:poll", () => newPollDown());
