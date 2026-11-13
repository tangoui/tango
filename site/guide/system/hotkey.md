# hotkey：全局快捷键与页面快捷键

`hotkey` 提供两种快捷键：

- **全局快捷键**（`Register`）：系统级，应用在后台、没有焦点时也会触发，适合「呼出主窗口」「快速记录」之类功能。
- **页面快捷键**（`Bind`）：只在某个 Surface 的页面拥有键盘焦点时触发。它在页面里监听 keydown 并吞掉按键，不需要平台支持。

加速键写法与菜单相同，例如 `"CmdOrCtrl+Shift+K"`。

```go
import "github.com/tangoui/tango/sys/hotkey"
```

## 用法

```go
hk, err := hotkey.Register(app, "CmdOrCtrl+Shift+K", func() {
	win.Show()
	win.Focus()
})
if err != nil {
	log.Println("hotkey:", err) // 写错、被占用或 ErrUnsupported
	return
}
defer hk.Unregister()
```

页面内快捷键：

```go
b, err := hotkey.Bind(win, "CmdOrCtrl+K", openPalette)
// ...
b.Unbind()
```

只校验不注册（例如设置页里让用户自定义快捷键）：

```go
if err := hotkey.Validate(userInput); errors.Is(err, hotkey.ErrBadAccelerator) {
	showError(err.Error()) // 错误文本说明哪里不对
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Register(a, accel, fn) (*Hotkey, error)` | 注册全局快捷键 |
| `(*Hotkey).Unregister()` / `Unregister(h)` | 注销；重复注销返回 `ErrUnregistered` |
| `Bind(s, accel, fn) (*Binding, error)` | 绑定页面快捷键；跨导航保留，Surface 关闭时结束 |
| `(*Binding).Unbind()` | 解除绑定，重复调用无害 |
| `Accelerator()` | 返回注册时的加速键字符串 |
| `Validate(accel) error` | 只检查写法 |
| `ErrBadAccelerator` `ErrUnregistered` `ErrUnsupported` | 错误值 |

**加速键语法**：修饰键 `CmdOrCtrl`、`Cmd`/`Command`/`Super`/`Meta`、`Ctrl`、`Alt`/`Option`、`Shift`，用 `+` 连接一个按键：`A`–`Z`、`0`–`9`、`F1`–`F24`、`Enter`、`Tab`、`Space`、`Backspace`、`Delete`、`Insert`、`Esc`、方向键 `Up/Down/Left/Right`、`Home`、`End`、`PageUp`、`PageDown`、标点、`num0`–`num9`。`CmdOrCtrl` 在 macOS 上是 Command，其他平台是 Control。

完整签名见 [API 参考](/reference/api/sys-hotkey)。

## 平台支持

| | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| `Register`（全局） | ✓ Carbon 热键，无需辅助功能权限 | ✓ `RegisterHotKey` | X11 ✓（`XGrabKey`）；Wayland — | — | — |
| `Bind`（页面内） | ✓ | ✓ | ✓ | 不依赖平台 | 不依赖平台 |

> Windows、Linux 实现尚未实机验证；`Bind` 在移动端未专门验证。

## 在页面里调用

全局快捷键对应 `@tangoui/runtime` 的 `Hotkey` 模块，需要开启 `Hotkeys`（默认关）：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.Hotkeys})
```

```ts
import { Hotkey } from "@tangoui/runtime";

const off = await Hotkey.register("CmdOrCtrl+Shift+K", () => console.log("pressed"));
// ...
await off();
```

写错的加速键返回 `bad_args`。页面注册的快捷键归该页面所有：页面重载或窗口关闭时自动注销，别的页面不能注销它。见 [JS API](/reference/js-api)。

## 注意

- 回调都在主线程运行，里面不要做耗时操作。
- 全局快捷键可能已被系统或其他应用占用，此时 `Register` 返回错误。
- Wayland 没有给应用的全局快捷键 API，Linux 在 Wayland 会话下 `Register` 返回 `ErrUnsupported`。
- 同一 Surface 上多个 `Bind` 使用相同加速键时，先绑定的生效；被绑定的按键不会再传给页面。
