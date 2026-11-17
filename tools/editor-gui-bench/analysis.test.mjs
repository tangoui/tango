import { test } from 'node:test';
import assert from 'node:assert/strict';
import { latencyBounds, summarize, comparePairs } from './analysis.mjs';

const event = { id: 1, postedBefore: 100, postedAfter: 102, deadline: 200, regions: 2, scale: 2, width: 400, height: 200 };
const frame = (time, extra = {}) => ({ time, status: 'complete', scale: 2, width: 400, height: 200, matches: 0, ...extra });

test('latency needs all visible body positions, not the first caret', () => {
  assert.deepEqual(latencyBounds(event, [frame(110), frame(120, { matches: 1 }), frame(135, { matches: 2 })]),
    { id: 1, lower: 18, upper: 35, complete: true, valid: true, first: 120, all: 135 });
});
test('input-post interval and last unchanged frame bound the response', () => {
  const got = latencyBounds(event, [frame(95), frame(110), frame(125, { matches: 2 })]);
  assert.equal(got.lower, 8);
  assert.equal(got.upper, 25);
});
test('idle does not mean capture loss and callback time is irrelevant', () => {
  const got = latencyBounds(event, [frame(95), frame(110, { status: 'idle', callback: 9999 }), frame(130, { matches: 2 })]);
  assert.equal(got.valid, true);
  assert.equal(got.upper, 30);
});
for (const [name, frames] of [
  ['backwards clock', [frame(120), frame(110)]],
  ['missing displayTime', [frame(null)]],
  ['blank frame', [frame(110, { status: 'blank' })]],
  ['scale changes', [frame(110, { scale: 1 })]],
  ['ROI changes', [frame(110, { width: 401 })]],
  ['capture reports a dropped sample', [frame(110, { dropped: true })]],
]) test(name + ' cannot support PASS', () => assert.equal(latencyBounds(event, frames).valid, false));

test('cursor-only differences cannot match body without templates', () => {
  const got = latencyBounds(event, [frame(110, { changed: true }), frame(130, { cursorChanged: true })]);
  assert.equal(got.complete, false);
  assert.equal(got.upper, null);
  assert.equal(got.lower, 98);
});
test('missing input stays right-censored, never disappears', () => {
  const results = Array.from({ length: 20 }, (_, i) => ({ id: i, lower: 5, upper: i < 18 ? 10 : null, valid: true, complete: i < 18 }));
  const s = summarize(results);
  assert.equal(s.posted, 20);
  assert.equal(s.complete, 18);
  assert.equal(s.p95Upper, null);
  assert.equal(s.verdict, 'not-proven');
});
test('nearest-rank p95 and invalid observation are separately reported', () => {
  const results = Array.from({ length: 100 }, (_, i) => ({ lower: i, upper: i + 2, complete: true, valid: i !== 0 }));
  const s = summarize(results);
  assert.equal(s.p95Lower, 94);
  assert.equal(s.p95Upper, 96);
  assert.equal(s.invalid, 1);
  assert.equal(s.verdict, 'not-proven');
});
test('three pairs retain pair identity, not a fake per-key confidence interval', () => {
  const a = [{ pair: 1, p95Lower: 10, p95Upper: 20 }];
  const b = [{ pair: 1, p95Lower: 15, p95Upper: 25 }];
  assert.deepEqual(comparePairs(a, b), [{ pair: 1, conclusion: 'cannot-distinguish', a: [10, 20], b: [15, 25] }]);
});
