# 开发功能与生产构建

Tango 只有一个构建开关：`tango_dev` 构建标签。

| | 默认构建（生产） | `-tags tango_dev`（开发） |
| --- | --- | --- |
| 怎么得到 | `go build`、`go run .`、`go install`、goreleaser、`tan build`（含 `-debug`）、`tan package`、`tan run` | `tan dev`（自动加），或手动 `go build -tags tango_dev` / `go test -tags tango_dev` |
| 前端资源 | `asset.FromEnv` 只服务嵌入的文件，**忽略 `TANGO_DEV_URL`** | `TANGO_DEV_URL` 指向的 dev server（只接受回环地址） |
| 自动化通道（`automation` 包） | 不链接；`TANGO_AUTOMATION*` 无效 | 设置 `TANGO_AUTOMATION` 时开启 |
| 自绘 UI 热加载（devhost） | 不链接；`TANGO_DEV_ROLE`、`TANGO_DEV_ADDR` 等无效 | `tan dev` 下工作 |
| 窗口位置恢复（`TANGO_DEV_WINDOW_STATE`） | 无效 | `tan dev` 重启应用后恢复窗口位置 |
| 未处理的 panic（`Options.OnPanic` 为 nil） | 弹错误框后以退出码 2 退出 | 记日志、继续运行 |
| `i18n` 缺失 key 警告、`env.Info().Debug` | 关闭 / `false` | 打开 / `true` |
| GPU 渲染校验（`internal/render/gpu` verify 模式）默认值 | 见 `internal/render/gpu/verify_release.go` | 打开 |
| IPC 快速通道的测试开关（`TANGO_IPC_LANE_FAULT`；macOS/iOS 另有 `TANGO_SCHEME_PROBE` 的探测路由和通道计数器、`TANGO_IPC_LANES_DARWIN=all`） | 无效（`all` 等同默认的开启值） | 生效 |
| iOS 模拟器自测钩子（`TANGO_SCHEME_PROBE`、`TANGO_IPC_LANES_DARWIN=all`、`TANGO_CANVAS_IME_SELFTEST`、`TANGO_CANVAS_A11Y_SELFTEST`）和通道故障注入 `TANGO_IPC_LANE_FAULT`（所有平台） | 无效；`-tags tango_selftest` 时生效（见下文） | 生效 |

**直接 `go run .` 或 `go build` 得到的就是生产行为。** 热加载、自动化、前端开发代理这些开发功能只能通过 `tan dev` 或手动加 `-tags tango_dev` 使用。

## 为什么是编译期开关

开发功能都由环境变量或本机端口触发。与应用同一用户的任何进程都能控制这些输入：`open -a App --env TANGO_DEV_URL=http://evil`、`launchctl setenv`、Windows 的用户环境变量、抢占本机端口。若发布版也响应它们，攻击者就能让一个已签名、已拿到 TCC 授权和钥匙串访问的应用加载自己的页面（页面可调用全部绑定），或通过自动化通道远程执行 JS。运行时检查挡不住同用户的攻击者，所以这些代码在默认构建里根本不编译进去：`go list -deps` 看不到 `internal/automation`、`internal/devhost`（`automation` 包的 `TestReleaseBuildHasNoChannel` 检查），二进制里也没有 `asset.DevProxy`。

默认构建仍会读取几个面向用户排障的变量：`TANGO_RENDERER`（自绘窗口渲染器）、`TANGO_TEXT_ENGINE`（文字引擎）、`TANGO_LINUX_TRAY`、`TANGO_ATSPI_EMBED`、`TANGO_WINDOWS_TOAST=0`（Windows 通知回退到气泡）。它们只在已有实现之间切换，不会加载外部代码或打开通道。

iOS 模拟器自测要用这些钩子，但 `tan build`（`mobile/ios/build.sh` 也走它）会去掉 `tango_dev`。所以 iOS 另有一个只编进测试钩子的标签 `tango_selftest`：`tan build` 保留 `GOFLAGS` 里除 `tango_dev` 以外的标签，`GOFLAGS=-tags=tango_selftest bash mobile/ios/build.sh <应用> simulator` 构建出带钩子的 .app（`scripts/smoke.sh ios-probe`、`ios-selftest` 就这样构建）。`tango_selftest` 不链接自动化通道、热加载或开发代理，只打开上表那几个自测钩子。不带这两个标签的 iOS 构建不读这些变量，桥接代码里的探测路由也不编译进去。快速通道的关闭开关（`TANGO_IPC_LANES_DARWIN=0`/`false`/`off`、`TANGO_IPC_LANES=0`）在所有构建里都生效。

## `tango_release` 已废弃

`tango_release` 以前是"发布构建"的标签；默认构建现在就是生产构建，它**不再起任何作用**，只作为兼容别名保留：`tan build` 仍会传它，手写的 `-tags tango_release` 也不会出错。`-tags tango_release,tango_dev` 等同于 `-tags tango_dev`。

## 开发与测试

- `tan dev`：桌面、`-platform android|ios-simulator` 的构建都带 `-tags tango_dev`（与 `GOFLAGS` 里的 `-tags` 合并）。`tan build`、`tan build -debug` 去掉 `tango_dev`，保持生产语义。
- 测试有两种状态，都要通过：

  ```bash
  go test ./...                    # 生产构建：开发功能不生效
  go test -tags tango_dev ./...    # 开发构建：热加载、自动化、开发代理等的测试
  ```

  依赖开发功能的测试文件带 `//go:build tango_dev`；验证生产行为的测试带 `//go:build !tango_dev`。
- `automation.Launch` 构建包时自动加 `-tags tango_dev`；传入已有可执行文件时，它必须用 `-tags tango_dev` 构建，否则 Launch 报错"has no automation channel: build it with -tags tango_dev"。端到端测试：`TANGO_E2E=1 go test -tags tango_dev -run E2E ./examples/ledger`。
- 手动调试 WebView 前端而不用 `tan dev`：`TANGO_DEV_URL=http://localhost:5173 go run -tags tango_dev .`。

## 开发构建里仍有的限制

- `asset.DevProxy` 默认只接受回环目标：`localhost`（及 `*.localhost`）、`127.0.0.0/8`、`::1`。Android 的 `adb reverse` 与 iOS 模拟器访问宿主都是 localhost，满足这个限制。要代理到局域网里的 dev server，在自己的代码里用 `asset.DevProxyWith(url, asset.DevProxyOptions{AllowRemote: true})`；`FromEnv` 不提供这个选项。
- `tan dev` 热加载的控制通道（Windows 上是 `127.0.0.1` 的 TCP 端口，其他平台是 0700 目录里 0600 的 unix socket）要求每个连接在第一帧带上本次会话的随机 token（`TANGO_DEV_TOKEN`，常量时间比较），其他本机用户连不进来。

## 与生产构建都有关的：资源源的跨源写请求

`Options.Assets` 不知道请求来自哪个页面，外站页面（用户点链接打开的、iframe 里的）可以向 `<资源源>/api` 发表单 POST。核心在请求到达 `Options.Assets` 之前检查：GET、HEAD 总是放行；其他方法的 `Origin` 必须是资源源本身（没有 `Origin` 头的请求放行），否则回 403。WebKit（macOS、iOS）对自定义 scheme 请求不带 `Origin`，后端用主文档 URL（`NSURLRequest.mainDocumentURL`）补上；Windows（WebView2）与 Android 的资源源是 `https://tango.localhost`，由 Chromium 内核按规范带 `Origin`（尚未在这两个平台上实测）。Linux（WebKitGTK）暂未补，请求不带 `Origin` 时放行。确实需要跨源写请求（自己做 CORS 或 token 校验）时设置 `tango.Options.AllowCrossOriginAssets`。

已知限制：补上的是主文档的来源，后端分不清请求来自哪个 frame，所以应用页面里嵌的跨源 iframe 发往资源源的写请求也能通过这项检查（IPC 快速通道的请求另有每世代令牌保护，不受影响）。不要在资源页面里嵌不可信的 frame。
