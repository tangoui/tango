// The duplex examples' benchmark (window.duplexBench), served as /bench.js
// by services.BenchRoute: examples/duplex's page (-bench) and
// examples/duplex-ios (index.html?bench) run the same code. See
// docs/ipc-bench.md for the fields.
"use strict";

(() => {
  const KiB = 1024;
  const round2 = (x) => Math.round(x * 100) / 100;
  const round3 = (x) => Math.round(x * 1000) / 1000;
  const mbps = (bytes, ms) => round2(bytes / 1e6 / (ms / 1000));

  function percentile(sorted, p) {
    return sorted[Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)];
  }

  /** Round trips of a 64-byte byte frame over one Echo stream, in batches:
   * batches × per trips, each batch timed as a whole. Returns the per-trip
   * times (when single) and the batch means. WebKit clamps performance.now()
   * to 1 ms when the page is not cross-origin isolated, which makes single
   * round trips unreadable there; a batch mean (batch total / per) is not. */
  async function echoTrips(batches, per) {
    const s = tango.stream("Echo");
    const it = s[Symbol.asyncIterator]();
    const payload = new Uint8Array(64);
    const times = [];
    const means = [];
    await s.accepted;
    const start = performance.now();
    for (let b = 0; b < batches; b++) {
      const b0 = performance.now();
      for (let i = 0; i < per; i++) {
        const t0 = performance.now();
        s.sendBytes(payload);
        const r = await it.next();
        times.push(performance.now() - t0);
        if (r.done || r.value.length !== 64) throw new Error("bad echo at " + (b * per + i));
      }
      means.push((performance.now() - b0) / per);
    }
    const mean = (performance.now() - start) / (batches * per);
    s.end();
    while (!(await it.next()).done);
    return { times, means, mean };
  }

  /** 1000 single round trips: p50/p99 of each trip, mean over the loop. */
  async function latency(n = 1000) {
    const { times, mean } = await echoTrips(1, n);
    times.sort((a, b) => a - b);
    return { p50: round2(percentile(times, 50)), p99: round2(percentile(times, 99)), mean: round3(mean) };
  }

  /** batches batches of per round trips: p50/p99 over the batch means
   * (with 20 batches the p99 is the slowest batch). */
  async function latencyBatches(batches = 20, per = 50) {
    const { means } = await echoTrips(batches, per);
    const sorted = means.slice().sort((a, b) => a - b);
    return {
      p50: round3(percentile(sorted, 50)),
      p99: round3(percentile(sorted, 99)),
      means: means.map(round3),
    };
  }

  /** JS → Go: 64 MiB in 64 KiB frames into Sink. */
  async function up(frames = 1024, size = 64 * KiB) {
    const buf = new Uint8Array(size);
    const t0 = performance.now();
    const s = tango.stream("Sink");
    for (let i = 0; i < frames; i++) await s.sendBytes(buf);
    s.end();
    let res;
    for await (const m of s) res = m;
    const ms = performance.now() - t0;
    if (!res || res.bytes !== frames * size) throw new Error("sink got " + JSON.stringify(res));
    return mbps(frames * size, ms);
  }

  /** Go → JS: 1024 frames of 64 KiB from Source. */
  async function down(frames = 1024, size = 64 * KiB) {
    const t0 = performance.now();
    let n = 0;
    for await (const b of tango.stream("Source", frames, size)) n += b.length;
    const ms = performance.now() - t0;
    if (n !== frames * size) throw new Error("source sent " + n);
    return mbps(n, ms);
  }

  /** Debug.Stats when the test hooks are on (TANGO_SCHEME_PROBE=1), else
   * null. */
  async function stats() {
    try {
      return await tango.call("Debug.Stats");
    } catch {
      return null;
    }
  }

  /** The whole benchmark; log gets one progress line at a time. */
  async function run(log = () => {}) {
    log("延迟…");
    const lat = await latency();
    log(`  p50 ${lat.p50} ms, p99 ${lat.p99} ms, mean ${lat.mean} ms`);
    log("分批延迟（20 批 × 50 次）…");
    const batch = await latencyBatches();
    log(`  批均值 p50 ${batch.p50} ms, p99 ${batch.p99} ms`);
    log("上行（JS→Go）…");
    const s0 = await stats();
    const upMBps = await up();
    const s1 = await stats();
    log(`  ${upMBps} MB/s`);
    log("下行（Go→JS）…");
    const downMBps = await down();
    log(`  ${downMBps} MB/s`);
    // The fast lanes the page used: {down, up}, each a lane name such as
    // "up:post" or "fallback" (byte frames on the message channel). Read
    // after the run, so a lane that failed midway shows as fallback.
    const lane = tango.lanes();
    log(`  通道：下行 ${lane.down}，上行 ${lane.up}`);
    const res = {
      lane,
      platform: navigator.userAgent,
      latencyP50Ms: lat.p50,
      latencyP99Ms: lat.p99,
      latencyMeanMs: lat.mean,
      latencyBatchP50Ms: batch.p50,
      latencyBatchP99Ms: batch.p99,
      latencyBatchMeansMs: batch.means,
      upMBps,
      downMBps,
    };
    if (s0 && s1) {
      // The upload's up:post requests and the main-thread time the macOS
      // scheme handler spent on them before their handler goroutine
      // started (zero on iOS).
      const n = s1.upRequests - s0.upRequests;
      res.upMain = {
        requests: n,
        bodyBytes: s1.upBodyBytes - s0.upBodyBytes,
        totalMs: round3((s1.upMainNanos - s0.upMainNanos) / 1e6),
        perRequestUs: n ? round2((s1.upMainNanos - s0.upMainNanos) / n / 1e3) : 0,
        maxUs: round2((s1.upMainMaxNanos || 0) / 1e3),
      };
    }
    return res;
  }

  window.duplexBench = { latency, latencyBatches, up, down, run };
})();
