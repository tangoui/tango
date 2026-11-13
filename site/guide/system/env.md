# env：运行环境信息

`env` 返回应用所处环境的一份快照：操作系统及版本、CPU 架构、是否调试构建，以及页面所用的 WebView 引擎和版本。常用于「关于」窗口、诊断日志、按平台调整行为。

```go
import "github.com/tangoui/tango/sys/env"
```

## 用法

```go
app.OnReady(func() {
	in := env.Info(app)
	log.Printf("%s %s (%s/%s) webview=%s %s debug=%v",
		in.OSName, in.OSVersion, in.OS, in.Arch, in.WebView, in.WebViewVersion, in.Debug)
})
```

不需要 WebView 信息时可以传 nil，随时调用：

```go
in := env.Info(nil) // WebView、WebViewVersion 为空
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Info(a) Environment` | 返回环境快照；`a` 可为 nil |
| `Environment.OS` / `Arch` | `runtime.GOOS` / `runtime.GOARCH` |
| `Environment.OSName` | 例如 `"macOS"`、`"Windows"`、`"Ubuntu 24.04.1 LTS"` |
| `Environment.OSVersion` | 例如 `"15.3.1"`、`"10.0.26100"`；Linux 上是内核版本 |
| `Environment.Debug` | 用 `-tags tango_dev` 构建（`tan dev`）时为 true，默认构建为 false |
| `Environment.WebView` / `WebViewVersion` | `"WebKit"` / `"WebView2"` / `"WebKitGTK"` 及版本，后端不报告时为空 |

完整签名见 [API 参考](/reference/api/sys-env)。

## 平台支持

| 字段 | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| OS / Arch / Debug | ✓ | ✓ | ✓ | ✓ | ✓ |
| OSName / OSVersion | ✓ | ✓ | ✓ | ✓ | ✓（OSName 为 `"Android"`，OSVersion 是内核版本） |
| WebView / WebViewVersion | ✓ | ✓ | ✓ | 空 | 空 |

> Windows、Linux 实现尚未实机验证；移动端仅在模拟器上验证，属实验性。

## 在页面里调用

对应 `@tangoui/runtime` 的 `App.info()`，属于默认开启的 `Info` 能力，不需要额外配置：

```ts
import { App } from "@tangoui/runtime";

const e = await App.info();
// { os, arch, osName, osVersion, debug, webView, webViewVersion }
console.log(e.osName, e.webView);
```

见 [JS API](/reference/js-api)。

## 注意

- `Info` 可在任意 goroutine 调用，但传入 app 时要等应用运行起来：它像 `tango.Await` 一样会等到 `Run`，在主 goroutine 上提前调用会 panic。放在 `OnReady` 里或之后的 goroutine 里最稳妥。
- 应用退出后 WebView 字段为空。
- 默认构建（`go build`、`go run`、`tan build`）是生产构建，`Debug` 为 false；`tan dev` 加 `-tags tango_dev`，`Debug` 为 true。`tango_release` 标签已无作用。见 [打包](/guide/packaging)。
