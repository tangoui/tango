# orientation：读取与锁定界面方向

`orientation` 读取当前界面方向、监听旋转，并可以把界面锁定在一组方向里（例如视频播放时只允许横屏）。它只在移动端有意义，桌面上返回 `ErrUnsupported`。

```go
import "github.com/tangoui/tango/sys/orientation"
```

## 用法

```go
o, err := orientation.Get(app)
if err == nil && orientation.IsLandscape(o) {
	layoutWide()
}

orientation.OnChange(app, func(o orientation.Orientation) {
	log.Println("now", orientation.String(o))
})
```

锁定与解锁：

```go
_ = orientation.Lock(app, orientation.Landscape) // 任一横向，必要时会转过去
// ...
_ = orientation.Unlock(app)
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Get(a) (Orientation, error)` | 当前界面方向 |
| `OnChange(a, fn)` | 方向变化时在主线程调用 `fn`；桌面上什么也不做 |
| `Lock(a, Mask)` / `Unlock(a)` | 限制到 `Mask` 中的方向 / 解除限制 |
| `IsLandscape(o)` / `String(o)` | 是否横向 / 名字（`"portrait"`、`"portraitUpsideDown"`、`"landscapeLeft"`、`"landscapeRight"`、`"unknown"`） |
| `Orientation` 常量 | `Unknown`、`Portrait`、`PortraitUpsideDown`、`LandscapeLeft`、`LandscapeRight` |
| `Mask` 常量 | `MaskPortrait`、`MaskPortraitUpsideDown`、`MaskLandscapeLeft`、`MaskLandscapeRight`、`Landscape`（任一横向）、`PortraitOnly`、`All` |

方向按设备顶边指向命名：`LandscapeLeft` 是设备从竖屏逆时针转 90°。

完整签名见 [API 参考](/reference/api/sys-orientation)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ `interfaceOrientation`；锁定用 `supportedInterfaceOrientations` + `requestGeometryUpdate` | ✓ `Display` 旋转；锁定用 `setRequestedOrientation` |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Screen struct{ app *tango.App }

func (s Screen) LockLandscape() error { return orientation.Lock(s.app, orientation.Landscape) }
func (s Screen) Unlock() error        { return orientation.Unlock(s.app) }

bind.Register(r, "Screen", Screen{app})
```

页面也可以直接用 CSS 媒体查询 `orientation` 调整布局，只在需要锁定时才调用 Go。

## 注意

- iOS 上只能在 `UISupportedInterfaceOrientations` 声明过的方向里选；Android 默认允许全部方向。
- iPad 上支持分屏多任务的应用无法锁定，系统会忽略请求；确实需要锁定就在 Info.plist 加 `UIRequiresFullScreen`。见 [移动端](/guide/mobile)。
- 不需要在 `tan.json` 的 `permissions` 里声明。
- 桌面返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
