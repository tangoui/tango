#!/usr/bin/env node
// Visible-window Monaco driver for the GUI acceptance probe.
//
//   node tools/editor-gui-bench/monaco-driver.mjs --probe <gui-probe> --out <dir>
//        [--scenarios type,scroll,multi] [--keys 10] [--scrolls 30] [--monaco <dir>] [--edge <bin>]
//
// One visible Edge --app window per scenario (fresh profile, normal vsync,
// real Retina scale, 1920×1080 window), the same synthetic document and
// carets as the Go fixture (fixture-text.mjs). CDP is used only for setup
// and for reading the model afterwards; every input is a CGEvent posted by
// gui-probe. Only this run's Edge process group and profile are removed.
// Without an unobscured display the capture is isolated-window functional
// evidence, never display latency.

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import { startServer } from '../monaco-bench/server.mjs';
import { fixtureText, caretOffsets, fnv64a } from './fixture-text.mjs';

const argv = process.argv.slice(2);
const opt = (k, d) => { const i = argv.indexOf(k); return i >= 0 && i + 1 < argv.length ? argv[i + 1] : d; };
const cfg = {
  probe: opt('--probe', ''),
  out: opt('--out', ''),
  scenarios: opt('--scenarios', 'type,scroll,multi').split(','),
  keys: Number(opt('--keys', 10)),
  scrolls: Number(opt('--scrolls', 30)),
  templates: argv.includes('--templates'), // record -> templates -> measure
  monaco: opt('--monaco', '/Users/admin/min/mincode/vue-app/node_modules/monaco-editor'),
  edge: opt('--edge', '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge'),
};
if (!cfg.probe || !cfg.out) {
  console.error('usage: monaco-driver.mjs --probe <gui-probe> --out <dir> [--scenarios type,scroll,multi]');
  process.exit(2);
}
const log = (...a) => console.error('[monaco-driver]', ...a);

// ------------------------------------------------------------------ cleanup
const live = { edge: null, tmp: null, ws: null, server: null };
function stopEdge() {
  try { live.ws?.close(); } catch {}
  live.ws = null;
  if (live.edge && live.edge.exitCode === null) {
    try { process.kill(-live.edge.pid, 'SIGKILL'); } catch {}
  }
  live.edge = null;
  if (live.tmp) {
    for (let i = 0; i < 20; i++) {
      try { fs.rmSync(live.tmp, { recursive: true, force: true }); break; } catch { execFileSync('/bin/sleep', ['0.1']); }
    }
  }
  live.tmp = null;
}
function cleanup() {
  stopEdge();
  try { live.server?.server.closeAllConnections?.(); live.server?.server.close(); } catch {}
}
process.on('exit', cleanup);
for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(sig, () => { cleanup(); process.exit(130); });

// --------------------------------------------------------------------- Edge
const EDGE_FLAGS = [
  '--remote-debugging-port=0',
  '--window-size=1920,1080',
  '--window-position=0,0',
  '--no-first-run',
  '--no-default-browser-check',
  '--disable-extensions',
  // the window may be partly covered by other apps: keep it rendering
  '--disable-background-timer-throttling',
  '--disable-renderer-backgrounding',
  '--disable-backgrounding-occluded-windows',
  '--disable-background-networking',
  '--disable-component-update',
  '--disable-sync',
  '--disable-default-apps',
  '--no-pings',
];

async function launchEdge(url) {
  live.tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'tango-gui-monaco-'));
  const errLog = fs.openSync(path.join(live.tmp, 'edge-stderr.log'), 'w');
  live.edge = spawn(cfg.edge, [...EDGE_FLAGS, `--user-data-dir=${live.tmp}`, `--app=${url}`], { detached: true, stdio: ['ignore', 'ignore', errLog] });
  const portFile = path.join(live.tmp, 'DevToolsActivePort');
  for (let i = 0; i < 300; i++) {
    if (fs.existsSync(portFile)) {
      const [port] = fs.readFileSync(portFile, 'utf8').split('\n');
      if (port) return Number(port);
    }
    if (live.edge.exitCode !== null) break;
    await sleep(100);
  }
  throw new Error('Edge did not write DevToolsActivePort');
}

class CDP {
  constructor(url) {
    this.id = 0; this.pending = new Map();
    this.ws = new WebSocket(url);
    this.ws.onmessage = (ev) => {
      const m = JSON.parse(ev.data);
      const p = m.id !== undefined && this.pending.get(m.id);
      if (!p) return;
      this.pending.delete(m.id);
      m.error ? p.reject(new Error(`${p.method}: ${m.error.message}`)) : p.resolve(m.result);
    };
    this.opened = new Promise((res, rej) => { this.ws.onopen = res; this.ws.onerror = () => rej(new Error('websocket error')); });
    this.ws.onclose = () => { for (const p of this.pending.values()) p.reject(new Error('websocket closed')); this.pending.clear(); };
  }
  send(method, params = {}) {
    const id = ++this.id;
    return new Promise((resolve, reject) => { this.pending.set(id, { resolve, reject, method }); this.ws.send(JSON.stringify({ id, method, params })); });
  }
}

async function evaluate(cdp, expression) {
  const r = await cdp.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, timeout: 120000 });
  if (r.exceptionDetails) throw new Error('page: ' + (r.exceptionDetails.exception?.description || r.exceptionDetails.text));
  return r.result.value;
}

async function connectPage(port, url) {
  for (let i = 0; i < 100; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const page = list.find((t) => t.type === 'page' && t.url.startsWith(url));
      if (page) {
        const cdp = new CDP(page.webSocketDebuggerUrl);
        await cdp.opened;
        live.ws = cdp.ws;
        return cdp;
      }
    } catch {}
    await sleep(100);
  }
  throw new Error('no app page target');
}

// ------------------------------------------------------------------- page
// Same document, carets, font size and light theme as the Tango fixture.
const SETUP = (text, offsets) => `(async () => {
  await bench.ready;
  bench.text = ${JSON.stringify(text)};
  await bench.open({ name: 'fixture' });
  monaco.editor.setTheme('vs');
  const ed = bench.ed, m = bench.model;
  // No popups over the body: Tango's editor has no suggest/hover widgets
  // here, and a suggest list would cover the other carets' rows.
  ed.updateOptions({ fontSize: 14, quickSuggestions: false, suggestOnTriggerCharacters: false, wordBasedSuggestions: 'off', parameterHints: { enabled: false }, hover: { enabled: false } });
  ed.layout();
  const sels = ${JSON.stringify(offsets)}.map((o) => { const p = m.getPositionAt(o); return new monaco.Selection(p.lineNumber, p.column, p.lineNumber, p.column); });
  ed.setSelections(sels);
  ed.revealPositionInCenterIfOutsideViewport(sels[0].getPosition());
  ed.focus();
  await bench.afterFrame(); await bench.afterFrame();
  return true;
})()`;

const STATE = `(() => {
  const ed = bench.ed, m = bench.model;
  return { version: m.getVersionId(), text: m.getValue(monaco.editor.EndOfLinePreference.LF),
    selections: ed.getSelections().map((s) => ({ anchor: m.getOffsetAt(s.getSelectionStart()), head: m.getOffsetAt(s.getPosition()) })),
    scrollTop: ed.getScrollTop(), textFocus: ed.hasTextFocus(), docFocus: document.hasFocus(),
    rows: ed.getSelections().map((s) => { const v = ed.getScrolledVisiblePosition(s.getPosition()); const top = ed.getDomNode().getBoundingClientRect().top; return v ? [top + v.top, v.height] : null; }),
    viewport: [innerWidth, innerHeight, devicePixelRatio], version_monaco: window.monacoVersion };
})()`;

// ----------------------------------------------------------------- oracle
function insertAll(text, offsets, s) {
  let out = text;
  for (const o of [...offsets].sort((a, b) => b - a)) out = out.slice(0, o) + s + out.slice(o);
  return out;
}

function probeRun(dir, name, job) {
  const jf = path.join(dir, name + '.json');
  fs.writeFileSync(jf, JSON.stringify(job));
  try {
    execFileSync(cfg.probe, ['--job', jf, '--out', path.join(dir, name)], { stdio: ['ignore', 'ignore', 'pipe'], timeout: 60000 });
    return { exit: 0, err: '' };
  } catch (e) {
    return { exit: e.status ?? -1, err: String(e.stderr || e.message).slice(-300) };
  }
}

function frameStats(dir, name) {
  const f = path.join(dir, name, 'capture.ndjson');
  if (!fs.existsSync(f)) return null;
  const rows = fs.readFileSync(f, 'utf8').trim().split('\n').filter(Boolean).map((l) => JSON.parse(l));
  const frames = rows.filter((r) => r.type === 'frame');
  const meta = rows.find((r) => r.type === 'metadata') || {};
  return { source: meta.source, scale: meta.scale, size: [meta.width, meta.height], frames: frames.length,
    complete: frames.filter((r) => r.status === 'complete').length,
    invalid: frames.filter((r) => r.invalid).length, privacyBlocked: rows.filter((r) => r.type === 'privacy-blocked').length };
}

async function runScenario(name, server) {
  const dir = path.join(cfg.out, name);
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
  const res = { scenario: name };
  try {
    const url = server.url;
    const port = await launchEdge(url);
    const cdp = await connectPage(port, url);
    for (let i = 0; i < 300; i++) {
      if (await evaluate(cdp, `typeof window.bench === 'object' && document.readyState === 'complete'`).catch(() => false)) break;
      await sleep(100);
    }
    const text = fixtureText(name === 'multi' ? 'multi' : 'type');
    const offsets = caretOffsets(text, name === 'multi' ? 3 : 1);
    await evaluate(cdp, SETUP(text, offsets));
    const before = await evaluate(cdp, STATE);
    res.setup = { checksum: fnv64a(before.text), goldenMatch: before.text === text, selections: before.selections, viewport: before.viewport, monaco: before.version_monaco, textFocus: before.textFocus };
    if (!res.setup.goldenMatch) throw new Error('Monaco model differs from the fixture document');

    let win = null;
    for (let i = 0; i < 30 && !win; i++) {
      const rows = JSON.parse(execFileSync(cfg.probe, ['--list'], { encoding: 'utf8' }));
      win = rows.filter((w) => w.pid === live.edge.pid && w.layer === 0).sort((a, b) => b.frame[2] * b.frame[3] - a.frame[2] * a.frame[3])[0] || null;
      if (!win) await sleep(200);
    }
    if (!win) throw new Error('Edge window not on screen');
    res.window = { windowID: win.windowID, frame: win.frame };
    const [fx, fy, fw, fh] = win.frame;

    if (cfg.templates) {
      Object.assign(res, await templatesFlow(name, dir, cdp, win, before, text, offsets));
      return res;
    }
    let actions;
    if (name === 'scroll') {
      actions = Array.from({ length: cfg.scrolls }, (_, i) => ({ id: i, kind: 'scroll', dy: -40, x: fx + fw / 2, y: fy + fh / 2, waitMS: 120 }));
    } else {
      // keycode 7 = "x" (ANSI); no click: a click would collapse the carets.
      actions = Array.from({ length: cfg.keys }, (_, i) => ({ id: i, kind: 'key', keycode: 7, waitMS: 150 }));
    }
    const job = { pid: live.edge.pid, windowID: win.windowID, isolatedWindow: true, inputSource: 'com.apple.keylayout.ABC', actions };
    res.probe = probeRun(dir, 'capture', job);
    res.capture = frameStats(dir, 'capture');
    const after = await evaluate(cdp, STATE);
    res.after = { version: after.version, checksum: fnv64a(after.text), selections: after.selections, scrollTop: after.scrollTop, textFocus: after.textFocus };
    if (name === 'scroll') {
      res.pass = res.probe.exit === 0 && after.text === text && after.scrollTop > before.scrollTop;
      res.scrolled = after.scrollTop - before.scrollTop;
      res.reason = 'text unchanged and scrollTop increased';
    } else {
      const expected = insertAll(text, offsets, 'x'.repeat(cfg.keys));
      res.pass = res.probe.exit === 0 && after.text === expected;
      res.inserted = after.text.length - text.length;
      res.reason = `exact text: ${cfg.keys} x at ${offsets.length} caret(s)`;
    }
  } catch (e) {
    res.pass = false;
    res.error = String(e.message || e);
  } finally {
    stopEdge();
  }
  return res;
}

const SCALE = 2;
function nodeTool(script, args) {
  try {
    const o = execFileSync(process.execPath, [new URL(script, import.meta.url).pathname, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
    return { ok: true, out: JSON.parse(o.trim().split('\n').pop()) };
  } catch (e) {
    let out = null;
    try { out = JSON.parse(String(e.stdout).trim().split('\n').pop()); } catch {}
    return { ok: false, out, err: String(e.stderr || e.message).slice(-300) };
  }
}

// templatesFlow: record pass (baseline wait + inputs, each recording the
// regions after settling), templates, the same initial state again, then
// the measure pass with the templates and its latency bounds.
async function templatesFlow(name, dir, cdp, win, before, text, offsets) {
  const res = {};
  const [fx, fy, fw, fh] = win.frame;
  const top = fh - before.viewport[1]; // the app window's titlebar in the capture
  let regions, inputs, recWait, settle;
  if (name === 'scroll') {
    regions = [{ x: 0, y: Math.floor((top + before.viewport[1] * 0.6) * SCALE), width: 800, height: 120 }];
    inputs = (extra) => Array.from({ length: cfg.scrolls }, (_, i) => ({ id: i + 1, kind: 'scroll', dy: -40, x: fx + fw / 2, y: fy + fh / 2, ...extra }));
    recWait = 700; settle = 250;
  } else {
    if (before.rows.some((r) => !r)) throw new Error('a caret row is not visible');
    regions = before.rows.map(([t, h]) => ({ x: 0, y: Math.floor((top + t) * SCALE), width: 1600, height: Math.ceil(h * SCALE) }));
    inputs = (extra) => Array.from({ length: cfg.keys }, (_, i) => ({ id: i + 1, kind: 'key', keycode: 7, ...extra }));
    recWait = 1300; settle = 300;
  }
  res.regions = regions;
  const job = (actions, extra = {}) => ({ pid: live.edge.pid, windowID: win.windowID, isolatedWindow: true, inputSource: 'com.apple.keylayout.ABC', actions, ...extra });
  const rec = [{ id: 0, kind: 'wait', waitMS: recWait, settleMS: settle, record: regions }, ...inputs({ waitMS: recWait, settleMS: settle, record: regions })];
  res.recordProbe = probeRun(dir, 'record', job(rec));
  if (res.recordProbe.exit) throw new Error('record probe failed: ' + res.recordProbe.err);
  const cols = offsets.map((o) => o - (text.lastIndexOf('\n', o - 1) + 1)).join(',');
  const t = nodeTool('./make-templates.mjs', ['--capture', path.join(dir, 'record'), '--kind', name === 'scroll' ? 'scroll' : 'key', '--columns', cols, '--out', path.join(dir, 'templates')]);
  res.templates = t.out || { ok: false, err: t.err };
  if (!t.ok) throw new Error('templates unusable');
  const patches = new Map(JSON.parse(fs.readFileSync(path.join(dir, 'templates', 'patches.json'), 'utf8')).map((p) => [p.id, p.patches]));
  await evaluate(cdp, SETUP(text, offsets));
  // Same lead-in as the record pass (its baseline wait), so both passes
  // start their inputs equally long after activation.
  res.measureProbe = probeRun(dir, 'measure', job([{ id: 0, kind: 'wait', waitMS: recWait, settleMS: settle, record: regions }, ...inputs({ waitMS: 1000 }).map((a) => ({ ...a, patches: patches.get(a.id) })),
    { id: 999, kind: 'wait', waitMS: recWait, settleMS: settle, record: regions }],
    // Align to the record pass's baseline; a failed alignment fails the pass.
    { alignTo: regions.map((_, i) => path.join(dir, 'record', `rec-0-${i}.bgra`)), alignAfter: 0 }));
  try {
    res.align = fs.readFileSync(path.join(dir, 'measure', 'capture.ndjson'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l)).find((r) => r.type === 'align') || null;
  } catch { res.align = null; }
  const l = nodeTool('./latency.mjs', [path.join(dir, 'measure', 'capture.ndjson')]);
  res.latency = l.out || { err: l.err };
  const after = await evaluate(cdp, STATE);
  res.pass = res.measureProbe.exit === 0 && (name === 'scroll' ? after.text === text : after.text === insertAll(text, offsets, 'x'.repeat(cfg.keys)));
  return res;
}

async function main() {
  fs.mkdirSync(cfg.out, { recursive: true });
  live.server = await startServer({ monacoDir: cfg.monaco });
  const results = [];
  for (const s of cfg.scenarios) {
    log('scenario', s);
    results.push(await runScenario(s, live.server));
  }
  fs.writeFileSync(path.join(cfg.out, 'summary.json'), JSON.stringify(results, null, 2));
  console.log(JSON.stringify(results));
  cleanup();
  process.exit(results.every((r) => r.pass) ? 0 : 1);
}

main().catch((e) => { console.error(e); cleanup(); process.exit(1); });
