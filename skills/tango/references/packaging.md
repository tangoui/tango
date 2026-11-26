# Build and package

- Inspect `tan.json` for the application ID, display name, version, icon, frontend build command, and platform overrides. Keep a stable ID for window state, app data, and notification permissions. [tan.json reference](https://tangoui.github.io/tango/reference/tan-json)
- Set `name` to a display name with at least one ASCII letter or digit. Tango uses the name for the application package and filters it to ASCII letters, digits, hyphens, and underscores for the executable filename. macOS uses the packaged name for the first menu label. With a bare executable, that label can be the process name, such as `app` under MCP. Go's `tango.Options.Name` configures the running app independently of packaging.
- Run `tan build` for the host platform. Use `tan build -platform <os/arch>` and `tan build -h` for other targets and prerequisites. On macOS the default artifact is an ad-hoc signed `.app` in `build/darwin-<arch>/`.
- Use `tan package` for installers or archives and `tan sign` for signing and macOS notarization. Review each command's help and the platform prerequisites before building distribution artifacts. [Packaging guide](https://tangoui.github.io/tango/guide/packaging)
- Exercise the installed artifact for OS features such as notification permission. Keep test data in a disposable location supported by the application. [Verification](verify.md)
