# 开发功能与生产构建

`tango_dev` 构建标签开启 Tango 的开发功能。

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
| macOS 通道计数器（`TANGO_SCHEME_PROBE`） | 无效 | 生效 |
| iOS 模拟器自测钩子（`TANGO_SCHEME_PROBE`、`TANGO_CANVAS_IME_SELFTEST`、`TANGO_CANVAS_A11Y_SELFTEST`）和通道故障注入 `TANGO_IPC_LANE_FAULT`（所有平台） | 无效；`-tags tango_selftest` 时生效（见下文） | 生效 |
| Android 模拟器自测钩子：启动参数 `TANGO_TEST_ENV`（还要求可调试 APK）、`TANGO_ANDROID_NO_ARRAYBUFFER`、快速通道计数（`android.LaneStatsSnapshot`） | 无效；`-tags tango_selftest` 时生效（见下文） | 生效 |

**直接 `go run .` 或 `go build` 得到的就是生产行为。** 热加载、自动化、前端开发代理这些开发功能只能通过 `tan dev` 或手动加 `-tags tango_dev` 使用。

## 为什么是编译期开关

开发功能都由环境变量或本机端口触发。与应用同一用户的任何进程都能控制这些输入：`open -a App --env TANGO_DEV_URL=http://evil`、`launchctl setenv`、Windows 的用户环境变量、抢占本机端口。若发布版也响应它们，攻击者就能让一个已签名、已拿到 TCC 授权和钥匙串访问的应用加载自己的页面（页面可调用全部绑定），或通过自动化通道远程执行 JS。运行时检查挡不住同用户的攻击者，所以这些代码在默认构建里根本不编译进去：`go list -deps` 看不到 `internal/automation`、`internal/devhost`（`automation` 包的 `TestReleaseBuildHasNoChannel` 检查），二进制里也没有 `asset.DevProxy`。

默认构建仍会读取几个面向用户排障的变量：`TANGO_RENDERER`（自绘窗口渲染器）、`TANGO_FRAME_TRACE`（自绘窗口帧时延日志，见下文）、`TANGO_TEXT_ENGINE`（文字引擎；默认 macOS 构建忽略它）、`TANGO_LINUX_TRAY`、`TANGO_ATSPI_EMBED`、`TANGO_WINDOWS_TOAST=0`（Windows 通知回退到气泡）、`TANGO_LAUNCH_BACKGROUND=1`（macOS：启动时不激活应用，窗口显示在其他应用窗口后面，相当于 `open -g`；`automation.Launch` 默认设置，测试脚本和自动化代理从命令行启动应用时也应设置）。它们只在已有实现之间切换、只写日志或少做一次激活，不会加载外部代码或打开通道。

macOS（非 iOS）的自绘文字默认用 CoreText 文字引擎：整形（CTLine）、断行（CFStringTokenizer）、双向文字排序、字体回退和光栅化都交给 CoreText，不链接 go-text（`scripts/check-gotext-free.sh` 和 `ui/text` 测试 `TestCoreTextBuildHasNoGoText` 检查），可执行文件约小 1.9 MiB（最小自绘样本 10,520,354 → 8,544,834 B，见 [体积报告](package-size-optimization.md)）。`text.RegisterFont` 的字体也由 CoreText 读取和绘制。这个构建里 `TANGO_TEXT_ENGINE` 被忽略，`text.UseSystemEngine(false)` 不会关掉 CoreText（返回 `true`），`text.Engine()` 总是 `"coretext"`；换行位置（中文禁则、URL、数字串）、字距和回退字体跟随系统，可能与其他平台（go-text/HarfBuzz）不同。性能（2026-11-18，Apple M4，同一会话和 go-text 交替测量）：编辑器无窗口打字、滚动、打开文件都在 1 帧以内（Markdown 每帧一页滚动 p99 12.25 → 12.69 ms）；counter/quotes/list/grid 应用进程 CPU 差 +0/−1.5/+0.9/+1.8 个百分点，内存低 4.9–8.3 MB，首帧 −51…+2 ms，帧间隔 P99 差 ±0.03 ms 以内；编辑器输入延迟打字 32.6 → 33.0 ms，三光标 35.7 → 32.4 ms。编辑器行缓存的等宽快速路径对中英混排行逐字符排版：每个字符用通用路径会选的字体，字体的 GSUB/GPOS 须证明默认启用的 lookup 不会让它受相邻字符或脚本影响（AAT 字体不走），再由一次 CoreText 探针实测字形和宽度（含 tracking），结果按字体、字号、语言缓存；日语（`ja`）含 CJK 的行仍走通用路径，因为 CoreText 的日文排版会按相邻字符调整间距和标点宽度。已知限制：数字和等宽快速路径的探针在全局排版锁内执行。

构建标签 `tango_gotext` 恢复 macOS 以前的 go-text 引擎：go-text/HarfBuzz 整形和断行、CoreText 光栅化系统字体，`TANGO_TEXT_ENGINE=go|system` 和 `text.UseSystemEngine` 可切换，需要纯 Go 路径的金样图测试只在这个构建里运行（CI 的 macOS 任务额外跑 `-tags tango_gotext`）。这个构建里另有可选的 `TANGO_TEXT_ENGINE=coretext` 轻量路径：无换行短文本用 CoreText glyph-run 排版，其他文本仍走 go-text。其他平台和 iOS 忽略这个标签。

iOS 模拟器自测要用这些钩子，但 `tan build`（`mobile/ios/build.sh` 也走它）会去掉 `tango_dev`。所以 iOS 另有一个只编进测试钩子的标签 `tango_selftest`：`tan build` 保留 `GOFLAGS` 里除 `tango_dev` 以外的标签，`GOFLAGS=-tags=tango_selftest bash mobile/ios/build.sh <应用> simulator` 构建出带钩子的 .app（`scripts/smoke.sh ios-probe`、`ios-selftest` 就这样构建）。`tango_selftest` 不链接自动化通道、热加载或开发代理，只打开上表那几个自测钩子。不带这两个标签的 iOS 构建不读这些变量，桥接代码里的探测路由也不编译进去。`TANGO_IPC_LANES_DARWIN` 的所有取值（macOS：`1`/`all` 开 `down:poll` + `up:post`，`up`、`down` 只开其一，`0`/`false`/`off` 关闭；iOS：默认开 `up:post`，`0`/`false`/`off` 关闭）和 `TANGO_IPC_LANES=0` 在所有构建里都生效，不再是开发构建专属的测试开关（`all` 以前只在开发构建里开 `down:scheme`，该通道已删除）。

Android 同样用 `tango_selftest`：`GOFLAGS=-tags=tango_selftest bash mobile/android/build.sh <应用>`（`scripts/smoke.sh android-selftest`、`android-lanefault` 就这样构建）。Android 进程没法从外部拿到环境变量，所以测试钩子构建另读启动 Intent 的字符串数组 extra `TANGO_TEST_ENV`：`adb shell am start -n <包名>/org.tango.host.TangoActivity --esa TANGO_TEST_ENV TANGO_A=1,TANGO_B=2`（`am` 按逗号拆分，所以值里不能有逗号），只接受 `TANGO_` 开头的键，在 `main.main` 运行之前 `os.Setenv`。Java 侧只在可调试 APK（`tan build -debug`，`mobile/android/build.sh` 的默认）里交出这个 extra。`TANGO_ANDROID_NO_ARRAYBUFFER=1` 让 Go 把 WebView 当成不支持 ArrayBuffer 消息（测试没有通道时的回退）。不带这两个标签的 Android 构建不读这些输入，`android.LaneStatsSnapshot` 始终为零；`TANGO_IPC_LANES_ANDROID` 是正式开关，任何构建都读。

Windows 和 Linux 构建也会读取以下设置。`TANGO_IPC_LANES=0` 和 `Options.DisableFastLanes` 优先于平台开关。

- `TANGO_IPC_LANES_WINDOWS` 在 WebView2 支持 SharedBuffer 时默认开启 `down:shm` 和 `up:shm`，设 `0`、`false` 或 `off` 关闭。
- `TANGO_IPC_LANES_LINUX` 默认关闭。`1` 或 `all` 开启 WebKitGTK 所支持的通道，`up` 只开 `up:post`，`down` 只开 `down:poll`，`0`、`false` 或 `off` 关闭。其他值使用默认设置。两个平台的开关都忽略大小写和首尾空白。
- `TANGO_IPC_SHM_BYTES` 设置 Windows 上、下行共享环各自的数据区容量（十进制字节数，环头另计），默认 4 MiB。取值为 64 KiB 到 2 GiB 的 2 的幂（容量加环头须能放进 Go `int`），例如 `8388608` 表示每个环 8 MiB。无效取值记录日志并使用默认值。

## 自绘窗口的渲染分级与字体回收

`TANGO_RENDERER` 是窗口创建时读取的运行参数，`ui.RunnerOptions.Renderer` 显式指定的渲染器始终优先：

- 未设置：macOS Apple Silicon 上的普通自绘窗口不论大小、不论 `ui.RunnerOptions.Content` 取值都先使用原生 Core Animation 图层和 CoreText；其他平台保持 GPU 优先，失败回退 CPU。透明/材质窗口、WebView 附加画布和调用方自选渲染器保留原路径。默认的 `ui.ContentGeneral` 窗口收到键盘输入（按下非修饰键，包括自动重复和被窗口快捷键处理的按键；提交文字；输入法组字）时，下一帧单向升级 Metal；只按 Shift、Control、Option、Command、Caps Lock，以及滚动、指针、触摸和窗口焦点变化都不触发升级。方向键、PageUp/PageDown、空格、Tab、Esc 和 Cmd+C 这类快捷键也算键盘输入，所以用键盘浏览的普通窗口同样会离开原生图层。只有送到这个窗口的键盘事件才触发：通过无障碍操作（VoiceOver、听写的 PerformAction）设置的文字，以及在嵌入的 WebView 里打字，都不触发升级。`ui.ContentGeneral` 窗口连续呈现 60 帧、相邻帧空闲不超过 100 ms，且期间没有窗口输入或 resize 打断时，会在下一帧仍连续出帧的情况下尝试升级。应用主动驱动的连续重绘也计入；不会为升级唤醒已闲置的窗口。`Content` 设为 `ui.ContentData` 的数据窗口（表格、网格、行情、列表、仪表盘）不因键盘或持续动画升级。所有默认原生窗口在连续六帧绘制和呈现工作超过 8 ms，或连续六帧整窗软件回退时，也尝试单向升级 Metal；相邻帧间的空闲时间需不超过 100 ms，单次冷启动/零散操作不触发。升级只作用于这一个窗口，只尝试一次：失败立即呈现 CPU 图像且不反复尝试，之后的按键也不再触发。
- `cpu`：始终使用 CPU，不创建原生 GPU 呈现目标。
- `gpu`：尝试 GPU（包括软件 OpenGL），失败仍回退 CPU。
- `native`：在支持的 macOS Apple Silicon 普通自绘窗口强制原生图层，不自动升级 Metal；不支持时沿用 GPU/CPU 回退。`uihost.Renderer(r)` 会报告 `native`、`gpu` 或 `cpu`。
- `auto`：面积不超过 100,000 逻辑像素的小窗口先使用 CPU；其他窗口 GPU 优先。CPU 起步的窗口在放大超过阈值、绘制列表达到 256 项或连续六个实际绘制帧的间隔不超过 100 ms 时，尝试升级 GPU。升级单向生效，不随缩小窗口来回切换；升级失败也不逐帧重试。这是保守的工作量启发式，不是对 CPU/GPU 快慢的保证。小型编辑器、终端或持续动画也可直接设 `gpu`。

大窗口也默认使用原生图层，因为它在内存上明显占优：2026-11-17 在 Apple M4 上和 NSGo、Viva、mygo 交错实测（quotes 560×860、list 480×800，各 5 轮），physical footprint 比 Metal 低 63–71%（list 39 对 135 MB，quotes 54 对 147 MB），回调间隔 P99 相同。2026-11-18 在 Apple M4 上和 mygo 同批配对实测 800×600 grid（原生图层，5 组 AB/BA + 65 秒长跑）：physical footprint 69.2 对 107.1 MB（长跑 68.4 对 111.5），CPU 22.1% 对 39.1%（长跑 20.2% 对 37.6%），更新间隔 P99 17.9 对 18.0 ms（长跑 18.9 对 18.0，在 mygo 1 ms 计时分辨率内）。但原生图层的输入到画面延迟比 Metal 慢约 2 帧：2026-11-18 在 1920×944 的编辑器窗口上配对实测（真实窗口、CGEvent 输入、ScreenCaptureKit 采样），打字 78.8 对 47.2 ms、三光标 87.6 对 47.2 ms、滚动约 80 对 46 ms（中位数）。所以默认 `ui.ContentGeneral` 的窗口（编辑器、终端、表单）在第一次按键后切到 Metal，输入延迟和固定用 Metal 相当。2026-11-18 在 Apple M4 上用 1920×944 的编辑器夹具实测（CGEvent 输入、ScreenCaptureKit 窗口采样，默认设置对 `TANGO_RENDERER=gpu`，同一会话内 AB/BA/AB 交替）：打字 100 键，从第 2 键起中位数 32.5 对 31.9 ms（p95 48.3 对 48.9）；三光标 30 次 32.7 对 31.8 ms（p95 45.5 对 45.2）；第一次按键（切换到 Metal 的那一帧）一次性多约 30–90 ms（默认 75–134 ms，gpu 41–46 ms）。第一次按键的那一帧要建立 Metal 设备、管线和 atlas，所以有这笔一次性开销；此后这个窗口一直留在 Metal，不再有原生图层的内存优势。小窗口原本就默认用原生图层，现在打字之后也会升级，内存随之上升到 Metal 的水平。只滚动浏览、不打字的普通窗口保留原生图层的内存优势，滚动仍按原生图层的延迟运行。表格、行情、仪表盘这类常用键盘浏览的窗口设 `Content: ui.ContentData`，按键不升级，一直保留原生图层的内存优势，代价是输入和滚动都按原生图层的延迟运行；只有帧持续变慢时才升级。需要固定使用 Metal 时设 `gpu`，需要固定使用原生图层时设 `native`。

GPU 默认路径还会对可重建的 atlas 页和闲置 Metal image texture 施加有界预算，在 frame boundary 淘汰未被当前 frame/command buffer 使用的资源；当前 glyph、present target 和活动图片不会被淘汰。`TANGO_GPU_BUDGET=off` 可关闭这项预算并恢复旧的资源常驻策略。该开关不改变 `TANGO_RENDERER` 的选择，也不保证每种场景都降低物理 footprint；如果重新上传导致帧尾部恶化，应先用 `TANGO_FRAME_TRACE=1` 对照。

### 帧时延日志（`TANGO_FRAME_TRACE`）

`TANGO_FRAME_TRACE=1` 让自绘窗口每 120 帧打一行摘要（`tango: frame trace surface N renderer=… n=120 …`），各项给 p50/p95：

- `frame`：一帧在 CPU 上的全程（布局、绘制、Render、Present）；`render`：渲染器的 Render；`present`：呈现（macOS 上包含 `nextDrawable` 的等待）；`gap`：相邻两帧开始时刻的间隔（显示刷新节拍，没有请求帧时会更长）。
- 仅 macOS Metal：`drawable`（`nextDrawable` 阻塞时长）、`misses`（拿不到 drawable 的 present 次数）、`gpu`（帧在 GPU 上的执行时长）。这三项只在该统计周期里 Metal 确实上报过时才出现。
- 各项的样本数不一定等于 `n`：跳过绘制或呈现的帧只计入 `frame`/`gap`；`gpu` 是周期内退役的 GPU 帧，异步退役会滞后一两帧，verify 模式下每帧会多一次。
- `renderer` 是周期结束时的渲染器：在 present 中丢失 GPU 的那一帧起就记为 `cpu`。

只有经 GPU 渲染器（`gpu` 或 `native`，包括原生窗口升级 Metal 之后）呈现的窗口会被统计；原生图层窗口没有 Metal 时间线，只有前四项。CPU 窗口（`TANGO_RENDERER=cpu`、没有可用 GPU、`ui.RunnerOptions` 自选渲染器）、`Attach` 附加层和 `tan dev` 窗口不统计，各打一行 `not traced` 说明原因。

没有任何平台报告帧何时真正上屏，所以这里的时间都不是"屏幕时延"。未设置时不计时、不分配。

原生图层保留未变化的文字和图片，整像素移动只更新位置；圆角、边框、阴影等使用有界且可复用的参考渲染图片。复杂裁剪、变换、透明度组和背景模糊会完整软件回退，超过 64 MiB 的预计图层位图也回退；不丢弃绘制命令。原生层的文字抗锯齿遵循 CoreGraphics。大量动态图片、复杂效果或持续动画可显式选择 `gpu`。Intel macOS 保留原渲染路径。

`text.TrimCaches()` 仅清排版缓存；`text.TrimFontCaches()` 还丢弃注册表持有的系统字体链。平台低内存通知会同时调用每个窗口的 `runner.TrimCaches()`，清理 CPU 字形遮罩、变换图片、阴影缓存及 GPU 可重建缓存并请求重绘，避免这些缓存继续持有旧字体。桌面应用也可以在自己的低内存策略中，在主线程上依次调用 `text.TrimFontCaches()` 和 `runner.TrimCaches()`。`paint.CPU.Trim()` 保留当前帧像素，不会立即把窗口清空。

仍被当前页面、`Layout` 或编辑器 `LineCache` 引用的字体继续有效，不会被提前销毁；macOS 的原生字体资源随最后一个拥有者不可达，由 Go cleanup 释放。回收不强制全局 GC，也不承诺物理内存立即下降。当前没有默认空闲计时器，避免多个活动窗口频繁丢弃彼此正在使用的字体链。

## macOS 原生轻量宿主（`macoslite`）

`macoslite` 是独立于 `uihost.Open` 的 macOS 小窗口宿主：只通过 purego 调 AppKit、CoreGraphics、CoreText，不加载 Tango App、WebView、`ui.Runner`、布局、无障碍或 GPU 渲染器，用于设置、更新器、提示这类小型工具窗口。非 macOS 平台 `New` 返回 `ErrUnsupported`。参考窗口 `examples/macoslite-counter` 空闲约 26 MiB（`scripts/macoslite-memory.sh`）；设计见 `docs/specs/macos-native-lite-design.md`。

## `tango_release` 已废弃

`tango_release` 以前是"发布构建"的标签；默认构建现在就是生产构建，它**不再起任何作用**，只作为兼容别名保留：`tan build` 仍会传它，手写的 `-tags tango_release` 也不会出错。`-tags tango_release,tango_dev` 等同于 `-tags tango_dev`。

## 开发与测试

- `tan dev`：桌面、`-platform android|ios-simulator` 的构建都带 `-tags tango_dev`（与 `GOFLAGS` 里的 `-tags` 合并）。`tan build`、`tan build -debug` 去掉 `tango_dev`，保持生产语义。桌面 `tan build` 和 `tan package` 还会合并 `tan.json` 中的 `desktopBuildTags`。桌面构建与热加载默认关闭 cgo，可在 `tan.json` 中设 `"cgo": true` 开启，交叉编译时需要目标平台的 C 编译器（`CC`）。见[桌面构建标签](packaging.md#桌面构建标签)。
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

`Options.Assets` 不知道请求来自哪个页面，外站页面（用户点链接打开的、iframe 里的）可能尝试访问 `<资源源>`。核心在请求到达 `Options.Assets` 之前检查：只要请求带 `Origin`，其值就必须是资源源本身，GET、HEAD 也不例外；没有 `Origin` 头的请求放行（例如 WebView 或核心自身发起的请求），否则回 403。WebKit（macOS、iOS）对自定义 scheme 请求不带 `Origin`，后端用主文档 URL（`NSURLRequest.mainDocumentURL`）补上；Windows（WebView2）与 Android 的资源源是 `https://tango.localhost`，由 Chromium 内核按规范带 `Origin`（尚未在这两个平台上实测）。Linux（WebKitGTK）暂未补，请求不带 `Origin` 时放行。确实需要跨源请求（自己做 CORS 或 token 校验）时设置 `tango.Options.AllowCrossOriginAssets`。

已知限制：补上的是主文档的来源，后端分不清请求来自哪个 frame，所以应用页面里嵌的跨源 iframe 发往资源源的写请求也能通过这项检查（IPC 快速通道的请求另有每世代令牌保护，不受影响）。不要在资源页面里嵌不可信的 frame。
