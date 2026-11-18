// Synthetic documents identical to the Go fixture (fixture/main.go: gen,
// typingDocument, the multi-caret setup) so Tango and Monaco edit the same
// bytes. Offsets are UTF-8 byte offsets like the Go fixture reports; the
// document is ASCII, so they equal UTF-16 offsets for Monaco too.

export function gen(n) {
  let s = '';
  for (let i = 0; i < n; i++) s += `export const value${i} = compute(${i}, "row ${i}"); // fixture\n`;
  return s;
}

const TYPING_LINE = 20;

export function typingDocument(text, line = TYPING_LINE) {
  const lines = text.split(/(?<=\n)/);
  if (line >= 0 && line < lines.length) lines[line] = '// qq' + ' '.repeat(160) + '\n';
  return lines.join('');
}

export function fixtureText(scenario) {
  return typingDocument(gen(scenario === 'multi' ? 6000 : 2000));
}

function lineStart(text, line) {
  let off = 0;
  for (let i = 0; i < line; i++) off = text.indexOf('\n', off) + 1;
  return off;
}

// caretOffsets: the first caret after "// qq" on line 20, the others two
// columns into lines 24, 28, ... (fixture/main.go).
export function caretOffsets(text, carets) {
  const out = [lineStart(text, TYPING_LINE) + '// qq'.length];
  for (let k = 1; k < carets; k++) out.push(lineStart(text, TYPING_LINE + k * 4) + 2);
  return out;
}

// fnv64a is the Go fixture's checksum (hash/fnv New64a over UTF-8 bytes).
export function fnv64a(s) {
  let h = 0xcbf29ce484222325n;
  for (const b of Buffer.from(s, 'utf8')) {
    h ^= BigInt(b);
    h = (h * 0x100000001b3n) & 0xffffffffffffffffn;
  }
  return h.toString(16).padStart(16, '0');
}
