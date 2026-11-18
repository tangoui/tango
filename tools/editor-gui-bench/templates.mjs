// Expected-body templates from recorded, caret-free display states.
//
// A record pass (gui-probe Action.record) gives, per input step, the
// per-pixel maximum of the settled display in a region: on the light theme
// the blinking dark caret drops out. From consecutive states:
//   typing: the newly inked glyph (one contiguous run of changed columns);
//           its template is the middle half of the ink box, so caret
//           positions at the cell edges never overlap the patch;
//   scroll: a fixed body patch per step.
// Each template is checked to be distinguishable from the state before the
// input and (typing) to stay unchanged by later steps; a failing step makes
// the whole set unusable rather than silently dropped.
// Images are BGRA, row-major, width w.

export const MATCH = 3; // gui-probe: a patch matches at mean distance <= 3
const INK = 48; //          a channel difference that counts as changed ink
const KEY_SEP = 20; //      typing: blank cell vs glyph
const SCROLL_SEP = 2 * MATCH;

export function distance(a, b) {
  if (a.length !== b.length || a.length === 0) return Infinity;
  let sum = 0;
  for (let i = 0; i < a.length; i++) if (i % 4 !== 3) sum += Math.abs(a[i] - b[i]);
  return sum / ((a.length / 4) * 3);
}

export function cropImage(img, w, p) {
  const out = new Uint8Array(p.width * p.height * 4);
  for (let y = 0; y < p.height; y++) {
    const s = ((p.y + y) * w + p.x) * 4;
    out.set(img.subarray(s, s + p.width * 4), y * p.width * 4);
  }
  return out;
}

// glyphBox is the bounding box of pixels changed from prev to cur, or null
// when nothing changed or the changed columns are not one contiguous run.
export function glyphBox(prev, cur, w, h) {
  let x0 = Infinity, x1 = -1, y0 = Infinity, y1 = -1;
  const cols = new Uint8Array(w);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const o = (y * w + x) * 4;
      const d = Math.max(Math.abs(prev[o] - cur[o]), Math.abs(prev[o + 1] - cur[o + 1]), Math.abs(prev[o + 2] - cur[o + 2]));
      if (d > INK) {
        cols[x] = 1;
        x0 = Math.min(x0, x); x1 = Math.max(x1, x); y0 = Math.min(y0, y); y1 = Math.max(y1, y);
      }
    }
  }
  if (x1 < 0) return null;
  for (let x = x0; x <= x1; x++) if (!cols[x] && !cols[x + 1]) return null; // a gap of 2+ columns
  return { x0, x1, y0, y1 };
}

export function centerPatch(b) {
  const w = b.x1 - b.x0 + 1, q = Math.floor(w / 4);
  return { x: b.x0 + q, y: b.y0, width: Math.max(2, w - 2 * q), height: b.y1 - b.y0 + 1 };
}

// A step whose difference is not one glyph (a renderer switch on the first
// key repaints the whole line) takes its cell from the column advance of the
// steps found directly, when there are two; it is checked like the others
// and marked derived.
export function buildKeyTemplates(steps, w, h) {
  const templates = [], errors = [];
  const found = new Map(); // step -> patch
  for (let k = 1; k < steps.length; k++) {
    const box = glyphBox(steps[k - 1], steps[k], w, h);
    if (box) found.set(k, centerPatch(box));
  }
  const known = [...found.keys()];
  const first = known[0], last = known.at(-1);
  const adv = known.length >= 2 ? (found.get(last).x - found.get(first).x) / (last - first) : 0;
  for (let k = 1; k < steps.length; k++) {
    let patch = found.get(k), derived = false;
    if (!patch && known.length >= 2) {
      const near = known.reduce((a, b) => (Math.abs(b - k) < Math.abs(a - k) ? b : a));
      patch = { ...found.get(near), x: Math.round(found.get(first).x + (k - first) * adv) };
      derived = true;
    }
    if (!patch || patch.x < 0 || patch.x + patch.width > w) { errors.push(`step ${k}: no single new glyph`); continue; }
    const pixels = cropImage(steps[k], w, patch);
    const before = distance(cropImage(steps[k - 1], w, patch), pixels);
    let persist = 0;
    for (let j = k + 1; j < steps.length; j++) persist = Math.max(persist, distance(cropImage(steps[j], w, patch), pixels));
    if (before < KEY_SEP) errors.push(`step ${k}: template too close to the state before (${before.toFixed(1)})`);
    if (persist > 1) errors.push(`step ${k}: a later step changed the glyph (${persist.toFixed(1)})`);
    templates.push({ step: k, patch, pixels, before, persist, derived });
  }
  return { ok: errors.length === 0 && templates.length === steps.length - 1, templates, errors };
}

export function buildScrollTemplates(steps, w, h, patch) {
  const templates = [], errors = [];
  for (let k = 1; k < steps.length; k++) {
    const pixels = cropImage(steps[k], w, patch);
    const before = distance(cropImage(steps[k - 1], w, patch), pixels);
    if (before < SCROLL_SEP) errors.push(`step ${k}: not distinguishable from step ${k - 1} (${before.toFixed(1)})`);
    templates.push({ step: k, patch, pixels, before });
  }
  return { ok: errors.length === 0, templates, errors };
}

// shiftedPatches moves the anchor caret's per-step patches by dcol whole
// columns of a monospace line: the column advance is the mean step of the
// anchor patches (each key moves the caret one column).
export function shiftedPatches(anchor, dcol) {
  if (anchor.length < 2) throw new Error('need two anchor steps for the column advance');
  const adv = (anchor.at(-1).x - anchor[0].x) / (anchor.length - 1);
  return anchor.map((p) => ({ ...p, x: Math.round(p.x + dcol * adv) }));
}

// buildKeyTemplatesAt checks templates at given per-step patches: for a
// caret inside text, where typing shifts the rest of the line and the new
// glyph cannot be isolated by difference (glyphBox).
export function buildKeyTemplatesAt(steps, w, patches) {
  const templates = [], errors = [];
  for (let k = 1; k < steps.length; k++) {
    const patch = patches[k - 1];
    if (!patch || patch.x < 0 || patch.y < 0 || patch.x + patch.width > w || (patch.y + patch.height) * w * 4 > steps[k].length) { errors.push(`step ${k}: patch outside the region`); continue; }
    const pixels = cropImage(steps[k], w, patch);
    const before = distance(cropImage(steps[k - 1], w, patch), pixels);
    let persist = 0;
    for (let j = k + 1; j < steps.length; j++) persist = Math.max(persist, distance(cropImage(steps[j], w, patch), pixels));
    if (before < KEY_SEP) errors.push(`step ${k}: template too close to the state before (${before.toFixed(1)})`);
    if (persist > 1) errors.push(`step ${k}: a later step changed the glyph (${persist.toFixed(1)})`);
    templates.push({ step: k, patch, pixels, before, persist });
  }
  return { ok: errors.length === 0 && templates.length === steps.length - 1, templates, errors };
}
