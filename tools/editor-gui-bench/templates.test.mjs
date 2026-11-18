import test from 'node:test';
import assert from 'node:assert/strict';
import { glyphBox, centerPatch, cropImage, distance, buildKeyTemplates, buildScrollTemplates, buildKeyTemplatesAt, shiftedPatches } from './templates.mjs';

// Synthetic BGRA images: white background, dark "ink" rectangles.
function img(w, h, rects = []) {
  const a = new Uint8Array(w * h * 4).fill(255);
  for (const [x, y, rw, rh, v = 30] of rects) {
    for (let j = y; j < y + rh; j++) for (let i = x; i < x + rw; i++) { const o = (j * w + i) * 4; a[o] = a[o + 1] = a[o + 2] = v; }
  }
  return a;
}
const W = 120, H = 20;
// glyph k occupies columns [10+12k+2, 10+12k+10) rows 4..16
const glyph = (k) => [10 + 12 * k + 2, 4, 8, 12];
const stepsTyping = (n) => Array.from({ length: n + 1 }, (_, s) => img(W, H, Array.from({ length: s }, (_, k) => glyph(k))));

test('glyphBox finds only the newly inked glyph', () => {
  const s = stepsTyping(2);
  assert.deepEqual(glyphBox(s[1], s[2], W, H), { x0: 24, x1: 31, y0: 4, y1: 15 });
});

test('glyphBox rejects no change and split changes', () => {
  const a = img(W, H);
  assert.equal(glyphBox(a, a, W, H), null);
  const split = img(W, H, [[10, 4, 4, 4], [60, 4, 4, 4]]);
  assert.equal(glyphBox(a, split, W, H), null);
});

test('centerPatch keeps the middle half, away from caret columns', () => {
  assert.deepEqual(centerPatch({ x0: 24, x1: 31, y0: 4, y1: 15 }), { x: 26, y: 4, width: 4, height: 12 });
});

test('distance is mean absolute RGB difference', () => {
  const a = img(2, 1), b = img(2, 1, [[0, 0, 1, 1, 0]]);
  assert.equal(distance(a, a), 0);
  assert.equal(distance(a, b), 127.5);
  assert.equal(cropImage(b, 2, { x: 0, y: 0, width: 1, height: 1 })[0], 0);
});

test('buildKeyTemplates produces one checked template per key', () => {
  const r = buildKeyTemplates(stepsTyping(3), W, H);
  assert.equal(r.ok, true);
  assert.equal(r.templates.length, 3);
  assert.deepEqual(r.templates.map((t) => t.patch.x), [14, 26, 38]);
  for (const t of r.templates) assert.ok(t.before >= 20 && t.persist <= 1);
});

test('buildKeyTemplates fails when a key leaves no visible glyph', () => {
  const s = stepsTyping(2);
  s.push(s[2]); // third key changed nothing
  const r = buildKeyTemplates(s, W, H);
  assert.equal(r.ok, false);
  assert.match(r.errors[0], /step 3/);
});

test('buildKeyTemplates fails when a later step overwrites a template', () => {
  const s = stepsTyping(2);
  s[2] = img(W, H, [glyph(1), [10 + 2, 4, 8, 12, 200]]); // glyph 0 repainted light
  const r = buildKeyTemplates(s, W, H);
  assert.equal(r.ok, false);
});

test('buildScrollTemplates requires each step to differ from the previous one', () => {
  const shift = (d) => img(W, H, [[10, (2 + d) % 16, 30, 3], [60, (8 + d) % 16, 30, 3]]);
  const ok = buildScrollTemplates([shift(0), shift(2), shift(4)], W, H, { x: 0, y: 0, width: W, height: H });
  assert.equal(ok.ok, true);
  assert.equal(ok.templates.length, 2);
  const same = buildScrollTemplates([shift(0), shift(2), shift(2)], W, H, { x: 0, y: 0, width: W, height: H });
  assert.equal(same.ok, false);
  assert.match(same.errors[0], /step 2/);
});

// A caret inside text: each key inserts a glyph at column 2+k and shifts
// the rest of the line right by one cell (12 px).
const cellGlyph = (col, v = 30) => [10 + 12 * col + 2, 4, 8, 12, v];
function shiftingSteps(n) {
  const tail = [0, 1, 2, 3, 4, 5]; // a line "ab|cdef..." as columns
  return Array.from({ length: n + 1 }, (_, s) => {
    const r = [];
    for (const c of tail) r.push(cellGlyph(c < 2 ? c : c + s, 80)); // shifted text, lighter
    for (let k = 0; k < s; k++) r.push(cellGlyph(2 + k)); // inserted glyphs
    return img(W + 120, H, r);
  });
}

test('glyphBox cannot isolate a glyph when the line tail shifts', () => {
  const s = shiftingSteps(1);
  assert.equal(buildKeyTemplates(s, W + 120, H).ok, false);
});

test('shiftedPatches moves anchor patches by whole columns', () => {
  const anchor = [{ x: 14, y: 4, width: 4, height: 12 }, { x: 26, y: 4, width: 4, height: 12 }, { x: 38, y: 4, width: 4, height: 12 }];
  assert.deepEqual(shiftedPatches(anchor, -3).map((p) => p.x), [-22, -10, 2]);
  assert.deepEqual(shiftedPatches(anchor, 2).map((p) => p.x), [38, 50, 62]);
});

test('buildKeyTemplatesAt checks given patches against shifting text', () => {
  const s = shiftingSteps(3);
  const at = [2, 3, 4].map((col) => centerPatch({ x0: 10 + 12 * col + 2, x1: 10 + 12 * col + 9, y0: 4, y1: 15 }));
  const r = buildKeyTemplatesAt(s, W + 120, at);
  assert.equal(r.ok, true, r.errors.join('; '));
  assert.ok(r.templates.every((t) => t.before >= 20 && t.persist <= 1));
  const wrong = [3, 4, 5].map((col) => centerPatch({ x0: 10 + 12 * col + 2, x1: 10 + 12 * col + 9, y0: 4, y1: 15 }));
  assert.equal(buildKeyTemplatesAt(s, W + 120, wrong).ok, false);
});

// A renderer switch on the first key (native layers to Metal) repaints the
// whole line a little differently: step 1 has no single new glyph, but its
// cell follows from the other steps' column advance.
test('buildKeyTemplates places a repainted step from the column advance', () => {
  const steps = Array.from({ length: 4 }, (_, s) => img(W, H, [
    [2, 4, 6, 12, s === 0 ? 30 : 80], // existing text, repainted on step 1
    ...Array.from({ length: s }, (_, k) => glyph(k)),
  ]));
  const r = buildKeyTemplates(steps, W, H);
  assert.equal(r.ok, true, r.errors.join('; '));
  assert.deepEqual(r.templates.map((t) => t.patch.x), [14, 26, 38]);
  assert.equal(r.templates[0].derived, true);
  assert.ok(r.templates.every((t) => t.before >= 20 && t.persist <= 1));
});

test('buildKeyTemplates does not derive a step without two found steps', () => {
  const steps = Array.from({ length: 3 }, (_, s) => img(W, H, [
    [2, 4, 6, 12, s === 0 ? 30 : 80],
    ...Array.from({ length: s }, (_, k) => glyph(k)),
  ]));
  steps[2] = steps[1]; // the second key changed nothing either
  assert.equal(buildKeyTemplates(steps, W, H).ok, false);
});
