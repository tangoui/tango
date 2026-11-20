# CI & smoke tests

The CI workflow is in `.github/workflows/ci.yml`. CI and local runs share `scripts/smoke.sh` on macOS and Linux and `scripts/smoke.ps1` on Windows.

## Run locally

```bash
bash scripts/smoke.sh                # all desktop smoke programs on this platform
bash scripts/smoke.sh list           # list program names
bash scripts/smoke.sh desktop hello # run one program
bash scripts/smoke.sh ios            # run mobile-hello on an iPhone simulator
bash scripts/smoke.sh android        # build, install and launch on a connected device or emulator
pwsh scripts/smoke.ps1               # Windows
```

The scripts build each program, run it under a hard timeout and produce a nonzero exit code on a failure or timeout. The output directory has one `.log` per program, `summary.txt` and optional screenshots. Examples such as `hello`, `runtime-demo` and `showcase` also accept `-smoke`, for example `go run ./examples/hello -smoke`.

| Variable | Purpose | Default |
|---|---|---|
| `SMOKE_OUT` | Logs, screenshots and summary | `build/smoke` |
| `SMOKE_TIMEOUT` | Hard timeout per desktop program, in seconds | 120 |
| `SMOKE_SCREENSHOT=1` | Capture the screen while running. The terminal needs Screen Recording permission on macOS | Off |
| `SMOKE_SHOT_DELAY` | Seconds from launch to screenshot | 2 |
| `TANGO_SMOKE_TIMEOUT_SCALE` | Scale the programs' own timeouts | 1 (CI uses 3) |
| `TANGO_SMOKE_SKIP` | Skip check groups such as `clipboard`, `hotkey`, `notify` and `shortcut` in notify-demo | None |
| `SMOKE_IOS_DEVICE` | Simulator name or UDID | First available iPhone |

## CI jobs

CI runs on pushes to `main` and `v*` tags, on pull requests, and on manual dispatch.

| Job | Runner | Coverage |
|---|---|---|
| `lint` | Ubuntu | gofmt, native and cross-platform vet, JS runtime tests and IIFE build check |
| `test` | Ubuntu / macOS / Windows | Production and `tango_dev` tests for the root module and tests for `cmd/tan`. macOS also exercises `tango_gotext`. Heavy rendering packages run serially |
| `gallery-contrast` | Ubuntu | Gallery text contrast across themes on main pushes and manual dispatch |
| `smoke` | Ubuntu / macOS / Windows | Real windows, with a virtual display on Linux |
| `mobile-ios` | macOS | Build and run mobile-hello on an iPhone simulator |
| `mobile-android` | Ubuntu | Build and upload the APK, then run it on an API 34 emulator. Set the repository variable `ANDROID_EMULATOR_REQUIRED=true` to require the emulator step to pass |

[Release workflow](https://github.com/tangoui/tango/blob/main/docs/release.md)

## CI for your app

- The default desktop build works with cgo disabled. One Linux runner can cross-compile with `tan build -platform darwin/arm64,windows/amd64,linux/amd64`. Making a DMG requires macOS.
- Give your app a `-smoke` mode that exits with a status based on a self-test. See `examples/showcase`.
- On Linux install GTK 3, WebKitGTK 4.1 and Xvfb, and run under `xvfb-run -a`. A headless session may lack a notification service.
