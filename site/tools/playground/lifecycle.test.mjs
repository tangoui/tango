import { test } from 'node:test';
import assert from 'node:assert/strict';
import { previewLifecycle } from '../../public/playground/lifecycle.mjs';

test('back/forward cache pauses and restores the existing Go session',()=>{
 const calls=[];
 const lifecycle=previewLifecycle({pause:()=>calls.push('pause'),resume:()=>calls.push('resume'),dispose:()=>calls.push('dispose')});
 lifecycle.hide({persisted:true});
 assert.deepEqual(calls,['pause']);
 lifecycle.show({persisted:true});
 assert.deepEqual(calls,['pause','resume']);
 lifecycle.hide({persisted:false});
 lifecycle.show({persisted:true});
 assert.deepEqual(calls,['pause','resume','dispose'],'a disposed frame cannot be revived');
});
