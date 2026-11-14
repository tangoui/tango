// The runtime-js side of the wire contract in api/jsapi-methods.txt (see
// the header of that file and jsapi/contract_test.go, which owns every
// column except js=).
//
// extract() reads src/api/*.ts with the TypeScript parser and finds every
// runtime API name the namespace functions use: invoke("<name>") is a
// method, listen("<name>") an event (both add the "tango:" prefix, see
// src/api/internal.ts). Names built by local helpers, such as
//   function launch(kind) { listen("app." + kind, ...) }   onOpenURL → launch("openURL")
// are resolved by evaluating the helper with the literal arguments of each
// call site. A name that is not a constant string, or a call that no
// exported function reaches, is reported as a problem rather than skipped.
//
//   node tools/contract.mjs          check (exit 1 on drift)
//   node tools/contract.mjs --update rewrite the js= column
//
// Structural only: it says which names are used, not that arguments and
// results have the shape the Go side expects.
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const ts = createRequire(import.meta.url)("typescript");

export const root = join(dirname(fileURLToPath(import.meta.url)), "..");
export const contractPath = join(root, "..", "api", "jsapi-methods.txt");
export const docsPath = join(root, "..", "docs", "runtime-api.md");
const PREFIX = "tango:";

function parse(file) {
  return ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.ES2020, true);
}

function where(sf, node) {
  const { line } = sf.getLineAndCharacterOfPosition(node.getStart(sf));
  return `src/${sf.fileName.split("/src/")[1]}:${line + 1}`;
}

function isExported(node) {
  return (ts.getCombinedModifierFlags(node) & ts.ModifierFlags.Export) !== 0;
}

/** Namespace name → module file, from `export * as X from "./api/x.js"`. */
export function namespaces(srcDir = join(root, "src")) {
  const sf = parse(join(srcDir, "index.ts"));
  const out = new Map();
  for (const st of sf.statements) {
    if (
      ts.isExportDeclaration(st) &&
      st.exportClause &&
      ts.isNamespaceExport(st.exportClause) &&
      st.moduleSpecifier &&
      ts.isStringLiteral(st.moduleSpecifier)
    ) {
      const file = st.moduleSpecifier.text.replace(/^\.\//, "").replace(/\.js$/, ".ts");
      out.set(file, st.exportClause.name.text);
    }
  }
  return out;
}

// Constant string value of e under env (param name → string), or undefined.
function evaluate(e, env) {
  if (ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e)) return e.text;
  if (ts.isParenthesizedExpression(e)) return evaluate(e.expression, env);
  if (ts.isIdentifier(e)) return env.get(e.text);
  if (ts.isBinaryExpression(e) && e.operatorToken.kind === ts.SyntaxKind.PlusToken) {
    const a = evaluate(e.left, env);
    const b = evaluate(e.right, env);
    return a === undefined || b === undefined ? undefined : a + b;
  }
  if (ts.isTemplateExpression(e)) {
    let s = e.head.text;
    for (const span of e.templateSpans) {
      const v = evaluate(span.expression, env);
      if (v === undefined) return undefined;
      s += v + span.literal.text;
    }
    return s;
  }
  return undefined;
}

const SINKS = { invoke: "method", listen: "event" };

/**
 * Extracts {uses: Map<"kind name", Set<"Ns.fn">>, problems: string[]}.
 */
export function extract(srcDir = join(root, "src")) {
  const uses = new Map();
  const problems = [];
  const ns = namespaces(srcDir);
  const apiDir = join(srcDir, "api");
  const files = readdirSync(apiDir).filter((f) => f.endsWith(".ts") && f !== "internal.ts");

  for (const f of files) {
    const rel = "api/" + f;
    const sf = parse(join(apiDir, f));
    const nsName = ns.get(rel);
    if (!nsName) problems.push(`src/${rel}: not re-exported as a namespace from src/index.ts`);

    // Top-level function declarations of the module, by name.
    const funcs = new Map();
    for (const st of sf.statements) {
      if (ts.isFunctionDeclaration(st) && st.name && st.body) funcs.set(st.name.text, st);
    }
    const allSinks = new Set();
    const seen = new Set();
    const visit = (n) => {
      if (ts.isCallExpression(n) && ts.isIdentifier(n.expression) && SINKS[n.expression.text]) allSinks.add(n);
      if ((ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n)) && n.text.startsWith(PREFIX)) {
        problems.push(`${where(sf, n)}: literal "${n.text}" bypasses invoke/listen; use the helpers of api/internal.ts`);
      }
      ts.forEachChild(n, visit);
    };
    visit(sf);

    // walk evaluates the sinks in body with env, attributing them to owner.
    const walk = (body, env, owner, stack) => {
      const go = (n) => {
        if (ts.isCallExpression(n) && ts.isIdentifier(n.expression)) {
          const callee = n.expression.text;
          if (SINKS[callee]) {
            seen.add(n);
            const arg = n.arguments[0];
            const name = arg && evaluate(arg, env);
            if (name === undefined) {
              problems.push(`${where(sf, n)}: cannot resolve the ${SINKS[callee]} name ${arg ? arg.getText(sf) : "(none)"}`);
            } else {
              const key = `${SINKS[callee]} ${PREFIX}${name}`;
              if (!uses.has(key)) uses.set(key, new Set());
              uses.get(key).add(`${nsName ?? rel}.${owner}`);
            }
          } else if (funcs.has(callee) && !stack.includes(callee)) {
            const fd = funcs.get(callee);
            const next = new Map();
            fd.parameters.forEach((p, i) => {
              if (!ts.isIdentifier(p.name)) return;
              const a = n.arguments[i];
              const v = a ? evaluate(a, env) : undefined;
              if (v !== undefined) next.set(p.name.text, v);
            });
            walk(fd.body, next, owner, [...stack, callee]);
          }
        }
        ts.forEachChild(n, go);
      };
      go(body);
    };
    for (const [name, fd] of funcs) {
      if (isExported(fd)) walk(fd.body, new Map(), name, [name]);
    }
    for (const c of allSinks) {
      if (!seen.has(c)) problems.push(`${where(sf, c)}: ${c.expression.text}() is not reached from any exported function`);
    }
  }
  return { uses, problems };
}

/** Members of the ErrorCode union in src/client.ts (string literals only). */
export function errorCodes(srcDir = join(root, "src")) {
  const sf = parse(join(srcDir, "client.ts"));
  for (const st of sf.statements) {
    if (ts.isTypeAliasDeclaration(st) && st.name.text === "ErrorCode" && ts.isUnionTypeNode(st.type)) {
      return st.type.types
        .filter((t) => ts.isLiteralTypeNode(t) && ts.isStringLiteral(t.literal))
        .map((t) => t.literal.text);
    }
  }
  throw new Error("type ErrorCode not found in src/client.ts");
}

/** Codes in the error table of docs/runtime-api.md (section 约定). */
export function documentedCodes(path = docsPath) {
  const md = readFileSync(path, "utf8");
  const start = md.indexOf("## 约定");
  const end = md.indexOf("\n## ", start + 1);
  const out = [];
  for (const m of md.slice(start, end).matchAll(/^\s*\| `([a-z_]+)` \|/gm)) out.push(m[1]);
  return out;
}

/** Parses api/jsapi-methods.txt into [{kind, name, cap?, js, raw}] (comments kept as {raw}). */
export function readContract(path = contractPath) {
  return readFileSync(path, "utf8")
    .split("\n")
    .map((raw) => {
      if (raw === "" || raw.startsWith("#")) return { raw };
      const [kind, name, ...kv] = raw.trim().split(/\s+/);
      const e = { kind, name, raw };
      for (const f of kv) {
        const i = f.indexOf("=");
        e[f.slice(0, i)] = f.slice(i + 1);
      }
      return e;
    });
}

function jsValue(entry, uses, codes) {
  if (entry.kind === "error") return codes.includes(entry.name) ? "ErrorCode" : "-";
  const s = uses.get(`${entry.kind} ${entry.name}`);
  return s ? [...s].sort().join(",") : "-";
}

function format(e, js) {
  return `${e.kind} ${e.name}${e.kind === "method" ? ` cap=${e.cap}` : ""} js=${js}`;
}

/**
 * Compares the sources with the contract. Returns {problems, lines} where
 * lines is the contract with the js= column recomputed.
 */
export function check({ srcDir = join(root, "src"), path = contractPath, docs = docsPath } = {}) {
  const { uses, problems } = extract(srcDir);
  const entries = readContract(path);
  const codes = errorCodes(srcDir);
  const known = new Map(entries.filter((e) => e.kind).map((e) => [`${e.kind} ${e.name}`, e]));

  for (const [key, owners] of uses) {
    if (known.has(key)) continue;
    const [kind, name] = key.split(" ");
    const other = known.has(`${kind === "method" ? "event" : "method"} ${name}`);
    problems.push(
      `${[...owners].join(", ")} uses ${kind} ${name}, which jsapi does not ${kind === "method" ? "register" : "emit"}` +
        (other ? ` (it is a${kind === "method" ? "n event" : " method"} there)` : ""),
    );
  }
  const errors = entries.filter((e) => e.kind === "error").map((e) => e.name);
  for (const c of codes) {
    if (!errors.includes(c)) problems.push(`ErrorCode in src/client.ts names "${c}", which jsapi/ipc never produce`);
  }
  const documented = documentedCodes(docs);
  for (const c of errors) {
    if (!documented.includes(c)) problems.push(`error code "${c}" is missing from the error table of docs/runtime-api.md`);
  }
  for (const c of documented) {
    if (!errors.includes(c)) problems.push(`docs/runtime-api.md documents error code "${c}", which jsapi/ipc never produce`);
  }

  const lines = entries.map((e) => (e.kind ? format(e, jsValue(e, uses, codes)) : e.raw));
  entries.forEach((e, i) => {
    if (e.kind && e.raw !== lines[i]) problems.push(`stale js= column: "${e.raw}" should be "${lines[i]}"`);
  });
  return { problems, lines };
}

if (process.argv[1] && basename(process.argv[1]) === "contract.mjs") {
  const update = process.argv.includes("--update");
  const { problems, lines } = check();
  const hard = problems.filter((p) => !p.startsWith("stale js="));
  if (update) {
    writeFileSync(contractPath, lines.join("\n"));
    console.log(`updated js= column of ${contractPath}`);
  }
  const report = update ? hard : problems;
  if (report.length) {
    console.error(`runtime API wire contract (api/jsapi-methods.txt):\n  ${report.join("\n  ")}`);
    if (!update) console.error("If intended: npm run contract:update (and go test ./jsapi -run TestWireContract -update for Go changes)");
    process.exit(1);
  }
  if (!update) console.log("wire contract OK");
}
