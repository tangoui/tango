# WebView apps

Start with the template's Go entry point and `frontend/`. Keep `@tangoui/runtime` aligned with the version of `github.com/tangoui/tango` in `go.mod`.

1. Build the frontend into `frontend/dist` before directly invoking `go build`. The scaffold embeds that directory. `asset.FromEnv` accepts the embedded filesystem for production and uses the development mode provided by `tan dev`.
2. Create an `ipc.NewRouter()` and register the Go service with `bind.Register(r, "Notes", service)`. Add `runtimejs.Script` from `github.com/tangoui/tango/runtime-js/runtimejs` with `app.AddBridgeScript`.
3. Forward incoming messages with `app.OnMessage(func(s *tango.Surface, msg string) { r.HandleMessage(s, msg) })`. This is sufficient for the scaffold's ordinary calls and events. If you enable IPC binary fast lanes, also wire incoming lane packets with `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })`.
4. In `app.OnReady`, create the page with `app.NewSurface` and register `s.OnClosed(func() { r.Drop(s) })`. To restore the window's placement, create it hidden, call `windowstate.Track`, and then call `s.Show()`.
5. For Go events, call `r.Emit` on each intended surface. For an operation such as a dialog initiated from a bound method, declare a leading `context.Context` parameter and look up that request's target with `ipc.ConnFrom(ctx)`. [Capabilities](capabilities.md)

```ts
import { call, on } from "@tangoui/runtime";

const stop = on("menu:save", () => {
  void saveNote().catch(console.error);
});

async function saveNote() {
  const name = await call<string>("Notes.Save", "# Draft");
  if (name) console.log("Saved " + name);
}

// When the subscribing component unmounts, call stop().
```

Use `window.tango.call` and `window.tango.on` with a plain embedded page that loads the injected runtime. With an npm frontend you can import the same client from `@tangoui/runtime`. To generate typed TypeScript clients, run `tan generate bindings`. Rerun it when Go service methods change. Run generation explicitly when using `tan dev` or `tan build`.

On macOS, put the application menu first and add `menu.EditMenu` at the menu bar level for copy and paste. [Verification](verify.md)

## Script dialogs

macOS dismisses unhandled JavaScript `alert`, `confirm`, and `prompt` dialogs. Other platforms can provide their own default UI. For consistent behavior, use a page dialog or register `Surface.SetWebDialogHandler` before the page calls these functions. For a Go handler, start blocking dialog work outside the callback and reply once. [Public Go API](https://pkg.go.dev/github.com/tangoui/tango#Surface.SetWebDialogHandler)

```go
func installScriptDialogs(app *tango.App, s *tango.Surface) error {
	return s.SetWebDialogHandler(func(req tango.WebDialogRequest, reply func(tango.WebDialogResponse)) {
		go func() {
			options := dialog.MessageOptions{Parent: s, Title: "Page message", Message: req.Message}
			switch req.Kind {
			case tango.WebDialogAlert:
				_, _ = dialog.Message(app, options)
				reply(tango.WebDialogResponse{})
			case tango.WebDialogConfirm, tango.WebDialogBeforeUnload:
				options.Buttons = []string{"OK", "Cancel"}
				button, err := dialog.Message(app, options)
				reply(tango.WebDialogResponse{Accepted: err == nil && button == 0})
			default:
				reply(tango.WebDialogResponse{})
			}
		}()
	})
}
```

This handler displays alerts and confirms and dismisses prompts. To support `prompt`, show a text field initialized from `req.DefaultText` and return `tango.WebDialogResponse{Accepted: true, Text: value}` when the user accepts it.

From your app directory, run `go run -mod=mod ./.agents/skills/tango/examples/webview-notes`. Go may add dependencies for the example to your `go.mod` and `go.sum`. The example embeds its static page and runs without npm. Use `-window-state` to choose where it saves window placement. Copy the complete example folder, including `page/`, to adapt it in your app. [WebView notes source](../examples/webview-notes/main.go)
