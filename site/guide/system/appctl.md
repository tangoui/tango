# appctl：控制整个应用在窗口管理器里的表现

`appctl` 作用于应用整体而不是某一个窗口：隐藏、显示全部窗口，在 Dock 图标或任务栏按钮上显示徽标和进度条，跳动 Dock 图标或闪烁任务栏按钮请求注意，以及在 macOS 上隐藏 Dock 图标（做成只有托盘图标的应用）。

徽标和进度设置之后，新建的窗口也会带上它们。

```go
import "github.com/tangoui/tango/sys/appctl"
```

## 用法

```go
app.OnReady(func() {
	_ = appctl.SetBadge(app, "3")      // "" 清除
	_ = appctl.SetProgress(app, 0.5)   // 负数隐藏，大于 1 按 1 处理
	_ = appctl.RequestAttention(app, false)
})
```

先查询再调用，避免在不支持的平台上报错：

```go
if appctl.Supports(app, appctl.CtlBadgeText) {
	appctl.SetBadge(app, "new")
} else if appctl.Supports(app, appctl.CtlBadge) {
	appctl.SetBadge(app, "1") // Windows / Linux 只能显示数字
}
```

只做托盘应用（macOS）：

```go
_ = appctl.SetDockVisible(app, false) // 同时没有菜单栏
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Hide(a)` / `Show(a)` | 隐藏全部窗口（macOS 上等同 Cmd+H）/ 恢复并把应用带到前台 |
| `SetBadge(a, text)` | Dock / 任务栏徽标，`""` 清除 |
| `SetProgress(a, p)` | 进度条，`p` 取 `[0, 1]`，负数隐藏 |
| `RequestAttention(a, critical)` | 跳动 Dock 图标或闪烁任务栏；macOS 上 `critical` 为 true 时持续跳动直到用户激活 |
| `SetDockVisible(a, visible)` | 显示 / 隐藏 Dock 图标（仅 macOS） |
| `Supports(a, op)` | 平台是否支持 `op` 里的全部操作 |
| `CtlHide` `CtlBadge` `CtlBadgeText` `CtlProgress` `CtlAttention` `CtlDock` | 给 `Supports` 用的操作位 |

完整签名见 [API 参考](/reference/api/sys-appctl)。

## 平台支持

| 操作 | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| `Hide` / `Show` | ✓ | ✓（全部窗口） | ✓（全部窗口） | — | — |
| `SetBadge` | ✓ 任意文字 | 仅数字（叠加图标） | 仅数字（Unity launcher API，如 Ubuntu Dock、KDE） | — | — |
| `SetProgress` | ✓ | ✓（任务栏） | ✓（Unity launcher API） | — | — |
| `RequestAttention` | ✓ | ✓（FlashWindowEx） | ✓（urgency hint） | — | — |
| `SetDockVisible` | ✓ | — | — | — | — |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

对应 `@tangoui/runtime` 的 `App` 模块，需要开启 `AppControl`（默认关）：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.AppControl})
```

```ts
import { App } from "@tangoui/runtime";

await App.setBadge("3");
await App.setProgress(0.4);   // null 隐藏
await App.requestAttention(true);
await App.hide();
```

`SetDockVisible` 和 `Supports` 没有 JS 对应。见 [JS API](/reference/js-api)。

## 注意

- 应用运行后可在任意 goroutine 调用；平台做不到时返回 `appctl.ErrUnsupported`，应用退出后返回 `tango.ErrNotRunning`。
- Windows、Linux 上给 `SetBadge` 传非数字文字会返回 `ErrUnsupported`，并清除徽标。
- 隐藏 Dock 图标后应用也没有菜单栏，记得用 [tray](/guide/system/tray) 留一个入口。
