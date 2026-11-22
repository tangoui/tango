---
title: Splitter 分割面板
---
# Splitter 分割面板

用可拖动的分隔条把工作区拆成多个可调整大小的面板。

## 何时使用

- IDE、文件管理器、属性检查器等需要用户调整两栏或多栏宽度的工作区。
- 需要上下分栏或嵌套工作区时使用 `Vertical()` 和嵌套 `Splitter`。
- 只是排列固定宽度内容用 [Grid 栅格](/components/gridrow) 或 [Flex 弹性布局](/components/flex)，不要用 Splitter。
- 移动端窄屏优先使用单栏或抽屉，避免强迫用户拖动很窄的面板。

## 在线体验

<GalleryPreview component="Splitter" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func pane(c *ui.Context, label string) func() {
    return func() { ui.Column(c, func() { ui.Text(c, label) }, layout.Center()) }
}

func page(c *ui.Context) {
    ui.Splitter(c).Style(layout.H(240)).Body(func() {
        ui.SplitterPanel(c).DefaultPercent(0.3).Min(160).Body(pane(c, "导航"))
        ui.SplitterPanel(c).Body(pane(c, "编辑区"))
    })
}
```

`SplitterPanel` 必须在 `Splitter.Body` 中创建。第一个面板初始占 30%，并限制最小宽度；未设置比例的面板会分配剩余空间。

## 常见用法

### 可折叠侧栏

`Collapsible()` 显示相邻分隔条的折叠箭头；`CollapseToggle` 只显示一个带提示的箭头，适合 IDE 侧栏。

在可折叠面板上调用 `AlwaysShowCollapse(true)` 可让相邻分隔条始终显示折叠按钮。对应 antd 6.7 的 `collapsible.showCollapsibleIcon: true`，在 Tango 中也可用于单箭头的 `CollapseToggle`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Splitter(c).Style(layout.H(200)).Body(func() {
        ui.SplitterPanel(c).DefaultPercent(0.28).Min(120).
            CollapseToggle("折叠工程侧栏", "展开工程侧栏").Body(func() { ui.Text(c, "工程") })
        ui.SplitterPanel(c).Body(func() { ui.Text(c, "编辑器") })
    })
}
```

### 固定尺寸与范围

`DefaultSize` 以像素指定初始尺寸，`Min` 和 `Max` 限制拖动范围；`Resizable(false)` 会锁住该面板旁的分隔条。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Splitter(c).Style(layout.H(220)).Body(func() {
        ui.SplitterPanel(c).DefaultSize(240).Min(180).Max(360).Body(func() { ui.Text(c, "属性") })
        ui.SplitterPanel(c).Resizable(false).Body(func() { ui.Text(c, "预览") })
    })
}
```

### 竖向与嵌套

`Vertical()` 将面板上下排列。面板 Body 内可以再创建一个 Splitter，用于“上方预览、下方左右分栏”等复杂工作区。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Splitter(c).Vertical().Style(layout.H(300)).Body(func() {
        ui.SplitterPanel(c).DefaultPercent(0.4).Body(func() { ui.Text(c, "上方") })
        ui.SplitterPanel(c).Body(func() {
            ui.Splitter(c).Style(layout.Grow(1)).Body(func() {
                ui.SplitterPanel(c).Body(func() { ui.Text(c, "左下") })
                ui.SplitterPanel(c).Body(func() { ui.Text(c, "右下") })
            })
        })
    })
}
```

### 延迟提交与拖拽事件

`Lazy()` 拖动时只显示预览线，松手才提交比例。`DragStarted`、`DragEnded` 可记录交互或触发松手后的计算。

```go
import (
    "fmt"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var dragMessage = "拖动分隔条试试"

func page(c *ui.Context) {
    sp := ui.Splitter(c).Lazy().Style(layout.H(160))
    sp.Body(func() {
        ui.SplitterPanel(c).DefaultPercent(0.4).Min(80).Body(func() { ui.Text(c, "预览") })
        ui.SplitterPanel(c).Min(80).Body(func() { ui.Text(c, "提交") })
    })
    if _, ok := sp.DragEnded(); ok {
        sizes := sp.Sizes()
        dragMessage = fmt.Sprintf("已提交：%.0f%% / %.0f%%", sizes[0]*100, sizes[1]*100)
    }
    ui.Text(c, dragMessage)
}
```

## 主题与样式

分隔条宽度、悬停色、焦点色和折叠箭头由 `theme.Components.Splitter` 控制。Ant 风格强调可拖拽的中性分隔条，Element Plus 更轻，shadcn 使用低对比边框；面板内容本身的背景和内边距应由面板 Body 设置。

## 注意事项

- 外层必须有确定的尺寸（例如 `layout.H(240)` 或 `layout.Grow(1)`），否则拖动比例没有可用空间。
- `Sizes()` 返回各面板的比例；它是当前帧的结果，不要在每帧把它重新写回 `DefaultPercent`。
- 给每个面板设置合理的 `Min`，避免键盘调整或窄窗口下内容不可读；键盘可聚焦分隔条并调整尺寸。
- 折叠箭头的提示文本同时用于屏幕阅读器名称；自定义 `CollapseToggle` 时应传入有意义的折叠与展开文案。
- RTL 下分隔条方向和箭头会自动镜像。

## 示例代码

<!--@include: ./_gen/splitter-examples.md-->

## API

<!--@include: ./_gen/splitter-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_splitter.go)
- [ui Go API](/reference/api/ui)
- [Layout 布局](/components/layout)
