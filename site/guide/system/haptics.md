# haptics：触感反馈

`haptics` 播放一次触感反馈：轻 / 中 / 重的撞击感、选择变化的轻触，以及成功、警告、错误三种通知节奏。iOS 上用 `UIFeedbackGenerator`，Android 上用 `View.performHapticFeedback`（不需要任何权限）。

它是「点一下」的提示，适合按钮确认、开关切换、拖动吸附等交互；不等待节奏播完就返回。

```go
import "github.com/tangoui/tango/sys/haptics"
```

## 用法

```go
_ = haptics.Play(app, haptics.Success)

// 在列表滚轮里每切换一项
_ = haptics.Play(app, haptics.Selection)
```

桌面上返回 `ErrUnsupported`，跨平台代码可以忽略这个错误：

```go
if err := haptics.Play(app, haptics.Light); errors.Is(err, haptics.ErrUnsupported) {
	// 桌面或没有触感的后端
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Play(a, k) error` | 播放 `k`，立即返回 |
| `Light` `Medium` `Heavy` | 撞击反馈 |
| `Selection` | 选择变化 |
| `Success` `Warning` `Error` | 通知反馈 |
| `Kind` | 反馈类型 |
| `ErrUnsupported` | 后端没有触感 API |

完整签名见 [API 参考](/reference/api/sys-haptics)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ `UIImpact/Selection/NotificationFeedbackGenerator` | ✓ `performHapticFeedback`，按 API 等级映射 |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Feedback struct{ app *tango.App }

func (f Feedback) Tap() error { return haptics.Play(f.app, haptics.Light) }

bind.Register(r, "Feedback", Feedback{app})
```

页面里 `await tango.call("Feedback.Tap")`。

## 注意

- 没有触感引擎的设备（iPad、iOS 模拟器、大多数台式机）静默不做任何事；只要平台有这个 API，调用仍返回 nil。因此模拟器上「成功」不代表真机上有振动。
- 桌面后端没有实现，返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
- 不需要在 `tan.json` 的 `permissions` 里声明任何东西。
