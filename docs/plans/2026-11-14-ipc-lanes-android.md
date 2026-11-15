# IPC 快速通道 · Android 平台接入 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Android WebView 页面用上平台通道 `down:android` / `up:android`：上行 JS `postMessage(ArrayBuffer)` 经 `WebMessageListener` 到 Go，下行 Go 经 `JavaScriptReplyProxy.postMessage(byte[])` 到页面。在模拟器上跑完"中途坏掉""重载""卡顿""iframe"等场景，按规格 §6.3 跑基准，默认开启由用户决定。

**Architecture:** 公共设施（`internal/lane`、`ipc`、runtime-js 通道框架）已完成并已整合进 main。本计划在三处落地：① Android 后端（Go：`internal/platform/android`；Java：`mobile/android/.../Page.java` 等）实现 `platform.BinaryLane`；② 页面桥接脚本（`internal/platform/android/script.go`）在闭包里持有页面令牌和 `__tangoAndroid`，向运行时暴露最小的通道接口；③ runtime-js 新增 `lanes/android.ts`。两个方向都**按通道世代绑定**：上行包带世代号，核心在同一把锁里校验世代并投递；下行包由核心带着世代号交给后端，后端只把它发给同一世代保存的回复代理。这样旧文档/旧世代的包无论在哪个环节延迟，都到不了新世代。为此 Task 1 改三处公共接口。

**Tech Stack:** Go 1.27 + cgo/JNI（`internal/platform/android`）；Java（androidx.webkit 1.11.0，已在 `mobile/android/app/build.gradle`；已离线核实含 `JavaScriptReplyProxy.postMessage(byte[])`、`WebMessageCompat.TYPE_ARRAY_BUFFER`/`getArrayBuffer()`、`WebViewFeature.WEB_MESSAGE_ARRAY_BUFFER`）；TypeScript（runtime-js）。验证：Android 模拟器 `emulator-5554`（AVD `tango36`，API 36，arm64-v8a，WebView 133.0.6943.137），SDK `~/Library/Android/sdk`，NDK 28.2.13676358，JDK 21。`mobile/android/build.sh` 默认产出可调试 APK（`assembleDebug`），`tan build` 保留 `GOFLAGS` 里的 `-tags=tango_selftest`。

**Spec:** `docs/specs/2026-11-13-ipc-duplex-streams.md` §6.1（公共设施与实施裁定）、§6.2「Android：WebMessageListener 的 ArrayBuffer 消息（上下行）」、§6.3 验收。参考已执行的 `docs/plans/2026-11-14-ipc-lanes-apple.md`。计划审查报告：`.superpowers/sdd/android-plan-builder-check.md`（28 条，已全部并入本版）。

## Global Constraints

- 通道名 `down:android` / `up:android`（`internal/lane` 已定义），`BinaryMinSize` 0，公共包格式 `count:u16 | (streamID:u64 seq:u64 len:u32 payload)*`。除 Task 1 的三处接口改动外，**不改** `internal/lane`、`ipc`、runtime-js 框架（`lanes.ts`、`client.ts`、`packet.ts`）的语义；发现其他公共缺陷，报告给控制者。
- 启用条件（全部满足才报告两条通道）：`WebViewFeature.isFeatureSupported(WEB_MESSAGE_LISTENER)` 且 `isFeatureSupported(WEB_MESSAGE_ARRAY_BUFFER)`（WebView ≥ 116）；页面桥接走 `WebMessageListener`（`JavascriptInterface` 兜底时没有通道）；开关打开。
- 开关：`TANGO_IPC_LANES_ANDROID` 在**创建页面时**读一次：未设、空、`0`/`false`/`off`（忽略大小写和首尾空格）→ 关；其他值 → 开。`TANGO_IPC_LANES=0` / `Options.DisableFastLanes` 由核心处理（`lanesOff`），后端不重复判断。基准达标且**用户确认**之前默认关闭（Task 6）。
- 上行包（页面 → Java → Go）：`token[16] | gen:u64(LE) | 公共包`。`token` 是当前文档页面令牌（`token.go`，32 位十六进制）解码成的 16 字节；`gen` 是 hello 给的通道世代（`LaneContext.gen`）。Go 校验：长度 ≥ 26；令牌与当前或待生效令牌常量时间相等（`subtle.ConstantTimeCompare`）；listener 的 `sourceOrigin` 与令牌所属来源 `sameOrigin`；`isMainFrame`（Java 已过滤）。令牌或来源不符：静默丢弃，不回复。
- **探测包**：恰好 `token[16] | gen | 00 00`（总长 26）。其他以 `00 00` 开头的包不是探测。
- 回复（Java → 页面，经保存的 `JavaScriptReplyProxy`，都是 `ArrayBuffer`）：长度 1 `[0]` = 探测确认；长度 1 `[1]` = 上行包/探测被核心拒绝；长度 ≥ 2 = 下行公共包。（不用长度 0：零长 `byte[]` 的投递没有核实过；Task 2 Step 4 在模拟器上先验证 `[0]`/`[1]` 能到达。）每个探测恰好得到一个回复（`[0]` 或 `[1]`），页面按先进先出与自己的探测配对。
- 下行：核心调用 `SendLanePacket(gen, p)`（任意 goroutine，**不阻塞**）：后端把 `(gen, p)` 放进本页面队列（上限 8 MiB 未交出的字节，超过返回 false），释放队列锁后 `Wake()`；主线程 `drain()` 先 `flushLanes()` 再 `h.Drain()`：每个包若 `gen == 已保存回复代理的世代` 就 JNI 调 `Page.laneSend(byte[])`，否则丢弃并计数。返回 true 后后端拥有 `p`；返回 false 时不保留、不修改。
- 回复代理只在**合法探测**被核心接受（`AllowUpGen(gen, probe)` 为真）时保存，连同它的 `gen`；保存后立即回复 `[0]`。之后同一页面的新探测（新世代或新文档）替换它。页面开始新文档（`onPageStarted`）、WebView 被替换（`recover()`）或销毁（`destroy()`）时清掉代理（新文档的探测在 `onPageStarted` 之前到达也没关系：被清掉的是代理，而队列里的包按世代判断，新文档会因为 Task 3 的"确认前丢弃"和下一次探测恢复——见 Task 2 的 `beginKeepsProbedGen`）。
- 锁顺序：`lane.State.mu` → 后端 `laneQueue.mu`；`SendLanePacket` 只拿队列锁，`Wake()` 在释放队列锁之后调用；`flushLanes` 和 `nativeLanePacket` 调核心或 JNI 时不持有队列锁。`surface.closed` 只在主线程读写，队列自己带 `closed` 标志（`release()` 设置）供其他线程判断。
- Java 侧 listener 回调、`laneSend`、`onPageStarted` 都在主线程；Go 的 `nativeLanePacket` 在主线程被调用并同步调用 `LaneHost.OnLanePacket`（核心只投递给 `OnBinary`，不占主线程做别的）。Listener 必须先按 `getType()` 分支，再调 `getData()`（androidx 1.11.0 对 ArrayBuffer 消息调 `getData()` 会抛 `IllegalStateException`）。
- 测试钩子只编进 `-tags tango_dev` 或 `-tags tango_selftest` 的构建（与 iOS 相同）：Android 的 `testHooks` 常量、通道计数、`TANGO_ANDROID_NO_ARRAYBUFFER`、测试环境变量透传。正式构建不读这些输入（`LaneStatsSnapshot` 在所有构建都存在，只是不计数）。
- 测试环境变量透传（仅测试钩子构建 + 可调试 APK）：启动 Intent 的**字符串数组** extra `TANGO_TEST_ENV`（`am start ... --esa TANGO_TEST_ENV TANGO_A=1,TANGO_B=2`，Java 用 `getStringArrayExtra`；值里不能有逗号），只接受 `TANGO_` 开头的键；在 `nativeInit` 里、`applyDevURL()` 之后、`userMain()` 之前 `os.Setenv`。
- 页面脚本给运行时的接口（仅 `lanes == true` 且走 `postMessage` 分支）：`__tangoNative.lane = Object.freeze({ post(u8), listen(fn) })`。`post` 要求 `u8.byteOffset === 0 && u8.byteLength === u8.buffer.byteLength && u8.length >= 26`，写入前 16 字节令牌，`ch.postMessage(u8.buffer)`（结构化克隆同步复制），然后 `u8.fill(0, 0, 16)` 清掉令牌（令牌只留在闭包里）；不符合要求抛 `TypeError`。`listen(fn)` 只能调用一次，之后 `ch` 的 `message` 事件中 `e.data instanceof ArrayBuffer` 的交给 `fn`（字符串回复照旧）。同源 iframe 能调用 `parent.__tangoNative.lane`——它本来就能用 `parent.__tangoNative.post` 发消息，属于应用代码（规格信任模型），文档写明。
- 不新增外部依赖。
- 工作区：`/Users/admin/min/tango/.claude/worktrees/ipc-duplex-merge`（基于 main，含已整合的前三期）。任务内不提交；计划完成后由控制者整合。
- 文件归属：Task 1：`internal/lane/lane.go`、`internal/lane/*_test.go`、`internal/platform/ext.go`、`lanes.go`、`lanes*_test.go`（根）、`internal/platform/fake/**`、`internal/platform/darwin/lanes.go`、`internal/platform/ios/lanes_ios.go`（只改 `SendLanePacket` 签名）、规格 §6.1。其余：`internal/platform/android/**`；`mobile/android/app/src/main/java/org/tango/host/{Page,Tango,Mobile}.java`；`runtime-js/src/lanes/android.ts`、`runtime-js/src/index.ts`（加 import）、`runtime-js/test/android-lanes.test.mjs`、生成的 `runtime-js/runtimejs/tango.iife.js` 与 `runtime-js/etc/runtime.api.md`；`examples/duplex-mobile/**`（由 `examples/duplex-ios` 改名）、`examples/duplex/services/**`；改名波及的注释：`internal/platform/ios/lanes.go`、`internal/platform/ios/bridge.m`、`internal/platform/darwin/lanes_flow_e2e_test.go`、`examples/duplex/frontend/{probe.html,app.js}`；`scripts/smoke.sh`；`docs/ipc-bench.md`、`docs/mobile.md`、`docs/dev-builds.md`、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`、`CHANGELOG.md`、规格 §6.2/§6.3（实测结论）。
- 每个任务结束（Go）：`go build ./... && go vet . ./internal/lane/... ./internal/platform/...`；`go test -race ./internal/lane/... ./internal/platform/... . ./ipc/...`；`go test -tags tango_selftest . ./internal/lane/...`；Android 交叉 vet：`NDK=~/Library/Android/sdk/ndk/28.2.13676358/toolchains/llvm/prebuilt/darwin-x86_64/bin; GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=$NDK/aarch64-linux-android24-clang go vet -tags "<空|tango_selftest>" ./internal/platform/android/... ./examples/duplex-mobile/...`。（JS）：`cd runtime-js && npm run build && npm run typecheck && node --test test/*.test.mjs && npm run api:check`。（模拟器）任务指定的 `bash scripts/smoke.sh android-selftest` 等。

## Review Focus

1. **旧世代的包串进新世代**：页面 A 正在收发大流时重载，新页面 B 的第一条 JS 流也是 stream 1。B 收发的每个字节都必须是 B 自己的；A 残留在页面、Java 主线程队列或 Go 下行队列里的包（两个方向）都不能进入 B。同一文档发第二次 hello 也一样 —— Task 1 `TestAllowUpGenRejectsOldGeneration`、`TestSendLanePacketCarriesGeneration`；Task 2 `TestLaneUpOldGenDropped`、`TestLaneFlushDropsOtherGen`、`TestLaneBeginKeepsProbedGen`；Task 3 `discards down packets before this probe's ack`；Task 5 `reloadSameID`（上下行两个方向）。
2. **上行通道坏掉页面不知道**：`up:android` 被核心拒收时页面立刻停用并回退，不把后续流送进黑洞 —— Task 1 `OnLanePacket` 返回值；Task 2 拒收回复 `[1]`；Task 3 `failsOnRefusal`、`stale reply ignored`；Task 5 `faultUp`。
3. **iframe 冒用**：同源 iframe 里也有自己的 `__tangoAndroid`（listener 按来源注入所有 frame，桥接脚本只在主 frame 删除它），用它直接 `postMessage(ArrayBuffer)` 或猜令牌，Go 侧一个包都收不到；不透明来源 iframe 没有该对象。（经 `parent.__tangoNative.lane` 发包是应用代码，允许，不在此列）—— Task 5 `subframe`。
4. **主线程长时间忙**：页面同步忙 6 秒期间正在传输的流不报 `transport` —— Task 5 `stall`。
5. **不支持 ArrayBuffer / JavascriptInterface 兜底**：不报告通道，全部走消息通道，语义不变 —— Task 2 `TestLaneCapsConditions`；Task 5 `capsOff`（测试钩子 `TANGO_ANDROID_NO_ARRAYBUFFER=1`：Go 把能力位当假，Java 不读这个变量）。

---

### Task 1: 公共设施的三处接口改动

**目的**：现有接口对 `down:scheme`/`up:post`（请求里带 `gen`+`tok`）足够，平台通道缺三样：① 上行包没有世代，`AllowUpSession()` 在包到达时取"当前"世代，延迟的旧包会被算进新世代（Review Focus 1）；② 下行 `platformSend(p)` 在锁外调用且不带世代，后端分不清包属于哪个世代；③ `OnLanePacket` 没有返回值，后端无法告诉页面上行被拒（Review Focus 2）。另外 `AllowUpSession` 把空探测包计入故障计数，和"只算数据包"的约定不一致。

**Files:**
- Modify: `internal/lane/lane.go`、`internal/lane/lane_test.go`
- Modify: `internal/platform/ext.go`、`lanes.go`、`lanes_test.go`（根）、新建 `lanes_fault_test.go`（`//go:build tango_dev || tango_selftest`）
- Modify: `internal/platform/fake/**`、`internal/platform/darwin/lanes.go`、`internal/platform/ios/lanes_ios.go`（签名）
- Modify: 规格 §6.1 实施裁定、§6.2 Android 小节

**Interfaces:**
- Produces: `func (s *State) AllowUpGen(gen uint64, p []byte) bool`：在同一把锁里检查 `gen == s.gen` 和上行可用性；只有非空包计入故障计数（`!emptyPacket(p)`）。取代 `AllowUpSession()`（唯一调用方是 `lanes.go`，删除旧方法）。
- Produces: `platformSend func(gen uint64, p []byte) bool`；`BinaryLane.SendLanePacket(gen uint64, p []byte) bool`（注释写明：后端只在该世代的页面通道仍有效时投递，否则丢弃；不得回调 `lane.State`）。darwin/iOS/fake 改签名（darwin/iOS 仍返回 false）。
- Produces: `LaneHost.OnLanePacket(id SurfaceID, origin string, gen uint64, p []byte) bool`：true = 交给了 `OnBinary`；false = 丢弃（页面已关、来源不符、`AllowUpGen` 拒绝）。

- [ ] **Step 1: 写失败的测试**

```go
// internal/lane/lane_test.go（同包，直接设置 s.faultLane / 计数，不依赖构建标签）
func TestAllowUpGenRejectsOldGeneration(t *testing.T) // Hello(caps up:android) 得 g1，再 Hello 得 g2：AllowUpGen(g1, pkt)=false，AllowUpGen(g2, pkt)=true；Reset 后都 false
func TestAllowUpGenSkipsEmptyForFault(t *testing.T)   // 故障 up:android:2：空包 [0,0]×3 允许且不计数；随后两个数据包允许，第三个拒绝；拒绝后空包也拒绝
func TestSendLanePacketCarriesGeneration(t *testing.T) // down:android ready 于 g1：platformSend 收到 (g1, pkt)；新 Hello 后 Send 返回 false 且不调用 platformSend

// lanes_test.go
func TestOnLanePacketReportsRefusal(t *testing.T) // 资源来源 + 有 OnBinary + 当前 gen：true 且处理函数收到 (gen, p)；旧 gen → false；非资源来源 → false；Reset 后 → false
// lanes_fault_test.go（tango_dev || tango_selftest）
func TestOnLanePacketFaulted(t *testing.T)        // TANGO_IPC_LANE_FAULT=up:android:1：第二个数据包 → false
```

- [ ] **Step 2: 运行确认失败**：`go test -race ./internal/lane/ -run 'AllowUpGen|SendLanePacketCarries'`、`go test . -run OnLanePacketReports`、`go test -tags tango_selftest . -run OnLanePacketFaulted`
- [ ] **Step 3: 实现**：`send` 的平台分支在锁内取 `gen` 后放锁调用 `platformSend(gen, p)`（锁外调用保留：世代号已经让后端能丢弃过期包，免去锁顺序问题）；`AllowUpGen`；`OnLanePacket` 新签名与返回值；更新所有实现与调用方（`grep -rn 'OnLanePacket\|AllowUpSession\|SendLanePacket\|platformSend'`）。
- [ ] **Step 4: 运行通过**：本任务门槛命令全过；`TANGO_E2E=1 go test ./examples/duplex -run E2E` 和 `TANGO_E2E=1 go test ./internal/platform/darwin -run 'Lane'` 仍通过（macOS 无回归）。
- [ ] **Step 5: 规格**：§6.1「实施裁定」追加：平台上行包带世代、核心原子校验；平台下行包带世代交给后端；`OnLanePacket` 返回是否投递；平台上行空包不计入故障计数。§6.2 Android 小节修订：包格式 `token|gen|packet`；回复代理在合法探测时按世代保存（Java 不解析 ipc 消息，不在 hello 时保存）；回复标记 `[0]`/`[1]`/≥2。
- [ ] **Step 6: 检查点**

---

### Task 2: Android 后端 + Java 宿主

**Files:**
- Create: `internal/platform/android/lanes.go`（纯 Go，无构建约束：开关、上行解析与校验、下行队列、探测/开始新文档的状态机，macOS 上可 `go test`）
- Create: `internal/platform/android/lanes_android.go`（`//go:build android`：`surface` 实现 `BinaryLane`，JNI 薄包装）
- Create: `internal/platform/android/lanes_test.go`
- Create: `internal/platform/android/testhooks_on.go`（`//go:build !android || tango_dev || tango_selftest`，`const testHooks = true`）、`testhooks_off.go`（`//go:build android && !tango_dev && !tango_selftest`）、`testenv_android.go`（`applyTestEnv()`）
- Modify: `exports_android.go`（新导出 `nativeLanePacket`；`nativeInit` 调 `applyTestEnv()`；`nativeBeginPage` 调 `lanes.begin`）、`backend_android.go`（`NewSurface` 在把 `s` 放进 `b.surfaces`、调用 `mNewPage` **之前**算好 `laneCaps`；`drain()` 先 `flushLanes()`；`release()` 关闭队列）、`jni_android.{c,h,go}`（`tg_call_boolean`、`tg_call_static_boolean`、`tg_get_bytes`（`GetByteArrayRegion` 拷进 Go 切片）；`flushLanes` 每包用现有 `env.bytes` 建 `jbyteArray` 并在循环内删除局部引用（`withEnv` 的局部帧容量 32）；新方法加入 `allMethods`）、`script.go`（`pageScript(token, bridge string, lanes bool)`）、`token.go`（令牌的 16 字节形式）
- Modify: `mobile/android/app/src/main/java/org/tango/host/Page.java`、`Tango.java`（native 声明、`static boolean laneSupported()`）、`Mobile.java`（`static String[] testEnv()`：可调试 APK 读 `getStringArrayExtra("TANGO_TEST_ENV")`，否则空）

**Interfaces:**
- Consumes: Task 1 的 `SendLanePacket(gen, p)`、`OnLanePacket(id, origin, gen, p) bool`。
- Produces（纯 Go，`lanes.go`）：
  - `func laneCapsFromEnv(getenv func(string) string, supported bool) []string`
  - `func parseUp(d *docTokens, sourceOrigin string, data []byte) (origin string, gen uint64, packet []byte, probe bool, ok bool)`
  - `type laneState struct{...}`（每个页面一个，并发安全）：`push(gen uint64, p []byte) bool`（8 MiB 上限、关闭后 false）；`take() []queued`；`probed(gen uint64)`（记录代理世代）；`begin(promotedPending bool, pendingProbed bool)`（见下）；`close()`。
  - `func handleUp(st *laneState, d *docTokens, src string, data []byte, deliver func(origin string, gen uint64, p []byte) bool) (code int)`：`0` 令牌/来源不符（不回复）；`1` 已投递（探测：同时 `st.probed(gen)`，Java 保存代理并回复 `[0]`）；`2` 核心拒收（Java 回复 `[1]`）。
  - 开始新文档：`begin` 之后代理世代清零（Java 同时清代理），**除非**这次 `begin` 把待生效令牌提升为当前令牌、且该令牌已经通过了探测（新文档的探测先于自己的 `onPageStarted` 到达主线程）——此时保留代理世代，`nativeBeginPage` 返回"保留"，Java 不清代理。`laneState` 记住探测所用令牌以作判断。
  - `LaneStats{UpPackets, UpProbes, UpRefused, UpBadToken, DownPackets, DownDropped, DownQueued int64}` 与 `func LaneStatsSnapshot() LaneStats`（所有构建都有，只在 `testHooks` 时计数）。
- Produces（JNI）：`Java_org_tango_host_Tango_nativeLanePacket(env, cls, id jlong, sourceOrigin jstring, data jbyteArray) jint`；`nativeBeginPage` 增加返回值（`jboolean keepLane`）或新增 `nativeLaneKeep(id)`——选前者，同时改 Java 调用处。
- Produces（Java）：
  - Listener：`if (message.getType() == WebMessageCompat.TYPE_ARRAY_BUFFER) { byte[] b = message.getArrayBuffer(); int r = Tango.nativeLanePacket(id, sourceOrigin.toString(), b); if (r == 1 && isProbe(b)) { laneReply = reply; reply.postMessage(new byte[]{0}); } else if (r == 2) reply.postMessage(new byte[]{1}); return; }`，之后才是原有的字符串路径（`isProbe`：长度恰为 26 且最后两字节为 0）。
  - `boolean laneSend(byte[] p)`：`closed || laneReply == null` → false；否则 `laneReply.postMessage(p)`。
  - `onPageStarted`：`boolean keep = Tango.nativeBeginPage(...); if (!keep) laneReply = null;`；`recover()`、`destroy()`：`laneReply = null`。
- Produces（页面脚本）：Global Constraints 里的 `__tangoNative.lane`；`lanes` 参数 = 该页面 `laneCaps` 非空，HTML 注入与 `evaluateJavascript` 注入两处都按它生成。

- [ ] **Step 1: 写失败的单元测试**（`lanes_test.go`，无构建约束）

```go
func TestLaneCapsConditions(t *testing.T)    // supported=false → nil；""/"0"/" OFF "/"false" → nil；"1"/"on" → [down:android up:android]
func TestLaneUpParse(t *testing.T)           // 当前令牌+同源 → ok，gen 与 packet 正确；待生效令牌也接受；错一字节/长度 25/异源/begin 后的旧令牌 → ok=false；
                                             // 恰为探测 → probe=true；长度 28 以 00 00 开头 → probe=false
func TestLaneUpOldGenDropped(t *testing.T)   // deliver 对旧 gen 返回 false → code 2（拒收回复）；对令牌不符 → code 0 且 deliver 未被调用
func TestLaneQueueCapAndClose(t *testing.T)  // push 到 8 MiB 为止 true，再 push false；take 后可继续；close 后 push false
func TestLaneFlushDropsOtherGen(t *testing.T) // probed(g2) 后，队列里 (g1,p1)(g2,p2)(g1,p3)：flush 只交出 p2，DownDropped += 2；没有 probed → 全部丢弃
func TestLaneBeginKeepsProbedGen(t *testing.T) // 待生效令牌 T2 的探测(g2)先到 → begin(提升 T2) 返回 keep=true，(g2,p) 仍能 flush；
                                               // 探测用的是旧令牌 T1 → begin 返回 keep=false，代理世代清零
func TestPageScriptLaneAPI(t *testing.T)     // lanes=true：含 lane.post/listen，post 之后清零前 16 字节（字符串检查含 fill(0, 0, 16)）；lanes=false 或 JavascriptInterface 分支：无 lane
```

- [ ] **Step 2: 运行确认失败**：`go test ./internal/platform/android/ -run 'Lane|PageScript'`
- [ ] **Step 3: 实现** Go（含 `applyTestEnv`、`flushLanes`、JNI 辅助、计数）与 Java。
- [ ] **Step 4: 先验证回复标记**：在模拟器上用最小页面（`examples/mobile-hello` 的临时调试页即可，不保留）确认 `reply.postMessage(new byte[]{0})` 在页面收到长度 1 的 `ArrayBuffer`；若不成立，停下报告控制者（回复格式需另定）。
- [ ] **Step 5: 运行通过**：单测；Android 交叉 vet（空标签与 `tango_selftest`）；`bash scripts/smoke.sh android`（原有 mobile-hello 冒烟，桥接脚本无回归）。
- [ ] **Step 6: 检查点**

---

### Task 3: runtime-js 的 `down:android` / `up:android` 模块

**Files:**
- Create: `runtime-js/src/lanes/android.ts`
- Modify: `runtime-js/src/index.ts`（`import "./lanes/android.js"`，与 `lanes/scheme.js` 同处）
- Create: `runtime-js/test/android-lanes.test.mjs`
- Regenerate: `runtime-js/runtimejs/tango.iife.js`、`runtime-js/etc/runtime.api.md`（公开 API 不应变化）

**Interfaces:**
- Consumes: `registerLane`、`LaneContext`、`DownLane`/`UpLane`（`src/lanes.ts`）；`PacketFrame`（`src/packet.ts`）；`__tangoNative.lane`（Task 2）。
- 行为：
  - **总是注册**两个模块（模块加载时）；`start` 时才检查 `globalThis.__tangoNative?.lane`，没有就立即 reject（运行时可能先于桥接脚本加载，见 `Page.java` 的 `evaluateJavascript` 注入路径）。
  - 连接对象放在 `globalThis[Symbol.for("tango.androidLane")]`，ESM 与 IIFE 两份运行时共用；`lane.listen` 全局只调一次。
  - 探测：每次 `start`（每个世代）发一个探测包（26 字节：16 字节预留 + `ctx.gen` 小端 + `00 00`），记入"未答探测"先进先出队列；收到长度 1 的回复时出队配对：只有与本世代探测配对的 `[0]` 算成功、`[1]` 算失败，先前探测的迟到回复只出队不生效；2000 ms 内没有本世代的回复 → 失败。两个模块共用同一世代的探测结果。
  - `down:android`：本世代探测确认**之前**收到的长度 ≥ 2 的 `ArrayBuffer` 全部丢弃（它们属于旧世代：同一代理上，旧包排在确认之前）；之后交给 `ctx.onPacket(new Uint8Array(ab))`。
  - `up:android`：`minSize = 0`；`send(frame)`：在 `android.ts` 内直接把单帧编码进 `26 + 20 + payload` 的新缓冲（前 16 字节预留、`gen`、`count=1`、帧头、数据；不改 `packet.ts`），`lane.post`，随即 `ctx.dispatched(stream, seq)`，返回 true；`post` 抛错 → 返回 false，并 `queueMicrotask(() => ctx.fail(err))`（`ctx.fail` 不得在 `send` 内同步调用，见 `scheme.ts` 的约定）。探测确认之后收到 `[1]` → `ctx.fail(new Error("up:android refused by Go"))`。
  - `stop()`：两个模块都停之后丢弃收到的一切；新 `start` 重新探测。
  - 攒包：本任务不做（每帧一包）；Task 6 若小帧吞吐不达标，按基准结果加 4 ms/16 KiB 攒批（与 `up:post` 相同参数），记 Ruling。

- [ ] **Step 1: 写失败的测试**（假的 `__tangoNative.lane`：记录 `post` 的缓冲，手动触发 `listen` 回调；假定时器）

```js
test("start rejects at once without __tangoNative.lane")
test("probe: 26 bytes, gen little-endian, ends 00 00; resolves on [0]")
test("probe: rejects on [1] and after 2000 ms without a reply")
test("stale reply ignored: a late [1] for the previous generation's probe does not fail the current start")
test("discards down packets before this probe's ack, delivers those after")
test("up send: one packet per frame with 16-byte headroom and gen; dispatched in seq order")
test("failsOnRefusal: [1] after start fails up:android via ctx.fail (not inside send)")
test("post throwing: send returns false, ctx.fail runs in a microtask")
test("two runtime copies share one connection; listen is called once")
test("after stop, messages are dropped; a new start probes again")
```

- [ ] **Step 2: 运行确认失败**：`cd runtime-js && node --test test/android-lanes.test.mjs`
- [ ] **Step 3: 实现**，重新生成 bundle 与 API 报告。
- [ ] **Step 4: 运行通过**：JS 门槛命令全过；`go test ./runtime-js/...`。
- [ ] **Step 5: 检查点**

---

### Task 4: 自测示例跨平台化 + Android 自测入口

**Files:**
- Rename: `examples/duplex-ios` → `examples/duplex-mobile`（`git mv`）；更新引用（`grep -rn duplex-ios`，含 Global Constraints 文件归属里列出的注释）。`tan.json` 的 `id`（`dev.tango.duplexios`）已是合法 applicationId，**不改**（iOS bundle id 跟着不变）。
- Create: `examples/duplex-mobile/footprint_android.go`（读 `/proc/self/status` 的 `VmRSS`，字节）；`footprint_other.go` 改为 `//go:build !darwin && !android || !cgo`（`checks.memory` 要求 `before > 0`）
- Modify: `examples/duplex-mobile/main.go`：新增 ipc 方法 `Debug.Platform` → `{"goos": runtime.GOOS, "androidLanes": <TANGO_IPC_LANES_ANDROID 是否为开>}`；`Selftest.Report` 把超过 3000 字节的结果拆成 `kind#1: … kind#n:` 多行（logcat 单条约 4 KB 上限），最后一行 `kind#end`
- Modify: `examples/duplex-mobile/web/selftest.js`：`lanes` 期望值按 `Debug.Platform`：`ios` → `{down:"fallback",up:"up:post"}`；`android` 且 `androidLanes` → `{down:"down:android",up:"up:android"}`，否则 fallback
- Modify: `examples/duplex/services/lanestats_other.go` 改为 `//go:build !darwin && !android`；新建 `lanestats_android.go`，映射（不碰 `upPackets`/`upFrames`/`upBytes`，它们来自 `OnBinary`）：`UpPackets` → `upRequests`；`UpPackets-UpRefused-UpBadToken` → `upAccepted`；`DownPackets` → `downWrites`；新增 JSON 字段 `laneUpProbes`、`laneUpBadToken`、`laneUpRefused`、`laneDownDropped`、`laneDownQueued`（`testroutes.go` 的 `Stats` 加这些字段，其他平台为 0）
- Modify: `scripts/smoke.sh`：`android_start` 增加可选的 extra 参数；新模式 `android-selftest`、`android-lanefault`；构建 `GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=tango_selftest" bash mobile/android/build.sh examples/duplex-mobile`；从 `adb logcat` 收集并拼接 `kind#n:` 行再按 iOS 的规则判定。各模式的 `TANGO_TEST_ENV`：
  - `android-selftest`（通道关）：`TANGO_SCHEME_PROBE=1,TANGO_SCHEME_SELFTEST=1`
  - `android-selftest` 第二轮（通道开，同一模式内连续两次启动）：再加 `TANGO_IPC_LANES_ANDROID=1`
  - `android-lanefault`：两次启动，`…,TANGO_IPC_LANES_ANDROID=1,TANGO_IPC_LANE_FAULT=down:android:3` 与 `…,TANGO_IPC_LANE_FAULT=up:android:3`；页面 URL `?selftest&lanefault=down|up`（`main.go` 按变量选择）
  - `capsOff` 在 `android-selftest` 里作为第三次启动：`…,TANGO_IPC_LANES_ANDROID=1,TANGO_ANDROID_NO_ARRAYBUFFER=1`，期望 fallback

- [ ] **Step 1**: 改名并修正引用；`bash scripts/smoke.sh ios-selftest` 与 `ios-lanefault` 仍通过（iOS 无回归）。
- [ ] **Step 2**: Android 通道关闭时跑完整 selftest：资源对照（index/big/redirect/head/echo/panic/stop/memory）、`echo_ipc`、`lanes` = fallback。任何检查在 Android 上不适用或失败，**停下报告控制者裁定**（不在示例里静默跳过）。
- [ ] **Step 3**: 通道打开：`lanes` 正确，`lane_echo`（16×64 KiB）逐字节正确，`Debug.Stats` 显示 `upRequests ≥ 16`、`downWrites ≥ 16`、`laneUpProbes ≥ 1`、`laneUpBadToken == 0`。
- [ ] **Step 4: 检查点**

---

### Task 5: 模拟器上的通道端到端

**Files:**
- Modify: `examples/duplex/services/services.go`：`Source(frames, size, mod?, add?)`——整条流第 j 字节 = `(j + add) % mod`（跨帧连续计数）；缺省参数时输出与现在完全相同（`bench.js` 依赖）；`services_test.go` 加单测
- Modify: `examples/duplex/services/lanetests.js`（新场景 `reloadSameID`、`subframeAndroid`；现有场景按 `tango.lanes()` 的实际通道给期望值）、`examples/duplex-mobile/web/selftest.js`、新建 `examples/duplex-mobile/web/iframe.html`、`scripts/smoke.sh`

**场景**（`android-selftest` 通道开那一轮 / `android-lanefault`；Go 侧计数取 `Debug.Stats`）：

```text
big          8 MiB 回显（64 KiB 一帧，第 i 字节 = i % 251）逐字节相等；upRequests ≥ 64 且 downWrites ≥ 64
mixed        同一条流交替 100 B / 64 KiB 共 40 帧，回显顺序一致；up:android 下 upFrames == 40（minSize 0，全部走通道；
             up:post 下仍是 20）
reload       下行大流进行中 location.reload()；新页面 lanes 仍为 android 两条；goroutine 数回到基线 ±5；laneDownQueued 回到 0
reloadSameID 页面 A：Source 流（stream 1，32 MiB，mod 251 add 0）收到 ≥ 1 MiB，同时向 Sink 上传 64 KiB 帧，立刻 reload；
             页面 B：第一条 JS 流（stream 1）请求 Source 4 MiB（mod 253 add 7）并逐字节比对；另开一条 Echo 流上传 4 MiB
             （第 i 字节 = (i + 11) % 241）逐字节比对回显。任一不符即失败；记录 laneDownDropped 与 laneUpRefused（Review Focus 1）
stall        传输 4 MiB 期间页面同步忙 6 s；流完整、无 transport
faultDown    TANGO_IPC_LANE_FAULT=down:android:3：之后的帧走消息通道，流完整（下行拒收不丢帧）
faultUp      TANGO_IPC_LANE_FAULT=up:android:3：第 4 个数据包被拒，页面收到 [1] 后 lanes().up 变为 "fallback"（≤ 1 s）；
             丢帧的流报 transport（≈5 s）；并行的另一条流完整；之后的帧走消息通道（Review Focus 2）
subframe     同源 iframe 用它自己的 window.__tangoAndroid.postMessage 发 50 个格式正确、令牌为猜测值的 ArrayBuffer；
             不透明来源 iframe（sandbox srcdoc）断言没有 __tangoAndroid；Go 侧 upRequests、laneUpBadToken 不因 iframe 增加
             （Java 按 isMainFrame 过滤，Go 一个包都看不到）；主页面 1 MiB 回显仍可用
capsOff      TANGO_ANDROID_NO_ARRAYBUFFER=1 + TANGO_IPC_LANES_ANDROID=1 → lanes() = fallback，echo 正常（Review Focus 5）
```

- [ ] **Step 1–4**：写场景 → 运行（失败的如实记录）→ 只在平台层修复 → 全部通过，连续 3 次稳定（每次全新安装）。
- [ ] **Step 5: 检查点**

---

### Task 6: 基准与默认开启判定（规格 §6.3）

**Files:** `docs/ipc-bench.md`、`internal/platform/android/lanes.go`（只在用户确认后改默认值）

- [ ] **Step 1: 方法**：`examples/duplex-mobile` 的 bench 模式（`TANGO_DUPLEX_BENCH=1`，`bench.js`），通道开/关各 5 次、交替、每次全新启动；宿主机 `uptime` 1 分钟负载低于核心数一半（否则等最多 10 分钟，仍不满足标"负载下"）；取中位数。64 B 延迟分批计时（每批 50 次，20 批，p50/p99）。数据标明"模拟器（arm64 镜像跑在 Apple Silicon 上）"，没有真机。
- [ ] **Step 2: 判定**：64 KiB 帧吞吐（上行、下行分别）≥ 兜底 2 倍；64 B 分批 p99 不比兜底差（±5% 噪声）。Android 的兜底：下行 `evaluateJavascript`，上行字符串 `postMessage`（base64）。
- [ ] **Step 3**：`docs/ipc-bench.md` 增加"第二期 Android"表格（宿主机、AVD、WebView 版本、负载、各项中位数、判定）。
- [ ] **Step 4: 默认值需要用户确认**：本步只产出数据和建议；确认后 `laneCapsFromEnv` 未设/空时返回两条，`0`/`false`/`off` 关闭；文档同步。
- [ ] **Step 5: 检查点**

---

### Task 7: 文档

**Files:** `docs/mobile.md`、`docs/dev-builds.md`（`TANGO_TEST_ENV`、`TANGO_ANDROID_NO_ARRAYBUFFER`、Android 测试钩子行）、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`、`CHANGELOG.md`、规格 §6.2 Android 小节（实测结论）

- [ ] 写明：Android 通道与条件（WebView ≥ 116、listener 路径）、开关（`TANGO_IPC_LANES_ANDROID`、`TANGO_IPC_LANES=0`、`Options.DisableFastLanes`）、真机上开启方式（创建第一个页面前 `os.Setenv`）、iframe 结论与信任模型（同源 iframe 经 `parent.__tangoNative` 等同应用代码）、已知限制（只测模拟器、每包一次跨进程拷贝加一次 JNI 拷贝）；`examples/duplex-ios` 改名；`TANGO_TEST_ENV` 只在测试钩子构建 + 可调试 APK 生效；Task 1 的接口改动（`SendLanePacket`/`OnLanePacket` 加世代，内部接口）写进 CHANGELOG。
- [ ] `cd site && npm run docs:build && npm run docs:links`（以本计划新增/修改的文件为准；main 上已有的死链 `docs/specs/gui-completeness-audit-2026-11-13.md → ../../ui/shader` 不在范围内）。
- [ ] 检查点

---

## 完成后

- 是否默认开启，以 Task 6 数据和用户决定为准。
- 未覆盖、留给后续：真机测量；Windows（SharedBuffer）与 Linux 平台计划；若 Task 6 显示小帧需要攒批，`up:android` 加攒批。
