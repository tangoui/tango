# 更新日志

本文件记录 Tango 各版本的用户可见变化，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。Go 模块、`cmd/tan` 与 `@tangoui/runtime` 使用同一个版本号，见 [docs/release.md](docs/release.md)。

## [未发布]

### 新增

- **IPC 双向流**（`ipc`、`@tangoui/runtime`）：流改为全双工，两个方向都能传 JSON 帧和字节帧，任一端都可以打开。
  - Go：`Router.HandleDuplex` 注册双向处理函数（类型 `ipc.DuplexFunc`）；`*ipc.Stream` 新增 `WriteBytes`、`Receive`（返回新类型 `ipc.Message`：`Binary`、`Bytes`、`JSON`）、`ReceiveBytes`、`ReceiveJSON`、`CloseSend`（半关闭）、`ReadWriter`（`io.ReadWriteCloser` 视图，可直接 `io.Copy`）；`Router.Open`、`Router.Call` 让同进程的 Go 代码（自绘界面）不经过页面直接使用同一批服务；`Router.SetStreamWindow` 设置 Go 的接收窗口；新增常量 `ProtoVersion`、`DefaultWindowBytes`、`FrameOverhead`、`MaxFrameBytes` 和错误码 `CodeTransport`（快速通道上丢帧时使用，见下）。
  - JS：`TangoStream` 新增 `accepted`、`send`、`sendBytes`、`end`、`bufferedAmount` 和 Web Streams 视图 `readable`/`writable`，迭代时字节帧产出 `Uint8Array`；新增 WebSocket 风格的 `socket()` / `TangoSocket`（`close()` 写完排队的消息后发 `close`，随即以 1000 关闭，不等 Go）；新增常量 `PROTO_VERSION`、`DEFAULT_WINDOW_BYTES`、`FRAME_OVERHEAD`、`MAX_FRAME_BYTES`。
  - 示例与基准：`examples/duplex`（回显、文件上传、小终端、`-panel` 进程内 Go 面板、`-bench` 基准），基线数据见 [docs/ipc-bench.md](docs/ipc-bench.md)。
- **IPC 快速通道（公共设施）**：流的字节帧可以走挨着消息通道的二进制旁路，不做 JSON 和 base64；JSON 帧、控制消息和握手仍走消息通道，语义不变。**macOS 默认开启 `up:post`（上行）；其他平台仍走消息通道**，各平台后端接入并通过基准对比后逐个开启。
  - Go：`App.OnBinary` 注册上行包处理函数，应用接线 `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })`；`ipc.BinaryConn` 和 `ipc.SessionBinaryConn`（`*tango.Surface` 实现，后者把发送和上行投递绑定到页面世代）、`Router.HandleBinarySession`（`OnBinary` 用它，带授权时的世代）、`Router.HandleBinary`（只给不复用页面的简单 `BinaryConn`；对 `SessionBinaryConn` 丢包并记一次 `slog.Warn`）、`ipc.GapTimeout`；一个 surface 只接一个 `Router`；`Options.DisableFastLanes` 和环境变量 `TANGO_IPC_LANES=0` 关闭快速通道；开发构建里 `TANGO_IPC_LANE_FAULT=<通道名>:<N>` 让某条通道在 N 个数据包后故障（空包不计，比如 `up:post` 的启动探测），用来测通道中途坏掉。
  - 开启条件：注册了 `OnBinary`、页面来自资源来源（直接从开发服务器加载的页面没有快速通道）、后端报告了能力、没有被关闭。
  - 丢帧：某帧缺了 5 秒仍没到的流以 `transport` 错误失败（两端都看到）。
  - JS：`lanes()` 返回当前用的通道（`{down, up}`，没有时为 `"fallback"`）；新增常量 `GAP_TIMEOUT_MS` 和 `createClient` 的第二个参数 `ClientOptions`（`gapTimeoutMs`）；内置 `down:scheme` / `up:post` 通道模块；`ErrorCode` 补上 `transport`。
  - 示例：`hello`、`runtime-demo`、`showcase`、`duplex` 接上 `OnBinary`；`examples/duplex -bench` 的 `lane` 字段改为 `lanes()` 的值（`{"down":…,"up":…}`）。
- **macOS 默认开启 `up:post`；iOS 试用（默认关闭）**：WKWebView 后端接入 `up:post`（上行字节帧经 `tango://` POST，ArrayBuffer 请求体）。macOS 上传 64 MiB 吞吐约为消息通道的 2.73 倍（[docs/ipc-bench.md](docs/ipc-bench.md)）。下行仍走消息通道，因为 WebKit 会扣住 `tango://` 流式响应里大块突发的一部分（见规格 [§6.2](docs/specs/2026-11-13-ipc-duplex-streams.md)）。环境变量 `TANGO_IPC_LANES_DARWIN`（窗口/页面创建时读一次；取值去掉首尾空白、不区分大小写；关闭值在任何构建都生效，macOS 上 `all` 只在开发构建生效）：

  | 取值 | macOS | iOS |
  |---|---|---|
  | 未设置、空 | `up:post`（默认开启） | 关闭（默认） |
  | `0`、`false`、`off` | 关闭 | 关闭 |
  | `all` | 开发构建（`-tags tango_dev`）：`down:scheme` + `up:post`（仅供实验）；生产构建：同默认（`up:post`） | `down:scheme` + `up:post`（仅供实验，任何构建，见「安全」） |
  | 其他值（如 `1`） | `up:post` | `up:post` |

  `TANGO_IPC_LANES=0` 和 `Options.DisableFastLanes` 在两个平台上都始终优先关闭。iOS 真机上没法从外部传环境变量，要试用就在创建第一个窗口/页面之前调用 `os.Setenv("TANGO_IPC_LANES_DARWIN", "1")`。
  - CSP：`up:post` 启动时先 POST 一个空包，收到 204 才算就绪。页面的 CSP 若不允许 `connect-src` 到资源来源（例如 `connect-src https://api.example.com`，或不含 `'self'` 的 `default-src`），这次探测被拒，通道不开启，字节帧照常走消息通道，不报错；想用快速通道就在 `connect-src` 里允许 `'self'`。
  - 已知限制：iframe 拿不到通道令牌（macOS 实测：不透明来源和同源 frame 的通道请求全部被拒；iOS 未单独测；同源 frame 能访问 `parent.tango`，那是应用代码）；WebKit 的 `tango://` 请求不带 `Origin`，后端用主文档的来源补上，所以应用页面里嵌的跨源 iframe 发往资源来源的请求能通过资源 CSRF 检查（原有行为；通道请求仍受每世代令牌保护），不要在资源页面里嵌不可信的 frame；WebKit 不交付 Blob 请求体；iOS 只在模拟器上测过；iOS 上通道中途坏掉的场景没有自动化测试。见 [docs/mobile.md](docs/mobile.md)。

- **终端视图**（`ui/terminal`）：`terminal.View` 把 `Terminal` 画成可交互的自绘终端（选择、滚动、链接 `OnOpenLink`、`OptionAsMeta`、`FontSize`）；配色 `Scheme` 和内置的 `SchemeDark`、`SchemeLight`、`SchemeDracula`、`SchemeSolarizedDark`、`SchemeSolarizedLight`，`ResolveColor` 把 `vt.Color` 解析成主题色；`vt.Screen.Evicted` 报告被挤出回滚区的行数。
- `ui/theme`：组件令牌 `TerminalToken`（`Components.Terminal`、`CompTerminal`），默认取 xterm.js 的 ANSI 调色板；HighContrast 会把对比度不够的调色板颜色调亮，对比度检查支持数组字段。
- `ui`：`Context.ClaimEscape`：获得键盘焦点的组件（例如终端）在构建时声明后，键盘 Esc 交给它，而不是关闭浮层；Android 系统返回键不受影响，照常关闭浮层。
- `ui/input`：`KeyBack`，Android 系统返回键的 `Code`（`Key` 仍是 `KeyEscape`，所以已有的 Esc 处理照常工作；返回键的抬起不会送给占用了 Esc 的组件）。
- `ui`：`ConfigProvider.Defaults` 覆盖所有组件，按组件名和属性名设置默认值（新增 `Button.AutoInsertSpace` 及一批可通过 `Defaults` 设置的属性，见 `api/go/ui.txt`）。
- **shadcn/ui v4 主题**：`theme.ShadcnOptions.Color` 叠加 v4 的 17 个彩色主题（`theme.ShadcnColors`：amber、blue、cyan……yellow），覆盖主色、次要色、图表色和侧栏主色；`theme.ShadcnBases` 增加 v4 的 mauve、olive、mist、taupe。
- `theme.ParseOKLCH` / `theme.OKLCH`：解析和转换 CSS `oklch()` 颜色（支持百分比亮度、`none` 色相和 `/ alpha`，色域外分量裁剪）；`theme.ParseColor` 也接受 `oklch()`。
- `internal/cmd/shadcngen`：从 shadcn/ui 的 `apps/v4/registry/themes.ts` 和 Tailwind CSS v4 的 `theme.css` 生成 `ui/theme/shadcn_data.go`。
- **默认更新界面** `service/updater/updateui`（实验性）：`updateui.Start(app, u, …)` 一行接上现成的更新窗口：后台定时检查、更新说明、下载进度（补丁优先）、立即重启或下次启动时安装、跳过此版本；`CheckNow` 给"检查更新…"菜单用。WebView 与自绘应用都能用，跟随主题与语言。见 [docs/updates.md](docs/updates.md#默认更新界面updateui)，示例 `examples/update-demo`。
- `tango.SurfaceOptions.Auxiliary`：辅助窗口（更新窗口、工具面板等）不算作"最后一个窗口"，`QuitOnLastClose` 下最后一个普通窗口关闭时应用照常退出，辅助窗口随之关闭；零值保持原行为。见 [生命周期](site/guide/concepts/lifecycle.md)。

### 变更

- **IPC 线协议升级到 v2，不兼容旧版 runtime-js**：握手带 `proto:2`，Go 拒绝其他版本（回 `runtime-js protocol mismatch` 并记 `slog.Warn`）。`@tangoui/runtime` 必须与 Go 模块同版本；自己实现协议的客户端需要按 `ipc` 包注释更新。流的背压从按条数改为按字节计的信用窗口（默认 1 MiB，每帧计负载 + 64 字节），`ack` 改为带归还字节数 `b`；单帧负载上限 16 MiB，超过时这次写入返回 `bad_args`。
- **macOS/iOS 的资源处理函数改在主线程之外运行**：`tango://` 协议处理器改为异步，`Options.Assets` 的处理函数每个请求在自己的 goroutine 里运行（请求可能并发处理），不再在主线程上同步执行。大文件、SSE 等大的或长时间不结束的响应不再卡住界面；WebKit 取消请求时 `r.Context()` 被取消，阻塞中的写入返回 `context.Canceled`。处理函数里若直接访问只能在主线程用的状态，改用 `app.Post` 投递到主线程或加锁；`tango.Await` 也可以，但它会一直阻塞到主线程执行完，这个请求的响应也跟着等（平台契约一直允许在任意线程调用处理函数）。
- **`Stream.Close` 语义改为优雅全关闭**：本方结束发送并不再接收，对方之后的写入返回 "stream closed by peer"。只想结束本方发送、继续读的用新增的 `CloseSend`。处理函数返回 nil 时 Router 发的就是这个 `close`。
- **shadcn 主题改为 shadcn/ui v4**（对照 `shadcn-ui/ui@e62c943` 与 `tailwindcss@4.3.5`，见 [docs/design-baseline.md](docs/design-baseline.md)）：颜色改用 v4 发布的 oklch 变量值；默认圆角从 8px（0.5rem）改为 10px（0.625rem），按钮和输入框随之为 8px、卡片 14px；暗色下的边框和输入框是半透明白色（`th.Shadcn.Border` 等保留半透明值，`CSSVars` 导出 `rgba()`，不透明的主题令牌按背景合成）；暗色侧栏主色为蓝色；`--destructive-foreground` 在 v4 中已删除，危险色上的文字固定为白色。`slate`、`gray` 两个底色 v4 已不再提供，作为旧名保留 v3 的颜色。要回到旧圆角可以传 `theme.ShadcnOptions{Radius: 8}`。

### 移除

- Go：`ipc.DefaultWindow`（按条数的窗口），由 `ipc.DefaultWindowBytes` 取代。
- JS：`WINDOW`、`ACK_EVERY` 导出，由 `DEFAULT_WINDOW_BYTES` 取代。

### 修复

- `sys/pty`（Windows）：关闭 ConPTY 前先读完剩余输出，不再丢掉最后一段；文档说明 Windows 上的 EOF 行为。
- **水波图文字**：`chart.Liquid` 水位过半时，数值和标题的颜色按合成后的水色明暗选白色或深色，不再固定为白色（shadcn/ui v4 neutral 的 chart-1 是浅灰，白字看不清）。

### 安全

- **快速通道的测试开关**：macOS 上流式探测路由（`TANGO_SCHEME_PROBE=1` 时的 `/__tango/probe/stream`）、协议处理器的通道计数器和 `TANGO_IPC_LANES_DARWIN=all` 只编进开发构建（`-tags tango_dev`），生产构建忽略它们（`all` 等同默认）。iOS 上这些钩子和更早就有的画布自测开关（`TANGO_CANVAS_IME_SELFTEST`、`TANGO_CANVAS_A11Y_SELFTEST`）只编进带 `-tags tango_dev` 或新标签 `-tags tango_selftest` 的构建：`tan build` 会去掉 `tango_dev` 但保留 `GOFLAGS` 里的其他标签，模拟器自测用 `GOFLAGS=-tags=tango_selftest bash mobile/ios/build.sh …` 构建（`scripts/smoke.sh ios-probe`、`ios-selftest` 已改）。`tango_selftest` 构建也带通道故障注入 `TANGO_IPC_LANE_FAULT`，新增 `scripts/smoke.sh ios-lanefault` 在模拟器上测 `up:post` 中途坏掉。**不兼容**：以前直接用普通 iOS 构建跑画布自测的，要加这个标签（`TANGO_SCHEME_SELFTEST` 只由 `examples/duplex-ios` 读，框架不读）。关闭开关（`TANGO_IPC_LANES_DARWIN=0`/`false`/`off`、`TANGO_IPC_LANES=0`）在所有平台的所有构建里都生效。

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
- `@tangoui/runtime` 的 `ErrorCode` 加入 `busy`（更新器在已有检查或下载进行时返回）。

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
- **`theme.NumComps` 改为未导出**（R1：计数常量每加一个组件就变值）。`theme.Comp` 的值只追加，文档不再把它和 `Components` 的字段顺序绑定。
- **枚举只追加（R1）**：`ui`、`ui/a11y`、`ui/input`、`ui/layout`、`ui/paint`、`ui/theme` 的枚举类型注释写明"只追加、零值不变"，现有常量值不变。
- **`asset.DevProxy` 恢复单参数（R3）**：`DevProxy(target string) http.Handler`；带选项的用法改为新增的 `DevProxyWith(target string, opts DevProxyOptions) http.Handler`。`DevProxy(url, opts)` 改成 `DevProxyWith(url, opts)`。

### 修复

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
