// Dev-time exporter for ui/editor/theme: Monaco's default colors.
//
// Loads the color registry of monaco-editor@0.56.0 (every module under
// esm/vs that calls registerColor, bundled with esbuild and run under a tiny
// DOM shim: the editor widgets touch window/document at import time) and
// writes ../../theme/colors_gen.go with, for every registered color id:
//
//   defaultColors  the final color under each base theme (vs, vs-dark,
//                  hc-black, hc-light), resolved as Monaco's StandaloneTheme
//                  does: the base theme's own colors first, then the
//                  registry default with its references and transforms
//                  (transparent, lighten, darken, oneOf, lessProminent, ...)
//                  resolved against that theme;
//   defaultUnset   the bases under which the id has no color (a null default);
//   colorDefaults  the registry default expressions themselves, so the Go side
//                  can resolve them against a custom theme's own colors.
//
// Usage: npm install && node export_colors.mjs
// It exits 1 without writing anything when a default references an id that
// is neither registered nor a base theme color, uses a transform it does not
// know, or the counts below differ (the pinned package changed).

import * as esbuild from 'esbuild';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const MONACO_VERSION = '0.56.0';
const EXPECTED = {
	// Modules under esm/vs that call registerColor( (colorUtils.js, which
	// defines it, excluded).
	modules: 25,
	// Registered color ids.
	colors: 431,
};

// Ids the registry defaults reference that the standalone editor never
// registers (VS Code workbench colors). Monaco resolves them to no color
// unless the theme defines them itself, and so does the Go side; any other
// unknown reference fails the export.
const DANGLING = new Set(['sideBar.background', 'sideBarSectionHeader.border', 'tab.inactiveBackground']);

const here = path.dirname(fileURLToPath(import.meta.url));
const monacoRoot = path.join(here, 'node_modules', 'monaco-editor');
const esmVs = path.join(monacoRoot, 'esm', 'vs');
const outFile = path.resolve(here, '..', '..', 'theme', 'colors_gen.go');

const errors = [];
const dangling = new Set();
const fail = (msg) => errors.push(msg);

// The DOM shim: every global the modules touch at import time is a proxy
// that absorbs any property access, call or construction. Nothing of the
// DOM is used by the color registry itself.
const SHIM_GLOBALS = [
	'window', 'document', 'navigator', 'self', 'matchMedia', 'requestAnimationFrame', 'HTMLElement', 'Element', 'Node',
	'MutationObserver', 'ResizeObserver', 'CSS', 'getComputedStyle', 'location', 'trustedTypes', 'customElements', 'UIEvent',
];
const SHIM = `
(() => {
	const stub = (name) => new Proxy(function () {}, {
		get(t, k) {
			if (k === Symbol.toPrimitive) return () => '';
			if (k === Symbol.iterator) return function* () {};
			if (k === 'then') return undefined;
			if (k === 'length') return 0;
			if (k === 'toString' || k === 'valueOf') return () => '';
			return stub(name + '.' + String(k));
		},
		set() { return true; },
		construct() { return stub('new ' + name); },
		apply() { return stub(name + '()'); },
		has() { return true; },
	});
	for (const g of ${JSON.stringify(SHIM_GLOBALS)}) {
		if (!(g in globalThis)) Object.defineProperty(globalThis, g, { value: stub(g), configurable: true, writable: true });
	}
})();
`;

function colorModules() {
	const out = [];
	const walk = (dir) => {
		for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
			const p = path.join(dir, e.name);
			if (e.isDirectory()) walk(p);
			else if (p.endsWith('.js') && !p.endsWith(path.join('theme', 'common', 'colorUtils.js')) &&
				fs.readFileSync(p, 'utf8').includes('registerColor(')) out.push(p);
		}
	};
	walk(esmVs);
	return out.sort();
}

async function load(modules) {
	const imp = (rel) => JSON.stringify(path.join(esmVs, rel));
	const entry = [
		...modules.map((m) => `import ${JSON.stringify(m)};`),
		`export { Registry } from ${imp('platform/registry/common/platform.js')};`,
		`export { Extensions } from ${imp('platform/theme/common/colorUtils.js')};`,
		`export { Color } from ${imp('base/common/color.js')};`,
		`export * as themes from ${imp('editor/standalone/common/themes.js')};`,
	].join('\n');
	const result = await esbuild.build({
		stdin: { contents: entry, resolveDir: here, loader: 'js' },
		bundle: true,
		write: false,
		format: 'esm',
		platform: 'node',
		logLevel: 'silent',
		loader: { '.css': 'empty' },
		banner: { js: SHIM },
	});
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tango-colors-gen-'));
	try {
		const file = path.join(dir, 'bundle.mjs');
		fs.writeFileSync(file, result.outputFiles[0].text);
		return await import(pathToFileURL(file).href);
	} finally {
		fs.rmSync(dir, { recursive: true, force: true });
	}
}

// Base theme order of the Go tables, and Monaco's theme type of each.
const BASES = [
	['vs', 'vs', 'light'],
	['vs-dark', 'vs_dark', 'dark'],
	['hc-black', 'hc_black', 'hcDark'],
	['hc-light', 'hc_light', 'hcLight'],
];

// A theme as Monaco's StandaloneTheme resolves colors (getColor, defines,
// type), for one base theme's own data.
function makeTheme(mod, registry, data, type, known) {
	const own = new Map(Object.entries(data.colors).map(([k, v]) => [k, mod.Color.fromHex(v)]));
	const memo = new Map();
	const theme = {
		type,
		defines: (id) => own.has(id),
		getColor(id) {
			if (own.has(id)) return own.get(id);
			if (!known.has(id)) {
				if (!DANGLING.has(id)) fail(`${data.base}: reference to unknown color id ${id}`);
				return undefined;
			}
			if (!memo.has(id)) memo.set(id, registry.resolveDefaultColor(id, theme));
			return memo.get(id);
		},
	};
	return theme;
}

// Go source of a color value (Monaco's ColorValue), checking every
// reference and transform.
function goValue(mod, v, where, known) {
	if (v === null || v === undefined) return 'nil';
	if (typeof v === 'string') {
		if (v.startsWith('#')) return goLit(mod.Color.fromHex(v));
		if (!known.has(v)) {
			if (DANGLING.has(v)) dangling.add(`${v} (from ${where.replace(/\[.*$/, '')})`);
			else fail(`${where}: reference to unknown color id ${v}`);
		}
		return `cRef(${JSON.stringify(v)})`;
	}
	if (v instanceof mod.Color) return goLit(v);
	if (typeof v !== 'object') {
		fail(`${where}: unsupported value ${v}`);
		return 'nil';
	}
	const g = (x, sub) => goValue(mod, x, `${where}.${sub}`, known);
	switch (v.op) {
		case 0: return `cDarken(${g(v.value, 'value')}, ${num(v.factor)})`;
		case 1: return `cLighten(${g(v.value, 'value')}, ${num(v.factor)})`;
		case 2: return `cTransparent(${g(v.value, 'value')}, ${num(v.factor)})`;
		case 3: return `cOpaque(${g(v.value, 'value')}, ${g(v.background, 'background')})`;
		case 4: return `cOneOf(${v.values.map((x, i) => g(x, `values[${i}]`)).join(', ')})`;
		case 5: return `cLessProminent(${g(v.value, 'value')}, ${g(v.background, 'background')}, ${num(v.factor)}, ${num(v.transparency)})`;
		case 6:
			if (typeof v.if !== 'string' || !known.has(v.if)) fail(`${where}: ifDefinedThenElse on ${v.if}`);
			return `cIfDefined(${JSON.stringify(v.if)}, ${g(v.then, 'then')}, ${g(v.else, 'else')})`;
		case 7: return `cMix(${g(v.color, 'color')}, ${g(v.with, 'with')}, ${num(v.ratio)})`;
	}
	fail(`${where}: unknown color transform op ${v.op}`);
	return 'nil';
}

function num(x) {
	if (typeof x !== 'number' || !Number.isFinite(x)) {
		fail(`bad number ${x}`);
		return '0';
	}
	return String(x);
}

function goLit(c) {
	const { r, g, b, a } = c.rgba;
	return `cLit(${r}, ${g}, ${b}, ${num(a)})`;
}

const hex2 = (n) => '0x' + n.toString(16).padStart(2, '0');
const nrgba = (c) => `{${hex2(c.rgba.r)}, ${hex2(c.rgba.g)}, ${hex2(c.rgba.b)}, ${hex2(Math.round(c.rgba.a * 255))}}`;
const css = (c) => '#' + [c.rgba.r, c.rgba.g, c.rgba.b, Math.round(c.rgba.a * 255)].map((n) => n.toString(16).padStart(2, '0')).join('');

async function main() {
	const pkg = JSON.parse(fs.readFileSync(path.join(monacoRoot, 'package.json'), 'utf8'));
	if (pkg.version !== MONACO_VERSION) {
		throw new Error(`monaco-editor ${pkg.version} installed, expected ${MONACO_VERSION} (run npm install)`);
	}
	const modules = colorModules();
	const mod = await load(modules);
	const registry = mod.Registry.as(mod.Extensions.ColorContribution);
	const contributions = registry.getColors();
	const ids = contributions.map((c) => c.id).sort();
	const known = new Set(ids);

	const themes = BASES.map(([name, exp, type]) => {
		const data = mod.themes[exp];
		if (!data) fail(`themes.js does not export ${exp}`);
		for (const k of Object.keys(data?.colors ?? {})) known.add(k);
		return { name, type, data };
	});
	const resolved = themes.map((t) => ({ ...t, theme: makeTheme(mod, registry, t.data, t.type, known) }));

	const isDefaults = (d) => d !== null && typeof d === 'object' && 'light' in d && 'dark' in d;
	const byId = new Map(contributions.map((c) => [c.id, c]));
	const finals = [];
	const exprs = [];
	for (const id of ids) {
		const c = byId.get(id);
		const row = [];
		const ex = [];
		for (const t of resolved) {
			let col;
			try {
				col = t.theme.getColor(id);
			} catch (err) {
				fail(`${t.name} ${id}: ${err.message}`);
			}
			row.push(col);
			const v = isDefaults(c.defaults) ? c.defaults[t.type] : c.defaults;
			ex.push(goValue(mod, v, `${id}[${t.name}]`, known));
		}
		finals.push([id, row]);
		exprs.push([id, ex]);
	}
	// Base theme colors that are not registered ids still resolve (own colors).
	const extra = [...known].filter((k) => !byId.has(k)).sort();
	for (const id of extra) {
		finals.push([id, resolved.map((t) => t.theme.getColor(id))]);
	}
	finals.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));

	const counts = { modules: modules.length, colors: ids.length };
	for (const [k, want] of Object.entries(EXPECTED)) {
		if (counts[k] !== want) fail(`count ${k} = ${counts[k]}, expected ${want}`);
	}

	console.log(`monaco-editor ${MONACO_VERSION}`);
	console.log(`color modules:      ${counts.modules} (expected ${EXPECTED.modules})`);
	console.log(`registered colors:  ${counts.colors} (expected ${EXPECTED.colors}), plus ${extra.length} base theme colors not registered: ${extra.join(', ') || '(none)'}`);
	const unset = finals.map(([id, row]) => [id, row.map((c, i) => (c ? '' : BASES[i][0])).filter(Boolean)]).filter(([, u]) => u.length);
	console.log(`unregistered references (no color unless a theme defines them): ${[...dangling].sort().join(', ') || '(none)'}`);
	for (const d of DANGLING) {
		if (![...dangling].some((x) => x.startsWith(d + ' '))) fail(`DANGLING lists ${d}, no longer referenced`);
	}
	console.log(`ids without a color under some base: ${unset.length}`);
	const samples = ['editorIndentGuide.background1', 'editorIndentGuide.activeBackground1', 'editorBracketHighlight.foreground1',
		'editorBracketHighlight.unexpectedBracket.foreground', 'editorWhitespace.foreground', 'editorBracketMatch.background',
		'editorBracketMatch.border', 'editor.selectionHighlightBackground'];
	for (const id of samples) {
		const row = finals.find((f) => f[0] === id)?.[1] ?? [];
		console.log(`  ${id}: ${row.map((c, i) => `${BASES[i][0]} ${c ? css(c) : '(none)'}`).join(', ')}`);
	}

	if (errors.length) {
		console.error(`\n${errors.length} error(s); nothing written:`);
		for (const e of errors) console.error(`  ${e}`);
		process.exitCode = 1;
		return;
	}

	let go = `// Code generated by ui/editor/langs/gen/export_colors.mjs from monaco-editor@${MONACO_VERSION}'s
// color registry; DO NOT EDIT. Copyright (c) Microsoft Corporation, MIT License (see NOTICE).

package theme

import "image/color"

// defaultColors is every color id's final color under the base themes vs,
// vs-dark, hc-black and hc-light (Monaco's StandaloneTheme.getColor: the
// base theme's own colors, then the registry default resolved against it).
// Bases where defaultUnset has the id's bit set have no color (zero here).
var defaultColors = map[string][4]color.NRGBA{
`;
	for (const [id, row] of finals) {
		go += `\t${JSON.stringify(id)}: {${row.map((c) => (c ? nrgba(c) : '{}')).join(', ')}},\n`;
	}
	go += `}

// defaultUnset marks the ids without a color under some base theme: bit i
// is base i of defaultColors (a null registry default).
var defaultUnset = map[string]uint8{
`;
	for (const [id, row] of finals) {
		const bits = row.reduce((m, c, i) => (c ? m : m | (1 << i)), 0);
		if (bits) go += `\t${JSON.stringify(id)}: ${bits},\n`;
	}
	go += `}

// colorDefaults are the registry's default values per base theme type
// (light, dark, hcDark, hcLight), resolved against a theme by
// Theme.Color for themes with colors of their own (colors.go).
var colorDefaults = map[string][4]*colorValue{
`;
	for (const [id, ex] of exprs) {
		go += `\t${JSON.stringify(id)}: {${ex.join(', ')}},\n`;
	}
	go += '}\n';
	fs.writeFileSync(outFile, go);
	try {
		execFileSync('gofmt', ['-w', outFile], { stdio: 'inherit' });
	} catch (err) {
		console.error(`gofmt: ${err.message}`);
		process.exitCode = 1;
		return;
	}
	console.log(`wrote ${path.relative(process.cwd(), outFile)}`);
}

try {
	await main();
} catch (err) {
	console.error(err);
	process.exitCode = 1;
}
