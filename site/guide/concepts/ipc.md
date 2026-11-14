# IPC：调用、事件、流

`ipc` 包在[原始消息](/guide/concepts/app-surface#原始消息)之上实现三种通信：**调用**（页面请求、Go 应答）、**事件**（双向、无应答）、**流**（任一端打开、全双工、JSON 与字节都能传，带背压）。页面侧是 `@tangoui/runtime`。

## 接线

```go
r := ipc.NewRouter()
app.AddBridgeScript(runtimejs.Script)   // 注入运行时：页面里出现 window.tango
app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
// 流的字节帧走快速通道（平台支持时）；不接这一行就全走消息通道，见下文「快速通道」
app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })
// 页面关闭时清理它的流和订阅
s.OnClosed(func() { r.Drop(s) })
```

`runtimejs.Script` 是打包好的 IIFE，注入后页面里直接用全局 `tango`；用打包工具的前端也可以 `npm install @tangoui/runtime`，`import { call, on, stream } from "@tangoui/runtime"`。两者是同一份代码。

`HandleMessage` 只处理带 `"tango"` 字段的 JSON 消息，其余原始消息原样留给你自己的 `OnMessage` 处理。

## 调用

最直接的写法是注册处理函数：

```go
r.Handle("Notes.Count", func(ctx context.Context, args json.RawMessage) (any, error) {
    return store.Count(), nil
})
```

更常用的是 **`bind`**：用反射把一个值的导出方法注册成 `名字.方法`：

```go
type Notes struct{ db *DB }

func (n *Notes) Get(ctx context.Context, id int) (*Note, error) { ... }
func (n *Notes) Search(q string, limit int) []Note             { ... }

bind.Register(r, "Notes", &Notes{db})
```

支持的方法形状：可选的第一个参数 `context.Context`，之后任意个能 JSON 解码的参数（允许变参），返回值是无、`T`、`error` 或 `(T, error)`。页面按位置传参：

```js
const note = await tango.call("Notes.Get", 42);
const hits = await tango.call("Notes.Search", "tango", 20);
```

处理函数里 `ipc.ConnFrom(ctx)` 能拿到发起调用的页面（`*tango.Surface` 实现了 `ipc.Conn`）。

### 错误

Go 返回的错误在页面里是被拒绝的 promise，值是 `TangoError`，带 `code`：

```go
return nil, ipc.NewError(ipc.CodeBadArgs, "limit 必须为正数")
```

```js
try { await tango.call("Notes.Search", "x", -1); }
catch (e) { if (e instanceof tango.TangoError && e.code === "bad_args") { ... } }
```

| code | 含义 |
|---|---|
| `not_found` | 方法不存在 |
| `bad_args` | 参数解码失败或取值不对 |
| `internal` | 其他 Go 错误（普通 `error` 都归到这里） |
| `canceled` | 调用或流被取消 |
| `transport` | 流的帧在快速通道上丢了（通道中途坏掉），见[快速通道](#快速通道) |

`jsapi` 另外定义了 `forbidden`、`unsupported`、`not_running`，见 [安全模型](/guide/concepts/security)。

## 事件

```go
// 页面 → Go
r.On("draft-changed", func(c ipc.Conn, data json.RawMessage) { ... })
// Go → 某个页面
r.Emit(s, "sync-progress", map[string]any{"pct": 42})
```

```js
tango.emit("draft-changed", { id: 1, text });
const off = tango.on("sync-progress", ({ pct }) => bar.value = pct);
off(); // 取消订阅
```

`tango.Events` 模块提供同一通道上的 `on` / `once` / `next(name)`（等下一次事件的 promise）。

## 流

流是一条有序的消息通道，每条消息（帧）是一个 JSON 值或一段字节。最简单的用法是日志跟随、进度、搜索结果分批这类「一个请求、很多结果」的场景，页面打开、Go 只发：

```go
r.HandleStream("Logs.Tail", func(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
    for line := range follow(ctx) {
        if err := w.Write(line); err != nil {
            return err // 页面取消或断开
        }
    }
    return nil // 正常结束
})
```

```js
for await (const line of tango.stream("Logs.Tail")) {
  append(line);
  if (enough) break;          // 跳出循环 = 在 Go 侧取消，w.Write 返回错误、ctx 被取消
}
```

**背压**按字节计：每条流每个方向一个信用窗口（默认 1 MiB，`ipc.DefaultWindowBytes`），每帧计负载字节数加 64（`ipc.FrameOverhead`）。页面读走数据后才归还信用，页面处理慢时 `w.Write` 阻塞，生产者自然放慢，消息不会在内存里堆积。在途为 0 时单帧可以超出窗口；单帧负载上限 16 MiB（`ipc.MaxFrameBytes`），超过时这次写入返回 `bad_args`，流不受影响。Go 的接收窗口用 `r.SetStreamWindow(n)` 调整（不小于 4096），在握手时告诉页面。

`HandleStream` 的处理函数只发不收（页面向它 `send` 会失败，`bad_args`），`w` 只有 `Write`（JSON）。要发字节就用下面的 `HandleDuplex`；从页面打开的流里，`w` 的具体类型是 `*ipc.Stream`，也可以断言出来调 `WriteBytes`（`examples/duplex` 的 `Source` 就这么做）。

### 双向流

`HandleDuplex` 注册双向处理函数，拿到的是流本身：`Receive*` 读、`Write*` 写，读写可以在两个 goroutine 里并发。返回 nil 时优雅关闭整条流，返回错误时流失败：

```go
// examples/duplex：原样回写每一帧，JSON 或字节
r.HandleDuplex("Echo", func(s *ipc.Stream, _ json.RawMessage) error {
    for {
        m, err := s.Receive()
        if errors.Is(err, io.EOF) {
            return nil // 页面 end 了它的方向
        }
        if err != nil {
            return err // 取消、出错或页面断开
        }
        if m.Binary {
            err = s.WriteBytes(m.Bytes)
        } else {
            err = s.Write(m.JSON)
        }
        if err != nil {
            return err
        }
    }
})
```

```js
const s = tango.stream("Echo");
await s.send({ hello: 1 });                  // JSON 帧
await s.sendBytes(new Uint8Array([1, 2, 3])); // 字节帧
s.end();                                     // 半关闭：不再发，还能收
for await (const v of s) console.log(v);     // JSON 帧给解析后的值，字节帧给 Uint8Array
```

| Go（`*ipc.Stream`） | 说明 |
|---|---|
| `Write(v)` / `WriteBytes(p)` | 发一帧 JSON / 字节；没有信用时阻塞；`WriteBytes` 返回后 `p` 可以复用 |
| `Receive()` | 下一帧（`ipc.Message`：`Binary`、`Bytes`、`JSON`）；对方 `end` 后返回 `io.EOF` |
| `ReceiveBytes()` / `ReceiveJSON(&v)` | 要求下一帧是字节 / JSON，否则返回 `bad_args`，这一帧留在队首 |
| `CloseSend()` | 半关闭（`end`）：本方不再发，继续收 |
| `Close()` | 优雅全关闭（`close`）：本方不再发也不再收，未读的数据丢弃；对方之后的写入返回 "stream closed by peer" |
| `CloseWithError(err)` | 让整条流失败：对方先收完本方之前发出的帧，再看到 `err`；本方自己之后的 `Receive*` / `Write*` 返回 `canceled`（"stream closed"） |
| `Context()` | 流完全结束（两个方向都结束，或被取消、失败）或连接断开时取消 |
| `ReadWriter()` | `io.ReadWriteCloser` 视图，见下 |

一帧就是一条消息，接收方拿到的边界和发送方一致。帧按序号交付；对方失败（`CloseWithError` 或处理函数返回错误）时，`Receive*` 先交付它失败前发出的帧，再返回它的错误，本方 `Write*` 立即返回这个错误；`Receive*` 在流被取消或连接断开时返回 `context.Cause(s.Context())`。

**连续字节流**：`s.ReadWriter()` 把字节帧当成 `io.ReadWriteCloser`：`Read` 看不到帧边界，`Write` 每次调用发一帧（超过 `MaxFrameBytes` 自动切开；空的 `Write` 不发帧），`Close` 等于 `CloseSend`，这样 `io.Copy` 结束后对方读到 EOF，本方还能继续读。队首是 JSON 帧时 `Read` 返回 `bad_args`，要先用 `Receive` 取走。`examples/duplex/term.go` 用它把一个本地 shell 接到流上：

```go
rw := s.ReadWriter()
// ...
go func() {
    io.WriteString(stdin, shellInit)
    io.Copy(stdin, rw)
    stdin.Close()
}()
```

页面侧可以用 Web Streams 接同一条流：`s.readable`（读，按读取进度归还信用）和 `s.writable`（写，`write()` 拿到信用才 resolve，所以 `pipeTo` 自动节流）。上传文件就是 `file.stream().pipeTo(s.writable)`。JS 侧的完整 API（`accepted`、`bufferedAmount`、`writable.abort`、WebSocket 风格的 `tango.socket`）见 [前端运行时 API](/reference/js-api#流)。

### Go 打开的流

Go 也可以主动向页面开流：`r.OpenStream(s, "Import.Progress", args...)` 返回 `*ipc.Stream`；页面用 `tango.onStream("Import.Progress", (stream, ...args) => { ... })` 接收，可以 `for await` 读，也可以 `stream.send(...)` 回写，Go 侧用 `Receive*` 读。用完一定要 `Close`（或 `CloseWithError`），否则页面那一侧一直等着。

### 进程内调用

同一进程里的 Go 代码（例如[自绘界面](/guide/ui#后台数据与线程)）可以直接用已经注册给页面的服务，不经过 JSON 消息和 base64：

```go
res, err := r.Call(ctx, "Notes.Count")   // Handle / bind 注册的方法；结果是 JSON
s, err := r.Open(ctx, "Echo")            // HandleStream / HandleDuplex 注册的流，拿到客户端一侧
defer s.Close()
```

语义与页面完全相同：同样的 `accept`、信用窗口、关闭和错误码，处理函数分不出对面是谁，只是 `ipc.ConnFrom(ctx)` 返回 `ok=false`。`Call` 的处理函数 ctx 会随调用方 `ctx` 取消（截止时间到了也一样），但不带它的值，`Deadline()` 也是空的。`Open` 打开的流不属于任何页面，`Drop` 管不到：用完必须 `Close` 或取消 `ctx`，否则处理函数的 goroutine 一直不退出。`Call`、`Open`、`Receive*`、`Write*` 都会阻塞，不要在 UI 线程上调用。

## 快速通道

默认情况下所有 IPC 都走原始消息通道：每帧是一条 JSON 字符串，字节帧编成 base64。**快速通道**是挨着消息通道的二进制旁路，只承载流的**字节帧**，省掉 JSON 和 base64；JSON 帧、`open`/`accept`/`ack`/`end` 等控制消息和握手仍然走消息通道。两个方向分别协商，同一条流的帧可以一部分走快速通道、一部分走消息通道，接收方按序号重排，所以语义完全不变：应用代码除了接线，什么都不用改。

快速通道按名字区分：`down:scheme` / `up:post`（资源来源上的 `/__tango/lane/down` 流式响应和 `/__tango/lane/up` POST），以及平台各自的 `down:android` / `up:android`、`down:shm` / `up:shm`。`down:scheme` 只接 4 KiB 及以上的帧，`up:post` 只接 16 KiB 到约 1 MiB 的帧，更小的帧照旧走消息通道（延迟更低）。

### 什么时候开启

页面每次握手时，下面四个条件**都满足**才会拿到快速通道，否则一切照旧：

1. 应用用 `app.OnBinary` 注册了至少一个处理函数（上面「接线」里那一行，把包交给 `r.HandleBinarySession(s, gen, p)`）。没人接收上行包，核心就不开通道。
2. 页面来自应用的资源来源（macOS/iOS/Linux 是 `tango://app`，Windows/Android 是 `https://tango.localhost`）。**直接从开发服务器加载的页面**（`SurfaceOptions.URL` 指向 `http://localhost:5173` 之类）永远拿不到快速通道，全部走消息通道；`tan dev` 通过 `asset.FromEnv` 反向代理开发服务器，页面仍在资源来源上，不受影响。
3. 平台后端报告了支持的通道（页面的 CSP 还得允许 `connect-src` 到资源来源，见下文「macOS / iOS」）。
4. 没有被关掉：`tango.Options{DisableFastLanes: true}`，或环境变量 `TANGO_IPC_LANES=0`（任何构建都生效，排查问题用）。

::: warning 目前只有 macOS 默认开启
公共设施（协议、核心、`@tangoui/runtime` 的通道模块）已经就绪。**macOS 默认报告 `up:post`**（上行字节帧走快速通道）；iOS 和 Android 可以用环境变量试用，默认关闭（见下文「macOS / iOS」「Android」）；Windows、Linux 仍然全部走消息通道，通过基准对比后才会逐个默认开启。现在就把 `OnBinary` 接上，到时候无需改代码。
:::

`OnBinary` 的处理函数在收到包的 goroutine 上调用，不在主线程；包只在调用期间有效。`r.HandleBinarySession` 满足这两点。`gen` 是包通过校验时所属的页面世代，原样交给 `HandleBinarySession`，旧页面的包就不会进新页面的流。不要用 `r.HandleBinary`：它只给不复用页面的简单连接用，对 `*tango.Surface` 会丢掉所有包（并记一条警告）。一个 surface 只接一个 `Router`：每次握手都会换通道世代，两个 Router 会互相作废，不支持。

快速通道的投递绑定页面会话（世代）：通道令牌每个世代一个，这个世代里的每个通道请求都带它，页面重新加载或离开资源来源后旧令牌失效。`up:post` 每个请求最多等 4 秒（比 5 秒的缺口超时短：卡住的请求先让通道失败、之后的帧改走消息通道，而不是先让流失败）；排队和在途的 POST 数据合计最多 8 MiB，超出的帧直接走消息通道，序号和信用都不变。通道请求只相对页面的真实地址发出（不受 HTML `<base>` 影响），并拒绝重定向。

页面导航时核心会立即暂停下行通道，页面离开资源来源时整个通道作废，新页面握手后重新协商；流的打开方在收到对方 `accept` 之前不用快速通道。这些都由核心和运行时处理，应用不用管。

### macOS / iOS

- **只提供 `up:post`**（上行字节帧经 `/__tango/lane/up` 的 POST 交给 Go）。**下行仍走消息通道**：WKWebView 对 `tango://` 流式响应的小块逐块交付，但会扣住大块突发的一部分，直到同一响应上有新数据才交出；突发之后没有新数据时，被扣住的帧迟迟不到，流会以 `transport` 失败。所以不报告 `down:scheme`。
- **macOS 默认开启 `up:post`，iOS 默认关闭**（iOS 只在模拟器上测过，等真机数据）。环境变量 `TANGO_IPC_LANES_DARWIN` 控制两个平台（每个窗口/页面创建时读一次；去掉首尾空白、不分大小写；关闭值在任何构建都生效）：

  | 取值 | macOS | iOS |
  |---|---|---|
  | 未设置、空 | `up:post`（默认开启） | 关闭（默认） |
  | `0`、`false`、`off` | 关闭 | 关闭 |
  | `all` | 只在开发构建（`-tags tango_dev`）生效：`down:scheme` + `up:post`，**仅供实验**，大块下行会被扣住，流可能以 `transport` 失败；生产构建等同默认 | `down:scheme` + `up:post`，**仅供实验**（任何构建都生效） |
  | 其他值（通常是 `1`） | `up:post` | `up:post` |

  `TANGO_IPC_LANES=0` 或 `DisableFastLanes` 始终优先关闭。iOS 真机上没法从外部传环境变量，要试用就在 Go 里、创建第一个窗口/页面之前调用 `os.Setenv("TANGO_IPC_LANES_DARWIN", "1")`。开启时（macOS 默认就是）`tango.lanes()` 返回 `{down: "fallback", up: "up:post"}`。macOS 上传 64 MiB 的吞吐约为消息通道的 2.73 倍，见 [docs/ipc-bench.md](https://github.com/tangoui/tango/blob/main/docs/ipc-bench.md)。
- **CSP**：`up:post` 启动时先向 `/__tango/lane/up` POST 一个空包，收到 204 才算就绪。页面的 CSP 若不允许 `connect-src` 到资源来源（比如 `connect-src https://api.example.com`，或者不含 `'self'` 的 `default-src`），探测被拒，这个页面就不开快速通道，字节帧照常走消息通道，不会报错。想用快速通道，就在 `connect-src` 里加上 `'self'`。
- 已知限制：
  - iframe 拿不到通道令牌。协议处理器看不到请求来自哪个 frame，安全性靠"令牌只经主框架的消息桥下发"；macOS 实测不透明来源的 frame（沙箱 `srcdoc`、`data:`）和同源 frame 发出的通道请求全部被拒（iOS 未单独测）。同源 frame 本来就能访问 `parent.tango`，那是应用自己的代码。
  - WebKit 不把 `tango://` 请求的 Blob 请求体交给处理器，所以 `up:post` 只用 ArrayBuffer。这条限制同样适用于应用自己的 fetch：iOS 有把 Blob/FormData 转成 ArrayBuffer 的注入脚本（见 [docs/mobile.md](https://github.com/tangoui/tango/blob/main/docs/mobile.md)），macOS 上请自己用 ArrayBuffer 或字符串作请求体（macOS 上 Blob 请求体没有单独测过，按 WebKit 的已知问题处理）。
  - WebKit 的 `tango://` 请求不带 `Origin`，Tango 用主文档的来源补上；所以应用页面里嵌的跨源 iframe 发往资源来源的请求能通过资源 CSRF 检查（原有行为；通道请求仍受每世代令牌保护）。不要在资源页面里嵌不可信的 frame。
  - iOS 只在模拟器上测过，未在真机上测；iOS 上"通道中途坏掉"的场景没有自动化测试（回退逻辑与平台无关，已在 macOS 上验证）。

### Android

- **提供 `down:android` 和 `up:android`**：两个方向的字节帧都以 `ArrayBuffer` 走页面已有的 `WebMessageListener` 通道，不做 JSON 和 base64。没有最小帧大小，所有字节帧都走通道。
- **默认关闭**（只在模拟器上测过）。条件：WebView 116 及以上（支持 `WebMessageListener` 的 ArrayBuffer 消息），且环境变量 `TANGO_IPC_LANES_ANDROID` 打开：每个页面创建时读一次，去掉首尾空白、不分大小写，未设置、空、`0`、`false`、`off` 为关闭，其他值（通常是 `1`）为开启。`TANGO_IPC_LANES=0` 或 `DisableFastLanes` 始终优先关闭。Android 应用没法从外部拿到环境变量：在 Go 里、创建第一个页面之前调用 `os.Setenv("TANGO_IPC_LANES_ANDROID", "1")`。开启后 `tango.lanes()` 返回 `{down: "down:android", up: "up:android"}`。
- 每个包都带页面 token 和通道世代，旧页面、旧世代的包两个方向都进不了新页面。Go 拒收的上行包会让页面立即停用 `up:android`，之后的帧改走消息通道。
- iframe：子框架的消息全部被丢弃（模拟器上实测：同源 iframe 用自己的 `__tangoAndroid` 伪造的包 Go 一个都收不到，不透明来源的 iframe 没有这个对象）。同源 iframe 能调用 `parent.__tangoNative`，那是应用代码。
- 已知限制：只在模拟器上测过，没有真机数据；每个包跨进程拷贝一次、JNI 拷贝一次。详见 [docs/mobile.md](https://github.com/tangoui/tango/blob/main/docs/mobile.md)。

为了接入通道，macOS/iOS 的 `tango://` 协议处理器改成了异步：应用的资源处理函数（`Options.Assets`）每个请求在自己的 goroutine 里运行，不在主线程（所以请求可能并发处理），大文件、SSE 等大的或长时间不结束的响应不再卡住界面；WebKit 取消请求时 `r.Context()` 被取消。这对所有应用都生效，与通道开关无关。

### 看当前用的是哪条通道

```js
tango.lanes(); // macOS 默认：{down: "fallback", up: "up:post"}；没有快速通道的方向是 "fallback"
```

`examples/duplex` 的基准结果里 `lane` 字段就是它的值。

### `transport` 错误

快速通道是本机进程间通道，正常不会丢帧；只有通道本身中途坏掉（页面拆除、请求被中断）时才会丢。通道一出错，这个方向立即停用，之后的帧全部走消息通道，其他流不受影响。已经在坏掉的通道上发出、没送到的帧不会重传：接收方发现某条流缺了一帧（后面的帧或结束消息已经到了，或者缺的正好是最后一帧：每个通道帧发出后，发送方都在消息通道上补一条 `boundary` 告诉对方"到这一帧为止都已发出"，所以流还开着、之后没有数据也能发现），而缺的这帧连续 5 秒（Go 的 `ipc.GapTimeout`、JS 的 `GAP_TIMEOUT_MS`）都没到，就让这条流失败，错误码 `transport`（Go 侧是 `ipc.CodeTransport`），两端都会看到。只有确实缺帧的流会失败，同一页面的其他流不受牵连。页面主线程卡住（`alert()`、长任务）时的保证：运行时在通道帧真正发出（`up:post`：所在批次交给 `fetch`）时才发它的 `boundary`，同一条流之后经消息通道发的数据、`end`、`close` 也等这些通道帧发出后才发，所以 Go 不会因为还排在页面里的帧开始计时；页面里的缺口计时器和 `up:post` 的请求计时器晚触发超过 1 秒时，各用完整超时再等一次，第二次到期照常失败。仍可能出现的情况：中止路径（writable 的 abort、被拒的写入、`cancel()`、本地出错）立即发 `close`/`error`，卡顿超过 5 秒时 Go 可能报 `transport` 而不是中止原因；卡顿长到连重新计时也到期，照常失败。遇到它可以像对待断开的网络连接一样重开这条流。

### 故障注入（仅开发构建）

开发构建（`-tags tango_dev`，`tan dev` 和 `automation.Launch` 自动加）和自测构建（`-tags tango_selftest`，`tan build` 会保留它，iOS 模拟器自测用）里，环境变量 `TANGO_IPC_LANE_FAULT=<通道名>:<N>` 让该通道在每个页面世代里成功发送（下行）或接收（上行）N 个数据包之后进入故障（空包不计，比如 `up:post` 启动时的探测）：之后的包被拒绝或丢弃，按「通道坏掉」处理。例如 `TANGO_IPC_LANE_FAULT=down:scheme:3`。格式不对时记一条警告并忽略；生产构建不读这个变量。用来验证应用在通道中途坏掉时的表现。

## TypeScript 绑定生成

```bash
tan generate bindings          # 默认写到 <frontend.dir>/src/bindings
```

`tan generate bindings` 用 `go/types` 读主包，找到所有 `bind.Register(r, "常量名", svc)`，为每个服务生成调用 `call("Name.Method", ...)` 的 TS 函数，Go 结构体按 `encoding/json` 的规则映射成 interface（json 标签、`omitempty` → 可选、指针 → `T | null`、`time.Time` → `string` 等）。参数：`-pkg` 指定 Go 包目录，`-o` 指定输出目录，`-runtime` 指定导出 `call` 的模块（默认 `@tangoui/runtime`）。

## 线上协议

一条 JSON 对象一条消息，`"tango"` 字段区分种类；流 id 由发起方分配（JS 用奇数，Go 用偶数）。当前是协议 v2，运行时启动时握手，`proto` 不是 2 的旧运行时会被拒绝（`runtime-js protocol mismatch`），所以 `@tangoui/runtime` 要和 Go 模块同版本：

```jsonc
{"tango":"hello","proto":2}
{"tango":"hello","proto":2,"win":1048576}
{"tango":"call","id":1,"method":"Notes.Get","args":[42]}
{"tango":"ret","id":1,"result":{"id":42,"title":"..."}}
{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
{"tango":"event","name":"sync-progress","data":{"pct":42}}
{"tango":"stream","id":3,"op":"open","method":"Term","args":[]}
{"tango":"stream","id":3,"op":"accept","in":true}
{"tango":"stream","id":3,"op":"data","seq":0,"c":75,"data":{"cols":80}}
{"tango":"stream","id":3,"op":"data","seq":1,"c":69,"bin":"aGVsbG8="}
{"tango":"stream","id":3,"op":"ack","b":65536}
{"tango":"stream","id":3,"op":"end","seq":2}
{"tango":"stream","id":3,"op":"close","seq":2}
{"tango":"stream","id":3,"op":"error","seq":2,"error":{"message":"...","code":"internal"}}
{"tango":"stream","id":3,"op":"cancel"}
```

`data` 帧二选一带 `data`（JSON 负载）或 `bin`（字节负载，标准 base64 带填充；`"bin":""` 是空字节帧）；`seq` 是这个方向的帧序号，`c` 是发送方算好的计费大小，`ack` 的 `b` 是归还的信用字节数。完整规则（握手、`accept` 的 `in`、`end`/`close`/`error` 的 `seq`、归还时机）写在 [`ipc` 包文档](/reference/api/ipc)的包注释里。协议很小，需要时可以在别的语言或测试里直接实现。

有快速通道时，Go 的 `hello` 回复多一个 `lanes` 字段（`{"gen":1,"tok":"…","list":["down:scheme","up:post"]}`），快速通道上的二进制包格式也在同一份包注释里。走快速通道的帧另有一条 `{"tango":"stream","id":3,"op":"boundary","seq":2}`（见上面的 `transport` 错误）。
