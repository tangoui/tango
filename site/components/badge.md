---
title: Badge 徽章
---
# Badge 徽章

在对象的右上角（RTL 下为左上角）显示数量、状态点或简短提醒。

## 何时使用

- 头像、图标或列表项上叠加未读数、数量和提醒。
- 用状态点表达在线、成功等状态时使用 `Status`。
- 需要完整提示内容时改用 [Alert](/components/alert) 或 [Notification](/components/notification)。

## 在线体验

<GalleryPreview component="Badge" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Badge(c).Count(5).Body(func() { ui.Avatar(c, "U") })
}
```

`Body` 把徽章叠加到内容的顶部末端。省略 `Body` 时显示独立徽标。

## 常见用法

### 超出上限和零值

```go
func counts(c *ui.Context, unread int) {
    ui.Badge(c).Count(unread).Max(99).ShowZero().Body(func() { ui.Text(c, "消息") })
}
```

### 状态点

```go
func state(c *ui.Context) {
    ui.Badge(c).Status("success", "在线")
    ui.Badge(c).Dot().Color("warning").Body(func() { ui.Text(c, "待处理") })
}
```

### 偏移与徽标尺寸

```go
func compact(c *ui.Context) {
    ui.Badge(c).Count(2).Small().Offset(4, -4).Body(func() { ui.Text(c, "任务") })
}
```

## 主题与样式

`Badge` 的令牌控制颜色、气泡高度和状态间距。全局默认键为 `size`、`overflowCount`、`showZero`、`color`、`dot`。计数默认上限为 99，超出显示为 `99+`。主题会处理 RTL 下的角落位置。

## 注意事项

数量应来自页面状态并在即时模式每帧重建；`Max` 只影响显示，不改变实际数量。徽标文本应短小，动态状态建议同时提供可读文本而不是只显示颜色。

## 示例代码

<!--@include: ./_gen/badge-examples.md-->

## API

<!--@include: ./_gen/badge-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_badge.go)
- [Avatar 头像](/components/avatar)
