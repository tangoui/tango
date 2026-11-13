// Compile the real gallery source with the docs host; generated Go overlay
// paths are local build artifacts, not duplicated source files in the repo.
import { execFileSync } from 'node:child_process';
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';
import { prepareGalleryOverlay } from './playground-sources.mjs';
import { parseComponentTable, mapComponentCatalog } from './component-catalog.mjs';
import { generateComponentPages } from './generate-component-pages.mjs';

const site=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const repo=path.dirname(site);
const out=path.join(site,'public/playground');
const overlay=prepareGalleryOverlay(repo,path.join(site,'.playground'));
mkdirSync(out,{recursive:true});
const raw=execFileSync('go',['run',`-overlay=${overlay}`,'./site/tools/playground','-catalog'],{cwd:repo,encoding:'utf8',maxBuffer:8*1024*1024});
const catalog=mapComponentCatalog(parseComponentTable(readFileSync(path.join(repo,'ui/AUTHORING.md'),'utf8')),JSON.parse(raw));
mkdirSync(path.join(site,'.vitepress/generated'),{recursive:true});
for(const file of [path.join(out,'catalog.json'),path.join(site,'.vitepress/generated/components.json')]) writeFileSync(file,JSON.stringify(catalog,null,2)+'\n');
generateComponentPages(site,catalog);
execFileSync('go',['build',`-overlay=${overlay}`,'-trimpath','-ldflags=-s -w','-o',path.join(out,'playground.wasm'),'./site/tools/playground'],{cwd:repo,env:{...process.env,GOOS:'js',GOARCH:'wasm',CGO_ENABLED:'0'},stdio:'inherit'});
const packed=gzipSync(readFileSync(path.join(out,'playground.wasm')),{level:9});
writeFileSync(path.join(out,'playground.wasm.gz'),packed);
console.log(`playground compressed transfer: ${(packed.length/1024/1024).toFixed(1)} MiB`);
const goroot=execFileSync('go',['env','GOROOT'],{cwd:repo,encoding:'utf8'}).trim();
copyFileSync(path.join(goroot,'lib/wasm/wasm_exec.js'),path.join(out,'wasm_exec.js'));
writeFileSync(path.join(out,'EDITOR-NOTICES.txt'),['langs','monarch','theme'].map(name=>readFileSync(path.join(repo,'ui/editor',name,'NOTICE'),'utf8')).join('\n\n'));
console.log(`playground: ${catalog.components.length} component entries, ${catalog.demos.length} live scenes, ${catalog.groups.length} groups; complete mapping verified`);
