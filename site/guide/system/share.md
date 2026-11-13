# share：系统分享面板

`share` 打开系统分享面板，分享文字、链接和 / 或文件：iOS 上是 `UIActivityViewController`，Android 上是 `ACTION_SEND` 选择器，macOS 上是 `NSSharingServicePicker`。

```go
import "github.com/tangoui/tango/sys/share"
```

## 用法

```go
go func() { // Android 上必须在 goroutine 里调用
	err := share.Share(app, share.Options{
		Text: "看看这个",
		URL:  "https://example.com",
	})
	if errors.Is(err, share.ErrCanceled) {
		// 用户关掉了面板（iOS、macOS）
	}
}()
```

分享文件：

```go
err := share.Share(app, share.Options{
	Files: []string{filepath.Join(dir, "report.pdf")}, // 必须是已存在的文件
	Title: "导出的报告",                                   // Android 选择器标题 / 邮件主题
})
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Share(a, Options) error` | 显示分享面板并等待 |
| `Options` | `Text`、`URL`、`Files`（已存在文件的路径）、`Title`（Android 选择器标题 / 邮件主题）；三项内容至少给一项 |
| `ErrCanceled` | 用户关掉面板 |
| `ErrUnsupported` | 后端没有分享面板 |

完整签名见 [API 参考](/reference/api/sys-share)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ `NSSharingServicePicker` | — | — | ✓ `UIActivityViewController`（iPad 用 popover） | ✓ `ACTION_SEND(_MULTIPLE)` 选择器 |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Sharing struct{ app *tango.App }

func (s Sharing) Link(url string) error { return share.Share(s.app, share.Options{URL: url}) }

bind.Register(r, "Sharing", Sharing{app})
```

页面里 `await tango.call("Sharing.Link", location.href)`。

## 注意

- **何时返回**：iOS、macOS 上阻塞到面板关闭；Android 在面板弹出后就返回，系统不会告诉应用用户做了什么，所以 Android 上不会得到 `ErrCanceled`。
- **线程**：Android 上不要在主线程回调里调用，主线程上会直接返回 `tango.ErrNestedWait`（见 [移动端](/guide/mobile)）。
- Android 分享文件经宿主自带的 `TangoFileProvider` 转为 content URI，无需额外配置。
- macOS 上面板指向 key 窗口的内容中央（没有 key 窗口时用主窗口或任一可见窗口）。
- 不需要在 `tan.json` 的 `permissions` 里声明。Windows、Linux 返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
