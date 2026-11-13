// Runs the actual Go WASM module under Node: no copied JS control behavior.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import '../../public/playground/wasm_exec.js';

const bus = new EventTarget();
globalThis.dispatchEvent = event => bus.dispatchEvent(event);
const ready = new Promise(resolve => bus.addEventListener('tango-preview-ready', resolve, { once: true }));
const go = new globalThis.Go();
const bytes = await readFile(new URL('../../public/playground/playground.wasm', import.meta.url));
const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
const running = go.run(instance);
running.catch(error => { throw error; });
await ready;
const api = globalThis.tangoPreview;
let state;
let frame;
const host = {
 requestFrame() {}, wake() {}, cursor() {}, clipboard() {}, textInput() {},
 present(data, width, height) { frame = { bytes: Uint8Array.from(data), width, height }; },
 state(...value) { state = value; },
 error(message) { throw new Error(message); },
};
const send = event => api.event({ mods: 0, ...event });
const key = code => { send({kind:'key',code,down:true,repeat:false});api.frame(); };

test('WASM renders real pixels, processes input, switches themes, and resets state', () => {
 assert.equal(api.start(host,'button',600,1,'antd',false),true);
 assert.equal(api.frame(),true);
 assert.equal(frame.width,600);
 assert.equal(frame.height,340);
 assert.equal(frame.bytes.length,600*340*4);
 assert.ok(frame.bytes.some((v,i)=>i%4!==3 && v>0),'frame has color pixels');
 const light=frame.bytes;
 key('Tab');key('Enter');
 assert.equal(state[3],1,'Go button click counter');
 api.theme('shadcn',true);api.frame();
 assert.notDeepEqual(frame.bytes,light,'theme changes the rendered frame');
 api.start(host,'input',600,1,'element',false);api.frame();key('Tab');
 send({kind:'text',text:'hello'});api.frame();
 assert.equal(state[0],'hello','Go input owns the text value');
 key('Backspace');assert.equal(state[0],'hell');
 api.start(host,'switch',600,1,'antd',false);api.frame();key('Tab');key('Space');
 assert.equal(state[4],true);
 api.start(host,'slider',600,1,'antd',false);api.frame();key('Tab');key('ArrowRight');
 assert.ok(state[2]>35);
 api.start(host,'select',600,1,'antd',false);api.frame();key('Tab');key('Enter');key('ArrowDown');key('Enter');
 assert.notEqual(state[1],'');
 api.start(host,'tabs',600,1,'antd',false);api.frame();key('Tab');key('ArrowRight');
 assert.equal(state[5],'events');
 api.start(host,'button',600,1,'antd',false);api.frame();
 assert.equal(state[3],0,'reset replaces the old Go state');
 api.resize(360,1);api.frame();assert.equal(frame.width,360);
 api.close();
});
