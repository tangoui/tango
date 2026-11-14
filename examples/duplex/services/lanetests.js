// IPC fast lane scenarios on a real WebView, shared by the macOS lane tests
// (internal/platform/darwin/lanes_flow_e2e_test.go, driving
// examples/duplex) and the iOS simulator self-test (examples/duplex-mobile
// web/selftest.js). TestRoutes serves this file as /__test/lanetests.js
// (TANGO_SCHEME_PROBE=1); a page loads it with loadLaneTests below or a
// <script> tag and gets window.laneTests.
//
// On Apple platforms the app runs with up:post on (the macOS default; the
// iOS self-test sets TANGO_IPC_LANES_DARWIN=1, iOS lanes being off by
// default): they offer only up:post (spec §6.2), so the lane assertions
// are about the upstream direction (Debug.Stats upFrames: frames that
// reached Go on the upstream lane). Downstream frames travel on the
// message channel; they are checked for being complete and in order only.
// On Android (examples/duplex-mobile with TANGO_IPC_LANES_ANDROID=1) the
// lanes are down:android and up:android: every byte frame takes them
// (minimum size 0), and Debug.Stats also counts lane messages each way
// (upRequests, downWrites) and the Android lane counters (lane*). The
// scenarios take the expected lanes from tango.lanes() when they start.
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

  const android = () => tango.lanes().up === "up:android";

  // bigBothWays: 8 MiB through Echo in 64 KiB frames (byte i of the stream
  // = i % 251), byte-exact both ways; at least half of the 128 frames
  // reached Go on the upstream lane. On Android at least 64 lane messages
  // each way (upRequests, downWrites).
  async function bigBothWays() {
    const size = 64 * KiB, n = 128, lanes = tango.lanes();
    const before = await stats();
    const r = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    r.lanes = lanes;
    r.upFrames = after.upFrames - before.upFrames;
    r.upRequests = after.upRequests - before.upRequests;
    r.downWrites = after.downWrites - before.downWrites;
    const conds = [
      [!r.err, "stream error " + JSON.stringify(r.err)],
      [r.frames === n, `echoed ${r.frames} frames, want ${n}`],
      [r.bytes === n * size, `echoed ${r.bytes} bytes, want ${n * size}`],
      [r.bad === 0, `${r.bad} frames differ (first ${r.firstBad})`],
      [r.upFrames >= n / 2, `upFrames ${r.upFrames}, want >= ${n / 2} (${lanes.up})`],
    ];
    if (lanes.up === "up:android") {
      conds.push([r.upRequests >= 64, `upRequests ${r.upRequests}, want >= 64`]);
      conds.push([r.downWrites >= 64, `downWrites ${r.downWrites}, want >= 64`]);
    }
    return verdict(r, conds);
  }

  // mixedOrder: 40 frames on one stream, alternately 100 B and 64 KiB,
  // echoed in send order. On up:post the 64 KiB frames (>= its 16 KiB
  // minimum) take the lane, the 100 B ones the message channel:
  // upFrames == 20. up:android has no minimum: upFrames == 40.
  async function mixedOrder() {
    const n = 40, sizeOf = (k) => (k % 2 === 0 ? 100 : 64 * KiB);
    const want = android() ? n : n / 2;
    const before = await stats();
    const r = await echoFrames(n, (k) => fill(sizeOf(k), k * 7), (k, m) => same(m, sizeOf(k), k * 7));
    const after = await stats();
    r.upFrames = after.upFrames - before.upFrames;
    return verdict(r, [
      [!r.err, "stream error " + JSON.stringify(r.err)],
      [r.frames === n, `echoed ${r.frames} frames, want ${n}`],
      [r.bad === 0, `${r.bad} frames out of order or wrong (first ${r.firstBad})`],
      [r.upFrames === want, `upFrames ${r.upFrames}, want ${want}`],
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
    const size = 64 * KiB, n = 64, up = tango.lanes().up;
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
      [r.upFrames >= 1, `upFrames ${r.upFrames}: ${up} was not used`],
      [r.lanes.up === up, `up lane after the stall is ${r.lanes.up}, want ${up}`],
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
  //
  // With lane "up:android" (TANGO_IPC_LANE_FAULT=up:android:3) the same,
  // except that up:android takes frames of any size: B's first (small)
  // frame is the first data packet, so A's frames 0-1 ride the lane and
  // frame 2's packet (the 4th) is refused; the page gets [1] and
  // up:android falls back within 1 s.
  async function fault(lane = "up:post") {
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
    // On a lane without a minimum size B's frame 0 is a data packet too.
    const bOnLane = lane === "up:android" &&
      !!(await until(async () => (await stats()).upFrames - before.upFrames >= 1, 5000));
    const aLane = bOnLane ? 2 : 3; // A's frames before the refused one
    r.bOnLane = bOnLane;

    const a = tango.stream("Echo");
    await a.accepted;
    const aRead = read(a, r.a, (k, m) => same(m, size, k));
    r.lanesBefore = tango.lanes();
    for (let k = 0; k < aLane; k++) {
      await a.sendBytes(fill(size, k));
      // One packet per frame: wait until Go got this one.
      const got = await until(async () => (await stats()).upFrames - before.upFrames >= (bOnLane ? 1 : 0) + k + 1, 5000);
      step(`A frame ${k} ${got ? "reached Go on " + lane : "did not reach Go within 5 s"}`);
    }
    const tFault = performance.now();
    await a.sendBytes(fill(size, aLane));
    const fellBack = await until(() => tango.lanes().up === "fallback", 6000, 10);
    r.fallMs = Math.round(performance.now() - tFault);
    step(`A frame ${aLane} sent; ${lane} ${fellBack ? "failed, fallback" : "still " + tango.lanes().up}`);
    for (let k = aLane + 1; k < 8; k++) await a.sendBytes(fill(size, k));
    a.end();
    step(`A frames ${aLane + 1}-7 sent on the message channel, end`);

    for (let k = 1; k < 6; k++) await b.sendBytes(fill(bSize(k), 1000 + k));
    b.end();
    step("B frames 1-5 sent");
    await Promise.all([aRead, bRead]);
    step(`A done (${r.a.frames} frames, ${r.a.err ? r.a.err.code : "no error"}), B done (${r.b.frames} frames)`);
    const after = await stats();
    r.upFrames = after.upFrames - before.upFrames;
    r.upRequests = after.upRequests - before.upRequests;
    r.upAccepted = after.upAccepted - before.upAccepted;
    r.downWrites = after.downWrites - before.downWrites;
    r.laneDownDropped = after.laneDownDropped - before.laneDownDropped;
    r.laneUpRefused = after.laneUpRefused - before.laneUpRefused;
    r.laneDownQueued = after.laneDownQueued;
    r.lanesAfter = tango.lanes();
    r.ms = Math.round(performance.now() - t0);
    return verdict(r, [
      [r.lanesBefore.up === lane, `lanes before ${JSON.stringify(r.lanesBefore)}`],
      [fellBack, `${lane} did not fail after the faulted packet`],
      [lane !== "up:android" || r.fallMs <= 1000, `${lane} fell back after ${r.fallMs} ms, want <= 1000`],
      [r.upFrames === 3, `upFrames ${r.upFrames}, want 3 (only the packets before the fault)`],
      [r.a.err && r.a.err.code === "transport", `stream A ended with ${JSON.stringify(r.a.err)}, want code transport`],
      [r.a.frames === aLane && r.a.bad === 0, `stream A echoed ${r.a.frames} frames (${r.a.bad} wrong), want frames 0-${aLane - 1}`],
      [!r.b.err, `stream B failed: ${JSON.stringify(r.b.err)}`],
      [r.b.frames === 6 && r.b.bad === 0, `stream B echoed ${r.b.frames} frames (${r.b.bad} wrong), want 6`],
      [r.lanesAfter.up === "fallback", `lanes after ${JSON.stringify(r.lanesAfter)}`],
    ]);
  }

  // faultDown needs TANGO_IPC_LANE_FAULT=down:android:3 (a tango_dev or
  // tango_selftest build): Go stops using down:android after 3 data
  // packets and sends the rest of the frames on the message channel. A
  // 1 MiB Echo (16 frames of 64 KiB) must come back whole and in order: a
  // refused down packet loses no frame. Exactly 3 down packets reached the
  // page on the lane (downWrites).
  async function faultDown() {
    const size = 64 * KiB, n = 16;
    const r = { lanesBefore: tango.lanes() };
    const before = await stats();
    const e = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    Object.assign(r, { frames: e.frames, bad: e.bad, err: e.err, ms: e.ms });
    r.downWrites = after.downWrites - before.downWrites;
    r.upFrames = after.upFrames - before.upFrames;
    return verdict(r, [
      [r.lanesBefore.down === "down:android", `lanes before ${JSON.stringify(r.lanesBefore)}`],
      [!e.err, "stream error " + JSON.stringify(e.err)],
      [e.frames === n && e.bad === 0, `echoed ${e.frames} frames (${e.bad} wrong), want ${n}`],
      [r.downWrites === 3, `downWrites ${r.downWrites}, want 3 (the packets before the fault)`],
    ]);
  }

  // sameIDStart is page A of reloadSameID (Android), run before the page
  // opened any stream or made any call, so that its first stream has the
  // same id as page B's: Source (stream 1) sends 32 MiB (byte j = j % 251)
  // and is read, while Sink (stream 3) takes 64 KiB uploads; it returns once
  // at least 1 MiB arrived. The caller reloads at once: the old page's
  // packets, either way, are then in flight.
  async function sameIDStart() {
    const size = 64 * KiB;
    const src = tango.stream("Source", 512, size, 251, 0);
    const sink = tango.stream("Sink");
    let got = 0;
    (async () => { for await (const m of src) got += m.length; })().catch(() => {});
    (async () => {
      await sink.accepted;
      for (let k = 0; k < 16384; k++) await sink.sendBytes(fill(size, k * size));
    })().catch(() => {});
    await until(() => got >= 1 << 20, 15000, 5);
    return { gotA: got };
  }

  // sameIDCheck is page B of reloadSameID, again before anything else: its
  // first stream (stream 1, as page A's Source) reads Source 4 MiB with
  // byte j = (j + 7) % 253, and an Echo stream (stream 3, as page A's
  // Sink) uploads 4 MiB with byte i = (i + 11) % 241; every byte of both
  // must be this page's. laneDownDropped and laneUpRefused show the old
  // page's packets that were dropped.
  async function sameIDCheck(a) {
    const size = 64 * KiB, n = 64;
    const r = { gotA: a && a.gotA };
    const src = tango.stream("Source", n, size, 253, 7);
    const pat = (off, len, mod, add) => {
      const b = new Uint8Array(len);
      for (let i = 0, v = (off + add) % mod; i < len; i++, v = v + 1 === mod ? 0 : v + 1) b[i] = v;
      return b;
    };
    const eq = (m, w) => m instanceof Uint8Array && m.length === w.length && m.every((v, i) => v === w[i]);
    const down = (async () => {
      let off = 0, bad = 0, frames = 0;
      for await (const m of src) {
        if (!eq(m, pat(off, m.length, 253, 7))) bad++;
        off += m.length;
        frames++;
      }
      return { bytes: off, frames, bad };
    })();
    const e = echoFrames(n, (k) => pat(k * size, size, 241, 11), (k, m) => eq(m, pat(k * size, size, 241, 11)));
    const [d, u] = await Promise.allSettled([down, e]);
    r.down = d.status === "fulfilled" ? d.value : { err: errOf(d.reason) };
    r.up = u.status === "fulfilled" ? { frames: u.value.frames, bad: u.value.bad, err: u.value.err } : { err: errOf(u.reason) };
    const st = await stats();
    r.laneDownDropped = st.laneDownDropped;
    r.laneUpRefused = st.laneUpRefused;
    r.lanes = tango.lanes();
    return verdict(r, [
      [!r.down.err && r.down.bytes === n * size && r.down.bad === 0, `Source on stream 1: ${JSON.stringify(r.down)}`],
      [!r.up.err && r.up.frames === n && r.up.bad === 0, `Echo on stream 3: ${JSON.stringify(r.up)}`],
      [r.lanes.up === "up:android" && r.lanes.down === "down:android", `lanes ${JSON.stringify(r.lanes)}`],
    ]);
  }

  // subframe (Android): a same-origin iframe (iframe.html) posts 50 well
  // formed lane packets with guessed tokens through its own
  // window.__tangoAndroid; an opaque-origin iframe (sandbox srcdoc) must
  // have no __tangoAndroid. Go must see none of them: upRequests and
  // laneUpBadToken do not move (Java drops messages from subframes). The
  // page's own lanes still carry a 1 MiB Echo afterwards.
  async function subframe() {
    const r = {};
    const before = await stats();
    const got = {};
    const onMsg = (e) => { if (e.data && e.data.kind) got[e.data.kind] = e.data; };
    window.addEventListener("message", onMsg);
    const sameFrame = document.createElement("iframe");
    sameFrame.src = "/iframe.html?n=50";
    const opaque = document.createElement("iframe");
    opaque.setAttribute("sandbox", "allow-scripts");
    opaque.srcdoc = "<script>parent.postMessage({kind:'opaque',has:typeof window.__tangoAndroid},'*')<\/script>";
    document.body.append(sameFrame, opaque);
    await until(() => got.same && got.opaque, 10000);
    await sleep(1000); // anything the iframe sent has reached Go by now
    window.removeEventListener("message", onMsg);
    sameFrame.remove();
    opaque.remove();
    const mid = await stats();
    r.same = got.same || null;
    r.opaque = got.opaque || null;
    r.upRequests = mid.upRequests - before.upRequests;
    r.badToken = mid.laneUpBadToken - before.laneUpBadToken;
    const size = 64 * KiB, n = 16;
    const e = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    r.echo = { frames: e.frames, bad: e.bad, err: e.err, upFrames: after.upFrames - mid.upFrames };
    return verdict(r, [
      [!!r.same && r.same.has === "object" && r.same.sent === 50, `same-origin iframe ${JSON.stringify(r.same)}`],
      [!!r.opaque && r.opaque.has === "undefined", `opaque iframe ${JSON.stringify(r.opaque)}`],
      [r.upRequests === 0, `upRequests +${r.upRequests} while the iframes posted`],
      [r.badToken === 0, `laneUpBadToken +${r.badToken}`],
      [!e.err && e.frames === n && e.bad === 0 && r.echo.upFrames >= n, `echo after ${JSON.stringify(r.echo)}`],
    ]);
  }

  // reloadStart is the first half of the reload check, run in the page that
  // is about to reload: it records the idle baseline (Debug.Stats), then
  // starts a Sink upload of 64 KiB frames that does not end on its own (up
  // to 1 GiB, not awaited; laneTests.uploaded counts the frames sent) and
  // returns once up:post requests are going. The caller then reloads the
  // page while the upload is in flight, and runs reloadCheck(result.base)
  // in the new page.
  //
  // On Android (down:android) a Source download (32 MiB) is going as well,
  // and the reload comes while its packets are on the way: reloadCheck then
  // also waits for the Android down queue to be empty (laneDownQueued).
  async function reloadStart() {
    await until(() => tango.lanes().up !== "fallback", 5000);
    await sleep(300);
    const base = await stats();
    const lanes = tango.lanes();
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
    if (lanes.down === "down:android") {
      const src = tango.stream("Source", 512, size);
      (async () => { for await (const m of src) window.laneTests.downloaded += m.length; })().catch(() => {});
    }
    const going = await until(async () => {
      const st = await stats();
      const up = st.upRequests - base.upRequests >= 2;
      const down = lanes.down !== "down:android" || st.downWrites - base.downWrites >= 2;
      return up && down ? st : null;
    }, 5000, 10);
    return { base, mid: going || (await stats()), going: !!going, lanes };
  }

  // reloadCheck is the second half, in the new page: its lanes are
  // base.lanes again (reloadStart's lanes; default {down: fallback, up:
  // up:post}); within waitMs the Go goroutines
  // are back to the old page's baseline (±5), and no lane request of the
  // old page is still open in the scheme handler (upActive, downActive 0);
  // a 1 MiB Echo then uses the new page's up:post.
  async function reloadCheck(base, waitMs = 20000) {
    const t0 = Date.now();
    const want = base.lanes || { down: "fallback", up: "up:post" };
    await until(() => tango.lanes().up === want.up, 5000);
    const r = { lanes: tango.lanes() };
    let st = await until(async () => {
      const s = await stats();
      return s.goroutines <= base.goroutines + 5 && s.upActive === 0 && s.downActive === 0 && s.laneDownQueued === 0 ? s : null;
    }, waitMs, 100);
    r.settled = !!st;
    if (!st) st = await stats();
    r.settleMs = Date.now() - t0;
    r.goroutines = st.goroutines;
    r.baseGoroutines = base.goroutines;
    r.upActive = st.upActive;
    r.downActive = st.downActive;
    r.laneDownQueued = st.laneDownQueued;
    r.hellos = st.hellos - base.hellos;
    const size = 64 * KiB, n = 16;
    const e = await echoFrames(n, (k) => fill(size, k * size), (k, m) => same(m, size, k * size));
    const after = await stats();
    r.echo = { frames: e.frames, bad: e.bad, err: e.err, upFrames: after.upFrames - st.upFrames };
    return verdict(r, [
      [r.lanes.down === want.down && r.lanes.up === want.up, `lanes ${JSON.stringify(r.lanes)}, want ${JSON.stringify(want)}`],
      [Math.abs(r.goroutines - r.baseGoroutines) <= 5, `goroutines ${r.goroutines}, baseline ${r.baseGoroutines} (±5)`],
      [r.upActive === 0, `${r.upActive} up:post requests still open`],
      [r.downActive === 0, `${r.downActive} down connections still open`],
      [r.laneDownQueued === 0, `${r.laneDownQueued} down:android packets still queued`],
      [r.hellos >= 1, `hellos +${r.hellos}: the new page did not shake hands`],
      [!e.err && e.frames === n && e.bad === 0, `echo after reload ${JSON.stringify(r.echo)}`],
      [r.echo.upFrames >= n / 2, `echo after reload: upFrames ${r.echo.upFrames}, want >= ${n / 2}`],
    ]);
  }

  window.laneTests = { bigBothWays, mixedOrder, stall, fault, faultDown, reloadStart, reloadCheck, sameIDStart, sameIDCheck, subframe,
    uploaded: 0, downloaded: 0 };
})();
