// refgen replays terminal output through headless xterm.js and writes the
// resulting screen as a JSON snapshot next to each input. See README.md.
//
// Usage: node refgen.mjs FILE.vt|FILE.ops ...

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { Terminal } = require('@xterm/headless');
const { UnicodeGraphemesAddon } = require('@xterm/addon-unicode-graphemes');

// Go vt.Color encoding: kind in the high byte (1 indexed, 2 RGB).
const COLOR_INDEXED = 1 << 24;
const COLOR_RGB = 2 << 24;

// Go vt.Attr bits.
const ATTR_BOLD = 1, ATTR_FAINT = 2, ATTR_ITALIC = 4, ATTR_UNDERLINE = 8,
  ATTR_BLINK = 16, ATTR_INVERSE = 32, ATTR_INVISIBLE = 64, ATTR_STRIKE = 128;

function fgOf(c) {
  if (c.isFgPalette()) return COLOR_INDEXED + c.getFgColor();
  if (c.isFgRGB()) return COLOR_RGB + c.getFgColor();
  return 0;
}

function bgOf(c) {
  if (c.isBgPalette()) return COLOR_INDEXED + c.getBgColor();
  if (c.isBgRGB()) return COLOR_RGB + c.getBgColor();
  return 0;
}

function attrsOf(c) {
  let a = 0;
  if (c.isBold()) a |= ATTR_BOLD;
  if (c.isDim()) a |= ATTR_FAINT;
  if (c.isItalic()) a |= ATTR_ITALIC;
  if (c.isUnderline()) a |= ATTR_UNDERLINE;
  if (c.isBlink()) a |= ATTR_BLINK;
  if (c.isInverse()) a |= ATTR_INVERSE;
  if (c.isInvisible()) a |= ATTR_INVISIBLE;
  if (c.isStrikethrough()) a |= ATTR_STRIKE;
  // Overline has no Go Attr bit (M1) and is not exported.
  return a;
}

// sizeOf reads NAME.size ("80x24") or returns the 80×24 default.
function sizeOf(base) {
  const f = base + '.size';
  if (!existsSync(f)) return [80, 24];
  const m = /^\s*(\d+)x(\d+)\s*$/.exec(readFileSync(f, 'utf8'));
  if (!m) throw new Error(`${f}: want COLSxROWS`);
  return [Number(m[1]), Number(m[2])];
}

// opsOf returns the steps to replay: a .vt file is one write of its raw
// bytes; a .ops file is a JSON array of {"write": "text"} and
// {"resize": [cols, rows]} steps.
function opsOf(file) {
  if (file.endsWith('.vt')) return [{ write: readFileSync(file) }];
  const ops = JSON.parse(readFileSync(file, 'utf8'));
  return ops.map((op) => {
    if (typeof op.write === 'string') return { write: Buffer.from(op.write, 'utf8') };
    if (Array.isArray(op.resize) && op.resize.length === 2) return { resize: op.resize };
    throw new Error(`${file}: bad step ${JSON.stringify(op)}`);
  });
}

function write(term, data) {
  return new Promise((resolve) => term.write(data, resolve));
}

function snapshot(term, replies) {
  const buf = term.buffer.active;
  const lines = [];
  const cell = buf.getNullCell();
  for (let y = 0; y < term.rows; y++) {
    const line = buf.getLine(buf.baseY + y);
    const out = { text: line.translateToString(true) };
    const cells = [];
    const wide = [];
    const chars = [];
    let seg = null;
    for (let x = 0; x < term.cols; x++) {
      line.getCell(x, cell);
      if (cell.getWidth() === 2) wide.push(x);
      // Cells holding anything but one ASCII character, so the column of
      // each code point is known (text alone does not tell a zero-width
      // mark joined to a character from one in a cell of its own).
      const ch = cell.getChars();
      if (ch.length > 1 || (ch.length === 1 && ch.charCodeAt(0) > 0x7e)) chars.push([x, ch]);
      const fg = fgOf(cell), bg = bgOf(cell), at = attrsOf(cell);
      if (fg === 0 && bg === 0 && at === 0) {
        seg = null;
        continue;
      }
      if (seg && seg[2] === fg && seg[3] === bg && seg[4] === at && seg[0] + seg[1] === x) {
        seg[1]++;
      } else {
        seg = [x, 1, fg, bg, at];
        cells.push(seg);
      }
    }
    if (cells.length) out.cells = cells;
    if (wide.length) out.wide = wide;
    if (chars.length) out.chars = chars;
    if (line.isWrapped) out.wrapped = true;
    lines.push(out);
  }
  const snap = { cols: term.cols, rows: term.rows, cursor: [buf.cursorX, buf.cursorY], lines };
  if (replies) snap.replies = replies;
  return snap;
}

async function gen(file) {
  const base = file.replace(/\.(vt|ops)$/, '');
  const [cols, rows] = sizeOf(base);
  const term = new Terminal({ cols, rows, scrollback: 10000, allowProposedApi: true });
  term.loadAddon(new UnicodeGraphemesAddon());
  if (term.unicode.activeVersion !== '15-graphemes') {
    throw new Error(`unicode version is ${term.unicode.activeVersion}, want 15-graphemes`);
  }
  let replies = '';
  term.onData((d) => { replies += d; });
  for (const op of opsOf(file)) {
    if (op.write) await write(term, op.write);
    else term.resize(op.resize[0], op.resize[1]);
  }
  const json = JSON.stringify(snapshot(term, replies), null, 0)
    // One line per screen row keeps diffs readable.
    .replace(/,\{"text"/g, ',\n{"text"').replace('"lines":[{', '"lines":[\n{');
  writeFileSync(base + '.json', json + '\n');
  term.dispose();
}

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error('usage: node refgen.mjs FILE.vt|FILE.ops ...');
  process.exit(2);
}
for (const f of files) {
  await gen(f);
  console.log(f.replace(/\.(vt|ops)$/, '.json'));
}
