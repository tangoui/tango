// Fast lane packets (Go: ipc.AppendPacket / ipc.ParsePacket). Big endian:
//
//   packet := count:u16 | frame*count
//   frame  := streamID:u64 | seq:u64 | len:u32 | payload[len]
//
// The empty packet (count 0) is valid. Stream ids and seqs are JS numbers:
// a value above Number.MAX_SAFE_INTEGER makes a packet corrupt.

/** Largest raw payload of one stream frame (Go: ipc.MaxFrameBytes). */
export const MAX_FRAME = 16777216;
const HEADER = 2;
const FRAME_HEADER = 8 + 8 + 4;
const MAX_FRAMES = 0xffff;
const TWO32 = 0x100000000;

/** One byte frame of a fast lane packet. */
export interface PacketFrame {
  stream: number;
  seq: number;
  payload: Uint8Array;
}

const isU53 = (n: unknown): n is number => typeof n === "number" && Number.isSafeInteger(n) && n >= 0;

function putU64(dv: DataView, at: number, n: number): void {
  dv.setUint32(at, Math.floor(n / TWO32));
  dv.setUint32(at + 4, n % TWO32);
}

/** Returns the u64 at `at`, or -1 if it does not fit a safe integer. */
function getU64(dv: DataView, at: number): number {
  const hi = dv.getUint32(at);
  if (hi > 0x1fffff) return -1;
  return hi * TWO32 + dv.getUint32(at + 4);
}

/** Encodes frames as one packet. Throws (a caller bug) for more than 65535
 * frames, a payload over MAX_FRAME_BYTES or an id/seq that is not a safe
 * non-negative integer. */
export function encodePacket(frames: PacketFrame[]): Uint8Array {
  if (frames.length > MAX_FRAMES) throw new RangeError("encodePacket: more than 65535 frames");
  let n = HEADER;
  for (const f of frames) {
    if (!isU53(f.stream) || !isU53(f.seq)) throw new RangeError("encodePacket: invalid stream id or seq");
    if (f.payload.length > MAX_FRAME) throw new RangeError("encodePacket: payload exceeds MAX_FRAME_BYTES");
    n += FRAME_HEADER + f.payload.length;
  }
  const out = new Uint8Array(n);
  const dv = new DataView(out.buffer);
  dv.setUint16(0, frames.length);
  let at = HEADER;
  for (const f of frames) {
    putU64(dv, at, f.stream);
    putU64(dv, at + 8, f.seq);
    dv.setUint32(at + 16, f.payload.length);
    out.set(f.payload, at + FRAME_HEADER);
    at += FRAME_HEADER + f.payload.length;
  }
  return out;
}

/** Decodes a packet; throws unless p holds exactly the frames its count
 * announces, each with a payload of at most MAX_FRAME_BYTES and a safe
 * integer id and seq, and nothing after them. Payloads are views into p. */
export function decodePacket(p: Uint8Array): PacketFrame[] {
  if (p.length < HEADER) throw new Error("lane packet shorter than its header");
  const dv = new DataView(p.buffer, p.byteOffset, p.byteLength);
  const count = dv.getUint16(0);
  let at = HEADER;
  // Every frame needs at least its header.
  if (count > Math.floor((p.length - at) / FRAME_HEADER)) {
    throw new Error(`lane packet announces ${count} frames in ${p.length - at} bytes`);
  }
  const frames: PacketFrame[] = new Array(count);
  for (let i = 0; i < count; i++) {
    if (p.length - at < FRAME_HEADER) throw new Error(`lane packet frame ${i}: truncated header`);
    const stream = getU64(dv, at);
    const seq = getU64(dv, at + 8);
    if (stream < 0 || seq < 0) throw new Error(`lane packet frame ${i}: id or seq beyond 2^53`);
    const n = dv.getUint32(at + 16);
    if (n > MAX_FRAME) throw new Error(`lane packet frame ${i}: ${n} bytes exceeds MAX_FRAME_BYTES`);
    const body = at + FRAME_HEADER;
    if (n > p.length - body) throw new Error(`lane packet frame ${i}: ${n} bytes past the end of the packet`);
    frames[i] = { stream, seq, payload: p.subarray(body, body + n) };
    at = body + n;
  }
  if (at !== p.length) throw new Error(`lane packet has ${p.length - at} trailing bytes`);
  return frames;
}
