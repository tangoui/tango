---
title: Divider 分割线
---
# Divider 分割线

分隔页面中的内容、章节或一组内联操作，可选地在水平线中间显示标题。

## 何时使用

- 在同一容器中分隔彼此独立的内容区块。
- 需要带标题的章节分隔线，或需要虚线、左右对齐的标题时使用。
- 在一行链接、标签或操作之间放短的竖向分隔线时使用 `VDivider`。
- 只是需要留白而不是视觉分组时，用 `Gap`、`Space` 或布局间距，不要堆叠分割线。

## 在线体验

基础 gallery 展示无标题、带标题和竖向分割线；进阶场景还展示虚线和标题位置。

<GalleryPreview component="Divider" />

## 基本用法

无标题的 `Divider` 绘制水平分割线；传入可变数量的字符串参数时，参数会以空格连接为标题。

```go
import "github.com/tangoui/tango/ui"

func dividerPage(c *ui.Context) {
    ui.Text(c, "上方内容")
    ui.Divider(c)
    ui.Text(c, "下方内容")
    ui.Divider(c, "章节标题")
}
```

`Divider` 返回 `*ui.DividerW`，可以继续使用分割线方法或通用 `Box` 的布局方法。

## 常见用法

### 虚线与标题位置

`Dashed()` 把实线改为虚线，`Variant` 可选 `"solid"`、`"dashed"` 或 `"dotted"`。`Orientation("vertical")` 创建无标题的竖分隔线，默认方向是水平。水平标题用 `TitlePlacement` 放在逻辑起始或结束边（`"start"` / `"end"`）、物理左或右（`"left"` / `"right"`）或中央（`"center"`，默认）。`Plain()` 使用普通文本字重，`Size` 调整水平分隔线的上下间距。

```go
import "github.com/tangoui/tango/ui"

func dividerVariantsPage(c *ui.Context) {
    ui.Divider(c, "左侧标题").TitlePlacement("left")
    ui.Divider(c, "右侧标题").TitlePlacement("right").Plain()
    ui.Divider(c, "居中虚线").Dashed()
}
```

### 内联操作的竖向分隔

`VDivider` 是一个固定高度的短竖线，不接收标题，也不需要保存状态。将它放进 `Row` 或 `Space` 并设置垂直居中即可。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func verticalDividerPage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Link(c, "编辑")
        ui.VDivider(c)
        ui.Link(c, "删除")
        ui.VDivider(c)
        ui.Link(c, "更多")
    }, layout.Middle())
}
```

### 在卡片内容中分组

分割线适合把同一张卡片中的摘要、正文和操作区分开；如果内容本来已经由多张卡片分组，不要再为每个卡片之间添加 Divider。

```go
import "github.com/tangoui/tango/ui"

func dividerCardPage(c *ui.Context) {
    ui.Card(c, "发布信息").Body(func() {
        ui.Text(c, "版本 1.4.0")
        ui.Divider(c, "变更说明").Plain()
        ui.Text(c, "修复同步和缓存问题。")
    })
}
```

### 全局默认值

可以用 `ConfigProvider.Defaults` 给作用域内的 Divider 设置默认虚线、标题位置或普通字重；单个实例的链式方法会覆盖默认值。

```go
import "github.com/tangoui/tango/ui"

func dividerDefaultsPage(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("Divider", ui.Props{
        "dashed":      true,
        "titlePlacement": "left",
        "plain":       true,
    }).Body(func() {
        ui.Divider(c, "默认左对齐虚线")
    })
}
```

## 主题与样式

- Ant Design 使用主题的分割线色、标题间距和标题字重；Element Plus 对应 `el-divider` 的内容位置语义；shadcn 使用较轻的边框色和普通文本样式。
- Divider 的令牌位于 `theme.Components.Divider`（`theme.DividerToken`），包括 `Color`、`LineWidth`、`VerticalHeight`、`MarginBlock`、`TextMarginBlock`、`TextPaddingInline`、`TextColor` 和 `PlainTextColor`。
- `ConfigProvider.Defaults("Divider", ...)` 支持 `variant`、`orientation`、`titlePlacement`、`size`，以及 `dashed`、`plain`、`vertical`（`bool`）。`Orientation("left")` 等旧标题位置写法仍可用。

## 注意事项

- **状态放在哪**：Divider 没有业务状态；标题和变体由当前帧参数决定，动态标题应来自页面状态而不是组件内部状态。
- **语义**：水平 Divider 暴露为水平分隔线，`VDivider` 暴露为垂直分隔线；不要把它当作可点击控件。
- **布局**：Divider 自带主题的上下间距，嵌入紧凑工具栏时可以继续使用返回的 `Box` 设置布局；不要用多个空 Divider 代替间距系统。
- **RTL**：`start` / `end` 会随阅读方向镜像，`left` / `right` 始终是物理左、右。

## 示例代码

<!--@include: ./_gen/divider-examples.md-->

## API

<!--@include: ./_gen/divider-api.md-->

## 相关链接

- 源码：[ui/display.go](https://github.com/tangoui/tango/blob/main/ui/display.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Space 间距](/components/space)、[Layout 布局](/components/layout)、[Text 排版](/components/text)
