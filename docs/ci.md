# CI 与冒烟测试

CI 配置在 `.github/workflows/ci.yml`。桌面冒烟测试的入口 `scripts/smoke.sh`（macOS / Linux）和 `scripts/smoke.ps1`（Windows）本地和 CI 共用，所以 CI 上失败的步骤都能在本机用同一条命令复现。

## 本地运行冒烟

```bash
bash scripts/smoke.sh                     # 依次跑本平台全部桌面冒烟程序
bash scripts/smoke.sh list                # 列出程序名
bash scripts/smoke.sh desktop hello smokeext   # 只跑指定的几个
bash scripts/smoke.sh ios                 # 构建 examples/mobile-hello，在 iPhone 模拟器里运行
bash scripts/smoke.sh android             # 构建 APK，装到已连接的设备/模拟器上运行
bash scripts/smoke.sh ios-ui              # 自绘示例 examples/mobile-ui：首帧、截图、自检触摸（TANGO_MOBILEUI_SELFTEST）点到计数行
bash scripts/smoke.sh android-ui          # 同上，Android：首帧、截图、按无障碍树找到“点我计数”后 adb input tap，查 logcat 的 tap #1
```

```powershell
pwsh scripts/smoke.ps1                    # Windows（Git Bash 下 smoke.sh 会转交给它）
pwsh scripts/smoke.ps1 desktop hello
```

每个程序会弹出真实窗口几秒，然后自己退出。脚本先 `go build` 再运行，给每个程序设硬超时，最后打印汇总；有任何 FAIL / TIMEOUT / 构建失败，退出码非零。

| 平台 | 程序 |
|---|---|
| macOS | `internal/platform/darwin/internal/{smoke,smokeext,smoketray,smokecanvas -smoke}` |
| Linux | `internal/platform/linux/internal/smoke` |
| Windows | `internal/platform/windows/internal/smoke` |
| 全部 | `examples/hello -smoke`、`examples/app-demo -smoke`、`examples/system-demo`、`examples/notify-demo` |

输出在 `build/smoke/`（`SMOKE_OUT` 可改）：每个程序一个 `<名字>.log`，构建失败时有 `<名字>.build.log`，`summary.txt` 是汇总，开了截图时有 `<名字>.png`。darwin 的 smoke / smokecanvas 自己截的图（`/tmp/tango-*.png`）也会复制进来。

### 环境变量

| 变量 | 作用 | 默认 |
|---|---|---|
| `SMOKE_OUT` | 日志、截图、汇总的目录 | `build/smoke` |
| `SMOKE_TIMEOUT` | 每个程序的硬超时（秒），到时强杀并记为 TIMEOUT | 120 |
| `SMOKE_SCREENSHOT=1` | 程序运行中截一张整屏图 | 关（macOS 本机截屏需要终端有"屏幕录制"权限） |
| `SMOKE_SHOT_DELAY` | 启动后几秒截图 | 2 |
| `TANGO_SMOKE_TIMEOUT_SCALE` | 冒烟程序**自己的**超时/等待乘以这个倍数（≥1），给慢机器用；CI 设为 3 | 不设即 1，时序与原来一样 |
| `TANGO_SMOKE_SKIP` | notify-demo 跳过的检查组，逗号分隔：`clipboard`、`hotkey`、`notify`、`shortcut` | 不跳过 |
| `SMOKE_IOS_DEVICE` | 模拟器名称或 UDID | 第一个可用 iPhone |
| `SMOKE_MOBILE_WAIT` | 等移动端页面给 Go 发消息的秒数 | 90 |
| `TANGO_APP_ID` / `SMOKE_APK` | Android 的 applicationId / 直接安装这个 APK 而不构建 | `examples/mobile-hello/tan.json` 的 `id` / 现场构建 |

`TANGO_SMOKE_TIMEOUT_SCALE` 和 `TANGO_SMOKE_SKIP` 是冒烟程序本身读取的，不设时行为与默认完全一致。

移动端判定：mobile-hello 的页面加载后会 `post` 一条消息，Go 打出 `mobile-hello: page says ...`。iOS 用 `simctl launch --console-pty` 收集 Go 的输出，Android 看 `adb logcat` 的 `Tango` 标签；在时限内出现这行即 PASS，并截一张模拟器截图。

## CI 作业

触发：推送到 `main`、`v*` 标签、所有 PR、手动运行。

| 作业 | 运行环境 | 做什么 |
|---|---|---|
| `lint` | ubuntu | 对已跟踪的 `.go` 文件跑 `gofmt -l`；`go vet ./...`（根模块和 `cmd/tan`；根模块另跑一遍 `-tags tango_dev`）；在 `CGO_ENABLED=0` 下对 windows/linux/darwin × amd64/arm64 交叉 vet（默认与 `-tags tango_dev` 各一遍）；`runtime-js` 里 `npm ci && npm test`，再检查 `runtimejs/tango.iife.js` 与源码构建结果一致 |
| `test` | ubuntu / macos / windows | `go test -race ./...`（默认 = 生产构建）、`go test -race -tags tango_dev ./...`（开发功能，见 [dev-builds.md](dev-builds.md)）和 `cmd/tan` 的测试；Windows 上有 gcc 时用 `CGO_ENABLED=1 -race`，没有就不带 `-race` 并给出警告 |
| `smoke` | ubuntu / macos / windows | 真实窗口冒烟。Linux 先 apt 装 `libgtk-3-0 libwebkit2gtk-4.1-0 libayatana-appindicator3-1 xvfb` 等，用 `dbus-run-session -- xvfb-run -a bash scripts/smoke.sh` 运行（`TANGO_SMOKE_SKIP=notify`：无头会话没有通知服务）；macOS 直接 `bash scripts/smoke.sh`；Windows `pwsh scripts/smoke.ps1`（runner 自带 WebView2）。每个程序 180 秒硬超时，步骤 20 分钟超时 |
| `mobile-ios` | macos | `bash scripts/smoke.sh ios`：`mobile/ios/build.sh examples/mobile-hello` → 启动第一个可用的 iPhone 模拟器 → install / launch → 等页面消息 → 截图 |
| `mobile-android` | ubuntu | JDK 17 + runner 自带的 Android SDK/NDK（缺 NDK 时用 sdkmanager 装），`mobile/android/build.sh` 打 APK 并作为产物上传；然后用 `reactivecircus/android-emulator-runner` 起 API 34 x86_64 模拟器跑 `bash scripts/smoke.sh android` |

Android 模拟器步骤目前是 `continue-on-error`（模拟器在共享 runner 上不够稳定）：把仓库变量 `ANDROID_EMULATOR_REQUIRED` 设为 `true` 后它才会让作业失败。

### API 兼容性检查

`.github/workflows/api.yml` 单独成一个工作流，这样加减 `api-break` 标签时只会重跑这一项。触发条件：推送到 `main`、PR（含加减标签）、手动运行（可以指定对比基准）。它的 `apidiff` 作业先运行 `go run ./internal/cmd/apitxt -check`（黄金文件 `api/go/*.txt` 是否最新、每个包是否在 `api/tiers.txt` 里分了层、白名单条目是否存在、实验性包和 split 包的包文档是否写明分层、稳定符号是否引用了非稳定符号；任何一项不通过都失败），接着在 `runtime-js` 里 `npm ci`、`npm run api:check`（构建后以 CI 模式运行 API Extractor，`etc/runtime.api.md` 与源码不一致就失败）、`npm run contract:check` 和 `node --test test/api-report.test.mjs test/contract.test.mjs`，并在仓库根运行 `go test ./jsapi -run '^TestWireContract'`（线上方法名、事件名、错误码与 `api/jsapi-methods.txt` 一致，见 [runtime-api.md 的「API 稳定性」](runtime-api.md#api-稳定性)），再运行 `bash scripts/apidiff.sh <基准>`，最后检查 `api/go/**`、`api/jsapi-methods.txt` 或 `runtime-js/etc/runtime.api.md` 有变化时 `CHANGELOG.md` 是否也有变化（第一周只警告，PR 可用 `no-changelog` 标签豁免）。apidiff 的基准：PR 的基准是目标分支的最新提交，推送的基准是推送前的 `main`。脚本按固定版本安装 `golang.org/x/exp/cmd/apidiff`，在 linux、darwin、windows 上（`CGO_ENABLED=0`）各导出一次两边的 API，合并后列出不兼容变化，结果写进作业 Summary。

稳定层（稳定包，以及 split 包里列入 `api/stable/<包>.txt` 白名单的符号）有不兼容变化时作业失败，实验层只给警告；分层和 `api-break` 流程见 [release.md 的「API 稳定性」](release.md#api-稳定性)。本地复现：`bash scripts/apidiff.sh origin/main`（对比的是工作区，含未提交的改动）。可选环境变量：`ALLOW_API_BREAK=1`（只报告不失败）、`APIDIFF`（指定 apidiff 可执行文件）、`APIDIFF_GOOS`（要检查的平台，默认 `linux darwin windows`）。

## 看失败产物

1. 打开失败的运行 → 失败作业的日志：冒烟脚本把每个程序的输出缩进打印在 `==> run <名字>` 下面，结尾有汇总；作业的 Summary 页也有同一份汇总。
2. 运行页底部 **Artifacts**：
   - `smoke-<os>`：`build/smoke/` 下的 `*.log`、`*.png`、`summary.txt`（不含二进制）；
   - `mobile-ios`、`mobile-android`：构建日志、运行日志（Android 为完整 logcat）、模拟器截图；
   - `mobile-hello-apk`：每次都上传的 debug APK。
   失败产物保留 14 天。
3. 本地复现：用同样的变量跑，例如 `TANGO_SMOKE_TIMEOUT_SCALE=3 bash scripts/smoke.sh desktop smokeext`。

## 以后要加的

- 发布由 `.github/workflows/release.yml` 负责（见 `docs/release.md`），ci.yml 不再包含发布作业。签名：macOS 用 `tan sign`（证书和公证凭据放 Secrets），Windows 用 Authenticode；Android release 需要 keystore（见 `docs/mobile.md`）。
- 新的冒烟程序：能自动退出、用退出码表示结果、默认不等待用户操作；然后加进 `scripts/smoke.sh` 的 `desktop_programs` 和 `scripts/smoke.ps1` 的 `$programs`。
