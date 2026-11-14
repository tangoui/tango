// IPC fast lane scenarios on a real WebView, shared by the macOS lane tests
// (internal/platform/darwin/lanes_flow_e2e_test.go, driving
// examples/duplex) and the iOS simulator self-test (examples/duplex-mobile
// web/selftest.js). TestRoutes serves this file as /__test/lanetests.js
// (TANGO_SCHEME_PROBE=1); a page loads it with loadLaneTests below or a
// <script> tag and gets window.laneTests.
//
// The app runs with up:post on (the macOS default; the iOS self-test sets
// TANGO_IPC_LANES_DARWIN=1, iOS lanes being off by default): Apple
// platforms offer only up:post (spec §6.2), so the lane assertions are about the upstream
// direction (Debug.Stats upFrames: frames that reached Go on up:post).
// Downstream frames travel on the message channel; they are checked for
// being complete and in order only.
//
// Every scenario waits for Go's accept before sending (byte frames sent
// earlier take the message channel) and returns a plain object with the
// numbers it measured, ok, and why (the failed conditions).
"use strict";

(() => {
  const KiB = 1024;
  const sleep = (ms) => new Promise((res) => setTimeout(res, ms));
  const stats = () => tango.call("Debug.Stats");
  const errOf = (e) => ({ message: String((e && e.message) || e), code: (e && e.code) || "" });

  // until polls cond (async) every step ms until it returns a truthy value
  // or ms passed; returns the last value.
  async function until(cond, ms, step = 50) {
    const t0 = Date.now();
    for (;;) {
      const v = await cond();
      if (v || Date.now() - t0 > ms) return v;
      await sleep(step);
    }
  }

  // verdict adds ok and why to r from [condition, description] pairs.
  function verdict(r, conds) {
    r.why = conds.filter(([c]) => !c).map(([, d]) => d);
    r.ok = r.why.length === 0;
    return r;
  }

  // fill returns n bytes, byte i = (off + i) % 251.
  function fill(n, off) {
    const b = new Uint8Array(n);
    for (let i = 0, v = off % 251; i < n; i++, v = v === 250 ? 0 : v + 1) b[i] = v;
    return b;
  }

  // same reports whether m is exactly fill(n, off).
  function same(m, n, off) {
    if (!(m instanceof Uint8Array) || m.length !== n) return false;
    for (let i = 0, v = off % 251; i < n; i++, v = v === 250 ? 0 : v + 1) if (m[i] !== v) return false;
    return true;
  }

  // echoFrames sends the frames (Uint8Arrays, made lazily by make(k)) of
  // count n through a new Echo stream after its accept while reading the
  // echo, and checks echo k with check(k, m). hook(k), if given, runs after
  // frame k was sent. Returns {frames, bytes, bad, err, ms}.
  async function echoFrames(n, make, check, hook) {
    const t0 = performance.now();
    const r = { frames: 0, bytes: 0, bad: 0, firstBad: -1, err: null, ms: 0 };
    let s;
    try {
      s = tango.stream("Echo");
      await s.accepted;
    } catch (e) {
      r.err = errOf(e);
      return r;
    }
    const send = (async () => {
      for (let k = 0; k < n; k++) {
        await s.sendBytes(make(k));
        if (hook) hook(k);
      }
      s.end();
    })();
    const recv = (async () => {
      for await (const m of s) {
        if (!check(r.frames, m)) {
          r.bad++;
          if (r.firstBad < 0) r.firstBad = r.frames;
        }
        r.bytes += m && m.length ? m.length : 0;
        r.frames++;
      }
    })();
    const [a, b] = await Promise.allSettled([send, recv]);
    if (b.status === "rejected") r.err = errOf(b.reason);
    else if (a.status === "rejected") r.err = errOf(a.reason);
    r.ms = Math.round(performance.now() - t0);
    return r;
  }

  // bigBothWays: 8 MiB through Echo in 64 KiB frames (byte i of the stream
  // = i % 251), byte-exact both ways; at least half of the 128 frames
  // reached Go on up:post.
  async function bigBothWays() {
    const size = 64 * KiB, n = 128;
    const before = await stats();
    const r = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    r.upFrames = after.upFrames - before.upFrames;
    r.upRequests = after.upRequests - before.upRequests;
    return verdict(r, [
      [!r.err, "stream error " + JSON.stringify(r.err)],
      [r.frames === n, `echoed ${r.frames} frames, want ${n}`],
      [r.bytes === n * size, `echoed ${r.bytes} bytes, want ${n * size}`],
      [r.bad === 0, `${r.bad} frames differ (first ${r.firstBad})`],
      [r.upFrames >= n / 2, `upFrames ${r.upFrames}, want >= ${n / 2} (up:post)`],
    ]);
  }

  // mixedOrder: 40 frames on one stream, alternately 100 B and 64 KiB,
  // echoed in send order. The 64 KiB frames (>= up:post's 16 KiB minimum)
  // take up:post, the 100 B ones the message channel: upFrames == 20.
  async function mixedOrder() {
    const n = 40, sizeOf = (k) => (k % 2 === 0 ? 100 : 64 * KiB);
    const before = await stats();
    const r = await echoFrames(n, (k) => fill(sizeOf(k), k * 7), (k, m) => same(m, sizeOf(k), k * 7));
    const after = await stats();
    r.upFrames = after.upFrames - before.upFrames;
    return verdict(r, [
      [!r.err, "stream error " + JSON.stringify(r.err)],
      [r.frames === n, `echoed ${r.frames} frames, want ${n}`],
      [r.bad === 0, `${r.bad} frames out of order or wrong (first ${r.firstBad})`],
      [r.upFrames === n / 2, `upFrames ${r.upFrames}, want ${n / 2} (the 64 KiB frames)`],
    ]);
  }

  // stall: 4 MiB through Echo; right after frame 16 went out (its up:post
  // request just started) the page's main thread busy-loops for busyMs.
  // The stream must still complete, byte-exact, without a transport error
  // (up:post re-arms its request deadline after a stall; Go's gap timer
  // must not fire for frames the stalled page dispatched), and up:post must
  // still be the upstream lane afterwards (a lane that failed during the
  // stall would fall back silently and hide exactly that regression).
  async function stall(busyMs = 6000) {
    const size = 64 * KiB, n = 64;
    const before = await stats();
    let busy = 0;
    const r = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size), (k) => {
      if (k !== 16) return;
      const t = performance.now();
      while (performance.now() - t < busyMs) {}
      busy = Math.round(performance.now() - t);
    });
    const after = await stats();
    r.busyMs = busy;
    r.upFrames = after.upFrames - before.upFrames;
    r.lanes = tango.lanes();
    return verdict(r, [
      [!r.err, "stream error " + JSON.stringify(r.err)],
      [r.frames === n && r.bytes === n * size, `echoed ${r.frames} frames / ${r.bytes} bytes, want ${n} / ${n * size}`],
      [r.bad === 0, `${r.bad} frames differ (first ${r.firstBad})`],
      [busy >= busyMs, `busy loop ran ${busy} ms`],
      [r.upFrames >= 1, `upFrames ${r.upFrames}: up:post was not used`],
      [r.lanes.up === "up:post", `up lane after the stall is ${r.lanes.up}, want up:post`],
    ]);
  }

  // fault needs a tango_dev build with TANGO_IPC_LANE_FAULT=up:post:3 (Go
  // drops every up:post data packet after the third, answering 503). Stream B
  // (Echo) is opened first and stays open throughout. Stream A (Echo) sends
  // 64 KiB frames one at a time: frames 0-2 each ride their own up:post
  // packet; frame 3's packet is refused, up:post fails and the page falls
  // back to the message channel; frames 4-7 go there. A must fail with
  // code transport (frame 3 is lost; nothing resends it) after echoing
  // frames 0-2. B meanwhile sends 64 KiB frames, which now take the message
  // channel, and must echo all of them, byte-exact, and end normally.
  async function fault() {
    const size = 64 * KiB, small = 100;
    const t0 = performance.now();
    const before = await stats();
    const r = { a: { frames: 0, bad: 0, err: null }, b: { frames: 0, bad: 0, err: null }, steps: [] };
    const step = (s) => r.steps.push(Math.round(performance.now() - t0) + " " + s);
    const read = (s, out, check) => (async () => {
      try {
        for await (const m of s) {
          if (!check(out.frames, m)) out.bad++;
          out.frames++;
        }
      } catch (e) {
        out.err = errOf(e);
      }
      out.endMs = Math.round(performance.now() - t0); // when this stream ended
    })();

    const b = tango.stream("Echo");
    await b.accepted;
    // B's frames: 0 small; 1-4 64 KiB (sent after the fault); 5 small.
    const bSize = (k) => (k === 0 || k === 5 ? small : size);
    const bRead = read(b, r.b, (k, m) => same(m, bSize(k), 1000 + k));
    await b.sendBytes(fill(bSize(0), 1000));

    const a = tango.stream("Echo");
    await a.accepted;
    const aRead = read(a, r.a, (k, m) => same(m, size, k));
    r.lanesBefore = tango.lanes();
    for (let k = 0; k < 3; k++) {
      await a.sendBytes(fill(size, k));
      // One packet per frame: wait until Go got this one.
      const got = await until(async () => (await stats()).upFrames - before.upFrames >= k + 1, 5000);
      step(`A frame ${k} ${got ? "reached Go on up:post" : "did not reach Go within 5 s"}`);
    }
    await a.sendBytes(fill(size, 3));
    const fellBack = await until(() => tango.lanes().up === "fallback", 6000);
    step(`A frame 3 sent; up:post ${fellBack ? "failed, fallback" : "still " + tango.lanes().up}`);
    for (let k = 4; k < 8; k++) await a.sendBytes(fill(size, k));
    a.end();
    step("A frames 4-7 sent on the message channel, end");

    for (let k = 1; k < 6; k++) await b.sendBytes(fill(bSize(k), 1000 + k));
    b.end();
    step("B frames 1-5 sent");
    await Promise.all([aRead, bRead]);
    step(`A done (${r.a.frames} frames, ${r.a.err ? r.a.err.code : "no error"}), B done (${r.b.frames} frames)`);
    const after = await stats();
    r.upFrames = after.upFrames - before.upFrames;
    r.upRequests = after.upRequests - before.upRequests;
    r.upAccepted = after.upAccepted - before.upAccepted;
    r.lanesAfter = tango.lanes();
    r.ms = Math.round(performance.now() - t0);
    return verdict(r, [
      [r.lanesBefore.up === "up:post", `lanes before ${JSON.stringify(r.lanesBefore)}`],
      [fellBack, "up:post did not fail after the faulted packet"],
      [r.upFrames === 3, `upFrames ${r.upFrames}, want 3 (only the packets before the fault)`],
      [r.a.err && r.a.err.code === "transport", `stream A ended with ${JSON.stringify(r.a.err)}, want code transport`],
      [r.a.frames === 3 && r.a.bad === 0, `stream A echoed ${r.a.frames} frames (${r.a.bad} wrong), want frames 0-2`],
      [!r.b.err, `stream B failed: ${JSON.stringify(r.b.err)}`],
      [r.b.frames === 6 && r.b.bad === 0, `stream B echoed ${r.b.frames} frames (${r.b.bad} wrong), want 6`],
      [r.lanesAfter.up === "fallback", `lanes after ${JSON.stringify(r.lanesAfter)}`],
    ]);
  }

  // reloadStart is the first half of the reload check, run in the page that
  // is about to reload: it records the idle baseline (Debug.Stats), then
  // starts a Sink upload of 64 KiB frames that does not end on its own (up
  // to 1 GiB, not awaited; laneTests.uploaded counts the frames sent) and
  // returns once up:post requests are going. The caller then reloads the
  // page while the upload is in flight, and runs reloadCheck(result.base)
  // in the new page.
  async function reloadStart() {
    await until(() => tango.lanes().up === "up:post", 5000);
    await sleep(300);
    const base = await stats();
    const s = tango.stream("Sink");
    await s.accepted;
    const size = 64 * KiB;
    (async () => {
      for (let k = 0; k < 16384; k++) {
        await s.sendBytes(fill(size, k * size));
        window.laneTests.uploaded = k + 1;
      }
      s.end();
    })().catch(() => {});
    const going = await until(async () => {
      const st = await stats();
      return st.upRequests - base.upRequests >= 2 ? st : null;
    }, 5000, 10);
    return { base, mid: going || (await stats()), going: !!going, lanes: tango.lanes() };
  }

  // reloadCheck is the second half, in the new page: its lanes are
  // {down: fallback, up: up:post} again; within waitMs the Go goroutines
  // are back to the old page's baseline (±5), and no lane request of the
  // old page is still open in the scheme handler (upActive, downActive 0);
  // a 1 MiB Echo then uses the new page's up:post.
  async function reloadCheck(base, waitMs = 20000) {
    const t0 = Date.now();
    await until(() => tango.lanes().up === "up:post", 5000);
    const r = { lanes: tango.lanes() };
    let st = await until(async () => {
      const s = await stats();
      return s.goroutines <= base.goroutines + 5 && s.upActive === 0 && s.downActive === 0 ? s : null;
    }, waitMs, 100);
    r.settled = !!st;
    if (!st) st = await stats();
    r.settleMs = Date.now() - t0;
    r.goroutines = st.goroutines;
    r.baseGoroutines = base.goroutines;
    r.upActive = st.upActive;
    r.downActive = st.downActive;
    r.hellos = st.hellos - base.hellos;
    const size = 64 * KiB, n = 16;
    const e = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    r.echo = { frames: e.frames, bad: e.bad, err: e.err, upFrames: after.upFrames - st.upFrames };
    return verdict(r, [
      [r.lanes.down === "fallback" && r.lanes.up === "up:post", `lanes ${JSON.stringify(r.lanes)}`],
      [Math.abs(r.goroutines - r.baseGoroutines) <= 5, `goroutines ${r.goroutines}, baseline ${r.baseGoroutines} (±5)`],
      [r.upActive === 0, `${r.upActive} up:post requests still open`],
      [r.downActive === 0, `${r.downActive} down connections still open`],
      [r.hellos >= 1, `hellos +${r.hellos}: the new page did not shake hands`],
      [!e.err && e.frames === n && e.bad === 0, `echo after reload ${JSON.stringify(r.echo)}`],
      [r.echo.upFrames >= n / 2, `echo after reload: upFrames ${r.echo.upFrames}, want >= ${n / 2}`],
    ]);
  }

  window.laneTests = { bigBothWays, mixedOrder, stall, fault, reloadStart, reloadCheck, uploaded: 0 };
})();
