# @tangoui/runtime

The page-side runtime of [Tango](https://github.com/tangoui/tango), the Go application framework for desktop and mobile: calls, events and flow-controlled, bidirectional streams to Go, plus the system capability modules (`Window`, `App`, `Theme`, `Screens`, `Clipboard`, `Dialog`, `Notify`, `Shell`, `Menu`, `Hotkey`, `Events`) backed by Tango's `jsapi` package.

```bash
npm install @tangoui/runtime
```

```ts
import { call, on, emit, stream, Window, Clipboard, TangoError } from "@tangoui/runtime";

const greeting = await call<string>("Greeter.Hello", "world");   // bind.Register(r, "Greeter", ...)
const off = on("progress", ({ pct }) => console.log(pct));       // Router.Emit in Go
for await (const line of stream<string>("Logs.Tail")) {          // Router.HandleStream in Go
  if (line.includes("ready")) break;                             // breaking out cancels it in Go
}

await Window.setTitle("Notes");
try {
  await Clipboard.writeText("hi");
} catch (e) {
  if (e instanceof TangoError && e.code === "forbidden") {
    // the Go side did not enable jsapi.ClipboardWrite
  }
}
```

Streams are full duplex and carry JSON values and bytes in both directions, with byte-counted credit flow control (Go's window, 1 MiB by default, limits what is in flight). A `TangoStream` has `send(v)` (a JSON frame), `sendBytes(b)` (a byte frame), `end()` (half close), `cancel()`, `accepted` (resolves with `{ in }` once Go accepted the stream; `in` is false for a send-only `Router.HandleStream`), `bufferedAmount`, and Web Streams views `readable` and `writable`. Iterating yields decoded JSON values and `Uint8Array`s for byte frames.

```ts
const s = stream("Echo");                                        // Router.HandleDuplex in Go
s.send({ hello: 1 });                                            // not awaiting is fine: frames are queued in order
await s.sendBytes(new Uint8Array([1, 2, 3]));                    // resolves once written; waits while Go's window is full
s.end();                                                         // no more sends; keep reading
for await (const v of s) console.log(v);

const up = stream("Upload", file.name, file.size);
await file.stream().pipeTo(up.writable);                         // paced by Go's reading
```

- `send`/`sendBytes` reject with `bad_args` for a payload over `MAX_FRAME_BYTES` (the stream stays usable) and once the stream can no longer send; an un-awaited send never causes an unhandled rejection.
- `readable` and the async iterator share one receive queue: use one of them. Merely accessing `s.readable` locks the stream to it.
- `writable.close()` writes the queued frames, then ends; `writable.abort(reason)` fails the stream with code `canceled` and message `String(reason?.message ?? reason)`. A write rejected with `bad_args` (a `BigInt`, an oversized chunk) fails the whole stream.
- Constants shared with Go's `ipc` package: `PROTO_VERSION` (2), `DEFAULT_WINDOW_BYTES`, `FRAME_OVERHEAD`, `MAX_FRAME_BYTES`, `GAP_TIMEOUT_MS`. This runtime speaks wire protocol v2 only and Go rejects any other version, so use the release matching your Go module.

`socket(method, ...args)` opens the same kind of stream as a WebSocket-like `TangoSocket` (an `EventTarget` with `open`/`message`/`error`/`close`, `readyState`, `send`, `close`, `binaryType`, `bufferedAmount`; `url` is `"tango:" + method`):

```ts
import { socket } from "@tangoui/runtime";

const ws = socket("Chat.Join", "lobby");                         // Router.HandleDuplex in Go
ws.onmessage = (e) => console.log(e.data);                       // string, or ArrayBuffer/Blob for bytes
ws.send("hello");                                                // fine before "open": it is queued
ws.close();                                                      // sends close after queued sends; "close" (1000) follows at once
```

Differences from `WebSocket`:

- `send()` while `CONNECTING` queues the message instead of throwing `InvalidStateError`.
- JSON values other than strings arrive as `JSON.stringify` text.
- `close(code, reason)` does not send code or reason to Go. It closes the stream (op `close`, like Go's `Stream.Close`) once the queued sends are written, then fires `close` (1000, `wasClean: true`) without waiting for Go; Go's writes then fail with "stream closed by peer". This also works while `CONNECTING` and on streams served by `HandleStream`. Messages arriving after `close()` are dropped.
- A Go error fires `error` and then `close` with code 1011, the error message as reason and `wasClean: false`; a normal end closes with 1000; a Go cancel, a client failure or a `send` rejected with `bad_args` (over `MAX_FRAME_BYTES`, or Go does not accept data) closes with 1006.
- The constructor is internal: create sockets only with `socket()`.
- There is no real back-pressure (only `bufferedAmount`, as with WebSocket); use `TangoStream` or its Web Streams views when you need it.

## Fast lanes

Byte frames of streams can travel on fast lanes: binary side channels next to the message channel that skip JSON and base64 (JSON frames, control messages and the handshake always use the message channel). This package negotiates them with Go at the handshake and starts the lane modules it has (`down:scheme` / `up:post`, a streaming response and POSTs under `/__tango/lane/` on the app's asset origin; platform lanes add their own modules). Nothing changes in your code: frames of one stream may be split between a lane and the message channel and are reordered by sequence number.

A page gets lanes only when all of these hold: the Go app wires `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })`; the page comes from the asset origin (a page loaded straight from a dev server URL never does); the platform backend reports lanes; and they are not turned off (`tango.Options.DisableFastLanes`, or `TANGO_IPC_LANES=0` in the app's environment). **Only macOS reports a lane by default** (`up:post`; `TANGO_IPC_LANES_DARWIN=0` turns it off); iOS can opt in with `TANGO_IPC_LANES_DARWIN=1` (on a device: `os.Setenv("TANGO_IPC_LANES_DARWIN", "1")` in Go before the first page is created), and the other platforms use the message channel until their backends land. `up:post` starts by POSTing an empty packet and is ready only on its 204, so a page whose CSP does not allow `connect-src` to the asset origin keeps its byte frames on the message channel, without an error.

```ts
import { lanes } from "@tangoui/runtime";

lanes(); // { down: "fallback", up: "up:post" } on macOS by default; "fallback" means a direction without a lane
```

`lanes()` is for diagnostics; its value changes as lanes become ready, break, or the page shakes hands again. A lane that breaks is dropped at once and later frames go on the message channel. Frames lost on it are not resent: when a stream misses a frame for `GAP_TIMEOUT_MS` (5000) while later frames or its end have arrived, the stream fails on both sides with a `TangoError` whose `code` is `"transport"`. Treat it like a dropped connection and reopen the stream. In development builds of the Go app (`-tags tango_dev`), `TANGO_IPC_LANE_FAULT=<lane>:<N>` (for example `down:scheme:3`) makes that lane fail after N data packets (empty packets, such as the `up:post` start probe, are not counted), to test this path.

Fast-lane delivery is bound to the document session: the lane token is per generation and used by every lane request in it. Reliable stream `boundary` messages (sent on the message channel once a lane frame is actually on its way, not when it is queued) expose even a lost final frame of an open duplex; only streams with unresolved receive sequences fail after 5 seconds. Under a stalled main thread: every later message-channel post of a stream (data, end, close) waits until its earlier lane frames went out, and the gap timer and the `up:post` request timer, when they fire more than 1 s late, wait one more full timeout (a second expiry fails as usual). Abort paths (writable abort, a refused chunk, `cancel()`, a local error) still post their close/error at once, so a stall longer than 5 s can make Go report `transport` instead of the abort reason. Each `up:post` request aborts after 4 seconds (below the gap timeout, so a hung POST fails the lane and later frames fall back first); retained encoded POST bytes, in flight included, are capped at 8 MiB, and excess frames take the message channel without changing sequence or credit. Lane requests resolve against the document's actual location, independent of HTML `<base>`, and refuse redirects.

The package talks to Go through `window.__tango`, which only exists inside a Tango app's WebView. Apps without a bundler do not need this package: the Go side injects the same code as a script (`app.AddBridgeScript(runtimejs.Script)`), available as the global `tango`. The IIFE build is also exported as `@tangoui/runtime/iife`.

The package version matches the Tango Go module version it was released with.

## Local development

Inside the Tango repository the sources are in `runtime-js/src`.

```bash
cd runtime-js
npm ci
npm run build   # dist/ (ESM, IIFE, .d.ts) and runtimejs/tango.iife.js
npm test        # build, unit tests, API report and wire contract checks
```

- **Injected script** (`runtimejs.Script`): it is `runtimejs/tango.iife.js` embedded with `go:embed`, so after `npm run build` the Go program must be rebuilt and the app restarted; reloading the page is not enough. Commit the regenerated `runtimejs/tango.iife.js` with the source change.
- **npm frontend**: point the dependency at the checkout, relative to the frontend's `package.json`. For `examples/<app>/frontend/package.json` in the repository: `"@tangoui/runtime": "file:../../../runtime-js"`. The package entry is the git-ignored `dist/`, so run `npm run build` first and again after each change. `tan init -local <tango checkout>` writes such a `file:` dependency for projects outside the repository.

## API stability

The exported names and type signatures, every namespace member (`Window.setTitle`, ...), the `TangoError` codes, and the wire names (`tango:<module>.<function>` methods, `tango:<module>.<event>` events) with their JSON shapes are public API. Two committed files make changes visible in review:

- `etc/runtime.api.md`: the API Extractor report of `dist/index.d.ts`, namespaces expanded member by member. `npm run api:update` rewrites it; `npm run api:check` (CI) fails when it is stale.
- `../api/jsapi-methods.txt`: every error code, method and event of the Go `jsapi` package and the functions here that use them. `npm run contract:update` rewrites the `js=` column, `npm run contract:check` checks it; the Go columns come from `go test ./jsapi -run TestWireContract -update`.

These checks are structural: they catch renamed, removed or re-typed exports and wire names, not behaviour changes or changes to the JSON exchanged with Go. See "API 稳定性" in `docs/runtime-api.md` for the full policy and limits.

- Documentation: <https://tangoui.github.io/tango/> (Chinese, with English pages in progress)
- API overview: [JS runtime API](https://tangoui.github.io/tango/reference/js-api)
- License: MIT
