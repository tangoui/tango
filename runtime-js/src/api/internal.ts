// Shared plumbing of the runtime API modules (Window, App, ...). Each
// module function is an ipc call named "tango:<module>.<function>" served
// by the Go package jsapi; events are named "tango:<module>.<event>".
//
// New modules (for example the mobile sys/ capabilities) follow the same
// pattern: a file in src/api exporting functions built on invoke/listen,
// re-exported as a namespace from index.ts.

import { getClient } from "../client.js";

/** Prefix of every runtime API method and event name. */
export const PREFIX = "tango:";

/** Calls the runtime API method PREFIX + method. */
export function invoke<T = void>(method: string, ...args: unknown[]): Promise<T> {
  try {
    return getClient().call<T>(PREFIX + method, ...args);
  } catch (e) {
    return Promise.reject(e);
  }
}

/** Subscribes to the runtime API event PREFIX + name; returns unsubscribe. */
export function listen<T>(name: string, fn: (data: T) => void): () => void {
  return getClient().on(PREFIX + name, fn);
}

/** Reports a listener exception without breaking the caller. */
export function report(err: unknown): void {
  (globalThis as any).setTimeout(() => {
    throw err;
  }, 0);
}

/** A rectangle in logical pixels. */
export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** A size in logical pixels. */
export interface Size {
  width: number;
  height: number;
}

/** A point in logical pixels. */
export interface Point {
  x: number;
  y: number;
}
