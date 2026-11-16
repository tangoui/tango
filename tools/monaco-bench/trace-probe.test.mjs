import test from 'node:test';
import assert from 'node:assert/strict';
import { analyzeTrace, parseTraceJson } from './trace-report.mjs';
import { consoleSessionState, gpuState, parseOptions, scrollProbe, tracingCategoryPlan } from './trace-probe.mjs';
import { CDP, navigateDocument } from './trace-cdp.mjs';

const metadata = { runId: 'fixture', steps: 1, tracingComplete: { dataLossOccurred: false } };
const trace = () => ({ traceEvents: [
  { name: 'scroll-fixture-0-start', cat: 'blink.user_timing', ph: 'I', ts: 100, pid: 1, tid: 1 },
  { name: 'scroll-fixture-0-dom-ready', cat: 'blink.user_timing', ph: 'I', ts: 200, pid: 1, tid: 1 },
  { name: 'Graphics.Pipeline.DrawAndSwap', cat: 'viz', ph: 'b', ts: 210,
    id2: { global: '0x123' }, args: { data: { frame_token: 7, display_trace_id: 9, step: 'DRAW' } } },
  { name: 'Graphics.Pipeline', cat: 'viz', ph: 's', ts: 220, id: '0x123', args: { frame_sink_id: '1:2' } },
] });

test('opaque uint64 identity values retain exact text instead of Number rounding collisions', () => {
  const raw = '{"traceEvents":[{"name":"Graphics.Pipeline","args":{"surface_frame_trace_id":8097343025625949283,"display_trace_id":8097343025625949284},"ts":1234}]}';
  const parsed = parseTraceJson(raw);
  const fields = parsed.traceEvents[0].args;
  assert.equal(fields.surface_frame_trace_id, '8097343025625949283');
  assert.equal(fields.display_trace_id, '8097343025625949284');
  assert.notEqual(fields.surface_frame_trace_id, fields.display_trace_id);
  assert.equal(parsed.traceEvents[0].ts, 1234);
});

test('inventory retains nested IDs and flows but produces no completion or nearest-time pairing', () => {
  const report = analyzeTrace(trace(), metadata);
  assert.equal(report.status, 'inventory-only');
  assert.equal(report.gpuComplete, null);
  assert.equal(report.presentationCorrelation, 'unverified');
  assert.deepEqual(report.candidateEvents[0].id2, { global: '0x123' });
  assert.equal(report.candidateEvents[0].identityCandidates['data.frame_token'], 7);
  assert.equal(report.flowEvents[0].id, '0x123');
  assert.equal(report.e2eMs, undefined);
  assert.equal(report.pairs, undefined);
});

test('empty discovery stays blocked by default and only explicit capability probing requests known categories', () => {
  assert.match(tracingCategoryPlan([]).blockedReason, /discovery empty/);
  const plan = tracingCategoryPlan([], true);
  assert.equal(plan.mode, 'direct-capability-probe');
  assert.equal(plan.blockedReason, null);
  for (const category of ['blink.user_timing', 'cc', 'viz', 'gpu']) assert.ok(plan.selected.includes(category));
  assert.equal(parseOptions(['--probe-unadvertised-tracing']).run, false);
  assert.equal(parseOptions(['--probe-unadvertised-tracing']).probeUnadvertisedTracing, true);
  assert.equal(tracingCategoryPlan(['cc'], true).mode, 'advertised-only');
  assert.ok(tracingCategoryPlan(['cc'], true).blockedReason);
  assert.throws(() => tracingCategoryPlan(null, true), /invalid/);
});

test('empty discovery plus real events confirms collection only, not GPU or presentation', () => {
  const m = { ...metadata, tracingStartSucceeded: true, traceCategories: tracingCategoryPlan([], true) };
  const t = trace();
  t.traceEvents.push({ name: 'PipelineReporter', cat: 'cc,benchmark', ph: 'X', ts: 150, dur: 100 });
  t.traceEvents.push({ name: 'GpuTask', cat: 'gpu', ph: 'X', ts: 175, dur: 10 });
  const r = analyzeTrace(t, m);
  assert.equal(r.status, 'inventory-only');
  assert.equal(r.tracingSupport.collection, 'events-observed');
  assert.deepEqual(r.tracingSupport.missingEmittedCategories, []);
  assert.equal(r.gpuComplete, null);
  assert.equal(r.presentationCorrelation, 'unverified');
});

test('empty discovery with no events or missing critical event categories remains pending', () => {
  const m = { ...metadata, tracingStartSucceeded: true, traceCategories: tracingCategoryPlan([], true) };
  const empty = analyzeTrace({ traceEvents: [] }, m);
  assert.equal(empty.status, 'pending');
  assert.equal(empty.tracingSupport.collection, 'unverified');
  const partial = analyzeTrace(trace(), m);
  assert.equal(partial.status, 'pending');
  assert.deepEqual(partial.tracingSupport.missingEmittedCategories, ['cc', 'gpu']);
  assert.equal(partial.tracingSupport.collection, 'events-observed');
  const startUnknown = analyzeTrace(trace(), { ...m, tracingStartSucceeded: false });
  assert.equal(startUnknown.status, 'pending');
});

test('known or unknown trace loss is pending', () => {
  for (const loss of [true, undefined]) {
    assert.equal(analyzeTrace(trace(), { ...metadata, tracingComplete: { dataLossOccurred: loss } }).status, 'pending');
  }
});

test('marker completeness is per run and step, not merely marker count', () => {
  const t = trace();
  t.traceEvents[1].name = 'scroll-other-0-dom-ready';
  const report = analyzeTrace(t, metadata);
  assert.deepEqual(report.missingMarkers, ['scroll-fixture-0-dom-ready']);
  assert.equal(report.status, 'pending');
});

test('no trace/unsupported presentation/lock gate cannot pass silently', () => {
  assert.equal(analyzeTrace({ traceEvents: [] }, metadata).status, 'pending');
  const t = trace();
  t.traceEvents = t.traceEvents.filter((e) => !e.name.includes('DrawAndSwap'));
  assert.equal(analyzeTrace(t, metadata).status, 'pending');
  assert.equal(analyzeTrace(trace(), { ...metadata, gateFailure: 'console locked' }).status, 'pending');
  assert.equal(analyzeTrace(trace(), { ...metadata, cleanupError: 'profile removal failed' }).status, 'pending');
  assert.throws(() => analyzeTrace({ events: [] }, metadata), /traceEvents/);
});

test('truncation is explicit and inventory counts still cover all events', () => {
  const r = analyzeTrace(trace(), metadata, 1);
  assert.equal(r.candidateEvents.length, 1);
  assert.equal(r.truncated.candidates, 1);
  assert.equal(Object.keys(r.eventInventory).length, 2);
});

const consolePlist = (flag) => [{ IOConsoleLocked: flag, IOConsoleUsers: [{
  kCGSSessionOnConsoleKey: true, kCGSSessionLoginDoneKey: true,
}] }];

test('console lock parser is fail-closed and does not expose session identities', () => {
  assert.equal(consoleSessionState(consolePlist(false)).status, 'unlocked');
  assert.equal(consoleSessionState(consolePlist(true)).status, 'locked');
  assert.equal(consoleSessionState(consolePlist(undefined)).status, 'unknown');
  assert.equal(consoleSessionState([]).status, 'unknown');
  const ambiguous = consolePlist(false);
  ambiguous[0].IOConsoleUsers.push({ ...ambiguous[0].IOConsoleUsers[0] });
  assert.equal(consoleSessionState(ambiguous).status, 'unknown');
  const conflict = consolePlist(false);
  conflict[0].IOConsoleUsers[0].CGSSessionScreenIsLocked = true;
  assert.equal(consoleSessionState(conflict).status, 'locked');
});

test('local ioreg dictionary root and single-S login key, including array/alias variants', () => {
  // Structural regression for this Mac; no session identity or project data.
  const local = { IOConsoleLocked: false, IOConsoleUsers: [{
    kCGSSessionOnConsoleKey: true, kCGSessionLoginDoneKey: true,
  }] };
  assert.equal(consoleSessionState(local).status, 'unlocked');
  assert.equal(consoleSessionState([local]).status, 'unlocked');
  const locked = structuredClone(local);
  locked.IOConsoleLocked = true;
  assert.equal(consoleSessionState(locked).status, 'locked');
  const conflict = structuredClone(local);
  conflict.IOConsoleUsers[0].kCGSSessionLoginDoneKey = false;
  assert.equal(consoleSessionState(conflict).status, 'unknown');
  const missing = structuredClone(local);
  delete missing.IOConsoleUsers[0].kCGSessionLoginDoneKey;
  assert.equal(consoleSessionState(missing).status, 'unknown');
  const noLock = structuredClone(local);
  delete noLock.IOConsoleLocked;
  assert.equal(consoleSessionState(noLock).status, 'unknown');
});

test('backend checks distinguish browser-reported hardware, software and unknown', () => {
  const info = (renderer, status) => ({ gpu: { auxAttributes: { glRenderer: renderer }, featureStatus: { gpu_compositing: status } } });
  assert.equal(gpuState(info('ANGLE (Apple, Metal)', 'enabled')).status, 'hardware-reported');
  assert.equal(gpuState(info('SwiftShader', 'enabled')).status, 'software-or-disabled');
  assert.equal(gpuState(info('ANGLE', 'disabled_software')).status, 'software-or-disabled');
  assert.equal(gpuState(info('ANGLE', undefined)).status, 'unknown');
  assert.equal(gpuState({}).status, 'unknown');
});

test('browser launch is opt-in, probe bounds and filenames are strict', () => {
  assert.equal(parseOptions([]).run, false);
  assert.equal(parseOptions(['--run']).run, true);
  assert.equal(parseOptions(['--help']).run, false);
  for (const args of [['--steps', '100'], ['--max-load', '7'], ['--gap-ms', '0'],
    ['--file', '../private'], ['--edge'], ['--unknown'], ['--steps', 'NaN']]) {
    assert.throws(() => parseOptions(args));
  }
});

test('injected function compiles without evaluating any browser or DOM code', () => {
  assert.doesNotThrow(() => new Function(`return (${scrollProbe.toString()});`));
});

test('navigation ignores stale document load, handles pre-response events and reports navigation errors', async () => {
  let listener;
  const cdp = {
    on(method, fn) { assert.equal(method, 'Page.lifecycleEvent'); listener = fn; return () => { listener = null; }; },
    async send(method) {
      if (method === 'Page.setLifecycleEventsEnabled') return {};
      assert.equal(method, 'Page.navigate');
      listener({ name: 'load', frameId: 'main', loaderId: 'old-about-blank' });
      listener({ name: 'load', frameId: 'main', loaderId: 'new-local-document' });
      return { frameId: 'main', loaderId: 'new-local-document' };
    },
  };
  assert.equal((await navigateDocument(cdp, 'http://127.0.0.1/')).loaderId, 'new-local-document');
  assert.equal(listener, null);
  const failed = { ...cdp, async send(method) {
    return method === 'Page.navigate' ? { errorText: 'net::ERR_NAME_NOT_RESOLVED' } : {};
  } };
  await assert.rejects(navigateDocument(failed, 'http://127.0.0.1/'), /ERR_NAME_NOT_RESOLVED/);
  assert.equal(listener, null);
});

test('adapted CDP client handles responses, events, timeout and closure without a socket', async () => {
  const RealWebSocket = globalThis.WebSocket;
  class FakeWebSocket {
    constructor() { queueMicrotask(() => this.onopen()); }
    send(text) {
      const message = JSON.parse(text);
      if (message.method === 'ok') queueMicrotask(() => this.onmessage({ data: JSON.stringify({ id: message.id, result: { value: 1 } }) }));
      if (message.method === 'unsupported') queueMicrotask(() => this.onmessage({ data: JSON.stringify({ id: message.id, error: { message: 'not supported' } }) }));
    }
    close() { this.onclose(); }
  }
  globalThis.WebSocket = FakeWebSocket;
  try {
    const cdp = new CDP('fake://no-network', 30);
    await cdp.opened;
    assert.deepEqual(await cdp.send('ok'), { value: 1 });
    await assert.rejects(cdp.send('unsupported'), /not supported/);
    await assert.rejects(cdp.send('never'), /timeout/);
    const once = cdp.once('done');
    cdp.ws.onmessage({ data: JSON.stringify({ method: 'done', params: { eof: true } }) });
    assert.deepEqual(await once, { eof: true });
    await assert.rejects(cdp.once('missing'), /event timeout/);
    const pending = cdp.send('pending');
    cdp.close();
    await assert.rejects(pending, /closed/);
    assert.equal(cdp.pending.size, 0);
  } finally { globalThis.WebSocket = RealWebSocket; }
});
