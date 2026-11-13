# push：远程推送通知

`push` 把应用注册到推送服务，拿到设备令牌（token）交给你的服务器，并报告应用运行时收到的推送和用户点击的推送。iOS 走 APNs；Android 走 Firebase Cloud Messaging（FCM），但 Tango 不打包 Google 服务，需要应用自己接入。

```go
import "github.com/tangoui/tango/sys/push"
```

## 用法

```go
// 在 Run 之前或 OnReady 里注册，避免漏掉「点推送启动应用」的那次点击
push.OnOpen(app, func(m push.Message) { route(m.Data) })
push.OnReceive(app, func(m push.Message) { log.Println("foreground push:", m.Title) })
push.OnToken(app, func(token string) { uploadToken(token) }) // 令牌轮换时也会调用

app.OnReady(func() {
	go func() { // Android 上必须在 goroutine 里调用
		token, err := push.Register(app)
		if err != nil {
			log.Println("push:", err) // ErrPermissionDenied、ErrUnsupported ...
			return
		}
		uploadToken(token)
	}()
})
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Register(a) (string, error)` | 注册推送并返回令牌（iOS 为小写十六进制） |
| `OnToken(a, fn)` | 平台签发新令牌时调用（`Register` 之后、FCM 轮换时） |
| `OnReceive(a, fn)` | 应用运行时收到推送 |
| `OnOpen(a, fn)` | 用户点击推送（包括启动应用的那次） |
| `Message` | `ID`、`Title`、`Body`、`Action`（通知按钮的 action 标识，普通点击为 `""`）、`Data`（完整载荷：iOS 为含 `aps` 的 userInfo，Android 为 FCM data） |
| `ErrPermissionDenied` / `ErrUnsupported` | 用户拒绝通知 / 不支持或 Android 未接入 FCM |

完整签名见 [API 参考](/reference/api/sys-push)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ APNs | 需自行接入 FCM，否则 `ErrUnsupported` |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。iOS 模拟器（iOS 16+、Apple 芯片）也能签发令牌，可用 `xcrun simctl push booted <bundle-id> payload.apns` 模拟推送。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露注册方法，并在回调里把消息转发给页面：

```go
type Push struct{ app *tango.App }

func (p Push) Register() (string, error) { return push.Register(p.app) }

bind.Register(r, "Push", Push{app})
push.OnOpen(app, func(m push.Message) {
	for _, s := range app.Surfaces() {
		r.Emit(s, "push-open", m.Data)
	}
})
```

`r.Emit` 的事件名由应用自己定，页面用 `tango.on("push-open", fn)` 订阅（见 [IPC](/guide/concepts/ipc)）。

## 注意

- **iOS**：`Register` 在从未询问过时先请求提醒 / 声音 / 角标权限。应用需要 `aps-environment` entitlement（描述文件开启 Push Notifications），否则返回系统错误。后台静默推送还需 `UIBackgroundModes` 含 `remote-notification`。前台收到的推送以横幅显示并报告给 `OnReceive`。
- **Android**：在 Gradle 工程里加入 Firebase 依赖和 `google-services.json`，写一个 `FirebaseMessagingService` 子类，在回调里调用 `org.tango.host.TangoPush.token` / `.received` / `.opened`，之后 `Register` 返回 FCM 令牌，事件开始生效。步骤见 [移动端](/guide/mobile)。
- **权限声明**：`tan.json` 的 `permissions` 加 `"notifications"`（Android 33+ 生成 `POST_NOTIFICATIONS`）。见 [tan.json](/reference/tan-json)。
- 回调在主线程运行；桌面返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
