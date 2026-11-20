---
title: Mobile Popup 弹出层
---
# Mobile Popup 弹出层

在当前页面上方显示带遮罩的滑入面板，可用于底部抽屉、侧边面板和临时内容。

## 何时使用

- 需要承载一段自定义内容并暂时阻断背景操作时使用 `Popup`。
- 只需要确认、取消语义时优先使用 `Dialog`；一组动作使用 `ActionSheet`。
- 轻量的短消息使用 `Toast`，不要用 Popup 模拟 Toast。
- `open` 指针由页面状态持有，Popup 只负责展示和关闭反馈。

## 在线体验

真实 gallery 展示底部 Popup 和 Dialog；可切换主题、明暗模式并打开浮层。

<GalleryPreview component="mobile.Popup" />

## 基本用法

Popup 的内容通过 `Body` 构建。遮罩默认显示，点击遮罩或按 Esc 可以关闭。用 `CloseOnMask(false)` 禁止点击遮罩关闭。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/mobile"
)

var open bool

func page(c *ui.Context) {
    if mobile.Button(c, "打开面板").Primary().Clicked() {
        open = true
    }
    mobile.Popup(c, &open).
        A11yLabel("设置面板").
        CloseOnMask(true).
        Body(func() {
            ui.Column(c, func() {
                ui.Text(c, "面板内容")
                mobile.Button(c, "完成").Block().Clicked()
            }, layout.Pad(16))
        })
}
```

## 常见用法

### 设置位置和圆角

`Position` 支持底部、顶部、左右物理边和逻辑起止边；底部面板可打开 `Handle`。

```go
func bottom(c *ui.Context, open *bool) {
    mobile.Popup(c, open).
        Position(mobile.PosBottom).
        Handle(true).
        Round(true).
        Body(func() { ui.Text(c, "底部内容") })
}
```

### 侧边面板

侧边 Popup 适合筛选或导航内容，逻辑位置用 `PosStart`、`PosEnd` 可随 RTL 自动适配。

```go
func side(c *ui.Context, open *bool) {
    mobile.Popup(c, open).
        Position(mobile.PosEnd).
        Layout(layout.W(300)).
        Body(func() { mobile.List(c, "筛选").Body(func() { mobile.Cell(c, "全部") }) })
}
```

### 自定义遮罩行为

关闭遮罩后，必须提供明确的关闭控件或由业务状态关闭；`Closed` 可观察本帧关闭事件。

```go
func explicitClose(c *ui.Context, open *bool) {
    popup := mobile.Popup(c, open).Mask(true).CloseOnMask(false).Body(func() {
        if mobile.Button(c, "关闭").Clicked() {
            *open = false
        }
    })
    if popup.Closed() {
        ui.Text(c, "面板已关闭")
    }
}
```

### 升级为 Dialog 或 ActionSheet

同一页面的浮层按交互语义选组件，而不是把所有内容都放进 Popup。

```go
func semanticOverlay(c *ui.Context, dialogOpen, actionsOpen *bool) {
    mobile.Dialog(c, dialogOpen, "删除确认").Content("删除后无法恢复").Confirm("删除", "取消").Show()
    mobile.ActionSheet(c, actionsOpen, []mobile.Action{{Text: "编辑"}, {Text: "删除", Danger: true}}).Show()
}
```

## 主题与样式

Popup 的遮罩、面板背景、圆角和拖拽把手读取移动主题令牌。shadcn 风格使用带把手的底部 sheet 语义；Ant Mobile 和 Vant 保持各自的面板间距。自定义背景可用 `Bg`，但优先使用 `mobile.ThemeOf(c)` 的颜色。

## 注意事项

`open` 必须是稳定状态，不能在每帧声明为 false。遮罩与键盘焦点由 Popup 处理；关闭时清理业务状态或异步任务。为无标题的面板设置 `A11yLabel`，为危险动作使用 Dialog 的确认语义。RTL 下 `PosStart`、`PosEnd` 是逻辑边，`PosLeft`、`PosRight` 是物理边。

## 示例代码

<!--@include: ./_gen/mobile-popup-examples.md-->

## API

<!--@include: ./_gen/mobile-popup-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/overlay.go)
- [PickerView 选择器视图](/components/mobile-pickerview)
- [Button 按钮](/components/mobile-button)
- [ui mobile Go API](/reference/api/ui-mobile)
