# 更新日志

本文件记录 Tango 各版本的用户可见变化，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。Go 模块、`cmd/tan` 与 `@tangoui/runtime` 使用同一个版本号，见 [docs/release.md](docs/release.md)。

## [未发布]

### 新增

- `ui.RunnerOptions.Content`（实验性）：用 `ui.ContentData` 标明窗口主要展示自行变化的数据（表格、网格、行情、列表、仪表盘）。macOS 上这类窗口不限大小默认走原生图层，列表、行情窗口 physical footprint 39–54 MB（Metal 135–147 MB），800×600 数据网格约 69 MB（mygo 107 MB），帧持续超过 8 ms 时仍单向升级 Metal；默认 `ui.ContentGeneral` 不变，大窗口仍走 Metal。`TANGO_RENDERER` 和 `RunnerOptions.Renderer` 优先。
- `ui/paint`（实验性）：新增 `(*CPU).RenderRegionInto(dst, ops, region, scale)`，区域与 `dst` 边界相同时清零并复用 `dst`，结果与 `RenderRegion` 一致；每帧重绘同一区域时不再每次新分配位图。
- 实验性 `macoslite` 包：独立的 macOS 原生轻量窗口宿主，只用 purego 调 AppKit/CoreGraphics/CoreText，不加载 WebView、`ui.Runner` 或 GPU 渲染器；提供窗口生命周期、鼠标键盘事件、`Canvas` 填充/圆角矩形/CoreText 文本。参考窗口 `examples/macoslite-counter` 空闲 physical footprint 约 26 MiB，测量脚本 `scripts/macoslite-memory.sh`。
- macOS 新增 `TANGO_LAUNCH_BACKGROUND=1`（常量 `automation.EnvLaunchBackground`）：应用启动时不激活，窗口显示在其他应用窗口之后，相当于 `open -g`。`automation.Launch` 默认设置它，端到端测试不再抢用户焦点、也不遮挡别的窗口；需要前台时在 `Options.Env` 里设为 `0`。
- `Appearance.AllSpaces`：窗口出现在每个桌面空间。macOS 上会出现在所有 Space，包括全屏应用旁边；Linux 上是粘滞窗口；Windows 没有对应 API，忽略这个字段。

- `@tangoui/runtime` 的 `ErrorCode` 加入 `busy`（更新器在已有检查或下载进行时返回）。

- macOS 自绘 UI 新增可选 `TANGO_TEXT_ENGINE=coretext` 轻量文字路径：无换行短文本使用 CoreText glyph-run 与系统字体回退，复杂排版自动继续使用 go-text/HarfBuzz；默认文字引擎和输出路径不变。

- 自绘窗口支持可选 `TANGO_RENDERER=auto`：小窗口先用 CPU，放大、内容复杂或持续高频刷新后单向升级 GPU；保持默认 GPU 优先和显式 Renderer 优先级不变。系统字体链可由 `text.TrimFontCaches()` 丢弃，平台低内存通知也会触发回收；仍被页面或编辑器持有的字体继续有效，macOS 原生字体句柄在最后一个拥有者不可达后释放（见 `docs/dev-builds.md`）。

- **实验性编译 profile**：`tan.json` 可配置 `features: ["webview", "canvas"]`，`tan build/package -features webview,canvas` 可覆盖；默认 `all` 保持兼容。Darwin profile 对缺少的 WebView/canvas 能力返回 `ErrUnsupported`；Windows/Linux 非 `all` profile 本阶段明确拒绝。最小样本测量显示当前 Go 链接器已自动裁掉大部分未用后端代码，因此 profile 暂不宣传为体积优化，见 [体积报告](docs/package-size-optimization.md)。

- **主题重建优化**：Alias 和动态主色重建不再重复创建完整图标映射和组件派生，公开主题 API 与 token 结果不变；运行时构造分配和耗时下降，安装后体积收益未显著测出。

- **IDE 工作台组件**（均为实验性 API）：
  - `Dock` 停靠分屏：可嵌套分割树、标签拖到组边缘拆分 / 中间合并（带落点预览）、分割条调整、组最大化，布局模型 `DockLayout` 可 JSON 保存与恢复；`Tabs` 新增 `Draggable` 拖拽排序与 `OnTearOff` 拖出窗口回调（示例 `examples/dock-demo`）。
  - `StatusBar` 状态栏、`TitleBar` 自绘标题栏与 `WindowControls` 窗口按钮（经新接口 `ui.WindowFrame` 控制真实窗口与拖动）、`NotificationCenter` 通知中心、`HotkeyInput` 快捷键录制框。
  - AI 对话组件：`Bubble` / `BubbleList`（流式打字、虚拟化、贴底跟随）、`Sender`（附件、@ 与 / 建议、发送 / 停止、输入法安全）、`ThoughtChain`、`ToolCall`、`Approval`、`Conversations`、`Prompts`。
  - 查看器：`CodeBlock` 只读代码块、`LogView` / `LogBuffer`（百万行、ANSI 颜色、级别过滤、搜索、跟随）、`JSONView`（懒加载值树、分组、复制值 / 路径、搜索）；代码高亮钩子 `SetCodeHighlighter`，`ui/codehl` 用编辑器语法实现，`Markdown` 代码块随之着色（新增 `Markdown.Highlighter`、`Markdown.Color`）。
  - `ui/graph`：关系图画布 `Diagram`（节点、连线、平移缩放、框选、小地图、分层自动布局）、ER 图 `ER`、Git 提交图 `GitGraph`（泳道布局 + 虚拟列表）。
  - `uihost.WebView` / `ui.NativeView`：在自绘布局里嵌入原生 WebView 区域，跟随布局与裁剪，被自绘浮层遮挡时冻结成截图；macOS 实机验证，Windows / Linux 未实机验证（示例 `examples/webview-embed`）。
  - `ui/editor/diff`：行级与字符级 diff 算法（对齐 Monaco 0.56 的结果，含移动块检测、未改动区域与差异导航），Diff 视图待后续。
- **IPC 双向流**（`ipc`、`@tangoui/runtime`）：流改为全双工，两个方向都能传 JSON 帧和字节帧，任一端都可以打开。
  - Go：`Router.HandleDuplex` 注册双向处理函数（类型 `ipc.DuplexFunc`）；`*ipc.Stream` 新增 `WriteBytes`、`Receive`（返回新类型 `ipc.Message`：`Binary`、`Bytes`、`JSON`）、`ReceiveBytes`、`ReceiveJSON`、`CloseSend`（半关闭）、`ReadWriter`（`io.ReadWriteCloser` 视图，可直接 `io.Copy`）；`Router.Open`、`Router.Call` 让同进程的 Go 代码（自绘界面）不经过页面直接使用同一批服务；`Router.SetStreamWindow` 设置 Go 的接收窗口；新增常量 `ProtoVersion`、`DefaultWindowBytes`、`FrameOverhead`、`MaxFrameBytes` 和错误码 `CodeTransport`（快速通道上丢帧时使用，见下）。
  - JS：`TangoStream` 新增 `accepted`、`send`、`sendBytes`、`end`、`bufferedAmount` 和 Web Streams 视图 `readable`/`writable`，迭代时字节帧产出 `Uint8Array`；新增 WebSocket 风格的 `socket()` / `TangoSocket`（`close()` 写完排队的消息后发 `close`，随即以 1000 关闭，不等 Go）；新增常量 `PROTO_VERSION`、`DEFAULT_WINDOW_BYTES`、`FRAME_OVERHEAD`、`MAX_FRAME_BYTES`。
  - 示例与基准：`examples/duplex`（回显、文件上传、小终端、`-panel` 进程内 Go 面板、`-bench` 基准），基线数据见 [docs/ipc-bench.md](docs/ipc-bench.md)。
- **IPC 快速通道（公共设施）**：流的字节帧可以走挨着消息通道的二进制旁路，不做 JSON 和 base64；JSON 帧、控制消息和握手仍走消息通道，语义不变。**macOS 默认开启 `up:post`（上行），Android 默认开启 `down:android` / `up:android`（上下行）；iOS 可以试用（默认关闭），Windows、Linux 仍走消息通道**，各平台后端接入并通过基准对比后逐个开启。
  - Go：`App.OnBinary` 注册上行包处理函数，应用接线 `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })`；`ipc.BinaryConn` 和 `ipc.SessionBinaryConn`（`*tango.Surface` 实现，后者把发送和上行投递绑定到页面世代）、`Router.HandleBinarySession`（`OnBinary` 用它，带授权时的世代）、`Router.HandleBinary`（只给不复用页面的简单 `BinaryConn`；对 `SessionBinaryConn` 丢包并记一次 `slog.Warn`）、`ipc.GapTimeout`；一个 surface 只接一个 `Router`；`Options.DisableFastLanes` 和环境变量 `TANGO_IPC_LANES=0` 关闭快速通道；开发构建里 `TANGO_IPC_LANE_FAULT=<通道名>:<N>` 让某条通道在 N 个数据包后故障（空包不计，比如 `up:post` 的启动探测），用来测通道中途坏掉。
  - 开启条件：注册了 `OnBinary`、页面来自资源来源（直接从开发服务器加载的页面没有快速通道）、后端报告了能力、没有被关闭。
  - 丢帧：某帧缺了 5 秒仍没到的流以 `transport` 错误失败（两端都看到）。
  - JS：`lanes()` 返回当前用的通道（`{down, up}`，没有时为 `"fallback"`）；新增常量 `GAP_TIMEOUT_MS` 和 `createClient` 的第二个参数 `ClientOptions`（`gapTimeoutMs`）；内置 `down:poll` / `up:post` 通道模块；`ErrorCode` 补上 `transport`。
  - 示例：`hello`、`runtime-demo`、`showcase`、`duplex` 接上 `OnBinary`；`examples/duplex -bench` 的 `lane` 字段改为 `lanes()` 的值（`{"down":…,"up":…}`）。
- **macOS 默认开启 `up:post`；iOS 试用（默认关闭）**：WKWebView 后端接入 `up:post`（上行字节帧经 `tango://` POST，ArrayBuffer 请求体）。macOS 上传 64 MiB 吞吐约为消息通道的 2.73 倍（[docs/ipc-bench.md](docs/ipc-bench.md)）。下行默认仍走消息通道（macOS 可开 `down:poll`，见下一条）。环境变量 `TANGO_IPC_LANES_DARWIN`（窗口/页面创建时读一次；取值去掉首尾空白、不区分大小写；所有取值在任何构建都生效）：

  | 取值 | macOS | iOS |
  |---|---|---|
  | 未设置、空、未列出的值 | `up:post`（默认） | 关闭（默认；未列出的值见下） |
  | `1`、`all` | `down:poll` + `up:post` | `up:post` |
  | `up` | `up:post` | `up:post` |
  | `down` | `down:poll`（排查用） | `up:post` |
  | `0`、`false`、`off` | 关闭 | 关闭 |

  iOS 上除了未设置、空和关闭值，其他取值都是 `up:post`。

  `TANGO_IPC_LANES=0` 和 `Options.DisableFastLanes` 在两个平台上都始终优先关闭。iOS 真机上没法从外部传环境变量，要试用就在创建第一个窗口/页面之前调用 `os.Setenv("TANGO_IPC_LANES_DARWIN", "up")`（不用 `1`：同一份代码在 macOS 上 `1` 还会打开实验性的 `down:poll`）。
  - CSP：`up:post` 启动时先 POST 一个空包，收到 204 才算就绪。页面的 CSP 若不允许 `connect-src` 到资源来源（例如 `connect-src https://api.example.com`，或不含 `'self'` 的 `default-src`），这次探测被拒，通道不开启，字节帧照常走消息通道，不报错；想用快速通道就在 `connect-src` 里允许 `'self'`。
  - 已知限制：iframe 拿不到通道令牌（macOS 实测：不透明来源和同源 frame 的通道请求全部被拒；iOS 未单独测；同源 frame 能访问 `parent.tango`，那是应用代码）；WebKit 的 `tango://` 请求不带 `Origin`，后端用主文档的来源补上，所以应用页面里嵌的跨源 iframe 发往资源来源的请求能通过资源 CSRF 检查（原有行为；通道请求仍受每世代令牌保护），不要在资源页面里嵌不可信的 frame；WebKit 不交付 Blob 请求体；iOS 只在模拟器上测过；通道中途坏掉的场景在 iOS 模拟器上由 `scripts/smoke.sh ios-lanefault` 自动测试（Android 为 `android-lanefault`）。见 [docs/mobile.md](docs/mobile.md)。
- **macOS 下行轮询通道 `down:poll`（默认关闭，`TANGO_IPC_LANES_DARWIN=1` 开启）**：核心的下行传输整体换成轮询。页面始终挂一个 `GET tango://app/__tango/lane/poll`，Go 有消息时用一批消息结束这次响应，页面立即再发下一个；`Surface.Send` 的每条消息（ipc 的 JSON 帧和控制消息、调用回复、jsapi 事件、只用 `__tango.onMessage` 的应用收的消息）和 ipc 的字节帧（原始字节，不做 base64）都按同一个序号序列走它，不再每条消息占一次主线程、调一次 `evaluateJavaScript`。轮询失败、页面 1 s 不来取（例如在后台）或通道空闲只有小消息时，消息带序号改走 `Eval`，页面按序号交付，顺序不变。开启条件：页面来自资源来源、没有关闭快速通道；不要求注册 `OnBinary`。macOS 实测（[docs/ipc-bench.md](docs/ipc-bench.md)）：下行 64 KiB 帧 306 → 1017 MB/s，1 KiB 小帧 4.5 万 → 50 万帧/s，2.3 KB JSON 3.4 万 → 11 万帧/s，64 B 往返平均慢 9%；64 KiB 和小帧仍低于 Wails v3.1.0（受 1 MiB 流控窗口限制），没达到默认开启的门槛。iOS、Windows、Linux、Android 不报告 `down:poll`；它们的下行走带序号的 `Eval` 路径（合批、与导航赛跑时丢弃，见"变更"）；`Surface.Eval` 在这些平台上仍是一次普通的 `evaluateJavaScript`，不进序列。
- `Surface.Send` 只在消息真的要走主线程时才检查调用方是不是主 goroutine（这个检查每次约 10 µs）。
- **页面通告流接收窗口**：runtime-js 的 `hello` 带 `win`（页面每流的接收窗口，1 MiB），`down:poll` 就绪后再发 `{"tango":"win","win":4194304}` 把窗口提到 4 MiB；Go 用它作为朝这个页面的发送窗口（新流用新值，已开的流只升不降）。没有 `win`（旧 runtime-js）或不合法时按 `ipc.DefaultWindowBytes`，超过新常量 `ipc.MaxPeerWindowBytes`（64 MiB）按 64 MiB；新增常量 `ipc.MinWindowBytes`（4096）。旧 Go 忽略这两处，照旧按 1 MiB 发。
- runtime-js 读走四分之一窗口（原来是一半）就归还信用。
- `ipc.OrderedBinaryConn`（`ipc.BinaryConn` 的可选扩展，`*tango.Surface` 实现）：`SendBinaryOrdered` 报告接受包的下行通道是否与消息通道同序、同可靠；是的话（`down:poll`）Go 不再为这帧发 `boundary` 控制消息，小帧下行时页面要处理的条目减半。其他下行通道照发；线路格式不变。

- **Android 默认开启快速通道**：`down:android` / `up:android`，两个方向的流字节帧都以 `ArrayBuffer` 走页面的 `WebMessageListener` 通道（上行 `postMessage(ArrayBuffer)`，下行 `JavaScriptReplyProxy.postMessage(byte[])`），不做 JSON 和 base64，没有最小帧大小。需要 WebView 116 及以上（更旧的 WebView 或退回 `JavascriptInterface` 时照旧走消息通道）。模拟器上 64 KiB 帧吞吐上行约为消息通道的 6.1 倍、下行约 4.4 倍，64 B 帧分批 p99 约为 0.16 倍；64 B 帧分批 p50 慢约 24%（[docs/ipc-bench.md](docs/ipc-bench.md)）。环境变量 `TANGO_IPC_LANES_ANDROID`（每个页面创建时读一次）为 `0`、`false`、`off` 时关闭，未设置或其他值为开启；`TANGO_IPC_LANES=0` 和 `Options.DisableFastLanes` 始终优先关闭。应用没法从外部拿到环境变量，要关闭就在创建第一个页面之前 `os.Setenv("TANGO_IPC_LANES_ANDROID", "0")`。每个包带页面 token 和通道世代，旧页面的包两个方向都进不了新页面；Go 拒收上行包时页面立即改走消息通道；子框架的消息一律丢弃。`@tangoui/runtime` 内置对应的通道模块。只在模拟器上测过，没有真机数据；见 [docs/mobile.md](docs/mobile.md)。
  - 自测：`examples/duplex-ios` 改名为 `examples/duplex-mobile`，同一份自测也在 Android 模拟器上跑（`scripts/smoke.sh android-selftest`、`android-lanefault`）。Android 测试钩子构建（`-tags tango_selftest`，可调试 APK）读启动参数 `TANGO_TEST_ENV`（`am start … --esa TANGO_TEST_ENV TANGO_A=1,TANGO_B=2`）和 `TANGO_ANDROID_NO_ARRAYBUFFER`，见 [docs/dev-builds.md](docs/dev-builds.md)。
  - 内部接口（`internal/platform`，不影响应用）：平台通道的包带世代：`BinaryLane.SendLanePacket(gen, p)`、`LaneHost.OnLanePacket(id, origin, gen, p) bool`（返回是否投递），`lane.State.AllowUpGen` 取代 `AllowUpSession`，平台上行的空包不计入故障注入计数。
- **终端视图**（`ui/terminal`）：`terminal.View` 把 `Terminal` 画成可交互的自绘终端（选择、滚动、链接 `OnOpenLink`、`OptionAsMeta`、`FontSize`）；配色 `Scheme` 和内置的 `SchemeDark`、`SchemeLight`、`SchemeDracula`、`SchemeSolarizedDark`、`SchemeSolarizedLight`，`ResolveColor` 把 `vt.Color` 解析成主题色；`vt.Screen.Evicted` 报告被挤出回滚区的行数。
- 示例 `examples/terminal`：本地 shell 的终端窗口（`-shell`、`-scheme`、`-font-size`、`-renderer`），无伪终端的平台（iOS、Android）自动改用回显连接，`-replay` 回放录制的输出；手机上视图留在安全区内、软键盘上方。`e2e_test.go` 是 M1 的实机验收（macOS），报告见 [docs/specs/terminal-m1-report.md](docs/specs/terminal-m1-report.md)。
- `ui/theme`：组件令牌 `TerminalToken`（`Components.Terminal`、`CompTerminal`），默认取 xterm.js 的 ANSI 调色板；HighContrast 会把对比度不够的调色板颜色调亮，对比度检查支持数组字段。
- `ui`：`Context.ClaimEscape`：获得键盘焦点的组件（例如终端）在构建时声明后，键盘 Esc 交给它，而不是关闭浮层；Android 系统返回键不受影响，照常关闭浮层。
- `ui/input`：`KeyBack`，Android 系统返回键的 `Code`（`Key` 仍是 `KeyEscape`，所以已有的 Esc 处理照常工作；返回键的抬起不会送给占用了 Esc 的组件）。
- `ui`：`ConfigProvider.Defaults` 覆盖所有组件，按组件名和属性名设置默认值（新增 `Button.AutoInsertSpace` 及一批可通过 `Defaults` 设置的属性，见 `api/go/ui.txt`）。
- **shadcn/ui v4 主题**：`theme.ShadcnOptions.Color` 叠加 v4 的 17 个彩色主题（`theme.ShadcnColors`：amber、blue、cyan……yellow），覆盖主色、次要色、图表色和侧栏主色；`theme.ShadcnBases` 增加 v4 的 mauve、olive、mist、taupe。
- `theme.ParseOKLCH` / `theme.OKLCH`：解析和转换 CSS `oklch()` 颜色（支持百分比亮度、`none` 色相和 `/ alpha`，色域外分量裁剪）；`theme.ParseColor` 也接受 `oklch()`。
- `internal/cmd/shadcngen`：从 shadcn/ui 的 `apps/v4/registry/themes.ts` 和 Tailwind CSS v4 的 `theme.css` 生成 `ui/theme/shadcn_data.go`。
- **默认更新界面** `service/updater/updateui`（实验性）：`updateui.Start(app, u, …)` 一行接上现成的更新窗口：后台定时检查、更新说明、下载进度（补丁优先）、立即重启或下次启动时安装、跳过此版本；`CheckNow` 给"检查更新…"菜单用。WebView 与自绘应用都能用，跟随主题与语言。见 [docs/updates.md](docs/updates.md#默认更新界面updateui)，示例 `examples/update-demo`。
- `tango.SurfaceOptions.Auxiliary`：辅助窗口（更新窗口、工具面板等）不算作"最后一个窗口"，`QuitOnLastClose` 下最后一个普通窗口关闭时应用照常退出，辅助窗口随之关闭，关掉辅助窗口本身不会让应用退出；零值保持原行为。见 [生命周期](site/guide/concepts/lifecycle.md)。
- **组件规范对齐 第 1 批：基础和表单输入**（均为实验性 API；逐组件对照记录见 [docs/specs/align/](docs/specs/align/) 的 `1-basic.md`、`1-input.md`、`1-select.md`、`1-datetime.md`）：
  - 基础与布局：Button `IconPlacement`（antd v6 iconPlacement，Defaults `iconPlacement`）、`LoadingIcon`、`LoadingDelay`；Link `UnderlineMode`（Defaults `underline`）、`Icon`、`Type`；Divider `Vertical`、`TitlePlacement`、`Variant`、`Size`（Defaults `titlePlacement`、`variant`、`size`、`vertical`、`orientation`）；Space `Orientation`、`Separator`（`Split` 弃用）、Flex `Orientation`；GridRow `VGutterAt`；Segmented `ItemRender`（自定义每项内容）、`Tooltips`、`Orientation`；Toggle / ToggleGroup `Variant`，ToggleGroup `Spacing`；Rate `Small/Large`、`Keyboard`、`Tooltips`、`CharacterFunc`、`Colors`、`Thresholds`、`VoidColor`、`Icons`、`ScoreFormat`；令牌 `ButtonToken.AutoInsertSpace`、`DividerToken.TextFontSize/MarginBlockSM/MarginBlockMD`、`TypographyToken.LinkFontWeight/LinkUnderline/LinkHoverUnderline`、`RateToken.StarSizeSM/StarSizeLG/HeightSM/HeightLG`。
  - 输入与表单：InputNumber `Mode("input"|"spinner")`（antd 6，Defaults `mode`）、`Controls`、`ReadOnly`、`Keyboard`、`StepStrictly`、`UpDisabled/DownDisabled`；OTP `Separator`、`SeparatorFunc`、`Variant`、`ReadOnly`；Search `Variant`；Input `VisibilityToggle`；TextArea `Variant`、`ReadOnly`；Compact `Size`、`Orientation`；Form `RequiredMarkOptional`（文案 `Locale.Form.Optional`）；RadioGroup `Vertical`、`Orientation`、`Block`；Switch `ActionIcons`；Slider `Reverse`、`Included`、`Keyboard`、`ChangeComplete`；Upload `ShowUploadList`；令牌 `InputNumber.SideHandleWidth*`、`InputNumber.HideControlsInactive`、`Input.TextAreaMinHeight`、`Form.LabelOptionalColor`、`Radio.DotHoverColor/DotInnerColor`、`Slider.HandleActiveOutlineWidth`。Defaults 的 `variant`：OTP、Search 回落到 `Input` 键，TextArea 只读 `TextArea` 键（同 antd）。
  - 选择器：Select `Placement`、`LabelRender`、`NotFoundContent`、`OpenChanged`（Defaults `variant`、`placement`）；MultiSelect `Placement`、`NotFoundContent`、`OpenChanged`、`MaxTagPlaceholder`、`MaxTagTextLength`；VirtualSelect、AutoComplete、Cascader、TreeSelect `Placement`、`NotFoundContent`（Cascader、TreeSelect 另有 `MaxTagPlaceholder`，Cascader `Separator`）；Mentions `Split`、`Placement`；InputTag `Trigger`；Transfer `Actions`（antd 6 actions；`Operations` 弃用）、`TargetOrder`、`ShowSelectAll`。shadcn NativeSelect、Combobox 对应已有的 `Select`（`Searchable`）/ `MultiSelect` / `AutoComplete`，不新增入口。
  - 日期时间与颜色：DatePicker 系列 `MinDate`、`MaxDate`、`ShowWeek`、`ShowNow`；日期、时间、颜色选择器 `Placement`（Defaults `placement`）；RangePicker / TimeRangePicker `Order`、`Separator`；TimePicker `NeedConfirm`；DateTimePicker `DefaultTime`；TimeSelect `IncludeEndTime`；ColorPicker / ColorPickerPanel `DisabledFormat`；令牌 `DatePickerToken.NavDisabledOpacity/NavDisabledColor`。
- **组件规范对齐 第 2 批：数据展示和表格**（均为实验性 API；逐组件对照记录见 [docs/specs/align/](docs/specs/align/) 的 `2-table-look.md`、`2-table-api.md`、`2-data.md`、`2-display.md`、`2-media.md`）：
  - 表格：`TableColumn.SortDirections`、`DefaultFilter`、`FilterResetToDefault`；Table `SortDirections`、`ShowSorterTooltip`、`PreserveSelectedRowKeys`、`CheckLinked`、`SelectionColumnWidth`、`SelectionFixed`、`ExpandRowByClick`、`HideExpandColumn`、`DefaultExpandAllRows`、`HideHeader`、`HeaderHeight`（EP TableV2）、`Title`、`Footer`、`PaginationPlacement`（`PageTopStart`…`PageBottomEnd`、`PageNone`）、`PaginationConfig`、`CurrentRowKey`/`CurrentChanged`、`ShowSummary`（文案 `Locale.Table.SumText`）、`ShowOverflowTooltip`、`ScrollToFirstRowOnChange`，Shift+点击按段勾选；Defaults `Table` 新键 `checkStrictly`、`preserveSelectedRowKeys`、`expandRowByClick`、`defaultExpandAllRows`、`showExpandColumn`、`showHeader`、`showSorterTooltip`、`indentSize`、`scrollToFirstRowOnChange`、`highlightCurrentRow`、`showOverflowTooltip`、`showSummary`；约 35 个 `TableToken` 外观令牌（排序、筛选、展开图标、选择列、底角、页脚区、滚动滑块、EP TableV2 行高等）。
  - 树、列表、卡片、折叠：Collapse `ExpandIconPlacement`（`IconEnd`/`IconStart` 弃用）、`Collapsible`（含 `CollapseItem.Collapsible`）、`ExpandIcon`、`BeforeCollapse`；Card `Variant`（`Borderless` 弃用）、Defaults `Card.type: "inner"`；Descriptions `DescItem.Filled`、`DescItem.Spans`；List `ItemLayout("vertical")`、`ItemExtra`、`PaginationPosition`、`PaginationAlign`；Tree `Selectable`、`Disabled`、`DefaultExpandAll`、`Accordion`、`CheckOnClick`、`CheckOnClickLeaf`、`SwitcherIcon`、`FilterMethod`、`AllowDrag`、`AllowDrop`、`EmptyText`；VirtualTree `Accordion`、`CheckOnClickLeaf`、`EmptyText`；对应 Defaults 键与 Collapse/Card/Descriptions/List/Tree 令牌。
  - 标签、徽标与状态：Tag `Variant("filled"|"solid"|"outlined")`（`Borderless` 弃用）、`Disabled`、`CloseIcon`、`Round`、Ant 预设色名、shadcn Badge 变体 `ghost`/`link`；`CheckableTagGroup`；Badge `Hidden`、`Title`；Avatar `Small`、`Large`、`Shape`、`Gap`，AvatarGroup `MaxPopover`；Statistic `GroupSeparator`、`DecimalSeparator`、`Formatter`；Empty `Title`；Skeleton `AvatarSize`、`TitleWidth`、`ParagraphWidth`、`Count`；Progress `GapPlacement`、`PercentPosition`、`RailColor`（`TrailColor` 弃用）、`ColorFunc`、`StepsGap`；Tooltip `Open`、`OpenChanged`、`Disabled`；Popover `MouseEnterDelay`、`MouseLeaveDelay`、`Arrow`；对应令牌（`AvatarToken.TextAutoScale` 等）。
  - 标签页、时间线与媒体：Tabs `TabPlacement`、`Indicator`、`TabBarGutter`、`Stretch`、`BeforeLeave`、`TabClicked`、`Variant("line")`（shadcn）、`AddIcon`/`RemoveIcon`/`MoreIcon`，`Tabs(c, nil, items)` 允许 nil；Timeline `Orientation`、`Variant`、`TitleSpan`、Mode `"alternate-reverse"`，`TimelineItem.Title`（`Label` 弃用）、`Placement`、`Loading`、`Center`；Image 预览 `OpenPreview`、`PreviewSrc`、`MinScale`/`MaxScale`/`ScaleStep`、`Movable`、`MaskClosable`、`Infinite`、`ShowProgress`、`CloseOnEscape`；Carousel `DotPlacement`（`DotPosition` 弃用）、`DotOutside`、`DotDuration`、`PauseOnHover`、`CardScale`、`Trigger`；Calendar `ValidRange`、`ShowWeek`、`ControllerType`、`Formatter`；QRCode `BoostLevel`、`MarginSize`；Watermark `MarkSize`、`FontStyle`；Masonry `LayoutChanged`；对应 Defaults 键与令牌。
  - Link 自己的颜色令牌 `TypographyToken.LinkTextColor/LinkTextHoverColor/LinkTextActiveColor`（零值回落到 `LinkColor*`）。
  - 第 2 批复审新增的实验层令牌：`TimelineToken.DotOutlined`、`BusyTailDashed`；`TagToken.PlainColor`、`PlainBorderColor`；`ProgressToken.ShowInfo`；`TableToken.SelectionBorderedExtra`、`SorterTooltip`。

- **组件规范对齐 第 4 批：Pro Components v3**（新独立组件及 Pro 类型扩展均为实验性；稳定类型上的新增成员仍按白名单承诺）：
  - 表格：`CellEditorTable`、`RowEditorTable` 复用 EditableProTable 编辑状态，支持保存、取消、同步/异步校验；`TableDropdown`、`ListToolBar`（ProTable 复用工具栏）；`ProEnum.Disabled`。
  - 布局：`ProBreadcrumb`、`PageLoading`、`TopNavHeader`；结构化 `RouteInfo.BreadcrumbItems`、`ProLayoutConfig.NavTheme/FixedHeader`，ProList 受控/默认展开选项。
  - 表单：ProFormCaptcha 对应已有 `ValueCaptcha` + `ProField.Captcha`（手机号校验、自定义倒计时文案/回调）；ProFormSegmented 对应已有 `ValueSegmented` + `ProField.Segmented`（禁用选项、自定义内容）；不重复新增同义构造函数。
  - 卡片：ProCard `BoxShadow`；ProDescriptions `ColumnAt/Columns`；StatisticCard 布局和独立 `ProStatistic`、`StatisticCardDivider`、`StatisticCardOperation`；CheckCard/Group 非受控值、嵌套组、自定义内容和操作。
  - gallery：登录、查询列表、基础表单、详情、结果、设置抽屉六种完整 Pro v6 页面组合，全部使用真实 Tango API 和本地示例数据。复核旧 28 项的结果与尚缺能力见 `docs/specs/align/4-*.md`，不再将其解释为全部 v3 API 已完整。

### 变更

- **删除 `down:scheme`**：单个长流式响应的下行通道（`GET /__tango/lane/down`）和 runtime-js 的对应模块删除，由 `down:poll` 取代；它在 Apple 上本来就因 WebKit 扣住大块突发而停用，其他平台没有接入。`TANGO_IPC_LANE_FAULT=down:scheme:N` 改为 `down:poll:N`（把带第 N+1 个字节包的轮询响应截断）。
- **`TANGO_IPC_LANES_DARWIN` 新取值**（macOS）：`up`、`down` 只开其一（排查用）；`1` 和 `all` 都表示 `down:poll` + `up:post`。**`all` 含义改变**：以前只在开发构建里开实验性的 `down:scheme`、生产构建等同默认，现在在任何构建里都开 `down:poll` + `up:post`。iOS 上 `all` 与 `1` 相同（`up:post`），也不再依赖自测构建。
- **`Surface.Eval` 与 `Send` 对任意 goroutine 都保序**：页面加载完成后，`Send` 的消息按序号进下行序列；`Eval` 只在当前页面开启了 `down:poll` 时进同一序列，作为栅栏，页面把它之前的消息交付完才执行（多一次页面到 Go 的往返，约 0.06 ms；通道空闲时直接走 `Eval`，没有这笔开销）。没开 `down:poll` 的页面（iOS、Windows、Linux、Android，以及默认的 macOS）`Eval` 不带序号，仍是一次普通的 `evaluateJavaScript`，与 `Send` 经同一个出队队列按调用顺序交给页面。所以"与 `Send` 保序"的承诺对任意 goroutine 都成立。主线程上调用 `Send`/`Eval` 且走 `Eval` 路径时，返回前已经交给页面（与以前一致）。
- **与页面导航赛跑的跨 goroutine `Send`、`Eval` 会被丢弃**：页面加载完成后，其他 goroutine 的 `Send` 和 `Eval` 现在调用时就进入当前页面世代的下行队列（以前先投递到主线程，执行时才看页面状态）。页面开始导航（用户点链接、重新加载，`SurfaceNavigating`；或 `Navigate`）时当前世代结束，已排队但还没交给页面的消息和脚本随之丢弃。以前这样的消息如果在导航开始后才轮到主线程执行，会暂存起来交给新页面，脚本则在新页面里执行；现在都不会。导航开始之后调用的 `Send` 照旧暂存，新页面加载完成后交付；这期间调用的 `Eval` 照旧在主线程上对当时的页面执行。需要新页面收到的消息或执行的脚本，在新页面加载后（例如 `Surface.OnLoaded` 的回调里）再发。
- **macOS 自绘 UI 减少默认字体常驻开销**：同一可变系统字体的各字重共用一份解析后的字体数据，只各自保留字形缓存和变体坐标；HVAR 字宽变体的专用化改为字体 Face 被整形超过 64 次才做（整形结果不变）。等宽行缓存的快路径改用按 Face 和坐标做缓存键的整形器，各字重仍按自己的坐标整形。
- **组件规范对齐 第 4 批：默认值和交互直接改为 Pro v3 规范**（逐项来源和测试见 `docs/specs/align/4-*.md`）：
  - Ant ProCard 默认无边框 → outlined（所有主题未显式设置时 bordered=true）；显式 Borderless、ghost、split 子卡仍无边框。依据 Pro v3 Card/index.js variant 默认 outlined。已有 `TestShad2dProCardAntUnchanged` 保留半径断言并新增 outlined 期望，未删用例。
  - ProSkeleton statistic 默认固定 4 → 2；遵循 Pro v3 浏览器实际行为，而不是 lookup 表表面上的 2/4/6。显式 Statistic(n) 和 Defaults statistic 保持优先。
  - 基础 Descriptions / ProDescriptions 默认固定 3 列 → xs 1、sm 2、md 起 3 列（Tango 无 xxxl）。窄宽度下布局行数会增加；显式 Column / Defaults 数字仍保持指定列数。
  - ProSkeleton descriptions 原固定 4 个总表格行且忽略 List → 默认 4 正文行 + 表头；List(n) 指定正文行，List(0) 隐藏分隔线/表格；描述项列数固定 3 → xs 1、其余 2，表格另有 flex-3 尾列。
  - ProSkeleton list 页 List(0) 原连 toolbar 一起隐藏 → 只隐藏 rows；Toolbar(false) 独立隐藏工具栏。已有 `TestPro2ProSkeleton` 的“全空”调用增加 Toolbar(false)，不删除原用例。
  - StatisticCardGroup 默认 SplitVertical → 无 split、body padding 0（Pro v3 Group 的真实定义，TestStatisticCardGroupDefaults）。旧用法要分隔线可显式 `.Split(SplitVertical)`。
  - StatisticCard 未赋值时显示空字符串 → 显示 0（antd/statistic/Statistic.js `value = 0`，TestStatisticCardDefaultValue）。
  - StatisticCard.Value 的数字格式化由 FormatNumber（四舍五入）→ statisticFormat（按 antd 截断精度），需要自定义格式可用 ValueText。
  - 移除 StatisticCard / CheckCard 内部按设计语言分支：所有主题标题用 Pro 的正文色、footer 保持 Pro 分隔线、CheckCard 保持 Pro 角标；字号、颜色、内外边距继续来自令牌。
  - StatisticCard 标题色 `TextSecondary` → `Text`（Pro v3 `pro/card/components/Statistic/style.js`）。
  - StatisticCard 趋势：数值随趋势染红/绿（原只染三角）；三角 12×8 → 7×9（同上 `&-trend-icon`）。
  - StatisticCard `Desc` 小指标：横排换行 → 竖排间距 4；字号 14 → 12（fontSizeSM）；标题次要色 → 正文色，值正文色 → 次要色；三角移到值前（同上 `& &-layout-inline`、`& + &`）。
  - StatisticCard tip 图标与标题间距 6 → 4；状态点列宽 12 → 14；`Description` 上边距 8 → 0。
  - StatisticCard 左右图表间距 24 → 16 且顶对齐（原垂直居中）；底部图表下方增加 8（`pro/card/components/StatisticCard/style.js` `&-chart`）。
  - StatisticCard footer 上外边距 16 → 8（同上 `&-footer` marginXS）。
  - CheckCard 圆角 RadiusLG → Radius（`pro/card/components/CheckCard/style.js` `borderRadius`）。
  - CheckCard 内容内边距：随尺寸 12/16/20 四边 → 横 12、竖 16（`&-content`）。
  - CheckCard 选中角标 12×12 贴角 → 20×20、距顶/末端 2、带上末端圆角（`genCheckedAfterBase`）。
  - CheckCard 禁用背景 FillTertiary → BgContainerDisabled，禁用选中边框 Primary.BorderHover → Border，禁用头像透明度 0.25（`proCheckCardDisabled`）。
  - CheckCard Large 标题字号 FontSizeLG → FontSize，标题色 → TextHeading（`&-title`）。
  - CheckCard 头像：32/40/48 随尺寸、圆形 → 固定 48、方形；只有头像和标题时垂直居中（index.js `Avatar size=48 shape=square`、`-avatar-header`）。
  - CheckCard 封面内边距 8 → 4（`&-cover` paddingXXS）。
  - CheckCard loading：骨架 3 → 4 行、内边距 20；loading 时不响应点击；CheckCardGroup loading 时卡片不显示选中（Group.js、index.js）。
  - ModalForm/DrawerForm 默认宽度：基础Modal/Drawer默认 → 800（v3对应index.js）。
  - QueryFilter 列断点：560/900/1500 → 576/768/1600；窄屏折叠时0字段 → 至少1字段（v3 getSpanConfig/showLength）。
  - ModalForm/DrawerForm 默认首次打开/重开清空 → 保留值；新增 DestroyOnHidden(true) 显式清空（v3默认不销毁）。TestModalFormClosesAfterSubmit 末尾重开期望从空字符串改为 x，与本行为修正对应；未删用例。
  - ProForm 默认提交按钮顺序 submit/reset → reset/submit（Submitter/index.js）。
  - ProLayout/DefaultProSettings：默认深侧栏→light，fixedHeader true→false（`layout/defaultSettings.js`）。需要深侧栏请显式 `NavTheme:"dark"`。
  - GridContent：Fixed上限1200→1152；在side/mix不再收窄（源码 `GridContent/style.js` 而非过时1200注释）；独立显式Fixed/MaxWidth保留。
  - PageContainer loading改用PageLoading（原minHeight200居中）；空页头不再占位，默认继承RouteContext的标题和面包屑；PageContainer存在时由其独占内容页头，避免top/mix重复自动标题与面包屑。
  - ProList 默认折叠→全展开；checkbox移动到expand图标前；content由标题右侧→下方；折叠同时隐藏description/content；grid卡片actions由body底部→header右侧。
  - SettingDrawer宽320→300；配色8→9补daybreak；切top自动Fixed，其他Fluid，mix强制light；top的fixed sider禁用。
  - TopNavHeader统一复用原普通header并消费新增独立Pro token：title18→16、logo/title间距12→6、logo行64→56、菜单padding6；Sider默认Ant208→256（mix215），基础Layout/Header/Sider默认令牌不改。shadcn top不再单独绘制shadBrandMark。
  - PageContainer Ant inline24→40、body block24→32、header top16→8/bottom16、children top24→0（无header时32）、背景surface→透明；FooterToolbar高56→min64；DefaultFooter gap40→8、margin top48/bottom24、inline16。以上均有Pro v3源码与共享token依据。
  - ProTable 全屏选项：默认显示 → 默认隐藏（pro/table/components/ToolBar/index.js 合并 options 时 fullScreen:false）；需要显示时调用 FullScreen(true)。主线程已为 a11y/i18n 原全屏用例显式开启保留测试覆盖。
  - ProTable toolbar：内部单独绘制 → 复用公开 ListToolBar；标题 Ant 原 Medium500 → Semibold600（ListToolBar style fontWeightStrong），组合增加8px统一间距；基础 Table 的外观/行为未改。
  - 密度菜单：鼠标打开时 hover tooltip 撤销可能导致首次键盘焦点丢失 → 初始焦点请求保持到菜单首次稳定绘制，且先回焦点再处理漫游键盘事件。
  - 基础 Table 内置筛选 Reset 修正：只改草稿并清搜索、面板不关闭，不立即 applyFilter；FilterResetToDefault 时，空草稿也允许恢复默认，草稿等于重置目标时 Reset 禁用。新增回归 `TestTableFilterResetOnlyDraftV3`（恢复默认和清空两分支）、`TestTableFilterResetRestoresDefaultFromEmptyDraft`。默认值不变，其余 Table 外观和行为不变。
  - 共享视觉令牌：ProCard small body 纵向内边距 8 → 12，hover 用三级阴影和透明边框（BoxShadow 卡片 hover 时阴影不变）；LoginForm 标识间距 12 → 16、标题字重 500 → 600、描述下间距 32 → 40。ProLayout / PageContainer / DefaultFooter / FooterToolbar 改读独立 Pro 令牌，基础 Layout / Header / Sider 外观不变。
  - FooterToolbar 默认改为固定在窗口或 Target 底部（Pro v3 position:fixed）；`Fixed(false)` 回到文档流，`Sticky()` 保留原到达底边才吸附的行为。App 的 maxCount 改走共享消息关闭流程，被挤掉的消息会触发 OnClose。

- **组件规范对齐 第 1 批：默认值和行为改为规范的样子**（依据见各对照记录；不加保留旧行为的开关）：
  - Button：Ant 主题两字中文按钮默认插入空格（「确定」→「确 定」，antd autoInsertSpace 默认 true；Element、shadcn 不插）。关掉用 `Defaults("Button", Props{"autoInsertSpace": false})` 或 `AutoInsertSpace(false)`；优先级：方法 > Defaults > 主题令牌。
  - Divider：`Orientation` 改为表示方向（horizontal / vertical，antd v6），旧的标题位置值仍可用；`"left"`/`"right"` 标题位置改为物理方向；Ant 带标题分割线字号 14 → 16、高 22 → 24，竖线高 14 → 12.6（0.9em）；shadcn 竖线撑满所在行。
  - Link：Element 主题字重 400 → 500，悬停显示下划线。
  - Layout：Footer 内容不再居中；Ant 亮色主题深色 Sider 触发器底色 #000c17 → #002140。
  - Rate：Ant 星星尺寸由 ControlHeight × 0.625 推导（默认仍 20，紧凑主题变小）。
  - Toggle：Small 内边距 8 → 6；ToggleGroup 默认间距 4 → 0（项相连，只圆外角）。
  - InputNumber：Ant 两侧 / spinner 按钮改为透明底、宽 37 / 29 / 39；Element 两侧按钮图标 14 → 13px；到达 Min/Max 的步进按钮禁用；Ant、shadcn 禁用或只读时不显示步进按钮（Element 保留）；只读时失焦不再改写值。
  - Radio：Ant 选中圆点 8 → 6，选中后悬停用 colorPrimaryHover，暗色内点改白色；Element 禁用选中点 8 → 4，暗色内点改白色。
  - Slider：Ant 悬停手柄描边 4 → 2.5，禁用轨道色改 colorBgContainerDisabled，光晕改为悬停 / 按下 / 聚焦时 6px，禁用时悬停不再放大；竖向 + Reverse 方向键与 rc-slider 一致；Element 禁用色改 TextQuaternary。
  - TextArea：默认 3 行 → 2 行；shadcn 最小高度 64。
  - Upload：`MaxCount(1)` 时新文件替换旧文件（原来到上限就拒绝）。
  - Select：值不在选项里时显示值本身（原来空白）；键盘移动跳过禁用选项；Select / MultiSelect / VirtualSelect `Loading(true)` 时箭头位置显示转圈（设了 `SuffixIcon` 时仍显示自定义图标）。
  - Transfer：新移入的键放在目标切片最前、右侧按目标切片顺序显示（antd；要 Element 的数据源顺序用 `TargetOrder("original")`）；Ant / shadcn 列表默认 200×240 → 180×200；Element 列表高 280 → 318。
  - 日期面板：Ant 日期面板行高 32 → 36（弹层高 28px），月 / 季 / 年面板行高 56 → 63，预设栏宽 110 → 120（shadcn 同）；“今天”/“此刻”在当前时刻不可选时显示为禁用；禁用的翻页箭头显示禁止光标。
- **组件规范对齐 第 2 批：默认值和行为改为规范的样子**（依据见各对照记录；不加保留旧行为的开关）：
  - 第 1 批复审修正：Element 主题 `Link` 默认用常规文字色、悬停变主色（EP `type="default"`）；所有选择器和日期 / 时间 / 颜色选择器的 Defaults `placement` 用同一个解析（`"top"` / `"bottom"` 为居中、大小写不敏感，Mentions 仍从起始边弹出）；`RadioGroup.Block()` 在纵向容器里只撑满宽度、不再撑满高度；Ant Radio 尺寸与 InputNumber spinner 按钮宽改为由令牌推导（默认值不变）；spinner 模式前缀位置按按钮实际宽度；Slider 拖动中被禁用不再补报 ChangeComplete。
  - 主题：Ant 亮色 `ShadowTertiary`（无边框 Card、Segmented 选中项等）.03/.02/.02 → .05/.03/.03（antd 6.7 boxShadowTertiary）。
  - Table：树形多选默认不再父子联动（antd `checkStrictly` 默认 true；联动用 `CheckLinked()`）；选择变化时丢掉数据里已不存在的键（保留用 `PreserveSelectedRowKeys()`）；展开列排在选择列之前；分页默认不显示总数、Small/Middle 表格用小号分页、没有数据时不显示分页；排序 / 筛选 / 翻页后有界表体滚回首行；`ShowSummary` 合计去掉尾零。ProTable / EditableProTable 仍显示总数，但跨服务端分页的勾选会在下次勾选变化时丢掉（与 pro-components 一致）。
  - Table 外观：Ant 选择列 48 → 32 且复选框居中（Element 选择格内边距 14）；Ant 带边框表格底角不圆；Ant 分组表头改为短分隔线、排序悬停箭头 .29 → .57、筛选按钮悬停圆角 4 → 6、筛选下拉圆角 8 → 6、展开图标 17 → 16、滚动滑块 6 → 8px；Ant 暗色行悬停 #262626 → #1d1d1d、表头图标 .25 → .29；Element 排序 caret 间距 3 → 2、筛选箭头 14px、筛选面板圆角 4 → 2、暗色斑马纹 #191919 → #1d1d1d、树形缩进 15 → 16、`Virtual()` 默认行高与表头高 50；shadcn 表头文字改 foreground、汇总行 muted/50 加粗、选中行 muted、外框 rounded-md、筛选圆角 8、无边框分组表头不画竖线。
  - Collapse：Ant 小 / 大尺寸与 ghost / borderless 内边距按 antd（小尺寸内容 8 → 12 等）；手风琴模式初始多个打开键只展开第一个；Element 箭头颜色同标题。
  - Card：Ant 无边框卡片默认带阴影，hoverable 悬停时描边变透明、阴影换成 boxShadowCard，inner 卡片内容内边距 24 → 16 24。
  - Descriptions：放不下的项截到本行剩余列（原来换行）；Ant 行距按尺寸 16/12/8、冒号前 2 后 8、bordered 标签色和边框色按 antd；Element 默认尺寸 large → default、颜色按 EP。ProDescriptions 随之变化。
  - List：条目两端对齐、小 / 大尺寸内边距、分隔线规则、bordered 列表分页移到框内、操作区间距 17 → 16、网格行距固定 16；Listy 分组标题颜色、底色按 antd，去掉下边线。
  - Tree / VirtualTree：Element 空树显示「暂无数据」；VirtualTree Element 缩进 18 → 16。
  - Tag：Ant 默认 filled（无边框）、底色 #fafafa → #f5f5f5，状态色 / 预设色标签去描边，无色 solid 用 colorBgSolid；shadcn Badge 改为 v4（rounded-full、font-medium、左右 7），暗色 destructive 底改为 destructive/60（白字对比 2.89 → ≥ 4.5）；CheckableTag Ant 未选中悬停加底色。
  - Badge：负数照常显示；Ant 多位数气泡左右内边距 6 → 8。Avatar：Ant 各尺寸文字 14、超宽文字按 gap 缩小（Element 不缩）、图标 18/24/14、方形圆角 4/6/8；shadcn 文字 muted-foreground；AvatarGroup「+N」不再用示例桃色。
  - Statistic：精度改为截断；NaN / ±Inf 正常显示。Empty：shadcn 内边距和间距按 v4。Skeleton：带头像或无标题时默认 2 行、头像 32 → 40、Ant 段落间距按 antd、`NoTitle()` 首行无上边距、方形头像修正。Progress：小号环形 80 → 60，NaN 按 0。
  - Tooltip / Popover：悬停延迟按主题（Ant 0.1 s / 0.1 s，Element 0 / 0.2 s）；Tooltip 与触发器的距离 Ant、Element 8 → 12；Popover 默认宽度 Element 150、shadcn 288（AvatarGroup 弹层仍自适应）。
  - Tabs：Ant Large 线条标签上下内边距 12 → 16、Small 卡片高 40 → 32、Large 卡片高 40 → 48；shadcn 暗色选中项 input/30 加描边；`*active` 不在 items 里（含空串）时回落到上次索引或第一个标签。Dock 的标签条不受影响。
  - Timeline：Ant 圆点描边 3 → 2；默认 mode `"left"` 改为同义的 `"start"`。Pending / Loading 的点线轴保持 antd 规则：不 `Reverse` 时是进入忙碌项的一段，`Reverse` 时是离开它的一段（横向同理，Element 不画点线）。
  - Image 预览：Ant 缩放范围 0.25–8 → 1–50、双击 ×2 → ×1.5、关闭按钮距角 32 → 12、按钮 42、工具栏颜色 .65 / .85、方向键到首尾不再循环（shadcn 同 Ant）；Element 缩放 0.2–7、每步 ×1.2、双击不缩放、「复位」切换适应 / 原始尺寸。
  - Carousel：Ant 箭头 28 → 16px、距边 16 → 8、透明度 .6 → .4、到头隐藏，指示点透明度 .3 → .2；Element 悬停指示点即切换；`Current` 越界夹到范围内。
  - 第 2 批复审修正：
    - Timeline：`Pending` / `Loading` 的点线段改回 antd 方向。上一版把方向写反了，普通 `Pending` 的点线尾巴变成了实线。
    - Tag：c04a878 让 Element 的 plain 状态色标签、Ant success / warning 和部分预设色的 plain 标签变成了正文色，现在恢复为类型色；无色 plain 标签改读新令牌 `TagToken.PlainColor` / `PlainBorderColor`（Ant 正文色、Element 主色、shadcn foreground），不再读 `DefaultColor` / `BorderColor`，对这两个令牌的覆盖不再作用于 plain 标签。`Color("blue-inverse")` 这类 antd 反色名改为实心预设色（原先是灰色）；只有 `Variant` 方法能改掉它，`Defaults("Tag", …)` 的 `variant` 不行（与 antd 优先级一致）。
    - Table：选择列宽规则变了。shadcn 选择列 48 → 32；带边框时只有 Ant 再加 16（令牌 `TableToken.SelectionBorderedExtra`），原先 shadcn 带边框也加。选择下拉箭头的预留宽度在所有主题下 18 → 16，Element 带 `Selections` 时列宽 66 → 64。带边框表格的 Title 只描上、左、右三条边，Footer 只描左、右、下三条边，不再出现 2px 线和 #fafafa 色线。Ant Footer 不论是否带边框，底角都改为圆角（`HeaderBorderRadius`）；有 Footer 时表体不再画圆底角。Element 展开列图标盒 17 → 16（读 `ExpandIconSize`）。Shift 区间勾选后的锚点改为这次点击的行。
    - List：分页放在上方时，末项也保留分隔线。
    - Tabs：拖拽激活标签时也经过 `BeforeLeave`。
    - Tabs：`Variant("line")` 不再按主题判断是否生效，改读新令牌 `TabsToken.SegmentedList`（shadcn true：默认是分段轨道，`line` 切成线条标签；Ant / Element false：默认就是线条标签，`line` 外观相同）。`TabBarGutter` 对所有类型生效：shadcn 分段 / 线条列表和 BorderCard 下原先被忽略（antd tabBarGutter）。Ant 默认外观不变。
    - Carousel：首帧遇到越界的 `Current` 时，直接显示夹值后的那一张，不再播一次动画。
    - Calendar：Element `ControllerType("select")` 的头部补上小号「今天」按钮。
    - Tree：禁用的树（`Disabled`、`ComponentDisabled`）不能再拖放；`DefaultExpandAll` 首帧不再让 `ExpandChanged()` 返回 true。VirtualTree 的 Ant / shadcn 缩进令牌在主题推导时取 `IndentSize`（数值不变；只覆盖 `Tree.IndentSize` 的组件令牌不会带动它）。
    - Descriptions：`DescriptionsToken.LabelColor` / `ContentColor` 现在只作用于无边框列表；带边框列表读新令牌 `BorderedLabelColor` / `BorderedContentColor`。只覆盖了 `LabelColor` 的旧代码，在带边框列表上不再生效。Element `LabelColor` 默认 TextSecondary → Text。
    - 主题默认值改由令牌给出（默认外观不变）：`TimelineToken.DotOutlined` / `BusyTailDashed`、`ProgressToken.ShowInfo`、`TableToken.SorterTooltip`。
    - 提示与状态：Alert 默认 `ShowIcon` 改为 false（Banner 仍显示，框架内部错误提示显式保留图标）；Spin/Loading 的 description、delay、percent、遮罩与 shadcn 尺寸按规范；Message 悬停暂停、Grouping 后执行 maxCount；Notification 的 actions、关闭按钮、role 与进度条令牌化；Result 副标题间距；Skeleton throttle 与 Empty 的插画默认改由主题令牌决定。
    - 浮层与对话框：Modal/Drawer/Popconfirm/Tour 的关闭按钮、间距、遮罩、焦点与目标交互按 antd/EP 对齐；Popover/Popconfirm 箭头与偏移改由令牌决定（Ant 有箭头 10 → 12）；shadcn Sheet 512–640 宽度断点和上下方向自适应高度暂不做。
  - 间接受影响的页面：PageContainer 页头（Descriptions、Tag）、ProLayout / GridContent 头像、Segmented 选中项、chat 气泡与 graph 节点（`ShadowTertiary`）、FooterToolbar 示例里的 Skeleton 等随上述组件变化。
  - QRCode：默认提升纠错级别（同一内容的码图会变）、边框色改 Split、无边框去掉内边距、空字符串不画码。Watermark：排布改为 antd 算法，图片标记随角度旋转、默认不透明度 .2 → 1。
- macOS：`KindPanel` 窗口不再出现在 Cmd+\` 窗口切换和“窗口”菜单里，与 Windows（工具窗口）和 Linux（跳过任务栏与切换器）一致。
- macOS：`AcceptFiles` 的 WebView 窗口在拖动经过时把悬停交给页面，页面会收到 `dragenter` / `dragover` / `dragleave`，与 Windows、Linux 一致，可以用来高亮放置区域。松手时文件仍交给 `OnFilesDropped`，页面收到的是 `dragleave`。
- **IPC 线协议升级到 v2，不兼容旧版 runtime-js**：握手带 `proto:2`，Go 拒绝其他版本（回 `runtime-js protocol mismatch` 并记 `slog.Warn`）。`@tangoui/runtime` 必须与 Go 模块同版本；自己实现协议的客户端需要按 `ipc` 包注释更新。流的背压从按条数改为按字节计的信用窗口（默认 1 MiB，每帧计负载 + 64 字节），`ack` 改为带归还字节数 `b`；单帧负载上限 16 MiB，超过时这次写入返回 `bad_args`。
- **macOS/iOS 的资源处理函数改在主线程之外运行**：`tango://` 协议处理器改为异步，`Options.Assets` 的处理函数每个请求在自己的 goroutine 里运行（请求可能并发处理），不再在主线程上同步执行。大文件、SSE 等大的或长时间不结束的响应不再卡住界面；WebKit 取消请求时 `r.Context()` 被取消，阻塞中的写入返回 `context.Canceled`。处理函数里若直接访问只能在主线程用的状态，改用 `app.Post` 投递到主线程或加锁；`tango.Await` 也可以，但它会一直阻塞到主线程执行完，这个请求的响应也跟着等（平台契约一直允许在任意线程调用处理函数）。
- **`Stream.Close` 语义改为优雅全关闭**：本方结束发送并不再接收，对方之后的写入返回 "stream closed by peer"。只想结束本方发送、继续读的用新增的 `CloseSend`。处理函数返回 nil 时 Router 发的就是这个 `close`。
- **shadcn 主题改为 shadcn/ui v4**（对照 `shadcn-ui/ui@e62c943` 与 `tailwindcss@4.3.5`，见 [docs/design-baseline.md](docs/design-baseline.md)）：颜色改用 v4 发布的 oklch 变量值；默认圆角从 8px（0.5rem）改为 10px（0.625rem），按钮和输入框随之为 8px、卡片 14px；暗色下的边框和输入框是半透明白色（`th.Shadcn.Border` 等保留半透明值，`CSSVars` 导出 `rgba()`，不透明的主题令牌按背景合成）；暗色侧栏主色为蓝色；`--destructive-foreground` 在 v4 中已删除，危险色上的文字固定为白色。`slate`、`gray` 两个底色 v4 已不再提供，作为旧名保留 v3 的颜色。要回到旧圆角可以传 `theme.ShadcnOptions{Radius: 8}`。

### 移除

- Go：`ipc.DefaultWindow`（按条数的窗口），由 `ipc.DefaultWindowBytes` 取代。
- JS：`WINDOW`、`ACK_EVERY` 导出，由 `DEFAULT_WINDOW_BYTES` 取代。

### 修复

- macOS 自绘窗口：停顿之后的输入（打字、滚动、点击）不再等显示刷新的下一拍才出帧，而是在请求它的那一轮 run loop 里立即画；连续变化的帧仍由 CADisplayLink 控制节奏，两拍之间最多立即出一帧。真实窗口实测（1920×944 编辑器，Metal），修复前的中位延迟随窗口服务器的合成状态在约 48 ms 和约 61 ms 之间切换，两种状态下分别做了修复前后 AB/BA/AB 配对，都快了约一帧：约 48 ms 状态下打字 47.5 → 32.1 ms、三光标 54.0 → 32.7 ms、滚动 48.6 → 30.7 ms；约 61 ms 状态下打字 61.5 → 44.8 ms、三光标 61.7 → 43.5 ms、滚动 62.9 → 46.8 ms。
- `HotkeyInput`：录制期间窗口快捷键、原生菜单加速键及 Menubar 的 Alt/F10 不再抢走按键；确认、取消、失焦、禁用或移除后自动恢复，保持输入法关闭。`tan dev` 同步录制状态，热重载或 worker 退出后不会遗留快捷键拦截；系统保留键和 OS 级全局热键不在拦截范围内。
- WebView 菜单图标：`menu.popup` 的 base64 PNG 先用 `DecodeConfig` 校验，像素总数超过 4M 或格式异常时拒绝，避免声明超大尺寸导致解码分配爆炸。
- WebView 资产安全：Darwin 自定义 scheme 的请求体读取移出主线程并限制为 64 MiB；带有跨源 `Origin` 的资产请求（包括 GET/HEAD）在进入应用 handler 前拒绝。
- 日志 ANSI 解析：SGR 参数改为无分配游标解析，保留分号与冒号颜色语法；带颜色日志行的解析分配从 7 次降到 1 次，追加路径从 7 次降到 3 次。
- Metal 内存：大块上传和实例缓冲区在连续 120 个低用量已提交帧后按近期需求缩小或回收，周期性大上传保留容量；重建保留当前打包数据，失败时继续使用旧缓冲区。
- CPU 文字绘制：字形完全处于不透明裁剪区域时复用缓存蒙版，避免逐字形临时分配；百万行日志测试每帧分配次数约少 60%。本轮耗时区间重叠，不额外宣称 CPU 加速，见 [第二轮性能记录](docs/perf/2026-11-16-memory-logview-round2.md)。
- 自绘 UI 内存：未使用的渐变与临时补丁纹理改为按需分配，大块补丁纹理在连续 120 个已执行帧未使用后回收；CPU 图层弹出时释放对旧图像和蒙版的引用，避免窗口缩小后仍保留大图像。
- 日志视图 CPU：圆角裁剪蒙版复用每行非圆角区域的覆盖率计算，保留逐像素结果；百万行日志追加并绘制的同机三轮基准耗时中位数从 3.47 ms 降至 0.72 ms。测试口径与内存实测见 [性能记录](docs/perf/2026-11-16-memory-logview.md)。
- 编辑器换行定位：短行密集的 rope 叶子里先按换行数量二分跳过半段，再逐个查找；稀疏叶子与长行保留原路径，不增加内存或分配。固定工作量的 1000 光标 Enter 线程 CPU 点估计减少约 40%（负载约 10.7，不等同屏幕输入时延），见 [性能记录](docs/perf/2026-11-16-editor-main-round1.md) 第 6 节。
- 编辑器行读取：同叶子内的行只定位一次，避免重复遍历 rope 和扫描换行；不增加节点字段、常驻行索引或公共 API，跨叶长行保留原拷贝路径。同时完整清理小地图 token scratch 的容量，避免旧语法字符串被尾部引用保留。固定工作量的 1000 光标 Enter CPU 点估计减少约 12%，不等同屏幕输入时延，口径和验证见 [性能记录](docs/perf/2026-11-16-editor-main-round1.md)。
- 编辑器输入与滚动的临时分配减少：小地图复用 token 查询 scratch，字符位图不再分配临时格子数组，纯 RE2 长行不再预先构造回溯用 rune 索引；保留默认高亮、小地图与像素结果。实测 TS20k 输入/滚动的每帧 Go 分配字节分别减少约 26%/40%；整帧时延仍受 GPU/调度噪声影响，不宣称等比例提速，测量口径和剩余门槛见 [性能报告](docs/specs/2026-11-16-editor-input-scroll-perf-report.md)。
- 应用体积：自绘确认框、右键菜单、消息、通知改为首次使用时按窗口自动安装渲染器，让未使用的实现可被链接器裁掉，保留 API、固定层序与同帧显示。最小自绘样本应用有效载荷减少约 377–419 KiB；实际收益取决于功能使用情况。Linux tar.gz 改用最高 gzip 压缩级别；四样本下载包减少约 2.6%–6.3%。并非所有格式都变小，默认自绘模板 dmg 本轮增大约 25 KiB，完整口径与数据见 [体积优化实测](docs/package-size-optimization.md)。
- 补齐 `ui/text.ShapeInto` 已有公开符号的 API 快照（实现来自 `38d0c1b`，本轮不改签名）。

- 编辑器：短文档的重新缩进、去尾空格等语法相关命令，不再仅因线程调度停顿耗尽 4ms 墙钟预算就直接放弃；截止时间检查前允许一小段有界分词进展，仍保留总行数、字节数、长行截断和病态正则保护，不采用不完整分词结果执行危险编辑。
- Linux GL：仅装 EGL/Mesa/libepoxy、缺少 `libGL.so.1` 与 `libOpenGL.so.0` 时，GPU 初始化返回不可用错误，允许上层退回 CPU，不再被 libepoxy 的延迟加载直接终止进程。
- `tan dev`：控制连接的登记与 `Welcome` 发送在同一把锁下完成，避免刚握手成功时遗漏热加载完成的 `Swapped` 通知。
- `sys/pty`（Windows）：关闭 ConPTY 前先读完剩余输出，不再丢掉最后一段；文档说明 Windows 上的 EOF 行为。
- **水波图文字**：`chart.Liquid` 水位过半时，数值和标题的颜色按合成后的水色明暗选白色或深色，不再固定为白色（shadcn/ui v4 neutral 的 chart-1 是浅灰，白字看不清）。
- Android：资源响应不再带两个 `Content-Length`（以前是 `585, 585`，流式响应是 `0, 3145728`）。WebView 自己按响应体流的 `available()` 设置这个头，后端改为通过它报告处理函数的长度，不再另传头；HEAD 响应报告 GET 的长度。资源处理函数 panic 时返回 `tango: asset handler panic: <原因>`，与 macOS/iOS 一致（以前是 `tango: asset handler panicked`）。
- **FooterToolbar 吸底回退**：渲染器不支持背景模糊时，吸底工具栏改用不透明的 `BgContainer`，避免底部内容从工具栏后穿透；HighContrast 下也保持不透明。

### 安全

- **快速通道的测试开关**：macOS 上流式探测路由（`TANGO_SCHEME_PROBE=1` 时的 `/__tango/probe/stream`）、协议处理器的通道计数器只编进开发构建（`-tags tango_dev`），生产构建忽略它们（`TANGO_IPC_LANES_DARWIN=all` 以前也属于这一类，现在是普通取值，见「变更」）。iOS 上这些钩子和更早就有的画布自测开关（`TANGO_CANVAS_IME_SELFTEST`、`TANGO_CANVAS_A11Y_SELFTEST`）只编进带 `-tags tango_dev` 或新标签 `-tags tango_selftest` 的构建：`tan build` 会去掉 `tango_dev` 但保留 `GOFLAGS` 里的其他标签，模拟器自测用 `GOFLAGS=-tags=tango_selftest bash mobile/ios/build.sh …` 构建（`scripts/smoke.sh ios-probe`、`ios-selftest` 已改）。`tango_selftest` 构建也带通道故障注入 `TANGO_IPC_LANE_FAULT`，新增 `scripts/smoke.sh ios-lanefault` 在模拟器上测 `up:post` 中途坏掉。**不兼容**：以前直接用普通 iOS 构建跑画布自测的，要加这个标签（`TANGO_SCHEME_SELFTEST` 只由 `examples/duplex-ios` 读，框架不读）。关闭开关（`TANGO_IPC_LANES_DARWIN=0`/`false`/`off`、`TANGO_IPC_LANES=0`）在所有平台的所有构建里都生效。

## [0.1.0] - 2026-11-14

首个公开版本。0.x 期间公开 API 可能有不兼容的变化。

**验证状态**：桌面端只在 macOS 实机验证；Windows、Linux 后端已实现，通过交叉编译与 CI 冒烟，尚未在实机上系统验证。iOS、Android 为实验性：iOS 已在真机（iPhone 12 Pro）上运行验证，Android 只在模拟器上验证。自绘 UI 为实验性，只在 macOS 上实机验证。

**API 基线**：这是第一个打标签的版本，也是兼容性检查的基线。Go 包按 `api/tiers.txt` 分为稳定与实验性两层，规则见 [docs/release.md](docs/release.md#api-稳定性)；之后稳定层的不兼容变化只进下一个 `0.MINOR.0` 并写明迁移方法。

### 新增

- **核心运行时**（根包 `tango`）：`App` / `Surface`，主线程队列与 `Post`、`Await`、`AwaitAsync`、`TryAwaitAsync`；生命周期事件、统一的应用事件（深链、打开文件、主题与显示器变化、Dock 重新打开、通知点击）、用户退出拦截、关闭最后一个窗口时的平台默认行为。
- **桌面后端**：macOS（AppKit + WKWebView）、Windows（Win32 + WebView2）、Linux（GTK 3 + WebKitGTK 4.1），均不需要 cgo；资源通过自定义 scheme 提供，不开本地端口。
- **窗口**：位置、尺寸限制、状态、居中、外观（隐藏标题栏 / 无边框、透明、毛玻璃与 Mica、置顶、不透明度）、面板 / 模态 / 表单窗口、无边框拖动区域、文件拖入与拖出、缩放、打印与导出 PDF、截图、页内查找、Cookie 与网站数据、User-Agent 与请求头。
- **WebView 策略**：导航策略、页面权限请求、新窗口请求、下载、HTTP 认证的处理函数。
- **IPC**（`ipc`、`bind`）：调用、双向事件、可取消且带背压的流（JS 打开或 Go 打开），反射绑定；`tan generate bindings` 生成 TypeScript 客户端。
- **前端运行时** `@tangoui/runtime`：`call` / `on` / `emit` / `stream`，以及 `Window`、`App`、`Theme`、`Screens`、`Clipboard`、`Dialog`、`Notify`、`Shell`、`Menu`、`Hotkey`、`Events` 模块；Go 侧 `jsapi` 按能力白名单开放，默认只开窗口自身、右键菜单和只读信息。
- **安全**：只接受资源 origin 与核心导航目标 origin 的消息；`shell` 协议白名单；`service/fileserver` 限制在目录内。
- **系统能力**（`sys/`）：剪贴板（文本、图片、文件）、对话框、菜单（应用、窗口、右键）、托盘、通知、全局与窗口快捷键、显示器、系统主题与页面同步、打开 URL 与文件、应用控制（徽标、进度、请求注意、Dock 图标）、开机启动、运行环境、最近文档、单实例、窗口状态记忆。
- **移动端**（实验性）：iOS（c-archive + 原生宿主）与 Android（c-shared + Java 宿主），用户代码零改动；权限、分享、触感、安全区、屏幕方向、生物识别、安全存储、相册与拍照、定位、远程推送（APNs；FCM 需自行接入）。
- **服务**（`service/`）：服务生命周期、应用目录、键值存储、日志、文件服务；签名的自动更新（`service/updater`：防降级、二进制补丁、启动确认与回滚，实验性）。
- **多语言**（`i18n`）：系统语言、语言匹配（简繁分流）、带具名参数与 CLDR 复数的文案目录；原生菜单标准项和对话框按钮支持 en、zh-CN、zh-TW、ja。
- **自绘 UI**（实验性，`ui`、`uihost`）：即时模式 API、Ant Design 与 Element Plus 两套主题令牌、约 90 个组件、无头 `Driver` 测试、`tan dev` 热加载；macOS 上 WebView 与自绘同窗（`uihost.Attach`）。
- **命令行 `tan`**：`init`（多种模板）、`dev`、`build`（桌面、iOS、Android）、`run`、`package`（dmg、NSIS、MSIX、tar.gz、AppImage）、`sign`（macOS 签名与公证、Windows Authenticode）、`keygen`、`release-manifest`、`sign-update`（更新清单与签名）、`generate bindings`、`doctor`。
- **示例**：`hello`、`runtime-demo`、`showcase`（IPC、流、系统能力、托盘、菜单、通知、主题跟随，带 `-smoke` 自检）等。
- **工程**：CI（lint、三平台测试与真实窗口冒烟、iOS 模拟器与 Android 模拟器冒烟）、文档站（VitePress，中文为主、英文首页与核心页面）。

- JS 运行时的 API 兼容检查：`runtime-js/etc/runtime.api.md`（API Extractor 报告，含各命名空间成员）与 `api/jsapi-methods.txt`（Go `jsapi` 与 TS 封装的方法名、事件名、错误码对账），CI 在漂移时失败；文档补充本地修改 runtime 的流程。

- **完整图标库**：新增 `ui/icon/element`（Element Plus 293 个）、`ui/icon/antd`（Ant Design 848 个，线框 / 实底 / 双色）、`ui/icon/lucide`（Lucide 2052 个及 300 个旧名别名，shadcn/ui 默认图标），由 `internal/cmd/icongen` 从官方 npm 包生成；图标是静态初始化的变量，程序只链接实际引用到的图标，调用各库的 `Register` 后可用 `icon.ByName("lucide:search")` 按名查找。
- **主题图标**：`theme.Theme.Icons`（`icon.Style`）让组件画出设计体系自己的图标：Ant Design 主题用 @ant-design/icons，Element Plus 主题用 Element Plus 图标，shadcn 主题用 Lucide 并把线宽改为 2；应用里用到的内置图标也会跟着替换。
- `ui/icon`：图标改为由多段 `Part`（填充 / 描边、主色 / 辅色）组成，支持任意 viewBox（`Box`）、自定义线宽、双色图标（`DrawOpts.Secondary`）、绘制时覆盖线宽（`DrawOpts.StrokeWidth`）、位图与单色蒙版图标（`FromImage`）、`Register` / `RegisterAs` 注册自定义图标；内置图标从 85 个增加到 136 个（调试、编辑、Git、窗口按钮等常用图标）。
- `ui/icon/filetype`：按文件名或扩展名解析文件类别并给出图标，三套设计体系各有一组；`filetype.Theme` 按 VS Code 文件图标主题的方式（文件名、扩展名、文件夹名）叠加自定义或插件提供的图标。
- **公开 API 分层与兼容性检查**：Go 包分为稳定与实验性两层（`ui`、`uihost`、`mobile`、`service/updater` 为实验性）；CI 新增 `API` 工作流，用 `scripts/apidiff.sh`（基于 apidiff）检查稳定包的不兼容变化，需要破坏兼容时给 PR 加 `api-break` 标签。见 [docs/release.md](docs/release.md#api-稳定性)。

- **组件补齐**（对照 Ant Design v6、Element Plus、Pro Components 与 shadcn/ui 的"部分"项，见 [docs/specs/ui-coverage.md](docs/specs/ui-coverage.md)）：
  - 新组件 `Menubar`（应用菜单栏）、`AspectRatio`、`Item`/`ItemGroup`、`ButtonGroup`、`MultiDatePicker`、`ColorPickerGradient`/`ColorPickerPanelGradient`、`StatisticTimer`、`ProFieldOf`。
  - Button 颜色 × 样式（`Color`/`Appearance`，13 个预设色）与 Element 的 success/warning/info/plain；Steps inline/panel 与自定义进度点；Segmented 竖向与胶囊形；Timeline 横向。
  - DatePicker `NeedConfirm`、按日期禁用时间（`DisabledTimes`）；DateTimePicker 禁用日期与快捷选项；Calendar `Range`。
  - Switch 任意值（`SwitchValue`）、切换前确认（同步/异步）、内嵌图标、外侧文字、大号与宽度；Checkbox/Radio 边框与按钮样式、尺寸，CheckboxGroup 最少/最多与整组禁用；InputTag 自定义分隔符、拖动排序、失焦保存。
  - AutoComplete/Mentions/Select/Cascader 的 `Variant`（`FieldVariant`）；Select 前缀与后缀图标；AutoComplete 聚焦触发；Mentions 清除与整体删除；Cascader 自定义搜索与显示；TreeSelect 父子不联动与自定义搜索。
  - Upload 图片列表与手动提交；Card 内页签；Countdown `OnChange`；Tour 每步遮罩/类型/按钮与自定义指示器、操作区，切步自动滚到目标；Alert 深色效果与居中；Message 位置、偏移、关闭按钮与朴素样式；MessageBox 区分取消与关闭、居中、可拖动、自定义图标；Pagination 自由排序（`Layout`）与页码数量。
  - ProForm 只读模式；ProField 单独渲染与读写切换；新增 13 种 valueType；SchemaForm `layoutType` 与列表字段；ProLayout 自动面包屑与各渲染钩子、页脚链接、固定侧栏；ProCard 底部操作栏。
- **CLI 契约**：`cmd/tan/testdata/cli.golden` 固定 `tan help` 与各子命令的参数（`TestCLIGolden`），参考站的参数列表由它生成；`tan.json` 的 JSON Schema `cmd/tan/tan.schema.json`（`TestConfigSchema` 保持同步）发布在 <https://tangoui.github.io/tango/schema/tan.schema.json>，`tan init` 在新项目的 `tan.json` 里写入 `"$schema"`，编辑器可据此补全和检查；`tan.json` 接受 `"$schema"` 键，遇到未知字段时提示可能需要更新的 tan。
- `tan dev` 判断应用是否用了 `uihost.Attach` 的符号名移到 `internal/devhost.AttachSymbol`，`uihost` 的测试固定它，改名或移动 `AttachWith` 会在测试里失败，而不是让热加载悄悄误判。

### 变更

以下只影响此前用伪版本跟踪 `main` 的用户。

- `tan package` 生成的 macOS `.dmg` 改用 LZMA 压缩（`hdiutil -format ULMO`，原为 UDZO/zlib），体积约小四分之一：最小 WebView 应用从 2.32 MiB 降到 1.80 MiB。需要 macOS 10.15 以上挂载，低于 tan 写入的最低系统版本 11.0。

冻结 API 前的一次性不兼容调整（[docs/specs/api-stability.md](docs/specs/api-stability.md) Phase 3，规则见 [docs/release.md](docs/release.md#api-稳定性)）。

- **结构体不可比较（R4）**：下列结构体的第一个字段改为 `_ [0]func()`，不能再用 `==` 比较，也不能写无键字面量（`T{a, b}` 改成 `T{Field: a, ...}`）。以后给它们加字段不再算破坏兼容。
  - 稳定层的 Options：`tango.SurfaceOptions`、`tango.WebViewOptions`、`asset.Options`、`asset.DevProxyOptions`、`jsapi.Options`、`service/fileserver.Options`、`service/log.Options`、`sys/biometric.Options`、`sys/flyout.Options`、`sys/location.Options`、`sys/media.PickOptions`、`sys/print.Options`（含有 `Options` 字段的 `service/fileserver.Server` 也随之不可比较）。
  - `ui` 的全部 `*W` 构建器与句柄（`Box`、`Context`、`Runner`、`Driver`、`Label`、`ScrollBox`、`AppAPI`、`ProSubmitter`、`TableFilterPanel`、`MessageRef`、`NotificationRef`）、`Locale` 及全部 `*Locale` 子结构，以及 `DriverOptions`、`WindowOptions`、`OverlayOptions`、`ContextMenuOptions`、`AppMessageConfig`、`AppNotificationConfig`、`ProSettings`、`Option`、`TabItem`、`StepItem`、`TransferItem`、`DatePreset`、`CheckCardOption`。
- **`ui` 构造函数返回专属类型（R2）**：
  - `Breadcrumb` 返回 `*BreadcrumbW`（原来返回 `int`）：`i := ui.Breadcrumb(c, ...)` 改成 `i := ui.Breadcrumb(c, ...).Clicked()`。
  - `CheckboxGroup` 返回 `*CheckboxGroupW`，`CheckAll` 返回 `*CheckAllW`（原来返回 `bool`）：`if ui.CheckboxGroup(...)` 改成 `if ui.CheckboxGroup(...).Changed()`。
  - `CardAction` 返回 `*CardActionW`（原来返回 `bool`），改用 `.Clicked()`；`CardMeta` 返回 `*CardMetaW`（原来无返回值）。
  - `Sider` 返回 `*SiderBoxW`，`Kbd`、`BadgeRibbon`、`ListItemMeta`、`ListActions`、`AnchorSection` 分别返回 `*KbdW`、`*BadgeRibbonW`、`*ListItemMetaW`、`*ListActionsW`、`*AnchorSectionW`（原来都返回 `*Box`）。这些类型内嵌 `*Box`，链式调用 `.Bg(...)` 等照常可用；需要 `*Box` 值时取 `.Box` 字段。
- **`layout.Option` 改为不透明类型**：原来是 `func(*layout.Style)`，现在是结构体，只能用 `layout` 包的构造函数（`Grow`、`W`、`H`、`Pad`、`Gap`、`Abs`……）得到，构造函数名称和签名不变。`layout.Style`、`Node`、`Pos`、`Solve`、`SolveAt` 移到内部包 `ui/internal/flexbox`。自己写的 `func(s *layout.Style){...}` 选项改用构造函数；新增 `layout.ScrollX/ScrollY/NoStretch/Stretch`（实验性）覆盖 `ui/mobile` 原来直接改字段的用法。传 `nil` 作为选项的地方改成 `layout.Option{}`。
- **`ProSettings.Apply` 应用 `FixSiderbar`**：以前这个字段被忽略、侧栏总是固定；现在 `FixSiderbar: false` 会让侧栏随页面滚动（`ProLayoutConfig.NoFixSiderbar`），与 `FixedHeader` 的处理一致。只填部分字段的 `ProSettings{...}` 请从 `DefaultProSettings()` 出发，否则侧栏和顶栏都不再固定。
- **`theme.NumComps` 改为未导出**（R1：计数常量每加一个组件就变值）。`theme.Comp` 的值只追加，文档不再把它和 `Components` 的字段顺序绑定。
- **枚举只追加（R1）**：`ui`、`ui/a11y`、`ui/input`、`ui/layout`、`ui/paint`、`ui/theme` 的枚举类型注释写明"只追加、零值不变"，现有常量值不变。
- **`asset.DevProxy` 恢复单参数（R3）**：`DevProxy(target string) http.Handler`；带选项的用法改为新增的 `DevProxyWith(target string, opts DevProxyOptions) http.Handler`。`DevProxy(url, opts)` 改成 `DevProxyWith(url, opts)`。

### 修复

- macOS：WebView 被键值观察（KVO）换成子类后，不含文件的拖动在 `TangoWebView` 的拖放方法里无限递归导致崩溃。现在调用父类时固定从 `TangoWebView` 开始查找。
- `icon.CheckFilled`、`CloseFilled`、`InfoFilled`、`WarningFilled` 原来只画一个实心圆，现在中间的符号是镂空的。
- **无障碍**：滚动或裁剪出视野、嵌套超过一层的内容（例如表单项里的滑块）不再从语义树里消失，读屏软件能找到它们并滚动显示（`ui/runner.go` `recordRects`）。所有平台的屏幕阅读器都受影响，在 iOS 真机上发现。
- **iOS 无障碍**：自绘界面的可编辑输入框对 VoiceOver 标记为文本输入：iOS 18.1 起用 `accessibilityTextInputResponder`，iOS 17–18.0 在提示里说明双击编辑。最低版本仍是 iOS 17。

### 验证

- iOS 首次真机运行（iPhone 12 Pro，iOS 27）：WebView 与自绘示例、触摸、输入法、Metal 与 CPU 渲染、无障碍，范围见 [docs/mobile.md](docs/mobile.md)。

### 安全

- **开发功能改为编译期开关 `tango_dev`**：默认构建（普通 `go build`/`go run`、goreleaser、`tan build` 含 `-debug`）即生产构建，不再响应 `TANGO_DEV_URL`、`TANGO_AUTOMATION*`、`TANGO_DEV_ROLE`、`TANGO_DEV_WINDOW_STATE` 等开发用环境变量，热加载宿主、自动化通道、开发代理不链接进二进制；未处理的 panic 默认弹框退出。`tan dev` 自动加 `-tags tango_dev`；`automation.Launch` 构建被测程序时也加。`tango_release` 标签不再起作用，仅作兼容保留。见 [docs/dev-builds.md](docs/dev-builds.md)。
- `asset.DevProxy` 默认只接受回环目标；要代理到其他主机用 `asset.DevProxyWith(target, asset.DevProxyOptions{AllowRemote: true})`。
- 资源源上 GET/HEAD 以外的跨源请求回 403（`Options.AllowCrossOriginAssets` 关闭）；macOS/iOS 后端用主文档 URL 补上 WebKit 缺失的 `Origin`。
- `tan dev` 热加载控制通道要求会话 token（`TANGO_DEV_TOKEN`），unix socket 权限 0600。

[未发布]: https://github.com/tangoui/tango/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tangoui/tango/releases/tag/v0.1.0
