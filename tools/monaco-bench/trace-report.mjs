// Inventory only. Never correlate by nearest timestamp or compute E2E latency.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const candidate = /FrameSequenceTracker|DrawFrame|DrawAndSwap|SwapBuffers|FramePresented|FrameDisplayed|PipelineReporter|Graphics\.Pipeline|SubmitCompositorFrame|Presentation|DidNotProduceFrame|RasterTask|ActivateLayerTree|BeginMainFrame|Commit/i;
const presentation = /FramePresented|FrameDisplayed|Presentation|PipelineReporter|Graphics\.Pipeline\.DrawAndSwap/i;
const flowPhases = new Set(['s', 't', 'f']);
const markerName = /^scroll-(.+)-(\d+)-(start|dom-ready)$/;

function fields(value, prefix = '', out = {}) {
  if (!value || typeof value !== 'object') return out;
  for (const [key, v] of Object.entries(value)) {
    const p = prefix ? `${prefix}.${key}` : key;
    if (v && typeof v === 'object') fields(v, p, out);
    else if (/id|token|sequence|source|flow|step|timestamp|flag|status|state|damage|present|missing|checkerboard/i.test(key)) out[p] = v;
  }
  return out;
}

function describe(e) {
  const out = {};
  for (const k of ['name', 'cat', 'ph', 'pid', 'tid', 'ts', 'dur', 'id', 'id2', 'scope', 's', 'bind_id', 'bp', 'flow_in', 'flow_out']) {
    if (e[k] !== undefined) out[k] = e[k];
  }
  out.argKeys = Object.keys(e.args || {});
  out.identityCandidates = fields(e.args);
  return out;
}

// Chromium JSON exports opaque uint64 identities as JSON numbers. Node26's
// reviver source context lets us preserve their exact source text, not round IDs.
export function parseTraceJson(text) {
  return JSON.parse(text, (key, value, context) => {
    if (typeof value === 'number' && Number.isInteger(value) && !Number.isSafeInteger(value)) {
      if (typeof context?.source !== 'string') throw new Error('lossless JSON integer source context unavailable');
      return context.source;
    }
    return value;
  });
}

export function analyzeTrace(trace, metadata = {}, limit = 500) {
  if (!trace || !Array.isArray(trace.traceEvents)) throw new Error('expected JSON object with traceEvents array');
  const events = trace.traceEvents;
  const emittedCategories = new Set(events.flatMap((e) => typeof e.cat === 'string' ? e.cat.split(',').map((c) => c.trim()) : []));
  const requiredCategories = metadata.traceCategories ? ['blink.user_timing', 'cc', 'viz', 'gpu'] : [];
  const missingEmittedCategories = requiredCategories.filter((c) => !emittedCategories.has(c));
  const selected = events.filter((e) => candidate.test(e.name || ''));
  const flows = events.filter((e) => flowPhases.has(e.ph) || e.bind_id !== undefined || e.flow_in || e.flow_out);
  const markers = events.filter((e) => markerName.test(e.name || ''));
  const inventory = new Map();
  for (const e of selected) {
    const key = `${e.cat || ''} / ${e.name}`;
    const item = inventory.get(key) || { count: 0, phases: new Set(), argumentPaths: new Set() };
    item.count++;
    item.phases.add(e.ph);
    for (const k of Object.keys(fields(e.args))) item.argumentPaths.add(k);
    inventory.set(key, item);
  }
  // User Timing can export more than one event per mark. Count names, not events.
  const names = new Set(markers.map((e) => e.name));
  const missingMarkers = [];
  if (metadata.runId && Number.isInteger(metadata.steps)) {
    for (let i = 0; i < metadata.steps; i++) for (const suffix of ['start', 'dom-ready']) {
      const name = `scroll-${metadata.runId}-${i}-${suffix}`;
      if (!names.has(name)) missingMarkers.push(name);
    }
  }
  const issues = [];
  if (metadata.traceCategories && metadata.tracingStartSucceeded !== true) issues.push('Tracing.start not confirmed');
  if (!events.length) issues.push('no actual trace events received');
  if (missingEmittedCategories.length) issues.push(`required emitted categories missing: ${missingEmittedCategories.join(', ')}`);
  if (metadata.tracingComplete?.dataLossOccurred !== false) issues.push('trace loss unknown or reported');
  if (!metadata.runId || !Number.isInteger(metadata.steps)) issues.push('expected marker manifest absent');
  if (missingMarkers.length || !markers.length) issues.push('User Timing markers missing');
  if (!selected.length) issues.push('no candidate frame events');
  if (!selected.some((e) => presentation.test(e.name || ''))) issues.push('no candidate presentation events');
  if (metadata.gateFailure) issues.push(metadata.gateFailure);
  if (metadata.error) issues.push(metadata.error);
  if (metadata.cleanupError) issues.push(`cleanup failed: ${metadata.cleanupError}`);
  return {
    status: issues.length ? 'pending' : 'inventory-only', issues,
    gpuComplete: null, presentationCorrelation: 'unverified',
    tracingSupport: {
      startSucceeded: metadata.tracingStartSucceeded === true,
      actualEventsObserved: events.length > 0,
      collection: metadata.tracingStartSucceeded === true && events.length > 0 ? 'events-observed' : 'unverified',
      discovery: metadata.traceCategories?.discovery || 'not-recorded',
      emittedCategories: [...emittedCategories].sort(), missingEmittedCategories,
    },
    limitations: [
      'Candidate events and IDs only; no temporal pairing, completion claims, FPS or E2E calculations.',
      'JSON may omit protobuf identity fields. An event name alone does not establish hardware presentation.',
      'DOM-ready markers do not prove that this content was rasterized or presented.',
    ],
    eventCount: events.length, missingMarkers,
    markers: markers.map((e) => ({ ...describe(e), args: e.args })),
    eventInventory: Object.fromEntries([...inventory].map(([k, v]) => [k, {
      count: v.count, phases: [...v.phases], argumentPaths: [...v.argumentPaths].sort(),
    }])),
    candidateEvents: selected.slice(0, limit).map(describe),
    flowEvents: flows.slice(0, limit).map(describe),
    truncated: { candidates: Math.max(0, selected.length - limit), flows: Math.max(0, flows.length - limit) },
    clock: 'raw trace timestamps (normally microseconds); no cross-clock subtraction performed',
    integerEncoding: 'JSON integers outside Number safe range are retained as exact source-text strings by parseTraceJson',
  };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [traceFile, metadataFile] = process.argv.slice(2);
    if (!traceFile || !metadataFile) throw new Error('usage: node trace-report.mjs TRACE.json METADATA.json');
    const report = analyzeTrace(parseTraceJson(fs.readFileSync(traceFile, 'utf8')), JSON.parse(fs.readFileSync(metadataFile, 'utf8')));
    console.log(JSON.stringify(report, null, 2));
    if (report.status === 'pending') process.exitCode = 2;
  } catch (e) { console.error(e.message); process.exitCode = 1; }
}
