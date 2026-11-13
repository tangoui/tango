// @tangoui/runtime: calls, events and streams between a Tango page and Go.
//
//   import { call, on, emit, stream } from "@tangoui/runtime";
//   const greeting = await call<string>("Greeter.Hello", "world");
//   for await (const line of stream<string>("Logs.Tail")) console.log(line);
//
// System capabilities (served by the Go package jsapi) are grouped in
// namespaces; see docs/runtime-api.md:
//
//   import { Window, Clipboard } from "@tangoui/runtime";
//   await Window.setTitle("Notes");
//   const off = Window.onResize(({ width, height }) => layout(width, height));
//
// The module-level functions share one client per window.__tango, so this
// package and the injected IIFE (window.tango) can be used together.

import { getClient, type TangoStream, type StreamHandler } from "./client.js";

export {
  ACK_EVERY,
  WINDOW,
  TangoError,
  type ErrorCode,
  createClient,
  getClient,
  type Bridge,
  type Client,
  type TangoStream,
  type StreamHandler,
} from "./client.js";

export * as Window from "./api/window.js";
export * as App from "./api/app.js";
export * as Theme from "./api/theme.js";
export * as Screens from "./api/screens.js";
export * as Clipboard from "./api/clipboard.js";
export * as Dialog from "./api/dialog.js";
export * as Notify from "./api/notify.js";
export * as Shell from "./api/shell.js";
export * as Menu from "./api/menu.js";
export * as Hotkey from "./api/hotkey.js";
export * as Events from "./api/events.js";
export * as Web from "./api/web.js";
export type { Point, Rect, Size } from "./api/internal.js";

/** Calls a Go method registered with Router.Handle / bind.Register. */
export function call<T = unknown>(method: string, ...args: unknown[]): Promise<T> {
  try {
    return getClient().call<T>(method, ...args);
  } catch (e) {
    return Promise.reject(e);
  }
}

/** Subscribes to an event Go sends with Router.Emit. Returns unsubscribe. */
export function on(name: string, fn: (data: any) => void): () => void {
  return getClient().on(name, fn);
}

/** Sends an event to Go (Router.On). */
export function emit(name: string, data?: unknown): void {
  getClient().emit(name, data);
}

/** Opens a stream served by Router.HandleStream. */
export function stream<T = unknown>(method: string, ...args: unknown[]): TangoStream<T> {
  return getClient().stream<T>(method, ...args);
}

/** Handles streams Go opens with Router.OpenStream. Returns unregister. */
export function onStream(name: string, fn: StreamHandler): () => void {
  return getClient().onStream(name, fn);
}

/** Subscribes to raw (non-ipc) messages from Go (Surface.Send). */
export function onRaw(fn: (msg: string) => void): () => void {
  return getClient().onRaw(fn);
}

/** Sends a raw message to Go (App.OnMessage). */
export function postRaw(msg: string): void {
  getClient().postRaw(msg);
}

// Start eagerly inside a Tango surface so the hello handshake and buffered
// messages are handled before user code subscribes.
if ((globalThis as any).__tango) getClient();
