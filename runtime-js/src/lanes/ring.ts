// Wire format mirrors internal/lane/shmring/ring.go. WebView2 exposes an
// ArrayBuffer, so the header and payload use ordinary little-endian accesses.
const HEADER = 64;
const WRITE = 0;
const READ = 4;
const READER_SLEEPING = 8;
const WRITER_WAITING = 12;
const GEN = 16;
const MAGIC = 24;
const CAPACITY = 28;
const RESERVED = 32;
const TGR1 = 0x31524754;
const WRAP = 0xffffffff;
const recordSize = (length: number) => (length + 7) & ~3;

export class Ring {
  readonly capacity: number;
  private readonly view: DataView;
  private readonly bytes: Uint8Array;

  constructor(buffer: ArrayBuffer, gen: bigint) {
    const capacity = buffer.byteLength - HEADER;
    if (capacity < 65536 || capacity > 2 ** 31 || (capacity & (capacity - 1)) !== 0) {
      throw new Error("shmring: invalid mapping size");
    }
    this.capacity = capacity;
    this.view = new DataView(buffer);
    this.bytes = new Uint8Array(buffer);
    if (!this.matchesFormat() || this.view.getBigUint64(GEN, true) !== gen) {
      throw new Error("shmring: invalid header");
    }
  }

  private load(offset: number): number {
    return this.view.getUint32(offset, true);
  }

  private store(offset: number, value: number): void {
    this.view.setUint32(offset, value, true);
  }

  private matchesFormat(): boolean {
    return this.load(MAGIC) === TGR1 && this.load(CAPACITY) === this.capacity;
  }

  private hasSpace(write: number, needed: number): boolean {
    const read = this.load(READ);
    return (read & 3) === 0 && ((write - read) >>> 0) <= this.capacity - needed;
  }

  /** A wrap marker alone can publish bytes and request a wake with ok=false. */
  tryWrite(packet: Uint8Array): { ok: boolean; wakeReader: boolean } {
    let write = this.load(WRITE);
    if (packet.length > this.capacity / 2 || (write & 3) !== 0) {
      return { ok: false, wakeReader: false };
    }
    const size = recordSize(packet.length);
    let offset = write & (this.capacity - 1);
    const tail = this.capacity - offset;
    const wraps = tail < size;
    const markerOnly = wraps && tail + size > this.capacity;
    const needed = markerOnly ? tail : size + (wraps ? tail : 0);
    if (!this.hasSpace(write, needed)) {
      this.store(WRITER_WAITING, 1);
      if (!this.hasSpace(write, needed)) return { ok: false, wakeReader: false };
    }
    if (wraps) {
      this.store(HEADER + offset, WRAP);
      write += tail;
      offset = 0;
      if (markerOnly) {
        this.store(WRITER_WAITING, 1);
        this.store(WRITE, write);
        return { ok: false, wakeReader: this.load(READER_SLEEPING) === 1 };
      }
    }
    const at = HEADER + offset;
    this.store(at, packet.length);
    this.bytes.set(packet, at + 4);
    this.bytes.fill(0, at + 4 + packet.length, at + size);
    this.writerWoken();
    this.store(WRITE, write + size);
    return { ok: true, wakeReader: this.load(READER_SLEEPING) === 1 };
  }

  /** Drains one fresh write snapshot. Views alias the mapping until dst returns. */
  read(dst: (packet: Uint8Array) => void): boolean {
    if (!this.matchesFormat() || this.load(READER_SLEEPING) > 1 || this.load(WRITER_WAITING) > 1) {
      throw new Error("shmring: invalid header");
    }
    for (let at = RESERVED; at < HEADER; at += 4) {
      if (this.load(at) !== 0) throw new Error("shmring: invalid header");
    }
    this.store(READER_SLEEPING, 0);
    let read = this.load(READ);
    const write = this.load(WRITE);
    if (((read | write) & 3) !== 0 || ((write - read) >>> 0) > this.capacity) {
      throw new Error("shmring: invalid cursors");
    }
    const start = read;
    while (read !== write) {
      const offset = read & (this.capacity - 1);
      const at = HEADER + offset;
      const tail = this.capacity - offset;
      const available = (write - read) >>> 0;
      const length = this.load(at);
      if (length === WRAP) {
        if (tail > this.capacity / 2 || tail > available) throw new Error("shmring: invalid wrap marker");
        read = (read + tail) >>> 0;
      } else {
        if (length > this.capacity / 2) throw new Error("shmring: invalid record length");
        const size = recordSize(length);
        if (size > available || size > tail) throw new Error("shmring: invalid record length");
        dst(this.bytes.subarray(at + 4, at + 4 + length));
        read = (read + size) >>> 0;
      }
      this.store(READ, read);
    }
    return read !== start && this.load(WRITER_WAITING) === 1;
  }

  /** Arms the wake, then rechecks for a write racing the sleep. */
  sleep(): boolean {
    this.store(READER_SLEEPING, 1);
    if (this.load(READ) === this.load(WRITE)) return false;
    this.store(READER_SLEEPING, 0);
    return true;
  }

  writerWoken(): void {
    this.store(WRITER_WAITING, 0);
  }
}
