#!/usr/bin/env node
// Monaco same-scenario baseline for the Tango editor (spec 6.6), driven over
// the Chrome DevTools Protocol with Node's built-in WebSocket. See README.md.
//
//   node tools/monaco-bench/bench.mjs --self-test
//   node tools/monaco-bench/bench.mjs --scenario <open|type|scroll|ts20k|memory|all> --runs 5 \
//        [--variant default|nolfo] [--out results.json]
//
// No packages, no network: a 127.0.0.1 static server (server.mjs) and one
// headless Edge with a fresh profile, both torn down on exit, error or signal.

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { startServer, defaultDataDir } from './server.mjs';

const DEFAULT_MONACO = '/Users/admin/min/mincode/vue-app/node_modules/monaco-editor';
const DEFAULT_EDGE = '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge';
const FILE_1M = 'gen1000000-15d1551d.ts'; // ui/editor/perf_test.go perfFile(1_000_000)
const FILE_20K = 'gen20000-15d1551d.ts'; //  ui/editor/perf_test.go perfFile(20_000)

// ---------------------------------------------------------------- arguments
const argv = process.argv.slice(2);
const flag = (k) => argv.includes(k);
const opt = (k, d) => { const i = argv.indexOf(k); return i >= 0 && i + 1 < argv.length ? argv[i + 1] : d; };
const cfg = {
  selfTest: flag('--self-test'),
  monaco: opt('--monaco', DEFAULT_MONACO),
  edge: opt('--edge', DEFAULT_EDGE),
  data: opt('--data', defaultDataDir),
  scenario: opt('--scenario', 'all'),
  runs: Number(opt('--runs', 5)),
  variant: opt('--variant', 'default'), // default | nolfo (largeFileOptimizations:false)
  out: opt('--out', ''),
  maxLoad: Number(opt('--max-load', 6)),
  loadBudgetMin: Number(opt('--load-wait-min', 30)),
  uncapped: !flag('--capped'), // add --disable-frame-rate-limit/--disable-gpu-vsync
  keys: Number(opt('--keys', 300)),
  frames: Number(opt('--frames', 300)),
  typeLine: Number(opt('--type-line', 500_001)), // 1-based: Tango's 0-based line 500 000
  scrollLine: Number(opt('--scroll-line', 100_001)),
  file1m: opt('--file', FILE_1M),
  screenshot: opt('--screenshot', ''),
  verbose: flag('-v'),
};
if (argv.includes('-h') || argv.includes('--help')) {
  console.log(fs.readFileSync(new URL('./README.md', import.meta.url), 'utf8'));
  process.exit(0);
}
const log = (...a) => console.error('[monaco-bench]', ...a);

// ------------------------------------------------------------------ cleanup
const state = { edge: null, tmp: null, server: null, ws: null };
let cleaned = false;
// stopEdgeSync kills the Edge process group and removes its profile dir.
function stopEdgeSync() {
  try { state.ws?.close(); } catch {}
  state.ws = null;
  if (state.edge && state.edge.exitCode === null) {
    try { process.kill(-state.edge.pid, 'SIGKILL'); } catch {}
    try { state.edge.kill('SIGKILL'); } catch {}
  }
  state.edge = null;
  if (state.tmp) {
    // Edge may still be writing for a moment after SIGKILL of the group.
    for (let i = 0; i < 20; i++) {
      try { fs.rmSync(state.tmp, { recursive: true, force: true }); break; } catch { execFileSync('/bin/sleep', ['0.1']); }
    }
  }
  state.tmp = null;
}
function cleanupSync() {
  if (cleaned) return;
  cleaned = true;
  stopEdgeSync();
  try { state.server?.server.closeAllConnections?.(); state.server?.server.close(); } catch {}
}
process.on('exit', cleanupSync);
for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
  process.on(sig, () => { log(`${sig}: cleaning up`); cleanupSync(); process.exit(130); });
}
process.on('uncaughtException', (e) => { console.error(e); cleanupSync(); process.exit(1); });
process.on('unhandledRejection', (e) => { console.error(e); cleanupSync(); process.exit(1); });

// --------------------------------------------------------------------- Edge
export const EDGE_FLAGS = [
  '--headless=new',
  '--remote-debugging-port=0',
  '--window-size=1920,1080',
  '--force-device-scale-factor=2',
  '--no-first-run',
  '--no-default-browser-check',
  '--disable-extensions',
  '--disable-background-timer-throttling',
  '--disable-renderer-backgrounding',
  '--disable-backgrounding-occluded-windows',
  // keep the browser off the network (updates, sync, telemetry pings)
  '--disable-background-networking',
  '--disable-component-update',
  '--disable-sync',
  '--disable-default-apps',
  '--no-pings',
  '--metrics-recording-only',
];
const UNCAPPED_FLAGS = ['--disable-frame-rate-limit', '--disable-gpu-vsync'];

async function launchEdge() {
  state.tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'monaco-bench-'));
  const flags = [...EDGE_FLAGS, ...(cfg.uncapped ? UNCAPPED_FLAGS : []), `--user-data-dir=${state.tmp}`];
  const errLog = fs.openSync(path.join(state.tmp, 'edge-stderr.log'), 'w');
  state.edge = spawn(cfg.edge, [...flags, 'about:blank'], { detached: true, stdio: ['ignore', 'ignore', errLog] });
  state.edge.on('exit', (c, s) => cfg.verbose && log('edge exited', c, s));
  const portFile = path.join(state.tmp, 'DevToolsActivePort');
  for (let i = 0; i < 300; i++) {
    if (fs.existsSync(portFile)) {
      const [port] = fs.readFileSync(portFile, 'utf8').split('\n');
      if (port) return { port: Number(port), flags };
    }
    if (state.edge.exitCode !== null) break;
    await sleep(100);
  }
  throw new Error('Edge did not write DevToolsActivePort; stderr: ' + safeRead(path.join(state.tmp, 'edge-stderr.log')));
}
const safeRead = (f) => { try { return fs.readFileSync(f, 'utf8').slice(-2000); } catch { return ''; } };

// ---------------------------------------------------------------------- CDP
class CDP {
  constructor(url) {
    this.id = 0;
    this.pending = new Map();
    this.listeners = new Map();
    this.ws = new WebSocket(url);
    this.ws.onmessage = (ev) => {
      const m = JSON.parse(ev.data);
      if (m.id !== undefined) {
        const p = this.pending.get(m.id);
        if (!p) return;
        this.pending.delete(m.id);
        if (m.error) p.reject(new Error(`${p.method}: ${m.error.message}`));
        else p.resolve(m.result);
      } else if (m.method) {
        for (const fn of this.listeners.get(m.method) || []) fn(m.params);
      }
    };
    this.opened = new Promise((res, rej) => { this.ws.onopen = res; this.ws.onerror = (e) => rej(new Error('websocket error ' + (e.message || ''))); });
    this.ws.onclose = () => { for (const p of this.pending.values()) p.reject(new Error('websocket closed')); this.pending.clear(); };
  }
  send(method, params = {}) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject, method });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }
  on(method, fn) { (this.listeners.get(method) || this.listeners.set(method, []).get(method)).push(fn); }
  once(method, timeoutMs = 60000) {
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error('timeout waiting for ' + method)), timeoutMs);
      const fn = (p) => { clearTimeout(t); this.listeners.set(method, this.listeners.get(method).filter((f) => f !== fn)); resolve(p); };
      this.on(method, fn);
    });
  }
  close() { this.ws.close(); }
}

async function evaluate(cdp, expression, timeoutMs = 600000) {
  const r = await cdp.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, timeout: timeoutMs });
  if (r.exceptionDetails) {
    const d = r.exceptionDetails;
    throw new Error(`page: ${d.exception?.description || d.text}`);
  }
  return r.result.value;
}

async function connectPage(port) {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const page = list.find((t) => t.type === 'page');
      if (page) {
        const cdp = new CDP(page.webSocketDebuggerUrl);
        await cdp.opened;
        state.ws = cdp.ws;
        return cdp;
      }
    } catch {}
    await sleep(100);
  }
  throw new Error('no page target');
}

const withTimeout = (p, ms, what) => Promise.race([p, sleep(ms).then(() => { throw new Error('timeout: ' + what); })]);

async function loadPage(cdp, url) {
  const loaded = cdp.once('Page.loadEventFired');
  loaded.catch(() => {}); // awaited below; never an unhandled rejection
  await withTimeout(cdp.send('Page.navigate', { url }), 60000, 'Page.navigate');
  await loaded;
  await withTimeout(evaluate(cdp, 'bench.ready'), 60000, 'bench.ready');
}

// --------------------------------------------------------------- statistics
const sorted = (a) => [...a].sort((x, y) => x - y);
// pct uses the Tango fixture's index rule: sorted[ceil(p*n)-1] (p=1 → max).
function pct(a, p) {
  if (!a.length) return null;
  const s = sorted(a);
  const i = Math.min(s.length - 1, Math.max(0, Math.ceil(p * s.length) - 1));
  return s[i];
}
const mean = (a) => (a.length ? a.reduce((x, y) => x + y, 0) / a.length : null);
const median = (a) => {
  const s = sorted(a.filter((x) => x !== null && x !== undefined));
  if (!s.length) return null;
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
};
const r3 = (x) => (x === null || x === undefined ? null : Math.round(x * 1000) / 1000);

// --------------------------------------------------------------------- load
const loadStart = Date.now();
const load1 = () => os.loadavg()[0];
// waitLoad waits (15 s steps) until the 1-minute load is below the limit;
// false once the overall waiting budget is spent.
let waitedMs = 0;
async function waitLoad() {
  while (load1() >= cfg.maxLoad) {
    if (waitedMs >= cfg.loadBudgetMin * 60000) return false;
    log(`load ${load1().toFixed(2)} >= ${cfg.maxLoad}: waiting (waited ${(waitedMs / 60000).toFixed(1)} min)`);
    await sleep(15000);
    waitedMs += 15000;
  }
  return true;
}
const uptime = () => { try { return execFileSync('/usr/bin/uptime', { encoding: 'utf8' }).trim(); } catch { return ''; } };

// ------------------------------------------------------------ process stats
// rendererRSS returns the renderer processes of our Edge (by user-data-dir)
// with RSS in MB (ps) and phys_footprint (footprint(1), when it works).
function rendererStats() {
  let out = '';
  try { out = execFileSync('/bin/ps', ['-axww', '-o', 'pid=,rss=,command='], { encoding: 'utf8', maxBuffer: 64 << 20 }); } catch { return []; }
  const procs = [];
  for (const line of out.split('\n')) {
    if (!line.includes(state.tmp) || !line.includes('--type=renderer')) continue;
    const m = line.trim().match(/^(\d+)\s+(\d+)\s/);
    if (m) procs.push({ pid: Number(m[1]), rssMB: Number(m[2]) * 1024 / 1e6 });
  }
  return procs.sort((a, b) => b.rssMB - a.rssMB);
}
function footprintMB(pid) {
  try {
    const o = execFileSync('/usr/bin/footprint', ['-p', String(pid)], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
    const m = o.match(/Footprint:\s*([\d.]+)\s*([KMG]B)/);
    if (!m) return null;
    const v = Number(m[1]);
    return m[2] === 'KB' ? v * 1024 / 1e6 : m[2] === 'MB' ? v * 1048576 / 1e6 : v * 1073741824 / 1e6;
  } catch { return null; }
}

// ---------------------------------------------------------------- scenarios
const lfo = () => (cfg.variant === 'nolfo' ? false : undefined);

async function openDoc(cdp, url, file, extra = {}) {
  await loadPage(cdp, url);
  await cdp.send('HeapProfiler.collectGarbage');
  const fetchRes = await evaluate(cdp, `bench.fetchText(${JSON.stringify(file)})`);
  const o = { language: 'typescript', largeFileOptimizations: lfo(), name: file.replace(/\W/g, '_'), ...extra };
  // JSON drops undefined: the default variant leaves Monaco's default.
  const res = await evaluate(cdp, `bench.open(${JSON.stringify(o)})`);
  res.fetchMs = fetchRes.ms;
  res.chars = fetchRes.chars;
  return res;
}

const scenarios = {
  async open(cdp, url) {
    const r = await openDoc(cdp, url, cfg.file1m, { tokenTimeoutMs: 5000 });
    return {
      fetchMs: r3(r.fetchMs), createModelMs: r3(r.createModelMs), createEditorMs: r3(r.createEditorMs),
      editableMs: r3(r.editableMs), tokensMs: r3(r.tokensMs), frames: r.frames, chars: r.chars,
      dpr: r.dpr, layout: r.layout, degrade: r.degrade,
    };
  },

  async type(cdp, url) {
    const o = await openDoc(cdp, url, cfg.file1m, { tokenTimeoutMs: 3000 });
    const prep = await evaluate(cdp, `bench.prepareType(${cfg.typeLine})`);
    if (!prep.focused) throw new Error('editor not focused: ' + JSON.stringify(prep));
    for (let i = 0; i < cfg.keys; i++) {
      const enter = i % 40 === 39; // perf_test.go keyAt
      const k = enter
        ? { key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, nativeVirtualKeyCode: 13, text: '\r', unmodifiedText: '\r' }
        : { key: 'x', code: 'KeyX', windowsVirtualKeyCode: 88, nativeVirtualKeyCode: 88, text: 'x', unmodifiedText: 'x' };
      await cdp.send('Input.dispatchKeyEvent', { type: 'keyDown', ...k });
      await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: k.key, code: k.code, windowsVirtualKeyCode: k.windowsVirtualKeyCode, nativeVirtualKeyCode: k.nativeVirtualKeyCode });
      const n = await evaluate(cdp, `bench.waitSamples(${i + 1}, 5000)`);
      if (n < i + 1) throw new Error(`key ${i}: no sample (model did not change?)`);
    }
    const res = await evaluate(cdp, 'bench.typeResult()');
    const ms = res.samples.map((s) => s.ms);
    const want = prep.lineCount + Math.floor(cfg.keys / 40);
    if (res.lineCount !== want) throw new Error(`line count ${res.lineCount}, want ${want}`);
    return {
      line: cfg.typeLine, keys: ms.length, lineCountOk: true, openEditableMs: r3(o.editableMs), degrade: o.degrade,
      p50: r3(pct(ms, 0.5)), p95: r3(pct(ms, 0.95)), max: r3(pct(ms, 1)), mean: r3(mean(ms)),
      workP50: r3(pct(res.samples.map((s) => s.workMs), 0.5)), workP95: r3(pct(res.samples.map((s) => s.workMs), 0.95)),
      changeP50: r3(pct(res.samples.map((s) => s.changeMs), 0.5)),
      rafP50: r3(pct(res.samples.map((s) => s.rafMs), 0.5)),
    };
  },

  async scroll(cdp, url) {
    const o = await openDoc(cdp, url, cfg.file1m, { tokenTimeoutMs: 3000 });
    const idle = await evaluate(cdp, 'bench.idleFrames(60)');
    const s = await evaluate(cdp, `bench.scroll(${cfg.scrollLine}, ${cfg.frames}, 2)`);
    const moved = (s.top1 - s.top0) / s.lineHeight;
    if (moved < cfg.frames * 2 - 1) throw new Error(`scrolled ${moved} lines, want ${cfg.frames * 2}`);
    return {
      startLine: cfg.scrollLine, frames: s.intervals.length, linesMoved: moved, firstVisible: s.firstVisible,
      fps: r3(1000 / mean(s.intervals)), meanMs: r3(mean(s.intervals)), p99: r3(pct(s.intervals, 0.99)), max: r3(pct(s.intervals, 1)),
      workMean: r3(mean(s.work)), workP99: r3(pct(s.work, 0.99)),
      idleIntervalMedian: r3(median(idle)), degrade: o.degrade,
    };
  },

  async ts20k(cdp, url) {
    // (a) background tokenization after opening, nothing else going on
    const o = await openDoc(cdp, url, FILE_20K, { tokenTimeoutMs: 10000, keepText: true });
    const bg = await evaluate(cdp, 'bench.waitBgDone(60000)');
    // open() ends after the first fully-tokenized screen; bg time counts
    // from createModel: open's duration (editableMs..tokensMs frames) + wait.
    const openDur = o.tokensMs ?? o.editableMs;
    // (b) synchronous forceTokenization(lineCount) of a detached model
    const force = await evaluate(cdp, 'bench.forceTokenizeDetached()');
    // (c) background tokenization while typing at line 11 (Tango: line 10,
    // 0-based), one key per >= 16 ms, then 100 keys after it finished
    await openDoc(cdp, url, FILE_20K, { tokenTimeoutMs: 10000 });
    await evaluate(cdp, 'bench.prepareType(11)');
    let i = 0;
    let during = 0;
    for (;; i++) {
      const t = Date.now();
      const enter = i % 40 === 39;
      const k = enter
        ? { key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, text: '\r' }
        : { key: 'x', code: 'KeyX', windowsVirtualKeyCode: 88, text: 'x' };
      await cdp.send('Input.dispatchKeyEvent', { type: 'keyDown', ...k });
      await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: k.key, code: k.code, windowsVirtualKeyCode: k.windowsVirtualKeyCode });
      await evaluate(cdp, `bench.waitSamples(${i + 1}, 5000)`);
      if (!during) {
        if (await evaluate(cdp, 'bench.bgDone()')) during = i + 1;
        else if (Date.now() - t < 16) await sleep(16 - (Date.now() - t));
      }
      if (during && i + 1 >= during + 100) break;
      if (i > 20000) throw new Error('ts20k: background tokenization never finished while typing');
    }
    const res = await evaluate(cdp, 'bench.typeResult()');
    const d = res.samples.slice(0, during).map((s) => s.ms);
    const a = res.samples.slice(during).map((s) => s.ms);
    return {
      openEditableMs: r3(o.editableMs), firstScreenTokensMs: r3(o.tokensMs),
      bgFromCreateMs: r3(openDur + bg.ms), bgDone: bg.done, bgWaitAfterFirstScreenMs: r3(bg.ms),
      forceMs: r3(force.ms), forceLines: force.lineCount, forceLastAccurate: force.lastAccurate,
      keysDuringBg: d.length, duringP50: r3(pct(d, 0.5)), duringP95: r3(pct(d, 0.95)), duringMax: r3(pct(d, 1)),
      duringWorkP95: r3(pct(res.samples.slice(0, during).map((s) => s.workMs), 0.95)), afterWorkP95: r3(pct(res.samples.slice(during).map((s) => s.workMs), 0.95)),
      afterP95: r3(pct(a, 0.95)), afterKeys: a.length,
      degrade: o.degrade,
    };
  },

  async memory(cdp, url) {
    await loadPage(cdp, url);
    await cdp.send('Performance.enable');
    const heap = async () => {
      await cdp.send('HeapProfiler.collectGarbage');
      await cdp.send('HeapProfiler.collectGarbage');
      const m = await cdp.send('Performance.getMetrics');
      const g = (n) => m.metrics.find((x) => x.name === n)?.value;
      const u = await cdp.send('Runtime.getHeapUsage');
      // Runtime.getHeapUsage: the page isolate, exact after GC;
      // Performance.getMetrics JSHeapUsedSize can lag (sampled).
      return { used: u.usedSize / 1e6, total: u.totalSize / 1e6, metricsUsed: g('JSHeapUsedSize') / 1e6, raw: u };
    };
    const proc = () => {
      const r = rendererStats()[0];
      return r ? { pid: r.pid, rssMB: r.rssMB, footprintMB: footprintMB(r.pid), renderers: rendererStats().length } : null;
    };
    const h0 = await heap();
    const p0 = proc();
    const fetchRes = await evaluate(cdp, `bench.fetchText(${JSON.stringify(cfg.file1m)})`);
    const o = await evaluate(cdp, `bench.open(${JSON.stringify({ language: 'typescript', name: 'mem', tokenTimeoutMs: 3000, largeFileOptimizations: lfo() })})`);
    for (let i = 0; i < 10; i++) await evaluate(cdp, 'bench.afterFrame()');
    const h1 = await heap();
    const p1 = proc();
    let full = null;
    if (!o.degrade.isTooLargeForTokenization) {
      const ok = await evaluate(cdp, 'bench.waitBgDone(600000)');
      const h2 = await heap();
      const p2 = proc();
      full = { bgMs: r3(ok.ms), done: ok.done, heapMB: r3(h2.used - h0.used), backingMB: r3((h2.raw.backingStorageSize - h0.raw.backingStorageSize) / 1e6), rssMB: p2 && p0 ? r3(p2.rssMB - p0.rssMB) : null, footprintMB: p2?.footprintMB != null && p0?.footprintMB != null ? r3(p2.footprintMB - p0.footprintMB) : null };
    }
    return {
      fileMB: r3(fetchRes.chars / 1e6),
      heapBaseMB: r3(h0.used), heapOpenMB: r3(h1.used - h0.used), metricsHeapOpenMB: r3(h1.metricsUsed - h0.metricsUsed),
      // V8 keeps large strings (the fetched text, Monaco's chunks) as external
      // strings: their bytes show in backingStorageSize, not usedSize.
      backingOpenMB: r3((h1.raw.backingStorageSize - h0.raw.backingStorageSize) / 1e6),
      jsAllOpenMB: r3((h1.raw.usedSize + (h1.raw.embedderHeapUsedSize || 0) + (h1.raw.backingStorageSize || 0) - h0.raw.usedSize - (h0.raw.embedderHeapUsedSize || 0) - (h0.raw.backingStorageSize || 0)) / 1e6),
      rssBaseMB: r3(p0?.rssMB), rssOpenMB: p1 && p0 ? r3(p1.rssMB - p0.rssMB) : null, samePid: p0?.pid === p1?.pid,
      fpBaseMB: r3(p0?.footprintMB), fpOpenMB: p1?.footprintMB != null && p0?.footprintMB != null ? r3(p1.footprintMB - p0.footprintMB) : null,
      full, degrade: o.degrade, heapRaw: [h0.raw, h1.raw],
    };
  },
};

// ---------------------------------------------------------------- summaries
const summaryKeys = {
  open: ['editableMs', 'tokensMs', 'createModelMs', 'fetchMs'],
  type: ['p50', 'p95', 'max', 'workP50', 'workP95'],
  scroll: ['fps', 'p99', 'meanMs', 'workMean', 'workP99', 'idleIntervalMedian'],
  ts20k: ['bgFromCreateMs', 'forceMs', 'firstScreenTokensMs', 'duringP50', 'duringP95', 'afterP95', 'duringWorkP95', 'afterWorkP95', 'keysDuringBg'],
  memory: ['heapOpenMB', 'backingOpenMB', 'jsAllOpenMB', 'rssOpenMB', 'fpOpenMB'],
};
function summarize(name, runs) {
  const ok = runs.filter((r) => r.accepted);
  const s = { scenario: name, accepted: ok.length, rejected: runs.length - ok.length };
  for (const k of summaryKeys[name]) {
    const vals = ok.map((r) => r.result[k]).filter((v) => v !== null && v !== undefined);
    s[k] = vals.length ? { median: r3(median(vals)), min: r3(Math.min(...vals)), max: r3(Math.max(...vals)) } : null;
  }
  if (name === 'memory') {
    const f = ok.map((r) => r.result.full).filter(Boolean);
    if (f.length) s.full = { heapMB: r3(median(f.map((x) => x.heapMB))), rssMB: r3(median(f.map((x) => x.rssMB))), bgMs: r3(median(f.map((x) => x.bgMs))) };
  }
  s.loads = ok.map((r) => [r.loadBefore, r.loadAfter]);
  return s;
}

// startBrowser launches a fresh Edge (new profile dir) and attaches to its page.
async function startBrowser() {
  const { port, flags } = await launchEdge();
  const cdp = await connectPage(port);
  await cdp.send('Page.enable');
  await cdp.send('Runtime.enable');
  await cdp.send('Emulation.setFocusEmulationEnabled', { enabled: true });
  // the headless window's viewport is smaller than --window-size (1912×996
  // seen): pin the page viewport to the reference 1920×1080 @2x.
  await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 2, mobile: false });
  const ver = await cdp.send('Browser.getVersion').catch(() => ({}));
  return { cdp, flags, ver };
}

// --------------------------------------------------------------------- main
async function main() {
  const server = await startServer({ monacoDir: cfg.monaco, dataDir: cfg.data });
  state.server = server;
  let { cdp, flags, ver } = await startBrowser();
  const url = server.url;

  if (cfg.selfTest) {
    await loadPage(cdp, url);
    const r = await evaluate(cdp, 'bench.selfTest()');
    const v = await evaluate(cdp, 'window.monacoVersion');
    const ok = v === '0.56.0' && r.hasCreate && r.renderedLines > 0;
    console.log(JSON.stringify({ selfTest: ok ? 'PASS' : 'FAIL', monacoVersion: v, renderedLines: r.renderedLines, crossOriginIsolated: r.isolated, viewport: r.viewport, browser: ver.product, errors: r.errors }));
    cleanupSync();
    process.exit(ok ? 0 : 1);
  }

  const names = cfg.scenario === 'all' ? ['open', 'type', 'scroll', 'ts20k', 'memory'] : cfg.scenario.split(',');
  for (const n of names) if (!scenarios[n]) throw new Error(`unknown scenario ${n}`);
  const out = {
    tool: 'tools/monaco-bench/bench.mjs', date: new Date().toISOString(), variant: cfg.variant,
    browser: ver.product, userAgent: ver.userAgent, monaco: await (async () => { await loadPage(cdp, url); return evaluate(cdp, 'window.monacoVersion'); })(),
    edgeFlags: flags.map((f) => (f.startsWith('--user-data-dir=') ? '--user-data-dir=<fresh tmp dir>' : f)),
    machine: { cpus: os.cpus().length, model: os.cpus()[0]?.model, memGB: os.totalmem() / 2 ** 30 },
    maxLoad: cfg.maxLoad, scenarios: {}, summaries: {}, pending: false,
  };
  const save = () => { if (cfg.out) fs.writeFileSync(cfg.out, JSON.stringify(out, null, 2)); };
  outer: for (const name of names) {
    const runs = (out.scenarios[name] = []);
    let attempts = 0;
    while (runs.filter((r) => r.accepted).length < cfg.runs) {
      if (++attempts > cfg.runs * 4) { out.pending = true; log(`${name}: too many rejected runs`); break; }
      if (!(await waitLoad())) { out.pending = true; log('load budget spent: stopping, results are partial'); break outer; }
      const loadBefore = r3(load1());
      const upBefore = uptime();
      const t = Date.now();
      let result, error;
      // A fresh browser (profile, renderer, V8 isolate) for every run, like
      // Tango's fresh process per round / per memory sample; it also keeps
      // a renderer left in a bad state by one run (a Page.navigate that
      // hung after a long load wait was seen once) from poisoning the next.
      stopEdgeSync();
      ({ cdp } = await startBrowser());
      try { result = await scenarios[name](cdp, url); } catch (e) { error = String(e.message || e); }
      const loadAfter = r3(load1());
      const accepted = !error && loadBefore < cfg.maxLoad && loadAfter < cfg.maxLoad;
      runs.push({ accepted, loadBefore, loadAfter, uptimeBefore: upBefore, uptimeAfter: uptime(), wallMs: Date.now() - t, result, error });
      log(`${name} run ${runs.length}: ${accepted ? 'ok' : 'REJECTED'} load ${loadBefore}→${loadAfter}`, error || JSON.stringify(Object.fromEntries((summaryKeys[name] || []).map((k) => [k, result?.[k]]))));
      if (error && runs.filter((r) => r.error).length >= 3) { log(`${name}: failing repeatedly, skipping`); break; }
      save();
      await sleep(1000);
    }
    out.summaries[name] = summarize(name, runs);
    save();
  }
  if (cfg.screenshot) {
    const shot = await cdp.send('Page.captureScreenshot', { format: 'png' });
    fs.writeFileSync(cfg.screenshot, Buffer.from(shot.data, 'base64'));
  }
  out.loadWaitMin = r3(waitedMs / 60000);
  out.elapsedMin = r3((Date.now() - loadStart) / 60000);
  save();
  console.log(JSON.stringify(out.summaries, null, 1));
  cleanupSync();
  process.exit(0);
}

main().catch((e) => { console.error(e); cleanupSync(); process.exit(1); });
