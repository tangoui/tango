# WebView 高级能力

权限请求、下载、Cookie 与网站数据、HTTP 认证、页内查找、User-Agent 与额外请求头、新窗口请求、导航策略、打印 PDF 与截图。

- 契约：`internal/platform/webview_ext.go`（契约第四轮，全部是可选接口，靠类型断言发现）。
- Go 公开 API：根包 `webview.go`（`App` / `Surface` 的方法）。
- 页面侧：`jsapi` 的 `Web*` 能力 + `@tangoui/runtime` 的 `Web` 模块，默认全部关闭，见 [runtime-api.md](runtime-api.md) 的 Web 一节。
- 示例与冒烟：`examples/webview-demo`（`go run ./examples/webview-demo -smoke`，已加入 `scripts/smoke.sh` 的 macOS 列表）。

## 决策回调：Surface → App → 默认

权限、下载位置、HTTP 认证、新窗口、导航这几类“要 Go 拿主意”的事件，都按同一顺序决定：先问 Surface 自己的处理函数，它不表态（返回零值）时问 App 级处理函数，最后用内置默认。处理函数在主线程运行，页面在等它，要尽快返回；桌面上可以在里面弹对话框（对话框函数会继续泵事件）。

```go
// 权限：默认策略可配置，只作用于本应用页面。没有任何策略时交给平台（询问用户）；外站请求默认拒绝。
app.SetPermissionPolicy("", tango.PermissionDeny)                // 默认拒绝一切
app.SetPermissionPolicy(tango.WebPermMicrophone, tango.PermissionPrompt)
s.SetPermissionHandler(func(r tango.PermissionRequest) tango.PermissionDecision {
	if r.Origin == "https://meet.example.com" { return tango.PermissionAllow }
	return tango.PermissionDefault // 交给 App 级处理函数 / 策略
})

// 下载：返回保存路径，"" 取消。默认（无处理函数）只保存本应用页面发起的下载，存到 App.DownloadDir()（用户的 Downloads 目录），重名自动加 " (1)"；外站页面发起的下载取消。
s.SetDownloadHandler(func(d tango.DownloadRequest) string { return filepath.Join(dir, d.Filename) })
s.OnDownloadEvent(func(ev tango.DownloadEvent) { log.Println(ev.State, ev.Received, ev.Total) })

// HTTP 认证（Basic / Digest / NTLM / Negotiate）
app.SetAuthHandler(func(s *tango.Surface, c tango.AuthChallenge) tango.AuthResponse {
	return tango.AuthResponse{Action: tango.AuthCredentials, User: "u", Password: "p"}
})

// 新窗口（window.open、target=_blank）
s.SetNewWindowHandler(func(r tango.NewWindowRequest) tango.NewWindowAction { return tango.NewWindowBrowser })

// 导航策略：返回 false 取消，返回 true 在本窗口打开。应用自己发起的导航（SurfaceOptions.URL、Navigate）不经过它。
// 设置了策略就由策略全权决定；没有策略时外站 http(s) 链接交给系统浏览器（见“默认安全策略”）。
app.SetNavigationPolicy(func(s *tango.Surface, n tango.NavigationRequest) bool {
	return strings.HasPrefix(n.URL, "tango://") || strings.HasPrefix(n.URL, "https://tango.localhost")
})
```

| 事件 | 处理函数返回 | 不表态的值 | 内置默认 |
|---|---|---|---|
| 权限请求 | `PermissionAllow` / `Deny` / `Prompt` | `PermissionDefault` | 本应用页面：`SetPermissionPolicy` 按类型合并：任一拒绝即拒绝，全部允许才允许，否则询问；无策略 = 询问。外站：拒绝 |
| 下载 | 保存路径（相对路径相对 `DownloadDir`），`""` 取消 | —（Surface 有处理函数就由它全权决定，其次 App 级） | 本应用页面发起：`App.DownloadPath(req)`；外站页面发起：取消 |
| HTTP 认证 | `AuthCredentials` / `AuthCancel` | `AuthDefault` | 平台默认（通常直接 401） |
| 新窗口 | `NewWindowBrowser` / `Surface` / `Current` / `Deny` | `NewWindowDefault` | 本应用页面（资源 origin 或该 Surface 导航到的 origin）开新 Surface；http/https/mailto 用系统浏览器；其它协议拒绝 |
| 导航 | `bool` | —（设置了任一级策略就由策略决定，Surface 与 App 两级都要允许） | 无策略时：外站 http(s) 取消并交给系统浏览器；本应用页面与其它协议允许 |

“本应用页面”（可信来源）与桥接消息的判定相同：资源 origin（`Options.Assets`），或应用用 `SurfaceOptions.URL` / `Surface.Navigate` 把该 Surface 导航到的 origin。其余都算外站。

## 默认安全策略

下面三条默认行为比早先版本收紧了；API 没有变化，每条都可以用已有的处理函数显式覆盖。

| 场景 | 新默认 | 旧默认 | 恢复旧行为 / 显式放行 |
|---|---|---|---|
| 顶层导航到外站 http(s)（点链接、`location.href=`、重定向） | 取消导航，URL 交给系统浏览器打开（同 `NewWindowBrowser`） | 在本窗口打开 | 设置任一导航策略，返回 `true` 即在本窗口打开：`app.SetNavigationPolicy(func(*tango.Surface, tango.NavigationRequest) bool { return true })`；只放行部分站点就按 URL 判断 |
| 外站页面发起的下载 | 取消（不再静默写入下载目录） | 存到 `DownloadPath` | 设置下载处理函数，它对所有来源全权决定：`app.SetDownloadHandler(func(_ *tango.Surface, d tango.DownloadRequest) string { return app.DownloadPath(d) })` |
| 外站的权限请求 | 处理函数不表态时拒绝；`SetPermissionPolicy` 只作用于本应用页面 | 按 `SetPermissionPolicy` / 询问 | 在权限处理函数里按 `PermissionRequest.Origin` 显式返回 `PermissionAllow` / `PermissionPrompt` |

细节：

- 只有没有任何导航策略（Surface 级与 App 级都没设）时才套用导航默认；应用自己发起的导航（`SurfaceOptions.URL`、`Navigate`、`NewWindowCurrent`）不受影响。非 http(s) 的导航（`about:`、`data:`、`blob:`、`file:` 等）照旧允许，需要限制时自己设策略。
- 链接到外站文件的下载会先作为导航被问到，默认交给系统浏览器下载；要在应用内下载，用导航策略放行该 URL。
- 下载来源按发起时窗口里显示的页面判定（平台不报告具体发起的 frame）；本应用页面里嵌的外站 iframe 发起的下载按本应用页面算。
- 权限请求的来源取平台报告的请求 frame 的 origin；平台不报告（macOS 旧系统）时按当前页面判定。
- Windows 拖放（`AcceptFiles`）：带完整路径的拖放消息只接受本应用页面发出的；旧版 WebView2 运行时把拖入的文件变成 `file:` 导航，只有在本应用页面报告的真实拖拽会话（受信任的 `dragenter`/`dragover` 事件）期间才当作拖放，其余 `file:` 导航走普通导航策略。

说明：

- 后端总是阻止平台自己的弹窗，页面的 `window.open` 返回 `null`（Android 例外，见下）；`NewWindowSurface` 新建的窗口继承打开者的 `Debug` 和 `WebView` 选项。WebKit 与 Android 的弹窗拦截仍然有效：没有用户手势的 `window.open` 不会到达 Go。
- 主框架导航变成下载时，原页面留在原处；核心会探测它是否还活着，消息照常发给它。
- 下载事件：每个被接受的下载最后恰好有一个 `DownloadDone` / `DownloadFailed` / `DownloadCanceled`；进度事件由后端节流（约 250ms）。`Surface.CancelDownload(id)` 取消。
- macOS 上来自资源 origin（`tango://app`）的下载：WebKit 不能把自定义协议的响应转成 `WKDownload`，后端取消该导航、在后台直接调用资源处理器写文件，进度与事件照常。

## 控制类方法

阻塞调用方直到平台返回；平台做不到返回 `ErrUnsupported`，窗口已关闭返回 `ErrSurfaceClosed`。Android 主线程不能嵌套等待，在那里调用返回 `ErrNestedWait`，请在其它 goroutine 调。

| 方法 | 说明 |
|---|---|
| `Find(text, FindOptions{MatchCase, Backward, NoWrap}) (FindResult, error)` | 高亮第一个匹配；`FindResult{Matches, Active, Found}`，`Matches` 为 -1 表示平台不计数，`Active` 从 1 开始，0 表示未知 |
| `FindNext()` / `FindPrevious()` / `StopFind()` | 下一个 / 上一个 / 结束并清除高亮 |
| `Cookies(url) ([]*http.Cookie, error)` | 会发往 url 的 Cookie；`""` 为全部。Cookie 存储所有 Surface 共享 |
| `SetCookie(c)` / `DeleteCookie(c)` / `ClearCookies(domain)` | `Domain` 必填（`example.com` 仅主机，`.example.com` 含子域）；`ClearCookies("")` 清空全部 |
| `ClearBrowsingData(DataCookies \| DataCache \| DataStorage)` | 清除所有站点的数据（`DataAll` 全部） |
| `SetUserAgent(ua)` / `UserAgent()` | 运行时修改；创建时用 `SurfaceOptions.WebView.UserAgent`（第一个请求前生效） |
| `SetExtraHeaders(h, scope)` | `HeadersAssets`：只加到本应用资源请求（核心实现，全平台）；`HeadersAll`：所有请求（仅 Windows，其它平台 `ErrUnsupported`，但仍加到资源请求）。创建时用 `SurfaceOptions.WebView.Headers/HeaderScope` |
| `PrintToPDF(PDFOptions) ([]byte, error)` | 当前页面的 PDF |
| `Screenshot() ([]byte, error)` | 可见区域的 PNG |
| `CancelDownload(id)` | 取消下载 |

## 支持矩阵

✓ 支持；部分 = 有限制（见备注）；— 不支持（`ErrUnsupported` / 事件不触发）。只有 macOS 实机冒烟过，其余平台目前只做了交叉编译与 vet。

| 能力 | macOS | Windows（WebView2） | Linux（WebKitGTK 4.1） | iOS | Android |
|---|---|---|---|---|---|
| 权限请求回调 | 部分：摄像头/麦克风（12+） | ✓ | ✓ 摄像头/麦克风/定位/通知/屏幕(2.34)/剪贴板(2.42) 等 | 部分：摄像头/麦克风/运动传感器（15+） | ✓ 摄像头/麦克风/定位/受保护媒体/MIDI |
| `PermissionPrompt` 的表现 | WebKit 系统提示 | 运行时自带提示 | 后端弹 GTK 对话框 | WebKit 系统提示 | 申请系统运行时权限；受保护媒体/MIDI 无系统提示，直接拒绝 |
| 导航策略 | ✓ | ✓ | 部分：iframe 导航也会被问到 | ✓ | 部分：`loadUrl` 与表单 POST 不回调 |
| 新窗口请求 | ✓（含窗口尺寸） | ✓ | ✓（无尺寸） | ✓ | 部分：`window.open` 页面拿到的不是 `null`，无尺寸 |
| HTTP 认证 | ✓ Basic/Digest/NTLM/Negotiate | 部分：仅 Basic，`Retry`/`Proxy` 不准 | ✓ | ✓ | 部分：无 URL、scheme、代理信息 |
| 下载（回调、进度、完成、取消） | ✓（11.3+） | ✓ | ✓（异步给路径在旧版 WebKitGTK 上可能不生效） | ✓（14.5+） | ✓（后端自己下载；POST、blob: 不支持） |
| Cookie 读/写/删/按域清 | ✓ | ✓ | 部分：列全部与按域清需 2.42 | ✓ | 部分：只有 name/value；不能列全部；按域清只覆盖根路径 |
| 清除缓存/存储 | ✓ | ✓ | ✓ | ✓ | ✓ |
| User-Agent（创建时/运行时） | ✓ | ✓ | ✓ | ✓ | ✓ |
| 额外请求头：资源 origin | ✓ | ✓ | ✓ | ✓ | ✓ |
| 额外请求头：所有请求 | — | ✓（worker 请求除外） | — | — | — |
| 页内查找 | 部分：11+；计数按页面文本统计，`Active` 按查找顺序推算 | 部分：运行时 1.0.3405+，旧版 `ErrUnsupported` | 部分：`Active` 恒为 0 | 部分：计数按页面文本统计，`Active` 恒为 0 | 部分：不支持 `MatchCase` |
| 打印到 PDF | 部分：11+，整页一张，选项忽略 | ✓（arm64 忽略纸张尺寸） | 部分：经 “Print to File” 打印机 | 部分：14+，整页一张；`NoBackground` 需 17+ | — |
| 截图 PNG | ✓ | ✓ | ✓ | ✓ | ✓ |

## 平台实现要点

- **macOS / iOS**：`WKUIDelegate`（`requestMediaCapturePermissionForOrigin`、`createWebViewWithConfiguration` 返回 nil）、`WKNavigationDelegate`（`decidePolicyForNavigationAction/Response`、`didBecomeDownload`、`didReceiveAuthenticationChallenge`）、`WKDownloadDelegate`、`WKHTTPCookieStore`、`WKWebsiteDataStore`、`findString:withConfiguration:`、`createPDFWithConfiguration`、`takeSnapshotWithConfiguration`。WebKit 没有公开的匹配计数，计数在页面 `innerText` 里统计。
- **Windows**：`PermissionRequested`、`NavigationStarting`、`NewWindowRequested`（总是 `Handled`）、`BasicAuthenticationRequested`、`DownloadStarting` + `DownloadOperation`、`CookieManager`、`Profile.ClearBrowsingData`、`Settings.UserAgent`、`WebResourceRequested` 改请求头、`PrintToPdf`、`CapturePreview`、`ICoreWebView2Find`（取不到接口时降级为 `ErrUnsupported`）。
- **Linux**：`permission-request`、`decide-policy`、`create`、`authenticate`、`download-started`、`WebKitCookieManager`、`WebKitWebsiteDataManager`、`WebKitFindController`、`WebKitPrintOperation`、`webkit_web_view_get_snapshot`；运行时缺少的符号用 `dlsym` 探测，缺了对应能力就返回 `ErrUnsupported`。
- **Android**：`WebChromeClient.onPermissionRequest` / `onGeolocationPermissionsShowPrompt` / `onCreateWindow`、`WebViewClient.shouldOverrideUrlLoading` / `onReceivedHttpAuthRequest`、`DownloadListener`（后端用 `HttpURLConnection` 带 Cookie/UA 下载到 Go 给的路径）、`CookieManager`、`WebStorage`、`findAllAsync`、`View.draw` 截图。网页要用摄像头、麦克风、定位时，需在 `tan.json` 的 permissions 里声明对应权限。

## 冒烟（macOS）

`examples/webview-demo -smoke` 依次检查：创建时与运行时 UA、资源请求额外头、页内查找计数/上一个/下一个/区分大小写/无匹配、`window.open` 与 `target=_blank` 被拦截并回调、导航策略取消外部跳转（应用设置了策略时由策略决定）、资源 origin 下载与 http 下载（WKDownload）到临时目录、下载后页面仍可通信、Cookie 设置/读取/删除/页面写入后 Go 可见/按域清除、清除缓存与存储、权限回调（getUserMedia，拒绝后页面收到 `NotAllowedError`）、Basic 认证、打印 PDF 与截图写文件。

没有摄像头的机器上 WebKit 不会发起权限请求，所以冒烟程序在 macOS 上用 WebKit 的私有测试开关（模拟采集设备）触发它，这只在示例里，框架代码不使用私有开关。
