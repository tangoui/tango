# 前端运行时 API

页面侧的 JS/TS 运行时是 npm 包 **`@tangoui/runtime`**。两种用法是同一份代码：

```go
// Go：注入 IIFE，页面里出现全局 window.tango（不需要打包工具）
app.AddBridgeScript(runtimejs.Script)
```

```bash
# 用打包工具的前端：安装 npm 包获得类型和 ESM 入口
npm install @tangoui/runtime
```

```ts
import { call, on, emit, stream, socket, Window, Clipboard, TangoError } from "@tangoui/runtime";
```

## 基础：调用、事件、流

| 函数 | 说明 |
|---|---|
| `call<T>(method, ...args): Promise<T>` | 调用 Go 方法（`Router.Handle` 或 `bind.Register` 注册的） |
| `on(name, fn): () => void` | 订阅 Go 发来的事件，返回取消函数 |
| `emit(name, data?)` | 向 Go 发事件 |
| `stream<T>(method, ...args): TangoStream<T>` | 打开流（Go 侧 `Router.HandleDuplex` 或只发的 `Router.HandleStream`）；见下面的[流](#流) |
| `socket(method, ...args): TangoSocket` | 同一种流的 WebSocket 风格包装，见 [TangoSocket](#tangosocket) |
| `onStream(name, fn)` | 处理 Go 用 `Router.OpenStream` 主动打开的流，`fn(stream, ...args)` 拿到的也是 `TangoStream` |
| `onRaw(fn)` / `postRaw(msg)` | 非 IPC 的原始消息 |
| `lanes(): { down, up }` | 当前用的[快速通道](/guide/concepts/ipc#快速通道)：每个方向是通道名（如 `"down:scheme"`、`"up:post"`），没有快速通道的方向是 `"fallback"`（字节帧走消息通道）。只用于诊断，值会随通道就绪、坏掉和页面重新握手而变化 |
| `TangoError` | 所有 reject 的错误类型，带 `code` |

协议、Go 侧 API 和错误码见 [IPC](/guide/concepts/ipc)。

## 流

`TangoStream` 是全双工的：JSON 帧和字节帧两个方向都能传，按字节计的信用窗口提供背压。

| 成员 | 说明 |
|---|---|
| `id` | 流 id（JS 打开的是奇数） |
| `accepted: Promise<{ in }>` | Go 接受这条流时 resolve；`in` 为 `false` 表示对面只发不收（`HandleStream`）。流失败（如 `not_found`）或先被取消时 reject |
| `send(v): Promise<void>` | 发一个 JSON 帧（调用时就编码；`undefined` 按 `null` 发） |
| `sendBytes(b): Promise<void>` | 发一个字节帧（`Uint8Array`、`ArrayBuffer` 或其他 `ArrayBufferView`，调用时拷贝） |
| `end()` | 半关闭：已排队的帧发完后结束发送方向，之后的 `send` reject，仍可继续读 |
| `cancel()` | 立即放弃整条流，Go 侧处理函数的 ctx 被取消；跳出 `for await` 也是 `cancel` |
| `bufferedAmount` | 已排队但还没写出的计费字节数（每帧负载 + `FRAME_OVERHEAD`） |
| `readable` | 读方向的 `ReadableStream`，读走一帧才归还信用；`readable.cancel()` 等于 `cancel()` |
| `writable` | 写方向的 `WritableStream`，见下 |
| `for await (const v of s)` | JSON 帧产出解析后的值，字节帧产出 `Uint8Array`；Go 正常结束后循环结束，Go 出错时抛出 `TangoError` |

- `send`/`sendBytes` 按调用顺序排队，帧写进传输时 resolve，Go 的窗口满了就等待。负载超过 `MAX_FRAME_BYTES` 时 reject `bad_args`，流不受影响；流不能再发时（`end()` 之后、Go 不收数据、Go 已关闭、出错或取消）这一帧和后面排队的帧都 reject。不 `await` 也可以，帧照样按序发出，被 reject 的 Promise 不会变成未处理的 rejection。
- `readable` 和异步迭代器读的是同一个队列，只能用其中一个。**访问一次 `s.readable` 就锁定到它**，之后再迭代会抛 `TangoError("stream already consumed", "bad_args")`，反之亦然。
- `writable.write(chunk)`：`Uint8Array`/`ArrayBuffer`/`ArrayBufferView` 按字节帧发，其他值按 JSON 帧发；拿到信用才 resolve，所以 `pipeTo` 跟着 Go 的读取速度走。`close()` 发完排队的帧再结束发送方向；`abort(reason)` 让整条流失败，Go 收到 `code` 为 `canceled`、消息为 `String(reason?.message ?? reason)` 的错误（不带参数时消息是 `"undefined"`）。某次写入被 `bad_args` 拒绝（例如 `BigInt`、超过 `MAX_FRAME_BYTES`）时整条流失败。

```ts
// 上传：边读文件边发，按 Go 的速度节流
const up = stream("Upload", file.name, file.size);
await file.stream().pipeTo(up.writable);

// 终端：字节流解码成文本
const term = stream("Term");
const reader = term.readable.pipeThrough(new TextDecoderStream()).getReader();
const writer = term.writable.getWriter();
await writer.write(new TextEncoder().encode("ls\n"));
```

常量（与 Go 的 `ipc` 包同值）：`PROTO_VERSION`（2，线协议版本）、`DEFAULT_WINDOW_BYTES`（1048576，默认接收窗口）、`FRAME_OVERHEAD`（64，每帧额外计费字节）、`MAX_FRAME_BYTES`（16777216，单帧负载上限）、`GAP_TIMEOUT_MS`（5000，流缺一帧最多等多久，超时以 `transport` 失败，见下面的错误码）。

### TangoSocket

`socket(method, ...args)` 在同一种流上包一层 WebSocket 风格的 `EventTarget`，方便沿用 WebSocket 写法的代码：`readyState`（`CONNECTING` → Go 接受后 `OPEN` → `CLOSING` → `CLOSED`）、`binaryType`（`"arraybuffer"` 默认，或 `"blob"`）、`bufferedAmount`、`url`（`"tango:" + method`）、`send`、`close`，以及 `onopen`/`onmessage`/`onerror`/`onclose` 和 `addEventListener`。

```ts
const ws = socket("Echo");
ws.onmessage = (e) => console.log(e.data); // 字符串；字节帧是 ArrayBuffer 或 Blob
ws.send("hello");                          // CONNECTING 时也可以发，会排队
ws.close();                                // 排队的 send 写完后发 close，随即触发 close 事件
```

- `send(data)`：字符串作为 JSON 字符串值发送，`ArrayBuffer`/`ArrayBufferView`/`Blob` 作为字节帧。`message` 的 `data`：JSON 字符串值给字符串本身，其他 JSON 值给 `JSON.stringify` 的文本，字节帧按 `binaryType` 给 `ArrayBuffer` 或 `Blob`。
- 正常结束：`close` 事件，code 1000，`wasClean: true`。Go 返回错误：先 `error` 事件，再 `close`，code 1011、`reason` 是错误消息、`wasClean: false`。Go 取消、客户端失败（如协议版本不匹配）、某次 `send` 被 `bad_args` 拒绝（超过 `MAX_FRAME_BYTES`，或 Go 侧是只发的 `HandleStream`）：`close`，code 1006。

与 `WebSocket` 的差异：

- `CONNECTING` 时 `send()` 排队，不抛 `InvalidStateError`。
- `close(code, reason)` 不把 code 和 reason 发给 Go。它等排队的 `send` 都写出后向 Go 发 `close`（相当于 Go 的 `Stream.Close`：Go 之后的写入返回 "stream closed by peer"），随即触发 `close` 事件（1000，`wasClean: true`），不等 Go 回应；`CONNECTING` 时和对只发的 `HandleStream` 也一样。`close()` 之后到达的消息丢弃。
- 构造函数是内部的，只能用 `socket()` 创建。
- 没有真正的背压（和 WebSocket 一样只有 `bufferedAmount`）；需要背压的新代码用 `TangoStream` 或它的 Web Streams 视图。

## 系统能力模块

这些模块对应 Go 侧的 `jsapi` 包，需要 Go 侧先 `jsapi.Install(app, r, opts)`。方法名在线上是 `tango:<模块>.<函数>`，不会和应用自己的方法撞名。

| 模块 | 需要的能力（`jsapi.Cap`） | 主要函数 |
|---|---|---|
| `Window` | `Window`（默认开） | `setTitle`、`size`/`setSize`、`bounds`/`setBounds`、`center`、`state`/`setState`、`minimize`/`maximize`/`fullscreen`/`restore`、`setAlwaysOnTop`、`setZoom`、`reload`、`print`、`openDevTools`；事件 `onResize`、`onMove`、`onStateChange`、`onFocus`/`onBlur`、`onFilesDropped` |
| `App` | `Info`（默认开）；控制类需 `AppControl` | `info()`；`onOpenURL`、`onOpenFiles`、`onReopen`、`onSecondInstance`；`quit`、`hide`/`show`、`setBadge`、`setProgress`、`requestAttention` |
| `Theme` | `Info`（`vars`/`onVarsChange` 不需要） | `get()` → `{dark, accent}`、`onChange`；`vars()` 读取 `uivars.SyncPage` 注入的设计令牌 CSS 变量，`onVarsChange(fn)` 在变量变化时回调 |
| `Screens` | `Info` | `all`、`primary`、`current`、`onChange` |
| `Clipboard` | `ClipboardRead` / `ClipboardWrite` | `readText`/`writeText`、`readImage`/`writeImage`（PNG base64）、`readFiles`/`writeFiles` |
| `Dialog` | `Dialogs` | `message`、`open`、`save` |
| `Notify` | `Notify` | `requestPermission`、`send`、`onClick` |
| `Shell` | `OpenURL` / `OpenPath` | `openURL`（协议白名单）、`openPath`、`revealPath` |
| `Menu` | `Menu`（默认开） | `popup(items, {x, y})`、`bindContextMenu(target, build)`；只允许编辑类 role |
| `Hotkey` | `Hotkeys` | `register(accelerator, fn)` → 注销函数；归注册它的页面所有 |
| `Events` | 总是可用 | `on`、`once`、`next`、`emit`（应用自己的事件） |

```ts
await Window.setTitle("Notes");
const off = Window.onResize(({ width, height }) => layout(width, height));
try {
  await Clipboard.writeText("hi");
} catch (e) {
  if (e instanceof TangoError && e.code === "forbidden") {
    // Go 侧没有开启 ClipboardWrite
  }
}
```

## 错误码

| code | 含义 |
|---|---|
| `forbidden` | 能力未开启；URL 协议不在白名单；菜单用了不允许的 role |
| `unsupported` | 当前平台不支持 |
| `canceled` | 用户取消对话框 |
| `bad_args` | 参数类型或取值不对 |
| `not_found` | 方法不存在；或调用方不是本应用的窗口 |
| `not_running` | 应用正在退出 |
| `internal` | 其他 Go 侧错误 |
| `transport` | 流的帧在[快速通道](/guide/concepts/ipc#快速通道)上丢了：某帧缺了 `GAP_TIMEOUT_MS` 仍没到（通道中途坏掉）。像断线一样重开这条流 |

## 平台差异

桌面三平台的后端都实现了这些接口，但只有 macOS 实机验证过。移动端（实验性）支持 `Window` 的基本项（标题、显示、关闭、可见性）、`App.info`/`quit`、`Clipboard` 文本、`Dialog.message` 与 `Dialog.open`；`Dialog.save`、窗口几何、菜单、托盘、通知、快捷键在移动端不可用（reject `unsupported`）。

每个函数在各平台的支持情况，见仓库里的完整对照表 [docs/runtime-api.md](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md)。

## 扩展

其他 Go 包可以用 `api.Handle(cap, "share.text", handler)` 注册 `tango:share.text`，自动套用同一套能力开关和错误码；JS 侧在运行时里加一个模块，用内部的 `invoke` / `listen` 调用。
