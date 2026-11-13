import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import '../../public/playground/wasm_exec.js';
import { keyboardModifiers, pasteInput, pointerInput } from '../../public/playground/input.mjs';

const bus=new EventTarget();
globalThis.dispatchEvent=event=>bus.dispatchEvent(event);
globalThis.tangoUseMonoFont=true;
const ready=new Promise(resolve=>bus.addEventListener('tango-preview-ready',resolve,{once:true}));
const go=new globalThis.Go();
const {instance}=await WebAssembly.instantiate(await readFile(new URL('../../public/playground/playground.wasm',import.meta.url)),go.importObject);
go.run(instance).catch(error=>{throw error});
await ready;
const api=globalThis.tangoPreview;
let summary='';let width=0;let colored=false;let clipboard='';
const errors=[];
const host={requestFrame(){},wake(){},cursor(){},clipboard(value){clipboard=value;},textInput(){},state(...args){summary=args[6]||'';},present(data,w){width=w;colored=data.some((value,index)=>index%4!==3&&value>0);},error(message){errors.push(message)}};
const send=event=>api.event({mods:0,...event});
const key=(code,mods=0)=>{send({kind:'key',code,mods,down:true,repeat:false});api.frame();};
const bytes=()=>Number(summary.match(/\| (\d+) bytes/)[1]);

test('code editor is catalogued and really edits, highlights, indents, pastes and undoes in WASM',async()=>{
 const pages=JSON.parse(api.pages());
 assert.ok(pages.some(page=>page.name==='CodeEditor'));
 assert.ok(pages.some(page=>page.name==='CodeEditorLarge'));
 assert.equal(api.start(host,'gallery:CodeEditor',1100,1,'antd',false),true);
 api.frame();
 const deadline=Date.now()+5000;
 while(!summary.includes('highlighted=true')&&Date.now()<deadline){api.frame();await new Promise(resolve=>setTimeout(resolve,0));}
 assert.ok(summary.includes('main.go | go'),summary);
 assert.ok(summary.includes('highlighted=true'),summary);
 assert.equal(width,1100);assert.equal(colored,true);
 const original=bytes();
 send({kind:'text',text:'demo'});api.frame();
 assert.equal(bytes(),original+4,summary);
 const commandMods=keyboardModifiers({metaKey:true});
 key('KeyZ',commandMods);assert.equal(bytes(),original,'Command undo');
 key('KeyZ',keyboardModifiers({metaKey:true,shiftKey:true}));assert.equal(bytes(),original+4,'Command Shift redo');
 key('Tab');assert.ok(bytes()>original+4,'Tab indents the actual Go document');
 const beforePaste=bytes();const paste='\nconst value = { ok: true };\n';
 for(const event of pasteInput(paste,true))send(event);api.frame();
 assert.equal(bytes(),beforePaste+paste.length,'paste must not auto-close brackets or alter indentation');
 key('KeyZ',2);assert.equal(bytes(),beforePaste,'whole paste is one undo step');
 for(let click=0;click<2;click++){
  for(const phase of [1,2]){send(pointerInput({pointerType:'mouse',button:0,detail:0},phase,{x:115,y:169}));api.frame();}
 }
 key('KeyC',2);assert.equal(clipboard,'import','double click selects a word when PointerEvent.detail is zero');
 api.theme('shadcn',true);assert.equal(api.frame(),true);
 assert.equal(api.start(host,'gallery:CodeEditorLarge',1100,1,'antd',true),true);api.frame();
 assert.ok(summary.includes('app.ts | typescript'),summary);
 assert.ok(Number(summary.match(/\| (\d+) lines/)[1])>=2000,summary);
 send({kind:'scroll',x:400,y:220,dx:0,dy:300,mods:0});assert.equal(api.frame(),true);
 assert.deepEqual(errors,[]);
 api.close();
});
