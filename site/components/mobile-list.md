---
title: Mobile List 列表
---
# Mobile List 列表

用分组容器和 Cell 行展示移动端设置、菜单、摘要与可操作列表。

## 何时使用

- 需要带标题的分组行、描述、尾部值、箭头或开关时使用 `List` 与 `Cell`。
- 需要滑动操作时使用 `SwipeCell` 包裹 Cell。
- 复杂表格、列排序或桌面密集数据应使用桌面端 `Table`，不要把 List 当表格。
- 超长列表把 List 放进 `ScrollView`，固定高度行可使用 `Rows`。

## 在线体验

真实 gallery 展示分组列表、卡片列表、开关和 SwipeCell 操作。

<GalleryPreview component="mobile.List" />

## 基本用法

List 通过 `Body` 包含多个 Cell；Cell 的状态指针应由页面持有。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/mobile"
)

var wifi = true

func page(c *ui.Context) {
    mobile.List(c, "网络").Body(func() {
        mobile.Cell(c, "无线局域网").Switch(&wifi)
        mobile.Cell(c, "蜂窝网络").Extra("已开启").Arrow()
    })
    _ = ui.ID(0)
}
```

## 常见用法

### 描述和尾部值

`Desc` 展示补充信息，`Extra` 展示右侧值，`Arrow` 表示可以进入详情。

```go
func summary(c *ui.Context) {
    mobile.List(c, "账号").Body(func() {
        mobile.Cell(c, "个人资料").Desc("头像、昵称、简介").Arrow()
        mobile.Cell(c, "通知").Extra("3 条未读").Arrow()
    })
}
```

### 可点击行

调用 `Clickable` 后读取 `Clicked`；`Changed` 用于开关等值变化。

```go
func actions(c *ui.Context) {
    cell := mobile.Cell(c, "清理缓存").Clickable()
    if cell.Clicked() {
        ui.Text(c, "正在清理")
    }
}
```

### 开关和禁用

`Switch` 绑定 bool 指针，`Disabled(true)` 禁止交互但仍保留说明。

```go
var enabled bool

func switches(c *ui.Context) {
    mobile.List(c, "功能").Body(func() {
        mobile.Cell(c, "通知").Switch(&enabled)
        mobile.Cell(c, "实验功能").Disabled(true).Switch(&enabled)
    })
}
```

### 卡片模式

`Card` 给分组增加独立卡片外观，适合页面中多个互相独立的设置组。

```go
func cards(c *ui.Context) {
    mobile.List(c, "账单").Card().Body(func() {
        mobile.Cell(c, "本月支出").Extra("¥ 128.00")
        mobile.Cell(c, "发票").Arrow()
    })
}
```

### 滑动操作

`SwipeCell` 的操作项使用 `SwipeAction`，正文仍用 `Body` 构建。

```go
func swipe(c *ui.Context) {
    mobile.SwipeCell(c).
        Right(mobile.SwipeAction{Text: "置顶", Color: "primary"}, mobile.SwipeAction{Text: "删除", Color: "danger"}).
        Body(func() { mobile.Cell(c, "向左滑动这一行") })
}
```

## 主题与样式

List、Cell 和 SwipeCell 使用 Provider 的移动令牌；Card、分隔线和操作色会随 Ant Mobile、Vant、shadcn 改变。`ConfigProvider.Defaults("mobile.List", ...)` 和 `mobile.Cell` 的组件默认键可用于统一页面设置，具体键以 API 注释为准。

## 注意事项

列表中的开关、输入和选择状态放在稳定的页面变量或闭包中；动态行必须用 `ui.Key` 保持身份。Cell 的点击和长按是逐帧状态，需在当前帧处理。操作文字应明确，危险操作使用 `Color: "danger"` 并提供确认流程。RTL 下尾部槽位和箭头会自动处理。

## 示例代码

<!--@include: ./_gen/mobile-list-examples.md-->

## API

<!--@include: ./_gen/mobile-list-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/list.go)
- [ScrollView 滚动视图](/components/mobile-scrollview)
- [Button 按钮](/components/mobile-button)
- [ui mobile Go API](/reference/api/ui-mobile)
