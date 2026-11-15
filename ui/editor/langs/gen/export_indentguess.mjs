// Dev-time exporter for ui/editor/indentguess_test.go: golden results of
// Monaco 0.56's guessIndentation (esm/vs/editor/common/model/indentationGuesser.js,
// which has no imports) over hand-made samples.
//
// Usage: node export_indentguess.mjs   (no network; needs node_modules/monaco-editor)
// Writes ../../testdata/indentguess/samples.json:
//   [{"name", "text", "results": [{"defaultTabSize", "defaultInsertSpaces",
//     "tabSize", "insertSpaces"}]}]
// Lines are split like Monaco's text buffer: "\r\n", "\r" and "\n" all break.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const MONACO_VERSION = '0.56.0';
const here = path.dirname(fileURLToPath(import.meta.url));
const monaco = path.join(here, 'node_modules', 'monaco-editor');
const pkg = JSON.parse(fs.readFileSync(path.join(monaco, 'package.json'), 'utf8'));
if (pkg.version !== MONACO_VERSION) {
	console.error(`monaco-editor ${pkg.version}, want ${MONACO_VERSION}`);
	process.exit(1);
}
const { guessIndentation } = await import(pathToFileURL(
	path.join(monaco, 'esm/vs/editor/common/model/indentationGuesser.js')).href);

function source(text) {
	const lines = text.split(/\r\n|\r|\n/);
	return {
		getLineCount: () => lines.length,
		getLineLength: (n) => lines[n - 1].length,
		getLineContent: (n) => lines[n - 1],
		getLineCharCode: (n, i) => lines[n - 1].charCodeAt(i),
	};
}

const rep = (s, n) => s.repeat(n);
const block = (ind, depth) => {
	let out = 'function f() {\n';
	for (let d = 1; d <= depth; d++) out += ind.repeat(d) + `if (a${d}) {\n`;
	out += ind.repeat(depth + 1) + 'call();\n';
	for (let d = depth; d >= 1; d--) out += ind.repeat(d) + '}\n';
	return out + '}\n';
};

const samples = [
	['tabs', block('\t', 3)],
	['spaces2', block('  ', 3)],
	['spaces4', block('    ', 3)],
	['spaces8', block('        ', 2)],
	['spaces3', block('   ', 3)],
	['spaces6', block('      ', 2)],
	['mixed_tabs_majority', block('\t', 3) + block('  ', 1)],
	['mixed_spaces_majority', block('    ', 3) + block('\t', 1)],
	['mixed_equal_lines', 'a\n\tb\nc\n    d\n'],
	['comments_only', '// first\n// second\n/* third */\n# fourth\n'],
	['empty', ''],
	['whitespace_only', '   \n\t\n  \n\n'],
	['over_10000_lines_tabs_then_spaces',
		rep('a\n\tb\n', 5000) + rep('c\n  d\n    e\n', 2000)],
	['over_10000_lines_flat_then_spaces',
		rep('x\n', 10000) + rep('c\n  d\n    e\n', 100)],
	['crlf_spaces2', block('  ', 3).replace(/\n/g, '\r\n')],
	['crlf_tabs', block('\t', 2).replace(/\n/g, '\r\n')],
	['lone_cr_spaces4', block('    ', 2).replace(/\n/g, '\r')],
	['jsdoc_spaces4', 'class A {\n    /**\n     * Doc line.\n     * @param x\n     */\n    m(x) {\n        return x;\n    }\n\n    /**\n     * More.\n     */\n    n() {}\n}\n'],
	['jsdoc_spaces2', 'class A {\n  /**\n   * Doc line.\n   * @param x\n   */\n  m(x) {\n    return x;\n  }\n}\n'],
	['jsdoc_tabs', 'class A {\n\t/**\n\t * Doc line.\n\t * @param x\n\t */\n\tm(x) {\n\t\treturn x;\n\t}\n}\n'],
	['one_space_alignment', 'a\n b\n  c\n   d\n    e\n'],
	['comma_alignment', 'const a = 1,\n      b = 2;\nlet c = 3,\n    d = 4;\nfunction g() {\n  x();\n}\n'],
	['comma_alignment_eq_default', 'foo(a,\n    b);\nbar(c,\n    d);\nif (x) {\n  y();\n}\n'],
	['yaml_2_beats_4', 'a:\n  b:\n    c: 1\n  d:\n    e:\n        f: 1\n    g: 2\nh:\n  i: 1\n'],
	['four_predominant_some_2', 'a {\n    b {\n        c;\n    }\n    d {\n        e;\n    }\n    f {\n      g;\n    }\n}\n'],
	['tab_plus_spaces_lines', 'a\n\t    b\n\t    c\n\td\n\t\te\n'],
	['tab_then_tab_spaces', '\tfoo\n\t    bar\n\tbaz\n\t    qux\n'],
	['tabs_vs_spaces_diff_multiple', 'a\n\t\tb\nc\n        d\n'],
	['single_space_lines', 'a\n b\n c\n d\n\te\n'],
	['python_spaces4_blank_lines', 'def f():\n    x = 1\n\n    if x:\n\n        return x\n\n\nclass C:\n    pass\n'],
	['jump_over_8', 'a\n            b\nc\n            d\ne\n  f\n'],
	['long_line_indented', '  ' + 'x'.repeat(70000) + '\nfoo\n  bar\n    baz\n'],
	['unicode_alignment_bmp', rep('é b,\n  x;\n', 5) + 'if (q) {\n    y();\n}\n'],
	['unicode_alignment_astral', rep('😀 b,\n   x;\n', 5) + 'if (q) {\n    y();\n}\n'],
	['html_nested_2', '<html>\n  <body>\n    <div>\n      <p>x</p>\n    </div>\n  </body>\n</html>\n'],
	['makefile_tabs_with_space_vars', 'VAR = a \\\n      b\nall:\n\tgo build\n\tgo test\n'],
];

const defaults = [[4, true], [2, false], [8, true], [2, true]];
const out = samples.map(([name, text]) => ({
	name,
	text,
	results: defaults.map(([ts, sp]) => {
		const g = guessIndentation(source(text), ts, sp);
		return { defaultTabSize: ts, defaultInsertSpaces: sp, tabSize: g.tabSize, insertSpaces: g.insertSpaces };
	}),
}));

const dir = path.join(here, '..', '..', 'testdata', 'indentguess');
fs.mkdirSync(dir, { recursive: true });
fs.writeFileSync(path.join(dir, 'samples.json'), JSON.stringify(out, null, 1) + '\n');
for (const s of out) {
	console.log(s.name.padEnd(36), s.results.map((r) => `${r.tabSize}${r.insertSpaces ? 's' : 't'}`).join(' '));
}
console.log(`${out.length} samples`);
