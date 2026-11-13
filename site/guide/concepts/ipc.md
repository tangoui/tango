# IPC：调用、事件、流

`ipc` 包在[原始消息](/guide/concepts/app-surface#原始消息)之上实现三种通信：**调用**（页面请求、Go 应答）、**事件**（双向、无应答）、**流**（Go 产生、页面消费，带背压）。页面侧是 `@tangoui/runtime`。

## 接线

```go
r := ipc.NewRouter()
app.AddBridgeScript(runtimejs.Script)   // 注入运行时：页面里出现 window.tango
app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
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

流适合日志跟随、进度、搜索结果分批这类「一个请求、很多结果」的场景。页面打开、Go 生产：

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

**背压**：发送方最多有 64 条未确认的消息（`ipc.DefaultWindow`），页面每消费 16 条确认一次。页面处理慢时 `w.Write` 会阻塞，生产者自然放慢，消息不会在内存里堆积。

Go 也可以主动向页面开流：`r.OpenStream(s, "Import.Progress", args...)` 返回 `*ipc.Stream`，写完 `Close`；页面用 `tango.onStream("Import.Progress", (stream, ...args) => { for await (...) })` 接收。

## TypeScript 绑定生成

```bash
tan generate bindings          # 默认写到 <frontend.dir>/src/bindings
```

`tan generate bindings` 用 `go/types` 读主包，找到所有 `bind.Register(r, "常量名", svc)`，为每个服务生成调用 `call("Name.Method", ...)` 的 TS 函数，Go 结构体按 `encoding/json` 的规则映射成 interface（json 标签、`omitempty` → 可选、指针 → `T | null`、`time.Time` → `string` 等）。参数：`-pkg` 指定 Go 包目录，`-o` 指定输出目录，`-runtime` 指定导出 `call` 的模块（默认 `@tangoui/runtime`）。

## 线上协议

一条 JSON 对象一条消息，`"tango"` 字段区分种类；流 id 由发起方分配（JS 用奇数，Go 用偶数）：

```jsonc
{"tango":"call","id":1,"method":"Notes.Get","args":[42]}
{"tango":"ret","id":1,"result":{"id":42,"title":"..."}}
{"tango":"ret","id":1,"error":{"message":"boom","code":"internal"}}
{"tango":"event","name":"sync-progress","data":{"pct":42}}
{"tango":"stream","id":3,"op":"open","method":"Logs.Tail","args":[]}
{"tango":"stream","id":3,"op":"data","data":"line"}
{"tango":"stream","id":3,"op":"ack","n":16}
{"tango":"stream","id":3,"op":"end"}
{"tango":"stream","id":3,"op":"cancel"}
```

协议很小，需要时可以在别的语言或测试里直接实现。
