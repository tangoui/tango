#!/usr/bin/env node
// Builds expected-body templates from a gui-probe record pass.
//
//   node make-templates.mjs --capture <dir> --kind key|scroll --out <dir>
//
// The record job's action 0 is a baseline `wait`, actions 1..N the inputs;
// every action records the same regions. Writes tpl-<step>-<region>.bgra and
// patches.json ([{ id, patches: [{x,y,width,height,template}] }], capture
// pixel coordinates) and report.json. Exits 1 when any step is unusable.
import fs from 'node:fs';
import path from 'node:path';
import { buildKeyTemplates, buildScrollTemplates, buildKeyTemplatesAt, shiftedPatches } from './templates.mjs';

const argv = process.argv.slice(2);
const opt = (k) => { const i = argv.indexOf(k); return i >= 0 ? argv[i + 1] : undefined; };
const capture = opt('--capture'), kind = opt('--kind'), out = opt('--out');
// --columns: each region's caret column. Region 0 finds its glyphs by
// difference; a caret inside text shifts the rest of its line, so the
// other regions take region 0's patches moved by the column difference.
const columns = opt('--columns')?.split(',').map(Number);
if (!capture || !out || !['key', 'scroll'].includes(kind)) {
  console.error('usage: make-templates.mjs --capture <dir> --kind key|scroll --out <dir>');
  process.exit(2);
}

const rows = fs.readFileSync(path.join(capture, 'capture.ndjson'), 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
const recs = rows.filter((r) => r.type === 'record');
const errors = [];
for (const r of recs) if (!(r.frames > 0) || r.invalid) errors.push(`record ${r.id}/${r.region}: ${r.invalid || 'no frame'}`);
const ids = [...new Set(recs.map((r) => r.id))].sort((a, b) => a - b);
const regionsN = Math.max(0, ...recs.map((r) => (r.region ?? -1) + 1));
if (ids[0] !== 0) errors.push('no baseline record (action 0)');

fs.mkdirSync(out, { recursive: true });
const steps = new Map(); // step -> patches
const report = { kind, steps: ids.length - 1, regions: regionsN, perRegion: [] };
for (let i = 0; i < regionsN && errors.length === 0; i++) {
  const rr = ids.map((id) => recs.find((r) => r.id === id && r.region === i));
  if (rr.some((r) => !r)) { errors.push(`region ${i}: missing step`); break; }
  const { width: w, height: h, x: rx, y: ry } = rr[0];
  const imgs = rr.map((r) => new Uint8Array(fs.readFileSync(path.join(capture, r.file))));
  const anchor = report.perRegion[0];
  const res = kind === 'key' && i > 0 && columns
    ? buildKeyTemplatesAt(imgs, w, shiftedPatches(anchor.map((t) => t.patch), columns[i] - columns[0]))
    : kind === 'key'
    ? buildKeyTemplates(imgs, w, h)
    : buildScrollTemplates(imgs, w, h, { x: 0, y: 0, width: w, height: h });
  errors.push(...res.errors.map((e) => `region ${i} ${e}`));
  report.perRegion.push(res.templates.map((t) => ({ step: t.step, patch: t.patch, before: +t.before.toFixed(2), persist: t.persist === undefined ? undefined : +t.persist.toFixed(2) })));
  for (const t of res.templates) {
    const file = path.resolve(out, `tpl-${t.step}-${i}.bgra`);
    fs.writeFileSync(file, t.pixels);
    const list = steps.get(t.step) || [];
    list.push({ x: rx + t.patch.x, y: ry + t.patch.y, width: t.patch.width, height: t.patch.height, template: file });
    steps.set(t.step, list);
  }
}
report.ok = errors.length === 0;
report.errors = errors;
fs.writeFileSync(path.join(out, 'report.json'), JSON.stringify(report, null, 2));
fs.writeFileSync(path.join(out, 'patches.json'), JSON.stringify([...steps].map(([id, patches]) => ({ id, patches }))));
console.log(JSON.stringify({ ok: report.ok, steps: report.steps, regions: regionsN, errors: errors.slice(0, 5) }));
process.exit(report.ok ? 0 : 1);
