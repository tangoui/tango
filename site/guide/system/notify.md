# notify：系统通知

`notify` 发送系统通知（macOS 和 iOS 通知中心、Windows 通知区域、Linux freedesktop 通知服务、Android 通知渠道），并报告用户点击了哪一条。发送前先用 `RequestPermission` 请求许可；`Send` 返回通知 id，点击时 `OnClick` 收到同一个 id。

```go
import "github.com/tangoui/tango/sys/notify"
```

## 用法

```go
// 在 Run 之前或 OnReady 里注册，避免漏掉「点通知启动应用」的那次点击
notify.OnClick(app, func(id string) {
	win.Show()
	win.Focus()
})

app.OnReady(func() {
	go func() {
		ok, err := notify.RequestPermission(app)
		if !ok || err != nil {
			return
		}
		id, err := notify.Send(app, notify.Message{Title: "构建完成", Body: "3 个警告"})
		log.Println("sent", id, err)
	}()
})
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `RequestPermission(a) (bool, error)` | 如平台需要且未询问过，则请求许可；返回能否显示通知。阻塞到用户作答 |
| `Send(a, Message) (string, error)` | 显示通知，返回 id |
| `OnClick(a, fn)` | 用户点击本应用发出的通知时在主线程调用 `fn(id)` |
| `Message` | `Title`、`Subtitle`（macOS 显示在标题下，其他平台并入正文）、`Body`、`Silent`（不响铃） |
| `ErrUnsupported` | 后端没有通知 |

完整签名见 [API 参考](/reference/api/sys-notify)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ 通知中心（需从 .app 运行） | ✓ 通知区域气泡（Windows 10+ 显示为 toast） | ✓ D-Bus `org.freedesktop.Notifications` | ✓ `UNUserNotificationCenter` | ✓ 通知渠道，Android 13+ 需声明 `notifications` |

> Windows、Linux 实现尚未实机验证。移动端本地通知使用 `notify`。远程推送见 [push](/guide/system/push)。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Notify` 模块，需要开启 `Notify`（默认关）：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.Notify})
```

```ts
import { Notify } from "@tangoui/runtime";

if (await Notify.requestPermission()) {
  const id = await Notify.send({ title: "下载完成", body: "report.pdf", silent: true });
}
const off = Notify.onClick((id) => console.log("clicked", id));
```

点击事件发给发送该通知的页面；那个窗口已关闭时发给所有页面。见 [JS API](/reference/js-api)。完整示例见仓库 `examples/notify-demo`。

## 注意

- macOS、iOS 和 Android 也会报告前次运行的通知点击，包括通过通知启动应用。id 在多次运行之间保持唯一。
- Windows 走通知区域气泡，不需要 AUMID 或安装程序。
- 应用运行后可在任意 goroutine 调用；Android 上用 goroutine 调用 `RequestPermission`，因为主线程不能嵌套等待。应用退出后返回 `tango.ErrNotRunning`。

## macOS 通知排障

从带 `CFBundleIdentifier` 的 `.app` 请求通知权限。用 `tan build` 生成带 ad-hoc 签名的应用包。`go run` 或直接运行普通 `go build` 产物会收到需要 `.app` 的错误。

如果 `RequestPermission` 返回 `Notifications are not allowed for this application`，且系统未弹出授权询问，先退出正在运行的测试副本。将完整的 `.app` 移到 `/Applications` 或 `~/Applications`，用 Finder 或 `open` 启动安装后的副本，再请求权限。

macOS 26.6 上，同一份 ad-hoc 签名的包从 `/tmp` 通过 `open -g` 启动时，系统日志中的 `usernoted` 报 `Failed to find or validate client`。用 LaunchServices 显式登记临时副本后仍有该错误。移到 `~/Applications` 后用 `open -g` 启动，系统正常发出首次授权询问。

如果系统已经询问过，前往「系统设置 → 通知」检查本应用的「允许通知」。
