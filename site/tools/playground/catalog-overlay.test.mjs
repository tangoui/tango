import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { prepareGalleryOverlay } from '../playground-sources.mjs';

test('Go overlay reuses original gallery demos without the screenshot CLI or test files',()=>{
 const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../..');
 const dir=mkdtempSync(path.join(tmpdir(),'tango-gallery-overlay-'));
 try {
  const overlay=prepareGalleryOverlay(repo,dir);
  const { Replace }=JSON.parse(readFileSync(overlay,'utf8'));
  const sources=Object.values(Replace);
  assert.ok(sources.some(file=>file.endsWith('/page_general.go')));
  assert.ok(sources.some(file=>file.endsWith('/page_chart_line.go')));
  assert.ok(sources.some(file=>file.endsWith('/page_mobile_screens.go')));
  assert.ok(sources.every(file=>!file.endsWith('_test.go') && !file.endsWith('/main.go')));
  assert.ok(sources.length>80);
 } finally { rmSync(dir,{recursive:true,force:true}); }
});
