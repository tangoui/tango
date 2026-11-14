// The wire contract api/jsapi-methods.txt against src/api/*.ts (see
// tools/contract.mjs). The Go side of the same file is checked by
// jsapi/contract_test.go.
import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { check, contractPath, docsPath, extract, readContract, root } from "../tools/contract.mjs";

test("api/jsapi-methods.txt matches src/api (run npm run contract:update if intended)", () => {
  const { problems } = check();
  assert.deepEqual(problems, []);
});

test("every TS wrapper name is a Go method or event, every namespace module is covered", () => {
  const { uses, problems } = extract();
  assert.deepEqual(problems, []);
  const entries = readContract().filter((e) => e.kind === "method" || e.kind === "event");
  const known = new Set(entries.map((e) => `${e.kind} ${e.name}`));
  for (const key of uses.keys()) assert.ok(known.has(key), key);
  // Spot checks that the helper evaluation works.
  assert.deepEqual([...uses.get("method tango:window.setTitle")], ["Window.setTitle"]);
  assert.deepEqual([...uses.get("event tango:app.openURL")], ["App.onOpenURL"]);
  assert.ok(uses.get("method tango:app.takePending").has("App.onReopen"));
});

// fixture copies src, the contract and the docs into a temp dir, applies
// edit(dir) and returns check()'s problems there. Nothing in the repo
// changes.
function fixture(edit) {
  const dir = mkdtempSync(join(tmpdir(), "tango-contract-"));
  try {
    cpSync(join(root, "src"), join(dir, "src"), { recursive: true });
    cpSync(contractPath, join(dir, "jsapi-methods.txt"));
    cpSync(docsPath, join(dir, "runtime-api.md"));
    edit(dir);
    return check({ srcDir: join(dir, "src"), path: join(dir, "jsapi-methods.txt"), docs: join(dir, "runtime-api.md") }).problems;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

function replaceIn(file, from, to) {
  const s = readFileSync(file, "utf8");
  assert.ok(s.includes(from), `${from} not in ${file}`);
  writeFileSync(file, s.replace(from, to));
}

test("drift: a TS wrapper calling a method Go does not register fails", () => {
  const p = fixture((d) => replaceIn(join(d, "src/api/window.ts"), '"window.setTitle"', '"window.setCaption"'));
  assert.ok(p.some((x) => x.includes("Window.setTitle uses method tango:window.setCaption")), p.join("\n"));
});

test("drift: a method removed on the Go side fails", () => {
  const p = fixture((d) => replaceIn(join(d, "jsapi-methods.txt"), "method tango:window.setTitle cap=Window js=Window.setTitle\n", ""));
  assert.ok(p.some((x) => x.includes("tango:window.setTitle, which jsapi does not register")), p.join("\n"));
});

test("drift: an event name built by a helper is checked too", () => {
  const p = fixture((d) => replaceIn(join(d, "src/api/app.ts"), 'launch<{ url: string }>("openURL"', 'launch<{ url: string }>("openUrl"'));
  assert.ok(p.some((x) => x.includes("event tango:app.openUrl")), p.join("\n"));
});

test("drift: a non-constant name or a bypass of invoke() fails", () => {
  const p = fixture((d) => {
    replaceIn(join(d, "src/api/shell.ts"), 'invoke("shell.openURL", url)', 'invoke("shell." + url, url)');
    replaceIn(join(d, "src/api/clipboard.ts"), 'invoke("clipboard.readText")', 'getClientCall("tango:clipboard.readText")');
  });
  assert.ok(p.some((x) => x.includes("cannot resolve the method name")), p.join("\n"));
  assert.ok(p.some((x) => x.includes("bypasses invoke/listen")), p.join("\n"));
});

test("drift: a method used as an event is reported", () => {
  const p = fixture((d) => replaceIn(join(d, "src/api/theme.ts"), 'listen("theme.change"', 'listen("theme.get"'));
  assert.ok(p.some((x) => x.includes("(it is a method there)")), p.join("\n"));
});

test("drift: ErrorCode and the docs error table follow the contract", () => {
  const p = fixture((d) => {
    replaceIn(join(d, "src/client.ts"), '| "not_running"', '| "not_running"\n  | "timeout"');
    replaceIn(join(d, "runtime-api.md"), "  | `busy` |", "  | `bussy` |");
  });
  assert.ok(p.some((x) => x.includes('names "timeout"')), p.join("\n"));
  assert.ok(p.some((x) => x.includes('"busy" is missing from the error table')), p.join("\n"));
  assert.ok(p.some((x) => x.includes('documents error code "bussy"')), p.join("\n"));
});
