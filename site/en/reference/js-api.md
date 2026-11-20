# JS runtime API

The page runtime is available as `@tangoui/runtime` for JS and TypeScript. Call `app.AddBridgeScript(runtimejs.Script)` to inject the same runtime as `window.tango` or import it after running `npm install @tangoui/runtime`.

```ts
import { call, on, emit, stream, socket, Window, Clipboard, TangoError } from "@tangoui/runtime";
```

## Calls, events and streams

| Function | Meaning |
|---|---|
| `call<T>(method, ...args): Promise<T>` | Call a Go method registered by `Router.Handle` or `bind.Register` |
| `on(name, fn): () => void` | Subscribe to a Go event and get an unsubscribe function |
| `emit(name, data?)` | Send Go an event |
| `stream<T>(method, ...args): TangoStream<T>` | Open a full-duplex or Go-send-only stream |
| `socket(method, ...args): TangoSocket` | Open a WebSocket-style wrapper over the same stream |
| `onStream(name, fn)` | Handle a Go-initiated stream from `Router.OpenStream`. The handler receives `(stream, ...args)` |
| `onRaw(fn)` / `postRaw(msg)` | Subscribe to or send non-IPC messages |
| `lanes(): { down, up }` | Inspect the current fast lane for each direction or `"fallback"` for the message channel |
| `TangoError` | An IPC error with a `code` |

[Go API and protocol](/en/guide/concepts#ipc-calls-events-streams)

## TangoStream

Streams carry JSON values and byte frames in both directions. A byte-credit window applies backpressure.

| Member | Meaning |
|---|---|
| `id` | Stream ID. JS-initiated streams use odd IDs |
| `accepted: Promise<{ in }>` | Resolves when Go accepts. `in: false` means the Go handler only sends. Failure or cancellation rejects the promise |
| `send(v)` | Queue a JSON frame and return a promise. Encoding happens at call time and `undefined` becomes `null` |
| `sendBytes(b)` | Queue a copy of a `Uint8Array`, `ArrayBuffer` or other `ArrayBufferView` and return a promise |
| `end()` | Finish sending after queued frames and keep receiving |
| `cancel()` | Abandon both directions and cancel the Go handler context |
| `bufferedAmount` | Queued payload bytes plus `FRAME_OVERHEAD` per frame |
| `readable` | `ReadableStream`. Consuming a frame returns credit and canceling the reader cancels the stream |
| `writable` | `WritableStream`. Sends buffers and views as bytes and other values as JSON |
| `for await (const v of s)` | Read decoded JSON or `Uint8Array` frames. Breaking the loop cancels the stream |

Sends resolve as frames are written to the transport and wait while the peer has no credit. Frames bigger than `MAX_FRAME_BYTES` reject with `bad_args`. Direct rejected sends for oversized frames leave the stream usable. Sends after `end()`, a peer close or a failure reject. Accessing `readable` reserves the receive queue for it. Combining it with iteration throws `bad_args`.

The writable view follows peer credit so `pipeTo` backpressures the source. Closing it ends the send direction after queued frames. Aborting it fails the stream with `canceled`. A write rejected with `bad_args` fails the writable stream and underlying Tango stream.

```ts
const up = stream("Upload", file.name, file.size);
await file.stream().pipeTo(up.writable);

const term = stream("Term");
const reader = term.readable.pipeThrough(new TextDecoderStream()).getReader();
const writer = term.writable.getWriter();
await writer.write(new TextEncoder().encode("ls\n"));
```

Constants shared with Go's `ipc` package are `PROTO_VERSION` (2), `DEFAULT_WINDOW_BYTES` (1048576), `FRAME_OVERHEAD` (64), `MAX_FRAME_BYTES` (16777216) and `GAP_TIMEOUT_MS` (5000).

### TangoSocket

A `TangoSocket` is an `EventTarget` with `readyState`, `binaryType` (default `"arraybuffer"`, or `"blob"`), `bufferedAmount`, `url` (`"tango:" + method`), `send`, `close` and the usual open, message, error and close handlers. Create it with `socket()`. Sends while connecting queue up. Strings go as JSON strings and buffers, views and blobs go as byte frames. Received JSON values become strings and bytes arrive in the selected binary type.

```ts
const ws = socket("Echo");
ws.onmessage = (e) => console.log(e.data);
ws.send("hello");
ws.close();
```

Closing waits for queued sends, then closes both directions and emits code 1000 without waiting for Go. The supplied code and reason are local arguments and are not sent. Go errors emit an error and close with 1011. Cancellations, protocol failures and bad sends close with 1006. For backpressure, use `TangoStream`.

## System capability modules

Call `jsapi.Install(app, r, opts)` in Go. On the wire the names use the form `tango:<module>.<function>`.

| Module | Capability | Main API |
|---|---|---|
| `Window` | `Window` (on by default) | Title, sizing and bounds, state, always on top, zoom, reload, print, inspector and window events |
| `App` | `Info` (on by default) and `AppControl` | Environment and lifecycle events. Quit, show/hide, badges, progress and attention require `AppControl` |
| `Theme` | `Info` for `get` and `onChange` | Read dark mode and accent. `vars()` and `onVarsChange(fn)` read tokens injected by `uivars.SyncPage` |
| `Screens` | `Info` | `all`, `primary`, `current`, `onChange` |
| `Clipboard` | `ClipboardRead` / `ClipboardWrite` | Text, base64 PNG and file lists |
| `Dialog` | `Dialogs` | `message`, `open`, `save` |
| `Notify` | `Notify` | `requestPermission`, `send`, `onClick` |
| `Shell` | `OpenURL` / `OpenPath` | Open a permitted URL or open or reveal a path |
| `Menu` | `Menu` (on by default) | `popup(items, {x, y})`, `bindContextMenu(target, build)`. Editing roles only |
| `Hotkey` | `Hotkeys` | `register(accelerator, fn): Promise<() => Promise<void>>`. The registering page owns it |
| `Web` | `WebFind`, `WebCapture`, `WebCookies`, `WebDownloads`, `WebControl` | Find, base64 PDF or PNG capture, cookies and site data, download progress/cancellation, user agent |
| `Events` | Always available | `on`, `once`, `next`, `emit` |

All five `Web` switches are off by default. `WebCookies` can access HttpOnly cookies and other sites' cookies in the WebView store and can clear all site data. Enable it for trusted pages only. [Go capability definitions](https://github.com/tangoui/tango/blob/main/jsapi/webview.go)

## Error codes

| Code | Meaning |
|---|---|
| `forbidden` | Capability disabled, URL scheme disallowed or menu role forbidden |
| `unsupported` | Platform does not support the operation |
| `canceled` | The user or an IPC participant canceled |
| `bad_args` | Invalid parameter type or value |
| `not_found` | No method or calling window |
| `not_running` | App is shutting down |
| `internal` | Other Go error |
| `busy` | An updater check or download is already running when `(*jsapi.API).InstallUpdater` is enabled |
| `transport` | A frame is missing for `GAP_TIMEOUT_MS` after a fast-lane fault. Reopen the stream |

## Platform differences and extensions

Mobile supports basic window operations, app info and quit, text clipboard, message and open dialogs, system appearance, local notifications and opening URLs or files. Window geometry, native menus, tray icons, `Shell.revealPath` and global hotkeys are desktop capabilities. The Web module varies by backend. [Platform matrix](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md)

Use `api.Handle(cap, "share.text", handler)` in Go to register `tango:share.text` with the same capability checks and error codes. Runtime modules use the internal `invoke` and `listen` helpers.
