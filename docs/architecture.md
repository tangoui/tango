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

## IPC 线上协议（v2，ipc + runtime-js 两端一致）

建立在原始消息之上。是 JSON 对象且含 `"tango"` 字段的消息归 ipc；其余都是用户原始消息，ipc 不碰。权威说明在 `ipc/wire.go` 的包注释，设计取舍见 [specs/2026-11-13-ipc-duplex-streams.md](specs/2026-11-13-ipc-duplex-streams.md)。

```jsonc
// 握手：runtime-js 启动时发 hello；Go Drop 该连接的旧状态，回自己的接收窗口（字节）
{"tango":"hello","proto":2}
{"tango":"hello","proto":2,"win":1048576}
// proto 不是 2：Go 回错误并 slog.Warn，不兼容 v1
{"tango":"hello","error":{"code":"bad_args","message":"runtime-js protocol mismatch"}}
// JS→Go 调用
{"tango":"call","id":1,"method":"Greeter.Hello","args":["world"]}
// Go→JS 返回
{"tango":"ret","id":1,"result":"Hello world"}
{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
// 事件（双向）
{"tango":"event","name":"progress","data":{"pct":42}}
// 流（全双工；任一端发起，id 由发起方分配，JS 用奇数，Go 用偶数；open 以外的消息两个方向都有）
{"tango":"stream","id":3,"op":"open","method":"Term.Attach","args":[]}
{"tango":"stream","id":3,"op":"accept","in":true}                        // in:false = HandleStream（只发）
{"tango":"stream","id":3,"op":"data","seq":0,"c":75,"data":{"cols":80}}  // JSON 负载
{"tango":"stream","id":3,"op":"data","seq":1,"c":69,"bin":"aGVsbG8="}    // 字节负载（标准 base64，带填充）
{"tango":"stream","id":3,"op":"ack","b":65536}                           // 归还的信用字节数
{"tango":"stream","id":3,"op":"end","seq":2}                             // 半关闭
{"tango":"stream","id":3,"op":"close","seq":2}                           // 优雅全关闭
{"tango":"stream","id":3,"op":"error","seq":2,"error":{"message":"...","code":"internal"}}
{"tango":"stream","id":3,"op":"cancel"}
```

- **帧**：一帧就是应用的一条消息，不拆不并。`data` 与 `bin` 二选一（`"bin":""` 是空字节帧，不同于没有 `bin`）；`seq` 按方向从 0 递增，接收方按序交付；`end`/`close`/`error` 的 `seq` 是本方向已发帧数，收齐之前的帧才处理。
- **背压**（按字节计的信用窗口）：每帧计费 `c` = 负载字节数 + 64（`FrameOverhead`），由发送方算好写进帧里（JSON 按发送方编码后的长度，字节按原始长度）。窗口是接收方通告的：Go 在 `hello` 里给出 `win`（`Router.SetStreamWindow`，默认 `DefaultWindowBytes` = 1 MiB），JS 侧固定 1 MiB。在途 + 本帧 > 窗口时发送方阻塞，在途为 0 时单帧可以超窗。接收方在应用读走一帧后累计归还，累计到半个窗口时发 `ack`；读走后接收队列已空也立即全部归还。单帧原始负载上限 `MaxFrameBytes` = 16 MiB，超过时发送调用返回 `bad_args`，流不受影响。
- **结束**：双方都 `end` 为正常完成；`close` = `end` 加"不再接收"，之后对方的写返回"stream closed by peer"，对方在这条流上不再发任何消息；`error` 让两端都失败；`cancel` 立即放弃并丢弃未读数据。Go 处理函数返回 nil 时发 `close`，返回错误时发 `error`。
- `accept` 之前打开方就可以发数据；收到 `in:false` 时本地发送失败（`bad_args`），对方丢弃已收到的数据并照常 `ack`。找不到处理函数回 `error`（`not_found`），不发 `accept`。未知 `op` 忽略。
- **进程内客户端**：`Router.Call`、`Router.Open` 让同进程的 Go 代码（自绘界面）使用同一批处理函数，走同一套流状态机，但不经过线协议（不编码 JSON 消息，不做 base64）。

错误 code：`not_found`、`bad_args`、`internal`、`canceled`、`transport`（快速通道上丢帧，见下）。

### 快速通道（IPC fast lanes）

规格见 [specs/2026-11-13-ipc-duplex-streams.md](specs/2026-11-13-ipc-duplex-streams.md) §6.1，包格式和协议规则写在 `ipc` 包注释的 "Fast lanes" 一节。

- **是什么**：挨着消息通道的二进制旁路，只承载流的**字节数据帧**（不做 JSON、不做 base64）；JSON 帧、全部控制消息、握手仍走消息通道。两个方向分别协商。同一条流的帧可以分走两条路，接收方按 `seq` 重排，语义不变。包格式 `count:u16 | (streamID:u64 | seq:u64 | len:u32 | payload)*count`（大端），快速通道上的帧不带 `c`，计费固定为 `len + 64`。
- **通道名**：`down:scheme` / `up:post`（核心在资源来源的 `/__tango/lane/down`、`/__tango/lane/up` 上服务：GET 长响应、POST 一包一请求，`gen` + `tok` 校验，Origin 不是资源来源一律 403），`down:android` / `up:android`、`down:shm` / `up:shm`（平台后端经 `platform.BinaryLane` / `platform.LaneHost` 实现）。`BinaryMinSize`：`down:scheme` 4096，平台通道 0；`up:post` 只接 16 KiB 以上、包不超过 1 MiB 的帧，攒 16 KiB 或 4 ms 发一次，同一页面同时只有一个 POST 在途。
- **分层**：`ipc.Router` 只管协议（`BinaryConn`：`LaneHello` / `SendBinary` / `BinaryMinSize`；可复用文档的连接另实现 `SessionBinaryConn`，上行入口 `Router.HandleBinarySession`），不知道平台；它不自己管理世代，只在发布连接状态时记下 `SessionBinaryConn.BinarySession()` 给出的世代和发送句柄，上行包按授权时捕获的世代匹配，旧世代的包不进新文档的流。`Router.HandleBinary` 只用于不复用文档的简单连接，对 `SessionBinaryConn`（如 `*tango.Surface`）丢包并按 Router 记一次 `slog.Warn`。一个 surface 只接一个 Router（每次 `hello` 都换通道世代，两个 Router 会互相作废，不支持）。`*tango.Surface`（根包 `lanes.go`）实现 `SessionBinaryConn`，包着 `internal/lane.State`——每页面世代号、每世代令牌（同一世代内的每个通道请求都用它，换世代才换）、每条通道的状态，是通道状态的唯一拥有者；平台后端只报告能力（`LaneCaps`）、发下行包、交上行包。runtime-js 的 `lanes.ts` 是通道模块注册表，`lanes/scheme.ts` 是 `down:scheme` / `up:post` 模块。
- **开启条件**（每次握手由 `Surface.LaneHello` 判断，四个都满足才在 `hello` 回复里带 `lanes` 字段）：应用注册了至少一个 `App.OnBinary` 处理函数（应用接线 `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte){ r.HandleBinarySession(s, gen, p) })`，处理函数在收包的 goroutine 上跑，不在主线程）；页面来自资源来源（直接从开发服务器加载的页面永远没有，`tan dev` 经 `asset.FromEnv` 代理的页面有）；后端报告了能力；没有被 `Options.DisableFastLanes` 或 `TANGO_IPC_LANES=0`（任何构建）关掉。**macOS 后端默认报告 `up:post`**（`TANGO_IPC_LANES_DARWIN=0`/`false`/`off` 关闭，`all` 另报 `down:scheme`、仅供实验，macOS 上只在 `tango_dev` 构建生效）；**iOS 后端默认不报告**，需 `TANGO_IPC_LANES_DARWIN=1`；其他平台后端还不报告能力，走消息通道，直到各平台计划落地。runtime-js 的 `up:post` 启动时先 POST 一个空包（`count=0`，Go 回 204），收到 204 才报告 `ready`，所以页面 CSP 挡住资源来源时通道在启动时就失败、不会有帧丢在上面。
- **控制消息**：JS 在核心内部频道上报告通道就绪/失败：`"\x00tango-int:lane:"` + `{"op":"ready"|"fail","lane":"<名>","gen":<n>}`，由 `App.dispatchInternal` 交给 `lane.State.Control`，不进 Router 和 `OnMessage`；`gen` 不是当前世代的丢弃。Go 只在 `ready` 之后往该通道发。JS 每个方向按 `list` 顺序一次启动一个模块，前一个失败才试下一个。
- **打开方等 `accept`**：流的打开方收到 `accept` 之前不把字节帧放上快速通道（否则包可能先于 `open` 到达、被当成未知流丢掉）；接收方可以立即用。Go、JS 规则相同。
- **导航**：`SurfaceNavigating` 时暂停下行通道（`SendBinary` false、`BinaryMinSize` -1、结束 `down:scheme` 响应）直到下次 `hello`，上行保留；核心导航到、或页面加载完成于资源来源以外时整个世代作废（`lane.State.Reset`）。上行包（`OnLanePacket`、`up:post`）只接受资源来源。
- **丢帧**：通道一出错，该方向立即停用，后续帧走消息通道。接收方有乱序帧缓存（或 `end`/`close`/`error` 在等之前的帧）而期待的那一帧连续 5 秒（`ipc.GapTimeout` / JS `GAP_TIMEOUT_MS`）没到，该流以 `transport` 失败，照常通知对端。不重传。
- **边界（`boundary`）**：每个通道帧发出后，发送方在消息通道上发 `boundary`（`seq` = 该帧序号 + 1），所以即使双工流丢的是最后一帧、之后没有数据也没有 `end`，接收方也能发现；只有接收序号确实没补齐的流在 5 秒后失败，其他流不受牵连。JS 在帧**真正发出**时才发 `boundary`（`up:post`：所在批次交给 `fetch` 时，不是排进队列时），同一条流之后经消息通道发的帧（JSON 或字节数据、`end`、`close`）也等它之前的通道帧发出后才发，页面主线程卡住时 Go 不会因为还排在页面里的帧提前开始计时。JS 的缺口计时器和 `up:post` 的请求计时器若晚触发超过 1 秒（主线程卡顿），用完整超时重新计时一次，第二次到期照常失败。中止路径（writable abort、被拒的写入、`cancel()`、本地出错，`_close`）的 `close`/`error` 仍立即发出，卡顿超过 5 秒时 Go 可能报 `transport` 而不是中止原因。
- **请求约束**：通道交付绑定文档会话（世代）。`up:post` 每个请求 4 秒超时（低于 5 秒的缺口超时：卡住的 POST 先让通道失败、后续帧回退）；连同在途包在内保留的编码字节最多 8 MiB，超出的帧不改 seq 和信用、直接走消息通道。请求只相对文档的真实位置解析（不受 HTML `<base>` 影响），拒绝重定向。
- **故障注入**（只在 `tango_dev` 构建）：`TANGO_IPC_LANE_FAULT=<通道名>:<N>`，该通道在每个世代成功发送（下行）/接收（上行）N 个数据包后进入故障（空包 `count=0` 不计：`down:scheme` 开头的 `[0,0]`、`up:post` 的启动探测；故障后的通道照样拒绝空包，探测因此失败），之后的包被拒绝/丢弃，按通道坏掉处理；格式不对时 `slog.Warn` 后忽略。各平台计划的"通道中途坏掉"验收用它。
- **诊断**：页面里 `tango.lanes()` 返回 `{down, up}`，没有通道的方向是 `"fallback"`；`examples/duplex -bench` 的 `lane` 字段就是它。

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
