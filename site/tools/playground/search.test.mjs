import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { searchCatalog } from '../../.vitepress/theme/components/catalog-search.mjs';
const catalog=JSON.parse(readFileSync(new URL('../../public/playground/catalog.json',import.meta.url),'utf8'));

test('Chinese demo titles and qualified API names can find their component entries',()=>{
 assert.ok(searchCatalog(catalog,'按钮','全部','components').some(item=>item.id==='Button'));
 assert.ok(searchCatalog(catalog,'chart.Line','图表','components').some(item=>item.id==='chart.Line'));
 assert.ok(searchCatalog(catalog,'mobile','移动端','components').every(item=>item.group==='移动端'));
 assert.ok(searchCatalog(catalog,'代码编辑器','全部','components').some(item=>item.id==='editor.View'));
 assert.equal(searchCatalog(catalog,'not-a-widget-12345','全部','components').length,0);
});
