# 前端运行时 API

页面里的 JS/TS 直接调用 Tango 的系统能力：Go 侧是 `jsapi` 包，JS 侧是 `@tangoui/runtime` 的命名空间模块（注入的 IIFE 下是 `window.tango.Window` 等）。

```go
r := ipc.NewRouter()
app.AddBridgeScript(runtimejs.Script)
app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) }) // 流的快速通道，见 site/guide/concepts/ipc.md
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.ClipboardWrite | jsapi.Dialogs})
// 可选：单实例。jsapi 已用 single.OnSecondInstance 自动把第二实例的参数转给页面
single.Claim(app, single.Options{})
```

```ts
import { Window, Screens, Clipboard, Dialog, TangoError } from "@tangoui/runtime";

await Window.setTitle("Notes");
const off = Window.onResize(({ width, height }) => layout(width, height)); // off() 取消订阅
const [main] = await Screens.all();
try {
  await Clipboard.writeText("hi");
} catch (e) {
  if (e instanceof TangoError && e.code === "forbidden") { /* 没在 Go 侧开启 */ }
}
```

完整示例：`examples/runtime-demo`（`go run ./examples/runtime-demo -smoke` 自动自检）。

## 约定

- **线上名字**：方法是 ipc 调用 `tango:<模块>.<函数>`，事件是 `tango:<模块>.<事件>`，不会和应用自己的方法、事件撞名。
- **调用方窗口**：窗口类方法作用于发起调用的那个 Surface（Go 侧用 `ipc.ConnFrom(ctx)` 识别），页面不能指定别的窗口。
- **错误**：一律 reject 为 `TangoError`，`code` 取值：

  | code | 含义 |
  |---|---|
  | `forbidden` | 能力未在 `jsapi.Options` 开启；或 URL 协议不在 `shell.AllowSchemes` 白名单；或菜单用了不允许的 role |
  | `unsupported` | 当前平台不支持 |
  | `canceled` | 用户取消对话框 |
  | `bad_args` | 参数类型或取值不对 |
  | `not_found` | 方法不存在；或调用方不是本应用的窗口（窗口类方法） |
  | `not_running` | 应用正在退出 |
  | `internal` | 其它 Go 侧错误 |
  | `busy` | 只有 `tango:updater.*`（`API.InstallUpdater`）：已有一次检查或下载在进行 |

- **事件订阅**：所有 `onXxx` 返回取消函数。
- **页面生命周期**：页面重新加载（运行时重新 hello）或窗口关闭时，该页面注册的全局快捷键自动注销。

## 能力开关

`jsapi.Options{Allow, Deny}`：生效集合 = `(Defaults | Allow) &^ Deny`。未开启的能力，其方法返回 `forbidden`，相关事件也不转发。

| Cap | 默认 | 包含 |
|---|---|---|
| `Window` | 开 | 当前窗口自身的控制与窗口事件 |
| `Menu` | 开 | 在当前窗口弹出页面描述的右键菜单（只允许编辑类 role） |
| `Info` | 开 | 环境信息、主题、屏幕（只读）；主题/屏幕变化与 OpenURL、OpenFiles、Reopen、第二实例事件 |
| `AppControl` | 关 | 退出、隐藏/显示应用、Dock 徽标与进度、请求注意 |
| `ClipboardRead` | 关 | 读剪贴板（文本、图片、文件列表） |
| `ClipboardWrite` | 关 | 写剪贴板（文本、图片、文件列表） |
| `Dialogs` | 关 | 消息框、打开、保存对话框 |
| `Notify` | 关 | 通知权限、发送、点击事件 |
| `OpenURL` | 关 | 用默认程序打开 URL（协议另受 `shell.AllowSchemes` 限制） |
| `OpenPath` | 关 | 打开文件/文件夹、在文件管理器中显示 |
| `Hotkeys` | 关 | 全局快捷键 |
| `WebFind` | 关 | 本页页内查找 |
| `WebCapture` | 关 | 本页打印为 PDF、截图 |
| `WebCookies` | 关 | 读写本应用 WebView 的全部 Cookie（含 HttpOnly 与其它站点）、清除网站数据 |
| `WebDownloads` | 关 | 本页下载的进度/完成事件、取消下载 |
| `WebControl` | 关 | 修改本页 User-Agent |

读剪贴板也默认关闭：剪贴板里常有密码等敏感内容，算不上“安全的只读能力”。

**扩展**：其他包（如之后 `sys/` 下的移动端能力）用 `api.Handle(cap, "share.text", handler)` 注册 `tango:share.text`，自动套用同一套开关和错误码；`Cap` 的第 32 位以上留给这些包。JS 侧在 `runtime-js/src/api/` 加一个模块，用 `invoke`/`listen`（`api/internal.ts`），再在 `index.ts` 里 `export * as Xxx`。

## JS API 一览

平台列：✓ 支持；— 不支持（reject `unsupported` 或事件不触发）；“数字” 表示只支持数字徽标。桌面三平台的后端都实现了对应接口，但只有 macOS 实机验证过；移动端后端目前只有剪贴板文本和对话框。

### Window（`Window`，默认开）

| JS | 对应 Go | macOS | Windows | Linux | iOS/Android |
|---|---|---|---|---|---|
| `id(): Promise<number>` | `Surface.ID` | ✓ | ✓ | ✓ | ✓ |
| `setTitle(title)` | `Surface.SetTitle` | ✓ | ✓ | ✓ | ✓ |
| `size()` / `setSize(w, h)` | `Surface.Size` / `SetSize` | ✓ | ✓ | ✓ | ✓ |
| `setSizeLimits({minWidth, minHeight, maxWidth, maxHeight})` | `Surface.SetSizeLimits` | ✓ | ✓ | ✓ | — |
| `position()` / `setPosition(x, y)` | `Surface.Bounds` / `SetBounds` | ✓ | ✓ | ✓ | — |
| `bounds()` / `setBounds(rect)` | `Surface.Bounds` / `SetBounds` | ✓ | ✓ | ✓ | — |
| `center()` | `Surface.Center` | ✓ | ✓ | ✓ | — |
| `state()` / `setState(s)` | `Surface.State` / `SetState` | ✓ | ✓ | ✓ | — |
| `minimize()` `maximize()` `toggleMaximize()` `fullscreen()` `restore()` | `Surface.Minimize` 等 | ✓ | ✓ | ✓ | — |
| `setAlwaysOnTop(on)` / `isAlwaysOnTop()` | `Surface.SetAppearance`（`AlwaysOnTop`） | ✓ | ✓ | ✓ | — |
| `show()` `hide()` `focus()` | `Surface.Show` / `Hide` / `Focus` | ✓ | ✓ | ✓ | ✓ |
| `close()` | `Surface.Close`（不询问 OnClose） | ✓ | ✓ | ✓ | ✓ |
| `zoom()` / `setZoom(f)` | `Surface.Zoom` / `SetZoom` | ✓ | ✓ | ✓ | — |
| `reload()` | `Surface.Reload` | ✓ | ✓ | ✓ | — |
| `print()` | `Surface.Print` | ✓ | ✓ | ✓ | — |
| `openDevTools()` | `Surface.OpenDevTools`（需 `Debug`；不支持时什么也不做） | ✓ | ✓ | ✓ | — |
| `title()` | `Surface.Title` | ✓ | ✓ | ✓ | ✓ |
| `isVisible()` / `isFocused()` | `Surface.IsVisible` / `IsFocused` | ✓ | ✓ | ✓ | ✓ |
| `onResize(fn({width, height}))` | `Surface.OnResized` | ✓ | ✓ | ✓ | — |
| `onMove(fn({x, y}))` | `Surface.OnMoved` | ✓ | ✓ | ✓ | — |
| `onStateChange(fn(state))` | `Surface.OnStateChanged` | ✓ | ✓ | ✓ | — |
| `onFilesDropped(fn({paths, x, y}))` | `Surface.OnFilesDropped`（需 `AcceptFiles`） | ✓ | ✓ | ✓ | — |
| `onFocus(fn)` / `onBlur(fn)` | `Surface.OnFocused` / `OnBlurred`（窗口系统的焦点） | ✓ | ✓ | ✓ | — |

`state` 取值 `"normal" | "minimized" | "maximized" | "fullscreen"`。`close()` 和 `reload()` 的 promise 在窗口/页面消失前 resolve。

### App（`Info` 默认开；控制类需 `AppControl`）

| JS | Cap | 对应 Go | macOS | Windows | Linux | 移动端 |
|---|---|---|---|---|---|---|
| `info(): Promise<Environment>` | Info | `env.Info` | ✓ | ✓ | ✓ | ✓ |
| `onOpenURL(fn(url))` | Info | `EventOpenURL` | ✓ | ✓（启动参数/第二实例） | ✓（同左） | — |
| `onOpenFiles(fn(paths))` | Info | `EventOpenFiles` | ✓ | ✓ | ✓ | — |
| `onReopen(fn)` | Info | `EventReopen` | ✓ | — | — | — |
| `onSecondInstance(fn({args, cwd}))` | Info | `single.OnSecondInstance`（`jsapi.Install` 自动注册；应用需调用 `single.Claim`） | ✓ | ✓ | ✓ | — |
| `quit()` | AppControl | `App.Quit` | ✓ | ✓ | ✓ | ✓ |
| `hide()` / `show()` | AppControl | `appctl.Hide` / `Show` | ✓ | ✓ | ✓ | — |
| `setBadge(text)` | AppControl | `appctl.SetBadge` | ✓ | 数字 | 数字 | — |
| `setProgress(p \| null)` | AppControl | `appctl.SetProgress` | ✓ | ✓ | ✓ | — |
| `requestAttention(critical?)` | AppControl | `appctl.RequestAttention` | ✓ | ✓ | ✓ | — |

`onOpenURL` / `onOpenFiles` / `onReopen`：启动应用的那个深链在页面出现前到达，Go 侧会先存着（最多 32 条），页面第一次订阅时补发。

### Theme、Screens（`Info`，默认开）

| JS | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|
| `Theme.get(): Promise<{dark, accent}>` | `theme.System` | ✓ | — |
| `Theme.onChange(fn(theme))` | `EventThemeChanged` | ✓ | — |
| `Theme.vars(): Record<string,string>` | `uivars.SyncPage` 注入的 CSS 变量 | ✓ | ✓ |
| `Theme.onVarsChange(fn(vars))` | 页面上的 `tangouivars` 事件 | ✓ | ✓ |
| `Screens.all()` / `primary()` | `screen.All` / `Primary` | ✓ | — |
| `Screens.current()` | `screen.At`（窗口中心点） | ✓ | — |
| `Screens.onChange(fn(displays))` | `EventDisplaysChanged` | ✓ | — |

`accent` 是 `#rrggbbaa`，未知时为 `""`。

`Theme.vars()` 和 `Theme.onVarsChange` 不走 IPC，也不受 `Info` 白名单限制：它们读取的是 Go 侧用 `sys/theme/uivars.SyncPage(app, gen)` 注入到页面最前面的 `<style id="tango-ui-vars">:root{…}</style>`（值和 `ui/theme` 的 `CSSVars()` 一致，变量名随所用主题而定，例如 shadcn 主题会带 `--background`、`--primary`、`--radius` 等同名变量）。应用没调用 `SyncPage` 时，`vars()` 返回空对象、`onVarsChange` 不会触发。CSS 里可以直接用 `var(--变量名)`；系统切换深浅色或强调色、或应用刷新主题时会派发 `tangouivars` 事件。

`Display` 为 `{id, name, bounds, workArea, scale, primary}`，单位是逻辑像素。

### Clipboard（`ClipboardRead` / `ClipboardWrite`，默认关）

| JS | Cap | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|---|
| `readText()` / `writeText(text)` | Read / Write | `clipboard.ReadText` / `WriteText` | ✓ | ✓ |
| `readImage(): Promise<string \| null>` | Read | `clipboard.ReadImage`（PNG，base64） | ✓ | — |
| `writeImage(base64)` | Write | `clipboard.WriteImage`（非 PNG 为 `bad_args`；接受 data URL） | ✓ | — |
| `readFiles(): Promise<string[]>` | Read | `clipboard.ReadFiles` | ✓ | — |
| `writeFiles(paths)` | Write | `clipboard.WriteFiles`（须为绝对路径，否则 `bad_args`） | ✓ | — |

### Dialog（`Dialogs`，默认关）

| JS | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|
| `message({kind, title, message, buttons}): Promise<number>` | `dialog.Message` | ✓ | ✓ |
| `open({title, directory, filters, multiple, folders, canCreateDirs, showHidden}): Promise<string[]>` | `dialog.Open` | ✓ | ✓ |
| `save({title, directory, filename, filters, ...}): Promise<string>` | `dialog.Save` | ✓ | ✓ |

对话框挂在调用方窗口上。取消时 reject `canceled`（单按钮消息框除外，返回 0）。

### Notify（`Notify`，默认关）

| JS | 对应 Go | macOS | Windows | Linux | 移动端 |
|---|---|---|---|---|---|
| `requestPermission(): Promise<boolean>` | `notify.RequestPermission` | ✓（需 .app） | ✓ | ✓ | — |
| `send({title, subtitle, body, silent}): Promise<string>` | `notify.Send` | ✓（需 .app） | ✓ | ✓ | — |
| `onClick(fn(id))` | `notify.OnClick` | ✓ | ✓ | ✓ | — |

点击事件发给发送该通知的页面；那个窗口已关闭时发给所有页面。上一次运行发出的通知被点击（包括点通知启动应用）时，macOS 也会报告，id 照常传给 `onClick`。

### Shell（`OpenURL` / `OpenPath`，默认关）

| JS | Cap | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|---|
| `openURL(url)` | OpenURL | `shell.OpenURL`（协议白名单，否则 `forbidden`） | ✓ | — |
| `openPath(path)` | OpenPath | `shell.OpenPath` | ✓ | — |
| `revealPath(path)` | OpenPath | `shell.RevealPath` | ✓ | — |

### Menu（`Menu`，默认开）

| JS | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|
| `popup(items, {x, y}): Promise<{id, checked} \| null>` | `menu.Popup` | ✓ | — |
| `bindContextMenu(target, build)` | 同上（页面 `contextmenu` 事件） | ✓ | — |

`MenuItem`：`{id?, type?: "normal"\|"checkbox"\|"radio"\|"separator", label, accelerator?, disabled?, checked?, group?, role?, icon?(base64 PNG), submenu?, onClick?(checked)}`。`role` 只能是 `copy`、`paste`、`cut`、`selectAll`、`undo`、`redo`，其它（如 `quit`）返回 `forbidden`。被选中项的 `onClick` 在 promise resolve 前调用；菜单被关掉时 resolve `null`。

### Hotkey（`Hotkeys`，默认关）

| JS | 对应 Go | 桌面三平台 | 移动端 |
|---|---|---|---|
| `register(accelerator, fn): Promise<() => Promise<void>>` | `hotkey.Register` | ✓ | — |

加速键写法同 `hotkey` 包（`"CmdOrCtrl+Shift+K"`），写错返回 `bad_args`（`hotkey.Validate`）。快捷键归注册它的页面所有：页面重载或窗口关闭时自动注销，别的页面不能注销它。

### Web（`Web*`，默认关）

作用于调用方页面自己的 WebView。权限请求、HTTP 认证、新窗口、导航策略只能由 Go 决定（`tango.App` / `Surface` 的处理函数），页面不能替自己回答。平台差异见 [webview.md](webview.md#支持矩阵)。

| JS | Cap | 对应 Go | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|---|---|
| `find(text, {matchCase, backward, noWrap}): Promise<{matches, active, found}>` | WebFind | `Surface.Find` | ✓ | ✓¹ | ✓ | ✓ | ✓² |
| `findNext()` / `findPrevious()` / `stopFind()` | WebFind | `Surface.FindNext` / `FindPrevious` / `StopFind` | ✓ | ✓¹ | ✓ | ✓ | ✓ |
| `printToPDF({landscape, noBackground, pageWidth, pageHeight}): Promise<string>` | WebCapture | `Surface.PrintToPDF`（base64） | ✓ | ✓ | ✓ | ✓ | — |
| `screenshot(): Promise<string>` | WebCapture | `Surface.Screenshot`（PNG，base64） | ✓ | ✓ | ✓ | ✓ | ✓ |
| `cookies(url?): Promise<Cookie[]>` | WebCookies | `Surface.Cookies` | ✓ | ✓ | ✓³ | ✓ | 只能按 url |
| `setCookie(c)` / `deleteCookie(c)` / `clearCookies(domain?)` | WebCookies | `Surface.SetCookie` / `DeleteCookie` / `ClearCookies` | ✓ | ✓ | ✓³ | ✓ | ✓ |
| `clearData(kinds?)` | WebCookies | `Surface.ClearBrowsingData`（`"cookies" \| "cache" \| "storage" \| "all"`，不传为全部） | ✓ | ✓ | ✓ | ✓ | ✓ |
| `onDownload(fn(download))` | WebDownloads | `Surface.OnDownloadEvent` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `cancelDownload(id)` | WebDownloads | `Surface.CancelDownload` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `setUserAgent(ua)` | WebControl | `Surface.SetUserAgent`（`""` 恢复默认） | ✓ | ✓ | ✓ | ✓ | ✓ |

¹ 需 WebView2 运行时 1.0.3405+，更旧返回 `unsupported`。² 不支持 `matchCase`。³ 列出全部与按域清除需 WebKitGTK 2.42。`matches` 为 -1 表示平台不计数，`active` 为 0 表示未知（Linux、iOS 恒为 0）。

`Cookie`：`{name, value, domain, path?, expires?: number | null（毫秒时间戳，null 为会话 Cookie）, secure?, httpOnly?, sameSite?: "" | "lax" | "strict" | "none"}`，`domain` 必填（`example.com` 仅主机，`.example.com` 含子域），否则 `bad_args`。`Download`：`{id, state: "progress" | "done" | "failed" | "canceled", received, total（未知为 -1）, path, error?}`，只发给发起下载的页面。

### Events（总是可用）

| JS | 对应 Go |
|---|---|
| `on(name, fn)` / `once(name, fn)` / `next(name)` | `Router.Emit` |
| `emit(name, data?)` | `Router.On` |

这是应用自己的事件（名字由应用定），和顶层的 `tango.on` / `tango.emit` 是同一个通道。

## 本地开发

源码在 `runtime-js/src`（`index.ts` 是包入口，`api/*.ts` 是各命名空间，`client.ts` 是调用、事件与流的客户端）。改完后：

```bash
cd runtime-js
npm ci            # 第一次
npm run build     # 生成 dist/（ESM、IIFE、.d.ts），并重写 runtimejs/tango.iife.js
npm test          # 构建 + 单元测试 + API 报告与线上契约检查
```

页面拿到新代码的方式取决于它怎么引入运行时：

- **Go 注入的脚本**（`app.AddBridgeScript(runtimejs.Script)`，页面里的全局 `tango`）：`runtimejs.Script` 是用 `go:embed` 嵌进 Go 二进制的 `runtime-js/runtimejs/tango.iife.js`。改了 `src` 之后要先 `npm run build` 重新生成这个文件，再重新编译并重启应用（例如重新 `go run ./examples/runtime-demo`）；刷新页面不会生效。`runtimejs/tango.iife.js` 要随源码一起提交，CI 会检查它是否与源码一致。
- **npm 前端**（`import ... from "@tangoui/runtime"`）：把依赖指向本地目录。以仓库里的 `examples/<应用>/frontend/package.json` 为例：

  ```json
  { "dependencies": { "@tangoui/runtime": "file:../../../runtime-js" } }
  ```

  路径相对于该 `package.json` 所在目录。npm 会建一个指向 `runtime-js` 的链接，而包的入口是 `dist/`（已被 gitignore），所以先在 `runtime-js` 里 `npm run build`，之后每次改源码都要重新 build，前端开发服务器才能拿到新代码。仓库外的项目可以用 `tan init -local <Tango 检出目录>`，它会写入指向该检出的 `runtime-js` 的绝对 `file:` 路径。

## API 稳定性

`@tangoui/runtime` 与 Go 侧 `jsapi` 一样是稳定 API（`docs/specs/api-stability.md` §6.1）。下面这些都是契约，改动即 API 变化，需要在 `CHANGELOG.md` 的「未发布」里写明，不兼容的改动按 [release.md 的「API 稳定性」](release.md#api-稳定性) 走 `api-break` 流程：

- 包入口导出的名字与类型签名：`call`、`on`、`emit`、`stream`、`onStream`、`onRaw`、`postRaw`、`createClient`、`getClient`、`TangoError`、`ErrorCode`、`ACK_EVERY`、`WINDOW` 及各类型。
- 命名空间（`Window`、`App`……`Web`）里每个成员的名字与签名，以及命名空间本身的名字。
- 错误：`TangoError` 的 `code` 取值（上面的错误码表、`ErrorCode` 联合类型、`jsapi.Code*` 与 `ipc.Code*`）。
- 线上名字：`tango:<模块>.<函数>` 方法名、`tango:<模块>.<事件>` 事件名、`jsapi.Cap` 的位值，以及参数与结果、事件数据的 JSON 形状（字段名、类型、取值）。

两份提交进仓库的黄金文件让这些变化出现在 diff 里：

| 文件 | 内容 | 生成 / 检查 |
|---|---|---|
| `runtime-js/etc/runtime.api.md` | API Extractor 从 `dist/index.d.ts` 生成的公开 API 报告；命名空间按成员展开，每个成员带完整签名（同名成员会被改名为 `hide_2` 这样，命名空间块里写明 `hide_2 as hide`） | `npm run api:update`（本地，重写报告）；`npm run api:check`（CI，报告与源码不一致就失败） |
| `api/jsapi-methods.txt` | 错误码、全部 `tango:*` 方法（带所需 `Cap`）与事件，以及 runtime-js 里用到每个名字的函数（`js=`；`js=-` 表示没有封装，例如 `updater.*`） | `go test ./jsapi -run TestWireContract -update` 重写 Go 侧各列（从 `api.Handle`/`Emit`/`Broadcast` 的注册代码提取，并用真实的 `Install` 交叉核对）；`npm run contract:update` 重写 `js=` 列（从 `src/api/*.ts` 的 `invoke`/`listen` 调用提取）；`npm run contract:check` 与 `go test ./jsapi -run TestWireContract` 只检查 |

改 API 时的流程：

```bash
cd runtime-js
npm run api:update        # 改了导出或签名
cd .. && go test ./jsapi -run TestWireContract -update   # 改了 jsapi 的方法/事件/错误码
cd runtime-js && npm run contract:update                 # 改了 src/api 用到的名字，或上一步之后
npm test                  # 含 API 报告是否最新、契约是否一致，以及故意制造漂移的自检
git diff ../api/jsapi-methods.txt etc/runtime.api.md     # 审查时看这两份 diff
```

TS 封装调用了 Go 没有注册的方法（或把方法当事件用）、Go 删掉了封装还在用的名字、名字不是常量字符串、`src/api` 里绕过 `invoke`/`listen` 直接写 `"tango:..."`、`ErrorCode` 或上面的错误码表与实际错误码不一致，都会让检查失败。

**这些检查的边界**：它们只比较结构，不保证兼容。

- API 报告只看名字和 TypeScript 类型。实现行为变了、默认值变了、`Promise` 何时 resolve 变了，报告都不会变。返回类型写成 `Promise<T>` 而 Go 实际返回的 JSON 换了字段，报告也看不出来。
- 线上契约只核对名字（以及方法所需的 `Cap`）。参数个数与顺序、参数和结果的 JSON 形状、事件数据的字段，由 `jsapi` 与 `runtime-js` 各自的单元测试覆盖，没有统一的黄金文件；IPC 消息本身也还没有协议版本号。
- 不在契约文件里的动态名字：应用自己的方法与事件（`call`/`on`/`emit`/`stream`、`Events.*`，名字由应用定）；其他包通过 `API.Handle` 注册的方法（`Cap` 第 32 位以上，目前仓库里没有）；`Theme.vars`/`onVarsChange` 读的页面全局变量 `__tangoUIVarsState` 和 DOM 事件 `tangouivars`（由 `sys/theme/uivars` 注入，不走 `jsapi`）。
- 已知差异：`updater.*` 方法与 `updater.event` 事件在 runtime-js 里没有封装。

## 已知缺口

- WebView 高级能力的平台差异（查找计数、PDF、Cookie 枚举等）见 [webview.md](webview.md#支持矩阵)。
- 移动端没有窗口焦点事件（`onFocus`/`onBlur` 不触发），请用 App 生命周期。
- Linux AppIndicator 托盘不区分左右键，`tray.Options.MenuOn` 在那里不起作用。
