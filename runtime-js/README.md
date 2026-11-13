# @tangoui/runtime

The page-side runtime of [Tango](https://github.com/tangoui/tango), the Go application framework for desktop and mobile: calls, events and flow-controlled streams to Go, plus the system capability modules (`Window`, `App`, `Theme`, `Screens`, `Clipboard`, `Dialog`, `Notify`, `Shell`, `Menu`, `Hotkey`, `Events`) backed by Tango's `jsapi` package.

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

The package talks to Go through `window.__tango`, which only exists inside a Tango app's WebView. Apps without a bundler do not need this package: the Go side injects the same code as a script (`app.AddBridgeScript(runtimejs.Script)`), available as the global `tango`. The IIFE build is also exported as `@tangoui/runtime/iife`.

The package version matches the Tango Go module version it was released with.

- Documentation: <https://tangoui.github.io/tango/> (Chinese, with English pages in progress)
- API overview: [JS runtime API](https://tangoui.github.io/tango/reference/js-api)
- License: MIT
