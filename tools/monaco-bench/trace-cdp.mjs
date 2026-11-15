// Adapted from bench.mjs's CDP/WebSocket client. Kept separate because importing
// bench.mjs runs its benchmark main(). No changes to the existing scenario path.
export class CDP {
  constructor(url, timeoutMs = 15000) {
    this.id = 0;
    this.timeoutMs = timeoutMs;
    this.pending = new Map();
    this.listeners = new Map();
    this.ws = new WebSocket(url);
    this.ws.onmessage = (ev) => {
      const m = JSON.parse(ev.data);
      if (m.id !== undefined) {
        const p = this.pending.get(m.id);
        if (!p) return;
        clearTimeout(p.timer);
        this.pending.delete(m.id);
        if (m.error) p.reject(new Error(`${p.method}: ${m.error.message}`));
        else p.resolve(m.result);
      } else if (m.method) {
        for (const fn of [...(this.listeners.get(m.method) || [])]) fn(m.params);
      }
    };
    this.opened = new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('CDP connect timeout')), timeoutMs);
      this.ws.onopen = () => { clearTimeout(timer); resolve(); };
      this.ws.onerror = () => { clearTimeout(timer); reject(new Error('CDP websocket error')); };
    });
    this.ws.onclose = () => {
      for (const p of this.pending.values()) {
        clearTimeout(p.timer);
        p.reject(new Error('CDP websocket closed'));
      }
      this.pending.clear();
    };
  }
  send(method, params = {}, timeoutMs = this.timeoutMs) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`${method}: timeout`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, method, timer });
      try { this.ws.send(JSON.stringify({ id, method, params })); }
      catch (e) { clearTimeout(timer); this.pending.delete(id); reject(e); }
    });
  }
  on(method, fn) {
    (this.listeners.get(method) || this.listeners.set(method, new Set()).get(method)).add(fn);
    return () => this.listeners.get(method)?.delete(fn);
  }
  once(method, timeoutMs = this.timeoutMs) {
    return new Promise((resolve, reject) => {
      const off = this.on(method, (params) => { clearTimeout(timer); off(); resolve(params); });
      const timer = setTimeout(() => { off(); reject(new Error(`${method}: event timeout`)); }, timeoutMs);
    });
  }
  close() { try { this.ws.close(); } catch {} }
}

// Match the new document, not a late about:blank load from browser startup.
export async function navigateDocument(cdp, url, timeoutMs = 15000) {
  await cdp.send('Page.setLifecycleEventsEnabled', { enabled: true });
  let expected, resolveLoad, timer;
  const seen = [];
  const loaded = new Promise((resolve, reject) => {
    resolveLoad = resolve;
    timer = setTimeout(() => reject(new Error('navigation load lifecycle timeout')), timeoutMs);
  });
  loaded.catch(() => {});
  const matches = (e) => expected && e.name === 'load' && e.frameId === expected.frameId && e.loaderId === expected.loaderId;
  const off = cdp.on('Page.lifecycleEvent', (e) => {
    seen.push(e);
    if (matches(e)) resolveLoad();
  });
  try {
    expected = await cdp.send('Page.navigate', { url });
    if (expected.errorText) throw new Error(`Page.navigate failed: ${expected.errorText}`);
    if (!expected.loaderId) throw new Error('Page.navigate returned no new document loader');
    if (seen.some(matches)) resolveLoad();
    await loaded;
    return expected;
  } finally { clearTimeout(timer); off(); }
}

export async function evaluate(cdp, expression, timeoutMs = 30000) {
  const r = await cdp.send('Runtime.evaluate', {
    expression, awaitPromise: true, returnByValue: true, timeout: timeoutMs,
  }, timeoutMs + 1000);
  if (r.exceptionDetails) {
    const d = r.exceptionDetails;
    throw new Error(`page: ${d.exception?.description || d.text}`);
  }
  return r.result.value;
}
