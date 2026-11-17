// Returns the only inserted contiguous text, or null when the edit also
// removed/replaced any byte. It never reads candidate lists.
export function insertedText(before, after) {
  let i = 0;
  while (i < before.length && i < after.length && before[i] === after[i]) i++;
  let b = before.length, a = after.length;
  while (b > i && a > i && before[b - 1] === after[a - 1]) { b--; a--; }
  return b === i ? after.slice(i, a) : null;
}

// Fixed IME oracles: expectations are chosen before typing. Candidate text
// is never copied into a report.
const committed = 'aa\n// qq你好\n';
function common(before, after) {
  const comp = after.trace?.some(r => r.Kind === 'composition' && r.Text);
  const commit = after.trace?.some(r => r.Kind === 'text' && r.Text);
  if (!comp || !commit) return 'missing real composition/commit trace';
  if (after.composingText !== before.text) return 'model composition changed before commit';
  return '';
}
export function checkIMECase(name, before, after) {
  let expected = committed;
  if (name === 'multi') expected = 'aa你好\nb你好\nc你好\n';
  if (name === 'cancel' || name === 'undo') expected = before.text;
  const reason = name === 'cancel' ? (after.trace?.some(r => r.Kind === 'composition' && r.Text) && after.composingText === before.text && after.text === before.text ? '' : 'cancel did not preserve the document')
    : common(before, after) || (after.text !== expected ? 'expected exact text' : '') ||
      (name === 'undo' && after.redoText !== committed ? 'redo did not restore commit' : '');
  return { case: name, pass: !reason, reason, expectedChecksumOnly: Boolean(expected) };
}
