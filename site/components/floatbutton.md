---
title: FloatButton 悬浮按钮
---
# FloatButton 悬浮按钮

固定在窗口边缘、提供快速操作的悬浮按钮，也支持带描述、徽标和展开菜单的按钮组。

## 何时使用

- 页面需要一个不随内容滚动、始终可达的帮助、反馈、创建或快捷入口。
- 多个低频操作需要收纳到一个角落时，用 `FloatButtonGroup`。
- 需要回到滚动容器顶部时，用 `BackTop`；不要自己用 FloatButton 模拟滚动逻辑。
- 只在当前内容流中操作的按钮，应使用 [Button 按钮](/components/button)，不要把所有操作都做成悬浮按钮。

## 在线体验

gallery 展示右下角悬浮按钮、方形组、带描述与徽标的按钮组，以及点击触发展开和常驻展开两种组模式。

<GalleryPreview component="FloatButton" />

## 基本用法

`FloatButton` 默认使用 `icon.Plus`，位于窗口右下角，距离右侧 24px、底部 48px。用 `Icon` 和 `Tooltip` 为图标操作提供语义。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func floatButtonPage(c *ui.Context) {
    if ui.FloatButton(c).Icon(icon.Question).Tooltip("帮助").Primary().Clicked() {
        ui.Message(c, "success", "已打开帮助")
    }
}
```

浮动按钮的点击结果只对当前帧有效；打开面板、计数或跳转等业务状态应由页面变量保存。

## 常见用法

### 位置与主色

`Right` 和 `Bottom` 设置到窗口逻辑末端的距离；`Primary` 使用品牌色，`Square` 改为圆角方形。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func floatPositionPage(c *ui.Context) {
    ui.FloatButton(c).Icon(icon.Question).Primary().Tooltip("帮助").Right(24).Bottom(40)
    ui.FloatButton(c).Icon(icon.Setting).Square().Tooltip("设置").Right(88).Bottom(40)
}
```

在 RTL 作用域中，`Right` 表示逻辑末端，组件会把悬浮位置镜像到对应一侧。

### 描述与徽标

`Description` 在图标下方显示短标题并自动使用方形外观；`Badge(n)` 显示数量，传入负数时显示点徽标。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func floatStatusPage(c *ui.Context) {
    ui.FloatButton(c).Icon(icon.Message).Description("客服").Badge(5).Right(24).Bottom(40)
    ui.FloatButton(c).Icon(icon.Bell).Badge(-1).Tooltip("有新通知").Right(88).Bottom(40)
}
```

### 常驻按钮组

把多个 `FloatButton` 放进 `FloatButtonGroup.Body` 会在同一位置垂直排列。组上的 `Square` 和 `Primary` 会影响组的外观。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func floatGroupPage(c *ui.Context) {
    ui.FloatButtonGroup(c).Square().Position(120, 40).Body(func() {
        ui.FloatButton(c).Icon(icon.Question).A11yLabel("帮助")
        ui.FloatButton(c).Icon(icon.Message).A11yLabel("消息").Badge(-1)
        ui.FloatButton(c).Icon(icon.Setting).A11yLabel("设置")
    })
}
```

### 点击触发展开

`Trigger("click")` 将成员收纳到触发按钮后；`Open(&open)` 把展开状态交给调用方，适合在点击成员后执行操作或与其他面板联动。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var floatOpen bool

func floatTriggerPage(c *ui.Context) {
    ui.FloatButtonGroup(c).Trigger("click").Open(&floatOpen).Primary().Position(200, 40).Body(func() {
        if ui.FloatButton(c).Icon(icon.Message).A11yLabel("消息").Clicked() {
            floatOpen = false
        }
        ui.FloatButton(c).Icon(icon.Edit).A11yLabel("编辑")
    })
}
```

### 默认属性

`ConfigProvider.Defaults("FloatButton", ...)` 可统一设置形状和类型；单个按钮的 `Square` 与 `Primary` 会覆盖作用域默认值。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func floatDefaultsPage(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("FloatButton", ui.Props{
        "shape": "square",
        "type":  "primary",
    }).Body(func() {
        ui.FloatButton(c).Icon(icon.Plus).A11yLabel("新增")
    })
}
```

## 主题与样式

- Ant Design 主题使用圆形默认外观、阴影和品牌色主按钮；Element Plus 按悬浮按钮的圆角方形语义展示；shadcn 采用页面背景上的浮动卡片样式。
- 组件令牌位于 `theme.Components.FloatButton`（`theme.FloatButtonToken`），包括 `Size`、`DescriptionHeight`、`IconSize`、`DescriptionIconSize`、`Radius`、`Shadow`、`Bg`、`HoverBg`、`PrimaryBg` 和 `PrimaryHoverBg`。
- `ConfigProvider.Defaults("FloatButton", ...)` 支持 `shape`（`"circle"` / `"square"`）和 `type`（`"default"` / `"primary"`）。

## 注意事项

- **状态放在哪**：`Open(&open)` 使用调用方持有的 bool；如果不绑定，组会在当前作用域内部保存展开状态。业务弹窗、未读数和点击结果仍应由页面状态保存。
- **无障碍**：图标按钮无可见文字时必须调用 `A11yLabel` 或 `Tooltip`；组内成员也要分别命名。键盘用户可以聚焦按钮并使用 Enter 或空格激活。
- **层级与遮挡**：悬浮按钮位于窗口浮层，不会随普通内容滚动；不要把它放在需要被滚动容器裁剪的布局假设中。
- **RTL**：位置使用逻辑末端，徽标和提示方向也随作用域方向镜像。

## 示例代码

<!--@include: ./_gen/floatbutton-examples.md-->

## API

<!--@include: ./_gen/floatbutton-api.md-->

## 相关链接

- 源码：[ui/nav_float.go](https://github.com/tangoui/tango/blob/main/ui/nav_float.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Button 按钮](/components/button)、[Anchor 锚点](/components/anchor)、[Icon 图标](/components/icon)
