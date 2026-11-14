# IPC 双向流 第一期（协议层）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `ipc` 的流升级为全双工、JSON/字节两种负载、两个方向按字节计信用的协议 v2；补上进程内客户端（`Router.Open`/`Router.Call`）、Go 的 `io` 适配、JS 的发送 API / Web Streams / `tango.socket()`；配套示例、基准和文档。传输只用现有字符串通道（字节走 base64）。

**Architecture:** Go 侧一份流状态机（`ipc/stream.go`），通过内部 `peer` 接口把帧和控制消息交给"对端"：`wirePeer` 编码成 JSON 消息经 `Conn.Send` 发给页面，`localPeer` 直接递交给进程内的另一端。这样 JS 客户端和进程内客户端走同一套语义。JS 侧（`runtime-js/src/client.ts`）实现对称的状态机，Web Streams 和 `socket` 都是建在 `TangoStream` 之上的薄适配层。

**Tech Stack:** Go 1.27（`ipc` 只依赖标准库和 `internal/corehook`）；TypeScript 5.9 + esbuild 0.25（`runtime-js`），测试用 `node --test`（本机 Node v26）；`automation` 包驱动示例的端到端测试。

**Spec:** `docs/specs/2026-11-13-ipc-duplex-streams.md`（§5 全部、§9、§10；§6 第二期不在本计划内，等本计划的基准数据出来后每个平台单独写计划）。

## Global Constraints

- 协议版本 `2`；不兼容 v1：`hello` 不带 `proto:2` 时回 `{"tango":"hello","error":{"code":"bad_args","message":"runtime-js protocol mismatch"}}` 并 `slog.Warn`。
- 常量（Go 与 JS 同值）：`DefaultWindowBytes = 1 << 20`（1048576），`FrameOverhead = 64`，`MaxFrameBytes = 16 << 20`（16777216），`ProtoVersion = 2`；新错误码 `CodeTransport = "transport"`（本期只定义，不使用）。
- 计费大小：JSON 帧 `c = len(发送方编码后的 JSON 字节) + 64`；字节帧 `c = len(原始字节) + 64`。接收方读走一帧后归还它的 `c`；累计归还量 ≥ 自己接收窗口的一半时发一次 `{"op":"ack","b":累计值}`；**另外，读走一帧后如果接收队列已空、且累计值 > 0，也立即发 `ack` 全部归还**（Task 3 审查后的裁定：否则读方追上后不足半窗的零头永远不还，空闲可超窗的大帧会永久阻塞）。
- 发送方阻塞条件：`inflight > 0 && inflight + c > 窗口`（在途为 0 时单帧可以超窗）。原始负载 > `MaxFrameBytes` 时发送调用直接返回 `bad_args` 错误，流不受影响。
- 字节帧在线上用标准 base64（带 `=` 填充）放在 `bin` 字段；长度为 0 的字节帧是 `"bin":""`，必须与"没有 `bin` 字段"区分开。
- 删除 Go 的 `DefaultWindow` 和 JS 的 `WINDOW`/`ACK_EVERY` 导出（0.x API 变更，写进 CHANGELOG）。
- 不新增任何外部依赖（Go 和 npm 都不加）。终端示例用 `os/exec` 管道，不用 pty 库。
- **文件归属**：只改 `ipc/**`、`runtime-js/**`（含 `npm run build` 生成的 `dist/` 和嵌入的 `runtimejs` 脚本）、`examples/duplex/**`（新建）、§10 列出的文档、`CHANGELOG.md`、`docs/ipc-bench.md`（新建）。工作区里 `ui/editor/**`、`internal/platform/a11ybridge/**` 有其他会话未提交的改动，**不要碰**。
- **不提交 git**（用户没有要求提交）。每个任务结束时停在检查点，由用户决定是否提交。
- 每个任务结束时都要通过：`go build ./... && go vet ./ipc/... ./runtime-js/...`（从 Task 12 起再加 `./examples/duplex/...`）；`go test ./ipc/... ./runtime-js/...`；`cd runtime-js && npm test && npm run typecheck`（改过 `runtime-js/src` 的任务必须先 `npm run build`，否则 CI 的"bundle 是否最新"检查会失败）。

## Review Focus

1. **对端不守信用规则**（JS 有 bug，或页面被注入脚本，持续发帧不等 ack）：Go 不能无限缓冲，应以 `bad_args`（"flow control violated"）让这条流失败，其他流不受影响 —— Task 3 `TestDuplexFlowViolation`。
2. **页面在 Go 阻塞读写时重载**：处理函数卡在 `Receive`、另一个 goroutine 卡在满窗口的 `Write`，页面重载后两者都应在 1 秒内返回错误，不泄漏 goroutine —— Task 4 `TestDropUnblocksDuplex`。
3. **JS 不 `await` 的 `send()` 在流被取消后**：不能在控制台刷出 unhandled rejection —— Task 8 `"un-awaited sends do not cause unhandled rejections"`。
4. **零长度字节帧**：`sendBytes(new Uint8Array(0))` / `WriteBytes(nil)` 要作为一帧送达（`bin:""`），不能被当成格式错误或被吞掉 —— Task 2 `TestStreamDataEncoding`、Task 7 `"binary frames decode, including empty"`。
5. **进程内客户端和 JS 客户端行为不一致**：同一组场景（混合负载回显、信用阻塞、close/error/cancel）两种客户端观察到的结果必须相同 —— Task 5 `TestParity`。

---

### Task 1: 线协议 v2 的类型与握手

**Files:**
- Modify: `ipc/wire.go`（常量、`wireIn` 字段、`helloOut`）
- Modify: `ipc/router.go:186-208`（`HandleMessage` 的 `hello` 分支）、`Router` 结构体加窗口字段
- Create: `ipc/hello_test.go`
- Modify: `ipc/ipc_test.go`（`TestHelloResetsConnection` 改发 `proto:2`；`out` 结构体加 `Proto`、`Win`、`Seq`、`C`、`Bin *string`、`B`、`In *bool` 字段）

**Interfaces:**
- Produces:
  - `const ProtoVersion = 2`、`DefaultWindowBytes = 1 << 20`、`FrameOverhead = 64`、`MaxFrameBytes = 16 << 20`、`CodeTransport = "transport"`
  - `func (r *Router) SetStreamWindow(n int)`：设置 Go 的接收窗口（字节），`n < 4096` 时 panic；零值 Router 用 `DefaultWindowBytes`。
  - `func (r *Router) streamWindow() int`（内部）
  - `wireIn` 新增：`Proto int "proto"`、`Seq uint64 "seq"`、`C int "c"`、`Bin *string "bin"`、`B int "b"`、`In *bool "in"`；删除 `N`。
  - `type helloOut struct { Tango string; Proto int "proto,omitempty"; Win int "win,omitempty"; Error *Error "error,omitempty" }`

- [ ] **Step 1: 写失败的测试**（`ipc/hello_test.go`）

```go
func TestHelloV2Reply(t *testing.T) {
	r := NewRouter(); c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	raw := <-c.ch
	if raw != `{"tango":"hello","proto":2,"win":1048576}` { t.Fatalf("reply %s", raw) }
}
func TestHelloSetStreamWindow(t *testing.T) {
	r := NewRouter(); r.SetStreamWindow(8192); c := newFake()
	send(t, r, c, `{"tango":"hello","proto":2}`)
	if o := c.next(t); o.Win != 8192 { t.Fatalf("win %d", o.Win) }
}
func TestHelloProtoMismatch(t *testing.T) {
	for _, m := range []string{`{"tango":"hello"}`, `{"tango":"hello","proto":1}`} {
		r := NewRouter(); c := newFake()
		send(t, r, c, m)
		o := c.next(t)
		if o.Tango != "hello" || o.Error == nil || o.Error.Code != CodeBadArgs || o.Error.Message != "runtime-js protocol mismatch" { t.Fatalf("%s → %+v", m, o) }
	}
}
func TestSetStreamWindowTooSmall(t *testing.T) { /* defer recover; r.SetStreamWindow(4095) must panic */ }
```

- [ ] **Step 2: 运行，确认失败**

Run: `go test ./ipc -run 'Hello|SetStreamWindow' -v`
Expected: FAIL（`SetStreamWindow` 未定义 / 回复不匹配）

- [ ] **Step 3: 实现**：`hello` 分支先 `r.Drop(c)`（保留现有行为），再按 `m.Proto == ProtoVersion` 回 `helloOut{Proto,Win}` 或错误并 `slog.Warn("ipc: runtime-js protocol mismatch", "proto", m.Proto)`。更新 `ipc/wire.go` 包注释里的握手两行（完整线协议注释在 Task 14 重写）。

- [ ] **Step 4: 运行 `go test ./ipc/...`**，Expected：新测试 PASS。此时依赖旧 `n` 字段的流测试可能失败，Task 2 会重写它们，在这里**只允许**这些测试失败：`TestStreamBackpressure`、`TestGoOpenedStream`。

- [ ] **Step 5: 检查点**（不提交）

---

### Task 2: 流状态机（发送侧）：seq、计费、字节信用、accept、WriteBytes

**Files:**
- Modify: `ipc/stream.go`（重写为双向状态机的发送半边 + 内部 `peer`）
- Create: `ipc/peer.go`（`frame`、`ctl`、`peer`、`wirePeer`）
- Modify: `ipc/router.go`（`openFromJS`、`OpenStream`、`handleStream` 解码 `ack`/`accept`/`cancel`/`error`）
- Modify: `ipc/wire.go`（出站结构体：`streamData` 加 `Seq`、`C`、`Data json.RawMessage "data,omitempty"`、`Bin *string "bin,omitempty"`；`streamCtl` 加 `Seq *uint64 "seq,omitempty"`、`B int "b,omitempty"`、`In *bool "in,omitempty"`）
- Create: `ipc/stream_send_test.go`
- Modify: `ipc/ipc_test.go`（删除 `TestStreamBackpressure`，由新测试取代；`TestGoOpenedStream` 改为按 `b` 归还信用）

**Interfaces:**
- Consumes: Task 1 的常量、`streamWindow()`
- Produces（后续任务都依赖这些名字）：

```go
// ipc/peer.go
type frame struct {
	seq     uint64
	cost    int    // 计费大小（c）
	bin     bool   // true: payload 是原始字节；false: payload 是 JSON 文本
	payload []byte // 发送方已拷贝/编码，接收方可以直接持有
}
type ctl struct {
	op  string // "accept" | "ack" | "end" | "close" | "error" | "cancel"
	seq uint64 // end/close/error：本方向已发出的帧数
	b   int    // ack：归还的字节数
	in  bool   // accept：这一端是否接收数据
	err *Error // error
}
// peer 把本端的帧和控制消息交给对端。实现不得阻塞，也不得回调本端加锁的方法。
type peer interface {
	sendFrame(id uint64, f frame)
	sendCtl(id uint64, c ctl)
}
type wirePeer struct{ cs *connState } // 编码为 v2 JSON 消息，经 cs.send 发出

// ipc/stream.go
func newStream(parent context.Context, p peer, id uint64, sendWin, recvWin int, release func(*Stream)) *Stream
func (s *Stream) Write(v any) error         // JSON 帧
func (s *Stream) WriteBytes(p []byte) error // 字节帧；返回后 p 可复用
func (s *Stream) onCtl(c ctl)               // 对端的控制消息（本任务处理 ack、accept；其余见 Task 4）
```

  - `sendWin`：对 JS 连接是 `DefaultWindowBytes`（JS 不通告窗口）；对进程内连接是 Router 的 `streamWindow()`。`recvWin` 是 Router 的 `streamWindow()`。
  - `release` 在流彻底结束时调用一次，用来从 `connState.streams`（或 Task 5 的本地表）里移除。
  - `Stream.cs` 字段删除，`Stream` 只通过 `peer`、`parent`、`release` 与外界交互。
- 行为：
  - JS 打开、处理函数是 `HandleStream` 注册的：Router 先发 `{"op":"accept","in":false}`，再启动处理函数。找不到处理函数的行为不变（`error`/`not_found`，不发 `accept`）。
  - 处理函数返回 nil → 发 `{"op":"close","seq":N}`；返回错误 → `{"op":"error","seq":N,"error":…}`（N = 已发帧数）。

- [ ] **Step 1: 写失败的测试**（`ipc/stream_send_test.go`）。辅助函数 `openJS(t, r, c, id, method)` 发 `open`，并读掉 `accept`。

```go
func TestStreamDataEncoding(t *testing.T)
// HandleStream 处理函数依次 Write(map[string]int{"a":1})、WriteBytes([]byte("hello"))、WriteBytes(nil)
// 断言三条 data：
//   seq 0, c == len(`{"a":1}`)+64 == 71, data == {"a":1}, 无 bin
//   seq 1, c == 69, bin == "aGVsbG8=", 无 data
//   seq 2, c == 64, bin != nil && *bin == ""
// 随后 close seq 3

func TestStreamCreditBytes(t *testing.T)
// 处理函数循环 WriteBytes(make([]byte,1000)) 共 10 次（c=1064），JS 发送窗口按 DefaultWindowBytes 计，
// 为了测阻塞，用 export_test.go 暴露 setJSSendWindow(r, 4096)
// 断言：先收到恰好 3 条 data，c.none(50ms)；发 {"op":"ack","b":1064} 后恰好再收到 1 条

func TestStreamOversizeFrameWhenIdle(t *testing.T)
// 窗口 4096：WriteBytes(10000 字节) 立即发出（c=10064）；下一次 WriteBytes(1) 阻塞，直到 ack b=10064

func TestStreamMaxFrame(t *testing.T)
// WriteBytes(make([]byte, MaxFrameBytes+1)) 返回 *Error，Code == CodeBadArgs；随后 WriteBytes([]byte("x")) 成功送达，seq 0

func TestStreamAcceptInFalse(t *testing.T)
// JS open 一个 HandleStream 流，第一条回复 == {"tango":"stream","id":1,"op":"accept","in":false}

func TestHandlerReturnSendsCloseOrError(t *testing.T)
// 写 2 帧后返回 nil → {"op":"close","seq":2}；另一条流写 2 帧后返回 errors.New("boom") → {"op":"error","seq":2,"error":{"message":"boom","code":"internal"}}
```

  在 `ipc/export_test.go` 里定义测试钩子 `func setJSSendWindow(r *Router, n int)`（只给测试用的 Router 字段，生产代码里 JS 方向固定为 `DefaultWindowBytes`）。

- [ ] **Step 2: 运行**：`go test ./ipc -run 'StreamDataEncoding|StreamCredit|Oversize|MaxFrame|AcceptInFalse|HandlerReturn' -v`，Expected：FAIL

- [ ] **Step 3: 实现 `peer.go`、重写 `stream.go` 的发送半边、更新 `router.go`**。`WriteBytes` 在拿发送锁之前就完成 base64 编码（wirePeer）或拷贝（之后的 localPeer），这样返回后 `p` 可复用。阻塞等待沿用现有的"`wake` 通道 + ctx"模式。

- [ ] **Step 4: 运行 `go test ./ipc/...`**，Expected：全部 PASS（含改写后的 `TestGoOpenedStream`、`TestStreamCancel`、`TestStreamDropReleasesWriter`）

- [ ] **Step 5: 检查点**

---

### Task 3: 接收侧与 `HandleDuplex`

**Files:**
- Modify: `ipc/stream.go`（接收队列、重排、归还信用、`Receive*`）
- Modify: `ipc/router.go`（`HandleDuplex`、`data`/`end` 从 JS 到 Go 的解码与校验）
- Create: `ipc/stream_recv_test.go`

**Interfaces:**
- Consumes: Task 2 的 `frame`、`peer`、`newStream`、`onCtl`
- Produces:

```go
type DuplexFunc func(s *Stream, args json.RawMessage) error
func (r *Router) HandleDuplex(name string, fn DuplexFunc) // 与 Handle/HandleStream 同样的 panic 规则；accept 时 in=true
type Message struct {
	Binary bool
	Bytes  []byte
	JSON   json.RawMessage
}
func (s *Stream) Receive() (Message, error)       // 对方 end/close 且帧已取完 → io.EOF
func (s *Stream) ReceiveBytes() ([]byte, error)   // 队首是 JSON 帧 → *Error(bad_args)，帧留在队首
func (s *Stream) ReceiveJSON(v any) error         // 队首是字节帧 → *Error(bad_args)，帧留在队首；解码失败 → *Error(bad_args)，帧已取走
func (s *Stream) onFrame(f frame)                 // 对端的数据帧
```

- 规则：
  - 按 `seq` 交付：`seq == next` 入队并把缓存里接续的帧一起放进去；`seq > next` 放进缓存；`seq < next` 或重复 → 流 `error`（`bad_args`，"duplicate frame"）。
  - 内存上限：已缓存未读的 `len(payload)+FrameOverhead` 之和 > `recvWin + MaxFrameBytes + FrameOverhead` → 流 `error`（`bad_args`，"flow control violated"）。
  - 归还：每读走一帧累计它的 `cost`；累计 ≥ `recvWin/2` 时 `sendCtl(ack{b: 累计})` 并清零。
  - `in=false` 的流（`HandleStream`）收到帧：不入队，直接按同样规则累计并归还。
  - 线上校验（`router.go`）：`data` 同时有 `data` 和 `bin`、或两者都没有；缺少 `c` 或 `c < 64`；`bin` 不是合法 base64 —— 都回 `error`（`bad_args`），并按流失败处理。
  - `end` 带 `seq` = N：收齐 seq < N 的所有帧后，`Receive` 返回 `io.EOF`；之后再来 seq ≥ N 的帧 → 流 `error`（`bad_args`）。

- [ ] **Step 1: 写失败的测试**（`ipc/stream_recv_test.go`）

```go
func TestDuplexEcho(t *testing.T)              // 处理函数循环 Receive，原样回写（JSON 用 Write(json.RawMessage)，字节用 WriteBytes）；JS 发 {"x":1} 和 bin "AQID" → 回 data {"x":1}、bin "AQID"，seq 0、1
func TestDuplexReorder(t *testing.T)           // JS 先发 seq 1（bin "Ag=="）再发 seq 0（bin "AQ=="）→ Receive 依次得到 [1]、[2]
func TestDuplexAckAfterConsume(t *testing.T)   // SetStreamWindow(4096)；JS 发 2 帧 c=1064；处理函数读之前 c.none(50ms)；读走 2 帧后收到 {"op":"ack","b":2128}
func TestDuplexReceiveTypeMismatch(t *testing.T) // 队首是 JSON：ReceiveBytes 返回 *Error bad_args；随后 Receive 拿到这一帧且 Binary == false
func TestDuplexBadFrames(t *testing.T)         // 三种坏帧各开一条流：data+bin 都有 / 缺 c / bin "!!" → Go 回 {"op":"error","error":{"code":"bad_args"}}，处理函数的 Receive 返回 *Error bad_args
func TestDuplexFlowViolation(t *testing.T)     // SetStreamWindow(4096)；处理函数不读；JS 连发 bin 帧共 4096+MaxFrameBytes+65 字节以上（用一帧 MaxFrameBytes 的大帧 + 若干小帧）→ 流 error bad_args "flow control violated"；同一连接上另一条流仍能正常回显
func TestStreamFuncDropsInbound(t *testing.T)  // HandleStream 流；SetStreamWindow(4096)；JS 发 2 帧 c=1064 → 收到 ack b=2128，没有 error
func TestDuplexEndEOF(t *testing.T)            // JS 发 seq 0、end seq 2、seq 1 → Receive 得到两帧后 io.EOF（end 先到也要等 seq 1）
func TestDuplexFrameAfterEnd(t *testing.T)     // JS 发 end seq 0 后又发 seq 0 → error bad_args
```

- [ ] **Step 2: 运行**：`go test ./ipc -run Duplex -v`，Expected：FAIL

- [ ] **Step 3: 实现**

- [ ] **Step 4: 运行 `go test ./ipc/... -race`**，Expected：PASS

- [ ] **Step 5: 检查点**

---

### Task 4: 关闭语义与生命周期

**Files:**
- Modify: `ipc/stream.go`（`CloseSend`、`Close`、`CloseWithError`、`onCtl` 的 end/close/error/cancel、完成判定）
- Modify: `ipc/router.go`（处理函数返回后的收尾、`Drop`、Go 打开的流收到 `accept`）
- Create: `ipc/stream_close_test.go`

**Interfaces:**
- Consumes: Task 2、3
- Produces:

```go
func (s *Stream) CloseSend() error               // 发 end{seq}；之后 Write* 返回 *Error(canceled, "stream closed")
func (s *Stream) Close() error                   // 发 close{seq}；本端不再接收：之后到达的帧丢弃（不归还信用），Receive* 返回 io.EOF
func (s *Stream) CloseWithError(err error) error // 发 error{seq, err}；两个方向都失败
func (s *Stream) Context() context.Context       // 只在流彻底结束（两个方向都终止）或被中止时取消
```

- 规则：
  - 对端 `close`：本端 `Write*` 返回 `*Error{Code: CodeCanceled, Message: "stream closed by peer"}`；`Receive*` 照常读到 seq 再返回 `io.EOF`。
  - 对端 `error`：本端 `Receive*`、`Write*` 都返回对端给的 `*Error`（码和消息原样）。
  - 对端 `cancel`、`Drop`、连接 ctx 取消：`Receive*`、`Write*` 都返回 `context.Cause`（`canceled`）。
  - 彻底结束 = 发送方向终止（本端 end/close/error，或对端 close/cancel/error）**且**接收方向终止（读到 EOF、本端 close，或对端 error/cancel）。此时调用 `release`，并取消 `Context()`。
  - 处理函数返回时：流已经彻底结束 → 不再发任何消息；否则按 Task 2 的规则发 `close`/`error`。
  - Go 打开（`OpenStream`）：在收到 JS 的 `accept` 之前就可以写；收到 `accept{in:false}` 时，后续 `Write*` 返回 `bad_args`（"stream does not accept data"）（JS 的 `onStream` 永远发 `in:true`，这条只为对称）。

- [ ] **Step 1: 写失败的测试**（`ipc/stream_close_test.go`）

```go
func TestCloseSendHalfClose(t *testing.T)   // 处理函数 CloseSend → 收到 end seq N；处理函数仍能 Receive JS 帧；JS end → Receive io.EOF；处理函数返回 nil → c.none(50ms)
func TestPeerCloseStopsWrites(t *testing.T) // JS 发 close seq 1（之前发过 1 帧）→ 处理函数 Receive 得到 1 帧后 io.EOF；随后 Write 返回 *Error code canceled、message "stream closed by peer"；Context() 在 1s 内 Done
func TestPeerErrorPropagates(t *testing.T)  // JS 发 error {code:"internal",message:"x"} → Receive 与 Write 都返回 *Error{internal, "x"}
func TestPeerCancelUnblocks(t *testing.T)   // 处理函数阻塞在 Receive，另一个 goroutine 阻塞在满窗口的 WriteBytes；JS 发 cancel → 两者都在 1s 内返回 canceled
func TestDropUnblocksDuplex(t *testing.T)   // 同上，但触发方式是 JS 重发 hello proto 2（页面重载）；之后 runtime.NumGoroutine 回到基线（允许 +2 容差，最多等 1s）
func TestGoOpenedDuplex(t *testing.T)       // OpenStream；JS 回 accept in:true，发 bin "AQ==" → s.ReceiveBytes 得到 [1]；s.Close → close seq 0
func TestDuplexPanic(t *testing.T)          // HandleDuplex 处理函数 panic → {"op":"error","error":{"code":"internal"}}；OnPanic 回调收到 where == "stream Panicky"
```

- [ ] **Step 2: 运行**：`go test ./ipc -run 'Close|Peer|Drop|GoOpenedDuplex|DuplexPanic' -v`，Expected：FAIL

- [ ] **Step 3: 实现**

- [ ] **Step 4: 运行 `go test ./ipc/... -race -count=3`**，Expected：PASS

- [ ] **Step 5: 检查点**

---

### Task 5: 进程内客户端 `Router.Open` / `Router.Call`

**Files:**
- Create: `ipc/local.go`（`localPeer`、`Open`、`Call`）
- Create: `ipc/local_test.go`
- Create: `ipc/wireclient_test.go`（测试用的"JS 端"，在 `fakeConn` 上说 v2 协议）
- Create: `ipc/parity_test.go`

**Interfaces:**
- Consumes: Task 2–4（`newStream`、`peer`、`onFrame`、`onCtl`、`DuplexFunc`）
- Produces:

```go
func (r *Router) Open(ctx context.Context, method string, args ...any) (*Stream, error)
// 收到 accept 后返回客户端一侧的 *Stream；找不到处理函数 → *Error(not_found)。
// ctx 取消（在 Open 返回之前或之后）等于对这条流发 cancel（用 context.AfterFunc）。
func (r *Router) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error)
// 错误一律经 toWireError 转成 *Error，码与 JS 客户端看到的一致。
```

  - `localPeer` 成对创建：`a.sendFrame` 直接调用 `b.onFrame`，反之亦然。两端的 `sendWin`、`recvWin` 都是 `r.streamWindow()`。
  - 进程内流的 id 从 Router 自己的计数器分配（不与任何连接共享），客户端一侧和服务端一侧用同一个 id。
  - 服务端一侧的 parent context 是 `context.Background()`：处理函数里 `ConnFrom(ctx)` 返回 `ok=false`。进程内流不在任何 `connState` 里，`Drop` 不影响它。
  - 处理函数 panic：`safeCall(nil, ...)`；`reportPanic` 在 `c == nil` 时 `corehook.ReportPanic` 返回 false（`panic.go:66` 只认 `*Surface`/`*App`），落到 `slog`。
  - `HandleStream` 处理函数的流：客户端 `Write*` 返回 `bad_args`（"stream does not accept data"）。
- `wireClient`（测试辅助）：`func newWireClient(t *testing.T, r *Router) *wireClient`，提供 `open(method string, args ...any) *wireStream`；`wireStream` 有 `writeJSON`、`writeBytes`、`receive() (Message, error)`、`closeSend`、`close`、`cancel`，按协议自动归还信用。它就是 parity 测试里的"JS 客户端"。

- [ ] **Step 1: 写失败的测试**

```go
// local_test.go
func TestLocalCall(t *testing.T)            // "Greeter.Hello" → `"Hello world"`；未注册 → *Error not_found；处理函数返回 NewError("x","y") → Code "x"；返回 errors.New("z") → Code internal；处理函数里 ConnFrom ok == false
func TestLocalOpenNotFound(t *testing.T)    // Open("Nope") → *Error not_found
func TestLocalOpenStreamFunc(t *testing.T)  // Open 一个 HandleStream（写 0,1,2）→ Write 返回 bad_args；Receive 得到 JSON 0、1、2 后 io.EOF
func TestLocalCtxCancel(t *testing.T)       // Open 后 cancel ctx → 服务端阻塞的 Receive 在 1s 内返回 canceled
func TestLocalUnaffectedByDrop(t *testing.T) // r.Drop(某个 fakeConn) 后进程内流照常回显
func TestLocalBytesCopied(t *testing.T)     // 客户端 WriteBytes(buf) 后立刻改写 buf，服务端收到的仍是原值
// parity_test.go
func TestParity(t *testing.T)
// 表驱动；每个场景分别跑在 wireClient 和 r.Open 上，断言观察结果（收到的 Message 序列、错误码、EOF 时机）完全相同：
//   "echo mixed"：JSON {"a":1}、字节 [1 2 3]、空字节、JSON "s"
//   "credit"：SetStreamWindow(4096)，服务端不读，客户端写 3 帧 1000 字节后第 4 帧阻塞，服务端读 2 帧后第 4 帧送达
//   "server close"：服务端写 1 帧后返回 nil → 客户端读 1 帧后 EOF，客户端 Write 返回 canceled
//   "server error"：返回 NewError("x","boom") → 客户端 Receive 返回 *Error{x, boom}
//   "client cancel"：客户端 cancel → 服务端 Receive 返回 canceled
```

- [ ] **Step 2: 运行**：`go test ./ipc -run 'Local|Parity' -v`，Expected：FAIL

- [ ] **Step 3: 实现**

- [ ] **Step 4: 运行 `go test ./ipc/... -race`**，Expected：PASS

- [ ] **Step 5: 检查点**

---

### Task 6: `Stream.ReadWriter()`（`io` 适配）

**Files:**
- Create: `ipc/rw.go`
- Create: `ipc/rw_test.go`

**Interfaces:**
- Consumes: Task 3–5
- Produces: `func (s *Stream) ReadWriter() io.ReadWriteCloser`
  - `Read`：从字节帧里连续读，跨 `Read` 调用保留没读完的帧余量；对方结束 → `io.EOF`；队首是 JSON 帧 → `*Error(bad_args)`。
  - `Write`：每次调用发一帧；`len(p) > MaxFrameBytes` 时切成多帧，每帧最多 `MaxFrameBytes`。
  - `Close`：等于 `CloseSend`。
  - 同一个 `*Stream` 多次调用 `ReadWriter()` 返回同一个视图（读余量只有一份）。

- [ ] **Step 1: 写失败的测试**（全部用 `r.Open` + `HandleDuplex`）

```go
func TestReadWriterCopy(t *testing.T)        // 服务端用 ReadWriter 做 io.Copy 回显；客户端写 40 KiB 随机字节，用 1000 字节的缓冲区 io.ReadFull 读回，完全相等
func TestReadWriterSplitLarge(t *testing.T)  // 客户端 rw.Write(MaxFrameBytes+10 字节) → 服务端 ReceiveBytes 两次，长度分别是 MaxFrameBytes 和 10
func TestReadWriterJSONFrame(t *testing.T)   // 服务端 Write(1) → 客户端 rw.Read 返回 *Error bad_args
func TestReadWriterCloseIsHalf(t *testing.T) // 客户端 rw.Close() → 服务端读到 io.EOF 后再 WriteBytes([]byte("bye")) 并返回 → 客户端 rw.Read 仍能读到 "bye" 然后 io.EOF
```

- [ ] **Step 2: 运行**：`go test ./ipc -run ReadWriter -v`，Expected：FAIL
- [ ] **Step 3: 实现**
- [ ] **Step 4: 运行 `go test ./ipc/... -race`**，Expected：PASS
- [ ] **Step 5: 检查点**

---

### Task 7: JS 协议：握手、接收侧、accept

**Files:**
- Modify: `runtime-js/src/client.ts`
- Modify: `runtime-js/src/index.ts`（导出）
- Modify: `runtime-js/test/runtime.test.mjs`（改写依赖 `WINDOW`/`ACK_EVERY` 的测试，新增下列测试）

**Interfaces:**
- Produces（`client.ts` 导出）：

```ts
export const PROTO_VERSION = 2;
export const DEFAULT_WINDOW_BYTES = 1048576;
export const FRAME_OVERHEAD = 64;
export const MAX_FRAME_BYTES = 16777216;
export interface TangoStream<T = unknown> extends AsyncIterable<T | Uint8Array> {
  readonly id: number;
  readonly accepted: Promise<{ in: boolean }>; // 收到 accept 时 resolve；accept 之前就收到 error 时 reject
  cancel(): void;
  // send/sendBytes/end/bufferedAmount 在 Task 8 加，readable/writable 在 Task 9 加
}
```

  - 删除 `WINDOW`、`ACK_EVERY` 导出（`index.ts` 同步删除）。
  - 启动时发 `{tango:"hello",proto:2}`。收到 Go 的 `hello`：有 `win` 就记为发送窗口（Task 8 使用）；有 `error` 时客户端进入失败状态：之后的 `call` 被 reject、`stream` 的迭代器 reject，错误都是 `TangoError(message, "bad_args")`，并 `report` 一次。
  - 接收：`data` 按 `seq` 重排；`bin` → `Uint8Array`（base64 解码），`data` → 值；读走一帧累计它的 `c`，累计 ≥ `DEFAULT_WINDOW_BYTES / 2` 时发 `{op:"ack",b}`；读走一帧后队列已空且累计 > 0 时也立即发（见 Global Constraints）。
  - `end`/`close` 带 `seq`：收齐之前的帧后迭代结束。`close` 另外标记"对端不再接收"（Task 8 用）。
  - `onStream` 处理函数：在调用 `fn` 之前发 `{op:"accept",in:true}`。

- [ ] **Step 1: 写失败的测试**（加在 `runtime.test.mjs`；`fakeBridge` 不变）

```js
test("hello is posted on start", ...)               // b.take() 深等于 [{ tango: "hello", proto: 2 }]
test("binary frames decode, including empty", ...)  // Go 发 seq0 bin "aGVsbG8="、seq1 bin ""、close seq2 → 迭代得到 Uint8Array[104,101,108,108,111]、Uint8Array(0)，然后结束
test("reorders by seq and end waits for gaps", ...) // 发 seq1 data "b"、end seq2、seq0 data "a" → 迭代得到 "a","b" 后结束
test("acks by bytes after consumption", ...)        // 发两帧 c=300000；读 1 帧后没有 ack；读第 2 帧后 posted 里有 {op:"ack",b:600000}
test("onStream sends accept in:true first", ...)    // Go 发 open id 2 → posted 第一条是 {tango:"stream",id:2,op:"accept",in:true}，然后处理函数被调用
test("accepted resolves with in", ...)              // stream("X") 后 Go 发 accept in:false → await s.accepted 深等于 {in:false}
test("error before accept rejects accepted", ...)   // Go 发 error not_found → s.accepted reject，code "not_found"
test("protocol mismatch fails the client", ...)     // Go 发 {tango:"hello",error:{code:"bad_args",message:"runtime-js protocol mismatch"}} → 之后 call("A") reject，code "bad_args"
```

- [ ] **Step 2: 运行**：`cd runtime-js && npm test`，Expected：上述测试 FAIL
- [ ] **Step 3: 实现**（base64 解码优先用 `Uint8Array.fromBase64`，没有就用 `atob` 逐字节转换）
- [ ] **Step 4: 运行 `npm test && npm run typecheck`，再 `go test ./runtime-js/...`**（嵌入脚本已更新，`TestScriptEmbedded` PASS；`TestEndToEndWithNode` 在 Task 11 改写之前允许失败，但只能是因为 harness 用到了被删除的语义）。
- [ ] **Step 5: 检查点**

---

### Task 8: JS 发送侧：`send`/`sendBytes`/`end`、信用、`bufferedAmount`

**Files:**
- Modify: `runtime-js/src/client.ts`
- Modify: `runtime-js/test/runtime.test.mjs`

**Interfaces:**
- Consumes: Task 7
- Produces（加到 `TangoStream`）：

```ts
send(v: unknown): Promise<void>;
sendBytes(b: Uint8Array | ArrayBuffer | ArrayBufferView): Promise<void>;
end(): void;
readonly bufferedAmount: number; // 已排队、还没写进传输的帧的 c 之和
```

  - JSON 帧 `c = new TextEncoder().encode(JSON.stringify(v)).length + 64`；字节帧 `c = byteLength + 64`。字节在调用时就复制或编码（调用方之后可以改写缓冲区）。
  - 发送窗口：收到 Go `hello` 之前用 `DEFAULT_WINDOW_BYTES`，之后用 `win`。阻塞条件与 Go 相同（在途为 0 时可超窗）。
  - 帧按调用顺序排队；`end()` 排在已排队的帧后面，发出时 `seq` = 本方向帧总数；`end()` 之后的 `send*` reject `TangoError("stream closed","canceled")`。
  - 原始负载 > `MAX_FRAME_BYTES` → 这次调用 reject `bad_args`，流不受影响。
  - 收到 `accept{in:false}`、或对端 `close`/`cancel`/`error`、或本端 `cancel()`：排队中的和之后的 `send*` 都 reject（`in:false` 时 `bad_args` "stream does not accept data"；对端 close → `canceled` "stream closed by peer"；error → 对端的错误）。
  - 每个返回的 Promise 内部都先挂一个空的 `.catch`，所以调用方不 `await` 时不会产生 unhandled rejection，`await` 的调用方仍然拿到 rejection。
  - base64 编码优先用 `Uint8Array.prototype.toBase64`，没有就按 0x8000 字节一段 `String.fromCharCode` + `btoa`。

- [ ] **Step 1: 写失败的测试**

```js
test("send JSON frame shape", ...)                 // s.send({a:1}) → posted {tango:"stream",id:1,op:"data",seq:0,c:71,data:{a:1}}
test("sendBytes frame shape and copy", ...)        // const u=new Uint8Array([104,105]); s.sendBytes(u); u[0]=0 → posted bin "aGk=", c 66
test("credit blocks until ack", ...)               // Go hello win 4096；5 次 sendBytes(1000 字节) → 只 posted 3 帧；bufferedAmount === 2*1064；Go ack b 1064 → 再多 1 帧
test("idle frame may exceed window", ...)          // win 4096；sendBytes(10000) 立即 posted
test("oversize frame rejected, stream usable", ...) // sendBytes(MAX_FRAME_BYTES+1) reject bad_args；随后 send(1) posted seq 0
test("accept in:false rejects sends", ...)         // 发了 1 帧后 Go 回 accept in:false → 之后 send reject bad_args "stream does not accept data"
test("end is queued after frames", ...)            // win 4096 下排队 5 帧后 end() → 直到所有帧都 posted 之后才 posted {op:"end",seq:5}
test("un-awaited sends do not cause unhandled rejections", ...) // process.on("unhandledRejection") 计数；排队 3 个不 await 的 send 后 s.cancel()；等两个 tick，计数为 0
test("remote close rejects pending sends; iterator drains", ...) // 排队中的 send reject canceled "stream closed by peer"；close 之前到的数据仍能迭代出来
```

- [ ] **Step 2: 运行 `npm test`**，Expected：FAIL
- [ ] **Step 3: 实现**
- [ ] **Step 4: 运行 `npm test && npm run typecheck && cd .. && go test ./runtime-js/...`**，Expected：PASS（`TestEndToEndWithNode` 同 Task 7 的说明）
- [ ] **Step 5: 检查点**

---

### Task 9: Web Streams：`readable` / `writable`

**Files:**
- Modify: `runtime-js/src/client.ts`（或新建 `runtime-js/src/webstreams.ts`，由 `client.ts` 引入）
- Modify: `runtime-js/test/runtime.test.mjs`

**Interfaces:**
- Consumes: Task 7、8
- Produces（加到 `TangoStream`）：

```ts
readonly readable: ReadableStream<T | Uint8Array>;
readonly writable: WritableStream<unknown>;
```

  - 两个属性都是第一次访问时才创建。
  - `readable`：`highWaterMark: 0` 的 pull 型流，`pull` 每次取一帧（所以读走才归还信用）；`cancel()` → 流的 `cancel()`。异步迭代器和 `readable` 共用一个接收队列：先用哪个就锁定哪个，再用另一个时抛 `TangoError("stream already consumed","bad_args")`。
  - `writable`：`highWaterMark: 1`；`write(chunk)`：`Uint8Array`/`ArrayBuffer`/其他 `ArrayBufferView` → `sendBytes`，其他值 → `send`，返回那个 Promise（拿到信用才 resolve，`pipeTo` 因此按信用节流）；`close()` → `end()`；`abort(reason)` → 发 `{op:"error",seq,error:{message:String(reason?.message ?? reason),code:"canceled"}}` 并让本端失败。

- [ ] **Step 1: 写失败的测试**

```js
test("readable yields values and bytes then closes", ...)
test("readable pulls lazily: ack only after reads", ...)   // 两帧 c=300000 到达后不读 → 没有 ack；reader.read() 两次 → ack b 600000
test("readable and iterator are exclusive", ...)            // 先取 s.readable.getReader()，再 s[Symbol.asyncIterator]().next() → 抛 code "bad_args"
test("writable maps chunks, close → end", ...)              // write(new Uint8Array([1])) → bin "AQ=="；write({a:1}) → data；close() → end seq 2
test("writable abort sends error", ...)                     // abort(new Error("stop")) → {op:"error",error:{message:"stop",code:"canceled"}}
test("pipeTo respects credit", ...)                         // win 4096；ReadableStream 产出 10 块 1000 字节，pipeTo(s.writable) → posted 3 帧后停；ack 后继续；全部 ack 后 pipeTo resolve，最后是 end seq 10
```

- [ ] **Step 2–4: 运行 / 实现 / 运行**：`npm test && npm run typecheck`，Expected：PASS
- [ ] **Step 5: 检查点**

---

### Task 10: `tango.socket()`

**Files:**
- Create: `runtime-js/src/socket.ts`
- Modify: `runtime-js/src/client.ts`（`Client` 接口加 `socket`）、`runtime-js/src/index.ts`（导出 `socket`、`TangoSocket`）
- Create: `runtime-js/test/socket.test.mjs`

**Interfaces:**
- Consumes: Task 7–8
- Produces:

```ts
export class TangoSocket extends EventTarget {
  static readonly CONNECTING = 0; static readonly OPEN = 1; static readonly CLOSING = 2; static readonly CLOSED = 3;
  readonly url: string;            // "tango:" + method
  readyState: number;
  binaryType: "arraybuffer" | "blob"; // 默认 "arraybuffer"
  readonly bufferedAmount: number;
  onopen: ((e: Event) => void) | null;
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  onclose: ((e: CloseEvent) => void) | null;
  send(data: string | ArrayBuffer | ArrayBufferView | Blob): void;
  close(code?: number, reason?: string): void;
}
// Client: socket(method: string, ...args: unknown[]): TangoSocket
// index.ts: export function socket(method: string, ...args: unknown[]): TangoSocket
```

  - 状态：`CONNECTING` → 收到 `accept` 后 `OPEN` 并派发 `open` → `close()` 后 `CLOSING` → 流结束后 `CLOSED` 并派发 `close`。
  - `message.data`：JSON 字符串值 → 这个字符串；其他 JSON 值 → `JSON.stringify(值)`；字节 → `ArrayBuffer`，或在 `binaryType === "blob"` 时 `Blob`。
  - `send`：字符串 → `stream.send(字符串)`；`ArrayBuffer`/`ArrayBufferView` → `sendBytes`；`Blob` → 读成 `ArrayBuffer` 后 `sendBytes`，顺序与其他 `send` 调用保持一致（排进同一个 Promise 链）。`CONNECTING` 时调用照样排队（不抛 `InvalidStateError`，与 WebSocket 不同，写进 README）。
  - `close()` → `stream.end()`，并在对端结束后派发 `close`（code 1000，`wasClean` true）。对端 `error` → 先派发 `error` 再派发 `close`（code 1011，reason = 错误消息）。对端 `cancel` 或客户端失败（协议不匹配）→ `close`（code 1006，`wasClean` false）。
  - `CloseEvent` 不存在时（旧环境），用继承 `Event` 的同名类代替，带 `code`、`reason`、`wasClean`。

- [ ] **Step 1: 写失败的测试**（`socket.test.mjs`）

```js
test("readyState transitions and open event", ...)
test("message data mapping", ...)        // data "hi" → "hi"；data {a:1} → '{"a":1}'；bin "AQ==" → ArrayBuffer [1]；binaryType "blob" → Blob 大小 1
test("send mapping keeps order across Blob", ...) // send("x"), send(new Blob([new Uint8Array([1])])), send(new Uint8Array([2])) → posted 依次 data "x"、bin "AQ=="、bin "Ag=="
test("close handshake → code 1000", ...)
test("remote error → error then close 1011", ...)
test("remote cancel → close 1006", ...)
```

- [ ] **Step 2–4: 运行 / 实现 / 运行**：`npm test && npm run typecheck`，Expected：PASS
- [ ] **Step 5: 检查点**

---

### Task 11: Go + node 端到端

**Files:**
- Modify: `runtime-js/runtimejs/e2e_test.go`
- Modify: `runtime-js/runtimejs/testdata/harness.mjs`

**Interfaces:**
- Consumes: Task 1–10 全部

- [ ] **Step 1: 改写 harness 与测试**。现有的 `Count`、`Forever`、`news` 场景保留（`news` 改为 JS 在 `onStream` 里也回写 1 帧，Go 用 `s.Receive` 收）。新增场景（每个场景在 harness 里一个 async 函数，结果用 `tango.emit("result:<名字>", …)` 报回，Go 端断言）：
  - `duplexEcho`：`HandleDuplex("Echo")`；JS 发 JSON `{"k":"v"}`、字节 `[0,255]`、空字节、1 MiB 字节（第 i 字节 = i % 251）→ 原样收回，逐字节相等。
  - `backpressureBothWays`：`r.SetStreamWindow(64 << 10)`；Go 向 JS 写 4 MiB（64 KiB 一帧），JS 向 Go 写 4 MiB，两边都校验总字节数和末字节。
  - `pipeUpload`：JS 用 `ReadableStream` 生成 3 MiB，`pipeTo(s.writable)`；Go 用 `ReadWriter()` + `io.Copy(io.Discard)` 计数，返回 3145728。
  - `socketEcho`：`tango.socket("Echo")` 收发字符串和 `Uint8Array`，`close()` 后收到 code 1000。
  - `inProcessParity`：同一个 `Echo` 用 `r.Open` 跑一遍 `duplexEcho` 的输入，结果与 JS 端一致。
- [ ] **Step 2: 运行**：`go test ./runtime-js/... -run EndToEnd -v`，Expected：PASS（找不到 node 时 skip）
- [ ] **Step 3: 检查点**

---

### Task 12: `examples/duplex`：回显、上传、基准

**Files:**
- Create: `examples/duplex/main.go`、`examples/duplex/services.go`、`examples/duplex/frontend/index.html`、`examples/duplex/frontend/app.js`
- Create: `examples/duplex/e2e_test.go`（照 `examples/hello/e2e_test.go` 的写法，`TANGO_E2E=1` 才跑）
- Create: `docs/ipc-bench.md`

**Interfaces:**
- Consumes: `HandleDuplex`、`HandleStream`、`ReadWriter`、JS `stream`/`writable`/`socket`
- Produces（Task 13 复用）：`func registerServices(r *ipc.Router)`（在 `services.go`），注册：
  - `"Echo"`（`HandleDuplex`）：原样回写每一帧。
  - `"Upload"`（`HandleDuplex`，参数 `[name string, size int]`）：字节帧写进 `os.CreateTemp("", "tango-upload-*")`；每多收 1 MiB 回一帧 JSON `{"received":n}`；对方 `end` 后回 `{"path":p,"bytes":n}` 并返回 nil。
  - `"Sink"`（`HandleDuplex`）：读完所有帧只计数，结束时回 `{"bytes":n}`。
  - `"Source"`（`HandleStream`，参数 `[frames int, size int]`）：写 `frames` 个 `size` 字节的字节帧。
  - `"Bench.Report"`（`Handle`）：把参数里的 JSON 原样打印成一行到 stdout；带 `-bench` 时随后 `app.Quit()`。
- 页面：三个区块（回显、上传 `<input type=file>` 配 `file.stream().pipeTo(s.writable)` 和进度、终端占位由 Task 13 填）。URL 带 `?bench` 时自动跑基准：
  - 延迟：1000 次 64 字节回显往返（`Echo`），算 p50/p99（毫秒，保留两位小数）。
  - 吞吐 JS→Go：`Sink` 发 64 MiB（64 KiB 一帧），MB/s。
  - 吞吐 Go→JS：`Source` 读 1024 帧 × 64 KiB，MB/s。
  - 结果 `{"lane":"fallback","platform":navigator.userAgent,"latencyP50Ms":…,"latencyP99Ms":…,"upMBps":…,"downMBps":…}` 交给 `Bench.Report`。
- `main.go` 的 `-bench` 标志让窗口加载 `index.html?bench`。

- [ ] **Step 1: 写 e2e 测试**：`TestE2EDuplex` 启动示例，`WaitJS("!!window.tango")`，在页面里跑回显（发 `"ping"` 收 `"ping"`）和一次 2 MiB 的上传，断言返回的 `bytes == 2097152`。
- [ ] **Step 2: 运行**：`TANGO_E2E=1 go test ./examples/duplex -run E2E -v`，Expected：FAIL（示例还不存在）
- [ ] **Step 3: 实现示例**
- [ ] **Step 4: 运行 e2e**，Expected：PASS；`GOOS=windows GOARCH=amd64 go build ./examples/duplex && GOOS=linux go build ./examples/duplex` 通过（两个桌面后端都是纯 Go/purego，交叉编译不需要 cgo）。
- [ ] **Step 5: 在 macOS 上跑基准**：`go run ./examples/duplex -bench`，把输出的 JSON 和机器型号记进 `docs/ipc-bench.md` 的 "第一期基线（兜底传输）" 表格；其他平台留空行，写明"待测"。
- [ ] **Step 6: 检查点**

---

### Task 13: `examples/duplex`：终端与混合界面 Go 面板

**Files:**
- Create: `examples/duplex/term.go`
- Create: `examples/duplex/panel.go`
- Create: `examples/duplex/panel_test.go`
- Modify: `examples/duplex/main.go`、`examples/duplex/frontend/app.js`、`examples/duplex/frontend/index.html`

**Interfaces:**
- Consumes: Task 12 的 `registerServices`；`r.Open`；`uihost.Attach(s *tango.Surface, layer uihost.Layer, page func(*ui.Context), ropts ui.RunnerOptions) (*ui.Runner, *uihost.Attached, error)`（用法参照 `examples/webview-overlay/main.go`）
- Produces:
  - `"Term"`（`HandleDuplex`）：Unix 启动 `/bin/sh -i`，Windows 启动 `cmd.exe`，`os/exec` 管道（没有 pty：没有作业控制、不支持全屏程序，在页面上写明）。`go io.Copy(stdin, s.ReadWriter())`；stdout、stderr 都 `io.Copy(s.ReadWriter(), …)`；进程退出后返回 nil；流结束时杀掉进程。
  - 页面终端区块：`readable` → `TextDecoderStream` → 追加到 `<pre>`；输入框回车 → `writable` 写入 `行 + "\n"` 的字节。
  - `type coalescer struct`（`panel.go`）：`func (c *coalescer) push(item string)` 可在任意 goroutine 调用，追加到加锁的缓冲区，只在"还没有待处理的刷新"时调用一次 `post(fn)`；`func (c *coalescer) drain() []string` 在 UI 线程取走全部条目并清掉"待处理"标志。构造：`func newCoalescer(post func(func())) *coalescer`。
  - `-panel` 标志：用 `uihost.Attach` 叠一个 Go 自绘面板，在 goroutine 里 `r.Open(ctx, "Echo")`，每秒发一帧 `{"t":时间}`，收到的回显经 `coalescer`（`post` = `app.Post` + `runner.Invalidate`）显示最近 5 条；页面上的回显区块同时可用，两者共用同一个 Router。
- 这一任务里的 `coalescer` 和面板代码就是 Task 14 写 `ui.md` 时引用的示例源码。

- [ ] **Step 1: 写失败的测试**（`panel_test.go`）

```go
func TestCoalescerPostsOnce(t *testing.T)    // post 计数；10 个 goroutine 各 push 100 次且在 drain 之前 → post 只被调用 1 次；drain 返回 1000 条；再 push 1 次 → post 第 2 次
func TestCoalescerNoLostItems(t *testing.T)  // push 与 drain 并发交错跑 10000 条（post 立即在另一个 goroutine 里 drain）→ 所有 drain 结果拼起来恰好 10000 条、无重复（-race）
func TestTermEcho(t *testing.T)              // 仅 Unix：r.Open("Term")，写 "echo tango-ok\n"，5 秒内从 ReadWriter 读到包含 "tango-ok" 的输出；写 "exit\n" 后读到 io.EOF
```

- [ ] **Step 2: 运行**：`go test ./examples/duplex -run 'Coalescer|TermEcho' -race -v`，Expected：FAIL
- [ ] **Step 3: 实现**
- [ ] **Step 4: 运行同一命令**，Expected：PASS；三平台 `go build ./examples/duplex` 通过。
- [ ] **Step 5: 手动验证**：用 `run` skill 启动 `go run ./examples/duplex -panel`，截图确认页面终端能执行 `ls`、Go 面板每秒刷新回显、上传进度会走动。截图附在任务报告里。
- [ ] **Step 6: 检查点**

---

### Task 14: 文档

**Files:**
- Modify: `ipc/wire.go` 包注释（v2 线协议全文，消息例子与 spec §5.2 一致；删掉 "In both directions Go is the producer"）
- Modify: `docs/architecture.md:95-140`（桥接层与线协议）
- Modify: `site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`
- Modify: `site/reference/js-api.md`（`send`/`sendBytes`/`end`/`accepted`/`readable`/`writable`/`socket`/常量）
- Regenerate: `site/reference/api/ipc.md`（**不要手改**，运行 `cd site && npm run docs:api`）
- Modify: `runtime-js/README.md`（含 `socket` 与 WebSocket 的差异：`CONNECTING` 时 `send` 排队不抛错）
- Modify: `site/guide/ui.md`、`site/en/guide/ui.md`：新增"后台数据与线程"一节，要点照 spec §5.10，代码片段从 `examples/duplex/panel.go`、`term.go` 摘录（不另写一份不可编译的代码）
- Modify: `CHANGELOG.md`（顶部"未发布"段：协议 v2 与不兼容旧 runtime-js；`DefaultWindow`、`WINDOW`、`ACK_EVERY` 删除；`Stream.Close` 语义改为优雅全关闭、新增 `CloseSend`；新增 `HandleDuplex`、`Open`、`Call`、`ReadWriter`、`SetStreamWindow`；JS 新增 API）

- [ ] **Step 1: 写文档**
- [ ] **Step 2: 校验**：
  - `grep -rn 'DefaultWindow\b\|ACK_EVERY\|\bWINDOW\b' --include='*.go' --include='*.ts' --include='*.md' --include='*.mjs' ipc runtime-js/src runtime-js/README.md site docs/architecture.md` 没有结果（`node_modules`、`dist` 除外）
  - `cd site && npm run docs:build && npm run docs:links`，Expected：成功，没有坏链接
- [ ] **Step 3: 检查点**

---

## 完成后

- 全量：`go build ./... && go vet ./... && go test ./ipc/... ./runtime-js/... ./examples/duplex/...`；`cd runtime-js && npm test && npm run typecheck`；`TANGO_E2E=1 go test ./examples/duplex -run E2E`。
- 第二期：拿 `docs/ipc-bench.md` 的基线数据，按 spec §6 为每个平台通道（Android、Windows、macOS/iOS、Linux）各写一份计划。
