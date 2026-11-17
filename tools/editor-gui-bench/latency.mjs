#!/usr/bin/env node
// Latency bounds for one gui-probe measure pass (analysis.mjs).
//   node latency.mjs <capture.ndjson>
// Every posted input with template regions is in the denominator; a frame
// counts for an input only while that input was armed (frame id).
import fs from 'node:fs';
import { latencyBounds, summarize } from './analysis.mjs';

const rows = fs.readFileSync(process.argv[2], 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
const meta = rows.find((r) => r.type === 'metadata') || {};
const frames = rows.filter((r) => r.type === 'frame');
const inputs = rows.filter((r) => r.type === 'input' && r.regions > 0);
const results = inputs.map((e) => latencyBounds(e, frames.filter((f) => f.id === e.id)));
const ms = (v) => (v == null ? null : +(v / 1e6).toFixed(2));
const s = summarize(results);
console.log(JSON.stringify({
  source: meta.source,
  ...s, p95Lower: ms(s.p95Lower), p95Upper: ms(s.p95Upper),
  upperMs: results.map((r) => ms(r.upper)),
  missed: results.filter((r) => !r.complete).map((r) => r.id),
}));
