# Tango 架构（第一阶段：核心运行时 + 桌面 WebView）

Tango 是开源的通用 Go 应用框架：一套 Go 代码跑在桌面（macOS / Windows / Linux）和移动端（iOS / Android），界面可以是系统 WebView，也可以是 Go 自绘（第二阶段），两者能放在同一界面里。

模块路径：`github.com/tangoui/tango`（占位，开源前可改）。Go 1.27。协议 MIT。

## 借鉴与取舍

| 来源 | 借鉴 | 避开 |
|---|---|---|
| mygo | 核心层管逻辑、后端只做翻译；桌面无 cgo（purego/syscall）；`AwaitAsync` 在主线程等待时泵事件；fake 后端测核心 | 过宽的后端接口；只有生成式绑定、没有原始通道 |
| Wails v3 | 主线程派发模型；资源服务是 `http.Handler`；移动端用 c-archive / c-shared + 原生宿主 | 单个大包 + 大量 tag；cgo/purego 双份实现；移动端硬套窗口抽象 |
| Viva | 即时式 API + 保留式场景（第二阶段）；热重载宿主/worker 双进程；发布构建用 build tag 去掉开发功能 | 各平台各写一套文本/渲染栈 |

参考源码（只读，借鉴代码须保留原许可证声明）：
- mygo（MIT）：`~/go/pkg/mod/github.com/egoist/mygo@v0.9.0`，`docs/architecture.md` 很详细
- Wails v3（MIT）：`~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.1.0`
- Viva（Apache-2.0）：`~/go/pkg/mod/github.com/vivaui/viva@v0.6.0`

## 分层

```
用户代码
  │
  ├─ tango（根包）        App / Surface / Post / Await / AwaitAsync / 生命周期 / 原始消息
  ├─ ipc / bind / asset  调用、事件、流；可选反射绑定；资源服务
  ├─ dialog / clipboard / menu ...   一个能力一个包，不支持返回 platform.ErrUnsupported
  │
  └─ internal/platform   Backend 契约（backend.go / types.go / caps.go）
        ├─ darwin  windows  linux   （无 cgo）
        ├─ ios  android             （cgo，build tag 隔离，第三子项目）
        └─ fake                     （无界面，测核心）
```

契约文件 `internal/platform/{backend,types,caps}.go` 是各方共同依据，**实现方不得修改**，需要改就提出来。

## 线程模型

- `tango.Run` 必须在 main goroutine 调用；根包 `init()` 里 `runtime.LockOSThread()`（桌面）。移动端主线程归原生宿主，不锁（第三子项目处理）。
- 主线程队列在核心层：`Post(fn)` 入队后调用 `Backend.Wake()`；后端在主线程调 `Host.Drain()` 执行队列。
- `Await(fn)`：从任意 goroutine 同步在主线程执行并取回结果；若调用方已在主线程则直接执行。
- `AwaitAsync(start)`：在主线程调用 `start(done)`，阻塞调用方直到 `done(v)`。若调用方就在主线程，则循环 `Backend.Step(true)` 泵事件直到完成（对话框等嵌套场景不死锁）。
- 后端回调（`Host.*`）里运行的用户回调都在主线程。

## 根包公开 API（核心子 agent 实现）

```go
package tango

type Options struct {
    Name, ID string
    Assets   http.Handler // 挂在 Backend.AssetOrigin 下；nil 则 404
}

func New(opts Options) *App
func (a *App) Run() error                 // main goroutine，阻塞到退出
func (a *App) Quit()                      // 任意 goroutine
func (a *App) Post(fn func())             // 任意 goroutine
func Await[T any](a *App, fn func() T) T
func AwaitAsync[T any](a *App, start func(done func(T))) T
func (a *App) OnReady(fn func())          // LifecycleStarted 后在主线程调用
func (a *App) OnLifecycle(fn func(Lifecycle))
func (a *App) NewSurface(opts SurfaceOptions) (*Surface, error) // 任意 goroutine
func (a *App) Surfaces() []*Surface
func (a *App) AddBridgeScript(js string)  // 追加到每个 Surface 的 BridgeScript（核心 shim 之后）
func (a *App) OnMessage(fn func(s *Surface, msg string)) // 所有 Surface 的原始消息
func (a *App) Backend() platform.Backend  // 供模块内能力包做类型断言
func (a *App) Go(fn func())               // 启动 goroutine，panic 走 Options.OnPanic（见 panics.md）
// Options.OnPanic func(Panic) (keepRunning bool)；type Panic struct{ Value any; Stack []byte; Where string }

type Lifecycle = platform.LifecycleEvent // 并导出 Started/Foreground/Background/Terminating 常量

type SurfaceOptions struct {
    Title                    string
    Width, Height            int
    MinWidth, MinHeight      int
    Resizable                bool
    Hidden                   bool   // 默认创建后即显示
    URL                      string // "/index.html" 这类相对路径按 AssetOrigin 解析；空为 "/"；也可绝对 URL
    Debug                    bool
}

// Surface 的方法任意 goroutine 可调（内部 Post/Await）
func (s *Surface) ID() platform.SurfaceID
func (s *Surface) Show() / Hide() / Close() / Focus()
func (s *Surface) SetTitle(string) / SetSize(w, h int) / Size() (w, h int)
func (s *Surface) Navigate(url string)     // 相对路径同上解析
func (s *Surface) Eval(js string)
func (s *Surface) Send(msg string)         // Go→JS 原始消息；页面未 Loaded 前缓存，Loaded 后按序冲刷
func (s *Surface) OnClose(fn func() bool)  // 用户关窗询问；任一返回 false 则不关
func (s *Surface) OnClosed(fn func())
func (s *Surface) OnLoaded(fn func())
func (s *Surface) Native() uintptr
```

测试用的 fake 后端注入方式由核心子 agent 定（模块内未导出构造函数或 internal 包）。

## JS 桥

1. 后端 shim（各后端实现，文档开始时注入，先于一切页面脚本）：
   `window.__tangoNative = { post(str) }` → 原生 postMessage → `Host.OnMessage`。
2. 核心 shim（核心子 agent 写在 Go 字符串里，作为 BridgeScript 开头）：
   ```js
   window.__tango = {
     post(str),            // 调 __tangoNative.post
     onMessage(fn),        // 订阅 Go→JS 原始消息，返回取消函数
     _recv(str),           // 核心通过 Eval 调用：window.__tango._recv(<JSON 字符串字面量>)
   }
   ```
   `_recv` 前到达的消息不存在（核心在 Loaded 前缓存），但 `onMessage` 订阅前收到的消息要在 JS 侧缓存，首个订阅者注册时冲刷。
3. `AddBridgeScript` 追加的脚本（如 ipc 运行时）放在核心 shim 之后。
4. 页面调用系统能力（窗口、剪贴板、对话框……）走 `jsapi` 包 + `@tangoui/runtime` 的 `Window`/`App`/... 模块，见 [runtime-api.md](runtime-api.md)。

## IPC 线上协议（ipc + runtime-js 子 agent 实现，两端一致）

建立在原始消息之上。是 JSON 对象且含 `"tango"` 字段的消息归 ipc；其余都是用户原始消息，ipc 不碰。

```jsonc
// JS→Go 调用
{"tango":"call","id":1,"method":"Greeter.Hello","args":["world"]}
// Go→JS 返回
{"tango":"ret","id":1,"result":"Hello world"}
{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
// 事件（双向）
{"tango":"event","name":"progress","data":{"pct":42}}
// 流（任一端发起；id 由发起方分配，JS 用奇数，Go 用偶数）
{"tango":"stream","id":3,"op":"open","method":"Logs.Tail","args":[]}
{"tango":"stream","id":3,"op":"data","data":"line"}
{"tango":"stream","id":3,"op":"ack","n":16}   // 接收方确认，发送方按窗口背压（默认窗口 64）
{"tango":"stream","id":3,"op":"end"}
{"tango":"stream","id":3,"op":"error","error":{"message":"..."}}
{"tango":"stream","id":3,"op":"cancel"}
```

错误 code：`not_found`、`bad_args`、`internal`、`canceled`。

## 能力包

`clipboard`、`dialog`、`menu` 第一批；签名接受 `*tango.App`，内部对 `a.Backend()` 做类型断言，没实现返回 `platform.ErrUnsupported`。对话框函数阻塞调用方（内部 `AwaitAsync`）。

## 平台后端要点

| 后端 | 窗口 | WebView | 资源 origin | 主线程唤醒 |
|---|---|---|---|---|
| darwin | AppKit NSWindow | WKWebView + WKURLSchemeHandler | `tango://app` | CFRunLoopSource / dispatch_async_f |
| windows | Win32 | WebView2 (COM, syscall) | `https://tango.localhost`（WebResourceRequested 拦截） | 隐藏消息窗口 PostMessage |
| linux | GTK3 | WebKitGTK 4.1（webkit_web_context_register_uri_scheme） | `tango://app` | g_idle_add / g_main_context_wakeup |

第一阶段只在 macOS 实机验证；Windows / Linux 需 `GOOS=windows|linux CGO_ENABLED=0 go build ./... && go vet` 通过，实机验证后补。

## 契约第二轮（功能移植）

新增内容都在 `internal/platform/ext.go`，全部是**可选**接口，由核心层做类型断言来发现，不增加最小后端的负担。设计按 Tango 自己的风格来，**不照搬 Wails/mygo**：
- **统一应用事件**：深链、打开文件、主题变化、显示器变化、Dock 点击都走 `HostEx.OnAppEvent`；`ShouldQuit` 用来拦截用户退出。根包的对应 API 是 `App.OnEvent` 和 `App.OnQuitRequest`（events.go）。
- **外观设置**：Surface 外观整体用一个 `Appearance` 结构体下发（无边框、透明、背景、材质、置顶、透明度、按钮开关），后端自己做差异比较。
- **窗口能力拆成小接口**：`Geometry`、`Stateful`、`Styled`、`PageControl`、`Dragger`；创建参数用 `CreatorEx` 加 `SurfaceExtras`（窗口类型、父窗口、位置、状态、外观、接收拖放）。
- **应用级能力**：`Displays`、`SystemTheme`、`Shell`、`AppControl`、`ClipboardRich`、`PopupMenu`、`Trays`、`Notifier`、`Hotkeys`。
- **新增 Surface 事件**：Navigating、LoadFailed、Moved、StateChanged、FilesDropped。
- **ServeAsset 可以在任意线程调用**（后端应在主线程外调用）。
- **规则统一**：OnMessage 只投递主框架的消息；对话框被直接关掉时返回 `(-1, ErrCanceled)`，单按钮对话框返回 `(0, nil)`；菜单项支持可勾选、单选组和图标。

公开 API 的约定：一个能力一个包，函数第一个参数是 `*tango.App`，平台不支持时返回 `ErrUnsupported`；窗口能力做成 `*tango.Surface` 的方法。

## 契约第三轮（统一落地各组报告的契约问题）

改动仍集中在 `internal/platform/ext.go`（另有 `types.go`、`backend.go`、`mobile_ext.go` 的说明），原则不变：新能力是可选接口，靠类型断言发现。这一轮把各组靠方法名临时约定的能力写进契约，并修掉几处零值和口径问题。

| 主题 | 旧 | 新 |
|---|---|---|
| 外观零值 | `platform.Appearance{Shadow, Closable, Minimizable, Maximizable}`，零值 = 无阴影、不可关闭/最小化/最大化 | `NoShadow`、`NoClose`、`NoMinimize`、`NoMaximize`，零值 = 普通窗口；根包 `tango.Appearance` 改为 `platform.Appearance` 的别名（字段名不变） |
| 无边框边缘缩放 | 鸭子类型 `HandlesEdgeResize()` | `platform.EdgeResizer`（Surface 可选） |
| 应用控制能力查询 | 鸭子类型 `AppControlSupports(op string)` | `AppControl.AppControlOps() AppControlOp`（位集 `CtlHide/CtlBadge/CtlBadgeText/CtlProgress/CtlAttention/CtlDock`）；`appctl.Supports(a, op)` |
| WebView 版本 | 鸭子类型 `WebViewVersion()` | `platform.WebViewInfo` |
| 最近文档 | 鸭子类型 `NoteRecentDocument/ClearRecentDocuments` | `platform.RecentDocuments` |
| 窗口可见/焦点 | 无 | `platform.SurfaceStatus{IsVisible, IsFocused}`（可选，核心没有时自己记账） |
| 剪贴板写文件 | 无 | `ClipboardRich.ClipboardWriteFiles`；`clipboard.WriteFiles` |
| 托盘菜单键 | 各平台固定 | `TrayOptions.MenuOn`（`TrayMenuDefault/Primary/Secondary`）；`tray.Options.MenuOn`、`Tray.SetMenuOn` |
| 移动端事件数据 | 借用 `AppEvent.URL/Paths`（推送 JSON、令牌、SurfaceID 字符串） | `AppEvent.Insets`、`Orientation`、`Push{Token, Payload}`、`Surface`；`PushPayload.ID`；`platform.ParsePushPayload` |
| 点击旧通知启动 | 丢失 | `AppNotificationOpened`（`AppEvent.Notification` = id）；`notify.OnClick` 也会收到；macOS 在启动前装好通知代理 |
| 主线程嵌套等待 | Android 上主线程 `AwaitAsync` 永远等 | `platform.NestedLoop`；`tango.TryAwaitAsync` 返回 `ErrNestedWait`，`AwaitAsync` 立即返回零值 |

**核心钩子与访问器**（根包）：
- `App.OnSurfaceCreated(func(*Surface))`：每个 Surface（WebView 或 canvas）创建完成后、`NewSurface` 返回前在主线程调用；`corehook.OnSurfaceCreated` 是给只持有 `any` 的内部代码（`tan dev` 给纯 WebView 应用接 windowstate）用的同一钩子。用它修掉了：Windows 后建窗口没有已设置的徽标/进度（`appctl` 记住并重放；Windows 后端另外在 `TaskbarButtonCreated` 时补设）；`theme.SyncPage` 新页面只能拿到启动时强调色（改为每个页面加载后推送当前主题）；`jsapi` 在窗口创建时就挂上窗口事件。
- `App.Name()`、`App.OnEventKind(kind, fn)`（只取一种事件，不吞掉其它种类的待投递启动事件）。
- `Surface.App()`（`menu.Popup`、`hotkey.Bind` 不再走已删除的 `corehook.SurfaceApp`）、`Surface.Title()`、`IsVisible()`、`IsFocused()`、`OnFocused/OnBlurred`（转发后端的 `SurfaceFocused/Blurred`）、`OpenDevTools()`。

**能力包补充**：`clipboard.WriteFiles`（及 `ErrNotAbsolute`）；`single.OnSecondInstance(a, fn)` 应用级第二实例钩子（`jsapi.Install` 自动注册，`API.SecondInstance` 删除）；`hotkey.Validate` 与 `ErrBadAccelerator`（jsapi 写错快捷键返回 `bad_args`）。

**坐标口径**：`Rect`/`Bounds` 的 X/Y 是窗口外框（含标题栏和边框）左上角，原点是主屏左上角、Y 向下的逻辑像素；Width/Height 是内容区大小。`Display.Bounds/WorkArea`、`Tray.Bounds`、`SurfaceMoved` 同一坐标系。Linux 修正了 GDK 坐标（原点是全部显示器的包围盒）没有平移到主屏原点的问题；Windows、macOS 核对后已符合。

**文档化的规则**：
- `ClipboardRich` 读不到对应内容（空或只有文本）返回 `(nil, nil)`，错误只用于读取/转换失败；写入替换整个剪贴板。
- `SurfaceClosed`：每个 Surface 恰好一次，在原生窗口销毁后（`Close`、用户关闭且 `ShouldClose` 同意、系统销毁）；`NewSurface` 返回前不发；`Run` 返回时仍开着的 Surface 不发（核心直接视为已关闭）；之后该 id 不再有事件，核心也不再调用它。
- `Host.OnMessage` 只投递主框架消息（第二轮已写，复核无误）。
- 移动端 `Init`/`Run` 在调用 `App.Run` 的 goroutine 上执行，`Run` 阻塞到退出；`Started`/`Terminating` 在主线程投递，`Run` 在 `Terminating` 之后返回。
- `Notifier`：本次运行发出的通知点击走 `onClick`；无法路由的点击（上次运行的通知、启动应用的那次点击）报 `AppNotificationOpened`。

## 契约 sysx（桌面系统能力）

新增契约在 `internal/platform/sysx_ext.go`，仍是可选接口、类型断言发现；后端实现放各自的 `sysx_*.go`。

| 能力 | 契约 | 公开 API | macOS | Windows | Linux |
|---|---|---|---|---|---|
| 安全存储 | `SecureStore`（mobile_ext.go，任意 goroutine） | `sys/securestore` | Keychain | 凭据管理器 `CredWriteW/CredReadW/CredDeleteW` | libsecret（dlopen）；没有库或 Secret Service 时 `ErrUnsupported` |
| 电源 | `Power`：`PowerStatus`、`SetPowerHandler`、`BlockSleep/UnblockSleep` | `sys/power`：`Get`、`OnChange/OnSuspend/OnResume`、`KeepAwake/KeepDisplayOn` → `*Block.Release` | IOKit 电源、`IOPMAssertion`、NSWorkspace 睡眠/唤醒、低电量模式 | `GetSystemPowerStatus`、`WM_POWERBROADCAST`、电源请求 | UPower、logind `Inhibit`/`PrepareForSleep`、power-profiles-daemon |
| 网络 | `Network`：`NetworkStatus`、`SetNetworkHandler` | `sys/network`：`Get`、`OnChange`（只报真实变化） | `nw_path_monitor`（Network.framework C API） | Network List Manager + IP 接口变化通知 | NetworkManager D-Bus；没有时 `ErrUnsupported` |
| 托盘弹出面板 | 无新契约（`Tray.Bounds` + Surface） | `sys/flyout`：`New(a, tray, Options)`、`Open/Close/Toggle/OpenAt` | ✓ | ✓ | SNI 用最近一次 Activate 坐标；AppIndicator 无坐标时放主屏工作区右上角 |
| 拖出文件 | Surface 可选 `FileDragger` | `Surface.DragFiles(paths, icon, done)`（根包 dragout.go） | `NSDraggingSession` | OLE `DoDragDrop`（模态） | `gtk_drag_begin_with_coordinates` |
| 应用快捷菜单 | `AppShortcuts`、`ShortcutArgPrefix` | `sys/jumplist`：`Set/Clear/OnClick` | Dock 菜单（`applicationDockMenu:`），点击进程内回调 | Jump List 任务（`ICustomDestinationList`），点击以 `--tango-shortcut=<id>` 重新启动，经 `sys/single` 转给主实例 | 无运行时 API，`ErrUnsupported`；`.desktop` 的 Desktop Actions 由 tan 打包写入（Exec 带同一参数），`OnClick` 照样收到 |

规则：
- `flyout` 纯 Go 实现：面板是 `KindPanel + TitleBarNone + AlwaysOnTop` 的隐藏 Surface，`Place` 按托盘图标在工作区哪一侧决定放在下/上/左/右并夹在工作区内；失焦（`OnBlurred`）自动隐藏，失焦后 400ms 内的托盘点击视为同一手势不重新打开。有托盘菜单时用 `tray.MenuOnSecondary`。
- `FileDragger.StartFileDrag` 必须在鼠标按住时调用（页面 mousedown 转给 Go）；没按住不开会话、`done(DragNone)`，避免拖到指针下的任意位置。macOS 只提供复制/链接（不让 Finder 移走应用的文件）。
- 后端内部改进（无契约变化）：Windows 通知改用 WinRT Toast（需要 AUMID：运行时 `SetCurrentProcessExplicitAppUserModelID`，失败退回托盘气泡）；Linux 托盘优先 StatusNotifierItem（支持左键 Activate、右键菜单），没有 Watcher 退回 AppIndicator；Linux Wayland 全局快捷键走 xdg-desktop-portal GlobalShortcuts；Linux 资源服务流式交付（SSE、大文件）；WebView2 拖入多文件拿全部路径。
- macOS 冒烟：`examples/sysx-demo -smoke`（已加入 `scripts/smoke.sh`）。

## 契约第四轮（WebView 高级能力）

新增 `internal/platform/webview_ext.go`，仍全是可选接口：核心的 Host 另外实现 `WebHost`（权限请求、导航策略、新窗口、HTTP 认证、下载开始/事件，后端从原生代理里回调，`done` 可同步也可稍后在主线程调用）；WebView Surface 可选实现 `WebFinder`、`WebCookies`、`WebData`、`WebUserAgent`、`WebHeaders`、`WebPDF`、`WebSnapshot`、`WebDownloads`；Backend 可选实现 `WebCreator`（创建时就带 UA / 请求头）。根包 API 在 `webview.go`（Surface → App → 默认的决策顺序），页面侧是 `jsapi` 的 `Web*` 能力（默认关）。用法与各平台支持矩阵见 [webview.md](webview.md)。
