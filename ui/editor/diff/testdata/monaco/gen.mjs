// Dev-time fixture generator: runs Monaco's DefaultLinesDiffComputer
// (monaco-editor 0.56, installed by `npm ci` in ui/editor/langs/gen) on the
// cases below and writes cases.json, which diff_monaco_test.go compares
// against. Positions are converted to 0-based lines and UTF-8 byte columns.
//
//   node ui/editor/diff/testdata/monaco/gen.mjs
//
// MONACO_DIR overrides the monaco-editor package directory.
import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const monacoDir = process.env.MONACO_DIR ?? join(here, '../../../langs/gen/node_modules/monaco-editor');
const { DefaultLinesDiffComputer } = await import(pathToFileURL(join(monacoDir,
  'esm/vs/editor/common/diff/defaultLinesDiffComputer/defaultLinesDiffComputer.js')).href);

const computer = new DefaultLinesDiffComputer();

const byteCol = (lines, lineNumber, column) =>
  Buffer.byteLength(lines[lineNumber - 1].slice(0, column - 1), 'utf8');

function range(lines, r) {
  return [r.startLineNumber - 1, byteCol(lines, r.startLineNumber, r.startColumn),
    r.endLineNumber - 1, byteCol(lines, r.endLineNumber, r.endColumn)];
}

function change(orig, mod, c) {
  return [c.original.startLineNumber - 1, c.original.endLineNumberExclusive - 1,
    c.modified.startLineNumber - 1, c.modified.endLineNumberExclusive - 1,
    (c.innerChanges ?? []).map(ic => [...range(orig, ic.originalRange), ...range(mod, ic.modifiedRange)])];
}

function run(name, original, modified, opts = {}) {
  const o = typeof original === 'string' ? original.split('\n') : original;
  const m = typeof modified === 'string' ? modified.split('\n') : modified;
  const options = {
    ignoreTrimWhitespace: !!opts.ignoreTrimWhitespace,
    computeMoves: !!opts.computeMoves,
    extendToSubwords: !!opts.extendToSubwords,
    maxComputationTimeMs: 0,
  };
  const r = computer.computeDiff(o, m, options);
  return {
    name, original: o, modified: m,
    ignoreTrimWhitespace: options.ignoreTrimWhitespace,
    computeMoves: options.computeMoves,
    extendToSubwords: options.extendToSubwords,
    changes: r.changes.map(c => change(o, m, c)),
    moves: r.moves.map(mv => [
      mv.lineRangeMapping.original.startLineNumber - 1, mv.lineRangeMapping.original.endLineNumberExclusive - 1,
      mv.lineRangeMapping.modified.startLineNumber - 1, mv.lineRangeMapping.modified.endLineNumberExclusive - 1,
      mv.changes.map(c => change(o, m, c))]),
  };
}

const cases = [];
let crashed = 0;
const add = (...args) => {
  // Monaco 0.56 occasionally throws in computeMovedLines (it reads line 0
  // when extending a move upwards); such inputs are skipped.
  try { cases.push(run(...args)); } catch (e) { crashed++; }
};

const goFile = `package main

import (
	"fmt"
	"strings"
)

// greet says hello.
func greet(name string) string {
	return "hello, " + name
}

func sum(xs []int) int {
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

func main() {
	fmt.Println(greet("world"))
	fmt.Println(sum([]int{1, 2, 3}))
	fmt.Println(strings.ToUpper("done"))
}`;

add('replace-line', 'a\nb\nc', 'a\nx\nc');
add('insert-lines', 'a\nb\nc', 'a\nb\nx\ny\nc');
add('delete-lines', 'a\nb\nc\nd', 'a\nd');
add('insert-start', 'a\nb', 'x\na\nb');
add('insert-end', 'a\nb', 'a\nb\nx');
add('delete-end', 'a\nb\nc', 'a\nb');
add('delete-start', 'a\nb\nc', 'b\nc');
add('add-trailing-newline', 'a\nb', 'a\nb\n');
add('remove-trailing-newline', 'a\nb\n', 'a\nb');
add('all-changed', 'one\ntwo\nthree', 'alpha\nbeta');
add('empty-original', '', 'a\nb');
add('empty-modified', 'a\nb', '');
add('single-line-edit', 'hello world', 'hello there world');
add('whitespace-consider', 'a\n  b\nc', 'a\n    b\nc');
add('whitespace-ignore', 'a\n  b\nc', 'a\n    b\nc', { ignoreTrimWhitespace: true });
add('trailing-ws-ignore', 'x = 1;  \ny = 2;', 'x = 1;\ny = 3;', { ignoreTrimWhitespace: true });
add('go-rename', goFile, goFile.replaceAll('greet', 'salute'));
add('go-insert-func', goFile, goFile.replace('func main() {', 'func double(x int) int {\n\treturn 2 * x\n}\n\nfunc main() {'));
add('go-delete-func', goFile, goFile.replace(/\/\/ greet says hello\.\nfunc greet[\s\S]*?\n}\n\n/, ''));
add('go-indent-block', goFile, goFile.replace('\tfor _, x := range xs {\n\t\ttotal += x\n\t}', '\tif len(xs) > 0 {\n\t\tfor _, x := range xs {\n\t\t\ttotal += x\n\t\t}\n\t}'));
add('go-indent-block-ignore-ws', goFile, goFile.replace('\tfor _, x := range xs {\n\t\ttotal += x\n\t}', '\tif len(xs) > 0 {\n\t\tfor _, x := range xs {\n\t\t\ttotal += x\n\t\t}\n\t}'), { ignoreTrimWhitespace: true });
add('words', 'const value = computeTotal(items, taxRate);', 'const result = computeSubtotal(items, discountRate);');
add('subwords', 'const value = computeTotal(items, taxRate);', 'const value = computeSubTotal(items, taxRates);', { extendToSubwords: true });
add('imports', 'import { IBar } from "foo";', 'import { IArr, IBar } from "foo";');
add('brackets', 'collectBrackets(level + 1, levelPerBracketType);', 'collectBrackets(level + 1, levelPerBracket + 1, levelPerBracketType);');
add('unicode', '你好，世界\nemoji 😀 here\nend', '你好，地球\nemoji 😃 here!\nend');
add('repeated-lines', 'a\n}\n\n}\nb', 'a\n}\n\n}\n\n}\nb');
add('blank-lines', 'x\n\n\ny', 'x\n\n\n\ny');
add('long-line', 'x'.repeat(400) + 'A' + 'y'.repeat(400), 'x'.repeat(400) + 'B' + 'y'.repeat(400));

const block = ['func moved(a, b int) int {', '\tif a > b {', '\t\treturn a - b', '\t}', '\treturn b - a', '}'];
const filler = Array.from({ length: 12 }, (_, i) => `var v${i} = compute(${i}, "filler line ${i}")`);
add('move-block', [...block, ...filler].join('\n'), [...filler, ...block].join('\n'), { computeMoves: true });
add('move-block-modified', [...block, ...filler].join('\n'),
  [...filler, ...block.map(l => l.replace('a - b', 'a-b+0'))].join('\n'), { computeMoves: true });
add('move-unchanged-inside-changes', ['start', ...block, 'x1', 'x2', 'x3', ...filler.slice(0, 4), 'end'].join('\n'),
  ['start', 'y1', 'y2', ...filler.slice(0, 4), ...block, 'y3', 'end'].join('\n'), { computeMoves: true });

// A file longer than the window margin, with scattered edits.
const big = Array.from({ length: 400 }, (_, i) => i % 7 === 0 ? '' : `\tline${i} := process(input[${i}], ${i * 3})`);
const big2 = big.slice();
big2[50] = '\tline50 := processFast(input[50], 150)';
big2.splice(200, 0, '\t// inserted comment', '\tdebug()');
big2.splice(350, 3);
add('big-scattered', big.join('\n'), big2.join('\n'));

// Seeded random cases to cross-check the port.
let seed = 12345;
const rnd = (n) => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed % n; };
const vocab = ['', '{', '}', '\treturn x', 'if (a) {', '  foo(bar);', 'let x = 1;', 'let y = x + 2;', '// comment', 'print(x, y)', '    indented', 'end'];
function mutateLine(l) {
  const chars = 'abxyz (){};,=+ \t';
  const p = rnd(l.length + 1);
  switch (rnd(3)) {
    case 0: return l.slice(0, p) + chars[rnd(chars.length)] + l.slice(p);
    case 1: return l.slice(0, p) + l.slice(p + 1 + rnd(3));
    default: return l.slice(0, p) + chars[rnd(chars.length)] + l.slice(p + 1);
  }
}
for (let t = 0; t < 150; t++) {
  const n = 1 + rnd(25);
  const a = Array.from({ length: n }, () => vocab[rnd(vocab.length)]);
  const b = a.slice();
  const edits = 1 + rnd(4);
  for (let e = 0; e < edits; e++) {
    const p = rnd(b.length + 1);
    switch (rnd(4)) {
      case 0: b.splice(p, 0, vocab[rnd(vocab.length)]); break;
      case 1: if (b.length > 1) b.splice(Math.min(p, b.length - 1), 1 + rnd(2)); break;
      case 2: if (b.length > 0) { const q = Math.min(p, b.length - 1); b[q] = mutateLine(b[q]); } break;
      default: { // move a few lines
        const len = 1 + rnd(4); const from = rnd(b.length); const moved = b.splice(from, len);
        b.splice(rnd(b.length + 1), 0, ...moved);
      }
    }
  }
  if (b.length === 0) b.push('');
  add(`random-${t}`, a, b, { ignoreTrimWhitespace: rnd(3) === 0, computeMoves: rnd(2) === 0, extendToSubwords: rnd(4) === 0 });
}

// Random cases with longer lines and block moves, to exercise move detection.
const vocab2 = Array.from({ length: 30 }, (_, i) => `\tresult${i % 9} = transform(value${i}, "${'abcdefghij'[i % 10]}", ${i * 7});`);
for (let t = 0; t < 60; t++) {
  const n = 10 + rnd(30);
  const a = Array.from({ length: n }, () => vocab2[rnd(vocab2.length)]);
  const b = a.slice();
  const edits = 1 + rnd(3);
  for (let e = 0; e < edits; e++) {
    const len = 3 + rnd(6); const from = rnd(b.length); const moved = b.splice(from, len);
    if (rnd(2) === 0 && moved.length > 0) moved[rnd(moved.length)] = mutateLine(moved[0]);
    b.splice(rnd(b.length + 1), 0, ...moved);
    if (rnd(2) === 0) b.splice(rnd(b.length + 1), 0, vocab2[rnd(vocab2.length)]);
  }
  add(`random-move-${t}`, a, b, { computeMoves: true, ignoreTrimWhitespace: rnd(2) === 0 });
}

// Large enough for Monaco to switch from dynamic programming to Myers.
const huge = Array.from({ length: 2000 }, (_, i) => i % 5 === 0 ? '}' : `\tstep${i}(state, ${i % 13});`);
const huge2 = huge.slice();
huge2[100] = '\tstep100(state, 99);';
huge2.splice(1000, 2, '\tnewStep(state);');
huge2.splice(1500, 0, '}', '\tstep1500(state, 5);');
add('myers-large', huge, huge2);

writeFileSync(join(here, 'cases.json'), JSON.stringify(cases, null, 0).replaceAll('},{"name"', '},\n{"name"') + '\n');
console.log(`wrote ${cases.length} cases, skipped ${crashed} that crash Monaco`);
