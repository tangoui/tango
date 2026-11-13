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
import { call, on, emit, stream, Window, Clipboard, TangoError } from "@tangoui/runtime";
```

## 基础：调用、事件、流

| 函数 | 说明 |
|---|---|
| `call<T>(method, ...args): Promise<T>` | 调用 Go 方法（`Router.Handle` 或 `bind.Register` 注册的） |
| `on(name, fn): () => void` | 订阅 Go 发来的事件，返回取消函数 |
| `emit(name, data?)` | 向 Go 发事件 |
| `stream<T>(method, ...args): TangoStream<T>` | 打开 Go 产生的流；`for await` 消费，`break` 或 `cancel()` 取消 |
| `onStream(name, fn)` | 处理 Go 用 `Router.OpenStream` 主动打开的流 |
| `onRaw(fn)` / `postRaw(msg)` | 非 IPC 的原始消息 |
| `TangoError` | 所有 reject 的错误类型，带 `code` |

协议、背压和错误码见 [IPC](/guide/concepts/ipc)。

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

## 平台差异

桌面三平台的后端都实现了这些接口，但只有 macOS 实机验证过。移动端（实验性）支持 `Window` 的基本项（标题、显示、关闭、可见性）、`App.info`/`quit`、`Clipboard` 文本、`Dialog.message` 与 `Dialog.open`；`Dialog.save`、窗口几何、菜单、托盘、通知、快捷键在移动端不可用（reject `unsupported`）。

每个函数在各平台的支持情况，见仓库里的完整对照表 [docs/runtime-api.md](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md)。

## 扩展

其他 Go 包可以用 `api.Handle(cap, "share.text", handler)` 注册 `tango:share.text`，自动套用同一套能力开关和错误码；JS 侧在运行时里加一个模块，用内部的 `invoke` / `listen` 调用。
