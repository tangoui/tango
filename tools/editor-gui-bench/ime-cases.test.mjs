import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkIMECase, insertedText } from './ime-cases.mjs';

const before = { text: 'aa\n// qq\n', selections: [{ anchor: 5, head: 5 }], trace: [] };
test('real commit needs marked composition and unchanged model before commit', () => {
  assert.equal(checkIMECase('commit', before, { text: 'aa\n// qq你好\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你好' }], composingText: before.text }).pass, true);
});
for (const [name, after, why] of [
  ['unicode', { text: 'aa\n// qq你好\n', trace: [{ Kind: 'text', Text: '你好' }], composingText: before.text }, 'composition'],
  ['marked edit', { text: 'aa\n// qq你好\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你好' }], composingText: 'aa\n// qqnihao\n' }, 'composition changed'],
  ['wrong word', { text: 'aa\n// qq你号\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你号' }], composingText: before.text }, 'expected'],
]) test(name + ' fails', () => assert.match(checkIMECase('commit', before, after).reason, new RegExp(why)));
test('undo and redo are exact', () => {
  assert.equal(checkIMECase('undo', before, { text: before.text, redoText: 'aa\n// qq你好\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你好' }], composingText: before.text }).pass, true);
});
test('multi-cursor commit reaches every caret', () => {
  const multi = { text: 'aa\nb\nc\n', selections: [{ anchor: 2, head: 2 }, { anchor: 4, head: 4 }, { anchor: 6, head: 6 }] };
  assert.equal(checkIMECase('multi', multi, { text: 'aa你好\nb你好\nc你好\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你好' }], composingText: multi.text }).pass, true);
});
test('insertedText proves the real insertion, not merely the whole model text', () => {
  assert.equal(insertedText('aa\n// qq\n', 'aa\n// qq你好\n'), '你好');
  assert.equal(insertedText('aa\n// qq\n', 'aa\n// qq\n'), '');
  assert.equal(insertedText('aa\n// qq\n', 'aa\n// qX\n'), null);
});

test('candidate list is not part of reported evidence', () => {
  const got = checkIMECase('commit', before, { text: 'aa\n// qq你好\n', trace: [{ Kind: 'composition', Text: 'nihao' }, { Kind: 'text', Text: '你好' }], composingText: before.text, candidates: ['private'] });
  assert.equal(JSON.stringify(got).includes('private'), false);
});
