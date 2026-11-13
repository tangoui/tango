// Pure browser-to-Tango translations, shared with the input regression tests.
export function point(event, rect, width, height) {
  return { x: (event.clientX - rect.left) * width / rect.width, y: (event.clientY - rect.top) * height / rect.height };
}
export function modifiers(event) {
  return (event.shiftKey ? 1 : 0) | (event.ctrlKey ? 2 : 0) | (event.altKey ? 4 : 0) | (event.metaKey ? 8 : 0);
}
export function keyboardModifiers(event) {
  const mods = modifiers(event);
  // The Go js target uses Ctrl bindings with exact modifier matching.
  return event.metaKey ? (mods & ~8) | 2 : mods;
}
export function shouldHandleKey(event, textActive = false, editorKeys = false) {
  if (event.isComposing) return false;
  if (event.code === 'Tab') return editorKeys && textActive;
  if (editorKeys && (event.ctrlKey || event.metaKey) && event.code === 'Slash') return true;
  if (textActive && event.code === 'Space' && !event.ctrlKey && !event.metaKey && !event.altKey) return false;
  if (event.ctrlKey || event.metaKey) return ['KeyA', 'KeyC', 'KeyV', 'KeyX', 'KeyZ', 'KeyY', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'Backspace', 'Delete'].includes(event.code);
  return ['Enter', 'Escape', 'Backspace', 'Delete', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown', 'Space'].includes(event.code);
}
export function textCommit(event) {
  if (event.isComposing || event.inputType === 'insertCompositionText') return null;
  return event.inputType?.startsWith('insert') && event.data ? event.data : null;
}
export function pointerInput(event, phase, position) {
  if (event.pointerType === 'touch') {
    if (phase === 3) return null; // pointerleave is not another touch contact.
    const phases = { 1: 0, 0: 1, 2: 2, 4: 3 };
    return { kind: 'touch', id: event.pointerId, phase: phases[phase], ...position, pressure: event.pressure || 0, mods: modifiers(event) };
  }
  const button = event.button === 2 ? 1 : event.button === 1 ? 2 : 0;
  // PointerEvent.detail may be zero; let Tango count successive clicks then.
  return { kind: 'pointer', phase, ...position, button, clicks: event.detail || 0, mods: modifiers(event) };
}
export function wheelInput(event, position, height, canPanOuter) {
  const factor=event.deltaMode===1?16:event.deltaMode===2?height:1;
  if(event.shiftKey && canPanOuter) return {outerDelta:(event.deltaX || event.deltaY)*factor};
  return {kind:'scroll',...position,dx:(event.shiftKey?(event.deltaX||event.deltaY):event.deltaX)*factor,dy:(event.shiftKey?0:event.deltaY)*factor,mods:modifiers(event)};
}

export function pasteInput(text, codeEditor = false) {
  if (codeEditor) return [{kind:'clipboard',text},{kind:'key',code:'KeyV',down:true,repeat:false,mods:2},{kind:'key',code:'KeyV',down:false,repeat:false,mods:0}];
  return [{kind:'text',text}];
}
