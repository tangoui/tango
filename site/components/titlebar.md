---
title: TitleBar 标题栏
---
# TitleBar 标题栏

无边框窗口的自绘标题栏：应用图标、窗口标题、菜单、居中的搜索框、工具按钮和随平台变化的窗口按钮；空白处可以拖动窗口，双击最大化或还原。配套的 `WindowControls` 可单独使用，`WindowFrame` 是窗口接入的接口。

::: warning 实验性
自绘窗口目前只在 macOS 上实机验证，见 [自绘 UI](/guide/ui)。
:::

## 何时使用

- 窗口使用 `TitleBarNone`（无边框），或 macOS 上的 `TitleBarHidden`（内容延伸到标题栏下），需要自己画标题区域。
- 想把菜单栏、命令搜索框、布局开关、账号菜单放进标题栏，做成 VS Code / JetBrains 风格的一体化标题栏。
- 保留系统标题栏的普通窗口不需要它；页面内的顶部导航用 [Layout 布局](/components/layout) 的 Header 或 [Menubar 菜单栏](/components/menubar)。

## 在线体验

<GalleryPreview component="TitleBar" />

## 基本用法

先把窗口设为无边框，再在页面顶部放 `ui.TitleBar`。`uihost` 打开的窗口实现了 `ui.WindowFrame`，标题栏会自动找到它：空白处拖动移动窗口，双击最大化 / 还原，窗口按钮按平台显示。

```go
import (
    "github.com/tangoui/tango"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/uihost"
)

func page(c *ui.Context) {
    ui.Column(c, func() {
        ui.TitleBar(c).Icon(icon.Code).Title("Tango — main.go")
        ui.Column(c, func() {
            ui.Text(c, "窗口内容")
        }, layout.Grow(1), layout.Pad(16))
    }, layout.Fill())
}

func openMain(app *tango.App) error {
    _, _, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
        Title:      "Tango",
        Width:      960,
        Height:     640,
        Resizable:  true,
        Appearance: tango.Appearance{TitleBar: tango.TitleBarNone},
    }}, page, ui.RunnerOptions{})
    return err
}
```

标题栏的高度、颜色由主题决定；`Title` 同时作为标题栏的无障碍名称。没有 `WindowFrame`（例如预览或测试环境）时，标题栏仍会绘制并通过 `Action()` 报告操作。

## 常见用法

### 菜单、居中搜索与工具按钮

`Menu` 放在图标后面（通常是 [Menubar](/components/menubar)，它会自动去掉自己的底色、边框和阴影，与标题栏等高），`Center` 放在中间（命令搜索框），`Tools` 放在窗口按钮之前。没有 `Center` 时，标题在 macOS 上居中，其他平台跟在菜单后面。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var (
    titleQuery string
    titleMenus = []ui.MenubarMenu{
        {Key: "file", Label: "文件", Items: []ui.MenubarItem{{Key: "open", Label: "打开…", Shortcut: "Ctrl+O"}}},
        {Key: "view", Label: "视图", Items: []ui.MenubarItem{{Key: "full", Label: "全屏", Shortcut: "F11"}}},
    }
)

func titleSlots(c *ui.Context) {
    ui.TitleBar(c).Icon(icon.Code).Title("Tango IDE").
        Menu(func() { ui.Menubar(c, titleMenus).A11yLabel("应用菜单") }).
        Center(func() {
            ui.Search(c, &titleQuery).Placeholder("搜索文件、命令").Size(ui.Small).Width(280)
        }).
        Tools(func() {
            ui.Tooltip(c, "切换侧栏", func() { ui.IconButton(c, icon.PanelLeft, "切换侧栏") }).Placement("bottom")
            ui.Tooltip(c, "设置", func() { ui.IconButton(c, icon.Setting, "设置") }).Placement("bottom")
        })
}
```

### 窗口按钮样式

`Controls` 选择窗口按钮的样式，默认 `WindowControlsAuto` 跟随平台：

| 值 | 效果 |
| --- | --- |
| `ui.WindowControlsAuto` | macOS 在最左侧画红绿灯，其他平台在最右侧画最小化 / 最大化 / 关闭；窗口保留系统标题栏时不画；macOS 隐藏标题栏（系统自己画红绿灯）时只留出位置 |
| `ui.WindowControlsCaption` | 强制 Windows / Linux 风格的三个按钮，在最右侧 |
| `ui.WindowControlsTraffic` | 强制 macOS 红绿灯，在最左侧，悬停整组时显示符号 |
| `ui.WindowControlsNone` | 不画任何窗口按钮 |

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func titleControls(c *ui.Context) {
    ui.TitleBar(c).Icon(icon.Code).Title("Windows 风格").Controls(ui.WindowControlsCaption)
    ui.TitleBar(c).Title("macOS 风格").Controls(ui.WindowControlsTraffic)
}
```

按钮是否可用由窗口外观决定：`WindowFrameInfo` 的 `NoMinimize`、`NoMaximize`、`NoClose` 为 true 时对应按钮置灰，窗口最大化时中间的按钮变为“还原”。

### 关闭前确认：Manual 与 Action

`Manual()` 让标题栏只报告用户要求的窗口操作，不自己去操作窗口，适合关闭前要确认、或者要把操作转给别处的应用。`Action()` 返回这一帧的 `ui.WindowAction`（`WindowActionClose`、`WindowActionMinimize`、`WindowActionMaximize`、`WindowActionRestore`、`WindowActionFullscreen`），双击标题栏产生的最大化 / 还原也在其中。拖动窗口不受 `Manual` 影响。

```go
import "github.com/tangoui/tango/ui"

var confirmClose bool

func titleManual(c *ui.Context) {
    bar := ui.TitleBar(c).Title("未保存的文档").Manual()
    act := bar.Action()
    if act == ui.WindowActionClose {
        confirmClose = true // 弹出确认框；确认后自行调用 CloseWindow
    }
    // Manual 下最大化、还原等操作需要自己执行
    if f, ok := c.Window().(ui.WindowFrame); ok {
        switch act {
        case ui.WindowActionMaximize:
            f.SetWindowState(ui.WindowMaximized)
        case ui.WindowActionRestore:
            f.SetWindowState(ui.WindowNormal)
        case ui.WindowActionMinimize:
            f.SetWindowState(ui.WindowMinimized)
        case ui.WindowActionFullscreen:
            f.SetWindowState(ui.WindowFullscreen)
        }
    }
    if confirmClose {
        ui.Text(c, "有未保存的修改，确定关闭吗？")
    }
}
```

确认后调用 `f.CloseWindow()` 关闭窗口（和系统关闭按钮一样，窗口的关闭处理函数仍可否决）。

### 单独使用 WindowControls

自定义标题区域时，可以不用 `TitleBar`，只放 `ui.WindowControls`。它有和标题栏相同的 `Style`、`Frame`、`Manual`、`Action`，以及设置 Caption 按钮高度的 `Height`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func customHeader(c *ui.Context) {
    ui.Row(c, func() {
        ui.Title(c, "自定义标题区", 5)
        ui.Spacer(c)
        ui.WindowControls(c).Style(ui.WindowControlsCaption).Height(32)
    }, layout.Middle(), layout.Pad(8))
}
```

自定义区域需要自己负责拖动窗口，标题栏的拖动和双击逻辑只在 `TitleBar` 里。

### 接入自己的窗口：WindowFrame

标题栏通过 `ui.WindowFrame` 驱动窗口：`FrameInfo()` 描述当前状态，`SetWindowState` 最小化 / 最大化 / 全屏 / 还原，`CloseWindow` 关闭，`StartWindowMove` 在按下标题栏的那一刻把拖动交给窗口管理器。`uihost` 窗口已经实现，页面通过 `Context.Window` 自动取得；其他宿主、预览或测试用 `Frame(f)` 显式传入。所有方法都在 UI 线程运行。

```go
import "github.com/tangoui/tango/ui"

type memoryFrame struct {
    state ui.WindowState
    moves int
}

func (f *memoryFrame) FrameInfo() ui.WindowFrameInfo   { return ui.WindowFrameInfo{State: f.state} }
func (f *memoryFrame) SetWindowState(s ui.WindowState) { f.state = s }
func (f *memoryFrame) CloseWindow()                    {}
func (f *memoryFrame) StartWindowMove()                { f.moves++ }

var demoFrame memoryFrame

func titleFrame(c *ui.Context) {
    ui.TitleBar(c).Title("演示窗口").Frame(&demoFrame).Controls(ui.WindowControlsCaption)
}
```

`WindowFrameInfo` 中，`SystemButtons` 表示平台自己在页面上方画窗口按钮（macOS 隐藏标题栏的红绿灯），标题栏只留出位置，不再画按钮；`SystemTitleBar` 表示窗口仍保留系统标题栏，标题栏不画窗口按钮。

## 主题与样式

外观来自 `theme.Components.TitleBar`（`theme.TitleBarToken`）：`Height`、`Bg`、下边线 `BorderColor`、`PaddingInline`、`Gap`，图标与标题（`IconSize`、`TitleFontSize`、`TitleColor`、窗口失焦时的 `TitleInactiveColor`），为系统红绿灯留出的 `SystemInset`，Caption 按钮（`ControlWidth`、`ControlHoverBg`、`CloseHoverBg` 等），以及红绿灯（`TrafficSize`、`TrafficGap`、`TrafficClose` 等）。用 `ui.ConfigProvider(c).Components(theme.Components{TitleBar: ...})` 局部覆盖，零值字段沿用主题。窗口失去焦点时，标题和按钮图标会变淡。

- **Ant Design**：容器背景，下方一条分割线。
- **Element Plus**：边框色更浅，标题字重 500。
- **shadcn**：背景与边框对应 `bg-background` / `border-b`，标题字重 500，窗口按钮图标稍大、颜色偏弱，悬停变为 accent。

TitleBar 没有 `ConfigProvider.Defaults` 键。按钮和提示文案来自 `Locale.TitleBar`（窗口按钮组名称、最小化、最大化、还原），关闭、全屏等来自 `Locale` 通用文案。

## 注意事项

- **窗口要先设为无边框**：`Appearance.TitleBar` 取 `TitleBarNone`，或 macOS 的 `TitleBarHidden`；保留系统标题栏（默认）时再放一个自绘标题栏会出现两条标题栏。窗口外观的设置见 [窗口](/guide/window)。
- **放在页面顶部**：TitleBar 是一行固定高度的元素，放在页面最外层列的第一个子元素，下面的内容区用 `Grow(1)`。
- **拖动与双击**：只有按在标题栏没有控件的空白处才会拖动窗口；双击（450ms 内、位置接近）在窗口不禁用最大化时切换最大化 / 还原，窗口处于最小化或全屏时不响应。自绘菜单、搜索框、工具按钮自己消费点击，不会触发拖动。
- **状态**：窗口状态保存在窗口里，标题栏每帧通过 `FrameInfo()` 读取；不要在页面里另存一份“是否最大化”。
- **无障碍**：标题栏是以标题命名的分组，窗口按钮是带名称的按钮，可用 Tab 聚焦、回车 / 空格触发。
- **RTL**：红绿灯的顺序、菜单与插槽的排列按阅读方向镜像。

## 示例代码

<!--@include: ./_gen/titlebar-examples.md-->

## API

<!--@include: ./_gen/titlebar-api.md-->

## 相关链接

- [源码：titlebar.go](https://github.com/tangoui/tango/blob/main/ui/titlebar.go)、[window_frame.go](https://github.com/tangoui/tango/blob/main/ui/window_frame.go)
- [ui Go API](/reference/api/ui)
- [窗口外观与无边框窗口](/guide/window)
- [Menubar 菜单栏](/components/menubar)
- [StatusBar 状态栏](/components/statusbar)
