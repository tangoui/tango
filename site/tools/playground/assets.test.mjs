import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';

test('compressed download is the same complete WASM module and saves transfer size',()=>{
 const wasm=readFileSync(new URL('../../public/playground/playground.wasm',import.meta.url));
 const packed=readFileSync(new URL('../../public/playground/playground.wasm.gz',import.meta.url));
 assert.deepEqual(gunzipSync(packed),wasm);
 assert.ok(packed.length<wasm.length*.5);
});

test('download decoder handles compressed and already-decoded bytes and rejects invalid assets',async()=>{
 const { decodeWasm }=await import('../../public/playground/wasm-loader.mjs');
 const wasm=readFileSync(new URL('../../public/playground/playground.wasm',import.meta.url));
 const packed=readFileSync(new URL('../../public/playground/playground.wasm.gz',import.meta.url));
 assert.deepEqual(Buffer.from(await decodeWasm(packed)),wasm);
 assert.deepEqual(Buffer.from(await decodeWasm(wasm)),wasm);
 await assert.rejects(decodeWasm(new Uint8Array([1,2,3])),/WASM/);
});
