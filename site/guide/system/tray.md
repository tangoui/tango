# tray：系统托盘图标

`tray` 在系统托盘放一个图标：macOS 菜单栏右侧的状态项、Windows 通知区域、Linux 的 AppIndicator / StatusNotifierItem。图标可以挂一个 [menu](/guide/system/menu) 菜单，也可以响应单击、双击、右键。

本页只列 API；托盘菜单、点击行为、只有托盘的应用等完整教程见 [菜单与托盘](/guide/menu-tray)。

```go
import "github.com/tangoui/tango/sys/tray"
```

## 用法

```go
//go:embed icon.png
var colourPNG []byte
//go:embed icon-mac.png
var templatePNG []byte

t, err := tray.New(app, tray.Options{
	Icon:     colourPNG,   // Windows / Linux
	MacIcon:  templatePNG, // macOS 菜单栏：黑色透明底的模板图
	Template: true,
	Tooltip:  "My App",
})
if err != nil {
	log.Println("tray:", err) // 例如 Linux 没装 AppIndicator 库
	return
}

m := menu.New()
m.Add("打开", show)
m.Separator()
m.Role(menu.Quit)
_ = t.SetMenu(m)
t.OnClick(show)
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `New(a, Options) (*Tray, error)` | 添加托盘图标 |
| `Options` | `Icon`（PNG，32×32 或 44×44 px 较合适）、`MacIcon`（仅 macOS 使用）、`Template`（macOS 模板图，跟随菜单栏外观）、`Tooltip`、`Title`（macOS 图标旁文字）、`MenuOn` |
| `SetMenu(m)` | 挂菜单（nil 移除）；之后对 `m` 的改动要调用 `m.Update()` |
| `SetMenuOn(trigger)` | 哪种点击打开菜单 |
| `OnClick` / `OnDoubleClick` / `OnRightClick` | 主键单击 / 双击 / 副键单击回调 |
| `SetIcon(png, template)` / `SetMacIcon(png)` / `SetTooltip` / `SetTitle` | 修改外观 |
| `Bounds() (Rect, bool)` | 图标在屏幕上的矩形（平台知道时），用于在图标下方放弹出窗口 |
| `Remove()` | 移除图标，之后 Tray 不能再用 |
| `MenuOnDefault` `MenuOnPrimary` `MenuOnSecondary` | 菜单触发方式 |

完整签名见 [API 参考](/reference/api/sys-tray)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ | ✓ | ✓（需要 AppIndicator 库；不报告点击和位置） | — | — |

> Windows、Linux 实现尚未实机验证。

**点击与菜单**：挂了菜单时，默认在 macOS、Linux 上左键打开菜单，Windows 上右键打开；`OnClick` 只在平台能区分两者时触发（Windows 左键）。`MenuOnSecondary` 让右键开菜单、左键走 `OnClick`（macOS、Windows）；`MenuOnPrimary` 在所有平台左键开菜单。没有菜单时所有点击都交给回调。`OnDoubleClick` 在 Windows 上触发；macOS 上在两次单击之后报告。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type TrayAPI struct{ t *tray.Tray }

func (x TrayAPI) SetTooltip(s string) { x.t.SetTooltip(s) }

bind.Register(r, "Tray", TrayAPI{t})
```

## 注意

- 所有方法可在任意 goroutine 调用；回调在主线程运行。
- 平台没有托盘，或 Linux 上没装 AppIndicator 库时，`New` 返回 `ErrUnsupported`。
- Linux AppIndicator 不区分左右键，任何点击都打开菜单，`MenuOn` 不起作用；在 Linux 上请务必设置菜单。
- 只用托盘、不要 Dock 图标的 macOS 应用，配合 [appctl](/guide/system/appctl) 的 `SetDockVisible(app, false)`。
