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
