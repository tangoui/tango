# tan CLI

`tan` creates, runs, builds and ships TanGo apps. `dev`, `build`, `run`, `package`, `release-manifest` and `applinks` read the project configuration from the current directory. Commands that accept `-C <dir>` can use it to select the project directory. `sign`, `generate bindings` and `doctor` can also run outside a project. [tan.json](/en/reference/tan-json)

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan help              # list of commands
tan <command> -h      # flags of one command
```

This page describes the CLI on the main branch. Install from source for features that have yet to appear in a tagged release. [Install tan](/en/guide/getting-started#install-tan)

## Commands

| Command | What it does |
|---|---|
| `tan init [dir]` | create a project from a template; asks a few questions in a terminal when no directory is given, `-yes` takes the defaults |
| `tan dev` | development mode: starts the front-end dev server (if any), builds and runs the app, rebuilds on Go changes, hot-reloads self-drawn windows |
| `tan build` | release build for one or more platforms: desktop, iOS, Android |
| `tan run` | build, install and run on the iOS Simulator or an Android device/emulator, with logs in the terminal |
| `tan package` | build, then make installers and archives: macOS `.dmg`, Windows NSIS or MSIX, Linux `.tar.gz`, AppImage, deb, rpm or Flatpak |
| `tan sign` | macOS code signing and notarization; Authenticode for Windows exe / msi / msix |
| `tan keygen` | create an Ed25519 key pair for update signing |
| `tan release-manifest` | make a release's update files, binary patches and signed `latest.json` |
| `tan sign-update` | sign an update manifest with the Ed25519 private key |
| `tan applinks` | generate iOS Universal Links and Android App Links verification files to publish under `/.well-known/` |
| `tan generate bindings` | TypeScript clients for services registered with `bind.Register` |
| `tan doctor` | check the toolchain and platform dependencies |
| `tan mcp` | provide project tools and UI automation for development builds to AI agents |
| `tan version` | print the version |

## Platform targets

`tan build -platform` takes the comma-separated targets below. `tan package` accepts desktop targets only.

| Form | Meaning |
|---|---|
| (empty) | this machine |
| `darwin/arm64`, `windows/amd64`, `linux/arm64` | desktop OS/arch; an OS alone means this machine's architecture |
| `darwin/universal` | both Mac architectures joined with `lipo` |
| `ios-simulator` / `ios` | iOS simulator `.app` / device `.app` + `.ipa` |
| `android`, `android/arm64,android/amd64` | Android; `android` alone means arm64 + amd64 |

Desktop output goes to `build/<os>-<arch>/` (`-o` changes the root); `-debug` keeps symbols and allows the web inspector (a debug APK on Android). `tan build` (`-debug` included) makes production builds: any `-tags` already in `GOFLAGS` are kept, minus `tango_dev` (plus `tango_release`, a compatibility tag that no longer does anything). `tan dev` builds (desktop and mobile) add `-tags tango_dev`, which links the development features: hot reload, automation, the front-end dev proxy.

`tan build` and `tan package` accept `-features` to override the project's desktop backend profile. See the scope of `desktopBuildTags` and `cgo` in [tan.json](/en/reference/tan-json#desktop-builds).

## MCP server

`tan mcp -root .` serves MCP over stdio by default. `-root` defaults to the current directory and bounds project paths passed to the tools. For HTTP, pass `-http`. Use `-addr` to set a loopback address (default `127.0.0.1:0`) and `-token` to set the Bearer token (by default tan generates a random token and prints it to stderr). `-addr` and `-token` require `-http`. `-stdio` and `-http` are mutually exclusive. UI tools require the app's `tango_dev` control channel. [Tools and client configuration](https://github.com/tangoui/tango/blob/main/docs/mcp.md)

## A typical session

```bash
tan init -template react-ts notes && cd notes
tan dev
tan generate bindings
tan build -platform darwin/universal
tan package -platform darwin/universal
tan sign -notarize build/darwin-universal/Notes.app
tan run -platform ios-simulator
tan build -platform android/arm64,android/amd64
```

## Environment variables

| Variable | Used by |
|---|---|
| `TANGO_DEV_URL` | set by `tan dev` for the app (read by `asset.FromEnv` only in `-tags tango_dev` builds, loopback addresses only) |
| `TANGO_HOTRELOAD=0` | turn off hot reload of self-drawn windows |
| `TANGO_LAUNCH_BACKGROUND=1` | on macOS, launch without activating the app and place its windows behind other apps. `tan mcp` sets this when it launches an app |
| `TANGO_SIGN_IDENTITY`, `TANGO_NOTARY_PROFILE`, `TANGO_APPLE_ID`, `TANGO_APPLE_TEAM_ID`, `TANGO_APPLE_PASSWORD` | macOS signing and notarization |
| `TANGO_WINDOWS_CERT`, `TANGO_WINDOWS_CERT_PASSWORD` | Windows signing certificate and password |
| `TANGO_UPDATE_KEY_FILE`, `TANGO_UPDATE_KEY` | private key for `tan release-manifest` and `tan sign-update` |
| `TANGO_IOS_SIGN_IDENTITY` | iOS device signing identity |
| `TANGO_ANDROID_KEYSTORE_PASSWORD`, `TANGO_ANDROID_KEY_PASSWORD` | Android release signing |
| `JAVA_HOME`, `ANDROID_HOME`, `ANDROID_NDK_HOME` | Android builds |

## All flags

Generated from `tan <command> -h` when the site is built.

<!--@include: ../../reference/_cli-flags.md-->
