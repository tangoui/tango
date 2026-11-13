// Every entry is checked against the module actually served by the docs site.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import '../../public/playground/wasm_exec.js';

const catalog=JSON.parse(await readFile(new URL('../../public/playground/catalog.json',import.meta.url),'utf8'));
const bus=new EventTarget();
globalThis.dispatchEvent=event=>bus.dispatchEvent(event);
const ready=new Promise(resolve=>bus.addEventListener('tango-preview-ready',resolve,{once:true}));
const go=new globalThis.Go();
const {instance}=await WebAssembly.instantiate(await readFile(new URL('../../public/playground/playground.wasm',import.meta.url)),go.importObject);
go.run(instance).catch(error=>{throw error;});
await ready;
const api=globalThis.tangoPreview;
let frame;
const errors=[];
const host={requestFrame(){},wake(){},cursor(){},clipboard(){},textInput(){},state(){},present(data,width,height){frame={length:data.length,width,height};},error(message){errors.push(message);}};

test('full component table and advanced scenes are present in the real WASM module',()=>{
 const pages=JSON.parse(api.pages());
 const known=new Set(pages.map(page=>page.name));
 assert.ok(catalog.components.length>=148);
 assert.ok(catalog.demos.length>=206);
 for(const entry of catalog.components){
  assert.ok(entry.demos.length,entry.id);
  for(const name of entry.demos)assert.ok(known.has(name),`${entry.id} -> ${name}`);
 }
 assert.ok(catalog.components.some(entry=>entry.id==='chart.Liquid'));
 assert.ok(catalog.components.some(entry=>entry.id==='mobile.ScrollView'));
});
test('every live gallery scene renders and takes scrolling without a native host',{timeout:180000},()=>{
 for(const page of catalog.demos){
  const name=`gallery:${page.name}`;
  assert.equal(api.start(host,name,960,1,'antd',false),true,name);
  assert.equal(api.frame(),true,name);
  assert.equal(frame.height,560,name);
  assert.equal(frame.length,frame.width*frame.height*4,name);
  api.event({kind:'scroll',x:400,y:350,dx:0,dy:240,mods:0});
  assert.equal(api.frame(),true,`${name} scroll`);
 }
 assert.deepEqual(errors,[]);
});
test('gallery themes, resize and unknown component behavior are explicit',()=>{
 for(const theme of ['antd','element','shadcn']){
  for(const dark of [false,true]){
   assert.equal(api.start(host,'gallery:Form',720,1,theme,dark),true);
   assert.equal(api.frame(),true);
  }
 }
 api.resize(390,1);api.frame();assert.equal(frame.width,390);
 assert.equal(api.start(host,'gallery:DoesNotExist',720,1,'antd',false),false);
 assert.ok(errors.at(-1).includes('Unknown component'));
 api.close();
});
