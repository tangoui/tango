// clidoc.mjs turns cmd/tan/testdata/cli.golden (the help text of tan and
// every command, pinned by cmd/tan's TestCLIGolden) into
// site/reference/_cli-flags.md, which the CLI reference pages include, so the
// flag lists never drift from cmd/tan.
//
//	node site/tools/clidoc.mjs      (part of npm run docs:gen)
//
// It also publishes cmd/tan/tan.schema.json (the JSON Schema of tan.json,
// pinned by TestConfigSchema) at /schema/tan.schema.json, the "$schema" URL
// tan init writes.
//
// After changing tan's flags, refresh the golden first:
//
//	cd cmd/tan && go test -run TestCLIGolden -update
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repo = path.resolve(site, "..");
const golden = path.join(repo, "cmd", "tan", "testdata", "cli.golden");
const out = path.join(site, "reference", "_cli-flags.md");

const text = fs.readFileSync(golden, "utf8").replace(/\r\n/g, "\n");
// Blocks start with "$ tan <args>" and run to the next one.
const blocks = text.split(/^(?=\$ tan )/m).filter((b) => b.trim());

let md = "<!-- 由 site/tools/clidoc.mjs 从 cmd/tan/testdata/cli.golden 生成，请勿手改 -->\n\n";
for (const [i, b] of blocks.entries()) {
  const nl = b.indexOf("\n");
  const cmd = b.slice(2, nl); // "tan build -h"
  const body = b.slice(nl + 1).trimEnd();
  if (i === 0) {
    md += "```text\n$ " + cmd + "\n" + body + "\n```\n";
  } else {
    md += `\n### ${cmd.replace(/ -h$/, "")}\n\n\`\`\`text\n${body}\n\`\`\`\n`;
  }
}
console.log(`clidoc: ${blocks.length - 1} commands → ${path.relative(repo, out)}`);
fs.writeFileSync(out, md);

const schemaDir = path.join(site, "public", "schema");
fs.mkdirSync(schemaDir, { recursive: true });
fs.copyFileSync(path.join(repo, "cmd", "tan", "tan.schema.json"), path.join(schemaDir, "tan.schema.json"));
