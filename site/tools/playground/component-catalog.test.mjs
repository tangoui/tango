import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseComponentTable, mapComponentCatalog } from '../component-catalog.mjs';

const authoring = readFileSync(new URL('../../../ui/AUTHORING.md',import.meta.url),'utf8');

test('full AUTHORING table includes qualified chart and mobile APIs without reading later tables',()=>{
 const rows=parseComponentTable(authoring);
 assert.ok(rows.length>=148);
 assert.ok(rows.some(row=>row.names.includes('chart.Liquid')));
 assert.ok(rows.some(row=>row.names.includes('editor.View')));
 assert.ok(rows.some(row=>row.names.includes('mobile.PullToRefresh')));
 assert.ok(rows.every(row=>!row.names.includes('StepBar')));
});
test('catalog maps aliases to actual registered demos and never silently falls back',()=>{
 const rows=[{group:'通用',names:['Text','Title'],reference:'Typography',source:'text.go'}];
 const pages=[{name:'Typography',title:'排版',group:'通用',source:'examples/ui-gallery/page_general.go',line:30},{name:'TypographyEditable',title:'可编辑排版',group:'通用',source:'examples/ui-gallery/page_data_misc2.go',line:15}];
 const catalog=mapComponentCatalog(rows,pages);
 assert.equal(catalog.components[0].demos[0],'Typography');
 assert.throws(()=>mapComponentCatalog([{...rows[0],names:['MissingWidget']}],pages),/MissingWidget/);
 assert.throws(()=>mapComponentCatalog(rows,[]),/Text/);
});
