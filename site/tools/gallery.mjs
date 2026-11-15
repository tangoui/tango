// gallery.mjs builds the component catalog page of the documentation site
// from the component table in ui/AUTHORING.md (section 14) and the
// screenshots that `go run ./examples/ui-gallery` writes to
// build/ui-gallery/components/.
//
//	node site/tools/gallery.mjs      (npm run docs:gallery)
//
// Screenshots are copied to site/public/gallery/ (ignored by git: they are
// large and can be regenerated). Without screenshots the page still lists
// every component and explains how to produce them.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseComponentTable } from "./component-catalog.mjs";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repo = path.resolve(site, "..");
const srcDir = path.join(repo, "build", "ui-gallery");
const shotsDir = path.join(srcDir, "components");
const outImg = path.join(site, "public", "gallery");
const outPage = path.join(site, "components", "screenshots.md");

// ---- Parse the table ---------------------------------------------------------
const authoring = fs.readFileSync(path.join(repo, "ui", "AUTHORING.md"), "utf8");
const rows = parseComponentTable(authoring).map(row => ({...row, ref:row.reference, file:row.source}));
const catalogFile=path.join(site,"public/playground/catalog.json");
const liveCatalog=fs.existsSync(catalogFile) ? JSON.parse(fs.readFileSync(catalogFile,"utf8")) : {components:[]};

// ---- Find screenshots ---------------------------------------------------------
const shots = fs.existsSync(shotsDir) ? fs.readdirSync(shotsDir).filter((f) => f.endsWith(".png")) : [];
const byName = new Map(shots.map((f) => [f.slice(0, -4), f]));
// Gallery page names that differ from the component's public name.
const aliases = { GridRow: "Grid", Col: "Grid", Header: "Layout", Sider: "Layout", Content: "Layout", Footer: "Layout" };

function shotsFor(row) {
  for (const n of [...row.names, ...(liveCatalog.components.find(item=>item.id===row.names[0])?.demos ?? [])]) {
    const base = byName.has(n) ? n : aliases[n] && byName.has(aliases[n]) ? aliases[n] : null;
    if (!base) continue;
    const variants = shots
      .map((f) => f.slice(0, -4))
      .filter((s) => s.toLowerCase().startsWith(base.toLowerCase() + "-"))
      .sort();
    return { base, variants };
  }
  return null;
}

fs.rmSync(outImg, { recursive: true, force: true });
const copied = new Set();
function copy(name) {
  if (copied.has(name)) return;
  fs.mkdirSync(outImg, { recursive: true });
  fs.copyFileSync(path.join(shotsDir, byName.get(name)), path.join(outImg, name + ".png"));
  copied.add(name);
}
for (const extra of ["overview.png", "index.png", "dashboard-antd.png", "dashboard-dark.png"]) {
  if (fs.existsSync(path.join(srcDir, extra))) {
    fs.mkdirSync(outImg, { recursive: true });
    fs.copyFileSync(path.join(srcDir, extra), path.join(outImg, extra));
  }
}

// ---- Write the page ------------------------------------------------------------
const label = (v) => {
  const s = v.split("-").slice(1).join("-").toLowerCase();
  return { dark: "深色", element: "Element 主题", "element-dark": "Element 深色" }[s] || s;
};
const esc = (s) => s.replace(/\{\{/g, "{ {");
let md = `---
title: 组件截图目录
outline: [2, 2]
editLink: false
---

# 组件截图目录

每个组件在默认主题下的静态截图，另有深色与 Element 风格变体。想直接点击、输入、拖动组件，从左侧选择组件，或打开 [在线演练场](/components/playground)。

<!-- 本页由 site/tools/gallery.mjs 生成，请勿手改；改 ui/AUTHORING.md 第 14 节的组件总表。 -->

自绘 UI（\`github.com/tangoui/tango/ui\`）的组件按 [ui/AUTHORING.md](https://github.com/tangoui/tango/blob/main/ui/AUTHORING.md) 第 14 节的总表列出，共 ${rows.length} 行。截图由 \`go run ./examples/ui-gallery\` 在本机渲染（无头、确定性时间），每个组件默认主题一张，另有深色与 Element 风格变体。

::: warning 实验性
自绘 UI 目前只在 macOS 上实机验证；Windows、Linux 的 canvas 上屏和移动端还在推进中，见 [自绘 UI](/guide/ui)。
:::
`;
if (!shots.length) {
  md += `
::: info 没有截图
构建本页时没找到 \`build/ui-gallery/components/*.png\`。在仓库根目录运行 \`go run ./examples/ui-gallery\` 生成截图，再执行 \`npm run docs:gallery\`。
:::
`;
} else if (fs.existsSync(path.join(outImg, "overview.png"))) {
  md += `\n![组件总览](/gallery/overview.png)\n`;
}

let group = "";
for (const row of rows) {
  if (row.group !== group) {
    group = row.group;
    md += `\n## ${group}\n`;
  }
  const title = row.names.slice(0, 3).join(" · ");
  md += `\n### ${title}\n\n`;
  const live = liveCatalog.components.find((entry) => entry.id === row.names[0]);
  md += `- [组件文档与在线演示](${live?.doc ?? "/components/catalog/" + row.names[0].replaceAll(".","-").toLowerCase()})\n- 公开入口：${esc(row.comps)}\n- 参考设计：${esc(row.ref)}\n- 源文件：\`ui/\` 下 ${esc(row.file)}\n`;
  const s = shotsFor(row);
  if (s) {
    copy(s.base);
    md += `\n![${s.base}](/gallery/${s.base}.png)\n`;
    const vs = s.variants.slice(0, 3);
    if (vs.length) {
      md += `\n<details><summary>主题变体（${vs.map(label).join("、")}）</summary>\n\n`;
      for (const v of vs) {
        copy(v);
        md += `![${v}](/gallery/${v}.png)\n\n`;
      }
      md += `</details>\n`;
    }
  } else if (shots.length) {
    md += `\n*暂无截图。*\n`;
  }
}
fs.writeFileSync(outPage, md);
console.log(`gallery: ${rows.length} rows, ${copied.size} screenshots copied to ${path.relative(repo, outImg)}, page ${path.relative(repo, outPage)}`);
