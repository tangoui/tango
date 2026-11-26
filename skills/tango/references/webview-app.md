# WebView apps

Start with the template's Go entry point and `frontend/`. Keep `@tangoui/runtime` aligned with the version of `github.com/tangoui/tango` in `go.mod`.

1. Build the frontend into `frontend/dist` before directly invoking `go build`. The scaffold embeds that directory. `asset.FromEnv` accepts the embedded filesystem for production and uses the development mode provided by `tan dev`.
2. Create an `ipc.NewRouter()` and register the Go service with `bind.Register(r, "Notes", service)`. Add `runtimejs.Script` from `github.com/tangoui/tango/runtime-js/runtimejs` with `app.AddBridgeScript`.
3. Forward incoming callbacks with `app.OnMessage(func(s *tango.Surface, msg string) { r.HandleMessage(s, msg) })` and preserve the template's `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })` wiring.
4. In `app.OnReady`, create the page with `app.NewSurface` and register `s.OnClosed(func() { r.Drop(s) })`. To restore the window's placement, create it hidden, call `windowstate.Track`, and then call `s.Show()`.
5. For Go events, call `r.Emit` on each intended surface. For an operation such as a dialog initiated from a bound method, declare a leading `context.Context` parameter and look up that request's target with `ipc.ConnFrom(ctx)`. [Capabilities](capabilities.md)

```ts
import { call, on } from "@tangoui/runtime";

const stop = on("menu:save", () => {
  void saveNote().catch(console.error);
});

async function saveNote() {
  await call<void>("Notes.Save", "draft.md", "# Draft");
}

// When the subscribing component unmounts, call stop().
```

Use `window.tango.call` and `window.tango.on` with a plain embedded page that loads the injected runtime. With an npm frontend you can import the same client from `@tangoui/runtime`. To generate typed TypeScript clients, run `tan generate bindings`. Rerun it when Go service methods change. Run generation explicitly when using `tan dev` or `tan build`.

On macOS, put the application menu first and include the `menu.EditMenu` role for copy and paste. [Verification](verify.md)
