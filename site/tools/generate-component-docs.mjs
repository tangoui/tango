// Usage documentation of the component pages: site/tools/compdoc extracts,
// from the Go source, the API (entry points, builder methods, option
// structs) and the Go code of every gallery demo of each catalog entry;
// this module turns it into two Markdown partials per component,
//
//	components/_gen/<slug>-examples.md   示例代码 (one collapsible block per demo)
//	components/_gen/<slug>-api.md        API (entry points and method tables)
//
// which the generated pages (components/catalog/<slug>.md) and the
// hand-written ones (components/<slug>.md) pull in with
// <!--@include: ...-->. The partials are not pages: srcExclude skips
// components/_gen/**, and the directory is gitignored.
//
// The demo source itself is not Markdown: rendered in place, its ~13 000
// highlighted lines cost the VitePress build 1.5 GB of heap and 50 s.
// Each demo is highlighted once here (same Shiki themes and markup as
// VitePress code blocks) into .vitepress/generated/demo-src/<demo>.json,
// and the <DemoSource> blocks of the partials load it when opened.
import { execFileSync } from 'node:child_process';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { createHighlighter } from 'shiki';

const blob = 'https://github.com/tangoui/tango/blob/main/';

// extractComponentDocs runs the Go extractor on the catalog.
export function extractComponentDocs(repo, catalog) {
  const input = JSON.stringify({
    components: catalog.components.map(({ id, names, demos }) => ({ id, names, demos })),
    demos: catalog.demos.map(({ name, title, source, line }) => ({ name, title, source, line })),
  });
  const raw = execFileSync('go', ['run', './site/tools/compdoc', '-root', '.'], { cwd: repo, input, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, stdio: ['pipe', 'pipe', 'inherit'] });
  return JSON.parse(raw);
}

export const partialDir = 'components/_gen';
export const demoSourceDir = '.vitepress/generated/demo-src';

export async function generateComponentDocs(site, catalog, docs) {
  const directory = path.join(site, partialDir);
  rmSync(directory, { recursive: true, force: true });
  mkdirSync(directory, { recursive: true });
  const byID = new Map(docs.components.map(c => [c.id, c]));
  for (const entry of catalog.components) {
    const doc = byID.get(entry.id);
    if (!doc) throw new Error(`compdoc: no documentation for ${entry.id}`);
    writeFileSync(path.join(directory, `${entry.slug}-examples.md`), examplesMarkdown(doc));
    writeFileSync(path.join(directory, `${entry.slug}-api.md`), apiMarkdown(doc));
  }
  return writeDemoSources(site, docs);
}

// ---- 示例代码 ----

function fence(code, lang = 'go') {
  const longest = Math.max(2, ...[...code.matchAll(/`+/g)].map(m => m[0].length));
  const ticks = '`'.repeat(longest + 1);
  return `${ticks}${lang}\n${code.replace(/\s+$/, '')}\n${ticks}`;
}

const attr = s => String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
const html = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

export function examplesMarkdown(doc) {
  const out = [];
  out.push('以下是上方在线演示运行的 Go 源码（来自官方 gallery），点击标题展开。函数接收的 `c *ui.Context` 由窗口的 UI 循环每帧传入，接入方式见 [Go GUI 入门](/guide/ui)。', '');
  for (const ex of doc.examples) {
    out.push(`<DemoSource demo="${attr(ex.demo)}" title="${attr(ex.title.replace(/\s+/g, ' '))}" kind="${ex.kind}" source="${attr(ex.source)}" :line="${ex.line}" />`, '');
  }
  return out.join('\n');
}

// writeDemoSources highlights every demo once (a demo can be shown by
// several components) and returns the number of files written.
async function writeDemoSources(site, docs) {
  const directory = path.join(site, demoSourceDir);
  rmSync(directory, { recursive: true, force: true });
  mkdirSync(directory, { recursive: true });
  const themes = { light: 'github-light', dark: 'github-dark' };
  const highlighter = await createHighlighter({ themes: Object.values(themes), langs: ['go'] });
  // VitePress code block markup, so its styles and copy button apply.
  const block = code => '<div class="language-go vp-adaptive-theme"><button title="Copy Code" class="copy"></button><span class="lang">go</span>' +
    highlighter.codeToHtml(code.replace(/\s+$/, ''), { lang: 'go', themes, defaultColor: false })
      .replace(/^<pre class="shiki shiki-themes github-light github-dark"[^>]*>/, '<pre class="shiki shiki-themes github-light github-dark vp-code" tabindex="0">') +
    '</div>';
  const link = (file, line) => `<a href="${attr(`${blob}${file}#L${line}`)}" target="_blank" rel="noreferrer">${html(path.posix.basename(file))}#L${line}</a>`;
  const done = new Set();
  for (const component of docs.components) {
    for (const ex of component.examples) {
      if (done.has(ex.demo)) continue;
      done.add(ex.demo);
      let out = block(ex.code);
      if (ex.helpers?.length) {
        out += `<p>示例里用到的 gallery 辅助声明：${ex.helpers.map(h => `<code>${html(h.name)}</code>（${link(h.source, h.line)}）`).join('、')}。</p>`;
        out += block(ex.helpers.map(h => h.code).join('\n\n'));
      }
      writeFileSync(path.join(directory, `${ex.demo}.json`), JSON.stringify({ html: out }) + '\n');
    }
  }
  highlighter.dispose();
  return done.size;
}

// ---- API ----

// cell escapes text for a table cell.
function cell(text) {
  return (text || '').replace(/\s+/g, ' ').trim()
    .replace(/[\\`*_[\]!|]/g, ch => '\\' + ch).replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// code renders a signature as a code span inside a table cell.
function code(text) {
  const s = text.replace(/\s+/g, ' ').trim().replace(/\|/g, '\\|');
  return s.includes('`') ? '`` ' + s + ' ``' : '`' + s + '`';
}

const pkgName = importPath => importPath.split('/').pop();

function typeAnchor(t) {
  return `api-${pkgName(t.importPath)}-${t.name}`;
}

function entryTitle(e) {
  return e.name.includes('.') ? e.name : `ui.${e.name}`;
}

// General-purpose types (compdoc's generic set) get a link, not a table.
const genericWhat = g => g.name === 'ui.Box' ? '通用容器' : '通用类型';
function genericMethods(g) {
  const short = g.name.split('.').pop();
  return g.name === 'ui.Box' ? `可用的布局与样式方法见 [${short} 的 API](${g.ref})` : `其方法见 [${short} 的 API](${g.ref})`;
}

export function apiMarkdown(doc) {
  const out = ['<div v-pre>', ''];
  const types = new Map(doc.types.map(t => [t.name, t]));
  const funcs = doc.entries.filter(e => e.kind === 'func');
  const others = doc.entries.filter(e => e.kind !== 'func');
  out.push('签名与说明由 `site/tools/compdoc` 从 Go 源码和文档注释提取；点击名称跳到 [Go API 参考](/reference/api/) 的对应条目。', '');
  if (funcs.length) {
    out.push('### 入口函数', '');
    for (const e of funcs) {
      out.push(`#### [\`${entryTitle(e)}\`](${e.ref})`, '');
      out.push(fence(e.signature), '');
      if (e.docMarkdown) out.push(e.docMarkdown, '');
      const t = e.returns && types.get(e.returns);
      if (t) out.push(`返回 \`*${t.name}\`，可链式调用的方法见 [${t.name} 方法](#${typeAnchor(t)})。`, '');
      if (e.generic) out.push(`返回${genericWhat(e.generic)} \`*${e.generic.name}\`，${genericMethods(e.generic)}。`, '');
    }
  }
  if (others.length) {
    out.push('### 相关类型与成员', '');
    for (const e of others) {
      out.push(`#### [\`${entryTitle(e)}\`](${e.ref})`, '');
      if (e.signature) out.push(fence((e.kind === 'field' ? '// 字段\n' : '') + e.signature), '');
      if (e.docMarkdown) out.push(e.docMarkdown, '');
    }
  }
  const order = { builder: 0, type: 1, param: 2 };
  for (const t of [...doc.types].sort((a, b) => order[a.role] - order[b.role])) {
    const kind = t.role === 'param' ? '参数类型' : t.methods.length ? '方法' : '类型';
    out.push(`### ${t.name} ${kind} {#${typeAnchor(t)}}`, '');
    const pkg = pkgName(t.importPath);
    const ref = t.ref ? `[\`${pkg}.${t.name}\`](${t.ref})` : `\`${pkg}.${t.name}\``;
    if (t.docMarkdown) out.push(`${ref}：${t.docMarkdown}`, '');
    else out.push(ref, '');
    if (t.decl) out.push(fence(t.decl), '');
    if (t.fields?.length) {
      out.push('| 字段 | 说明 | 类型 |', '| --- | --- | --- |');
      for (const f of t.fields) out.push(`| \`${f.name}\` | ${cell(f.synopsis)} | ${code(f.type)} |`);
      out.push('');
    }
    for (const g of t.embeds ?? []) out.push(`${t.name} 嵌入${genericWhat(g)} \`*${g.name}\`，${genericMethods(g)}；下表只列 ${t.name} 自己的方法。`, '');
    if (t.methods.length) {
      if (t.fields?.length) out.push(`${t.name} 的方法：`, '');
      out.push('| 方法 | 说明 | 签名 |', '| --- | --- | --- |');
      for (const m of t.methods) {
        const name = m.ref ? `[\`${m.name}\`](${m.ref})` : `\`${m.name}\``;
        const note = m.promoted ? `${m.synopsis ? ' ' : ''}（来自嵌入的 \`${m.promoted}\`）` : '';
        out.push(`| ${name} | ${cell(m.synopsis)}${note} | ${code(m.signature)} |`);
      }
      out.push('');
    }
  }
  out.push('</div>', '');
  return out.join('\n');
}
