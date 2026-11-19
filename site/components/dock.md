---
title: Dock 停靠布局
---
# Dock 停靠布局

IDE 工作台式的停靠布局：嵌套的水平 / 垂直分割，叶子是标签组；用户可以拖动标签页移动、拆分、合并，拖动分割条调整大小，最大化某个组，也可以把标签页撕出窗口。布局数据由应用持有，可以序列化为 JSON 保存和恢复。

::: warning 实验性
`Dock` 目前标注为 Experimental，API 在任何 0.x 版本中都可能调整。
:::

## 何时使用

- 编辑器、终端、问题面板、大纲这类需要用户自己重排的工作台，并且要记住上次的布局。
- 标签页需要在多个组之间拖动、拆分成并排的编辑器，或撕出成独立窗口。
- 只需要一行标签切换内容用 [Tabs 标签页](/components/tabs)；只需要固定的两栏 / 三栏可拖动分割用 [Splitter 分割面板](/components/splitter)。

## 在线体验

<GalleryPreview component="Dock" />

## 基本用法

用 `ui.NewDockLayout` 创建布局（所有标签先放进同一个组），交给 `ui.Dock`；`Content` 为每个组当前激活的标签页构建内容。布局变量放在包级，`Dock` 会就地修改它。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var workbench = ui.NewDockLayout("main.go", "README.md", "terminal")

func page(c *ui.Context) {
    ui.Dock(c, workbench).
        Content(func(key string) {
            ui.Column(c, func() {
                ui.Text(c, "当前标签："+key)
            }, layout.Pad(12))
        }).
        Style(layout.H(360)).
        Show()
}
```

`Style` 给 Dock 一个大小：固定高度用 `layout.H(360)`，撑满窗口用 `layout.Grow(1)`。所有选项都要在 `Show()` 之前设置。没有 `Tab` 回调时，标签的文字就是它的 key。

## 常见用法

### 标签的标题与图标

`Tab` 回调按 key 返回 `ui.TabItem`：`Label`、`Icon`，`NoClose` 让该标签没有关闭按钮，`Disabled` 禁用。`Key` 字段会被忽略，以布局里的 key 为准。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var tabInfo = map[string]ui.TabItem{
    "main.go":  {Label: "main.go", Icon: icon.Code},
    "terminal": {Label: "终端", Icon: icon.Terminal, NoClose: true},
}

var tabbed = ui.NewDockLayout("main.go", "terminal")

func dockTabs(c *ui.Context) {
    ui.Dock(c, tabbed).
        Tab(func(key string) ui.TabItem { return tabInfo[key] }).
        Content(func(key string) { ui.Text(c, key) }).
        Style(layout.H(320)).
        Show()
}
```

### 预设初始布局

`DockLayout` 提供一组修改方法，可以在启动时把标签摆成想要的样子。`SplitTab(key, group, side)` 把标签拆到目标组旁边的新组并返回新组的 ID，`side` 取 `DockLeft`、`DockRight`、`DockTop`、`DockBottom`（`DockCenter` 表示并入该组）；`MoveTab` 把标签移到指定组的指定位置（负数追加到末尾）；`Activate` 激活标签并聚焦它所在的组；分割后各子节点的占比放在 `Root.Sizes`（总和为 1）。

```go
import "github.com/tangoui/tango/ui"

func presetLayout() *ui.DockLayout {
    l := ui.NewDockLayout("main.go", "dock.go", "terminal", "output", "outline")
    l.SplitTab("terminal", "g1", ui.DockRight) // 新组 g2 在 g1 右侧
    l.MoveTab("output", "g2", -1)              // 输出放进 g2
    l.SplitTab("outline", "g2", ui.DockBottom) // 大纲在 g2 下方
    l.Root.Sizes = []float64{0.62, 0.38}
    l.Activate("main.go")
    return l
}

var preset = presetLayout()
```

`NewDockLayout` 创建的第一个组 ID 是 `g1`，之后拆出的组依次是 `g2`、`g3`……。没有被任何组引用的 key 不会出现在界面上，需要显示新标签时用 `AddTab`。

### 保存与恢复布局

`DockLayout` 能直接用 `encoding/json` 序列化。`Changed()` 在布局发生任何编辑（移动、拆分、关闭、调整大小、最大化、聚焦、切换激活标签）的那一帧返回 true，正好用来保存；启动时用 `ui.ParseDockLayout` 读回，它会自动修复重复 key、错误的占比、缺失的组 ID 等问题，手改或旧版本的文件也能加载。

```go
import (
    "encoding/json"
    "os"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

const layoutFile = "layout.json"

func loadLayout() *ui.DockLayout {
    if data, err := os.ReadFile(layoutFile); err == nil {
        if l, err := ui.ParseDockLayout(data); err == nil {
            return l
        }
    }
    return ui.NewDockLayout("main.go", "README.md", "terminal")
}

var saved = loadLayout()

func dockSaved(c *ui.Context) {
    d := ui.Dock(c, saved).
        Content(func(key string) { ui.Text(c, key) }).
        Style(layout.Grow(1)).
        Show()
    if d.Changed() {
        if data, err := json.Marshal(saved); err == nil {
            _ = os.WriteFile(layoutFile, data, 0o644)
        }
    }
}
```

恢复后，布局里可能缺少应用新增的标签，或仍留着已经不存在的标签：启动时遍历你的标签 key，对 `GroupOf(key) == nil` 的调用 `AddTab(key, "")`，对不再提供的调用 `RemoveTab`。

### 打开、关闭与拦截关闭

标签的关闭按钮（或选中后按 Delete）会把它从布局里移除，一个组空了就消失，邻居接管它的空间。`BeforeClose` 在关闭前被询问，返回 false 保留标签（例如未保存的文档）；这一帧实际关闭的标签 key 从 `Closed()` 读取。程序里打开标签用 `AddTab(key, group)`，group 传 `""` 表示当前聚焦的组；key 已经在布局中时只会激活它。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var (
    editor   = ui.NewDockLayout("main.go", "notes.txt")
    dirty    = map[string]bool{"notes.txt": true}
    lastShut string
)

func dockClosing(c *ui.Context) {
    if ui.Button(c, "打开 README").Clicked() {
        editor.AddTab("README.md", "")
    }
    d := ui.Dock(c, editor).
        Content(func(key string) { ui.Text(c, key) }).
        BeforeClose(func(key string) bool { return !dirty[key] }).
        Style(layout.H(320)).
        Show()
    if k := d.Closed(); k != "" {
        lastShut = k
    }
    ui.Text(c, "最近关闭："+lastShut).Tertiary()
}
```

### 拖放、拆分与最大化

这些交互不需要写代码，由 Dock 内部处理：

- 把标签拖到另一个组的**标签栏**上，移到该组并插在落点位置（同一组内拖动就是排序）。
- 把标签拖到另一个组**内容区**的上、下、左、右四分之一边缘，会在那一侧拆出新组；拖到中间则并入该组。拖动过程中会显示落点预览。
- 拖动组之间的分割条调整大小，每个组至少保留 `MinPaneSize` 的空间；分割条获得焦点后可以用方向键微调。
- 组头部右侧有两个按钮：向右拆分当前标签（组内至少两个标签时），以及最大化 / 还原该组（有多个组时）。

程序里也可以直接改字段：`workbench.Maximized = "g2"` 让指定的组铺满整个 Dock，设为 `""` 还原；`workbench.Focus` 是聚焦组的 ID，新标签默认进入这个组。

### 撕出为独立窗口

调用 `OnTearOff` 后，标签页拖出窗口并松开时，它会从布局里移除，回调收到标签 key 和松开位置（窗口坐标），由应用打开新窗口承载它。不设置 `OnTearOff` 时，拖到窗口外松开不会发生任何事。本帧也可以用 `TornOff()` 读取撕出事件。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/geom"
    "github.com/tangoui/tango/ui/layout"
)

var (
    tearable = ui.NewDockLayout("main.go", "terminal")
    popped   []string
)

func dockTear(c *ui.Context) {
    d := ui.Dock(c, tearable).
        Content(func(key string) { ui.Text(c, key) }).
        OnTearOff(func(key string, pos geom.Point) {
            // 这里在 pos 附近打开一个窗口显示 key；窗口关闭后用 AddTab 放回
            popped = append(popped, key)
        }).
        Style(layout.H(320)).
        Show()
    if key, _, ok := d.TornOff(); ok {
        _ = key // 同一帧也能读到撕出的标签
    }
}
```

回调在构建界面的过程中被调用，不要在里面直接打开窗口，而是用 `app.Post` 送到主线程去开（`uihost.Open` 必须在主线程调用；macOS 上页面运行在窗口自己的 UI 线程，不是主线程）；完整的“撕出 + 停靠回主窗口”流程见 `examples/dock-demo`。

## 主题与样式

Dock 的外观由 `theme.Components.Dock`（`theme.DockToken`）控制：分割条之间的底色 `Bg`、组内容区 `GroupBg`、标签栏 `HeaderBg` / `HeaderBorder`、聚焦组顶部高亮线 `ActiveBorder`、分割条命中宽度 `GutterSize`、最小面板尺寸 `MinPaneSize`、拖放预览 `DropBg` / `DropBorder`、拖动中的标签 `GhostBg` 等。分割条悬停和拖动的高亮取自 `Splitter` 令牌。可以用 `ui.ConfigProvider(c).Components(theme.Components{Dock: ...})` 局部覆盖，零值字段沿用主题。

- **Ant Design / Element Plus**：分割条之间露出布局底色，标签栏用浅填充，聚焦的组在标签栏顶部有主色细线。
- **shadcn**：分割条之间是边框色，标签栏为容器背景并带下边线，聚焦高亮和拖放预览使用文字色。

Dock 没有 `ConfigProvider.Defaults` 键。组头部按钮的提示、空组的文案和组的无障碍名称来自 `Locale.Dock`（`Maximize`、`Restore`、`Split`、`Empty`、`Group`），随语言设置切换。

## 注意事项

- **布局由应用持有**：`*ui.DockLayout` 放在包级变量或应用状态里，不要在页面函数里每帧新建，否则每帧都会回到初始布局。Dock 就地修改它，也会在每帧开头调用 `Normalize()` 修复你手工编辑造成的不一致；直接改 `Root`、`Sizes`、`Tabs` 之后同样建议自己调用一次。
- **标签 key 要稳定且唯一**：key 是标签的身份，布局里每个 key 只能出现一次，也不要拿会变化的文字当 key。标签内容的控件状态按 key 保留，标签被移到另一个组时输入框内容、光标等状态会跟着走；但没有被激活的标签不会被构建，业务数据（文档内容、滚动位置）应放在你自己的变量里，不要依赖控件内部状态。
- **`Content` 每个可见组只运行一次**：只构建每组当前激活的标签；组为空时显示空组提示。
- **给 Dock 尺寸**：Dock 没有固有高度，必须用 `Style` 设置 `layout.H(...)` 或 `layout.Grow(1)`（父容器要有确定的高度），否则看不到内容。
- **无障碍与键盘**：每个组是一个以激活标签名为名称的分组；分割条是带 0–100 数值的分隔符，聚焦后可用方向键调整；标签栏复用 Tabs，标签可聚焦、可用键盘关闭和排序。
- **RTL**：`DockLeft` / `DockRight` 是逻辑方向，在从右到左的环境里分别表示右侧和左侧，拖放落点区域和分割条方向相应镜像。

## 示例代码

<!--@include: ./_gen/dock-examples.md-->

## API

<!--@include: ./_gen/dock-api.md-->

## 相关链接

- [源码：dock.go](https://github.com/tangoui/tango/blob/main/ui/dock.go)、[dock_layout.go](https://github.com/tangoui/tango/blob/main/ui/dock_layout.go)
- 完整示例：[examples/dock-demo](https://github.com/tangoui/tango/blob/main/examples/dock-demo/main.go)
- [ui Go API](/reference/api/ui)
- [Tabs 标签页](/components/tabs)
- [Splitter 分割面板](/components/splitter)
