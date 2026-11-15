---
title: Tour 漫游式引导
---
# Tour 漫游式引导

通过遮罩和分步浮层引导用户认识页面中的关键功能。

## 何时使用

- 新功能首次使用时，需要按顺序解释多个真实控件。
- 目标分散在页面不同位置，需要自动滚动并逐步聚焦时。
- 只需解释一个控件时用 [Tooltip](/components/tooltip)；需要复杂交互或确认时用 [Popover](/components/popover)。

## 在线体验

<GalleryPreview component="Tour" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var tourOpen bool
var tourCurrent int

func tourPage(c *ui.Context) {
    upload := ui.Button(c, "上传").ID()
    steps := []ui.TourStep{{
        Target: upload, Title: "上传文件", Description: "从这里选择要导入的文件。",
    }}
    if ui.Button(c, "开始引导").Clicked() {
        tourOpen = true
        tourCurrent = 0
    }
    ui.Tour(c, &tourOpen, &tourCurrent, steps).Show()
}
```

`Tour` 绑定打开状态、当前步骤和步骤切片。目标控件通过其 `ID()` 传入；`Target` 为零时显示居中面板。

## 常见用法

### 多步骤与位置

```go
var tourOpen2 bool
var tourCurrent2 int

func tourSteps(c *ui.Context) {
    first := ui.Button(c, "第一步").ID()
    second := ui.Button(c, "第二步").ID()
    steps := []ui.TourStep{
        {Target: first, Title: "创建", Description: "先创建一条记录。"},
        {Target: second, Title: "保存", Description: "完成后保存。", Placement: "right", PrevText: "返回"},
    }
    ui.Tour(c, &tourOpen2, &tourCurrent2, steps).Primary().Gap(8).Show()
}
```

`Placement` 使用 Popover 的位置字符串；`Primary` 使用主色面板，`Gap` 设置目标洞口的留白。

### 自定义操作和指示器

```go
func tourRender(c *ui.Context) {
    target := ui.Button(c, "功能").ID()
    steps := []ui.TourStep{{Target: target, Title: "功能", Description: "这里是关键入口。"}}
    ui.Tour(c, &tourOpen2, &tourCurrent2, steps).
        IndicatorsRender(func(current, total int) {
            ui.Text(c, "步骤").Tertiary()
        }).ActionsRender(func(a *ui.TourActions) {
            a.Default()
        }).Show()
}
```

`IndicatorsRender` 和 `ActionsRender` 替换默认区域。自定义操作中可调用 `a.Prev()`、`a.Next()`、`a.GoTo(i)` 或 `a.Close()`。

### 每步覆盖选项

```go
func tourStepOptions(c *ui.Context) {
    target := ui.Button(c, "更多").ID()
    steps := []ui.TourStep{{
        Target: target, Title: "更多操作", Description: "这里没有遮罩。",
        Mask: ui.TourMaskOff, Type: ui.TourTypePrimary,
        HidePrev: true, NextText: "知道了",
    }}
    ui.Tour(c, &tourOpen2, &tourCurrent2, steps).Show()
}
```

`Mask`、`Type`、按钮文案和隐藏按钮都可在单步设置，覆盖 Tour 的默认选项。

### 结束和关闭

```go
func tourEvents(c *ui.Context) {
    target := ui.Button(c, "目标").ID()
    tour := ui.Tour(c, &tourOpen2, &tourCurrent2, []ui.TourStep{{Target: target, Title: "提示"}}).Show()
    if tour.Closed() || tour.Finished() {
        tourOpen2 = false
    }
}
```

`Show()` 返回 Tour 实例，可在同一帧读取 `Closed()` 或 `Finished()`；打开状态和当前步骤必须放在函数外。Esc 和关闭按钮都会产生关闭事件。

## 主题与样式

Tour 使用 Popover 的定位语义，并由主题提供面板、遮罩、箭头和按钮样式。`Primary()`、`NoMask()`、`NoArrow()` 是页面级选项；不同主题会调整主色和阴影，但不改变步骤数据结构。

## 注意事项

目标控件必须在同一帧先构建并取得 `ID()`，再构建 Tour；动态列表目标变化时要重新绑定正确 ID。步骤过多会降低理解度，建议只覆盖关键路径。遮罩开启时应确保目标和操作按钮仍可键盘访问；目标被销毁时应关闭或跳过该步骤。滚动默认会把目标带入可视区域，可用 `ScrollIntoView(false)` 关闭。

## 示例代码

<!--@include: ./_gen/tour-examples.md-->

## API

<!--@include: ./_gen/tour-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_popover.go)
- [ui Go API](/reference/api/ui)
- [Popover 气泡卡片](/components/popover)
- [Tooltip 文字提示](/components/tooltip)
