// @tangoui/runtime: calls, events and streams between a Tango page and Go.
//
//   import { call, on, emit, stream, socket } from "@tangoui/runtime";
//   const greeting = await call<string>("Greeter.Hello", "world");
//   for await (const line of stream<string>("Logs.Tail")) console.log(line);
//   const ws = socket("Chat.Join"); ws.onmessage = (e) => console.log(e.data);
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
import type { TangoSocket } from "./socket.js";
// Registers the fast lane modules (down:scheme and up:post; down:android
// and up:android). Imports are evaluated before this module's body, so
// they are registered before the eager getClient() below (and in the IIFE,
// which bundles this entry).
import "./lanes/scheme.js";
import "./lanes/android.js";

export { TangoSocket } from "./socket.js";

export {
  PROTO_VERSION,
  DEFAULT_WINDOW_BYTES,
  FRAME_OVERHEAD,
  MAX_FRAME_BYTES,
  GAP_TIMEOUT_MS,
  TangoError,
  type ErrorCode,
  createClient,
  getClient,
  type Bridge,
  type Client,
  type ClientOptions,
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

/** Opens a stream served by Router.HandleDuplex (both directions) or
 * Router.HandleStream (Go sends only). */
export function stream<T = unknown>(method: string, ...args: unknown[]): TangoStream<T> {
  return getClient().stream<T>(method, ...args);
}

/** Opens a stream served by Router.HandleDuplex (or HandleStream) as a
 * WebSocket-like TangoSocket (send() while CONNECTING is queued instead of
 * throwing). */
export function socket(method: string, ...args: unknown[]): TangoSocket {
  return getClient().socket(method, ...args);
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

/** The fast lanes the page uses: the name of the ready downstream and
 * upstream lane (e.g. "down:scheme", "up:post"), or "fallback" when byte
 * frames in that direction go on the message channel. For diagnostics. */
export function lanes(): { down: string; up: string } {
  return getClient().lanes();
}

// Start eagerly inside a Tango surface so the hello handshake and buffered
// messages are handled before user code subscribes.
if ((globalThis as any).__tango) getClient();
