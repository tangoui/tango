import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureText, caretOffsets, fnv64a } from './fixture-text.mjs';

// Goldens read from the Go fixture's /setup (tools/editor-gui-bench/fixture).
test('type document matches the Go fixture byte for byte', () => {
  const t = fixtureText('type');
  assert.equal(Buffer.byteLength(t), 122779);
  assert.equal(fnv64a(t), 'dbee5b8a51b8222e');
  assert.deepEqual(caretOffsets(t, 1), [1115]);
});

test('multi document and carets match the Go fixture', () => {
  const t = fixtureText('multi');
  assert.equal(Buffer.byteLength(t), 374779);
  assert.equal(fnv64a(t), '0217bdb73deec4ae');
  assert.deepEqual(caretOffsets(t, 3), [1115, 1449, 1677]);
});

test('fnv64a hashes UTF-8 bytes', () => {
  assert.equal(fnv64a(''), 'cbf29ce484222325');
  assert.notEqual(fnv64a('你好'), fnv64a('ni'));
});
