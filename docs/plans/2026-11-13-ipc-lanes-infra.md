# IPC 快速通道 · 公共基础设施 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭好第二期所有平台通道共用的部分：ipc 的二进制包格式与按帧选路、核心 `Surface` 的通道状态与令牌、`hello` 里的 `lanes`、通道控制消息、`tango://` 上的 `down:scheme` / `up:post` 端点、JS 的通道框架与这两条通道的 JS 模块、丢帧检测（`transport`）、关闭开关和故障注入。完成后，任何平台后端只要报告能力并实现 `platform.BinaryLane`（或支持流式响应 / POST 请求体），就能接入快速通道。

**Architecture:** 三层，职责按规格 §6.1：`ipc.Router` 只管协议（编码/解码包、按 `BinaryMinSize` 和 `SendBinary` 返回值选路、按 seq 重排、缺口超时报 `transport`）；新包 `internal/lane` 拥有通道状态（世代、令牌、每条通道的状态、下行队列、scheme 端点的 HTTP 处理）；核心 `*tango.Surface` 把两者接起来并实现 `ipc.BinaryConn`；平台后端只报告能力、发下行包、交上行包。JS 侧 `runtime-js/src/lanes.ts` 是通道框架，`lanes/scheme.ts` 实现 `down:scheme`（流式 `fetch`）和 `up:post`（攒批 POST）。

**Tech Stack:** Go 1.27（标准库）；TypeScript 5.9 + esbuild（runtime-js）；`node --test`；Go+node 端到端沿用 `runtime-js/runtimejs/e2e_test.go` 的 harness，另起本地 HTTP 服务承载 scheme 端点。

**Spec:** `docs/specs/2026-11-13-ipc-duplex-streams.md` §6.1、§6.3（第一期已实现的协议见 §5 与 `ipc/wire.go` 包注释）。

## Final implementation ruling (I1–I7)

The final controller/review rulings supersede earlier task sketches: immutable
`ipc.SessionBinaryConn.BinarySession()` handles, atomic `DropBinarySession()` retirement and generation-bearing
`App.OnBinary`/`Router.HandleBinarySession`; reliable v2 stream `boundary` controls
for bounded missing-tail detection in both directions; per-POST 10-second abort;
8 MiB aggregate encoded POST bytes including flight; single-packet downlink
write detachment retaining/accounting the untouched tail; location-pinned,
redirect-refusing production fetch. Exact contracts are in specification §6.1.
Native backends remain out of scope and no capabilities are enabled.

## Global Constraints

- 包格式（所有平台相同，大端，不做 base64）：`packet := count:u16 | frame*count`，`frame := streamID:u64 | seq:u64 | len:u32 | payload[len]`。`count=0` 的空包合法（scheme 下行用它尽快发出响应头）。
- 快速通道**只承载字节帧**；JSON 帧、控制消息、握手一律走消息通道。快速通道上帧的计费大小固定为 `len + 64`（`FrameOverhead`），不带 `c`。
- 单帧上限仍是 `MaxFrameBytes = 16 << 20`；`up:post` 单个包上限 1 MiB（`MaxPostPacket = 1 << 20`），超过 1 MiB 的帧不走 `up:post`。
- `BinaryMinSize` 初始值：`down:scheme` 4096；`down:android`、`down:shm` 0。`up:post` 攒批阈值 16 KiB（`PostBatchBytes = 16 << 10`）或 4 ms（`PostBatchDelay`），同一页面同时只有一个 POST 在途。
- 通道名固定为：`down:scheme`、`up:post`、`down:android`、`up:android`、`down:shm`、`up:shm`。
- `hello` 回复的 `lanes` 字段：`{"gen":<uint>,"tok":"<32 位十六进制>","list":[...]}`；没有通道时**不出现**这个字段（第一期行为不变）。
- 通道控制消息走核心内部频道（`corehook.InternalPrefix` + `"lane:"` + JSON），JSON 为 `{"op":"ready"|"fail","lane":"<名>","gen":<uint>}`；`gen` 不是当前世代的直接丢弃。（**裁定**：规格 §6.1 写的是 `{"tango":…}` 形式的 `lane` 消息，但核心的内部消息分发 `App.dispatchInternal` 按前缀工作，`{"tango":...}` 会被 Router 认领后丢弃；改走内部频道，规格同步修改。）
- scheme 端点路径：`/__tango/lane/down?gen=<gen>&tok=<tok>`（GET，长连接流式响应，`Content-Type: application/octet-stream`，`Cache-Control: no-store`）和 `/__tango/lane/up?gen=<gen>&tok=<tok>`（POST，body 为一个包，成功回 204）。JS 用相对路径请求，只在页面来自资源来源时启用。令牌用 `crypto/rand` 16 字节，十六进制；比较用 `subtle.ConstantTimeCompare`。
- 丢帧检测（两端相同）：接收方有缓存的乱序帧、且期待的下一帧连续 5 秒（`GapTimeout = 5 * time.Second`）没到 → 该流 `error`，码 `transport`。（**裁定**：规格写的是"且该方向通道已停用或刚停用过"；去掉这个条件，接收方不需要知道通道状态。只有快速通道会造成缺口，正常情况下毫秒级就补上，5 秒足够宽。）
- 通道只在以下条件都满足时开启：应用注册了至少一个 `OnBinary` 处理函数；页面来自资源来源；后端报告了能力；没有被关闭开关关掉。
- 关闭开关：`tango.Options.DisableFastLanes bool`，以及环境变量 `TANGO_IPC_LANES=0`（任何构建都生效，用于排查）。
- 故障注入（只在 `tango_dev` 构建生效）：环境变量 `TANGO_IPC_LANE_FAULT=<通道名>:<N>`，该通道成功发送/接收 N 个包后进入故障（之后的包丢弃并按"通道坏掉"处理）。各平台计划的"通道中途坏掉"验收都靠它。
- 不新增外部依赖。不改任何平台后端（`internal/platform/{darwin,ios,linux,windows,android}`）——它们属于各平台计划；本计划只改 `internal/platform/ext.go`（接口）和 `internal/platform/fake`。
- 不提交 git；在 worktree `/Users/admin/min/tango/.claude/worktrees/ipc-duplex` 里工作（第一期的改动也在这里，尚未整合）。
- 每个任务结束时：`go build ./... && go vet ./ipc/... ./internal/lane/... ./internal/platform/... ./runtime-js/... .`；`go test -race ./ipc/... ./internal/lane/...`；`go test ./runtime-js/... .`（根包）；改了 `runtime-js/src` 的任务再 `cd runtime-js && npm run build && npm test && npm run typecheck`。

## Review Focus

1. **页面重载时 scheme 下行连接还没断**：旧世代的长连接响应必须在新 `hello` 时结束，旧连接上排队的包不能写进新世代的流 —— Task 5 `TestDownEndpointEndsOnNewGen`。
2. **其他页面或 iframe 冒用端点**：错误/旧世代的 `tok`、错误的 `gen`、非资源来源的请求，都必须被拒绝且不影响当前通道 —— Task 5 `TestEndpointsRejectBadTokenGenOrigin`。
3. **大小帧交错**：同一条流先发 100 B（走消息通道）再发 64 KiB（走快速通道）再发 100 B，接收方顺序必须是原顺序 —— Task 2 `TestRoutingKeepsOrderAcrossLanes`、Task 8 `mixedSizesOrder`。
4. **通道中途坏掉**：已交给坏通道的帧丢失 → 对应的流 5 秒后报 `transport`；之后的帧自动走消息通道；同一页面其他流不受影响 —— Task 3 `TestGapTimeoutTransport`、Task 8 `laneFailure`。
5. **恶意或损坏的上行包**：截断的包、`len` 超出包体、`len` > `MaxFrameBytes`、未知流 id —— 整包丢弃（`slog.Warn`），不 panic，不影响其他流 —— Task 1 `TestHandleBinaryMalformed`。

---

### Task 1: ipc 包格式、`BinaryConn`、`HandleBinary`、`hello` 的 `lanes`

**Files:**
- Create: `ipc/packet.go`、`ipc/packet_test.go`、`ipc/binary_test.go`
- Modify: `ipc/router.go`（`hello` 回复、`HandleBinary`）、`ipc/wire.go`（`helloOut` 加 `Lanes json.RawMessage "lanes,omitempty"`；包注释补快速通道一节）

**Interfaces:**
- Produces:

```go
// ipc/packet.go
type PacketFrame struct {
	Stream  uint64
	Seq     uint64
	Payload []byte // 解码时指向输入切片内部，调用方需要保留就自己拷贝
}
func AppendPacket(dst []byte, frames ...PacketFrame) []byte // 编码
func ParsePacket(p []byte) ([]PacketFrame, error)            // 校验：长度、count、每帧 len ≤ MaxFrameBytes、无多余尾部字节

// ipc/router.go
type BinaryConn interface {
	Conn
	LaneHello() json.RawMessage   // nil：不放 lanes 字段
	SendBinary(packet []byte) bool // 非阻塞；false = 不可用，调用方走消息通道
	BinaryMinSize() int           // 走下行快速通道的最小 payload 字节数；<0 表示下行通道不可用
}
func (r *Router) HandleBinary(c Conn, packet []byte)
```

  - `hello`（版本匹配时）：`Drop` 之后，若 `c` 实现 `BinaryConn` 且 `LaneHello()` 非 nil，回复里带 `"lanes":<原样>`。
  - `HandleBinary`：`ParsePacket` 失败 → `slog.Warn("ipc: malformed lane packet", ...)` 整包丢弃；每帧按 `Stream` 找到该连接上的流，拷贝 payload 后 `onFrame(frame{seq, cost: len+FrameOverhead, bin: true, payload})`；找不到的流忽略。

- [ ] **Step 1: 写失败的测试**

```go
// packet_test.go
func TestPacketRoundTrip(t *testing.T)    // 0 帧、1 帧空 payload、3 帧（含 64 KiB）编码后解码相等；空包编码为 []byte{0,0}
func TestParsePacketRejects(t *testing.T) // 少于 2 字节；count=1 但不够帧头；len 超出剩余；len = MaxFrameBytes+1；有多余尾部字节 —— 都返回 error
// binary_test.go（fakeBinConn：在 fakeConn 上实现 BinaryConn，字段 hello json.RawMessage、min int、sent [][]byte、accept bool）
func TestHelloIncludesLanes(t *testing.T)   // hello 回复 == {"tango":"hello","proto":2,"win":1048576,"lanes":{"gen":1,"tok":"ab","list":["down:scheme"]}}
func TestHelloOmitsLanesWhenNil(t *testing.T) // LaneHello 返回 nil → 回复不含 "lanes"
func TestHandleBinaryDelivers(t *testing.T) // HandleDuplex 流 id 1：HandleBinary 一个含 seq 0（3 字节）的包 → 处理函数 ReceiveBytes 得到这 3 字节；读走后 ack b == 67
func TestHandleBinaryMalformed(t *testing.T) // 截断包、len>MaxFrameBytes、未知流 id：不 panic，流状态不变，同连接上另一条流照常回显
```

- [ ] **Step 2: 运行确认失败**：`go test -timeout 120s ./ipc -run 'Packet|HelloIncludes|HelloOmits|HandleBinary' -v`，Expected：编译失败
- [ ] **Step 3: 实现**
- [ ] **Step 4: 运行 `go test -timeout 180s -race ./ipc/...`**，Expected：PASS
- [ ] **Step 5: 检查点**（不提交）

---

### Task 2: ipc 下行选路：字节帧按条件走快速通道

**Files:**
- Modify: `ipc/peer.go`（`wirePeer` 发字节帧时的选路）、`ipc/stream.go`（若需要在 `prepareBytes` 处保留原始字节）
- Create: `ipc/route_test.go`

**Interfaces:**
- Consumes: Task 1 的 `BinaryConn`、`AppendPacket`
- 行为（只影响 `wirePeer`；`localPeer` 不变）：
  - 字节帧发送时，若连接实现 `BinaryConn`、`min := BinaryMinSize()` ≥ 0、`len(payload) >= min`：编码单帧包 `AppendPacket(nil, PacketFrame{id, seq, payload})`，`SendBinary` 返回 true 就结束；返回 false 则改走消息通道（base64 `bin`）。
  - 这意味着字节帧在进入选路前必须保留原始字节：`bytesPreparer` 对 wirePeer 改为"拷贝原始字节"，base64 推迟到确定走消息通道时再做（编码仍在调用 `sendFrame` 的 goroutine 上、`mu` 之外）。
  - 信用、seq、计费完全不变（快速通道帧的计费 `len+64` 与消息通道相同）。
  - JSON 帧永远走消息通道。

- [ ] **Step 1: 写失败的测试**（`route_test.go`，复用 Task 1 的 `fakeBinConn`）

```go
func TestRoutingByMinSize(t *testing.T)            // min=4096：WriteBytes(100B) → 消息通道 bin；WriteBytes(64KiB) → sent 里一个包，ParsePacket 得 {id, seq 1, 64KiB}，消息通道没有 data
func TestRoutingFallbackWhenRejected(t *testing.T) // accept=false：64KiB 帧走消息通道（bin 为 base64），seq 连续
func TestRoutingDisabledWhenMinNegative(t *testing.T) // min=-1：所有字节帧走消息通道
func TestRoutingKeepsOrderAcrossLanes(t *testing.T)   // 交替发 100B/64KiB/100B：消息通道 seq 0、2，包里 seq 1；JSON 帧总在消息通道
func TestRoutingCreditUnchanged(t *testing.T)      // 窗口 4096：走快速通道的帧同样占用信用，ack 后才继续
```

- [ ] **Step 2–4**：`go test -timeout 120s ./ipc -run Routing -v` 先失败；实现；`go test -timeout 180s -race ./ipc/...` PASS
- [ ] **Step 5: 检查点**

---

### Task 3: ipc 接收缺口超时 → `transport`

**Files:**
- Modify: `ipc/stream.go`（接收侧缺口计时）、`ipc/wire.go`（`GapTimeout` 常量；`CodeTransport` 已有）
- Create: `ipc/gap_test.go`；`ipc/export_test.go` 加测试钩子 `setGapTimeout(r *Router, d time.Duration)`

**Interfaces:**
- Produces: `const GapTimeout = 5 * time.Second`
- 行为：流的接收侧从"缓存里有乱序帧且 `recvNext` 未到"开始计时；`recvNext` 推进时重新判断（缺口补上则停表，仍有缺口则从推进时刻重新计时）；到时 → 本端按 `fail(NewError(CodeTransport, "frames lost on a fast lane"))` 处理（向对端发 `error`，本端 `Receive*` 返回该错误）。计时器在流结束时停止，不泄漏 goroutine（用 `time.AfterFunc`，回调只拿 `mu`）。

- [ ] **Step 1: 写失败的测试**

```go
func TestGapTimeoutTransport(t *testing.T)  // setGapTimeout 100ms；HandleBinary 送 seq 1（缺 seq 0）→ 100ms 后对端收到 {"op":"error","error":{"code":"transport"}}，处理函数 Receive 返回 *Error code transport；同连接另一条流照常回显
func TestGapFilledInTime(t *testing.T)      // 先送 seq 1，50ms 内经消息通道送 seq 0 → 依次收到 seq 0、1，没有 error
func TestGapTimerStopsOnFinish(t *testing.T) // 有缺口时对端 cancel → 流结束；等 2×超时，不再发任何消息；goroutine 数回到基线
```

- [ ] **Step 2–4**：先失败；实现；`go test -timeout 180s -race -count=3 ./ipc/...` PASS
- [ ] **Step 5: 检查点**

---

### Task 4: `internal/lane` 状态 + 核心接线 + 平台接口 + fake 后端

**Files:**
- Create: `internal/lane/lane.go`、`internal/lane/fault.go`（`//go:build tango_dev`）、`internal/lane/fault_release.go`（`//go:build !tango_dev`）、`internal/lane/lane_test.go`
- Modify: `internal/platform/ext.go`（`BinaryLane`、`LaneHost`）、`internal/platform/fake/fake.go`（实现两者）
- Create: `lanes.go`（根包：`Surface` 实现 `ipc.BinaryConn`、`App.OnBinary`、内部频道 `lane`、`host.OnLanePacket`）、`lanes_test.go`
- Modify: `tango.go`（`Options.DisableFastLanes`；`host` 实现 `platform.LaneHost`）、`surface.go`（`Surface` 持有 `*lane.State`）

**Interfaces:**
- Produces:

```go
// internal/platform/ext.go
type BinaryLane interface { // 可选，由平台 Surface 实现
	LaneCaps() []string           // 本页面当前可用的通道名（含 down:scheme / up:post）
	SendLanePacket(p []byte) bool // 只用于平台通道（down:android / down:shm）；非阻塞，线程安全
}
type LaneHost interface { // 由核心 host 实现，后端断言得到
	OnLanePacket(id SurfaceID, origin string, p []byte) // 任意线程
}

// internal/lane/lane.go
type State struct{ /* mu、gen、tok、caps、每条通道状态、scheme 下行队列 */ }
func New(platformSend func([]byte) bool) *State
func (s *State) Hello(caps []string, enabled bool) json.RawMessage // 新世代：gen++、新 tok、所有通道置为"未就绪"、结束旧 scheme 下行；enabled=false 或 caps 空 → nil
func (s *State) Control(payload string)  // 内部频道 lane 的 JSON：ready/fail；gen 不符丢弃
func (s *State) Send(p []byte) bool      // 选已就绪的下行通道；平台通道调 platformSend；down:scheme 入队；都没有 → false
func (s *State) MinSize() int            // 已就绪下行通道的 BinaryMinSize；没有 → -1
func (s *State) CheckToken(gen uint64, tok string) bool
func (s *State) UpAllowed() bool         // 上行包到达时调用（平台通道与 up:post 都调）：当前就绪的上行通道仍可用就返回 true；故障注入计数也在这里
func (s *State) Close()                  // Surface 关闭时
const (
	DownScheme, UpPost, DownAndroid, UpAndroid, DownShm, UpShm = "down:scheme", "up:post", "down:android", "up:android", "down:shm", "up:shm"
	MaxPostPacket  = 1 << 20
	PostBatchBytes = 16 << 10
)

// 根包
func (a *App) OnBinary(fn func(s *Surface, packet []byte))
func (s *Surface) LaneHello() json.RawMessage // 实现 ipc.BinaryConn
func (s *Surface) SendBinary(p []byte) bool
func (s *Surface) BinaryMinSize() int
// Options.DisableFastLanes bool
```

  - `Surface.LaneHello()`：启用条件全部满足（Global Constraints）才把 `ns.(platform.BinaryLane).LaneCaps()` 交给 `State.Hello`；资源来源判断复用 `s.app.assetOrigin()` 与页面当前来源（`trusts` 里用的同一套信息，取"当前页面来源 == 资源来源"）。
  - `host.OnLanePacket`：`surface(id)` 找到、`trusts(origin)` 且 `s.lane.UpAllowed()` → 依次调用所有 `OnBinary` 处理函数（在调用线程上，不 `enterMain`；处理函数通常就是 `r.HandleBinary`，本身线程安全）。
  - 内部频道 `lane` 用现有 `handleInternal` 注册，转给 `s.lane.Control`。
  - 故障注入：`TANGO_IPC_LANE_FAULT=<名>:<N>` 在 `tango_dev` 构建里让 `State` 对该通道计数，第 N 个包之后 `Send` 对它返回 false 并把它标成"已停用"（上行通道：核心收到第 N 个之后的上行包直接丢弃）。release 构建里是空实现。
  - `TANGO_IPC_LANES=0` 与 `Options.DisableFastLanes` 等价。
  - fake 后端：`Backend.SetLaneCaps(id, caps []string)`；fake surface 实现 `BinaryLane`，`SendLanePacket` 记录到 `Backend.LanePackets(id) [][]byte`；`Backend.LaneUp(id, origin string, p []byte)` 调宿主的 `OnLanePacket`。

- [ ] **Step 1: 写失败的测试**

```go
// internal/lane/lane_test.go
func TestHelloNewGenAndToken(t *testing.T)  // 两次 Hello：gen 1→2，tok 32 位十六进制且不同；CheckToken 只认当前 gen+tok
func TestHelloNilWhenDisabledOrNoCaps(t *testing.T)
func TestControlReadyFail(t *testing.T)     // ready(down:android) 后 MinSize()==0、Send 走 platformSend；fail 后 MinSize()==-1、Send==false；旧 gen 的 ready 被忽略
func TestSchemeMinSize(t *testing.T)        // ready(down:scheme) → MinSize()==4096
// lanes_test.go（根包，fake 后端）
func TestSurfaceLaneHelloConditions(t *testing.T) // 没注册 OnBinary / 页面非资源来源 / DisableFastLanes / TANGO_IPC_LANES=0 / caps 空 → LaneHello()==nil；全部满足 → {"gen":1,"tok":...,"list":[caps...]}
func TestLaneControlChannel(t *testing.T)   // 页面发 InternalPrefix+"lane:"+`{"op":"ready","lane":"down:android","gen":1}` → SendBinary 走 fake SendLanePacket；不会进 OnMessage
func TestOnLanePacketRouting(t *testing.T)  // Backend.LaneUp(id, 资源来源, p) → OnBinary 收到 (s, p)；不受信任来源 → 不调用
func TestRouterOverSurfaceLanes(t *testing.T) // 根包里接 ipc.Router：页面 hello → 回复含 lanes；ready(down:android) 后 64KiB 字节帧出现在 LanePackets，100B 帧在消息通道
```

- [ ] **Step 2–4**：`go test -timeout 120s ./internal/lane/... . -run 'Hello|Control|Scheme|Lane|Router' -v` 先失败；实现；本任务的全部门槛命令 PASS（另跑 `go test -tags tango_dev ./internal/lane/...` 覆盖故障注入，测试 `TestFaultAfterN`：`TANGO_IPC_LANE_FAULT=down:android:2` → 第 3 个包起 `Send` 返回 false）
- [ ] **Step 5: 检查点**

---

### Task 5: scheme 端点：`/__tango/lane/down`、`/__tango/lane/up`

**Files:**
- Create: `internal/lane/http.go`、`internal/lane/http_test.go`
- Modify: `tango.go`（`host.ServeAsset` 在 `opts.Assets` 之前拦截 `/__tango/lane/`）

**Interfaces:**
- Consumes: Task 4 的 `State`
- Produces:

```go
// internal/lane/http.go
func (s *State) ServeDown(w http.ResponseWriter, r *http.Request)                  // GET；校验 gen/tok；需要 http.Flusher，否则 500 且该通道 fail
func (s *State) ServeUp(w http.ResponseWriter, r *http.Request, deliver func([]byte)) // POST；校验；body ≤ MaxPostPacket+包头开销，否则 413；deliver(body) 后回 204
```

  - `ServeDown`：同一世代只允许一个活动下行连接（后来的连接让前一个结束）；先写空包 `[]byte{0,0}` 并 `Flush`；然后循环取队列里的包写出并 `Flush`；`r.Context()` 结束、新世代（`Hello`）、`Close`、该通道 fail 时返回。写出错误 → 该通道 fail（之后 `Send` 不再选它）。队列上限 8 MiB，满了 `Send` 返回 false（帧走消息通道）。
  - `ServeUp`：`deliver` 由核心传入，等于"调用所有 `OnBinary`"；调用前先 `UpAllowed()`，不允许时回 503（JS 侧据此 `fail`）。
  - 核心拦截：路径以 `/__tango/lane/` 开头时，先做现有的 `assetRequestAllowed` 来源检查（不受 `AllowCrossOriginAssets` 放宽），再按 `down`/`up` 分发；其他路径照旧交给 `opts.Assets`。即使 `opts.Assets == nil` 也要能服务通道端点。

- [ ] **Step 1: 写失败的测试**（`http_test.go` 用 `httptest.NewServer` 包 `ServeDown`/`ServeUp`；根包另加一个经 `host.ServeAsset` 的测试）

```go
func TestDownEndpointStreams(t *testing.T)            // GET 后 1s 内读到 []byte{0,0}；Send 两个包 → 客户端按序读到两个包（逐包到达，不等响应结束）
func TestDownEndpointEndsOnNewGen(t *testing.T)       // 连接中 Hello() → 响应结束（读到 EOF）；旧队列里的包没写出
func TestUpEndpointDelivers(t *testing.T)             // POST 一个包 → 204，deliver 收到原样字节
func TestUpEndpointTooLarge(t *testing.T)             // body > MaxPostPacket+14 → 413，deliver 未调用
func TestEndpointsRejectBadTokenGenOrigin(t *testing.T) // 错 tok / 旧 gen / 缺参数 → 403；当前下行连接不受影响
func TestServeAssetInterceptsLanePaths(t *testing.T)  // 根包：Assets==nil 时 /__tango/lane/up 仍可用；/index.html 仍交给 Assets；跨来源（Origin: http://evil）→ 403
```

- [ ] **Step 2–4**：先失败；实现；门槛命令 PASS（`-race`）
- [ ] **Step 5: 检查点**

---

### Task 6: runtime-js 通道框架

**Files:**
- Create: `runtime-js/src/packet.ts`、`runtime-js/src/lanes.ts`、`runtime-js/test/lanes.test.mjs`
- Modify: `runtime-js/src/client.ts`（hello 的 `lanes`、下行包入流、上行选路、缺口超时）、`runtime-js/src/index.ts`（导出 `lanes()`、`registerLane` 不导出到包入口，只供平台模块内部用）

**Interfaces:**
- Produces:

```ts
// packet.ts
export interface PacketFrame { stream: number; seq: number; payload: Uint8Array }
export function encodePacket(frames: PacketFrame[]): Uint8Array
export function decodePacket(p: Uint8Array): PacketFrame[] // 校验同 Go；streamID/seq 用 Number（≤ 2^53，超出视为损坏）

// lanes.ts
export interface LaneContext {
  gen: number; tok: string;
  onPacket(p: Uint8Array): void;      // 下行通道收到的包
  fail(err: unknown): void;           // 通道自己发现坏了
  fetch: typeof fetch;                // 默认 globalThis.fetch，测试可注入
}
export interface DownLane { start(ctx: LaneContext): Promise<void>; stop(): void }
export interface UpLane {
  readonly minSize: number;           // 小于它的帧走消息通道
  start(ctx: LaneContext): Promise<void>;
  send(frame: PacketFrame): boolean;  // false = 不可用，调用方走消息通道
  stop(): void;
}
export function registerLane(name: string, make: () => DownLane | UpLane): void
// index.ts
export function lanes(): { down: string; up: string } // 当前就绪的通道名，没有则 "fallback"
export const GAP_TIMEOUT_MS = 5000;
```

  - 收到 Go `hello` 的 `lanes`：对 `list` 里每个已注册的名字建模块、`start`；成功后经桥发 `InternalPrefix + "lane:" + JSON {op:"ready",lane,gen}`，失败或运行中 `fail` 发 `op:"fail"` 并 `stop`。每个方向最多一条就绪通道（列表顺序优先）。新的 `hello`（页面不会重载时不会发生）或客户端失败 → 全部 `stop`。
  - 下行：`onPacket` → `decodePacket` → 每帧按流 id 找流，按字节帧 `_data`（计费 `len+64`），走第一期已有的 seq 重排。坏包丢弃并 `console.warn` 一次。
  - 上行：流的 `flush` 发字节帧时，若有就绪上行通道且 `payload.length >= minSize`，交给 `send`；`true` 则按已发出计（seq、inflight 与消息通道相同），`false` 走消息通道。JSON 帧、控制消息不走通道。
  - 缺口超时：与 Go 相同规则，到时本端 `_close(new TangoError("frames lost on a fast lane","transport"))`（向 Go 发 error）。测试可通过 `createClient(bridge, { gapTimeoutMs })` 覆盖（只加这一个选项）。
  - `InternalPrefix` 的值 `"\x00tango-int:"` 在 TS 里定义为常量并注明与 `internal/corehook` 同步。

- [ ] **Step 1: 写失败的测试**（`lanes.test.mjs`，用测试内注册的假通道 `test:down` / `test:up`）

```js
test("packet round trip and rejects", ...)      // 与 Go 测试同样的用例；空包 = Uint8Array [0,0]
test("hello lanes starts modules and posts ready", ...) // Go hello 带 {gen:3,tok:"t",list:["test:down","test:up","nope"]} → 两个模块 start；posted 里有 "\x00tango-int:lane:" + {"op":"ready","lane":"test:down","gen":3} 和 test:up；未注册的 nope 忽略
test("down packets feed streams in seq order", ...) // 消息通道 seq 0 + 通道包 seq 1 + 消息通道 seq 2 → 迭代顺序 0,1,2；读走后 ack 计费 len+64
test("up routing by minSize", ...)               // test:up minSize 1000：sendBytes(10B) → 消息通道；sendBytes(2000B) → 模块 send 收到 {stream, seq:1, payload}；信用照常
test("up lane rejection falls back", ...)       // send 返回 false → 帧走消息通道 bin
test("lane fail posts fail and falls back", ...)  // 模块调 ctx.fail → posted op:"fail"；之后的大帧走消息通道；lanes().up === "fallback"
test("gap timeout fails stream with transport", ...) // gapTimeoutMs 50：只到 seq 1 → 50ms 后 posted {op:"error",error:{code:"transport"}}，迭代抛 code transport；另一条流不受影响
test("lanes() reports active lanes", ...)
```

- [ ] **Step 2–4**：`npm test` 先失败；实现；`npm run build && npm test && npm run typecheck` PASS，`go test ./runtime-js/...` PASS
- [ ] **Step 5: 检查点**

---

### Task 7: runtime-js `down:scheme` 与 `up:post` 模块

**Files:**
- Create: `runtime-js/src/lanes/scheme.ts`、`runtime-js/test/scheme-lanes.test.mjs`
- Modify: `runtime-js/src/index.ts`（引入 `lanes/scheme.ts` 以注册两条通道）

**Interfaces:**
- Consumes: Task 6 的 `registerLane`、`LaneContext`、`encodePacket`
- 行为：
  - `down:scheme`：`ctx.fetch("/__tango/lane/down?gen=..&tok=..", {cache:"no-store"})`；响应非 200 或没有 `body` → `start` 失败。读 `ReadableStream`，跨 chunk 拼包（包可能被切开，也可能一个 chunk 含多个包），每个完整包交给 `ctx.onPacket`；首个空包到达即 `start` 成功。读出错或流结束（非 `stop` 引起）→ `ctx.fail`。`stop` → `AbortController.abort()`。
  - `up:post`：`minSize = 16 * 1024`。`send` 把帧放进当前批；批里字节 ≥ `PostBatchBytes` 或距第一帧 4 ms 时发出；同时只有一个 POST 在途，在途期间新帧继续攒；单包 ≤ 1 MiB（放不下的帧 `send` 返回 false）。`body` 用 `ArrayBuffer`（`packet.buffer.slice(...)`），**不能用 Blob**。响应非 204 或网络错误 → `ctx.fail`（这一批的帧已经计为发出，靠缺口超时发现丢失）。`start` 不发请求，直接成功。
  - 请求都用相对路径（页面来自资源来源时，相对路径就是 `tango://app/...` 或 `https://tango.localhost/...`）。

- [ ] **Step 1: 写失败的测试**（注入假 `fetch`：返回可控的 `ReadableStream` / 记录 POST）

```js
test("down:scheme parses packets split across chunks", ...) // chunk 序列：[0,0] | 包1 前半 | 包1 后半+包2 → onPacket 依次收到包1、包2；首个 [0,0] 后 start resolve
test("down:scheme start fails on non-200", ...)
test("down:scheme stream end triggers fail", ...)
test("up:post batches to 16KiB or 4ms", ...)   // 3 帧各 6 KiB → 第 3 帧时发出一个 POST（body 解码出 3 帧）；单帧 20 B 4ms 后单独发出
test("up:post one request in flight", ...)    // 第一个 POST 未完成时再攒的帧不发；完成后立刻发下一批
test("up:post uses ArrayBuffer body and fails on non-204", ...) // init.body instanceof ArrayBuffer；返回 500 → ctx.fail 被调用
test("up:post rejects frames that cannot fit 1MiB", ...) // send(2 MiB 帧) === false
```

- [ ] **Step 2–4**：先失败；实现；`npm run build && npm test && npm run typecheck` PASS
- [ ] **Step 5: 检查点**

---

### Task 8: Go + node 端到端（scheme 通道走真实 HTTP）

**Files:**
- Modify: `runtime-js/runtimejs/e2e_test.go`、`runtime-js/runtimejs/testdata/harness.mjs`
- Create: `runtime-js/runtimejs/lanes_e2e_test.go`（若放在同一文件太长）

**Interfaces:**
- Consumes: `ipc.BinaryConn`、`internal/lane.State`（`Hello`/`Control`/`Send`/`MinSize`/`ServeDown`/`ServeUp`）、JS `down:scheme`/`up:post`
- 做法：测试里的 `pipeConn` 增加一个实现 `ipc.BinaryConn` 的变体，内部持有 `lane.State`，caps 为 `[down:scheme, up:post]`；起 `httptest.NewServer` 挂 `ServeDown`/`ServeUp`（`deliver` = `r.HandleBinary(conn, p)`）；把服务地址传给 harness，harness 用 node 自带的 `fetch` 并把相对路径解析到这个地址（vm 上下文里提供一个包装的 `fetch`）。内部频道消息（`\x00tango-int:lane:`）在 Go 侧测试读循环里转给 `State.Control`。

- [ ] **Step 1: 写场景**（每个场景用 `tango.emit("result:<名>", …)` 报回，Go 断言）
  - `lanesUp`：harness 确认 `tango.lanes()` 为 `{down:"down:scheme", up:"up:post"}`。
  - `bigBothWays`：`HandleDuplex("Echo")`，JS 发 8 MiB（64 KiB 一帧，第 i 字节 = i % 251）→ 原样收回，逐字节相等；Go 侧计数确认至少一半的字节帧经 `HandleBinary` 到达、至少一半的回程帧经 `ServeDown` 写出。
  - `mixedSizesOrder`：同一条流交替发 100 B 与 64 KiB 共 40 帧 → 回显顺序与发送顺序一致。
  - `laneFailure`：中途让下行通道失败（测试钩子：服务端关闭下行响应），之后的帧照常回显（走消息通道）；在失败点之前已交给通道但没写出的帧所在的流在 5 秒内（测试里把两端缺口超时都设为 300 ms）报 `transport`；另一条并行的流不受影响。
  - `disabled`：`LaneHello` 返回 nil 的连接上，同样的 `bigBothWays` 全部走消息通道且成功。
- [ ] **Step 2: 运行**：`go test -timeout 180s -run 'EndToEnd|Lanes' -v ./runtime-js/...`，Expected：PASS；连续 `-count=10` 不出错
- [ ] **Step 3: 检查点**

---

### Task 9: 应用接线、示例、基准字段、文档

**Files:**
- Modify: `examples/hello/main.go`、`examples/runtime-demo/main.go`、`examples/showcase/main.go`、`examples/duplex/main.go`（加 `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte){ r.HandleBinarySession(s, gen, p) })`）
- Modify: `examples/duplex/frontend/app.js`（基准结果的 `lane` 字段改为 `tango.lanes()` 的实际值）
- Modify: `docs/architecture.md`、`site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`、`site/reference/js-api.md`（`lanes()`）、`runtime-js/README.md`、`CHANGELOG.md`、`docs/specs/2026-11-13-ipc-duplex-streams.md`（§6.1 的通道控制消息改为内部频道、缺口规则简化，两处标"实施裁定"）
- Modify: `examples/showcase/frontend/index.html:51`（"64 条窗口背压"改为按字节信用窗口的说法——第一期遗留）

- [ ] **Step 1**：改接线；`go build ./examples/...`；`TANGO_E2E=1 go test ./examples/duplex -run E2E` 仍然 PASS（本机后端尚未报告能力，所以仍走兜底，`lane` 字段为 `fallback`）
- [ ] **Step 2**：写文档：快速通道是什么、什么时候开启（四个条件）、`OnBinary` 接线、`DisableFastLanes` / `TANGO_IPC_LANES=0`、`TANGO_IPC_LANE_FAULT`（仅开发构建）、`tango.lanes()`、`transport` 错误的含义
- [ ] **Step 3: 校验**：`cd site && npm run docs:build && npm run docs:links` 成功；全部门槛命令 PASS。检查旧通道消息写法时，`grep -rn 'tango":"lane' docs site runtime-js/README.md` 的每处命中须按实际接收路径审查，不要求零命中：页面→Go 的桥接控制消息必须走内部频道；§6.2 的 SharedBuffer `additionalDataAsJson`（`sharedbufferreceived` 接收）保留 `tango`/`ring`/`gen` 元数据，下行 `wake` 门铃由页面侧平台模块接收，也不经过 `App.dispatchInternal`。不能为消除 grep 命中而删除这些协议字段或 `op:"wake"` 语义。
- [ ] **Step 4: 检查点**

---

## 完成后

- 公共设施就绪，但没有任何平台后端报告能力，所以真实应用里快速通道仍然不会开启——这符合预期。
- 接着按顺序写平台计划：macOS/iOS（协议处理器异步化 + 报告 `down:scheme`/`up:post`）、Android（`down:android`/`up:android` 的 Java 与 JS 模块）、Windows（`down:shm`/`up:shm`）、Linux（报告 `down:scheme`/`up:post`，2.36/2.40 版本门槛）。每份计划都要包含：能力探测、JS 模块（如需要）、该平台上 `TANGO_IPC_LANE_FAULT` 驱动的"通道中途坏掉"验收、按 §6.3 的基准对比（macOS 用分批计时的百分位）。
