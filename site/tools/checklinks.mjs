// checklinks.mjs checks the relative links in the repository's Markdown:
// the documentation site (site/**/*.md, VitePress routes such as
// /guide/window, ./clipboard, /gallery/x.png) and the Markdown files at the
// repository root, in docs/ and in .github/. External links (http, mailto)
// and anchors are not checked.
//
//	node site/tools/checklinks.mjs      (npm run docs:links)
//
// Run it after npm run docs:gen so generated pages exist. Exits 1 on a dead
// link.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repo = path.resolve(site, "..");
const skipDirs = new Set(["node_modules", ".vitepress", ".git", "build", "dist", "plans"]);

function walk(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (skipDirs.has(e.name)) continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else if (e.name.endsWith(".md")) out.push(p);
  }
  return out;
}

const files = [
  ...walk(site).filter((f) => !f.includes(`${path.sep}tools${path.sep}`)),
  ...fs.readdirSync(repo).filter((f) => f.endsWith(".md")).map((f) => path.join(repo, f)),
  ...walk(path.join(repo, "docs")),
  ...walk(path.join(repo, ".github")),
];

// Links in Markdown: [text](target) and ![alt](target); <img src="...">.
const linkRe = /!?(?<!\\)\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)|<img[^>]+src="([^"]+)"/g;

function siteTarget(from, target) {
  // VitePress route: absolute (/guide/x) or relative (./x, ../x).
  const abs = target.startsWith("/") ? path.join(site, target) : path.resolve(path.dirname(from), target);
  const cands = [abs, abs + ".md", path.join(abs, "index.md")];
  if (target.startsWith("/")) cands.push(path.join(site, "public", target)); // public assets
  return cands.find((c) => fs.existsSync(c) && (fs.statSync(c).isFile()));
}

let dead = 0;
let checked = 0;
for (const file of files) {
  let text = fs.readFileSync(file, "utf8");
  // Skip fenced code blocks: links there are examples, not navigation.
  text = text.replace(/```[\s\S]*?```/g, "");
  const inSite = file.startsWith(site + path.sep);
  for (const m of text.matchAll(linkRe)) {
    let target = m[1] || m[2];
    // Links to this repository on GitHub must name a file that exists here.
    const gh = target.match(/^https:\/\/github\.com\/tangoui\/tango\/(?:blob|tree)\/main\/([^#?]+)/);
    if (gh) {
      checked++;
      if (!fs.existsSync(path.join(repo, decodeURI(gh[1])))) {
        dead++;
        console.log(`${path.relative(repo, file)}: dead repository link ${gh[1]}`);
      }
      continue;
    }
    if (/^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith("#")) continue;
    target = decodeURI(target.split("#")[0].split("?")[0]);
    if (!target) continue;
    checked++;
    const ok = inSite ? siteTarget(file, target) : fs.existsSync(path.resolve(path.dirname(file), target));
    if (!ok) {
      dead++;
      console.log(`${path.relative(repo, file)}: dead link ${target}`);
    }
  }
}
// VitePress includes (<!--@include: path-->).
for (const file of files.filter((f) => f.startsWith(site))) {
  for (const m of fs.readFileSync(file, "utf8").matchAll(/<!--@include:\s*([^\s]+?)\s*-->/g)) {
    checked++;
    if (!fs.existsSync(path.resolve(path.dirname(file), m[1]))) {
      dead++;
      console.log(`${path.relative(repo, file)}: missing include ${m[1]}`);
    }
  }
}
console.log(`checklinks: ${files.length} files, ${checked} relative links, ${dead} dead`);
process.exit(dead ? 1 : 0);
