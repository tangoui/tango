// Events: the app's own events between the page and Go (Router.Emit /
// Router.On). Always available; the names are the app's.

import { getClient } from "../client.js";

/** Subscribes to an event Go sends with Router.Emit; returns unsubscribe. */
export function on<T = any>(name: string, fn: (data: T) => void): () => void {
  return getClient().on(name, fn);
}

/** Like on, but unsubscribes after the first event. */
export function once<T = any>(name: string, fn: (data: T) => void): () => void {
  const off = getClient().on(name, (d: T) => {
    off();
    fn(d);
  });
  return off;
}

/** Resolves with the data of the next event called name. */
export function next<T = any>(name: string): Promise<T> {
  return new Promise((resolve) => once<T>(name, resolve));
}

/** Sends an event to Go (Router.On). */
export function emit(name: string, data?: unknown): void {
  getClient().emit(name, data);
}
