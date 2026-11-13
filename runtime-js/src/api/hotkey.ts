// Hotkey: system-wide shortcuts that fire even when the app is inactive.
// Needs Hotkeys (off by default). Shortcuts are dropped when the page
// reloads or its window closes.

import { invoke, listen } from "./internal.js";

const handlers = new Map<number, () => void>();
let off: (() => void) | null = null;

/** Registers a global shortcut such as "CmdOrCtrl+Shift+K" and calls fn
 * when it is pressed. Resolves to a function that unregisters it. */
export async function register(accelerator: string, fn: () => void): Promise<() => Promise<void>> {
  if (!off) {
    off = listen<{ id: number }>("hotkey.fire", (d) => handlers.get(d.id)?.());
  }
  const id = await invoke<number>("hotkey.register", accelerator);
  handlers.set(id, fn);
  return async () => {
    if (!handlers.delete(id)) return;
    await invoke("hotkey.unregister", id);
  };
}
