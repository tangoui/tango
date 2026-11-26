# Runtime pitfalls

- After `Item.SetLabel` or `Item.SetChecked`, call `Menu.Update()` on the installed root menu. This applies to the tray menu too.
- Wrap dynamic native UI rows in `ui.Key` with a stable record ID. Deleting or moving rows can otherwise move focus and local component state to another record.
- Use `Runner.Post` to change native UI state from a worker and `Context.WakeAt` to request a future frame. `App.Post` schedules work on the platform thread without requesting a Go-drawn UI frame.
- Capture `uihost.Open` as `runner, surface, err`. Call `ui.Checkbox(c, label, &value)` with the label before the pointer.
- Keep `@tangoui/runtime` and the Go module on the same Tango release. Run `tan generate bindings` after Go API changes if using generated clients. Build frontend assets before running a direct `go build` of a project that embeds `frontend/dist`.
- Give macOS a first submenu for the application and an Edit submenu with the `menu.EditMenu` role for standard WebView clipboard shortcuts. Use monochrome transparent PNG tray artwork with `tray.Options.Template` enabled on macOS.
- Verify macOS notifications in a bundle with a `CFBundleIdentifier`. On macOS 26.6, a temporary `/tmp` bundle can fail with `Notifications are not allowed for this application` even with valid ad-hoc signing. Quit that copy, move the entire `.app` to `~/Applications` or `/Applications`, launch it, and request permission again. If prompted previously, check System Settings → Notifications. [Tango notification guide](https://tangoui.github.io/tango/guide/system/notify)
- The tango module's `AGENTS.md` applies to framework contributors. For app work, use this skill, your project's own instructions, the generated project, and the documentation for the installed module.
