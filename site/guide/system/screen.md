# screen：显示器信息

`screen` 列出连接到电脑的显示器：每块屏幕的边界、工作区（去掉菜单栏、Dock、任务栏后的区域）、缩放比例，以及哪一块是主屏。常用于把窗口放到指定屏幕、在鼠标所在屏幕居中、根据工作区计算窗口大小。

坐标是逻辑像素（DIP），原点是主屏左上角；位于主屏左侧或上方的屏幕坐标为负。`Scale` 是物理像素与逻辑像素之比。

```go
import "github.com/tangoui/tango/sys/screen"
```

## 用法

```go
ds, err := screen.All(app) // 主屏排在第一个
for _, d := range ds {
	log.Printf("%s %v work=%v scale=%.1f primary=%v", d.Name, d.Bounds, d.WorkArea, d.Scale, d.Primary)
}

p, _ := screen.Primary(app)
d, _ := screen.At(app, x, y) // 包含该点的屏幕，不在任何屏幕上时取最近的
```

把窗口放进某块屏幕的工作区：

```go
wa := d.WorkArea
win.SetBounds(tango.Rect{X: wa.X + 40, Y: wa.Y + 40, Width: wa.Width - 80, Height: wa.Height - 80})
```

监听变化：

```go
screen.OnChange(app, func() { log.Println("displays changed") })
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `All(a) ([]Display, error)` | 所有屏幕，主屏在前 |
| `Primary(a) (Display, error)` | 主屏（macOS 上带菜单栏的那块，Windows 上主任务栏所在的那块） |
| `At(a, x, y) (Display, error)` | 包含该点或离它最近的屏幕 |
| `OnChange(a, fn)` | 屏幕增减、重新排列或缩放变化时在主线程调用；等价于监听 `tango.EventDisplaysChanged` |
| `Display` | `ID`、`Name`、`Bounds`、`WorkArea`、`Scale`、`Primary` |
| `ErrNoDisplay` | 系统报告没有任何显示器（无头会话） |

完整签名见 [API 参考](/reference/api/sys-screen)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ | ✓ | ✓ | — | — |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Screens` 模块，属于默认开启的 `Info` 能力：

```ts
import { Screens } from "@tangoui/runtime";

const [main] = await Screens.all();
const here = await Screens.current(); // 当前窗口中心点所在的屏幕
const off = Screens.onChange((displays) => console.log(displays.length));
```

见 [JS API](/reference/js-api)。

## 注意

- 应用运行后可在任意 goroutine 调用；移动端和最小后端返回 `ErrUnsupported`，应用退出后返回 `tango.ErrNotRunning`。
- `Rect` 的坐标口径与窗口的 `Bounds` 相同，详见 [窗口](/guide/window)。
