// The committed API report etc/runtime.api.md (API Extractor, see
// api-extractor.json) against the sources, plus proof that the strict
// check notices drift: each case builds an edited copy of src/ in a temp
// dir and runs API Extractor there in CI mode. Nothing in the repo changes.
//
// The report covers names and type signatures only. It cannot tell a
// behaviour change, and it does not describe the JSON exchanged with Go
// (see api/jsapi-methods.txt for the wire names).
import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { Extractor, ExtractorConfig } = require("@microsoft/api-extractor");
const root = fileURLToPath(new URL("..", import.meta.url));
const tsc = require.resolve("typescript/bin/tsc");

// extract copies the package into a temp dir, applies edit(dir), emits
// the declarations and runs API Extractor without --local (as api:check
// does). Returns {succeeded, apiReportChanged, report}.
function extract(edit = () => {}) {
  const dir = mkdtempSync(join(tmpdir(), "tango-api-report-"));
  try {
    for (const f of ["src", "etc", "tsconfig.json", "api-extractor.json", "package.json"]) {
      cpSync(join(root, f), join(dir, f), { recursive: true });
    }
    edit(dir);
    execFileSync(process.execPath, [tsc, "-p", dir, "--emitDeclarationOnly"], { stdio: "pipe" });
    const config = ExtractorConfig.loadFileAndPrepare(join(dir, "api-extractor.json"));
    const r = Extractor.invoke(config, {
      localBuild: false,
      messageCallback(m) {
        m.handled = true; // keep the test output quiet; the result says enough
      },
    });
    let report = "";
    try {
      report = readFileSync(join(dir, "dist/api-extractor/runtime.api.md"), "utf8");
    } catch {}
    return { succeeded: r.succeeded, apiReportChanged: r.apiReportChanged, report };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

function replaceIn(file, from, to) {
  const s = readFileSync(file, "utf8");
  assert.ok(s.includes(from), `${from} not in ${file}`);
  writeFileSync(file, s.replace(from, to));
}

test("etc/runtime.api.md is up to date (npm run api:update if the change is intended)", () => {
  const r = extract();
  assert.equal(r.apiReportChanged, false);
  assert.equal(r.succeeded, true);
});

test("the report lists namespace members with their signatures", () => {
  const md = readFileSync(join(root, "etc/runtime.api.md"), "utf8");
  // API Extractor renames a namespace that shares its name with a global of
  // the DOM lib (Window, Clipboard, ...) to Name_2 and exports it "as Name".
  assert.match(md, /declare namespace Window(?:_\d+)? \{\n\s+export \{[^}]*\bsetTitle,/);
  assert.match(md, /^function setTitle\(title: string\): Promise<void>;$/m);
  const index = readFileSync(join(root, "src/index.ts"), "utf8");
  for (const [, ns] of index.matchAll(/^export \* as (\w+) from/gm)) {
    assert.match(md, new RegExp(`^declare namespace ${ns}(?:_\\d+)? \\{`, "m"), `namespace ${ns} missing from the report`);
  }
});

test("drift: a namespace member signature change (Window.setTitle) fails the check", () => {
  const r = extract((d) =>
    replaceIn(join(d, "src/api/window.ts"), "export function setTitle(title: string)", "export function setTitle(title: string, animate?: boolean)"),
  );
  assert.equal(r.apiReportChanged, true);
  assert.equal(r.succeeded, false);
  assert.match(r.report, /function setTitle\(title: string, animate\?: boolean\): Promise<void>;/);
});

test("drift: removing a namespace member fails the check", () => {
  const r = extract((d) => replaceIn(join(d, "src/api/window.ts"), "export function center()", "function center()"));
  assert.equal(r.apiReportChanged, true);
  assert.equal(r.succeeded, false);
});

test("drift: a top-level export change fails the check", () => {
  const r = extract((d) => replaceIn(join(d, "src/index.ts"), "export function postRaw(", "export function postRawMessage("));
  assert.equal(r.apiReportChanged, true);
  assert.equal(r.succeeded, false);
});
