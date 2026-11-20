# IPC 快速通道：Windows / Linux / iOS 真机 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补完 IPC 第二期剩下的平台快速通道——Windows 的 `down:shm`/`up:shm`、Linux 的 `up:post`/`down:poll`——在 CI 上按 §6.3 判定默认开关，并在 iOS 真机上验收 `up:post`。

**Architecture:** 核心（`lanes.go`、`internal/lane`、`internal/downq`）的协议、世代和令牌不动。Linux 不需要新协议：只让后端报告 `up:post`/`down:poll`，复用核心的 HTTP 通道和 runtime-js 已有的 `lanes/scheme.ts`、`lanes/poll.ts`。Windows 新增两块按世代创建的共享内存环（纯 Go 环实现放 `internal/lane/shmring`，可在 macOS 上测），平台层负责 COM 绑定、门铃和投递；runtime-js 新增 `lanes/shm.ts`。Windows 和 Linux 的验证都在 GitHub CI 的新工作流上跑。

**Tech Stack:** Go（purego/COM vtable 调用，无 cgo 的 Windows 后端），WebView2（`ICoreWebView2Environment12`、`ICoreWebView2_17`、`ICoreWebView2SharedBuffer`），WebKitGTK 4.1，TypeScript（runtime-js，node --test），GitHub Actions。

**Spec:** `docs/specs/2026-11-13-ipc-duplex-streams.md`（§6.1 分层与世代、§6.2 Windows/Linux 通道、§6.3 验收）；`docs/superpowers/specs/2026-11-14-down-poll-design.md`（Linux 的下行沿用它）。

## Global Constraints

- 提交作者固定 `tango <903300326@qq.com>`，提交信息**不加** `Co-Authored-By` 或任何 Claude 署名（用户规则，优先于其他指示）。
- 推送前必须全部通过：`gofmt -l .`（含带构建标签的文件）为空、`GOOS=linux go vet ./...`、`GOOS=windows go vet ./...`、`apitxt -check`、相关包测试；runtime-js 改动后 `npm run build` 并提交 `runtime-js/runtimejs/tango.iife.js`（CI 会检查它是否过期）。
- 只推功能分支 `feat/ipc-lanes-win-linux`；**推 main 前必须先问用户**。
- go 命令统一：`nice -n 19 env GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0 GOPROXY=off go … -p 1`（zsh 下用 shell 函数包一层，不要把命令放进变量）。
- 本机测试不得抢前台；前台测量要先和 min-78 排期。
- 快速通道只开给资源来源的页面：macOS/iOS/Linux `tango://app`，Windows/Android `https://tango.localhost`（§6.1）。
- 默认开启的门槛（§6.3）：在该平台跑 §5.8 基准，64 KiB 帧吞吐 ≥ 兜底传输的 2 倍，且 64 B 帧 p99 不比兜底差；不满足就保留代码、默认关闭，文档写明数据和原因。
- 每个平台都要有"通道中途坏掉"的测试：后续帧自动走消息通道、丢帧的流报 `transport` 错误、其他流不受影响。
- `TANGO_IPC_LANES=0` 和 `Options.DisableFastLanes` 永远优先于各平台开关。
- Windows SharedBuffer 条件：能 `QueryInterface` 到 `ICoreWebView2Environment12` 和 `ICoreWebView2_17`（Runtime ≥ 111）；Linux `up:post` 要 WebKitGTK ≥ 2.40（`webkit_uri_scheme_request_get_http_body`），`down:poll` 要 2.36+ 的流式协议处理（`schemestream_linux.go`）。
- WebView2 的 IID 和 vtable 槽位只能抄自官方 `WebView2.h`（Microsoft.Web.WebView2 NuGet 包），不许凭记忆写；在代码旁注明来源版本。

## Review Focus

1. **导航与发送共享内存赛跑**：hello 之后、`PostSharedBufferToScript` 之前页面导航或重载了——旧世代的环绝不能交给新页面（Task 4 测试 `TestShmPostSkippedAfterNavigation`）。
2. **ARM64 内存序**：JS 侧对共享内存只能普通读写（不是 SharedArrayBuffer，没有 Atomics），在 Windows on ARM 上可能乱序看到写位置先于数据——CI 工作流必须包含 `windows-11-arm`（Task 3），读方对每条记录做长度校验（Task 2）。
3. **页面写入垃圾数据**：上行环的长度、位置被页面写乱时，Go 不得 panic、越界读或死循环，只停用通道（Task 2 的 `FuzzReader`）。
4. **页面卡死时下行环满**：`SendLanePacket` 按 `lane.New` 的约定必须非阻塞，所以排队等空间的是后端的写 goroutine；页面卡住时队列到上限就拒收（帧改走消息通道），页面导航、关闭或世代结束时写 goroutine 退出、旧世代的排队包丢弃（Task 4 测试 `TestShmQueueDroppedOnNavigation`、`TestShmQueueFullRefuses`）。
5. **Linux 能力不足时不能报告通道**：WebKitGTK < 2.40 或没有流式协议处理时不报告对应通道，尤其不能让 25 s 的轮询请求落到主线程同步路径上（Task 5 测试 `TestLinuxLaneCapsGating`）。

---

### Task 1: 核心为每个世代通知平台层

**Files:**
- Modify: `internal/platform/ext.go`（BinaryLane 一节后）
- Modify: `lanes.go`（`LaneHello`）
- Test: `lanes_test.go`（或现有的核心通道测试文件）

**Interfaces:**
- Produces:
  ```go
  // LaneGenerations is optionally implemented by a BinaryLane whose lanes
  // need setting up for each page generation (Windows shared buffers).
  type LaneGenerations interface {
      // LaneGeneration is called after the core started lane generation gen
      // for the page's hello, with the lanes it offered (nil: none, the
      // generation has no lanes). Any goroutine; must not block and must
      // not call back into the core's lane state.
      LaneGeneration(gen uint64, lanes []string)
  }
  ```

- [ ] **Step 1: 写失败测试** `TestLaneHelloNotifiesGeneration`：用 fake 平台 surface 同时实现 `BinaryLane` 和 `LaneGenerations`，资源来源页面发两次 hello，断言回调收到两次，`gen` 递增且等于 `s.lane.Generation()`，`lanes` 等于 `LaneCaps()`；`DisableFastLanes` 时回调收到 `lanes == nil`。
- [ ] **Step 2: 跑测试确认失败**（`go test -run TestLaneHelloNotifiesGeneration .`，预期：接口未定义）。
- [ ] **Step 3: 实现**：`LaneHello` 在 `s.lane.Hello(...)` 之后，若 `bl` 实现 `LaneGenerations`，用 `s.lane.Generation()` 和本次实际提供的通道调用它；不在任何核心锁内调用。
- [ ] **Step 4: 跑测试确认通过**，再跑 `go test . ./internal/lane/...`。
- [ ] **Step 5: 提交** `feat(lane): tell backends when a lane generation starts`。

### Task 2: 共享内存环（纯 Go，平台无关）

**Files:**
- Create: `internal/lane/shmring/ring.go`、`internal/lane/shmring/ring_test.go`
- Create: `internal/lane/shmring/testdata/golden.bin`（由测试 `-update` 生成，供 Task 6 的 TS 测试比对）

**环格式（Go 与 TS 必须一致，写进 ring.go 包注释）：** 小端。头部 64 字节：`@0 write u32`（写方拥有，已写字节累计，按 2^32 回绕）、`@4 read u32`（读方拥有）、`@8 readerSleeping u32`（读方拥有，1 = 已休眠、要门铃）、`@12 writerWaiting u32`（写方拥有，1 = 环满在等）、`@16 gen u64`、`@24 magic u32 = 0x31524754`（"TGR1"）、`@28 capacity u32`（数据区字节数，2 的幂，≥ 64 KiB），其余保留为 0。数据区从 64 开始，位置 = 计数 & (capacity−1)。记录 = `len u32 | payload`，整条按 4 字节对齐；末尾连续空间放不下整条记录时，若剩余 ≥ 4 字节写 `len = 0xFFFFFFFF` 回绕标记，然后从数据区开头写；剩余正好为 0 时不写标记。单条 payload 上限 `capacity/2`。

**休眠/门铃协议（两个方向相同）：** 读方读空后：写 `readerSleeping=1` → 重读 `write` → 若有新数据则写回 0 并继续读，否则等门铃；收到门铃先写 `readerSleeping=0` 再读。写方推进 `write` 后若读到 `readerSleeping==1` 就发门铃（可合并，重复门铃无害）。环满时写方写 `writerWaiting=1` → 重读 `read` → 仍满则等门铃；读方推进 `read` 后若 `writerWaiting==1` 就给写方发门铃。Go 侧对头部字段一律用 `sync/atomic` 的 32 位读写。

**Interfaces:**
- Produces:
  ```go
  const HeaderSize = 64
  const Magic uint32 = 0x31524754
  type Ring struct{ /* mem []byte */ }
  func Init(mem []byte, gen uint64) (*Ring, error) // 写头部；len(mem)-64 必须是 ≥64 KiB 的 2 的幂
  func Open(mem []byte, gen uint64) (*Ring, error) // 校验 magic/capacity/gen，不改头部
  func (r *Ring) MaxPacket() int                   // capacity/2
  func (r *Ring) TryWrite(p []byte) (ok, wakeReader bool) // 放不下返回 ok=false 并置 writerWaiting
  func (r *Ring) Read(dst func(p []byte)) (n int, wakeWriter bool, err error) // 读完所有完整记录；每条记录在 dst 返回后才推进 read；记录非法时 err != nil
  func (r *Ring) Sleep() (mustDrain bool)          // 读方：置 readerSleeping 并复查
  func (r *Ring) Woken()                           // 读方：清 readerSleeping
  func (r *Ring) WriterWoken()                     // 写方：清 writerWaiting
  ```

- [ ] **Step 1: 写失败测试**：`TestRingRoundTrip`（1 B、4 KiB、`MaxPacket()` 三种大小各 1000 条，读出逐字节相等）；`TestRingWrapMarker`（构造写位置使下一条正好跨过末尾、正好落在末尾、末尾剩 4 字节三种情况）；`TestRingFullSetsWriterWaiting`（写满后 `TryWrite` 返回 false 且头部 `@12 == 1`，读出一条后 `Read` 返回 `wakeWriter=true`）；`TestRingSleepRecheck`（`Sleep` 前后插入一次写，`Sleep` 返回 `mustDrain=true`）；`TestRingRejectsBadHeader`（magic、capacity 非 2 的幂、gen 不符三种 `Open` 报错）；`TestRingGolden`（固定序列写入后与 `testdata/golden.bin` 逐字节相等，`-update` 时重写）。
- [ ] **Step 2: 写 `FuzzReader`**：任意字节填充头部与数据区后调用 `Read`，不得 panic、不得读出数据区之外、调用 `dst` 的总字节数不超过 capacity；长度超过 `MaxPacket()`、超过已写字节或不是回绕标记的非法值都返回 err。
- [ ] **Step 3: 跑测试确认失败**，实现 `ring.go`，再跑 `go test ./internal/lane/shmring/ -run . ` 和 `go test ./internal/lane/shmring/ -fuzz FuzzReader -fuzztime 60s`。
- [ ] **Step 4: 提交** `feat(lane): shared-memory ring for down:shm and up:shm`。

### Task 3: CI 通道工作流

**Files:**
- Create: `.github/workflows/ipc-lanes.yml`
- Create: `scripts/ipc-bench-ab.sh`

**Interfaces:**
- Produces: 工作流 `ipc-lanes`，触发：`workflow_dispatch` 和推送到 `feat/ipc-lanes-*`；矩阵 `ubuntu-latest`、`windows-latest`、`windows-11-arm`。产物 `ipc-lanes-<os>`：`e2e.log`、`bench.jsonl`。
- Produces: `scripts/ipc-bench-ab.sh <rounds> <out.jsonl>`：交替跑 A（`TANGO_IPC_LANES=0`）和 B（该平台的开关设为全开：Linux `TANGO_IPC_LANES_LINUX=1`，Windows `TANGO_IPC_LANES_WINDOWS=1`）各 `<rounds>` 次 `go run ./examples/duplex -bench`，每行 JSON 加字段 `"arm":"A"|"B"` 和 `"round":i`。e2e 步骤的"通道开"一次用 `TANGO_E2E_LANES=1`（Task 5 定义）。仓库是公开的，可以用 `windows-11-arm` 托管机。

- [ ] **Step 1**：Linux 依赖与 xvfb 包装照抄 `ci.yml` 的 smoke 任务（`libwebkit2gtk-4.1-0`、`xvfb`、`dbus-run-session -- xvfb-run -a -s "-screen 0 1280x1024x24"`）。
- [ ] **Step 2**：步骤：`TANGO_E2E=1 go test ./examples/duplex -run 'E2E' -v`（通道开和关各一次，日志写 `e2e.log`）；`bash scripts/ipc-bench-ab.sh 5 bench.jsonl`；`actions/upload-artifact` 上传两者，`if: always()`。
- [ ] **Step 3**：本地用 `bash -n scripts/ipc-bench-ab.sh` 检查语法，在 macOS 上用 `TANGO_IPC_LANES_DARWIN` 替代开关试跑 1 轮（需前台时先和 min-78 排期，否则只检查 JSONL 格式）。
- [ ] **Step 4: 提交** `ci: ipc-lanes workflow with interleaved lane benchmark`，推到 `feat/ipc-lanes-win-linux`，确认工作流能启动（此时 Windows/Linux 还没有通道，B 组应与 A 组相同，`lane` 字段为 `fallback`）。

### Task 4: Windows 后端 down:shm / up:shm

**Files:**
- Create: `internal/platform/windows/lanes.go`（纯 Go，无构建标签，本机可测：开关解析，以及 `shmLanes` 控制器——世代、导航序号、下行发送队列与写 goroutine、上行读 goroutine、门铃合并；所有 COM 操作通过注入的函数完成）、`internal/platform/windows/lanes_test.go`
- Create: `internal/platform/windows/lanes_windows.go`（只做 COM 绑定和把注入函数接到真实 WebView2 上）
- Modify: `internal/platform/windows/webview2_vtbl_windows.go`（新增三个接口的 vtable 与 IID）
- Modify: `internal/platform/windows/surface_windows.go`（创建时读开关；`WebMessageReceived` 拦截门铃；导航开始、关闭时释放环）

**Interfaces:**
- Consumes: Task 1 `platform.LaneGenerations`；Task 2 `shmring`。
- Produces: `func laneCapsFromEnv(getenv func(string) string, supported bool) []string`，开关 `TANGO_IPC_LANES_WINDOWS`，取值语义同 Android 的 `laneCapsFromEnv`（`"0"/"false"/"off"` 关闭；其余开启）。**默认值先设为关**（`shmDefault = false`，未设变量时不报告），Task 7 按基准决定是否改为开。
- Produces: 门铃消息格式。Go→页面：`PostWebMessageAsString` 发 `{"tango":"lane","op":"wake","ring":"down"|"up","gen":N}`；页面→Go：`chrome.webview.postMessage("\x00tango-int:lane:" + JSON.stringify({op:"wake",ring:"up"|"down",gen:N}))`。后端在 `webMessageReceived` 里只拦截 `op=="wake"` 的这一种并且不转发给 Host（与现有 `sxDropMessage` 的拦截方式相同），且只接受来自资源来源 `https://tango.localhost` 的门铃；其余 lane 控制消息照旧交给核心。
- Produces（`lanes.go`，注入点）:
  ```go
  type shmOps struct {
      create func(size int) (mem []byte, release func(), err error) // CreateSharedBuffer + get_Buffer
      post   func(ring string, gen uint64, buf any) bool             // PostSharedBufferToScript，UI 线程
      wake   func(msg string)                                        // PostWebMessageAsString，UI 线程
      onUI   func(fn func())                                         // 投递到 UI 线程
      deliver func(gen uint64, p []byte) bool                        // LaneHost.OnLanePacket
  }
  ```
- Produces: 共享内存的 `additionalDataAsJson`：`{"tango":"lane","ring":"down"|"up","gen":N}`；每环大小 `4 MiB + 64`（`TANGO_IPC_SHM_BYTES` 可改数据区大小，必须是 2 的幂）。

- [ ] **Step 1: 纯 Go 开关测试** `TestLaneCapsFromEnvWindows`：未设变量（默认关）→ nil；`"1"` → `[down:shm up:shm]`；`supported=false` → nil；`"off"` → nil。
- [ ] **Step 2: COM 绑定**：从官方 `WebView2.h` 抄 `ICoreWebView2Environment12`（`CreateSharedBuffer`）、`ICoreWebView2_17`（`PostSharedBufferToScript`）、`ICoreWebView2SharedBuffer`（`get_Size`、`get_Buffer`、`Close`）的 IID 与槽位，注明头文件版本；`LaneCaps` 报告的前提是两次 `QueryInterface` 都成功。
- [ ] **Step 3: 世代处理**：`LaneGeneration(gen, lanes)` 若含 `down:shm` 就记下"文档序号 + gen"并投递到 UI 线程；UI 线程上若此后没有开始新的导航，才 `CreateSharedBuffer` 两块、`shmring.Init`、`PostSharedBufferToScript`（READ_WRITE）；否则什么都不做。新世代、导航开始、surface 关闭都 `Close` 旧环并唤醒所有等待者。
- [ ] **Step 4: 下行**：`SendLanePacket(gen, p)` 必须**非阻塞**（`internal/lane/lane.go` 中 `New` 的约定）：gen 不是当前环的世代 → false；`len(p) > MaxPacket()` → false；未写入字节超过 `shmQueueMax = 8 << 20` → false；否则拷贝进队列并唤醒该世代的写 goroutine，返回 true。写 goroutine 用 `TryWrite` 写环，环满时等页面的 `ring:"down"` 门铃再写；需要叫醒页面时经 `onUI` 投递一次合并的 `wake`。世代结束、导航开始、surface 关闭时写 goroutine 退出，队列丢弃（与 Android 的 `laneState.flush` 同语义）。
- [ ] **Step 5: 上行**：每个世代一个读 goroutine：被门铃唤醒后 `Read`，每条记录**先拷贝**再交给 `deliver`（`LaneHost.OnLanePacket(id, origin, gen, copy)`，`origin` 用后端记录的当前文档来源）；`Read` 报错就停用该环（停止读、不再唤醒），由核心的 5 s 间隙超时让受影响的流报 `transport`。
- [ ] **Step 6: 控制器测试**（`lanes_test.go`，用假的 `shmOps` 和普通 `[]byte` 当共享内存，本机就能跑）：`TestShmPostSkippedAfterNavigation`（`LaneGeneration` 后、`onUI` 回调执行前调用导航开始，断言 `post` 没被调用、`create` 出来的环被 release）；`TestShmQueueDroppedOnNavigation`（环满且队列有包时导航开始，写 goroutine 100 ms 内退出，之后新世代的环里没有旧包）；`TestShmQueueFullRefuses`（页面不读，队列到 `shmQueueMax` 后 `SendLanePacket` 返回 false）；`TestShmUpCopiesBeforeDeliver`（`deliver` 返回后改写共享内存，处理函数保存的字节不变）；`TestShmWakeRejectsOtherOrigin`（非资源来源的门铃被忽略）。`go test -race ./internal/platform/windows/ -run Shm`。
- [ ] **Step 7: 故障注入**：`internal/lane/lane.go` 的 `known` 已包含 `down:shm`/`up:shm`，核心按名字计数和拒绝，`TANGO_IPC_LANE_FAULT=down:shm:N`/`up:shm:N` 无需后端改动；这里只确认核心拒收后后端不会继续投递同一世代的包。
- [ ] **Step 8**：`GOOS=windows go vet ./internal/platform/windows/...`、`go test -race ./internal/platform/windows/ -run 'LaneCaps|Shm'`（本机跑纯 Go 部分），提交 `feat(windows): down:shm and up:shm over WebView2 shared buffers`。

### Task 5: Linux 后端 up:post / down:poll

**Files:**
- Create: `internal/platform/linux/lanes.go`（纯 Go 开关，无构建标签）、`internal/platform/linux/lanes_test.go`
- Create: `internal/platform/linux/lanes_linux.go`（`BinaryLane` 实现）
- Modify: `internal/platform/linux/surface_linux.go` 或创建 surface 的文件（创建时读开关）

**Interfaces:**
- Produces: `func laneCapsFromEnv(getenv func(string) string, hasBody, hasStream bool) []string`，开关 `TANGO_IPC_LANES_LINUX`，取值语义照抄 darwin（`"1"/"all"` 两条、`"up"`/`"down"` 单条、`"0"/"false"/"off"` 无；未设或其他值 = 默认）。默认值先设 `upPostDefault = false`、`downPollDefault = false`，Task 7 按基准改。`up:post` 需要 `hasBody`（`webkitURISchemeRequestGetHTTPBody != nil`），`down:poll` 需要 `hasStream`（`bindSchemeStream()` 成功）；缺哪个就不报告哪个。顺序 `down:poll, up:post`，与 darwin 相同。
- Produces: `SendLanePacket` 恒返回 false（两条都是核心的 HTTP 通道，没有平台自己的下行）。

- [ ] **Step 1: 写失败测试** `TestLinuxLaneCapsGating`：`"1"` 且 `hasBody,hasStream` → `[down:poll up:post]`；`"1"` 且 `hasBody=false` → `[down:poll]`；`"1"` 且 `hasStream=false` → `[up:post]`；未设 → nil（默认关）；`TANGO_IPC_LANES=0` → nil。
- [ ] **Step 2: 实现**开关与 `BinaryLane`；在 `lanes_linux.go` 包注释写明与 darwin 一样依赖令牌而不是帧来源挡子帧。
- [ ] **Step 3: 确认 POST 请求体到达**：读 `asset.go`/`scheme_linux.go`，确保 `/__tango/lane/up` 的 POST 走 `webkit_uri_scheme_request_get_http_body` 读出完整 ArrayBuffer 请求体，`/__tango/lane/poll` 走流式（goroutine）路径；缺的话在这里补上并加单元测试。
- [ ] **Step 4: E2E 断言**：`examples/duplex/e2e_test.go` 的 `TestE2EDuplex` 先把子进程环境里的 `TANGO_IPC_LANES_LINUX`、`TANGO_IPC_LANES_WINDOWS` 和现有的 `TANGO_IPC_LANES_DARWIN` 一样清空，再由测试自己按 `TANGO_E2E_LANES=1` 设置；linux 在开启时要求 `{"down":"down:poll","up":"up:post"}`，未开时要求两者都是 `fallback`。（Windows 的断言在 Task 6 里加，那时前端模块才有。）
- [ ] **Step 5**：`GOOS=linux go vet ./internal/platform/linux/...`、`go test ./internal/platform/linux/ -run LaneCaps`，提交 `feat(linux): offer up:post and down:poll`，推功能分支，看 `ipc-lanes` 工作流 ubuntu 的 e2e 是否通过。

### Task 6: runtime-js 的 down:shm / up:shm 模块

**Files:**
- Create: `runtime-js/src/lanes/ring.ts`（与 Task 2 同格式的读写）、`runtime-js/src/lanes/shm.ts`
- Modify: `runtime-js/src/index.ts` / `iife.ts`（与 `android.ts` 相同的方式引入，模块加载时 `registerLane`）
- Test: `runtime-js/test/shm-lanes.test.mjs`
- Regenerate: `runtime-js/runtimejs/tango.iife.js`

**Interfaces:**
- Consumes: Task 2 的环格式与 `internal/lane/shmring/testdata/golden.bin`；Task 4 的门铃与 `additionalDataAsJson` 格式。
- Produces: `registerLane("down:shm", …)`、`registerLane("up:shm", …)`。

- [ ] **Step 1: 写失败测试**：`ring golden`（读 `golden.bin` 解出与 Go 测试相同的序列；按同一序列写出的字节与 `golden.bin` 相等）；`buffers before hello`（`sharedbufferreceived` 先到、`start(ctx)` 后到，仍能用上同世代的环）；`buffers of another gen are released`（gen 不符的环立刻 `chrome.webview.releaseBuffer`）；`no buffers within 5 s fails the lane`（`ctx.fail` 被调用）；`up writes queue while full and dispatch on wake`（环满时帧留在 JS 队列，收到 `ring:"up"` 门铃后写入并按 seq 顺序调用 `ctx.dispatched`）；`down wake drains and sleeps`（收到门铃读出全部记录交给 `ctx.onPacket`，读空后头部 `@8 == 1`）。用 fake `chrome.webview`（`addEventListener('sharedbufferreceived'|'message')`、`postMessage`、`releaseBuffer`）。
- [ ] **Step 2: 跑** `cd runtime-js && npm test`，确认失败。
- [ ] **Step 3: 实现**：环到了且 `ctx.gen` 匹配后发 `ready`（走现有 lanes.ts 的控制消息）；`UpLane.send` 在包超过 `capacity/2` 时返回 false；被替换的世代立即 `releaseBuffer`；`BinaryMinSize` 由 Go 侧定为 0，JS 不另设阈值。
- [ ] **Step 4**：`npm test`、`npm run typecheck`、`npm run api:check`（API 报告有变化就 `api:update` 并说明），`npm run build`，提交源码和 `tango.iife.js`：`feat(runtime-js): down:shm and up:shm lanes`。
- [ ] **Step 5: Windows E2E 断言**：`TestE2EDuplex` 在 windows 且 `TANGO_E2E_LANES=1` 时要求 `{"down":"down:shm","up":"up:shm"}`，未开时两者都是 `fallback`；推分支，看 `windows-latest` 与 `windows-11-arm` 的 e2e 通过。

### Task 7: CI 验收、默认值与通道故障测试

**Files:**
- Modify: `examples/duplex/e2e_test.go`（新增 `TestE2ELaneSelftest`）、按需 `examples/duplex/main.go`
- Modify: `internal/platform/windows/lanes.go`、`internal/platform/linux/lanes.go`（只改默认值常量）

- [ ] **Step 1: 故障场景**：桌面的 `examples/duplex` 目前只有 `TANGO_SCHEME_PROBE=1` 时的 `/__test` 路由，没有自测页面。把 `examples/duplex-mobile` 的自测接法搬过来：`TANGO_SCHEME_SELFTEST=1` 时打开 `index.html?selftest`（与 `TANGO_IPC_LANE_FAULT` 同时设时打开 `index.html?selftest&lanefault`，只跑通道故障场景），页面复用 `duplex-mobile/web/selftest.js` 的场景代码（移到两个示例都能引用的位置，不要复制一份），结果打印成一行 `selftest: {...}` 到 stdout。新增 `TestE2ELaneSelftest`：`-tags tango_selftest` 构建、开启该平台通道，要求顶层 `"ok":true`；再分别以 `TANGO_IPC_LANE_FAULT=<下行通道>:20`、`<上行通道>:20` 跑 lanefault，同样要求 `"ok":true`（lanefault 场景本身检查改走消息通道、丢帧的流报 `transport`、另一条流不受影响）。把它加进 `ipc-lanes` 工作流的 e2e 步骤。
- [ ] **Step 2: 跑基准**：推分支，取 `ubuntu-latest`、`windows-latest`、`windows-11-arm` 三份 `bench.jsonl`（5 轮交替）。按 §6.3 对每条通道判定：B 组 `upMBps`/`downMBps` 中位数 ≥ A 组 2 倍；B 组 `latencyBatchP99Ms` 中位数 ≤ A 组。CI 机器噪声大，如果某条通道判定结果在两次工作流运行之间翻转，就按不达标处理。若只有小帧延迟不达标（核心 `lane.State.MinSize` 对所有通道都返回 0，小帧也走通道），不要在这里改核心阈值机制，把数据报给用户决定。
- [ ] **Step 3: 改默认值**：达标的通道把对应默认常量改为开，并把 e2e 断言里"未设变量"的期望同步改掉；不达标的保留关闭。
- [ ] **Step 4**：工作流全绿后提交 `feat(lanes): Windows and Linux lane defaults from the CI benchmark`。

### Task 8: 文档

**Files:**
- Modify: `docs/specs/2026-11-13-ipc-duplex-streams.md`（文首状态改为"已实现"并写日期；§6.2 Windows、Linux 两节补"状态"段：开关名、默认值、基准数字、判定；§6.3 状态段补 Windows/Linux）
- Modify: `docs/ipc-bench.md`（新增"第二期 Windows（down:shm / up:shm）"和"第二期 Linux（up:post / down:poll）"两节：方法 = CI 交替 5 轮，注明机型和 Runtime/WebKitGTK 版本，表格同 Android 一节）
- Modify: `CHANGELOG.md`、`docs/dev-builds.md`（新开关 `TANGO_IPC_LANES_WINDOWS`、`TANGO_IPC_LANES_LINUX`、`TANGO_IPC_SHM_BYTES`）、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`（各平台通道表）

- [ ] **Step 1**：按上面逐个更新；数字只从 Task 7 的 `bench.jsonl` 里取，不估算。
- [ ] **Step 2**：跑 `apitxt -check` 和站点文档检查（与 CI 的 Docs 任务相同的命令），提交 `docs(ipc): Windows and Linux lanes`。

### Task 9: iOS 真机验收 up:post（需要用户在场）

**Files:**
- Modify（视结果）: `internal/platform/ios/lanes.go`（默认值）、`docs/ipc-bench.md`（iOS 真机一节）、规格 §6.3 状态段

- [ ] **Step 1**：请用户连上 iPhone 并信任这台 Mac；`xcrun devicectl list devices` 确认可见。若 `TANGO_IOS_SIGN_IDENTITY` 未设，请用户提供签名身份（不要自己挑证书）。
- [ ] **Step 2**：`bash mobile/ios/build.sh examples/duplex-mobile device`（`-tags tango_selftest`），`xcrun devicectl device install app` 安装。
- [ ] **Step 3**：交替各 5 次启动：A 组 `TANGO_DUPLEX_BENCH=1`，B 组 `TANGO_DUPLEX_BENCH=1 TANGO_IPC_LANES_DARWIN=1`，用 `xcrun devicectl device process launch --console` 收集 `bench: {...}` 行（传环境变量的参数名以本机 `xcrun devicectl device process launch --help` 为准）；再跑一次 selftest（`TANGO_SCHEME_SELFTEST=1`）要求 `"ok":true`，一次 `TANGO_IPC_LANE_FAULT=up:post:20`。
- [ ] **Step 4**：按 §6.3 判定 `up:post`（只看上行，`upMBps` ≥ 2 倍且 `latencyBatchP99Ms` 不差）。达标就把 iOS 默认改为开，文档补真机数据；不达标保留关闭并写明数据。
- [ ] **Step 5**：提交 `feat(ios): up:post default from the device benchmark`（或 `docs(ipc): iOS device benchmark` 若不改默认）。

### 收尾

- [ ] 全分支：`gofmt -l .` 为空、`GOOS=linux go vet ./...`、`GOOS=windows go vet ./...`、`apitxt -check`、`go test ./...`（本机）、`ipc-lanes` 与 `ci` 两个工作流在功能分支上全绿。
- [ ] 向用户汇报每条通道的判定和数字，**问过用户后**再合并到 main 并推送。
