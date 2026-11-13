import { test } from 'node:test';
import assert from 'node:assert/strict';
import { point, modifiers, keyboardModifiers, shouldHandleKey, textCommit } from '../../public/playground/input.mjs';

test('pointer coordinates follow displayed canvas bounds and logical width',()=>{
 assert.deepEqual(point({clientX:210,clientY:120},{left:10,top:20,width:400,height:200},800,400),{x:400,y:200});
});
test('keyboard modifiers preserve multiple held keys',()=>{
 assert.equal(modifiers({shiftKey:true,ctrlKey:false,altKey:true,metaKey:true}),13);
 assert.equal(keyboardModifiers({shiftKey:true,metaKey:true}),3);
 assert.equal(keyboardModifiers({ctrlKey:true,altKey:true}),6);
});
test('Tab can leave preview with Escape then Tab; browser refresh is preserved',()=>{
 assert.equal(shouldHandleKey({code:'ArrowLeft',ctrlKey:false,metaKey:false,isComposing:false}),true);
 assert.equal(shouldHandleKey({code:'KeyR',ctrlKey:true,isComposing:false}),false);
 assert.equal(shouldHandleKey({code:'KeyA',ctrlKey:true,isComposing:false}),true);
 assert.equal(shouldHandleKey({code:'KeyA',isComposing:true}),false);
 assert.equal(shouldHandleKey({code:'Tab',isComposing:false}),false);
});
test('IME provisional text is not committed; insertText commits once',()=>{
 assert.equal(textCommit({inputType:'insertCompositionText',isComposing:true,data:'ni'}),null);
 assert.equal(textCommit({inputType:'insertText',isComposing:false,data:'你'}),'你');
 assert.equal(textCommit({inputType:'deleteContentBackward',isComposing:false,data:null}),null);
});

test('text fields keep native Space input while buttons handle Space activation', () => {
  const event = { code: 'Space', isComposing: false };
  assert.equal(shouldHandleKey(event, true), false);
  assert.equal(shouldHandleKey(event, false), true);
});

test('touch contacts are sent once as real Tango touch events with stable IDs', async () => {
 const { pointerInput } = await import('../../public/playground/input.mjs');
 const touch={pointerType:'touch',pointerId:42,pressure:.5};
 assert.deepEqual(pointerInput(touch,1,{x:10,y:20}),{kind:'touch',id:42,phase:0,x:10,y:20,pressure:.5,mods:0});
 assert.equal(pointerInput(touch,3,{x:10,y:20}),null);
 assert.equal(pointerInput(touch,4,{x:10,y:20}).phase,3);
 const mouse=pointerInput({pointerType:'mouse',button:2,detail:2},1,{x:2,y:3});
 assert.equal(mouse.kind,'pointer');assert.equal(mouse.button,1);assert.equal(mouse.clicks,2);
});

test('native horizontal wheel stays in Go; Shift pans the outer canvas only when it overflows',async()=>{
 const { wheelInput } = await import('../../public/playground/input.mjs');
 assert.deepEqual(wheelInput({deltaX:80,deltaY:0,deltaMode:0}, {x:5,y:6},560,true),{kind:'scroll',x:5,y:6,dx:80,dy:0,mods:0});
 assert.equal(wheelInput({deltaX:0,deltaY:90,deltaMode:0,shiftKey:true},{x:5,y:6},560,true).outerDelta,90);
 assert.equal(wheelInput({deltaX:0,deltaY:90,deltaMode:0,shiftKey:true},{x:5,y:6},560,false).dx,90);
});

test('code editor captures Tab for indentation and releases it in exit mode',()=>{
 const tab={code:'Tab',isComposing:false};
 assert.equal(shouldHandleKey(tab,true,true),true);
 assert.equal(shouldHandleKey(tab,true,false),false);
 assert.equal(shouldHandleKey({code:'Space',isComposing:false},true,true),false);
 assert.equal(shouldHandleKey({code:'Slash',ctrlKey:true,isComposing:false},true,true),true);
});
test('editor paste uses clipboard insertion rather than smart typing assistance',async()=>{
 const { pasteInput }=await import('../../public/playground/input.mjs');
 const events=pasteInput('const value = { ok: true };',true);
 assert.equal(events[0].kind,'clipboard');
 assert.equal(events[1].code,'KeyV');
 assert.equal(events[1].mods,2);
 assert.equal(events[2].down,false);assert.equal(events[2].mods,0);
 assert.deepEqual(pasteInput('hello',false),[{kind:'text',text:'hello'}]);
});
