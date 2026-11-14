# IPC 双向流与二进制负载

状态：设计稿，待审 · 2026-11-13

## 1. 背景与目的

Tango 的 `ipc` 流目前只有 Go→JS 一个方向，负载只能是 JSON 值：JS 只能发 `open`/`ack`/`cancel`，`ipc/router.go` 收到 JS 的 `data`/`end` 直接丢弃。

终端、文件上传、交互式会话（例如 AI 对话中途追加指令）、协议代理这类场景都需要双向流，而且很多场景传的是字节而不是 JSON。现在的 Tango 只能用"Go→JS 流 + 一串编号的调用"拼出双向通道，没有统一的背压，字节也只能应用自己编码。

目标是 Tango 自身的通用能力：任何 Tango 应用都能在一条流上双向收发 JSON 值或字节，所有平台语义一致，并且在平台允许时走原生二进制通道。设计不针对某个具体应用。

### "完美"的标准

1. **语义一致**：所有平台、所有传输下，顺序、背压、半关闭、取消、断开行为完全相同。
2. **小帧低延迟**：终端按键这类几十字节的帧，延迟不比现有消息通道差。
3. **大流量不浪费**：日志、文件、大块终端输出，在平台允许时不做 base64、不多拷贝。
4. **上层无感**：换传输不改 API、不改协议语义，也不影响应用代码。

## 2. 目标 / 非目标 / 反目标

**目标**

- 流两端（不论哪边打开）都能发、能收，支持半关闭。
- 每条消息的负载是 JSON 值或字节，同一条流可以混用。
- 两个方向都有按字节计的信用窗口背压。
- 第一期：协议层完整，传输只用现有字符串通道（字节走 base64），全平台可用。
- 第二期：按平台接入原生二进制快速通道，能力协商，失败自动退回兜底。
- Go 侧除了消息式 API，还提供 `io.Reader`/`io.Writer` 适配，能直接接 `io.Copy`、pty、`bufio`。
- JS 侧除了异步迭代器，还提供 Web Streams 互通（`readable`/`writable`），以及类似 WebSocket 的适配器（方便现有 WebSocket 风格的代码，包括从 Wails `Stream`/`JSONStream` 迁移的代码）。
- **同一个服务给两种界面用**：Go 自绘界面（含混合界面里的 Go 面板）能以进程内客户端的身份打开同一批流服务、调用同一批方法，服务端处理函数分不出对面是 JS 还是 Go（§5.9）。
- **Go 自绘界面的数据传递写进文档**：后台 goroutine 取数据 → 切回 UI 线程 → 请求重绘的基本模式和线程规则（§5.10）。

**非目标**

- bindgen 为流生成 TypeScript 类型（现在也不生成，另立项）。
- 一次调用（`call`）的二进制参数/返回值。
- 跨页面/跨窗口的流，或页面重载后流自动恢复。

**反目标**（技术上能用但算失败的情况）

- 平台之间行为不一致：例如某平台顺序不保证、某平台背压失效。
- 为了快速通道让兜底路径变复杂或变慢。
- 快速通道出问题时静默丢数据或静默降级成错误语义。降级必须是透明且正确的，丢帧必须让流明确报错。
- 引入每帧一次 HTTP 往返的上行方案作为小帧的默认路径（Wails 的做法，小帧延迟变差）。

## 3. 约束（已核实的平台事实）

调研结论经过独立反向核实，本地 macOS SDK 27.2 头文件也逐条对照过。

| 平台 | 消息通道传二进制 | 自定义协议 Go→JS 流式 | 自定义协议 JS→Go 请求体 |
| --- | --- | --- | --- |
| Windows WebView2 | `postMessage` 只传 JSON。**SharedBuffer**（`ICoreWebView2Environment12::CreateSharedBuffer` + `ICoreWebView2_17::PostSharedBufferToScript`）：Go 创建，JS 映射为 `ArrayBuffer`；READ_WRITE 模式下 JS 写入对 Go 可见。JS 不能自己创建。每次 post 生成独立的 `ArrayBuffer`，要用 `chrome.webview.releaseBuffer` 释放。需 SDK 1.0.1661.34 / **Runtime ≥ 111**。没有 Atomics.wait 可用（不是 SharedArrayBuffer），同步靠消息门铃 | ❌ 响应必须在 deferral 完成时全部就绪（MicrosoftEdge/WebView2Feedback #3519 至今未修） | 可读，不流式 |
| Android WebView | ✅ `WebViewCompat.addWebMessageListener`：JS `postMessage(ArrayBuffer)` → `WebMessageCompat.getArrayBuffer()`；Go→JS `JavaScriptReplyProxy.postMessage(byte[])` → `ArrayBuffer`。需 androidx.webkit ≥ 1.8.0（Tango 用 1.11.0）、`WEB_MESSAGE_ARRAY_BUFFER` 特性、**WebView ≥ 116**。跨进程各拷贝一次 | ✅ 流式。默认按 2 KiB 一块读；`InputStream.available()` 返回准确值时按该值读取并立即送达 | ❌ 不暴露请求体 |
| macOS / iOS WKWebView | ❌ `WKScriptMessage.body` 只允许 NSNumber/NSString/NSDate/NSArray/NSDictionary/NSNull；`callAsyncJavaScript` 参数同样不收 NSData。macOS/iOS 27 的 `addBuffer:` 在 JS 侧只能读成字符串，不是二进制通道 | 官方文档说 `didReceiveData` 可多次调用、增量送达；**页面 `fetch` 能否逐块拿到需要实测** | ArrayBuffer、字符串能到；**Blob 会丢**（WebKit bug 197237 未修）；请求体不流式 |
| Linux WebKitGTK | 2.50 起脚本消息不带二进制类型；2.48 及以前可能能拿到 ArrayBuffer 字节（未实测，不依赖）。`call_async_javascript_function` 参数不收 `ay` | ✅ 2.36+，按 8 KiB 增量读取、逐块送达；响应头要等第一次读完成才发 | 2.40+ 可读，不流式 |

**Tango 现状的约束**

- JS↔Go 每条消息都是 JSON 字符串（`bridge.go` 的 `__tango.post` 只收字符串）。Go→JS 每条消息一次 `evaluateJavaScript`，无批处理（`surface.go` 的 `flush`）。
- `ipc` 不依赖根包：连接就是 `Conn{Send(string)}`，`*tango.Surface` 实现它。Router 由应用自己接到 `App.OnMessage` 上。
- 消息投递和 Eval 都在主线程（`internal/platform/backend.go` 的约定）。
- darwin/iOS 的 `tango://` 协议处理器原先在**主线程同步**执行 `ServeAsset`（按 64 KiB flush）；Apple 通道计划（2026-11-14，Task 2/3）起改为异步：`ServeAsset` 在每个请求自己的 goroutine 里运行，`WKURLSchemeTask` 的调用排队回主线程（见 §6.2）。Windows 整个响应缓冲在内存流里。Linux 2.36+ 用 goroutine + 管道。Android 用 goroutine + `GoStream`。
- Android 页面已经用 `WebMessageListener`（不支持时退回 `JavascriptInterface`），并带页面 token 校验。
- 可选能力的约定：在 `Backend`/`Surface` 上定义可选接口，核心层用类型断言发现（`internal/platform/ext.go`）。
- 最低版本：macOS 11、iOS 17、Android minSdk 24、WebKitGTK 4.1（可退 4.0）、WebView2 没有声明最低版本。

## 4. 方案取舍

讨论过的三种做法：

**A. 协议层统一 + 现有字符串通道传 base64。** 全平台一套实现，背压和取消复用现有机制。代价：二进制体积多约 33%，有编解码开销。小帧几乎无影响，大流量多耗 CPU。mygo 就是这条路，但只做了 Go→JS。

**B. 二进制全部走 `tango://` 自定义协议。** Wails v3 的做法：Go→JS 长轮询拿二进制包，JS→Go 每帧一个 POST，满了回 429 让 JS 重试。代价：大量平台垫片；macOS 的 JS→Go 照样要 base64（WKWebView 丢二进制请求体）；Windows 每次轮询限 1 MB；Wails 的 Android 实现实际不可用；JS→Go 每帧一次协议往返，小帧延迟比消息通道差；一个窗口所有连接共用一条管道。

**C. A 打底 + 按平台接原生快速通道。** 协议层与传输解耦。第一期只有兜底传输（即 A），第二期每个平台挑最好的原生通道，能力协商不通过就退回兜底。

**选定：C**，分两期交付。理由：第一期就是终局的一部分，不是以后要扔掉的临时方案；B 在终端这个最主要的双向场景（小帧、要低延迟）上反而更差；各平台最好的通道差别很大（共享内存、ArrayBuffer 消息、流式响应），只有把传输做成可插拔，才能各取最优。

另外从两边各借了一点：从 Wails 借类似 WebSocket 的 JS API 形状（方便迁移），从 mygo 借按字节计的信用窗口。

## 5. 第一期：协议层

### 5.1 流模型

- 每条流都是**全双工**，由一方打开（JS 用奇数 id，Go 用偶数 id，沿用现有规则）。
- 两个方向各自独立：`data* → end`（正常结束本方向）。
- 整条流的结束：
  - **双方都 `end`** → 正常完成。
  - **`close`**：一方优雅地全关闭。等于发 `end`，并声明自己不再接收；对方随后发来的数据被丢弃，对方的发送调用返回"已关闭"。Go 处理函数返回时用这个。
  - **`error`**：一方让整条流失败，双方都看到这个错误。
  - **`cancel`**：一方立刻放弃整条流，丢弃尚未读取的数据。
- 数据帧按方向带**序号**（`seq`，从 0 递增）。接收方按序号交付；`end`/`close`/`error` 带上本方向已发出的帧数（`seq` = 总帧数），接收方收齐这之前的所有帧后才处理结束。第一期只有一条通道，序号天然有序；这个机制是给第二期多条通道并存准备的（§6.2），第一期就要实现并测试。

### 5.2 线协议（v2）

依旧是每条原始消息一个 JSON 对象，用 `tango` 字段区分类型。`call`/`ret`/`event` 不变。

**握手**

```
JS → Go  {"tango":"hello","proto":2}
Go → JS  {"tango":"hello","proto":2,"win":1048576}
```

- JS 启动时发 `hello`（现有行为），新增 `proto`。Go 收到后照旧 `Drop` 掉这个连接的旧状态，然后回一个 `hello`，带上 Go 端的接收窗口 `win`。第二期有快速通道时再加 `lanes` 字段（§6.1）；没有这个字段就表示没有快速通道。
- 没带 `proto` 或 `proto` 不是 2 的 `hello`：Go 回 `{"tango":"hello","error":{"code":"bad_args","message":"runtime-js protocol mismatch"}}`，并用 `slog.Warn` 记一条日志（不走 `OnPanic`：这不是 panic，走那条路会误触发应用的崩溃处理）。**不兼容 v1**：runtime-js 嵌在 Go 模块里（`runtime-js/runtimejs`），npm 包 `@tangoui/runtime` 和 Go 模块同版本发布，0.x 阶段不值得维护两套协议（这是我的判断）。
- JS 在收到 Go 的 `hello` 之前，可以照常发 `call`/`event`/`stream open`（不阻塞现有调用）；发送流数据的信用按默认窗口预先计算（见 5.3）。

**流消息**

```
{"tango":"stream","id":3,"op":"open","method":"Term.Attach","args":[...]}
{"tango":"stream","id":3,"op":"accept","in":true}
{"tango":"stream","id":3,"op":"data","seq":0,"c":75,"data":{"cols":80}}  JSON 负载
{"tango":"stream","id":3,"op":"data","seq":1,"c":69,"bin":"aGVsbG8="}    字节负载（标准 base64）
{"tango":"stream","id":3,"op":"ack","b":65536}
{"tango":"stream","id":3,"op":"end","seq":2}
{"tango":"stream","id":3,"op":"close","seq":2}
{"tango":"stream","id":3,"op":"error","seq":2,"error":{"message":"...","code":"internal"}}
{"tango":"stream","id":3,"op":"cancel"}
```

- `accept`：接收 `open` 的一方确认流已建立（JS 打开时由 Router 在启动处理函数前发出；Go 打开时由 JS 在调用 `onStream` 处理函数前发出）。`in` 表示这一方是否接收数据：用 `HandleStream` 注册的老式只发处理函数为 `false`。找不到处理函数仍回 `error`（`not_found`），不发 `accept`。
- 打开方在收到 `accept` 之前**可以**发数据（不用多等一个往返），但如果最后收到 `in:false`，就让本地的发送调用失败（`bad_args`，"stream does not accept data"），并把流的发送方向置为关闭；已经发出的数据被对方丢弃。
- 只发处理函数（`in:false`）一侧收到 `data` 时直接丢弃并照常回 `ack`，不报错，避免竞态（打开方在 `accept` 前已经发了的情况）。
- `data` 必须二选一：有 `data` 字段是 JSON 负载，有 `bin` 字段是字节负载。两个都有或都没有：接收方让流 `error`（`bad_args`）。
- `ack` 的 `b` 是本次归还的信用字节数（见 5.3）。现有的 `n`（按条数）删除。
- 收到未知 `op`：忽略（为以后扩展留余地）。

### 5.3 背压：按字节计的信用窗口

- 每条流每个方向一个窗口，初始大小 = 对方在 `hello` 里通告的 `win`（默认 `DefaultWindowBytes = 1 MiB`，Go 侧可在 `Router` 上配置）。JS 在收到 Go 的 `hello` 之前按 1 MiB 计算。
- 每条数据帧的**计费大小** = 负载字节数 + 64（每帧固定开销，防止大量空帧绕过背压）。JSON 负载按**发送方**编码后的字节数；字节负载按原始长度（不算 base64 膨胀，这样换传输不影响计费）。
- **计费大小由发送方算出并写在帧的 `c` 字段里**，接收方读走一帧后归还的就是这个 `c`，自己不重新计算。原因：Go 的 `json.Marshal` 和 JS 的 `JSON.stringify` 转义与数字格式不同，两边各算各的会对不上，导致在途计数永远回不到 0（流最终卡死）或信用泄漏。快速通道上的字节帧没有 `c` 字段，计费大小固定为 `len + 64`，两边算法相同。缺少 `c` 或 `c` 小于 64 的 JSON 帧：接收方让流 `error`（`bad_args`）。
- 发送方：在途（已发未归还）字节数 + 本帧计费 > 窗口时阻塞；**例外**：在途为 0 时，单帧可以超出窗口（否则大于窗口的帧永远发不出去）。
- 接收方：**应用读走**一帧后才累计归还，累计到窗口一半时发一次 `ack`；另外，读走一帧后如果接收队列已经空了（读方追上了发送方）且还有没归还的信用，也立即全部归还。否则不足半窗的零头会一直留在发送方的在途量里，"在途为 0 时可超窗"的大帧就会永久阻塞（Task 3 审查中发现）。
- 单帧上限 `MaxFrameBytes = 16 MiB`（原始负载）。超过时发送调用直接返回错误，流不受影响。消息不拆分：一帧就是应用的一条消息，接收方拿到的边界和发送方一致（消息型流的基本语义，应用层协议可以依赖它）。需要连续字节流的场景用 §5.4 的 `io` 适配。
- 这替换掉现有的 `DefaultWindow = 64`（按条数）。这是 0.x 的公开 API 变更，写进 CHANGELOG。

### 5.4 Go API

在现有 `*ipc.Stream` 上补齐接收和字节方法；新增 `HandleDuplex` 注册双向处理函数。

```go
// 现有，签名不变：只发处理函数（accept 时 in=false）
func (r *Router) HandleStream(name string, fn StreamFunc)

// 新增：双向处理函数（accept 时 in=true）。返回 nil → close；返回错误 → error。
type DuplexFunc func(s *Stream, args json.RawMessage) error
func (r *Router) HandleDuplex(name string, fn DuplexFunc)

// OpenStream 不变，返回的 *Stream 现在也能接收（JS 侧 onStream 处理函数可以回写）。
func (r *Router) OpenStream(c Conn, method string, args ...any) (*Stream, error)

// *Stream 新增/调整的方法
func (s *Stream) Write(v any) error            // 现有：JSON 负载
func (s *Stream) WriteBytes(p []byte) error    // 新增：字节负载；调用返回后 p 可复用（内部已拷贝或已编码）
func (s *Stream) Receive() (Message, error)    // 新增：阻塞到下一帧；对方 end 后返回 io.EOF
func (s *Stream) ReceiveBytes() ([]byte, error)      // 新增：下一帧必须是字节负载，否则返回 *Error(bad_args)；这一帧留在队首，可以再用 Receive 取走
func (s *Stream) ReceiveJSON(v any) error            // 新增：下一帧必须是 JSON 负载并解码进 v
func (s *Stream) CloseSend() error             // 新增：半关闭（end）
func (s *Stream) Close() error                 // 现有，语义改为 close（优雅全关闭）
func (s *Stream) CloseWithError(err error) error // 现有：error
func (s *Stream) Context() context.Context     // 现有：流结束（任何原因）或连接断开时取消

type Message struct {
	Binary bool            // true: Bytes 有效；false: JSON 有效
	Bytes  []byte
	JSON   json.RawMessage
}
```

- `Receive*` 在流被取消、出错或连接断开时返回 `context.Cause(s.Context())`；对方正常 `end` 后返回 `io.EOF`。
- 同一条流上，`Write*` 之间、`Receive*` 之间各自串行（各自一把锁）；写和读可以并发，这是双向流的基本用法。
- `StreamWriter` 接口保持现状（`Write`、`Context`），不加新方法，避免破坏已有实现者。

**`io` 适配**（把字节帧当成连续字节流）

```go
// ReadWriter 返回 s 的 io.ReadWriteCloser 视图：
//   Read  从字节帧里连续读取，帧边界不可见；对方 end 后返回 io.EOF；
//         收到 JSON 帧时返回 *Error(bad_args)。
//   Write 每次调用发一个字节帧；超过 MaxFrameBytes 的 p 自动切成多帧
//         （这里不保留边界，所以可以拆）。
//   Close 等于 CloseSend（半关闭），这样 io.Copy 完成后对方能读到 EOF，
//         而本方仍可继续读。
func (s *Stream) ReadWriter() io.ReadWriteCloser
```

用法示例：`go io.Copy(pty, s.ReadWriter())` 加上 `io.Copy(s.ReadWriter(), pty)`，就是一个终端。

### 5.5 JS API

```ts
interface TangoStream<T = unknown> extends AsyncIterable<T | Uint8Array> {
  readonly id: number;
  readonly accepted: Promise<{ in: boolean }>;   // 收到 accept 时 resolve；被拒（not_found 等）时 reject
  send(v: unknown): Promise<void>;               // JSON 负载；在信用内立即 resolve，否则等 ack
  sendBytes(b: Uint8Array | ArrayBuffer): Promise<void>;  // 字节负载
  end(): void;                                   // 半关闭
  cancel(): void;                                // 现有
  readonly bufferedAmount: number;               // 已排队未发出的计费字节数
  readonly readable: ReadableStream<T | Uint8Array>;  // Web Streams 视图（读）
  readonly writable: WritableStream<unknown>;         // Web Streams 视图（写）
}
```

- 迭代时：JSON 负载产出解析后的值，字节负载产出 `Uint8Array`。
- **Web Streams 互通**（形状参照浏览器的 `WebSocketStream` / `WebTransport`）：
  - `readable` 和异步迭代器读的是同一个队列，二者只能用一个（先用的那个锁定，另一个再用会抛错）。`readable` 用 pull 模式：消费者读走一块才归还信用，背压一直传到 Go 的写方。`readable.cancel()` 等于 `cancel()`。
  - `writable` 的 `write(chunk)`：`Uint8Array`/`ArrayBuffer`/`ArrayBufferView` 作为字节负载，其他值作为 JSON 负载。`write()` 返回的 Promise 等到这一帧拿到信用才 resolve（`highWaterMark` 为 1），所以 `pipeTo` 会自动按信用节流。`writable.close()` 等于 `end()`，`writable.abort(reason)` 等于发 `error`。
  - 典型用法：`file.stream().pipeTo(s.writable)` 上传文件；`s.readable.pipeThrough(new DecompressionStream("gzip"))` 解压后再读。
- `send`/`sendBytes` 返回的 Promise 在帧被写进传输时 resolve，信用不足就等待（背压）；流关闭/出错后 reject。调用方不 `await` 也能用，帧会按顺序排队。
- `tango.stream(method, ...args)` 和 `tango.onStream(name, fn)` 签名不变，拿到的就是上面这个对象。
- 新增 `tango.socket(method, ...args): TangoSocket`，类似 WebSocket 的 `EventTarget` 适配器，建在 `TangoStream` 之上：
  - `readyState`（`CONNECTING` → 收到 `accept` 后 `OPEN` → `CLOSING` → `CLOSED`）、`binaryType`（`"arraybuffer"` 默认，或 `"blob"`）、`bufferedAmount`。
  - `send(string | ArrayBuffer | ArrayBufferView | Blob)`：字符串作为 JSON 字符串值发送，二进制作为字节负载发送。
  - `close(code?, reason?)` 映射为 `close`；对方 `error` → `error` 事件 + `close` 事件（code 1011）；正常结束 → `close` 事件（code 1000）；连接断开 → 1006。
  - `onopen`/`onmessage`/`onerror`/`onclose` 与 `addEventListener`。`message` 事件的 `data`：JSON 负载如果是字符串值就给这个字符串，其他 JSON 值给它的 JSON 文本（行为同 WebSocket 文本帧，调用方自己 `JSON.parse`）；字节负载按 `binaryType` 给 `ArrayBuffer` 或 `Blob`。
  - 目的：让现有 WebSocket 风格的代码（包括从 Wails `Stream`/`JSONStream`/`WailsSocket` 迁移的代码）以很小的改动接入。WebSocket 这套 API 没有真正的背压（只有 `bufferedAmount`），新代码推荐用 `TangoStream` 或 Web Streams。

### 5.6 生命周期与错误

- 页面重载/导航：JS 新文档发 `hello` → Router `Drop` 该连接的所有流，处理函数的 `Context` 被取消，`Receive*` 返回错误。沿用现有机制。
- 处理函数 panic：沿用 `safeCall`，转成 `internal` 错误，流 `error`，并经 `OnPanic` 上报。
- 错误码沿用 `not_found`/`bad_args`/`internal`/`canceled`，新增 `transport`（第二期快速通道丢帧时使用，见 6.2）。

### 5.7 兜底传输

就是现有的字符串通道：所有 `stream` 消息（包括 `bin` 字段的 base64）照常 `post` / `Send`。第一期不改 `Surface.flush` 的逐条 Eval；如果基准测试显示 Go→JS 小帧被逐条 Eval 拖慢，再把同一主线程轮次里的多条消息合并成一次 Eval（这属于传输优化，不改协议）。

### 5.8 第一期测试与基准

- Go：`ipc` 包的协议语义测试优先用 §5.9 的进程内客户端来写（两端都是 Go，不需要假的 JS 端）；线协议本身（消息格式、base64、`c` 字段、乱序注入、协议版本不匹配）用内存 `Conn` 对测。覆盖：双向收发、混合负载、半关闭各组合、`close`/`error`/`cancel` 的竞态、`accept` 前发送 + `in:false`、乱序帧按 seq 重排（直接注入乱序消息）、窗口阻塞与"在途为 0 可超窗"、MaxFrame、Drop、panic、协议版本不匹配。另外用同一组用例分别跑"JS 客户端（内存 `Conn`）"和"进程内客户端"，断言两者行为一致。
- JS：`runtime-js/test` 下用 node 测试：同样的语义矩阵，加 `TangoSocket` 的事件序列和 readyState。
- 端到端：Go 侧 + 真实 runtime-js 的集成测试（用 fake 后端，`internal/platform/fake`）。
- 基准（同一个基准程序贯穿两期，结果写进 `docs/`）：
  - 延迟：64 B 帧双向回显，p50/p99。
  - 吞吐：64 KiB 帧，单向持续发送，两个方向分别测。
  - 每个平台各跑一次，作为第二期的对照基线。

### 5.9 进程内客户端（Go 自绘界面使用同一批服务）

Go 自绘界面和服务在同一个进程里，本来不需要 IPC。但如果一个服务已经用 `Handle`/`HandleStream`/`HandleDuplex` 注册给 WebView 页面用了，Go 自绘界面（尤其是混合界面里和页面挨着的 Go 面板）应该能直接用它，而不是另写一套 Go 接口。

```go
// 在进程内打开一条流，拿到客户端一侧的 *Stream，方法与服务端一侧对称
// （Write/WriteBytes/Receive*/CloseSend/Close/CloseWithError/ReadWriter/Context）。
// 收到 accept 后返回；找不到处理函数返回 *Error(not_found)。
// ctx 被取消等于 cancel。
func (r *Router) Open(ctx context.Context, method string, args ...any) (*Stream, error)

// 在进程内调用 Handle 注册的方法，结果是处理函数返回值的 JSON 编码。
func (r *Router) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error)
```

- **语义完全相同**：`accept`（含 `in:false`）、序号、按字节计的信用窗口（计费规则和 `c` 字段一样）、半关闭、`close`/`error`/`cancel`、`MaxFrameBytes`、panic 处理，全都走同一套代码。服务端处理函数分不出对面是 JS 还是 Go。
- **不经过线协议**：两端直接交换帧对象，不做 JSON 消息封装，不做 base64。字节帧拷贝一次（保证双方都能复用自己的缓冲区）。JSON 帧照样用 `marshalValue` 编码、接收方照样解码，这样编码失败、类型不匹配等错误出现的位置和 JS 客户端一致。
- **连接身份**：进程内流不属于任何页面。处理函数里 `ipc.ConnFrom(ctx)` 返回 `ok=false`，需要页面的处理函数（例如要对某个窗口操作）应当自己处理这种情况。页面重载、`Drop` 不影响进程内流；`Router` 没有全局关闭，进程内流随 `ctx` 或任一端关闭而结束。
- **线程**：`Open`/`Call`/`Receive*`/`Write*` 都可能阻塞，**不能在 UI 帧里（page 函数里）调用**，要在 goroutine 里调用，按 §5.10 的模式把结果交回界面。
- 实现上，Router 内部把"连接"抽象成 JS 连接（经 `Conn` 编码成消息）和本地连接（直接递交帧）两种，协议状态机只有一份。具体拆法留给实现者。

### 5.10 Go 自绘界面的数据传递（文档任务）

在 `site/guide/ui.md`（以及英文版 `site/en/guide/ui.md`）新增"后台数据与线程"一节，内容要点：

- Go 自绘界面和后端在同一个进程里，直接调用 Go 函数、读 Go 结构体，不需要序列化，也不需要 IPC。
- **线程规则**：界面状态只在 UI 线程读写（page 函数运行在 UI 线程）。耗时或阻塞的操作放到 goroutine 里做；结果用 `app.Post` 切回 UI 线程写进状态，再调用 `r.Invalidate()` 请求重绘（`Invalidate` 任何 goroutine 都能安全调用）。
- **流式数据**就是 Go 原生的 channel、`io.Reader`/`io.Writer`、回调，背压由阻塞读写自然提供。给一个终端的例子：goroutine 读 pty → `app.Post` 追加到缓冲区 → `Invalidate`；按键在事件处理里直接写 pty。
- **高频数据**：每来一块数据就 `app.Post` 一次会在 UI 线程上堆积。示例里演示合并：goroutine 把数据追加到加锁的缓冲区，只在"还没有待处理的刷新"时才 `Post` 一次。
- **复用 WebView 的服务**：服务已经用 `Handle`/`HandleDuplex` 注册过的，Go 界面用 §5.9 的 `r.Call` / `r.Open` 接入，配合上面的线程规则使用；混合界面里页面和 Go 面板因此共用同一个服务实例。
- 文档里的每段示例代码都要能编译（放进 `examples/` 或有对应的测试）。

## 6. 第二期：原生快速通道

### 6.1 总体结构

- **快速通道只承载字节负载的数据帧。** JSON 负载帧、所有控制消息（`open`/`accept`/`ack`/`end`/`close`/`error`/`cancel`）、握手，一律继续走消息通道。这样快速通道的格式可以非常简单，也不用处理控制语义。
- 两个方向分开协商：一个连接可以只有下行快速通道、只有上行，或两者都有。
- 同一条流的字节帧可能一部分走快速通道、一部分走消息通道（例如小帧走消息、大帧走快速通道），靠 §5.1 的 `seq` 在接收端重排。
- 结束类消息带 `seq` = 本方向总帧数，接收方收齐之前的帧才处理结束，所以控制消息不会"跑到数据前面"。

**快速通道包格式**（所有平台相同，二进制，大端）：

```
packet := count:u16 | frame*count
frame  := streamID:u64 | seq:u64 | len:u32 | payload[len]
```

一个包可以带多条流的多帧（批处理）。包本身不做 base64。

**分层与职责**（谁拥有通道状态）

| 层 | 负责 |
| --- | --- |
| `ipc.Router` | 协议：序号、重排、信用、决定每帧走哪条路（按 `BinaryMinSize` 和 `SendBinary` 的返回值）。不知道平台和令牌；只保存核心提供的不透明文档世代身份和绑定发送句柄。 |
| 核心 `*tango.Surface`（根包） | **通道状态的唯一拥有者**：页面世代号、每世代令牌（同一世代内的每个通道请求都用它）、每条通道的状态（未就绪 / 就绪 / 已停用）、停用后的回退。处理通道控制消息，服务 `tango://` 通道端点。 |
| 平台后端（`internal/platform/*`） | 只提供原始能力：报告本页面支持哪些通道、发送下行包、把上行包交给核心。Android 的 ArrayBuffer 消息、Windows 的共享内存和门铃都在这一层实现。 |

**能力协商**

- JS 发 `hello` 后，Router 在回复里放入 `conn.(BinaryConn).LaneHello()` 返回的 JSON（不实现 `BinaryConn` 就不放）。Router 只是原样转发，不解析。由 `*tango.Surface` 生成，例如：
  `"lanes":{"gen":7,"tok":"…","list":["down:scheme","up:post"]}`。
  `list` 里可能有：`down:android`/`up:android`、`down:shm`/`up:shm`、`down:scheme`/`up:post`。
- JS 按 `list` 初始化对应的通道模块，成功后在核心内部频道上发 `"\x00tango-int:lane:"` + `{"op":"ready","lane":"down:scheme","gen":7}`，失败发 `"op":"fail"`。**Go 只在收到 `ready` 之后才往该通道发数据**；在此之前和失败之后，一律走消息通道。
- 通道控制消息走核心内部频道（`corehook.InternalPrefix` + `"lane:"` + JSON，JSON 为 `{"op":"ready"|"fail","lane":"<名>","gen":<uint>}`），由核心在 `App.dispatchInternal` 里处理，不进 Router，也不交给应用的 `OnMessage`。`gen` 不是当前世代的消息直接丢弃。（**实施裁定**：原文写的是 `{"tango":…}` 形式的 `lane` 消息（`"op"` 为 `ready`/`fail`/`wake`）；但 `App.dispatchInternal` 按内部前缀分发，`{"tango":...}` 开头的消息会被 Router 认领后丢弃，到不了核心，所以改走内部频道。）
- JS 每个方向按 `list` 的顺序一次只启动一个通道模块，前一个失败才试下一个；每个方向最多一个通道就绪。所以每个模块的启动都必须自己超时（`down:scheme` 5 秒内没有响应即失败）。（**实施裁定**：避免同时建立多条连接。）
- **打开方等 `accept`**：流的打开方在收到对方的 `accept` 之前，不把这条流的字节帧放上快速通道（先走消息通道）；接收方（发 `accept` 的一方）可以立即使用。原因：快速通道上的包可能跑到 `open` 消息前面，到达一个还不认识这条流的对端，被当作未知流丢弃，之后该流因缺口超时报 `transport`。Go 和 JS 两端规则相同。（**实施裁定**：审查中发现的竞态。代价：新流的头几帧走消息通道。）
- **导航**：页面开始导航时（`SurfaceNavigating`），核心立即暂停**下行**通道（`SendBinary` 返回 false、`BinaryMinSize` 为 -1、结束 `down:scheme` 响应），直到下一次 `hello`；上行通道保留（有令牌保护，在新页面加载完成前仍是旧文档；导航可能是下载或失败，不会替换页面）。页面加载完成（`SurfaceLoaded`）或核心发起导航时，如果目标不在资源来源，整个世代立即作废（令牌失效、所有通道停用）。资源来源的页面加载不作废世代，因为新页面的 `hello` 可能先于加载事件到达。（**实施裁定**：防止快速通道在页面离开资源来源后，绕过核心对不受信页面的消息拦截把应用数据送出去。代价：导航失败后下行回落到消息通道，直到页面重新加载。）
- 平台、WebView 版本、特性检测都在各自的后端里做。

**Go 侧接口**

```go
// ipc 包：连接可选实现。*tango.Surface 实现它。
type BinaryConn interface {
	Conn
	// LaneHello 返回放进 hello 回复 "lanes" 字段的 JSON；nil 表示本页面没有快速通道。
	// Router 每次收到 JS 的 hello、Drop 旧状态之后调用一次。
	LaneHello() json.RawMessage
	// SendBinary 把一个下行包交给已就绪的下行通道。返回 false 表示当前不可用，
	// Router 改走消息通道（这一帧和之后的帧都正确，靠 seq 重排）。
	SendBinary(packet []byte) bool
	// BinaryMinSize 返回走下行快速通道的最小帧大小；更小的帧走消息通道（延迟更低）。
	BinaryMinSize() int
}

// Router 新增：核心收到上行快速通道包时调用。
func (r *Router) HandleBinary(c Conn, packet []byte) // 仅用于不复用文档的简单连接
func (r *Router) HandleBinarySession(c Conn, generation uint64, packet []byte)

// 可复用 Surface 必须提供每个文档独立的传输句柄（Router 发布 connState 时捕获）。
type SessionBinaryConn interface {
    BinarySession() (generation uint64, transport BinaryConn)
    // Router.mu 内调用；只作身份失效，不得回调或重入 Router。
    DropBinarySession()
}
```

```go
// internal/platform：Surface 可选接口，按 ext.go 的约定用类型断言发现
type BinaryLane interface {
	LaneCaps() []string                       // 本页面当前可用的平台通道（down:android、down:shm……）
	SendLanePacket(gen uint64, p []byte) bool // 下行（平台通道）；gen 为包所属世代
}
// HostEx 扩展：后端把上行包交给核心；返回是否交给了 OnBinary
OnLanePacket(id SurfaceID, origin string, gen uint64, p []byte) bool
```

`down:scheme`/`up:post` 不经过 `BinaryLane`：它们是 `tango://` 请求，由核心在 `host.ServeAsset` 里先拦截 `/__tango/lane/` 路径（在交给 `opts.Assets` 之前），校验 `gen` 和 `tok` 后处理。下行端点是一个长期不结束的响应，所以依赖 §6.2 里 darwin/iOS 协议处理器的异步化；Linux 2.36+ 已经是异步的。

**应用侧接线**：和 `OnMessage` 一样由应用自己接，新增 `app.OnBinary(func(s *tango.Surface, gen uint64, p []byte){ r.HandleBinarySession(s, gen, p) })`。核心只在注册了至少一个 `OnBinary` 处理函数时才让 `LaneHello` 返回非空，没人接收就不开通道。所有示例（`examples/hello`、`runtime-demo`、`showcase`）和文档里的接线代码一并更新。jsapi 不负责接线，保持现状。

**来源限制**：快速通道只在页面来自应用资源来源时启用（macOS/iOS/Linux 是 `tango://app`，Windows/Android 是 `https://tango.localhost`）。从开发服务器（例如 `http://localhost:5173`）加载的页面，`LaneHello` 返回 nil，全部走兜底传输。语义完全相同，只是开发时没有性能优化。第二期的基准和验收都在资源来源上测。

**丢帧处理**

快速通道是本机进程间通道，正常情况下不丢；丢帧只会发生在通道本身坏掉时（页面拆除、fetch 被中断、共享内存被释放）。处理规则：

- 通道一旦报错，该方向立刻停用，后续帧全部走消息通道。
- 接收方有缓存的乱序帧（后面的帧已到）、或者 `end`/`close`/`error` 在等它之前的帧，而期待的下一帧连续 5 秒（`GapTimeout`，JS 为 `GAP_TIMEOUT_MS`）没到：让该流 `error`，错误码 `transport`（照常发给对端）。等待期间有任何进展（缺的帧到了）就重新计时。不尝试重传（没有发送端缓存，复杂度不值得）。（**实施裁定**：原文还要求"该方向已经停用或刚停用过"；去掉这个条件，接收方不需要知道通道状态。只有快速通道会造成缺口，正常情况下毫秒级就补上，5 秒足够宽。）
- **实施裁定（最终修复 I3）**：每次快速通道接受一帧后，发送方立即在可靠的**消息通道**发送 `{"tango":"stream","id":ID,"op":"boundary","seq":N}`，其中 N 为该帧序号加一（已接受帧数的排他上界）。`boundary` 不结束方向、不归还信用，也不承载字节；双方均实现。接收方每流只保存一个最大水位（常量空间），`recvNext < N` 即是缺口证据；连续进展重新计时，`recvNext >= N` 清掉未确认水位和对应缺口计时。这样即使最后一帧丢失、方向未结束、信用窗口已满、没有任何后续数据，也在可靠边界到达后最多 5 秒失败，并用既有 error 通知对端。已交付流、纯消息流、其他没有未确认边界的流不因通道失败被连带关闭。无需发送端缓存或逐帧确认列表；可靠水位和连续接收位置就是有界的未确认所有权表示。与旧 v2（忽略未知 op）互通不破坏数据格式，但丢尾活性保证需要双方升级。

**实施裁定（最终修复 I1/I2、I4–I7）**

- `OnBinary` 的签名为 `func(*Surface, uint64, []byte)`；中间参数是**授权时捕获**的不可变文档世代，必须交给 `HandleBinarySession`，不得在回调时重读当前世代。HTTP 的 `ServeUpSession` 把经过令牌校验的世代交给回调，所有回调在 State 锁外执行。Router 在其连接状态快照处匹配世代，只向该状态的流投递；Drop 在 Router.mu 内先调用 `DropBinarySession` 原子作废传输身份，再从 map 删除旧状态，解锁后取消流；与 connState 发布使用同一把 Router 锁，连 Drop 后、LaneHello 前创建的同 ID 过渡流也不会接受旧授权。State.mu 不跨应用回调；DropBinarySession 必须不回调、不重入 Router。
- `BinarySession()` 返回绑定世代的发送句柄。Router 在 LaneHello 后发布 connState 时捕获一次；旧 writer 无论在取消检查后暂停多久，都仍然使用旧句柄。`State.SendGeneration` 在同一锁内校验世代并准入 scheme 队列。Hello 先 Drop（作废传输世代），LaneHello 再启动新 lane 世代，然后只清理连接状态（不再作废刚创建的 lane 世代），清掉过渡区间并发发布的状态，最后发布新 connState；世代是身份而非连续计数（Drop 和 Hello 都可能增加它）。没有 LaneHello 的简单 Conn/BinaryConn 和进程内调用保持支持。原生正在进行的调用仍必须在后端绑定原文档/端口/环，不能发送给“当前页面”。
- `up:post` 每个请求 4 秒超时（`POST_TIMEOUT_MS`，低于 5 秒的 `GAP_TIMEOUT_MS`：卡住的 POST 先让通道失败、后续帧回退，而不是先让流因缺口失败），abort 并只 fail 一次；成功、失败、stop 均清理计时器和队列，晚到的 fetch 结果无效。连接级保留的**编码字节数**（包头+帧头+payload，包含在途包）最多 8 MiB；超限帧在保留或修改之前返回 false，使用原 seq/credit 走消息通道。成功结算释放在途包的计数，stop/fail 释放全部计数。
- **实施裁定（复查修复 I2，主线程卡顿时与消息通道语义一致）**：
  - JS 的 `boundary` 不在上行通道**接收**帧（`send()` 返回 true）时发，而在该帧**真正发出**时发：上行模块通过 `LaneContext.dispatched(stream, seq)` 报告（`up:post`：装着这帧的批次交给 `fetch` 时；可能在 `send()` 内同步调用），客户端随即发 `boundary`，`seq` 仍是"最后一个已发出的通道帧序号加一"。同一条流之后的每条消息通道发送——数据帧（JSON 和字节）、`end`、队列中的 `close` 标记——都等到它之前所有通道帧都发出后才发（复查第二轮 N1：否则更大的 seq 同样会让 Go 开始缺口计时）；通道帧本身不受阻挡（通道保持顺序）。`send()`/`sendBytes()` 的 promise 仍在通道接收时 resolve（信用语义不变）。上行通道停用/失败时，接收了却没发出的帧按丢失处理：客户端为它们补发 `boundary`、放行被扣住的 `end`/`close`，由 Go 的缺口计时照常判定 `transport`。在发出前已经结束的流忽略晚到的 dispatched。发出时 `boundary` 发送抛错：与原规则相同，帧保留 seq 和信用、不经消息通道重放，流以 `transport` 失败，队列里的发送全部 reject。中止路径（`_close`：writable abort、本地错误）的 `close`/`error` 仍立即发出。线路格式不变（没有新增 op 或字段）。Go 发送方在 `SendBinary` 接受包后发 `boundary`，不变（Go 的发送不受页面主线程影响）。
  - JS 的缺口计时器若比预定时间晚超过 1000 ms 才触发（页面主线程卡住，缺的帧可能就在通道缓冲里没被读），用完整超时重新计时一次；第二次到期照常失败。Go 侧不做对应处理（Go 计时器不随页面卡顿）。`up:post` 的 4 秒请求计时器用同样规则（复查第二轮 N2）：晚触发超过 1000 ms 重新计时一次，第二次到期让通道失败。
  - **复查第二轮 N3**：Go 分配的流 id 在同一 (Router, Conn) 上跨 `hello` 单调递增、不复用（连接状态替换时带上最后的 id），被回收流的 `cancel` 不会命中新文档的流；`Router.Drop`（页面关闭）忘掉它，Router 不为已关闭的 Conn 留任何东西。
- `down:scheme` 一次只从队列分离即将尝试写出的**一个包**；其余未尝试尾部始终留在队列最前面。连接替换不重发已尝试包、不丢尾部；后来准入的包排在尾部之后。在途包直到 Write 和 Flush 结算仍计入 8 MiB 队列额度。Reset/fail/结束世代丢弃尾部而不跨世代搬运。实际 Write/Flush 的阻塞解除仍是 §6.2 后端能力启用前提。
- 通道模块可继续请求内部相对路径，但生产 fetch 适配器只相对该世代捕获的 `location.href`（真实文档位置，**不是 document.baseURI**）解析，并校验协议+host、拒绝 credentials 和任意外部目的地。设置 `redirect:"error"`，并拒绝 redirected 响应。自定义 scheme 的 `origin` 可能为 `"null"`，不能仅比较 origin 字符串。Node 注入 fetch 仍支持相对路径。
- **实施裁定（Android 计划 Task 1，平台通道按世代绑定）**：
  - 平台上行包带世代：后端把页面随包发来的 `gen` 传给 `OnLanePacket(id, origin, gen, p)`，核心用 `lane.State.AllowUpGen(gen, p)` 在同一把锁里校验 `gen` 是当前世代且上行可用，再交给 `OnBinary`。取代原来在包到达时读取"当前"世代的 `AllowUpSession()`：延迟到达的旧世代包不会被算进新世代。
  - 平台下行包带世代：`platformSend(gen, p)` / `SendLanePacket(gen, p)` 仍在 State 锁外调用（避免锁顺序问题），`gen` 是包被准入时的世代；后端只在该世代的页面通道仍有效时投递，否则丢弃，并且不得回调 `lane.State`。
  - `OnLanePacket` 返回是否交给了 `OnBinary`：false 表示丢弃（页面已关、来源不符、`AllowUpGen` 拒绝），后端据此告诉页面上行被拒。
  - 平台上行的空包（count 0，例如 `up:android` 的探测包）检查可用性但不计入故障计数，与下行及 `up:post` 一致；通道故障后空包同样被拒。

### 6.2 各平台通道

#### Android：WebMessageListener 的 ArrayBuffer 消息（上下行）

- 条件：androidx.webkit 的 `WebViewFeature.isFeatureSupported(WEB_MESSAGE_ARRAY_BUFFER)` 为真（WebView ≥ 116），且页面走的是 `WebMessageListener` 而不是 `JavascriptInterface` 兜底。
- 上行：JS `postMessage(ArrayBuffer)`，`Page.java` 的 listener 收到 `TYPE_ARRAY_BUFFER` 后，经 JNI 交给 `OnLanePacket`。现有的字符串消息带页面 token；二进制消息改用 listener 提供的 `sourceOrigin` 和 `isMainFrame` 校验。包格式为 `token[16] | gen:u64(LE) | 公共包`：`token` 与字符串消息的页面 token 相同，不匹配就丢弃；`gen` 是 hello 给的通道世代，交给 `OnLanePacket` 由核心校验（§6.1 实施裁定）。
- 下行：`replyProxy.postMessage(byte[])`。`replyProxy` 来自 JS 的每条消息；Java 不解析 ipc 消息，所以不在 `hello` 时保存，而是在**合法探测包**（`token | gen | 00 00`）被核心接受时连同它的 `gen` 保存，之后的新探测替换它。核心交来的下行包只发给同一世代保存的 `replyProxy`，其他世代的丢弃。在主线程调用。
- 回复（都是 `ArrayBuffer`）：长度 1 的 `[0]` = 探测确认，`[1]` = 上行包或探测被核心拒绝（`OnLanePacket` 返回 false）；长度 ≥ 2 = 下行公共包。
- `BinaryMinSize`：0（全部字节帧走快速通道；它本来就是消息通道，没有额外延迟）。
- 各方向各拷贝一次（跨进程），可以接受。
- **实测（2026-11-14，模拟器 tango36：API 36，WebView 133，arm64 镜像跑在 Apple M4 上；Android 计划 Task 2–6）**：
  - 回复标记 `[0]` / `[1]` 以长度 1 的 `ArrayBuffer` 到达页面；探测确认、旧世代探测拒绝、数据包按世代交给 `OnBinary` 都符合设计。
  - 自测（`scripts/smoke.sh android-selftest` / `android-lanefault`，连续 3 次、每次全新安装都通过）：8 MiB 双向逐字节正确；大小交替的 40 帧全部走 `up:android`、顺序正确；重载后通道恢复、goroutine 回到基线、下行队列清空；`reloadSameID`（旧页面的 stream 1/3 与新页面的 stream 1/3 同号）两个方向都没有串流；主线程忙 6 s 不报 `transport`；同源 iframe 经自己的 `__tangoAndroid` 发的 50 个伪造包 Go 一个都没收到，不透明来源 iframe 没有该对象；`down:android:3` 故障后帧改走消息通道、不丢；`up:android:3` 故障后页面收到 `[1]` 在 12 ms 内回退，丢帧的流约 5 s 报 `transport`，并行的流完整；没有 ArrayBuffer 能力（测试钩子）时不报告通道。
  - 基准（[docs/ipc-bench.md](../ipc-bench.md)「第二期 Android」）：64 KiB 帧 JS→Go 6.11×、Go→JS 4.39×，分批 p99 0.16（过 §6.3 判定）；分批 p50 慢约 24%（每帧一次 ArrayBuffer 消息 + 一次 JNI 拷贝）。用户据此决定**默认开启**，`TANGO_IPC_LANES_ANDROID=0` 关闭。
  - 顺带修正（与通道无关）：Android 资源响应原先有两个 `Content-Length`（WebView 按 `InputStream.available()` 自己加一个），改为把长度经 `available()` 报告、不再放进头；资源处理函数 panic 的正文与 Apple 一致。

#### Windows：SharedBuffer 环形缓冲区（上下行）

- 条件：能 `QueryInterface` 到 `ICoreWebView2Environment12` 和 `ICoreWebView2_17`（Runtime ≥ 111）。
- 每个页面世代（每次 `hello`）创建两块 READ_WRITE 共享内存：下行环、上行环，各 4 MiB（可配置）。用 `PostSharedBufferToScript` 发给页面，`additionalDataAsJson` 带 `{"tango":"lane","ring":"down"|"up","gen":N}`。JS 在 `sharedbufferreceived` 里拿到，初始化后发 `ready`。这是 SharedBuffer 元数据，不经过 `App.dispatchInternal`，不适用 §6.1 的内部频道裁定；保留 `tango` 判别字段、`ring` 方向和 `gen` 世代号。
- 环的布局：头部 64 字节（写位置、读位置、读方休眠标志、世代号），后面是数据区。写方写入完整包后推进写位置；读方读完推进读位置。Go 侧对头部字段用原子读写（`sync/atomic` 作用在映射内存上）；JS 侧普通读写即可（单线程）。
- 同步靠门铃消息：写方推进写位置后，如果读方休眠标志为真，发一条 `op:"wake"` 门铃（下行 `PostWebMessageAsString("{\"tango\":\"lane\",\"op\":\"wake\"}")`，由页面侧平台通道模块接收；上行 `chrome.webview.postMessage` 发送 `"\x00tango-int:lane:"` + `{"op":"wake"}`，走 §6.1 的核心内部频道，而不是 Router）。环满时写方阻塞，等读方推进读位置后发回同样的 `wake` 门铃。跨进程消息本身充当内存屏障；文档里写明这一依赖。
- 单个包大于环容量的一半时，这个包走消息通道（不在环里拆包）。
- 重载/导航：旧世代的环在 Go 侧 `Close()`，JS 侧 `releaseBuffer`；新世代重新创建。
- `BinaryMinSize`：0。门铃合并之后，小帧也不比消息通道慢；如果基准不支持这一点，改成按基准结果设阈值。

#### macOS / iOS / Linux：下行走 `tango://` 流式响应

- 条件：
  - macOS/iOS：**先写一个实测用例**，确认 `didReceiveData` 的每一块都会被页面 `fetch` 的 `ReadableStream` 单独读到（而不是攒到 `finish`）。确认之前这条通道默认关闭。
  - Linux：WebKitGTK ≥ 2.36（已有 goroutine + 管道的流式实现）。
- JS 收到 `hello` 后发起 `fetch("tango://app/__tango/lane/down?gen=N&tok=…")`，读 `ReadableStream`，按包格式解析。`tok` 是 Go 在 `hello` 回复里给的一次性随机令牌，与世代号绑定，防止其他页面/iframe 接入。
- Go 侧这条请求长期不结束：每写完一个包就立即 flush。连接建立后先写一个空包（`count=0`），让 Linux 尽快把响应头发出去、`fetch` 尽快 resolve。
- **前置改造**：darwin/iOS 的协议处理器现在是在主线程同步执行 `ServeAsset`。要改成：处理函数在 goroutine 里运行，每次 flush 时切回主线程调用 `didReceiveData`；任务被 WebKit 停止（`stopURLSchemeTask`）时取消 goroutine 的 context。这个改造对普通静态资源也有好处（大文件不再卡主线程），但改动面较大，单独测试。（**状态，2026-11-14**：macOS 与 iOS 都已完成，Apple 通道计划 Task 2/3；应用的资源处理函数也一并改在主线程之外运行，见 CHANGELOG。）
- 对应的 JS 读循环，不需要额外的 ack，背压由协议层信用窗口负责（WebKit 这一层本身没有流控）。
- `BinaryMinSize`：4 KiB（初始值，按基准调整）。更小的帧走消息通道，因为一次 Eval 的延迟低于穿过网络栈的一块数据。
- **实测结论（2026-11，macOS 26 / iOS 27.2 模拟器）**：两个平台都**逐块交付**，`down:scheme` 的前提成立。
  - 方法：环境变量 `TANGO_SCHEME_PROBE=1` 时，后端自己在主线程回答 `/__tango/probe/stream`（不经 `ServeAsset`）：立即发响应头和第 1 块，之后每 200 ms 一次 `didReceiveData:`，共 8 块、每块 1 KiB，最后 `didFinish`；页面用 `fetch` 的 `ReadableStream` 记录每块到达的时间（相对 `fetch()` 调用）。判定：块数 ≥ 4，且第 1 块比最后一块早 1000 ms 以上。macOS：`TANGO_E2E=1 go test ./internal/platform/darwin -run StreamingDeliveryProbe -v`（`examples/duplex -page probe.html`）；iOS：`bash scripts/smoke.sh ios-probe`（`examples/duplex-ios`，iPhone 18 Pro 模拟器）。
  - macOS 26.6（WKWebView）：8 块，未合并，每块正好 1024 字节；响应头 14 ms 到达页面，各块到达 16 / 204 / 406 / 607 / 808 / 1010 / 1212 / 1413 ms——与发送节奏一致，第一块延迟约 16 ms。
  - iOS 27.2 模拟器：8 块，未合并，每块 1024 字节；响应头 19 ms，各块到达 22 / 203 / 403 / 603 / 804 / 1003 / 1203 / 1404 ms，第一块延迟约 22 ms。
  - 实测响应带 `Content-Type: application/octet-stream` 和 `X-Content-Type-Options: nosniff`，第 1 块（1 KiB）没有被攒着等更多字节；不带这两个头时的行为未测，通道响应应同样带明确的 `Content-Type`。真机 iOS 未测。
- **实测补充（Task 4，2026-11-14）**：小块逐块交付成立，但**大块突发会被 WebKit 部分扣住**，`down:scheme` 在 Apple 平台上不可用。（**实施裁定**：macOS/iOS 只报告 `up:post`，不报告 `down:scheme`；下行字节帧走消息通道。）
  - 现象：经 `down:scheme` 连续下发 16 个 64 KiB 包（后端约 2 ms 内全部 `didReceiveData:` 交给 WebKit），页面 `ReadableStream` 只读到其中一部分（例如 11/16，扣住 5 个），其余留在 WebKit 里，直到同一响应上有新数据到达才一起交出；若这段时间流已结束，JS 的缺口计时器 5 s 后报 `frames lost on a fast lane`。macOS 26.6 与 iOS 27.2 模拟器都如此（iOS：16 帧只到 3 帧）。按帧大小扫描（macOS，同一进程）：16×64 KiB 三次均失败，64×16 KiB 三次失败两次，256×4 KiB 均成功，2×64 KiB 也会失败。
  - 我们的写出端没有拖延：临时追踪（`schemeTask.data` 与写出端 `Write` 打时间戳，页面包装 `ReadableStreamDefaultReader.prototype.read`）显示 16 次 `didReceiveData:` 都在 2 ms 内完成，被扣住的字节在约 5 s 后、下一批数据到达时才被页面读到（例：后端 t=25437 ms 交出新数据，页面 t=25441 ms 的第一次读取拿到被扣住的 4 个包 262232 字节 = 4 × 65558；扣住的第 5 个包没有在追踪里单独记下，是在之后的读取里随新数据一起到的，追踪没有区分）。每 50 ms 唤醒一次主 run loop 不能消除，排除了 UI 进程主线程饥饿；iOS 的写出端与桥是另一套实现，表现相同，所以判定是 WebKit 自身的行为。
  - 处理：Apple 平台的 `LaneCaps` 只返回 `["up:post"]`（macOS 默认开启，iOS 由 `TANGO_IPC_LANES_DARWIN=1` 开启；`TANGO_IPC_LANES_DARWIN=0`/`false`/`off` 或 `TANGO_IPC_LANES=0` 关闭）；`TANGO_IPC_LANES_DARWIN=all` 仍可报告 `down:scheme`，仅供实验。可能的绕法——突发之后补写一个空包（`count=0`）把扣住的数据"推"出来——需要改公共设施（`internal/lane` 的写出节奏），本计划未做。

#### macOS / iOS / Linux：上行走 POST

- 条件：macOS/iOS 都可用（ArrayBuffer 请求体能到达）；Linux 需要 WebKitGTK ≥ 2.40（`webkit_uri_scheme_request_get_http_body`）。
- **状态（2026-11-14）**：macOS/iOS 已接入，是 Apple 平台上唯一报告的通道。**macOS 默认开启**（§6.3 基准，Apple 通道计划 Task 7：上传 64 KiB 帧吞吐 2.73 倍，64 B 帧 p99 两种配置走的都是消息通道，见 [docs/ipc-bench.md](../ipc-bench.md)）；`TANGO_IPC_LANES_DARWIN=0`/`false`/`off` 关闭，`all` 另报 `down:scheme`（仅供实验）。**iOS 默认关闭**，等真机测量，`TANGO_IPC_LANES_DARWIN=1` 开启（两个平台的取值见 `internal/platform/darwin/lanes.go`、`internal/platform/ios/lanes.go`）。runtime-js 的 `up:post` 启动时先 POST 空包（`count=0`），收到 204 才就绪：页面 CSP 不允许 `connect-src` 到资源来源时通道在启动时失败、帧全部走消息通道，不会丢帧（`TestLaneCSPBlocked`）。实测（macOS 26.6、iOS 27.2 模拟器）：8 MiB 双向回显、大小帧交替、页面卡住 6 s、页面重新加载均通过；iframe 拿不到令牌（macOS 实测：不透明来源与同源 frame 的通道请求全部被拒；iOS 未单独测）；iOS 真机与 iOS 上的通道故障场景未测。
- JS 把待发的字节帧攒成包，`fetch("tango://app/__tango/lane/up?gen=N&tok=…", {method:"POST", body: arrayBuffer})`。**必须用 ArrayBuffer 作为 body，不能用 Blob**（WebKit bug 197237）。iOS 现有的 `bodyshim` 只处理 Blob/File/FormData，ArrayBuffer 不受影响。
- 同一时刻每个页面只有一个上行 POST 在途；在途期间新帧继续攒进下一个包。包大小上限 1 MiB。
- 只有攒够 `BinaryMinSize`（初始 16 KiB）或者已经攒了 4 ms 才发包；小帧（例如按键）直接走消息通道，延迟不受影响。
- Go 侧收到请求体后调用 `HandleBinary`，立即回 204。

#### 不做的通道（以及原因）

- Windows 的 `tango://` 流式响应：WebView2 不支持（§3）。
- Android 的 `shouldInterceptRequest` 流式响应：ArrayBuffer 消息更直接，两个方向都有，不需要第二条路。
- Linux 2.48 及以前的脚本消息 ArrayBuffer：没实测、2.50 起就没有了，不值得为它写一条通道。
- macOS/iOS 27 的 `addBuffer:`：JS 侧只能读成字符串。

### 6.3 第二期验收

- 每条通道独立上线，默认开启的前提：在对应平台上跑 §5.8 的基准，满足：
  - 64 KiB 帧吞吐至少是兜底传输的 2 倍；
  - 64 B 帧的 p99 延迟不比兜底差。
  - 不满足的通道保留代码但默认关闭，文档里写明原因和数据。
  - **状态（2026-11-14）**：macOS `up:post` 达标（上传 2.73×；64 B 帧在两种配置下都走消息通道，p99 只说明没有变差），**默认开启**，`TANGO_IPC_LANES_DARWIN=0` 关闭。iOS `up:post` 只有模拟器数据（2.26×，参考），**默认关闭**，等真机。Apple 上的 `down:scheme` 不达标（WebKit 扣住大块突发，§6.2），只能 `TANGO_IPC_LANES_DARWIN=all` 实验性开启。Android `down:android` / `up:android` 达标（模拟器：上行 6.11×、下行 4.39×，分批 p99 0.16），**默认开启**，`TANGO_IPC_LANES_ANDROID=0` 关闭；没有真机数据。数据见 [docs/ipc-bench.md](../ipc-bench.md)。
- 每个平台都要有"通道中途坏掉"的测试：强制让通道失败，验证后续帧自动走消息通道、丢帧的流报 `transport` 错误、其他流不受影响。
- 提供关闭快速通道的开关（`tango.Options` 或环境变量），方便排查问题。

## 7. 已定的决策

- 选方案 C，分两期；第二期各平台通道都要设计，都写进本文档。
- 二进制和 JSON 两种负载都是一等公民（用户的要求）。
- 第二期各平台通道选型以 §3 已核实的事实为准：Android 用 ArrayBuffer 消息，Windows 用 SharedBuffer，macOS/iOS/Linux 下行用流式响应、上行用 POST。
- 多条通道并存时用每帧序号保证顺序，第一期就实现。
- 第一期加入进程内客户端（Go 自绘界面使用同一批服务），并补写 Go 自绘界面"后台数据与线程"的文档。

**以下是我的判断（未经讨论，请重点看）**

1. 不兼容 v1 协议，版本不匹配直接报错。
2. 兜底传输把字节帧编成 base64 放在 JSON 的 `bin` 字段里。
3. 默认窗口 1 MiB、每帧固定开销 64 B、单帧上限 16 MiB、消息不拆分。
4. `Close()` 的语义从"结束发送"改为"优雅全关闭"；新增 `CloseSend()` 表示半关闭。
5. 只发处理函数收到数据时静默丢弃，打开方通过 `accept.in=false` 得知。
6. JS 侧三种用法都做（用户的要求）：异步迭代器、Web Streams（`readable`/`writable`）、`tango.socket()`。我的判断：`readable` 与迭代器共用一个队列、互斥使用；`socket` 的字符串按 JSON 字符串值发送。
7. 快速通道只承载字节帧；丢帧不重传，直接让流以 `transport` 错误失败。
8. 各通道的 `BinaryMinSize`、环大小、POST 攒包时间等数值是初始值，以基准结果为准调整。
9. 每帧的计费大小由发送方写在 `c` 字段里，接收方照单归还（避免两端 JSON 编码长度对不上导致卡死）。
10. 通道状态（世代号、令牌、就绪/停用）归核心 `*tango.Surface` 所有；Router 只管协议，平台后端只提供原始能力。
11. `OnBinary` 和 `OnMessage` 一样由应用自己接；没有注册时不开快速通道。
12. 从开发服务器加载的页面不开快速通道，全部走兜底传输。
13. Go 侧加 `ReadWriter()` 的 `io` 适配；它的 `Write` 可以把超大写入拆成多帧，`Close` 是半关闭。
14. 进程内客户端除了 `Open`（流）也提供 `Call`（方法调用），这样一个服务的全部能力 Go 界面都能用；JSON 帧照样编解码，保证与 JS 客户端行为一致。
15. 进程内流不属于任何页面，不受页面重载 / `Drop` 影响，处理函数里 `ConnFrom` 返回 `ok=false`。

## 8. 留给实现者的事

- 文件拆分和内部数据结构（重排缓冲、信用计数器、JS 侧发送队列）。
- 门铃合并、空闲检测的具体做法。
- 基准程序的形式（`examples/` 下的程序或 `automation` 驱动）。
- darwin/iOS 协议处理器异步化的具体做法。
- 测试的组织方式。

## 9. 示例与基准程序

新增 `examples/duplex`，一个程序同时充当示例和 §5.8 / §6.3 的基准：

- **回显**：JS 发 JSON 和字节，Go 原样回写，演示 `HandleDuplex` + `Receive`/`Write*`。
- **小终端**：Go 侧用 `ReadWriter()` 接本地 shell 的 pty（桌面平台），JS 侧用 `readable`/`writable` 接一个最简终端视图，演示 `io` 适配和 Web Streams。
- **文件上传**：`file.stream().pipeTo(s.writable)`，Go 侧边收边写临时文件并回报进度。
- **基准模式**：命令行参数切换，输出 64 B 回显延迟 p50/p99 和 64 KiB 单向吞吐，并标明当前用的是哪条通道。
- **混合界面**：同一个窗口里，WebView 页面和 Go 自绘面板同时接同一个回显/终端服务（Go 面板用 `r.Open`），演示 §5.9 和 §5.10。

## 10. 需要更新的文档

- `ipc/wire.go` 包注释（线协议）
- `docs/architecture.md` 的桥接层与线协议部分
- `site/guide/concepts/ipc.md`、`site/en/guide/concepts.md`
- `site/reference/js-api.md`、`site/reference/api/ipc.md`
- `runtime-js/README.md`
- `site/guide/ui.md`、`site/en/guide/ui.md`：新增"后台数据与线程"一节（§5.10）
- `CHANGELOG.md`（`DefaultWindow` 删除、`Close` 语义变更、协议 v2）
- 第二期：`docs/webview.md`、`docs/windows.md`、`docs/mobile.md`（顺便修正 `docs/mobile.md` 关于是否依赖 androidx.webkit 的前后矛盾）
