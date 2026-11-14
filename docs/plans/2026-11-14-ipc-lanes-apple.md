# IPC 快速通道 · macOS / iOS 平台接入 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 macOS 和 iOS 的 WKWebView 页面真正用上快速通道：下行 `down:scheme`（`tango://app/__tango/lane/down` 流式响应），上行 `up:post`（攒批 POST）。前提是先把两个平台的 `tango://` 协议处理器改成异步（不再在主线程同步跑 `ServeAsset`），并实测 WebKit 确实逐块交付流式响应。按规格 §6.3 跑基准，达标才默认开启。

**Architecture:** 三层不变：公共设施（`internal/lane`、`ipc`、runtime-js 的 `down:scheme`/`up:post` 模块）已完成；本计划只改平台后端。darwin（purego）和 ios（cgo + bridge.m）各自把 `WKURLSchemeTask` 的处理从"主线程同步"改为"请求在 goroutine 里跑、对 WebKit 的调用切回主线程"，写出端实现 `http.Flusher` 和 `FlushError`、可被 `stopURLSchemeTask` 取消、`Write`/`FlushError` 在取消后立即返回错误。两个后端的 surface 实现 `platform.BinaryLane.LaneCaps()`，在通过实测和基准之前报告空能力（默认关闭），用开发开关强制开启以便测试。

**Tech Stack:** Go 1.27；darwin：purego/objc（`internal/platform/darwin`）；iOS：cgo + Objective-C（`internal/platform/ios/bridge.m`）；验证：`automation` 包（macOS 真实窗口）、`xcrun simctl`（iOS 模拟器，iOS 27.2 运行时，本机可用）。

**Spec:** `docs/specs/2026-11-13-ipc-duplex-streams.md` §6.2「macOS / iOS / Linux：下行走 `tango://` 流式响应」「上行走 POST」、§6.3 验收。公共设施计划：`docs/plans/2026-11-13-ipc-lanes-infra.md`（其中 Task 5 报告与最终审查列出的"平台前置条件"是本计划的输入）。

## Global Constraints

- 通道名、端点、包格式、令牌、`BinaryMinSize`（`down:scheme` 4096）、`up:post` 参数（16 KiB / 4 ms 攒批、单包 ≤ 1 MiB、单 POST 超时 4000 ms、聚合上限 8 MiB）全部沿用公共设施，**本计划不改任何公共协议或 `internal/lane`、`ipc`、runtime-js 的语义**。若发现公共设施缺陷，报告给控制者，不在平台任务里顺手改。
- 平台前置条件（公共设施最终审查列出，必须全部满足才能报告 `down:scheme`）：
  1. 长连接响应不在主线程/共享请求线程上阻塞；
  2. 写出端实现 `http.Flusher` **和 `FlushError() error`**（`internal/lane/http.go` 经 `http.ResponseController` 调用，优先用 `FlushError`），每次刷新立刻把已写字节交给 WebKit（`didReceiveData:`）；
  3. WebKit 放弃请求（`webView:stopURLSchemeTask:`）时取消请求的 context，并让阻塞中的 `Write` 和 `FlushError` 立即返回 `context.Canceled`；
  4. 停止之后对该 `WKURLSchemeTask` 不再有任何调用（Apple 文档：停止后调用会抛异常）。
- 每个 `WKURLSchemeTask` 的 `didReceiveResponse:`/`didReceiveData:`/`didFinish`/`didFailWithError:` **只在主线程调用**；跨线程时用 darwin 的 `runOnMain` / iOS bridge.m 的 `onMain`（`CFRunLoopPerformBlock` + 公共模式；**不要用主 dispatch 队列**，原因见 Task 3）。
- 上行请求体：WebKit 只可靠地交付 ArrayBuffer/字符串请求体（Blob 会丢，WebKit bug 197237）；`up:post` 已经只发 ArrayBuffer。iOS 现有 `bodyShim` 不受影响。读取请求体时总量上限为公共设施的 `lane.MaxUpBody`（`MaxPostPacket + 22`）加任意普通资源请求的现有行为——**普通资源请求不得因本计划被截断**。
- 来源校验：macOS/iOS 的协议处理器已把 `mainDocumentURL` 作为 `Origin` 注入（`darwin/scheme.go:155`、`ios/bridge.m` startURLSchemeTask）。保留这一行为；通道请求必须经过核心的 `laneRequestAllowed`。iframe：`WKURLSchemeTask` 没有公开的 frame 接口，后端无法按 frame 拒绝请求；安全性依赖"令牌只经主 frame 的消息桥下发"。Task 4 实测这一点，发现绕过就停下交裁定。
- 默认关闭：`LaneCaps()` 在本计划的基准（Task 7）达标并由控制者确认之前返回空；开发/测试通过环境变量 `TANGO_IPC_LANES_DARWIN=1`（任何构建都读，仅用于强制开启，`TANGO_IPC_LANES=0` 仍优先关闭）强制报告 `["down:scheme","up:post"]`。达标后改为默认开启，环境变量保留为可关闭的开关语义不变。
- 不新增外部依赖。不提交 git；在 worktree `/Users/admin/min/tango/.claude/worktrees/ipc-duplex` 工作（前两期改动也在这里，均未整合）。
- 文件归属：只改 `internal/platform/darwin/**`、`internal/platform/ios/**`、`examples/duplex/**`（基准和自测页；服务注册抽成 `examples/duplex/services` 子包）、`examples/duplex-ios/**`（新建，iOS 自测示例）、`docs/ipc-bench.md`、`docs/mobile.md`、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`、`docs/specs/2026-11-13-ipc-duplex-streams.md`（只更新 §6.2/§6.3 的实测结论）、`CHANGELOG.md`、`scripts/smoke.sh`（只加 iOS 通道自测）。不改其他平台后端。
- 每个任务结束时（darwin 相关）：`go build ./... && go vet ./internal/platform/darwin/... .`；`go test -timeout 300s ./internal/platform/darwin/... . ./ipc/... ./internal/lane/...`；`TANGO_E2E=1 go test -timeout 300s ./examples/duplex -run E2E`。（iOS 相关）：`GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC=$(xcrun --sdk iphonesimulator --find clang) CGO_CFLAGS="-isysroot $(xcrun --sdk iphonesimulator --show-sdk-path) -target arm64-apple-ios17.0-simulator" CGO_LDFLAGS="$CGO_CFLAGS" go vet ./internal/platform/ios/...`，以及 `bash scripts/smoke.sh ios`（或任务指定的模拟器自测）。

## Review Focus

1. **普通资源回归**：协议处理器异步化后，普通页面、图片、大文件、重定向、`HEAD`、带请求体的 POST、处理函数 panic，行为和改造前完全一致（状态码、头、`Content-Length`、正文字节）—— Task 2 `TestAsyncSchemeParity`（macOS），Task 3 的 iOS 自测页。
2. **WebKit 中途放弃请求**：页面重载/关窗口时，长连接下行响应所在的 goroutine 在 1 秒内退出，不再调用已停止的 task，不崩溃 —— Task 2 `TestSchemeStopCancelsHandler`、Task 5 `laneReloadNoLeak`。
3. **iframe 冒用**：不透明来源 iframe 猜测令牌请求通道，Go 侧收不到任何数据、也不会建立下行连接；同源 iframe 拿不到自己的 hello 回复；主文档的通道不受影响 —— Task 4 `TestLaneSubframeAccess`。
4. **主线程卡顿**：页面执行 6 秒同步任务期间，正在传输的流不报 `transport`（公共设施的卡顿规则在真实 WebKit 上成立）—— Task 5 `laneStall`。
5. **WebKit 实际不逐块交付**：如果 Task 1 实测发现 `fetch` 的 `ReadableStream` 要等 `didFinish` 才拿到数据，`down:scheme` 必须保持关闭、只开 `up:post`，并在文档写明 —— Task 1 的判定分支。

---

### Task 1: 实测 WKWebView 流式响应是否逐块交付（macOS + iOS 模拟器）

**目的**：规格要求"确认之前这条通道默认关闭"。这是后续所有任务的前提，结论写进规格。

**Files:**
- Create: `internal/platform/darwin/streamprobe_test.go`（`TANGO_E2E` 门控）
- Create: `examples/duplex/frontend/probe.html`（自测页：`fetch("/__tango/probe/stream")`，记录每个 chunk 到达的时间和大小；结果写进 `window.__probe = {chunks:[{t,n},...], done:true}`，测试用 `Window.WaitJS("window.__probe && window.__probe.done")` 再 `EvalInto` 读出——`automation` 不能接收页面发给应用的事件）
- Modify: `examples/duplex/main.go`（新增 `-page <文件名>` 标志，窗口加载指定页面，默认 `index.html`）
- Modify: `internal/platform/darwin/scheme.go`（仅加一个测试钩子：当环境变量 `TANGO_SCHEME_PROBE=1` 时，路径 `/__tango/probe/stream` 由后端直接处理——**不经过 `ServeAsset`**——在主线程用 `runOnMain` 定时器每 200 ms 调一次 `didReceiveData:`（8 个 1 KiB 块），最后 `didFinish`；正常构建不读此变量以外的任何东西）
- Modify: `internal/platform/ios/bridge.m` + `scheme.go`（iOS 同样的钩子，用 `dispatch_after` 在主队列上每 200 ms 发一块）
- Modify: `docs/specs/2026-11-13-ipc-duplex-streams.md` §6.2（写入实测结论）

**判定**：页面记录的第 1 块到达时间 < 第 8 块到达时间 − 1000 ms，并且块数 ≥ 4（WebKit 可能合并相邻块）→ "逐块交付"成立。否则不成立。

- [ ] **Step 1: 写 macOS 实测**：`TestStreamingDeliveryProbe` 用 `automation.LaunchOptions` 启动 `examples/duplex`（加 `TANGO_SCHEME_PROBE=1` 和 `-page probe.html` 参数，Task 1 在 `examples/duplex/main.go` 增加 `-page` 标志），等待 `probe` 结果，按判定规则断言，并把原始时间序列写进测试日志。
- [ ] **Step 2: 运行**：`TANGO_E2E=1 go test -timeout 120s ./internal/platform/darwin -run StreamingDeliveryProbe -v`，记录输出。
- [ ] **Step 3: iOS 实测**：iOS 自测用新示例 `examples/duplex-ios`（Task 3 正式建立；本步先建最小版本）：`main.go` 复用 `examples/duplex` 的服务（把 `examples/duplex/services.go` 里与平台无关的服务注册抽到 `examples/duplex/services` 子包 `services.Register(r *ipc.Router)`，`examples/duplex` 和 `examples/duplex-ios` 都调用它），接好 `ipc.Router`、`OnMessage`、`OnBinary`，页面 `web/index.html` 加载 `selftest.js`；URL 带 `?probe` 时跑与 probe.html 相同的脚本，结果用 `log.Printf("probe: %s", …)` 输出一行。`scripts/smoke.sh` 增加 `run_ios_probe`：`bash mobile/ios/build.sh examples/duplex-ios simulator`（环境变量 `SIMCTL_CHILD_TANGO_SCHEME_PROBE=1` 在 `simctl launch` 时传入），从 `--console-pty` 日志里读出 `probe:` 行并按同一规则判定。（`examples/mobile-hello` 只用原始消息，不接 `ipc.Router`，不能用于流和通道测试。）
- [ ] **Step 4: 运行** `bash scripts/smoke.sh ios-probe`，记录输出。
- [ ] **Step 5: 写结论**：在规格 §6.2「macOS / iOS」小节追加"实测结论（2026-11，macOS 26 / iOS 27.2 模拟器）"：两个平台各自是否逐块交付、块的合并情况、第一块延迟。**若任一平台不成立**：在本计划账本记 Ruling，后续任务对该平台只做 `up:post`，`LaneCaps` 不报 `down:scheme`，Task 2/3 的异步化仍要做（上行和普通大文件也受益）。
- [ ] **Step 6: 检查点**（不提交）

---

### Task 2: macOS 协议处理器异步化

**Files:**
- Modify: `internal/platform/darwin/scheme.go`、`internal/platform/darwin/asset.go`
- Create: `internal/platform/darwin/scheme_async_test.go`（单元测试，用假的 `taskSink`；不需要窗口）
- Create: `internal/platform/darwin/scheme_async_e2e_test.go`（`TANGO_E2E` 门控）

**Interfaces / 行为:**
- `serveSchemeTask` 在主线程只做：保留 task、构建 `*http.Request`（读请求体）、登记到 `assetBridge.inFlight`，然后 `go` 一个 goroutine 运行 `callAssetHandler`。
- 新的 `assetWriter` 在 goroutine 上被调用，线程安全（内部 `sync.Mutex`）。`Flush`/写满 `streamChunk` 时，把要发给 WebKit 的动作（respond / data）**按顺序**排进该 task 的主线程队列，用 `runOnMain` 执行；`finish` 同理，且一定是最后一个动作。
- **背压**：每个 task 未交给 WebKit 的字节有上限 1 MiB（`pendingLimit`）；超过时 `Write`/`Flush` 阻塞，直到主线程把数据交出去（条件变量），或 task 被停止/context 取消时返回错误。这样长连接下行不会在主线程卡住的情况下无限堆积内存。
- `webView:stopURLSchemeTask:`（主线程）：标记 task 已停止、丢弃尚未执行的主线程动作、取消请求 context、唤醒阻塞的写方；之后该 task 的任何主线程动作都不执行（Apple：停止后调用会抛异常）。
- panic：在 goroutine 内 `recover`；头还没发出时改为 500（与现有 `replaceWithError` 语义相同），已发出则截断并 `didFinish`。
- `Content-Length`：处理函数返回时若头还没发出，仍按现有逻辑设置精确 `Content-Length`（保持小文件行为不变）。

- [ ] **Step 1: 写失败的单元测试**（`scheme_async_test.go`，把"主线程队列"抽象成可注入的执行器，测试里同步执行或手动推进）

```go
func TestAsyncWriterOrdersActions(t *testing.T)     // Write 3 次 + Flush + finish → 执行器收到 respond, data(合并), data, finish，顺序固定，respond 恰好一次
func TestAsyncWriterBackpressure(t *testing.T)      // 执行器不推进时，累计写超过 1 MiB 的 Write 阻塞；推进后返回
func TestAsyncWriterStopUnblocks(t *testing.T)      // 阻塞中的 Write 和阻塞中的 FlushError 在 stop 后 100 ms 内都返回 context.Canceled；stop 之后再调 FlushError 也立即返回 context.Canceled；之后执行器不再收到任何动作
func TestAsyncWriterPanicBeforeHeaders(t *testing.T) // handler panic 且未发头 → respond(500) + 正文 "tango: asset handler panic: …" + finish
func TestAsyncWriterContentLength(t *testing.T)     // 小响应不 Flush 直接返回 → respond 头里 Content-Length 等于正文长度
```

- [ ] **Step 2: 写 E2E 对照测试**（`scheme_async_e2e_test.go`）：

```go
func TestAsyncSchemeParity(t *testing.T)
// 启动 examples/duplex（TANGO_E2E），页面里用 fetch 依次请求：index.html、一个 3 MiB 的静态文件（examples/duplex 新增 testdata 路由 /__test/big，仅在 TANGO_SCHEME_PROBE=1 时注册）、
// 一个 302 重定向、HEAD /index.html、POST 64 KiB ArrayBuffer 到回显路由、一个 panic 路由；断言状态码、关键头、正文 SHA-256 与改造前在同一测试里用"同步模式"（环境变量 TANGO_SCHEME_SYNC=1 保留旧路径，仅测试用）得到的结果一致。
func TestSchemeStopCancelsHandler(t *testing.T)
// 打开一个永不结束的流式路由，页面 abort() 该 fetch → 1 s 内 Debug.Stats().handlerExits 加一（Go 侧处理函数因 r.Context() Done 而返回；取值方式见 Task 5 开头）；随后 app.Output() 里没有 "NSInternalInconsistencyException"。Task 2 先建好 Debug.Stats 的 handlerExits 与 goroutines 两项，Task 5 补其余计数。
```

  注：`TANGO_SCHEME_SYNC=1` 是为对照测试保留旧同步路径的开关，Task 7 达标后删除。

- [ ] **Step 3: 运行确认失败**
- [ ] **Step 4: 实现**
- [ ] **Step 5: 运行** 本任务门槛命令 + 新测试，全部通过；另跑 `TANGO_E2E=1 go test ./examples/... -run E2E` 确认其他示例（hello、showcase 等）不受影响。
- [ ] **Step 6: 检查点**

---

### Task 3: iOS 协议处理器异步化

**Files:**
- Modify: `internal/platform/ios/scheme.go`、`internal/platform/ios/bridge.m`、`internal/platform/ios/bridge.h`
- Create: `internal/platform/ios/scheme_writer.go`（纯 Go 的异步写出逻辑，**不加构建约束、不用 cgo**，照 `canvas_keys.go` 的做法：`internal/platform/ios` 在 macOS 上用普通 `go test` 就能编译测试）。同时把 `buildRequest`、`responseWriter`、`flushAt`、`hasBody`、`contentType` 从带 `//go:build ios` 的 `scheme.go` 挪进来，避免重名
- Create: `internal/platform/ios/scheme_writer_test.go`（不加构建约束；`go test ./internal/platform/ios/` 直接运行）
- Modify: `examples/duplex-ios/web/selftest.js`（Task 1 建立的 iOS 自测示例；URL 带 `?selftest` 时运行与 Task 2 对照测试相同的请求序列，结果 `log.Printf("selftest: %s", …)` 输出一行；`examples/duplex-ios` 用环境变量 `TANGO_SCHEME_PROBE=1` 注册与 Task 2 相同的测试路由）
- Modify: `scripts/smoke.sh`（`run_ios_selftest`）

**行为**：与 Task 2 相同的契约（goroutine 运行 `ServeAsset`、主线程执行 `tg_task_*`、1 MiB 背压、停止后不再调用 task、panic 处理、精确 `Content-Length`）。主线程派发**不能用** `dispatch_async(dispatch_get_main_queue(), …)`：`bridge.m:31-36` 写明主队列上的块在 `tg_step` 启动的嵌套运行循环里不执行，而 `AwaitAsync` 在主线程上就是靠 `tg_step` 等待（`loop.go:280-285`）——用主队列会让嵌套等待期间所有 task 的 `didReceiveData`/`didFinish` 停住，等待若依赖一个 `tango://` 响应就死锁。改用 bridge.m 现有的 `onMain`（`mobile.m:33-43`，`CFRunLoopPerformBlock` + 公共模式 + `CFRunLoopWakeUp`），经新增的 `tg_task_post(uint64_t h, int op, int status, const char *headers, const void *p, size_t n)` 进入；**在 `tg_task_post` 返回前把 `p` 拷贝成 `NSData`**（cgo 指针规则：C 代码不能在调用返回后持有 Go 内存）。`tangoIOSTaskStop` 唤醒阻塞写方。`bridge.m` 的 `tasks` 字典只在主线程访问（现状如此，保持）。

- [ ] **Step 1: 写失败的单元测试**（`scheme_writer_test.go`，同 Task 2 的五个测试，名字加前缀 `TestIOS`）
- [ ] **Step 2: 运行确认失败**：`go test ./internal/platform/ios/ -run IOS`
- [ ] **Step 3: 实现**（`scheme_writer.go` + 接线 `scheme.go`/`bridge.m`）
- [ ] **Step 4: 运行**：单元测试通过；iOS vet（Global Constraints 里的命令）通过；`bash scripts/smoke.sh ios`（原有冒烟）和 `bash scripts/smoke.sh ios-selftest` 都通过，`selftest` 结果里每一项与预期一致。
- [ ] **Step 5: 检查点**

---

### Task 4: 两个平台报告通道能力 + 主 frame 校验

**Files:**
- Modify: `internal/platform/darwin/surface.go`（或新建 `internal/platform/darwin/lanes.go`）：darwin `surface` 实现 `platform.BinaryLane`
- Modify: `internal/platform/darwin/scheme.go`：通道路径的主 frame 判定
- Create/Modify: `internal/platform/ios/lanes.go`、`internal/platform/ios/bridge.m`：同上
- Create: `internal/platform/darwin/lanes_e2e_test.go`

**Interfaces / 行为:**
- `LaneCaps() []string`：Task 1 结论成立的平台，在启用条件满足时返回 `["down:scheme","up:post"]`；不成立的平台只返回 `["up:post"]`。启用条件：`TANGO_IPC_LANES_DARWIN=1`（Task 7 之前），`TANGO_IPC_LANES=0` 时一律返回 nil。核心已负责 OnBinary/资源来源/关闭开关等其余条件。
- `SendLanePacket`：返回 false（这两个平台没有平台专属下行通道；`down:scheme` 由核心经 HTTP 服务）。
- 主 frame 判定（实测优先，不在本任务里改公共协议）：`WKURLSchemeTask` 没有公开的 frame 接口（已核对 macOS 27.2 SDK 的 `WKURLSchemeTask.h`），后端无法区分通道请求来自主 frame 还是同源 iframe。现有的消息通道只信任主 frame（`darwin/surface.go:435` 检查 `isMainFrame`），所以令牌 `tok` 只经主 frame 的 `hello` 回复下发；同源 iframe 想冒用通道，必须先从主 frame 拿到 `tok`。本任务**只做实测并如实报告**（测试见下）：
  - 跨源 iframe：核心的 `laneRequestAllowed` 用 `mainDocumentURL` 注入的 Origin 判断，拿不到 `tok` 时也会被令牌校验拒绝，预期被拒。
  - 同源 iframe：与主文档同属应用代码，安全边界上视同主 frame（规格的信任模型：同源脚本不是独立的安全主体）。实测确认它**只能**在主 frame 主动把 `tok` 交给它时才能用通道，不能自己拿到。
  - 若实测发现任何不经主 frame 就能拿到有效 `tok` 的路径，**停下，报告给控制者裁定**（可能需要回到公共设施加 frame 绑定），不在平台层临时打补丁。
- `LaneCaps` 读取环境变量只在 surface 创建时读一次。

- [ ] **Step 1: 写测试**（`lanes_e2e_test.go`，`TANGO_E2E` 门控）。**每个测试通过 `automation.Options.Env` 显式设置 `TANGO_IPC_LANES_DARWIN`**（开：`=1`；关：`=`）——`automation` 会把测试进程自身的环境原样传给应用（`automation/automation.go:149`），不能靠测试进程的环境变量门控。iOS 自测同理：`simctl launch` 时通过 `SIMCTL_CHILD_TANGO_IPC_LANES_DARWIN=1` 打开（Task 4 Step 5、Task 5 Step 5 都要传）。

```go
func TestLanesAdvertised(t *testing.T)    // examples/duplex 页面 tango.lanes() 等于 {down:"down:scheme", up:"up:post"}（或 Task 1 结论对应的组合）
func TestLanesOffByDefault(t *testing.T)  // 不设 TANGO_IPC_LANES_DARWIN → {down:"fallback", up:"fallback"}
func TestLanesKillSwitch(t *testing.T)    // TANGO_IPC_LANES_DARWIN=1 且 TANGO_IPC_LANES=0 → fallback
func TestLaneSubframeAccess(t *testing.T)
// 事实前提（已核对代码）：runtime 脚本只注入主 frame（darwin/surface.go:182-186、ios/bridge.m:352 forMainFrameOnly:YES），子 frame 的脚本消息被丢弃（darwin/surface.go:435、ios/bridge.m:212），所以 iframe 里没有 window.tango，也收不到 hello 回复。
// 1) 不透明来源 iframe（sandbox="allow-scripts" 的 srcdoc，或 data: URL）：猜测 gen=1..5、空 tok，请求 /__tango/lane/up（POST 一个合法包）和 /__tango/lane/down。
//    JS 看不到跨源响应状态，所以**在 Go 侧断言**：examples/duplex 的 OnBinary 计数器与下行连接计数器（见 Task 5 的 Debug.Stats）都没有增加。
// 2) 同源 iframe（资源来源下的 /iframe.html）：按设计属于应用代码，可以访问 parent.tango——这是允许的，不算冒用。只断言它自己发出的 hello（直接 webkit.messageHandlers 发送）得不到任何回复、不会让主页面的通道世代变化。
// 3) 主页面通道在上述过程中持续可用（一次 1 MiB 回显成功）。
// 若 1) 中出现任何 Go 侧计数增加 → 测试失败，按 Interfaces 里的规则停下交裁定。
```

- [ ] **Step 2–4: 运行确认失败 / 实现 / 运行通过**
- [ ] **Step 5: iOS**：`selftest.js` 增加 `lanes` 检查（`tango.lanes()` 与预期一致），模拟器自测通过。
- [ ] **Step 6: 检查点**

---

### Task 5: 真实 WebKit 上的通道端到端

**Files:**
- Create: `internal/platform/darwin/lanes_flow_e2e_test.go`
- Modify: `examples/duplex/frontend/app.js`（若需要新增场景按钮/自动化入口）
- Modify: `examples/duplex-ios/web/selftest.js`、`scripts/smoke.sh`

**场景**（macOS 用 `automation` 驱动 `examples/duplex`；iOS 用 selftest 在模拟器跑同样的脚本，结果逐项打日志）。

**Go 侧计数的取得方式**（`automation` 只能 Eval 页面和读进程输出，测试拿不到子进程里的 Go 状态）：`examples/duplex/services` 在环境变量 `TANGO_SCHEME_PROBE=1` 时额外注册 ipc 方法 `Debug.Stats`，返回 `{"goroutines": runtime.NumGoroutine(), "upFrames": n, "upBytes": n, "downConns": n, "downWrites": n, "handlerExits": n}`：
- `upFrames`/`upBytes`：在 `examples/duplex` 的 `OnBinary` 钩子里解析包计数（`ipc.ParsePacket`）后再转给 `HandleBinarySession`。
- `downConns`/`downWrites`：在 **darwin/ios 后端**里计数——协议处理器对路径 `/__tango/lane/down` 的请求次数和交给 WebKit 的数据块数（同样只在 `TANGO_SCHEME_PROBE=1` 时开启，经一个包级 `atomic` 计数器 + 后端导出给示例的只读函数 `darwin.LaneDownStats()` / `ios.LaneDownStats()`；示例用构建约束分别引用）。不改 `internal/lane`。
- `handlerExits`：Task 2 的永不结束流式测试路由在处理函数返回时加一。
页面用 `tango.call("Debug.Stats")` 取值后写进 `window.__stats`，测试 `EvalInto` 读出；iOS selftest 把它打进日志行。

```go
func TestLaneBigBothWays(t *testing.T)   // 8 MiB 回显（64 KiB 一帧，第 i 字节 = i % 251），逐字节相等；Debug.Stats：upFrames ≥ 64（至少一半字节帧经上行通道到达），downWrites ≥ 64（至少一半回程帧经下行通道写出）
func TestLaneMixedOrder(t *testing.T)     // 同一条流交替 100 B / 64 KiB 共 40 帧，回显顺序与发送一致
func TestLaneReloadNoLeak(t *testing.T)   // 下行长连接打开时 location.reload()；新页面里 Debug.Stats().downConns 增加 1 且旧连接已结束（后端计数器另记"活动下行连接数"，1 s 内回到 1）；goroutines 回到重载前基线（±5）；新页面 tango.lanes() 重新为 down:scheme
func TestLaneStall(t *testing.T)          // 传输 4 MiB 期间页面执行 6 s 同步 busy-loop；流最终完整、无 transport 错误
func TestLaneFault(t *testing.T)          // tango_dev 构建（automation 已加 -tags tango_dev）+ TANGO_IPC_LANE_FAULT=down:scheme:3：之后的帧走消息通道，丢帧的流报 transport，并行的另一条流不受影响
```

- [ ] **Step 1–4**：写测试 → 运行确认失败（未开通道时计数断言失败）→ 修复后端问题（**仅限平台层**）→ 全部通过；`-count=3` 稳定。
- [ ] **Step 5: iOS** 同样五个场景在模拟器 selftest 中通过。**`laneFault` 在 iOS 上的构建方式**：`tan build` 会去掉 `GOFLAGS` 里的 `tango_dev`（`cmd/tan/build_tags.go:24-34`），所以 `GOFLAGS=-tags=tango_dev` 无效。改用 `tan dev -platform ios-simulator` 的构建路径：它调用 `buildIOS(..., dev=true)`（`cmd/tan/dev_mobile.go:123`），产物带 `tango_dev`。`scripts/smoke.sh` 的 `run_ios_lanefault` 用 `cmd/tan` 的 `dev` 构建出 `.app` 后（若 `tan dev` 只能以交互循环方式运行、无法单独产出 `.app`，则停下报告给控制者，由控制者裁定是否在 `cmd/tan` 加一个只构建不运行的选项——`cmd/tan` 不在本计划的文件范围内），用 `simctl launch` 传 `SIMCTL_CHILD_TANGO_IPC_LANE_FAULT=down:scheme:3` 和 `SIMCTL_CHILD_TANGO_IPC_LANES_DARWIN=1`。其余四个场景用普通 `bash mobile/ios/build.sh examples/duplex-ios simulator` 构建。
- [ ] **Step 6: 检查点**

---

### Task 6: 文档

**Files:** `docs/mobile.md`、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`、`CHANGELOG.md`、`docs/specs/2026-11-13-ipc-duplex-streams.md`（§6.2 macOS/iOS 小节状态）

- [ ] 写明：macOS/iOS 支持哪些通道（按 Task 1 结论）；协议处理器已异步（普通资源也受益：大文件不再卡主线程）；`TANGO_IPC_LANES_DARWIN`（Task 7 前）与 `TANGO_IPC_LANES=0`；已知限制（iframe 结论、Blob 请求体、模拟器与真机差异未测）。
- [ ] `cd site && npm run docs:build && npm run docs:links` 通过。
- [ ] 检查点

---

### Task 7: 基准与默认开启判定（规格 §6.3）

**Files:** `docs/ipc-bench.md`、`internal/platform/darwin/lanes.go`、`internal/platform/ios/lanes.go`

- [ ] **Step 1: 基准方法**：`go run ./examples/duplex -bench`，分别在 `TANGO_IPC_LANES_DARWIN=1` 与未设置两种情况下各跑 5 次（同一台机器；每次跑之前确认 `uptime` 的 1 分钟负载低于核心数的一半，否则等待最多 10 分钟再测，仍不满足就在报告里写明当时的负载、照常记录但标为"负载下"，判定以空载数据为准；两组交替跑，避免负载变化只影响其中一组），取中位数。macOS WebView 计时器 1 ms 精度，p99 改用**分批计时**：每批 50 次 64 B 回显，记录每批总耗时 ÷ 50，对 20 批取 p50/p99。iOS 在模拟器上同样跑（模拟器数据只作参考，写明）。
- [ ] **Step 2: 判定**：64 KiB 帧吞吐（上行、下行分别）≥ 兜底的 2 倍，且 64 B 分批 p99 不比兜底差（允许 ±5% 测量噪声）。
  - 达标的方向/平台：`LaneCaps` 改为默认报告；`TANGO_IPC_LANES_DARWIN` 改为"设为 0 关闭"语义（文档同步）；删除 Task 2 的 `TANGO_SCHEME_SYNC` 测试开关。
  - 不达标：保持默认关闭，`docs/ipc-bench.md` 写明数据和原因。
  - **默认开启需要控制者确认**：本步只产出数据和建议，改默认值的代码改动在控制者在账本里记下 Ruling 后再做。
- [ ] **Step 3**：`docs/ipc-bench.md` 增加"第二期 macOS/iOS"表格（机器型号、系统版本、负载、每项中位数、判定结果）。
- [ ] **Step 4: 检查点**

---

## 完成后

- macOS/iOS 是否默认开启快速通道，以 Task 7 数据和控制者裁定为准。
- 未覆盖、留给后续：iOS 真机（本计划只测模拟器）；若 Task 4 发现令牌可绕过主 frame 获取，需回到公共设施加 frame 绑定（Task 4 会停下来交裁定）；Windows、Android、Linux 平台计划。
