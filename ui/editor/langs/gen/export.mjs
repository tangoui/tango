// Dev-time exporter for ui/editor/langs.
//
// Converts monaco-editor@0.56.0 data into JSON that the Go package embeds:
//   ../data/<id>.json              one per registered language id
//   ../data/themes.json            the four built-in base themes
//   ../testdata/tokenize/<id>.json official tokenization cases (tag v0.56.0)
//
// Usage: npm install && node export.mjs
// The script ends with a summary of counts and exits 1 if any count differs
// from the EXPECTED table below, or if any value cannot be represented in
// JSON (functions, cycles, non-finite numbers, ...).

import * as esbuild from 'esbuild';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const MONACO_VERSION = '0.56.0';
const MONACO_TAG = 'v0.56.0';
const TARBALL_URL = `https://codeload.github.com/microsoft/monaco-editor/tar.gz/refs/tags/${MONACO_TAG}`;

// Counts observed in monaco-editor@0.56.0. A mismatch means the pinned
// package changed underneath us (or the script regressed) — fail loudly.
const EXPECTED = {
	// Directories under esm/vs/languages/definitions (each one grammar module).
	grammarModules: 81,
	// registerLanguage() calls in Monaco (cpp registers c+cpp, systemverilog
	// registers systemverilog+verilog, freemarker2 registers 7 ids) = 89,
	// plus the extra .vue registration = 90.
	registrations: 90,
	themes: 4,
	// *.test.ts files under src/languages/definitions at tag v0.56.0.
	testFiles: 85,
	// Distinct main language ids across those files (= fixture files written).
	// 85 files but 83 ids: pgsql.test.ts and redshift.test.ts at v0.56.0 call
	// testTokenization('sql', ...), so their cases land in sql.json.
	fixtures: 83,
};

const here = path.dirname(fileURLToPath(import.meta.url));
const langsDir = path.resolve(here, '..');
const dataDir = path.join(langsDir, 'data');
const fixturesDir = path.join(langsDir, 'testdata', 'tokenize');
const monacoRoot = path.join(here, 'node_modules', 'monaco-editor');
const esmVs = path.join(monacoRoot, 'esm', 'vs');
const definitionsDir = path.join(esmVs, 'languages', 'definitions');

const errors = [];

function fail(msg) {
	errors.push(msg);
}

// ---------------------------------------------------------------------------
// JSON conversion: RegExp -> {"$regex": source, "flags": flags}; anything that
// JSON cannot carry faithfully is reported with its path.

function toJSONValue(value, where, stack = []) {
	if (value === null) return null;
	switch (typeof value) {
		case 'string':
		case 'boolean':
			return value;
		case 'number':
			if (!Number.isFinite(value)) fail(`${where}: non-finite number ${value}`);
			return value;
		case 'function':
			fail(`${where}: function value (${value.name || 'anonymous'}) cannot be exported`);
			return null;
		case 'undefined':
			return undefined;
		case 'object':
			break;
		default:
			fail(`${where}: unsupported ${typeof value} value`);
			return null;
	}
	if (value instanceof RegExp) {
		return { $regex: value.source, flags: value.flags };
	}
	if (stack.includes(value)) {
		fail(`${where}: cyclic reference`);
		return null;
	}
	const next = [...stack, value];
	if (Array.isArray(value)) {
		return value.map((v, i) => {
			const out = toJSONValue(v, `${where}[${i}]`, next);
			return out === undefined ? null : out;
		});
	}
	const proto = Object.getPrototypeOf(value);
	if (proto !== Object.prototype && proto !== null) {
		fail(`${where}: unsupported object type ${value.constructor?.name ?? '?'}`);
		return null;
	}
	const out = {};
	for (const key of Object.keys(value)) {
		const v = toJSONValue(value[key], `${where}.${key}`, next);
		if (v !== undefined) out[key] = v;
	}
	for (const sym of Object.getOwnPropertySymbols(value)) {
		fail(`${where}: symbol key ${String(sym)}`);
	}
	return out;
}

// ---------------------------------------------------------------------------
// Bundling helpers: build an ESM bundle in memory and import it via a temp file.

let tmpRoot;
function tmp() {
	tmpRoot ??= fs.mkdtempSync(path.join(os.tmpdir(), 'tango-langs-gen-'));
	return tmpRoot;
}

let bundleSeq = 0;
async function bundleAndImport(options) {
	const result = await esbuild.build({
		bundle: true,
		write: false,
		format: 'esm',
		platform: 'node',
		logLevel: 'silent',
		...options,
	});
	const file = path.join(tmp(), `bundle-${bundleSeq++}.mjs`);
	fs.writeFileSync(file, result.outputFiles[0].text);
	return import(pathToFileURL(file).href);
}

function virtualModulePlugin(name, filter, contents) {
	return {
		name,
		setup(build) {
			build.onResolve({ filter }, (args) => ({ path: args.path, namespace: name }));
			build.onLoad({ filter: /.*/, namespace: name }, () => ({
				contents,
				loader: 'js',
				resolveDir: here,
			}));
		},
	};
}

// ---------------------------------------------------------------------------
// 1. Languages

const REGISTRATION_KEYS = ['id', 'extensions', 'filenames', 'filenamePatterns', 'aliases', 'mimetypes', 'firstLine'];

function normalizeRegistration(def) {
	for (const key of Object.keys(def)) {
		if (key !== 'loader' && !REGISTRATION_KEYS.includes(key)) {
			fail(`registration ${def.id}: unknown field ${key}`);
		}
	}
	return {
		id: def.id,
		extensions: def.extensions ?? [],
		filenames: def.filenames ?? [],
		filenamePatterns: def.filenamePatterns ?? [],
		aliases: def.aliases ?? [],
		mimetypes: def.mimetypes ?? [],
		firstLine: def.firstLine ?? '',
	};
}

async function exportLanguages() {
	const grammarModules = fs
		.readdirSync(definitionsDir, { withFileTypes: true })
		.filter((d) => d.isDirectory())
		.map((d) => d.name)
		.sort();
	for (const dir of grammarModules) {
		if (!fs.existsSync(path.join(definitionsDir, dir, `${dir}.js`))) fail(`definitions/${dir}: missing ${dir}.js`);
		if (!fs.existsSync(path.join(definitionsDir, dir, 'register.js'))) fail(`definitions/${dir}: missing register.js`);
	}

	// Bundle Monaco's register.all.js (it imports every register.js; using it
	// keeps Monaco's registration order) with registerLanguage() replaced by a
	// collector and editor.api.js reduced to the enum the grammars reference
	// (languages.IndentAction, taken from Monaco's own languageConfiguration.js).
	// esbuild inlines the loaders' dynamic import() so loader() runs offline.
	const registerAll = path.join(definitionsDir, 'register.all.js');
	const imported = [...fs.readFileSync(registerAll, 'utf8').matchAll(/^import '\.\/([^/]+)\/register\.js';$/gm)].map(
		(m) => m[1],
	);
	const missing = grammarModules.filter((d) => !imported.includes(d));
	if (missing.length) fail(`register.all.js does not import: ${missing.join(', ')}`);
	const entry = `import ${JSON.stringify(registerAll)};`;
	const languageConfigurationJs = path.join(esmVs, 'editor', 'common', 'languages', 'languageConfiguration.js');
	const mod = await bundleAndImport({
		stdin: { contents: entry + '\nexport const registrations = globalThis.__tangoRegistrations;\n', resolveDir: here, loader: 'js' },
		plugins: [
			virtualModulePlugin(
				'contribution',
				/[\\/]_\.contribution\.js$/,
				`globalThis.__tangoRegistrations ??= [];
export function registerLanguage(def) { globalThis.__tangoRegistrations.push(def); }`,
			),
			virtualModulePlugin(
				'editor-api',
				/[\\/]editor\.api\.js$/,
				`import { IndentAction } from ${JSON.stringify(languageConfigurationJs)};
export const languages = { IndentAction };`,
			),
		],
	});

	const registrations = [];
	for (const def of mod.registrations) {
		const loaded = await def.loader();
		if (!loaded || typeof loaded !== 'object') {
			fail(`registration ${def.id}: loader returned ${loaded}`);
			continue;
		}
		if (!('language' in loaded)) fail(`registration ${def.id}: grammar module has no 'language' export`);
		if (!('conf' in loaded)) fail(`registration ${def.id}: grammar module has no 'conf' export`);
		registrations.push({ registration: normalizeRegistration(def), language: loaded.language, conf: loaded.conf });
	}

	// MinCode (vue-app/src/services/monaco-runtime.js) registers .vue as its own
	// language id that reuses the html Monarch grammar and configuration.
	const html = registrations.find((r) => r.registration.id === 'html');
	if (!html) fail('html registration missing (needed for vue)');
	else {
		registrations.push({
			registration: normalizeRegistration({
				id: 'vue',
				extensions: ['.vue'],
				aliases: ['Vue', 'vue'],
				mimetypes: ['text/x-vue'],
			}),
			language: html.language,
			conf: html.conf,
		});
	}

	const seen = new Set();
	const files = [];
	for (const r of registrations) {
		const id = r.registration.id;
		if (seen.has(id)) fail(`duplicate language id ${id}`);
		seen.add(id);
		files.push([
			`${id}.json`,
			{
				// Position in Monaco's registration sequence (register.all.js
				// order, then vue). Monaco resolves file-association ties in
				// favour of the last registration (e.g. .pp: pascal vs ruby).
				registrationOrder: files.length,
				registration: r.registration,
				language: toJSONValue(r.language, `${id}.language`),
				conf: toJSONValue(r.conf, `${id}.conf`),
			},
		]);
	}
	return { grammarModules: grammarModules.length, files, ids: seen };
}

// ---------------------------------------------------------------------------
// 2. Base themes

async function exportThemes() {
	const themesJs = path.join(esmVs, 'editor', 'standalone', 'common', 'themes.js');
	const mod = await bundleAndImport({
		stdin: { contents: `export * from ${JSON.stringify(themesJs)};`, resolveDir: here, loader: 'js' },
	});
	const named = { vs: mod.vs, 'vs-dark': mod.vs_dark, 'hc-black': mod.hc_black, 'hc-light': mod.hc_light };
	const out = {};
	for (const [name, theme] of Object.entries(named)) {
		if (!theme) {
			fail(`theme ${name} not exported by themes.js`);
			continue;
		}
		const extra = Object.keys(theme).filter((k) => !['base', 'inherit', 'rules', 'colors'].includes(k));
		if (extra.length) fail(`theme ${name}: unexpected fields ${extra.join(', ')}`);
		out[name] = toJSONValue(
			{ base: theme.base, inherit: theme.inherit, rules: theme.rules, colors: theme.colors },
			`themes.${name}`,
		);
	}
	return out;
}

// ---------------------------------------------------------------------------
// 3. Official tokenization tests (source repo, tag v0.56.0)

async function exportFixtures(knownIds) {
	const dl = path.join(tmp(), 'src');
	fs.mkdirSync(dl);
	const tarball = path.join(dl, 'monaco-editor.tar.gz');
	const res = await fetch(TARBALL_URL);
	if (!res.ok) throw new Error(`download ${TARBALL_URL}: HTTP ${res.status}`);
	fs.writeFileSync(tarball, Buffer.from(await res.arrayBuffer()));
	const prefix = `monaco-editor-${MONACO_TAG.replace(/^v/, '')}`;
	execFileSync('tar', ['-xzf', tarball, '-C', dl, `${prefix}/src/languages/definitions`], { stdio: 'inherit' });
	const defs = path.join(dl, prefix, 'src', 'languages', 'definitions');

	const testFiles = [];
	for (const dir of fs.readdirSync(defs).sort()) {
		const abs = path.join(defs, dir);
		if (!fs.statSync(abs).isDirectory()) continue;
		for (const f of fs.readdirSync(abs).sort()) {
			if (f.endsWith('.test.ts')) testFiles.push(path.join(abs, f));
		}
	}

	const byLanguage = new Map();
	let duplicates = 0;
	for (const file of testFiles) {
		const rel = path.relative(defs, file);
		globalThis.__tangoTests = [];
		await bundleAndImport({
			entryPoints: [file],
			plugins: [
				virtualModulePlugin(
					'test-runner',
					/^\.\.\/test\/testRunner(\.ts)?$/,
					`export function testTokenization(language, tests) { globalThis.__tangoTests.push({ language, tests }); }`,
				),
				{
					// Test files must not depend on anything but the runner.
					name: 'deny-other-imports',
					setup(build) {
						build.onResolve({ filter: /.*/ }, (args) => {
							if (args.kind === 'entry-point') return undefined;
							return { errors: [{ text: `${rel}: unexpected import ${args.path}` }] };
						});
					},
				},
			],
		});
		const calls = globalThis.__tangoTests;
		if (calls.length === 0) fail(`${rel}: no testTokenization() call`);
		for (const { language, tests } of calls) {
			const langs = typeof language === 'string' ? [language] : language;
			for (const l of langs) if (!knownIds.has(l)) fail(`${rel}: unknown language id ${l}`);
			const main = langs[0];
			const cases = toJSONValue(tests, `${rel}(${main})`);
			for (const [i, c] of cases.entries()) {
				for (const [j, item] of c.entries()) {
					const keys = Object.keys(item).sort().join(',');
					if (keys !== 'line,tokens') fail(`${rel}(${main})[${i}][${j}]: unexpected keys ${keys}`);
				}
			}
			// Several files may target the same id (see EXPECTED.fixtures);
			// keep the first copy of byte-identical cases.
			if (!byLanguage.has(main)) byLanguage.set(main, { cases: [], seen: new Set() });
			const bucket = byLanguage.get(main);
			for (const c of cases) {
				const key = JSON.stringify(c);
				if (bucket.seen.has(key)) {
					duplicates++;
					continue;
				}
				bucket.seen.add(key);
				bucket.cases.push(c);
			}
		}
	}
	return {
		testFiles: testFiles.length,
		duplicates,
		byLanguage: new Map([...byLanguage].map(([id, b]) => [id, b.cases])),
	};
}

// ---------------------------------------------------------------------------

function writeJSON(file, value) {
	fs.writeFileSync(file, JSON.stringify(value, null, '\t') + '\n');
}

function resetDir(dir) {
	fs.mkdirSync(dir, { recursive: true });
	for (const f of fs.readdirSync(dir)) if (f.endsWith('.json')) fs.rmSync(path.join(dir, f));
}

async function main() {
	const pkg = JSON.parse(fs.readFileSync(path.join(monacoRoot, 'package.json'), 'utf8'));
	if (pkg.version !== MONACO_VERSION) {
		throw new Error(`monaco-editor ${pkg.version} installed, expected ${MONACO_VERSION} (run npm install)`);
	}

	const langs = await exportLanguages();
	const themes = await exportThemes();
	const fixtures = await exportFixtures(langs.ids);

	const counts = {
		grammarModules: langs.grammarModules,
		registrations: langs.files.length,
		themes: Object.keys(themes).length,
		testFiles: fixtures.testFiles,
		fixtures: fixtures.byLanguage.size,
	};
	for (const [k, want] of Object.entries(EXPECTED)) {
		if (counts[k] !== want) fail(`count ${k} = ${counts[k]}, expected ${want}`);
	}

	if (errors.length === 0) {
		resetDir(dataDir);
		resetDir(fixturesDir);
		for (const [name, value] of langs.files) writeJSON(path.join(dataDir, name), value);
		writeJSON(path.join(dataDir, 'themes.json'), themes);
		for (const [id, cases] of [...fixtures.byLanguage].sort()) writeJSON(path.join(fixturesDir, `${id}.json`), cases);
	}

	const caseCount = [...fixtures.byLanguage.values()].reduce((n, c) => n + c.length, 0);
	console.log(`monaco-editor ${MONACO_VERSION} / source tag ${MONACO_TAG}`);
	console.log(`grammar modules:        ${counts.grammarModules} (expected ${EXPECTED.grammarModules})`);
	console.log(`language registrations: ${counts.registrations} (expected ${EXPECTED.registrations}, incl. vue)`);
	console.log(`base themes:            ${counts.themes} (expected ${EXPECTED.themes})`);
	console.log(`test files:             ${counts.testFiles} (expected ${EXPECTED.testFiles})`);
	console.log(`tokenize fixtures:      ${counts.fixtures} (expected ${EXPECTED.fixtures}), ${caseCount} cases (${fixtures.duplicates} duplicate cases dropped)`);
	const untested = [...langs.ids].filter((id) => !fixtures.byLanguage.has(id)).sort();
	console.log(`languages without official tests: ${untested.join(', ') || '(none)'}`);

	if (errors.length) {
		console.error(`\n${errors.length} error(s); nothing written:`);
		for (const e of errors) console.error(`  ${e}`);
		process.exitCode = 1;
	}
}

try {
	await main();
} catch (err) {
	console.error(err);
	process.exitCode = 1;
} finally {
	if (tmpRoot) fs.rmSync(tmpRoot, { recursive: true, force: true });
}
