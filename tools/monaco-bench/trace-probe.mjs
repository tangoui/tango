#!/usr/bin/env node
// Opt-in, short real-window trace probe. NOT a benchmark or a completion fence.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { setTimeout as sleep } from 'node:timers/promises';
import { startServer, defaultDataDir } from './server.mjs';
import { CDP, evaluate, navigateDocument } from './trace-cdp.mjs';
import { analyzeTrace, parseTraceJson } from './trace-report.mjs';

class Pending extends Error {}
const here = path.dirname(fileURLToPath(import.meta.url));
const CATEGORIES = [
  'blink.user_timing', 'devtools.timeline', 'cc', 'viz', 'gpu', 'benchmark',
  'graphics.pipeline', 'disabled-by-default-devtools.timeline.frame',
  'disabled-by-default-display.framedisplayed',
];

// Fail closed: do not assume an absent lock property means "unlocked".
export function consoleSessionState(plist) {
  const root = Array.isArray(plist) ? plist[0] : plist;
  const users = root?.IOConsoleUsers;
  if (!Array.isArray(users)) return { status: 'unknown', reason: 'IOConsoleUsers absent' };
  const active = users.filter((u) => u.kCGSSessionOnConsoleKey === true);
  const loginFlags = active.length === 1
    ? [active[0].kCGSessionLoginDoneKey, active[0].kCGSSessionLoginDoneKey].filter((x) => x !== undefined)
    : [];
  if (active.length !== 1 || !loginFlags.length || loginFlags.some((x) => x !== true)) {
    return { status: 'unknown', reason: 'no unique logged-in console session' };
  }
  const flags = [root.IOConsoleLocked, active[0].CGSSessionScreenIsLocked].filter((x) => typeof x === 'boolean');
  if (!flags.length) return { status: 'unknown', reason: 'no explicit lock boolean' };
  return { status: flags.some(Boolean) ? 'locked' : 'unlocked',
    evidence: { IOConsoleLocked: root.IOConsoleLocked, CGSSessionScreenIsLocked: active[0].CGSSessionScreenIsLocked } };
}

function lockState() {
  if (process.platform !== 'darwin') return { status: 'unknown', reason: 'macOS lock probe only' };
  try {
    const xml = execFileSync('/usr/sbin/ioreg', ['-n', 'Root', '-d1', '-a'], { timeout: 3000, maxBuffer: 8 << 20 });
    const json = execFileSync('/usr/bin/plutil', ['-convert', 'json', '-o', '-', '-'], { input: xml, timeout: 3000 });
    return consoleSessionState(JSON.parse(json));
  } catch (e) { return { status: 'unknown', reason: e.message }; }
}

export function gpuState(info) {
  const gpu = info?.gpu;
  const renderer = gpu?.auxAttributes?.glRenderer;
  const feature = gpu?.featureStatus?.gpu_compositing;
  if (typeof renderer !== 'string' || !renderer) return { status: 'unknown', reason: 'glRenderer unavailable' };
  if (/swiftshader|llvmpipe|software rasterizer/i.test(renderer) || /software|disabled|unavailable/i.test(feature || '')) {
    return { status: 'software-or-disabled', renderer, feature };
  }
  if (!/enabled/i.test(feature || '')) return { status: 'unknown', renderer, feature, reason: 'GPU compositing state unavailable' };
  return { status: 'hardware-reported', renderer, feature,
    limitation: 'Browser-reported backend; this does not prove completion or actual target dimensions.' };
}

export function tracingCategoryPlan(available, probeUnadvertised = false) {
  if (!Array.isArray(available) || available.some((c) => typeof c !== 'string')) {
    throw new Error('invalid Tracing.getCategories response');
  }
  const discovery = available.length ? 'advertised' : 'empty';
  const directProbe = discovery === 'empty' && probeUnadvertised;
  return {
    available, discovery, unadvertisedProbeRequested: probeUnadvertised,
    mode: directProbe ? 'direct-capability-probe' : 'advertised-only',
    selected: directProbe ? [...CATEGORIES] : CATEGORIES.filter((c) => available.includes(c)),
    missing: CATEGORIES.filter((c) => !available.includes(c)),
    blockedReason: discovery === 'empty' && !directProbe
      ? 'trace category discovery empty; tracing support unverified'
      : !directProbe && ['blink.user_timing', 'cc', 'viz'].some((c) => !available.includes(c))
        ? 'required trace categories not advertised; support unverified' : null,
  };
}

export function parseOptions(args) {
  const cfg = { run: false, probeUnadvertisedTracing: false, steps: 8, gapMs: 100, maxLoad: 6,
    monaco: '/Users/admin/min/mincode/vue-app/node_modules/monaco-editor',
    edge: '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
    data: defaultDataDir, file: 'gen20000-15d1551d.ts', startLine: 1001 };
  const options = new Map([
    ['--steps', 'steps'], ['--gap-ms', 'gapMs'], ['--max-load', 'maxLoad'],
    ['--monaco', 'monaco'], ['--edge', 'edge'], ['--data', 'data'],
    ['--file', 'file'], ['--start-line', 'startLine'], ['--out', 'out'],
  ]);
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--run') { cfg.run = true; continue; }
    if (args[i] === '--probe-unadvertised-tracing') { cfg.probeUnadvertisedTracing = true; continue; }
    if (args[i] === '--help' || args[i] === '-h') { cfg.help = true; continue; }
    const key = options.get(args[i]);
    if (!key || !args[i + 1] || args[i + 1].startsWith('--')) throw new Error(`unknown or incomplete option: ${args[i]}`);
    const value = args[++i];
    cfg[key] = ['steps', 'gapMs', 'maxLoad', 'startLine'].includes(key) ? Number(value) : value;
  }
  if (!Number.isInteger(cfg.steps) || cfg.steps < 1 || cfg.steps > 30) throw new Error('--steps must be 1..30');
  if (!Number.isFinite(cfg.gapMs) || cfg.gapMs < 50 || cfg.gapMs > 1000) throw new Error('--gap-ms must be 50..1000');
  if (!Number.isFinite(cfg.maxLoad) || cfg.maxLoad <= 0 || cfg.maxLoad > 6) throw new Error('--max-load must be >0 and <=6');
  if (!Number.isInteger(cfg.startLine) || cfg.startLine < 1) throw new Error('--start-line must be positive');
  if (!/^[A-Za-z0-9._-]+$/.test(cfg.file)) throw new Error('--file must be a plain local data filename');
  return cfg;
}

// Executed only in the existing page. DOM checks are diagnostic, not pixel proof.
export async function scrollProbe({ runId, steps, gapMs }) {
  const ed = window.bench.ed, model = window.bench.model;
  const lh = ed.getOption(monaco.editor.EditorOption.lineHeight);
  const normalize = (s) => s.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
  const samples = [];
  for (let i = 0; i < steps; i++) {
    if (document.visibilityState !== 'visible' || !document.hasFocus()) throw new Error('page not visible/focused');
    const before = ed.getScrollTop(), target = before + 2 * lh;
    const name = `scroll-${runId}-${i}`;
    performance.mark(`${name}-start`, { detail: { runId, i, before, target } });
    ed.setScrollTop(target, monaco.editor.ScrollType.Immediate);
    let snapshot;
    for (let attempt = 0; attempt < 12; attempt++) {
      await new Promise(requestAnimationFrame);
      const ranges = ed.getVisibleRanges();
      const first = ranges[0]?.startLineNumber;
      const pos = first && ed.getScrolledVisiblePosition({ lineNumber: first, column: 1 });
      const host = document.getElementById('host').getBoundingClientRect();
      const line = pos && [...document.querySelectorAll('.view-lines .view-line')].find((el) =>
        Math.abs(el.getBoundingClientRect().top - host.top - pos.top) <= 2);
      const expected = first ? model.getLineContent(first) : '';
      if (Math.abs(ed.getScrollTop() - target) < 0.5 && line && normalize(line.textContent) === normalize(expected)) {
        snapshot = { i, before, target, top: ed.getScrollTop(), firstLine: first,
          domText: line.textContent, expectedText: expected, domTop: line.getBoundingClientRect().top,
          expectedTop: host.top + pos.top, attempts: attempt + 1 };
        break;
      }
    }
    if (!snapshot) throw new Error(`scroll ${i}: matching DOM line not observed within 12 rAFs`);
    performance.mark(`${name}-dom-ready`, { detail: { runId, i, top: snapshot.top, firstLine: snapshot.firstLine } });
    samples.push(snapshot);
    // Deliberate spacing, NOT waiting for GPU or presentation. Never use as fps.
    await new Promise((resolve) => setTimeout(resolve, gapMs));
  }
  return { samples, visibility: document.visibilityState, focused: document.hasFocus(),
    errors: window.benchErrors, tokenizationState: model.tokenization.backgroundTokenizationState };
}

async function main(cfg) {
  const runId = randomUUID();
  const prefix = path.resolve(cfg.out || path.join(here, '../../.perf', `monaco-trace-probe-${runId}`));
  if (!fs.statSync(path.dirname(prefix)).isDirectory()) throw new Error('output parent must exist');
  const files = { trace: `${prefix}.trace.json`, metadata: `${prefix}.metadata.json`, report: `${prefix}.report.json` };
  // Reserve all outputs before launching anything; never overwrite another run.
  const reserved = [];
  try { for (const file of Object.values(files)) { fs.closeSync(fs.openSync(file, 'wx')); reserved.push(file); } }
  catch (e) { for (const file of reserved) fs.unlinkSync(file); throw e; }
  fs.writeFileSync(files.trace, JSON.stringify({ traceEvents: [], probeIncomplete: true }));
  const metadata = { tool: 'monaco-trace-probe', runId, steps: cfg.steps, startedAt: new Date().toISOString(),
    status: 'pending', files, config: cfg, gateChecks: [], tracingComplete: null, traceSaved: false,
    gpuComplete: null, presentationCorrelation: 'unverified',
    limitations: ['Short diagnostic only; spaced programmatic scroll, not a benchmark.',
      'No screenshots, presentation pairing, E2E latency or GPU fence claims.',
      'Device pixels from JS are inferred; actual GPU target dimensions require trace validation.'] };
  const state = { server: null, edge: null, tmp: null, page: null, browser: null, tracing: false, stopReason: null };
  const check = () => { if (state.stopReason) throw new Pending(state.stopReason); };
  const abort = (reason) => { state.stopReason ||= reason; state.page?.close(); };
  const gate = (phase) => {
    check();
    const sample = { phase, at: new Date().toISOString(), load1: os.loadavg()[0], lock: lockState() };
    metadata.gateChecks.push(sample);
    if (sample.lock.status !== 'unlocked') throw new Pending(`console ${sample.lock.status}: ${sample.lock.reason || 'locked'}`);
    if (sample.load1 >= cfg.maxLoad) throw new Pending(`load ${sample.load1} >= ${cfg.maxLoad}`);
  };
  const onInt = () => abort('interrupted: SIGINT');
  const onTerm = () => abort('interrupted: SIGTERM');
  const onHup = () => abort('interrupted: SIGHUP');
  process.on('SIGINT', onInt); process.on('SIGTERM', onTerm); process.on('SIGHUP', onHup);
  const watchdog = setTimeout(() => abort('probe overall timeout (120s)'), 120000);
  let monitor;

  async function flushTrace() {
    if (!state.tracing) return;
    const done = state.browser.once('Tracing.tracingComplete', 15000);
    done.catch(() => {});
    await state.browser.send('Tracing.end');
    metadata.tracingComplete = await done;
    metadata.tracingFinishedAt = new Date().toISOString();
    state.tracing = false;
    const stream = metadata.tracingComplete.stream;
    if (!stream) throw new Error('Tracing returned no stream');
    let bytes = 0;
    const fd = fs.openSync(files.trace, 'w');
    try {
      for (;;) {
        const chunk = await state.browser.send('IO.read', { handle: stream, size: 1 << 20 });
        const data = Buffer.from(chunk.data, chunk.base64Encoded ? 'base64' : 'utf8');
        bytes += data.length;
        if (bytes > 128 * 1024 * 1024) throw new Error('trace exceeds 128 MiB safety limit');
        fs.writeFileSync(fd, data);
        if (chunk.eof) break;
      }
      metadata.traceSaved = true;
      metadata.traceBytes = bytes;
    } finally {
      fs.closeSync(fd);
      await state.browser.send('IO.close', { handle: stream }).catch(() => {});
    }
    if (metadata.tracingComplete.dataLossOccurred !== false) throw new Pending('trace data lost or loss status unavailable');
  }

  try {
    gate('before-launch');
    const pkg = JSON.parse(fs.readFileSync(path.join(cfg.monaco, 'package.json'), 'utf8'));
    if (pkg.version !== '0.56.0') throw new Pending(`expected Monaco 0.56.0, got ${pkg.version}`);
    metadata.monacoVersion = pkg.version;
    fs.accessSync(path.join(cfg.data, cfg.file), fs.constants.R_OK);
    fs.accessSync(cfg.edge, fs.constants.X_OK);
    state.server = await startServer({ monacoDir: cfg.monaco, dataDir: cfg.data });
    check();
    state.tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'monaco-trace-probe-'));
    const flags = [
      '--remote-debugging-port=0', '--window-size=1920,1080', '--force-device-scale-factor=2',
      '--no-first-run', '--no-default-browser-check', '--disable-extensions',
      '--disable-background-timer-throttling', '--disable-renderer-backgrounding', '--disable-backgrounding-occluded-windows',
      '--disable-background-networking', '--disable-component-update', '--disable-sync', '--disable-default-apps',
      '--no-pings', '--metrics-recording-only', '--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1',
      `--user-data-dir=${state.tmp}`,
    ];
    metadata.edgeFlags = flags.map((f) => f.startsWith('--user-data-dir=') ? '--user-data-dir=<fresh tmp dir>' : f);
    const stderr = fs.openSync(path.join(state.tmp, 'stderr.log'), 'w');
    metadata.edgeLaunchRequestedAt = new Date().toISOString();
    try { state.edge = spawn(cfg.edge, [...flags, 'about:blank'], { detached: true, stdio: ['ignore', 'ignore', stderr] }); }
    finally { fs.closeSync(stderr); }
    state.edge.once('spawn', () => { metadata.edgeSpawnedAt = new Date().toISOString(); metadata.edgePid = state.edge.pid; });
    state.edge.once('exit', (code, signal) => { metadata.edgeExitedAt = new Date().toISOString(); metadata.edgeExit = { code, signal }; });
    let launchError;
    state.edge.on('error', (e) => { launchError = e; });
    const portFile = path.join(state.tmp, 'DevToolsActivePort');
    for (let i = 0; !fs.existsSync(portFile); i++) {
      check();
      if (launchError) throw launchError;
      if (state.edge.exitCode !== null || i >= 150) throw new Error('Edge did not expose a CDP port');
      await sleep(100);
    }
    const [port, browserPath] = fs.readFileSync(portFile, 'utf8').trim().split('\n');
    if (!/^\d+$/.test(port) || !browserPath?.startsWith('/devtools/browser/')) throw new Error('invalid DevToolsActivePort');
    state.browser = new CDP(`ws://127.0.0.1:${port}${browserPath}`);
    await state.browser.opened;
    metadata.browserVersion = await state.browser.send('Browser.getVersion');
    metadata.systemInfo = await state.browser.send('SystemInfo.getInfo');
    metadata.gpu = gpuState(metadata.systemInfo);
    if (metadata.gpu.status !== 'hardware-reported') throw new Pending(`GPU backend ${metadata.gpu.status}`);
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) })).json();
    const target = targets.find((t) => t.type === 'page');
    if (!target) throw new Error('no page target');
    // Only trust our loopback debugging endpoint, never an arbitrary returned host.
    const ws = new URL(target.webSocketDebuggerUrl);
    if (ws.hostname !== '127.0.0.1' || ws.port !== port) throw new Error('nonlocal page WebSocket');
    metadata.targetId = target.id;
    state.page = new CDP(ws.href);
    await state.page.opened;
    monitor = setInterval(() => {
      if (state.stopReason) return;
      try { gate('monitor'); } catch (e) { metadata.gateFailure = e.message; abort(e.message); }
    }, 1000);
    const page = state.page;
    await page.send('Page.enable'); await page.send('Runtime.enable');
    // Block page network requests outside this server before navigation.
    metadata.blockedRequests = [];
    page.on('Fetch.requestPaused', (e) => {
      const allowed = e.request.url.startsWith(`${state.server.url}`) || /^(blob:|data:|about:)/.test(e.request.url);
      if (!allowed) metadata.blockedRequests.push({ url: e.request.url, resourceType: e.resourceType });
      page.send(allowed ? 'Fetch.continueRequest' : 'Fetch.failRequest',
        allowed ? { requestId: e.requestId } : { requestId: e.requestId, errorReason: 'BlockedByClient' })
        .catch((err) => abort(err.message));
      if (!allowed) abort('unexpected nonlocal page request blocked');
    });
    await page.send('Fetch.enable', { patterns: [{ urlPattern: '*', requestStage: 'Request' }] });
    await page.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 2, mobile: false });
    // Do not emulate focus/visibility: a locked or background window must not pass.
    await page.send('Page.bringToFront');
    metadata.navigation = await navigateDocument(page, state.server.url);
    metadata.loadedDocument = await evaluate(page, `({url:location.href,title:document.title,
      readyState:document.readyState,hasBench:!!window.bench,errors:window.benchErrors||[]})`);
    if (metadata.loadedDocument.url !== state.server.url || !metadata.loadedDocument.hasBench) {
      throw new Error(`unexpected loaded document: ${JSON.stringify(metadata.loadedDocument)}`);
    }
    await evaluate(page, 'bench.ready');
    await evaluate(page, `bench.fetchText(${JSON.stringify(cfg.file)})`);
    metadata.open = await evaluate(page, 'bench.open({tokenTimeoutMs:5000})');
    metadata.preparation = await evaluate(page, `(async()=>{
      if(bench.model.getLineCount() < ${cfg.startLine + cfg.steps * 2 + 100}) throw Error('document too short');
      bench.ed.setPosition({lineNumber:${cfg.startLine},column:1});
      bench.ed.setScrollTop((${cfg.startLine}-1)*bench.ed.getOption(monaco.editor.EditorOption.lineHeight),monaco.editor.ScrollType.Immediate);
      bench.ed.focus(); for(let i=0;i<5;i++) await bench.afterFrame();
      return {top:bench.ed.getScrollTop(),degrade:bench.model.isTooLargeForTokenization()};
    })()`);
    metadata.viewport = await evaluate(page, `({width:innerWidth,height:innerHeight,dpr:devicePixelRatio,
      inferredDevicePixels:[innerWidth*devicePixelRatio,innerHeight*devicePixelRatio],
      screen:{width:screen.width,height:screen.height,availWidth:screen.availWidth,availHeight:screen.availHeight},
      visibility:document.visibilityState,focused:document.hasFocus(),isolated:crossOriginIsolated})`);
    metadata.layoutMetrics = await page.send('Page.getLayoutMetrics');
    metadata.window = await state.browser.send('Browser.getWindowForTarget', { targetId: target.id });
    metadata.fullTargetVisibilityVerified = false;
    metadata.limitations.push('Native window bounds include browser chrome; an emulated 1920x1080 viewport is not proof that its full target is physically visible.');
    if (metadata.viewport.width !== 1920 || metadata.viewport.height !== 1080 || metadata.viewport.dpr !== 2) throw new Pending('viewport/DPR mismatch');
    if (metadata.viewport.visibility !== 'visible' || !metadata.viewport.focused) throw new Pending('page not actually visible/focused');
    gate('before-trace');
    const available = (await state.browser.send('Tracing.getCategories')).categories;
    metadata.traceCategories = tracingCategoryPlan(available, cfg.probeUnadvertisedTracing);
    if (metadata.traceCategories.blockedReason) throw new Pending(metadata.traceCategories.blockedReason);
    await state.browser.send('Tracing.start', {
      transferMode: 'ReturnAsStream', streamFormat: 'json', streamCompression: 'none',
      traceConfig: { recordMode: 'recordUntilFull', traceBufferSizeInKb: 65536,
        includedCategories: metadata.traceCategories.selected, excludedCategories: ['disabled-by-default-devtools.screenshot'] },
    });
    metadata.tracingStartSucceeded = true;
    metadata.tracingStartedAt = new Date().toISOString();
    state.tracing = true;
    check();
    metadata.scroll = await evaluate(page, `(${scrollProbe.toString()})(${JSON.stringify({ runId, steps: cfg.steps, gapMs: cfg.gapMs })})`);
    if (metadata.scroll.errors.length || metadata.scroll.visibility !== 'visible' || !metadata.scroll.focused) throw new Pending('page errors or visibility/focus lost');
    // Capture a short tail; not a GPU barrier or assertion of presentation.
    await sleep(250); check();
    await flushTrace();
    gate('after-trace');
    metadata.status = 'inventory-only';
  } catch (e) {
    metadata.status = e instanceof Pending || state.stopReason ? 'pending' : 'failed';
    metadata.error = state.stopReason || e.message;
    if (state.tracing) {
      try { await flushTrace(); } catch (flushError) { metadata.flushError = flushError.message; }
    }
  } finally {
    clearTimeout(watchdog); clearInterval(monitor);
    state.page?.close(); state.browser?.close();
    const cleanupFailure = (e) => { metadata.cleanupError = e.message; metadata.status = 'failed'; };
    let canRemoveProfile = true;
    if (state.edge?.pid) {
      const signalOwnedGroup = (signal) => {
        try { process.kill(-state.edge.pid, signal); }
        catch (e) { if (e.code !== 'ESRCH') { cleanupFailure(e); canRemoveProfile = false; } }
      };
      const waitExit = async () => {
        if (state.edge.exitCode !== null || state.edge.signalCode !== null) return;
        let onExit;
        try {
          await Promise.race([new Promise((r) => { onExit = r; state.edge.once('exit', r); }), sleep(1500)]);
        } finally { if (onExit) state.edge.off('exit', onExit); }
      };
      signalOwnedGroup('SIGTERM');
      await waitExit();
      // Only this freshly spawned process group, never other Edge instances.
      signalOwnedGroup('SIGKILL');
      await waitExit();
      if (state.edge.exitCode === null && state.edge.signalCode === null) {
        cleanupFailure(new Error('owned Edge process did not exit; profile retained'));
        canRemoveProfile = false;
      }
    }
    if (state.server) {
      try { await state.server.close(); } catch (e) { cleanupFailure(e); }
    }
    if (state.tmp) {
      try { metadata.edgeStderrTail = fs.readFileSync(path.join(state.tmp, 'stderr.log'), 'utf8').slice(-4000); } catch {}
      if (canRemoveProfile) {
        try { fs.rmSync(state.tmp, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 }); }
        catch (e) { cleanupFailure(e); metadata.retainedProfile = state.tmp; }
      } else metadata.retainedProfile = state.tmp;
    }
    process.off('SIGINT', onInt); process.off('SIGTERM', onTerm); process.off('SIGHUP', onHup);
  }
  if (state.stopReason && !metadata.error) {
    metadata.error = state.stopReason;
    if (metadata.status !== 'failed') metadata.status = 'pending';
  }
  metadata.finishedAt = new Date().toISOString();
  let report;
  try {
    report = analyzeTrace(parseTraceJson(fs.readFileSync(files.trace, 'utf8')), metadata);
    if (metadata.status === 'failed') report.status = 'failed';
    else if (report.status === 'pending') metadata.status = 'pending';
  } catch (e) {
    metadata.status = 'failed'; metadata.parseError = e.message;
    report = { status: 'failed', error: e.message, gpuComplete: null, presentationCorrelation: 'unverified' };
  }
  fs.writeFileSync(files.metadata, JSON.stringify(metadata, null, 2));
  fs.writeFileSync(files.report, JSON.stringify(report, null, 2));
  console.log(JSON.stringify({ status: metadata.status, files, error: metadata.error, issues: report.issues }, null, 2));
  process.exitCode = metadata.status === 'inventory-only' ? 0 : metadata.status === 'pending' ? 2 : 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const cfg = parseOptions(process.argv.slice(2));
    if (cfg.help || !cfg.run) {
      console.log('Preparation only; no browser started. After explicit approval: node tools/monaco-bench/trace-probe.mjs --run [--out EXISTING_DIR/prefix] [--probe-unadvertised-tracing] [--steps 8] [--file NAME] [--start-line 1001] [--max-load 6]. See README.');
      if (!cfg.help) process.exitCode = 2;
    } else await main(cfg);
  } catch (e) { console.error(e.message); process.exitCode = 1; }
}
