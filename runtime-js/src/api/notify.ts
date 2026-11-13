// Notify: system notifications. Needs Notify (off by default). On macOS
// the app must run from a signed .app bundle.

import { invoke, listen } from "./internal.js";

export interface Notification {
  title: string;
  /** Under the title on macOS; merged into the body elsewhere. */
  subtitle?: string;
  body?: string;
  /** No sound. */
  silent?: boolean;
}

/** Asks the user (if needed) to allow notifications; resolves to whether
 * they may be shown. */
export function requestPermission(): Promise<boolean> {
  return invoke("notify.requestPermission");
}

/** Shows a notification and resolves to its id. */
export function send(n: Notification): Promise<string> {
  return invoke("notify.send", n);
}

/** Called with the id of a notification this page sent when the user
 * clicks it. */
export function onClick(fn: (id: string) => void): () => void {
  return listen<{ id: string }>("notify.click", (d) => fn(d.id));
}
