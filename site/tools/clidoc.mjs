// clidoc.mjs captures the help text of every tan command into
// site/reference/_cli-flags.md, which the CLI reference pages include, so the
// flag lists never drift from cmd/tan.
//
//	node site/tools/clidoc.mjs      (part of npm run docs:gen)
//
// If cmd/tan does not build, the file says so instead of failing the docs
// build.
import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repo = path.resolve(site, "..");
const out = path.join(site, "reference", "_cli-flags.md");
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "tan-clidoc-"));
const bin = path.join(tmp, process.platform === "win32" ? "tan.exe" : "tan");

let md = "<!-- 由 site/tools/clidoc.mjs 生成（tan <command> -h），请勿手改 -->\n\n";
try {
  execFileSync("go", ["build", "-o", bin, "."], { cwd: path.join(repo, "cmd", "tan"), stdio: ["ignore", "ignore", "pipe"] });
  const help = (args) => {
    const r = spawnSync(bin, args, { encoding: "utf8", cwd: tmp });
    return ((r.stdout || "") + (r.stderr || "")).trim();
  };
  md += "```text\n$ tan help\n" + help(["help"]) + "\n```\n";
  const cmds = [...help(["help"]).matchAll(/^ {2}([a-z][a-z-]*) /gm)].map((m) => m[1]);
  for (const c of cmds) {
    if (c === "version") continue;
    const args = c === "generate" ? ["generate", "bindings", "-h"] : [c, "-h"];
    md += `\n### tan ${args.slice(0, -1).join(" ")}\n\n\`\`\`text\n${help(args)}\n\`\`\`\n`;
  }
  console.log(`clidoc: ${cmds.length} commands → ${path.relative(repo, out)}`);
} catch (e) {
  md += "::: warning\n构建 cmd/tan 失败，未能生成参数列表；请在本地运行 `tan <command> -h` 查看。\n:::\n";
  console.warn("clidoc: cmd/tan does not build; wrote a placeholder:", String(e.stderr || e.message).split("\n")[0]);
} finally {
  fs.rmSync(tmp, { recursive: true, force: true });
}
fs.writeFileSync(out, md);
