// Self-test of examples/duplex-mobile (iOS simulator, Android emulator;
// also runs on the desktop). Every result goes to Go through
// Selftest.Report(kind, result), which logs "kind: {json}" (on Android
// long results in "kind#n:" pieces, see main.go).
//
// The lanes the page must get depend on the platform (Debug.Platform):
// Apple platforms fallback / up:post (smoke.sh ios-selftest turns them on);
// Android down:android / up:android when TANGO_IPC_LANES_ANDROID is on,
// else fallback / fallback. The lane scenarios of lanetests.js (reload,
// 8 MiB, mixed, stall) run whenever the page has an upstream lane (Apple:
// up:post; Android: up:android); without lanes (Android off, capsOff) the
// checks are the scheme parity ones, stop, memory, lanes and echo_ipc.
// Android with the lanes adds lane_echo, reloadSameID and subframe:
//
//   index.html?selftest            page A of reloadSameID: before any call
//                                  or other stream, Source on stream 1 and
//                                  Sink on stream 3, reload mid-transfer as
//                                  ...&phase=b&sameA=...
//   ...&phase=b                    page B: Source on stream 1 and Echo on
//                                  stream 3 again, every byte its own; then
//                                  the reload scenario (reloadPhase) with
//                                  a Source download going as well
//   ...&reloaded=...               the rest, as on Apple platforms
//   index.html?selftest&lanefault=down|up
//                                  laneTests.faultDown / fault("up:android")
//                                  (TANGO_IPC_LANE_FAULT=down:android:3 /
//                                  up:android:3; scripts/smoke.sh
//                                  android-lanefault)
//
//   index.html         Echo round trip over an ipc stream ("selftest").
//   index.html?selftest
//                      Scheme-handler self-test (needs TANGO_SCHEME_PROBE=1
//                      for the /__test routes and Debug.Stats): the request
//                      sequence of internal/platform/darwin's
//                      TestAsyncSchemeParity ("parity", the raw results),
//                      then the checks against the expected answers, an
//                      aborted never-ending /__test/stream whose Go handler
//                      must return, the IPC fast lanes the page got
//                      (smoke.sh launches with TANGO_IPC_LANES_DARWIN=1,
//                      needed on iOS where lanes are off by default:
//                      tango.lanes() must be fallback / up:post, and a 1 MiB
//                      Echo must come back whole with at least half its
//                      frames on up:post), the lane scenarios of
//                      examples/duplex/services/lanetests.js (served as
//                      /__test/lanetests.js; the macOS lane tests run the
//                      same script): 8 MiB both ways, mixed sizes in order,
//                      a 6 s main-thread stall, and a reload during an
//                      up:post upload (the page first starts the upload and
//                      reloads itself as index.html?selftest&reloaded=
//                      <baseline>; the new page checks for leaks first and
//                      then runs the rest),
//                      and the Echo round trip ("selftest",
//                      ok = every check passed). scripts/smoke.sh
//                      ios-selftest also compares the index sha with
//                      web/index.html.
//   index.html?selftest&lanefault
//                      Only the lane fault scenario (laneTests.fault).
//                      main.go opens it when TANGO_IPC_LANE_FAULT is set;
//                      it needs a tango_dev build and
//                      TANGO_IPC_LANE_FAULT=up:post:3 ("lanefault").
//   index.html?bench   The duplex benchmark of /bench.js (window.duplexBench,
//                      shared with examples/duplex -bench; "bench").
//   index.html?probe   WebKit streaming probe ("probe"): fetches
//                      /__tango/probe/stream, which the iOS backend answers
//                      itself under TANGO_SCHEME_PROBE=1 (8 chunks of 1 KiB,
//                      200 ms apart), and records when each chunk reached
//                      the ReadableStream. Kept in step with
//                      examples/duplex/frontend/probe.html.
"use strict";

const out = document.getElementById("out");

async function runProbe() {
  const p = { chunks: [], done: false };
  window.__probe = p;
  const t0 = performance.now();
  try {
    const res = await fetch("/__tango/probe/stream", { cache: "no-store" });
    p.status = res.status;
    p.head = Math.round(performance.now() - t0); // response head reached the page
    const reader = res.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      p.chunks.push({ t: Math.round(performance.now() - t0), n: value.byteLength });
    }
  } catch (e) {
    p.error = String((e && e.message) || e);
  }
  p.bytes = p.chunks.reduce((a, c) => a + c.n, 0);
  p.done = true;
  return p;
}

// sha256 is a plain SHA-256 over a Uint8Array (crypto.subtle is not
// available on every custom-scheme page). Same as jsSHA256 in
// internal/platform/darwin/scheme_async_e2e_test.go.
function sha256(m){
  const K=new Uint32Array([0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2]);
  const H=new Uint32Array([0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19]);
  const n=m.length, total=((n+9+63)>>6)<<6, b=new Uint8Array(total);
  b.set(m); b[n]=0x80; const bits=n*8;
  b[total-4]=bits>>>24; b[total-3]=bits>>>16; b[total-2]=bits>>>8; b[total-1]=bits;
  b[total-5]=Math.floor(bits/4294967296);
  const W=new Uint32Array(64);
  for(let o=0;o<total;o+=64){
    for(let i=0;i<16;i++)W[i]=(b[o+4*i]<<24)|(b[o+4*i+1]<<16)|(b[o+4*i+2]<<8)|b[o+4*i+3];
    for(let i=16;i<64;i++){const x=W[i-15],y=W[i-2];
      const s0=((x>>>7)|(x<<25))^((x>>>18)|(x<<14))^(x>>>3), s1=((y>>>17)|(y<<15))^((y>>>19)|(y<<13))^(y>>>10);
      W[i]=(W[i-16]+s0+W[i-7]+s1)|0;}
    let a=H[0],c=H[2],d=H[3],e=H[4],f=H[5],g=H[6],h=H[7],bb=H[1];
    for(let i=0;i<64;i++){
      const S1=((e>>>6)|(e<<26))^((e>>>11)|(e<<21))^((e>>>25)|(e<<7)), ch=(e&f)^(~e&g);
      const t1=(h+S1+ch+K[i]+W[i])|0;
      const S0=((a>>>2)|(a<<30))^((a>>>13)|(a<<19))^((a>>>22)|(a<<10)), mj=(a&bb)^(a&c)^(bb&c);
      const t2=(S0+mj)|0;
      h=g;g=f;f=e;e=(d+t1)|0;d=c;c=bb;bb=a;a=(t1+t2)|0;}
    H[0]+=a;H[1]+=bb;H[2]+=c;H[3]+=d;H[4]+=e;H[5]+=f;H[6]+=g;H[7]+=h;
  }
  return Array.from(H,v=>v.toString(16).padStart(8,"0")).join("");
}

// fetchOne fetches url and describes the answer like the darwin parity
// test does.
async function fetchOne(name, url, init) {
  try {
    const r = await fetch(url, init);
    const b = new Uint8Array(await r.arrayBuffer());
    return { name, status: r.status, type: r.headers.get("content-type") || "",
      length: r.headers.get("content-length") || "", bytes: b.length, sha: sha256(b),
      text: new TextDecoder().decode(b.subarray(0, 64)) };
  } catch (e) { return { name, err: String((e && e.message) || e) }; }
}

// postBody is the 64 KiB POST of the parity sequence: byte i = (i*7+3) & 255.
function postBody() {
  const post = new Uint8Array(65536);
  for (let i = 0; i < post.length; i++) post[i] = (i * 7 + 3) & 255;
  return post;
}

// runParity is TestAsyncSchemeParity's request sequence, in its order.
async function runParity() {
  return [
    await fetchOne("index", "/index.html"),
    await fetchOne("big", "/__test/big"),
    await fetchOne("redirect", "/__test/redirect"),
    await fetchOne("head", "/index.html", { method: "HEAD" }),
    await fetchOne("echo", "/__test/echo", { method: "POST", body: postBody().buffer }),
    await fetchOne("panic", "/__test/panic"),
  ];
}

// runStop is TestSchemeStopCancelsHandler: read the first chunk of the
// never-ending /__test/stream, abort, and wait (≤ 10 s) for Debug.Stats to
// count the Go handler's return; then the page must still load assets.
async function runStop() {
  try {
    const before = await tango.call("Debug.Stats");
    const ac = new AbortController();
    const r = await fetch("/__test/stream", { signal: ac.signal });
    const rd = r.body.getReader();
    const first = await rd.read();
    const t0 = performance.now();
    ac.abort();
    let after;
    for (;;) {
      after = await tango.call("Debug.Stats");
      if (after.handlerExits > before.handlerExits || performance.now() - t0 > 10000) break;
      await new Promise(res => setTimeout(res, 20));
    }
    const wait = Math.round(performance.now() - t0);
    await new Promise(res => setTimeout(res, 500));
    const again = await fetch("/index.html", { cache: "no-store" });
    await again.arrayBuffer();
    return { before, after, chunk: first.value ? first.value.length : 0, wait, again: again.status };
  } catch (e) { return { err: String((e && e.message) || e) }; }
}

// checkScheme compares the results with the expected answers, computed the
// way TestAsyncSchemeParity computes them on the Go side. The index body is
// compared with web/index.html by scripts/smoke.sh (the page cannot read
// the file); here the redirect must equal it.
function checkScheme(parity, stop) {
  const by = {};
  for (const r of parity) by[r.name] = r;
  const big = new Uint8Array(3 << 20);
  for (let i = 0; i < big.length; i++) big[i] = i % 251;
  const want = (r, status, type) => !!r && !r.err && r.status === status && r.type.startsWith(type);
  const index = by.index;
  const checks = {
    index: want(index, 200, "text/html") && index.bytes > 0 && index.length === String(index.bytes),
    big: want(by.big, 200, "application/octet-stream") && by.big.length === "3145728" &&
      by.big.bytes === big.length && by.big.sha === sha256(big),
    redirect: want(by.redirect, 200, "text/html") && by.redirect.bytes === index.bytes && by.redirect.sha === index.sha,
    head: want(by.head, 200, "text/html") && by.head.bytes === 0 && by.head.length === index.length,
    echo: want(by.echo, 200, "application/octet-stream") && by.echo.bytes === 65536 && by.echo.sha === sha256(postBody()),
    panic: want(by.panic, 500, "text/plain") && by.panic.text.startsWith("tango: asset handler panic:"),
    // The darwin test allows 3 s on a loaded machine; same here.
    stop: !stop.err && stop.chunk > 0 && stop.after.handlerExits === stop.before.handlerExits + 1 &&
      stop.wait <= 3000 && stop.again === 200,
  };
  return checks;
}

// loadLaneTests loads the shared lane scenarios (window.laneTests).
function loadLaneTests() {
  return new Promise((res, rej) => {
    if (window.laneTests) return res(window.laneTests);
    const s = document.createElement("script");
    s.src = "/__test/lanetests.js";
    s.onload = () => res(window.laneTests);
    s.onerror = () => rej(new Error("cannot load /__test/lanetests.js"));
    document.head.appendChild(s);
  });
}

// reloadPhase is the first half of the reload scenario: it starts an
// up:post upload that does not end on its own and reloads the page while
// it is in flight, carrying the idle baseline in the URL
// (history.replaceState, then location.reload()). Returns only if the page
// did not reload.
async function reloadPhase(extra) {
  const lt = await loadLaneTests();
  const r = await lt.reloadStart();
  // Reload once another up:post request has started and one is open now
  // (upActive, recorded: reloadCheck's "nothing left open" needs it). On
  // Android: once more down:android packets arrived (the download is on
  // its way).
  const android = r.lanes.up === "up:android";
  let st = null;
  for (const t0 = Date.now(); Date.now() - t0 < 3000; await new Promise(res => setTimeout(res, 2))) {
    st = await tango.call("Debug.Stats");
    if (android ? st.downWrites > r.mid.downWrites : st.upRequests > r.mid.upRequests && st.upActive >= 1) break;
  }
  const base = { goroutines: r.base.goroutines, hellos: r.base.hellos, lanes: r.lanes, going: r.going, uploaded: lt.uploaded,
    downloaded: lt.downloaded, upActiveAtReload: st ? st.upActive : 0 };
  let q = location.search + "&reloaded=" + encodeURIComponent(JSON.stringify(base));
  if (extra) q += "&sameid=" + encodeURIComponent(JSON.stringify(extra));
  history.replaceState(null, "", location.pathname + q);
  location.reload();
  await new Promise(res => setTimeout(res, 30000));
  return { ok: false, failed: ["reload"], err: "location.reload() did not reload the page" };
}

// laneScenario runs one laneTests scenario; it never throws.
async function laneScenario(fn) {
  try { return await fn(); } catch (e) { return { ok: false, why: ["threw: " + String((e && e.message) || e)] }; }
}

// platform returns Debug.Platform and the lanes the page must get.
async function platform() {
  const p = await tango.call("Debug.Platform");
  let want = { down: "fallback", up: "up:post" };
  if (p.goos === "android") want = p.androidLanes ? { down: "down:android", up: "up:android" } : { down: "fallback", up: "fallback" };
  return { ...p, want };
}

async function runSelftest(reloaded, plat, sameID) {
  const apple = plat.goos !== "android";
  const androidLanes = !apple && plat.want.up === "up:android";
  let lt = null, laneReload = null;
  if (reloaded) {
    // The second half of the reload scenario comes first, before anything
    // else runs on this page: no lane request of the old page still open,
    // goroutines back to its baseline, up:post working again.
    await runLanes();
    lt = await loadLaneTests();
    laneReload = await laneScenario(() => lt.reloadCheck(reloaded, 20000));
    laneReload.before = reloaded;
  }
  const parity = await runParity();
  // The raw results first, in case a later step hangs.
  try { await tango.call("Selftest.Report", "parity", parity.map(r => ({ ...r, text: undefined }))); } catch (e) {}
  const stop = await runStop();
  const checks = checkScheme(parity, stop);
  const memory = await runMemory();
  checks.memory = !memory.err && memory.before > 0 && memory.growth < memoryLimit;
  const lanes = await runLanes();
  checks.lanes = lanes.down === plat.want.down && lanes.up === plat.want.up;
  const withLanes = plat.want.up !== "fallback";
  const laneEcho = checks.lanes && withLanes ? await runLaneEcho() : null;
  if (withLanes) {
    const echoed = !!laneEcho && !laneEcho.err && laneEcho.frames === 16 && laneEcho.bad === 0;
    checks.lane_echo = apple ? echoed && laneEcho.upFrames >= 8
      // Android: every frame on up:android, every echo frame on
      // down:android (upRequests, downWrites: lane messages each way).
      : echoed && laneEcho.upRequests >= 16 && laneEcho.downWrites >= 16 &&
        laneEcho.after.laneUpProbes >= 1 && laneEcho.after.laneUpBadToken === 0;
  }
  let laneBig = null, laneMixed = null, laneStall = null, laneSubframe = null;
  if (apple || androidLanes) {
    checks.lane_reload = !!reloaded && !!reloaded.going && (androidLanes || reloaded.upActiveAtReload >= 1) &&
      !!laneReload && laneReload.ok === true;
    if (checks.lanes) {
      laneBig = await laneScenario(() => lt.bigBothWays());
      laneMixed = await laneScenario(() => lt.mixedOrder());
      laneStall = await laneScenario(() => lt.stall(6000));
    }
    checks.lane_big = !!laneBig && laneBig.ok === true;
    checks.lane_mixed = !!laneMixed && laneMixed.ok === true;
    checks.lane_stall = !!laneStall && laneStall.ok === true;
  }
  if (androidLanes) {
    checks.lane_reload_same_id = !!sameID && sameID.ok === true;
    if (checks.lanes) laneSubframe = await laneScenario(() => lt.subframe());
    checks.lane_subframe = !!laneSubframe && laneSubframe.ok === true;
  }
  const echo = await runEcho();
  checks.echo_ipc = !!echo.echo && echo.echo.ping === "selftest";
  const failed = Object.keys(checks).filter(k => !checks[k]);
  return { ok: failed.length === 0, failed, checks, platform: plat, indexSha: parity[0].sha, indexBytes: parity[0].bytes, stop, memory, lanes, laneEcho,
    laneReload, laneBig, laneMixed, laneStall, laneSameID: sameID || null, laneSubframe };
}

// runLaneFault is index.html?selftest&lanefault: laneTests.fault only.
// runLaneFault is index.html?selftest&lanefault[=down|up]: up:post's fault
// scenario, or on Android faultDown (=down) / fault("up:android") (=up).
async function runLaneFault(which) {
  const lanes = await runLanes();
  const lt = await loadLaneTests();
  if (which === "down") {
    const fault = await laneScenario(() => lt.faultDown());
    return { ok: lanes.down === "down:android" && fault.ok === true, lanes, fault };
  }
  const lane = which === "up" ? "up:android" : "up:post";
  const fault = await laneScenario(() => lt.fault(lane));
  return { ok: lanes.up === lane && fault.ok === true, lanes, fault };
}

// androidLanesUp waits up to 5 s for down:android and up:android, without
// a call or a stream (reloadSameID needs this page's first stream id).
async function androidLanesUp() {
  if (!/Android/.test(navigator.userAgent)) return false;
  const t0 = Date.now();
  const on = () => tango.lanes().up === "up:android" && tango.lanes().down === "down:android";
  while (!on() && Date.now() - t0 < 5000) await new Promise(res => setTimeout(res, 20));
  return on();
}

// compact keeps what the final report needs of a scenario result (it rides
// in the URL across a reload).
function compact(r) {
  const { ok, why, gotA, down, up, laneDownDropped, laneUpRefused, lanes } = r;
  return { ok, why, gotA, down, up, laneDownDropped, laneUpRefused, lanes };
}

// runLanes waits up to 5 s for the upstream lane to become ready, then
// 500 ms more (a downstream lane, if one were offered, would be ready too),
// and returns tango.lanes() ("fallback": none). Apple platforms offer
// up:post only (WebKit holds back large down:scheme bursts, spec §6.2).
async function runLanes() {
  const t0 = Date.now();
  while (tango.lanes().up === "fallback" && Date.now() - t0 < 5000) await new Promise(res => setTimeout(res, 50));
  await new Promise(res => setTimeout(res, 500));
  return tango.lanes();
}

// runLaneEcho sends 16 byte frames of 64 KiB through Echo after Go accepted
// the stream (earlier byte frames take the message channel) and checks
// every byte that comes back; Debug.Stats shows how many frames went
// through up:post (the echo comes back on the message channel).
async function runLaneEcho() {
  const size = 64 * 1024, n = 16, t0 = Date.now();
  let k = 0, bad = 0, err = "";
  const before = await tango.call("Debug.Stats");
  try {
    const s = tango.stream("Echo");
    await s.accepted;
    for (let j = 0; j < n; j++) {
      const b = new Uint8Array(size);
      for (let i = 0; i < size; i++) b[i] = (i + j) % 251;
      await s.sendBytes(b);
    }
    s.end();
    for await (const m of s) {
      if (!(m instanceof Uint8Array) || m.length !== size) bad++;
      else for (let i = 0; i < size; i++) if (m[i] !== (i + k) % 251) { bad++; break; }
      k++;
    }
  } catch (e) { err = String((e && e.message) || e); }
  const after = await tango.call("Debug.Stats");
  const d = (f) => after[f] - before[f];
  return { frames: k, bad, err, ms: Date.now() - t0, upFrames: d("upFrames"), downWrites: d("downWrites"),
    upRequests: d("upRequests"), after: { laneUpProbes: after.laneUpProbes, laneUpBadToken: after.laneUpBadToken } };
}

// runMemory streams memoryRounds × 3 MiB (/__test/big) through the scheme
// handler and reports the app process's phys_footprint (Debug.Footprint)
// before and after. The copies the backend makes of each chunk must not
// pile up (e.g. autoreleased on a thread without a pool): growth must stay
// well under what was streamed. The web content process holds the bytes
// the page reads; it is not this process.
const memoryRounds = 22; // 66 MiB
const memoryLimit = 24 << 20;

async function runMemory() {
  try {
    const drain = async () => { const r = await fetch("/__test/big", { cache: "no-store" }); return (await r.arrayBuffer()).byteLength; };
    await drain(); // warm up: first-use allocations are not growth
    const before = await tango.call("Debug.Footprint");
    let streamed = 0;
    for (let i = 0; i < memoryRounds; i++) streamed += await drain();
    await new Promise(res => setTimeout(res, 300));
    const after = await tango.call("Debug.Footprint");
    return { before, after, growth: after - before, streamed };
  } catch (e) { return { err: String((e && e.message) || e) }; }
}

async function runEcho() {
  const s = tango.stream("Echo");
  s.send({ ping: "selftest" });
  s.end();
  let got;
  for await (const m of s) got = m;
  return { echo: got };
}

// runBench loads /bench.js and runs the duplex benchmark.
function runBench() {
  return new Promise((res, rej) => {
    const s = document.createElement("script");
    s.src = "/bench.js";
    s.onload = () => res(window.duplexBench.run((line) => { out.textContent += "\n" + line; }));
    s.onerror = () => rej(new Error("cannot load /bench.js"));
    document.head.appendChild(s);
  });
}

(async () => {
  const q = new URLSearchParams(location.search);
  const kind = q.has("probe") ? "probe" : q.has("lanefault") ? "lanefault" : q.has("bench") ? "bench" : "selftest";
  let res;
  try {
    if (kind === "probe") res = await runProbe();
    else if (kind === "bench") res = await runBench();
    else if (kind === "lanefault") res = await runLaneFault(q.get("lanefault"));
    else if (!q.has("selftest")) res = await runEcho();
    else if (q.has("reloaded")) {
      const sameID = q.has("sameid") ? JSON.parse(q.get("sameid")) : null;
      res = await runSelftest(JSON.parse(q.get("reloaded")), await platform(), sameID);
    } else if (q.get("phase") === "b") {
      // reloadSameID page B, then the reload scenario (reloads the page).
      await androidLanesUp();
      const lt = await loadLaneTests();
      const same = await laneScenario(() => lt.sameIDCheck(JSON.parse(q.get("sameA") || "null")));
      res = await reloadPhase(compact(same));
    } else if (await androidLanesUp()) {
      // reloadSameID page A: reloads the page mid-transfer.
      const lt = await loadLaneTests();
      const a = await lt.sameIDStart();
      history.replaceState(null, "", location.pathname + location.search + "&phase=b&sameA=" + encodeURIComponent(JSON.stringify(a)));
      location.reload();
      await new Promise(res => setTimeout(res, 30000));
      res = { ok: false, failed: ["reload"], err: "location.reload() did not reload the page" };
    } else {
      const plat = await platform();
      if (plat.goos === "android") res = await runSelftest(null, plat);
      else res = await reloadPhase(); // reloads the page
    }
  } catch (e) {
    res = { error: String((e && e.message) || e) };
  }
  out.textContent = kind + ": " + JSON.stringify(res, null, 1);
  try {
    await tango.call("Selftest.Report", kind, res);
  } catch (e) {
    out.textContent += "\nreport failed: " + e;
  }
})();
