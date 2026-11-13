import { point, keyboardModifiers, shouldHandleKey, textCommit, pointerInput, wheelInput, pasteInput } from './input.mjs';
import { previewLifecycle } from './lifecycle.mjs';
import { decodeWasm } from './wasm-loader.mjs';

const canvas = document.querySelector('#canvas');
const ctx = canvas.getContext('2d');
const ime = document.querySelector('#ime');
const theme = document.querySelector('#theme');
const dark = document.querySelector('#dark');
const reset = document.querySelector('#reset');
const loading = document.querySelector('#loading');
const errorBox = document.querySelector('#error');
const status = document.querySelector('#state');
const requested = new URLSearchParams(location.search).get('component');
const component = requested || 'button';
const isGallery = component.startsWith('gallery:');
const isCodeEditor = component.startsWith('gallery:CodeEditor');
let editorKeys = isCodeEditor;
if (isCodeEditor) document.querySelector('#keyboard-help').textContent = 'Esc 后按 Tab 离开画布';
const stage = document.querySelector('#stage');
const logicalHeight = isGallery ? 560 : 340;
let preferredWidth = 0;
let sceneTitle = component;
document.body.classList.toggle('gallery', isGallery);
const renderWidth = () => isGallery ? Math.max(stage.clientWidth, Math.min(preferredWidth || 960, 1440)) : stage.clientWidth;
function fitCanvas() { canvas.style.width = `${renderWidth()}px`; }
const scale = () => Math.min(devicePixelRatio || 1, 1.5);
let api;
let frame = 0;
let wakeTimer = 0;
let wakeDeadline = Infinity;
let stopped = false;
let paused = false;
let composing = false;
let skipCommit = null;
let skipUntil = 0;
let clipboard = '';
let image;
let textActive = false;
let readyTimer;

function fail(message) {
  stopped = true;
  clearTimeout(readyTimer);
  cancelAnimationFrame(frame);
  clearTimeout(wakeTimer);
  loading.hidden = true;
  reset.disabled = true;
  errorBox.hidden = false;
  errorBox.textContent = `预览加载失败：${message}。请刷新重试。`;
  status.textContent = '预览不可用';
}
function requestFrame() {
  if (frame || stopped || paused || !api) return;
  frame = requestAnimationFrame(() => {
    frame = 0;
    try { api.frame(); } catch (error) { fail(error.message); }
  });
}
const host = {
  requestFrame,
  wake(delay) {
    const deadline = performance.now() + delay;
    if (deadline >= wakeDeadline) return;
    clearTimeout(wakeTimer);
    wakeDeadline = deadline;
    wakeTimer = setTimeout(() => { wakeDeadline = Infinity; requestFrame(); }, delay);
  },
  cursor(value) { canvas.style.cursor = value; },
  clipboard(value) { clipboard = value; },
  textInput(enabled, x, y, height) {
    textActive = enabled;
    ime.style.left = `${Math.max(0, Math.min(canvas.clientWidth - 4, x))}px`;
    ime.style.top = `${Math.max(0, Math.min(logicalHeight - 30, y))}px`;
    ime.style.height = `${Math.max(18, height)}px`;
    if (enabled && document.activeElement === canvas) ime.focus({ preventScroll: true });
    if (!enabled && document.activeElement === ime) canvas.focus({ preventScroll: true });
  },
  present(bytes, width, height) {
    if (!image || image.width !== width || image.height !== height) {
      canvas.width = width;
      canvas.height = height;
      image = ctx.createImageData(width, height);
    }
    image.data.set(bytes);
    ctx.putImageData(image, 0, 0);
    loading.hidden = true;
    reset.disabled = false;
    clearTimeout(readyTimer);
  },
  state(name, language, level, clicks, enabled, tab, sceneStatus) {
    const values = { button: `点击次数：${clicks}`, input: `输入内容：${name || '（空）'}`, switch: `开关：${enabled ? '开启' : '关闭'}`, slider: `数值：${Math.round(level)}`, select: `选择：${language || '（未选择）'}`, tabs: `当前标签：${tab}` };
    const summary = sceneStatus || (isGallery ? `${sceneTitle} · 可滚动、点击和输入` : values[component]);
    if (status.textContent !== summary) status.textContent = summary;
  },
  error: fail,
};
function send(event) {
  if (!api || stopped || paused) return;
  api.event({ mods: 0, ...event });
}
function position(event) { return point(event, canvas.getBoundingClientRect(), canvas.clientWidth, logicalHeight); }
for (const [name, phase] of [['pointermove', 0], ['pointerdown', 1], ['pointerup', 2], ['pointerleave', 3], ['pointercancel', 4]]) {
  canvas.addEventListener(name, event => {
    if (name === 'pointerdown') {
      editorKeys = isCodeEditor;
      canvas.focus({ preventScroll: true });
      canvas.setPointerCapture(event.pointerId);
    }
    if (name === 'pointerleave' && canvas.hasPointerCapture(event.pointerId)) return;
    const input = pointerInput(event, phase, position(event));
    if (input) send(input);
    if (name === 'pointerdown') {
      api?.frame(); // SetTextInput can focus the IME in this user gesture.
      event.preventDefault();
    }
  });
}
canvas.addEventListener('contextmenu', event => event.preventDefault());
canvas.addEventListener('wheel', event => {
  const input = wheelInput(event, position(event), logicalHeight, isGallery && stage.scrollWidth > stage.clientWidth);
  if ('outerDelta' in input) stage.scrollLeft += input.outerDelta;
  else send(input);
  event.preventDefault();
}, { passive: false });
canvas.addEventListener('focus', () => {
  send({ kind: 'focus', focused: true });
  if (textActive) { ime.focus({ preventScroll: true }); return; }
  // One focusable canvas represents the Go controls. Editors capture Tab for
  // indentation until Escape releases it back to browser navigation.
  if (api && !textActive) {
    send({ kind: 'key', code: 'Tab', down: true, repeat: false });
  }
});
for (const target of [canvas, ime]) {
  target.addEventListener('keydown', event => {
    if ((event.ctrlKey || event.metaKey) && ['KeyC', 'KeyV', 'KeyX'].includes(event.code)) return;
    if (shouldHandleKey(event, textActive, editorKeys)) {
      // Go's js target uses Ctrl shortcuts; map Command to Ctrl as well.
      send({ kind: 'key', code: event.code, down: true, repeat: event.repeat, mods: keyboardModifiers(event) });
      if (event.code === 'Escape') { editorKeys = false; ime.blur(); canvas.focus({ preventScroll: true }); }
      event.preventDefault();
    }
  });
  target.addEventListener('keyup', event => {
    if (shouldHandleKey(event, textActive, editorKeys)) send({ kind: 'key', code: event.code, down: false, repeat: false, mods: keyboardModifiers(event) });
  });
  target.addEventListener('paste', event => {
    if (!textActive) return;
    for (const input of pasteInput(event.clipboardData.getData('text/plain'), isCodeEditor)) send(input);
    event.preventDefault();
  });
  for (const name of ['copy', 'cut']) target.addEventListener(name, event => {
    if (!api) return;
    send({ kind: 'key', code: name === 'copy' ? 'KeyC' : 'KeyX', down: true, repeat: false, mods: 2 });
    api.frame();
    event.clipboardData.setData('text/plain', clipboard);
    event.preventDefault();
  });
}
ime.addEventListener('beforeinput', event => {
  if (event.isComposing) return;
  const code = { deleteContentBackward: 'Backspace', deleteContentForward: 'Delete' }[event.inputType];
  if (code) { send({ kind: 'key', code, down: true, repeat: false }); event.preventDefault(); }
});
ime.addEventListener('input', event => {
  if (composing) return;
  const text = textCommit(event);
  if (text && !(text === skipCommit && performance.now() < skipUntil)) send({ kind: 'text', text });
  skipCommit = null;
  ime.value = '';
});
ime.addEventListener('compositionstart', () => { composing = true; });
ime.addEventListener('compositionupdate', event => send({ kind: 'composition', text: event.data || '' }));
ime.addEventListener('compositionend', event => {
  composing = false;
  send({ kind: 'composition', text: '' });
  if (event.data) send({ kind: 'text', text: event.data });
  skipCommit = event.data;
  skipUntil = performance.now() + 100;
  ime.value = '';
});
window.addEventListener('blur', () => send({ kind: 'focus', focused: false }));
const observer = new ResizeObserver(() => { fitCanvas(); api?.resize(renderWidth(), scale()); });
observer.observe(stage);
function start() {
  composing = false;
  textActive = false;
  ime.value = '';
  clearTimeout(wakeTimer);
  wakeDeadline = Infinity;
  fitCanvas();
  api.start(host, component, renderWidth(), scale(), theme.value, dark.checked);
}
reset.addEventListener('click', () => { if (isGallery) location.reload(); else start(); });
document.querySelector('#font').addEventListener('change', async event => {
  const file = event.target.files?.[0];
  if (!file || !api) return;
  try {
    if (file.size > 30 * 1024 * 1024) throw new Error('字体文件超过 30 MB');
    const error = api.font(new Uint8Array(await file.arrayBuffer()));
    if (error) throw new Error(error);
    status.textContent = `已导入字体：${file.name}`;
  } catch (error) { status.textContent = `字体加载失败：${error.message}`; }
  event.target.value = '';
});
for (const control of [theme, dark]) control.addEventListener('change', () => {
  document.documentElement.classList.toggle('dark', dark.checked);
  api?.theme(theme.value, dark.checked);
});
function clearScheduledWork() {
  observer.disconnect();
  cancelAnimationFrame(frame);
  frame = 0;
  clearTimeout(wakeTimer);
  wakeDeadline = Infinity;
}
const lifecycle = previewLifecycle({
  pause() { paused = true; clearScheduledWork(); },
  resume() { paused = false; observer.observe(stage); requestFrame(); },
  dispose() {
    stopped = true;
    clearScheduledWork();
    clearTimeout(readyTimer);
    api?.close();
  },
});
window.addEventListener('pagehide', lifecycle.hide);
window.addEventListener('pageshow', lifecycle.show);

async function load() {
  if (!ctx || !globalThis.WebAssembly || !globalThis.Go) throw new Error('浏览器缺少 Canvas、WebAssembly 或 Go 运行时');
  readyTimer = setTimeout(() => fail('WASM 加载超时'), 45000);
  const go = new globalThis.Go();
  let response = await fetch(typeof DecompressionStream === 'function' ? './playground.wasm.gz' : './playground.wasm');
  if (!response.ok) response = await fetch('./playground.wasm');
  if (!response.ok) throw new Error(`WASM HTTP ${response.status}`);
  // ArrayBuffer instantiation also works on hosts without application/wasm MIME.
  const result = await WebAssembly.instantiate(await decodeWasm(await response.arrayBuffer()), go.importObject);
  window.addEventListener('tango-preview-ready', () => { queueMicrotask(() => {
    api = globalThis.tangoPreview;
    if (isGallery) {
      const scene = JSON.parse(api.pages()).find(page => `gallery:${page.name}` === component);
      if (!scene) { fail('Unknown component'); return; }
      preferredWidth = scene.width || 960;
      sceneTitle = scene.title;
      canvas.setAttribute('aria-label', `${sceneTitle} · 点击、拖动或滚动，${isCodeEditor ? 'Esc 后按 Tab' : 'Tab'} 离开预览`);
    } else if (!['button', 'input', 'switch', 'slider', 'select', 'tabs'].includes(component)) { fail('Unknown component'); return; }
    start();
  }); }, { once: true });
  globalThis.tangoUseMonoFont = isCodeEditor;
  go.run(result.instance).catch(error => fail(error.message));
}
load().catch(error => fail(error.message));
