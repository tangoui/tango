# Build and package

- Inspect `tan.json` for the application ID, display name, version, icon, frontend build command, and platform overrides. Keep a stable ID for window state, app data, and notification permissions. [tan.json reference](https://tangoui.github.io/tango/reference/tan-json)
- Run `tan build` for the host platform. Use `tan build -platform <os/arch>` and `tan build -h` for other targets and prerequisites. On macOS the default artifact is an ad-hoc signed `.app` in `build/darwin-<arch>/`.
- Use `tan package` for installers or archives and `tan sign` for signing and macOS notarization. Review each command's help and the platform prerequisites before building distribution artifacts. [Packaging guide](https://tangoui.github.io/tango/guide/packaging)
- Exercise the installed artifact for OS features such as notification permission. Keep test data in a disposable location supported by the application. [Verification](verify.md)
